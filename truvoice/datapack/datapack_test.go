package datapack_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/internal/conformance"
)

func TestSynthesizer(t *testing.T) {
	synthesizer, err := conformance.Pack(t).LoadSynthesizer()
	if err != nil {
		t.Fatal(err)
	}
	if synthesizer.HalfCosine[0] != 32760 || synthesizer.HalfCosine[128] != 0 || synthesizer.HalfCosine[255] != -32760 {
		t.Errorf("half-cosine %d %d %d", synthesizer.HalfCosine[0], synthesizer.HalfCosine[128], synthesizer.HalfCosine[255])
	}
	var filters [datapack.VoiceCount]int32
	for index, voice := range synthesizer.Voices {
		filters[index] = voice.OutputFilter
	}
	if expected := [datapack.VoiceCount]int32{0, 2, -80, -22, 11, -42, 26, -85, -85, -36}; filters != expected {
		t.Errorf("output filters %v", filters)
	}
	if synthesizer.Decibels[200] != 8192 || synthesizer.Voices[0].Formant5 != 3970 || synthesizer.Voices[9].SourceGain != 20000 {
		t.Errorf("decibels[200] %d, formant5 %d, source gain %d", synthesizer.Decibels[200], synthesizer.Voices[0].Formant5, synthesizer.Voices[9].SourceGain)
	}
	if synthesizer.ForSampleRate(11025).SourceStage != [3]int16{11354, 0, 0} || synthesizer.ForSampleRate(8000).SourceStage != [3]int16{5000, 0, 0} {
		t.Errorf("source stages %v %v", synthesizer.Standard.SourceStage, synthesizer.Telephone.SourceStage)
	}
}

func writePack(t *testing.T, name string, content []byte) *datapack.Pack {
	t.Helper()
	folder := t.TempDir()
	manifest := `{"format":"opensapi4-truvoice","formatVersion":1,"files":{"` + name + `":""}}`
	if err := os.WriteFile(filepath.Join(folder, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, name), content, 0o644); err != nil {
		t.Fatal(err)
	}
	pack, err := datapack.Open(folder)
	if err != nil {
		t.Fatal(err)
	}
	return pack
}

func TestDamagedSynthesizer(t *testing.T) {
	halfCosine := `"halfCosine":[` + strings.Repeat("0,", 255) + `0]`
	tests := []struct{ content, message string }{
		{`{"halfCosine":[1],"sourceShapes":[]}`, "halfCosine has 1 values"},
		{`{` + halfCosine + `,"sourceShapes":[` + strings.Repeat("0,", 31) + `40000]}`, "sourceShapes[31] is 40000"},
	}
	for _, test := range tests {
		if _, err := writePack(t, "synthesizer.json", []byte(test.content)).LoadSynthesizer(); err == nil || !strings.Contains(err.Error(), test.message) {
			t.Errorf("%s: %v, expected %q", test.content, err, test.message)
		}
	}
}

func TestChangedSynthesizer(t *testing.T) {
	original, err := os.ReadFile(conformance.Path(t, "datapack", "truvoice", "synthesizer.json"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		change  func(map[string]any)
		message string
	}{
		{func(content map[string]any) { content["jitterDepths"].([]any)[3] = 32767 }, "allow a pitch period of 0"},
		{func(content map[string]any) { content["voices"].([]any)[3].(map[string]any)["jitter"] = 16 }, "voices[3].jitter is 16"},
		{func(content map[string]any) { delete(content["voices"].([]any)[2].(map[string]any), "sourceGain") }, "voices[2].sourceGain is missing"},
		{func(content map[string]any) { content["voices"] = content["voices"].([]any)[:9] }, "voices has 9 entries"},
		{func(content map[string]any) { delete(content["rates"].(map[string]any), "8000") }, "rates has 1 entries"},
		{func(content map[string]any) {
			rates := content["rates"].(map[string]any)
			rates["8001"] = rates["8000"]
			delete(rates, "8000")
		}, "rates has no 8000"},
		{func(content map[string]any) {
			content["rates"].(map[string]any)["11025"].(map[string]any)["cosines"] = []any{1}
		}, "rates.11025.cosines has 1 values"},
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
		if _, err := writePack(t, "synthesizer.json", changed).LoadSynthesizer(); err == nil || !strings.Contains(err.Error(), test.message) {
			t.Errorf("%v, expected %q", err, test.message)
		}
	}
}

func TestPipeline(t *testing.T) {
	pipeline, err := conformance.Pack(t).LoadPipeline()
	if err != nil {
		t.Fatal(err)
	}
	var pitches [datapack.VoiceCount]int32
	for index, voice := range pipeline.Voices {
		pitches[index] = voice.Pitch
	}
	if expected := [datapack.VoiceCount]int32{85, 50, 125, 73, 129, 89, 117, 203, 208, 152}; pitches != expected {
		t.Errorf("pitches %v", pitches)
	}
	if pipeline.Voices[5] != (datapack.VoiceDefaults{Pitch: 89, Rate: 9, Speed: 120}) {
		t.Errorf("voice 5 %+v", pipeline.Voices[5])
	}
	if pipeline.InitialFrame[3] != 60 || pipeline.InitialFrame[9] != 100 || pipeline.GeneratedFrame[0] != 60 || pipeline.GeneratedFrame[10] != 113 {
		t.Errorf("frames %v %v", pipeline.InitialFrame, pipeline.GeneratedFrame)
	}
}

func TestChangedPipeline(t *testing.T) {
	original, err := os.ReadFile(conformance.Path(t, "datapack", "truvoice", "pipeline.json"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		change  func(map[string]any)
		message string
	}{
		{func(content map[string]any) { content["voices"] = content["voices"].([]any)[:9] }, "voices has 9 entries"},
		{func(content map[string]any) { delete(content["voices"].([]any)[4].(map[string]any), "rate") }, "voices[4].rate is missing"},
		{func(content map[string]any) { content["voices"].([]any)[1].(map[string]any)["pitch"] = 401 }, "voices[1].pitch is 401"},
		{func(content map[string]any) { content["voices"].([]any)[7].(map[string]any)["speed"] = 49 }, "voices[7].speed is 49"},
		{func(content map[string]any) { content["initialFrame"] = []any{1} }, "initialFrame has 1 values"},
		{func(content map[string]any) { content["generatedFrame"].([]any)[21] = 256 }, "generatedFrame[21] is 256"},
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
		if _, err := writePack(t, "pipeline.json", changed).LoadPipeline(); err == nil || !strings.Contains(err.Error(), test.message) {
			t.Errorf("%v, expected %q", err, test.message)
		}
	}
}

func TestParameters(t *testing.T) {
	parameters, err := conformance.Pack(t).LoadParameters()
	if err != nil {
		t.Fatal(err)
	}
	if len(parameters.Shapes[0]) != 0 || !slices.Equal(parameters.Shapes[3], []byte{255, 114, 28}) || len(parameters.Shapes[30]) != 29 {
		t.Errorf("shapes %v %v %v", parameters.Shapes[0], parameters.Shapes[3], parameters.Shapes[30])
	}
	index, found := parameters.Index('B')
	if !found || index != 34 || parameters.Phonemes[index].Burst == nil || parameters.Phonemes[index].Burst[0] != [4]int32{50, 65, 40, 0} {
		t.Errorf("B is phoneme %d, %+v", index, parameters.Phonemes[index])
	}
	if _, found := parameters.Index('!'); found {
		t.Error("! is a phoneme")
	}
	if vowel := parameters.Phonemes[0].Vowel; vowel == nil || vowel.Transition != 7 || vowel.Offset != 14760 || parameters.Phonemes[19].Vowel != nil {
		t.Errorf("vowels %+v %+v", parameters.Phonemes[0].Vowel, parameters.Phonemes[19].Vowel)
	}
	rules := 0
	for _, set := range parameters.RuleSets {
		rules += len(set)
	}
	if len(parameters.RuleSets) != 54 || rules != 468 || parameters.Classes[0]['B'] != 0xa4 || parameters.Voices[1].Tracks != [3]int32{17, 8, 5} || parameters.CategoryWeights[3][0] != 21320 {
		t.Errorf("%d rule sets, %d rules, class %#x, voice %+v, weight %d", len(parameters.RuleSets), rules, parameters.Classes[0]['B'], parameters.Voices[1], parameters.CategoryWeights[3][0])
	}
	if frame := parameters.SegmentFrames[0]; frame[0] != 255 || frame[12] != 225 || parameters.SegmentFrames[11][6] != 15 || parameters.ToneLevels[15] != 58 {
		t.Errorf("segment frames %v %v, tone levels %v", frame, parameters.SegmentFrames[11], parameters.ToneLevels)
	}
	if parameters.NeutralFormants != [3]int32{490, 1450, 2500} || parameters.ReductionWeights[0] != 28864 || parameters.ReductionWeights[15] != 528 {
		t.Errorf("neutral formants %v, reduction weights %v", parameters.NeutralFormants, parameters.ReductionWeights)
	}
	liquids := &parameters.Liquids
	if row := liquids.R[1].Row('G', 'v'); *row != (datapack.LiquidRow{Duration: 75, Values: [3]int32{1065, 945, -5}, Kinds: [3]int32{0, 0, 1}}) {
		t.Errorf("R's F2 row for G and v %+v", row)
	}
	if row := liquids.L[0].Row('K', 'E'); row != &liquids.L[0].Rows[7] || len(liquids.Y[2].Rows) != 2 {
		t.Errorf("L's F1 row for K and E %+v, Y's F3 rows %d", row, len(liquids.Y[2].Rows))
	}
	if len(liquids.RAcrossMark[1]) != 5 || liquids.TargetsAcrossMark[3][2] != 3600 || liquids.RAfterVowel[2][1].Values[1] != 1500 {
		t.Errorf("R across a mark %v, targets %v, R after a vowel %v", liquids.RAcrossMark, liquids.TargetsAcrossMark, liquids.RAfterVowel)
	}
	first := parameters.RuleSets[0][0]
	if len(first.Conditions) != 1 || first.Conditions[0] != (datapack.Condition{Kind: datapack.SymbolCondition, Phoneme: datapack.CurrentPhoneme, Symbols: "EUY4", Is: true}) ||
		len(first.Assignments) != 2 || first.Assignments[1] != (datapack.Assignment{Operation: datapack.Set, To: datapack.Place{Field: datapack.TargetField, Track: 4}}) || !slices.Equal(first.Actions, []int{2, 3, 4, 10}) {
		t.Errorf("rule set 0's first rule %+v", first)
	}
}

func phoneme(content map[string]any, index int) map[string]any {
	return content["phonemes"].([]any)[index].(map[string]any)
}

func rule(content map[string]any, set, number int) map[string]any {
	return content["ruleSets"].([]any)[set].([]any)[number].(map[string]any)
}

func liquids(content map[string]any) map[string]any {
	return content["liquids"].(map[string]any)
}

func TestChangedParameters(t *testing.T) {
	original, err := os.ReadFile(conformance.Path(t, "datapack", "truvoice", "parameters.json"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		change  func(map[string]any)
		message string
	}{
		{func(content map[string]any) { content["shapes"] = content["shapes"].([]any)[:30] }, "shapes has 30 entries"},
		{func(content map[string]any) { content["shapes"].([]any)[2] = []any{255, 1, 1} }, "shapes[2] has 3 weights"},
		{func(content map[string]any) { content["shapes"].([]any)[4].([]any)[0] = 254 }, "shapes[4] starts with 254"},
		{func(content map[string]any) { content["shapes"].([]any)[5].([]any)[2] = 0 }, "shapes[5][2] is 0"},
		{func(content map[string]any) { phoneme(content, 3)["symbol"] = "4" }, `phonemes[3].symbol "4" is also phonemes[0]'s`},
		{func(content map[string]any) { phoneme(content, 58)["symbol"] = "!" }, "the pause is phonemes[-1]"},
		{func(content map[string]any) { delete(phoneme(content, 34), "voicing") }, "phonemes[34].voicing is missing"},
		{func(content map[string]any) { phoneme(content, 34)["burst"] = []any{[]any{1, 2, 3, 4}} }, "phonemes[34].burst has 1 rows"},
		{func(content map[string]any) { content["classes"] = content["classes"].([]any)[:5] }, "classes has 5 planes"},
		{func(content map[string]any) { content["defaultShapes"].([]any)[3] = 31 }, "defaultShapes[3] is 31"},
		{func(content map[string]any) { phoneme(content, 34)["articulator"] = 8 }, "phonemes[34].articulator is 8"},
		{func(content map[string]any) { content["articulatorShapes"].([]any)[1] = 31 }, "articulatorShapes[1] is 31"},
		{func(content map[string]any) { delete(phoneme(content, 3), "offset") }, "phonemes[3].offset is missing"},
		{func(content map[string]any) { content["neutralFormants"] = []any{490, 1450} }, "neutralFormants has 2"},
		{func(content map[string]any) { content["reductionWeights"].([]any)[2] = -1 }, "reductionWeights[2] is -1"},
		{func(content map[string]any) { content["ruleMatrix"].([]any)[2].([]any)[5] = 54 }, "ruleMatrix[2][5] is 54, outside 0 to 53"},
		{func(content map[string]any) { content["segmentFrames"] = content["segmentFrames"].([]any)[:10] }, "segmentFrames has 10 entries"},
		{func(content map[string]any) { content["segmentFrames"].([]any)[4].([]any)[21] = 256 }, "segmentFrames[4][21] is 256"},
		{func(content map[string]any) { content["toneLevels"] = content["toneLevels"].([]any)[:15] }, "toneLevels has 15 values"},
		{func(content map[string]any) { liquids(content)["L"] = liquids(content)["L"].([]any)[:3] }, "liquids.L has 3 tracks"},
		{func(content map[string]any) {
			track := liquids(content)["R"].([]any)[2].(map[string]any)
			track["rows"] = track["rows"].([]any)[1:]
		}, "liquids.R[2] has 25 rows for 25 contexts"},
		{func(content map[string]any) {
			liquids(content)["Y"].([]any)[0].(map[string]any)["contexts"].([]any)[1] = []any{[]any{"y", ""}}
		}, `liquids.Y[0].contexts[1] has ["y" ""]`},
		{func(content map[string]any) {
			liquids(content)["rAcrossMark"].([]any)[1].([]any)[0].([]any)[2] = 3
		}, "liquids.rAcrossMark[1][0] kinds[2] is 3"},
		{func(content map[string]any) {
			liquids(content)["rAfterVowel"].([]any)[0] = []any{}
		}, "liquids.rAfterVowel[0] has 0 rows, expected 1"},
		{func(content map[string]any) {
			liquids(content)["targetsAcrossMark"].([]any)[0] = []any{1}
		}, "liquids.targetsAcrossMark[0] has 1 values, expected 2"},
		{func(content map[string]any) { rule(content, 3, 0)["do"] = []any{17} }, "ruleSets[3][0].do[0] is 17"},
		{func(content map[string]any) {
			rule(content, 3, 0)["if"] = []any{map[string]any{"flag": 11, "is": true}}
		}, "ruleSets[3][0].if[0].flag is 11"},
		{func(content map[string]any) {
			rule(content, 3, 0)["if"] = []any{map[string]any{"phoneme": "next", "plane": 2, "bits": 3, "is": true}}
		}, "bits is 3, expected one bit"},
		{func(content map[string]any) {
			rule(content, 3, 0)["if"] = []any{map[string]any{"phoneme": "previous", "in": "a", "is": true}}
		}, `phoneme is "previous"`},
		{func(content map[string]any) {
			rule(content, 3, 0)["set"] = []any{map[string]any{"to": []any{"pitch"}, "set": 1}}
		}, `field is "pitch"`},
		{func(content map[string]any) {
			rule(content, 3, 0)["set"] = []any{map[string]any{"to": []any{"burstLevel", 3}, "set": 1}}
		}, "track is 3, outside 0 to 2"},
		{func(content map[string]any) {
			rule(content, 3, 0)["set"] = []any{map[string]any{"to": []any{"target", 1}, "set": 1, "add": 2}}
		}, "has 2 of set, add, subtract and copy"},
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
		if _, err := writePack(t, "parameters.json", changed).LoadParameters(); err == nil || !strings.Contains(err.Error(), test.message) {
			t.Errorf("%v, expected %q", err, test.message)
		}
	}
}

func TestVowels(t *testing.T) {
	vowels, err := conformance.Pack(t).LoadVowels()
	if err != nil {
		t.Fatal(err)
	}
	if vowels.Index('@') != 0 || vowels.Index('c') != 23 || vowels.Index('p') != -1 || vowels.Index(200) != -1 {
		t.Errorf("indexes %d %d %d", vowels.Index('@'), vowels.Index('c'), vowels.Index('p'))
	}
	if vowels.Contexts[' '] != 33 || vowels.Contexts['d'] != 5 || vowels.Contexts['H'] != 5 || vowels.Contexts['A'] != -1 {
		t.Errorf("contexts %d %d %d %d", vowels.Contexts[' '], vowels.Contexts['d'], vowels.Contexts['H'], vowels.Contexts['A'])
	}
	schwa := &vowels.Vowels[vowels.Index('@')]
	if len(schwa.Records) != 695 || !datapack.Reduced(schwa.Symbol) || schwa.Trajectories[0] != nil {
		t.Errorf("@ has %d records", len(schwa.Records))
	}
	if record := schwa.Find(0, 0); record == nil || record.Start != [4]byte{100, 65, 159, 207} || record.Middle != [4]byte{106, 63, 160, 209} || record.Timing != [4]int32{11, 11, 11, 11} || record.BandwidthEnds != [3]byte{25, 52, 47} {
		t.Errorf("@ record %+v", record)
	}
	alternatives := 0
	for _, record := range vowels.Vowels[vowels.Index('a')].Records {
		if record.Alternative != nil {
			alternatives++
		}
	}
	if alternatives != 23 || len(vowels.Vowels[vowels.Index('O')].Trajectories[1]) != 263 {
		t.Errorf("a has %d alternatives, O %d F2 trajectories", alternatives, len(vowels.Vowels[vowels.Index('O')].Trajectories[1]))
	}
}

func TestChangedVowels(t *testing.T) {
	original, err := os.ReadFile(conformance.Path(t, "datapack", "truvoice", "vowels.json"))
	if err != nil {
		t.Fatal(err)
	}
	vowel := func(content map[string]any, index int) map[string]any {
		return content["vowels"].([]any)[index].(map[string]any)
	}
	tests := []struct {
		change  func(map[string]any)
		message string
	}{
		{func(content map[string]any) { content["contexts"] = []any{1} }, "contexts has 1 values"},
		{func(content map[string]any) { content["contexts"].([]any)[65] = 34 }, "contexts[65] is 34"},
		{func(content map[string]any) { content["vowels"] = content["vowels"].([]any)[:23] }, "vowels has 23 entries"},
		{func(content map[string]any) { vowel(content, 3)["symbol"] = "O" }, `vowels[3] has symbol "O"`},
		{func(content map[string]any) {
			records := vowel(content, 0)["records"].([]any)
			records[0], records[1] = records[1], records[0]
		}, "vowels[0].records[1] is out of order"},
		{func(content map[string]any) { vowel(content, 2)["records"].([]any)[5] = []any{0, 5, 1} }, "vowels[2].records[5] has 3 values, expected 23"},
		{func(content map[string]any) { vowel(content, 2)["records"].([]any)[5].([]any)[10] = 9999 }, "vowels[2].records[5] trajectory 0 is 9999"},
		{func(content map[string]any) { vowel(content, 0)["records"].([]any)[0].([]any)[14] = 16 }, "vowels[0].records[0] timing[0] is 16"},
		{func(content map[string]any) { vowel(content, 0)["trajectories"] = vowel(content, 2)["trajectories"] }, "vowels[0] has 4 trajectory tables"},
		{func(content map[string]any) {
			vowel(content, 6)["alternatives"].([]any)[0].(map[string]any)["neighbour"] = "after"
		}, "vowels[6].alternatives[0] has no neighbour or a wrong one"},
		{func(content map[string]any) {
			alternatives := vowel(content, 6)["alternatives"].([]any)
			alternatives[1].(map[string]any)["record"] = alternatives[0].(map[string]any)["record"]
		}, "vowels[6].alternatives[1] has no record or a wrong one"},
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
		if _, err := writePack(t, "vowels.json", changed).LoadVowels(); err == nil || !strings.Contains(err.Error(), test.message) {
			t.Errorf("%v, expected %q", err, test.message)
		}
	}
}

func TestProsody(t *testing.T) {
	prosody, err := conformance.Pack(t).LoadProsody()
	if err != nil {
		t.Fatal(err)
	}
	for voice, pitchRange := range prosody.PitchRanges {
		if pitchRange != 32766 {
			t.Errorf("voice %d has pitch range %d", voice, pitchRange)
		}
	}
	if prosody.PausePercents[0] != 2200 || prosody.PausePercents[datapack.RateCount-1] != 30 {
		t.Errorf("pause percents %v", prosody.PausePercents)
	}
	if prosody.SlowPausePatterns[0] != 0xbf7f || prosody.SlowPausePatterns[datapack.SlowRateCount-1] != 0x284a {
		t.Errorf("slow pause patterns %x", prosody.SlowPausePatterns)
	}
}

func TestChangedProsody(t *testing.T) {
	tests := []struct{ content, message string }{
		{`{"pitchRanges":[1,2]}`, "pitchRanges has 2 values"},
		{`{"pitchRanges":[1,2,3,4,5,6,7,8,9,40000]}`, "pitchRanges[9] is 40000"},
		{`{"pitchRanges":[1,2,3,4,5,6,7,8,9,-1]}`, "pitchRanges[9] is -1"},
		{`{"pitchRanges":[1,2,3,4,5,6,7,8,9,10],"pausePercents":[1]}`, "pausePercents has 1 values"},
		{`{"pitchRanges":[1,2,3,4,5,6,7,8,9,10],"pausePercents":[30,30,30,30,30,30,30,30,30,30,30,30,30,30,30,30,30,30,30,30,30,30,30,30,30,-1]}`, "pausePercents[25] is -1"},
		{`{"pitchRanges":[1,2,3,4,5,6,7,8,9,10],"pausePercents":[30,30,30,30,30,30,30,30,30,30,30,30,30,30,30,30,30,30,30,30,30,30,30,30,30,1],"slowPausePatterns":[1,1,1,1,1,1,1,1]}`, "slowPausePatterns has 8 values"},
		{`{"pitchRanges":[1,2,3,4,5,6,7,8,9,10],"pausePercents":[30,30,30,30,30,30,30,30,30,30,30,30,30,30,30,30,30,30,30,30,30,30,30,30,30,1],"slowPausePatterns":[1,1,1,1,1,1,1,1,65536]}`, "slowPausePatterns[8] is 65536"},
	}
	for _, test := range tests {
		if _, err := writePack(t, "prosody.json", []byte(test.content)).LoadProsody(); err == nil || !strings.Contains(err.Error(), test.message) {
			t.Errorf("%v, expected %q", err, test.message)
		}
	}
}
