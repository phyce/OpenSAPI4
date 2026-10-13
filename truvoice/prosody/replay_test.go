package prosody

import (
	"bytes"
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
// the original's started from and leave what it left.

type (
	call     = threadstate.Call
	replayer = threadstate.Replayer
)

func replay(t *testing.T, replayers map[string]replayer) map[string]int {
	t.Helper()
	return threadstate.Replay(t, "prosody.jsonl.gz", replayers)
}

// stageFields lists the stage's values with their places in the thread state.
func stageFields(stage *Stage) []threadstate.Field {
	at, flag := threadstate.At, threadstate.FlagAt
	context := &stage.Context
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
		at(0x1dd2, "preceding symbol", &context.PrecedingSymbol), at(0x1dd3, "before preceding symbol", &context.BeforePrecedingSymbol)}
	for index, name := range []string{"first mark", "second mark", "third mark", "fourth mark"} {
		fields = append(fields, at(0x1d78+4*index, name, &stage.Marks[index]))
	}
	return fields
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

// load returns a stage with the thread state's values when the call started.
func (data *testData) load(call *call) *Stage {
	stage := New(data.prosody, &data.parameters.Classes, pipeline.New(data.pipeline, new(synthesizer.Synthesizer), nil, nil))
	mapping := threadstate.Mapping{Pipeline: stage.Pipeline, Base: call.Base}
	mapping.LoadPipeline(call.Before)
	mapping.LoadFields(stageFields(stage), call.Before)
	return stage
}

var stateNames = sync.OnceValue(func() map[int]string {
	stage := New(nil, nil, pipeline.New(nil, new(synthesizer.Synthesizer), nil, nil))
	return threadstate.Names(stageFields(stage), threadstate.PipelineFields(stage.Pipeline))
})

func describe(offset int) string {
	return threadstate.Describe(offset, stateNames())
}

// compare reports where the state the stage leaves differs from the original's.
func compare(stage *Stage, call *call) error {
	state := slices.Clone(call.Before)
	mapping := threadstate.Mapping{Pipeline: stage.Pipeline, Base: call.Base}
	mapping.StorePipeline(state)
	mapping.StoreFields(stageFields(stage), state)
	if err := threadstate.Differences(state, call.After, describe); err != nil {
		return err
	}
	if !bytes.Equal(call.GlobalsBefore, call.GlobalsAfter) {
		return fmt.Errorf("the original changed the global variables")
	}
	return nil
}
