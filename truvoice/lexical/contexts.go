package lexical

import (
	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
)

// The kinds of a clause's test, its high three bits (lexical.md §8.1): a word mark before the nearest vowel, or
// not; a single vowel before the place, or not; letter-to-sound's consonants; a doubled consonant. Other tests
// compare the nodes with the clause's elements.
const (
	testKind            = 0xe0
	testConsonants      = 0x20
	testMarkBeforeVowel = 0x40
	testNoMark          = 0x60
	testDoubled         = 0x80
	testOneVowel        = 0xa0
	testNotOneVowel     = 0xc0
)

// The bits of an element test: necessary rather than sufficient, its result inverted, one node for each element,
// the nodes after the first two, and the bits that end a sequence at its first difference.
const (
	testNecessary = 0x08
	testInverted  = 0x04
	testSequence  = 0x10
	testCount     = 0x03
	testUntilDiff = 0x1c
)

// contextMatches reports whether the nodes next to a node are in a context: before it, or after it when forward
// (lexical.md §8.1).
func (stage *Stage) contextMatches(context datapack.Context, at *pipeline.Node, forward bool) bool {
	matches := true
	for index := 0; index < len(context) && !context[index].End; {
		clause := &context[index]
		necessary, passesOver := false, false
		switch kind := clause.Test & testKind; kind {
		case testConsonants:
			return stage.Consonants > 1
		case testMarkBeforeVowel, testNoMark:
			matches = stage.markBeforeVowel(at)
			if kind == testNoMark {
				matches, necessary = !matches, true
			}
		case testOneVowel, testNotOneVowel:
			matches = stage.vowelsBefore(at) == 1
			if kind == testNotOneVowel {
				matches, necessary = !matches, true
			}
		case testDoubled:
			matches = stage.doubledConsonant(at, forward)
		default:
			matches, passesOver = stage.elementsMatch(clause, at, forward)
			necessary = clause.Test&testNecessary != 0
		}
		if matches != necessary {
			return matches
		}
		index++
		if passesOver {
			index++
		}
	}
	return matches
}

// markBeforeVowel reports whether a word mark is among the nodes before a node, back to the nearest vowel or Y
// after a consonant.
func (stage *Stage) markBeforeVowel(at *pipeline.Node) bool {
	list := stage.Pipeline
	marked := false
	for node := list.Previous(at); node != nil; node = list.Previous(node) {
		symbol := node.Symbol()
		if stage.hasClass(symbol, vowel) {
			break
		}
		if stage.hasClass(symbol, wordMark) {
			marked = true
		}
		if symbol == 'Y' && stage.hasClass(node.Previous.Symbol(), consonant) {
			break
		}
	}
	return marked
}

// vowelsBefore counts the vowels, and Ys after a consonant, among the nodes before a node, back to the nearest
// word mark.
func (stage *Stage) vowelsBefore(at *pipeline.Node) int {
	list := stage.Pipeline
	vowels := 0
	for node := list.Previous(at); node != nil; node = list.Previous(node) {
		symbol := node.Symbol()
		if stage.hasClass(symbol, vowel) {
			vowels++
		}
		if stage.hasClass(symbol, wordMark) {
			break
		}
		if symbol == 'Y' && stage.hasClass(node.Previous.Symbol(), consonant) {
			vowels++
		}
	}
	return vowels
}

// doubledConsonant reports whether the node before a node, or after it when forward, is a consonant the node
// beyond it in the list doubles.
func (stage *Stage) doubledConsonant(at *pipeline.Node, forward bool) bool {
	list := stage.Pipeline
	var node, beyond *pipeline.Node
	if forward {
		if node = list.Next(at); node == nil {
			return false
		}
		beyond = node.Next
	} else {
		if node = list.Previous(at); node == nil {
			return false
		}
		beyond = node.Previous
	}
	return stage.hasClass(node.Symbol(), consonant) && node.Symbol() == beyond.Symbol()
}

// elementsMatch compares the nodes before a node, or after it, with a clause's elements: one node with them all,
// or a node with each in turn. It reports the clause's result, and whether it read past its last element, which
// makes the context pass over the next item.
func (stage *Stage) elementsMatch(clause *datapack.Clause, at *pipeline.Node, forward bool) (bool, bool) {
	list := stage.Pipeline
	test := clause.Test
	sequence := test&testSequence != 0
	count := 1
	if sequence {
		count = int(test&testCount) + 2
	}
	inverted := test&testInverted != 0
	byDifference := inverted == (test&testNecessary != 0)
	matches, passesOver := !inverted, false
	element := 0
	node := at
	for position := 1; position <= count; position++ {
		if forward {
			node = list.Next(node)
		} else {
			node = list.Previous(node)
		}
		if node == nil {
			return false, passesOver
		}
		for {
			if element == len(clause.Elements) {
				passesOver = true
				break
			}
			decides := stage.elementMatches(clause.Elements[element], node.Symbol()) != byDifference
			element++
			if decides {
				matches = inverted
				break
			}
			if sequence {
				break
			}
		}
		if test&testUntilDiff == testUntilDiff && matches {
			break
		}
	}
	return matches, passesOver
}

// elementMatches reports whether a symbol is an element's symbol, or in its class.
func (stage *Stage) elementMatches(element datapack.Element, symbol byte) bool {
	if element.Symbol != 0 {
		return symbol == element.Symbol
	}
	return symbol < datapack.SymbolCount && stage.classes[element.Plane][symbol]&element.Bits != 0
}
