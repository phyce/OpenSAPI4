package truvoice

import (
	"errors"
	"testing"

	"github.com/phyce/OpenSAPI4/truvoice/text"
)

func TestPhonemeString(t *testing.T) {
	engine := newTestEngine(t)
	tests := map[string]string{
		"Hello world.":           "&HeLO1&W31jDp.",
		`Hello \Pit=200\ world.`: `&HeLO1\Pit=200\ &W31jDp.`,
		"":                       "",
	}
	for value, expected := range tests {
		if phonemes := engine.PhonemeString(value); phonemes != expected {
			t.Errorf("%q gave %q, expected %q", value, phonemes, expected)
		}
	}
}

func TestPhonemeCharacterSets(t *testing.T) {
	engine := newTestEngine(t)
	for _, characterSet := range []text.CharacterSet{text.Text, 3} {
		if _, err := engine.Phoneme([]uint16{'a', 0}, true, characterSet); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("character set %d: %v", characterSet, err)
		}
	}
	for _, characterSet := range []text.CharacterSet{text.Text, text.IPA} {
		if _, err := engine.PhonemeANSI([]byte{'a', 0}, true, characterSet); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("ANSI character set %d: %v", characterSet, err)
		}
	}
	phonemes, err := engine.PhonemeANSI([]byte("Hello world.\x00"), false, text.EnginePhonetic)
	if err != nil || string(phonemes) != "&HeLO1&W31jDp.\x00" {
		t.Errorf("ANSI gave %q, %v", phonemes, err)
	}
}
