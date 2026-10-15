package text_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
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
}

type goldenRecord struct {
	Kind  string `json:"kind"`
	Bytes string `json:"bytes"`
}

type goldenCase struct {
	Name     string          `json:"name"`
	Mode     json.RawMessage `json:"mode"`
	Texts    []string        `json:"texts"`
	Steps    []goldenStep    `json:"steps"`
	Untagged bool            `json:"untagged"`
	Inject   string          `json:"inject"`
	Records  []goldenRecord  `json:"records"`
}

type tables struct {
	text     *datapack.Text
	modes    *datapack.Modes
	pipeline *datapack.Pipeline
}

// counts are what the replays met: texts compared, texts left out because the original pre-formats them, and
// cases left out because an Inject call changes what the conversion reads.
type counts struct {
	texts, preFormatted, injected, failed int
}

var (
	pronunciation = regexp.MustCompile(`\x1b\[\d+;\d+;\d+;\d+n`)
	voiceCommand  = regexp.MustCompile(`\x1b\[(\d+)V`)
	contextChange = regexp.MustCompile(`\x1b\[(\d)X`)
)

// The address and e-mail contexts' empty text, as the original's pre-formatter gives it.
const emptyFormatted = "\x1b[7F\x1b[8F\x1b[7N\x1b[8N"

// TestConversion replays the TextData calls of the conversion's golden data, and of the other sets' cases, through
// a converter per case, and compares what it queues with the original's feed records. \Prn= tags' numbers are
// the original's record addresses, so they aren't compared.
func TestConversion(t *testing.T) {
	pack := conformance.Pack(t)
	var loaded tables
	var err error
	if loaded.text, err = pack.LoadText(); err != nil {
		t.Fatal(err)
	}
	if loaded.modes, err = pack.LoadModes(); err != nil {
		t.Fatal(err)
	}
	if loaded.pipeline, err = pack.LoadPipeline(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"conversion.jsonl.gz", "input.jsonl.gz", "normalizer.jsonl.gz", "stage.jsonl.gz"} {
		var total counts
		cases := 0
		conformance.EachLine(t, func(golden goldenCase) {
			cases++
			replay(t, &loaded, golden, &total)
		}, name)
		t.Logf("%s: %d cases, %d texts compared, %d failed as expected, %d left in the pre-formatter's contexts, %d cases with Inject left out",
			name, cases, total.texts, total.failed, total.preFormatted, total.injected)
	}
}

func replay(t *testing.T, loaded *tables, golden goldenCase, total *counts) {
	steps := golden.Steps
	if steps == nil {
		var flags uint32 = text.Tagged
		if golden.Untagged {
			flags = 0
		}
		for index := range golden.Texts {
			steps = append(steps, goldenStep{Call: "Speak", Text: &golden.Texts[index], Flags: &flags})
		}
	}
	if golden.Inject != "" && !strings.HasPrefix(golden.Inject, `\gain=`) {
		total.injected++
		return
	}
	for _, step := range steps {
		if step.Call == "Inject" {
			total.injected++
			return
		}
	}
	voice := modeVoice(t, golden.Mode)
	defaults := loaded.pipeline.Voices[voice]
	converter := text.New(loaded.text, loaded.modes, text.NewProcess(), int32(voice),
		text.Defaults{Pitch: uint16(defaults.Pitch), Speed: uint32(defaults.Speed), Volume: 0xffff})
	var feeds [][]byte
	for _, record := range golden.Records {
		if record.Kind == "feed" {
			data, err := hex.DecodeString(record.Bytes)
			if err != nil {
				t.Fatal(err)
			}
			feeds = append(feeds, data)
		}
	}
	var converted [][]byte
	for number, step := range steps {
		if step.Call != "Speak" && step.Call != "Queue" {
			continue
		}
		flags := uint32(text.Tagged)
		if golden.Untagged {
			flags = 0
		}
		if step.Flags != nil {
			flags = *step.Flags
		}
		context := converter.Context
		var queued []byte
		var err error
		if step.ANSI {
			characters := []rune(*step.Text)
			data := make([]byte, len(characters), len(characters)+1)
			for index, character := range characters {
				data[index] = byte(character)
			}
			if !step.Uncounted {
				data = append(data, 0)
			}
			queued, err = converter.ANSI(data, text.CharacterSet(step.CharacterSet), flags, !step.Sinkless)
		} else {
			characters := utf16.Encode([]rune(*step.Text))
			if !step.Uncounted {
				characters = append(characters, 0)
			}
			queued, err = converter.Wide(characters, text.CharacterSet(step.CharacterSet), flags, !step.Sinkless)
		}
		if err != nil {
			total.failed++
			continue
		}
		if len(feeds) == 0 {
			t.Fatalf("%s step %d: the original queued no text", golden.Name, number)
		}
		expected := feeds[0]
		feeds = feeds[1:]
		converted = append(converted, queued)
		if step.Call == "Speak" {
			for _, done := range converted {
				for _, match := range voiceCommand.FindAllSubmatch(done, -1) {
					value, _ := strconv.Atoi(string(match[1]))
					converter.Voice = int32(min(value, datapack.VoiceCount-1))
				}
			}
			converted = nil
		}
		if step.CharacterSet == uint32(text.Text) && preFormatterReached(expected, context, contextChange.MatchString(*step.Text)) {
			total.preFormatted++
			continue
		}
		total.texts++
		if got, want := pronunciation.ReplaceAll(queued, []byte("\x1b[n")), pronunciation.ReplaceAll(expected, []byte("\x1b[n")); !bytes.Equal(got, want) {
			t.Errorf("%s step %d %q:\n got  %q\n want %q", golden.Name, number, *step.Text, got, want)
		}
	}
	if len(feeds) != 0 {
		t.Errorf("%s: %d texts of the original weren't converted", golden.Name, len(feeds))
	}
}

// preFormatterReached is whether the original's text has characters in the address, e-mail or spreadsheet context,
// which its pre-formatter formats; context is the engine's context before the text. The ESC[nX of a text that has
// one of its own are the caller's, which change no context.
func preFormatterReached(original []byte, context int, typed bool) bool {
	original = bytes.TrimSuffix(original, []byte{0})
	var changes [][]int
	if !typed {
		changes = contextChange.FindAllSubmatchIndex(original, -1)
	}
	start := 0
	for _, match := range append(changes, []int{len(original), len(original), 0, 0}) {
		segment := string(original[start:match[0]])
		switch context {
		case text.AddressContext, text.EmailContext:
			if segment != emptyFormatted {
				return true
			}
		case text.SpreadsheetContext:
			if segment != "" {
				return true
			}
		}
		if match[0] < len(original) {
			context = int(original[match[2]] - '0')
		}
		start = match[1]
	}
	return false
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
