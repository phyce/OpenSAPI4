package input

import (
	"bytes"
	"slices"
)

const (
	inputRingSize = 4096
	tokenWindow   = 210
	tokenLength   = 100
	runTokens     = 10
	keptTokens    = 100
)

// A character's token classes, the data pack's tokenClasses (input.md §4.1).
const (
	classSeparator   = 0x01
	classTrailing    = 0x02
	classLetter      = 0x04
	classDigit       = 0x08
	classPunctuation = 0x20
	classAgain       = 0x40
)

// The marks of a token's characters: where a split starts a new piece, and the dots that end one.
const (
	markNone  = 1
	markStart = 2
	markDot   = 3
)

// A token is a piece of the text the tokenizer reads (input.md §4.2): length characters (a NUL among them
// ends what it says of them), what it says instead of them, the character after it, where it splits, and
// whether it is a number, an escape sequence, or held from the expansions.
type token struct {
	previous, next *token
	text           []byte
	length         int
	replacement    string
	delimiter      byte
	marks          []byte
	number         bool
	command        bool
	held           bool
}

// The tokenizer's state (input.md §4): its list of tokens, from first to last, the current one, which it
// expanded last, and how many there are; the modes the escape sequences set; and what quote mode follows.
type tokenizer struct {
	first, last, current *token
	count                int
	phonetic, quoting    int32
	wordStart            bool
	quote                [quoteSize]byte
	quoted               bool
	prefix, repeats      int16
}

// Pump gives the escape parser the input ring's text through the TextIn tokenizer (input.md §4.5); reset
// starts a new text.
func (stage *Stage) Pump(reset bool) {
	if reset {
		stage.tokenizer.reset()
	}
	stage.output = stage.output[:0]
	stage.tokenize()
	for _, character := range stage.output {
		stage.Put(character)
	}
	if len(stage.output) > 0 {
		stage.Pipeline.OutOfInput = false
	}
}

func (tokenizer *tokenizer) reset() {
	tokenizer.first, tokenizer.last, tokenizer.current = nil, nil, nil
	tokenizer.count = 0
	tokenizer.quote[0] = 0
	tokenizer.quoted = false
}

// tokenize reads tokens, expands ten of them and says those more than a hundred before the current one; at
// the end of the text, with the input ring drained, it expands and says them all.
func (stage *Stage) tokenize() {
	stage.read()
	if !stage.Drained || !stage.AllFed {
		for range runTokens {
			stage.process()
		}
		stage.flush(false)
		return
	}
	for stage.process() {
	}
	stage.flush(true)
}

// read reads tokens from the input ring while the list holds fewer than 210 (input.md §4.2).
func (stage *Stage) read() {
	for range tokenWindow - stage.tokenizer.count {
		token := stage.readToken()
		if token == nil {
			return
		}
		stage.tokenizer.split(token)
		if stage.tokenizer.quoting == quoteMode && token.previous != nil && token.previous.line() {
			stage.quote(token)
		}
	}
}

// readToken reads the next token from the input ring, or returns nil when the ring has none.
func (stage *Stage) readToken() *token {
	var characters [tokenLength]byte
	var marks [tokenLength]byte
	classes := &stage.tables.TokenClasses
	length, leading, splits := 0, 0, 0
	number, word := false, false
	character, class, previousClass := int32(0), byte(0), byte(0)
	for {
		if length == tokenLength {
			character, class = 0, 0
			break
		}
		character = stage.get()
		if character == nothing {
			if length == 0 {
				return nil
			}
			character, class = 0, 0
			break
		}
		if character == escape {
			if length != 0 {
				class = classes[escape]
				break
			}
			if character = stage.readCommands(); character == nothing {
				return nil
			}
		}
		if stage.tables.Symbols[character] != "" {
			if length == 0 {
				characters[0] = byte(character)
				length = 1
			} else {
				stage.unget()
			}
			character, class = 0, 0
			break
		}
		class = classes[character]
		marks[length] = markNone
		if character == '\n' {
			if length == 0 {
				characters[0] = '\n'
				length = 1
				character, class = 0, 0
			}
			break
		}
		if class&classSeparator != 0 {
			break
		}
		if character == '.' && word {
			marks[length] = markDot
			splits++
		} else if class&classPunctuation != 0 {
			if number || word {
				break
			}
			leading++
		}
		if class&classDigit != 0 {
			if length == 0 {
				number = true
			} else if leading != 0 || previousClass&classPunctuation != 0 {
				break
			}
			if previousClass == classLetter {
				marks[length] = markStart
				splits++
			}
			word = false
		}
		if class&classLetter != 0 {
			if length == 0 {
				word = true
			} else if leading != 0 {
				break
			}
			if previousClass == classDigit {
				marks[length] = markStart
				splits++
			}
			number = false
		}
		characters[length] = byte(character)
		previousClass = class
		length++
	}
	if leading < length {
		for classes[characters[length-1]]&classTrailing != 0 {
			length--
			character = int32(characters[length])
			class = classes[character]
			stage.unget()
		}
	}
	token := stage.tokenizer.insertAfter(stage.tokenizer.last)
	token.length = length
	if length > 0 {
		token.text = slices.Clone(characters[:length])
	}
	if class&(classLetter|classDigit|classAgain) != 0 {
		character = 0
		stage.unget()
	}
	token.delimiter = byte(character)
	if string(token.text) == "-$" && character == 0 {
		token.text[1] = 0
		token.delimiter = '$'
	}
	if number {
		token.number = true
		return token
	}
	if splits != 0 {
		token.marks = slices.Clone(marks[:length])
	}
	return token
}

// readCommands reads the escape sequences after an escape character into tokens of their own (input.md
// §4.2), and returns the character after them, or -1 when the input ring runs out first.
func (stage *Stage) readCommands() int32 {
	character := stage.get()
	for character != nothing {
		if character != escape {
			if character != '[' {
				return character
			}
			command := []byte{escape, '['}
			for {
				if character = stage.get(); character == nothing {
					return nothing
				}
				command = append(command, byte(character))
				if stage.tables.TokenClasses[character]&classLetter != 0 {
					break
				}
			}
			token := stage.tokenizer.insertAfter(stage.tokenizer.last)
			token.text, token.length, token.command = command, len(command), true
			if character = stage.get(); character != escape {
				return character
			}
		}
		character = stage.get()
	}
	return nothing
}

// split splits a token where its marks say, and makes the tokens it splits into numbers when their characters
// are all digits (input.md §4.3).
func (tokenizer *tokenizer) split(whole *token) {
	if whole.marks == nil {
		return
	}
	var first *token
	piece := make([]byte, 0, whole.length)
	for position := 0; position < whole.length; position++ {
		if mark := whole.marks[position]; mark != markNone {
			before := tokenizer.insertBefore(whole)
			if first == nil {
				first = before
			}
			if mark == markDot {
				before.delimiter = whole.text[position]
				position++
			}
			before.text, before.length = slices.Clone(piece), len(piece)
			piece = piece[:0]
		}
		piece = append(piece, whole.text[position])
	}
	if first == nil {
		return
	}
	whole.text, whole.length, whole.marks = slices.Clone(piece), len(piece), nil
	for piece := whole; ; piece = piece.previous {
		piece.classifyNumber()
		if piece == first {
			return
		}
	}
}

func (token *token) classifyNumber() {
	for _, character := range cString(token.text) {
		if character < '0' || character > '9' {
			return
		}
	}
	token.number = true
}

// process expands the token after the current one, which becomes the current one, and reports whether there
// was one (input.md §4.4).
func (stage *Stage) process() bool {
	tokenizer := &stage.tokenizer
	next := tokenizer.first
	if tokenizer.current != nil {
		next = tokenizer.current.next
	}
	if next == nil {
		return false
	}
	if (tokenizer.phonetic != phoneticMode || next.command) && !next.held {
		if next.text != nil && tokenizer.wordStart && isLetter(next.text[0]) {
			tokenizer.wordStart = false
			mark := tokenizer.insertBefore(next)
			mark.text, mark.length = []byte(stage.tables.WordStart), 1
		}
		stage.expand(next)
	}
	tokenizer.current = next
	return true
}

// flush says the tokens before the hundred before the current one, or all of them, and drops them.
func (stage *Stage) flush(all bool) {
	tokenizer := &stage.tokenizer
	if tokenizer.first == nil || tokenizer.current == nil {
		return
	}
	var end *token
	if !all {
		end = tokenizer.current
		for range keptTokens {
			if end.previous == nil {
				break
			}
			end = end.previous
		}
	}
	for token := tokenizer.first; token != nil && token != end; token = tokenizer.dropFirst() {
		switch {
		case token.replacement != "":
			stage.output = append(stage.output, token.replacement...)
		case token.text != nil:
			stage.output = append(stage.output, cString(token.text)...)
		}
		if token.delimiter != 0 {
			stage.output = append(stage.output, token.delimiter)
		}
	}
}

// insertAfter links a new token after another, or makes it the first when there is none.
func (tokenizer *tokenizer) insertAfter(previous *token) *token {
	token := &token{previous: previous}
	if previous == nil {
		tokenizer.first = token
	} else {
		token.next = previous.next
		previous.next = token
		if token.next != nil {
			token.next.previous = token
		}
	}
	if tokenizer.last == previous {
		tokenizer.last = token
	}
	tokenizer.count++
	return token
}

// insertBefore links a new token before another.
func (tokenizer *tokenizer) insertBefore(next *token) *token {
	token := &token{previous: next.previous, next: next}
	next.previous = token
	if token.previous == nil {
		tokenizer.first = token
	} else {
		token.previous.next = token
	}
	tokenizer.count++
	return token
}

// dropFirst unlinks the first token and returns the one after it. As in the original, the last token stays the
// last.
func (tokenizer *tokenizer) dropFirst() *token {
	first := tokenizer.first
	tokenizer.first = first.next
	if first.next != nil {
		first.next.previous = nil
	}
	tokenizer.count--
	return tokenizer.first
}

// line reports whether a token is a line end.
func (token *token) line() bool {
	return token.text != nil && token.text[0] == '\n'
}

// get takes the next character of the input ring, or -1 when it is empty, which marks it drained.
func (stage *Stage) get() int32 {
	if stage.InputRead == stage.InputWritten {
		stage.Drained = true
		return nothing
	}
	character := stage.Input[stage.InputRead]
	stage.InputRead = (stage.InputRead + 1) % inputRingSize
	return int32(character)
}

// unget gives the input ring back the character taken last.
func (stage *Stage) unget() {
	stage.InputRead = (stage.InputRead + inputRingSize - 1) % inputRingSize
}

// cString is a token's characters up to the first NUL.
func cString(text []byte) []byte {
	if end := bytes.IndexByte(text, 0); end >= 0 {
		return text[:end]
	}
	return text
}
