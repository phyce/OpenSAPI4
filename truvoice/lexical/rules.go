package lexical

import (
	"strings"

	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
)

// The flag bit a vowel gets for a glottal stop before it (lexical.md §13).
const glottalFlag = 0x40

// The first phonemes of the keys' names: eight, four, five, nine, pound, six, seven, star, two, one, three and
// zero (lexical.md §13.2).
var keypadNames = []string{"AT", "FwR", "FIV", "NIN", "PfND", "SiKS", "SeV|N", "SToR", "Tb", "WvN", "XRE", "Z4RO"}

// around is what the post-lexical rules know of the phoneme they work on (lexical.md §13.1): its node and the
// node the closing rules work on, the phonemes before it and after it and the one after that, their symbols, the
// sounds before and after it, whether the current word is a break, whether the phoneme before it is a word mark
// at the list's start, and the node the rules return.
type around struct {
	node, current                                       *pipeline.Node
	previous, next, afterNext                           *pipeline.Node
	symbol, previousSymbol, nextSymbol, afterNextSymbol byte
	soundBefore, soundAfter                             byte
	beforeBreak, listStart                              bool
	result                                              *pipeline.Node
}

// PostLexicalRules applies the post-lexical rules to a phoneme of the previous word and returns the node the
// pass goes on with (lexical.md §13).
func (stage *Stage) PostLexicalRules(node *pipeline.Node) *pipeline.Node {
	at := stage.around(node)
	if !stage.keypadPause(at) && !stage.hasClass(at.symbol, consonant) {
		stage.vowelRules(at)
	}
	if at.current != nil {
		stage.closingRules(at)
	}
	if at.result == nil {
		return at.current
	}
	return at.result
}

func (stage *Stage) around(node *pipeline.Node) *around {
	at := &around{node: node, current: node, symbol: node.Symbol()}
	at.beforeBreak = stage.Word != nil && stage.hasClass(stage.Word.Symbol(), phraseBreak)
	at.next = stage.nextPhoneme(node)
	at.previous = stage.previousPhoneme(node)
	at.afterNext = stage.nextPhoneme(at.next)
	at.nextSymbol, at.previousSymbol, at.afterNextSymbol = symbolOf(at.next), symbolOf(at.previous), symbolOf(at.afterNext)
	for walker := at.previous; walker != nil && !stage.hasClass(walker.Symbol(), phraseBreak); walker = stage.previousPhoneme(walker) {
		if stage.hasClass(walker.Symbol(), phoneme) {
			at.soundBefore = walker.Symbol()
			break
		}
	}
	at.listStart = at.previous == nil || isWordMark(at.previousSymbol) && at.previous.Previous.Symbol() == 0
	if at.soundBefore == 0 {
		at.soundBefore = byte(stage.PreviousSymbol)
	} else {
		stage.PreviousSymbol = 0
	}
	for walker := at.next; walker != nil && !stage.hasClass(walker.Symbol(), phraseBreak); walker = stage.nextPhoneme(walker) {
		if stage.hasClass(walker.Symbol(), phoneme) {
			at.soundAfter = walker.Symbol()
			break
		}
	}
	return at
}

// nextPhoneme returns the first phoneme node, of type 3, after the node in the window.
func (stage *Stage) nextPhoneme(node *pipeline.Node) *pipeline.Node {
	if node == nil {
		return nil
	}
	for node = stage.Pipeline.Next(node); node != nil && node.Type() != pipeline.Phoneme; node = stage.Pipeline.Next(node) {
	}
	return node
}

func symbolOf(node *pipeline.Node) byte {
	if node == nil {
		return 0
	}
	return node.Symbol()
}

func markByte(node *pipeline.Node) byte {
	if node == nil {
		return 0
	}
	return node.Data[markByteOffset]
}

// insert puts a phoneme with the symbol and value, in frames, before the node or after it, and returns it.
func (stage *Stage) insert(node *pipeline.Node, after bool, symbol byte, value uint32) *pipeline.Node {
	inserted := stage.Pipeline.NewNode(node, after, pipeline.Phoneme, symbol)
	inserted.SetValue(value)
	return inserted
}

func (stage *Stage) isRColoured(symbol byte) bool {
	return symbol == '3' || stage.hasClass(symbol, rColoured)
}

// keypadPause puts a pause before a key's name said after "press" at a phrase's end, and reports whether it did
// (lexical.md §13.2).
func (stage *Stage) keypadPause(at *around) bool {
	before := at.node.Previous
	if !at.beforeBreak || before == nil {
		return false
	}
	walker := before
	for _, symbol := range []byte("SeRP&") {
		if walker = walker.Previous; walker == nil || walker.Symbol() != symbol {
			return false
		}
	}
	if walker.Previous == nil || stage.hasClass(walker.Previous.Symbol(), phoneme) {
		return false
	}
	if before.Data[markByteOffset] != 2 && !stage.keypadWord() {
		return false
	}
	at.result = stage.insert(at.node, false, ' ', 27)
	return true
}

// keypadWord reports whether the previous word starts with a key's name.
func (stage *Stage) keypadWord() bool {
	for _, name := range keypadNames {
		node := stage.PreviousWord.Next
		for index := 0; index < len(name) && node != nil && node.Symbol() == name[index]; index++ {
			if index == len(name)-1 {
				return true
			}
			node = node.Next
		}
	}
	return false
}

// vowelRules are the rules for a phoneme that isn't a consonant: the vowels, the syllabic consonants, `Y` and
// the pause (lexical.md §13.3).
func (stage *Stage) vowelRules(at *around) {
	list := stage.Pipeline
	cursor := stage.cursor()
	node := at.node
	isVowel := stage.hasClass(at.symbol, vowel)
	afterMark := isWordMark(at.previousSymbol)
	stressed := node.Flags()&onsetFlag != 0
	marked := markByte(stage.PreviousWord) == 1 || markByte(stage.Word) == 1
	if isVowel && at.symbol != 'U' && afterMark && cursor.Pitch != 0 {
		if at.symbol == at.soundBefore && stage.hasClass(at.symbol, yGlide) && !marked {
			at.result = stage.insert(node, false, 'Y', 4)
			return
		}
		stage.glottalStop(at, marked)
	}
	if at.symbol == 'e' && !stressed && (at.nextSymbol == 'K' && at.afterNextSymbol == 'S' || at.nextSymbol == 'G' && at.afterNextSymbol == 'Z') &&
		(afterMark || at.previousSymbol == ' ') {
		node.SetSymbol('|')
		return
	}
	if at.symbol == '|' && !stressed && at.previousSymbol == 'B' && (at.soundAfter == 'H' || at.soundAfter == 'd') && !isWordMark(at.nextSymbol) {
		node.SetSymbol('E')
		return
	}
	if at.symbol == 'Y' && afterMark && stage.PreviousWord.Second() == pronounClass &&
		(at.soundBefore == 'z' || at.soundBefore == 's' && stage.thirdBeforeScan() == 'C') {
		at.result = list.FreeNode(node, false)
		return
	}
	if at.symbol == 'l' && (at.nextSymbol == '[' || at.nextSymbol == 'D') {
		node.SetSymbol('j')
		at.result = stage.insert(node, false, '@', 4)
		return
	}
	if at.symbol == 'm' && strings.IndexByte(".?,", at.nextSymbol) < 0 {
		node.SetSymbol('M')
		at.result = stage.insert(node, false, '@', 0)
		return
	}
	if at.symbol == 'n' && at.previousSymbol != 'q' && stage.hasClass(at.soundAfter, vowel) {
		node.SetSymbol('N')
		at.result = stage.insert(node, false, '@', 0)
		return
	}
	if stage.hasClass(at.soundBefore, vowel) && at.previous != nil && at.previous.Previous != nil && at.previous.Previous.Flags()&onsetFlag != 0 &&
		stressed && cursor.Pitch != 0 && isVowel && afterMark {
		at.result = stage.insert(node, false, ' ', 5)
		return
	}
	if strings.IndexByte("@ov", at.soundBefore) >= 0 && stressed && cursor.Pitch != 0 {
		if isVowel && afterMark {
			at.result = stage.insert(node, false, ' ', 5)
			node.SetFlags(node.Flags() | glottalFlag)
			return
		}
	} else if isVowel && stressed && afterMark && stage.hasClass(at.soundBefore, vowel) && !stage.isRColoured(at.soundBefore) && !marked {
		at.result = stage.insert(node, false, ' ', 3)
		node.SetFlags(node.Flags() | glottalFlag)
		return
	}
	if at.symbol == 'i' && node.Flags()&(stressBits|onsetFlag) == 0 && !weakClass(stage.PreviousWord.Second()) && !at.listStart &&
		stage.insidePhrase(node) {
		at.symbol = '|'
		node.SetSymbol('|')
	}
	if (at.symbol == '@' || at.symbol == '|') && at.nextSymbol == 'L' && !stage.hasClass(at.previousSymbol, nasal) &&
		(at.afterNext == nil || at.afterNext.Next.Symbol() != 'L' && (stage.hasClass(at.afterNextSymbol, reduced) ||
			stage.hasClass(at.afterNextSymbol, wordMark) || stage.hasClass(at.afterNextSymbol, boundary)) || at.afterNextSymbol == 'Z') {
		node.SetSymbol('l')
		at.result = list.FreeNode(at.next, false)
		return
	}
	if (stage.hasClass(at.previousSymbol, consonant) && !stage.hasClass(at.previousSymbol, sonorant) || stage.hasClass(at.previousSymbol, boundary)) &&
		stage.hasClass(at.symbol, neutral) && at.nextSymbol == 'N' && at.soundBefore == 'q' {
		node.SetSymbol('n')
		at.result = list.FreeNode(at.next, false)
		return
	}
	if at.symbol == 'E' && at.nextSymbol == '@' && at.previousSymbol == 'j' && stressOf(node) == 0 {
		at.symbol = 'Y'
		node.SetSymbol('Y')
	}
	if stage.hasClass(at.symbol, syllabic) && at.nextSymbol == 'R' && (at.afterNext == nil || stressOf(at.afterNext) == 0) && stage.rColour(at) {
		return
	}
	if node.Symbol() == '3' && node.Previous.Symbol() == 'I' && node.Previous.Flags()&onsetFlag != 0 && !beforeSchwa(node.Next) {
		node.Previous.SetSymbol('5')
		at.result = list.FreeNode(node, false)
		at.current = at.result
		return
	}
	if stage.isRColoured(at.symbol) && at.nextSymbol == 'R' && strings.IndexByte("EAeaIifvOy@b|", at.afterNextSymbol) >= 0 {
		at.result = list.FreeNode(at.next, false)
	}
	if at.symbol == '3' && at.soundBefore == '3' && at.previousSymbol != '&' {
		list.NewNode(node, false, pipeline.Phoneme, 'R')
		return
	}
	if stage.isRColoured(node.Symbol()) && afterMark && at.previous.Previous != nil && stage.isRColoured(at.previous.Previous.Symbol()) {
		walker := at.previous.Previous.Previous
		for walker != nil && (!stage.hasClass(walker.Symbol(), phoneme) || walker.Symbol() == ' ') {
			walker = stage.previousPhoneme(walker)
		}
		if walker == nil {
			return
		}
		if !stage.isRColoured(walker.Symbol()) {
			at.result = stage.insert(node, false, ' ', 6)
			return
		}
	}
	if node.Symbol() == 'b' && at.previousSymbol == 'Y' && stressOf(node) != 0 && at.previous.Previous != nil && stage.hasClass(at.previous.Previous.Symbol(), consonant) {
		node.SetSymbol('U')
		at.result = list.FreeNode(at.previous, false)
		return
	}
	if stage.hasClass(at.symbol, vowel) &&
		(stage.hasClass(at.previousSymbol, yGlide) || at.previousSymbol == '[' && stage.hasClass(at.previous.Previous.Symbol(), yGlide)) {
		at.result = stage.insert(node, false, 'Y', 3)
	}
}

// glottalStop gives a word's first vowel a glottal stop, by the sound before it and the rate (lexical.md §13.3).
func (stage *Stage) glottalStop(at *around, marked bool) {
	node, before := at.node, at.soundBefore
	rate := stage.cursor().Rate
	if (before == 0 || before == at.symbol) && !marked {
		if before == at.symbol || stressOf(node) != 0 && stage.hasClass(at.symbol, front) && at.symbol != 'a' {
			node.SetFlags(node.Flags() | glottalFlag)
		}
		return
	}
	switch stress := stressOf(node); {
	case stress == 0:
	case before == '@', stage.hasClass(before, vowel) && rate < 13, stress == 2 && rate < 13 && stage.hasClass(before, syllabic),
		rate < 7 && (stage.hasClass(before, fricative) || stage.hasClass(before, sonorant)):
		node.SetFlags(node.Flags() | glottalFlag)
	}
}

// thirdBeforeScan returns the symbol of the third node before the cursor's scan node in the list.
func (stage *Stage) thirdBeforeScan() byte {
	node := stage.cursor().Scan
	for range 3 {
		if node == nil {
			return 0
		}
		node = node.Previous
	}
	return symbolOf(node)
}

// insidePhrase reports whether the node has two nodes before it in the list, the one before that not in class
// break and the one before it not a space, and three after it, none of them a full stop or a question mark.
func (stage *Stage) insidePhrase(node *pipeline.Node) bool {
	before := node.Previous
	if before == nil || before.Previous == nil || stage.hasClass(before.Previous.Symbol(), phraseBreak) || before.Symbol() == ' ' {
		return false
	}
	for range 3 {
		if node = node.Next; node == nil || node.Symbol() == '.' || node.Symbol() == '?' {
			return false
		}
	}
	return true
}

// beforeSchwa reports whether the node, or the node after it when it is `[`, is `|`, `@` or `O`.
func beforeSchwa(node *pipeline.Node) bool {
	if node == nil {
		return false
	}
	if node.Symbol() == '[' {
		node = node.Next
	}
	return strings.IndexByte("|@O", node.Symbol()) >= 0
}

// rColour makes a syllabic phoneme before `R` r-coloured, and frees the `R`, and reports whether that ends the
// rules (lexical.md §13.3, rule 14).
func (stage *Stage) rColour(at *around) bool {
	list := stage.Pipeline
	node, symbol := at.node, at.symbol
	if after := node.Next.Next; node.Previous.Symbol() == 'F' && after != nil && (symbol == '|' || symbol == '@') && after.Symbol() == symbol {
		list.FreeNode(node, false)
		return true
	}
	switch {
	case strings.IndexByte("v|@i", symbol) >= 0:
		node.SetSymbol('3')
	case symbol == 'E':
		node.SetSymbol('4')
	case symbol == 'I' && !beforeSchwa(at.afterNext):
		node.SetSymbol('5')
	case strings.IndexByte("aeA", symbol) >= 0:
		node.SetSymbol('k')
	case symbol == 'o' && strings.IndexByte("Ei|e", at.afterNextSymbol) < 0:
		node.SetSymbol('r')
	case symbol == 'O' || symbol == 'w':
		node.SetSymbol('g')
	case symbol == 'b' || symbol == 'u':
		node.SetSymbol('c')
	case symbol == 'I' && strings.IndexByte("@O|", at.afterNextSymbol) < 0, symbol == 'f', symbol == 'y':
		at.next.SetSymbol('3')
	}
	if after := node.Next.Next; symbol != node.Symbol() &&
		(stage.fullVowel(at.afterNextSymbol) || after != nil && after.Symbol() == '[' && stage.fullVowel(after.Next.Symbol())) {
		at.result = list.FreeNode(at.next, false)
	}
	if !stage.hasClass(at.afterNextSymbol, vowel) {
		if at.afterNextSymbol == '[' && stage.hasClass(at.afterNext.Next.Symbol(), vowel) {
			return true
		}
		if at.next.Symbol() != '3' {
			at.result = list.FreeNode(at.next, false)
		}
	}
	return false
}

// fullVowel reports whether the symbol is a vowel other than `p`.
func (stage *Stage) fullVowel(symbol byte) bool {
	return symbol != 'p' && stage.hasClass(symbol, vowel)
}

// closingRules are the rules for the node the rules end on, after the others (lexical.md §13.5).
func (stage *Stage) closingRules(at *around) {
	list := stage.Pipeline
	current := at.current
	if (at.soundBefore == '@' || at.soundBefore == '|') && (current.Symbol() == '@' || current.Symbol() == '|') && isWordMark(at.previousSymbol) {
		at.result = stage.insert(current, false, ' ', 4)
		current.SetFlags(current.Flags() | glottalFlag)
	}
	if current.Symbol() == 'l' && current.Next.Symbol() == '@' {
		current.SetSymbol('j')
		at.result = stage.insert(current, false, '@', 0)
	}
	if current.Symbol() == 'l' {
		before := current.Previous
		if strings.IndexByte("TFKRSsX", before.Symbol()) >= 0 || (before.Symbol() == 'W' || before.Symbol() == '[') && before.Previous.Symbol() == 'K' {
			stage.splitSyllabicL(at, '@')
		}
	}
	if current.Symbol() == 'l' {
		before := current.Previous
		if strings.IndexByte("GPZB", before.Symbol()) >= 0 || before.Symbol() == 'D' && before.Previous.Symbol() == 'N' {
			stage.splitSyllabicL(at, '|')
		}
	}
	if current.Symbol() == 'j' && current.Previous.Symbol() == '@' && current.Previous.Previous.Symbol() == 't' && strings.IndexByte(".&% ,?", at.nextSymbol) >= 0 {
		current.SetSymbol('l')
		at.result = list.FreeNode(current.Previous, false)
	}
	if current.Symbol() == '~' && (stage.hasClass(current.Next.Symbol(), phraseBreak) || current.Next.Symbol() == ' ') {
		at.result = stage.insert(current, true, 'p', 0)
	}
	if current.Symbol() == 'l' && current.Previous.Symbol() == '3' && current.Previous.Previous.Symbol() == 't' {
		at.result = stage.insert(current, false, 'R', 0)
	}
	if after := current.Next.Symbol(); current.Symbol() == 'l' && current.Previous.Symbol() == '3' &&
		(after == ' ' || stage.hasClass(after, wordMark) || stage.hasClass(after, phraseBreak)) {
		at.result = stage.insert(current, false, 'R', 5)
	}
	if stage.hasClass(current.Symbol(), vowel) && at.previous != nil {
		switch {
		case at.soundBefore == '4' || at.soundBefore == '5':
			at.result = stage.insert(at.previous, !isWordMark(at.previous.Symbol()), 'R', 0)
		case at.previousSymbol == 'g':
			at.result = stage.insert(at.previous, true, 'R', 3)
		}
	}
	if current.Symbol() == 'l' && current.Previous.Symbol() == 'Y' {
		at.result = list.FreeNode(current.Previous, false)
	}
}

// splitSyllabicL makes a syllabic `l` the vowel with 4 frames, followed by `j`.
func (stage *Stage) splitSyllabicL(at *around, vowel byte) {
	at.current.SetSymbol(vowel)
	at.current.SetValue(4)
	at.result = stage.insert(at.current, true, 'j', 0)
}
