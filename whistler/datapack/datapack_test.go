package datapack_test

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/phyce/OpenSAPI4/whistler/datapack"
	"github.com/phyce/OpenSAPI4/whistler/internal/conformance"
)

func TestVoices(t *testing.T) {
	pack := conformance.Pack(t)
	expected := map[string]struct {
		phones, units, sampleRate, order, transformSize, alternatives int
	}{
		"female":  {58, 3334, 22050, 20, 512, 0},
		"female8": {58, 3334, 8000, 12, 256, 0},
		"male":    {58, 3334, 22050, 20, 512, 0},
		"male8":   {58, 3334, 8000, 12, 256, 0},
		"sam":     {52, 3366, 22050, 20, 512, 2},
	}
	voices := map[string]*datapack.Voice{}
	for name, facts := range expected {
		voice := conformance.Voice(t, pack, name)
		voices[name] = voice
		inventory := voice.Inventory
		actual := struct {
			phones, units, sampleRate, order, transformSize, alternatives int
		}{len(voice.Phones), inventory.UnitCount(), voice.SampleRate, inventory.PredictorOrder, inventory.TransformSize, len(voice.AlternativeUnits)}
		if actual != facts {
			t.Errorf("%s: %+v, expected %+v", name, actual, facts)
		}
		if voice.Phones[voice.SilencePhone] != "SIL" || voice.SilencePhone != 18 {
			t.Errorf("%s: silence phone %d", name, voice.SilencePhone)
		}
		if voice.Trees.SenoneCount != 3333 || len(voice.Senones.MeanDurations) != 3333 {
			t.Errorf("%s: %d senones in the trees, %d in the senone file", name, voice.Trees.SenoneCount, len(voice.Senones.MeanDurations))
		}
	}
	if voices["female8"].Trees != voices["female"].Trees || voices["male"].Trees != voices["female"].Trees || voices["female8"].Senones != voices["female"].Senones {
		t.Error("shared trees or senones are loaded twice")
	}
	if voices["sam"].Trees == voices["female"].Trees || voices["male"].Senones == voices["female"].Senones {
		t.Error("different trees or senones are shared")
	}
}

func writePack(t *testing.T, files map[string][]byte, manifestFiles ...string) string {
	t.Helper()
	rootPath := t.TempDir()
	names := []string{}
	for _, name := range manifestFiles {
		names = append(names, `"`+name+`": ""`)
	}
	for name, data := range files {
		path := filepath.Join(rootPath, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		names = append(names, `"`+name+`": ""`)
	}
	manifest := `{"format": "opensapi4-microsoft", "formatVersion": 1, "voices": ["test"], "files": {` + strings.Join(names, ", ") + `}}`
	if err := os.WriteFile(filepath.Join(rootPath, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return rootPath
}

func littleEndian(values ...any) []byte {
	var data []byte
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			data = append(data, typed...)
		case []byte:
			data = append(data, typed...)
		default:
			data, _ = binary.Append(data, binary.LittleEndian, typed)
		}
	}
	return data
}

func testVoiceJSON(visual string, alternative string) []byte {
	return []byte(`{"phones": ["SIL", "A", "B"], "silencePhone": 0, "sampleRate": 8000, "trees": "trees.bin", "senones": "senones.bin", "inventory": "inventory.bin",
		"visual": [` + visual + `], "alternativeUnits": [` + alternative + `]}`)
}

const (
	testVisual      = `{"ipa": 0, "engine": 32, "mouth": [0, 117, 153, 0, 0, 0, 0, 0]}, {"ipa": 593, "engine": 111, "mouth": [16, 160, 160, 0, 128, 0, 0, 208]}, {"ipa": 98, "engine": 66, "mouth": [0, 64, 128, 0, 0, 0, 0, 166]}`
	testAlternative = `{"key": [1, 2], "flags": 3, "codes": [49155, 3], "replacementPhones": [2]}`
)

func TestDamagedFiles(t *testing.T) {
	voiceJSON := testVoiceJSON(testVisual, testAlternative)
	trees := littleEndian("WTRE", uint32(1), uint32(3), uint32(2), uint32(0), uint32(2), uint32(0), []int32{-1, 0, 1}, []uint32{0, 1, 2}, []uint32{0, 1, 1},
		[]int32{-1, -1, 0, 0, 0}, []int32{-1, -1, 0, 0, 0})
	senones := littleEndian("WSEN", uint32(1), uint32(2), []float32{50, 1, 60, 2})
	codebooks := littleEndian(int32(2), int32(2), []float32{0.1, 0.2, 0.3, 0.4}, int32(2), int32(2), []float32{1, 2, 3, 4})
	record := littleEndian(int32(1), int8(3), uint8(1), float32(0.5), uint8(1))
	inventory := littleEndian("WINV", uint32(1), []int32{8000, 2, 1, 1, 0, 4, 2}, codebooks, []float64{1, 2, 3, 4}, uint32(4), []uint32{0, 0, 11, 22, 33},
		record, record, record)
	valid := map[string][]byte{"voices/test/voice.json": voiceJSON, "trees.bin": trees, "senones.bin": senones, "inventory.bin": inventory}
	open := func(files map[string][]byte, manifestFiles ...string) error {
		pack, err := datapack.Open(writePack(t, files, manifestFiles...))
		if err != nil {
			return err
		}
		_, err = pack.LoadVoice("test")
		return err
	}
	if err := open(valid); err != nil {
		t.Fatalf("valid pack: %v", err)
	}
	for _, alternative := range []string{"", `{"key": [0, 1, 2], "flags": 1, "codes": [0, 3], "replacementPhones": []}`, `{"key": [2, 1, 1, 2], "flags": 0, "codes": [49155, 3], "replacementPhones": [2]}`} {
		if err := open(map[string][]byte{"voices/test/voice.json": testVoiceJSON(testVisual, alternative), "trees.bin": trees, "senones.bin": senones, "inventory.bin": inventory}); err != nil {
			t.Errorf("valid alternative unit %s: %v", alternative, err)
		}
	}
	damagedVoices := map[string][2]string{
		"Visual entry missing":                        {testVisual[:strings.LastIndex(testVisual, ", {")], testAlternative},
		"Visual engine phoneme 0x7f":                  {strings.Replace(testVisual, `"engine": 66`, `"engine": 127`, 1), testAlternative},
		"Visual IPA character a surrogate":            {strings.Replace(testVisual, `"ipa": 98`, `"ipa": 55296`, 1), testAlternative},
		"Visual mouth with seven values":              {strings.Replace(testVisual, `[0, 64, 128, 0, 0, 0, 0, 166]`, `[0, 64, 128, 0, 0, 0, 0]`, 1), testAlternative},
		"Visual mouth value 256":                      {strings.Replace(testVisual, `166]`, `256]`, 1), testAlternative},
		"alternative unit with codes past its span":   {testVisual, strings.Replace(testAlternative, `[49155, 3]`, `[49155, 3, 3]`, 1)},
		"alternative unit with an empty span":         {testVisual, `{"key": [1, 2], "flags": 0, "codes": [], "replacementPhones": []}`},
		"alternative unit with context lists":         {testVisual, strings.Replace(testAlternative, `"flags": 3`, `"flags": 7`, 1)},
		"alternative unit replacing its last phone":   {testVisual, strings.Replace(testAlternative, `[49155, 3]`, `[3, 49155]`, 1)},
		"alternative unit playing a tree senone":      {testVisual, strings.Replace(testAlternative, `[49155, 3]`, `[49155, 2]`, 1)},
		"alternative unit beyond the inventory":       {testVisual, strings.Replace(testAlternative, `[49155, 3]`, `[49155, 4]`, 1)},
		"alternative unit silence on a phone":         {testVisual, strings.Replace(testAlternative, `[49155, 3]`, `[49155, 0]`, 1)},
		"alternative unit insert code":                {testVisual, strings.Replace(testAlternative, `[49155, 3]`, `[32771, 3]`, 1)},
		"alternative unit replaced by SIL":            {testVisual, strings.Replace(testAlternative, `"replacementPhones": [2]`, `"replacementPhones": [0]`, 1)},
		"alternative unit without replacement phones": {testVisual, strings.Replace(testAlternative, `"replacementPhones": [2]`, `"replacementPhones": []`, 1)},
		"alternative unit phone out of range":         {testVisual, strings.Replace(testAlternative, `"key": [1, 2]`, `"key": [1, 3]`, 1)},
	}
	for name, parts := range damagedVoices {
		err := open(map[string][]byte{"voices/test/voice.json": testVoiceJSON(parts[0], parts[1]), "trees.bin": trees, "senones.bin": senones, "inventory.bin": inventory})
		if err == nil {
			t.Errorf("%s: loaded", name)
			continue
		}
		t.Logf("%s: %v", name, err)
	}
	changedByte := func(offset int, value byte) []byte {
		changed := append([]byte{}, inventory...)
		changed[len(changed)+offset] = value
		return changed
	}
	damaged := map[string]map[string][]byte{
		"truncated inventory":       {"inventory.bin": inventory[:len(inventory)-1]},
		"extra byte":                {"inventory.bin": append(append([]byte{}, inventory...), 0)},
		"wrong magic":               {"trees.bin": append([]byte("WXXX"), trees[4:]...)},
		"spectrum index too large":  {"inventory.bin": changedByte(-1, 2)},
		"line index too large":      {"inventory.bin": changedByte(-6, 2)},
		"period of 0":               {"inventory.bin": changedByte(-7, 0)},
		"period of 2N":              {"inventory.bin": changedByte(-7, 8)},
		"silence phone not SIL":     {"voices/test/voice.json": []byte(strings.Replace(string(voiceJSON), `"SIL"`, `"S"`, 1))},
		"file outside the manifest": {"voices/test/voice.json": []byte(strings.Replace(string(voiceJSON), `"trees.bin"`, `"other.bin"`, 1))},
	}
	for name, replacements := range damaged {
		files := map[string][]byte{}
		for path, data := range valid {
			files[path] = data
		}
		for path, data := range replacements {
			files[path] = data
		}
		if err := open(files); err == nil {
			t.Errorf("%s: loaded", name)
		}
	}
	outside := map[string][]byte{"voices/test/voice.json": []byte(strings.Replace(string(voiceJSON), `"trees.bin"`, `"../trees.bin"`, 1)), "senones.bin": senones, "inventory.bin": inventory}
	if err := open(outside, "../trees.bin"); err == nil {
		t.Error("a file outside the pack loaded")
	}
}

func TestDamagedModes(t *testing.T) {
	valid := `{"modes": [
		{"name": "Base", "modeId": "{00000000-0000-0000-0000-000000000001}", "voice": "test", "variant": 0, "gender": "female", "speed": 170, "pitch": 169, "effects": []},
		{"name": "Whisper", "modeId": null, "voice": "test", "variant": 1, "gender": "male", "speed": 150, "pitch": 100, "characterTag": "\\Chr=\"Whisper\"\\",
			"effects": [{"effect": "whisper"}, {"effect": "postFilter", "coefficients": [0.25, -0.5, 0.25]}]},
		{"name": "Hall", "modeId": "{00000000-0000-0000-0000-000000000002}", "voice": "test", "variant": 2, "gender": "female", "speed": 130, "pitch": 120,
			"effects": [{"effect": "reverb", "preset": 1}, {"effect": "monotone"}]}],
		"reverbPresets": [{"inputGain": 1, "dryGain": 1, "lengthModulation": 0.05, "lines": [{"delay": 10, "gain": 0.5}]}]}`
	load := func(modes string) error {
		pack, err := datapack.Open(writePack(t, map[string][]byte{"modes.json": []byte(modes)}))
		if err != nil {
			return err
		}
		_, err = pack.LoadModes()
		return err
	}
	if err := load(valid); err != nil {
		t.Fatalf("valid modes: %v", err)
	}
	damaged := map[string][2]string{
		"no name":                   {`"name": "Hall"`, `"name": ""`},
		"unknown voice":             {`"voice": "test", "variant": 2`, `"voice": "nobody", "variant": 2`},
		"variant out of order":      {`"variant": 2`, `"variant": 3`},
		"variant before its base":   {`"voice": "test", "variant": 0`, `"voice": "test", "variant": 1`},
		"two base modes":            {`"effects": []},`, `"effects": []}, {"name": "Base two", "modeId": "{00000000-0000-0000-0000-000000000003}", "voice": "test", "gender": "male", "speed": 1, "pitch": 1},`},
		"duplicate mode id":         {`000000000002}`, `000000000001}`},
		"malformed mode id":         {`{00000000-0000-0000-0000-000000000002}`, `{0000-2}`},
		"hidden base mode":          {`"modeId": "{00000000-0000-0000-0000-000000000001}"`, `"modeId": null`},
		"base mode with effects":    {`"effects": []`, `"effects": [{"effect": "whisper"}]`},
		"base mode with a tag":      {`"pitch": 169,`, `"pitch": 169, "characterTag": "\\Chr=\"Base\"\\",`},
		"unknown gender":            {`"gender": "male"`, `"gender": "robot"`},
		"speed 0":                   {`"speed": 150`, `"speed": 0`},
		"pitch 65535":               {`"pitch": 100`, `"pitch": 65535`},
		"unknown effect":            {`{"effect": "monotone"}`, `{"effect": "echo"}`},
		"whisper with a preset":     {`{"effect": "whisper"}`, `{"effect": "whisper", "preset": 1}`},
		"post-filter without taps":  {`[0.25, -0.5, 0.25]`, `[]`},
		"post-filter with a preset": {`[0.25, -0.5, 0.25]}`, `[0.25], "preset": 1}`},
		"reverb preset 0":           {`"preset": 1`, `"preset": 0`},
		"reverb preset 2 of 1":      {`"preset": 1`, `"preset": 2`},
		"two reverbs":               {`{"effect": "monotone"}`, `{"effect": "reverb", "preset": 1}`},
		"preset without lines":      {`"lines": [{"delay": 10, "gain": 0.5}]`, `"lines": []`},
		"preset with five lines":    {`[{"delay": 10, "gain": 0.5}]`, `[{"delay": 10, "gain": 0.5}, {"delay": 1}, {"delay": 2}, {"delay": 3}, {"delay": 4}]`},
		"modulation 0.2":            {`0.05`, `0.2`},
		"negative modulation":       {`0.05`, `-0.05`},
		"delay 0":                   {`"delay": 10`, `"delay": 0`},
		"delay of a minute":         {`"delay": 10`, `"delay": 60000`},
		"gain beyond float32":       {`"gain": 0.5`, `"gain": 1e39`},
	}
	for name, replacement := range damaged {
		changed := strings.Replace(valid, replacement[0], replacement[1], 1)
		if changed == valid {
			t.Fatalf("%s: nothing replaced", name)
		}
		if err := load(changed); err == nil {
			t.Errorf("%s: loaded", name)
		} else {
			t.Logf("%s: %v", name, err)
		}
	}
	if err := load(`{"modes": [], "reverbPresets": []}`); err == nil {
		t.Error("a pack without modes loaded")
	}
}
