package text

import "bytes"

const (
	tokenGap              = 1
	tokenWord             = 3
	tokenAbbreviation     = 4
	tokenTitle            = 5
	tokenEllipsis         = 6
	tokenSpacedEllipsis   = 7
	tokenGroupBegin       = 8
	tokenGroupEnd         = 9
	tokenEndPunctuation   = 10
	tokenQuote            = 11
	tokenOtherPunctuation = 12
	tokenOther            = 13
	tokenParagraphEnd     = 14
	tokenEndOfText        = 15
	tokenSmiley           = 16

	classConsonant     = 1
	classVowel         = 2
	classDigit         = 3
	classPeriod        = 6
	classSpace         = 7
	classApostrophe    = 8
	classOpenGuillemet = 12
	classShutGuillemet = 13
	classPause         = 14
	classParagraphEnd  = 17
	classOther         = 21

	bracketDepth = 50
)

var singleTokens = map[byte]int{15: tokenEllipsis, 4: tokenEndPunctuation, 5: tokenEndPunctuation, 10: tokenGroupBegin, 11: tokenGroupEnd,
	9: tokenQuote, 8: tokenQuote, 12: tokenQuote, 13: tokenQuote, 14: tokenOtherPunctuation}

type sentenceToken struct {
	kind   int
	start  int
	length int
	isWord bool
}

// sentenceScanner hands out one token at a time and gives the same token again until it is accepted
// (text.md §6.2). Offsets are relative to the sentence start.
type sentenceScanner struct {
	breaker      *sentenceBreaker
	base         int
	length       int
	position     int
	scanPosition int
	pending      *sentenceToken
	saved        scannerMark
}

type scannerMark struct {
	position     int
	scanPosition int
	pending      *sentenceToken
}

func (scanner *sentenceScanner) at(offset int) byte {
	if offset < scanner.length {
		return scanner.raw(offset)
	}
	return 0
}

func (scanner *sentenceScanner) raw(offset int) byte {
	if index := scanner.base + offset; index >= 0 && index < len(scanner.breaker.text) {
		return scanner.breaker.text[index]
	}
	return 0
}

func (scanner *sentenceScanner) classAt(offset int) byte {
	if offset < scanner.length {
		return scanner.breaker.class(scanner.at(offset))
	}
	return 0
}

func (scanner *sentenceScanner) next() sentenceToken {
	if scanner.pending == nil {
		token := scanner.scan()
		scanner.pending = &token
	}
	return *scanner.pending
}

func (scanner *sentenceScanner) accept() {
	scanner.position = scanner.scanPosition
	scanner.pending = nil
}

func (scanner *sentenceScanner) mark() {
	scanner.saved = scannerMark{position: scanner.position, scanPosition: scanner.scanPosition}
	if scanner.pending != nil {
		token := *scanner.pending
		scanner.saved.pending = &token
	}
}

func (scanner *sentenceScanner) rewind() {
	scanner.position, scanner.scanPosition, scanner.pending = scanner.saved.position, scanner.saved.scanPosition, nil
	if scanner.saved.pending != nil {
		token := *scanner.saved.pending
		scanner.pending = &token
	}
}

func (scanner *sentenceScanner) matchSmiley(start int) int {
	index := start
	if character := scanner.raw(index); character == '>' || character == '}' {
		index++
	}
	if bytes.IndexByte([]byte(":;8%"), scanner.raw(index)) >= 0 && bytes.IndexByte([]byte("-^"), scanner.raw(index+1)) >= 0 &&
		bytes.IndexByte([]byte(")(>IOo0"), scanner.raw(index+2)) >= 0 && scanner.raw(index) != 0 {
		return index - start + 3
	}
	return 0
}

func (scanner *sentenceScanner) scan() sentenceToken {
	start := scanner.position
	scanner.scanPosition = start
	if start == scanner.length {
		return sentenceToken{kind: tokenEndOfText, start: start}
	}
	class := scanner.breaker.class(scanner.at(start))
	if class == classParagraphEnd {
		scanner.scanPosition = start + 1
		return sentenceToken{kind: tokenParagraphEnd, start: start, length: 1}
	}
	if length := scanner.matchSmiley(start); length != 0 {
		scanner.scanPosition = start + length
		return sentenceToken{kind: tokenSmiley, start: start, length: length}
	}
	switch class {
	case classSpace:
		position := start + 1
		escapeLength := 0
		for position < scanner.length {
			character := scanner.at(position)
			if scanner.breaker.class(character) != classSpace {
				if character != escape {
					break
				}
				escapeLength = 1
				for {
					escapeLength++
					position++
					if scanner.breaker.class(scanner.raw(position)) == classSpace || scanner.base+position >= len(scanner.breaker.text) {
						break
					}
				}
			}
			position++
		}
		scanner.scanPosition = position
		return sentenceToken{kind: tokenGap, start: start, length: position - escapeLength - start}
	case classPeriod:
		return scanner.scanPeriods(start)
	case classConsonant, classVowel, classDigit:
		return scanner.scanWord(start)
	}
	kind, found := singleTokens[class]
	if !found {
		kind = tokenOther
	}
	scanner.scanPosition = start + 1
	return sentenceToken{kind: kind, start: start, length: 1}
}

// scanPeriods reads ".", "...", or 3, 5 or 7 dots separated by single spaces.
func (scanner *sentenceScanner) scanPeriods(start int) sentenceToken {
	accepted := start + 1
	position := start + 1
	for state := 1; state > 0; {
		class := scanner.classAt(position)
		switch state {
		case 1:
			switch class {
			case classPeriod:
				state = 2
				position++
			case classSpace:
				position++
				state = 3
			default:
				state = -1
			}
		case 2:
			if position-start > 2 {
				accepted = position
				state = -1
			} else if class != classPeriod {
				state = -1
			} else {
				position++
			}
		case 3:
			if class != classPeriod {
				state = -1
			} else {
				state = 4
				position++
			}
		case 4:
			dots := (position - start) >> 1
			if dots == 2 || dots == 4 {
				accepted = position
			} else if dots == 6 {
				accepted = position
				state = -1
				continue
			}
			if class == classSpace {
				position++
				state = 3
			} else {
				state = -1
			}
		}
	}
	scanner.scanPosition = accepted
	length := accepted - start
	kind := tokenSpacedEllipsis
	switch length {
	case 1:
		kind = tokenEndPunctuation
	case 3:
		kind = tokenEllipsis
	}
	return sentenceToken{kind: kind, start: start, length: length}
}

// scanWord runs the word machine of text.md §6.2.
func (scanner *sentenceScanner) scanWord(start int) sentenceToken {
	breaker := scanner.breaker
	position := start
	state := 0
	for state >= 0 {
		class := scanner.classAt(position)
		character := scanner.at(position)
		hyphen := class == classOther && character == '-'
		apostrophe := class == classApostrophe && character == '\''
		switch state {
		case 0:
			switch class {
			case classConsonant:
				state = 10
				position++
			case classVowel:
				state = 11
				position++
			case classDigit:
				state = 60
				position++
			}
		case 10, 11:
			switch {
			case class == classConsonant || class == classDigit || hyphen:
				state = 10
				position++
			case class == classVowel || class == 16:
				state = 11
				position++
			case class == classPeriod:
				state = 20
				position++
			case apostrophe:
				state = 40
				position++
			default:
				state = -4
			}
		case 20:
			switch {
			case class == classConsonant || class == classDigit:
				state = 30
				position++
			case class == classVowel:
				state = 31
				position++
			case apostrophe:
				state = 40
				position++
			default:
				state = 50
			}
		case 30, 31:
			switch {
			case class == classConsonant || class == classDigit || hyphen:
				state = 30
				position++
			case class == classVowel || class == 16:
				state = 31
				position++
			case class == classPeriod:
				state = 20
				position++
			default:
				state = -4
			}
		case 40, 41:
			switch {
			case class == classConsonant || class == classDigit || hyphen:
				state = 40
				position++
			case class == classVowel || class == 16:
				state = 41
				position++
			case scanner.raw(position-1) == '\'' && scanner.raw(position-2) == '.':
				position--
				state = 50
			default:
				state = -4
			}
		case 50:
			word := breaker.text[scanner.base+start : scanner.base+position]
			switch {
			case breaker.isTitle(word, false):
				state = -3
			case breaker.isAbbreviation(word):
				state = -2
			case breaker.isTitle(word, true):
				state = -3
			default:
				position--
				state = -1
			}
		case 60:
			switch {
			case class == classDigit:
				position++
			case class == classPeriod || class == classPause && (character == ':' || character == ',') || hyphen:
				state = 61
				position++
			default:
				state = 66
			}
		case 61:
			if class == classDigit {
				state = 60
				position++
			} else {
				position--
				state = 66
			}
		case 66:
			switch {
			case class == classConsonant:
				state = 11
				position++
			case class == classVowel:
				state = 10
				position++
			case class == classApostrophe:
				if character != '\'' {
					state = -1
				} else {
					state = 40
					position++
				}
			default:
				state = -4
			}
		}
	}
	kind := tokenWord
	switch state {
	case -4:
		if position+3 <= scanner.length && scanner.raw(position) == '(' && scanner.raw(position+1) == 's' && scanner.raw(position+2) == ')' {
			position += 3
		}
	case -3:
		kind = tokenTitle
	case -2:
		kind = tokenAbbreviation
	}
	scanner.scanPosition = position
	return sentenceToken{kind: kind, start: start, length: position - start, isWord: true}
}

// breakerStep is one step of the state machine as the spec traces it: the state, the token's
// type, start and length, the two previous token types, words, labelEnd, quoteEnd, bracketSentence,
// atBreak, the quote counts (double, single, possessive, digit, guillemet), and the bracket stack's top
// index and top start (-1 for none).
type breakerStep [18]int

type sentenceBreaker struct {
	preprocessor *Preprocessor
	text         []byte
	base         int
	trace        *[]breakerStep
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (breaker *sentenceBreaker) class(character byte) byte {
	return breaker.preprocessor.tables.SentenceClasses[character]
}

func (breaker *sentenceBreaker) upper(character byte) bool {
	return breaker.preprocessor.tables.Upper[character]
}

func (breaker *sentenceBreaker) digit(character byte) bool {
	return isDigitByte(character)
}

func (breaker *sentenceBreaker) lowered(word []byte) []byte {
	lowered := make([]byte, len(word))
	for index, character := range word {
		lowered[index] = breaker.preprocessor.tables.ToLower[character]
	}
	return lowered
}

func (breaker *sentenceBreaker) word(token sentenceToken) []byte {
	start := breaker.base + token.start
	return breaker.text[start:min(start+token.length, len(breaker.text))]
}

func (breaker *sentenceBreaker) at(index int) byte {
	if index >= 0 && index < len(breaker.text) {
		return breaker.text[index]
	}
	return 0
}

func (breaker *sentenceBreaker) first(token sentenceToken) byte {
	return breaker.at(breaker.base + token.start)
}

func (breaker *sentenceBreaker) last(token sentenceToken) byte {
	return breaker.at(breaker.base + token.start + token.length - 1)
}

func (breaker *sentenceBreaker) equalsIgnoringCase(literal string, token sentenceToken) bool {
	word := breaker.word(token)
	return len(word) == len(literal) && bytes.Equal(breaker.lowered(word), breaker.lowered([]byte(literal)))
}

func (breaker *sentenceBreaker) isTitle(word []byte, capitalise bool) bool {
	if capitalise && len(word) > 0 {
		word = append([]byte{breaker.preprocessor.tables.ToUpper[word[0]]}, word[1:]...)
	}
	return breaker.preprocessor.tables.Titles[string(word)]
}

func (breaker *sentenceBreaker) isAbbreviation(word []byte) bool {
	tables := breaker.preprocessor.tables
	part := word
	for index := len(word) - 1; index > 0; index-- {
		if word[index] == '-' {
			part = word[index+1:]
			break
		}
	}
	if tables.Abbreviations[string(part)] {
		return true
	}
	if len(part) == 0 || !breaker.upper(part[0]) {
		return false
	}
	if tables.Abbreviations[string(breaker.lowered(part))] {
		return true
	}
	result := false
	if len(part) > 2 {
		vowels := 0
		anyLower := false
		for _, character := range part {
			if bytes.IndexByte([]byte("AEIOUYaeiouy"), character) >= 0 {
				vowels++
			}
			anyLower = anyLower || tables.Lower[character]
		}
		result = vowels == 0 && anyLower
	}
	if !result && len(part) > 3 && len(part)%2 == 0 {
		for index, character := range part {
			if index%2 == 0 && !tables.Alphabetic[character] || index%2 == 1 && character != '.' {
				return result
			}
		}
		result = true
	}
	return result
}

func (breaker *sentenceBreaker) isOrdinaryWord(token sentenceToken) bool {
	if token.kind != tokenWord || token.length < 2 {
		return false
	}
	word := breaker.word(token)
	if breaker.digit(word[0]) || bytes.IndexByte(word[1:], '.') >= 0 {
		return false
	}
	for _, character := range word {
		if bytes.IndexByte([]byte("IVXivx"), character) < 0 {
			return true
		}
	}
	return false
}

func (breaker *sentenceBreaker) isInitials(token sentenceToken) bool {
	if token.kind != tokenWord || token.length < 3 || token.length%2 != 1 {
		return false
	}
	for index, character := range breaker.word(token) {
		if index%2 == 0 && !breaker.upper(character) || index%2 == 1 && character != '.' {
			return false
		}
	}
	return true
}

func (breaker *sentenceBreaker) startsWithDigit(token sentenceToken) bool {
	return token.kind == tokenWord && breaker.digit(breaker.first(token))
}

func (breaker *sentenceBreaker) isNumberWord(token sentenceToken) bool {
	return breaker.preprocessor.tables.NumberWords[string(breaker.lowered(breaker.word(token)))]
}

func (breaker *sentenceBreaker) isSentenceStarter(token sentenceToken) bool {
	return breaker.upper(breaker.first(token)) && breaker.preprocessor.tables.Starters[string(breaker.word(token))]
}

// bracketStack holds the open brackets (text.md §6.3). The 51st push stores its kind in the first
// entry's start; the next one overflows the stack for good.
type bracketStack struct {
	overflow bool
	topIndex int
	kinds    [bracketDepth + 1]int
	starts   [bracketDepth + 1]int
}

func (stack *bracketStack) update(token sentenceToken, breaker *sentenceBreaker) {
	if stack.overflow {
		return
	}
	var kind int
	opening := true
	switch breaker.first(token) {
	case '(':
	case ')':
		opening = false
	case '[':
		kind = 2
	case ']':
		kind, opening = 2, false
	case '{':
		kind = 1
	case '}':
		kind, opening = 1, false
	default:
		return
	}
	if !opening {
		if stack.topIndex >= 0 {
			stack.topIndex--
		}
		return
	}
	old := stack.topIndex
	stack.topIndex++
	if old >= bracketDepth {
		stack.overflow = true
		return
	}
	if stack.topIndex == bracketDepth {
		stack.starts[0] = kind
	} else {
		stack.kinds[stack.topIndex] = kind
	}
	stack.starts[stack.topIndex] = token.start
}

func (stack *bracketStack) top() (int, bool) {
	if stack.overflow || stack.topIndex < 0 {
		return 0, false
	}
	return stack.starts[stack.topIndex], true
}

type quoteCounter struct {
	double     int
	single     int
	possessive int
	digit      int
	guillemet  int
}

func (counter *quoteCounter) count(token sentenceToken, opening bool, apostrophe bool, digitStart bool, breaker *sentenceBreaker) {
	first := breaker.first(token)
	if class := breaker.class(first); class == classOpenGuillemet || class == classShutGuillemet {
		if class == classOpenGuillemet {
			counter.guillemet++
		} else {
			counter.guillemet--
		}
		return
	}
	character := first
	if token.kind != tokenQuote {
		character = breaker.last(token)
	}
	switch character {
	case '"':
		if opening {
			counter.double++
		} else {
			counter.double--
		}
	case '\'':
		switch {
		case apostrophe:
			counter.possessive++
		case opening:
			if digitStart {
				counter.digit++
			}
			counter.single++
		default:
			counter.single--
		}
	}
}

func (counter *quoteCounter) countTrailingApostrophe(token sentenceToken, breaker *sentenceBreaker) bool {
	if breaker.last(token) != '\'' {
		return false
	}
	before := byte(0)
	if token.length > 1 {
		before = breaker.at(breaker.base + token.start + token.length - 2)
	}
	counter.count(token, false, token.length > 1 && (before == 's' || before == 'S'), false, breaker)
	return true
}

func (counter *quoteCounter) normalize() {
	counter.double = max(counter.double, 0)
	counter.guillemet = max(counter.guillemet, 0)
	counter.single = max(counter.single, 0)
}

func (counter *quoteCounter) hasOpen() bool {
	return counter.double > 0 || counter.guillemet > 0
}

type breakerSaved struct {
	words     int
	current   sentenceToken
	previous  sentenceToken
	previous2 sentenceToken
	brackets  bracketStack
	quotes    quoteCounter
}

// next finds the length of the sentence that starts at position (text.md §6.4).
func (breaker *sentenceBreaker) next(position int) int {
	breaker.base = position
	scanner := &sentenceScanner{breaker: breaker, base: position, length: len(breaker.text) - position}
	brackets := &bracketStack{topIndex: -1}
	quotes := &quoteCounter{}
	var current, previous, previous2 sentenceToken
	var saved breakerSaved
	words, labelEnd, quoteEnd := 0, 0, 0
	bracketSentence, atBreak := false, false
	currentType := 0
	accept := func() {
		scanner.accept()
		previous2 = previous
		previous = current
	}
	for state := 0; state != 23; {
		previousType := previous.kind
		if token := scanner.next(); token != current {
			current = token
			currentType = current.kind
			if currentType > 2 && currentType < 6 {
				words++
			}
		}
		kind := currentType
		if breaker.trace != nil {
			top, found := brackets.top()
			if !found {
				top = -1
			}
			*breaker.trace = append(*breaker.trace, breakerStep{state, kind, current.start, current.length, previous.kind, previous2.kind, words, labelEnd,
				quoteEnd, boolInt(bracketSentence), boolInt(atBreak), quotes.double, quotes.single, quotes.possessive, quotes.digit, quotes.guillemet,
				brackets.topIndex, top})
		}
		following := state
		switch state {
		case 0:
			following = 2
			switch {
			case kind <= 0:
			case kind < 3:
				accept()
				following = 0
			case kind == tokenGroupBegin:
				brackets.update(current, breaker)
				accept()
				following = 1
			case kind == tokenQuote:
				quotes.count(current, true, false, false, breaker)
				accept()
				following = 0
			}
		case 1:
			following = 2
			switch {
			case kind <= 0:
			case kind < 3:
				accept()
				following = 1
			case kind == tokenGroupBegin || kind == tokenGroupEnd:
				brackets.update(current, breaker)
				accept()
				following = 1
			}
		case 2:
			following = 7
			switch {
			case kind <= 0:
			case kind < 3:
				accept()
				following = 2
			case kind == tokenWord && !breaker.isOrdinaryWord(current):
				scanner.mark()
				saved = breakerSaved{words: words, current: current, previous: previous, previous2: previous2, brackets: *brackets, quotes: *quotes}
				accept()
				following = 3
			}
		case 3:
			following = 5
			switch {
			case kind < 1:
			case kind < 3:
				accept()
				following = 6
			case kind == tokenGroupEnd:
				brackets.update(current, breaker)
				accept()
				following = 4
			case kind == tokenEndPunctuation && breaker.first(current) == '.':
				accept()
				following = 4
			}
		case 4:
			following = 5
			switch {
			case kind <= 0:
			case kind < 3:
				labelEnd = current.start
				words = 0
				accept()
				following = 7
			case kind == tokenGroupEnd:
				brackets.update(current, breaker)
				accept()
				following = 4
			case kind == tokenEndPunctuation && breaker.first(current) == '.':
				accept()
				following = 4
			}
		case 5:
			scanner.rewind()
			words, current, previous, previous2 = saved.words, saved.current, saved.previous, saved.previous2
			restoredBrackets, restoredQuotes := saved.brackets, saved.quotes
			brackets, quotes = &restoredBrackets, &restoredQuotes
			currentType = tokenWord
			following = 7
		case 6:
			following = 5
			if kind == tokenGroupEnd {
				brackets.update(current, breaker)
				accept()
				following = 4
			}
		case 7:
			following = 7
			switch {
			case kind <= 0:
				accept()
			case kind < 3:
				accept()
				following = 18
			case kind < 6:
				bracketSentence = false
				if quotes.countTrailingApostrophe(current, breaker) {
					quotes.normalize()
				}
				switch kind {
				case tokenWord:
					accept()
					if current.length == 1 {
						if !breaker.digit(breaker.first(current)) {
							following = 11
						}
					} else if current.length%2 == 1 {
						following = 8
					}
				case tokenAbbreviation:
					accept()
					if !(words == 1 && (breaker.equalsIgnoringCase("PS.", current) || breaker.equalsIgnoringCase("P.S.", current))) {
						following = 12
					}
				default:
					if breaker.equalsIgnoringCase("Dr.", current) || breaker.equalsIgnoringCase("St.", current) {
						if previous2.kind != 0 && breaker.upper(breaker.first(previous2)) {
							current.kind = tokenAbbreviation
							following = 12
						}
					}
					accept()
				}
			case kind == tokenEllipsis || kind == tokenSpacedEllipsis:
				accept()
				following = 17
			case kind == tokenGroupBegin:
				brackets.update(current, breaker)
				accept()
			case kind == tokenGroupEnd:
				if top, found := brackets.top(); found && labelEnd <= top {
					bracketSentence = true
					following = 22
					for offset := 0; offset < top-labelEnd; offset++ {
						if breaker.class(breaker.at(breaker.base+offset)) != classSpace {
							bracketSentence = false
							following = 7
							break
						}
					}
				}
				brackets.update(current, breaker)
				accept()
			case kind == tokenEndPunctuation:
				accept()
				following = 14
			case kind == tokenQuote:
				accept()
				following = 10
			case kind == tokenOtherPunctuation || kind == tokenOther:
				accept()
			case kind == tokenParagraphEnd || kind == tokenEndOfText:
				following = 23
			case kind == tokenSmiley:
				accept()
				following = 17
			default:
				accept()
			}
		case 8:
			following = 7
			if kind == tokenEndPunctuation && breaker.first(current) == '.' && breaker.isInitials(previous) {
				accept()
				following = 9
			}
		case 9:
			following = 7
			if kind == tokenParagraphEnd || kind == tokenEndOfText {
				following = 23
			}
		case 10:
			digitStart := kind == tokenWord && breaker.startsWithDigit(current)
			if previous2.kind == 0 || previous2.kind == tokenGap || previous2.kind == tokenGroupBegin {
				quotes.count(previous, true, false, digitStart, breaker)
				quoteEnd = current.start
			} else {
				opening := previous.start == quoteEnd
				apostrophe := false
				if !opening {
					apostrophe = previous.length != 0 && breaker.first(previous) == '\'' && current.length != 0 &&
						breaker.preprocessor.tables.Alphanumeric[breaker.first(current)]
				} else {
					quoteEnd = current.start
				}
				quotes.count(previous, opening, apostrophe, digitStart, breaker)
			}
			quotes.normalize()
			following = 7
		case 11:
			following = 7
			if kind == tokenEndPunctuation && breaker.first(current) == '.' {
				accept()
				following = 12
			}
		case 12:
			following = 7
			switch {
			case kind == tokenGroupEnd:
				if previous2.kind != tokenGroupBegin {
					following = 19
				}
			case kind == 1 || kind == 2:
				if current.length < 2 {
					accept()
					following = 13
				} else {
					following = 19
				}
			case kind == tokenEndPunctuation:
				following = 19
			case kind == tokenQuote:
				if character := breaker.first(current); character == '"' || breaker.class(character) == classShutGuillemet {
					following = 19
				}
			case kind == tokenParagraphEnd || kind == tokenEndOfText:
				following = 23
			case kind == tokenSmiley:
				following = 19
			}
		case 13:
			following = 7
			switch {
			case kind > 2 && kind < 6:
				if breaker.equalsIgnoringCase("no.", previous2) || breaker.equalsIgnoringCase("fig.", previous2) || breaker.equalsIgnoringCase("figs.", previous2) {
					if !breaker.startsWithDigit(current) && !breaker.isNumberWord(current) {
						following = 23
					}
				} else if breaker.upper(breaker.first(current)) && (previous2.kind == tokenAbbreviation || previous2.kind == tokenEndPunctuation) {
					end := current.start + current.length + position
					if breaker.equalsIgnoringCase("in.", previous2) || breaker.equalsIgnoringCase("etc.", previous2) {
						following = 23
					} else if breaker.isSentenceStarter(current) && end < len(breaker.text) && breaker.text[end] != '.' {
						following = 23
					}
				}
			case kind == tokenGroupEnd || kind == tokenEndPunctuation:
				following = 19
			}
		case 14:
			following = 7
			switch {
			case kind <= 0:
			case kind < 3:
				following = 19
			case kind == tokenGroupEnd:
				bracketSentence = true
				following = 19
				if top, found := brackets.top(); found {
					for offset := 0; offset < top; offset++ {
						if breaker.preprocessor.tables.Alphanumeric[breaker.at(breaker.base+offset)] {
							accept()
							bracketSentence = false
							break
						}
					}
				}
			case kind == tokenEndPunctuation || kind == tokenSmiley:
				following = 19
			case kind == tokenQuote:
				if breaker.first(current) == '\'' {
					quotes.count(current, false, false, false, breaker)
					quotes.normalize()
					accept()
				}
				following = 19
			}
		case 15:
			following = 7
			switch {
			case kind > 2 && kind < 6:
				if breaker.upper(breaker.first(current)) {
					following = 23
				}
			case kind > 7 && kind < 11:
				following = 19
			case kind == tokenQuote:
				following = 23
			}
		case 17:
			following = 7
			if kind > 0 && kind < 3 {
				accept()
				following = 15
			}
		case 18:
			if kind == tokenEllipsis || kind == tokenSpacedEllipsis {
				accept()
			}
			following = 7
		case 19, 20, 21:
			if state == 19 {
				state = 20
				if quotes.hasOpen() {
					state = 21
				}
			}
			following = state
			switch {
			case kind == tokenGroupEnd:
				top, found := brackets.top()
				brackets.update(current, breaker)
				if !found || top == 0 {
					bracketSentence = true
				}
				accept()
			case kind == tokenGap:
				_, open := brackets.top()
				if !bracketSentence && current.length == 1 && previousType == tokenQuote {
					following = 22
				} else if state == 20 {
					following = 23
					if !bracketSentence && current.length == 1 && (previousType == tokenQuote || previousType == tokenGroupEnd || open) {
						following = 22
					} else if previous2.kind == tokenWord && previousType == tokenEndPunctuation && previous.length == 1 && breaker.first(previous) == '.' {
						atBreak = breaker.equalsIgnoringCase("at", previous2)
						if atBreak {
							following = 22
						}
					}
				}
				accept()
			case kind == 2 || kind == tokenEndPunctuation || kind == tokenSmiley:
				accept()
			case kind == tokenQuote:
				quotes.count(current, false, false, false, breaker)
				quotes.normalize()
				accept()
				if breaker.first(current) == '"' {
					following = 19
					if state == 20 {
						following = 21
					}
				}
			case kind == tokenOtherPunctuation:
				following = 7
			default:
				following = 23
			}
		case 22:
			following = 7
			switch {
			case kind <= 0:
			case kind < 3:
				accept()
				following = 22
			case kind < 6:
				if !atBreak {
					if breaker.upper(breaker.first(current)) {
						following = 23
					}
				} else if !breaker.equalsIgnoringCase("no.", current) {
					following = 23
				}
			case kind == tokenGroupBegin:
				following = 23
			case kind == tokenGroupEnd:
				following = 19
			case kind == tokenQuote:
				if previousType == tokenGap {
					following = 23
				}
			}
			atBreak = false
		}
		state = following
	}
	token := scanner.next()
	if token.kind == tokenParagraphEnd {
		current = token
		accept()
		token = scanner.next()
	}
	return token.start
}

// splitSentences copies each sentence with CR and LF as spaces and appends " \r\n" (text.md §6). Where
// the original would find an empty sentence and loop forever, the rest of the text is one sentence.
func (preprocessor *Preprocessor) splitSentences(text []byte, trace *[]breakerStep) []byte {
	var output []byte
	breaker := &sentenceBreaker{preprocessor: preprocessor, text: text, trace: trace}
	for position := 0; position < len(text); {
		length := breaker.next(position)
		if length <= 0 {
			length = len(text) - position
		}
		for _, character := range text[position : position+length] {
			if character == carriageReturn || character == lineFeed {
				character = space
			}
			output = append(output, character)
		}
		output = append(output, " \r\n"...)
		position += length
	}
	return output
}
