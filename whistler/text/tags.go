package text

import (
	"bytes"
	"strconv"

	"github.com/phyce/OpenSAPI4/whistler/datapack"
	"github.com/phyce/OpenSAPI4/whistler/internal/locale"
)

type token int

const (
	tagCharacter  token = 0
	tagContext    token = 3
	tagEmphasis   token = 4
	tagMark       token = 9
	tagPause      token = 10
	tagPitch      token = 11
	tagPart       token = 14
	tagReset      token = 15
	tagSpeed      token = 16
	tagVolume     token = 18
	tagTransplant token = 19

	tokenEquals    token = 0x16
	tokenComma     token = 0x17
	tokenLess      token = 0x18
	tokenSemicolon token = 0x19
	tokenColon     token = 0x1A
	tokenQuoted    token = 0x1C
	tokenBraced    token = 0x1D
	tokenUnknown   token = 0x1E
	tokenNumber    token = 0x1F
	tokenBackslash token = 0x22
	tokenEnd       token = 0x23

	tokenSize        = 0x50
	addressContext   = 2
	addressOnEscape  = "\x1b[addr] "
	addressOffEscape = "\x1b[\\addr] "
)

var simpleTokens = map[int8]token{':': tokenColon, ';': tokenSemicolon, '<': tokenLess, '=': tokenEquals, '\\': tokenBackslash}

var tagKeywords = map[string]token{
	"chr": 0, "wrd": 1, "com": 2, "ctx": 3, "emp": 4, "eng": 5, "eow": 6, "fs": 7, "hs": 8, "mrk": 9, "pau": 10,
	"pit": 11, "prn": 12, "pro": 13, "prt": 14, "rst": 15, "spd": 16, "vce": 17, "vol": 18, "btp": 19, "etp": 20, "tp": 19,
}

// tagCursor reads the tokens of a tag (text.md §3.3) from the scanned text, which ends at its first NUL;
// marked whitespace is NUL too.
type tagCursor struct {
	text     []byte
	position int
}

func (cursor *tagCursor) byteAt(offset int) byte {
	if index := cursor.position + offset; index < len(cursor.text) {
		return cursor.text[index]
	}
	return 0
}

func lowerASCII(word []byte) []byte {
	lowered := make([]byte, len(word))
	for index, character := range word {
		if character >= 'A' && character <= 'Z' {
			character += 'a' - 'A'
		}
		lowered[index] = character
	}
	return lowered
}

func isDigitByte(character byte) bool {
	return character >= '0' && character <= '9'
}

func (cursor *tagCursor) next(size int) (token, []byte, uint32) {
	character := int8(cursor.byteAt(0))
	if character > 0x2F && character < 0x3A {
		var value uint32
		for isDigitByte(cursor.byteAt(0)) {
			value = uint32(cursor.byteAt(0)-'0') + value*10
			cursor.position++
		}
		return tokenNumber, nil, value
	}
	if character < 0x2D {
		switch character {
		case ',':
			cursor.position++
			return tokenComma, nil, 0
		case '"':
			kind, value := cursor.readQuoted(size)
			return kind, value, 0
		}
	} else {
		if kind, found := simpleTokens[character]; found {
			cursor.position++
			return kind, nil, 0
		}
		if character == '{' {
			kind, value := cursor.readBraced(size)
			return kind, value, 0
		}
	}
	kind, value := cursor.readWord(size)
	return kind, value, 0
}

func (cursor *tagCursor) readWord(size int) (token, []byte) {
	var word []byte
	for count := 0; count <= size-1; count++ {
		character := cursor.byteAt(0)
		value := int8(character)
		if character == 0 {
			return tokenEnd, word
		}
		if character == ',' || value > 0x39 && (value < 0x3C || character == '=' || character == '\\') {
			if kind, found := tagKeywords[locale.TagKey(word)]; found {
				return kind, word
			}
			return tokenUnknown, word
		}
		word = append(word, character)
		cursor.position++
	}
	return tokenEnd, word
}

func (cursor *tagCursor) readQuoted(size int) (token, []byte) {
	value := []byte{cursor.byteAt(0)}
	cursor.position++
	for count := 0; count < size-1; count++ {
		character := cursor.byteAt(0)
		if character == 0 {
			return tokenEnd, nil
		}
		if character == '"' {
			cursor.position++
			return tokenQuoted, append(value, character)
		}
		if character == '\\' {
			if cursor.byteAt(1) != '\\' {
				return tokenEnd, nil
			}
			cursor.position++
		}
		value = append(value, cursor.byteAt(0))
		cursor.position++
	}
	return tokenEnd, nil
}

func (cursor *tagCursor) readBraced(size int) (token, []byte) {
	var value []byte
	for {
		size--
		if size < 1 {
			return tokenEnd, nil
		}
		character := cursor.byteAt(0)
		if character == 0 || character == '\\' {
			return tokenEnd, nil
		}
		if character == '}' {
			break
		}
		value = append(value, character)
		cursor.position++
	}
	return tokenBraced, append(value, '}')
}

func (cursor *tagCursor) skipPastEnd() {
	for cursor.byteAt(0) != '\\' && cursor.byteAt(0) != 0 {
		cursor.position++
	}
	if cursor.byteAt(0) != 0 {
		cursor.position++
	}
}

// scanInteger reads "=%i" as msvcrt's sscanf does: optional whitespace and sign, 0x hexadecimal, a
// leading 0 octal, else decimal, wrapping modulo 2³².
func scanInteger(text []byte, position int) (int32, bool) {
	at := func(index int) byte {
		if index < len(text) {
			return text[index]
		}
		return 0
	}
	if at(position) != '=' {
		return 0, false
	}
	index := position + 1
	for at(index) == ' ' || at(index) >= '\t' && at(index) <= '\r' {
		index++
	}
	negative := false
	if at(index) == '+' || at(index) == '-' {
		negative = at(index) == '-'
		index++
	}
	base := uint32(10)
	digits := 0
	var value uint32
	if at(index) == '0' {
		index++
		digits = 1
		if at(index) == 'x' || at(index) == 'X' {
			index++
			base = 16
			digits = 0
		} else {
			base = 8
		}
	}
	for {
		character := at(index)
		var digit uint32
		switch {
		case isDigitByte(character) && uint32(character-'0') < base:
			digit = uint32(character - '0')
		case base == 16 && (character >= 'A' && character <= 'F' || character >= 'a' && character <= 'f'):
			digit = uint32(character|0x20) - 'a' + 10
		default:
			if digits == 0 {
				return 0, false
			}
			if negative {
				value = -value
			}
			return int32(value), true
		}
		value = value*base + digit
		digits++
		index++
	}
}

type tagScanner struct {
	tables    *datapack.Text
	controls  []Control
	output    []byte
	formatted []byte
}

func (scanner *tagScanner) addControl(kind ControlKind, value string) int {
	scanner.controls = append(scanner.controls, Control{Kind: kind, Value: value})
	return len(scanner.controls) - 1
}

func (scanner *tagScanner) addNumber(kind ControlKind, value uint32) int {
	return scanner.addControl(kind, strconv.FormatUint(uint64(value), 10))
}

// write appends an escape and remembers it as the last one formatted (an unknown \Prt= value repeats it).
func (scanner *tagScanner) write(escapeText []byte) {
	scanner.formatted = escapeText
	scanner.output = append(scanner.output, escapeText...)
}

func (scanner *tagScanner) reference(index int) {
	scanner.write([]byte("\x1bN" + strconv.Itoa(index) + " "))
}

// scan turns the tags into escape sequences and the control table and marks every word (text.md §3).
func (scanner *tagScanner) scan(ansi []byte, tagged bool, characterSet CharacterSet, address bool) ([]byte, []Control, bool) {
	text := bytes.ReplaceAll(ansi, []byte{escape}, []byte{space})
	end := len(text)
	inside := false
	for index, character := range text {
		if character == backslash {
			if tagged {
				inside = !inside
			}
		} else if (character == space || character == tab || character == lineFeed) && !inside {
			text[index] = 0
		}
	}
	text = append(text, 0)
	scanner.output = append(scanner.output, "\x1bAP "...)
	if address {
		scanner.write([]byte(addressOnEscape))
	}
	wordScale := 1
	if characterSet == UnicodeText || characterSet == UnicodeIPA {
		wordScale = 2
	}
	quotedLimit := tokenSize
	inWord := false
	pending := false
	var pendingEnd int64
	for position := 0; position < end; {
		if text[position] == 0 {
			text[position] = space
			if position > 0 && text[position-1] == carriageReturn {
				text[position] = lineFeed
			}
		}
		if pending && pendingEnd <= int64(position) {
			scanner.reference(scanner.addNumber(ControlTransplantEnd, 0))
			pending = false
		}
		character := text[position]
		if !tagged || character != backslash {
			switch {
			case character == space || character == lineFeed:
				inWord = false
			case character == tab || character == carriageReturn:
			case !inWord:
				inWord = true
				scanner.reference(scanner.addNumber(ControlWord, uint32(position*wordScale)))
			}
			scanner.output = append(scanner.output, character)
			position++
			continue
		}
		inWord = false
		cursor := &tagCursor{text: text, position: position + 1}
		tag, _, _ := cursor.next(tokenSize)
		skip := true
		switch tag {
		case tagContext, tagPart:
			if first, _, _ := cursor.next(tokenSize); first != tokenEquals {
				break
			}
			second, quoted, _ := cursor.next(quotedLimit)
			if second != tokenQuoted {
				break
			}
			if third, _, _ := cursor.next(tokenSize); third != tokenBackslash {
				break
			}
			skip = false
			if tag == tagContext {
				address = scanner.applyContext(quoted, address)
			} else {
				scanner.applyPartOfSpeech(quoted)
			}
		case tagCharacter:
			scanner.reference(scanner.addControl(ControlCharacter, scanner.tables.CharacterStyles[0]))
			if kind, _, _ := cursor.next(tokenSize); kind != tokenEquals {
				break
			}
			for {
				kind, quoted, _ := cursor.next(quotedLimit)
				if kind != tokenQuoted {
					break
				}
				scanner.reference(scanner.addControl(ControlCharacter, string(quoted)))
				separator, _, _ := cursor.next(tokenSize)
				if separator == tokenBackslash {
					skip = false
					break
				}
				if separator != tokenComma {
					break
				}
			}
		case tagEmphasis:
			scanner.output = append(scanner.output, "\x1b\" "...)
		case tagMark, tagPause, tagPitch, tagSpeed, tagVolume:
			if kind, _, _ := cursor.next(tokenSize); kind != tokenEquals {
				break
			}
			kind, _, value := cursor.next(4)
			if kind != tokenNumber {
				break
			}
			if closing, _, _ := cursor.next(tokenSize); closing != tokenBackslash {
				break
			}
			skip = false
			switch tag {
			case tagPause:
				scanner.write([]byte("\x1bH" + strconv.FormatUint(uint64(value), 10) + " "))
			case tagPitch:
				scanner.write([]byte("\x1bI" + strconv.FormatUint(uint64(value), 10) + " "))
			}
			scanner.reference(scanner.addNumber(ControlKind(tag), value))
		case tagReset:
			if address {
				scanner.write([]byte(addressOffEscape))
			}
			address = false
			scanner.output = append(scanner.output, "\x1bI0 "...)
			scanner.reference(scanner.addNumber(ControlReset, 0))
		case tagTransplant:
			skip = false
			if pending {
				scanner.reference(scanner.addNumber(ControlTransplantEnd, 0))
				pending = false
			}
			amount, read := scanInteger(text, cursor.position)
			closing := -1
			if limit := bytes.IndexByte(text[cursor.position:], 0); limit >= 0 {
				if found := bytes.IndexByte(text[cursor.position:cursor.position+limit], backslash); found >= 0 {
					closing = cursor.position + found
				}
			}
			switch {
			case closing < 0:
				cursor.position = end
			case !read:
				cursor.position = closing + 1
			default:
				offset := cursor.position
				if characterSet == UnicodeText || characterSet == UnicodeIPA {
					offset *= -2
				}
				scanner.reference(scanner.addNumber(ControlTransplantOffset, uint32(int32(offset))))
				quotedLimit = max(quotedLimit, closing+1-cursor.position)
				scanner.reference(scanner.addControl(ControlTransplant, string(text[cursor.position:closing])))
				pending = true
				pendingEnd = int64(closing) + 1 + int64(amount)
				cursor.position = closing + 1
			}
		case tokenBackslash:
			skip = false
			inWord = true
			scanner.reference(scanner.addNumber(ControlWord, uint32(cursor.position*wordScale)))
			scanner.output = append(scanner.output, backslash)
		}
		if skip {
			cursor.skipPastEnd()
		}
		position = cursor.position
	}
	if address {
		scanner.output = append(scanner.output, addressOffEscape...)
	}
	return scanner.output, scanner.controls, address
}

func (scanner *tagScanner) applyContext(name []byte, address bool) bool {
	for _, context := range scanner.tables.Contexts {
		if locale.TagKey([]byte(context.Name)) != locale.TagKey(name) {
			continue
		}
		if context.Value == addressContext {
			if !address {
				scanner.write([]byte(addressOnEscape))
			}
			return true
		}
		if address {
			scanner.write([]byte(addressOffEscape))
		}
		return false
	}
	return address
}

// applyPartOfSpeech writes a known value's escape; an unknown value writes the last escape formatted
// again, or nothing if there is none yet (text.md §10).
func (scanner *tagScanner) applyPartOfSpeech(value []byte) {
	for _, part := range scanner.tables.PartsOfSpeech {
		if locale.TagKey([]byte(part.Value)) == locale.TagKey(value) {
			scanner.formatted = []byte(part.Escape)
			break
		}
	}
	scanner.output = append(scanner.output, scanner.formatted...)
}
