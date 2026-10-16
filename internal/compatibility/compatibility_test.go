package compatibility

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"
	"unicode/utf16"

	"github.com/phyce/OpenSAPI4/internal/voicedata"
	"github.com/phyce/OpenSAPI4/internal/wave"
)

func loadVoices(t *testing.T) *voicedata.Voices {
	root := os.Getenv("OPENSAPI4_CONFORMANCE")
	if root == "" {
		t.Skip("OPENSAPI4_CONFORMANCE is not set")
	}
	t.Setenv(voicedata.EnvironmentVariable, filepath.Join(root, "datapack"))
	voices, err := voicedata.Load()
	if err != nil {
		t.Fatal(err)
	}
	return voices
}

func TestModeNames(t *testing.T) {
	names, err := ModeNames(loadVoices(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 29 || names[0] != "Mary" || names[18] != "Mike in Space" ||
		names[19] != "Adult Male #1, American English (TruVoice)" || names[28] != "Adult Female #2, American English (TruVoice)" {
		t.Errorf("names %q", names)
	}
}

func TestOpen(t *testing.T) {
	voices := loadVoices(t)
	cases := []struct {
		name         string
		expected     string
		pitch, speed Limits
	}{
		{"Sam", "Sam", Limits{100, 50, 200}, Limits{150, 30, 450}},
		{"sam", "Mary", Limits{169, 84, 338}, Limits{170, 30, 510}},
		{"Adult Male #1, American English (TruVoice)", "Adult Male #1, American English (TruVoice)", Limits{85, 50, 400}, Limits{150, 50, 250}},
		{"Adult Male #6, American English (TruVoice)", "Adult Male #6, American English (TruVoice)", Limits{89, 50, 400}, Limits{120, 50, 250}},
		{"adult male #1, american english (truvoice)", "Mary", Limits{169, 84, 338}, Limits{170, 30, 510}},
		{"Peter", "Mary", Limits{169, 84, 338}, Limits{170, 30, 510}},
		{"Adult Male #1, American English (TruVoice) ", "Mary", Limits{169, 84, 338}, Limits{170, 30, 510}},
	}
	for _, testCase := range cases {
		voice, err := Open(voices, []byte(testCase.name))
		if err != nil {
			t.Fatal(err)
		}
		if voice.Name != testCase.expected || voice.Pitch != testCase.pitch || voice.Speed != testCase.speed {
			t.Errorf("Open(%q) = %q, pitch %v, speed %v; expected %q, %v, %v", testCase.name, voice.Name, voice.Pitch,
				voice.Speed, testCase.expected, testCase.pitch, testCase.speed)
		}
	}
}

// TestSpeakOutOfRange checks that a TruVoice pitch or speed out of range leaves the maximum the limits set.
func TestSpeakOutOfRange(t *testing.T) {
	voices := loadVoices(t)
	speak := func(pitch uint16, speed uint32) []byte {
		voice, err := Open(voices, []byte("Adult Female #2, American English (TruVoice)"))
		if err != nil {
			t.Fatal(err)
		}
		var output writeSeeker
		writer := wave.NewWriter(&output, voice.Format())
		if err := voice.Speak(pitch, speed, utf16.Encode([]rune("Hello world.")), writer); err != nil {
			t.Fatal(err)
		}
		if err := writer.Finish(); err != nil {
			t.Fatal(err)
		}
		return output.data
	}
	highest := speak(400, 250)
	if len(highest) <= wave.HeaderSize {
		t.Fatalf("%d bytes", len(highest))
	}
	if !bytes.Equal(speak(401, 251), highest) || !bytes.Equal(speak(49, 49), highest) {
		t.Error("a value out of range changed the pitch or speed")
	}
	if bytes.Equal(speak(152, 150), highest) {
		t.Error("the default pitch and speed give the highest's audio")
	}
}

type writeSeeker struct {
	data     []byte
	position int64
}

func (output *writeSeeker) Write(data []byte) (int, error) {
	end := output.position + int64(len(data))
	if end > int64(len(output.data)) {
		output.data = append(output.data, make([]byte, end-int64(len(output.data)))...)
	}
	copy(output.data[output.position:], data)
	output.position = end
	return len(data), nil
}

func (output *writeSeeker) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case 0:
		output.position = offset
	case 1:
		output.position += offset
	default:
		output.position = int64(len(output.data)) + offset
	}
	return output.position, nil
}

func TestAtoi(t *testing.T) {
	cases := map[string]int32{
		"":                     0,
		"abc":                  0,
		"100":                  100,
		" \t\n\v\f\r100":       100,
		"+100":                 100,
		"-1":                   -1,
		"100abc":               100,
		"1 00":                 1,
		"- 1":                  0,
		"+-1":                  0,
		"007":                  7,
		"2147483647":           math.MaxInt32,
		"2147483648":           math.MaxInt32,
		"-2147483648":          math.MinInt32,
		"-2147483649":          math.MinInt32,
		"4294967396":           math.MaxInt32,
		"99999999999999999999": math.MaxInt32,
		"\xa0100":              0,
	}
	for value, expected := range cases {
		if got := Atoi([]byte(value)); got != expected {
			t.Errorf("Atoi(%q) = %d, expected %d", value, got, expected)
		}
	}
}

func TestParseCommandLine(t *testing.T) {
	cases := []struct {
		line     string
		expected []string
	}{
		{``, []string{""}},
		{`sapi4out.exe`, []string{"sapi4out.exe"}},
		{`"C:\Program Files\sapi4out.exe" Sam`, []string{`C:\Program Files\sapi4out.exe`, "Sam"}},
		{`C:\a"b c"d\sapi4out.exe x`, []string{`C:\ab cd\sapi4out.exe`, "x"}},
		{`p Sam 100 150 Hello world`, []string{"p", "Sam", "100", "150", "Hello", "world"}},
		{"p  \tSam\t100   ", []string{"p", "Sam", "100"}},
		{`p "Mary in Hall" 169 "Hello, world."`, []string{"p", "Mary in Hall", "169", "Hello, world."}},
		{`p "" x`, []string{"p", "", "x"}},
		{`p a\b\\c`, []string{"p", `a\b\\c`}},
		{`p a\"b`, []string{"p", `a"b`}},
		{`p a\\"b c"`, []string{"p", `a\b c`}},
		{`p a\\\"b`, []string{"p", `a\"b`}},
		{`p "a""b" c`, []string{"p", `a"b`, "c"}},
		{`p "a"""b c"`, []string{"p", `a"b`, "c"}},
		{`p a""b`, []string{"p", "ab"}},
		{`p "He said ""hi"" there"`, []string{"p", `He said "hi" there`}},
		{`p "unterminated text`, []string{"p", "unterminated text"}},
		{`p x\`, []string{"p", `x\`}},
	}
	for _, testCase := range cases {
		parsed := parseCommandLine([]byte(testCase.line), func(byte) bool { return false })
		got := make([]string, len(parsed))
		for index, argument := range parsed {
			got[index] = string(argument)
		}
		if !reflect.DeepEqual(got, testCase.expected) {
			t.Errorf("parseCommandLine(%q) = %q, expected %q", testCase.line, got, testCase.expected)
		}
	}
}

func TestParseCommandLineLeadBytes(t *testing.T) {
	isLeadByte := func(character byte) bool { return character >= 0x81 && character <= 0x9F }
	parsed := parseCommandLine([]byte("p\x81 q \x81\" \x81\\\"x\""), isLeadByte)
	expected := [][]byte{[]byte("p\x81 q"), []byte("\x81\""), []byte("\x81\\x")}
	if !reflect.DeepEqual(parsed, expected) {
		t.Errorf("got %q, expected %q", parsed, expected)
	}
}

func TestFileName(t *testing.T) {
	pattern := regexp.MustCompile(`^[A-Za-z]{16}$`)
	for range 100 {
		if name := FileName(); !pattern.MatchString(name) {
			t.Fatalf("file name %q", name)
		}
	}
}
