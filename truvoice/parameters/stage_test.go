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
	"github.com/phyce/OpenSAPI4/truvoice/internal/normalizerstate"
	"github.com/phyce/OpenSAPI4/truvoice/internal/prosodystate"
	"github.com/phyce/OpenSAPI4/truvoice/internal/threadstate"
	"github.com/phyce/OpenSAPI4/truvoice/lexical"
	"github.com/phyce/OpenSAPI4/truvoice/normalizer"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
	"github.com/phyce/OpenSAPI4/truvoice/prosody"
	"github.com/phyce/OpenSAPI4/truvoice/synthesizer"
)

// The stage's golden data (golden/truvoice/stage.jsonl.gz) records the calls of the original's scheduler, and of
// the stages it runs before the parameter generator, with the changes of the speaking thread's state and of the
// prosody stage's global variables around them, and each text as the speaking thread takes it. Between two steps,
// the speaking thread's input side has fed the input ring more of the text and pumped it into the escape parser.

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
	normalizer  *datapack.Normalizer
}

// stageReplay runs Go speaking threads on one recorded run: their input side, scheduler, input stage, normalizer,
// lexical stage, prosody stage, parameter generator, output stage and synthesizer. The engine queues each text
// where the original's thread took it.
type stageReplay struct {
	data        *testData
	tables      *stageTables
	userLexicon lexical.Lexicon
	records     []threadstate.Record
	position    int
	thread      *threadstate.State
	engine      *pipeline.Engine
	pipeline    *pipeline.Pipeline
	generator   *Generator
	input       *input.Stage
	normalizer  *normalizer.Stage
	pointers    *normalizerstate.Pointers
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

// next returns the next call or return record, with the state brought to it, or nil at the end. It queues the
// texts of the feed records it passes: a text without a notification sink has no text for its events.
func (replay *stageReplay) next() *threadstate.Record {
	for replay.position < len(replay.records) {
		record := &replay.records[replay.position]
		replay.position++
		if record.Kind == "feed" {
			characters, err := record.Fed()
			if err != nil {
				replay.fail("text %d: %v", record.Text, err)
			}
			queued := pipeline.QueuedText{Characters: characters, Text: &pipeline.Text{Number: record.Text}}
			if record.Sinkless {
				queued.Text = nil
			}
			replay.engine.Texts = append(replay.engine.Texts, queued)
			continue
		}
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

// text returns the text a text pointer stands for. A pointer comes first as the one to the text the speaking
// thread has just taken, so it stands for the text the Go thread took.
func (replay *stageReplay) text(pointer uint32) *pipeline.Text {
	if replay.texts[pointer] == nil && replay.input != nil {
		replay.texts[pointer] = replay.input.Text
	}
	return replay.texts[pointer]
}

// start starts a Go speaking thread with the engine's sample rate and gain as the state has them at the thread's
// first step. The prosody stage's shared state and the affixes' pointers are the process's, which every thread
// shares. The rings start with what the original's hold at its first step, the first state recorded: whatever its
// stack held, but where the thread's first turn wrote, which the Go thread's first turn writes over. A thread reads
// its rings only where it has written, and where it has written to is compared.
func (replay *stageReplay) start() {
	state := replay.thread.Bytes
	engine := replay.engine
	engine.SampleRate = int(binary.LittleEndian.Uint16(state[threadstate.SampleRate:]))
	engine.Gain = uint32(binary.LittleEndian.Uint16(state[stateGain:]))
	replay.pipeline = pipeline.New(replay.data.pipeline, synthesizer.New(replay.tables.synthesizer, engine.SampleRate), replay, engine)
	replay.generator = New(replay.data.parameters, replay.data.vowels, replay.pipeline)
	replay.pipeline.Generator = countedGenerator{replay.generator, replay.counts}
	replay.input = input.New(replay.tables.input, replay.pipeline)
	replay.normalizer = normalizer.New(replay.tables.normalizer, replay.data.parameters, replay.pipeline)
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
	copy(replay.input.Input[:], state[inputstate.Input:inputstate.InputEnd])
	copy(replay.input.Commands[:], state[inputstate.Commands:inputstate.CommandsEnd])
	replay.pipeline.Start()
	replay.counts["threads"]++
}

// end checks that the Go speaking thread ends where the original's ended.
func (replay *stageReplay) end() error {
	if replay.pipeline == nil {
		return nil
	}
	if replay.input.Turn() {
		return fmt.Errorf("the original's speaking thread ended, the Go thread went on")
	}
	replay.pipeline = nil
	return nil
}

// mapping maps the state to the Go pipeline.
func (replay *stageReplay) mapping() threadstate.Mapping {
	return threadstate.Mapping{Pipeline: replay.pipeline, Base: replay.thread.Base, Text: replay.text}
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
	replay.pointers.Store(replay.mapping(), replay.normalizer, state, replay.thread.Bytes)
	if err := threadstate.Differences(state, replay.thread.Bytes, describeStage); err != nil {
		return err
	}
	if err := threadstate.Differences(globals, prosodystate.Padded(replay.thread.Globals), prosodystate.DescribeGlobal); err != nil {
		return err
	}
	if pointer := threadstate.Dword(replay.thread.Bytes, inputstate.Text); pointer == 0 && replay.input.Text != nil ||
		pointer != 0 && replay.text(pointer) != replay.input.Text {
		return fmt.Errorf("the thread reads text %v, expected the one at %08x", replay.input.Text, pointer)
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
	if name, found := normalizerstate.Names()[offset]; found {
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

func (replay *stageReplay) Input() bool {
	return replay.runStage("Input::MakeRecords", replay.input.MakeRecords)
}
func (replay *stageReplay) Normalizer() bool {
	return replay.runStage("Normalizer::Run", replay.normalizer.Run)
}
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
	replay.normalizer.Reset()
	replay.lexical.Reset()
	replay.prosody.Reset()
}

// step goes round the Go speaking thread's loop for the original's step: its input side, its attributes and the
// step, and compares the state before the step, what the step reports and the state it leaves. The engine's
// attributes are the thread's copies at the original's step: the thread had just applied them.
func (replay *stageReplay) step() error {
	if replay.pipeline == nil {
		replay.start()
	}
	state := replay.thread.Bytes
	engine := replay.engine
	engine.Pitch = uint16(threadstate.Dword(state, threadstate.Pitch))
	engine.Speed = threadstate.Dword(state, threadstate.Speed)
	engine.Volume = threadstate.Dword(state, threadstate.Volume)
	engine.Character = threadstate.Dword(state, threadstate.Character)
	engine.Voice = int32(int16(binary.LittleEndian.Uint16(state[threadstate.OutputVoice:])))
	texts := len(engine.Texts)
	if !replay.input.Turn() {
		return fmt.Errorf("the Go speaking thread ended, the original's stepped")
	}
	if len(engine.Texts) < texts {
		replay.counts["texts"]++
	}
	replay.pipeline.ApplyAttributes()
	if err := replay.compare(); err != nil {
		return fmt.Errorf("before the step: %v", err)
	}
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
	if result&pipeline.StepIdle != 0 {
		replay.input.Busy = false
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
			if replay.counts["threads"] == 0 {
				return fmt.Errorf("no step recorded")
			}
			if err := replay.end(); err != nil {
				return fmt.Errorf("at the end: %v", err)
			}
			for _, event := range golden.Events {
				if event[0] != nil && (event[1] == "word" || event[1] == "mark") {
					replay.counts[event[1].(string)]++
				}
			}
			if err := compareMessages(replay.engine.Messages, golden); err != nil {
				return err
			}
			return compareQueue(replay.engine.Queue, golden)
		case record.Function == "Pipeline::Initialize":
			if record.Kind != "call" {
				break
			}
			if err := replay.end(); err != nil {
				return fmt.Errorf("record %d: %v", index, err)
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

// compareMessages compares the starts and ends of texts the Go threads posted with the ones the original reported,
// but for texts without a notification sink.
func compareMessages(messages []pipeline.Message, golden stageCase) error {
	var actual, expected []string
	for _, message := range messages {
		switch {
		case message.Text == nil:
		case message.Kind == pipeline.TextStarted:
			actual = append(actual, fmt.Sprint("started ", message.Text.Number))
		case message.Kind == pipeline.TextDone:
			actual = append(actual, fmt.Sprint("done ", message.Text.Number))
		}
	}
	for _, event := range golden.Events {
		if event[0] == nil && (event[1] == "started" || event[1] == "done") {
			expected = append(expected, fmt.Sprint(event[1], " ", int(event[2].(float64))))
		}
	}
	if !slices.Equal(actual, expected) {
		return fmt.Errorf("texts %v, expected %v", actual, expected)
	}
	return nil
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

// TestStage runs Go speaking threads from the texts the original's took, for every recorded step of the original's
// scheduler: it checks the state before each step, after the thread's input side fed and pumped the text, the stages
// the scheduler runs and the state before each of them, what each step and stage report and the whole state they
// leave, with the prosody stage's shared state, where each thread ends, that the audio handed over is the original's
// PCM, with the bookmarks and words where the original's came, and that the texts start and end in the original's
// order.
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
	if tables.normalizer, err = pack.LoadNormalizer(); err != nil {
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
					records: golden.Records, thread: threadstate.NewState(), engine: &pipeline.Engine{}, texts: map[uint32]*pipeline.Text{},
					pointers: normalizerstate.NewPointers(), counts: map[string]int{}}
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
	t.Logf("%d runs, %d speaking threads, %d texts: %d scheduler steps, %d input, %d normalizer, %d lexical, %d prosody and %d parameter generator steps (%d waits), %d hand-overs of %d samples, %d words and %d bookmarks",
		runs, counts["threads"], counts["texts"], counts["scheduler"], counts["Input::MakeRecords"], counts["Normalizer::Run"], counts["Lexical::Step"],
		counts["Prosody::Run"], counts["parameter generator"], counts["waits"], counts["hand-overs"], counts["samples"],
		counts["word"], counts["mark"])
}
