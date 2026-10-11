package text

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/phyce/OpenSAPI4/whistler/internal/conformance"
)

func loadPreprocessor(t *testing.T) *Preprocessor {
	t.Helper()
	tables, err := conformance.Pack(t).LoadText()
	if err != nil {
		t.Fatal(err)
	}
	return New(tables)
}

func optionalBytes(text *string) []byte {
	if text == nil {
		return nil
	}
	return conformance.Bytes(*text)
}

func describe(data []byte) string {
	return fmt.Sprintf("%q", data)
}

func TestPreprocessing(t *testing.T) {
	preprocessor := loadPreprocessor(t)
	cases := conformance.ReadLines[conformance.TextCase](t, "golden", "text", "texts.jsonl.gz")
	failures := 0
	segments := 0
	steps := 0
	check := func(textCase *conformance.TextCase, condition bool, format string, arguments ...any) {
		if condition {
			return
		}
		failures++
		if failures <= 20 {
			t.Errorf("%s (tagged %v, set %d, context %d): "+format, append([]any{textCase.Name, textCase.Tagged, textCase.CharacterSet, textCase.ContextBefore}, arguments...)...)
		}
	}
	for index := range cases {
		if failures > 20 {
			t.FailNow()
		}
		textCase := &cases[index]
		prepared := preprocessor.Prepare(textCase.Units(t), textCase.Tagged, CharacterSet(textCase.CharacterSet), textCase.ContextBefore == addressContext)
		check(textCase, bytes.Equal(prepared.ANSI, conformance.Bytes(textCase.ANSI)) && prepared.OneCharacter == textCase.OneCharacter,
			"ANSI %s %v, expected %q %v", describe(prepared.ANSI), prepared.OneCharacter, textCase.ANSI, textCase.OneCharacter)
		check(textCase, bytes.Equal(prepared.Tagged, conformance.Bytes(textCase.TaggedText)), "tagged %s, expected %q", describe(prepared.Tagged), textCase.TaggedText)
		controls := make([]conformance.TextControl, len(prepared.Controls))
		for index, control := range prepared.Controls {
			controls[index] = conformance.TextControl{Kind: int(control.Kind), Value: string(control.Value)}
		}
		expectedControls := make([]conformance.TextControl, len(textCase.Controls))
		for index, control := range textCase.Controls {
			expectedControls[index] = conformance.TextControl{Kind: control.Kind, Value: string(conformance.Bytes(control.Value))}
		}
		check(textCase, fmt.Sprint(controls) == fmt.Sprint(expectedControls), "controls %q, expected %q", controls, expectedControls)
		check(textCase, prepared.AddressContext == (textCase.ContextAfter == addressContext), "address context %v, expected %d", prepared.AddressContext, textCase.ContextAfter)
		check(textCase, bytes.Equal(prepared.Joined, optionalBytes(textCase.Joined)) && (prepared.Joined == nil) == (textCase.Joined == nil),
			"joined %s, expected %v", describe(prepared.Joined), textCase.Joined)
		check(textCase, bytes.Equal(prepared.Rewritten, optionalBytes(textCase.Rewritten)) && (prepared.Rewritten == nil) == (textCase.Rewritten == nil),
			"rewritten %s, expected %v", describe(prepared.Rewritten), textCase.Rewritten)
		check(textCase, bytes.Equal(prepared.Final, conformance.Bytes(textCase.Final)), "final %s, expected %q", describe(prepared.Final), textCase.Final)
		if prepared.Joined != nil {
			sentences := prepared.Joined
			if prepared.Rewritten != nil {
				sentences = prepared.Rewritten
			}
			var trace []breakerStep
			preprocessor.splitSentences(sentences, &trace)
			first := min(len(trace), len(textCase.BreakerTrace))
			for index := range first {
				if trace[index] != breakerStep(textCase.BreakerTrace[index]) {
					first = index
					break
				}
			}
			check(textCase, first == len(trace) && first == len(textCase.BreakerTrace), "sentence breaker step %d of %d (expected %d) differs: %v",
				first, len(trace), len(textCase.BreakerTrace), trace[first:min(first+1, len(trace))])
			steps += len(trace)
		}
		actual := Segments(prepared.Final)
		matches := len(actual) == len(textCase.Segments)
		for index := 0; matches && index < len(actual); index++ {
			expected := textCase.Segments[index]
			matches = bytes.Equal(actual[index].Text, conformance.Bytes(expected.Text)) && (actual[index].Pitch == nil) == (expected.Pitch == nil) &&
				bytes.Equal(actual[index].Pitch, optionalBytes(expected.Pitch))
		}
		check(textCase, matches, "segments %q, expected %v", actual, textCase.Segments)
		segments += len(actual)
	}
	t.Logf("%d texts, %d segments, %d sentence breaker steps", len(cases), segments, steps)
	if len(cases) < 27000 {
		t.Errorf("only %d texts", len(cases))
	}
	if failures > 0 {
		t.Errorf("%d differences", failures)
	}
}

func TestSegments(t *testing.T) {
	cases := []struct {
		final    string
		expected []Segment
	}{
		{"", []Segment{{Text: []byte{}}}},
		{"a \r\n", []Segment{{Text: []byte("a \r\n")}}},
		{"\x1bAP a\x1bI120 \x1bN1 b \r\n", []Segment{{Text: []byte("\x1bAP a")}, {Pitch: []byte("120"), Text: []byte("\x1bN1 b \r\n")}}},
		{"a\x1bI0 \x1bI5 ", []Segment{{Text: []byte("a")}, {Pitch: []byte("0"), Text: []byte{}}, {Pitch: []byte("5"), Text: []byte{}}}},
		{"a\x1bI7", []Segment{{Text: []byte("a")}, {Pitch: []byte("7"), Text: []byte{}}}},
	}
	for _, testCase := range cases {
		actual := Segments([]byte(testCase.final))
		if fmt.Sprintf("%q", actual) != fmt.Sprintf("%q", testCase.expected) {
			t.Errorf("%q: %q, expected %q", testCase.final, actual, testCase.expected)
		}
	}
}

// TestUnusualTexts runs random texts of tags, escapes, line breaks, quotes, brackets, surrogates and
// other code units through every character set; none may make the preprocessing fail.
func TestUnusualTexts(t *testing.T) {
	preprocessor := loadPreprocessor(t)
	random := rand.New(rand.NewPCG(18, 6))
	pieces := []string{"\\", "\\Pau=", "\\Pit=9\\", "\\Btp=", "\\Chr=\"", "\\Ctx=\"Address\"\\", "\\Prt=\"", "\"", "'", "(", ")", "[", "]", "{",
		"}", ".", "...", " . . .", "!", "?", ",", ":-)", ";", "~", "*", "@", "<", ">", "-", "_", "+", "\r", "\n", "\t", " ", "  ", "\x1b",
		"a", "Dr.", "U.S.", "1234", "www.x.com", "hh", "ax", "1", "2", "thh", "\u00e9", "\u00ab", "\u00bb", "\u2026", "\u0259", "\u02c8"}
	for count := range 20000 {
		var units []uint16
		for range random.IntN(60) {
			if random.IntN(8) == 0 {
				units = append(units, uint16(random.IntN(0x10000)))
				continue
			}
			for _, character := range pieces[random.IntN(len(pieces))] {
				units = append(units, uint16(character))
			}
		}
		if count%50 == 0 {
			for range 400 {
				units = append(units, 'w', ' ')
			}
		}
		characterSet := []CharacterSet{UnicodeText, UnicodeIPA, ANSIText, ANSIPhonetic}[random.IntN(4)]
		prepared := preprocessor.Prepare(units, random.IntN(4) != 0, characterSet, random.IntN(3) == 0)
		Segments(prepared.Final)
	}
}
