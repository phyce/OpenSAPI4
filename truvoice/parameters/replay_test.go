package parameters

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/internal/conformance"
	"github.com/phyce/OpenSAPI4/truvoice/synthesizer"
)

// The parameter generator's golden data (golden/truvoice/parameters.jsonl.gz) records calls of the original's
// functions with the changes of the speaking thread's state around them. replay rebuilds the state at every
// call and return, so that a Go function can start from the state the original's started from and leave what
// it left.

const stateSize = 0x9184

// The thread state's fields the ported part of the parameter generator keeps.
const (
	statePosition  = 0x4
	stateEnd       = 0x8
	stateWritten   = 0xc
	stateFurthest  = 0x10
	stateMarkers   = 0x14
	stateNext      = 0x3c
	stateEarliest  = 0x94
	stateSegments  = 0x144
	segmentSize    = 0x1c
	stateFrames    = 0x3ac
	stateLast      = 0x1e74
	stateTones     = 0x1fbd
	stateLookahead = 0x1fe0
	stateMargin    = 0x1fe4
)

type diffRun struct {
	offset int
	data   []byte
}

func (run *diffRun) UnmarshalJSON(data []byte) error {
	var pair [2]json.RawMessage
	if err := json.Unmarshal(data, &pair); err != nil {
		return err
	}
	var text string
	if err := json.Unmarshal(pair[0], &run.offset); err != nil {
		return err
	}
	if err := json.Unmarshal(pair[1], &text); err != nil {
		return err
	}
	var err error
	run.data, err = hex.DecodeString(text)
	return err
}

type callRecord struct {
	Kind      string    `json:"kind"`
	Function  string    `json:"function"`
	Site      string    `json:"site"`
	ECX       uint32    `json:"ecx"`
	EDX       uint32    `json:"edx"`
	Arguments []uint32  `json:"arguments"`
	EAX       uint32    `json:"eax"`
	Base      uint32    `json:"base"`
	Diff      []diffRun `json:"diff"`
}

type goldenCase struct {
	Name    string       `json:"name"`
	Records []callRecord `json:"records"`
}

// A call is a recorded call of one of the original's functions, with its return and the thread state at both.
type call struct {
	record, result *callRecord
	before, after  []byte
	base           uint32
}

func (call *call) argument(index int) int32 {
	return int32(call.record.Arguments[index])
}

// ring returns the track whose ring an argument points to.
func (call *call) ring(index int) (int, error) {
	offset := int64(call.record.Arguments[index]) - int64(call.base) - stateFrames
	if offset < 0 || offset%synthesizer.RingLength != 0 || offset/synthesizer.RingLength >= datapack.TrackCount {
		return 0, fmt.Errorf("argument %d isn't a ring: %08x", index, call.record.Arguments[index])
	}
	return int(offset / synthesizer.RingLength), nil
}

type replayer func(call *call) error

// replay hands each recorded call of the functions given to their replayer, reports the calls that differ (a
// few per function) and returns how many calls each replayed. The cases are replayed in parallel.
func replay(t *testing.T, replayers map[string]replayer) map[string]int {
	t.Helper()
	var lock sync.Mutex
	counts := map[string]int{}
	failures := map[string]int{}
	cases := make(chan goldenCase)
	var workers sync.WaitGroup
	for range runtime.GOMAXPROCS(0) {
		workers.Go(func() {
			for golden := range cases {
				caseCounts, caseFailures, messages := replayCase(golden, replayers)
				lock.Lock()
				for function, count := range caseCounts {
					counts[function] += count
				}
				for function, count := range caseFailures {
					for _, message := range messages[function] {
						if failures[function] < 5 {
							t.Error(message)
						}
						failures[function]++
					}
					failures[function] += count - len(messages[function])
				}
				lock.Unlock()
			}
		})
	}
	conformance.EachLine(t, func(golden goldenCase) { cases <- golden }, "parameters.jsonl.gz")
	close(cases)
	workers.Wait()
	for function, count := range failures {
		t.Errorf("%s: %d of %d calls differ", function, count, counts[function])
	}
	return counts
}

func replayCase(golden goldenCase, replayers map[string]replayer) (map[string]int, map[string]int, map[string][]string) {
	counts := map[string]int{}
	failures := map[string]int{}
	messages := map[string][]string{}
	state := make([]byte, stateSize)
	var base uint32
	type open struct {
		record *callRecord
		before []byte
	}
	var stack []open
	for index := range golden.Records {
		record := &golden.Records[index]
		if record.Kind != "call" && record.Kind != "return" {
			continue
		}
		if record.Base != 0 {
			base = record.Base
			clear(state)
		}
		for _, run := range record.Diff {
			copy(state[run.offset:], run.data)
		}
		if record.Kind == "call" {
			entry := open{record: record}
			if replayers[record.Function] != nil {
				entry.before = slices.Clone(state)
			}
			stack = append(stack, entry)
			continue
		}
		if len(stack) == 0 || stack[len(stack)-1].record.Function != record.Function {
			messages["replay"] = append(messages["replay"], fmt.Sprintf("%s: record %d returns from %s, which isn't the call open", golden.Name, index, record.Function))
			failures["replay"]++
			return counts, failures, messages
		}
		entry := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		replayer := replayers[record.Function]
		if replayer == nil {
			continue
		}
		counts[record.Function]++
		if err := replayer(&call{record: entry.record, result: record, before: entry.before, after: state, base: base}); err != nil {
			failures[record.Function]++
			if len(messages[record.Function]) < 5 {
				messages[record.Function] = append(messages[record.Function], fmt.Sprintf("%s: %s called at %s (record %d): %v", golden.Name, record.Function, entry.record.Site, index, err))
			}
		}
	}
	return counts, failures, messages
}

type dwordField struct {
	offset int
	value  *int32
}

func generatorFields(generator *Generator) []dwordField {
	ring := generator.Synthesizer
	fields := []dwordField{{statePosition, &ring.Position}, {stateEnd, &ring.End}, {stateWritten, &ring.Written},
		{stateFurthest, &generator.Furthest}, {stateLookahead, &ring.Lookahead}, {stateMargin, &generator.Margin}}
	for track := range generator.Tracks {
		fields = append(fields, dwordField{stateNext + 4*track, &generator.Tracks[track].Next},
			dwordField{stateEarliest + 4*track, &generator.Tracks[track].Earliest},
			dwordField{stateLast + 4*track, &generator.Last[track]})
		segment := &generator.Segments[track]
		for index, value := range []*int32{&segment.Mode, &segment.Back, &segment.Forward, &segment.Frames,
			&segment.BackTarget, &segment.Onset, &segment.Target} {
			fields = append(fields, dwordField{stateSegments + segmentSize*track + 4*index, value})
		}
	}
	return fields
}

func bit(state []byte, offset, slot int) bool {
	return state[offset+slot>>3]&(1<<(slot&7)) != 0
}

func setBit(state []byte, offset, slot int, value bool) {
	if value {
		state[offset+slot>>3] |= 1 << (slot & 7)
	} else {
		state[offset+slot>>3] &^= 1 << (slot & 7)
	}
}

// load returns a generator with the thread state's values.
func load(tables *datapack.Parameters, state []byte) *Generator {
	generator := New(tables, new(synthesizer.Synthesizer))
	for _, field := range generatorFields(generator) {
		*field.value = int32(binary.LittleEndian.Uint32(state[field.offset:]))
	}
	ring := generator.Synthesizer
	for slot := range synthesizer.RingLength {
		for track := range datapack.TrackCount {
			ring.Frames[slot][track] = state[stateFrames+synthesizer.RingLength*track+slot]
		}
		ring.Markers[slot] = bit(state, stateMarkers, slot)
		ring.Tones[slot] = bit(state, stateTones, slot)
	}
	return generator
}

// store writes the generator's values into the thread state.
func store(generator *Generator, state []byte) {
	for _, field := range generatorFields(generator) {
		binary.LittleEndian.PutUint32(state[field.offset:], uint32(*field.value))
	}
	ring := generator.Synthesizer
	for slot := range synthesizer.RingLength {
		for track := range datapack.TrackCount {
			state[stateFrames+synthesizer.RingLength*track+slot] = ring.Frames[slot][track]
		}
		setBit(state, stateMarkers, slot, ring.Markers[slot])
		setBit(state, stateTones, slot, ring.Tones[slot])
	}
}

// describe names a byte of the thread state, for the reports.
func describe(offset int) string {
	switch {
	case offset >= stateFrames && offset < stateFrames+datapack.TrackCount*synthesizer.RingLength:
		return fmt.Sprintf("track %d frame %d", (offset-stateFrames)/synthesizer.RingLength, (offset-stateFrames)%synthesizer.RingLength)
	case offset >= stateSegments && offset < stateSegments+datapack.TrackCount*segmentSize:
		field := []string{"mode", "back", "forward", "frames", "back target", "onset", "target"}[(offset-stateSegments)%segmentSize/4]
		return fmt.Sprintf("track %d segment %s", (offset-stateSegments)/segmentSize, field)
	case offset >= stateNext && offset < stateNext+4*datapack.TrackCount:
		return fmt.Sprintf("track %d next", (offset-stateNext)/4)
	case offset >= stateEarliest && offset < stateEarliest+4*datapack.TrackCount:
		return fmt.Sprintf("track %d earliest", (offset-stateEarliest)/4)
	case offset >= stateLast && offset < stateLast+4*datapack.TrackCount:
		return fmt.Sprintf("track %d last", (offset-stateLast)/4)
	}
	return fmt.Sprintf("%04x", offset)
}

// compare reports where the state the generator leaves differs from the original's.
func compare(generator *Generator, call *call) error {
	state := slices.Clone(call.before)
	store(generator, state)
	var differences []string
	for offset := range state {
		if state[offset] != call.after[offset] {
			if len(differences) == 8 {
				differences = append(differences, "...")
				break
			}
			differences = append(differences, fmt.Sprintf("%s is %d, expected %d", describe(offset), state[offset], call.after[offset]))
		}
	}
	if differences != nil {
		return fmt.Errorf("%s", strings.Join(differences, "; "))
	}
	return nil
}

func loadParameters(t *testing.T) *datapack.Parameters {
	t.Helper()
	tables, err := conformance.Pack(t).LoadParameters()
	if err != nil {
		t.Fatal(err)
	}
	return tables
}
