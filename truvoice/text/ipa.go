package text

import (
	"slices"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
)

// The commands around phonetic text: phonetic input on, then off.
const (
	phoneticStart = "\x1b[1I"
	phoneticEnd   = "\x1b[0I"
)

// ipa converts an IPA text to engine phonemes between the phonetic commands (engine.md §1.5): each IPA sequence
// the rules know gives its phonemes, the longest first, and a character that starts none gives nothing. A tagged
// text keeps its tags, but for their characters above 255, and drops \\. The caller cuts the result at its first
// NUL, which a NUL in a tag, or a backslash as the last character, puts in it.
func (converter *Converter) ipa(characters []uint16, tagged bool) []byte {
	length := len(characters)
	read := func(index int) uint16 {
		if index < length {
			return characters[index]
		}
		return 0
	}
	converted := []byte(phoneticStart)
	for position := 0; position < length; {
		if tagged && characters[position] == backslash {
			if read(position+1) == backslash {
				position += 2
				continue
			}
			end := position + 2
			for end < length {
				end++
				if characters[end-1] == backslash {
					break
				}
			}
			for index := position; index < end; index++ {
				if character := read(index); character < 0x100 {
					converted = append(converted, byte(character))
				}
			}
			position = end
			continue
		}
		if rule := converter.longestRule(characters, position); rule != nil {
			converted = append(converted, rule.Phonemes...)
			position += len(rule.Sequence)
		} else {
			position++
		}
	}
	return append(converted, phoneticEnd...)
}

// longestRule is the longest IPA rule whose sequence the characters have at position, or nil.
func (converter *Converter) longestRule(characters []uint16, position int) *datapack.IPARule {
	var longest *datapack.IPARule
	for index := range converter.tables.IPARules {
		rule := &converter.tables.IPARules[index]
		sequence := rule.Sequence
		if position+len(sequence) <= len(characters) && slices.Equal(characters[position:position+len(sequence)], sequence) &&
			(longest == nil || len(sequence) > len(longest.Sequence)) {
			longest = rule
		}
	}
	return longest
}
