package input

import (
	"strings"

	"github.com/phyce/OpenSAPI4/internal/locale"
)

const (
	phoneticMode   = 1
	quoteMode      = 4
	quotePrefix    = 5
	quoteSize      = 8
	quotedLines    = 3
	repeatedVowels = 3
)

// expand applies the tokenizer's rules to a token (input.md §4.4).
func (stage *Stage) expand(token *token) {
	tables, tokenizer := stage.tables, &stage.tokenizer
	text := cString(token.text)
	switch {
	case string(text) == "-":
		if (token.delimiter == 0 || token.delimiter == '$') && token.next != nil && token.next.number {
			if previous := token.previous; previous == nil || previous.delimiter == ' ' ||
				previous.delimiter == 0 && string(cString(previous.text)) == "\n" {
				token.replacement = tables.Minus
			}
		}
	case token.length >= repeatedVowels && strings.IndexByte("aeiouy", token.text[0]) >= 0 &&
		strings.Count(string(token.text[:token.length]), string(token.text[0])) == token.length:
		token.replacement = strings.ToUpper(string(text))
	case token.command:
		switch byteAt(text, 3) {
		case 'X':
			tokenizer.quoting = int32(int8(token.text[2])) - '0'
		case 'I':
			tokenizer.phonetic = int32(int8(token.text[2])) - '0'
		case 'e':
			tokenizer.wordStart = true
		}
	}
	if token.length == 1 && tables.Symbols[token.text[0]] != "" {
		token.replacement = tables.Symbols[token.text[0]]
	}
	if token.text != nil {
		for _, words := range tables.SplitWords {
			if sameWord(text, words[0]) {
				token.replacement = words[1]
				break
			}
		}
	}
	if token.length == 2 && sameWord(text, tables.Meridiem[0]) && token.previous != nil {
		previous := token.previous
		for previous != nil && (previous.delimiter == 0 || previous.delimiter == ' ') && (previous.command || previous.length == 0) {
			previous = previous.previous
		}
		if previous != nil && previous.number && (previous.delimiter == ' ' || previous.delimiter == 0) {
			token.replacement = tables.Meridiem[1]
		}
	}
	if token.length == 3 {
		for _, month := range tables.Months {
			if sameWord(text, month[0]) && stage.inDate(token) {
				token.replacement = month[1]
			}
		}
	}
	if token.delimiter == '!' {
		for previous := token.previous; previous != nil; previous = previous.previous {
			if previous.delimiter == '.' || previous.delimiter == '!' || previous.delimiter == '?' {
				return
			}
			if previous.length == 1 && strings.IndexByte("\"\\(", previous.text[0]) >= 0 {
				tokenizer.insertAfter(previous).text = []byte(tables.Exclamation)
				return
			}
		}
	}
}

// inDate reports whether a month's token is next to a number: after one, or before one, which a dot or a
// hyphen after the month then becomes a space before.
func (stage *Stage) inDate(month *token) bool {
	skipped := func(other *token) bool { return other.command || other.length == 0 && other.delimiter == ' ' }
	previous := month.previous
	for previous != nil && skipped(previous) {
		previous = previous.previous
	}
	if previous != nil && previous.number && strings.IndexByte(" -\x00", previous.delimiter) >= 0 {
		return true
	}
	next := month.next
	for next != nil && skipped(next) {
		next = next.next
	}
	if next == nil || !next.number || strings.IndexByte(" -.\x00", month.delimiter) < 0 {
		return false
	}
	if month.delimiter == '.' || month.delimiter == '-' {
		month.delimiter = ' '
	}
	return true
}

// quote follows quoted text in quote mode, at the first token of a line (input.md §4.6).
func (stage *Stage) quote(token *token) {
	tokenizer, marks := &stage.tokenizer, stage.tables.QuoteMarks
	matched, quoted, marked := 0, -1, false
	var prefix [quoteSize]byte
	if token.text == nil {
		tokenizer.quote[0] = 0
	} else {
		place := 0
		for ; place < min(token.length, quotePrefix); place++ {
			character := token.text[place]
			prefix[place] = character
			if character == tokenizer.quote[place] {
				matched++
			}
			if character == 0 || strings.IndexByte(marks, character) >= 0 {
				marked = true
				if matched == place+1 {
					quoted = place
				}
			}
		}
		if token.length < quotePrefix {
			character := token.delimiter
			prefix[place] = character
			if character == tokenizer.quote[place] {
				matched++
			}
			if character != 0 && strings.IndexByte(marks, character) >= 0 {
				marked = true
				if matched == place+1 {
					quoted = place
				}
			}
		}
		switch {
		case !marked:
			tokenizer.quote[0] = 0
		case tokenizer.quote[0] != 0 && quoted >= 0:
			tokenizer.prefix = int16(quoted + 1)
		default:
			name := cString(prefix[:])
			copy(tokenizer.quote[:], name)
			tokenizer.quote[len(name)] = 0
			tokenizer.repeats = 0
		}
	}
	if tokenizer.quoted {
		if quoted < 0 {
			tokenizer.announce(token, stage.tables.QuoteEnd).held = true
			tokenizer.quoted, tokenizer.repeats = false, 0
		} else {
			tokenizer.blank(token)
		}
		return
	}
	if quoted < 0 {
		return
	}
	if tokenizer.repeats++; tokenizer.repeats <= 1 {
		return
	}
	start := token
	for range quotedLines {
		for start.previous != nil && !start.previous.line() {
			start = start.previous
		}
		tokenizer.blank(start)
		if start.previous != nil {
			start = start.previous
		}
	}
	tokenizer.announce(start, stage.tables.QuoteStart)
	tokenizer.quoted = true
}

// blank makes the quote prefix of a line's first token spaces, unless the prefix is the whole token.
func (tokenizer *tokenizer) blank(token *token) {
	if int(tokenizer.prefix) < token.length {
		for place := range tokenizer.prefix {
			token.text[place] = ' '
		}
	}
}

// announce links a token saying an announcement before another.
func (tokenizer *tokenizer) announce(next *token, announcement string) *token {
	token := tokenizer.insertBefore(next)
	token.replacement, token.delimiter = announcement, ' '
	return token
}

// sameWord reports whether a token's characters are a word of the data pack's, as lstrcmpiA compares them.
func sameWord(text []byte, word string) bool {
	return locale.Key(text) == locale.Key([]byte(word))
}

func isLetter(character byte) bool {
	return character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z'
}

func byteAt(text []byte, place int) byte {
	if place < len(text) {
		return text[place]
	}
	return 0
}
