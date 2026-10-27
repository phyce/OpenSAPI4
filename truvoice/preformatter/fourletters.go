package preformatter

import (
	"slices"
	"strings"
)

var (
	softStarts     = []string{"CE", "CI", "GE", "GI"}
	spellingStarts = []string{"CE", "CI", "GE", "GI", "IO", "OS", "PC", "QA", "QE", "QI", "QO", "XO", "XU"}
	capitalStarts  = []string{"NY", "UC", "UK", "US"}
	strictStarts   = []string{"JY", "QY", "XY", "YY"}
	unsaidCodas    = []string{"JX", "QJ", "QQ", "QX", "QY", "QZ", "ZS"}
	hardCodas      = []string{"GH", "GM", "GN", "MB", "PC", "ZS"}
)

func oneOf(letter byte, letters string) bool {
	return strings.IndexByte(letters, letter) >= 0
}

// vocal is whether a letter is a vowel or Y, small or capital.
func vocal(letter byte) bool {
	return oneOf(upperCase(letter), "AEIOUY")
}

// letterKinds is a word's letters as V for a vowel, Y for a Y, C for a consonant and X for a [.
func letterKinds(word string) string {
	kinds := make([]byte, len(word))
	for index := range len(word) {
		switch {
		case vowel(word[index]):
			kinds[index] = 'V'
		case word[index] == 'Y':
			kinds[index] = 'Y'
		case consonant(word[index]):
			kinds[index] = 'C'
		default:
			kinds[index] = 'X'
		}
	}
	return string(kinds)
}

// soundKinds is a word's letters as V for a vowel, C for a consonant and X for a [, a Y as V when a letter one or two
// before it is read as C, and as C otherwise (section 3.9).
func soundKinds(word string) string {
	kinds := make([]byte, len(word))
	for index := range len(word) {
		switch {
		case vowel(word[index]):
			kinds[index] = 'V'
		case consonant(word[index]):
			kinds[index] = 'C'
		case word[index] != 'Y':
			kinds[index] = 'X'
		case slices.Contains(kinds[max(index-2, 0):index], 'C'):
			kinds[index] = 'V'
		default:
			kinds[index] = 'C'
		}
	}
	return string(kinds)
}

// endPair is whether two letters end a word as a vowel pair: one of lastPairs, or a vowel and a Y either way round.
func (tables *letterTables) endPair(pair string) bool {
	return tables.lastPairs[pair] || vowel(pair[0]) && pair[1] == 'Y' || pair[0] == 'Y' && vowel(pair[1])
}

// spelledLetter is one letter of a word as spelled writes it.
func spelledLetter(word string, index int) string {
	return spelled(word[index:index+1], index == 0, index == len(word)-1)
}

// pieceSaid is whether three letters of a word of four capitals are said as a word (section 3.9).
func (tables *letterTables) pieceSaid(piece string) bool {
	return !tables.unkept[piece] && tables.saidPattern(piece, false)
}

// qStart is whether a word of capitals starts with Q before a consonant (section 3.9).
func qStart(word string) bool {
	return word[0] == 'Q' && consonant(word[1])
}

// finalQ is whether a word of four capitals that ends in Q or QS is said (section 3.9), and whether it ends so.
func (tables *letterTables) finalQ(word string) (said, ends bool) {
	var head string
	switch {
	case word[3] == 'Q':
		head = word[:3]
	case word[2:] == "QS":
		head = word[:2]
	default:
		return false, false
	}
	if strings.IndexFunc(head, func(letter rune) bool { return vocal(byte(letter)) }) < 0 ||
		strings.Contains(head, "IW") || strings.Contains(head, "UW") {
		return false, true
	}
	for index := range len(head) {
		switch {
		case head[index] != 'Q' || word[0] == 'X' && index > 1:
		case index == 0 && !qStart(word) && (len(head) == 2 || vocal(head[2]) || oneOf(head[2], "HQ")):
		case index >= 2 && head[index-1] == 'Y' && !vowel(head[index-2]):
		default:
			return false, true
		}
	}
	switch {
	case head[:2] == "YY" && len(head) == 3 && consonant(head[2]) && head[2] != 'Q':
		return false, true
	case len(head) == 2 && vowel(head[0]) && vowel(head[1]) && !tables.firstPairs[head]:
		return false, true
	case len(head) == 3 && vowel(head[0]) && !vowel(head[1]):
		return !vocal(head[2]), true
	case len(head) == 3 && vowel(head[0]):
		return tables.firstPairs[head[:2]], true
	}
	return true, true
}

// framed is whether a word of four capitals has its middle letters in a frame that keeps it from being said
// (section 3.9).
func (tables *letterTables) framed(word string) bool {
	first, last := word[0], word[3]
	switch {
	case first == 'W' && last == 'W':
		return oneOf(word[2], "AE")
	case first == 'H' && last == 'H':
		return vowel(word[2])
	}
	return oneOf(first, "JQX") && word[1] == 'Y' && vocal(word[2]) && tables.unkept[string([]byte{first, 'Y', last})]
}

// qInside is whether a word of four capitals has a Q second or third, not after a Y, before A, E, I or O, or before
// a Y but not after a first pair (section 3.9).
func (tables *letterTables) qInside(word string) bool {
	for index := 1; index <= 2; index++ {
		if word[index] != 'Q' || word[index-1] == 'Y' {
			continue
		}
		if oneOf(word[index+1], "AEIO") || word[index+1] == 'Y' && !tables.firstPairs[word[:index]] {
			return true
		}
	}
	return false
}

// coda is whether two letters end a word of four capitals that is said, after a vowel or Y (section 3.9).
func (tables *letterTables) coda(pair string, after byte) bool {
	switch {
	case oneOf(after, "IUY") && pair[0] == 'W' && !(after == 'Y' && (pair == "WJ" || pair == "WX")):
		return false
	case after == 'Y' && pair == "SS":
		return false
	case after == 'Y' && pair == "QZ":
		return true
	case slices.Contains(unsaidCodas, pair):
		return false
	}
	return tables.codaPairs[pair] || oneOf(pair[1], "JSXZ") || pair[0] == pair[1]
}

// closed is whether a word of four capitals read as consonant, vowel, consonant, consonant is said (section 3.9).
func (tables *letterTables) closed(word string) bool {
	pair := word[2:]
	if slices.Contains(strictStarts, word[:2]) {
		switch {
		case word[0] == 'X' && (pair[1] == 'J' && pair[0] != 'Q' || pair == "NX" || pair == "RX"):
			return true
		case pair == "SS" || pair == "ZS" || pair[0] == 'W':
			return false
		}
		return pair[0] == 'H' || pair[0] == pair[1] || oneOf(pair[1], "SZ")
	}
	if oneOf(word[0], "JX") && pair[0] != 'W' {
		if slices.Contains(hardCodas, pair) || pair[0] == 'Q' && !(word[0] == 'X' && pair == "QZ") {
			return false
		}
		return !(word[0] == 'X' && pair[0] == 'J' && !oneOf(pair[1], "JSZ"))
	}
	return tables.coda(pair, word[1])
}

// signedSaid is whether two or three capitals before a sign, the last of a word of four, are said with it, the sign
// after them with no dash (section 3.9): BAH~ and JA~ are said, BAB~ is BAB-~.
func (tables *letterTables) signedSaid(letters string) bool {
	if len(letters) == 2 {
		return oneOf(letters[0], "JX") && vowel(letters[1]) || oneOf(letters[1], "JX") && vowel(letters[0]) ||
			letters == "QU"
	}
	first, second, third := letters[0], letters[1], letters[2]
	switch {
	case oneOf(first, "JX") && vowel(second):
		return !(first == 'J' && third == 'Q')
	case letters[1:] == "WH" || letters[1:] == "WR":
		return oneOf(first, "AEO")
	case third == 'Y':
		return vowel(second) && consonant(first)
	case oneOf(third, "JX"):
		return vowel(second) && (!vowel(first) || tables.firstPairs[letters[:2]])
	case third == 'H':
		return vocal(second) && !vowel(first)
	case third == 'W':
		return oneOf(second, "AEO") && !vowel(first)
	case third == 'Q':
		return second == 'Y' && consonant(first) && first != 'Q'
	case letters[1:] == "QU":
		return !vowel(first) && first != 'Q'
	}
	return letters[:2] == "QU" && vowel(third)
}

// said is whether a word of four capitals is said as a word (section 3.9), exact as capitalsWritten says: the starts
// NY, UC, UK, US and LA and the ends LA, NY and US count only in capitals (section 3.12: nYRE is said as a word, bDLa
// isn't).
func (tables *letterTables) said(word, exact string) bool {
	switch {
	case tables.listed(tables.kept, word, exact):
		return true
	case tables.tails[word[1:]] || tables.heads[word[:3]]:
		return false
	case exact[2:] == "LA" || exact[2:] == "NY" || exact[2:] == "US":
		return exact[:2] != "NY"
	case (slices.Contains(spellingStarts, word[:2]) || slices.Contains(capitalStarts, exact[:2])) &&
		!tables.openHeads[word[:3]]:
		return false
	case exact[:2] == "LA":
		return vocal(word[2]) || vocal(word[3])
	case word[2:] == "QU" && !vowel(word[1]) && word[1] != 'Q' && !qStart(word),
		(word[1:] == "QUE" || word[1:] == "QUO") && !vowel(word[0]) && !qStart(word),
		word[:3] == "GNU" && (consonant(word[3]) && word[3] != 'W' || oneOf(word[3], "EOY")),
		word[1:] == "GNU" && vowel(word[0]):
		return true
	case vowel(word[0]) && word[0] == word[1] && word[1] == word[2], word[1] == word[2] && word[2] == word[3]:
		return false
	case word[0] == 'X' && oneOf(word[1], "IY") && word[2] == 'W' && vocal(word[3]):
		return false
	}
	if letters, ok := tables.unkeptHeads[word[:3]]; ok {
		return oneOf(word[3], letters)
	}
	if said, ends := tables.finalQ(word); ends {
		return said
	}
	switch {
	case tables.framed(word),
		word[3] == 'W' && (oneOf(word[2], "IU") || word[2] == 'Y' && (!vowel(word[1]) || word[0] == 'Y')),
		tables.qInside(word) && word[0] != 'X', word[2:] == "YY", tables.unkept[word[1:]]:
		return false
	case !alphabetic(word[3]):
		return tables.signedSaid(word[:3])
	}
	switch soundKinds(word) {
	case "CVCC":
		return tables.closed(word)
	case "CCVC":
		return tables.onsets[word[:2]]
	case "CVVC", "CVCV":
		return true
	case "CVVV":
		return tables.endPair(word[2:])
	case "CCVV":
		return tables.onsets[word[:2]] && tables.endPair(word[2:])
	case "VCCV":
		return tables.onsets[word[1:3]]
	case "VVCV", "VVVC":
		return tables.firstPairs[word[:2]]
	case "VVVV":
		return tables.firstPairs[word[:2]] && tables.endPair(word[2:])
	case "VVCC":
		return tables.firstPairs[word[:2]] && tables.vowelCodas[word[2:]] && !(oneOf(word[1], "IU") && word[2] == 'W')
	case "CCCV":
		return oneOf(word[2], tables.clusters[word[:2]])
	case "VCCC":
		return (word[1:3] == "PH" || word[1:3] == "SH" || word[1:3] == "TH") && word[3] == 'H' ||
			word[1] == 'W' && oneOf(word[2], "HR") && oneOf(word[0], "AEO")
	}
	return false
}

// kew is whether a word of four capitals reads its QU as KEW (section 3.9).
func kew(word string) bool {
	switch {
	case word[:2] == "QU":
		third, fourth := word[2], word[3]
		if word[2:] == "LA" || word[2:] == "NY" || word[2:] == "PC" {
			return false
		}
		return consonant(third) && third != 'W' || third == 'W' && (vocal(fourth) || !alphabetic(fourth)) ||
			third == 'Y' && fourth != 'Y'
	case word[1:3] == "QU":
		fourth := word[3]
		return fourth == 'Y' || consonant(fourth) && !oneOf(fourth, "SW") || fourth == 'W' && word[0] == 'W'
	}
	return false
}

// kewWritten is how a word of four capitals that reads its QU as KEW is written (section 3.9).
func kewWritten(word string) string {
	switch {
	case word[1:3] == "QU" && (vowel(word[0]) || word[0] == 'Q'):
		return spelledLetter(word, 0) + "-KEW" + word[3:]
	case word[1:3] == "QU":
		return word[:1] + "KEW" + word[3:]
	case slices.Contains(hardCodas, word[2:]):
		return "KEW" + word[2:3] + "-" + word[3:]
	}
	return "KEW" + word[2:]
}

// headSaid is whether the first three letters of a word of four capitals are said as a word (section 3.9), exact as
// capitalsWritten says: a second letter that is Q or the same as the third counts only in the same case (section
// 3.12: acCC is a-c-C-C, iqQL i-q-Q-L).
func (tables *letterTables) headSaid(word, exact string) bool {
	head, following := word[:3], word[3]
	switch {
	case head[2] == 'P' && following == 'C':
		return false
	case head[2] == 'Q':
		return word[0] == 'X' && vowel(head[1]) || head[1] == 'Y' || vowel(head[0]) && exact[1] == 'Q' ||
			tables.firstPairs[head[:2]]
	case vocal(head[2]) && vocal(following):
		return false
	case head[1:] == "QU" && !vowel(head[0]) && head[0] != 'Q':
		return true
	}
	kinds := letterKinds(head)
	switch {
	case kinds[:2] == "YC":
		return false
	case kinds == "VCC" && following == 'Q':
		return false
	case kinds == "VCC":
		letters, cluster := tables.clusters[head[1:]]
		return exact[1] == exact[2] || cluster && oneOf(following, letters) && head[1] != 'W'
	case kinds == "VYC" || kinds == "CVV" || kinds == "YVV" || kinds == "CYY" || kinds == "YYY":
		return true
	case kinds == "VVC" || kinds == "VVV":
		return tables.firstPairs[head[:2]] && !(head[2] == 'W' && oneOf(following, "HQR"))
	}
	return head == "GNU" || tables.pieceSaid(head)
}

// tailSaid is whether the last three letters of a word of four capitals are said as a word (section 3.9).
func (tables *letterTables) tailSaid(word string) bool {
	tail := word[1:]
	switch {
	case tail == "GNU", tail[1:] == "QU" && consonant(tail[0]) && tail[0] != 'Q':
		return true
	case vowel(tail[0]):
		return tail[1:] == "QY" && !vowel(word[0])
	case tail[0] == 'Q' && oneOf(tail[1], "AEIO") || tail[0] == 'Y' && !vocal(tail[1]):
		return false
	case !alphabetic(tail[2]) && (tail[:2] == "WH" || tail[:2] == "WR"):
		return word[0] == 'Y'
	case !alphabetic(tail[2]):
		return tables.signedSaid(tail[:2])
	case letterKinds(tail) == "CCC":
		return word[0] == 'Y' && (tail == "SPH" || tail[:2] == "WH" || tail[:2] == "WR")
	}
	return tables.pieceSaid(tail)
}

// restSaid is whether the three letters after a first Q, or an X before O or U, are said as a word (section 3.9).
func (tables *letterTables) restSaid(rest string) bool {
	if !alphabetic(rest[2]) {
		return tables.signedSaid(rest[:2])
	}
	switch letterKinds(rest) {
	case "VCV":
		return rest[1] != 'Q' || rest[2] == 'U'
	case "VVC":
		return !(rest[2] == 'W' && oneOf(rest[1], "IU"))
	case "VCY", "VVY", "VYV":
		return true
	case "VVV":
		return tables.lastPairs[rest[1:]]
	case "VCC", "VYC":
		return tables.pieceSaid(rest) || tables.vowelCodas[rest[1:]] && rest[1] != 'W'
	}
	return false
}

// restLetters is how the three letters after a first Q, or an X before O or U, that aren't said are written
// (section 3.9).
func (tables *letterTables) restLetters(rest string) string {
	if tables.twoLetters[rest[:2]] && (consonant(rest[2]) || !alphabetic(rest[2])) && !tables.onsets[rest[1:]] {
		return rest[:2] + "-" + rest[2:]
	}
	return spelled(rest, false, true)
}

// keptEnd is whether a word of four capitals that is spelled keeps its last two letters together (section 3.9).
func keptEnd(word string) bool {
	start, end := word[:2], word[2:]
	among := func(word string, words ...string) bool { return slices.Contains(words, word) }
	switch {
	case end == "PC",
		oneOf(word[0], "HW") && vocal(word[1]) && end[0] == word[0] && oneOf(end[1], word[:1]+"SZ"),
		among(end, "HH", "HS", "HZ") && among(start, "IW", "UW", "YY"),
		among(end, "JJ", "JS", "JZ") && among(start, "JY", "QY", "YY"),
		among(end, "XS", "XX", "XZ") && among(start, "JY", "QY", "XY", "YY"),
		among(end, "QS", "RR", "RS", "RZ", "WQ") && among(start, "IW", "UW"),
		among(end, "WS", "WW", "WZ") && !vowel(word[0]) && oneOf(word[1], "IUY"),
		end[0] == 'W' && vocal(end[1]) && among(start, "XI", "XY"):
		return true
	}
	return end[0] == 'X' && vocal(end[1]) && start == "XY"
}

// spelledFour is a word of four capitals letter by letter (section 3.9): three same vowels as triple and that vowel,
// and a kept end together.
func (tables *letterTables) spelledFour(word string) string {
	switch {
	case vowel(word[0]) && word[0] == word[1] && word[1] == word[2]:
		return "triple-" + spelledLetter(word, 2) + "-" + spelledLetter(word, 3)
	case vowel(word[1]) && word[1] == word[2] && word[2] == word[3]:
		return spelledLetter(word, 0) + "-triple-" + spelledLetter(word, 3)
	case keptEnd(word) && word[2:] == "PC" && tables.twoLetters[word[:2]]:
		return word[:2] + "-PC"
	case keptEnd(word):
		return spelledLetter(word, 0) + "-" + spelledLetter(word, 1) + "-" + word[2:]
	}
	return spelled(word, true, true)
}

// split is how a word of four capitals that isn't said as a word is written (section 3.9), exact as capitalsWritten
// says: the starts NY, UC, UK, US, but before U and a tail, ED or ES, and LA count only in capitals, and EDU and IO are
// written as ed-yoo and eye only in capitals (section 3.12: eDUI is eDU-I, iOGF i-O-GF, uSED u-SED), and USE is USE
// before the last only when its E is a capital (section 3.11: USex is U-S-e-x).
func (tables *letterTables) split(word, exact string) string {
	start, writtenStart, head, tail := word[:2], exact[:2], word[:3], word[1:]
	switch {
	case slices.Contains(softStarts, start) && vocal(word[2]) && vocal(word[3]) && !tables.endPair(word[2:]) &&
		!(word[1] == word[2] && word[2] == word[3]):
		return word[:1] + "-" + word[1:2]
	case head == "EDU" && exact[:3] == "EDU":
		return "ed-yoo-" + spelledLetter(word, 3)
	case head == "EDU":
		return "EDU-" + spelledLetter(word, 3)
	case start == "IO" && word[2:] == "NE":
		return "eye-ONE"
	case start == "IO" && word[2] == 'N':
		return "eye-O-N-" + spelledLetter(word, 3)
	case start == "IO":
		return "eye-O-" + word[2:]
	case start == "OS" || start == "PC":
		return word[:1] + "-" + word[1:2] + "-" + word[2:]
	case writtenStart == "NY" && (vocal(word[2]) || vocal(word[3])):
		return "NY-" + word[2:]
	case writtenStart == "NY" || writtenStart == "UC" || writtenStart == "UK":
		return spelled(word, true, true)
	case start == "US" && (word[2] == 'U' && tables.tailSaid(word) || word[2:] == "ED" || word[2:] == "ES"):
		return "U-" + tail
	case head == "USE" && exact[2] == 'E' || writtenStart == "US" && head == "USH" && tables.headSaid(word, exact):
		return head + "-" + spelledLetter(word, 3)
	case writtenStart == "US":
		return spelled(word, true, true)
	case word[0] == 'Q' && oneOf(word[1], "AEIO") || start == "XO" || start == "XU" || word[0] == '[' && vowel(word[1]):
		switch {
		case tables.tails[tail]:
			return word[:1] + "-" + tail
		case tail[0] == tail[1] && tail[1] == tail[2]:
			return word[:1] + "-triple-" + spelledLetter(word, 3)
		case tail[1:] == "PC":
			return word[:1] + "-" + tail[:1] + "-PC"
		case tables.restSaid(tail):
			return word[:1] + "-" + tail
		}
		return word[:1] + "-" + tables.restLetters(tail)
	case vowel(word[1]) && word[1] == word[2] && (word[0] == word[1] || word[2] == word[3]):
		return tables.spelledFour(word)
	case writtenStart == "LA":
		return spelled(word, true, true)
	case tables.heads[head]:
		return head + "-" + spelledLetter(word, 3)
	case tables.tails[tail]:
		return spelledLetter(word, 0) + "-" + tail
	case slices.Contains(softStarts, start) && tables.openHeads[head]:
		return head + "-" + spelledLetter(word, 3)
	case slices.Contains(softStarts, start) && word[2:] == "PC":
		return spelledLetter(word, 0) + "-" + spelledLetter(word, 1) + "-PC"
	case slices.Contains(softStarts, start):
		return spelled(word, true, true)
	case tables.headSaid(word, exact):
		return head + "-" + spelledLetter(word, 3)
	case tables.tailSaid(word):
		return spelledLetter(word, 0) + "-" + tail
	case word[2:] != "PC" && tables.twoLetters[start] && vowel(word[0]) && consonant(word[2]):
		return start + "-" + spelledLetter(word, 2) + "-" + spelledLetter(word, 3)
	}
	return tables.spelledFour(word)
}

// fourLetters is how a word of four capitals that no list has, not in a run, is written (section 3.9), exact as
// capitalsWritten says.
func (tables *letterTables) fourLetters(word, exact string) string {
	switch {
	case kew(word):
		return kewWritten(word)
	case tables.said(word, exact):
		return word
	}
	return tables.split(word, exact)
}
