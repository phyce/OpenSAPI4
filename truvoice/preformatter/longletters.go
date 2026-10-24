package preformatter

import (
	"slices"
	"strings"
)

// headed is whether a word of five capitals or more splits off a piece or head it starts with (section 3.9): not when
// an unsplit word that starts with the head starts the word too (ALTAR, BITE, BITTERLY, INFORMATION), nor when the
// word is the head (GOPHER).
func (tables *letterTables) headed(word, head string) bool {
	if len(word) <= len(head) || !strings.HasPrefix(word, head) {
		return false
	}
	return !slices.ContainsFunc(tables.unsplit, func(unsplit string) bool {
		return strings.HasPrefix(word, unsplit) && strings.HasPrefix(unsplit, head)
	})
}

// tailed is whether a word of five capitals or more splits off a piece or tail it ends with (section 3.9): not when
// an unsplit word that ends with the tail ends the word too (HABIT, BONE, MAGNET, TROUSER), nor when the word is the
// tail (GOPHER).
func (tables *letterTables) tailed(word, tail string) bool {
	if len(word) <= len(tail) || !strings.HasSuffix(word, tail) {
		return false
	}
	return !slices.ContainsFunc(tables.unsplit, func(unsplit string) bool {
		return strings.HasSuffix(word, unsplit) && strings.HasSuffix(unsplit, tail)
	})
}

// longLetters is how a word of five capitals or more that no list has, not in a run, is written (section 3.9).
func (tables *letterTables) longLetters(word string) string {
	start, rest, end := word[:2], word[2:], word[len(word)-2:]
	switch {
	case tables.kept[word]:
		return word
	case start == "NY" && strings.ContainsAny(rest, "AEIOUY"):
		return "NY-" + rest
	case start == "NY":
		return spelled(word, true, true)
	case start == "LA" && strings.Trim(rest, "BCDFGHJKLMNPQRSTVWXZ") == "":
		return spelled(word, true, true)
	case start == "LA" || end == "LA" || end == "NY":
		return word
	case start == "US" && !oneOf(word[2], "EHU"):
		return "U-S-" + rest
	case end == "US":
		return word
	case start == "IO" && word[2] != 'N':
		return "eye-oh-" + rest
	case word[:3] == "EDU":
		return "ed-yoo-" + word[3:]
	case start == "UK":
		return "U-K-" + rest
	case start == "OS" || start == "PC":
		return start + "-" + rest
	}
	for _, head := range slices.Concat(tables.pieces, tables.longHeads) {
		if !tables.headed(word, head) {
			continue
		}
		if len(word)-len(head) == 1 {
			return head + "-" + spelledLetter(word, len(word)-1)
		}
		return head + "-" + word[len(head):]
	}
	for _, tail := range slices.Concat(tables.pieces, tables.longTails) {
		if !tables.tailed(word, tail) {
			continue
		}
		front := word[:len(word)-len(tail)]
		switch {
		case len(front) == 1:
			front = spelledLetter(word, 0)
		case front == "IO":
			front = "eye-oh"
		}
		return front + "-" + tail
	}
	return word
}
