package truvoice

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"unicode/utf16"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/internal/conformance"
	"github.com/phyce/OpenSAPI4/truvoice/text"
)

type goldenStep struct {
	Call         string  `json:"call"`
	Text         *string `json:"text"`
	Sinkless     bool    `json:"sinkless"`
	ANSI         bool    `json:"ansi"`
	CharacterSet uint32  `json:"characterSet"`
	Flags        *uint32 `json:"flags"`
	Uncounted    bool    `json:"uncounted"`
	Tolerant     bool    `json:"tolerant"`
}

type goldenCase struct {
	Name      string          `json:"name"`
	Mode      json.RawMessage `json:"mode"`
	Texts     []string        `json:"texts"`
	Steps     []goldenStep    `json:"steps"`
	Untagged  bool            `json:"untagged"`
	Inject    string          `json:"inject"`
	Lexicon   json.RawMessage `json:"lexicon"`
	Racy      bool            `json:"racy"`
	Accept    string          `json:"accept"`
	Pitch     *int64          `json:"pitch"`
	Speed     *int64          `json:"speed"`
	Volume    *int64          `json:"volume"`
	Format    [2]int          `json:"format"`
	Samples   int64           `json:"samples"`
	Digest    string          `json:"digest"`
	Events    [][]any         `json:"events"`
	Positions []goldenRuns    `json:"positions"`
}

// goldenRuns is how many of the original's runs of a case gave the same events, and the events: where each text
// started and ended, and how many word, mark and visual events came before each attribute change.
type goldenRuns struct {
	Count  int     `json:"count"`
	Events [][]any `json:"events"`
}

// The sets whose runs the engine's test replays.
var engineSets = []string{"affixes", "conversion", "dictionary", "durations", "input", "letters", "lexical", "lexicon",
	"normalizer", "parameters", "pipeline", "positions", "postlexical", "prosody", "stage", "synthesizer"}

// recorder is a sink that keeps the audio and the events as the golden data writes them.
type recorder struct {
	pcm       []byte
	texts     []string
	marks     []string
	visuals   []string
	positions []string
	position  int64
	numbers   map[int]int
	err       error
}

func (recorder *recorder) Audio(data []byte) (int, error) {
	recorder.pcm = append(recorder.pcm, data...)
	return len(data), nil
}

func (recorder *recorder) Event(event Event) error {
	if event.Sample < recorder.position && recorder.err == nil {
		recorder.err = fmt.Errorf("event %+v before sample %d, given earlier", event, recorder.position)
	}
	recorder.position = event.Sample
	number := recorder.numbers[event.Text]
	switch event.Kind {
	case TextStarted:
		recorder.texts = append(recorder.texts, fmt.Sprint("started ", number))
		recorder.positions = append(recorder.positions, fmt.Sprint("started ", number, " ", event.Sample))
	case TextDone:
		recorder.texts = append(recorder.texts, fmt.Sprint("done ", number))
		recorder.positions = append(recorder.positions, fmt.Sprint("done ", number, " ", event.Sample))
	case WordPosition:
		recorder.marks = append(recorder.marks, fmt.Sprint(event.Sample, " word ", number, " ", event.Value))
	case Bookmark:
		recorder.marks = append(recorder.marks, fmt.Sprint(event.Sample, " mark ", number, " ", event.Value))
	case Visual:
		var mouth []int
		if event.Visual.Mouth != nil {
			shape := *event.Visual.Mouth
			mouth = []int{int(shape.Height), int(shape.Width), int(shape.Upturn), int(shape.JawOpen), int(shape.UpperTeethVisible),
				int(shape.LowerTeethVisible), int(shape.TonguePosition), int(shape.LipTension)}
		}
		recorder.visuals = append(recorder.visuals, fmt.Sprint(event.Sample, " visual ", event.Visual.IPA, " ", event.Visual.Engine, " 2 ", mouth))
	}
	return nil
}

var attributeNames = map[Attribute]string{PitchAttribute: "pitch", SpeedAttribute: "speed", VolumeAttribute: "volume"}

func (recorder *recorder) attributeChanged(attribute Attribute) {
	recorder.positions = append(recorder.positions, fmt.Sprint("attribute ", attributeNames[attribute], " ",
		len(recorder.marks)+len(recorder.visuals)))
}

// expected writes the golden events as the recorder does.
func expected(golden goldenCase) (texts, marks, visuals []string) {
	for _, event := range golden.Events {
		switch event[1] {
		case "started", "done":
			texts = append(texts, fmt.Sprint(event[1], " ", event[2]))
		case "word", "mark":
			marks = append(marks, fmt.Sprint(int64(event[0].(float64)), " ", event[1], " ", event[2], " ", uint32(event[3].(float64))))
		case "visual":
			var mouth []int
			if shape, found := event[5].([]any); found {
				for _, value := range shape {
					mouth = append(mouth, int(value.(float64)))
				}
			}
			visuals = append(visuals, fmt.Sprint(int64(event[0].(float64)), " visual ", event[2], " ", event[3], " ", event[4], " ", mouth))
		}
	}
	return texts, marks, visuals
}

// leftOut says why the test leaves a run out: what later parts of the port bring, or what isn't ported.
func leftOut(golden goldenCase) string {
	switch {
	case golden.Racy:
		return "racy"
	case golden.Inject != "":
		return "Inject"
	case golden.Lexicon != nil:
		return "user lexicon"
	}
	for _, step := range golden.Steps {
		if step.Call != "Speak" && step.Call != "Queue" {
			return step.Call
		}
	}
	for _, value := range golden.texts() {
		lowered := strings.ToLower(value)
		if strings.Contains(lowered, `\prn=`) {
			return "pronunciation"
		}
		if strings.Contains(lowered, "ctx=") && (strings.Contains(lowered, "address") || strings.Contains(lowered, "mail") ||
			strings.Contains(lowered, "spreadsheet")) {
			return "pre-formatter"
		}
	}
	return ""
}

func (golden goldenCase) texts() []string {
	values := slices.Clone(golden.Texts)
	for _, step := range golden.Steps {
		if step.Text != nil {
			values = append(values, *step.Text)
		}
	}
	return values
}

func modeVoice(t *testing.T, mode json.RawMessage) int {
	var index int
	if json.Unmarshal(mode, &index) == nil {
		return index % datapack.VoiceCount
	}
	var identifier string
	if err := json.Unmarshal(mode, &identifier); err != nil || len(identifier) < 3 {
		t.Fatalf("mode %s", mode)
	}
	value, err := strconv.ParseUint(identifier[len(identifier)-3:len(identifier)-1], 16, 8)
	if err != nil {
		t.Fatalf("mode %s", mode)
	}
	return int(value) % datapack.VoiceCount
}

func acceptFormats(t *testing.T, list string) AcceptFormat {
	if list == "" {
		return nil
	}
	accepted := map[Format]bool{}
	for _, item := range strings.Split(list, ",") {
		var format Format
		if _, err := fmt.Sscanf(item, "%d/%d", &format.SampleRate, &format.Bits); err != nil {
			t.Fatalf("accept %q", list)
		}
		accepted[format] = true
	}
	return func(format Format) error {
		if accepted[format] {
			return nil
		}
		return fmt.Errorf("format %v not accepted", format)
	}
}

// speak runs the golden case's calls on a new engine: the attributes, then its texts, all in one session, or its
// steps, where a Speak waits for the speaking thread to end (the host waits after a Speak without a notification
// sink too) and a Queue doesn't.
func speak(t *testing.T, pack *datapack.Pack, golden goldenCase) (*recorder, error) {
	data, err := Load(pack)
	if err != nil {
		return nil, err
	}
	engine, err := data.NewEngineAccepting(modeVoice(t, golden.Mode), acceptFormats(t, golden.Accept))
	if err != nil {
		return nil, err
	}
	if engine.Format() != (Format{golden.Format[0], golden.Format[1]}) {
		return nil, fmt.Errorf("format %v, expected %v", engine.Format(), golden.Format)
	}
	sink := &recorder{numbers: map[int]int{}}
	engine.AttributeChanged = sink.attributeChanged
	if golden.Pitch != nil {
		engine.SetPitch(uint16(*golden.Pitch))
	}
	if golden.Speed != nil {
		engine.SetSpeed(uint32(*golden.Speed))
	}
	if golden.Volume != nil {
		engine.SetVolume(uint32(*golden.Volume))
	}
	steps := golden.Steps
	for index := range golden.Texts {
		steps = append(steps, goldenStep{Call: "Queue", Text: &golden.Texts[index]})
	}
	if golden.Steps == nil {
		steps = append(steps, goldenStep{Call: "Run"})
	}
	for number, step := range steps {
		if step.Call == "Run" {
			if err := engine.Run(sink); err != nil {
				return nil, err
			}
			continue
		}
		tagged := !golden.Untagged
		if step.Flags != nil {
			tagged = *step.Flags == text.Tagged
		}
		characterSet := text.CharacterSet(step.CharacterSet)
		var queued int
		if step.ANSI {
			characters := []rune(*step.Text)
			data := make([]byte, len(characters), len(characters)+1)
			for index, character := range characters {
				data[index] = byte(character)
			}
			if !step.Uncounted {
				data = append(data, 0)
			}
			queued, err = engine.QueueANSI(data, tagged, characterSet, !step.Sinkless)
		} else {
			characters := utf16.Encode([]rune(*step.Text))
			if !step.Uncounted {
				characters = append(characters, 0)
			}
			queued, err = engine.Queue(characters, tagged, characterSet, !step.Sinkless)
		}
		if err != nil && !step.Tolerant {
			return nil, fmt.Errorf("step %d: %v", number, err)
		}
		if err == nil {
			sink.numbers[queued] = number
		}
		if step.Call == "Speak" {
			if err := engine.Run(sink); err != nil {
				return nil, err
			}
		}
	}
	return sink, engine.Run(sink)
}

func compare(t *testing.T, pack *datapack.Pack, golden goldenCase) error {
	sink, err := speak(t, pack, golden)
	if err != nil {
		return err
	}
	if sink.err != nil {
		return sink.err
	}
	if samples := int64(len(sink.pcm) * 8 / golden.Format[1]); samples != golden.Samples {
		return fmt.Errorf("%d samples, expected %d", samples, golden.Samples)
	}
	if digest := sha256.Sum256(sink.pcm); hex.EncodeToString(digest[:]) != golden.Digest {
		return fmt.Errorf("the PCM differs")
	}
	texts, marks, visuals := expected(golden)
	if !slices.Equal(sink.texts, texts) {
		return fmt.Errorf("texts %v, expected %v", sink.texts, texts)
	}
	if !slices.Equal(sink.marks, marks) {
		return fmt.Errorf("words and bookmarks %v, expected %v", sink.marks, marks)
	}
	if index := firstDifference(sink.visuals, visuals); index >= 0 {
		return fmt.Errorf("%d visual events, expected %d; event %d: %s, expected %s", len(sink.visuals), len(visuals), index,
			at(sink.visuals, index), at(visuals, index))
	}
	return nil
}

func firstDifference(actual, expected []string) int {
	for index := range max(len(actual), len(expected)) {
		if at(actual, index) != at(expected, index) {
			return index
		}
	}
	return -1
}

func at(values []string, index int) string {
	if index < len(values) {
		return values[index]
	}
	return "none"
}

// TestEngine speaks the recorded runs of every set with the engine, each once, and compares the audio, the texts'
// starts and ends, the words and bookmarks where they stand in the audio, and the visual events where they stand.
// It leaves out the runs that need what later parts of the port bring, Inject, the user lexicon, pronunciations,
// pauses and resets, the racy runs and the runs with texts in the contexts of the pre-formatter, which isn't ported.
func TestEngine(t *testing.T) {
	pack := conformance.Pack(t)
	seen := map[string]bool{}
	leftOutCounts := map[string]int{}
	var lock sync.Mutex
	compared := 0
	cases := make(chan goldenCase)
	var workers sync.WaitGroup
	for range runtime.GOMAXPROCS(0) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for golden := range cases {
				if err := compare(t, pack, golden); err != nil {
					t.Errorf("%s: %v", golden.Name, err)
				}
			}
		}()
	}
	for _, set := range engineSets {
		conformance.EachLine(t, func(golden goldenCase) {
			key, _ := json.Marshal([]any{golden.Mode, golden.Texts, golden.Steps, golden.Untagged, golden.Accept, golden.Pitch,
				golden.Speed, golden.Volume, golden.Inject, golden.Lexicon})
			lock.Lock()
			defer lock.Unlock()
			if seen[string(key)] {
				return
			}
			seen[string(key)] = true
			if reason := leftOut(golden); reason != "" {
				leftOutCounts[reason]++
				return
			}
			compared++
			golden.Name = set + ": " + golden.Name
			cases <- golden
		}, set+".jsonl.gz")
	}
	close(cases)
	workers.Wait()
	t.Logf("%d runs compared, left out %v", compared, leftOutCounts)
}

// comparePositions compares each of the events with the value most of the original's runs that gave the same events
// in the same order had.
func comparePositions(actual []string, runs []goldenRuns) error {
	shape := func(events []string) []string {
		shapes := make([]string, len(events))
		for index, event := range events {
			shapes[index] = event[:strings.LastIndexByte(event, ' ')]
		}
		return shapes
	}
	counts := make([]map[string]int, len(actual))
	for index := range counts {
		counts[index] = map[string]int{}
	}
	matched, total := 0, 0
	for _, run := range runs {
		total += run.Count
		events := make([]string, len(run.Events))
		for index, event := range run.Events {
			events[index] = fmt.Sprint(event[0], " ", event[1], " ", event[2])
		}
		if !slices.Equal(shape(events), shape(actual)) {
			continue
		}
		matched += run.Count
		for index, event := range events {
			counts[index][event] += run.Count
		}
	}
	if matched == 0 {
		return fmt.Errorf("none of the %d runs gave the events %v in that order", total, actual)
	}
	for index, event := range actual {
		mostCommon := event
		for value, count := range counts[index] {
			if count > counts[index][mostCommon] {
				mostCommon = value
			}
		}
		if mostCommon != event {
			return fmt.Errorf("event %d: %s in %d of %d runs, %s in %d", index, event, counts[index][event], matched, mostCommon,
				counts[index][mostCommon])
		}
	}
	return nil
}

// TestPositions speaks the runs of the positions set, each of which the original spoke ten times, and compares
// where the texts' starts and ends and the attribute changes land with where most of the original's runs had them.
func TestPositions(t *testing.T) {
	pack := conformance.Pack(t)
	compared := 0
	conformance.EachLine(t, func(golden goldenCase) {
		compared++
		sink, err := speak(t, pack, golden)
		if err == nil {
			err = comparePositions(sink.positions, golden.Positions)
		}
		if err != nil {
			t.Errorf("%s: %v", golden.Name, err)
		}
	}, "positions.jsonl.gz")
	t.Logf("%d runs compared", compared)
}
