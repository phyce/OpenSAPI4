package text

import (
	"bytes"
	"strconv"
)

const (
	recordSize   = 201
	recordText   = 101
	escapeLimit  = 100
	recordLimit  = 99
	emoticonSide = 33
)

func isLetter(character byte) bool {
	return character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z'
}

func isLetterOrDigit(character byte) bool {
	return isLetter(character) || isDigitByte(character)
}

func cString(data []byte, offset int) []byte {
	if offset >= len(data) {
		return nil
	}
	end := bytes.IndexByte(data[offset:], 0)
	if end < 0 {
		return data[offset:]
	}
	return data[offset : offset+end]
}

func concatenate(parts ...[]byte) []byte {
	return bytes.Join(parts, nil)
}

// splitPunctuation splits a text into the bytes before its first letter or digit, the core up to its
// last letter or digit, and the rest (text.md §5.2).
func splitPunctuation(text []byte) (lead []byte, core []byte, trail []byte, found bool) {
	index := 0
	for index >= len(text) || !isLetterOrDigit(text[index]) {
		if index >= len(text) || index+1 > recordLimit {
			return nil, nil, nil, false
		}
		index++
	}
	last := len(text) - 1
	for !isLetterOrDigit(text[last]) {
		if len(text)-last > recordLimit {
			return nil, nil, nil, false
		}
		last--
	}
	return text[:index], text[index : last+1], text[last+1:], true
}

// specialTokens rewrites records of an escape group and one word (text.md §5) in one flat buffer, as the
// original does: a text longer than its record runs on into the next record. Writes past the end of the
// buffer are dropped.
type specialTokens struct {
	preprocessor *Preprocessor
	oneCharacter bool
	memory       []byte
}

func (preprocessor *Preprocessor) rewriteSpecialTokens(joined []byte, oneCharacter bool) []byte {
	tokens := &specialTokens{preprocessor: preprocessor, oneCharacter: oneCharacter}
	return tokens.rewrite(joined)
}

func (tokens *specialTokens) write(offset int, data []byte) {
	if offset >= len(tokens.memory) {
		return
	}
	data = append(append([]byte(nil), data...), 0)
	copy(tokens.memory[offset:], data)
}

func (tokens *specialTokens) rewrite(joined []byte) []byte {
	count := len(joined) >> 1
	if count == 0 {
		return nil
	}
	tokens.memory = make([]byte, count*recordSize)
	text := append(append([]byte(nil), joined...), 0)
	position := 0
	record := 0
	for text[position] != 0 {
		for text[position] == space {
			position++
		}
		if text[position] != escape {
			return nil
		}
		var escapes []byte
		for text[position] == escape {
			end := position
			for text[end] != space {
				if text[end] == 0 {
					return nil
				}
				end++
			}
			escapes = append(escapes, text[position:end]...)
			position = end
			for {
				escapes = append(escapes, space)
				position++
				if text[position] != space {
					break
				}
			}
			if len(escapes) > escapeLimit {
				return nil
			}
		}
		tokens.write(record*recordSize, escapes)
		var words []byte
		for text[position] != escape && text[position] != 0 {
			words = append(words, text[position])
			if len(words) > recordLimit {
				return nil
			}
			position++
		}
		tokens.write(record*recordSize+recordText, words)
		if text[position] == 0 {
			break
		}
		record++
	}
	changed := false
	for index := 0; index <= record; index++ {
		if tokens.rewriteRecord(index * recordSize) {
			tokens.memory[index*recordSize+escapeLimit] = 0
			changed = true
		}
	}
	if !changed {
		return nil
	}
	var output []byte
	for index := 0; index <= record; index++ {
		output = append(output, cString(tokens.memory, index*recordSize)...)
		output = append(output, cString(tokens.memory, index*recordSize+recordText)...)
	}
	return output
}

func (tokens *specialTokens) name(character byte) []byte {
	return []byte(tokens.preprocessor.tables.CharacterNames[character])
}

func (tokens *specialTokens) rewriteRecord(offset int) bool {
	text := append([]byte(nil), cString(tokens.memory, offset+recordText)...)
	if len(text) == 0 {
		return false
	}
	if tokens.oneCharacter {
		tokens.write(offset+recordText, concatenate([]byte{space}, tokens.name(text[0]), []byte{space}))
		return true
	}
	recognisers := []func([]byte) []byte{tokens.expandEmoticon, tokens.cleanCharacters, tokens.expandAddress, groupThousands, spacePlusSigns}
	for _, recognise := range recognisers {
		if result := recognise(text); result != nil {
			tokens.write(offset+recordText, result)
			return true
		}
	}
	return false
}

func (tokens *specialTokens) expandEmoticon(text []byte) []byte {
	var eyes, mouth byte
	var before, after []byte
	for _, character := range text {
		switch {
		case eyes == 0:
			if character == ':' || character == ';' {
				eyes = character
				continue
			}
			before = append(before, character)
			if len(before) > emoticonSide {
				return nil
			}
		case mouth != 0:
			after = append(after, character)
			if len(after) > emoticonSide {
				return nil
			}
		case character == '(' || character == ')':
			mouth = character
		case character != '-':
			before = append(before, character)
			if len(before) > emoticonSide {
				return nil
			}
			eyes = 0
		}
	}
	if mouth == 0 {
		return nil
	}
	result := append(before, " ("...)
	if eyes == ';' {
		result = append(result, "winking "...)
	}
	if mouth == ')' {
		result = append(result, "smiley face "...)
	} else {
		result = append(result, "frowning face "...)
	}
	return append(append(result, ')'), after...)
}

func (tokens *specialTokens) expandTilde(text []byte) []byte {
	start := 0
	for text[start] == space {
		start++
		if start >= len(text) {
			return nil
		}
	}
	var word []byte
	if start+1 < len(text) && isDigitByte(text[start+1]) {
		word = []byte("approximately")
		if len(word)+5+len(text) > recordLimit {
			return nil
		}
	} else {
		word = tokens.name(text[start])
	}
	if len(word) == 0 {
		return nil
	}
	rest := 0
	for rest < len(text) && (text[rest] == '~' || text[rest] == space) {
		rest++
	}
	return concatenate([]byte{space}, word, []byte{space}, text[rest:])
}

func (tokens *specialTokens) cleanCharacters(text []byte) []byte {
	start := 0
	for text[start] == space {
		start++
		if start >= len(text) {
			return nil
		}
	}
	last := len(text) - 1
	for start < last && (text[last] == space || text[last] == carriageReturn) {
		last--
	}
	if last-start == 2 && text[start] == '"' && text[start+2] == '"' && (text[start+1] > 0x20 && text[start+1] < 0x41 || text[start+1] > 0x7D) {
		name := tokens.name(text[start+1])
		if len(name)+2 > recordLimit {
			return nil
		}
		return append(name, space)
	}
	if text[start] == '~' {
		return tokens.expandTilde(text)
	}
	if bytes.IndexByte(text[start:], '*') >= 0 {
		if len(text) > 0x62 {
			return nil
		}
		lead, core, trail, found := splitPunctuation(text)
		if !found {
			return nil
		}
		for _, character := range lead {
			if character != '*' && character != space {
				return nil
			}
		}
		for _, character := range core {
			if !isLetter(character) && character != '\'' {
				return nil
			}
		}
		return concatenate(lead, []byte{space}, core, []byte{space}, trail)
	}
	var output []byte
	changed := false
	for _, character := range text[start:] {
		switch {
		case character < 0x80 && character != '\\' && character != '_', character == 0xA0, character >= 0x91 && character <= 0x94, character == 0xAB, character == 0xBB:
			switch character {
			case 0xA0:
				character, changed = space, true
			case 0x91, 0x92:
				character, changed = '\'', true
			case 0x93, 0x94:
				character, changed = '"', true
			}
			if (character > 0x7F || character < 0x20) && character != lineFeed && character != carriageReturn && character != escape {
				character, changed = space, true
			}
			output = append(output, character)
			if len(output) > escapeLimit {
				return nil
			}
		default:
			if folded := tokens.preprocessor.tables.AccentFolding[character]; character > 0x7F && folded != 0 {
				output = append(output, folded)
			} else {
				output = append(append(append(output, space), tokens.name(character)...), space)
				if len(output) > recordLimit {
					return nil
				}
			}
			changed = true
		}
	}
	if !changed || len(output) > escapeLimit {
		return nil
	}
	return output
}

var addressSeparators = map[byte]string{'-': "-", '/': "/", ':': ":", '<': "<", '>': "<", '@': "@"}

var spokenSeparators = map[string]string{".": " dot ", "-": " dash ", "_": " underscore ", ":": " colon ", "/": " slash ", "@": " at "}

func (tokens *specialTokens) addressPart(part []byte, separator string) []byte {
	if len(part) == 1 && !isDigitByte(part[0]) {
		part = tokens.name(part[0])
	} else if string(part) == "edu" {
		part = []byte("ee dee yuu")
	}
	return append(append([]byte(nil), part...), spokenSeparators[separator]...)
}

func (tokens *specialTokens) expandAddress(text []byte) []byte {
	if len(text) >= recordLimit {
		return nil
	}
	lead, core, trail, found := splitPunctuation(text)
	if !found {
		return nil
	}
	dots := 0
	lastDot := -1
	special := false
	letters := false
	for index, character := range core {
		if character == '.' {
			dots++
			lastDot = index
		} else if !isLetterOrDigit(character) {
			special = true
		}
		if isLetter(character) {
			letters = true
		}
		if character == space {
			dots = 0
			break
		}
	}
	if dots == 1 && lastDot >= 0 && (!special || !letters) {
		count := 0
		for _, character := range core[lastDot+1:] {
			if !isLetter(character) {
				dots = 0
				break
			}
			count++
		}
		if count == 3 {
			special = true
		} else {
			letters = false
		}
	}
	if dots == 0 || !special && dots <= 1 || !letters && dots <= 1 {
		return nil
	}
	output := []byte{space}
	if string(lead) == "<" {
		output = append(output, "left angle bracket "...)
	} else {
		output = append(output, lead...)
	}
	var part []byte
	for index, character := range core {
		separator, isSeparator := addressSeparators[character]
		if character == '.' {
			following := byte(0)
			if index+1 < len(core) {
				following = core[index+1]
			}
			if following != space && following != lineFeed && following != carriageReturn {
				separator, isSeparator = ".", true
			} else {
				output = append(append(output, tokens.addressPart(part, "")...), ',')
				part = nil
				continue
			}
		}
		if isSeparator {
			output = append(output, tokens.addressPart(part, separator)...)
			if len(output) >= recordText {
				return nil
			}
			part = nil
		} else if character != space {
			part = append(part, character)
		}
	}
	output = append(output, tokens.addressPart(part, "")...)
	if len(trail) > 0 && trail[0] == '>' {
		if len(trail) == 1 {
			trail = []byte("right angle bracket")
		} else {
			output = append(output, "right angle bracket "...)
			trail = trail[1:]
		}
	}
	return append(output, trail...)
}

func groupThousands(text []byte) []byte {
	if len(text) >= recordLimit {
		return nil
	}
	lead, core, trail, found := splitPunctuation(text)
	if !found {
		return nil
	}
	var digits []byte
	for _, character := range core {
		if character == space || character == lineFeed || character == carriageReturn {
			break
		}
		digits = append(digits, character)
		if !isDigitByte(character) {
			return nil
		}
	}
	if len(digits) != 4 {
		return nil
	}
	if value, _ := strconv.Atoi(string(digits)); value > 1491 && value < 2100 {
		return nil
	}
	return concatenate(lead, digits[:1], []byte{','}, digits[1:], []byte{space}, trail)
}

func spacePlusSigns(text []byte) []byte {
	if len(text) >= recordLimit {
		return nil
	}
	lead, core, trail, found := splitPunctuation(text)
	if !found {
		return nil
	}
	for _, character := range core {
		if !isLetterOrDigit(character) && character != '+' && character != lineFeed && character != carriageReturn {
			return nil
		}
	}
	spaced := bytes.ReplaceAll(core, []byte{'+'}, []byte(" + "))
	if len(spaced) > recordLimit {
		return nil
	}
	return concatenate(lead, spaced, trail)
}
