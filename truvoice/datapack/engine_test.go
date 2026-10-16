package datapack_test

import (
	"strings"
	"testing"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/internal/conformance"
)

func TestEngine(t *testing.T) {
	engine, err := conformance.Pack(t).LoadEngine()
	if err != nil {
		t.Fatal(err)
	}
	visuals := engine.Visuals
	if visuals['A'].IPA != 'e' || *visuals['A'].Mouth != (datapack.Mouth{60, 185, 160, 60, 105, 105, 200, 160}) {
		t.Errorf("A: %+v", visuals['A'])
	}
	if visuals['H'].IPA != 'h' || visuals['H'].Mouth != nil || visuals['U'].IPA != 'u' || visuals['X'].IPA != 0x3b8 {
		t.Errorf("H %+v, U %+v, X %+v", visuals['H'], visuals['U'], visuals['X'])
	}
	if visuals[' '].IPA != 0 || *visuals[' '].Mouth != (datapack.Mouth{0, 128, 128, 0, 0, 0, 0, 0}) {
		t.Errorf("space: %+v", visuals[' '])
	}
	if visuals['p'] != (datapack.Visual{}) || visuals['Q'] != (datapack.Visual{}) || visuals[0] != (datapack.Visual{}) {
		t.Errorf("p %+v, Q %+v", visuals['p'], visuals['Q'])
	}
	listed := 0
	for _, visual := range visuals {
		if visual != (datapack.Visual{}) {
			listed++
		}
	}
	if listed != 64 {
		t.Errorf("%d symbols listed", listed)
	}
}

func TestDamagedEngine(t *testing.T) {
	tests := []struct{ content, message string }{
		{`{"visuals":[["A",101]]}`, "visuals[0] is not"},
		{`{"visuals":[["AB",101,null]]}`, `symbol "AB"`},
		{`{"visuals":[["A",101,null],["A",101,null]]}`, `visuals[1]: symbol "A"`},
		{`{"visuals":[["\u0080",101,null]]}`, "is not a new printable"},
		{`{"visuals":[["A",55296,null]]}`, "IPA character 55296"},
		{`{"visuals":[["A",101,[1,2,3]]]}`, "has 3 values"},
		{`{"visuals":[["A",101,[1,2,3,4,5,6,7,256]]]}`, "mouth value 256"},
		{`{"visuals":[["A",0,null]]}`, "neither"},
	}
	for _, test := range tests {
		if _, err := writePack(t, "engine.json", []byte(test.content)).LoadEngine(); err == nil || !strings.Contains(err.Error(), test.message) {
			t.Errorf("%s: %v, expected %q", test.content, err, test.message)
		}
	}
}
