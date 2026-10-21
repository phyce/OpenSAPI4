package preformatter

import (
	"strings"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
)

const (
	paragraphLimit = 5001
	shortLine      = 39
	dotRunSpaces   = 6
	// keptSpace is a space in a word that the spacing keeps whole.
	keptSpace = "\x00"
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
	headers                                           map[string]datapack.Header
	messageStart                                      string
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
	// state is where the e-mail reading is, messages how many messages it has seen start, and author whether the
	// last block had an Author: line (section 3.6).
	state    state
	messages int
	author   bool
	// afterSubject is whether the line before was a Subject: line, and subjectGoesOn whether it was one in a header
	// block.
	afterSubject, subjectGoesOn bool
	// namesGoOn is how the next line goes on with an Author:, names or other value that ended in a comma.
	namesGoOn datapack.HeaderAction
	// held is a line of a header block that ended in a comma, if holding.
	held    string
	holding bool
}

func newAddress(words *datapack.Address) *address {
	headers := make(map[string]datapack.Header, len(words.Headers))
	for _, header := range words.Headers {
		headers[header.Name] = header
	}
	return &address{functionWords: wordSet(words.FunctionWords), abbreviations: wordSet(words.Abbreviations),
		spacedAbbreviations: wordSet(words.SpacedAbbreviations), headers: headers, messageStart: words.MessageStart}
}

func wordSet(words []string) map[string]bool {
	set := make(map[string]bool, len(words))
	for _, word := range words {
		set[word] = true
	}
	return set
}

// piece reads a piece the reading gives as part of an e-mail message (section 3.6) and joins what is left of it
// to the paragraph.
func (address *address) piece(piece string, end ending) {
	if rest, left := address.email(piece, end); left {
		address.join(rest, end, true, piece, true)
	}
}

// join joins a piece to the paragraph (sections 3.1 to 3.3), with a blank line's end if edits is set. If colons is
// set, its colons get spaces when the line it was read as gets them (section 3.2).
func (address *address) join(piece string, end ending, edits bool, read string, colons bool) {
	line := clean(piece)
	if colons && colonsSpaced(read) {
		line = spaceColons(line)
	}
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
	if edits && strings.Trim(line, " ") == "" {
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
		address.paragraph += line + ".  "
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
		address.paragraph = last
		address.flush()
	} else {
		address.output.WriteString(" " + paragraphEnd)
	}
}

// flush reads the paragraph's marks (section 3.4), spaces its words and writes it. A paragraph starts where the one
// before it ended, or where a sentence starts after a blank line; one that starts with a bracket where none does.
// One that its marks leave no word of is a space.
func (address *address) flush() {
	marked := marks(address.paragraph)
	var paragraph string
	if strings.HasPrefix(address.paragraph, "(") {
		paragraph, address.start = address.spacing(" "+marked, false, false)
	} else {
		paragraph, address.start = address.spacing(marked, address.start, true)
	}
	if strings.Trim(marked, " ") == "" {
		paragraph = " "
	} else {
		paragraph = strings.TrimRight(unmarked(paragraph), " ")
	}
	address.output.WriteString(paragraph + paragraphEnd)
	address.paragraph, address.size, address.lineStart = "", 0, 0
}

// clean makes a line's tabs spaces and each run of spaces one, but those after a dot, and drops its leading spaces.
func clean(line string) string {
	return strings.TrimLeft(collapse(strings.ReplaceAll(line, "\t", " ")), " ")
}

// collapse makes each run of two spaces or more one space, but those after a dot.
func collapse(line string) string {
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
	return cleaned.String()
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
	paragraph = startDots(address.dotRuns(address.loosened(commas(paragraph, true))))
	paragraph = commas(underscores(slashes(dashes(pluses(paragraph)))), false)
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
	skipped, gap, joined := 0, 0, 0
	terminalBefore := false
	held := ""
	for index, token := range tokens {
		if index > 0 {
			if initial && skipped > 0 && capitalized(token) {
				gap = 2
			}
			width := max(gap, 1)
			if terminal(token) && !terminalBefore {
				width = joined
			}
			if held != "" && !terminal(token) {
				output.WriteString(held)
				held = ""
			}
			output.WriteString(strings.Repeat(" ", width))
		}
		terminalBefore = index > 0 && terminal(token)
		spread := strings.HasSuffix(token, keptSpace+"/")
		if strings.HasSuffix(token, "/") && token != "/" && token != ">/" && !strings.HasSuffix(token, "(/") {
			held = " /"
			if spread {
				held = "  /"
			}
			token = strings.TrimRight(token[:len(token)-1], keptSpace)
		}
		gap, joined = 0, 0
		following, followingSpaces := "", trailing
		if index+1 < len(tokens) {
			following, followingSpaces = tokens[index+1], len(spaces[index+1])
		}
		if dotted, closed, ok := sentenceEnding(token); ok {
			kind := address.dotKind(dotted, initial, closed)
			gap = address.dotGap(dotted, kind, followingSpaces, following)
			if kind == wordKind && followingSpaces >= 2 {
				joined = 1
			}
			if (followingSpaces >= 2 || spread) && (kind == wordKind || kind == startKind || kind == functionKind) {
				if cut := cutClosers(token); cut != token {
					token, joined = cut, 1
				}
				if strings.HasSuffix(token, "\".") {
					token = quotedDot(token)
				}
			}
			if initial && kind == numberKind {
				skipped++
			} else {
				initial, skipped = address.startsAfter(dotted, kind), 0
			}
		} else if exclaimed(token) {
			gap = 2
			bare := strings.TrimRight(strings.TrimRight(strings.TrimRight(token, ",):;\""), "!?"), keptSpace)
			if dotted, closed, ok := sentenceEnding(bare); ok {
				initial, skipped = address.startsAfter(dotted, address.dotKind(dotted, initial, closed)), 0
			} else if initial && address.skippable(bare) {
				skipped++
			} else {
				initial, skipped = false, 0
			}
		} else if dotted, closed, rest, ok := splitEnding(token); ok {
			initial, skipped = address.startsAfter(dotted, address.dotKind(dotted, initial, closed)), 0
			if initial && rest != "" && strings.Trim(rest, ",;)") == "" {
				skipped = 1
			}
		} else if initial && address.skippable(token) {
			skipped++
		} else {
			initial, skipped = false, 0
		}
		output.WriteString(token)
	}
	output.WriteString(held)
	output.WriteString(strings.Repeat(" ", trailing))
	return strings.ReplaceAll(output.String(), keptSpace, " "), initial
}

// dotRuns spaces each run of dots and the spaces after it.
func (address *address) dotRuns(paragraph string) string {
	var output strings.Builder
	for index := 0; index < len(paragraph); {
		if paragraph[index] != '.' {
			output.WriteByte(paragraph[index])
			index++
			continue
		}
		dots := index
		for dots < len(paragraph) && paragraph[dots] == '.' {
			dots++
		}
		stop := spaceRun(paragraph, dots)
		output.WriteString(address.dotRun(paragraph, index, dots, stop))
		index = stop
	}
	return output.String()
}

// dotRun is a run of dots and the spaces after it: a dot and two spaces, and a dot and a space more for every six
// dots and spaces after the first two, a run before a word counted as if one space followed it and keeping its last
// one or two dots on the word. A dot before one space or none stays. Two dots before a word are a dot: before a + a
// dot and two spaces; after a word that is no function word, letter or number (or a quoted one) and before what
// splits it, or before a slash both parts keep, a dot and a space that keeps the word whole; after a single
// character that is no digit, a dot, a space and a dot, but before a slash, a bracket, a dash or a word of commas,
// semicolons, ! and ?.
func (address *address) dotRun(paragraph string, start, dots, stop int) string {
	count, spaces := dots-start, stop-dots
	attached := spaces == 0 && stop < len(paragraph)
	if count == 1 && spaces <= 1 {
		return paragraph[start:stop]
	}
	if count == 2 && attached {
		before := paragraph[strings.LastIndexAny(paragraph[:start], " ,")+1 : start]
		following := paragraph[stop:]
		if strings.HasPrefix(following, "+") {
			return ".  "
		}
		wordy := address.plainKind(before+".", false) == wordKind ||
			strings.HasSuffix(before, "\"") && len(core(before+".")) > 1
		if wordy && (splits(following) || strings.HasPrefix(following, "/") && kept(before+".")) {
			return "." + keptSpace
		}
		if len(before) == 1 && !digit(before[0]) && !pairStays(following) {
			return ". ."
		}
		return "."
	}
	extra := (count + max(spaces, 1) - 2) / dotRunSpaces
	if attached && count-dotRunSpaces*extra <= 2 {
		return ".  " + strings.Repeat(". ", extra-1) + "."
	}
	return ".  " + strings.Repeat(". ", extra)
}

func splits(text string) bool {
	switch {
	case strings.HasPrefix(text, ",") || strings.HasPrefix(text, "-"):
		return true
	case strings.HasPrefix(text, "; ") || strings.HasPrefix(text, ") "):
		return true
	}
	run := terminalRun(text, 0)
	switch {
	case run == 0:
		return false
	case run == len(text) || text[run] == ' ' || text[run] == ',':
		return true
	}
	return strings.HasPrefix(text[run:], ". ") && (run+2 == len(text) || text[run+2] != ' ')
}

// pairStays is whether two dots after a single character stay a dot before a text: one that starts with a slash, an
// opening bracket or a dash, or with commas, semicolons, ! and ? before a space, a dot, a comma or its end.
func pairStays(text string) bool {
	if text != "" && strings.IndexByte("/(-", text[0]) >= 0 {
		return true
	}
	run := 0
	for run < len(text) && strings.IndexByte(",;!?", text[run]) >= 0 {
		run++
	}
	return run > 0 && (run == len(text) || text[run] == ' ' || text[run] == '.' || strings.Contains(text[1:run], ","))
}

func terminalRun(text string, index int) int {
	for index < len(text) && (text[index] == '!' || text[index] == '?') {
		index++
	}
	return index
}

// startDots makes a paragraph's first dot of a dot, a dot and two spaces one, but before a word of ! and ?.
func startDots(paragraph string) string {
	for strings.HasPrefix(paragraph, ". .  ") {
		run := terminalRun(paragraph, 5)
		if run > 5 && (run == len(paragraph) || paragraph[run] == ' ') {
			break
		}
		paragraph = ". " + paragraph[5:]
	}
	return paragraph
}

// commas spaces the commas of a paragraph's words, with signs only those before a sign.
func commas(paragraph string, signs bool) string {
	words := strings.Split(paragraph, " ")
	for index, word := range words {
		words[index] = wordCommas(word, signs)
	}
	return strings.Join(words, " ")
}

// wordCommas puts a space after each comma of a word that doesn't start with one: after a character that is no
// digit, dot, comma or sign (+, - or /), or a dot after a character that is no dot, if more of the word follows;
// after a digit, or a dot that starts the word, if the commas after it are followed by a character that is no digit,
// dot, comma or opening bracket, nor a dash or a slash in a word that starts with a digit or a dot. With signs, only
// the commas before a sign are spaced, and in a word that starts with no digit or dot a comma right before a + after
// no sign is doubled first.
func wordCommas(word string, signs bool) string {
	if word == "" || word[0] == ',' {
		return word
	}
	number := digit(word[0]) || word[0] == '.'
	if signs && !number {
		var doubled strings.Builder
		for index := 0; index < len(word); index++ {
			doubled.WriteByte(word[index])
			if word[index] == ',' && index+1 < len(word) && word[index+1] == '+' && !sign(word[index-1]) {
				doubled.WriteByte(',')
			}
		}
		word = doubled.String()
	}
	var spaced strings.Builder
	for index := 0; index < len(word); index++ {
		spaced.WriteByte(word[index])
		if word[index] == ',' && commaSpaced(word, index, number) && (!signs || signed(word[index+1:])) {
			spaced.WriteByte(' ')
		}
	}
	return spaced.String()
}

func commaSpaced(word string, index int, number bool) bool {
	previous := word[index-1]
	switch {
	case !digit(previous) && previous != '.' && previous != ',' && !sign(previous),
		previous == '.' && index >= 2 && word[index-2] != '.':
		return index+1 < len(word)
	case digit(previous) || number && previous == '.' && index == 1:
		rest := strings.TrimLeft(word[index+1:], ",")
		return rest != "" && !digit(rest[0]) && strings.IndexByte(".,(", rest[0]) < 0 &&
			!(number && (rest[0] == '-' || rest[0] == '/'))
	}
	return false
}

func sign(character byte) bool {
	return character == '+' || character == '-' || character == '/'
}

// signed is whether a text starts with a sign after its commas.
func signed(text string) bool {
	text = strings.TrimLeft(text, ",")
	return text != "" && sign(text[0])
}

// cutClosers keeps the last two of three or more closing marks before a word's last dot.
func cutClosers(token string) string {
	if !strings.HasSuffix(token, ".") {
		return token
	}
	run := len(token)
	for run > 0 && strings.IndexByte(")];!?.", token[run-1]) >= 0 {
		run--
	}
	if run == 0 || len(token)-run <= 3 {
		return token
	}
	return token[:run] + token[len(token)-3:]
}

func terminal(token string) bool {
	return terminalRun(token, 0) == len(token)
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

// splitEnding is whether a word that the spacing keeps whole ends like a sentence before its kept space. It gives
// the word up to its dot, whether a bracket or a quote closes it, and the word's part after the space.
func splitEnding(token string) (dotted string, closed bool, rest string, ok bool) {
	before, after, found := strings.Cut(token, keptSpace)
	if !found {
		return "", false, "", false
	}
	dotted, closed, ok = sentenceEnding(before)
	rest, _, _ = strings.Cut(after, keptSpace)
	return dotted, closed, rest, ok
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
		switch {
		case len(letter) > 1:
			return wordKind
		case letter != "" && alphabetic(letter[0]):
			return letterKindOf(letter[0])
		case letter != "" && digit(letter[0]):
			return numberKind
		}
		return emptyKind
	}
	word := core(token)
	switch {
	case strings.HasSuffix(word, "\"") && len(word) > 1:
		return wordKind
	case word == "" || word == "\"":
		return emptyKind
	case !strings.ContainsFunc(word, func(character rune) bool { return character < 128 && alphanumeric(byte(character)) }):
		if len(word) == 1 {
			return initialKind
		}
		return wordKind
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

// core is a word ending in a dot without the dot, the marks before it, its opening brackets and a leading > that
// goes.
func core(token string) string {
	word := token[:len(token)-1]
	if strings.HasPrefix(word, ">") && (len(word) == 1 || !digit(word[1])) {
		word = word[1:]
	}
	return strings.TrimLeft(strings.TrimRight(word, ")]!?:;,."), "([")
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

// skippable is whether a word keeps a sentence's start going: a function word or a number (a dot may start it), in
// brackets or before a comma.
func (address *address) skippable(token string) bool {
	word := strings.TrimLeft(strings.TrimRight(strings.TrimLeft(token, "("), ",);:\""), "~")
	return address.functionWord(word) || numeral(strings.TrimPrefix(strings.TrimLeft(word, "\""), "."), true)
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
	word = strings.TrimLeft(word, "(\">")
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
