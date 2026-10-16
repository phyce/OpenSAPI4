package text

import (
	"slices"
	"strconv"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
)

const (
	firstKeyRank = 10
	keyRankStep  = 4
)

// The keys \Vce=\ reads, by their place; only the speaker and the gender count.
var voiceKeys = []string{"speaker", "gender", "language", "accent", "dialect", "age", "style"}

// findVoice writes \Vce=\'s voice: the mode that matches the tag's speaker and gender best (engine.md §1.3). The
// first of the two keys ranks 10 and the second 6, a mode scores the ranks of the keys it matches, and the first
// mode of the highest score wins. The first \Vce\ in the process saves the engine's voice for \Rst\.
func (converter *Converter) findVoice(converted, text []byte, position, closing int) []byte {
	var speaker string
	var gender uint16
	var speakerRank, genderRank int
	if at(text, position) != backslash {
		rank := firstKeyRank
		for position < closing {
			before := position
			key, length, found := position, 0, false
			for length < longestValue {
				if text[key] == '=' {
					found = true
					break
				}
				key, length = key+1, length+1
				if key >= closing {
					break
				}
			}
			stopped := key
			if found {
				name := slices.Index(voiceKeys, lowered(text[position:key]))
				position = key + 1
				if at(text, position) == '"' {
					position++
				}
				value := field(text, position)
				position += len(value)
				if length := len(value); length > 0 && value[length-1] == '"' {
					value = value[:length-1]
				}
				stopped = len(value)
				switch name {
				case 0:
					speaker, speakerRank = lowered(value), rank
					rank -= keyRankStep
				case 1:
					switch lowered(value) {
					case "male":
						gender = datapack.GenderMale
					case "female":
						gender = datapack.GenderFemale
					case "neutral":
						gender = datapack.GenderNeutral
					}
					genderRank = rank
					rank -= keyRankStep
				}
			}
			if stopped == longestValue {
				for at(text, position) != backslash && position < closing && at(text, position) != ',' {
					position++
				}
			}
			if at(text, position) == ',' {
				position++
			}
			if at(text, position) == backslash {
				break
			}
			if position == before {
				// The original reads the same key again for ever; the port ends the tag.
				break
			}
		}
	}
	best, bestScore := 0, -1
	for index, mode := range converter.modes.Modes {
		score := 0
		if speakerRank != 0 && lowered([]byte(mode.Speaker)) == speaker {
			score += speakerRank
		}
		if genderRank != 0 && mode.Gender == gender {
			score += genderRank
		}
		if score > bestScore {
			best, bestScore = index, score
		}
	}
	converter.process.lock.Lock()
	if converter.process.savedVoice == noVoice {
		converter.process.savedVoice = converter.Voice
	}
	converter.process.lock.Unlock()
	converted = append(converted, escape, '[')
	converted = strconv.AppendInt(converted, int64(best), 10)
	return append(converted, 'V')
}

// pronunciation records a \Prn=word=pronunciation\ tag, or \Prn=word=//phonemes\ in engine phonemes, each part
// at most 19 characters, and writes ESC[b0;b1;b2;b3n and a space with its number. A tag without a pronunciation
// removes the word. The original writes the record's address.
func (converter *Converter) pronunciation(converted, text []byte, position, closing int) []byte {
	var record Pronunciation
	start := position
	for position < closing && position-start < longestValue && text[position] != '=' && text[position] != backslash {
		position++
	}
	record.Word = string(text[start:position])
	if text[position] == '=' {
		start = position + 1
		end := byte(backslash)
		if at(text, start) == '/' && at(text, start+1) == '/' {
			record.Phonetic, start, end = true, start+2, '/'
		}
		position = start
		for position < closing && position-start < longestValue && text[position] != end {
			position++
		}
		record.Pronunciation = string(text[start:position])
	}
	converter.pronunciationCount++
	converter.pronunciations[converter.pronunciationCount] = record
	converted = append(converted, escape, '[')
	converted = appendBytes(converted, converter.pronunciationCount)
	return append(converted, 'n', ' ')
}
