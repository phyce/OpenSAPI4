package datapack

import (
	"encoding/json"
	"fmt"
	"unicode/utf16"
)

// LongestIPARule is the most characters an IPA rule reads.
const LongestIPARule = 3

// Text is what TextData converts a text with: code page 1252's best fit, the byte Windows writes for each
// UTF-16 code unit it maps to something other than '?', and the IPA rules, the IPA sequences the engine reads
// with the engine phonemes each gives.
type Text struct {
	BestFit  map[uint16]byte
	IPARules []IPARule
}

type IPARule struct {
	Sequence []uint16
	Phonemes string
}

type textDescription struct {
	BestFit  [][2]int64  `json:"bestFit"`
	IPARules [][2]string `json:"ipaRules"`
}

func (pack *Pack) LoadText() (*Text, error) {
	return load(pack, "text.json", parseText)
}

func parseText(data []byte) (*Text, error) {
	var description textDescription
	if err := json.Unmarshal(data, &description); err != nil {
		return nil, err
	}
	text := &Text{BestFit: map[uint16]byte{}}
	for index, pair := range description.BestFit {
		code, value := pair[0], pair[1]
		if code <= 0 || code > 0xffff || utf16.IsSurrogate(rune(code)) || value <= 0 || value > 0xff || value == '?' {
			return nil, fmt.Errorf("bestFit[%d] is %v, not a code unit and a byte other than NUL and '?'", index, pair)
		}
		if _, found := text.BestFit[uint16(code)]; found {
			return nil, fmt.Errorf("bestFit[%d]: code unit %#x is mapped already", index, code)
		}
		text.BestFit[uint16(code)] = byte(value)
	}
	sequences := map[string]bool{}
	for index, pair := range description.IPARules {
		sequence := utf16.Encode([]rune(pair[0]))
		if len(sequence) == 0 || len(sequence) > LongestIPARule || pair[1] == "" || sequences[pair[0]] {
			return nil, fmt.Errorf("ipaRules[%d] is %q, not a new sequence of 1 to %d characters and its phonemes", index, pair, LongestIPARule)
		}
		sequences[pair[0]] = true
		text.IPARules = append(text.IPARules, IPARule{Sequence: sequence, Phonemes: pair[1]})
	}
	return text, nil
}
