package lexical

import (
	"strings"

	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
)

// The flag bits a phoneme gets for a glottal stop before it, and a word mark to join its word to the one before
// (lexical.md §13).
const (
	glottalFlag = 0x40
	flaggedMark = 0x80
)

// The first phonemes of the keys' names: eight, four, five, nine, pound, six, seven, star, two, one, three and
// zero (lexical.md §13.2).
var keypadNames = []string{"AT", "FwR", "FIV", "NIN", "PfND", "SiKS", "SeV|N", "SToR", "Tb", "WvN", "XRE", "Z4RO"}

// around is what the post-lexical rules know of the phoneme they work on (lexical.md §13.1): its node and the
// node the closing rules work on, the phonemes before it and after it and the one after that, their symbols, the
// sounds before and after it and the sound after's node, whether the current word is a break, whether the phoneme
// before it is a word mark at the list's start, the node the rules return, and whether the word mark after it or
// before it gets flagged.
type around struct {
	node, current                                       *pipeline.Node
	previous, next, afterNext                           *pipeline.Node
	symbol, previousSymbol, nextSymbol, afterNextSymbol byte
	soundBefore, soundAfter                             byte
	soundAfterNode                                      *pipeline.Node
	beforeBreak, listStart                              bool
	result                                              *pipeline.Node
	flagNext, flagPrevious                              bool
}

// PostLexicalRules applies the post-lexical rules to a phoneme of the previous word and returns the node the
// pass goes on with (lexical.md §13).
func (stage *Stage) PostLexicalRules(node *pipeline.Node) *pipeline.Node {
	at := stage.around(node)
	if !stage.keypadPause(at) {
		if stage.hasClass(at.symbol, consonant) {
			stage.consonantRules(at)
		} else {
			stage.vowelRules(at)
		}
	}
	if at.current != nil {
		stage.closingRules(at)
	}
	if at.flagNext && isWordMark(at.nextSymbol) {
		at.next.SetFlags(at.next.Flags() | flaggedMark)
	} else if at.flagPrevious && isWordMark(at.previousSymbol) {
		at.previous.SetFlags(at.previous.Flags() | flaggedMark)
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
			at.soundAfter, at.soundAfterNode = walker.Symbol(), walker
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

// consonantRules are the rules for a consonant (lexical.md §13.4).
func (stage *Stage) consonantRules(at *around) {
	list := stage.Pipeline
	cursor := stage.cursor()
	node, symbol := at.node, at.symbol
	afterMark := isWordMark(at.previousSymbol)
	if at.soundBefore == 'q' && cursor.Pitch != 0 && stage.hasClass(symbol, voiced) && at.previous != nil {
		if before := at.previous.Previous.Previous; before != nil && (!stage.hasClass(before.Symbol(), vowel) || symbol != 'x') {
			at.flagPrevious = true
			node.SetFlags(node.Flags() | glottalFlag)
		}
	}
	if symbol == 'Q' {
		if !stage.hasClass(at.nextSymbol, phoneme) {
			node.SetSymbol(' ')
			return
		}
		if stage.hasClass(at.nextSymbol, vowel) && cursor.Pitch != 0 {
			at.next.SetFlags(at.next.Flags() | glottalFlag)
		}
		at.result = list.FreeNode(node, false)
		return
	}
	if symbol == 'Z' && !stage.hasClass(at.soundAfter, voiced) && !isWordMark(at.nextSymbol) && !stage.hasClass(at.nextSymbol, phraseBreak) {
		node.SetSymbol('S')
		return
	}
	if at.nextSymbol == '[' && node.Next.Next.Symbol() == symbol {
		at.result = list.FreeNode(node, true)
		return
	}
	if symbol == 'x' && afterMark && at.previous.Previous.Symbol() == 'D' && at.previous.Previous.Previous != nil && at.previous.Previous.Previous.Symbol() == 'N' {
		at.result = list.FreeNode(node, false)
		at.flagPrevious = true
		return
	}
	if stage.lRules(at) {
		return
	}
	if stage.hasClass(symbol, approximant) && afterMark && node.Flags()&onsetFlag != 0 && cursor.Pitch != 0 && at.soundBefore == 0 && cursor.Rate < 25 {
		node.SetFlags(node.Flags() | glottalFlag)
	}
	if symbol == 'N' && at.nextSymbol == '[' && stage.hasClass(at.soundAfter, velar) && cursor.Rate > 9 {
		node.SetSymbol('~')
		return
	}
	if symbol == 'H' && at.previous != nil &&
		(cursor.Rate > 13 && stage.hasClass(at.previousSymbol, wordMark) && node.Flags()&onsetFlag == 0 && stage.hasClass(at.soundBefore, consonant) ||
			!stage.hasClass(at.nextSymbol, vowel) && (at.nextSymbol != 'Y' || symbolOf(stage.nextInWindow(at.next)) != 'b')) {
		at.result = list.FreeNode(node, false)
		return
	}
	if cursor.Rate > 6 && stage.hasClass(at.soundBefore, voiced) && (symbol == 'H' || symbol == 'h') {
		at.flagPrevious = true
		node.SetSymbol(map[byte]byte{'H': 'd', 'h': 'W'}[symbol])
		return
	}
	if (symbol == 'L' || stage.hasClass(symbol, nasal)) && stage.hasClass(at.soundAfter, vowel) {
		place, last := node, at.previous
		if stage.hasClass(at.previousSymbol, wordMark) {
			place, last = at.previous, at.previous.Previous
		}
		if last != nil && last.Previous != nil && (last.Symbol() == 'z' && last.Previous.Symbol() == 'J' || last.Symbol() == 's' && last.Previous.Symbol() == 'C') {
			at.result = stage.insert(place, false, ' ', 3)
			return
		}
	}
	if (at.soundBefore == 'S' || at.soundBefore == 'Z') && (symbol == 'M' || symbol == 'N' || stage.hasClass(symbol, approximant)) {
		frames := uint32(2)
		if at.soundBefore == 'S' {
			frames = 3
			if symbol == 'M' && at.nextSymbol == '.' {
				frames = 4
			}
		}
		at.result = stage.insert(node, false, ' ', frames)
		return
	}
	if stage.hasClass(symbol, stop) {
		stage.stopRules(at)
	}
}

// nextInWindow returns the node after the node in the window, or nil for none.
func (stage *Stage) nextInWindow(node *pipeline.Node) *pipeline.Node {
	if node == nil {
		return nil
	}
	return stage.Pipeline.Next(node)
}

// lRules are the rules that make an `L` the dark `j` or a syllabic `l`, and free an `L` after a juncture, and
// report whether one ended the rules (lexical.md §13.4, rules 6 to 8).
func (stage *Stage) lRules(at *around) bool {
	node, symbol := at.node, at.symbol
	if (symbol == 'L' || symbol == 'j') && at.nextSymbol == '[' && at.soundAfter == 'L' && at.soundBefore != '@' && at.soundBefore != '|' {
		stage.Pipeline.FreeNode(at.soundAfterNode, false)
	}
	if symbol != 'L' {
		return false
	}
	switch {
	case at.soundBefore == 'L' && (isWordMark(at.previousSymbol) || at.previousSymbol == '['),
		at.soundBefore == '@' && (at.soundAfter == 'D' || at.soundAfter == 'E') && !isWordMark(at.nextSymbol) && !isWordMark(at.previousSymbol),
		(at.previousSymbol == '@' || at.previousSymbol == '|') && at.nextSymbol == 'Z' &&
			(stage.hasClass(at.afterNextSymbol, wordMark) || stage.hasClass(at.afterNextSymbol, boundary)):
		node.SetSymbol('j')
		return true
	}
	if !stage.hasClass(at.previousSymbol, syllabic) || at.next == nil || stressOf(at.next) != 0 {
		return false
	}
	switch {
	case at.previousSymbol == 'y' && (isWordMark(at.nextSymbol) || stage.hasClass(at.nextSymbol, phraseBreak)):
		node.SetSymbol('l')
	case at.previousSymbol == 'b' && at.previous.Previous.Symbol() == 'L' && stage.hasClass(at.nextSymbol, vowel):
	default:
		node.SetSymbol('j')
	}
	return true
}

// stopRules are the rules for a stop (lexical.md §13.4, rule 15).
func (stage *Stage) stopRules(at *around) {
	list := stage.Pipeline
	node, symbol := at.node, at.symbol
	if (at.previousSymbol == 'S' || at.previousSymbol == '[' && at.soundBefore == 'S') && (symbol == 'P' || symbol == 'K' || symbol == 'T' && at.nextSymbol != 'R') &&
		at.next != nil && stressOf(at.next) != 0 {
		node.SetSymbol(map[byte]byte{'P': 'B', 'K': 'G', 'T': 'D'}[symbol])
		return
	}
	if (symbol == 'D' || symbol == 'T') && at.nextSymbol == 'R' && at.previousSymbol != 'S' {
		symbol = map[byte]byte{'D': 'J', 'T': 'C'}[symbol]
		node.SetSymbol(symbol)
	}
	beforeYou := isWordMark(at.nextSymbol) && at.soundAfter == 'Y' && stage.Word.Second() == pronounClass
	if symbol == 'D' && beforeYou {
		symbol = 'J'
		node.SetSymbol(symbol)
	}
	if symbol == 'T' && beforeYou {
		if at.previous != nil && at.previousSymbol != 's' && at.previous.Previous.Symbol() != 'C' {
			symbol = 'C'
			node.SetSymbol(symbol)
		}
		at.result = list.FreeNode(at.soundAfterNode, true)
	}
	if symbol == 'C' && at.nextSymbol != 's' || symbol == 'J' && at.nextSymbol != 'z' {
		at.result = stage.insert(node, true, map[byte]byte{'C': 's', 'J': 'z'}[symbol], 0)
		at.result.SetFlags(at.result.Flags() | node.Flags()&onsetFlag)
		return
	}
	if at.next != nil && (symbol == 'D' || symbol == 'T') && stage.alveolarRules(at, symbol) {
		return
	}
	if stage.hasClass(at.nextSymbol, boundary) && stage.cursor().Rate < 25 {
		at.result = stage.insert(node, true, 'p', 0)
	}
}

// alveolarRules are the rules for a `D` or `T` before a phoneme: they drop it, or make it `q`, `D` or a flap, and
// report whether one ended the rules (lexical.md §13.4, the alveolar rules).
func (stage *Stage) alveolarRules(at *around, symbol byte) bool {
	if at.previousSymbol != '~' && stage.alveolarChanges(at, symbol) {
		return true
	}
	if symbol == 'T' && at.nextSymbol == 'u' && isWordMark(at.previousSymbol) && stage.PreviousWord.Second() == shortPrepositionClass &&
		stage.hasClass(at.soundBefore, vowel) && (at.previous.Previous.Flags()&onsetFlag != 0 || stage.hasClass(at.soundBefore, high)) {
		if after := at.next.Next.Next; after != nil && !stage.hasClass(after.Symbol(), phraseBreak) && !stage.hasClass(after.Symbol(), wordMark) {
			flap(at.node, 'T')
			return true
		}
	}
	return false
}

// alveolarChanges are the alveolar rules but the last, and report whether one applied.
func (stage *Stage) alveolarChanges(at *around, symbol byte) bool {
	node := at.node
	cursor := stage.cursor()
	rate := cursor.Rate
	if symbol == 'T' && stage.hasClass(at.soundBefore, stop) && isWordMark(at.nextSymbol) && stage.hasClass(at.soundAfter, vowel) {
		node.SetSymbol('D')
		return true
	}
	if stage.dropsAlveolar(at, symbol) {
		at.result = stage.Pipeline.FreeNode(node, false)
		at.flagNext = true
		return true
	}
	sonorantBefore := stage.hasClass(at.previousSymbol, sonorant)
	if sonorantBefore && at.previous.Flags()&onsetFlag != 0 && stage.hasClass(at.nextSymbol, neutral) && at.afterNextSymbol == 'N' {
		if symbol == 'T' {
			node.SetSymbol('q')
		} else {
			flap(node, symbol)
		}
		return true
	}
	if symbol == 'T' {
		if sonorantBefore && (stage.hasClass(at.nextSymbol, wordMark) && stage.hasClass(at.soundAfter, sonorant) && !stage.hasClass(at.soundAfter, syllabic) && at.soundAfter != 'H' ||
			at.soundAfter == 'D' || at.soundAfter == 'L') {
			at.flagNext = true
			node.SetSymbol('q')
			return true
		}
		if at.soundAfter == 'x' && stage.hasClass(at.previousSymbol, vowel) {
			node.SetSymbol('q')
			return true
		}
	}
	after := at.soundAfter
	if rate > 6 && cursor.Phrase != 0 {
		if stage.hasClass(at.previousSymbol, vowel) && at.previous.Previous.Symbol() != 't' && (stage.hasClass(after, vowel) || after == 'l') &&
			at.next.Flags()&onsetFlag == 0 && (stage.hasClass(after, reduced) || after == 'E' || after == 'O') &&
			(at.afterNextSymbol != 'N' || stage.hasClass(at.afterNext.Next.Symbol(), phoneme)) {
			flap(node, symbol)
			return true
		}
		if symbol == 'T' && sonorantBefore && (isWordMark(at.nextSymbol) || at.nextSymbol == '[' && at.previousSymbol != 'L') &&
			(stage.hasClass(after, vowel) || after == 'H') {
			class := stage.PreviousWord.Second()
			at.flagNext = true
			if class != 0 && class != 6 || !stage.hasClass(at.soundBefore, nasal) && at.soundBefore != 'j' ||
				after == 'H' && at.previous.Previous.Flags()&onsetFlag == 0 {
				flap(node, 'T')
			} else {
				node.Data[originalOffset] = 'T'
			}
			return true
		}
	}
	if symbol == 'D' && !stage.hasClass(at.previousSymbol, wordMark) && stage.hasClass(at.soundBefore, vowel) && stage.hasClass(after, vowel) &&
		at.next.Flags()&onsetFlag == 0 && (at.nextSymbol != 'A' || at.next.Next.Symbol() != 'T') {
		flap(node, 'D')
		return true
	}
	return false
}

// dropsAlveolar reports whether a `D` or `T` goes: before the affricate it starts, or before a consonant after
// some consonants at the faster rates (lexical.md §13.4, the alveolar rules).
func (stage *Stage) dropsAlveolar(at *around, symbol byte) bool {
	rate := stage.cursor().Rate
	before, after := at.soundBefore, at.soundAfter
	if symbol == 'T' && at.nextSymbol == 'C' || symbol == 'D' && at.nextSymbol == 'J' {
		return true
	}
	if rate >= 14 && stage.hasClass(after, consonant) && !stage.hasClass(at.previousSymbol, wordMark) && after != 'R' && after != 'W' &&
		(symbol == 'T' && strings.IndexByte("FSPK", before) >= 0 || symbol == 'D' && before == 'N' || rate >= 25 && strings.IndexByte("szV", before) >= 0) {
		return true
	}
	return rate >= 10 && at.nextSymbol == '[' && symbol == 'T' && (before == 'N' || before == 'n') && after == 'S'
}

// flap makes the stop a flap `t`, whose byte 0x18 keeps the stop.
func flap(node *pipeline.Node, stop byte) {
	node.SetSymbol('t')
	node.Data[originalOffset] = stop
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
