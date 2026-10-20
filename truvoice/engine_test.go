package truvoice

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	"github.com/phyce/OpenSAPI4/truvoice/lexicon"
	"github.com/phyce/OpenSAPI4/truvoice/text"
)

type goldenStep struct {
	Call          string  `json:"call"`
	Text          *string `json:"text"`
	Sinkless      bool    `json:"sinkless"`
	ANSI          bool    `json:"ansi"`
	CharacterSet  uint32  `json:"characterSet"`
	Flags         *uint32 `json:"flags"`
	Uncounted     bool    `json:"uncounted"`
	Tolerant      bool    `json:"tolerant"`
	Value         int64   `json:"value"`
	Wide          bool    `json:"wide"`
	Word          string  `json:"word"`
	Pronunciation string  `json:"pronunciation"`
	Size          uint32  `json:"size"`
}

type goldenCase struct {
	Name         string          `json:"name"`
	Mode         json.RawMessage `json:"mode"`
	Texts        []string        `json:"texts"`
	Steps        []goldenStep    `json:"steps"`
	Untagged     bool            `json:"untagged"`
	Inject       string          `json:"inject"`
	Lexicon      json.RawMessage `json:"lexicon"`
	File         *string         `json:"file"`
	LexiconFile  json.RawMessage `json:"lexiconFile"`
	Racy         bool            `json:"racy"`
	Unstable     bool            `json:"unstable"`
	PreFormatter *bool           `json:"preFormatter"`
	Accept       string          `json:"accept"`
	Pitch        *int64          `json:"pitch"`
	Speed        *int64          `json:"speed"`
	Volume       *int64          `json:"volume"`
	Format       [2]int          `json:"format"`
	Samples      int64           `json:"samples"`
	Digest       string          `json:"digest"`
	Events       [][]any         `json:"events"`
	Calls        [][]any         `json:"calls"`
	Positions    []goldenRuns    `json:"positions"`
}

// goldenRuns is how many of the original's runs of a case gave the same events, and the events: where each text
// started and ended, and how many word, mark and visual events came before each attribute change.
type goldenRuns struct {
	Count  int     `json:"count"`
	Events [][]any `json:"events"`
}

// The sets whose runs the engine's test replays.
var engineSets = []string{"affixes", "conversion", "dictionary", "durations", "inject", "input", "letters", "lexical",
	"lexicon", "normalizer", "parameters", "pause", "phoneme", "pipeline", "positions", "postlexical", "prosody",
	"pronounce", "spreadsheet", "stage", "synthesizer"}

// The calls besides Speak and Queue the test makes.
var portedCalls = []string{"Inject", "PitchGet", "PitchSet", "SpeedGet", "SpeedSet", "VolumeGet", "VolumeSet", "ModeGet",
	"Limit", "Pump", "AudioPause", "AudioResume", "AudioReset", "Phoneme", "Add", "Get", "Remove"}

// recorder is a sink that keeps the audio and the events as the golden data writes them. Like the recorded runs'
// destination, it takes at most limit bytes in all (none when limit is negative): it pauses the engine at the
// first buffer that doesn't fit with 50 bytes to spare, as the original's window stops there and the runs pause.
type recorder struct {
	pcm          []byte
	texts        []string
	marks        []string
	visuals      []string
	positions    []string
	calls        []string
	position     int64
	numbers      map[int]int
	limit        int64
	pausedBySink bool
	lexiconFile  *string
	err          error
}

// spareSpace is the room the original's window leaves in the destination.
const spareSpace = 50

func (recorder *recorder) Audio(data []byte) (int, error) {
	if recorder.limit >= 0 && recorder.limit-int64(len(recorder.pcm)) <= int64(len(data))+spareSpace {
		recorder.pausedBySink = true
		return 0, ErrPaused
	}
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
		recorder.calls = append(recorder.calls, entry("started", number))
	case TextDone:
		kind := "done"
		if event.Aborted {
			kind = "aborted"
		}
		recorder.texts = append(recorder.texts, fmt.Sprint(kind, " ", number))
		recorder.positions = append(recorder.positions, fmt.Sprint(kind, " ", number, " ", event.Sample))
		recorder.calls = append(recorder.calls, entry(kind, number))
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
	recorder.calls = append(recorder.calls, entry("attribute", attributeNames[attribute]))
}

// entry writes an entry of the calls stream as the golden data has it.
func entry(values ...any) string {
	words := make([]string, len(values))
	for index, value := range values {
		switch value := value.(type) {
		case nil:
			words[index] = "null"
		case float64:
			words[index] = strconv.FormatFloat(value, 'f', -1, 64)
		default:
			words[index] = fmt.Sprint(value)
		}
	}
	return strings.Join(words, " ")
}

const (
	succeeded       = "0x00000000"
	outOfRange      = "0x8000FFFF"
	alreadyPaused   = "0x80040502"
	notPaused       = "0x80040501"
	invalidArgument = "0x80070057"
	// The size Phoneme leaves when it fails: the recorded runs fill the SDATA with 0xDEADBEEF first.
	unwrittenSize = 0xdeadbeef
	// PitchGet writes a WORD, and the recorded runs read a DWORD they filled with 0xDEADBEEF first.
	pitchGetFill = 0xdead0000
	// What the recorded runs fill a Get's needed size and part of speech with, and its buffer, which has some bytes
	// more than the size it is given.
	unwritten   = 0xdeadbeef
	bufferFill  = 0xcc
	bufferSlack = 16
	// The message box the original shows once when its lexicon file has invalid entries: MB_OK | MB_ICONEXCLAMATION
	// | MB_TASKMODAL.
	invalidEntriesBox = 0x2030
	// bufferTooSmall is LEXERR_PRNBUFTOOSMALL.
	bufferTooSmall = "0x80040807"
)

// lexiconResults are the HRESULTs of the lexicon's errors.
var lexiconResults = map[error]string{lexicon.ErrInvalidArgument: invalidArgument, lexicon.ErrInvalidWord: "0x80040801",
	lexicon.ErrNotInLexicon: "0x80040803", lexicon.ErrInvalidPronunciation: "0x80040805",
	lexicon.ErrAlreadyInLexicon: "0x80040806"}

func lexiconResult(err error) string {
	if err == nil {
		return succeeded
	}
	return lexiconResults[err]
}

// call makes a call of a golden step other than Speak and Queue and records it in the calls stream, before the
// attribute changes it made.
func (recorder *recorder) call(engine *Engine, step goldenStep) error {
	position := len(recorder.calls)
	values := []any{"call", step.Call, succeeded}
	switch step.Call {
	case "Inject":
		engine.Inject(*step.Text)
	case "PitchGet":
		values = append(values, pitchGetFill|uint32(engine.Pitch()))
	case "SpeedGet":
		values = append(values, engine.Speed())
	case "VolumeGet":
		values = append(values, engine.Volume())
	case "PitchSet":
		if engine.SetPitch(uint16(step.Value)) != nil {
			values[2] = outOfRange
		}
	case "SpeedSet":
		if engine.SetSpeed(uint32(step.Value)) != nil {
			values[2] = outOfRange
		}
	case "VolumeSet":
		engine.SetVolume(uint32(step.Value))
	case "ModeGet":
		values = append(values, strings.ToUpper(engine.ModeInfo().ModeID))
	case "Limit":
		recorder.limit = int64(int32(uint32(step.Value)))
		values = values[:2]
		values = append(values, nil)
	case "AudioPause":
		if recorder.pausedBySink {
			recorder.pausedBySink = false
		} else if engine.Pause() != nil {
			values[2] = alreadyPaused
		}
		values = append(values, len(recorder.pcm))
	case "AudioResume":
		values = append(values, len(recorder.pcm))
		if err := engine.Resume(recorder); errors.Is(err, ErrNotPaused) {
			values[2] = notPaused
		} else if err != nil && !errors.Is(err, ErrPaused) {
			return err
		}
	case "AudioReset":
		engine.Reset(recorder)
		recorder.pausedBySink = false
		values = append(values, len(recorder.pcm))
	case "Add", "Get", "Remove":
		values = lexiconCall(engine.data.Lexicon(), step, values)
	case "Phoneme":
		tagged := step.Flags == nil || *step.Flags == text.Tagged
		characterSet := text.CharacterSet(step.CharacterSet)
		var output []byte
		var err error
		if step.ANSI {
			output, err = engine.PhonemeANSI(step.bytes(), tagged, characterSet)
		} else {
			var phonemes []uint16
			phonemes, err = engine.Phoneme(step.units(), tagged, characterSet)
			for _, unit := range phonemes {
				output = append(output, byte(unit), byte(unit>>8))
			}
		}
		if errors.Is(err, ErrInvalidArgument) {
			values[2] = invalidArgument
			values = append(values, unwrittenSize, nil)
		} else {
			values = append(values, len(output), hex.EncodeToString(output))
		}
	default:
		return fmt.Errorf("call %s", step.Call)
	}
	recorder.calls = slices.Insert(recorder.calls, position, entry(values...))
	return nil
}

// lexiconCall makes an ILexPronounce call of a golden step on the user lexicon. A Get records what it gives as the
// recorded runs do: its buffer, with the pronunciation and its NUL, or untouched; the size it needs, which it writes
// only when the buffer is too small (in characters, but in bytes for IPA); and the part of speech, 0 when it found the
// word. An ANSI Get whose wide Get fails converts what its own buffer held; its buffer isn't recorded.
func lexiconCall(user *lexicon.Lexicon, step goldenStep, values []any) []any {
	characterSet := text.CharacterSet(step.CharacterSet)
	var err error
	switch step.Call {
	case "Add":
		if step.Wide {
			err = user.Add(characterSet, wideString(step.Word), wideString(step.Pronunciation))
		} else {
			err = user.AddANSI(characterSet, ansiString(step.Word), ansiString(step.Pronunciation))
		}
	case "Remove":
		if step.Wide {
			err = user.Remove(wideString(step.Word))
		} else {
			err = user.RemoveANSI(ansiString(step.Word))
		}
	case "Get":
		var written []byte
		var needed uint32
		if step.Wide {
			var pronunciation []uint16
			pronunciation, err = user.Get(characterSet, wideString(step.Word))
			for _, unit := range append(pronunciation, 0) {
				written = append(written, byte(unit), byte(unit>>8))
			}
			needed = uint32(len(pronunciation) + 1)
			if characterSet == text.IPA {
				needed *= 2
			}
		} else {
			var pronunciation []byte
			pronunciation, err = user.GetANSI(characterSet, ansiString(step.Word))
			written = append(pronunciation, 0)
			needed = uint32(len(written))
		}
		buffer := bytes.Repeat([]byte{bufferFill}, int(step.Size)+bufferSlack)
		result, neededValue, partOfSpeech := lexiconResult(err), any(uint32(unwritten)), uint32(unwritten)
		if err == nil && step.Size < needed {
			result, neededValue = bufferTooSmall, needed
		} else if err == nil {
			copy(buffer, written)
			partOfSpeech = 0
		}
		values[2] = result
		var recorded any = hex.EncodeToString(buffer)
		if !step.Wide && result != succeeded {
			recorded = nil
		}
		return append(values, neededValue, partOfSpeech, recorded)
	}
	values[2] = lexiconResult(err)
	return values
}

// wideString is a lexicon call's word or pronunciation for the wide interface, with its NUL.
func wideString(value string) []uint16 {
	return append(utf16.Encode([]rune(value)), 0)
}

// ansiString is a lexicon call's word or pronunciation for the ANSI interface: each character's code as a byte, with
// a NUL.
func ansiString(value string) []byte {
	characters := []rune(value)
	data := make([]byte, len(characters)+1)
	for index, character := range characters {
		data[index] = byte(character)
	}
	return data
}

// savedLexicon is the user lexicon file after a run in hexadecimal: the one the original writes when it is released
// after a change, or else the file it read, if any.
func savedLexicon(user *lexicon.Lexicon, file *string) *string {
	if !user.Changed() {
		return file
	}
	var saved bytes.Buffer
	user.Write(&saved)
	written := hex.EncodeToString(saved.Bytes())
	return &written
}

// expected writes the golden events as the recorder does.
func expected(golden goldenCase) (texts, marks, visuals []string) {
	for _, event := range golden.Events {
		switch event[1] {
		case "started", "done", "aborted":
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

// leftOut says why the test leaves a run out: what later parts of the port bring, or what isn't ported. A string
// given to Inject whose first \Vol=\ is empty sets the volume the original reads from a variable it never set.
func leftOut(golden goldenCase) string {
	switch {
	case golden.Racy:
		return "racy"
	}
	for _, step := range golden.Steps {
		if step.Call != "Speak" && step.Call != "Queue" && !slices.Contains(portedCalls, step.Call) {
			return step.Call
		}
		if step.Call != "Inject" {
			continue
		}
		lowered := strings.ToLower(*step.Text)
		if index := strings.Index(lowered, `\vol=`); index >= 0 && strings.HasPrefix(lowered[index:], `\vol=\`) {
			return "uninitialized value"
		}
	}
	for _, value := range golden.texts() {
		lowered := strings.ToLower(value)
		if golden.PreFormatter == nil && strings.Contains(lowered, "ctx=") && (strings.Contains(lowered, "address") ||
			strings.Contains(lowered, "mail")) {
			return "pre-formatter"
		}
	}
	return ""
}

// texts are the texts the case speaks.
func (golden goldenCase) texts() []string {
	values := slices.Clone(golden.Texts)
	for _, step := range golden.Steps {
		if step.Call == "Speak" || step.Call == "Queue" {
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

// speak runs the golden case's calls on a new engine: the user lexicon's file, the attributes, the Inject string and
// the user lexicon's words, then its texts, all in one session, or each in a session of its own when it has a lexicon,
// or its steps, where a Speak waits for the speaking thread to end (the recorded runs wait after a Speak without a
// notification sink too) and a Queue doesn't. Texts are numbered in the order they are queued.
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
	sink := &recorder{numbers: map[int]int{}, limit: -1}
	if golden.File != nil {
		file, err := hex.DecodeString(*golden.File)
		if err != nil {
			return nil, err
		}
		if err := data.Lexicon().Read(bytes.NewReader(file)); errors.Is(err, lexicon.ErrInvalidEntries) {
			sink.calls = append(sink.calls, entry("messagebox", invalidEntriesBox))
		} else if err != nil {
			return nil, err
		}
	}
	var entries [][2]string
	if golden.Lexicon != nil {
		if err := json.Unmarshal(golden.Lexicon, &entries); err != nil {
			return nil, err
		}
	}
	for _, pair := range entries {
		if err := data.Lexicon().Add(text.EnginePhonetic, utf16.Encode([]rune(pair[0])), utf16.Encode([]rune(pair[1]))); err != nil {
			return nil, fmt.Errorf("lexicon %q: %v", pair, err)
		}
	}
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
	if golden.Inject != "" {
		engine.Inject(golden.Inject)
	}
	steps := golden.Steps
	call := "Queue"
	if golden.Lexicon != nil {
		call = "Speak"
	}
	for index := range golden.Texts {
		steps = append(steps, goldenStep{Call: call, Text: &golden.Texts[index]})
	}
	if golden.Steps == nil {
		steps = append(steps, goldenStep{Call: "Run"})
	}
	texts := 0
	for number, step := range steps {
		switch step.Call {
		case "Run":
			if err := engine.Run(sink); err != nil {
				return nil, err
			}
			continue
		case "Pump":
			if err := engine.Run(sink); err != nil && !errors.Is(err, ErrPaused) {
				return nil, err
			}
			continue
		case "Speak", "Queue":
			sink.calls = append(sink.calls, entry("call", step.Call, nil))
		default:
			if err := sink.call(engine, step); err != nil {
				return nil, fmt.Errorf("step %d: %v", number, err)
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
			queued, err = engine.QueueANSI(step.bytes(), tagged, characterSet, !step.Sinkless)
		} else {
			queued, err = engine.Queue(step.units(), tagged, characterSet, !step.Sinkless)
		}
		if err != nil && !step.Tolerant {
			return nil, fmt.Errorf("step %d: %v", number, err)
		}
		if err == nil {
			sink.numbers[queued] = texts
		}
		texts++
		if step.Call == "Speak" {
			if err := engine.Run(sink); err != nil {
				return nil, err
			}
		}
	}
	err = engine.Run(sink)
	sink.lexiconFile = savedLexicon(data.Lexicon(), golden.File)
	return sink, err
}

// bytes is the step's text for the ANSI interface, with its NUL unless it is uncounted.
func (step goldenStep) bytes() []byte {
	characters := []rune(*step.Text)
	data := make([]byte, len(characters), len(characters)+1)
	for index, character := range characters {
		data[index] = byte(character)
	}
	if !step.Uncounted {
		data = append(data, 0)
	}
	return data
}

// units is the step's text for the wide interface, with its NUL unless it is uncounted.
func (step goldenStep) units() []uint16 {
	characters := utf16.Encode([]rune(*step.Text))
	if !step.Uncounted {
		characters = append(characters, 0)
	}
	return characters
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
	if golden.LexiconFile != nil {
		var saved *string
		if err := json.Unmarshal(golden.LexiconFile, &saved); err != nil {
			return err
		}
		if (saved == nil) != (sink.lexiconFile == nil) || saved != nil && *saved != *sink.lexiconFile {
			return fmt.Errorf("the lexicon file differs")
		}
	}
	if golden.Calls == nil {
		return nil
	}
	calls := make([]string, len(golden.Calls))
	for index, values := range golden.Calls {
		calls[index] = entry(values...)
		if values[0] == "call" && values[1] == "AudioReset" {
			// The recorded runs write a call when it has returned: after the TextDataDone that AudioReset gives the
			// texts the thread hadn't all fed itself, and before those the window gives later.
			first := index
			for first > 0 && strings.HasPrefix(calls[first-1], "aborted ") {
				first--
			}
			copy(calls[first+1:index+1], calls[first:index])
			calls[first] = entry(values...)
		}
	}
	if golden.Unstable {
		sink.calls, calls = withoutPhonemes(sink.calls), withoutPhonemes(calls)
	}
	if index := firstDifference(sink.calls, calls); index >= 0 {
		return fmt.Errorf("%d calls, expected %d; call %d: %s, expected %s", len(sink.calls), len(calls), index,
			at(sink.calls, index), at(calls, index))
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

// withoutPhonemes leaves Phoneme's output out of the calls, but its size: in an unstable run, the original reads
// past the listing, whatever memory holds there.
func withoutPhonemes(calls []string) []string {
	kept := make([]string, len(calls))
	for index, call := range calls {
		if strings.HasPrefix(call, "call Phoneme ") {
			call = call[:strings.LastIndexByte(call, ' ')]
		}
		kept[index] = call
	}
	return kept
}

func at(values []string, index int) string {
	if index < len(values) {
		return values[index]
	}
	return "none"
}

// TestEngine speaks the recorded runs of every set with the engine, each once, and compares the audio, the texts'
// starts and ends, the words and bookmarks where they stand in the audio, the visual events where they stand, and,
// for the runs that keep them, the calls with what they gave between the attribute changes and the texts' starts
// and ends, and the user lexicon's file. It leaves out the runs with calls the engine doesn't have (RealTime,
// QueryInterface), an empty \Vol=\ given to Inject, the racy runs and the runs with texts in the address and e-mail
// contexts, whose pre-formatter isn't ported.
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
				golden.Speed, golden.Volume, golden.Inject, golden.Lexicon, golden.File})
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

// compareLaterPositions checks that each of the original's runs gave the events in the same order, each where the
// port has it or later, and the texts a reset aborted where the port has them.
func compareLaterPositions(actual []string, runs []goldenRuns) error {
	for _, run := range runs {
		if len(run.Events) != len(actual) {
			return fmt.Errorf("%d events, expected %d", len(actual), len(run.Events))
		}
		for index, event := range run.Events {
			place := strings.LastIndexByte(actual[index], ' ')
			value, _ := strconv.ParseInt(actual[index][place+1:], 10, 64)
			original := int64(event[2].(float64))
			if fmt.Sprint(event[0], " ", event[1]) != actual[index][:place] || original < value ||
				event[0] == "aborted" && original != value {
				return fmt.Errorf("event %d: %s, expected %v", index, actual[index], event)
			}
		}
	}
	return nil
}

// TestPositions speaks the runs of the positions set, each of which the original spoke ten times, and compares
// where the texts' starts and ends and the attribute changes land with where most of the original's runs had them.
// It also speaks the runs of the pause set, each of which the original spoke three times, and checks that in each,
// the original had them where the port has them or later (compareLaterPositions).
func TestPositions(t *testing.T) {
	pack := conformance.Pack(t)
	sets := []struct {
		name    string
		compare func([]string, []goldenRuns) error
	}{{"positions", comparePositions}, {"pause", compareLaterPositions}}
	for _, set := range sets {
		compared := 0
		conformance.EachLine(t, func(golden goldenCase) {
			compared++
			sink, err := speak(t, pack, golden)
			if err == nil {
				err = set.compare(sink.positions, golden.Positions)
			}
			if err != nil {
				t.Errorf("%s: %v", golden.Name, err)
			}
		}, set.name+".jsonl.gz")
		t.Logf("%s: %d runs compared", set.name, compared)
	}
}
