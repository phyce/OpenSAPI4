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
	"github.com/phyce/OpenSAPI4/truvoice/input"
	"github.com/phyce/OpenSAPI4/truvoice/internal/conformance"
	"github.com/phyce/OpenSAPI4/truvoice/internal/inputstate"
	"github.com/phyce/OpenSAPI4/truvoice/internal/lexicalstate"
	"github.com/phyce/OpenSAPI4/truvoice/internal/prosodystate"
	"github.com/phyce/OpenSAPI4/truvoice/internal/threadstate"
	"github.com/phyce/OpenSAPI4/truvoice/lexical"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
	"github.com/phyce/OpenSAPI4/truvoice/prosody"
	"github.com/phyce/OpenSAPI4/truvoice/synthesizer"
)

// The stage's golden data (golden/truvoice/stage.jsonl.gz) records the calls of the original's scheduler, and of
// the stages it runs before the parameter generator, with the changes of the speaking thread's state and of the
// prosody stage's global variables around them. Between two calls, the speaking thread's input side has given the
// escape parser more of the text.

// The thread state's places of the scheduler's state and the synthesizer's flags.
const (
	stateMarkersPassed = 0x34
	stateReady         = 0x19ad
	stateGain          = 0x2104
	stateOutOfInput    = 0x2112
	stateHasWork       = 0x2113
	stateWaiting       = 0x2117
	stateRetired       = 0x2118
	stateBytes         = 0x2124
	stateRendering     = 0x212e
)

type stageCase struct {
	Name    string               `json:"name"`
	Lexicon [][2]string          `json:"lexicon"`
	Format  [2]int               `json:"format"`
	Digest  string               `json:"digest"`
	Events  [][]any              `json:"events"`
	Records []threadstate.Record `json:"records"`
}

// The data pack's tables the stage test needs beside the generator's.
type stageTables struct {
	synthesizer *datapack.Synthesizer
	prosody     *datapack.Prosody
	lexical     *datapack.Lexical
	dictionary  *datapack.Dictionary
	input       *datapack.Input
}

// stageReplay runs the Go scheduler, input stage, lexical stage, prosody stage, parameter generator, output stage
// and synthesizer on one recorded run, and stands in for the normalizer, whose call takes the state the original's
// left, and the escape parser: each step starts with what the original's escape parser had written. A run can start
// new speaking threads; queue holds what the ended ones queued.
type stageReplay struct {
	data        *testData
	tables      *stageTables
	userLexicon lexical.Lexicon
	records     []threadstate.Record
	position    int
	thread      *threadstate.State
	pipeline    *pipeline.Pipeline
	queue       []pipeline.Item
	generator   *Generator
	input       *input.Stage
	lexical     *lexical.Stage
	affixes     *lexicalstate.Affixes
	prosody     *prosody.Stage
	texts       map[uint32]*pipeline.Text
	stepping    bool
	ran         []string
	err         error
	counts      map[string]int
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
func (replay *stageReplay) next() *threadstate.Record {
	for replay.position < len(replay.records) {
		record := &replay.records[replay.position]
		replay.position++
		if record.Kind != "call" && record.Kind != "return" {
			continue
		}
		replay.thread.Apply(record)
		return record
	}
	return nil
}

func named(record *threadstate.Record) string {
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

// start starts a Go speaking thread with the engine's sample rate, gain and attributes as the state has them at
// the thread's first step, and compares the state it leaves with that, but for what the input side wrote: the
// escape parser's state, settings and command ring, the short phrase flag and that the input has come. The
// prosody stage's shared state and the affixes' pointers are the process's, which every thread shares.
func (replay *stageReplay) start() error {
	state := replay.thread.Bytes
	halfWord := func(offset int) uint16 { return binary.LittleEndian.Uint16(state[offset:]) }
	engine := &pipeline.Engine{SampleRate: int(halfWord(threadstate.SampleRate)), Gain: uint32(halfWord(stateGain)),
		Pitch: uint16(threadstate.Dword(state, threadstate.Pitch)), Speed: threadstate.Dword(state, threadstate.Speed), Volume: threadstate.Dword(state, threadstate.Volume),
		Character: threadstate.Dword(state, threadstate.Character), Voice: int32(int16(halfWord(threadstate.OutputVoice)))}
	replay.pipeline = pipeline.New(replay.data.pipeline, synthesizer.New(replay.tables.synthesizer, engine.SampleRate), replay, engine)
	replay.generator = New(replay.data.parameters, replay.data.vowels, replay.pipeline)
	replay.pipeline.Generator = countedGenerator{replay.generator, replay.counts}
	replay.input = input.New(replay.tables.input, replay.pipeline)
	replay.lexical = lexical.New(replay.tables.lexical, replay.tables.dictionary, replay.data.parameters, replay.pipeline)
	replay.lexical.Lexicon = replay.userLexicon
	if replay.affixes == nil {
		replay.affixes = lexicalstate.NewAffixes()
	}
	shared := new(prosody.Shared)
	if replay.prosody != nil {
		shared = replay.prosody.Shared
	}
	replay.prosody = prosody.New(replay.tables.prosody, replay.data.parameters, replay.pipeline)
	replay.prosody.Shared = shared
	replay.pipeline.Start()
	replay.pipeline.ApplyAttributes()
	inputstate.LoadParser(replay.mapping(), replay.input, state)
	replay.pipeline.OutOfInput = state[stateOutOfInput] != 0
	replay.pipeline.ShortPhrase = state[threadstate.ShortPhrase] != 0
	if err := replay.compare(); err != nil {
		return fmt.Errorf("after the start: %v", err)
	}
	replay.counts["threads"]++
	return nil
}

// mapping maps the state to the Go pipeline.
func (replay *stageReplay) mapping() threadstate.Mapping {
	return threadstate.Mapping{Pipeline: replay.pipeline, Base: replay.thread.Base, Text: replay.text}
}

// load gives the Go pipeline the state's values, and the prosody stage the global variables'.
func (replay *stageReplay) load() {
	state := replay.thread.Bytes
	loadInto(replay.generator, state, replay.thread.Base, replay.text)
	inputstate.Load(replay.mapping(), replay.input, state)
	lexicalstate.Load(replay.mapping(), replay.lexical, state)
	replay.affixes.Load(replay.lexical, state)
	prosodystate.Load(replay.mapping(), replay.prosody, state, replay.thread.Globals)
	stages := replay.pipeline
	ring := stages.Synthesizer
	stages.OutOfInput = state[stateOutOfInput] != 0
	for stage := range stages.HasWork {
		stages.HasWork[stage] = state[stateHasWork+stage] != 0
	}
	stages.WaitingForRoom = state[stateWaiting] != 0
	stages.Retired = int(int32(threadstate.Dword(state, stateRetired)))
	stages.Character = threadstate.Dword(state, threadstate.Character)
	ring.Ready = state[stateReady] != 0
	ring.MarkersPassed = int32(threadstate.Dword(state, stateMarkersPassed))
	ring.Gain = binary.LittleEndian.Uint16(state[stateGain:])
	ring.Rendering = binary.LittleEndian.Uint16(state[stateRendering:]) != 0
}

// compare reports where the Go pipeline differs from the state, and the prosody stage from the global variables.
func (replay *stageReplay) compare() error {
	state := slices.Clone(replay.thread.Bytes)
	store(replay.generator, state, replay.thread.Base)
	inputstate.Store(replay.mapping(), replay.input, state)
	lexicalstate.Store(replay.mapping(), replay.lexical, state)
	replay.affixes.Store(replay.lexical, state, replay.thread.Bytes)
	globals := prosodystate.Store(replay.mapping(), replay.prosody, state, replay.thread.Globals)
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
	flag(stateReady, ring.Ready)
	binary.LittleEndian.PutUint32(state[stateRetired:], uint32(stages.Retired))
	binary.LittleEndian.PutUint32(state[threadstate.Character:], stages.Character)
	binary.LittleEndian.PutUint32(state[stateMarkersPassed:], uint32(ring.MarkersPassed))
	binary.LittleEndian.PutUint32(state[stateBytes:], uint32(2*len(ring.Samples)))
	binary.LittleEndian.PutUint16(state[stateGain:], ring.Gain)
	binary.LittleEndian.PutUint16(state[stateRendering:], 0)
	if ring.Rendering {
		state[stateRendering] = 1
	}
	if err := threadstate.Differences(state, replay.thread.Bytes, describeStage); err != nil {
		return err
	}
	if err := threadstate.Differences(globals, prosodystate.Padded(replay.thread.Globals), prosodystate.DescribeGlobal); err != nil {
		return err
	}
	for index, offset := range threadstate.NodeOffsets() {
		node := stages.Node(index)
		if pointer := threadstate.Dword(replay.thread.Bytes, offset+threadstate.NodeText); pointer != 0 && node.Text != replay.text(pointer) {
			return fmt.Errorf("node %d has text %v, expected %d", index, node.Text, replay.text(pointer).Number)
		}
	}
	return nil
}

// describeStage names a byte of the thread state, for the reports.
func describeStage(offset int) string {
	if name, found := prosodystate.Names()[offset]; found {
		return name
	}
	if name, found := lexicalstate.Names()[offset]; found {
		return name
	}
	if name, found := inputstate.Names()[offset]; found {
		return name
	}
	return describe(offset)
}

// enter checks that the original ran a stage here and from the same state.
func (replay *stageReplay) enter(function string) bool {
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
	return true
}

// leave returns the stage's return, or nil when the original went on with something else.
func (replay *stageReplay) leave(function string) *threadstate.Record {
	record := replay.next()
	if record == nil || record.Kind != "return" || record.Function != function {
		replay.fail("%s went on with %s", function, named(record))
		return nil
	}
	replay.counts[function]++
	return record
}

// stage stands in for a stage's run: it checks that the original ran it here and from the same state, and
// takes the state it left.
func (replay *stageReplay) stage(function string) bool {
	if !replay.enter(function) {
		return false
	}
	record := replay.leave(function)
	if record == nil {
		return false
	}
	replay.load()
	return record.EAX&0xff != 0
}

func (replay *stageReplay) Input() bool {
	return replay.runStage("Input::MakeRecords", replay.input.MakeRecords)
}
func (replay *stageReplay) Normalizer() bool { return replay.stage("Normalizer::Run") }
func (replay *stageReplay) Lexical() bool {
	return replay.runStage("Lexical::Step", replay.lexical.Step)
}
func (replay *stageReplay) Prosody() bool { return replay.runStage("Prosody::Run", replay.prosody.Run) }

// runStage runs a Go stage where the original ran its own, from the same state, and checks what it reports and
// leaves.
func (replay *stageReplay) runStage(function string, run func() bool) bool {
	if !replay.enter(function) {
		return false
	}
	handed := run()
	record := replay.leave(function)
	switch {
	case record == nil:
	case handed != (record.EAX&0xff != 0):
		replay.fail("%s reported %t, expected %t", function, handed, !handed)
	default:
		if err := replay.compare(); err != nil {
			replay.fail("after %s: %v", function, err)
		}
	}
	return handed
}

func (replay *stageReplay) Reset() {
	if replay.stepping {
		replay.fail("the Go scheduler reset the stages")
	}
	replay.input.Reset()
	replay.lexical.Reset()
	replay.prosody.Reset()
}

// step runs the Go scheduler's step for the original's, from the state it started from, and compares what it
// reports and leaves.
func (replay *stageReplay) step() error {
	if replay.pipeline == nil {
		if err := replay.start(); err != nil {
			return err
		}
	}
	if bytes := 2 * len(replay.pipeline.Synthesizer.Samples); uint32(bytes) != threadstate.Dword(replay.thread.Bytes, stateBytes) {
		return fmt.Errorf("%d bytes of samples before the step, expected %d", bytes, threadstate.Dword(replay.thread.Bytes, stateBytes))
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
			return compareQueue(append(replay.queue, replay.pipeline.Engine.Queue...), golden)
		case record.Function == "Pipeline::Initialize":
			if record.Kind == "call" && replay.pipeline != nil {
				replay.queue = append(replay.queue, replay.pipeline.Engine.Queue...)
				replay.pipeline = nil
			}
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

// TestStage runs the Go scheduler, input stage, lexical stage, prosody stage, parameter generator, output stage and
// synthesizer for every recorded step of the original's scheduler, with the normalizer and the escape parser taken
// from the records: it checks the stages the scheduler runs, the state before each of them, what each step, the
// input stage, the lexical stage and the prosody stage report and the whole state they leave, with the prosody
// stage's shared state, and that the audio handed over is the original's PCM, with the bookmarks and words where
// the original's came.
func TestStage(t *testing.T) {
	data := loadTestData(t)
	pack := conformance.Pack(t)
	var tables stageTables
	var err error
	if tables.synthesizer, err = pack.LoadSynthesizer(); err != nil {
		t.Fatal(err)
	}
	if tables.prosody, err = pack.LoadProsody(); err != nil {
		t.Fatal(err)
	}
	if tables.lexical, err = pack.LoadLexical(); err != nil {
		t.Fatal(err)
	}
	if tables.dictionary, err = pack.LoadDictionary(); err != nil {
		t.Fatal(err)
	}
	if tables.input, err = pack.LoadInput(); err != nil {
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
				replay := &stageReplay{data: data, tables: &tables, userLexicon: lexicalstate.NewUserLexicon(golden.Lexicon),
					records: golden.Records, thread: threadstate.NewState(), texts: map[uint32]*pipeline.Text{}, counts: map[string]int{}}
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
	t.Logf("%d runs, %d speaking threads: %d scheduler steps, %d input, %d normalizer, %d lexical, %d prosody and %d parameter generator steps (%d waits), %d hand-overs of %d samples, %d words and %d bookmarks",
		runs, counts["threads"], counts["scheduler"], counts["Input::MakeRecords"], counts["Normalizer::Run"], counts["Lexical::Step"],
		counts["Prosody::Run"], counts["parameter generator"], counts["waits"], counts["hand-overs"], counts["samples"],
		counts["word"], counts["mark"])
}
