package input_test

import (
	"encoding/binary"
	"fmt"
	"slices"
	"testing"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/input"
	"github.com/phyce/OpenSAPI4/truvoice/internal/conformance"
	"github.com/phyce/OpenSAPI4/truvoice/internal/inputstate"
	"github.com/phyce/OpenSAPI4/truvoice/internal/threadstate"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
	"github.com/phyce/OpenSAPI4/truvoice/synthesizer"
)

// The input side's golden data (golden/truvoice/input.jsonl.gz) records the calls of the original's escape parser
// and input stage with the changes of the speaking thread's state around them. The replay rebuilds the state at
// every call and return (internal/threadstate, internal/inputstate), so that a Go function can start from the
// state the original's started from and leave what it left.

// The escape parser's own ring in the thread state: its characters, and where it reads and writes.
const (
	parserRing        = 0x8070
	parserRingRead    = 0x8170
	parserRingWritten = 0x8174
	parserRingEnd     = 0x8178
)

type testData struct {
	input    *datapack.Input
	pipeline *datapack.Pipeline
}

func loadTestData(t *testing.T) *testData {
	t.Helper()
	pack := conformance.Pack(t)
	inputTables, err := pack.LoadInput()
	if err != nil {
		t.Fatal(err)
	}
	pipelineTables, err := pack.LoadPipeline()
	if err != nil {
		t.Fatal(err)
	}
	return &testData{input: inputTables, pipeline: pipelineTables}
}

// A run is the input side running a recorded call, with the texts the state's pointers stand for.
type run struct {
	stage    *input.Stage
	mapping  threadstate.Mapping
	pointers map[*pipeline.Text]uint32
}

// load returns the input side with the thread state's values when the call started.
func (data *testData) load(call *threadstate.Call) *run {
	stage := input.New(data.input, pipeline.New(data.pipeline, new(synthesizer.Synthesizer), nil, nil))
	run := &run{stage: stage, pointers: map[*pipeline.Text]uint32{}}
	texts := map[uint32]*pipeline.Text{}
	text := func(pointer uint32) *pipeline.Text {
		if texts[pointer] == nil {
			texts[pointer] = &pipeline.Text{Number: len(texts)}
			run.pointers[texts[pointer]] = pointer
		}
		return texts[pointer]
	}
	run.mapping = threadstate.Mapping{Pipeline: stage.Pipeline, Base: call.Base, Text: text}
	run.mapping.LoadPipeline(call.Before)
	inputstate.Load(run.mapping, stage, call.Before)
	return run
}

// finish reports the first difference of a replayed call: the state the call left, written over the state it
// started from. The escape parser's own ring is taken from the state the call left: the parser has read every
// character the call put in it.
func (run *run) finish(call *threadstate.Call) error {
	state := slices.Clone(call.Before)
	run.mapping.StorePipeline(state)
	inputstate.Store(run.mapping, run.stage, state)
	for index, offset := range threadstate.NodeOffsets() {
		if text := run.stage.Pipeline.Node(index).Text; text != nil {
			binary.LittleEndian.PutUint32(state[offset+threadstate.NodeText:], run.pointers[text])
		}
	}
	copy(state[parserRing:parserRingEnd], call.After[parserRing:parserRingEnd])
	return threadstate.Differences(state, call.After, inputstate.Describe)
}

// parserRingEmpty reports an error when the escape parser's own ring holds a character it hasn't read.
func parserRingEmpty(state []byte, when string) error {
	if read, written := threadstate.Dword(state, parserRingRead), threadstate.Dword(state, parserRingWritten); read != written {
		return fmt.Errorf("the parser's ring reads at %d and writes at %d %s", read, written, when)
	}
	return nil
}

// TestInput replays every recorded call of the escape parser's and the input stage's.
func TestInput(t *testing.T) {
	data := loadTestData(t)
	counts := threadstate.Replay(t, "input.jsonl.gz", map[string]threadstate.Replayer{
		"ParserRing::Put": func(call *threadstate.Call) error {
			if err := parserRingEmpty(call.Before, "before the call"); err != nil {
				return err
			}
			if err := parserRingEmpty(call.After, "after it"); err != nil {
				return err
			}
			run := data.load(call)
			run.stage.Put(byte(call.Argument(0)))
			return run.finish(call)
		},
		"Input::MakeRecords": func(call *threadstate.Call) error {
			run := data.load(call)
			if made, expected := run.stage.MakeRecords(), call.Result.EAX&0xff != 0; made != expected {
				return fmt.Errorf("result %t, expected %t", made, expected)
			}
			return run.finish(call)
		},
	})
	if counts["ParserRing::Put"] == 0 || counts["Input::MakeRecords"] == 0 {
		t.Errorf("calls replayed: %v", counts)
	}
	t.Logf("calls replayed: %v", counts)
}
