package whistler

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"unicode/utf16"

	"github.com/phyce/OpenSAPI4/whistler/internal/conformance"
	"github.com/phyce/OpenSAPI4/whistler/text"
)

const blockSize = 8192

type goldenCase struct {
	Number          int               `json:"number"`
	Group           string            `json:"group"`
	Mode            string            `json:"mode"`
	Tagged          bool              `json:"tagged"`
	CharacterSet    int               `json:"characterSet"`
	Texts           []string          `json:"texts"`
	Attributes      map[string]uint32 `json:"attributes"`
	Accept          [][2]int          `json:"accept"`
	Format          *[2]int           `json:"format"`
	Faults          map[string]int    `json:"faults"`
	Samples         int64             `json:"samples"`
	Digest          string            `json:"digest"`
	Blocks          []string          `json:"blocks"`
	Events          []json.RawMessage `json:"events"`
	AttributeEvents struct {
		Pitch       []string `json:"pitch"`
		SpeedVolume []string `json:"speedVolume"`
	} `json:"attributeEvents"`
}

type recorder struct {
	digest       hash.Hash
	block        hash.Hash
	sampleSize   int
	blockLength  int
	blocks       []string
	samples      int64
	largestPiece int
	events       []string
	pitch        []string
	speedVolume  []string
}

func newRecorder() *recorder {
	return &recorder{digest: sha256.New(), block: sha256.New(), sampleSize: 2}
}

func (recorder *recorder) Audio(data []byte) (int, error) {
	taken := len(data)
	recorder.largestPiece = max(recorder.largestPiece, len(data)/recorder.sampleSize)
	recorder.samples += int64(len(data) / recorder.sampleSize)
	recorder.digest.Write(data)
	for len(data) > 0 {
		piece := data[:min(len(data), blockSize*recorder.sampleSize-recorder.blockLength)]
		recorder.block.Write(piece)
		recorder.blockLength += len(piece)
		data = data[len(piece):]
		if recorder.blockLength == blockSize*recorder.sampleSize {
			recorder.endBlock()
		}
	}
	return taken, nil
}

func (recorder *recorder) endBlock() {
	recorder.blocks = append(recorder.blocks, hex.EncodeToString(recorder.block.Sum(nil))[:16])
	recorder.block.Reset()
	recorder.blockLength = 0
}

func (recorder *recorder) Event(event Event) error {
	var fields []any
	switch event.Kind {
	case TextStarted:
		fields = []any{event.Sample, "started", event.Text}
	case TextDone:
		fields = []any{event.Sample, "done", event.Text}
	case WordPosition:
		fields = []any{event.Sample, "word", event.Text, event.Value}
	case Bookmark:
		fields = []any{event.Sample, "mark", event.Text, event.Value}
	case Visual:
		phoneme := uint16(event.Visual.Engine)
		if phoneme >= 0x80 {
			phoneme |= 0xFF00
		}
		mouth := event.Visual.Mouth
		fields = []any{event.Sample, "visual", event.Visual.IPA, phoneme, []byte{mouth.Height, mouth.Width, mouth.Upturn, mouth.JawOpen,
			mouth.UpperTeethVisible, mouth.LowerTeethVisible, mouth.TonguePosition, mouth.LipTension}}
	}
	recorder.events = append(recorder.events, canonical(fields))
	return nil
}

func (recorder *recorder) attributeChanged(attribute Attribute) {
	switch attribute {
	case PitchAttribute:
		recorder.pitch = append(recorder.pitch, "pitch")
	case SpeedAttribute:
		recorder.speedVolume = append(recorder.speedVolume, "speed")
	case VolumeAttribute:
		recorder.speedVolume = append(recorder.speedVolume, "volume")
	}
}

func canonical(value any) string {
	if bytes, isBytes := value.([]byte); isBytes {
		numbers := make([]int, len(bytes))
		for index, number := range bytes {
			numbers[index] = int(number)
		}
		value = numbers
	}
	if fields, isFields := value.([]any); isFields {
		for index, field := range fields {
			if bytes, isBytes := field.([]byte); isBytes {
				numbers := make([]int, len(bytes))
				for position, number := range bytes {
					numbers[position] = int(number)
				}
				fields[index] = numbers
			}
		}
	}
	data, _ := json.Marshal(value)
	return string(data)
}

func canonicalJSON(data json.RawMessage) string {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return string(data)
	}
	return canonical(value)
}

func loadData(t *testing.T) *Data {
	t.Helper()
	data, err := Load(conformance.Pack(t))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func (golden *goldenCase) texts(t *testing.T) [][]uint16 {
	return decodeTexts(t, golden.Texts)
}

// decodeTexts reads texts written as UTF-16LE in hexadecimal.
func decodeTexts(t *testing.T, encoded []string) [][]uint16 {
	texts := make([][]uint16, len(encoded))
	for index, value := range encoded {
		bytes, err := hex.DecodeString(value)
		if err != nil || len(bytes)%2 != 0 {
			t.Errorf("text %q", value)
			continue
		}
		texts[index] = make([]uint16, len(bytes)/2)
		for position := range texts[index] {
			texts[index][position] = binary.LittleEndian.Uint16(bytes[2*position:])
		}
	}
	return texts
}

var errRefused = errors.New("format refused")

// accepting is a destination that takes only the formats listed, or every format when there is no list.
func accepting(formats [][2]int) AcceptFormat {
	if formats == nil {
		return nil
	}
	return func(format Format) error {
		for _, accepted := range formats {
			if format == (Format{SampleRate: accepted[0], Bits: accepted[1]}) {
				return nil
			}
		}
		return errRefused
	}
}

// engine makes the case's engine: by the mode's name, or with Select by a mode ID.
func (golden *goldenCase) engine(data *Data) (*Engine, error) {
	if strings.HasPrefix(golden.Mode, "{") {
		return data.SelectAccepting(golden.Mode, accepting(golden.Accept))
	}
	mode, found := data.Mode(golden.Mode)
	if !found {
		return nil, fmt.Errorf("no mode %q", golden.Mode)
	}
	return data.NewEngineAccepting(mode, accepting(golden.Accept))
}

// runCase speaks the case's texts. It returns no recorder when the engine can't be made because the
// destination refuses every format, as the case expects.
func runCase(data *Data, golden *goldenCase, texts [][]uint16) (*recorder, error) {
	engine, err := golden.engine(data)
	if golden.Accept != nil && golden.Format == nil {
		if err == nil {
			return nil, fmt.Errorf("the engine took %+v, expected every format refused", engine.Format())
		}
		if !errors.Is(err, errRefused) {
			return nil, err
		}
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if golden.Format != nil && engine.Format() != (Format{SampleRate: golden.Format[0], Bits: golden.Format[1]}) {
		return nil, fmt.Errorf("format %+v, expected %v", engine.Format(), *golden.Format)
	}
	recorder := newRecorder()
	recorder.sampleSize = engine.Format().Bits / 8
	engine.AttributeChanged = recorder.attributeChanged
	if pitch, found := golden.Attributes["pitch"]; found {
		engine.SetPitch(uint16(pitch))
	}
	if speed, found := golden.Attributes["speed"]; found {
		engine.SetSpeed(speed)
	}
	if volume, found := golden.Attributes["volume"]; found {
		engine.SetVolume(volume)
	}
	for _, characters := range texts {
		if err := engine.Speak(characters, golden.Tagged, text.CharacterSet(golden.CharacterSet), recorder); err != nil {
			return nil, err
		}
	}
	if recorder.blockLength > 0 {
		recorder.endBlock()
	}
	return recorder, nil
}

func compareCase(golden *goldenCase, recorder *recorder) []string {
	var differences []string
	digest := hex.EncodeToString(recorder.digest.Sum(nil))
	if digest != golden.Digest || recorder.samples != golden.Samples {
		block := 0
		for block < min(len(recorder.blocks), len(golden.Blocks)) && recorder.blocks[block] == golden.Blocks[block] {
			block++
		}
		differences = append(differences, fmt.Sprintf("%d samples, expected %d; first different block of 8192 samples: %d", recorder.samples, golden.Samples, block))
	}
	expected := make([]string, len(golden.Events))
	for index, event := range golden.Events {
		expected[index] = canonicalJSON(event)
	}
	if !slices.Equal(recorder.events, expected) {
		index := 0
		for index < min(len(recorder.events), len(expected)) && recorder.events[index] == expected[index] {
			index++
		}
		differences = append(differences, fmt.Sprintf("%d events, expected %d; first difference at %d: %v, expected %v", len(recorder.events), len(expected), index,
			recorder.events[index:min(index+2, len(recorder.events))], expected[index:min(index+2, len(expected))]))
	}
	if !slices.Equal(recorder.pitch, golden.AttributeEvents.Pitch) || !slices.Equal(recorder.speedVolume, golden.AttributeEvents.SpeedVolume) {
		differences = append(differences, fmt.Sprintf("attribute changes %v %v, expected %v %v", recorder.pitch, recorder.speedVolume,
			golden.AttributeEvents.Pitch, golden.AttributeEvents.SpeedVolume))
	}
	if recorder.largestPiece > 1<<15 {
		differences = append(differences, fmt.Sprintf("a piece of %d samples", recorder.largestPiece))
	}
	return differences
}

func TestEngine(t *testing.T) {
	data := loadData(t)
	cases := conformance.ReadLines[goldenCase](t, "golden", "engine", "cases.jsonl.gz")
	if testing.Short() {
		cases = slices.DeleteFunc(cases, func(golden goldenCase) bool { return golden.Group == "long" || golden.Number%10 != 0 })
	}
	testCases(t, data, cases)
	if !testing.Short() && len(cases) < 3000 {
		t.Errorf("only %d cases", len(cases))
	}
}

// testCases speaks each case's texts and compares everything with the golden data, in parallel.
func testCases(t *testing.T, data *Data, cases []goldenCase) {
	type outcome struct {
		differences []string
		err         error
		words       int
		marks       int
	}
	outcomes := make([]outcome, len(cases))
	texts := make([][][]uint16, len(cases))
	for number := range cases {
		texts[number] = cases[number].texts(t)
	}
	numbers := make(chan int)
	var group sync.WaitGroup
	for range runtime.GOMAXPROCS(0) {
		group.Go(func() {
			for number := range numbers {
				golden := &cases[number]
				recorder, err := runCase(data, golden, texts[number])
				if err != nil || recorder == nil {
					outcomes[number].err = err
					continue
				}
				outcomes[number].differences = compareCase(golden, recorder)
				for _, event := range recorder.events {
					var fields []any
					_ = json.Unmarshal([]byte(event), &fields)
					switch fields[1] {
					case "word":
						outcomes[number].words++
					case "mark":
						outcomes[number].marks++
					}
				}
			}
		})
	}
	for number := range cases {
		numbers <- number
	}
	close(numbers)
	group.Wait()
	failures, words, marks, faults := 0, 0, 0, 0
	groups := map[string]int{}
	for number, outcome := range outcomes {
		golden := &cases[number]
		groups[golden.Group]++
		if len(golden.Faults) > 0 {
			faults++
		}
		words += outcome.words
		marks += outcome.marks
		if outcome.err == nil && len(outcome.differences) == 0 {
			continue
		}
		failures++
		if failures <= 20 {
			quoted := ""
			for _, characters := range texts[number] {
				quoted += fmt.Sprintf(" %q", string(utf16Runes(characters)))
			}
			t.Errorf("case %d (%s, %s, tagged %t, character set %d, %v):%s", golden.Number, golden.Group, golden.Mode, golden.Tagged, golden.CharacterSet,
				golden.Attributes, quoted)
			if outcome.err != nil {
				t.Errorf("  %v", outcome.err)
			}
			for _, difference := range outcome.differences {
				t.Errorf("  %s", difference)
			}
		}
	}
	t.Logf("%d cases %v (%d meet the original's faults): %d different; %d word positions, %d bookmarks", len(cases), groups, faults, failures, words, marks)
}

func utf16Runes(characters []uint16) []rune {
	runes := make([]rune, len(characters))
	for index, character := range characters {
		runes[index] = rune(character)
	}
	return runes
}

type stoppingSink struct {
	recorder *recorder
	limit    int64
}

var errStopped = errors.New("stopped")

func (sink *stoppingSink) Audio(data []byte) (int, error) {
	room := int(min(int64(len(data)), (sink.limit-sink.recorder.samples)*int64(sink.recorder.sampleSize)))
	taken, _ := sink.recorder.Audio(data[:room])
	if taken < len(data) {
		return taken, errStopped
	}
	return taken, nil
}

func (sink *stoppingSink) Event(event Event) error {
	return sink.recorder.Event(event)
}

func TestSinkStops(t *testing.T) {
	data := loadData(t)
	mode, _ := data.Mode("Mary")
	engine, err := data.NewEngine(mode)
	if err != nil {
		t.Fatal(err)
	}
	sink := &stoppingSink{recorder: newRecorder(), limit: 100000}
	if err := engine.SpeakString(`One \Pau=600000\ two.`, sink); !errors.Is(err, errStopped) {
		t.Fatalf("the sink's error came back as %v", err)
	}
	if sink.recorder.samples != sink.limit || sink.recorder.largestPiece > 1<<14 {
		t.Errorf("%d samples in pieces up to %d", sink.recorder.samples, sink.recorder.largestPiece)
	}
}

func TestInvalidCharacterSet(t *testing.T) {
	data := loadData(t)
	mode, _ := data.Mode("Mike")
	speak := func(refused []text.CharacterSet) *recorder {
		engine, err := data.NewEngine(mode)
		if err != nil {
			t.Fatal(err)
		}
		attributes := newRecorder()
		engine.AttributeChanged = attributes.attributeChanged
		for _, characterSet := range refused {
			sink := newRecorder()
			if err := engine.Speak(utf16.Encode([]rune(`\Spd=300\Hello.`)), true, characterSet, sink); !errors.Is(err, ErrInvalidCharacter) {
				t.Errorf("character set %d gave %v", characterSet, err)
			}
			if sink.samples != 0 || len(sink.events) != 0 || len(attributes.speedVolume) != 0 {
				t.Errorf("character set %d spoke", characterSet)
			}
		}
		sink := newRecorder()
		if err := engine.SpeakString("Hello, world.", sink); err != nil {
			t.Fatal(err)
		}
		return sink
	}
	refused, fresh := speak([]text.CharacterSet{2, 3, 6, -1, 0x7FFFFFFF}), speak(nil)
	if !slices.Equal(refused.events, fresh.events) || string(refused.digest.Sum(nil)) != string(fresh.digest.Sum(nil)) {
		t.Error("refused texts changed the engine")
	}
}

func TestHiddenModes(t *testing.T) {
	data := loadData(t)
	hidden := 0
	for index := range data.Modes.Modes {
		mode := &data.Modes.Modes[index]
		if !mode.Hidden() {
			if found, _ := data.Mode(mode.Name); found != mode {
				t.Errorf("mode %q was not found", mode.Name)
			}
			continue
		}
		hidden++
		if _, found := data.Mode(mode.Name); found {
			t.Errorf("hidden mode %q was found", mode.Name)
		}
		if _, err := data.NewEngine(mode); err == nil {
			t.Errorf("an engine was made with hidden mode %q", mode.Name)
		}
	}
	if hidden == 0 {
		t.Error("no hidden modes")
	}
}
