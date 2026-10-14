package prosody

import (
	"slices"
	"strings"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
)

// vowelGrids are the vowels in the order of their duration grids.
const vowelGrids = "EiAeaowOubvIfUygkcr345@|"

// A duration grid's columns; the first row of an unstressed vowel's, and the rows a vowel in the phrase's last word
// skips. The columns of a vowel between approximants, before W or Y, and before a voiced closure after a glide.
const (
	gridColumns        = 20
	unstressedRow      = 4
	lastWordRows       = 2
	approximantsColumn = 2
	glidesColumn       = 15
	glideClosureColumn = 18
)

// Where a stressed vowel's break reductions start.
const stressedReductions = 23

// The places in a node's data of byte 0x17, a phoneme's syllable in its word, and byte 0x19, a vowel's word class.
const (
	syllableOffset  = 0x0f
	wordClassOffset = 0x11
)

// The conditions of a function word's rule, a bit each: the vowel's phrase starts at its word or doesn't, its
// word is the phrase's last or isn't, the phrase doesn't end in a question mark, the vowel is unstressed or
// stressed.
const (
	phraseStartCondition = 1 << iota
	lastWordCondition
	notPhraseStartCondition
	notLastWordCondition
	notQuestionCondition
	unstressedCondition
	stressedCondition
)

func among(symbol byte, symbols string) bool {
	return strings.IndexByte(symbols, symbol) >= 0
}

func isGlideOrJ(symbol byte) bool {
	return isGlide(symbol) || symbol == 'j'
}

func isReducedVowel(symbol byte) bool {
	return among(symbol, "iv|@")
}

// symbolOrZero is a node's symbol, or 0 for none.
func symbolOrZero(node *pipeline.Node) byte {
	if node == nil {
		return 0
	}
	return node.Symbol()
}

// before returns the node count places before the node in the list, or none.
func before(node *pipeline.Node, count int32) *pipeline.Node {
	for ; node != nil && count > 0; count-- {
		node = node.Previous
	}
	return node
}

// after returns the node count places after the node in the list, or none.
func after(node *pipeline.Node, count int32) *pipeline.Node {
	for ; node != nil && count > 0; count-- {
		node = node.Next
	}
	return node
}

// spells reports whether the node and the nodes after it in the list have the symbols.
func spells(node *pipeline.Node, symbols string) bool {
	for index := range len(symbols) {
		if node == nil || node.Symbol() != symbols[index] {
			return false
		}
		node = node.Next
	}
	return true
}

// flapped is T for a flap whose node's byte 0x18 is T, and otherwise the flap.
func flapped(node *pipeline.Node) byte {
	if node != nil && node.Data[originalOffset] == 'T' {
		return 'T'
	}
	return flap
}

// findClass returns the first of up to count phonemes after the node going forward, or before it going back, in
// the class.
func (stage *Stage) findClass(node *pipeline.Node, forward bool, count int32, kind class) *pipeline.Node {
	for ; count > 0; count-- {
		if forward {
			node = stage.nextPhoneme(node)
		} else {
			node = stage.previousPhoneme(node)
		}
		if node == nil || stage.inClass(node, kind) {
			return node
		}
	}
	return nil
}

// startsPhrase reports whether the word mark before the node, up to 16 phonemes back, comes after a P command or
// a comma.
func (stage *Stage) startsPhrase(node *pipeline.Node) bool {
	mark := stage.findClass(node, false, 16, wordMark)
	if mark == nil || mark.Previous == nil {
		return false
	}
	previous := mark.Previous
	return previous.Symbol() == 'P' && previous.Type() == pipeline.Control || previous.Symbol() == ','
}

// gridValue is the byte at the index in the grid; before the grids' bytes it is 0.
func (stage *Stage) gridValue(grid int, index int32) int32 {
	position := stage.tables.GridOffsets[grid] + index
	if position < 0 {
		return 0
	}
	return int32(stage.tables.DurationGrids[position])
}

// gridOrDuration is the byte at the index in the grid, or when it is 0 the symbol's inherent duration if stressed and
// its shortest one if not.
func (stage *Stage) gridOrDuration(grid int, index int32, symbol byte, stressed bool) int32 {
	if duration := stage.gridValue(grid, index); duration != 0 {
		return duration
	}
	durations := stage.durations(symbol)
	if stressed {
		return durations.Inherent
	}
	return durations.Shortest
}

// VowelDuration is a vowel's duration in frames: its grid's value for the sounds after it, whether it is
// stressed, in the phrase's last word and inside its word, less what its glides, nasals and laterals, its stress
// and its function word take (prosody.md §6.7).
func (stage *Stage) VowelDuration() int32 {
	context := &stage.Context
	node := stage.cursor().Scan
	symbol, next := context.Symbol, context.NextSymbol
	preceding, beforePreceding := context.PrecedingSymbol, context.BeforePrecedingSymbol
	following, afterFollowing := context.FollowingSymbol, context.AfterFollowingSymbol
	afterNextNode := stage.nextPhoneme(context.Next)
	afterNext := symbolOrZero(afterNextNode)
	if following == flap {
		following = flapped(node.Next)
	}
	if afterFollowing == flap {
		afterFollowing = flapped(after(node, 2))
	}
	if symbol == 'p' {
		switch preceding {
		case 'B':
			return 3
		case 'D', 'M', 'N', 'P':
			return 4
		case 'G':
			return 5
		case 'K':
			return 8
		case 'T':
			if stage.hasClass(beforePreceding, nasal) {
				return 4
			}
			return 6
		case '~':
			return 6
		}
		return 7
	}
	stressed := node.Flags()&stressedFlag != 0
	medial := !stage.hasClass(next, wordMark) && following != 'p' && following != ' ' && following != ',' &&
		!stage.hasClass(next, boundary)
	lastWord := stage.scanForClass(true, 16, phraseBreak, -wordMark, byClass)
	if following == ' ' {
		following = afterFollowing
		afterFollowing = symbolOrZero(stage.nextSound(context.AfterFollowing))
	}
	if lastWord && medial && !stage.hasClass(next, vowel) && !among(next, "lmn") {
		medial = stage.syllableFollows(afterNextNode)
	}
	if following == flap {
		medial = true
	} else if symbol == 'y' && next == 'l' && (isWordMark(afterNext) || stage.hasClass(afterNext, phraseBreak)) {
		medial = false
	}
	grid := max(strings.IndexByte(vowelGrids, symbol), 0)
	firstRow := int32(0)
	if !stressed {
		firstRow = unstressedRow
	}
	row := firstRow
	if lastWord {
		row += lastWordRows
	}
	if medial {
		row++
	}
	column := stage.vowelColumn(preceding, beforePreceding, following, afterFollowing, afterNext)
	duration := stage.gridValue(grid, row*gridColumns+column)
	beforeFortisStop := stage.hasClass(afterNext, fortis) && stage.hasClass(afterNext, closure) || afterNext == 'q'
	if column == glideClosureColumn && stage.hasClass(next, nasal) && beforeFortisStop {
		duration -= nasalReduction(symbol, stressed, lastWord, medial)
	}
	if column == approximantsColumn && stage.hasClass(next, lateral) && beforeFortisStop {
		duration -= lateralReduction(symbol, stressed, lastWord, medial)
	}
	if stage.hasClass(symbol, frontEnding) && stage.hasClass(next, vowel) {
		duration += 3
	}
	if !isReducedVowel(symbol) && isGlideOrJ(preceding) && stage.hasClass(following, vowel) {
		switch {
		case lastWord && stressed:
			duration -= 2
		case lastWord || stressed:
			duration--
		}
	}
	if duration == 0 {
		duration = stage.gridOrDuration(grid, (firstRow+1)*gridColumns+glidesColumn, symbol, stressed)
		if stress(node) == 1 {
			duration -= 2
		}
		return max(duration, shortestByClass)
	}
	if stress(node) == 1 {
		duration -= 2
	}
	if isReducedVowel(symbol) && isGlideOrJ(preceding) && (stage.hasClass(following, vowel) || isGlideOrJ(following)) &&
		stressed {
		duration--
	}
	if node.Data[syllableOffset] == 1 && !isReducedVowel(symbol) && isGlideOrJ(context.PreviousSymbol) {
		cluster := symbolOrZero(before(node, 2))
		if stage.hasClass(cluster, closure) && stage.hasClass(cluster, consonant) && symbolOrZero(before(node, 3)) == 'S' {
			duration--
			if lastWord {
				duration--
			}
		}
	}
	if stage.hasClass(next, phraseBreak) && isGlideOrJ(preceding) {
		reduction := grid
		if stressed {
			reduction += stressedReductions
		}
		duration -= stage.tables.BreakReductions[reduction]
	}
	wordClass := node.Data[wordClassOffset]
	if wordClass != 0 {
		duration -= stage.FunctionWordVowelRule(following, lastWord, stressed)
		duration -= stage.functionWordReduction(wordClass, preceding, following, lastWord, stressed)
	}
	if wordClass == 10 && symbol == 'u' {
		duration = min(duration, 6)
		if among(following, "PTK") || following == 'S' && among(afterFollowing, "PTKBDG") {
			duration = 3
		}
	}
	if symbol == 'v' && symbolOrZero(node.Next) == 'N' && symbolOrZero(node.Previous) == '&' {
		duration = max(duration, 8)
	}
	return max(duration, shortestByClass)
}

// syllableFollows reports whether a vowel but p, or l, m or n, comes in the list from the node on, before a word
// mark or a sentence's end.
func (stage *Stage) syllableFollows(node *pipeline.Node) bool {
	for ; node != nil && !among(node.Symbol(), "&%.?!"); node = node.Next {
		if symbol := node.Symbol(); (stage.hasClass(symbol, vowel) || among(symbol, "lmn")) && symbol != 'p' {
			return true
		}
	}
	return false
}

// vowelColumn is the column of a vowel's grid for the sounds before and after it.
func (stage *Stage) vowelColumn(preceding, beforePreceding, following, afterFollowing, afterNext byte) int32 {
	if isGlideOrJ(preceding) && (stage.hasClass(following, fricative) || stage.hasClass(following, closure)) {
		column := int32(16)
		if stage.hasClass(following, voiced) {
			column = glideClosureColumn
		}
		if !stage.hasClass(following, closure) {
			column++
		}
		return column
	}
	switch {
	case preceding == ' ' && beforePreceding == ' ' && stage.hasClass(following, boundary):
		return 1
	case !stage.hasClass(following, phoneme):
		return 0
	case stage.hasClass(preceding, approximant) && stage.hasClass(following, approximant):
		return approximantsColumn
	case stage.hasClass(following, vowel):
		return 13
	}
	switch following {
	case 'C', 'K', 'P', 'T', 'q':
		return 3
	case 'F', 'H', 'S', 'X', 's':
		return 4
	case 'B', 'D', 'G', 'J', 't':
		return 5
	case 'V', 'Z', 'd', 'x', 'z':
		return 6
	case 'L', 'j', 'l':
		return stage.columnBeforeSonorant(afterFollowing, afterNext, 11, 7, 8)
	case 'M', 'N', 'm', 'n', '~':
		return stage.columnBeforeSonorant(afterFollowing, afterNext, 12, 9, 10)
	case 'R':
		return 14
	case 'W', 'Y':
		return glidesColumn
	}
	return -1
}

// columnBeforeSonorant is the column of a vowel before a lateral or a nasal: before no stop or at the word's end,
// before a voiceless stop or q, or before a voiced stop.
func (stage *Stage) columnBeforeSonorant(afterFollowing, afterNext byte, noStopColumn, voicelessColumn, voicedColumn int32) int32 {
	switch {
	case !stage.hasClass(afterFollowing, stop) || isWordMark(afterNext):
		return noStopColumn
	case !stage.hasClass(afterFollowing, voiced) || afterFollowing == 'q':
		return voicelessColumn
	}
	return voicedColumn
}

// byPlace is a reduction for a vowel not in the phrase's last word, inside its word in it, or at its end.
func byPlace(lastWord, medial bool, other, inside, end int32) int32 {
	switch {
	case !lastWord:
		return other
	case medial:
		return inside
	}
	return end
}

// nasalReduction is the frames a vowel loses after a glide, before a nasal and a fortis stop.
func nasalReduction(symbol byte, stressed, lastWord, medial bool) int32 {
	switch {
	case !stressed:
		switch {
		case among(symbol, "@|"):
			return 0
		case among(symbol, "3aeiuv"):
			return byPlace(lastWord, medial, 1, 2, 4)
		case among(symbol, "AEObow"):
			return byPlace(lastWord, medial, 2, 3, 6)
		}
		return byPlace(lastWord, medial, 3, 4, 7)
	case !lastWord:
		switch {
		case among(symbol, "3@v|"):
			return 1
		case among(symbol, "AEIOabefiu"):
			return 3
		case among(symbol, "ow"):
			return 4
		}
		return 2
	case medial:
		switch {
		case among(symbol, "3@|"):
			return 1
		case among(symbol, "AEiu"):
			return 3
		case among(symbol, "Oabeovw"):
			return 4
		}
		return 2
	}
	switch {
	case among(symbol, "@|"):
		return 0
	case among(symbol, "iuv"):
		return 7
	case among(symbol, "3AEOabefow"):
		return 10
	}
	return 12
}

// lateralReduction is the frames a vowel loses between approximants, before a lateral and a fortis stop.
func lateralReduction(symbol byte, stressed, lastWord, medial bool) int32 {
	switch {
	case !stressed && lastWord && !medial:
		switch {
		case among(symbol, "@|"):
			return 1
		case among(symbol, "AEOabeiouvw"):
			return 2
		}
		return 4
	case !stressed:
		switch {
		case among(symbol, "@u|"):
			return 0
		case among(symbol, "AEOabeiovw"):
			return 1
		}
		return 3
	case !lastWord:
		switch {
		case among(symbol, "@e|"):
			return 0
		case among(symbol, "biuv"):
			return 1
		case among(symbol, "3AEOaow"):
			return 2
		}
		return 3
	case medial:
		switch {
		case among(symbol, "@e|"):
			return 0
		case among(symbol, "biuv"):
			return 1
		case among(symbol, "3AO"):
			return 2
		case among(symbol, "Eaow"):
			return 3
		}
		return 4
	}
	switch {
	case among(symbol, "@|"):
		return 0
	case among(symbol, "beiuvw"):
		return 2
	case among(symbol, "3AEOao"):
		return 3
	}
	return 4
}

// functionWordReduction is the frames a function word's vowel loses by the words around it.
func (stage *Stage) functionWordReduction(wordClass, preceding, following byte, lastWord, stressed bool) int32 {
	node := stage.cursor().Scan
	symbol := node.Symbol()
	markClass := func(from *pipeline.Node, classes ...byte) bool {
		mark := stage.findClass(from, false, 16, wordMark)
		return mark != nil && slices.Contains(classes, mark.Second())
	}
	frames := int32(0)
	if wordClass == 15 && symbol == 'k' && preceding == 'x' && stage.hasClass(symbolOrZero(node.Next), phraseBreak) &&
		lastWord && stressed && markClass(before(node, 3), 2, 8, 9, 11, 12) {
		frames += 10
	}
	if wordClass == 1 && symbol == 'v' && following == 'S' && markClass(before(node, 2), 3, 4, 10, 12) {
		frames += 3
	}
	if wordClass == 5 && symbol == 'a' && preceding == 'x' && !lastWord && !stage.startsPhrase(node) {
		if mark := stage.findClass(before(node, 2), false, 16, wordMark); mark != nil {
			byMark := mark.Second() == 7 && !spells(mark.Next, "wj") && !spells(before(node, 5), "Ex3")
			nextWordClass := byte(0)
			if nextWord := after(node, 2); nextWord != nil {
				nextWordClass = nextWord.Second()
			}
			switch nextWordClass {
			case 1, 5:
				byMark = true
			case 2:
				byMark = byMark || !spells(after(node, 3), "KaN")
			case 13:
				byMark = byMark || !spells(after(node, 3), "WiCs")
			}
			if byMark {
				frames += 4
			}
		}
	}
	if wordClass == 1 && symbol == 'E' {
		if preceding != 'W' && among(following, "jDt") {
			frames += 3
		}
		if preceding == 'W' && stage.hasClass(following, consonant) && !isGlideOrJ(following) {
			frames += 3
		}
	}
	return frames
}

// FunctionWordVowelRule is the frames a function word's vowel loses by the first of its word class's rules that it
// passes (prosody.md §6.8).
func (stage *Stage) FunctionWordVowelRule(following byte, lastWord, stressed bool) int32 {
	node := stage.cursor().Scan
	symbol := node.Symbol()
	wordClass := node.Data[wordClassOffset]
	if stage.hasClass(following, approximant) {
		if wordClass == 13 && symbol == 'k' || wordClass == 12 && symbol == '3' || wordClass == 7 && symbol == 'b' {
			return 0
		}
		if stage.hasClass(symbolOrZero(node.Previous), wordMark) && stage.hasClass(symbolOrZero(before(node, 2)), approximant) &&
			(wordClass == 12 && symbol == 'r' || wordClass == 14 && symbol == 'g' || wordClass == 7 && symbolOrZero(node.Next) == 'j') {
			return 0
		}
	}
	if int(wordClass) >= len(stage.tables.FunctionWordRules) {
		return 0
	}
	conditions := int32(unstressedCondition)
	if stressed {
		conditions = stressedCondition
	}
	if stage.startsPhrase(node) {
		conditions |= phraseStartCondition
	} else {
		conditions |= notPhraseStartCondition
	}
	if lastWord {
		conditions |= lastWordCondition
	} else {
		conditions |= notLastWordCondition
	}
	if end := stage.findClass(node, true, 16, phraseBreak); end == nil || end.Symbol() != '?' {
		conditions |= notQuestionCondition
	}
	for _, rule := range stage.tables.FunctionWordRules[wordClass] {
		if rule.Conditions&^conditions == 0 && stage.passes(rule, node, following) {
			return rule.Frames
		}
	}
	return 0
}

// passes reports whether the nodes around the vowel's node spell the rule's symbols and pass its tests.
func (stage *Stage) passes(rule datapack.FunctionWordRule, vowel *pipeline.Node, following byte) bool {
	node := before(vowel, rule.Back)
	if !spells(node, rule.Symbols) {
		return false
	}
	if node = after(node, int32(len(rule.Symbols))); node == nil {
		return false
	}
	grouped, inGroup := false, false
	for _, test := range rule.Tests {
		if test.Direction == 1 {
			node = vowel
			for range test.Steps {
				if node = stage.nextPhoneme(node); node == nil {
					return false
				}
			}
		}
		kind := class(test.Value)
		switch test.Kind {
		case datapack.NodeInClass:
			if !stage.hasClass(node.Symbol(), kind) {
				return false
			}
		case datapack.NodeInWordClass:
			grouped = true
			inGroup = inGroup || int32(node.Second()) == test.Value
		case datapack.NodeNotInWordClass:
			if int32(node.Second()) == test.Value {
				return false
			}
		case datapack.SoundInClass:
			if !stage.hasClass(following, kind) {
				return false
			}
		case datapack.SoundNotInClass:
			if stage.hasClass(following, kind) {
				return false
			}
		case datapack.SoundNotSymbol:
			if int32(following) == test.Value {
				return false
			}
		}
	}
	return !grouped || inGroup
}
