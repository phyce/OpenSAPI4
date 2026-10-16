package datapack_test

import (
	"testing"
	"unicode/utf16"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/internal/conformance"
)

func TestText(t *testing.T) {
	text, err := conformance.Pack(t).LoadText()
	if err != nil {
		t.Fatal(err)
	}
	if len(text.BestFit) != 706 || text.BestFit['A'] != 'A' || text.BestFit[0x0100] != 'A' || text.BestFit[0x20ac] != 0x80 ||
		text.BestFit[0x2019] != 0x92 {
		t.Errorf("best fit: %d code units", len(text.BestFit))
	}
	if _, found := text.BestFit[0x4e00]; found {
		t.Error("best fit maps U+4E00")
	}
	if text.Wide['A'] != 'A' || text.Wide[0x80] != 0x20ac || text.Wide[0x81] != 0x81 || text.Wide[0x83] != 0x192 || text.Wide[0x98] != 0x2dc ||
		text.Wide[0x9f] != 0x178 || text.Wide[0xa0] != 0xa0 || text.Wide[0xff] != 0xff || text.Wide[0] != 0 {
		t.Errorf("wide %x", text.Wide[0x80:0xa0])
	}
	rules := map[string]string{}
	for _, rule := range text.IPARules {
		rules[string(utf16.Decode(rule.Sequence))] = rule.Phonemes
	}
	if len(rules) != 64 || rules["aɪɹ"] != "5" || rules["aɪ"] != "I" || rules["θ"] != "X" || rules["ʔ"] != "q" || rules["a"] != "" {
		t.Errorf("IPA rules %q", rules)
	}
}

func TestModes(t *testing.T) {
	modes, err := conformance.Pack(t).LoadModes()
	if err != nil {
		t.Fatal(err)
	}
	speakers := []string{"Peter", "Sidney", "Eddie", "Douglas", "Biff", "Amos", "Melvin", "Alex", "Wanda", "Julia"}
	for index, mode := range modes.Modes {
		gender := uint16(datapack.GenderMale)
		if index >= 8 {
			gender = datapack.GenderFemale
		}
		if mode.Speaker != speakers[index] || mode.Gender != gender || mode.Product != "TruVoice" || mode.Age != 30 ||
			mode.Mode != "{CA141FD0-AC7F-11D1-97A3-00600827300"+string(rune('0'+index))+"}" {
			t.Errorf("mode %d: %+v", index, mode)
		}
	}
	if modes.Modes[8].Name != "Adult Female #1, American English (TruVoice)" {
		t.Errorf("mode 8's name is %q", modes.Modes[8].Name)
	}
}
