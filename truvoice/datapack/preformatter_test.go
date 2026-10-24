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
	letters := address.Letters
	if len(letters.TwoLetters) != 39 || letters.TwoLetters[0] != "AD" || len(letters.Onsets) != 36 ||
		len(letters.Codas) != 13 || letters.Codas["PH"] != "AEIOU" || len(letters.FirstPairs) != 9 ||
		len(letters.LastPairs) != 14 || len(letters.Spelled) != 52 || letters.Spelled[0] != "ATM" ||
		len(letters.Unkept) != 16 || len(letters.Kept) != 117 || len(letters.MarkedSpelled) != 2 ||
		len(letters.Respelled) != 16 || letters.Respelled["ISO"] != "eye-SO" || letters.Respelled["XMAS"] != "X-MAS" ||
		letters.Respelled["LOGIN"] != "log-in" {
		t.Errorf("address letters %+v", letters)
	}
	if len(letters.Heads) != 13 || letters.Heads[0] != "ALT" || len(letters.Tails) != 10 || letters.Tails[3] != "EDU" ||
		len(letters.OpenHeads) != 7 || letters.OpenHeads[6] != "USH" || len(letters.UnkeptHeads) != 20 ||
		letters.UnkeptHeads["XYX"] != "JQT" || len(letters.Clusters) != 11 || letters.Clusters["ST"] != "R" ||
		len(letters.VowelCodas) != 11 || len(letters.CodaPairs) != 69 || letters.CodaPairs[68] != "XT" {
		t.Errorf("four-letter tables %+v", letters)
	}
	if len(letters.Pieces) != 13 || letters.Pieces[0] != "ARPA" || len(letters.LongHeads) != 14 ||
		letters.LongHeads[13] != "USE" || len(letters.LongTails) != 16 || letters.LongTails[15] != "PC" ||
		len(letters.Unsplit) != 62 || letters.Unsplit[0] != "ALTAR" {
		t.Errorf("five-letter tables %+v", letters)
	}
	if len(letters.Readings) != 105 || letters.Readings["PM"] != "P.m." || letters.Readings["XMAS"] != "Christmas" {
		t.Errorf("readings %v", letters.Readings)
	}
}
