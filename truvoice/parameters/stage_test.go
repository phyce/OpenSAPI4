package parameters

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"runtime"
	"slices"
	"sync"
	"testing"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/internal/conformance"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
	"github.com/phyce/OpenSAPI4/truvoice/synthesizer"
)

// The stage's golden data (golden/truvoice/stage.jsonl.gz) records the calls of the original's scheduler, and of
// the stages it runs before the parameter generator, with the changes of the speaking thread's state around
// them.

// The thread state's places of the scheduler's state and the synthesizer's flags.
const (
	stateMarkersPassed = 0x34
	stateHold          = 0x19ac
	stateReady         = 0x19ad
	stateGain          = 0x2104
	stateCharacter     = 0x2108
	stateOutOfInput    = 0x2112
	stateHasWork       = 0x2113
	stateWaiting       = 0x2117
	stateRetired       = 0x2118
	stateBytes         = 0x2124
	stateRendering     = 0x212e
)

type stageCase struct {
	Name    string       `json:"name"`
	Format  [2]int       `json:"format"`
	Digest  string       `json:"digest"`
	Events  [][]any      `json:"events"`
	Records []callRecord `json:"records"`
}

// stageReplay runs the Go scheduler, parameter generator, output stage and synthesizer on one recorded run, and
// stands in for the stages before the parameter generator: a stage's call takes the state the original's left.
type stageReplay struct {
	data              *testData
	synthesizerTables *datapack.Synthesizer
	records           []callRecord
	position          int
	state             []byte
	base              uint32
	pipeline          *pipeline.Pipeline
	generator         *Generator
	texts             map[uint32]*pipeline.Text
	stepping          bool
	ran               []string
	err               error
	counts            map[string]int
}

// countedGenerator counts the parameter generator's steps and waits.
type countedGenerator struct {
	*Generator
	counts map[string]int
}

func (generator countedGenerator) Step() int {
	result := generator.Generator.Step()
	generator.counts["parameter generator"]++
	if result == pipeline.ParametersWaiting {
		generator.counts["waits"]++
	}
	return result
}

func (replay *stageReplay) fail(format string, arguments ...any) {
	if replay.err == nil {
		replay.err = fmt.Errorf(format, arguments...)
	}
}

// next returns the next call or return record, with the state brought to it, or nil at the end.
func (replay *stageReplay) next() *callRecord {
	for replay.position < len(replay.records) {
		record := &replay.records[replay.position]
		replay.position++
		if record.Kind != "call" && record.Kind != "return" {
			continue
		}
		if record.Base != 0 {
			replay.base = record.Base
			clear(replay.state)
		}
		for _, run := range record.Diff {
			copy(replay.state[run.offset:], run.data)
		}
		return record
	}
	return nil
}

func named(record *callRecord) string {
	if record == nil {
		return "the end of the records"
	}
	return record.Kind + " " + record.Function
}

// text returns the text a text pointer stands for, numbered in the order the texts appear.
func (replay *stageReplay) text(pointer uint32) *pipeline.Text {
	if replay.texts[pointer] == nil {
		replay.texts[pointer] = &pipeline.Text{Number: len(replay.texts)}
	}
	return replay.texts[pointer]
}

// start starts the Go speaking thread with the engine's sample rate, gain and attributes as the state has them
// at the first step, and compares the state it leaves with that, but for what the input side wrote.
func (replay *stageReplay) start() error {
	state := replay.state
	halfWord := func(offset int) uint16 { return binary.LittleEndian.Uint16(state[offset:]) }
	engine := &pipeline.Engine{SampleRate: int(halfWord(stateSampleRate)), Gain: uint32(halfWord(stateGain)),
		Pitch: uint16(dword(state, statePitch)), Speed: dword(state, stateSpeed), Volume: dword(state, stateVolume),
		Character: dword(state, stateCharacter), Voice: int32(int16(halfWord(stateOutputVoice)))}
	replay.pipeline = pipeline.New(replay.data.pipeline, synthesizer.New(replay.synthesizerTables, engine.SampleRate), replay, engine)
	replay.generator = New(replay.data.parameters, replay.data.vowels, replay.pipeline)
	replay.pipeline.Generator = countedGenerator{replay.generator, replay.counts}
	replay.pipeline.Start()
	replay.pipeline.ApplyAttributes()
	replay.pipeline.OutOfInput = state[stateOutOfInput] != 0
	replay.pipeline.ShortPhrase = state[stateShortPhrase] != 0
	if err := replay.compare(); err != nil {
		return fmt.Errorf("after the start: %v", err)
	}
	return nil
}

// load gives the Go pipeline the state's values.
func (replay *stageReplay) load() {
	state := replay.state
	loadInto(replay.generator, state, replay.base, replay.text)
	stages := replay.pipeline
	ring := stages.Synthesizer
	stages.OutOfInput = state[stateOutOfInput] != 0
	for stage := range stages.HasWork {
		stages.HasWork[stage] = state[stateHasWork+stage] != 0
	}
	stages.WaitingForRoom = state[stateWaiting] != 0
	stages.Retired = int(int32(dword(state, stateRetired)))
	stages.Character = dword(state, stateCharacter)
	ring.Hold, ring.Ready = state[stateHold] != 0, state[stateReady] != 0
	ring.MarkersPassed = int32(dword(state, stateMarkersPassed))
	ring.Gain = binary.LittleEndian.Uint16(state[stateGain:])
	ring.Rendering = binary.LittleEndian.Uint16(state[stateRendering:]) != 0
}

// compare reports where the Go pipeline differs from the state.
func (replay *stageReplay) compare() error {
	state := slices.Clone(replay.state)
	store(replay.generator, state, replay.base)
	stages := replay.pipeline
	ring := stages.Synthesizer
	flag := func(offset int, value bool) {
		state[offset] = 0
		if value {
			state[offset] = 1
		}
	}
	flag(stateOutOfInput, stages.OutOfInput)
	for stage, value := range stages.HasWork {
		flag(stateHasWork+stage, value)
	}
	flag(stateWaiting, stages.WaitingForRoom)
	flag(stateHold, ring.Hold)
	flag(stateReady, ring.Ready)
	binary.LittleEndian.PutUint32(state[stateRetired:], uint32(stages.Retired))
	binary.LittleEndian.PutUint32(state[stateCharacter:], stages.Character)
	binary.LittleEndian.PutUint32(state[stateMarkersPassed:], uint32(ring.MarkersPassed))
	binary.LittleEndian.PutUint32(state[stateBytes:], uint32(2*len(ring.Samples)))
	binary.LittleEndian.PutUint16(state[stateGain:], ring.Gain)
	binary.LittleEndian.PutUint16(state[stateRendering:], 0)
	if ring.Rendering {
		state[stateRendering] = 1
	}
	if err := differences(state, replay.state); err != nil {
		return err
	}
	for index, offset := range nodeOffsets() {
		node := stages.Node(index)
		if pointer := dword(replay.state, offset+nodeText); pointer != 0 && node.Text != replay.text(pointer) {
			return fmt.Errorf("node %d has text %v, expected %d", index, node.Text, replay.text(pointer).Number)
		}
	}
	return nil
}

// stage stands in for a stage's run: it checks that the original ran it here and from the same state, and
// takes the state it left.
func (replay *stageReplay) stage(function string) bool {
	replay.ran = append(replay.ran, function)
	if replay.err != nil {
		return false
	}
	if record := replay.next(); record == nil || record.Kind != "call" || record.Function != function {
		replay.fail("the Go scheduler ran %s, the original went on with %s", function, named(record))
		return false
	}
	if err := replay.compare(); err != nil {
		replay.fail("before %s: %v", function, err)
		return false
	}
	record := replay.next()
	if record == nil || record.Kind != "return" || record.Function != function {
		replay.fail("%s went on with %s", function, named(record))
		return false
	}
	replay.load()
	replay.counts[function]++
	return record.EAX&0xff != 0
}

func (replay *stageReplay) Input() bool      { return replay.stage("Input::MakeRecords") }
func (replay *stageReplay) Normalizer() bool { return replay.stage("Normalizer::Run") }
func (replay *stageReplay) Lexical() bool    { return replay.stage("Lexical::Step") }
func (replay *stageReplay) Prosody() bool    { return replay.stage("Prosody::Run") }

func (replay *stageReplay) Reset() {
	if replay.stepping {
		replay.fail("the Go scheduler reset the stages")
	}
}

// step runs the Go scheduler's step for the original's, from the state it started from, and compares what it
// reports and leaves.
func (replay *stageReplay) step() error {
	if replay.pipeline == nil {
		if err := replay.start(); err != nil {
			return err
		}
	}
	if bytes := 2 * len(replay.pipeline.Synthesizer.Samples); uint32(bytes) != dword(replay.state, stateBytes) {
		return fmt.Errorf("%d bytes of samples before the step, expected %d", bytes, dword(replay.state, stateBytes))
	}
	replay.load()
	replay.ran = nil
	replay.stepping = true
	result := replay.pipeline.Step()
	replay.stepping = false
	if replay.err != nil {
		return replay.err
	}
	record := replay.next()
	if record == nil || record.Kind != "return" || record.Function != "Pipeline::Step" {
		return fmt.Errorf("after %v the original went on with %s", replay.ran, named(record))
	}
	if uint32(result) != record.EAX {
		return fmt.Errorf("Step reported %d after %v, expected %d", result, replay.ran, record.EAX)
	}
	if err := replay.compare(); err != nil {
		return fmt.Errorf("after %v: %v", replay.ran, err)
	}
	if result&pipeline.StepHandOver != 0 {
		replay.counts["hand-overs"]++
		replay.counts["samples"] += len(replay.pipeline.Synthesizer.Samples)
		replay.pipeline.HandOver()
	}
	replay.counts["scheduler"]++
	return nil
}

func (replay *stageReplay) run(golden stageCase) error {
	for {
		index := replay.position
		record := replay.next()
		switch {
		case record == nil:
			if replay.pipeline == nil {
				return fmt.Errorf("no step recorded")
			}
			for _, event := range golden.Events {
				if event[0] != nil && (event[1] == "word" || event[1] == "mark") {
					replay.counts[event[1].(string)]++
				}
			}
			return compareQueue(replay.pipeline.Engine.Queue, golden)
		case record.Function == "Pipeline::Initialize":
		case record.Kind == "call" && record.Function == "Pipeline::Step":
			if err := replay.step(); err != nil {
				return fmt.Errorf("record %d: %v", index, err)
			}
		default:
			return fmt.Errorf("record %d: %s outside the scheduler", index, named(record))
		}
	}
}

// compareQueue compares the audio the Go thread queued with the original's PCM, and the bookmarks and words
// queued with it with the ones the original reported, at the sample where each was queued.
func compareQueue(queue []pipeline.Item, golden stageCase) error {
	var pcm []byte
	var actual, expected []string
	position := 0
	for _, item := range queue {
		if item.Event == nil {
			pcm = synthesizer.AppendPCM(pcm, item.Samples, golden.Format[1])
			position += len(item.Samples)
			continue
		}
		event := item.Event
		switch {
		case event.Kind == pipeline.WordPosition:
			actual = append(actual, fmt.Sprint(position, " word ", event.Text.Number, " ", event.Value))
		case event.Kind == pipeline.Mark && event.Value != 0:
			actual = append(actual, fmt.Sprint(position, " mark ", event.Text.Number, " ", event.Value))
		}
	}
	for _, event := range golden.Events {
		if event[0] != nil && (event[1] == "word" || event[1] == "mark") {
			expected = append(expected, fmt.Sprint(int(event[0].(float64)), " ", event[1], " ", int(event[2].(float64)), " ", uint32(event[3].(float64))))
		}
	}
	if digest := sha256.Sum256(pcm); hex.EncodeToString(digest[:]) != golden.Digest {
		return fmt.Errorf("the PCM differs (%d bytes)", len(pcm))
	}
	if !slices.Equal(actual, expected) {
		return fmt.Errorf("events %v, expected %v", actual, expected)
	}
	return nil
}

// TestStage runs the Go scheduler, parameter generator, output stage and synthesizer for every recorded step of
// the original's scheduler, with the stages before the parameter generator taken from the records: it checks the
// stages the scheduler runs, the state before each of them, what each step reports and the whole state it
// leaves, and that the audio handed over is the original's PCM, with the bookmarks and words where the
// original's came.
func TestStage(t *testing.T) {
	data := loadTestData(t)
	synthesizerTables, err := conformance.Pack(t).LoadSynthesizer()
	if err != nil {
		t.Fatal(err)
	}
	var lock sync.Mutex
	failures, runs := 0, 0
	counts := map[string]int{}
	cases := make(chan stageCase)
	var workers sync.WaitGroup
	for range runtime.GOMAXPROCS(0) {
		workers.Go(func() {
			for golden := range cases {
				replay := &stageReplay{data: data, synthesizerTables: synthesizerTables, records: golden.Records,
					state: make([]byte, stateSize), texts: map[uint32]*pipeline.Text{}, counts: map[string]int{}}
				err := replay.run(golden)
				lock.Lock()
				runs++
				for name, count := range replay.counts {
					counts[name] += count
				}
				if err != nil {
					failures++
					if failures <= 10 {
						t.Errorf("%s: %v", golden.Name, err)
					}
				}
				lock.Unlock()
			}
		})
	}
	conformance.EachLine(t, func(golden stageCase) { cases <- golden }, "stage.jsonl.gz")
	close(cases)
	workers.Wait()
	if failures > 0 || counts["scheduler"] == 0 {
		t.Errorf("%d of %d runs differ", failures, runs)
	}
	t.Logf("%d runs: %d scheduler steps, %d input, %d normalizer, %d lexical, %d prosody and %d parameter generator steps (%d waits), %d hand-overs of %d samples, %d words and %d bookmarks",
		runs, counts["scheduler"], counts["Input::MakeRecords"], counts["Normalizer::Run"], counts["Lexical::Step"],
		counts["Prosody::Run"], counts["parameter generator"], counts["waits"], counts["hand-overs"], counts["samples"],
		counts["word"], counts["mark"])
}
