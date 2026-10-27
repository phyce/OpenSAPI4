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
			spaced.WriteString(broken(spacedDash(stop-character, word[:character], word[stop:], following, index > 0)))
			character = stop
		}
		words[index] = spaced.String()
	}
	return strings.Join(words, " ")
}

// broken is a dash or slash written as a space or nothing with a runBreak: between spaces, or after a space at a
// word's end.
func broken(written string) string {
	switch written {
	case " ":
		return " " + runBreak + " "
	case "":
		return " " + runBreak
	}
	return written
}

// spacedDash is a run of dashes in a word, with the word's characters before and after it and the two words after the
// word, spaced if a space stands before the word. Dashes two or more together each get a space on each side, the space
// before the word counting for the first. A word's first dash, or one after a slash, gets a space after it but before
// the ! and ? that end the word, and one after an opening quote or bracket that a closing one follows closely; a dash
// between digits stays, any other is a space, none at the word's end.
func spacedDash(run int, before, after string, following []string, spaced bool) string {
	switch {
	case run > 1 && spaced && before == "":
		return strings.Repeat(" - ", run)[1:]
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

// slashKept is whether a part of a word beside a slash keeps it, a space following the part.
func (tables *letterTables) slashKept(part string) bool {
	return tables.slashKeptBefore(part, " ")
}

// slashKeptBefore is whether a part of a word beside a slash keeps it, after being what follows the part: one with a
// small letter, or of marks that don't stop it, or none, a > that starts it not counted, up to the first comma its
// commas space. It is read as its underscores make it, and one that lowered changes has a small letter.
func (tables *letterTables) slashKeptBefore(part, after string) bool {
	dotted := strings.HasPrefix(after, " ") && !strings.HasPrefix(after, "  ")
	part, _, _ = strings.Cut(wordUnderscores(strings.TrimPrefix(part, ">"), dotted), " ")
	return strings.ContainsFunc(part, func(character rune) bool { return character >= 'a' && character <= 'z' }) ||
		strings.Trim(part, slashMarks) == "" || tables.lowered(part) != ""
}

// slashes spaces each word's slashes. One that ends a word after something has no space after it, and a word they
// leave nothing of goes. A word that ends in a slash after something and the next word, when it has a slash after
// something, are a path.
func (address *address) slashes(paragraph string) string {
	words := strings.Split(paragraph, " ")
	paths := make([]bool, len(words))
	upcoming := make([]string, len(words))
	endings := make([]string, len(words))
	for index := 1; index < len(words); index++ {
		endings[index-1] = "  "
		if words[index] != "" {
			endings[index-1] = " " + words[index][:1]
		}
	}
	last := -1
	for index, word := range words {
		if word == "" {
			continue
		}
		if last >= 0 {
			upcoming[last] = word
			if strings.HasSuffix(words[last], "/") && strings.Trim(words[last], "/") != "" &&
				strings.Contains(strings.TrimLeft(word, "/"), "/") {
				paths[last], paths[index] = true, true
			}
		}
		last = index
	}
	following := ""
	for index := len(words) - 1; index >= 0; index-- {
		word := words[index]
		if strings.Contains(word, "/") {
			ended := strings.HasSuffix(word, "/") && strings.Trim(word, "/") != ""
			word = address.slashed(word, following, paths[index], upcoming[index], endings[index])
			if ended {
				word = strings.TrimSuffix(word, " ")
			}
			if word == "" {
				words = slices.Delete(words, index, index+1)
				continue
			}
			words[index] = word
		}
		if strings.HasPrefix(word, openingGone) {
			word = word[min(len(openingGone)+1, len(word)):]
		}
		if first, _, _ := strings.Cut(word, " "); first != "" {
			following = first
		}
	}
	return strings.Join(words, " ")
}

// slashed is a word's slashes, following being the next word as its slashes left it, upcoming the next word as it was
// and ending what follows the word: the ones that start it stay, all stay in a path (two slashes with something between
// them, or a word of a path with more than one) but the first of a run between two parts, the first of two or more
// together is a space, and a single one stays in a path or if the parts beside it (or the next word, for one that ends
// the word, but none at the paragraph's end) keep it, a kept space before it going with it otherwise, as one before a
// run does (goneBefore). A slash that stays has a space after it, one that goes is a space. A list word before a single
// slash that stays drops it (slashDropped after the slash), and one before a slash that went is marked (slashGone). A (
// or > that starts the word goes with the slash after it when the part after that slash doesn't keep it (for a >, one
// not starting with a digit), or when another slash follows; for one that ends the word, a ( reads the upcoming word's
// part before its first slash, and goes with its slash, and nothing stays, when that word starts with a slash. When the
// part after the slash keeps neither, an openingGone stands before it (section 3.10).
func (address *address) slashed(word, following string, path bool, upcoming, ending string) string {
	rest := strings.TrimLeft(word, "/")
	output := strings.Repeat("/ ", len(word)-len(rest))
	if len(rest) >= 2 && (rest[0] == '(' || rest[0] == '>') && rest[1] == '/' {
		after := rest[2:]
		if strings.HasPrefix(after, "/") {
			return output + address.slashed(after, following, path, "", ending)
		}
		if rest[0] == '>' {
			upcoming = ""
		}
		if after == "" && strings.HasPrefix(upcoming, "/") {
			return output
		}
		first := following
		switch {
		case after != "":
			first, _, _ = strings.Cut(after, "/")
		case upcoming != "":
			first, _, _ = strings.Cut(upcoming, "/")
		}
		if !address.letters.slashKept(first) && !(rest[0] == '>' && first != "" && digit(first[0])) {
			return output + openingGone + " " + strings.ReplaceAll(after, "/", "/ ")
		}
	}
	parts := strings.Split(rest, "/")
	switch {
	case len(parts) == 1:
		return output + rest
	case strings.Join(parts[1:len(parts)-1], "") != "" || path && len(parts) > 2:
		return output + strings.Join(thinned(parts), "/ ")
	case len(parts) > 2:
		return output + address.goneBefore(parts[0]) + strings.Repeat(" /", len(parts)-2) + " " + parts[len(parts)-1]
	}
	left, right := parts[0], parts[1]
	rightKept := following != "" && address.letters.slashKept(following)
	if right != "" {
		rightKept = address.letters.slashKeptBefore(right, ending)
	}
	switch {
	case !path && (!address.letters.slashKept(left) || !rightKept):
		return output + address.goneBefore(left) + " " + runBreak + " " + right
	case left != "" && strings.Trim(left, "!?") == "":
		return output + left + " / " + right
	}
	if _, ok := address.listed(left, 1, ""); ok {
		return output + left + "/" + slashDropped + " " + right
	}
	return output + left + "/ " + right
}

// thinned is the parts between a word's slashes when they all stay: a run of slashes after a part and before one
// with a letter or digit loses its first.
func thinned(parts []string) []string {
	var kept []string
	for index, part := range parts {
		if part == "" && index > 0 && parts[index-1] != "" && slices.ContainsFunc(parts[index+1:], hasAlphanumeric) {
			continue
		}
		kept = append(kept, part)
	}
	return kept
}

// goneBefore is a part before a slash that went: without the kept spaces that end it, which a list word that isn't
// plain keeps, and with a list word's mark.
func (address *address) goneBefore(part string) string {
	bare := strings.TrimRight(part, keptSpace)
	if entry, ok := address.listed(bare, 1, ""); ok && !plainListed(entry.Kind) {
		return part + slashGone
	}
	return bare + address.goneSlash(bare)
}

// goneSlash is the mark a list word gets for the slash after it that went.
func (address *address) goneSlash(part string) string {
	if _, ok := address.listed(part, 1, ""); ok {
		return slashGone
	}
	return ""
}

// commasAndUnderscores spaces the commas of a paragraph's words and reads their underscores. A slash's mark that
// ends a word stays at its end. When lowered changes a word's first piece, that piece is the whole word, before the
// pieces after it: XB,. is x B,. . (section 3.9).
func (tables *letterTables) commasAndUnderscores(paragraph string) string {
	words := strings.Split(paragraph, " ")
	for index, word := range words {
		mark := ""
		if strings.HasSuffix(word, slashDropped) || strings.HasSuffix(word, slashGone) {
			word, mark = word[:len(word)-1], word[len(word)-1:]
		}
		dotted := index+1 < len(words) && (words[index+1] != "" || index+2 == len(words))
		spaced := wordUnderscores(word, dotted)
		if first, rest, ok := strings.Cut(spaced, " "); ok && tables.lowered(first) != "" {
			spaced = word + " " + rest
		}
		words[index] = spaced + mark
	}
	return strings.Join(words, " ")
}

// wordUnderscores spaces a word's commas and reads the underscores of the words that leaves, dotted if one space
// follows the word. In a word that doesn't end in a comma or a dot, what ends in a comma so spaced is no word in
// underscores.
func wordUnderscores(word string, dotted bool) string {
	held := !strings.HasSuffix(word, ",") && !strings.HasSuffix(word, ".")
	parts := strings.Split(wordCommas(word, false), " ")
	for part := range parts {
		if last := part == len(parts)-1; last || !held {
			parts[part] = underscored(parts[part], last && dotted)
		}
	}
	return strings.Join(parts, " ")
}

// underscored makes a word of underscores, or one in underscores, a ~ and the word without its underscores, in small
// letters. A comma or a slash may end it, or a dot if dotted (one space follows the word).
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
			return "~" + lower(strings.ReplaceAll(word[lead:start], "_", "")) + word[stop:]
		}
	}
	if ends(word[lead:]) {
		return "~" + word[lead:]
	}
	return word
}

// starred reads stars around words. A word that starts with a * and has a letter opens, but one with a dot after its
// only star: its stars are a ~ (three before a single character, a slash after it aside, leave one). The first stars
// after it in the paragraph that don't start a word close, and go when no letter or digit follows them in their word.
func starred(paragraph string) string {
	words := strings.Split(paragraph, " ")
	opened := false
	for index, word := range words {
		prefix := ""
		lettered := strings.ContainsFunc(word, func(character rune) bool {
			return character < 0x80 && alphabetic(byte(character))
		})
		if strings.HasPrefix(word, "*") && !strings.HasPrefix(word, "*.") && lettered {
			stars := len(word) - len(strings.TrimLeft(word, "*"))
			word = word[stars:]
			prefix = "~"
			if stars == 3 && len(strings.TrimRight(word, "/")) == 1 {
				prefix = "~*"
			}
			opened = true
		}
		if opened {
			lead := len(word) - len(strings.TrimLeft(word, "*"))
			if start := strings.IndexByte(word[lead:], '*'); start >= 0 {
				start += lead
				stop := start
				for stop < len(word) && word[stop] == '*' {
					stop++
				}
				if after := word[stop:]; !hasAlphanumeric(after) {
					word = word[:start] + strings.ReplaceAll(after, "*", "")
				}
				opened = false
			}
		}
		words[index] = prefix + word
	}
	return strings.Join(words, " ")
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
				!(address.letters.slashKept(before) && address.letters.slashKept(cmp.Or(after, following)))
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
