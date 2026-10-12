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
