package datapack_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/internal/conformance"
)

func TestNormalizer(t *testing.T) {
	normalizer, err := conformance.Pack(t).LoadNormalizer()
	if err != nil {
		t.Fatal(err)
	}
	if len(normalizer.Program) != 3076 || normalizer.Program[0] != 0x0f || len(normalizer.Strings) != 224 {
		t.Errorf("program of %d bytes, strings of %d", len(normalizer.Program), len(normalizer.Strings))
	}
	if string(normalizer.String(0)) != "%@ND" || string(normalizer.String(5)) != "aA" || normalizer.Punctuation != ` "$%&),.?@[]|~\` {
		t.Errorf("strings %q, punctuation %q", normalizer.Strings, normalizer.Punctuation)
	}
	if len(normalizer.Lists) != 22 || normalizer.Lists[0][0] != (datapack.ListEntry{Match: "million", Replacement: "&Mi1jY@N"}) ||
		len(normalizer.Lists[2]) != 101 || normalizer.Lists[18][0] != (datapack.ListEntry{Match: "an"}) {
		t.Errorf("lists %v", normalizer.Lists[:3])
	}
}

func TestChangedNormalizer(t *testing.T) {
	original, err := os.ReadFile(conformance.Path(t, "datapack", "truvoice", "normalizer.json"))
	if err != nil {
		t.Fatal(err)
	}
	block := func(index int, value string) func(map[string]any) {
		return func(content map[string]any) { content["program"].([]any)[index] = value }
	}
	tests := []struct {
		change  func(map[string]any)
		message string
	}{
		{block(0, "zz"), "program[0]: encoding/hex"},
		{block(0, "13"), "program[0]: doesn't start with a condition"},
		{block(0, "0f"), "program[0]: has no branch byte"},
		{block(1, "0f10ff"), "program[1]: has a goto at 2 without its second byte"},
		{block(1, "0f1017"), "program[1]: has an unknown action 23"},
		{block(1, "0f200f41"), "program[1]: has an unknown action 15"},
		{block(1, "080810"), "program[1]: has class 8, expected 4 to 7, 14 or 19"},
		{block(1, "0f200600"), "program[1]: ends inside an operand"},
		{block(1, "0f30060c00"), "program[1]: has register 12"},
		{block(1, "0c16000"), "program[1]: encoding/hex"},
		{block(1, "0c1600"), "program[1]: has list 22"},
		{block(1, "022000"), "program[1]: has flag 32"},
		{block(1, "1100e000"), "program[1]: has string offset 224, which starts no string"},
		{block(1, "1100000000"), "program[1]: has 1 bytes after its actions"},
		{block(1, "0f208001"), "program[1] goes to 0x1, which starts no block"},
		{block(2, "0f101300"), "program[2]: has 1 bytes after its actions"},
		{func(content map[string]any) {
			program := content["program"].([]any)
			program[len(program)-1] = "0f100a"
		}, "is the last and has a run that doesn't end in a goto or a return"},
		{func(content map[string]any) {
			program := content["program"].([]any)
			program[len(program)-1] = "0f20c00d"
		}, "is the last and has a run that doesn't end in a goto or a return"},
		{func(content map[string]any) { content["program"] = []any{} }, "program is empty"},
		{func(content map[string]any) { content["strings"].([]any)[3] = "\u00e9" }, "strings[3] has \"\u00e9\", not printable ASCII"},
		{func(content map[string]any) { content["punctuation"] = "\t" }, "punctuation has \"\\t\""},
		{func(content map[string]any) { content["lists"].([]any)[4] = []any{} }, "lists[4] is empty"},
		{func(content map[string]any) { content["lists"].([]any)[5].([]any)[1] = []any{"", "x"} }, "lists[5][1] has an empty word"},
		{func(content map[string]any) { content["lists"].([]any)[5].([]any)[2] = []any{"a", "\n"} }, "lists[5][2] has \"a\\n\""},
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
		if _, err := writePack(t, "normalizer.json", changed).LoadNormalizer(); err == nil || !strings.Contains(err.Error(), test.message) {
			t.Errorf("%v, expected %q", err, test.message)
		}
	}
}
