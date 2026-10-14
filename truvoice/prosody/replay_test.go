package prosody_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/internal/conformance"
	"github.com/phyce/OpenSAPI4/truvoice/internal/prosodystate"
	"github.com/phyce/OpenSAPI4/truvoice/internal/threadstate"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
	"github.com/phyce/OpenSAPI4/truvoice/prosody"
	"github.com/phyce/OpenSAPI4/truvoice/synthesizer"
)

// The prosody stage's golden data (golden/truvoice/prosody.jsonl.gz) records calls of the original's functions
// with the changes of the speaking thread's state and of the stage's global variables around them. The replay
// rebuilds both at every call and return (internal/threadstate, internal/prosodystate), so that a Go function
// can start from the state the original's started from and leave what it left. The durations' golden data
// (durations.jsonl.gz) records the same runs' durations and their parts.

type (
	call     = threadstate.Call
	replayer = threadstate.Replayer
)

func replay(t *testing.T, replayers map[string]replayer) map[string]int {
	t.Helper()
	return threadstate.Replay(t, "prosody.jsonl.gz", replayers)
}

func replayDurations(t *testing.T, replayers map[string]replayer) map[string]int {
	t.Helper()
	return threadstate.Replay(t, "durations.jsonl.gz", replayers)
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
	prosodyTables, err := pack.LoadProsody()
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
	return &testData{prosodyTables, parameters, pipelineTables}
}

// load returns a stage, with its own shared state, with the thread state's and global variables' values when
// the call started.
func (data *testData) load(call *call) *prosody.Stage {
	stage := prosody.New(data.prosody, data.parameters, pipeline.New(data.pipeline, new(synthesizer.Synthesizer), nil, nil))
	stage.Shared = new(prosody.Shared)
	mapping := threadstate.Mapping{Pipeline: stage.Pipeline, Base: call.Base}
	mapping.LoadPipeline(call.Before)
	prosodystate.Load(mapping, stage, call.Before, call.GlobalsBefore)
	return stage
}

// compare reports where the state the stage leaves differs from the original's.
func compare(stage *prosody.Stage, call *call) error {
	state := slices.Clone(call.Before)
	mapping := threadstate.Mapping{Pipeline: stage.Pipeline, Base: call.Base}
	mapping.StorePipeline(state)
	globals := prosodystate.Store(mapping, stage, state, call.GlobalsBefore)
	if err := threadstate.Differences(state, call.After, prosodystate.Describe); err != nil {
		return err
	}
	return threadstate.Differences(globals, prosodystate.Padded(call.GlobalsAfter), prosodystate.DescribeGlobal)
}

func expectResult(expected, result int32) error {
	if result != expected {
		return fmt.Errorf("result %d, expected %d", result, expected)
	}
	return nil
}
