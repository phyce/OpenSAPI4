package preformatter_test

import (
	"bytes"
	"testing"

	"github.com/phyce/OpenSAPI4/truvoice/internal/conformance"
	"github.com/phyce/OpenSAPI4/truvoice/preformatter"
)

type goldenCase struct {
	Set     string  `json:"set"`
	Mode    int     `json:"mode"`
	Text    string  `json:"text"`
	Output  *string `json:"output"`
	Differs string  `json:"differs"`
}

// TestSpreadsheet formats the golden texts in the spreadsheet context and compares the output with the original's,
// but for the texts section 3 of the spec leaves out, which it only formats.
func TestSpreadsheet(t *testing.T) {
	tables, err := conformance.Pack(t).LoadPreFormatter()
	if err != nil {
		t.Fatal(err)
	}
	compared, failed := 0, 0
	left := map[string]int{}
	conformance.EachLine(t, func(golden goldenCase) {
		if golden.Mode != 2 {
			return
		}
		output := preformatter.Spreadsheet(&tables.Spreadsheet, latin1(golden.Text))
		if golden.Differs != "" {
			left[golden.Differs]++
			return
		}
		compared++
		if expected := latin1(*golden.Output); !bytes.Equal(output, expected) {
			failed++
			if failed <= 10 {
				t.Errorf("%s: %q\n   gave %q\n   expected %q", golden.Set, golden.Text, output, expected)
			}
		}
	}, "preformatter.jsonl.gz")
	if failed > 0 {
		t.Errorf("%d of %d texts differ", failed, compared)
	}
	t.Logf("%d texts compared, left out: %d with a month of more than three characters, %d long",
		compared, left["month"], left["long"])
}

func latin1(text string) []byte {
	converted := make([]byte, 0, len(text))
	for _, character := range text {
		converted = append(converted, byte(character))
	}
	return converted
}
