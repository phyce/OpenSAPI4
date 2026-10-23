package preformatter

import (
	"cmp"
	"strings"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
)

const (
	paragraphLimit = 5001
	shortLine      = 39
	dotRunSpaces   = 6
	// markLimit is what a paragraph's measure and what its marks add stay below for them to be read (section 3.4),
	// longRaw what its lines reach for it to be joined the long way and wideningLimit where a word starts for the gap
	// before it to keep more spaces (section 3.3).
	markLimit     = 4999
	longRaw       = 5000
	wideningLimit = 5000
	// keptSpace is a space in a word that the spacing keeps whole.
	keptSpace = "\x00"
	// cutSpace is a space that a closing run keeps in its marks, droppedDot a dot that a dot run leaves out, and
	// hidden what starts and ends the marks that a closing run takes out.
	cutSpace   = "\x01"
	droppedDot = "\x02"
	hidden     = "\x03"
	// joinedSpace stands between a word and the ! and ? joined to it, and droppedSquare for a [ that goes (a [ after
	// it stays). A paragraph has no line feeds or tabs of its own.
	joinedSpace   = "\n"
	droppedSquare = "\t"
	// keptGap is the two spaces a long paragraph keeps between two lines, and widenedOnce and widenedTwice a gap that
	// keeps one or two spaces more when the word after it starts at wideningLimit or later.
	keptGap      = "\x04"
	widenedOnce  = "\x05"
	widenedTwice = "\x06"
	// closingRoom is how many marks a closing run keeps.
	closingRoom = 4
)

// widenings marks a gap by how many spaces more it keeps.
var widenings = [...]string{1: widenedOnce, 2: widenedTwice}

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
	// longParagraph is the paragraph joined the long way and rawSize the measures that decide whether it is (section
	// 3.3).
	longParagraph string
	rawSize       int
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
	if trimmed := strings.TrimRight(line, " "); end == cutEnding && trimmed != line {
		measure = len(trimmed) + 1
	}
	if end == cutEnding && len(content) <= 1 {
		character := strings.Trim(line, " ")
		if address.paragraph == "" {
			address.output.WriteString(character + paragraphEnd)
			return
		}
		address.flush()
		address.paragraph = character
		address.flush()
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
	short := strings.HasSuffix(line, ":") || end == lineFeed && length < shortLine
	address.longParagraph += longJoined(line, end, short)
	trimmed := strings.TrimRight(line, " ")
	switch {
	case short:
		address.paragraph += line + ".  "
	case end == lineFeed && endsInDotSpaces(line):
		address.paragraph += line
	default:
		address.paragraph += trimmed + " "
	}
	switch {
	case end != lineFeed:
		address.rawSize += rawLength(trimmed) + 1
	case short:
		address.rawSize += rawLength(line) + len(lineEnd) + 1
	default:
		address.rawSize += rawLength(line) + len(lineEnd)
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
		address.longParagraph += last
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
// One that its marks leave no word of is a space. One whose lines reach longRaw, one more if a separator unit stands
// twice in it, is joined the long way, without a kept gap at its end, and of its marks only its opening brackets
// change (section 3.3).
func (address *address) flush() {
	paragraph := address.paragraph
	rawSize := address.rawSize
	if doubledSeparator(paragraph) {
		rawSize--
	}
	long := rawSize >= longRaw
	var marked string
	if long {
		paragraph, _ = strings.CutSuffix(address.longParagraph, keptGap+" ")
		marked = brackets(paragraph, 0, true)
	} else {
		marked = marks(paragraph, address.rawSize)
	}
	if strings.HasPrefix(paragraph, "(") {
		paragraph, address.start = address.spacing(marked, false, false, long)
	} else {
		paragraph, address.start = address.spacing(marked, address.start, true, long)
	}
	if strings.Trim(marked, " ") == "" {
		paragraph = " "
	} else {
		paragraph = strings.TrimRight(unmarked(paragraph), " ")
	}
	address.output.WriteString(widened(paragraph) + paragraphEnd)
	address.paragraph, address.size, address.lineStart = "", 0, 0
	address.longParagraph, address.rawSize = "", 0
}

// rawLength is a line's length as the long paragraph test counts it: a run of dots and the spaces after it counts up
// to two less, one for each of its dots and spaces after the fifth.
func rawLength(line string) int {
	length := len(line)
	for index := 0; index < len(line); {
		if line[index] != '.' {
			index++
			continue
		}
		dots := index
		for dots < len(line) && line[dots] == '.' {
			dots++
		}
		stop := spaceRun(line, dots)
		if stop > dots {
			length -= min(max(stop-index-5, 0), 2)
		}
		index = stop
	}
	return length
}

// longJoined is a piece as a long paragraph joins it: a short line as any paragraph does, and a line that ends in
// spaces, a comma or a dash with two spaces kept whatever follows (its dot runs see the spaces it ends in), but not
// one that ends in commas the spacing leaves a word of their own (after a space, comma, dash, plus or slash).
func longJoined(line string, end ending, short bool) string {
	trimmed := strings.TrimRight(line, " ")
	ended := strings.HasSuffix(trimmed, ",") || strings.HasSuffix(trimmed, "-")
	alone := len(trimmed) >= 2 && trimmed[len(trimmed)-1] == ',' && strings.IndexByte(" ,-+/", trimmed[len(trimmed)-2]) >= 0
	switch {
	case short:
		return line + ".  "
	case end != lineFeed || trimmed == line && (!ended || alone):
		return trimmed + " "
	case trimmed != line:
		return line + keptGap + " "
	}
	return trimmed + " " + keptGap + " "
}

// widened gives each gap that keeps more spaces its spaces when the word after it starts at wideningLimit or later
// with them.
func widened(paragraph string) string {
	var output strings.Builder
	for index := 0; index < len(paragraph); index++ {
		var extra int
		switch paragraph[index : index+1] {
		case widenedOnce:
			extra = 1
		case widenedTwice:
			extra = 2
		default:
			output.WriteByte(paragraph[index])
			continue
		}
		if output.Len()+spaceRun(paragraph, index+1)-index-1+extra >= wideningLimit {
			output.WriteString(strings.Repeat(" ", extra))
		}
	}
	return output.String()
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

// dashRunsSpaced makes a space of the first dash of each run of dashes after something else.
func dashRunsSpaced(paragraph string) string {
	spaced := []byte(paragraph)
	for index := 1; index+1 < len(paragraph); index++ {
		if paragraph[index] == '-' && paragraph[index+1] == '-' && paragraph[index-1] != ' ' && paragraph[index-1] != '-' {
			spaced[index] = ' '
		}
	}
	return string(spaced)
}

// spacing spaces a paragraph's words (section 3.4), as a sentence's start if start is set or, with restart, if
// it has a closed number, and as a long paragraph if long is set (section 3.3), where a run of dashes after something
// else starts with a space instead. It reports whether the paragraph ends where a sentence starts. A gap that keeps
// more spaces near the limit is marked widenedOnce or widenedTwice after its first space.
func (address *address) spacing(paragraph string, start, restart, long bool) (string, bool) {
	if long {
		paragraph = dashRunsSpaced(paragraph)
	}
	paragraph = startDots(namedMarks(address.dotRuns(address.loosened(commas(paragraph, true)), long)))
	paragraph = cappedTerminals(commasAndUnderscores(slashes(dashes(pluses(paragraph)))))
	paragraph = keptGaps(closingRuns(starred(paragraph)))
	trailing := len(paragraph) - len(strings.TrimRight(paragraph, " "))
	parts := words(paragraph)
	initial := start
	for index := 0; !initial && restart && index < len(parts); index++ {
		initial = restarts(parts[index].token)
	}
	var output []string
	if len(parts) > 0 {
		output = append(output, parts[0].spaces)
	}
	skipped, gap, joined := 0, 0, 0
	terminalBefore := false
	held := ""
	// excess is how many spaces more the gap after a word keeps near the limit, one more before a capitalized word if
	// capitalCounts; exclaiming is whether the word ended in ! or ?.
	excess, capitalCounts, exclaiming := 0, false, false
	for index, part := range parts {
		token := part.token
		if index > 0 {
			if initial && skipped > 0 && capitalized(token) {
				gap = 2
				if exclaiming {
					excess = max(excess, 1)
				}
			}
			width := max(gap, 1)
			joining := terminal(token) && !terminalBefore && joined == 0
			if terminal(token) && !terminalBefore {
				width = joined
				if last := len(output) - 1; joining && strings.HasSuffix(output[last], "\"") {
					output[last] = joinedWord(cmp.Or(parts[index-1].whole, output[last]))
				}
			}
			switch part.spaces {
			case cutSpace:
				width = 1
			case keptGap:
				width, initial, skipped = 2, false, 0
			}
			if held != "" && !terminal(token) {
				output = append(output, held)
				held = ""
			}
			if capitalCounts && capitalized(token) {
				excess++
			}
			switch {
			case excess > 0 && width >= 2:
				output = append(output, " "+widenings[excess]+strings.Repeat(" ", width-1))
			case joining && width == 0:
				output = append(output, joinedSpace)
			default:
				output = append(output, strings.Repeat(" ", width))
			}
		}
		excess, capitalCounts, exclaiming = 0, false, false
		terminalBefore = index > 0 && terminal(token)
		spread := strings.HasSuffix(token, keptSpace+"/")
		if strings.HasSuffix(token, "/") && token != "/" && token != ">/" && !strings.HasSuffix(token, "(/") {
			held = " /"
			if spread {
				held = "  /"
			}
			token = strings.TrimRight(token[:len(token)-1], keptSpace)
		}
		joined = 0
		if part.spaces == cutSpace {
			output = append(output, token)
			continue
		}
		gap = 0
		later, cutMarks := index+1, ""
		for later < len(parts) && parts[later].spaces == cutSpace {
			cutMarks += parts[later].token
			later++
		}
		written, whole := token+cutMarks, cmp.Or(part.whole, token)+cutMarks
		following, followingSpaces := "", trailing
		if later < len(parts) {
			following, followingSpaces = parts[later].token, spaceWidth(parts[later].spaces)
		}
		nextSpaces, nextKept := trailing, false
		if index+1 < len(parts) {
			nextSpaces, nextKept = spaceWidth(parts[index+1].spaces), parts[index+1].spaces == keptGap
		}
		if dotted, closed, ok := wholeEnding(written, whole); ok {
			kind := address.dotKind(dotted, initial, closed)
			if closingLength(whole)+later-index-1 > closingRoom {
				switch kind {
				case functionKind, startKind:
					kind = wordKind
				case initialKind:
					kind = letterKind
				}
			}
			gap = address.dotGap(dotted, kind, followingSpaces, following, written)
			if (kind == wordKind || kind == startKind || kind == functionKind) && !closed {
				typed := 1
				if kind == wordKind && !nextKept {
					typed = min(nextSpaces, 2)
				}
				excess, capitalCounts = typed-2, kind != functionKind
				if exclaimed(dotted, closingRoom) {
					excess++
				}
			}
			if kind == wordKind && nextSpaces >= 2 {
				joined = 1
			}
			if (nextSpaces >= 2 || spread) && (kind == wordKind || kind == startKind || kind == functionKind) {
				if cut := cutClosers(token); cut != token {
					token, joined = cut, 1
				}
				token = quotedDot(token, following)
			}
			if initial && kind == numberKind {
				skipped++
			} else {
				initial, skipped = address.startsAfter(dotted, kind), 0
			}
		} else if exclaimed(written, closingRoom) {
			gap, exclaiming = 2, true
			if nextKept {
				excess = 1
			}
			bare := strings.TrimRight(strings.TrimRight(unclosed(written), "!?"), keptSpace)
			if dotted, closed, ok := sentenceEnding(bare); ok && !markedDot(bare) {
				initial, skipped = address.startsAfter(dotted, address.dotKind(dotted, initial, closed)), 0
			} else if initial && address.skippable(bare) {
				skipped++
			} else {
				initial, skipped = false, 0
			}
		} else if dotted, closed, rest, ok := splitEnding(written); ok {
			if len(dotted) >= 2 && strings.IndexByte(")]:;!?", dotted[len(dotted)-2]) >= 0 && rest != "" &&
				strings.IndexByte(")]:;,", rest[0]) >= 0 {
				if exclaimed(dotted, closingRoom) {
					gap = 2
				}
				initial, skipped = false, 0
			} else {
				kind := address.dotKind(dotted, initial, closed)
				gap = address.dotGap(dotted, kind, 1, following, dotted)
				initial, skipped = address.startsAfter(dotted, kind), 0
				if initial && rest != "" && strings.Trim(rest, ",;)") == "" {
					skipped = 1
				}
			}
		} else if initial && address.skippable(written) {
			skipped++
		} else {
			initial, skipped = false, 0
		}
		output = append(output, settledSquare(token, part.whole != "", nextSpaces))
	}
	output = append(output, held, strings.Repeat(" ", trailing))
	paragraph = strings.ReplaceAll(strings.Join(output, ""), keptSpace, " ")
	return strings.ReplaceAll(paragraph, droppedDot, ""), initial
}

// settledSquare settles a [ that starts a word, after its opening brackets: it goes when a closing run cut the word
// (cut), and before a dot that ends a sentence (two spaces or more follow it) the word without that dot decides.
func settledSquare(token string, cut bool, spaces int) string {
	opening := len(token) - len(strings.TrimLeft(token, "("))
	if opening == len(token) || token[opening] != '[' {
		return token
	}
	switch {
	case cut:
		return token[:opening] + droppedSquare + token[opening+1:]
	case strings.HasSuffix(token, ".") && len(token) >= opening+2 && spaces >= 2:
		if squareGoes(token[opening+1 : len(token)-1]) {
			return token[:opening] + droppedSquare + token[opening+1:]
		}
		return token[:opening] + droppedSquare + token[opening:]
	}
	return token
}

// joinedWord is a word ending in a quote that a word of ! and ? joins: as written, its closing marks none hidden, and
// then its closing marks and quote again (the last four) when they are more than four, or when something that is no
// number stands before them. Opening brackets don't count, but a [ does when the word so written loses it.
func joinedWord(token string) string {
	start := len(strings.TrimRight(token[:len(token)-1], "!?)]:;.,"))
	run := token[start:]
	repeated := token + run[max(len(run)-closingRoom, 0):]
	if len(run) > closingRoom {
		return repeated
	}
	opening := len(token[:start]) - len(strings.TrimLeft(token[:start], "("))
	square := opening < start && token[opening] == '['
	before := token[opening:start]
	if square {
		before = before[1:]
	}
	switch {
	case before != "" && numeric(before):
		return token
	case before != "":
		return repeated
	case square && squareGoes(repeated[opening+1:]):
		return repeated
	}
	return token
}

// cappedTerminals keeps the first four marks of each word part of five ! and ? or more after a space, a + or a -, at
// the end of its word.
func cappedTerminals(paragraph string) string {
	var output strings.Builder
	for index := 0; index < len(paragraph); index++ {
		output.WriteByte(paragraph[index])
		if strings.IndexByte(" +-", paragraph[index]) < 0 {
			continue
		}
		if run := terminalRun(paragraph, index+1); run-index-1 > closingRoom && wordEnds(paragraph, run) {
			output.WriteString(paragraph[index+1 : index+1+closingRoom])
			index = run - 1
		}
	}
	return output.String()
}

// dotRuns spaces each run of dots and the spaces after it, each as the runs before it left the paragraph, as a long
// paragraph does if long is set.
func (address *address) dotRuns(paragraph string, long bool) string {
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
		if long {
			output.WriteString(address.longDotRun(output.String(), paragraph[stop:], dots-index, stop-dots))
		} else {
			output.WriteString(address.dotRun(output.String(), paragraph[stop:], dots-index, stop-dots))
		}
		index = stop
	}
	return output.String()
}

// longDotRun is a run of dots and the spaces after it in a long paragraph (section 3.3): after a word a lone dot for
// each dot and space after the third, two at most, after anything else one after the fourth. Without lone dots, or
// spaces, it is a run as any paragraph spaces it.
func (address *address) longDotRun(built, following string, count, spaces int) string {
	lone := min(max(count+spaces-4, 0), 1)
	if address.plainKind(alonePart(built)+".", false) == wordKind {
		lone = min(max(count+spaces-3, 0), 2)
	}
	if spaces == 0 || lone == 0 {
		return address.dotRun(built, following, count, spaces)
	}
	return "." + strings.Repeat(" .", lone) + " "
}

// dotRun is a run of dots and the spaces after it, built being the paragraph before it as the runs before it left it
// and following what follows it: a dot and two spaces, and a dot and a space more for every six dots and spaces after
// the first two, a run before a word counted as if one space followed it and keeping its last one or two dots on the
// word. A dot before one space or none stays, but one right after a single character that is no digit (a comma that
// starts a word counts) is a dot and a space before marks with a closing bracket or a quote after the first, or
// before commas and a letter or a digit, and one after a single ! or ? a dot and two spaces before a letter. Two dots
// before a word are a dot pair.
func (address *address) dotRun(built, following string, count, spaces int) string {
	attached := spaces == 0 && following != ""
	alone := alonePart(built)
	split := len(alone) == 1 && !digit(alone[0]) && splitsSingle(following)
	switch {
	case count == 1 && attached && split:
		return ". "
	case count == 1 && attached && (alone == "!" || alone == "?") && alphabetic(following[0]):
		return ".  "
	case count == 1 && spaces <= 1:
		return "." + strings.Repeat(" ", spaces)
	case count == 2 && attached:
		return address.dotPair(built, following, alone, split)
	}
	extra := (count + max(spaces, 1) - 2) / dotRunSpaces
	if attached && count-dotRunSpaces*extra <= 2 {
		return ".  " + strings.Repeat(". ", extra-1) + "."
	}
	return ".  " + strings.Repeat(". ", extra)
}

// dotPair is two dots before a word, built being the paragraph before them, alone its single character before them
// and split whether that splits there: a dot, but a dot and two spaces before a +; a dot after a word of two
// characters or more whose closing marks a closing run would cut; a dot and a space that keeps the word whole after a
// word (a function word whose closing marks are more than a closing run keeps, a quoted word) and before what splits
// it, or a slash both parts keep; a dot, a space and a dot after a single character that is no digit, also before
// marks that aren't those ending a word of signs, a slash, a bracket or a dash; a dropped dot and a dot after a letter
// before those marks, or after a letter or a digit and closing marks with a ! or ?.
func (address *address) dotPair(built, following, alone string, split bool) string {
	before := built[strings.LastIndexAny(built, " ,")+1:]
	if strings.HasPrefix(following, "+") {
		return ".  "
	}
	marked := len(before) - len(strings.TrimRight(before, "!?)]:;."))
	run := 0
	if closing, ok := pairClosing(following); ok {
		run = marked + 2 + closing
	}
	if len(before)-marked >= 2 && run-closingRoom > marked {
		return "."
	}
	kind := address.plainKind(before+".", false)
	wordy := kind == wordKind || kind == functionKind && run > closingRoom ||
		strings.HasSuffix(before, "\"") && len(core(before+".")) > 1
	if wordy && (splits(following) || strings.HasPrefix(following, "/") && kept(before+".")) {
		return "." + keptSpace
	}
	if split {
		return ". ."
	}
	firstWord, _, _ := strings.Cut(following, " ")
	signs := signRun(following) && !hasAlphanumeric(firstWord)
	if len(alone) == 1 && !digit(alone[0]) && !signs && strings.IndexByte("/(-", following[0]) < 0 {
		return ". ."
	}
	if len(before) == 1 && alphabetic(before[0]) && signs || exclaimedCharacter(before) {
		return droppedDot + "."
	}
	return "."
}

// alonePart is a paragraph's part after its last space, or after its last comma after a character that is no comma
// or space.
func alonePart(built string) string {
	for index := len(built) - 1; index >= 0; index-- {
		if built[index] == ' ' || built[index] == ',' && index > 0 && built[index-1] != ',' && built[index-1] != ' ' {
			return built[index+1:]
		}
	}
	return built
}

// splitsSingle is whether a dot after a single character is a dot and a space before a text: one that starts with
// marks with a closing bracket or a quote after the first, or with commas and a letter or a digit.
func splitsSingle(text string) bool {
	run := len(text) - len(strings.TrimLeft(text, "!?)]:;.,\"'"))
	if run >= 2 && strings.ContainsAny(text[1:run], ")]\"'") {
		return true
	}
	commas := len(text) - len(strings.TrimLeft(text, ","))
	return commas > 0 && commas < len(text) && alphanumeric(text[commas])
}

// pairClosing is how long the closing marks after two dots are, and whether they end there: a run of ! ? ) ] : ; and
// dots and a quote before a space, a comma or the end, and a comma after them that no mark follows.
func pairClosing(text string) (int, bool) {
	run := len(text) - len(strings.TrimLeft(text, "!?)]:;."))
	if run < len(text) && text[run] == '"' {
		run++
	}
	switch {
	case run < len(text) && text[run] == ',' && (run+1 == len(text) || strings.IndexByte("!?)]:;.,\"", text[run+1]) < 0):
		return run + 1, true
	case wordEnds(text, run) || text[run] == ',':
		return run, true
	}
	return 0, false
}

// signRun is whether a text starts with the marks that end a word of signs: commas, semicolons, colons, ! and ? before
// a space, a dot, a comma or the end.
func signRun(text string) bool {
	run := len(text) - len(strings.TrimLeft(text, ",;:!?"))
	return run > 0 && (run == len(text) || text[run] == ' ' || text[run] == '.' || strings.Contains(text[1:run], ","))
}

// exclaimedCharacter is whether a word is a letter, a digit or an underscore and closing marks with a ! or ?.
func exclaimedCharacter(word string) bool {
	return len(word) >= 2 && (alphanumeric(word[0]) || word[0] == '_') && strings.Trim(word[1:], ")]:;!?") == "" &&
		strings.ContainsAny(word[1:], "!?")
}

// splits is whether a text splits a word that two dots end: one that starts with a comma or a dash; ! and ? before a
// space, a comma, the end, or a dot and one space; a closing mark, ! or ? before a comma, a slash or a dash; or a
// quote, or a closing mark, ! or ? and maybe another one, a quote, or a dot and one space, before a space or the end.
func splits(text string) bool {
	if text == "" {
		return false
	}
	switch {
	case text[0] == ',' || text[0] == '-':
		return true
	case text[0] == '"':
		return wordEnds(text, 1)
	}
	if run := terminalRun(text, 0); run > 0 &&
		(wordEnds(text, run) || text[run] == ',' || text[run] == '.' && singleSpace(text, run+1)) {
		return true
	}
	if strings.IndexByte(")]:;!?", text[0]) < 0 {
		return false
	}
	switch {
	case wordEnds(text, 1), strings.IndexByte(",/-", text[1]) >= 0:
		return true
	case strings.IndexByte(")]:;!?\"", text[1]) >= 0 && wordEnds(text, 2):
		return true
	}
	return text[1] == '.' && singleSpace(text, 2)
}

// wordEnds is whether a word ends at index in a text: a space stands there, or the text ends.
func wordEnds(text string, index int) bool {
	return index == len(text) || text[index] == ' '
}

// singleSpace is whether one space stands at index in a text, and no other after it.
func singleSpace(text string, index int) bool {
	return index < len(text) && text[index] == ' ' && (index+1 == len(text) || text[index+1] != ' ')
}

func terminalRun(text string, index int) int {
	for index < len(text) && (text[index] == '!' || text[index] == '?') {
		index++
	}
	return index
}

// namedMarks reads a semicolon or a colon that is a word of its own after a dot and spaces.
func namedMarks(paragraph string) string {
	var output strings.Builder
	for index := 0; index < len(paragraph); index++ {
		character := paragraph[index]
		if character == ';' || character == ':' {
			before := strings.TrimRight(paragraph[:index], " ")
			if len(before) < index && strings.HasSuffix(before, ".") && wordEnds(paragraph, index+1) {
				if character == ';' {
					output.WriteString("semicolon")
				} else {
					output.WriteString("colon")
				}
				continue
			}
		}
		output.WriteByte(character)
	}
	return output.String()
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

// wordCommas puts a space after a comma of a word that doesn't start with one: after a character that is no digit,
// dot, comma or sign (+, - or /), or a dot after a character that is no dot, if more of the word follows; after a
// digit if the commas after it are followed by a character that is no digit, dot, comma or opening bracket, nor a
// dash or a slash in a word that starts with a digit or a dot; after a dot that starts such a word if the commas and
// dots after it are. Without signs, the first comma spaced ends the word, and what follows it is a word of its own.
// With signs, all the commas before a sign are spaced, and in a word that starts with no digit or dot a comma right
// before a + after no sign is doubled first.
func wordCommas(word string, signs bool) string {
	if word == "" || word[0] == ',' {
		return word
	}
	number := digit(word[0]) || word[0] == '.'
	if !signs {
		for index := 1; index < len(word); index++ {
			if word[index] == ',' && commaSpaced(word, index, number) {
				return word[:index+1] + " " + wordCommas(word[index+1:], false)
			}
		}
		return word
	}
	if !number {
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
		if word[index] == ',' && commaSpaced(word, index, number) && signed(word[index+1:]) {
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
	case digit(previous):
		return commaMark(strings.TrimLeft(word[index+1:], ","), number)
	case number && previous == '.' && index == 1:
		return commaMark(strings.TrimLeft(word[index+1:], ",."), true)
	}
	return false
}

// commaMark is whether a text after a comma and the commas after it starts with a mark the comma is spaced before: a
// character that is no digit, dot, comma or opening bracket, nor a dash or a slash in a number.
func commaMark(rest string, number bool) bool {
	return rest != "" && !digit(rest[0]) && strings.IndexByte(".,(", rest[0]) < 0 &&
		!(number && (rest[0] == '-' || rest[0] == '/'))
}

func sign(character byte) bool {
	return character == '+' || character == '-' || character == '/'
}

// signed is whether a text starts with a sign after its commas.
func signed(text string) bool {
	text = strings.TrimLeft(text, ",")
	return text != "" && sign(text[0])
}

// closingMarks are the marks of a closing run, a kept space among them.
const closingMarks = "!?)]:;.," + keptSpace

// closingRuns keeps the last four characters of each closing run: a run of closing marks (a quote may end it) before
// a space or the end, after a character that is no mark after a character that is no space, in a word with two
// characters or more before it (a > that starts the word, the opening brackets after that and a [ after them aside).
// What goes is kept hidden, for the sentence end test, and a kept space in what stays is a cut space.
func closingRuns(paragraph string) string {
	var output strings.Builder
	for index := 0; index < len(paragraph); {
		stop := closingRun(paragraph, index)
		if stop-index <= closingRoom || !cuttable(paragraph[:index]) {
			stop = max(stop, index+1)
			output.WriteString(paragraph[index:stop])
			index = stop
			continue
		}
		shown := stop - closingRoom
		output.WriteString(hidden + strings.ReplaceAll(paragraph[index:shown], keptSpace, "") + hidden)
		output.WriteString(strings.ReplaceAll(paragraph[shown:stop], keptSpace, cutSpace))
		index = stop
	}
	return output.String()
}

// cuttable is whether a closing run may cut after the paragraph before it: its word there has two characters or
// more, a > that starts it, the opening brackets after that and a [ after them aside.
func cuttable(before string) bool {
	before = before[strings.LastIndexAny(before, " "+keptSpace)+1:]
	before = strings.TrimPrefix(strings.TrimLeft(strings.TrimPrefix(before, ">"), "("), "[")
	return len(before) >= 2
}

// closingRun is where a closing run that starts at index ends, or index if none does.
func closingRun(paragraph string, index int) int {
	if index < 2 || strings.IndexByte(" "+closingMarks+droppedDot, paragraph[index-1]) >= 0 ||
		paragraph[index-2] == ' ' || paragraph[index-2] == keptSpace[0] {
		return index
	}
	stop := index
	for stop < len(paragraph) && strings.IndexByte(closingMarks, paragraph[stop]) >= 0 {
		stop++
	}
	if stop == index {
		return index
	}
	if stop < len(paragraph) && paragraph[stop] == '"' {
		stop++
	}
	if !wordEnds(paragraph, stop) {
		return index
	}
	return stop
}

// closingLength is how many closing marks and quotes end a word, its kept spaces left out.
func closingLength(word string) int {
	word = strings.ReplaceAll(word, keptSpace, "")
	return len(word) - len(strings.TrimRight(word, "!?)]:;.,\""))
}

// cutClosers keeps the last two of three or more closing marks before a word's last dot.
func cutClosers(token string) string {
	if !strings.HasSuffix(token, ".") {
		return token
	}
	run := len(token)
	for run > 0 && strings.IndexByte(")]:;!?.", token[run-1]) >= 0 {
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

// part is a word of a paragraph being spaced, or a piece of one after a cut space: the spaces before it (or the cut
// space), its characters as written and, if a closing run hid marks of it, its characters with them.
type part struct {
	spaces, token, whole string
}

// words splits a paragraph into its words, at spaces, cut spaces and kept gaps.
func words(paragraph string) []part {
	var parts []part
	for index := 0; index < len(paragraph); {
		run := index
		for run < len(paragraph) && gapCharacter(paragraph[run]) {
			run++
		}
		if run == len(paragraph) {
			break
		}
		stop := run
		for stop < len(paragraph) && !gapCharacter(paragraph[stop]) {
			stop++
		}
		spaces := paragraph[index:run]
		if marker := strings.LastIndexAny(spaces, cutSpace+keptGap); marker >= 0 && marker == len(spaces)-1 {
			spaces = spaces[marker:]
		} else {
			spaces = spaces[marker+1:]
		}
		next := part{spaces: spaces, token: paragraph[run:stop]}
		if strings.Contains(next.token, hidden) {
			next.token, next.whole = visible(next.token), strings.ReplaceAll(next.token, hidden, "")
		}
		parts = append(parts, next)
		index = stop
	}
	return parts
}

func gapCharacter(character byte) bool {
	return character == ' ' || character == cutSpace[0] || character == keptGap[0]
}

// keptGaps drops the spaces around each kept gap.
func keptGaps(paragraph string) string {
	gaps := strings.Split(paragraph, keptGap)
	for index := range gaps {
		if index > 0 {
			gaps[index] = strings.TrimLeft(gaps[index], " ")
		}
		if index < len(gaps)-1 {
			gaps[index] = strings.TrimRight(gaps[index], " ")
		}
	}
	return strings.Join(gaps, keptGap)
}

// spaceWidth is how many spaces stand between two words: two for a kept gap.
func spaceWidth(spaces string) int {
	if spaces == keptGap {
		return 2
	}
	return len(spaces)
}

// visible is a word without the marks a closing run hid.
func visible(token string) string {
	var shown strings.Builder
	for {
		start := strings.Index(token, hidden)
		if start < 0 {
			break
		}
		stop := strings.Index(token[start+1:], hidden)
		if stop < 0 {
			break
		}
		shown.WriteString(token[:start])
		token = token[start+stop+2:]
	}
	shown.WriteString(token)
	return shown.String()
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

// wholeEnding is whether a word ends like a sentence both as written and with the marks a closing run hid (whole). It
// gives the ending as written if there is one.
func wholeEnding(written, whole string) (dotted string, closed, ok bool) {
	dotted, closed, ok = sentenceEnding(written)
	if whole == written {
		return dotted, closed, ok
	}
	wholeDotted, wholeClosed, wholeOk := sentenceEnding(whole)
	switch {
	case !wholeOk:
		return "", false, false
	case !ok:
		return wholeDotted, wholeClosed, true
	}
	return dotted, closed, ok
}

// sentenceEnding is whether a word ends like a sentence: in a dot; in a dot after a character that is no closing mark,
// ! or ?, and closing marks (brackets, colons, semicolons, commas and dots, a quote may end them); as a dot and
// closing marks; or in a dot and one closing mark, but a bracket, colon, semicolon or comma after one of those or a !
// or ? before the dot. A dropped dot doesn't count. It gives the word up to its dot and whether a bracket or a quote
// closes it.
func sentenceEnding(token string) (dotted string, closed, ok bool) {
	if strings.HasSuffix(token, ".") {
		return token, false, true
	}
	if strings.Contains(token, droppedDot) {
		dotted, closed, ok = sentenceEnding(strings.ReplaceAll(token, droppedDot, ""))
		if !ok {
			return "", false, false
		}
		size := 0
		for index := 0; index < len(token); index++ {
			if token[index] != droppedDot[0] {
				size++
			}
			if size == len(dotted) {
				return token[:index+1], closed, true
			}
		}
	}
	if len(token) >= 2 && token[0] == '.' && strings.Trim(token[1:len(token)-1], ")]:;") == "" &&
		strings.IndexByte(")]:;\"", token[len(token)-1]) >= 0 {
		return ".", token[1] == ')' || token[1] == ']', true
	}
	marked := strings.TrimSuffix(token, "\"")
	if dot := len(strings.TrimRight(marked, ")]:;,.")); dot >= 1 && dot+2 <= len(marked) && marked[dot] == '.' &&
		marked[dot-1] != '!' && marked[dot-1] != '?' {
		return marked[:dot+1], marked[dot+1] == ')' || marked[dot+1] == ']', true
	}
	if len(token) < 2 || token[len(token)-2] != '.' {
		return "", false, false
	}
	last := token[len(token)-1]
	if strings.IndexByte(");,:]", last) >= 0 && len(token) >= 3 && strings.IndexByte(")]:;,!?", token[len(token)-3]) >= 0 {
		return "", false, false
	}
	switch last {
	case ')', ']', '"':
		return token[:len(token)-1], true, true
	case ';', ',', ':':
		return token[:len(token)-1], false, true
	}
	return "", false, false
}

// markedDot is whether a text ends in a dot after a closing mark, ! or ?.
func markedDot(text string) bool {
	return len(text) >= 2 && text[len(text)-1] == '.' && strings.IndexByte(")]!?:;,", text[len(text)-2]) >= 0
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

// plainKind is what a word ending in a dot is by its characters before the dot. A quoted single character with
// closing marks before its quote and a word that a dropped dot ends are empty.
func (address *address) plainKind(token string, initial bool) kind {
	if strings.HasSuffix(token, "\".") {
		quoted := strings.TrimLeft(token[:len(token)-2], "([")
		letter := strings.TrimLeft(strings.TrimRight(token[:len(token)-2], ".!?)]:;"+droppedDot), "([")
		switch {
		case len(letter) > 1:
			return wordKind
		case letter != quoted:
			return emptyKind
		case letter != "" && alphabetic(letter[0]):
			return letterKindOf(letter[0])
		case letter != "" && digit(letter[0]):
			return numberKind
		}
		return emptyKind
	}
	word := core(token)
	switch {
	case strings.HasSuffix(word, droppedDot):
		return emptyKind
	case strings.HasSuffix(word, "\"") && len(word) > 1:
		return wordKind
	case word == "" || word == "\"":
		return emptyKind
	case !hasAlphanumeric(word):
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

// dotGap is how many spaces follow a word ending in a dot, which spaces follow in the paragraph, whole being the word
// with the marks after its dot: two after ! or ? and fewer closing marks than a closing run keeps (one fewer before
// two spaces, but after a word). A shouted word is spaced as an empty one.
func (address *address) dotGap(token string, kind kind, spaces int, following, whole string) int {
	if shouted(token) {
		kind = emptyKind
	}
	room := closingRoom
	if spaces >= 2 && kind != wordKind && kind != startKind && kind != functionKind {
		room--
	}
	if exclaimed(whole, room) {
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

// shouted is whether a word ending in a dot is a single character or a number and closing marks with a ! or ? before
// the dot, no quote before it.
func shouted(token string) bool {
	if strings.HasSuffix(token, "\".") {
		return false
	}
	word := core(token)
	closing := token[len(strings.TrimRight(token[:len(token)-1], ")]!?:;,.")) : len(token)-1]
	return strings.ContainsAny(closing, "!?") && (len(word) == 1 || numeral(word, false))
}

// quotedDot drops a quote before a word's last dot and the closing marks before the quote, but after a comma or
// before a word of ! and ?.
func quotedDot(token, following string) string {
	if !strings.HasSuffix(token, "\".") {
		return token
	}
	word := strings.TrimRight(token[:len(token)-2], ".!?)]:;")
	if strings.HasSuffix(word, ",") || following != "" && terminal(following) {
		return token
	}
	return word + "."
}

// unclosed is a word without its closing marks: one quote at its end, then brackets, commas, colons, semicolons, dots
// and dropped dots.
func unclosed(token string) string {
	return strings.TrimRight(strings.TrimSuffix(token, "\""), ",):;]."+droppedDot)
}

// exclaimed is whether a word ends in ! or ? and closing marks, fewer than room of them.
func exclaimed(token string, room int) bool {
	bare := unclosed(token)
	return bare != "" && (bare[len(bare)-1] == '!' || bare[len(bare)-1] == '?') && len(token)-len(bare) < room
}

func hasAlphanumeric(text string) bool {
	for index := 0; index < len(text); index++ {
		if alphanumeric(text[index]) {
			return true
		}
	}
	return false
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
