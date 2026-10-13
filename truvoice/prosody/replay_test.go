package prosody

import (
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/internal/conformance"
	"github.com/phyce/OpenSAPI4/truvoice/internal/threadstate"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
	"github.com/phyce/OpenSAPI4/truvoice/synthesizer"
)

// The prosody stage's golden data (golden/truvoice/prosody.jsonl.gz) records calls of the original's functions
// with the changes of the speaking thread's state and of the stage's global variables around them. The replay
// rebuilds both at every call and return (internal/threadstate), so that a Go function can start from the state
// the original's started from and leave what it left. The durations aren't ported yet: a replayed call takes
// them from the original's calls within it.

type (
	call     = threadstate.Call
	replayer = threadstate.Replayer
)

const durationFunction = "Prosody::AssignDuration"

func replay(t *testing.T, replayers map[string]replayer) map[string]int {
	t.Helper()
	return threadstate.Replay(t, "prosody.jsonl.gz", replayers, durationFunction)
}

// stageFields lists the stage's values with their places in the thread state, and the synthesizer's the walk
// reads.
func stageFields(stage *Stage) []threadstate.Field {
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
		at(0x1d50, "preset duration", &stage.PresetDuration), flag(0x1d55, "prosody off", &stage.ProsodyOff, prosodyOffFlag),
		at(0x1d58, "countdown", &stage.Countdown), at(0x1d5c, "phrase limit", &stage.PhraseLimit),
		at(0x1d64, "sentence phonemes", &stage.SentencePhonemes), at(0x1d68, "padding", &stage.Padding),
		at(0x1d6c, "scan result", &stage.ScanResult), flag(0x1d70, "no phrasing", &stage.NoPhrasing, 1),
		at(0x1d88, "since mark", &stage.SinceMark), at(0x1e1c, "emphasis", &stage.Emphasis),
		at(0x4, "synthesizer position", &ring.Position), at(0xc, "written", &ring.Written),
		at(0x1fe0, "lookahead", &ring.Lookahead)}
	for index, name := range []string{"first mark", "second mark", "third mark", "fourth mark"} {
		fields = append(fields, at(0x1d78+4*index, name, &stage.Marks[index]))
	}
	for index := range stage.WordMarks {
		fields = append(fields, at(0x1c7c+4*index, fmt.Sprintf("word mark %d", index), &stage.WordMarks[index]))
	}
	return fields
}

// globalsSize is the size of the global variables recorded, from 10128830.
const globalsSize = 0x38

// sharedFields lists the shared state's values with their places in the global variables.
func sharedFields(shared *Shared) []threadstate.Field {
	at, flag := threadstate.At, threadstate.FlagAt
	return []threadstate.Field{at(0x00, "scanned", &shared.Scanned), flag(0x04, "in phrase", &shared.InPhrase, 1),
		at(0x08, "last node", &shared.LastNode), at(0x0c, "preset symbol", &shared.PresetSymbol),
		at(0x10, "pending", &shared.Pending), at(0x14, "walk", &shared.Walk), flag(0x18, "held", &shared.Held, 1),
		at(0x1c, "preset pitch", &shared.PresetPitch), at(0x20, "saved result", &shared.SavedResult),
		at(0x24, "preset node", &shared.PresetNode), at(0x28, "counted", &shared.Counted),
		at(0x2c, "slow pauses", &shared.SlowPauses)}
}

func padded(globals []byte) []byte {
	return append(slices.Clone(globals), make([]byte, max(globalsSize-len(globals), 0))...)
}

// testData are the data pack's tables the replays need.
type testData struct {
	prosody    *datapack.Prosody
	parameters *datapack.Parameters
	pipeline   *datapack.Pipeline
}

func loadTestData(t *testing.T) *testData {
	t.Helper()
	pack := conformance.Pack(t)
	prosody, err := pack.LoadProsody()
	if err != nil {
		t.Fatal(err)
	}
	parameters, err := pack.LoadParameters()
	if err != nil {
		t.Fatal(err)
	}
	pipelineTables, err := pack.LoadPipeline()
	if err != nil {
		t.Fatal(err)
	}
	return &testData{prosody, parameters, pipelineTables}
}

// load returns a stage, with its own shared state, with the thread state's and global variables' values when
// the call started.
func (data *testData) load(call *call) *Stage {
	stage := New(data.prosody, &data.parameters.Classes, pipeline.New(data.pipeline, new(synthesizer.Synthesizer), nil, nil))
	stage.Shared = new(Shared)
	loadState(stage, call.Base, call.Before, call.GlobalsBefore)
	return stage
}

func loadState(stage *Stage, base uint32, state, globals []byte) {
	mapping := threadstate.Mapping{Pipeline: stage.Pipeline, Base: base}
	mapping.LoadPipeline(state)
	mapping.LoadFields(stageFields(stage), state)
	mapping.LoadFields(sharedFields(stage.Shared), padded(globals))
}

var stateNames = sync.OnceValue(func() map[int]string {
	stage := New(nil, nil, pipeline.New(nil, new(synthesizer.Synthesizer), nil, nil))
	return threadstate.Names(stageFields(stage), threadstate.PipelineFields(stage.Pipeline))
})

var globalNames = sync.OnceValue(func() map[int]string {
	return threadstate.Names(sharedFields(new(Shared)))
})

func describe(offset int) string {
	return threadstate.Describe(offset, stateNames())
}

func describeGlobal(offset int) string {
	if name, found := globalNames()[offset]; found {
		return "shared " + name
	}
	return fmt.Sprintf("global %02x", offset)
}

// compare reports where the state the stage leaves differs from the original's.
func compare(stage *Stage, call *call) error {
	return compareFrom(stage, call.Base, call.Before, call.GlobalsBefore, call.After, call.GlobalsAfter)
}

// compareFrom reports where the stage's state differs from the original's, after and globalsAfter, with the bytes
// it doesn't map from state and globals.
func compareFrom(stage *Stage, base uint32, state, globals, after, globalsAfter []byte) error {
	state = slices.Clone(state)
	mapping := threadstate.Mapping{Pipeline: stage.Pipeline, Base: base}
	mapping.StorePipeline(state)
	mapping.StoreFields(stageFields(stage), state)
	if err := threadstate.Differences(state, after, describe); err != nil {
		return err
	}
	globals = padded(globals)
	mapping.StoreFields(sharedFields(stage.Shared), globals)
	return threadstate.Differences(globals, padded(globalsAfter), describeGlobal)
}

// durations gives a stage the durations of the original's calls within a replayed call: before each it checks the
// stage's state against the original's, and after it gives the stage the original's.
type durations struct {
	calls          []*call
	state, globals []byte
	taken          int
	err            error
}

func takeDurations(stage *Stage, outer *call) *durations {
	recorded := &durations{state: outer.Before, globals: outer.GlobalsBefore}
	for _, inner := range outer.Inner {
		if inner.Record.Function == durationFunction {
			recorded.calls = append(recorded.calls, inner)
		}
	}
	stage.duration = func() {
		if recorded.taken == len(recorded.calls) {
			if recorded.err == nil {
				recorded.err = errors.New("more durations than the original's")
			}
			return
		}
		inner := recorded.calls[recorded.taken]
		recorded.taken++
		if err := compareFrom(stage, outer.Base, recorded.state, recorded.globals, inner.Before, inner.GlobalsBefore); err != nil && recorded.err == nil {
			recorded.err = fmt.Errorf("before duration %d: %v", recorded.taken, err)
		}
		loadState(stage, outer.Base, inner.After, inner.GlobalsAfter)
		recorded.state, recorded.globals = inner.After, inner.GlobalsAfter
	}
	return recorded
}

// check reports the first difference the durations found, or where the stage's state differs from the
// original's at the call's end.
func (recorded *durations) check(stage *Stage, outer *call) error {
	if recorded.err != nil {
		return recorded.err
	}
	if recorded.taken != len(recorded.calls) {
		return fmt.Errorf("%d durations, expected %d", recorded.taken, len(recorded.calls))
	}
	return compareFrom(stage, outer.Base, recorded.state, recorded.globals, outer.After, outer.GlobalsAfter)
}
