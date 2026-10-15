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

	tables *datapack.Input
}

func New(tables *datapack.Input, pipeline *pipeline.Pipeline) *Stage {
	return &Stage{Pipeline: pipeline, tables: tables}
}

// Reset empties the command ring and starts the escape parser on text (input.md §3).
func (stage *Stage) Reset() {
	stage.Parsing = parsingText
	stage.Read, stage.Written = 0, 0
	stage.Pending = nothing
	stage.Flags, stage.InputFlags = defaultFlags, defaultInputFlags
}

// room is the room the command ring has left, as the original counts it: one less when Written is past Read.
func (stage *Stage) room() int32 {
	room := stage.Read - stage.Written - 1
	if room < 0 {
		room += commandRingSize - 1
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
