package preformatter

import (
	"cmp"
	"slices"
	"strings"
)

// slashMarks are the marks that don't stop a slash beside them.
const slashMarks = "!\"(),.:;?[]"

// pluses drops a + after the ! and ? that start a word, and puts a space on each side of every other + where there
// is none, but before the ! and ? that end its word.
func pluses(paragraph string) string {
	marked := []byte(paragraph)
	for index := range marked {
		if index > 0 && marked[index-1] != ' ' {
			continue
		}
		run := index
		for run < len(marked) && (marked[run] == '!' || marked[run] == '?') {
			run++
		}
		if run > index && run < len(marked) && marked[run] == '+' {
			marked[run] = ' '
		}
	}
	paragraph = string(marked)
	var output strings.Builder
	for index := 0; index < len(paragraph); index++ {
		if paragraph[index] != '+' {
			output.WriteByte(paragraph[index])
			continue
		}
		if index > 0 && paragraph[index-1] != ' ' {
			output.WriteByte(' ')
		}
		output.WriteByte('+')
		if rest := paragraph[index+1:]; rest != "" && rest[0] != ' ' && !terminalWord(rest) {
			output.WriteByte(' ')
		}
	}
	return output.String()
}

// terminalWord is whether a text starts with a word of ! and ?.
func terminalWord(text string) bool {
	run := terminalRun(text, 0)
	return run > 0 && (run == len(text) || text[run] == ' ')
}

// dashes spaces each word's runs of dashes.
func dashes(paragraph string) string {
	words := strings.Split(paragraph, " ")
	for index, word := range words {
		if !strings.Contains(word, "-") {
			continue
		}
		following := words[index+1 : min(index+3, len(words))]
		var spaced strings.Builder
		for character := 0; character < len(word); {
			if word[character] != '-' {
				spaced.WriteByte(word[character])
				character++
				continue
			}
			stop := character
			for stop < len(word) && word[stop] == '-' {
				stop++
			}
			spaced.WriteString(spacedDash(stop-character, word[:character], word[stop:], following))
			character = stop
		}
		words[index] = spaced.String()
	}
	return strings.Join(words, " ")
}

// spacedDash is a run of dashes in a word, with the word's characters before and after it and the two words after
// the word. Dashes two or more together each get a space on each side. A word's first dash, or one after a slash,
// gets a space after it but before the ! and ? that end the word, and one after an opening quote or bracket that a
// closing one follows closely; a dash between digits stays, any other is a space, none at the word's end.
func spacedDash(run int, before, after string, following []string) string {
	switch {
	case run > 1:
		return strings.Repeat(" - ", run)
	case before == "" || strings.HasSuffix(before, "/"):
		if strings.Trim(after, "!?") == "" {
			return "-"
		}
		return "- "
	case digit(before[len(before)-1]) && after != "" && digit(after[0]):
		return "-"
	case opened(before) && closes(after, following):
		return "- "
	case after != "":
		return " "
	}
	return ""
}

// opened is whether an opening quote or bracket stands in a word's characters before a dash, with no dash after it.
func opened(before string) bool {
	return strings.ContainsAny(before[strings.LastIndexByte(before, '-')+1:], "\"'([")
}

// closes is whether a closing quote or bracket follows a dash closely: the last one in the rest of its word, its
// commas spaced, or in the next word for a dash that ends its word, is at most two characters in with no dash or
// slash before it; but not in a word that ends a sentence with more than the closer before its dot.
func closes(after string, following []string) bool {
	rest, next := after, following
	if after == "" {
		next = nil
		if len(following) > 0 {
			rest, next = following[0], following[1:]
		}
	}
	if strings.HasSuffix(rest, ".") && len(rest) > 2 && len(next) > 0 && next[0] == "" {
		return false
	}
	word, _, _ := strings.Cut(wordCommas(rest, false), " ")
	last := strings.LastIndexAny(word, "\"')]")
	return last >= 0 && last <= 2 && !strings.ContainsAny(word[:last], "-/")
}

// kept is whether a part of a word beside a slash keeps it: one with a small letter, or of marks that don't stop it,
// or none, a > that starts it not counted.
func kept(part string) bool {
	part = strings.TrimPrefix(part, ">")
	return strings.ContainsFunc(part, func(character rune) bool { return character >= 'a' && character <= 'z' }) ||
		strings.Trim(part, slashMarks) == ""
}

// slashes spaces each word's slashes. One that ends a word after something has no space after it, and a word they
// leave nothing of goes.
func slashes(paragraph string) string {
	words := strings.Split(paragraph, " ")
	following := ""
	for index := len(words) - 1; index >= 0; index-- {
		word := words[index]
		if strings.Contains(word, "/") {
			ended := strings.HasSuffix(word, "/") && strings.Trim(word, "/") != ""
			word = slashed(word, following)
			if ended {
				word = strings.TrimSuffix(word, " ")
			}
			if word == "" {
				words = slices.Delete(words, index, index+1)
				continue
			}
			words[index] = word
		}
		if first, _, _ := strings.Cut(word, " "); first != "" {
			following = first
		}
	}
	return strings.Join(words, " ")
}

// slashed is a word's slashes, following being the next word: the ones that start it stay, all stay in a path (two
// slashes with something between them), the first of two or more together goes, and a single one stays if the
// parts beside it (or the next word, for one that ends the word) keep it. A slash that stays has a space after it,
// one that goes is a space. A ( or > that starts the word goes with the slash after it when the part after that
// slash doesn't keep it (for a >, one not starting with a digit), or when another slash follows.
func slashed(word, following string) string {
	rest := strings.TrimLeft(word, "/")
	output := strings.Repeat("/ ", len(word)-len(rest))
	if len(rest) >= 2 && (rest[0] == '(' || rest[0] == '>') && rest[1] == '/' {
		after := rest[2:]
		if strings.HasPrefix(after, "/") {
			return output + slashed(after, following)
		}
		first := following
		if after != "" {
			first, _, _ = strings.Cut(after, "/")
		}
		if !kept(first) && !(rest[0] == '>' && first != "" && digit(first[0])) {
			return output + strings.ReplaceAll(after, "/", "/ ")
		}
	}
	parts := strings.Split(rest, "/")
	switch {
	case len(parts) == 1:
		return output + rest
	case strings.Join(parts[1:len(parts)-1], "") != "":
		return output + strings.ReplaceAll(rest, "/", "/ ")
	case len(parts) > 2:
		return output + parts[0] + strings.Repeat("/ ", len(parts)-2) + parts[len(parts)-1]
	}
	left, right := parts[0], parts[1]
	switch {
	case !kept(left) || !kept(cmp.Or(right, following)):
		return output + left + " " + right
	case left != "" && strings.Trim(left, "!?") == "":
		return output + left + " / " + right
	}
	return output + left + "/ " + right
}

// underscores spaces each word's underscores.
func underscores(paragraph string) string {
	words := strings.Split(paragraph, " ")
	for index, word := range words {
		words[index] = underscored(word, index+1 < len(words) && (words[index+1] != "" || index+2 == len(words)))
	}
	return strings.Join(words, " ")
}

// underscored makes a word of underscores, or one in underscores, a ~ and the word without its underscores. A comma
// or a slash may end it, or a dot if dotted (one space follows the word).
func underscored(word string, dotted bool) string {
	lead := len(word) - len(strings.TrimLeft(word, "_"))
	if lead == 0 {
		return word
	}
	ends := func(end string) bool {
		return end == "" || end == "," || end == "/" || end == "." && dotted
	}
	for start := lead + 1; start < len(word); start++ {
		if word[start] != '_' || word[start-1] == '_' {
			continue
		}
		stop := start
		for stop < len(word) && word[stop] == '_' {
			stop++
		}
		if ends(word[stop:]) {
			return "~" + strings.ReplaceAll(word[lead:start], "_", "") + word[stop:]
		}
	}
	if ends(word[lead:]) {
		return "~" + word[lead:]
	}
	return word
}

// loosened makes two dots and a dash or a slash after them two dots and a space where the dash or the slash goes
// (for the sentence end test): a dash with no space after it, or a word's only slash after a part that is no word
// and that the parts beside it don't keep.
func (address *address) loosened(paragraph string) string {
	words := strings.Split(paragraph, " ")
	for index, word := range words {
		mark := looseMark(word)
		if mark < 0 {
			continue
		}
		before, after := word[:mark], word[mark+1:]
		var goes bool
		if word[mark] == '-' {
			goes = !(opened(before) && closes(after, nil))
		} else {
			following := ""
			for _, later := range words[index+1:] {
				if later != "" {
					following = later
					break
				}
			}
			stem := before[:len(before)-2]
			stem = stem[strings.LastIndexByte(stem, ',')+1:]
			goes = address.plainKind(stem+".", false) != wordKind && !strings.Contains(after, "/") &&
				!(kept(before) && kept(cmp.Or(after, following)))
		}
		if goes {
			words[index] = before
			if after != "" {
				words[index] += " " + after
			}
		}
	}
	return strings.Join(words, " ")
}

// looseMark is where a word's first single dash or slash after two dots after a character that is no dot is, or -1.
func looseMark(word string) int {
	for index := 1; index+2 < len(word); index++ {
		if word[index-1] != '.' && word[index] == '.' && word[index+1] == '.' && slashOrDash(word[index+2]) &&
			(index+3 == len(word) || !slashOrDash(word[index+3])) {
			return index + 2
		}
	}
	return -1
}

func slashOrDash(character byte) bool {
	return character == '-' || character == '/'
}
