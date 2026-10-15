package normalizer_test

import (
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/internal/conformance"
	"github.com/phyce/OpenSAPI4/truvoice/internal/normalizerstate"
	"github.com/phyce/OpenSAPI4/truvoice/internal/threadstate"
	"github.com/phyce/OpenSAPI4/truvoice/normalizer"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
	"github.com/phyce/OpenSAPI4/truvoice/synthesizer"
)

// The normalizer's golden data (golden/truvoice/normalizer.jsonl.gz) and the stages' (stage.jsonl.gz) record the
// original normalizer's runs with the changes of the speaking thread's state around them. The replay rebuilds the
// state at every run (internal/threadstate, internal/normalizerstate), so that the Go normalizer can start from
// the state the original's started from and leave what it left.

// testData are the data pack's tables the replay needs, and each run's pointers into the program and the lists.
type testData struct {
	normalizer *datapack.Normalizer
	parameters *datapack.Parameters
	pipeline   *datapack.Pipeline
	pointers   sync.Map
}

func loadTestData(t *testing.T) *testData {
	t.Helper()
	pack := conformance.Pack(t)
	normalizerTables, err := pack.LoadNormalizer()
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
	return &testData{normalizer: normalizerTables, parameters: parameters, pipeline: pipelineTables}
}

// run runs the Go normalizer from the state a recorded run started from, and compares what it reports and the
// state it leaves.
func (data *testData) run(call *threadstate.Call) (err error) {
	defer func() {
		if failure := recover(); failure != nil {
			err = fmt.Errorf("panic: %v", failure)
		}
	}()
	stage := normalizer.New(data.normalizer, data.parameters, pipeline.New(data.pipeline, new(synthesizer.Synthesizer), nil, nil))
	mapping := threadstate.Mapping{Pipeline: stage.Pipeline, Base: call.Base}
	mapping.LoadPipeline(call.Before)
	pointers, _ := data.pointers.LoadOrStore(call.Case, normalizerstate.NewPointers())
	if err := pointers.(*normalizerstate.Pointers).Load(mapping, stage, call.Before); err != nil {
		return err
	}
	handed := stage.Run()
	if expected := call.Result.EAX&0xff != 0; handed != expected {
		return fmt.Errorf("reported %t, expected %t", handed, expected)
	}
	state := slices.Clone(call.Before)
	mapping.StorePipeline(state)
	pointers.(*normalizerstate.Pointers).Store(mapping, stage, state, call.After)
	return threadstate.Differences(state, call.After, normalizerstate.Describe)
}

// TestNormalizer replays every recorded run of the normalizer.
func TestNormalizer(t *testing.T) {
	data := loadTestData(t)
	runs := 0
	for _, file := range []string{"normalizer.jsonl.gz", "stage.jsonl.gz"} {
		runs += threadstate.Replay(t, file, map[string]threadstate.Replayer{"Normalizer::Run": data.run})["Normalizer::Run"]
	}
	t.Logf("%d runs", runs)
}
