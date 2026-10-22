package preformatter

import "strings"

// separatorUnits are the marks that go everywhere in a paragraph where one of them stands twice in a row, tried in
// this order.
var separatorUnits = []string{"%%", "&&", "**", "++", "--", "//", "::", ";;", "<<", "==", "@@", "^^", "__", "||", "##",
	">>", "+-", "+_", "-=", "-_", "<>"}

// arrowMarks are the arrows, tried in this order.
var arrowMarks = []string{"<-->", "<==>", "<->", "<=>", "-->", "==>", "->", "=>"}

// faces are the faces and how they are read, tried in this order.
var faces = []struct{ face, reading string }{
	{":-)", "ha ha"}, {"8-)", "ha ha"}, {":)", "ha ha"}, {";-)", "wink"}, {";)", "wink"}, {":-o", "uh oh"},
	{":o", "uh oh"}, {":-(", "groan"}, {":-/", "oh yeah?"}, {"|-O", "yawn"},
}

// markNames are the names of ! and ?, read in this order.
var markNames = []struct{ mark, name string }{{"!", "exclamation"}, {"?", "question"}}

// marks reads a paragraph's marks before its words are spaced (section 3.4), rawSize being what its lines measure:
// faces then arrows, each kind from the first, while rawSize and what they add stay below markLimit; a separator unit
// that stands twice somewhere goes everywhere, then every backslash if two stand together. The brackets count the
// paragraph with the spaces that went with separators and the two spaces around each quoted mark; after them a | that
// ends a word is a comma.
func marks(paragraph string, rawSize int) string {
	room := markLimit - rawSize
	for _, face := range faces {
		paragraph, room = replaced(paragraph, face.face, face.reading, room)
	}
	for _, arrow := range arrowMarks {
		reading := "to"
		if arrow[0] == '<' {
			reading = "to and from"
		}
		paragraph, room = replaced(paragraph, arrow, reading, room)
	}
	unwritten := 0
	for _, unit := range separatorUnits {
		if strings.Contains(paragraph, unit+unit) {
			var spaces int
			paragraph, spaces = separated(paragraph, unit)
			unwritten += spaces
		}
	}
	if strings.Contains(paragraph, `\\`) {
		paragraph = strings.ReplaceAll(paragraph, `\`, "")
	}
	paragraph = terminalMarks(paragraph)
	words := strings.Split(paragraph, " ")
	for index, word := range words {
		if index > 0 && len(word) > 4 && terminal(word) {
			words[index] = word[:4]
		}
	}
	paragraph, spaces := terminalNames(strings.Join(words, " "))
	unwritten += spaces
	words = strings.Split(paragraph, " ")
	for index, word := range words {
		switch {
		case index == 0:
		case word == ";":
			words[index] = "semicolon"
		case word == ":":
			words[index] = "colon"
		}
	}
	paragraph = approximately(strings.Join(words, " "))
	return bars(brackets(paragraph, len(paragraph)+unwritten, false))
}

// doubledSeparator is whether a separator unit stands twice in a paragraph.
func doubledSeparator(paragraph string) bool {
	for _, unit := range separatorUnits {
		if strings.Contains(paragraph, unit+unit) {
			return true
		}
	}
	return false
}

// replaced reads each mark from the left as its reading while it fits the room: it takes the reading's length and two
// spaces less the mark's. The reading gets a space on each side where there is none. It gives the room left.
func replaced(paragraph, mark, reading string, room int) (string, int) {
	parts := strings.Split(paragraph, mark)
	growth := len(reading) + 2 - len(mark)
	var output strings.Builder
	output.WriteString(parts[0])
	for _, part := range parts[1:] {
		if growth >= room {
			output.WriteString(mark + part)
			continue
		}
		room -= growth
		if !strings.HasSuffix(output.String(), " ") {
			output.WriteByte(' ')
		}
		output.WriteString(reading)
		if !strings.HasPrefix(part, " ") {
			output.WriteByte(' ')
		}
		output.WriteString(part)
	}
	return output.String(), room
}

// separated takes a separator unit out of a paragraph, and with a run of it between spaces the space before it. It
// gives how many spaces went.
func separated(paragraph, unit string) (string, int) {
	var output strings.Builder
	spaces := 0
	for index := 0; index < len(paragraph); {
		if paragraph[index] == ' ' {
			stop := index + 1
			for strings.HasPrefix(paragraph[stop:], unit) {
				stop += len(unit)
			}
			if stop > index+1 && stop < len(paragraph) && paragraph[stop] == ' ' {
				spaces++
				index = stop
				continue
			}
		}
		if strings.HasPrefix(paragraph[index:], unit) {
			index += len(unit)
			continue
		}
		output.WriteByte(paragraph[index])
		index++
	}
	return output.String(), spaces
}

// terminalNames reads a ! or ? in quotes or brackets as its name, a word of its own, and a word of !s or !'s (?s or
// ?'s) after a space as the plural of its name, with a comma on each side. The ! is read before the ?. It gives how
// many spaces the quoted marks had around them.
func terminalNames(paragraph string) (string, int) {
	spaces := 0
	for _, named := range markNames {
		var quoted int
		paragraph, quoted = quotedMark(paragraph, named.mark, named.name)
		spaces += quoted
		for _, plural := range []string{"s", "'s"} {
			paragraph = pluralMark(paragraph, " "+named.mark+plural, ", "+named.name+"s"+named.mark+",")
		}
	}
	return paragraph, spaces
}

// quotedMark reads each mark in quotes or brackets as its name, with a space on each side where there is none (the
// paragraph's start and end count as none). It gives how many spaces there were.
func quotedMark(paragraph, mark, name string) (string, int) {
	var output strings.Builder
	spaces := 0
	for index := 0; index < len(paragraph); {
		rest := paragraph[index:]
		if !strings.HasPrefix(rest, `"`+mark+`"`) && !strings.HasPrefix(rest, "("+mark+")") {
			output.WriteByte(paragraph[index])
			index++
			continue
		}
		stop := index + 3
		if index > 0 && paragraph[index-1] == ' ' {
			spaces++
		} else {
			output.WriteByte(' ')
		}
		output.WriteString(paragraph[index:index+1] + name + paragraph[index+1:stop])
		if stop < len(paragraph) && paragraph[stop] == ' ' {
			spaces++
		} else {
			output.WriteByte(' ')
		}
		index = stop
	}
	return output.String(), spaces
}

// pluralMark reads each word that is a plural mark after a space, the space included, as its reading.
func pluralMark(paragraph, plural, reading string) string {
	var output strings.Builder
	for index := 0; index < len(paragraph); {
		if stop := index + len(plural); strings.HasPrefix(paragraph[index:], plural) && stop < len(paragraph) &&
			paragraph[stop] == ' ' {
			output.WriteString(reading)
			index = stop
			continue
		}
		output.WriteByte(paragraph[index])
		index++
	}
	return output.String()
}

// bars makes a comma of each | that ends a word.
func bars(paragraph string) string {
	barred := []byte(paragraph)
	for index, character := range barred {
		if character == '|' && (index+1 == len(barred) || barred[index+1] == ' ') {
			barred[index] = ','
		}
	}
	return string(barred)
}

// terminalMarks makes each run of the same ! or ? one.
func terminalMarks(paragraph string) string {
	var output strings.Builder
	for index := 0; index < len(paragraph); index++ {
		output.WriteByte(paragraph[index])
		if paragraph[index] == '!' || paragraph[index] == '?' {
			for index+1 < len(paragraph) && paragraph[index+1] == paragraph[index] {
				index++
			}
		}
	}
	return output.String()
}

// approximately reads each ~ after a space as "approximately".
func approximately(paragraph string) string {
	var output strings.Builder
	spaced := false
	for index := 0; index < len(paragraph); index++ {
		if paragraph[index] == '~' && spaced {
			output.WriteString("approximately ")
			continue
		}
		output.WriteByte(paragraph[index])
		spaced = paragraph[index] == ' '
	}
	return output.String()
}

// unmarked reads a paragraph's marks after its words are spaced (section 3.4): words of a > go, a word's leading >
// before a character that is no digit, and a word's leading [ (unbracketed).
func unmarked(paragraph string) string {
	var output strings.Builder
	for index := 0; index < len(paragraph); {
		if stop, replacement, ok := angleRun(paragraph, index); ok {
			output.WriteString(replacement)
			index = stop
			continue
		}
		output.WriteByte(paragraph[index])
		index++
	}
	paragraph = output.String()
	output.Reset()
	for index := 0; index < len(paragraph); index++ {
		if paragraph[index] == '>' && (index == 0 || paragraph[index-1] == ' ') && index+1 < len(paragraph) &&
			paragraph[index+1] != ' ' && !digit(paragraph[index+1]) {
			continue
		}
		output.WriteByte(paragraph[index])
	}
	words := strings.Split(output.String(), " ")
	for index, word := range words {
		words[index] = unbracketed(word)
	}
	return strings.Join(words, " ")
}

// unbracketed drops a [ that starts a word, after its opening brackets, when what follows it makes it go, the ! and ?
// joined to the word aside. A dropped square before it has settled that already.
func unbracketed(word string) string {
	opening := len(word) - len(strings.TrimLeft(word, "("))
	if opening < len(word) && word[opening] == '[' {
		if rest, _, _ := strings.Cut(word[opening+1:], joinedSpace); squareGoes(rest) {
			word = word[:opening] + word[opening+1:]
		}
	}
	return strings.ReplaceAll(strings.ReplaceAll(word, joinedSpace, ""), droppedSquare, "")
}

// squareGoes is whether what follows a [ makes it go: two characters or more that are no number (a quote after it
// counted in), a quote at its end and the closing marks, dashes and slashes before that aside.
func squareGoes(rest string) bool {
	rest = strings.TrimRight(strings.TrimSuffix(rest, "\""), "])!?.,;:/-")
	return len(rest) >= 2 && !numeric(strings.TrimSuffix(rest, "\""))
}

// numeric is whether a word is a number, a dot may start it.
func numeric(word string) bool {
	return numeral(strings.TrimPrefix(word, "."), false)
}

// brackets spaces a paragraph's brackets, size being its length as they count it. First a ( loses the spaces after
// it and, in a paragraph with a (, a ) the spaces before it (a dot before two or more is doubled): each space that
// goes gives one more of what size leaves of markLimit, and then each ( and ) takes one. A ( gets a space before it
// while some is left; past that one with no space before it goes, but at the start and after a dot, comma, dash, plus
// or slash. A ) then gets a space after it but before a comma, dot or question mark while some is left; past that it
// loses the spaces after it before a comma, dot or ). In a long paragraph every ( is past the limit and nothing else
// changes.
func brackets(paragraph string, size int, long bool) string {
	trimmed := paragraph
	if !long {
		trimmed = openingsTrimmed(paragraph)
		if strings.Contains(trimmed, "(") {
			trimmed = closingsTrimmed(trimmed)
		}
	}
	room := markLimit - size + len(paragraph) - len(trimmed)
	parts := strings.Split(trimmed, "(")
	var opened strings.Builder
	opened.WriteString(parts[0])
	for _, part := range parts[1:] {
		room--
		written := opened.String()
		switch {
		case strings.HasSuffix(written, " "):
			opened.WriteByte('(')
		case !long && room > 0:
			opened.WriteString(" (")
		case written == "" || strings.IndexByte(".,-+/", written[len(written)-1]) >= 0:
			opened.WriteByte('(')
		}
		opened.WriteString(part)
	}
	paragraph = opened.String()
	if long || len(parts) == 1 {
		return paragraph
	}
	var closed strings.Builder
	for index := 0; index < len(paragraph); index++ {
		closed.WriteByte(paragraph[index])
		if paragraph[index] != ')' {
			continue
		}
		room--
		stop := spaceRun(paragraph, index+1)
		if stop == len(paragraph) {
			continue
		}
		switch following := paragraph[stop]; {
		case room > 0 && stop == index+1 && strings.IndexByte(",.?", following) < 0:
			closed.WriteByte(' ')
		case room <= 0 && strings.IndexByte(",.)", following) >= 0:
			index = stop - 1
		}
	}
	return closed.String()
}

// openingsTrimmed takes the spaces after each ( out of a paragraph.
func openingsTrimmed(paragraph string) string {
	var output strings.Builder
	for index := 0; index < len(paragraph); index++ {
		output.WriteByte(paragraph[index])
		if paragraph[index] == '(' {
			index = spaceRun(paragraph, index+1) - 1
		}
	}
	return output.String()
}

// closingsTrimmed takes the spaces before each ) out of a paragraph, a dot before two or more doubled.
func closingsTrimmed(paragraph string) string {
	var output strings.Builder
	for index := 0; index < len(paragraph); index++ {
		if run := spaceRun(paragraph, index); run > index && run < len(paragraph) && paragraph[run] == ')' {
			if run-index >= 2 && index > 0 && paragraph[index-1] == '.' {
				output.WriteByte('.')
			}
			index = run - 1
			continue
		}
		output.WriteByte(paragraph[index])
	}
	return output.String()
}

// angleRun is the words of a > from index, the spaces around them included: before a number one > stays, a space
// before it if there were more, between words they are two spaces, and else only their spaces stay.
func angleRun(paragraph string, index int) (int, string, bool) {
	angle := spaceRun(paragraph, index)
	if angle == index && index > 0 && paragraph[index-1] != ' ' || angle == len(paragraph) || paragraph[angle] != '>' {
		return 0, "", false
	}
	more := angle + 1
	for more+1 < len(paragraph) && paragraph[more] == ' ' && paragraph[more+1] == '>' {
		more += 2
	}
	if more < len(paragraph) && paragraph[more] != ' ' {
		if more == angle+1 {
			return 0, "", false
		}
		more -= 2
	}
	stop := spaceRun(paragraph, more)
	before, after := paragraph[index:angle], paragraph[more:stop]
	switch {
	case after != "" && stop < len(paragraph) && digit(paragraph[stop]):
		if more > angle+1 {
			before += " "
		}
		return stop, before + ">" + after, true
	case before != "" && index > 0 && stop < len(paragraph):
		return stop, "  ", true
	}
	return stop, before + after, true
}
