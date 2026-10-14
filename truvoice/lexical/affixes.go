package lexical

import (
	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
)

// The node that marks an affix off from its stem, the affix stress of the endings of foreign names, and the
// least affix stress an affix gives the word rather than its header.
const (
	affixMark        = '['
	foreignEnding    = 2
	wordAffixStress  = 2
	shortestStemNode = 2
)

// Lookup looks the word up whole in the user lexicon and the dictionary, and reports whether it found it.
// Otherwise it strips the word's suffixes, then its prefixes, and looks up the stems they leave; a stem found
// leaves the affixes' letters to letter-to-sound (lexical.md §9).
func (stage *Stage) Lookup() bool {
	cursor := stage.cursor()
	cursor.Working = [2]*pipeline.Node{stage.FirstLetter, stage.LastNode}
	if stage.UserLexiconLookup() || stage.DictionaryLookup() {
		return true
	}
	stage.TriedE = false
	stage.Affix = nil
	if !stage.stripAffixes() && stage.TriedE {
		cursor.Working[1] = stage.Pipeline.NewNode(cursor.Working[1], true, pipeline.Letter, 'E')
	}
	return false
}

// stripAffixes strips the word's suffixes, then its prefixes, and reports whether it found a stem one left.
func (stage *Stage) stripAffixes() bool {
	cursor := stage.cursor()
	limit := stage.shortestStem(stage.FirstLetter, stage.LastNode, true)
	if limit == stage.LastNode {
		return false
	}
	list := affixList(&stage.tables.Suffixes, cursor.Working[1].Symbol())
	for first := true; list != nil; first = false {
		affix := stage.matchAffix(cursor.Working[1], limit, *list, false)
		stage.Affix = affix
		if affix == nil {
			break
		}
		if first {
			switch affix.Class {
			case verbClass, adverbClass:
				stage.WordClass = affix.Class
			case negationClass:
				stage.WordClass = affix.Class
				stage.Word.SetSymbol('%')
			}
		}
		if affix.Lookup && (stage.UserLexiconLookup() || stage.DictionaryLookup() || stage.restoreStem(true)) {
			return true
		}
		list = affix.Next
	}
	limit = stage.shortestStem(cursor.Working[1], stage.FirstLetter, false)
	if limit == stage.FirstLetter || stage.AffixStress == foreignEnding {
		return false
	}
	list = affixList(&stage.tables.Prefixes, cursor.Working[0].Symbol())
	for list != nil {
		affix := stage.matchAffix(cursor.Working[0], limit, *list, true)
		if affix == nil {
			break
		}
		if affix.Lookup && (stage.DictionaryLookup() || stage.restoreStem(false)) {
			return true
		}
		list = affix.Next
	}
	return false
}

// affixList returns the list of affixes of a letter, or nil.
func affixList(lists *[datapack.AffixLetters]*datapack.AffixList, letter byte) *datapack.AffixList {
	if letter < '@' || letter > 'Z' {
		return nil
	}
	return lists[letter-'@']
}

// shortestStem returns the node a stem keeps, walking from one end of the word towards the other: the third, or
// the first after it that isn't a consonant when none of the first three is; or the other end, when the walk
// reaches it first.
func (stage *Stage) shortestStem(from, end *pipeline.Node, forward bool) *pipeline.Node {
	node, vowel := from, false
	for index := 0; node != end; index++ {
		vowel = vowel || !stage.hasClass(node.Symbol(), consonant)
		if vowel && index >= shortestStemNode {
			break
		}
		if forward {
			node = node.Next
		} else {
			node = node.Previous
		}
	}
	return node
}

// matchAffix finds the first affix of a list whose letters the word has from a node on, short of the stem's
// shortest end, and whose context the nodes beyond its letters are in. It marks the affix off from the stem,
// which it takes from the cursor's working nodes, and applies its stress (lexical.md §9.2).
func (stage *Stage) matchAffix(from, limit *pipeline.Node, list datapack.AffixList, prefix bool) *datapack.Affix {
	for _, affix := range list {
		node, matched := affixLetters(affix.Letters, from, limit, prefix)
		if !matched || !stage.contextMatches(affix.Context, node, prefix) {
			continue
		}
		cursor := stage.cursor()
		if prefix {
			cursor.Working[0] = node.Next
		} else {
			cursor.Working[1] = node.Previous
		}
		stage.Pipeline.NewNode(node, prefix, pipeline.Letter, affixMark)
		if affix.Stress < wordAffixStress {
			word := stage.Word
			word.SetFlags(word.Flags()&^stressBits | uint32(affix.Stress)&3<<stressShift)
		} else {
			stage.AffixStress = byte(affix.Stress)
		}
		return affix
	}
	return nil
}

// affixLetters reports whether the letters from a node on are an affix's, a prefix's read forward and a suffix's
// back from its end, without reaching the limit, and returns the node of the affix's letter farthest from it.
func affixLetters(letters string, from, limit *pipeline.Node, prefix bool) (*pipeline.Node, bool) {
	letter := func(position int) byte {
		if prefix {
			return letters[position]
		}
		return letters[len(letters)-1-position]
	}
	node := from
	if letter(0) != node.Symbol() {
		return node, false
	}
	for position := 1; node != limit; position++ {
		if position == len(letters) {
			return node, true
		}
		if prefix {
			node = node.Next
		} else {
			node = node.Previous
		}
		if letter(position) != node.Symbol() {
			break
		}
	}
	return node, false
}

// restoreStem looks up the stem an affix left with the letters the affix may have changed: a doubled consonant
// single, a silent E, a final I as Y, a T lost before a C. It reports whether the dictionary has one
// (lexical.md §9.3).
func (stage *Stage) restoreStem(suffix bool) bool {
	list := stage.Pipeline
	cursor := stage.cursor()
	end := cursor.Working[1]
	if stage.hasClass(end.Symbol(), consonant) && end.Symbol() == end.Previous.Symbol() {
		cursor.Working[1] = end.Previous
		if stage.DictionaryLookup() {
			list.FreeNode(end, false)
			return true
		}
		cursor.Working[1] = end
	} else if stage.TriedE || stage.lostE(end) {
		cursor.Working[1] = list.NewNode(end, true, pipeline.Letter, 'E')
		if stage.DictionaryLookup() {
			return true
		}
		end = list.FreeNode(cursor.Working[1], false)
		cursor.Working[1] = end
		stage.TriedE = true
	}
	yForI := suffix && (stage.Affix == nil || !stage.Affix.KeepsI)
	if end.Symbol() == 'E' && end.Previous.Symbol() == 'I' {
		stem := list.FreeNode(end, false)
		cursor.Working[1] = stem
		stem.SetSymbol('Y')
		if stage.DictionaryLookup() {
			return true
		}
	}
	if end.Symbol() == 'I' && yForI && stage.AffixStress != foreignEnding {
		cursor.Working[1].SetSymbol('Y')
		if stage.DictionaryLookup() {
			return true
		}
	}
	if end != stage.LastNode && stage.contextMatches(stage.tables.StemConditions.LostT, end.Next, false) && stage.beforeCSuffix() {
		cursor.Working[1] = list.NewNode(end, true, pipeline.Letter, 'T')
		return stage.DictionaryLookup()
	}
	return false
}

// lostE reports whether the stem may have lost a silent E before the suffix found: when the suffix may drop one,
// after a consonant or U, unless the stem ends as noSilentE says and not as silentE does.
func (stage *Stage) lostE(end *pipeline.Node) bool {
	affix := stage.Affix
	stems := &stage.tables.StemConditions
	if affix == nil || affix.SilentE == 0 || affix.SilentE == 2 && stage.contextMatches(stems.WithoutE, end.Next, false) {
		return false
	}
	if !stage.hasClass(end.Symbol(), consonant) && end.Symbol() != 'U' {
		return false
	}
	return stage.contextMatches(stems.SilentE, end.Next, false) || !stage.contextMatches(stems.NoSilentE, end.Next, false)
}

// beforeCSuffix reports whether the stem is followed by an affix mark and a C, not before an R or an A.
func (stage *Stage) beforeCSuffix() bool {
	list := stage.Pipeline
	mark := list.Next(stage.cursor().Working[1])
	if mark == nil || mark.Symbol() != affixMark {
		return false
	}
	letter := list.Next(mark)
	if letter == nil || letter.Symbol() != 'C' {
		return false
	}
	next := list.Next(letter)
	return next != nil && next.Symbol() != 'R' && next.Symbol() != 'A'
}
