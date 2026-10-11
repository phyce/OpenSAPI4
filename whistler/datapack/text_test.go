package datapack_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/phyce/OpenSAPI4/whistler/datapack"
	"github.com/phyce/OpenSAPI4/whistler/internal/conformance"
)

func TestText(t *testing.T) {
	text, err := conformance.Pack(t).LoadText()
	if err != nil {
		t.Fatal(err)
	}
	if len(text.Titles) != 87 || len(text.Abbreviations) != 1415 || len(text.Starters) != 96 || len(text.NumberWords) != 10 {
		t.Errorf("%d titles, %d abbreviations, %d starters, %d number words", len(text.Titles), len(text.Abbreviations), len(text.Starters), len(text.NumberWords))
	}
	if !text.Titles["Dr."] || !text.Titles["e.g."] || !text.Abbreviations["etc."] || !text.Starters["However"] || !text.NumberWords["zero"] || text.Titles["dr."] {
		t.Errorf("word lists")
	}
	classes := map[byte]byte{'b': 1, 'a': 2, 'Y': 2, '0': 3, 0xBD: 3, '!': 4, '?': 5, '.': 6, ' ': 7, 0xA0: 7, '\'': 8, '"': 9, '(': 10, '}': 11,
		0xAB: 12, 0xBB: 13, ',': 14, 0x85: 15, '\r': 17, 0: 17, '\n': 21, 0x1B: 21, '-': 21}
	for character, class := range classes {
		if text.SentenceClasses[character] != class {
			t.Errorf("byte 0x%02x: class %d, expected %d", character, text.SentenceClasses[character], class)
		}
	}
	names := map[byte]string{'a': "eigh", '?': "question mark", '5': "five", ' ': "", 0x7F: ""}
	for character, name := range names {
		if text.CharacterNames[character] != name {
			t.Errorf("byte 0x%02x is named %q, expected %q", character, text.CharacterNames[character], name)
		}
	}
	if text.AccentFolding[0xE9] != 'e' || text.AccentFolding[0x8A] != 'S' || text.AccentFolding[0x80] != 0 {
		t.Errorf("accent folding")
	}
	if text.CharacterStyles[0] != `"Normal"` || len(text.CharacterStyles) != 17 || len(text.Contexts) != 8 || text.Contexts[2] != (datapack.TextContext{Name: `"Address"`, Value: 2}) {
		t.Errorf("tag tables %q %v", text.CharacterStyles, text.Contexts)
	}
	if len(text.PartsOfSpeech) != 5 || text.PartsOfSpeech[3] != (datapack.PartOfSpeech{Value: `"Adv"`, Escape: "\x1b@V "}) {
		t.Errorf("parts of speech %q", text.PartsOfSpeech)
	}
	phonemes := map[int]datapack.Phoneme{
		1:  {EngineName: "ao", WideName: "ao", InternalSymbol: "A", IPA: 0x254, ARPAbetName: "ao"},
		3:  {EngineName: "ae", WideName: "ae", InternalSymbol: "\x91", IPA: 0xE6, ARPAbetName: "ae"},
		8:  {EngineName: "ao", WideName: "ao", InternalSymbol: "o", IPA: 0x254, ARPAbetName: "ao"},
		12: {EngineName: "ax", WideName: "ax", InternalSymbol: "$", IPA: 0x259, ARPAbetName: "ah"},
		14: {EngineName: "aw", WideName: "aw", InternalSymbol: "@", ARPAbetName: "aw"},
		22: {EngineName: "dx", WideName: "dx", InternalSymbol: "R", IPA: 0x27E, ARPAbetName: "t"},
		40: {EngineName: "nx", WideName: "nx", InternalSymbol: "N", IPA: 0x14B, ARPAbetName: "ng"},
		41: {EngineName: "jh", WideName: "jh", InternalSymbol: "dZ", IPA: 0x2A4, ARPAbetName: "jh"},
		46: {EngineName: "_", WideName: "_", InternalSymbol: "#", IPA: 0x5F},
		47: {EngineName: "1", WideName: "1", InternalSymbol: "'", IPA: 0x2C8, ARPAbetName: "1"},
		53: {WideName: "l", IPA: 0x26B},
	}
	for code, phoneme := range phonemes {
		if text.Phonemes[code-1] != phoneme {
			t.Errorf("phonetic symbol %d: %+v, expected %+v", code, text.Phonemes[code-1], phoneme)
		}
	}
	if len(text.BestFit) != 706 || text.BestFit[0x100] != 'A' || text.BestFit[0x20AC] != 0x80 || text.BestFit['a'] != 'a' {
		t.Errorf("%d best-fit characters", len(text.BestFit))
	}
	if _, found := text.BestFit[0x4E00]; found {
		t.Errorf("U+4E00 has a best fit")
	}
	if !text.Upper['A'] || !text.Upper[0x8A] || text.Upper[0xD7] || !text.Lower[0xAA] || !text.Lower[0xFF] || text.Alphabetic['1'] || !text.Alphanumeric[0xB2] ||
		text.ToUpper[0xFF] != 0x9F || text.ToUpper[0xDF] != 0xDF || text.ToLower['A'] != 'a' {
		t.Errorf("code page 1252 classification")
	}
	entries := map[int]datapack.PhoneSetEntry{0: {IPA: []uint16{0x2C8, 'i'}, Name: "IY1"}, 39: {IPA: []uint16{'t', 0x283}, Name: "CH"},
		64: {IPA: []uint16{' '}, Name: "#"}, 65: {IPA: []uint16{'#'}, Name: "#"}, 70: {IPA: []uint16{','}, Name: ","}}
	for index, expected := range entries {
		if entry := text.PhoneSet[index]; !slices.Equal(entry.IPA, expected.IPA) || entry.Name != expected.Name {
			t.Errorf("phone set entry %d: %+v, expected %+v", index, entry, expected)
		}
	}
	if len(text.PhoneSet) != 71 {
		t.Errorf("%d phone set entries", len(text.PhoneSet))
	}
}

func testText() map[string]any {
	classes := make([]int, 256)
	names := make([]string, 256)
	for index := range classes {
		classes[index] = 21
	}
	phonemes := []map[string]any{}
	for range 52 {
		phonemes = append(phonemes, map[string]any{"engineName": "a", "wideName": "a", "internalSymbol": "a", "ipa": 0, "arpabetName": "aa"})
	}
	phonemes = append(phonemes, map[string]any{"engineName": "", "wideName": "l", "internalSymbol": "", "ipa": 0x26B, "arpabetName": ""})
	toLower := make([]rune, 256)
	toUpper := make([]rune, 256)
	for index := range 256 {
		toLower[index], toUpper[index] = rune(index), rune(index)
	}
	toLower['A'], toUpper['a'] = 'a', 'A'
	return map[string]any{
		"sentenceClasses": classes,
		"characterNames":  names,
		"accentFolding":   strings.Repeat("\u0000", 128),
		"titles":          []string{"Dr.", "Mr."},
		"abbreviations":   []string{"aé", "ab"},
		"starters":        []string{"The"},
		"numberWords":     []string{"one"},
		"characterStyles": []string{`"Normal"`},
		"contexts":        []map[string]any{{"name": `"Address"`, "value": 2}},
		"partsOfSpeech":   []map[string]any{{"value": `"N"`, "escape": "\u001b@N "}},
		"phonemes":        phonemes,
		"bestFit":         [][]any{{0x100, "A"}, {0x20AC, "\u0080"}},
		"upper":           "A",
		"lower":           "a",
		"alphabetic":      "Aa",
		"alphanumeric":    "Aa0123456789²",
		"toLower":         string(toLower),
		"toUpper":         string(toUpper),
		"phoneSet":        [][]any{{[]int{0x2C8, 0x69}, "IY1"}, {[]int{0x20}, "#"}},
	}
}

func TestDamagedText(t *testing.T) {
	load := func(description map[string]any) error {
		data, err := json.Marshal(description)
		if err != nil {
			t.Fatal(err)
		}
		pack, err := datapack.Open(writePack(t, map[string][]byte{"text.json": data}))
		if err != nil {
			return err
		}
		_, err = pack.LoadText()
		return err
	}
	if err := load(testText()); err != nil {
		t.Fatalf("valid text tables: %v", err)
	}
	set := func(key string, value any) func(map[string]any) {
		return func(description map[string]any) { description[key] = value }
	}
	at := func(key string, index int, value any) func(map[string]any) {
		return func(description map[string]any) {
			switch list := description[key].(type) {
			case []int:
				list[index] = value.(int)
			case []string:
				list[index] = value.(string)
			case []map[string]any:
				list[index] = value.(map[string]any)
			}
		}
	}
	mapping := func(key string, from byte, to rune) func(map[string]any) {
		return func(description map[string]any) {
			table := []rune(description[key].(string))
			table[from] = to
			description[key] = string(table)
		}
	}
	phoneme := func(index int, engineName string, wideName string, symbol string, ipa int) func(map[string]any) {
		return at("phonemes", index, map[string]any{"engineName": engineName, "wideName": wideName, "internalSymbol": symbol, "ipa": ipa, "arpabetName": "aa"})
	}
	arpabetName := func(name any) func(map[string]any) {
		return func(description map[string]any) {
			entry := description["phonemes"].([]map[string]any)[0]
			if name == nil {
				delete(entry, "arpabetName")
			} else {
				entry["arpabetName"] = name
			}
		}
	}
	damaged := map[string]func(map[string]any){
		"255 classes":                      set("sentenceClasses", make([]int, 255)),
		"class 0":                          at("sentenceClasses", 'a', 0),
		"class 16":                         at("sentenceClasses", 'a', 16),
		"class 22":                         at("sentenceClasses", 'a', 22),
		"255 names":                        set("characterNames", make([]string, 255)),
		"name of 30 bytes":                 at("characterNames", 'a', strings.Repeat("e", 30)),
		"name with a tab":                  at("characterNames", 'a', "ei\tgh"),
		"name with 0x80":                   at("characterNames", 'a', "\u0080"),
		"folding of 127 bytes":             set("accentFolding", strings.Repeat("\u0000", 127)),
		"folding to a digit":               set("accentFolding", "1"+strings.Repeat("\u0000", 127)),
		"titles out of order":              set("titles", []string{"Mr.", "Dr."}),
		"title twice":                      set("titles", []string{"Dr.", "Dr."}),
		"abbreviations in unsigned order":  set("abbreviations", []string{"ab", "aé"}),
		"empty starter":                    set("starters", []string{""}),
		"number word with a NUL":           set("numberWords", []string{"on\u0000e"}),
		"word past 0xff":                   set("numberWords", []string{"Ā"}),
		"no character styles":              set("characterStyles", []string{}),
		"first style not Normal":           set("characterStyles", []string{`"Whisper"`}),
		"unquoted style":                   set("characterStyles", []string{`"Normal"`, "Whisper"}),
		"unquoted context":                 set("contexts", []map[string]any{{"name": "Address", "value": 2}}),
		"part of speech without its space": set("partsOfSpeech", []map[string]any{{"value": `"N"`, "escape": "\u001b@N"}}),
		"part of speech without ESC":       set("partsOfSpeech", []map[string]any{{"value": `"N"`, "escape": " @N "}}),
		"52 phonetic symbols": func(description map[string]any) {
			description["phonemes"] = description["phonemes"].([]map[string]any)[:52]
		},
		"searchable symbol without a name": phoneme(0, "", "a", "a", 0),
		"searchable symbol without symbol": phoneme(0, "a", "a", "", 0),
		"engine name of three letters":     phoneme(0, "aaa", "a", "a", 0),
		"internal symbol 0xb0":             phoneme(0, "a", "a", "°", 0),
		"internal symbol ESC":              phoneme(0, "a", "a", "\u001b", 0),
		"IPA surrogate":                    phoneme(0, "a", "a", "a", 0xD800),
		"IPA past the BMP":                 phoneme(0, "a", "a", "a", 0x10000),
		"last symbol with an engine name":  phoneme(52, "l", "l", "", 0x26B),
		"empty wide name":                  phoneme(52, "", "", "", 0x26B),
		"no ARPAbet name":                  arpabetName(nil),
		"ARPAbet name of three letters":    arpabetName("aaa"),
		"ARPAbet name with a space":        arpabetName("a "),
		"ARPAbet name with a tab":          arpabetName("a	"),
		"best fit to a question mark":      set("bestFit", [][]any{{0x100, "?"}}),
		"best fit to NUL":                  set("bestFit", [][]any{{0x100, "\u0000"}}),
		"best fit of two bytes":            set("bestFit", [][]any{{0x100, "AA"}}),
		"best fit out of order":            set("bestFit", [][]any{{0x20AC, "\u0080"}, {0x100, "A"}}),
		"best fit of a surrogate":          set("bestFit", [][]any{{0xD800, "A"}}),
		"best fit of U+0000":               set("bestFit", [][]any{{0, "A"}}),
		"upper and lower":                  set("lower", "aA"),
		"letter missing from alphabetic":   set("alphabetic", "A"),
		"digit missing from alphanumeric":  set("alphanumeric", "Aa012345678"),
		"member twice":                     set("upper", "AA"),
		"lowercase to a non-lower":         mapping("toLower", 'A', 'x'),
		"uppercase of a non-lower":         mapping("toUpper", 'b', 'A'),
		"uppercase mapping of 255 bytes":   set("toUpper", strings.Repeat("a", 255)),
		"phone set IPA empty":              set("phoneSet", [][]any{{[]int{}, "IY1"}}),
		"phone set IPA of 11 characters":   set("phoneSet", [][]any{{[]int{0x69, 0x69, 0x69, 0x69, 0x69, 0x69, 0x69, 0x69, 0x69, 0x69, 0x69}, "IY1"}}),
		"phone set IPA with 0":             set("phoneSet", [][]any{{[]int{0x69, 0}, "IY1"}}),
		"phone set IPA past 16 bits":       set("phoneSet", [][]any{{[]int{0x10000}, "IY1"}}),
		"phone set IPA twice":              set("phoneSet", [][]any{{[]int{0x69}, "IY1"}, {[]int{0x69}, "IY0"}}),
		"phone set name empty":             set("phoneSet", [][]any{{[]int{0x69}, ""}}),
		"phone set name with a space":      set("phoneSet", [][]any{{[]int{0x69}, "I Y"}}),
		"phone set name of 11 characters":  set("phoneSet", [][]any{{[]int{0x69}, "IYIYIYIYIYI"}}),
	}
	for name, change := range damaged {
		description := testText()
		change(description)
		if err := load(description); err == nil {
			t.Errorf("%s: loaded", name)
		} else {
			t.Logf("%s: %v", name, err)
		}
	}
}
