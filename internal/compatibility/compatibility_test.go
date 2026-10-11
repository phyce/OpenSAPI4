package compatibility

import (
	"math"
	"reflect"
	"regexp"
	"testing"
)

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
