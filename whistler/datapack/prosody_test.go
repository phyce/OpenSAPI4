package datapack_test

import (
	"encoding/json"
	"testing"

	"github.com/phyce/OpenSAPI4/whistler/datapack"
	"github.com/phyce/OpenSAPI4/whistler/internal/conformance"
)

func TestProsody(t *testing.T) {
	prosody, err := conformance.Pack(t).LoadProsody()
	if err != nil {
		t.Fatal(err)
	}
	unitsByPhones := map[int]int{}
	for unit := 1; unit <= len(prosody.UnitFrames); unit++ {
		unitsByPhones[prosody.UnitPhones(unit)]++
	}
	if len(prosody.UnitLists) != 98 || unitsByPhones[2] != 1543 || unitsByPhones[3] != 251 || unitsByPhones[4] != 48 || unitsByPhones[5] != 41 {
		t.Errorf("%d unit lists, units by phone count %v", len(prosody.UnitLists), unitsByPhones)
	}
	if list, fallback := prosody.UnitPair('#', 't', false); list != 185 || fallback != 185 {
		t.Errorf("pair #t: list %d, fallback %d", list, fallback)
	}
	for _, code := range []byte("aAEIOUeiou$@JYyO\x91") {
		if prosody.PhoneClasses[code] != 'V' {
			t.Errorf("class of %q is %q", code, prosody.PhoneClasses[code])
		}
	}
	classes := map[byte]string{'C': "DSTZfhjsvwz", 'L': "Nlmnr\xa4", 'P': "kpt", 'Q': "bdg", 'a': "\"'`", 's': " #-", 't': "~"}
	for class, codes := range classes {
		for _, code := range []byte(codes) {
			if prosody.PhoneClasses[code] != class {
				t.Errorf("class of %q is %q, expected %q", code, prosody.PhoneClasses[code], class)
			}
		}
	}
	features := map[byte]string{0x10: "fvszTDZ", 0x90: "S", 0x01: "hjlrw", 0x08: " -~", 0x80: "dpPXxMcC+"}
	for feature, codes := range features {
		for _, code := range []byte(codes) {
			if prosody.PhoneFeatures[code] != feature {
				t.Errorf("feature of %q is 0x%02x, expected 0x%02x", code, prosody.PhoneFeatures[code], feature)
			}
		}
	}
	factors := map[string][2]byte{" #,.": {120, 0}, "DZvz": {160, 140}, "bdg": {120, 120}, "kpt": {70, 70}, "Nmn": {85, 0}, "aSlh": {0, 0}}
	for codes, values := range factors {
		for _, code := range []byte(codes) {
			if prosody.FollowingFactors[code] != values[0] || prosody.AfterApproximantFactors[code] != values[1] {
				t.Errorf("factors of %q: %d, %d", code, prosody.FollowingFactors[code], prosody.AfterApproximantFactors[code])
			}
		}
	}
	first := datapack.Tone{Mark: '1', Level: 0, SecondOffset: 24, Duration: 340, TimeOffset: -200}
	sixth := datapack.Tone{Mark: '8', Level: -8, SecondOffset: 38, Duration: 120, FromPhoneEnd: true, TimeOffset: -110}
	if prosody.Tones[0] != first || prosody.Tones[5] != sixth || prosody.Tones[16].BaselineStep != 2 || prosody.Tones[19].BaselineStep != -4 {
		t.Errorf("tone records 0, 5, 16, 19: %+v, %+v, %+v, %+v", prosody.Tones[0], prosody.Tones[5], prosody.Tones[16], prosody.Tones[19])
	}
	declination := [5]datapack.Declination{{76, 30}, {80, 14}, {80, 10}, {80, 8}, {80, 8}}
	if prosody.Declination != declination {
		t.Errorf("declination %v", prosody.Declination)
	}
	vowels := 0
	for _, name := range prosody.PhoneNames {
		if name.Vowel {
			vowels++
		}
	}
	names := map[byte]datapack.PhoneName{0x91: {"AE", true}, '$': {"AH", true}, 'R': {"DX", false}, '#': {"SIL", false}, '?': {"#", false}, 'X': {"JH", false}}
	for code, name := range names {
		if prosody.PhoneNames[code] != name {
			t.Errorf("name of %q: %+v", code, prosody.PhoneNames[code])
		}
	}
	if vowels != 17 || prosody.PhoneNames['G'].Name != "" {
		t.Errorf("%d vowel names, G named %q", vowels, prosody.PhoneNames['G'].Name)
	}
	order := map[byte]int{}
	for index, code := range prosody.PhoneCodes {
		order[code] = index
	}
	if len(prosody.PhoneCodes) != 45 || len(order) != 45 || order['A'] > order['o'] || order['^'] > order['$'] {
		t.Errorf("phone code order %q", prosody.PhoneCodes)
	}
}

func testProsody() map[string]any {
	zeros := func() []int { return make([]int, 256) }
	unitClasses := zeros()
	unitClasses['#'], unitClasses['t'] = 1, 2
	pairs := make([][]int, datapack.UnitClassCount)
	for left := range pairs {
		pairs[left] = make([]int, datapack.UnitClassCount)
	}
	pairs[1][2] = 0x4001
	pairs[2][1] = 0x4001
	tones := make([]map[string]any, datapack.ToneCount)
	for index := range tones {
		tones[index] = map[string]any{"mark": 49, "level": -8, "secondOffset": 24, "duration": 340, "fromPhoneEnd": false, "timeOffset": -200, "baselineStep": 2}
	}
	declination := make([]map[string]any, datapack.SentenceTypes)
	for index := range declination {
		declination[index] = map[string]any{"floor": 80, "slope": 8}
	}
	return map[string]any{
		"phoneClasses": zeros(), "phoneFeatures": zeros(), "inherentDurations": zeros(), "minimumDurations": zeros(),
		"followingFactors": zeros(), "afterApproximantFactors": zeros(), "unitClasses": unitClasses, "unitPairs": pairs,
		"unitLists":   []map[string]any{{"list": 1, "lengths": []any{[]any{map[string]any{"phones": []int{116}, "unit": 2}}}}},
		"unitFrames":  [][]int{{3, 4}, {1, 2, 3}},
		"tones":       tones,
		"declination": declination,
		"phoneNames":  []map[string]any{{"code": 97, "name": "AA", "vowel": true}, {"code": 35, "name": "SIL", "vowel": false}},
	}
}

func TestDamagedProsody(t *testing.T) {
	load := func(description map[string]any) error {
		data, err := json.Marshal(description)
		if err != nil {
			t.Fatal(err)
		}
		pack, err := datapack.Open(writePack(t, map[string][]byte{"prosody.json": data}))
		if err != nil {
			return err
		}
		_, err = pack.LoadProsody()
		return err
	}
	if err := load(testProsody()); err != nil {
		t.Fatalf("valid prosody: %v", err)
	}
	set := func(key string, index int, value any) func(map[string]any) {
		return func(description map[string]any) { description[key].([]int)[index] = value.(int) }
	}
	setPair := func(left, right, value int) func(map[string]any) {
		return func(description map[string]any) { description["unitPairs"].([][]int)[left][right] = value }
	}
	setTone := func(key string, value any) func(map[string]any) {
		return func(description map[string]any) { description["tones"].([]map[string]any)[3][key] = value }
	}
	setList := func(list map[string]any) func(map[string]any) {
		return func(description map[string]any) {
			description["unitLists"] = append(description["unitLists"].([]map[string]any), list)
		}
	}
	setName := func(entry map[string]any) func(map[string]any) {
		return func(description map[string]any) {
			description["phoneNames"] = append(description["phoneNames"].([]map[string]any), entry)
		}
	}
	oneString := func(phones []int, unit int) []any { return []any{map[string]any{"phones": phones, "unit": unit}} }
	damaged := map[string]func(map[string]any){
		"255 phone classes":          func(description map[string]any) { description["phoneClasses"] = make([]int, 255) },
		"phone feature 256":          set("phoneFeatures", 7, 256),
		"inherent duration 40000":    set("inherentDurations", 97, 40000),
		"unit class 42":              set("unitClasses", 98, 42),
		"unit pair with both flags":  setPair(3, 4, 0x6001),
		"unit pair with bit 15":      setPair(3, 4, 0x8001),
		"default unit of 3 phones":   setPair(1, 2, 0x4002),
		"fallback of 3 phones":       setPair(5, 6, 0x4002),
		"dash fallback beyond units": setPair(5, 6, 1),
		"pair fallback beyond units": setPair(5, 6, 0x2001),
		"unit of one phone":          func(description map[string]any) { description["unitFrames"].([][]int)[0] = []int{3} },
		"unit of six phones":         func(description map[string]any) { description["unitFrames"].([][]int)[1] = []int{1, 2, 3, 4, 5, 6} },
		"unit frame count 256":       func(description map[string]any) { description["unitFrames"].([][]int)[1][2] = 256 },
		"string giving a 2-phone":    setList(map[string]any{"list": 2, "lengths": []any{oneString([]int{116}, 1)}}),
		"string of the wrong length": setList(map[string]any{"list": 2, "lengths": []any{oneString([]int{116, 116}, 2)}}),
		"repeated string":            setList(map[string]any{"list": 2, "lengths": []any{append(oneString([]int{116}, 2), oneString([]int{116}, 2)...)}}),
		"string phone 256":           setList(map[string]any{"list": 2, "lengths": []any{oneString([]int{256}, 2)}}),
		"list 0":                     setList(map[string]any{"list": 0, "lengths": []any{oneString([]int{116}, 2)}}),
		"list twice":                 setList(map[string]any{"list": 1, "lengths": []any{oneString([]int{117}, 2)}}),
		"list of four lengths":       setList(map[string]any{"list": 2, "lengths": []any{[]any{}, []any{}, []any{}, []any{}}}),
		"32 tone records":            func(description map[string]any) { description["tones"] = description["tones"].([]map[string]any)[1:] },
		"tone mark 256":              setTone("mark", 256),
		"tone duration -1":           setTone("duration", -1),
		"tone level 32768":           setTone("level", 32768),
		"four declination entries": func(description map[string]any) {
			description["declination"] = description["declination"].([]map[string]any)[1:]
		},
		"declination floor 65536":     func(description map[string]any) { description["declination"].([]map[string]any)[0]["floor"] = 65536 },
		"phone name code 0":           setName(map[string]any{"code": 0, "name": "AO", "vowel": true}),
		"phone name code twice":       setName(map[string]any{"code": 97, "name": "AO", "vowel": true}),
		"empty phone name":            setName(map[string]any{"code": 65, "name": "", "vowel": true}),
		"phone name of five letters":  setName(map[string]any{"code": 65, "name": "ABCDE", "vowel": true}),
		"phone name with a space":     setName(map[string]any{"code": 65, "name": "A B", "vowel": true}),
		"phone name with a non-ASCII": setName(map[string]any{"code": 65, "name": "Å", "vowel": true}),
	}
	for name, change := range damaged {
		description := testProsody()
		change(description)
		if err := load(description); err == nil {
			t.Errorf("%s: loaded", name)
		} else {
			t.Logf("%s: %v", name, err)
		}
	}
}
