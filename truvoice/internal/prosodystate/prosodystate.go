// Package prosodystate maps the prosody stage's state to the speaking thread's state and the global variables the
// TruVoice golden data records, and back, for the tests.
package prosodystate

import (
	"fmt"
	"slices"
	"sync"

	"github.com/phyce/OpenSAPI4/truvoice/internal/threadstate"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
	"github.com/phyce/OpenSAPI4/truvoice/prosody"
	"github.com/phyce/OpenSAPI4/truvoice/synthesizer"
)

// Fields lists the stage's values with their places in the thread state, and the synthesizer's the walk reads.
func Fields(stage *prosody.Stage) []threadstate.Field {
	at, flag := threadstate.At, threadstate.FlagAt
	context := &stage.Context
	ring := stage.Pipeline.Synthesizer
	fields := []threadstate.Field{at(0x1d60, "phonemes", &stage.Phonemes), at(0x1d74, "contour", &stage.Contour),
		at(0x1d8c, "position", &stage.Position), flag(0x1d90, "rise", &stage.Rise, 1), at(0x1d94, "fall", &stage.Fall),
		flag(0x1d98, "word end", &stage.WordEnd, 1), flag(0x1d99, "phrase end", &stage.PhraseEnd, 1),
		flag(0x1d9a, "at nucleus", &stage.AtNucleus, 1), flag(0x1d9b, "after stress", &stage.AfterStress, 1),
		at(0x1d9c, "nucleus", &stage.Nucleus), at(0x1da0, "following consonant", &stage.FollowingConsonant),
		flag(0x1da4, "raised", &stage.Raised, 1), flag(0x1dd4, "lowered", &stage.Lowered, 1),
		flag(0x1da5, "affricate", &context.Affricate, 0x08), flag(0x1da6, "fricative", &context.Fricative, 0x40),
		flag(0x1da7, "nasal", &context.Nasal, 0x10), flag(0x1da8, "phoneme", &context.Phoneme, 0x80),
		flag(0x1da9, "sonorant", &context.Sonorant, 0x02), flag(0x1daa, "closure", &context.Closure, 0x01),
		flag(0x1dab, "stressed", &context.Stressed, 1), flag(0x1dac, "syllabic", &context.Syllabic, 0x01),
		flag(0x1dad, "vowel", &context.Vowel, 0x02), flag(0x1dae, "preceding syllabic", &context.PrecedingSyllabic, 1),
		flag(0x1daf, "next stressed", &context.NextStressed, 1), flag(0x1db0, "following syllabic", &context.FollowingSyllabic, 1),
		at(0x1db1, "symbol", &context.Symbol), at(0x1db4, "next", &context.Next), at(0x1db8, "following", &context.Following),
		at(0x1dbc, "after following", &context.AfterFollowing), at(0x1dc0, "previous", &context.Previous),
		at(0x1dc4, "preceding", &context.Preceding), at(0x1dc8, "before preceding", &context.BeforePreceding),
		at(0x1dcc, "following symbol", &context.FollowingSymbol), at(0x1dcd, "after following symbol", &context.AfterFollowingSymbol),
		at(0x1dce, "next symbol", &context.NextSymbol), at(0x1dd0, "previous symbol", &context.PreviousSymbol),
		at(0x1dd2, "preceding symbol", &context.PrecedingSymbol), at(0x1dd3, "before preceding symbol", &context.BeforePrecedingSymbol),
		at(0x1c74, "words", &stage.Words), at(0x1c78, "sentence words", &stage.SentenceWords),
		flag(0x1d44, "slow speech", &stage.SlowSpeech, 1), flag(0x1d45, "first phrase", &stage.FirstPhrase, 1),
		at(0x1d48, "presets", &stage.Presets), at(0x1d4c, "previous presets", &stage.PreviousPresets),
		at(0x1d50, "preset duration", &stage.PresetDuration),
		at(0x1d58, "countdown", &stage.Countdown), at(0x1d5c, "phrase limit", &stage.PhraseLimit),
		at(0x1d64, "sentence phonemes", &stage.SentencePhonemes), at(0x1d68, "padding", &stage.Padding),
		at(0x1d6c, "scan result", &stage.ScanResult), flag(0x1d70, "no phrasing", &stage.NoPhrasing, 1),
		at(0x1d88, "since mark", &stage.SinceMark), at(0x1dd8, "inherent", &stage.Inherent),
		at(0x1ddc, "shortest", &stage.Shortest), at(0x1e1c, "emphasis", &stage.Emphasis),
		flag(0x1e20, "geminate", &stage.Geminate, 1),
		at(0x4, "synthesizer position", &ring.Position), at(0xc, "written", &ring.Written),
		at(0x1fe0, "lookahead", &ring.Lookahead)}
	for index, name := range []string{"first mark", "second mark", "third mark", "fourth mark"} {
		fields = append(fields, at(0x1d78+4*index, name, &stage.Marks[index]))
	}
	for index := range stage.Percents {
		fields = append(fields, at(0x1de0+4*index, fmt.Sprintf("percent %d", index), &stage.Percents[index]))
	}
	for index := range stage.WordMarks {
		fields = append(fields, at(0x1c7c+4*index, fmt.Sprintf("word mark %d", index), &stage.WordMarks[index]))
	}
	return fields
}

// GlobalsSize is the size of the global variables recorded, from 10128830.
const GlobalsSize = 0x38

// SharedFields lists the shared state's values with their places in the global variables.
func SharedFields(shared *prosody.Shared) []threadstate.Field {
	at, flag := threadstate.At, threadstate.FlagAt
	return []threadstate.Field{at(0x00, "scanned", &shared.Scanned), flag(0x04, "in phrase", &shared.InPhrase, 1),
		at(0x08, "last node", &shared.LastNode), at(0x0c, "preset symbol", &shared.PresetSymbol),
		at(0x10, "pending", &shared.Pending), at(0x14, "walk", &shared.Walk), flag(0x18, "held", &shared.Held, 1),
		at(0x1c, "preset pitch", &shared.PresetPitch), at(0x20, "saved result", &shared.SavedResult),
		at(0x24, "preset node", &shared.PresetNode), at(0x28, "counted", &shared.Counted),
		at(0x2c, "slow pauses", &shared.SlowPauses)}
}

// Padded returns a copy of the global variables recorded, with zeros up to GlobalsSize.
func Padded(globals []byte) []byte {
	return append(slices.Clone(globals), make([]byte, max(GlobalsSize-len(globals), 0))...)
}

// Load gives the stage the thread state's values, but its pipeline's, and its shared state the global variables'.
func Load(mapping threadstate.Mapping, stage *prosody.Stage, state, globals []byte) {
	mapping.LoadFields(Fields(stage), state)
	mapping.LoadFields(SharedFields(stage.Shared), Padded(globals))
}

// Store writes the stage's values, but its pipeline's, into the thread state, and returns the global variables
// with the shared state's.
func Store(mapping threadstate.Mapping, stage *prosody.Stage, state, globals []byte) []byte {
	mapping.StoreFields(Fields(stage), state)
	globals = Padded(globals)
	mapping.StoreFields(SharedFields(stage.Shared), globals)
	return globals
}

// Names names the thread state's bytes the stage's and its pipeline's values cover.
var Names = sync.OnceValue(func() map[int]string {
	stage := prosody.New(nil, nil, pipeline.New(nil, new(synthesizer.Synthesizer), nil, nil))
	return threadstate.Names(Fields(stage), threadstate.PipelineFields(stage.Pipeline))
})

var globalNames = sync.OnceValue(func() map[int]string {
	return threadstate.Names(SharedFields(new(prosody.Shared)))
})

// Describe names a byte of the thread state, for the reports.
func Describe(offset int) string {
	return threadstate.Describe(offset, Names())
}

// DescribeGlobal names a byte of the global variables, for the reports.
func DescribeGlobal(offset int) string {
	if name, found := globalNames()[offset]; found {
		return "shared " + name
	}
	return fmt.Sprintf("global %02x", offset)
}
