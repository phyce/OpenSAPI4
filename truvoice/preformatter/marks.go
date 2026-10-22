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

// marks reads a paragraph's marks before its words are spaced (section 3.4).
func marks(paragraph string) string {
	paragraph = arrows(readFaces(paragraph))
	for _, unit := range separatorUnits {
		if strings.Contains(paragraph, unit+unit) {
			paragraph = strings.ReplaceAll(paragraph, unit, "")
		}
	}
	if strings.Contains(paragraph, `\\`) {
		paragraph = strings.ReplaceAll(paragraph, `\`, "")
	}
	paragraph = bars(paragraph)
	paragraph = terminalMarks(paragraph)
	words := strings.Split(paragraph, " ")
	for index, word := range words {
		if index > 0 && len(word) > 4 && terminal(word) {
			words[index] = word[:4]
		}
	}
	words = strings.Split(terminalNames(strings.Join(words, " ")), " ")
	for index, word := range words {
		switch {
		case index == 0:
		case word == ";":
			words[index] = "semicolon"
		case word == ":":
			words[index] = "colon"
		}
	}
	return brackets(approximately(strings.Join(words, " ")))
}

// readFaces reads each face, with a space on each side.
func readFaces(paragraph string) string {
	var output strings.Builder
	for index := 0; index < len(paragraph); {
		read := false
		for _, face := range faces {
			if strings.HasPrefix(paragraph[index:], face.face) {
				output.WriteString(" " + face.reading + " ")
				index += len(face.face)
				read = true
				break
			}
		}
		if !read {
			output.WriteByte(paragraph[index])
			index++
		}
	}
	return output.String()
}

// terminalNames reads a ! or ? in quotes or brackets as its name, a word of its own, and a word of !s or !'s (?s or
// ?'s) after a space as the plural of its name, with a comma on each side. The ! is read before the ?.
func terminalNames(paragraph string) string {
	for _, named := range markNames {
		paragraph = quotedMark(paragraph, named.mark, named.name)
		for _, plural := range []string{"s", "'s"} {
			paragraph = pluralMark(paragraph, " "+named.mark+plural, ", "+named.name+"s"+named.mark+",")
		}
	}
	return paragraph
}

// quotedMark reads each mark in quotes or brackets as its name, with a space on each side where there is none.
func quotedMark(paragraph, mark, name string) string {
	var output strings.Builder
	for index := 0; index < len(paragraph); {
		rest := paragraph[index:]
		if !strings.HasPrefix(rest, `"`+mark+`"`) && !strings.HasPrefix(rest, "("+mark+")") {
			output.WriteByte(paragraph[index])
			index++
			continue
		}
		stop := index + 3
		if index == 0 || paragraph[index-1] != ' ' {
			output.WriteByte(' ')
		}
		output.WriteString(paragraph[index:index+1] + name + paragraph[index+1:stop])
		if !wordEnds(paragraph, stop) {
			output.WriteByte(' ')
		}
		index = stop
	}
	return output.String()
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

// arrows reads each arrow as "to and from" or "to", with a space on each side where there is none.
func arrows(paragraph string) string {
	var output strings.Builder
	for index := 0; index < len(paragraph); {
		arrow := ""
		for _, mark := range arrowMarks {
			if strings.HasPrefix(paragraph[index:], mark) {
				arrow = mark
				break
			}
		}
		if arrow == "" {
			output.WriteByte(paragraph[index])
			index++
			continue
		}
		stop := index + len(arrow)
		if index == 0 || paragraph[index-1] != ' ' {
			output.WriteByte(' ')
		}
		if arrow[0] == '<' {
			output.WriteString("to and from")
		} else {
			output.WriteString("to")
		}
		if stop == len(paragraph) || paragraph[stop] != ' ' {
			output.WriteByte(' ')
		}
		index = stop
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

// brackets spaces a paragraph's brackets: a ( has a space before it and none after it. In a paragraph with a (, a )
// has none before it, a dot before two or more doubled, and one after it but before a comma, a dot or a question mark.
func brackets(paragraph string) string {
	var opened strings.Builder
	for index := 0; index < len(paragraph); index++ {
		if paragraph[index] != '(' {
			opened.WriteByte(paragraph[index])
			continue
		}
		if written := opened.String(); written == "" || written[len(written)-1] != ' ' {
			opened.WriteByte(' ')
		}
		opened.WriteByte('(')
		index = spaceRun(paragraph, index+1) - 1
	}
	paragraph = opened.String()
	if !strings.Contains(paragraph, "(") {
		return paragraph
	}
	var closed strings.Builder
	for index := 0; index < len(paragraph); index++ {
		if run := spaceRun(paragraph, index); run > index && run < len(paragraph) && paragraph[run] == ')' {
			if run-index >= 2 && index > 0 && paragraph[index-1] == '.' {
				closed.WriteByte('.')
			}
			index = run - 1
			continue
		}
		closed.WriteByte(paragraph[index])
	}
	paragraph = closed.String()
	var spaced strings.Builder
	for index := 0; index < len(paragraph); index++ {
		spaced.WriteByte(paragraph[index])
		if paragraph[index] == ')' && index+1 < len(paragraph) && strings.IndexByte(" ,.?", paragraph[index+1]) < 0 {
			spaced.WriteByte(' ')
		}
	}
	return spaced.String()
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
