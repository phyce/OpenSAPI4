package preformatter

import (
	"cmp"
	"strings"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
)

// letterTables is the address context's tables for words in capitals of two to four letters (section 3.9).
type letterTables struct {
	twoLetters, onsets, firstPairs, lastPairs, spelled, markedSpelled, unkept, kept map[string]bool
	heads, tails, openHeads, vowelCodas, codaPairs                                  map[string]bool
	codas, respelled, unkeptHeads, clusters, readings                               map[string]string
}

func newLetterTables(letters *datapack.Letters) letterTables {
	return letterTables{twoLetters: wordSet(letters.TwoLetters), onsets: wordSet(letters.Onsets),
		firstPairs: wordSet(letters.FirstPairs), lastPairs: wordSet(letters.LastPairs),
		spelled: wordSet(letters.Spelled), markedSpelled: wordSet(letters.MarkedSpelled),
		unkept: wordSet(letters.Unkept), kept: wordSet(letters.Kept), heads: wordSet(letters.Heads),
		tails: wordSet(letters.Tails), openHeads: wordSet(letters.OpenHeads), vowelCodas: wordSet(letters.VowelCodas),
		codaPairs: wordSet(letters.CodaPairs), codas: letters.Codas, respelled: letters.Respelled,
		unkeptHeads: letters.UnkeptHeads, clusters: letters.Clusters, readings: letters.Readings}
}

// lowered is a word in capitals of two to four letters with no opening marks, before any marks, that starts with Q, X
// or Z before a consonant (not ZW, and not of Roman numerals' letters only), with that letter small and a space after
// it (section 3.9); "" for any other word and a respelled one without closing marks, which is read or respelled and
// counts as capitals beside other words. A word is tested by its first piece as its commas space it, the rest of it
// written after that piece: XB,dog is x B,dog.
func (tables *letterTables) lowered(token string) string {
	piece, _, _ := strings.Cut(wordCommas(token, false), " ")
	letters := 0
	for letters < len(piece) && upper(piece[letters]) {
		letters++
	}
	word, closing := piece[:letters], piece[letters:]
	_, respelled := tables.respelled[word]
	if len(word) < 2 || len(word) > 4 || hasAlphanumeric(closing) ||
		strings.IndexByte("QXZ", word[0]) < 0 || !consonant(word[1]) || word[:2] == "ZW" ||
		strings.Trim(word, "CDILMVX") == "" || respelled && closing == "" {
		return ""
	}
	return lower(token[:1]) + " " + token[1:]
}

// counted is whether a word counts in a run of words in capitals (section 3.9): one with two capitals or more, as
// lowered leaves it.
func (tables *letterTables) counted(token string) bool {
	capitals := 0
	for _, character := range []byte(cmp.Or(tables.lowered(token), token)) {
		if upper(character) {
			capitals++
		}
	}
	return capitals >= 2
}

// unbroken is a paragraph's words without the runBreaks among them, and which words a runBreak stood before.
func unbroken(parts []part) ([]part, map[int]bool) {
	kept := parts[:0]
	breaks := map[int]bool{}
	for _, part := range parts {
		if part.token == runBreak {
			breaks[len(kept)] = true
			continue
		}
		kept = append(kept, part)
	}
	return kept, breaks
}

// capitalRuns is whether each word stands in a run of three counted words or more (section 3.9), each read with the
// pieces cut from it. A word that isn't counted and a runBreak before a word end a run, and so do a slash a word holds
// and closing marks after a counted word, which stays in it.
func (tables *letterTables) capitalRuns(parts []part, breaks map[int]bool) []bool {
	runs := make([]bool, len(parts))
	var run []int
	closeRun := func() {
		if len(run) >= 3 {
			for _, index := range run {
				runs[index] = true
			}
		}
		run = run[:0]
	}
	for index, part := range parts {
		if part.spaces == cutSpace {
			continue
		}
		rest := ""
		for later := index + 1; later < len(parts) && parts[later].spaces == cutSpace; later++ {
			rest += parts[later].token
		}
		test, whole := part.token+rest, cmp.Or(part.whole, part.token)+rest
		if breaks[index] || !tables.counted(test) {
			closeRun()
		}
		if tables.counted(test) {
			run = append(run, index)
		}
		if bare := strings.TrimRight(whole, keptSpace); bare != "" && strings.IndexByte(")]!?:;,.\"", bare[len(bare)-1]) >= 0 ||
			holdsSlash(part.token) {
			closeRun()
		}
	}
	closeRun()
	return runs
}

func vowel(letter byte) bool {
	return strings.IndexByte("AEIOU", letter) >= 0
}

// consonant is whether a character is a capital other than a vowel or Y: a [ in a word of capitals is none (section
// 3.9).
func consonant(letter byte) bool {
	return upper(letter) && !vowel(letter) && letter != 'Y'
}

// spelled is a word in capitals letter by letter with dashes between them: an I that starts it is eye, if start is
// set, and an A that ends it Aye, if end is.
func spelled(word string, start, end bool) string {
	letters := make([]string, len(word))
	for index := range len(word) {
		switch {
		case start && index == 0 && word[index] == 'I':
			letters[index] = "eye"
		case end && index == len(word)-1 && word[index] == 'A':
			letters[index] = "Aye"
		default:
			letters[index] = word[index : index+1]
		}
	}
	return strings.Join(letters, "-")
}

// keeps is whether a word in capitals of three letters is said as a word (section 3.9), bare as spelledEntry says.
func (tables *letterTables) keeps(word string, bare bool) bool {
	first, middle := word[0], word[1]
	switch {
	case tables.unkept[word]:
		return false
	case tables.kept[word]:
		return true
	}
	switch word[:2] {
	case "CE", "CI", "GE", "GI", "NY", "OS", "UC", "UK":
		return false
	case "XO", "XU":
		if word != "XUS" {
			return false
		}
	case "US":
		if word != "USH" && word != "USE" {
			return false
		}
	}
	if first == 'Q' && strings.IndexByte("AEIO", middle) >= 0 {
		return false
	}
	switch {
	case word[:2] == "LA", word[1:] == "LA", word[1:] == "NY", word[1:] == "US",
		word[1:] == "QU" && !vowel(first) && first != 'Q', word == "GNU", word == "EDU", word == "USE":
		return true
	}
	return tables.saidPattern(word, bare)
}

// saidPattern is whether three capitals are said as a word by their pattern (section 3.9), bare as spelledEntry says.
// A [ first is said only before a vowel or Y and a Q.
func (tables *letterTables) saidPattern(word string, bare bool) bool {
	first, middle, last := word[0], word[1], word[2]
	finalS := bare && last == 'S'
	wAfterIOrU := last == 'W' && (middle == 'I' || middle == 'U')
	switch letterKinds(word) {
	case "XVC", "XYC":
		return last == 'Q'
	case "CVC", "YVC":
		return !finalS && !wAfterIOrU
	case "CYC", "YYC":
		return !finalS && last != 'W' && last != 'Y'
	case "CVY", "CYV", "YVY", "YYV":
		return true
	case "CVV", "YVV":
		return tables.lastPairs[word[1:]]
	case "CCV", "CCY":
		return tables.onsets[word[:2]]
	case "VCC", "VYC", "YCC":
		vowels, coda := tables.codas[word[1:]]
		return last == 'Q' && middle != 'Q' && !(middle == 'W' && (first == 'I' || first == 'U')) ||
			!bare && word[1:] == "QS" || coda && strings.IndexByte(vowels, first) >= 0
	case "VVC", "VVY":
		return tables.firstPairs[word[:2]] && !finalS && !wAfterIOrU
	case "VVV":
		return tables.firstPairs[word[:2]] && tables.lastPairs[word[1:]]
	}
	return false
}

// written is how a word in capitals of two to four letters is written (section 3.9), run being whether it stands in
// a run of words in capitals and bare and marked as spelledEntry says. In a run a respelled word is respelled only
// when it is said in part, with small letters: CITR, respelled CIT-R, stays CITR.
func (tables *letterTables) written(word string, run, bare, marked bool) string {
	if respelled, ok := tables.respelled[word]; ok && (!run || hasSmall(respelled)) {
		return respelled
	}
	switch {
	case tables.spelled[word] || marked && tables.markedSpelled[word]:
		return spelled(word, true, true)
	case run:
		return word
	case len(word) == 2 && tables.twoLetters[word]:
		return word
	case len(word) == 2:
		return spelled(word, true, true)
	case len(word) == 4:
		return tables.fourLetters(word)
	case word[:2] == "QU" && strings.IndexByte("AEIOUSW", word[2]) < 0:
		return "KEW" + word[2:]
	case tables.keeps(word, bare):
		return word
	case word[1:] == "PC":
		return spelled(word[:1], true, false) + "-PC"
	case tables.twoLetters[word[:2]] && vowel(word[0]) && !vowel(word[1]) && consonant(word[2]) &&
		(word[2] != 'S' || !bare) && !tables.onsets[word[1:]]:
		return word[:2] + "-" + word[2:]
	}
	return spelled(word, true, true)
}

// spelledEntry is a word whose word in capitals of two to four letters the letter rules change, or whose word in
// capitals is abbreviated, with a plain list word's marks around it, as a plain list word whose expansion is what
// they write, the word itself when it is longer (section 3.9). A function word with the marks that tilded reads and a
// word that lowered changes are none. The [ after the first of the opening brackets belong to the word, letters of no
// kind: [[AB is [-A-B, the first [ going as settledSquare says. So do the marks after the capitals before the longest
// closing marks that signedClosing takes, but a dot: AB~ is A-B-~, AB)". is A-B-)".; they are written before the rest
// of the closing marks (expanded). The word is bare when no quote or ~ stands before it and its closing marks are an
// abbreviation's, a quote perhaps at their end; marked when the marks before it have a ' or ~, or two of [ and quotes.
func (address *address) spelledEntry(token string, run bool) (datapack.Word, bool) {
	opening, word, closing := parted(token)
	if squares := strings.TrimLeft(strings.TrimPrefix(opening, ">"), "("); len(squares) >= 2 &&
		strings.Trim(squares, "[") == "" {
		opening, word = opening[:len(opening)-len(squares)+1], squares[1:]+word
	}
	signs := 0
	for !signedClosing(closing[signs:]) {
		signs++
	}
	letters := strings.TrimLeft(word, "[")
	abbreviated := signs == 0 && abbreviatedWord(word)
	if len(letters) < 2 || len(word)+signs > 4 && !abbreviated || !capitalLetters(letters) ||
		address.letters.lowered(token) != "" || address.functionWord(word) && tildeMarks(opening, closing) ||
		!listOpening(opening, true) || strings.Contains(closing[:signs], ".") {
		return datapack.Word{}, false
	}
	word, closing = word+closing[:signs], closing[signs:]
	bare := !strings.ContainsAny(opening, "\"'~") &&
		strings.Trim(strings.TrimSuffix(closing, "\""), ")]!?:;,."+keptSpace) == ""
	marked := strings.ContainsAny(opening, "'~") || strings.Count(opening, "[")+strings.Count(opening, "\"") >= 2
	written := word
	if len(word) <= 4 {
		written = address.letters.written(word, run, bare, marked)
	}
	if written == word && !abbreviated {
		return datapack.Word{}, false
	}
	return datapack.Word{Word: word, Expansion: written, Kind: datapack.PlainWord}, true
}

// signedClosing is whether marks after a word in capitals and the marks spelled with it are closing marks a plain
// list word may stand before: a quote or a star and then anything, or brackets, ! ? : ; commas, dots and kept spaces,
// a quote at their end.
func signedClosing(closing string) bool {
	if closing != "" && strings.IndexByte("\"'*", closing[0]) >= 0 {
		return true
	}
	return strings.Trim(strings.TrimSuffix(closing, "\""), ")]!?:;,."+keptSpace) == ""
}

// abbreviatedWord is whether a word in capitals is spaced as a plain list word even when the letter rules keep it
// (section 3.9): one that starts with LA or ends in LA, NY or US.
func abbreviatedWord(word string) bool {
	return strings.HasPrefix(word, "LA") || strings.HasSuffix(word, "LA") || strings.HasSuffix(word, "NY") ||
		strings.HasSuffix(word, "US")
}

// reading is a word whose word in capitals is one of the readings, with an abbreviation's marks around it, as an
// abbreviation whose expansion is the reading (section 3.9); none for a word that lowered changes (XMAS, is x MAS,).
func (address *address) reading(token string) (datapack.Word, bool) {
	opening, word, closing := parted(token)
	reading, ok := address.letters.readings[word]
	if !ok || !listOpening(opening, false) || !listClosing(closing) || address.letters.lowered(token) != "" {
		return datapack.Word{}, false
	}
	return datapack.Word{Word: word, Expansion: reading, Kind: datapack.AbbreviationWord}, true
}

// sentenceStarting is whether a word, as lowered leaves it, starts a sentence after a dot (section 3.9): after (, [
// and quotes, one that starts with a capital or is a number; one of closing marks, ! ? and > only; and none.
func (tables *letterTables) sentenceStarting(following string) bool {
	word := strings.TrimLeft(cmp.Or(tables.lowered(following), following), "([\"")
	return word != "" && upper(word[0]) || raisingNumber(word) || strings.Trim(word, ")]!?:;,.>") == ""
}
