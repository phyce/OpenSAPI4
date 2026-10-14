package lexical

import (
	"strings"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
)

// searchLimit is how many nodes the search for the next word passes before it takes the node it has reached.
const searchLimit = 11

// Step runs the stage once: it converts the next word, and the previous word gets its post-lexical pass once
// the word after it is converted. It reports whether it handed nodes on (lexical.md §2.1).
func (stage *Stage) Step() bool {
	list := stage.Pipeline
	cursor := stage.cursor()
	list.Begin(cursor)
	if cursor.First == cursor.Done {
		if !stage.ConvertWord() {
			return list.Finish()
		}
		stage.shiftWords()
	}
	if !stage.findNextWord() {
		if cursor.Done != nil {
			for stage.hasClass(cursor.Done.Symbol(), phraseBreak) {
				stage.PreviousSymbol = 0
				cursor.Done = list.Next(cursor.Done)
				if cursor.Done == nil {
					return list.Finish()
				}
			}
		}
		return list.Finish()
	}
	stage.ConvertWord()
	done := stage.PostLexicalWord()
	cursor.Scan, cursor.Done = done, done
	cursor.First = list.Next(stage.LastNode)
	stage.shiftWords()
	return list.Finish()
}

// findNextWord moves the search on to the next node of the stage's types, or the control command C, and makes
// it the cursor's first node. It takes the node it has reached after searchLimit nodes, and reports whether it
// found a node before the window's end (lexical.md §2.2).
func (stage *Stage) findNextWord() bool {
	list := stage.Pipeline
	cursor := stage.cursor()
	if stage.Search == cursor.End {
		cursor.First = stage.Search
		return false
	}
	for {
		node := list.Next(stage.Search)
		stage.Search = node
		if node != nil && cursor.Takes(node) {
			break
		}
		stage.Searched++
		if stage.Searched > searchLimit || node.Type() == pipeline.Control && node.Symbol() == 'C' {
			break
		}
		if node == cursor.End {
			cursor.First = node
			return false
		}
	}
	cursor.First = stage.Search
	return true
}

// shiftWords makes the current word the previous one, and starts the search for the next word after it.
func (stage *Stage) shiftWords() {
	stage.PreviousWord = stage.Word
	stage.Search = stage.LastNode
	stage.PreviousEnd = stage.LastNode
	stage.Searched = 0
	stage.LastNode = nil
	stage.Word = nil
}

// ConvertWord converts the word at the cursor's first node, and reports whether there was one (lexical.md §3).
func (stage *Stage) ConvertWord() bool {
	list := stage.Pipeline
	word := stage.cursor().First
	if word == nil {
		return false
	}
	stage.Word, stage.LastNode = word, word
	if node := list.Next(word); node != nil && node.Data[markByteOffset] == 1 {
		node.Data[markByteOffset] = 0
		for node = list.Next(node); node != nil; node = list.Next(node) {
			if !stage.hasClass(node.Symbol(), stressMark) && node.Data[markByteOffset] != 1 {
				break
			}
		}
		if node == nil {
			stage.Word.Data[markByteOffset] = 1
		}
	}
	if list.NumberWord || stage.letterAfterS(word) {
		list.NumberWord = false
		word.Data[markByteOffset] = 2
	}
	switch stage.Word.Type() {
	case pipeline.Phoneme:
		for node := list.Next(word); node != nil && node.Type() == pipeline.Phoneme &&
			(stage.hasClass(node.Symbol(), phoneme) || stage.hasClass(node.Symbol(), stressMark)); node = list.Next(node) {
			stage.LastNode = node
		}
	case pipeline.Letter:
		stage.convertLetters()
	default:
		return true
	}
	stage.finishWord()
	return true
}

// letterAfterS reports whether the word follows a node S and is a single letter, or the letters A Y, before a
// space or a boundary.
func (stage *Stage) letterAfterS(word *pipeline.Node) bool {
	first := word.Next
	letter := first.Symbol()
	ends := func(node *pipeline.Node) bool {
		return node.Symbol() == ' ' || stage.hasClass(node.Symbol(), boundary)
	}
	if !stage.hasClass(word.Symbol(), wordMark) || word.Previous.Symbol() != 'S' {
		return false
	}
	return letter >= 'A' && letter <= 'Z' && ends(first.Next) || letter == 'A' && first.Next.Symbol() == 'Y' && ends(first.Next.Next)
}

// The letters that emphasize a word and that don't, the word mark that ends a word's letters, and the most
// letters a word can have and be classified.
const (
	emphasis       = '~'
	deemphasis     = '`'
	wordSeparator  = '&'
	maximumLetters = 96
)

// convertLetters converts a word of letters: it takes its emphasis, finds its letters, classifies it, and
// looks it up or converts it by letter-to-sound (lexical.md §3.3).
func (stage *Stage) convertLetters() {
	list := stage.Pipeline
	node := list.Next(stage.Word)
	stage.Emphasized = node.Symbol() == emphasis
	if stage.Emphasized {
		node = list.FreeNode(node, true)
	}
	stage.Deemphasized = node.Symbol() == deemphasis
	if stage.Deemphasized {
		node = list.FreeNode(node, true)
	}
	word := stage.Word
	switch word.Value() {
	case codeDollar, codeFunctionWord, codeJuncture:
		stage.Deemphasized = true
	case codeEmphasis:
		stage.Emphasized = true
	}
	word.SetFlags(word.Flags()&^letterBit | pipeline.Phoneme)
	stage.FirstLetter = node
	for {
		stage.LastNode = node
		node = list.Next(node)
		if node == nil || node.Type() != pipeline.Letter || node.Symbol() == wordSeparator {
			break
		}
	}
	stage.StressPattern, stage.WordClass, stage.AffixStress = 0, 0, 0
	if spelling, found := stage.spelling(); found {
		stage.classify(spelling, stage.Word)
	}
	if !stage.Lookup() || stage.StressPattern != 0 {
		stage.LetterToSound()
	}
	stage.Word.SetSecond(stage.WordClass)
}

// spelling returns the word's letters between two #, as a C string, unless the word has more than
// maximumLetters letters.
func (stage *Stage) spelling() (string, bool) {
	letters := []byte{'#'}
	end := stage.LastNode.Next
	for letter := stage.FirstLetter; letter != end; letter = letter.Next {
		if len(letters) > maximumLetters {
			return "", false
		}
		letters = append(letters, letter.Symbol())
	}
	spelling := string(append(letters, '#'))
	if end := strings.IndexByte(spelling, 0); end >= 0 {
		spelling = spelling[:end]
	}
	return spelling, true
}

// classify gives the word's header the class of the first list that has its spelling, or else of its ending,
// in flag bits 8 to 15 (lexical.md §3.4).
func (stage *Stage) classify(spelling string, word *pipeline.Node) {
	setClass := func(class int32) {
		word.SetFlags(word.Flags()&^classBits | uint32(class)<<classShift)
	}
	setClass(0)
	for _, list := range stage.tables.FunctionWords {
		if strings.Contains(list.Words, spelling) {
			setClass(exceptionClass(list.Class, list.Exceptions, spelling))
			return
		}
	}
	for _, ending := range stage.tables.FunctionWordEndings {
		length := len(spelling)
		if length >= len(ending.Ending)+3 && spelling[length-1-len(ending.Ending):length-1] == ending.Ending {
			setClass(exceptionClass(ending.Class, ending.Exceptions, spelling))
		}
	}
}

func exceptionClass(class int32, exceptions []datapack.WordException, spelling string) int32 {
	for _, exception := range exceptions {
		if exception.Word == spelling {
			return exception.Class
		}
	}
	return class
}

// The word class's bits in a word header's flags.
const (
	classBits  = 0xff00
	classShift = 8
)

// digitStress is the stress a stress digit gives the syllabic symbol before it, or 0 for another symbol.
func digitStress(symbol byte) uint32 {
	switch symbol {
	case '1':
		return 2
	case '2':
		return 1
	case '"':
		return 3
	}
	return 0
}

// finishWord gives the word's syllabic symbols the stress of the digits after them, removes the digits, marks
// the stressed syllables' onsets, and applies the word's code (lexical.md §3.5).
func (stage *Stage) finishWord() {
	list := stage.Pipeline
	node := stage.Word
	for {
		next := list.Next(node)
		if next != nil && node != stage.LastNode {
			if stress := digitStress(next.Symbol()); stress != 0 {
				if stage.hasClass(node.Symbol(), syllabic) {
					node.SetFlags(node.Flags()&^stressBits | stress<<stressShift)
				}
				if stage.LastNode == next {
					stage.LastNode = node
				}
				next = list.FreeNode(next, true)
			}
		}
		if stage.Word.Symbol() != '%' && node.Flags()&stressBits != 0 {
			stage.markStressedOnset(node)
		}
		last := node == stage.LastNode
		node = next
		if last {
			break
		}
	}
	stage.applyWordCode()
}

// The codes the stages before give a word (lexical.md §3.6), and the class of a juncture the codes make.
const (
	codeDollar       = 1
	codeFunctionWord = 2
	codeContentWord  = 3
	codeJuncture     = 5
	codeEmphasis     = 9
	codeLongJuncture = 11
	junctureClass    = 0x3b
)

// applyWordCode applies the code the stages before gave the word.
func (stage *Stage) applyWordCode() {
	word := stage.Word
	if word.Flags()&codeFlag == 0 {
		return
	}
	cursor := stage.cursor()
	switch word.Value() {
	case codeDollar:
		if cursor.First != cursor.Done {
			word.SetSymbol('$')
		}
	case codeFunctionWord:
		word.SetSymbol('%')
		word.SetSecond(1)
	case codeContentWord:
		word.SetSymbol('&')
		word.SetSecond(0)
	case codeJuncture:
		word.SetSymbol(' ')
		word.SetValue(4)
		word.SetSecond(junctureClass)
	case codeEmphasis:
		stage.Emphasized = true
	case codeLongJuncture:
		juncture := stage.Pipeline.NewNode(word, true, pipeline.Phoneme, ' ')
		juncture.SetValue(8)
		word.SetSecond(junctureClass)
	}
}

// markStressedOnset marks a stressed syllable's nucleus and the consonants of its onset: one before it, or Y,
// and one more that isn't a sonorant, nor a closure before a closure, with an S before it (lexical.md §3.5).
func (stage *Stage) markStressedOnset(node *pipeline.Node) {
	node.SetFlags(node.Flags() | onsetFlag)
	previous := stage.previousPhoneme(node)
	if previous == nil {
		return
	}
	symbol := previous.Symbol()
	if !stage.hasClass(symbol, consonant) && symbol != 'Y' || symbol == '~' {
		return
	}
	previous.SetFlags(previous.Flags() | onsetFlag)
	before := stage.previousPhoneme(previous)
	if before == nil {
		return
	}
	symbol = before.Symbol()
	if !stage.hasClass(symbol, consonant) || stage.hasClass(symbol, sonorant) ||
		stage.hasClass(symbol, closure) && stage.hasClass(before.Next.Symbol(), closure) {
		return
	}
	before.SetFlags(before.Flags() | onsetFlag)
	if third := stage.previousPhoneme(before); third != nil && third.Symbol() == 'S' {
		third.SetFlags(third.Flags() | onsetFlag)
	}
}

// previousPhoneme returns the last phoneme or segment node, of type 3 or 4, before the node in the window.
func (stage *Stage) previousPhoneme(node *pipeline.Node) *pipeline.Node {
	for node = stage.Pipeline.Previous(node); node != nil && node.Type() != pipeline.Phoneme && node.Type() != pipeline.Segment; node = stage.Pipeline.Previous(node) {
	}
	return node
}
