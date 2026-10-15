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
