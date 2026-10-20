package datapack_test

import (
	"testing"

	"github.com/phyce/OpenSAPI4/truvoice/internal/conformance"
)

func TestPreFormatter(t *testing.T) {
	preFormatter, err := conformance.Pack(t).LoadPreFormatter()
	if err != nil {
		t.Fatal(err)
	}
	words := preFormatter.Spreadsheet
	if words.Months[0] != "January" || words.Months[11] != "December" || words.Denominators[1] != "half" ||
		words.Teens[2] != "twelfth" || words.Tens[2] != "fourtieth" || words.Ordinals[0] != "first" ||
		words.OtherUnits != [2]string{"one", "inetieth"} || words.Minus != "minus" || words.And != "and" ||
		words.Exponent != "times 10 to the" {
		t.Errorf("spreadsheet words %+v", words)
	}
}
