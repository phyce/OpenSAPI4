package datapack_test

import (
	"os"
	"path/filepath"
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
	if expected := [datapack.VoiceCount]int32{0, 2, -80, -22, 11, -42, 26, -85, -85, -36}; synthesizer.OutputFilters != expected {
		t.Errorf("output filters %v", synthesizer.OutputFilters)
	}
}

func TestDamagedSynthesizer(t *testing.T) {
	halfCosine := `"halfCosine":[` + strings.Repeat("0,", 255) + `0]`
	tests := []struct{ content, message string }{
		{`{"halfCosine":[1],"sourceShapes":[],"outputFilters":[]}`, "halfCosine has 1 values"},
		{`{` + halfCosine + `,"sourceShapes":[` + strings.Repeat("0,", 31) + `40000],"outputFilters":[]}`, "sourceShapes[31] is 40000"},
	}
	for _, test := range tests {
		content, message := test.content, test.message
		folder := t.TempDir()
		manifest := `{"format":"opensapi4-truvoice","formatVersion":1,"files":{"synthesizer.json":""}}`
		if err := os.WriteFile(filepath.Join(folder, "manifest.json"), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(folder, "synthesizer.json"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		pack, err := datapack.Open(folder)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pack.LoadSynthesizer(); err == nil || !strings.Contains(err.Error(), message) {
			t.Errorf("%s: %v, expected %q", content, err, message)
		}
	}
}
