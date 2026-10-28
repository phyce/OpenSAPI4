package preformatter

import (
	"cmp"
	"slices"
	"strings"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
)

// letterTables is the address context's tables for words in capitals of two letters or more (section 3.9).
type letterTables struct {
	twoLetters, onsets, firstPairs, lastPairs, spelled, markedSpelled, unkept, kept map[string]bool
	capitalsOnly, heads, tails, openHeads, vowelCodas, codaPairs                    map[string]bool
	codas, respelled, unkeptHeads, clusters, readings                               map[string]string
	pieces, longHeads, longTails, unsplit                                           []string
}

func newLetterTables(letters *datapack.Letters) letterTables {
	return letterTables{twoLetters: wordSet(letters.TwoLetters), onsets: wordSet(letters.Onsets),
		firstPairs: wordSet(letters.FirstPairs), lastPairs: wordSet(letters.LastPairs),
		spelled: wordSet(letters.Spelled), markedSpelled: wordSet(letters.MarkedSpelled),
		unkept: wordSet(letters.Unkept), kept: wordSet(letters.Kept), capitalsOnly: wordSet(letters.CapitalsOnly),
		heads: wordSet(letters.Heads),
		tails: wordSet(letters.Tails), openHeads: wordSet(letters.OpenHeads), vowelCodas: wordSet(letters.VowelCodas),
		codaPairs: wordSet(letters.CodaPairs), codas: letters.Codas, respelled: letters.Respelled,
		unkeptHeads: letters.UnkeptHeads, clusters: letters.Clusters, readings: letters.Readings,
		pieces: letters.Pieces, longHeads: letters.LongHeads, longTails: letters.LongTails, unsplit: letters.Unsplit}
}

// lowered is a word of two letters or more, in capitals or not, with no opening marks, before nothing or a character
// that isn't a letter and then anything, that starts with Q, X or Z before a consonant (not ZW, and not of Roman
// numerals' letters only, in either case), with that letter small and a space after it (section 3.9): Xbox, xbox, xBOX
// and QsmBS are x box, x box, x BOX and q smBS. It is "" for any other word and a respelled one in any case without
// closing marks, which is read or respelled and counts as capitals beside other words. A word is tested by its first
// piece as its commas space it, the rest of it written after that piece: XB,dog is x B,dog and XB~dog x B~dog. The
// Roman numerals' test reads the word in capitals with the digits, the characters from @ to ~ and those from 128 up of
// the rest of the piece: XCC is X-C-C and XCC$ X-C-C-$, but XCC~ and XCC5 are x CC~ and x CC5.
func (tables *letterTables) lowered(token string) string {
	piece, _, _ := strings.Cut(wordCommas(token, false), " ")
	letters := 0
	for letters < len(piece) && alphabetic(piece[letters]) {
		letters++
	}
	word, closing := strings.ToUpper(piece[:letters]), piece[letters:]
	_, respelled := tables.respelled[word]
	if len(word) < 2 || strings.IndexByte("QXZ", word[0]) < 0 || !consonant(word[1]) || word[:2] == "ZW" ||
		strings.Trim(romanTested(word, closing), "CDILMVX") == "" || respelled && closing == "" {
		return ""
	}
	return lower(token[:1]) + " " + token[1:]
}

// romanTested is a word in capitals with the characters of the rest of its piece that the Roman numerals' test reads:
// digits, the characters from @ to ~ and those from 128 up.
func romanTested(word, rest string) string {
	tested := []byte(word)
	for index := range len(rest) {
		if character := rest[index]; digit(character) || character >= '@' && character != 0x7f {
			tested = append(tested, character)
		}
	}
	return string(tested)
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

// unbroken is a paragraph's words without the runBreaks and openingGones among them, which words a runBreak stood
// before and which an openingGone did. A word after an openingGone has the spaces before the openingGone in place of
// its first space.
func unbroken(parts []part) ([]part, map[int]bool, map[int]bool) {
	kept := parts[:0]
	breaks, opened := map[int]bool{}, map[int]bool{}
	before, carried := "", false
	for _, part := range parts {
		switch part.token {
		case runBreak:
			breaks[len(kept)] = true
			continue
		case openingGone:
			opened[len(kept)] = true
			before, carried = part.spaces, true
			continue
		}
		if carried && strings.HasPrefix(part.spaces, " ") {
			part.spaces = before + part.spaces[1:]
		}
		carried = false
		kept = append(kept, part)
	}
	return kept, breaks, opened
}

// runLetter is a single letter of a run (section 3.10): the ( before it, if one does, the letter and the closing marks
// after it.
type runLetter struct {
	opening string
	letter  byte
	closing string
}

// readRunLetter reads a word as a single letter of a run: a capital or small letter, a > and a ( perhaps before it and
// closing marks after it.
func readRunLetter(token string) (runLetter, bool) {
	rest := strings.TrimPrefix(token, ">")
	opening := ""
	if strings.HasPrefix(rest, "(") {
		opening, rest = "(", rest[1:]
	}
	if rest == "" || !alphabetic(rest[0]) || !closingOnly(rest[1:]) {
		return runLetter{}, false
	}
	return runLetter{opening, rest[0], rest[1:]}, true
}

// closingOnly is whether marks are closing marks only: brackets, ! ? : ; commas and dots, a quote perhaps at their end.
func closingOnly(marks string) bool {
	return strings.Trim(strings.TrimSuffix(marks, "\""), ")]!?:;,.") == ""
}

// oneCharacter is whether a word is one character, a > not before a digit and a ( perhaps before it and closing marks
// after it.
func oneCharacter(token string) bool {
	candidates := []string{token}
	if strings.HasPrefix(token, ">") && (len(token) == 1 || !digit(token[1])) {
		candidates = append(candidates, token[1:])
	}
	for _, candidate := range candidates {
		if strings.HasPrefix(candidate, "(") && len(candidate) >= 2 && closingOnly(candidate[2:]) ||
			candidate != "" && closingOnly(candidate[1:]) {
			return true
		}
	}
	return false
}

// letterRuns writes each run of three single letters or more as one word (section 3.10). A letter of a run is a word
// of its own that readRunLetter reads, with no cut space before or after it, not cut by a closing run, and not before
// a runBreak or a word of ! and ? only, nor, with closing marks, before a word that is oneCharacter. Closing marks end
// a run, and so does an openingGone before the next letter, which may start one. The letters before the last are
// written as their (, and the last as its (, a ~, the run's letters and its closing marks, and is lettered. A word
// after letters that went is dropped: two spaces stand before it and it starts a new run of words in capitals; after
// a single letter that went at the paragraph's start it has one space and isn't dropped, but still starts a new run.
// Each word keeps its index before the letters were joined as its slot. It gives the words, the runBreaks and the
// openingGones as they then stand, and the indexes the letters of runs had.
func letterRuns(parts []part, breaks, opened map[int]bool) ([]part, map[int]bool, map[int]bool, map[int]bool) {
	letterAt := func(index int) (runLetter, bool) {
		part := parts[index]
		next := index+1 < len(parts)
		if part.spaces == cutSpace || part.whole != "" || breaks[index+1] ||
			next && (parts[index+1].spaces == cutSpace || terminal(parts[index+1].token)) {
			return runLetter{}, false
		}
		letter, ok := readRunLetter(part.token)
		if ok && letter.closing != "" && next && oneCharacter(parts[index+1].token) {
			return runLetter{}, false
		}
		return letter, ok
	}
	written := map[int]string{}
	for index := 0; index < len(parts); {
		start := index
		var run []runLetter
		for index < len(parts) {
			letter, ok := letterAt(index)
			if !ok {
				break
			}
			run = append(run, letter)
			index++
			if letter.closing != "" || opened[index] {
				break
			}
		}
		if len(run) == 0 {
			index++
			continue
		}
		if len(run) < 3 {
			continue
		}
		letters := make([]byte, len(run))
		for position, letter := range run {
			written[start+position] = letter.opening
			letters[position] = letter.letter
		}
		last := run[len(run)-1]
		written[start+len(run)-1] = last.opening + "~" + string(letters) + last.closing
	}
	kept := parts[:0]
	keptBreaks, keptOpened := map[int]bool{}, map[int]bool{}
	dropped := 0
	for index, part := range parts {
		if breaks[index] {
			keptBreaks[len(kept)] = true
		}
		if opened[index] {
			keptOpened[len(kept)] = true
		}
		if text, ok := written[index]; ok {
			part.token, part.lettered = text, strings.Contains(text, "~")
		}
		if part.token == "" {
			dropped++
			continue
		}
		if dropped > 0 {
			keptBreaks[len(kept)] = true
			part.spaces, part.dropped = " ", dropped > 1 || len(kept) > 0
		}
		part.slot = index
		kept = append(kept, part)
		dropped = 0
	}
	if breaks[len(parts)] {
		keptBreaks[len(kept)] = true
	}
	inRuns := map[int]bool{}
	for index := range written {
		inRuns[index] = true
	}
	return kept, keptBreaks, keptOpened, inRuns
}

// ayeAfter is whether a word makes a capital A after it read as aye (section 3.10): one without a small letter, read
// lowered, with something between its ayeOpening and its closing marks, but not a number between quotes and brackets
// and before closing marks, nor a + or - before ! and ? only; or one that starts with a capital after its ayeOpening
// and has no other capital.
func (tables *letterTables) ayeAfter(token string) bool {
	loud := cmp.Or(tables.lowered(token), token)
	if !hasSmall(loud) && !closingOnly(loud[ayeOpening(loud):]) && !markedNumber(loud) &&
		!(len(loud) >= 2 && (loud[0] == '+' || loud[0] == '-') && strings.Trim(loud[1:], "!?") == "") {
		return true
	}
	word := token[ayeOpening(token):]
	return word != "" && upper(word[0]) && !strings.ContainsFunc(word[1:], func(character rune) bool {
		return character >= 'A' && character <= 'Z'
	})
}

// ayeOpening is how long a word's opening marks are for ayeAfter: a > and ( perhaps, then a [ and the ( after it, or a
// quote.
func ayeOpening(token string) int {
	index := 0
	if strings.HasPrefix(token, ">") {
		index++
	}
	for index < len(token) && token[index] == '(' {
		index++
	}
	switch {
	case index < len(token) && token[index] == '[':
		index++
		for index < len(token) && token[index] == '(' {
			index++
		}
	case index < len(token) && token[index] == '"':
		index++
	}
	return index
}

// markedNumber is whether a word is a number, dashes among its digits, between quotes and brackets and before closing
// marks.
func markedNumber(token string) bool {
	number := strings.TrimPrefix(strings.TrimLeft(token, "\"(["), ".")
	return number != "" && digit(number[0]) &&
		strings.Trim(strings.TrimLeft(number, "0123456789.,-"), ")]!?:;,.\"") == ""
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

// consonant is whether a character is a letter other than a vowel or Y, small or capital: a [ or a sign in a word of
// capitals is none, and so is a letter with an accent (section 3.9).
func consonant(letter byte) bool {
	return alphabetic(letter) && !vocal(letter)
}

// upperCase is a small letter's capital, and any other character itself.
func upperCase(character byte) byte {
	if small(character) {
		return character - 'a' + 'A'
	}
	return character
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

// keeps is whether a word in capitals of three letters is said as a word (section 3.9), bare and exact as
// capitalsWritten says. The starts NY, UC, UK, US and LA count only in capitals (section 3.12: nYV, uCH and uSP are
// said as words). Ended is whether the capitals end the word: before small letters the ends LA, NY and US don't
// count, XUS included (section 3.11: ONYx is O-N-Y-x, XUSx X-U-S-x).
func (tables *letterTables) keeps(word string, bare bool, exact string, ended bool) bool {
	first, middle := word[0], word[1]
	switch {
	case tables.unkept[word]:
		return false
	case tables.listed(tables.kept, word, exact):
		return true
	}
	switch word[:2] {
	case "CE", "CI", "GE", "GI", "OS":
		return false
	case "XO", "XU":
		if word != "XUS" || !ended {
			return false
		}
	}
	switch exact[:2] {
	case "NY", "UC", "UK":
		return false
	case "US":
		if word != "USH" && word != "USE" {
			return false
		}
	}
	if first == 'Q' && strings.IndexByte("AEIO", middle) >= 0 {
		return false
	}
	switch {
	case exact[:2] == "LA", ended && (word[1:] == "LA" || word[1:] == "NY" || word[1:] == "US"),
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

// written is how a word in capitals of two letters or more, a case piece or one with a small head, is written, run
// being whether it stands in a run of words in capitals and bare and marked as spelledEntry says: one in capitals as
// capitalsWritten says, a case piece as caseWritten says (section 3.11), one with a small head as its capitals are
// written outside a run, in the word's own case as recased says (section 3.12): bCD is b-C-D, iBC i-B-C, and in a run
// too, BCD bCD BCD being BCD b-C-D BCD. A case piece with signs after it, and the letters after them, is written so
// too when its capitals come first or caseWhole says so of its capitals with the signs (section 3.11: DOg~ is DOg-~,
// ABc~ A-B-c-~, USe~~ U-S-e~~, EDu~dog EDu-~dog, UsER~ UsER-~), and is its front, a dash and the rest as it is
// otherwise (BcDF~ is Bc-DF~, McDO~ Mc-DO~, IoNE~ Io-NE~).
func (tables *letterTables) written(word string, run, bare, marked bool) string {
	if front, ok := casePiece(word); ok {
		return tables.caseWritten(word, front)
	}
	capitalBytes := []byte(word)
	for index, character := range capitalBytes {
		capitalBytes[index] = upperCase(character)
	}
	capitals := string(capitalBytes)
	if piece, ok := signedMixed(word); ok {
		if front, ok := casePiece(piece); ok && !upper(front[1]) && !tables.caseWhole(capitals, word) {
			return front + "-" + word[len(front):]
		}
	} else if !smallHead(word) {
		return tables.capitalsWritten(word, run, bare, marked, word)
	}
	return recased(word, tables.capitalsWritten(capitals, false, bare, marked, word))
}

// signedMixed is whether a word is a case piece or one with a small head and then signs, perhaps letters after them,
// with no quote, ' or * (section 3.11), and that case piece or word.
func signedMixed(word string) (string, bool) {
	end := 0
	for end < len(word) && alphabetic(word[end]) {
		end++
	}
	piece := word[:end]
	if end == len(word) || strings.ContainsAny(word[end:], "\"'*") {
		return "", false
	}
	if _, ok := casePiece(piece); ok || smallHead(piece) && letterWord(piece) {
		return piece, true
	}
	return "", false
}

// smallHead is whether a word starts with small letters and then two capitals (section 3.12).
func smallHead(word string) bool {
	head := 0
	for head < len(word) && small(word[head]) {
		head++
	}
	return head > 0 && head+2 <= len(word) && upper(word[head]) && upper(word[head+1])
}

// letterWord is whether a word is letters, small or capital, one or more.
func letterWord(word string) bool {
	return word != "" && strings.Trim(word, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz") == ""
}

// recased is how the capitals of a word with a small head or a case piece were written (sections 3.11 and 3.12), in
// the word's own case: each capital is the word's next letter as the word has it, eye is i for a small i, KEW for the
// word's next two letters Q and U stays KEW (bqUC is bKEWC), ed-yoo and eye-oh stand for the word's next three and two
// letters (EDUcation is ed-yoo-cation), triple and the letter after it for the word's next three letters, the first
// as the word has it when they start the word and the last otherwise (aAAH is triple-a-H, baAA b-triple-Aye), and the
// rest stays as it is. Writing without capitals stays whole: aSAP is aye-sap and iSA eye-sa.
func recased(word, written string) string {
	if !hasCapital(written) {
		return written
	}
	var letters []byte
	for index := 0; index < len(word); index++ {
		if alphabetic(word[index]) {
			letters = append(letters, word[index])
		}
	}
	var output strings.Builder
	for position := 0; position < len(written); {
		switch {
		case strings.HasPrefix(written[position:], "ed-yoo"):
			output.WriteString("ed-yoo")
			letters = letters[3:]
			position += 6
		case strings.HasPrefix(written[position:], "eye-oh"):
			output.WriteString("eye-oh")
			letters = letters[2:]
			position += 6
		case strings.HasPrefix(written[position:], "eye"):
			if letters[0] == 'I' {
				output.WriteString("eye")
			} else {
				output.WriteByte('i')
			}
			letters = letters[1:]
			position += 3
		case strings.HasPrefix(written[position:], "KEW") && len(letters) >= 2 && upperCase(letters[0]) == 'Q' &&
			upperCase(letters[1]) == 'U':
			output.WriteString("KEW")
			letters = letters[2:]
			position += 3
		case strings.HasPrefix(written[position:], "triple-") && position == 0:
			output.WriteString("triple-")
			output.WriteByte(letters[0])
			letters = letters[3:]
			position += 8
		case strings.HasPrefix(written[position:], "triple-"):
			output.WriteString("triple-")
			output.WriteByte(letters[2])
			letters = letters[3:]
			position += 8
		case upper(written[position]):
			output.WriteByte(letters[0])
			letters = letters[1:]
			position++
		default:
			output.WriteByte(written[position])
			position++
		}
	}
	return output.String()
}

// capitalsWritten is how a word in capitals of two letters or more is written (section 3.9), run, bare and marked as
// written says and exact being the word as written: a word with a small head is written by its capitals, but listed
// reads the spelled and kept words and the starts that count only in capitals are read from exact (section 3.12). A
// marked-spelled word is spelled when marked or with a small head (cYA is c-Y-Aye). In a run a respelled word is
// respelled only when it is said in part, with small letters, or has five letters or more: CITR, respelled CIT-R,
// stays CITR, TROFF is T-ROFF. Any other in a run stays as it is but one that placed says is written as outside one.
func (tables *letterTables) capitalsWritten(word string, run, bare, marked bool, exact string) string {
	if respelled, ok := tables.respelled[word]; ok && (!run || len(word) >= 5 || hasSmall(respelled)) {
		return respelled
	}
	switch {
	case tables.listed(tables.spelled, word, exact) || (marked || exact != word) && tables.markedSpelled[word]:
		return spelled(word, true, true)
	case run && !placed(word):
		return word
	case len(word) == 2 && tables.twoLetters[word]:
		return word
	case len(word) == 2:
		return spelled(word, true, true)
	case len(word) == 4:
		return tables.fourLetters(word, exact)
	case len(word) >= 5:
		return tables.longLetters(word, exact)
	case !alphabetic(word[2]) && tables.signedSaid(word[:2]) && word[:2] != "XO" && word[:2] != "XU":
		return word
	case word[:2] == "QU" && strings.IndexByte("AEIOUSW", word[2]) < 0:
		return "KEW" + word[2:]
	case tables.keeps(word, bare, exact, true):
		return word
	case word[1:] == "PC":
		return spelled(word[:1], true, false) + "-PC"
	case tables.twoLetterFirst(word, bare):
		return word[:2] + "-" + word[2:]
	}
	return spelled(word, true, true)
}

// twoLetterFirst is whether a word in capitals of three letters is its first two letters and the third (section
// 3.9), bare as spelledEntry says: the first two are a two-letter word that starts with a vowel and ends in a
// consonant, before a consonant other than S (any consonant when not bare) or a sign (AD-~), and the last two aren't
// an onset.
func (tables *letterTables) twoLetterFirst(word string, bare bool) bool {
	return tables.twoLetters[word[:2]] && vowel(word[0]) && !vowel(word[1]) &&
		(consonant(word[2]) || !alphabetic(word[2])) && (word[2] != 'S' || !bare) && !tables.onsets[word[1:]]
}

// listed is whether a word in capitals, written as exact, is one of a table's: a capitals-only one only when it is
// written in capitals (section 3.12: mIT stays as it is, as if MIT weren't spelled).
func (tables *letterTables) listed(table map[string]bool, word, exact string) bool {
	return table[word] && (exact == word || !tables.capitalsOnly[word])
}

// placed is whether a word in capitals in a run is written as outside one (section 3.9): one that starts with NY, LA,
// UK, or US but before E, H or U (BCD BCD NYAB is NY-AB, USB is U-S-B).
func placed(word string) bool {
	switch {
	case strings.HasPrefix(word, "NY"), strings.HasPrefix(word, "LA"), strings.HasPrefix(word, "UK"):
		return true
	}
	return strings.HasPrefix(word, "US") && (len(word) == 2 || !oneOf(word[2], "EHU"))
}

// spelledEntry is a word whose word in capitals of two letters or more the letter rules change, or whose word in
// capitals is abbreviated, with a plain list word's marks around it, as a plain list word whose expansion is what they
// write (section 3.9), and the closing marks written after it. A case piece is one too (section 3.11: DOg is DO-g, DOg~
// DOg-~). A function word with the marks that tilded reads and a word that lowered changes are none. The [ after the
// first of the opening brackets belong to the word, letters of no kind: [[AB is [-A-B, the first [ going as
// settledSquare says. So do the marks after the capitals before the longest closing marks that signedClosing takes,
// letters of no kind too: AB~ is A-B-~, AB)". is A-B-)". and USKN~ is U-S-KN~. A word that starts with five capitals or
// more also takes any letters after its marks (EDULC~dog is ed-yoo-LC~dog) and its marks may hold a dot (LASRB.~ is
// L-A-S-R-B-.-~); a shorter one with a dot in them is none. So does a case piece or a word with a small head, of any
// length, but not after a quote, ' or * (USe~dog is U-S-e~dog, LAn~.~ L-A-n-~-.-~, iOS~dog iO-S~dog). A digit ends the
// word and its marks, and it and what follows are written after it as they are, a quote, ' or * before the digit going
// with it: BCD!5 is B-C-D-!5, BCD"5 is B-C-D"5. The word is bare when no quote or ~ stands before it and its closing
// marks are an abbreviation's, a quote perhaps at their end; marked when the marks before it have a ' or ~, or two of [
// and quotes, but not when it is a run of single letters (lettered, section 3.10: S O S is ~SOS, ~SOS is ~S-O-S).
func (address *address) spelledEntry(token string, run, lettered bool) (datapack.Word, string, bool) {
	opening, word, closing := parted(token)
	if squares := strings.TrimLeft(strings.TrimPrefix(opening, ">"), "("); len(squares) >= 2 &&
		strings.Trim(squares, "[") == "" {
		opening, word = opening[:len(opening)-len(squares)+1], squares[1:]+word
	}
	if cut := strings.IndexAny(word, "0123456789"); cut >= 0 {
		stem := word[:cut]
		for stem != "" && !alphabetic(stem[len(stem)-1]) {
			stem = stem[:len(stem)-1]
		}
		word, closing = stem, word[len(stem):]+closing
	}
	signs := 0
	for !signedClosing(closing[signs:]) && !digit(closing[signs]) {
		signs++
	}
	letters := strings.TrimLeft(word, "[")
	abbreviated := abbreviatedWord(word + closing[:signs])
	long := len(letters) >= 5 && capitalLetters(letters[:5])
	_, piece := casePiece(word)
	_, signedPiece := signedMixed(word)
	if !(len(letters) >= 2 && capitalLetters(letters) || long || smallHead(word) && letterWord(word) || piece ||
		signedPiece) ||
		address.letters.lowered(token) != "" || address.functionWord(word) && tildeMarks(opening, closing) ||
		!listOpening(opening, true) ||
		strings.Contains(closing[:signs], ".") && !long && !hasSmall(word) {
		return datapack.Word{}, "", false
	}
	word, closing = word+closing[:signs], closing[signs:]
	bare := !strings.ContainsAny(opening, "\"'~") &&
		strings.Trim(strings.TrimSuffix(closing, "\""), ")]!?:;,."+keptSpace) == ""
	marked := !lettered &&
		(strings.ContainsAny(opening, "'~") || strings.Count(opening, "[")+strings.Count(opening, "\"") >= 2)
	written := address.letters.written(word, run, bare, marked)
	if written == word && !abbreviated {
		return datapack.Word{}, "", false
	}
	return datapack.Word{Word: word, Expansion: written, Kind: datapack.PlainWord}, closing, true
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
// (section 3.9): one that starts with LA or ends in LA, NY or US, the [ and marks spelledEntry gives it included.
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

// casePieces is a word's pieces at its changes of case (section 3.11): a capital after a small letter and before
// anything but a capital starts a piece, but not in a piece that starts with Mc and a capital; a small letter before
// two capitals ends the splitting. A word that starts with a digit, or with a small letter and Hz, isn't split.
func casePieces(word string) []string {
	if word != "" && digit(word[0]) || len(word) >= 3 && small(word[0]) && word[1:3] == "Hz" {
		return []string{word}
	}
	var pieces []string
	start := 0
	for index := 1; index < len(word); index++ {
		if !small(word[index-1]) || !upper(word[index]) {
			continue
		}
		mc := strings.HasPrefix(word[start:], "Mc") && start+2 < len(word) && upper(word[start+2])
		if index+1 < len(word) && upper(word[index+1]) || mc {
			break
		}
		pieces = append(pieces, word[start:index])
		start = index
	}
	return append(pieces, word[start:])
}

// caseSplit is a paragraph with its words split at their changes of case (section 3.11), a word ending at a space,
// comma, +, /, # or dash. After a word so split, a comma drops what follows it up to one of those but a comma; before
// a + (that comma and what it drops perhaps between), each piece but the last of two characters or more is followed by
// a space and its last character again. A piece but the last that lowered changes is its first letter small, a
// loweredSpace and the rest of the word from its second letter on, through commas and #, up to a space, +, / or dash:
// qxZ is q xZ Z, qxZbXc q xZbXc z bXc Xc and xbAb,dog x bAb,dog Ab,.
func (tables *letterTables) caseSplit(paragraph string) string {
	var parts []string
	start := 0
	for index := 0; index < len(paragraph); index++ {
		if strings.IndexByte(" ,+/#-", paragraph[index]) >= 0 {
			parts = append(parts, paragraph[start:index], paragraph[index:index+1])
			start = index + 1
		}
	}
	parts = append(parts, paragraph[start:])
	for index := 0; index < len(parts); index += 2 {
		pieces := casePieces(parts[index])
		if len(pieces) == 1 {
			continue
		}
		rest := parts[index]
		for later := index + 1; later+1 < len(parts) && (parts[later] == "," || parts[later] == "#"); later += 2 {
			rest += parts[later] + parts[later+1]
		}
		following := ""
		if index+1 < len(parts) {
			following = parts[index+1]
		}
		if following == "," {
			later := index + 2
			parts[later] = ""
			for later+1 < len(parts) && parts[later+1] == "," {
				parts[later+1], parts[later+2] = "", ""
				later += 2
			}
			following = ""
			if later+1 < len(parts) {
				following = parts[later+1]
			}
		}
		written := make([]string, len(pieces))
		start := 0
		for position, piece := range pieces {
			written[position] = piece
			if position < len(pieces)-1 && tables.lowered(piece) != "" {
				written[position] = lower(piece[:1]) + loweredSpace + rest[start+1:]
			}
			if following == "+" && position < len(pieces)-1 && len(piece) > 1 {
				written[position] += " " + piece[len(piece)-1:]
			}
			start += len(piece)
		}
		parts[index] = strings.Join(written, " ")
	}
	return strings.Join(parts, "")
}

// casePiece is whether a word is a case piece (section 3.11), and its front: two capitals or more and then small
// letters (the capitals), a capital and small letters before two capitals or more and any letters (the capital and
// small letters), or Mc, a capital, small letters perhaps, a capital and any letters (Mc).
func casePiece(word string) (string, bool) {
	if !letterWord(word) || !upper(word[0]) {
		return "", false
	}
	capitals := 1
	for capitals < len(word) && upper(word[capitals]) {
		capitals++
	}
	if capitals >= 2 {
		if capitals == len(word) || hasCapital(word[capitals:]) {
			return "", false
		}
		return word[:capitals], true
	}
	smalls := capitals
	for smalls < len(word) && small(word[smalls]) {
		smalls++
	}
	if smalls+2 <= len(word) && upper(word[smalls]) && upper(word[smalls+1]) {
		return word[:smalls], true
	}
	if !strings.HasPrefix(word, "Mc") || len(word) < 4 || !upper(word[2]) {
		return "", false
	}
	next := 3
	for next < len(word) && small(word[next]) {
		next++
	}
	return "Mc", next < len(word)
}

// frontWritten is how the front of a case piece of four letters or fewer is written, in capitals (section 3.11), read
// as the start of a longer word: two letters said when they are a two-letter word and spelled otherwise; three
// letters QU before a consonant other than S and W as KEW and that letter, said when keeps says so for capitals that
// don't end the word (ONYx is O-N-Y-x), the first two and the third when twoLetterFirst says so, spelled otherwise.
// Spelling ends in A, not Aye: BCAt is B-C-A-t, GEt G-E-t, GETx GET-x and ADBc AD-B-c.
func (tables *letterTables) frontWritten(front string) string {
	switch {
	case len(front) == 2 && tables.twoLetters[front]:
		return front
	case len(front) == 2:
		return spelled(front, true, false)
	case front[:2] == "QU" && strings.IndexByte("AEIOUSW", front[2]) < 0:
		return "KEW" + front[2:]
	case tables.keeps(front, false, front, false):
		return front
	case tables.twoLetterFirst(front, false):
		return front[:2] + "-" + front[2:]
	}
	return spelled(front, true, false)
}

// caseWhole is whether a case piece in capitals (word), written as exact, is written whole (section 3.11): its capitals
// read as a word in capitals and recased, not its front, a dash and the rest. One of three letters when it starts with
// LA or is EDU (LAn, EDu; USe is U-S-e). One of four letters when it is a piece (UsER); said as said says, when it
// starts with LA or ends in LA, NY or US (LAte, McLA; SCAm is SCA-m, BONe BON-e); otherwise not IO before N but IONE
// (IoNA is eye-o-NA, IoNE eye-oNE), US before H, before U and a tail tailSaid says, or before ED or ES (USHx is USH-x,
// USEd USE-d, UsED U-s-ED), but any that starts with EDU, USE, IO, OS or PC, with NY, UC, UK, US or LA in capitals,
// with a long head or ends in a long tail (BCAm is B-CAm, BCAt B-C-A-t). One of five letters or more when it is a
// piece, starts with NY, LA or UK in capitals, US in capitals but before E, H or U, IO but before N, EDU, OS or PC,
// ends in LA, NY or US in capitals, or when headed says it starts with a piece or long head or tailed says it ends in a
// piece or long tail (PkieVNY stays, USINg is U-S-INg).
func (tables *letterTables) caseWhole(word, exact string) bool {
	if len(word) == 3 {
		return exact[:2] == "LA" || word == "EDU"
	}
	if slices.Contains(tables.pieces, word) {
		return true
	}
	start, writtenStart, writtenEnd := word[:2], exact[:2], exact[len(exact)-2:]
	if len(word) == 4 {
		if tables.said(word, exact) {
			return writtenStart == "LA" || writtenEnd == "LA" || writtenEnd == "NY" || writtenEnd == "US"
		}
		if start == "IO" && word[2] == 'N' && word[3] != 'E' || start == "US" &&
			(word[2] == 'H' || word[2] == 'U' && tables.tailSaid(word) || word[2:] == "ED" || word[2:] == "ES") {
			return false
		}
		switch {
		case word[:3] == "EDU", word[:3] == "USE", start == "IO", start == "OS", start == "PC":
			return true
		}
		switch writtenStart {
		case "NY", "UC", "UK", "US", "LA":
			return true
		}
		return slices.Contains(tables.longHeads, word[:3]) || slices.Contains(tables.longTails, word[1:])
	}
	switch {
	case writtenStart == "NY", writtenStart == "LA", writtenStart == "UK",
		writtenStart == "US" && !oneOf(word[2], "EHU"),
		writtenEnd == "LA", writtenEnd == "NY", writtenEnd == "US",
		start == "IO" && word[2] != 'N', word[:3] == "EDU", start == "OS", start == "PC":
		return true
	}
	return slices.ContainsFunc(slices.Concat(tables.pieces, tables.longHeads), func(head string) bool {
		return tables.headed(word, head)
	}) || slices.ContainsFunc(slices.Concat(tables.pieces, tables.longTails), func(tail string) bool {
		return tables.tailed(word, tail)
	})
}

// caseWritten is how a case piece is written, front being its front as casePiece says (section 3.11): whole when
// caseWhole says so, its capitals as capitalsWritten says outside a run, recased (EDUcation is ed-yoo-cation, UsET
// UsE-T); otherwise its front, a dash and the rest, the front as frontWritten says, recased, when the piece has four
// letters or fewer and as it is otherwise (DOg is DO-g, MCDog MCD-og, McDOg Mc-DOg), and a rest of one a aye (CAa is
// C-A-aye).
func (tables *letterTables) caseWritten(word, front string) string {
	capitals := strings.ToUpper(word)
	if tables.caseWhole(capitals, word) {
		return recased(word, tables.capitalsWritten(capitals, false, false, false, word))
	}
	rest := word[len(front):]
	if len(word) <= 4 {
		front = recased(front, tables.frontWritten(strings.ToUpper(front)))
	}
	if rest == "a" {
		rest = "aye"
	}
	return front + "-" + rest
}
