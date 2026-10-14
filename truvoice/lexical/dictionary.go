package lexical

import (
	"slices"
	"strings"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
)

// The bits of a letter the dictionary compares, the code of a word that takes its second entry, and the most
// symbols of an ending the homographs test.
const (
	letterCode      = 0x1f
	codeAlternative = 4
	maximumEnding   = 30
)

// DictionaryLookup looks up the stem between the cursor's working nodes in the dictionary. An entry with
// phonemes replaces the stem's letters with them; one without gives the word its stress pattern. It reports
// whether it found the stem (lexical.md §6).
func (stage *Stage) DictionaryLookup() bool {
	cursor := stage.cursor()
	first, last := cursor.Working[0], cursor.Working[1]
	toWordEnd := stage.LastNode == last
	fromWordStart := stage.FirstLetter == first
	var letters []byte
	for node := first.Next; node != last.Next; node = node.Next {
		if len(letters) == datapack.DictionaryLetters {
			return false
		}
		letters = append(letters, node.Symbol())
	}
	initial := first.Symbol()
	if initial < 'A' || initial > 'Z' {
		return false
	}
	buckets := stage.dictionary.Entries[initial-'A']
	if len(letters) >= len(buckets) {
		return false
	}
	entries := buckets[len(letters)]
	readFirst, alternative := true, -1
	search := func(start int) bool {
		for index := start; index < len(entries); index++ {
			entry := &entries[index]
			if !sameLetters(letters, entry.Letters) {
				continue
			}
			if word := stage.Word; word.Flags()&codeFlag != 0 && word.Value() == codeAlternative {
				alternative = index
				word.SetFlags(word.Flags() &^ codeFlag)
				continue
			}
			if entry.Class == verbClass && !stage.takesVerbEntry(toWordEnd, &readFirst) {
				continue
			}
			stage.takeEntry(entry, fromWordStart, toWordEnd)
			return true
		}
		return false
	}
	return search(0) || alternative >= 0 && search(alternative)
}

// sameLetters reports whether a stem's letters after its first are an entry's, compared by their five-bit codes.
func sameLetters(letters []byte, entry string) bool {
	if len(letters) != len(entry) {
		return false
	}
	for index, letter := range letters {
		if letter&letterCode != entry[index]&letterCode {
			return false
		}
	}
	return true
}

// spellingBetween returns the symbols from one node up to another, between two #, as the homographs' words are.
func spellingBetween(first, end *pipeline.Node) string {
	symbols := []byte{'#'}
	for node := first; node != end; node = node.Next {
		symbols = append(symbols, node.Symbol())
	}
	return string(append(symbols, '#'))
}

// The function words' classes the homographs test (section 3.4): the list's class in the low four bits, the
// subclass in the high four.
const (
	articleWords       = 0x01
	pronounWords       = 0x12
	quantifierWords    = 0x03
	conjunctionWords   = 0x04
	prepositionWords   = 0x05
	numberWords        = 0x06
	possessiveWords    = 0x08
	beWords            = 0x0c
	haveWords          = 0x0d
	apostropheSWords   = 0x0e
	demonstrativeWords = 0x0f
)

// takesVerbEntry reports whether a verb's entry fits the words before the stem: it does unless the stem is a
// homograph, or the past tense word, that the previous word, or the one before it, makes a noun or a past
// tense (lexical.md §6.3).
func (stage *Stage) takesVerbEntry(toWordEnd bool, readFirst *bool) bool {
	cursor := stage.cursor()
	dictionary := stage.dictionary
	previous := stage.PreviousWord
	stem := spellingBetween(cursor.Working[0], cursor.Working[1].Next)
	plural := false
	if toWordEnd {
		if stem == dictionary.PastTense && previous != nil && *readFirst {
			*readFirst = false
			if class := previous.Flags() >> classShift & 0xff; class&0xf == beWords || class&0xf == haveWords || class == pronounWords {
				return false
			}
		}
	} else {
		var symbols []byte
		for node := cursor.Working[1].Next; node != stage.LastNode.Next && len(symbols) < maximumEnding; node = node.Next {
			if node.Symbol() != affixMark {
				symbols = append(symbols, node.Symbol())
			}
		}
		ending := string(symbols)
		if slices.Contains(dictionary.VerbEndings, ending) {
			return true
		}
		plural = slices.Contains(dictionary.PluralEndings, ending)
	}
	if !strings.Contains(dictionary.Homographs, stem) {
		return true
	}
	if next := stage.LastNode.Next; stem == dictionary.NounBeforeColon && next != nil && next.Type() == pipeline.Character && next.Symbol() == ':' {
		return false
	}
	if previous == nil {
		return true
	}
	class := previous.Flags() >> classShift & 0xff
	subclass := class >> 4
	switch class & 0xf {
	case articleWords, possessiveWords, apostropheSWords, beWords:
		return false
	case quantifierWords, prepositionWords, demonstrativeWords:
		return subclass == 1 && !plural
	case numberWords:
		return subclass == 1 && plural
	case conjunctionWords:
		switch subclass {
		case 1:
			return plural
		case 2:
			return false
		}
		return true
	}
	before := stage.wordBefore(previous)
	return before == nil || before.Flags()>>classShift&0xf != articleWords
}

// wordBefore returns the header of the word before a word's header: the nearest word mark & or % among the 8
// nodes before it, before a break.
func (stage *Stage) wordBefore(word *pipeline.Node) *pipeline.Node {
	node := word.Previous
	for passed := 0; node != nil && passed < 8 && !stage.hasClass(node.Symbol(), phraseBreak); passed++ {
		if node.Symbol() == '&' || node.Symbol() == '%' {
			return node
		}
		node = node.Previous
	}
	return nil
}

// takeEntry gives the word the entry's class, and replaces the stem's letters with the entry's phonemes, or
// gives the word the entry's stress pattern (lexical.md §6.4).
func (stage *Stage) takeEntry(entry *datapack.DictionaryEntry, fromWordStart, toWordEnd bool) {
	if entry.Class != 0 && !stage.Emphasized {
		if fromWordStart && toWordEnd && entry.Class != verbClass && entry.Class != adverbClass {
			stage.Word.SetSymbol('%')
		}
		if stage.WordClass == 0 {
			stage.WordClass = entry.Class
		}
	}
	if entry.Phonemes == "" {
		stage.StressPattern = entry.StressPattern
		return
	}
	node := stage.freeStem()
	primary, unstressed := false, false
	var stress uint32
	var full, weak *pipeline.Node
	for index := range len(entry.Phonemes) {
		switch symbol := entry.Phonemes[index]; symbol {
		case '0':
			unstressed = true
		case '1':
			primary = true
			stress = stage.primaryStress()
		case '2':
			stress = 1
		default:
			node = stage.Pipeline.NewNode(node, true, pipeline.Phoneme, symbol)
			if stress != 0 {
				node.SetFlags(node.Flags()&^stressBits | stress<<stressShift)
			} else if !primary && stage.hasClass(symbol, syllabic) && full == nil && !unstressed {
				if !stage.hasClass(symbol, reduced) {
					full = node
				} else if weak == nil {
					weak = node
				}
			}
			unstressed, stress = false, 0
		}
	}
	stage.finishStem(node, fromWordStart, toWordEnd)
	if !primary {
		stage.stressVowel(full, weak)
	}
}

// freeStem frees the stem between the cursor's working nodes and returns the node before it, where the
// phonemes that replace it go.
func (stage *Stage) freeStem() *pipeline.Node {
	list := stage.Pipeline
	cursor := stage.cursor()
	node := cursor.Working[0]
	for node != cursor.Working[1] {
		node = list.FreeNode(node, true)
	}
	node = list.FreeNode(node, false)
	cursor.Working[0] = node
	return node
}

// finishStem makes the phonemes after the node freeStem returned, up to the last, the cursor's working nodes,
// and the word's first or last nodes when the stem started or ended the word.
func (stage *Stage) finishStem(last *pipeline.Node, fromWordStart, toWordEnd bool) {
	cursor := stage.cursor()
	cursor.Working[1] = last
	cursor.Working[0] = cursor.Working[0].Next
	if fromWordStart {
		stage.FirstLetter = cursor.Working[0]
	}
	if toWordEnd {
		stage.LastNode = last
	}
}

// stressVowel gives a word without a primary stress one, on its first full vowel without a stress digit, else
// on its first reduced one.
func (stage *Stage) stressVowel(full, weak *pipeline.Node) {
	if full == nil {
		full = weak
	}
	if full != nil {
		full.SetFlags(full.Flags()&^stressBits | stage.primaryStress()<<stressShift)
	}
}

// primaryStress is a word's primary stress: 3 when it is emphasized, 1 when it is de-emphasized, else 2.
func (stage *Stage) primaryStress() uint32 {
	switch {
	case stage.Emphasized:
		return 3
	case stage.Deemphasized:
		return 1
	}
	return 2
}
