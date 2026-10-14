package lexical

import (
	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
)

// The bits of Following: the word's end follows, a vowel of a stem follows, and the bits a rule's flags carry on
// to the next rule's. The bit of a rule's first mask that makes the masks a test that none of their bits is set,
// or of its second that one is.
const (
	followingEnd   = 0x0001
	followingVowel = 0x0002
	carriedBits    = 0x2048
	maskMode       = 0x8000
)

// The bits of Ending: a prefix's mark follows, the stress the ending gives, and of it the stress under which an S
// after a T isn't counted; and a rule's flag that keeps the vowel after its phonemes full.
const (
	endingMark   = 0x0004
	endingStress = 0x1f80
	stressAfterT = 0x0400
	keepsFull    = 0x0008
)

// secondaryStress is the stress letter-to-sound gives the vowels after the primary stress in turn.
const secondaryStress = 1

// A conversion is letter-to-sound's state for one word, besides the stage's.
type conversion struct {
	stage *Stage
	rule  *datapack.LetterRule
	// The stress pattern's codes left and the vowels it may still stress, the word's primary stress, and the stress
	// the word's ending gives.
	pattern        byte
	patternVowels  int8
	primary        uint32
	endingStresses uint32
	// The vowels' weight towards the stress, and the rule's; the Following bits carried on; the Ending bit of a
	// prefix's mark once a rule has given it.
	weight, ruleWeight int32
	carried            uint32
	mark               uint32
	// The symbol written before the current one, which follows it in the word.
	last byte
	// Whether no vowel nor weight is written yet, the word has its primary stress, the vowel written last is
	// stressed, the stem's start is passed, and it was before the rule.
	atEnd, stressed, marked, pastStem, pastStemBefore bool
}

// LetterToSound converts the word's letters to phonemes by rules, from the stem's end back to the word's first
// letter, then from the word's last letter back to the stem (lexical.md §10).
func (stage *Stage) LetterToSound() {
	list := stage.Pipeline
	cursor := stage.cursor()
	word, lastNode, stemEnd := stage.Word, stage.LastNode, cursor.Working[1]
	conversion := &conversion{stage: stage, patternVowels: 4, primary: stage.primaryStress(), atEnd: true}
	stage.Vowel, stage.Reached = nil, nil
	stage.AfterStem = false
	stage.Consonants = 1
	stage.Following = followingEnd
	stage.Ending = 0
	stage.Reducible = false
	stage.Letter = stemEnd
	conversion.pattern = stage.StressPattern
	if stage.AffixStress != 0 {
		conversion.endingStresses = affixEndings(stage.AffixStress)
		stage.Ending = conversion.endingStresses
	}
	for {
		node := stage.Letter
		if node.Type() != pipeline.Letter {
			if conversion.passPhoneme(node) {
				return
			}
		} else if symbol := node.Symbol(); symbol < '@' || symbol > '[' {
			stage.Letter = list.FreeNode(node, false)
		} else {
			conversion.convert(node)
		}
		if stage.Letter == nil {
			stage.Letter = word
		}
		if stage.Letter == word && stemEnd != lastNode {
			stage.Letter = stage.LastNode
			stage.Following = followingEnd
			if conversion.endingStresses == 0 {
				stage.Ending = 0
			} else {
				stage.Ending |= conversion.endingStresses
			}
			conversion.mark = 0
			stage.AfterStem = true
			conversion.carried, conversion.weight = 0, 0
			stage.Consonants = 1
			conversion.last = 0
			conversion.marked = false
		}
		if stage.Letter == word {
			return
		}
	}
}

// affixEndings is the Ending bit of an affix's stress.
func affixEndings(stress byte) uint32 {
	switch stress {
	case 2:
		return 0x80
	case 3:
		return 0x100
	case 4:
		return 0x200
	case 5:
		return 0x400
	case 6:
		return 0x1000
	case 7:
		return 0x800
	}
	return 0
}

// passPhoneme passes a node that isn't a letter: a phoneme of the stem, whose stress the vowels before it count
// with. It reports whether the conversion ends there, after the stem.
func (conversion *conversion) passPhoneme(node *pipeline.Node) bool {
	stage := conversion.stage
	conversion.pastStem = true
	if stage.AfterStem {
		if stage.Vowel != nil {
			stage.ReduceVowel(false)
		}
		return true
	}
	if stress := stressOf(node); stress == 0 {
		if stage.hasClass(node.Symbol(), vowel) {
			conversion.marked = false
		}
	} else {
		conversion.marked = true
		if stress != secondaryStress || stage.Deemphasized {
			conversion.stressed = true
		}
	}
	stage.Following = 0
	previous := stage.Pipeline.Previous(node)
	if previous != nil && previous.Symbol() == '[' {
		if !stage.hasClass(node.Symbol(), consonant) {
			stage.Following = followingVowel
			if !stage.hasClass(node.Symbol(), reduced) {
				stage.Following = fullVowel | followingVowel
			}
		} else {
			stage.Consonants++
		}
		stage.Ending |= endingMark
	}
	stage.Letter = previous
	return false
}

// convert converts the letter by its first rule that holds: the rule's letters become its phonemes.
func (conversion *conversion) convert(letter *pipeline.Node) {
	stage := conversion.stage
	list := stage.Pipeline
	cursor := stage.cursor()
	rules := stage.tables.LetterRules[letter.Symbol()-'@']
	for index := range rules {
		conversion.rule = &rules[index]
		stage.Reached = letter
		if stage.masksMatch(conversion.rule.Masks) && stage.lettersMatch(conversion.rule.Before) &&
			stage.contextMatches(conversion.rule.If, stage.Reached, false) {
			break
		}
	}
	rule := conversion.rule
	for stage.Reached != letter {
		stage.Reached = list.FreeNode(stage.Reached, true)
	}
	stage.Letter = list.FreeNode(letter, false)
	if cursor.Working[0].Type() == pipeline.Free {
		conversion.pastStem = true
	}
	stage.Reached = nil
	if rule.JoinsPrefix && !conversion.pastStem {
		if mark := cursor.Working[0].Previous; mark.Symbol() == '[' {
			first := stage.FirstLetter
			if (first.Symbol() != 'M' || first.Next.Symbol() != 'C') && (first.Symbol() != 'H' || first.Next.Symbol() != '[') {
				list.FreeNode(mark, false)
				cursor.Working[0] = stage.FirstLetter
			}
		}
	}
	if rule.Weight != 0 {
		conversion.ruleWeight = int32(rule.Weight)
		stage.Consonants = 0
		conversion.atEnd = false
	}
	for index := len(rule.Phonemes) - 1; index >= 0; index-- {
		symbol := rule.Phonemes[index]
		if symbol != conversion.last || !stage.hasClass(symbol, consonant) {
			stage.Reached = list.NewNode(stage.Letter, true, pipeline.Phoneme, symbol)
			if stage.LastNode.Type() == pipeline.Free {
				stage.LastNode = stage.Reached
			}
		}
		switch {
		case stage.hasClass(symbol, syllabic):
			conversion.writeVowel(symbol)
		case symbol == '[':
			if stage.hasClass(conversion.last, phoneme) {
				symbol = conversion.last
			}
			if word := stage.Word; word.Flags()&stressBits == secondaryStress<<stressShift {
				conversion.marked = true
				word.SetFlags(word.Flags() &^ stressBits)
			}
		default:
			var next byte
			if index > 0 {
				next = rule.Phonemes[index-1]
			}
			conversion.writeConsonant(symbol, next)
		}
		conversion.last = symbol
	}
	conversion.finishRule()
}

// writeConsonant counts a consonant the rule wrote, but the second of a cluster that starts a syllable.
func (conversion *conversion) writeConsonant(symbol, next byte) {
	stage := conversion.stage
	consonants := stage.Consonants
	if consonants > 0 && conversion.weight != 0 || consonants > 1 {
		conversion.carried = 0
	}
	if conversion.endingStresses&stressAfterT != 0 && next == 'T' && symbol == 'S' || next == '[' {
		return
	}
	if consonants != 1 || (conversion.last != 'R' || !stage.hasClass(symbol, closure)) && (symbol != 'K' && symbol != 'G' || conversion.last != 'W') {
		stage.Consonants = consonants + 1
	}
}

// writeVowel settles the vowel written before (lexical.md §11) and stresses the vowel the rule wrote, by the
// stress pattern or by the weight of the vowels after it, and decides whether it may be reduced.
func (conversion *conversion) writeVowel(symbol byte) {
	stage := conversion.stage
	node := stage.Reached
	if conversion.atEnd && stage.Consonants > 0 {
		stage.Consonants--
	}
	if conversion.rule.Flags[1]&keepsFull != 0 {
		stage.Reducible = false
	}
	if stage.Vowel != nil {
		stage.ReduceVowel(false)
		if !stage.hasClass(node.Symbol(), syllabic) {
			stage.Reducible = false
			conversion.weight++
			conversion.finishVowel(node)
			return
		}
	}
	byPattern := false
	if stage.StressPattern != 0 && !conversion.pastStemBefore {
		byPattern = conversion.patternVowels >= 1
		conversion.patternVowels--
	}
	if byPattern {
		code := conversion.pattern & 3
		conversion.pattern = byte(int8(conversion.pattern) >> 2)
		conversion.marked = code >= 2
		stage.Reducible = code == 0
		switch code {
		case 2:
			setStress(node, secondaryStress)
		case 3:
			conversion.stressed = true
			setStress(node, conversion.primary)
		}
		conversion.finishVowel(node)
		return
	}
	if !conversion.stressed {
		if conversion.weight == 1 && stage.Consonants > 1 {
			conversion.weight = 2
		}
		conversion.weight += conversion.ruleWeight + 1
		conversion.ruleWeight = 0
		if conversion.weight > 2 {
			conversion.stressed = true
			setStress(node, conversion.primary)
			conversion.marked = true
		}
	} else if !stage.AfterStem {
		if !conversion.marked {
			setStress(node, secondaryStress)
		}
		conversion.marked = !conversion.marked
	}
	switch {
	case conversion.atEnd && stage.hasClass(node.Next.Symbol(), velar):
		stage.Reducible = false
	case symbol == 'U' && stage.hasClass(node.Previous.Symbol(), wordMark):
		stage.Reducible = false
	default:
		stage.Reducible = !conversion.marked && (stage.Consonants != 0 || symbol == 'o' || symbol == 'a') &&
			!stage.hasClass(symbol, reduced) && conversion.rule.Flags[0]&fullVowel == 0
	}
	conversion.finishVowel(node)
}

// finishVowel makes the vowel the one written last.
func (conversion *conversion) finishVowel(node *pipeline.Node) {
	stage := conversion.stage
	stage.Consonants = 0
	conversion.carried = 0
	stage.Vowel = node
	conversion.atEnd = false
}

// finishRule settles the vowel written last once the stem's start is passed, and tells the next rule what follows
// its letters: the rule's flags, and the marks around a prefix.
func (conversion *conversion) finishRule() {
	stage := conversion.stage
	rule := conversion.rule
	if (conversion.pastStem || stage.AfterStem) && stage.Vowel != nil {
		if !conversion.stressed {
			conversion.stressed = true
			setStress(stage.Vowel, conversion.primary)
			conversion.marked = true
			stage.Reducible = false
		}
		stage.ReduceVowel(true)
		if stage.Following&fullVowel != 0 {
			stage.Reducible = false
		}
	}
	following := rule.Flags[0] &^ maskMode
	if conversion.marked && stage.hasClass(conversion.last, vowel) {
		following |= fullVowel
	}
	stage.Ending = rule.Flags[1] &^ maskMode
	if conversion.mark == 0 {
		conversion.mark = rule.Flags[1] & endingMark
	} else {
		stage.Ending |= conversion.mark
	}
	if conversion.endingStresses == 0 {
		conversion.endingStresses = stage.Ending & endingStress
	} else {
		stage.Ending |= conversion.endingStresses
	}
	if following != 0 {
		if following == followingEnd {
			conversion.carried = stage.Following
		}
		stage.Following = conversion.carried | following
		conversion.carried = following & carriedBits
		if conversion.carried != 0 && conversion.weight != 0 {
			stage.Consonants = 0
		}
	}
	conversion.pastStemBefore = conversion.pastStem
	if stage.Letter.Symbol() == '[' {
		if stage.StressPattern != 0 {
			stage.Ending |= endingMark
		} else if conversion.marked || !stage.Reducible {
			stage.Following |= fullVowel
		}
	}
}

func stressOf(node *pipeline.Node) uint32 {
	return node.Flags() >> stressShift & 3
}

func setStress(node *pipeline.Node, stress uint32) {
	node.SetFlags(node.Flags()&^stressBits | stress<<stressShift)
}

// masksMatch reports whether the stage's Following and Ending have all the masks' bits, or, when the masks'
// mode bit is in the second, any of them, or, when it is in the first, none.
func (stage *Stage) masksMatch(masks [2]uint32) bool {
	following, ending := masks[0], masks[1]
	switch {
	case following&maskMode != 0:
		return stage.Following&following == 0 && stage.Ending&ending == 0
	case ending&maskMode != 0:
		return stage.Following&following != 0 || stage.Ending&ending != 0
	}
	return stage.Following&following == following && stage.Ending&ending == ending
}

// lettersMatch reports whether the letters before the one Reached are the rule's, moving Reached back over them.
func (stage *Stage) lettersMatch(before string) bool {
	for index := len(before) - 1; index >= 0; index-- {
		stage.Reached = stage.Pipeline.Previous(stage.Reached)
		if stage.Reached == nil || stage.Reached.Symbol() != before[index] || stage.Reached.Type() != pipeline.Letter {
			return false
		}
	}
	return true
}
