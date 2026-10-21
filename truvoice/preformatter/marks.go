package preformatter

import "strings"

// separatorUnits are the marks that go when one of them stands twice or more in a row, tried in this order.
var separatorUnits = []string{"%%", "&&", "**", "++", "--", "//", "::", ";;", "<<", "==", "@@", "^^", "__", "||", "##",
	">>", "+-", "+_", "-=", "-_", "<>"}

// marks reads a paragraph's marks before its words are spaced (section 3.4).
func marks(paragraph string) string {
	paragraph = separators(paragraph)
	if strings.Contains(paragraph, `\\`) {
		paragraph = strings.ReplaceAll(paragraph, `\`, "")
	}
	paragraph = bars(paragraph)
	paragraph = terminalMarks(paragraph)
	words := strings.Split(paragraph, " ")
	for index, word := range words {
		switch {
		case index == 0:
		case len(word) > 4 && terminal(word):
			words[index] = word[:4]
		case word == ";":
			words[index] = "semicolon"
		case word == ":":
			words[index] = "colon"
		}
	}
	words = strings.Split(approximately(strings.Join(words, " ")), " ")
	for index, word := range words {
		if word != "" && len(word) <= 3 && strings.Trim(word, "_") == "" {
			words[index] = "~"
		}
	}
	return strings.Join(words, " ")
}

// separators drops each run of a separator unit twice or more.
func separators(paragraph string) string {
	var output strings.Builder
	for index := 0; index < len(paragraph); {
		if run := separatorRun(paragraph, index); run > index {
			index = run
			continue
		}
		output.WriteByte(paragraph[index])
		index++
	}
	return output.String()
}

func separatorRun(paragraph string, index int) int {
	for _, unit := range separatorUnits {
		run := index
		for strings.HasPrefix(paragraph[run:], unit) {
			run += len(unit)
		}
		if run-index >= 2*len(unit) {
			return run
		}
	}
	return index
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

// unmarked reads a paragraph's marks after its words are spaced (section 3.4): words of a > go, and a word's leading >
// before a character that is no digit.
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
