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

// longLetters is how a word of five capitals or more that no list has, not in a run, is written (section 3.9), with
// the signs and letters after them that spelledEntry gives it: the letters after NY and LA are tested small or
// capital (NYMVK~dog is NY-MVK~dog, LASRB$a is kept). Exact is as capitalsWritten says: the starts NY, LA, US and UK
// count only in capitals, and IO and EDU, at the start or IO as the front, are eye-oh and ed-yoo only in capitals
// (section 3.12: nYBAPL is kept, eDUAH is eDU-AH, iODBR iO-DBR, iONET iO-NET).
func (tables *letterTables) longLetters(word, exact string) string {
	start, writtenStart, rest, end := word[:2], exact[:2], word[2:], word[len(word)-2:]
	vocalRest := strings.ContainsAny(rest, "AEIOUYaeiouy")
	switch {
	case tables.listed(tables.kept, word, exact):
		return word
	case writtenStart == "NY" && vocalRest:
		return "NY-" + rest
	case writtenStart == "NY":
		return spelled(word, true, true)
	case writtenStart == "LA" && !vocalRest:
		return spelled(word, true, true)
	case writtenStart == "LA" || end == "LA" || end == "NY":
		return word
	case writtenStart == "US" && !oneOf(word[2], "EHU"):
		return "U-S-" + rest
	case end == "US":
		return word
	case start == "IO" && word[2] != 'N' && writtenStart == "IO":
		return "eye-oh-" + rest
	case start == "IO" && word[2] != 'N':
		return "IO-" + rest
	case word[:3] == "EDU" && exact[:3] == "EDU":
		return "ed-yoo-" + word[3:]
	case word[:3] == "EDU":
		return "EDU-" + word[3:]
	case writtenStart == "UK":
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
		case front == "IO" && writtenStart == "IO":
			front = "eye-oh"
		}
		return front + "-" + tail
	}
	return word
}
