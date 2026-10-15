// Package input is the TruVoice engine's input side (docs/truvoice/input.md): the escape parser, which reads the
// text's characters and commands into the command ring, and the input stage, which makes the pipeline's nodes of
// them a word at a time.
package input

import (
	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
)

const (
	commandRingSize = 4096
	parameterCount  = 16
	nothing         = -1
)

// What the escape parser reads (input.md §1).
const (
	parsingText = 1 + iota
	parsingEscape
	parsingParameters
)

// The N and A flags as the escape parser's settings and the input stage keep them at a reset.
const (
	defaultFlags      = 0x17c0
	defaultInputFlags = 0x41
)

// A Stage is the input side's state.
type Stage struct {
	Pipeline *pipeline.Pipeline

	// The escape parser (input.md §1): what it reads, the parameter its digits go to, whether a parameter has
	// gone past 255, and the parameters.
	Parsing    int32
	Parameter  byte
	Overflow   bool
	Parameters [parameterCount]int32

	// The command ring (input.md §2.1): the characters and commands the escape parser writes and the input stage
	// reads, from Read up to Written. Pending is the character the input stage read and hasn't made a node of
	// yet, or -1.
	Commands      [commandRingSize]byte
	Read, Written int32
	Pending       int32

	// Flags and InputFlags are the N and A flags as the input stage last made their commands' nodes. Text is the
	// text the speaking thread is reading, which the nodes of its bookmarks and words carry.
	Flags, InputFlags uint32
	Text              *pipeline.Text

	// The input ring (input.md §4.1): the text the speaking thread feeds, which the tokenizer reads from
	// InputRead up to InputWritten. Drained is set when the tokenizer has found it empty, AllFed while the text
	// being fed is all in it.
	Input                   [inputRingSize]byte
	InputRead, InputWritten int32
	Drained, AllFed         bool

	// The feeder (input.md §5): how far it has fed the text, and whether the last line it fed ended with a line
	// end. Busy is set while the speaking thread has pumped text its steps haven't finished (input.md §6).
	Position     int32
	AfterLineEnd bool
	Busy         bool

	feeding   []byte
	tokenizer tokenizer
	output    []byte
	tables    *datapack.Input
}

// New returns the input side as the speaking thread constructs it: as if the last text were all fed, so that the
// thread takes a text first.
func New(tables *datapack.Input, pipeline *pipeline.Pipeline) *Stage {
	return &Stage{Pipeline: pipeline, tables: tables, AllFed: true}
}

// Reset empties the input ring and the command ring, and starts the escape parser on text (input.md §3).
func (stage *Stage) Reset() {
	stage.InputRead, stage.InputWritten = 0, 0
	stage.Drained = true
	stage.Parsing = parsingText
	stage.Read, stage.Written = 0, 0
	stage.Pending = nothing
	stage.Flags, stage.InputFlags = defaultFlags, defaultInputFlags
}

// room is the room the command ring has left: 4,095 bytes less those it holds.
func (stage *Stage) room() int32 {
	return ringRoom(stage.Read, stage.Written)
}

// ringRoom is the room a ring of 4,096 bytes has left, from where it is read and written: 4,095 bytes less those
// it holds.
func ringRoom(read, written int32) int32 {
	room := read - written - 1
	if room < 0 {
		room += commandRingSize
	}
	return room
}

// put writes a byte into the command ring, unless it has no room.
func (stage *Stage) put(value byte) {
	if stage.room() < 1 {
		return
	}
	stage.Commands[stage.Written] = value
	stage.Written = (stage.Written + 1) % commandRingSize
}

// take reads the next byte of the command ring, or -1 when it has none.
func (stage *Stage) take() int32 {
	if stage.Read == stage.Written {
		return nothing
	}
	value := stage.Commands[stage.Read]
	stage.Read = (stage.Read + 1) % commandRingSize
	return int32(value)
}
