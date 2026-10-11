package conformance

import (
	"bufio"
	"compress/gzip"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

// ReadLines reads a gzip-compressed file of JSON lines from the conformance data folder.
func ReadLines[T any](t testing.TB, elements ...string) []T {
	t.Helper()
	file, err := os.Open(Path(t, elements...))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(nil, 1<<26)
	var lines []T
	for scanner.Scan() {
		var line T
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return lines
}

// Bytes turns a golden string whose characters U+0000 to U+00FF stand for bytes into those bytes.
func Bytes(text string) []byte {
	data := make([]byte, 0, len(text))
	for _, character := range text {
		data = append(data, byte(character))
	}
	return data
}

// RuleSnapshot is the analyser state over cells 1..End.
type RuleSnapshot struct {
	End     int       `json:"end"`
	Tiers   [5]string `json:"tiers"`
	Anchors []int     `json:"anchors"`
}

type RuleRead struct {
	DurationStyle int      `json:"durationStyle"`
	End           int      `json:"end"`
	Tier1         string   `json:"tier1"`
	Anchors       []int    `json:"anchors"`
	Beyond        string   `json:"beyond"`
	AnchorsBeyond [][2]int `json:"anchorsBeyond"`
}

type RuleTranscription struct {
	RuleSnapshot
	Result int    `json:"result"`
	Hash   string `json:"hash"`
}

type RuleAnalysis struct {
	RuleSnapshot
	Result       int        `json:"result"`
	Start        int        `json:"start"`
	SentenceFlag int        `json:"sentenceFlag"`
	Durations    []int16    `json:"durations"`
	Points       [][3]int32 `json:"points"`
	PointIndex   []int16    `json:"pointIndex"`
	Hash         string     `json:"hash"`
}

type RuleSentence struct {
	Read        RuleRead           `json:"read"`
	AfterC      *RuleSnapshot      `json:"afterC"`
	Transcribed *RuleTranscription `json:"transcribed"`
	Analysis    *RuleAnalysis      `json:"analysis"`
	Lookups     [][3]any           `json:"lookups"`
	Error       string             `json:"error"`
}

// RuleText is a text read sentence by sentence by one front end (golden/rules/texts.jsonl.gz).
type RuleText struct {
	Name          string         `json:"name"`
	DurationStyle int            `json:"durationStyle"`
	Sentences     []RuleSentence `json:"sentences"`
}

// RuleLookup is one dictionary lookup (golden/rules/lookups.jsonl.gz).
type RuleLookup struct {
	Procedure int       `json:"procedure"`
	Word      string    `json:"word"`
	Found     bool      `json:"found"`
	End       int       `json:"end"`
	Flag      *int      `json:"flag"`
	Tiers     [5]string `json:"tiers"`
}

// TextControl is a control table entry, stored as [type, value].
type TextControl struct {
	Kind  int
	Value string
}

func (control *TextControl) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &[]any{&control.Kind, &control.Value})
}

type TextRead struct {
	Code          int        `json:"code"`
	DurationStyle int        `json:"durationStyle"`
	Consumed      int        `json:"consumed"`
	Start         int        `json:"start"`
	End           int        `json:"end"`
	NaturalEnd    int        `json:"naturalEnd"`
	Tier1         string     `json:"tier1"`
	Anchors       []int      `json:"anchors"`
	Beyond        string     `json:"beyond"`
	AnchorsBeyond [][2]int   `json:"anchorsBeyond"`
	Events        [][4]int64 `json:"events"`
	Transcribed   *struct {
		Result int    `json:"result"`
		Hash   string `json:"hash"`
	} `json:"transcribed"`
	Analysis *struct {
		Result int    `json:"result"`
		Start  int    `json:"start"`
		End    int    `json:"end"`
		Hash   string `json:"hash"`
	} `json:"analysis"`
	Error string `json:"error"`
}

type TextSegment struct {
	Pitch *string    `json:"pitch"`
	Text  string     `json:"text"`
	Reads []TextRead `json:"reads"`
}

// TextCase is one TextData text with every preprocessing stage and the reads of its segments
// (golden/text/texts.jsonl.gz).
type TextCase struct {
	Name          string        `json:"name"`
	UTF16         string        `json:"utf16"`
	Tagged        bool          `json:"tagged"`
	CharacterSet  int           `json:"characterSet"`
	ContextBefore int           `json:"contextBefore"`
	ANSI          string        `json:"ansi"`
	OneCharacter  bool          `json:"oneCharacter"`
	TaggedText    string        `json:"taggedText"`
	Controls      []TextControl `json:"controls"`
	ContextAfter  int           `json:"contextAfter"`
	Joined        *string       `json:"joined"`
	Rewritten     *string       `json:"rewritten"`
	Final         string        `json:"final"`
	Uninitialised bool          `json:"uninitialised"`
	BreakerTrace  [][18]int     `json:"breakerTrace"`
	Segments      []TextSegment `json:"segments"`
}

// Units decodes the text's UTF-16LE hexadecimal into code units.
func (textCase *TextCase) Units(t testing.TB) []uint16 {
	t.Helper()
	data, err := hex.DecodeString(textCase.UTF16)
	if err != nil || len(data)%2 != 0 {
		t.Fatalf("%s: utf16 %q", textCase.Name, textCase.UTF16)
	}
	units := make([]uint16, len(data)/2)
	for index := range units {
		units[index] = uint16(data[2*index]) | uint16(data[2*index+1])<<8
	}
	return units
}
