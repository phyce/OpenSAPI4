// Package inputstate maps the input side's state to the speaking thread's state the TruVoice golden data records,
// and back, for the tests.
package inputstate

import (
	"fmt"
	"sync"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/input"
	"github.com/phyce/OpenSAPI4/truvoice/internal/threadstate"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
	"github.com/phyce/OpenSAPI4/truvoice/synthesizer"
)

// The places of the command ring and the text being read in the thread state. The escape parser's own ring,
// 0x8070 to 0x8177, holds no character between two calls of the parser: each is parsed as it comes.
const (
	Commands    = 0x8178
	CommandsEnd = 0x9178
	Text        = 0x2138
)

// ParserFields lists the escape parser's values, and the pipeline's it sets, with their places in the thread
// state: its settings and its own state.
func ParserFields(stage *input.Stage) []threadstate.Field {
	at := threadstate.At
	settings := &stage.Pipeline.Settings
	fields := []threadstate.Field{at(0x1a00, "phonetic input setting", &settings.PhoneticInput),
		at(0x1a04, "phrase setting", &settings.Phrase), at(0x1a08, "rate setting", &settings.Rate),
		at(0x1a0c, "speed setting", &stage.Pipeline.SettingsSpeed), at(0x1a10, "pitch setting", &settings.Pitch),
		at(0x1a14, "attenuation setting", &settings.Attenuation), at(0x1a18, "filter setting", &settings.Filter),
		at(0x1a1c, "flags setting", &settings.Flags), at(0x1a20, "input flags setting", &settings.InputFlags),
		at(0x1a24, "voice setting", &settings.Voice), at(0x1af0, "parsing", &stage.Parsing),
		at(0x1af5, "parameter", &stage.Parameter), at(0x1af6, "overflow", &stage.Overflow),
		at(0x917c, "written", &stage.Written), threadstate.FlagAt(0x19ac, "hold", &stage.Pipeline.Synthesizer.Hold, 1)}
	for track := range datapack.TrackCount {
		fields = append(fields, at(0x1a28+track, fmt.Sprintf("level setting %d", track), &stage.Pipeline.SettingsLevels[track]))
	}
	for index := range stage.Parameters {
		fields = append(fields, at(0x1af8+4*index, fmt.Sprintf("parameter %d", index), &stage.Parameters[index]))
	}
	return fields
}

// Fields lists the input side's values with their places in the thread state.
func Fields(stage *input.Stage) []threadstate.Field {
	at := threadstate.At
	return append(ParserFields(stage), at(0x1ae8, "flags", &stage.Flags), at(0x1aec, "input flags", &stage.InputFlags),
		at(0x9178, "read", &stage.Read), at(0x9180, "pending", &stage.Pending))
}

// LoadParser gives the escape parser, and the pipeline's settings, the thread state's values, with the command
// ring's characters and the text being read.
func LoadParser(mapping threadstate.Mapping, stage *input.Stage, state []byte) {
	mapping.LoadFields(ParserFields(stage), state)
	loadRing(mapping, stage, state)
}

// Load gives the input side, and the pipeline's settings, the thread state's values.
func Load(mapping threadstate.Mapping, stage *input.Stage, state []byte) {
	mapping.LoadFields(Fields(stage), state)
	loadRing(mapping, stage, state)
}

func loadRing(mapping threadstate.Mapping, stage *input.Stage, state []byte) {
	copy(stage.Commands[:], state[Commands:CommandsEnd])
	stage.Text = nil
	if pointer := threadstate.Dword(state, Text); pointer != 0 && mapping.Text != nil {
		stage.Text = mapping.Text(pointer)
	} else if pointer != 0 {
		stage.Text = &pipeline.Text{}
	}
}

// Store writes the input side's values, and the pipeline's settings, into the thread state. The text being read
// is the speaking thread's, which the input side doesn't write.
func Store(mapping threadstate.Mapping, stage *input.Stage, state []byte) {
	mapping.StoreFields(Fields(stage), state)
	copy(state[Commands:CommandsEnd], stage.Commands[:])
}

// Names names the thread state's bytes the input side's and its pipeline's values cover.
var Names = sync.OnceValue(func() map[int]string {
	stage := input.New(nil, pipeline.New(nil, new(synthesizer.Synthesizer), nil, nil))
	names := threadstate.Names(Fields(stage), threadstate.PipelineFields(stage.Pipeline))
	for offset := Commands; offset < CommandsEnd; offset++ {
		names[offset] = fmt.Sprintf("command ring %d", offset-Commands)
	}
	return names
})

// Describe names a byte of the thread state, for the reports.
func Describe(offset int) string {
	return threadstate.Describe(offset, Names())
}
