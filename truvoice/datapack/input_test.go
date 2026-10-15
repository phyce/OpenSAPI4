package datapack_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/internal/conformance"
)

func TestInput(t *testing.T) {
	input, err := conformance.Pack(t).LoadInput()
	if err != nil {
		t.Fatal(err)
	}
	letters := func(character int) string {
		return strings.TrimRight(string(input.HighCharacters[character-datapack.FirstHighCharacter][:]), "\x00")
	}
	if letters(0xc0) != "A" || letters(0xd1) != "Nj" || letters(0xdf) != "ss" || letters(0xe9) != "ay" || letters(0xf7) != " " {
		t.Errorf("high characters %q", input.HighCharacters)
	}
	if input.FilterRates != [datapack.FilterCount]int32{25, 20, 16, 13, 13, 13, 13, 13, 13, 13} {
		t.Errorf("filter rates %v", input.FilterRates)
	}
	if input.LevelLimits[0] != 80 || input.LevelLimits[9] != 255 || input.LevelLimits[21] != 127 {
		t.Errorf("level limits %v", input.LevelLimits)
	}
	if input.TokenClasses[' '] != 0x03 || input.TokenClasses['.'] != 0x22 || input.TokenClasses['a'] != 0x04 ||
		input.TokenClasses['7'] != 0x08 || input.TokenClasses[0x1b] != 0x42 || input.TokenClasses[0xe9] != 0x04 {
		t.Errorf("token classes %v", input.TokenClasses)
	}
	if input.Symbols[0xa3] != " pound " || input.Symbols[0xf7] != " divided by " || input.Symbols[0xa4] != "" {
		t.Errorf("symbols %q", input.Symbols)
	}
	if input.Minus != "minus " || input.SplitWords[1] != [2]string{"unsent", "un sent"} || input.Meridiem[1] != "AM" ||
		input.Months[0] != [2]string{"jan", "January"} || input.Exclamation != "[C" || input.WordStart != "~" ||
		input.QuoteMarks != "!@#%*+|><:" || !strings.Contains(input.QuoteStart, "Anfang") || !strings.Contains(input.QuoteEnd, "Ende") {
		t.Errorf("tokenizer words %+v", input)
	}
}

func TestChangedInput(t *testing.T) {
	original, err := os.ReadFile(conformance.Path(t, "datapack", "truvoice", "input.json"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		change  func(map[string]any)
		message string
	}{
		{func(content map[string]any) { content["highCharacters"] = content["highCharacters"].([]any)[:63] }, "highCharacters has 63 entries"},
		{func(content map[string]any) { content["highCharacters"].([]any)[5] = "" }, "highCharacters[5] is \"\", not one or two"},
		{func(content map[string]any) { content["highCharacters"].([]any)[6] = "Aeh" }, "highCharacters[6] is \"Aeh\", not one or two"},
		{func(content map[string]any) { content["highCharacters"].([]any)[7] = "é" }, "highCharacters[7] is \"é\", not printable"},
		{func(content map[string]any) { content["filterRates"].([]any)[3] = 26 }, "filterRates[3] is 26"},
		{func(content map[string]any) { content["levelLimits"] = []any{1} }, "levelLimits has 1 values"},
		{func(content map[string]any) { content["tokenClasses"].([]any)[9] = 256 }, "tokenClasses[9] is 256"},
		{func(content map[string]any) { content["symbols"].([]any)[2] = []any{127, "x"} }, "symbols[2] is [127 x], not a character"},
		{func(content map[string]any) { content["symbols"].([]any)[2] = []any{163, "x"} }, "character 163 has a word already"},
		{func(content map[string]any) { content["symbols"].([]any)[3] = []any{200, ""} }, "symbols[3] has an empty word"},
		{func(content map[string]any) { content["minus"] = "minüs" }, "minus has \"minüs\", not ASCII"},
		{func(content map[string]any) { content["months"].([]any)[1] = []any{"mar", ""} }, "splitWords and months has an empty word"},
	}
	for _, test := range tests {
		var content map[string]any
		if err := json.Unmarshal(original, &content); err != nil {
			t.Fatal(err)
		}
		test.change(content)
		changed, err := json.Marshal(content)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writePack(t, "input.json", changed).LoadInput(); err == nil || !strings.Contains(err.Error(), test.message) {
			t.Errorf("%v, expected %q", err, test.message)
		}
	}
}
