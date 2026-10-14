package lexical

import "github.com/phyce/OpenSAPI4/truvoice/pipeline"

// fullVowel is the bit of Following that a vowel kept full sets.
const fullVowel = 0x4000

// ReduceVowel settles the vowel letter-to-sound wrote last, once it writes the vowel before it, or, when final,
// at the end of a rule after the stem's start (lexical.md §11): a U becomes b, after a Y or a consonant that takes the Y in, and a
// vowel that may be reduced becomes a schwa, a barred i or a 3, unless the nodes around it keep it full.
func (stage *Stage) ReduceVowel(final bool) {
	nucleus := stage.Vowel
	before := nucleus.Previous
	beforeSymbol := before.Symbol()
	if nucleus.Symbol() == 'U' {
		stage.splitU(nucleus, before, final)
		if beforeSymbol == 'H' {
			stage.Reducible = false
		}
	}
	if stage.Reducible {
		stage.reduce(nucleus, before, beforeSymbol)
	}
	stage.Vowel = nil
}

// splitU makes a U b, with a Y before it, or with the consonant before it made palatal.
func (stage *Stage) splitU(nucleus, before *pipeline.Node, final bool) {
	symbol := before.Symbol()
	nucleus.SetSymbol('b')
	switch {
	case !stage.hasClass(symbol, alveolar) && !stage.hasClass(symbol, approximant):
		if !stage.hasClass(symbol, palatal) && !stage.hasClass(symbol, vowel) && symbol != 'X' {
			stage.Pipeline.NewNode(nucleus, false, pipeline.Phoneme, 'Y')
		}
	case symbol == 'S':
		if after := nucleus.Next.Symbol(); after == 'R' || after != 'L' && !final {
			before.SetSymbol('s')
		}
	default:
		after := nucleus.Next.Symbol()
		ends := stage.hasClass(after, wordMark) || stage.hasClass(after, phraseBreak) || after == ' '
		if nucleus.Flags()&stressBits != 0 || final || ends && symbol != 'N' {
			return
		}
		switch symbol {
		case 'D':
			before.SetSymbol('J')
		case 'N':
			stage.Pipeline.NewNode(nucleus, false, pipeline.Phoneme, 'Y')
		case 'T':
			before.SetSymbol('C')
		case 'Z':
			before.SetSymbol('z')
		}
	}
}

// reduce makes a vowel that may be reduced a schwa, a barred i or a 3, unless the nodes around it keep it full.
// An E before it after an L becomes a Y.
func (stage *Stage) reduce(nucleus, before *pipeline.Node, beforeSymbol byte) {
	list := stage.Pipeline
	context := stage.tables.ReductionContext
	symbol := nucleus.Symbol()
	if beforeSymbol == 'E' && before.Previous.Symbol() == 'L' && symbol != 'v' && symbol != 'a' && symbol != 'e' &&
		!stage.contextMatches(context, before, false) {
		before.SetSymbol('Y')
	}
	var next, beyond byte
	if node := list.Next(nucleus); node != nil {
		next = node.Symbol()
		if node = list.Next(node); node != nil {
			beyond = node.Symbol()
		}
	}
	unstressed := stage.StressPattern == 0
	starts := stage.contextMatches(context, nucleus, false)
	if !starts && symbol == 'O' && stage.hasClass(next, consonant) && (stage.hasClass(beyond, wordMark) || beyond == 0) && unstressed {
		return
	}
	closed := stage.hasClass(beyond, consonant) && (!stage.hasClass(next, stop) || beyond != 'R') && (next != 'K' || beyond != 'W') &&
		next != beyond && (!stage.hasClass(before.Previous.Symbol(), wordMark) || beforeSymbol != 'K' || symbol != 'o' || next != 'N')
	if starts && !stage.AfterStem && unstressed && stage.hasClass(next, consonant) && (closed || symbol == 'b' || symbol == 'O' && beyond != '[') {
		stage.Following |= fullVowel
		return
	}
	if !stage.hasClass(symbol, reduced) {
		nucleus.SetSymbol(stage.reducedVowel(symbol, beforeSymbol, next))
	}
}

// reducedVowel is the reduced vowel a vowel becomes between symbols.
func (stage *Stage) reducedVowel(symbol, beforeSymbol, next byte) byte {
	switch symbol {
	case 'E', 'I', 'i':
		return '|'
	case 'e':
		if next == 'L' || next == 'R' {
			return '@'
		}
		return '|'
	case 'g', 'k', 'r':
		return '3'
	}
	if (stage.hasClass(next, palatal) || stage.hasClass(next, velar)) && !stage.hasClass(beforeSymbol, wordMark) ||
		stage.hasClass(beforeSymbol, palatal) && next == 'S' {
		return '|'
	}
	return '@'
}
