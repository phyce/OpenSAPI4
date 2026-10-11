package datapack_test

import (
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/phyce/OpenSAPI4/whistler/datapack"
	"github.com/phyce/OpenSAPI4/whistler/internal/conformance"
)

func TestRules(t *testing.T) {
	rules, err := conformance.Pack(t).LoadRules()
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string][6]int{"A": {176, 3439, 34, 1, 0, 7}, "B": {351, 7591, 45, 2, 2, 6}, "C": {1467, 29409, 86, 8, 8, 29}}
	for name, counts := range expected {
		module := rules.Modules[name]
		if module == nil {
			t.Fatalf("module %s missing", name)
		}
		bytecode := 0
		for _, rule := range module.Rules {
			bytecode += len(rule.Code)
		}
		actual := [6]int{len(module.Rules), bytecode, len(module.Classes), len(module.Series), len(module.SeriesMaps), len(module.Variables)}
		if actual != counts {
			t.Errorf("module %s: rules, bytecode, classes, series, maps, variables %v, expected %v", name, actual, counts)
		}
	}
	a, b, c := rules.Modules["A"], rules.Modules["B"], rules.Modules["C"]
	groups := map[*datapack.RuleModule]map[int]datapack.RuleGroup{
		a: {5: {109, 110}, 10: {70, 108}, 16: {147, 176}},
		b: {3: {1, 13}, 6: {14, 31}, 16: {310, 351}},
		c: {50: {708, 947}, 28: {1462, 1467}, 56: {1126, 1156}},
	}
	for module, expectedGroups := range groups {
		for group, rules := range expectedGroups {
			if module.Groups[group] != rules {
				t.Errorf("module %s group %d: %v", module.Name, group, module.Groups[group])
			}
		}
	}
	procedures := map[*datapack.RuleModule]map[int]datapack.ProcedureKind{
		a: {3: datapack.NativeProcedure, 5: datapack.GroupProcedure, 6: datapack.GroupTrueProcedure, 13: datapack.NativeProcedure},
		b: {4: datapack.NativeProcedure, 9: datapack.GroupProcedure, 11: datapack.GroupTrueProcedure},
		c: {22: datapack.NativeProcedure, 24: datapack.GroupFalseProcedure, 30: datapack.NativeProcedure, 58: datapack.GroupTrueProcedure},
	}
	for module, expectedProcedures := range procedures {
		for number, kind := range expectedProcedures {
			if module.Procedures[number] != kind {
				t.Errorf("module %s procedure %d: kind %d", module.Name, number, module.Procedures[number])
			}
		}
	}
	if code := hex.EncodeToString(a.Rule(1).Code); code != "630105ffffffff" {
		t.Errorf("module A rule 1: %s", code)
	}
	if rule := c.Rule(1392); rule.Mode() != 13 || rule.BlockCount != 67 || rule.Required1 != '#' {
		t.Errorf("module C rule 1392: %+v", rule)
	}
	if !b.Series[0].Members['q'] || b.Series[0].Indexes['q'] != 17 || b.SeriesMaps[1][16] != 'Q' {
		t.Errorf("module B series 1 or map 2 differ")
	}
	if !c.Series[4].Members[0x81] || c.Series[4].Indexes[0x81] != 16 || c.SeriesMaps[5][15] != 'u' {
		t.Errorf("module C series 5 or map 6 differ")
	}
	if c.Variables[9] != (datapack.RuleVariable{Series: 1}) || a.Variables[0] != (datapack.RuleVariable{Class: 12}) {
		t.Errorf("variables: C 10 %+v, A 1 %+v", c.Variables[9], a.Variables[0])
	}
	code := c.Rule(13).Code
	choice := strings.Index(hex.EncodeToString(code), "f6012df600f60323f600") / 2
	if found, landed := datapack.FindRuleMarker(code, choice+2, 0xF6, 2); found || landed != choice+3 {
		t.Errorf("module C rule 13: the search for alternative 2 gives %t at %d, expected the close at %d", found, landed, choice+3)
	}
	if found, landed := datapack.FindRuleMarker(code, choice+6, 0xF6, 0); found || landed != choice+8 {
		t.Errorf("module C rule 13: the search from the dead alternative gives %t at %d, expected %d", found, landed, choice+8)
	}
}

func TestFindRuleMarker(t *testing.T) {
	cases := []struct {
		code     string
		position int
		marker   byte
		number   byte
		found    bool
		landed   int
	}{
		{"f50161f50062ff", 1, 0xF5, 0, false, 3},
		{"f50161f501f50062f50063ff", 1, 0xF5, 0, false, 8},
		{"f60161f60262f60063ff", 2, 0xF6, 2, true, 3},
		{"f60161f60262f60063ff", 2, 0xF6, 3, false, 6},
		{"f60161f601f60062f60063ff", 2, 0xF6, 0, false, 8},
		{"f50161ff", 1, 0xF5, 0, false, 4},
		{"f50161f50162f500f50063f500ff", 1, 0xF5, 0, false, 11},
		{"f60161f60162f600f60263f600ff", 2, 0xF6, 2, false, 11},
		{"f50161f50162f500f50063ff", 1, 0xF5, 0, false, 12},
	}
	for _, test := range cases {
		code, _ := hex.DecodeString(test.code)
		found, landed := datapack.FindRuleMarker(code, test.position, test.marker, test.number)
		if found != test.found || landed != test.landed {
			t.Errorf("%s from %d for (%02x, %d): %t at %d, expected %t at %d", test.code, test.position, test.marker, test.number,
				found, landed, test.found, test.landed)
		}
	}
}

func testRuleModule(name string) map[string]any {
	seriesMap := make([]int, 26)
	for index := range seriesMap {
		seriesMap[index] = 'A' + index
	}
	return map[string]any{
		"name":   name,
		"groups": []map[string]any{{"group": 1, "first": 1, "last": 2}, {"group": 3, "first": 3, "last": 3}},
		"rules": []map[string]any{
			testRule("0561fffc01ffff62ff", 1),
			testRule("64ffffffff", 0),
			testRule("0561fff6012df600f60323f600fffffe03ff", 0),
		},
		"classes":    [][]int{{'a', 'b'}},
		"series":     [][][2]int{{{'a', 1}, {'b', 2}}},
		"seriesMaps": [][]int{seriesMap},
		"variables":  []map[string]any{{"class": 1}},
		"procedures": []map[string]any{{"number": 3, "kind": "groupTrue"}, {"number": 4, "kind": "native"}},
	}
}

func testRule(code string, outputLength int) map[string]any {
	return map[string]any{"code": code, "minimumLeft": 0, "minimumRight": 0, "outputLength": outputLength, "skipOnSuccess": 0,
		"blockCount": 0, "required1": 14, "required2": 14}
}

func TestDamagedRules(t *testing.T) {
	load := func(description map[string]any) error {
		data, err := json.Marshal(description)
		if err != nil {
			t.Fatal(err)
		}
		pack, err := datapack.Open(writePack(t, map[string][]byte{"rules.json": data}))
		if err != nil {
			return err
		}
		_, err = pack.LoadRules()
		return err
	}
	valid := map[string]any{"modules": []any{testRuleModule("A")}}
	if err := load(valid); err != nil {
		t.Fatalf("valid rules: %v", err)
	}
	module := func(change func(map[string]any)) func(map[string]any) {
		return func(description map[string]any) {
			change(description["modules"].([]any)[0].(map[string]any))
		}
	}
	rule := func(code string, outputLength int) func(map[string]any) {
		return module(func(description map[string]any) {
			description["rules"].([]map[string]any)[0] = testRule(code, outputLength)
		})
	}
	field := func(key string, value any) func(map[string]any) {
		return module(func(description map[string]any) { description["rules"].([]map[string]any)[0][key] = value })
	}
	set := func(key string, value any) func(map[string]any) {
		return module(func(description map[string]any) { description[key] = value })
	}
	accepted := map[string]func(map[string]any){
		"a variable bound in the focus":     rule("05fd01ffffff6164fd01ff", 3),
		"a variable bound in a context":     rule("0561fffd01ffff62fd01ff", 2),
		"a series map after a focus series": rule("05f801ffffff62f801ff", 2),
		"STOP in a forward scan":            rule("0561fffffffe02ff", 0),
	}
	for name, change := range accepted {
		description := map[string]any{"modules": []any{testRuleModule("A")}}
		change(description)
		if err := load(description); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	damaged := map[string]func(map[string]any){
		"no modules": func(description map[string]any) { description["modules"] = []any{} },
		"module twice": func(description map[string]any) {
			description["modules"] = []any{testRuleModule("A"), testRuleModule("A")}
		},
		"module without a name":          set("name", ""),
		"no rules":                       set("rules", []map[string]any{}),
		"code that isn't hex":            rule("zz", 0),
		"empty code":                     rule("", 0),
		"mode 9":                         rule("0961ffffff62ff", 1),
		"mode 0":                         rule("0061ffffff62ff", 1),
		"directive tier 6":               rule("630106ffffffff", 0),
		"directive too short":            rule("6301ffffffff", 0),
		"no-op with an operand":          rule("6401ffffffff", 0),
		"code that ends early":           rule("0561ff", 0),
		"bytes after the action":         rule("0561ffffff62ff00", 1),
		"scan mode 1 without a literal":  rule("01fc01ffffff62ff", 1),
		"empty focus in scan mode 2":     rule("02ffffff62ff", 1),
		"focus matches the edge":         rule("050effffff62ff", 1),
		"context matches the edge":       rule("0561ff0effff62ff", 1),
		"class 2":                        rule("05fc02ffffff62ff", 1),
		"series 0":                       rule("05fa00ffffff62ff", 1),
		"variable 2":                     rule("05fd02ffffff62ff", 1),
		"tier 6":                         rule("05ef0661ffffff62ff", 1),
		"unknown test":                   rule("05f40101e001f4ffffff62ff", 1),
		"test operand is the terminator": rule("05f40101f3f4f4ffffff62ff", 1),
		"f4 of maximum 0 in a focus":     rule("05f40100f361f4ffffff62ff", 1),
		"f4 of maximum 0 in a context":   rule("0561fff40000f362f4ffff62ff", 1),
		"count not closed":               rule("05ed0001f361eeffffff62ff", 1),
		"count of class 2":               rule("05ed0001fc02edffffff62ff", 1),
		"unclosed context group":         rule("0561fff50162ffff62ff", 1),
		"group closed before opened":     rule("0561fff600ffff62ff", 1),
		"optional closed by f6":          rule("0561fff50162f600ffff62ff", 1),
		"21 backtracking points":         rule("0561ff"+strings.Repeat("f501f500", 21)+"ffff62ff", 1),
		"action writes 0xb0":             rule("0561ffffffb0ff", 1),
		"action writes the edge":         rule("0561ffffff0eff", 1),
		"output length 2 for 1":          rule("0561ffffff62ff", 2),
		"unbound variable":               rule("0561fffffffd01ff", 1),
		"variable bound in a group":      rule("0561fff501fd01f500fffffd01ff", 1),
		"variable bound in an f4 of 0":   rule("05f40001fd01f4fffffffd01ff", 1),
		"series map without a series":    rule("0561fffffff801ff", 1),
		"series map 2":                   rule("05f801fffffff802ff", 1),
		"call to procedure 7":            rule("0561fffffffe07ff", 0),
		"call to procedure 0":            rule("0561fffffffe00ff", 0),
		"bytes after a call":             rule("0561fffffffe0461ff", 0),
		"STOP in a backward scan":        rule("0661fffffffe02ff", 0),
		"STOP in a backward block": module(func(description map[string]any) {
			rules := description["rules"].([]map[string]any)
			rules[0] = testRule("1061ffffff62ff", 1)
			rules[0]["blockCount"] = 1
			rules[1] = testRule("1061fffffffe02ff", 0)
		}),
		"block past the last rule": module(func(description map[string]any) {
			rules := description["rules"].([]map[string]any)
			rules[0] = testRule("0d61ffffff62ff", 1)
			rules[0]["blockCount"] = 3
		}),
		"required symbol 0x0d":          field("required1", 0x0D),
		"required symbol 0xb0":          field("required2", 0xB0),
		"minimum left 256":              field("minimumLeft", 256),
		"group past the rules":          set("groups", []map[string]any{{"group": 1, "first": 1, "last": 4}}),
		"group with first after last":   set("groups", []map[string]any{{"group": 1, "first": 2, "last": 1}}),
		"group twice":                   set("groups", []map[string]any{{"group": 1, "first": 1, "last": 1}, {"group": 1, "first": 2, "last": 2}}),
		"group 0":                       set("groups", []map[string]any{{"group": 0, "first": 1, "last": 1}}),
		"class with the edge":           set("classes", [][]int{{14, 'a'}}),
		"class with 0xb0":               set("classes", [][]int{{0xB0}}),
		"class member twice":            set("classes", [][]int{{'a', 'a'}}),
		"series index 27":               set("series", [][][2]int{{{'a', 27}}}),
		"series index 0":                set("series", [][][2]int{{{'a', 0}}}),
		"series with the edge":          set("series", [][][2]int{{{14, 1}}}),
		"series map of 25 values":       set("seriesMaps", [][]int{make([]int, 25)}),
		"series map value 256":          set("seriesMaps", [][]int{append(make([]int, 25), 256)}),
		"variable without a type":       set("variables", []map[string]any{{}}),
		"variable of class 2":           set("variables", []map[string]any{{"class": 2}}),
		"variable of class and series":  set("variables", []map[string]any{{"class": 1, "series": 1}}),
		"procedure of an unknown kind":  set("procedures", []map[string]any{{"number": 4, "kind": "macro"}}),
		"procedure 2":                   set("procedures", []map[string]any{{"number": 2, "kind": "native"}}),
		"procedure twice":               set("procedures", []map[string]any{{"number": 4, "kind": "native"}, {"number": 4, "kind": "native"}}),
		"group procedure without group": set("procedures", []map[string]any{{"number": 5, "kind": "group"}}),
	}
	for name, change := range damaged {
		description := map[string]any{"modules": []any{testRuleModule("A")}}
		change(description)
		if err := load(description); err == nil {
			t.Errorf("%s: loaded", name)
		} else {
			t.Logf("%s: %v", name, err)
		}
	}
}

func TestDictionaries(t *testing.T) {
	dictionaries, err := conformance.Pack(t).LoadDictionaries()
	if err != nil {
		t.Fatal(err)
	}
	counts := map[int]int{22: 31617, 21: 542, 25: 206, 26: 576, 27: 549, 34: 321, 35: 380, 36: 55, 53: 492, 54: 5768}
	for procedure, count := range counts {
		if dictionary := dictionaries.Dictionaries[procedure]; dictionary == nil || len(dictionary.Words) != count {
			t.Errorf("dictionary %d: %v", procedure, dictionary)
		}
	}
	if len(dictionaries.Dictionaries) != len(counts) {
		t.Errorf("%d dictionaries", len(dictionaries.Dictionaries))
	}
	fields := map[int][]datapack.DictionaryField{
		22: {{1, '!'}, {3, 0}},
		25: {{1, 0}},
		26: {{1, '"'}, {3, '"'}, {4, 0}},
		34: {{1, '#'}, {3, 0}},
		53: {{3, '#'}, {1, '#'}, {4, 0}},
	}
	for procedure, expected := range fields {
		if actual := dictionaries.Dictionaries[procedure].Fields; len(actual) != len(expected) || actual[0] != expected[0] || actual[len(actual)-1] != expected[len(expected)-1] {
			t.Errorf("dictionary %d fields %v", procedure, actual)
		}
	}
	entries := map[int]map[string]string{
		22: {"eeriest": "'i-ri-$st!N", "abacus": "'\x91-b$-k$s!N", "eisenhhower": "'Y-z$n-`h@-$r!N"},
		21: {"a": "'G$!d1!", "be": "'bi!X0!001"},
		25: {"ab": "\x91b\xa5"},
		26: {"cy": "0si\"J\\N\""},
		27: {"!": "`Eks-kl$-'mJ-S$n-pynt!!"},
		34: {"AC": "`J-'si#N"},
		35: {"Ms": "'mIz!d!"},
		36: {"a bit": "$-'bIt!A"},
		53: {"do": "H0M\\V#'du#'dO"},
		54: {"do": "'du!M0!400"},
	}
	for procedure, words := range entries {
		for word, entry := range words {
			if actual := dictionaries.Dictionaries[procedure].Words[word]; actual != entry {
				t.Errorf("dictionary %d, %q: %q, expected %q", procedure, word, actual, entry)
			}
		}
	}
	if _, found := dictionaries.Dictionaries[22].Words["The"]; found {
		t.Errorf("the main dictionary holds \"The\"")
	}
	exceptions := dictionaries.BuiltInExceptions
	if len(exceptions) != 160 || exceptions[3] != (datapack.BuiltInException{Word: "ALLAH", Pronunciation: "AA1 L AH0 "}) {
		t.Errorf("%d built-in exceptions, the fourth %+v", len(exceptions), exceptions[3])
	}
}

func testDictionaries() map[string]any {
	return map[string]any{
		"dictionaries": []map[string]any{{
			"procedure": 22,
			"fields":    []map[string]any{{"tier": 1, "separator": "!"}, {"tier": 3}},
			"words":     map[string]string{"ab": "'ab!N", "a\u0091": "'\u0091!"},
		}},
		"builtInExceptions": []map[string]string{{"word": "AA", "pronunciation": "EY1 EY1 "}},
	}
}

func TestDamagedDictionaries(t *testing.T) {
	load := func(description map[string]any) error {
		data, err := json.Marshal(description)
		if err != nil {
			t.Fatal(err)
		}
		pack, err := datapack.Open(writePack(t, map[string][]byte{"dictionaries.json": data}))
		if err != nil {
			return err
		}
		_, err = pack.LoadDictionaries()
		return err
	}
	if err := load(testDictionaries()); err != nil {
		t.Fatalf("valid dictionaries: %v", err)
	}
	dictionary := func(change func(map[string]any)) func(map[string]any) {
		return func(description map[string]any) { change(description["dictionaries"].([]map[string]any)[0]) }
	}
	fields := func(values ...map[string]any) func(map[string]any) {
		return dictionary(func(entry map[string]any) { entry["fields"] = values })
	}
	word := func(word string, entry string) func(map[string]any) {
		return dictionary(func(description map[string]any) { description["words"].(map[string]string)[word] = entry })
	}
	exception := func(word string, pronunciation string) func(map[string]any) {
		return func(description map[string]any) {
			description["builtInExceptions"] = []map[string]string{{"word": word, "pronunciation": pronunciation}}
		}
	}
	damaged := map[string]func(map[string]any){
		"procedure 2":   dictionary(func(entry map[string]any) { entry["procedure"] = 2 }),
		"procedure 256": dictionary(func(entry map[string]any) { entry["procedure"] = 256 }),
		"dictionary twice": func(description map[string]any) {
			list := description["dictionaries"].([]map[string]any)
			description["dictionaries"] = append(list, list[0])
		},
		"no fields":                      fields(),
		"four fields":                    fields(map[string]any{"tier": 1, "separator": "!"}, map[string]any{"tier": 3, "separator": "!"}, map[string]any{"tier": 4, "separator": "!"}, map[string]any{"tier": 5}),
		"field of tier 2":                fields(map[string]any{"tier": 2}),
		"tier 1 twice":                   fields(map[string]any{"tier": 1, "separator": "!"}, map[string]any{"tier": 1}),
		"last field with a separator":    fields(map[string]any{"tier": 1, "separator": "!"}, map[string]any{"tier": 3, "separator": "!"}),
		"field without a separator":      fields(map[string]any{"tier": 1}, map[string]any{"tier": 3}),
		"separator of two characters":    fields(map[string]any{"tier": 1, "separator": "!!"}, map[string]any{"tier": 3}),
		"space separator":                fields(map[string]any{"tier": 1, "separator": " "}, map[string]any{"tier": 3}),
		"empty word":                     word("", "a!N"),
		"word of 32 bytes":               word(strings.Repeat("a", 32), "a!N"),
		"word with 0x1f":                 word("a\x1f", "a!N"),
		"word with 0xff":                 word("aÿ", "a!N"),
		"word with a character past 255": word("aĀ", "a!N"),
		"entry without its separator":    word("ac", "'ac"),
		"entry with 0xb0":                word("ac", "'a°!N"),
		"entry with the edge":            word("ac", "'a\x0e!N"),
		"entry of 65 bytes":              word("ac", strings.Repeat("a", 63)+"!N"),
		"exception with a tab":           exception("AA", "EY1\tEY1"),
		"empty exception word":           exception("", "EY1"),
		"exception with 0x80":            exception("A\u0080", "EY1"),
	}
	for name, change := range damaged {
		description := testDictionaries()
		change(description)
		if err := load(description); err == nil {
			t.Errorf("%s: loaded", name)
		} else {
			t.Logf("%s: %v", name, err)
		}
	}
}
