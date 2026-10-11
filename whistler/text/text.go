// Package text is the Microsoft preprocessing of a TextData text (text.md §1–7): character sets, tags
// and the control table, line breaks, special tokens, the sentence breaker, phonetic input and the
// segments the analyser reads.
package text

import (
	"bytes"

	"github.com/phyce/OpenSAPI4/whistler/datapack"
)

type CharacterSet int

const (
	UnicodeText  CharacterSet = 0
	UnicodeIPA   CharacterSet = 1
	ANSIText     CharacterSet = 4
	ANSIPhonetic CharacterSet = 5
)

const (
	escape         = 0x1B
	space          = ' '
	tab            = '\t'
	lineFeed       = '\n'
	carriageReturn = '\r'
	backslash      = '\\'
)

type ControlKind byte

const (
	ControlCharacter        ControlKind = 0
	ControlWord             ControlKind = 1
	ControlMark             ControlKind = 9
	ControlPause            ControlKind = 0xA
	ControlPitch            ControlKind = 0xB
	ControlReset            ControlKind = 0xF
	ControlSpeed            ControlKind = 0x10
	ControlVolume           ControlKind = 0x12
	ControlTransplantOffset ControlKind = 0x13
	ControlTransplantEnd    ControlKind = 0x14
	ControlTransplant       ControlKind = 0x15
)

// Control is an entry of the control table (text.md §3.4). Value is a decimal number or a text.
type Control struct {
	Kind  ControlKind
	Value string
}

// Prepared holds every stage of the preprocessing of one text. Joined is nil for phonetic input, and
// Rewritten is nil when the special tokens changed nothing or gave up.
type Prepared struct {
	ANSI           []byte
	OneCharacter   bool
	Tagged         []byte
	Controls       []Control
	AddressContext bool
	Joined         []byte
	Rewritten      []byte
	Final          []byte
}

// Segment is a part of the final text that the analyser reads on its own. Pitch holds the digits of the
// ESC I<n> that starts it, and is nil for the first segment.
type Segment struct {
	Pitch []byte
	Text  []byte
}

type Preprocessor struct {
	tables *datapack.Text
}

func New(tables *datapack.Text) *Preprocessor {
	return &Preprocessor{tables: tables}
}

// Prepare runs the preprocessing of one TextData text. addressContext is the address context the
// previous text left on; the result holds the one this text leaves.
func (preprocessor *Preprocessor) Prepare(text []uint16, tagged bool, characterSet CharacterSet, addressContext bool) *Prepared {
	source := text
	if characterSet == UnicodeIPA {
		source = preprocessor.ipaToEngineNames(text, tagged)
	}
	prepared := &Prepared{ANSI: preprocessor.ToANSI(source)}
	prepared.OneCharacter = len(prepared.ANSI) == 1
	scanner := tagScanner{tables: preprocessor.tables}
	prepared.Tagged, prepared.Controls, prepared.AddressContext = scanner.scan(prepared.ANSI, tagged, characterSet, addressContext)
	if characterSet == UnicodeText || characterSet == ANSIText {
		prepared.Joined = joinLines(prepared.Tagged)
		prepared.Rewritten = preprocessor.rewriteSpecialTokens(prepared.Joined, prepared.OneCharacter)
		sentences := prepared.Joined
		if prepared.Rewritten != nil {
			sentences = prepared.Rewritten
		}
		prepared.Final = preprocessor.splitSentences(sentences, nil)
	} else {
		prepared.Final = preprocessor.convertEnginePhonemes(prepared.Tagged)
	}
	return prepared
}

// ipaToEngineNames replaces each IPA symbol with its engine phoneme name and copies tags (text.md §2.2).
// A symbol not in the table, or a backslash in untagged text, ends the conversion.
func (preprocessor *Preprocessor) ipaToEngineNames(text []uint16, tagged bool) []uint16 {
	var converted []uint16
	insideTag := false
	for _, unit := range text {
		if unit == 0 {
			break
		}
		if unit == backslash {
			if !tagged {
				break
			}
			insideTag = !insideTag
			converted = append(converted, unit)
			continue
		}
		if insideTag {
			converted = append(converted, unit)
			continue
		}
		code := preprocessor.ipaCode(unit)
		if code == 0 {
			break
		}
		for _, character := range []byte(preprocessor.tables.Phonemes[code-1].WideName) {
			converted = append(converted, uint16(character))
		}
	}
	return converted
}

func (preprocessor *Preprocessor) ipaCode(unit uint16) int {
	for index, phoneme := range preprocessor.tables.Phonemes {
		if phoneme.IPA == unit {
			return index + 1
		}
	}
	return 0
}

// ToANSI converts to code page 1252 with best fit, up to the first U+0000; every other code unit,
// each half of a surrogate pair included, becomes '?' (text.md §2.1).
func (preprocessor *Preprocessor) ToANSI(text []uint16) []byte {
	converted := make([]byte, 0, len(text))
	for _, unit := range text {
		if unit == 0 {
			break
		}
		character, found := preprocessor.tables.BestFit[unit]
		if !found {
			character = '?'
		}
		converted = append(converted, character)
	}
	return converted
}

// Segments cuts the final text at every ESC I<n> (text.md §7).
func Segments(final []byte) []Segment {
	var segments []Segment
	var pitch []byte
	start := 0
	for {
		index := bytes.Index(final[start:], []byte{escape, 'I'})
		if index < 0 {
			return append(segments, Segment{Pitch: pitch, Text: final[start:]})
		}
		index += start
		segments = append(segments, Segment{Pitch: pitch, Text: final[start:index]})
		end := bytes.IndexByte(final[index+2:], space)
		if end < 0 {
			end = len(final)
		} else {
			end += index + 2
		}
		pitch = final[index+2 : end]
		start = min(end+1, len(final))
	}
}
