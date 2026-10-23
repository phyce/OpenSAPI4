package datapack_test

import (
	"testing"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
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
	address := preFormatter.Address
	if len(address.FunctionWords) != 59 || address.FunctionWords[0] != "about" || len(address.Abbreviations) != 163 ||
		address.Abbreviations[0] != "abil" || len(address.SpacedAbbreviations) != 2 {
		t.Errorf("address words %d function words, %d abbreviations, %d spaced", len(address.FunctionWords),
			len(address.Abbreviations), len(address.SpacedAbbreviations))
	}
	first, mister, all := address.Words[0], address.Words[218], address.Words[3]
	if len(address.Words) != 375 || first.Word != "Acct" || first.Expansion != "Account" ||
		first.Kind != datapack.AbbreviationWord || first.Dotted || mister.Word != "Mr" || mister.Expansion != "Mister" ||
		mister.Kind != datapack.TitleWord || !mister.Dotted || all.Word != "ALL" || all.Expansion != "~ALL" ||
		all.Kind != datapack.CapitalWord {
		t.Errorf("%d list words, the first %+v", len(address.Words), first)
	}
	if len(address.NumberWords) != 19 || address.NumberWords[0] != "apr" {
		t.Errorf("number words %v", address.NumberWords)
	}
}
