package preformatter

import (
	"strings"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
)

const (
	paragraphLimit = 5001
	shortLine      = 39
	dotRunSpaces   = 6
)

// Address formats a text in the address and e-mail contexts (section 3): it reads the text's lines as the
// spreadsheet context does (sections 1.1 and 1.2), joins them into paragraphs and spaces their words.
func Address(words *datapack.Address, text []byte) []byte {
	address := newAddress(words)
	read(string(text), address.piece)
	address.end()
	return []byte(address.output.String())
}

// address is the address context's words and what it has written of a text.
type address struct {
	functionWords, abbreviations, spacedAbbreviations map[string]bool
	output                                            strings.Builder
	paragraph                                         string
	// size is the measures of the paragraph's pieces (section 3.3).
	size int
	// lineStart is where the paragraph's last line starts in it.
	lineStart int
	// start is whether the next paragraph starts where a sentence starts.
	start bool
	// last is the text's last line when no line feed ends it, and single whether it is one character.
	last    string
	hasLast bool
	single  bool
}

func newAddress(words *datapack.Address) *address {
	return &address{functionWords: wordSet(words.FunctionWords), abbreviations: wordSet(words.Abbreviations),
		spacedAbbreviations: wordSet(words.SpacedAbbreviations)}
}

func wordSet(words []string) map[string]bool {
	set := make(map[string]bool, len(words))
	for _, word := range words {
		set[word] = true
	}
	return set
}

// piece joins a piece the reading gives to the paragraph (sections 3.1 to 3.3).
func (address *address) piece(piece string, end ending) {
	line := clean(piece)
	content := strings.TrimRight(strings.TrimLeft(piece, " \t"), " ")
	if end == textEnd {
		address.last, address.hasLast, address.single = line, true, len(content) == 1
		return
	}
	lineEndSize := 0
	if end == lineFeed {
		lineEndSize = len(lineEnd)
	}
	measure := len(line) + lineEndSize
	if end == cutEnding && len(content) <= 1 {
		character := strings.Trim(line, " ")
		if address.paragraph != "" {
			address.flush()
			if character == "" {
				character = " "
			}
		}
		address.output.WriteString(character + paragraphEnd)
		return
	}
	if strings.Trim(line, " ") == "" {
		if address.paragraph == "" {
			address.output.WriteString(lineEnd)
			return
		}
		address.flush()
		address.output.WriteString("." + paragraphEnd)
		address.start = true
		return
	}
	length := len(line)
	if strings.HasSuffix(line, "-") {
		if len(line) > 1 && line[len(line)-2] != ' ' {
			line = line[:len(line)-1]
		}
		line += " "
	}
	if strings.HasSuffix(line, ":") && !strings.Contains(line, " ") &&
		!strings.Contains(strings.TrimLeft(piece, " "), "\t") {
		line += " "
	}
	overflow := address.paragraph != "" && address.size+2*(len(line)+lineEndSize) > paragraphLimit
	if overflow {
		address.flush()
	}
	length += len(address.paragraph) - address.lineStart
	if address.lineStart > 0 {
		length++
	}
	switch {
	case strings.HasSuffix(line, ":") || end == lineFeed && length < shortLine:
		if strings.HasSuffix(line, ".") {
			address.paragraph += line + "  "
		} else {
			address.paragraph += line + ".  "
		}
	case end == lineFeed && endsInDotSpaces(line):
		address.paragraph += line
	default:
		address.paragraph += strings.TrimRight(line, " ") + " "
	}
	if end == lineFeed {
		address.lineStart = len(address.paragraph)
	}
	address.size += measure
	if overflow {
		address.flush()
	}
}

// end writes the text's last line and ends the last paragraph (section 3.5).
func (address *address) end() {
	if !address.hasLast {
		if address.paragraph != "" {
			address.flush()
			address.output.WriteString(" " + paragraphEnd)
		}
		return
	}
	last := strings.Trim(address.last, " ")
	if last != "" && !address.single && address.paragraph != "" &&
		address.size+2*(len(last)+1) > paragraphLimit {
		address.flush()
		address.paragraph = last
		address.flush()
		return
	}
	if last != "" && !address.single {
		address.paragraph += last
	}
	if address.paragraph == "" {
		address.output.WriteString(last + paragraphEnd)
		return
	}
	address.flush()
	if address.single {
		address.output.WriteString(last + paragraphEnd)
	} else {
		address.output.WriteString(" " + paragraphEnd)
	}
}

// flush spaces the paragraph's words (section 3.4) and writes it. A paragraph starts where the one before it
// ended, or where a sentence starts after a blank line; one that starts with a bracket where none does.
func (address *address) flush() {
	var paragraph string
	if strings.HasPrefix(address.paragraph, "(") {
		paragraph, address.start = address.spacing(" "+address.paragraph, false, false)
	} else {
		paragraph, address.start = address.spacing(address.paragraph, address.start, true)
	}
	address.output.WriteString(strings.TrimRight(paragraph, " ") + paragraphEnd)
	address.paragraph, address.size, address.lineStart = "", 0, 0
}

// clean makes a line's tabs spaces and each run of spaces one, but those after a dot, and drops its leading spaces.
func clean(line string) string {
	line = strings.ReplaceAll(line, "\t", " ")
	var cleaned strings.Builder
	for index := 0; index < len(line); {
		if line[index] != ' ' {
			cleaned.WriteByte(line[index])
			index++
			continue
		}
		run := spaceRun(line, index)
		if run-index >= 2 && (index == 0 || line[index-1] != '.') {
			cleaned.WriteByte(' ')
		} else {
			cleaned.WriteString(line[index:run])
		}
		index = run
	}
	return strings.TrimLeft(cleaned.String(), " ")
}

func spaceRun(text string, index int) int {
	for index < len(text) && text[index] == ' ' {
		index++
	}
	return index
}

func endsInDotSpaces(line string) bool {
	trimmed := strings.TrimRight(line, " ")
	return len(trimmed) < len(line) && strings.HasSuffix(trimmed, ".")
}

// kind is what a word ending in a dot is, for the sentence end test (section 3.4).
type kind int

const (
	wordKind kind = iota
	letterKind
	initialKind
	emptyKind
	numberKind
	startKind
	functionKind
)

// spacing spaces a paragraph's words (section 3.4), as a sentence's start if start is set or, with restart, if
// it has a closed number. It reports whether the paragraph ends where a sentence starts.
func (address *address) spacing(paragraph string, start, restart bool) (string, bool) {
	paragraph = wordDashes(commaDots(dotSpaces(dotRuns(paragraph))))
	trailing := len(paragraph) - len(strings.TrimRight(paragraph, " "))
	tokens, spaces := words(paragraph)
	initial := start
	for index := 0; !initial && restart && index < len(tokens); index++ {
		initial = restarts(tokens[index])
	}
	var output strings.Builder
	if len(tokens) > 0 {
		output.WriteString(spaces[0])
	}
	skipped, gap := 0, 0
	for index, token := range tokens {
		if index > 0 {
			if initial && skipped > 0 && capitalized(token) {
				gap = 2
			}
			output.WriteString(strings.Repeat(" ", max(gap, 1)))
		}
		gap = 0
		following, followingSpaces := "", trailing
		if index+1 < len(tokens) {
			following, followingSpaces = tokens[index+1], len(spaces[index+1])
		}
		if dotted, closed, ok := sentenceEnding(token); ok && (following != "" || followingSpaces > 0) {
			kind := address.dotKind(dotted, initial, closed)
			gap = address.dotGap(dotted, kind, followingSpaces, following)
			if strings.HasSuffix(token, "\".") && kind == wordKind && followingSpaces >= 2 {
				token = quotedDot(token)
			}
			if initial && kind == numberKind {
				skipped++
			} else {
				initial, skipped = address.startsAfter(dotted, kind), 0
			}
		} else if exclaimed(token) {
			gap = 2
			bare := strings.TrimRight(strings.TrimRight(token, ",):;\""), "!?")
			if dotted, closed, ok := sentenceEnding(bare); ok {
				initial, skipped = address.startsAfter(dotted, address.dotKind(dotted, initial, closed)), 0
			} else if initial && address.skippable(bare) {
				skipped++
			} else {
				initial, skipped = false, 0
			}
		} else if initial && address.skippable(token) {
			skipped++
		} else {
			initial, skipped = false, 0
		}
		output.WriteString(token)
	}
	output.WriteString(strings.Repeat(" ", trailing))
	return output.String(), initial
}

// dotRuns makes each run of two dots or more a dot and two spaces.
func dotRuns(paragraph string) string {
	var output strings.Builder
	for index := 0; index < len(paragraph); {
		if paragraph[index] != '.' {
			output.WriteByte(paragraph[index])
			index++
			continue
		}
		run := index
		for run < len(paragraph) && paragraph[run] == '.' {
			run++
		}
		if run-index >= 2 {
			output.WriteString(".  ")
		} else {
			output.WriteByte('.')
		}
		index = run
	}
	return output.String()
}

// dotSpaces makes each run of k spaces after a dot, k two or more, two spaces and (k-1)/6 dots, each followed by
// a space.
func dotSpaces(paragraph string) string {
	var output strings.Builder
	for index := 0; index < len(paragraph); {
		if paragraph[index] != ' ' {
			output.WriteByte(paragraph[index])
			index++
			continue
		}
		run := spaceRun(paragraph, index)
		if run-index >= 2 && index > 0 && paragraph[index-1] == '.' {
			output.WriteString("  " + strings.Repeat(". ", (run-index-1)/dotRunSpaces))
		} else {
			output.WriteString(paragraph[index:run])
		}
		index = run
	}
	return output.String()
}

// commaDots puts a space between a comma and a dot after a character that is no digit, comma or space.
func commaDots(paragraph string) string {
	var output strings.Builder
	for index := 0; index < len(paragraph); index++ {
		if paragraph[index] == ',' && index+1 < len(paragraph) && paragraph[index+1] == '.' && index > 0 &&
			!digit(paragraph[index-1]) && paragraph[index-1] != ',' && paragraph[index-1] != ' ' {
			output.WriteString(", .")
			index++
			continue
		}
		output.WriteByte(paragraph[index])
	}
	return output.String()
}

// wordDashes makes a space of each dash after a letter or a digit that ends a word.
func wordDashes(paragraph string) string {
	dashed := []byte(paragraph)
	for index := 1; index < len(dashed); index++ {
		if paragraph[index] == '-' && alphanumeric(paragraph[index-1]) &&
			(index+1 == len(paragraph) || paragraph[index+1] == ' ') {
			dashed[index] = ' '
		}
	}
	return string(dashed)
}

// words splits a paragraph into its words and the spaces before each.
func words(paragraph string) (tokens, spaces []string) {
	for index := 0; index < len(paragraph); {
		run := spaceRun(paragraph, index)
		if run == len(paragraph) {
			break
		}
		stop := run
		for stop < len(paragraph) && paragraph[stop] != ' ' {
			stop++
		}
		spaces = append(spaces, paragraph[index:run])
		tokens = append(tokens, paragraph[run:stop])
		index = stop
	}
	return tokens, spaces
}

// restarts is whether a word is a number of two digits or more, not all zeros, its dot and a closing bracket or
// a quote not followed by a bracket.
func restarts(token string) bool {
	digits := 0
	for digits < len(token) && digit(token[digits]) {
		digits++
	}
	if digits < 2 || digits+1 >= len(token) || token[digits] != '.' || strings.Trim(token[:digits], "0") == "" {
		return false
	}
	switch token[digits+1] {
	case ')', ']':
		return true
	case '"':
		return digits+2 == len(token) || token[digits+2] != ')'
	}
	return false
}

// sentenceEnding is whether a word ends like a sentence, in a dot or in a dot and one closing mark. It gives the word up
// to its dot and whether a bracket or a quote closes it.
func sentenceEnding(token string) (dotted string, closed, ok bool) {
	if strings.HasSuffix(token, ".") {
		return token, false, true
	}
	if len(token) < 2 || token[len(token)-2] != '.' {
		return "", false, false
	}
	switch token[len(token)-1] {
	case ')', '"':
		return token[:len(token)-1], true, true
	case ';', ',', ':':
		return token[:len(token)-1], false, true
	}
	return "", false, false
}

// startsAfter is whether a sentence starts after a word ending in a dot.
func (address *address) startsAfter(dotted string, kind kind) bool {
	return kind == startKind || kind != functionKind && kind != numberKind && !address.abbreviation(dotted)
}

// dotKind is what a word ending in a dot is, by its characters before the dot. A number closed by a bracket or a
// quote is a word where a sentence starts.
func (address *address) dotKind(token string, initial, closed bool) kind {
	kind := address.plainKind(token, initial)
	if closed && kind == numberKind && initial {
		return wordKind
	}
	return kind
}

func (address *address) plainKind(token string, initial bool) kind {
	if strings.HasSuffix(token, "\".") {
		letter := strings.TrimRight(token[:len(token)-2], ".!?")
		if len(letter) == 1 && alphabetic(letter[0]) {
			return letterKindOf(letter[0])
		}
		return wordKind
	}
	word := core(token)
	switch {
	case strings.HasSuffix(word, "\""):
		return wordKind
	case !strings.ContainsFunc(word, func(character rune) bool { return character < 128 && alphanumeric(byte(character)) }):
		return emptyKind
	case numeral(word, false):
		return numberKind
	case len(word) == 1 && alphabetic(word[0]):
		return letterKindOf(word[0])
	case address.functionWord(word) && initial:
		return startKind
	case address.functionWord(word):
		return functionKind
	}
	return wordKind
}

func letterKindOf(letter byte) kind {
	if upper(letter) {
		return initialKind
	}
	return letterKind
}

// core is a word ending in a dot without the dot, the marks before it and its opening brackets.
func core(token string) string {
	return strings.TrimLeft(strings.TrimRight(token[:len(token)-1], ")!?:;,."), "(")
}

func (address *address) abbreviation(token string) bool {
	if strings.HasSuffix(token, "\".") {
		token = token[:len(token)-2] + "."
	}
	return address.abbreviations[lower(core(token))]
}

func (address *address) functionWord(word string) bool {
	return len(word) > 1 && address.functionWords[lower(word)]
}

// skippable is whether a word keeps a sentence's start going: a function word or a number, in brackets or before
// a comma.
func (address *address) skippable(token string) bool {
	word := strings.TrimLeft(strings.TrimRight(strings.TrimLeft(token, "("), ",);:\""), "~")
	return address.functionWord(word) || numeral(strings.TrimLeft(word, "\""), true)
}

// dotGap is how many spaces follow a word ending in a dot, which spaces follow in the paragraph.
func (address *address) dotGap(token string, kind kind, spaces int, following string) int {
	if exclaimed(token[:len(token)-1]) {
		return 2
	}
	capital := 1
	if capitalized(following) {
		capital = 2
	}
	if spaces >= 2 {
		switch kind {
		case wordKind:
			return 2
		case letterKind, startKind, emptyKind, initialKind:
			return capital
		}
		return 1
	}
	if word := lower(core(token)); kind != startKind && address.abbreviations[word] && !address.spacedAbbreviations[word] {
		return 1
	}
	switch kind {
	case wordKind, letterKind, startKind, emptyKind:
		return capital
	}
	return 1
}

// quotedDot drops a quote before a word's last dot and the dot, ! or ? before the quote.
func quotedDot(token string) string {
	token = token[:len(token)-2]
	if token != "" && strings.IndexByte(".!?", token[len(token)-1]) >= 0 {
		token = token[:len(token)-1]
	}
	return token + "."
}

func exclaimed(token string) bool {
	token = strings.TrimRight(token, ",):;\"")
	return token != "" && (token[len(token)-1] == '!' || token[len(token)-1] == '?')
}

func capitalized(word string) bool {
	word = strings.TrimLeft(word, "(\"")
	return word != "" && upper(word[0]) && (len(word) == 1 || !upper(word[1]))
}

// numeral is whether a word is a digit followed by digits and dots, and dashes with dashes set.
func numeral(word string, dashes bool) bool {
	if word == "" || !digit(word[0]) {
		return false
	}
	for index := 1; index < len(word); index++ {
		if !digit(word[index]) && word[index] != '.' && (!dashes || word[index] != '-') {
			return false
		}
	}
	return true
}

func lower(word string) string {
	lowered := []byte(word)
	for index, character := range lowered {
		if upper(character) {
			lowered[index] = character + 'a' - 'A'
		}
	}
	return string(lowered)
}

func upper(character byte) bool {
	return character >= 'A' && character <= 'Z'
}

func alphabetic(character byte) bool {
	return upper(character) || character >= 'a' && character <= 'z'
}

func alphanumeric(character byte) bool {
	return alphabetic(character) || digit(character)
}
