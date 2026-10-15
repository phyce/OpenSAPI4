// Package normalizerstate maps the normalizer's state to the speaking thread's state the TruVoice golden data
// records, and back, for the tests.
package normalizerstate

import (
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/internal/threadstate"
	"github.com/phyce/OpenSAPI4/truvoice/normalizer"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
	"github.com/phyce/OpenSAPI4/truvoice/synthesizer"
)

// The places of the normalizer's pointers in the thread state: its place in the program, the replacement it
// inserts, the calls and how far they go, and the pending node and the pointer to it. The pending node's links and
// text are never written.
const (
	Counter        = 0x1b38
	Replacement    = 0x1b40
	Calls          = 0x1b48
	CallSize       = 8
	Depth          = 0x1be8
	Pending        = 0x1bf8
	PendingData    = Pending + threadstate.NodeData
	PendingPointer = 0x1c18
)

// Fields lists the normalizer's values but its pointers, and the hold flag its phonetic input sets, with their
// places in the thread state.
func Fields(stage *normalizer.Stage) []threadstate.Field {
	at := threadstate.At
	fields := []threadstate.Field{at(0x1b3c, "testing", &stage.Testing), at(0x1b3d, "yielded", &stage.Yielded),
		at(0x1b44, "remaining", &stage.Remaining), at(0x1b45, "skip", &stage.Skip),
		threadstate.FlagAt(0x19ac, "hold", &stage.Pipeline.Synthesizer.Hold, 1)}
	for index := range stage.Registers {
		fields = append(fields, at(0x1bec+index, fmt.Sprintf("register %d", index), &stage.Registers[index]))
	}
	for index := range stage.Calls {
		call := &stage.Calls[index]
		fields = append(fields, at(Calls+CallSize*index+4, fmt.Sprintf("call %d remaining", index), &call.Remaining),
			at(Calls+CallSize*index+5, fmt.Sprintf("call %d skip", index), &call.Skip))
	}
	return fields
}

// Pointers maps the normalizer's places in the program and its list entries to their pointers in a run's thread
// states: the original's program and lists are at addresses that vary between runs. The program's address is
// learned from the first state loaded, which starts the program, or the first stored; an entry's from the first
// state stored with it.
type Pointers struct {
	program uint32
	entries map[*datapack.ListEntry]uint32
	pointed map[uint32]*datapack.ListEntry
	unknown *datapack.ListEntry
	loaded  uint32
}

func NewPointers() *Pointers {
	return &Pointers{entries: map[*datapack.ListEntry]uint32{}, pointed: map[uint32]*datapack.ListEntry{},
		unknown: new(datapack.ListEntry)}
}

// Load gives the stage the thread state's values.
func (pointers *Pointers) Load(mapping threadstate.Mapping, stage *normalizer.Stage, state []byte) error {
	mapping.LoadFields(Fields(stage), state)
	copy(stage.Pending.Data[:], state[PendingData:])
	depth := threadstate.Dword(state, Depth) - mapping.Base - Calls
	if depth%CallSize != 0 || depth/CallSize > normalizer.CallDepth {
		return fmt.Errorf("the calls go to %#x", threadstate.Dword(state, Depth))
	}
	stage.Depth = int32(depth / CallSize)
	counter := threadstate.Dword(state, Counter)
	if pointers.program == 0 {
		if !stage.Testing || stage.Depth != 0 {
			return fmt.Errorf("the first state loaded doesn't start the program")
		}
		pointers.program = counter
	}
	stage.Counter = int32(counter - pointers.program)
	for index := range stage.Calls {
		stage.Calls[index].Counter = 0
		if pointer := threadstate.Dword(state, Calls+CallSize*index); pointer != 0 {
			stage.Calls[index].Counter = int32(pointer - pointers.program)
		}
	}
	pointer := threadstate.Dword(state, Replacement)
	pointers.loaded = pointer
	stage.Entry, stage.Inserted = nil, false
	switch {
	case pointer == 0:
	case pointers.pointed[pointer] != nil:
		stage.Entry = pointers.pointed[pointer]
	default:
		stage.Entry = pointers.unknown
		for entry, address := range pointers.entries {
			if address+uint32(len(entry.Replacement)) == pointer {
				stage.Entry, stage.Inserted = entry, true
			}
		}
	}
	return nil
}

// Store writes the stage's values into the thread state, learning the addresses it doesn't know from the
// expected state.
func (pointers *Pointers) Store(mapping threadstate.Mapping, stage *normalizer.Stage, state, expected []byte) {
	mapping.StoreFields(Fields(stage), state)
	copy(state[PendingData:PendingData+len(stage.Pending.Data)], stage.Pending.Data[:])
	binary.LittleEndian.PutUint32(state[PendingPointer:], mapping.Base+Pending)
	binary.LittleEndian.PutUint32(state[Depth:], mapping.Base+Calls+CallSize*uint32(stage.Depth))
	if pointers.program == 0 {
		pointers.program = threadstate.Dword(expected, Counter) - uint32(stage.Counter)
	}
	binary.LittleEndian.PutUint32(state[Counter:], pointers.program+uint32(stage.Counter))
	for index, call := range stage.Calls {
		pointer := uint32(0)
		if call.Counter != 0 {
			pointer = pointers.program + uint32(call.Counter)
		}
		binary.LittleEndian.PutUint32(state[Calls+CallSize*index:], pointer)
	}
	var pointer uint32
	switch entry := stage.Entry; entry {
	case nil:
	case pointers.unknown:
		pointer = pointers.loaded
	default:
		length := uint32(0)
		if stage.Inserted {
			length = uint32(len(entry.Replacement))
		}
		address, known := pointers.entries[entry]
		if !known {
			address = threadstate.Dword(expected, Replacement) - length
			if other := pointers.pointed[address]; address+length == 0 || other != nil && *other != *entry {
				address = ^address
			} else {
				pointers.entries[entry], pointers.pointed[address] = address, entry
			}
		}
		pointer = address + length
	}
	binary.LittleEndian.PutUint32(state[Replacement:], pointer)
}

// Names names the thread state's bytes the normalizer's values cover.
var Names = sync.OnceValue(func() map[int]string {
	stage := normalizer.New(nil, nil, pipeline.New(nil, new(synthesizer.Synthesizer), nil, nil))
	names := threadstate.Names(Fields(stage), threadstate.PipelineFields(stage.Pipeline))
	for offset := range 4 {
		names[Counter+offset] = "counter"
		names[Replacement+offset] = "replacement"
		names[Depth+offset] = "depth"
		names[PendingPointer+offset] = "pending pointer"
		for index := range normalizer.CallDepth {
			names[Calls+CallSize*index+offset] = fmt.Sprintf("call %d", index)
		}
	}
	for offset := range len(pipeline.Node{}.Data) {
		names[PendingData+offset] = fmt.Sprintf("pending byte %#x", threadstate.NodeData+offset)
	}
	return names
})

// Describe names a byte of the thread state, for the reports.
func Describe(offset int) string {
	return threadstate.Describe(offset, Names())
}
