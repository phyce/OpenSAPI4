package prosody

import (
	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
)

// A class is a symbol class (prosody.md §2.1): bits of a class byte, the byte's plane times 256 plus the bits.
type class int32

const (
	syllabic    class = 0x001
	sonorant    class = 0x002
	voiced      class = 0x004
	wordMark    class = 0x008
	nasal       class = 0x010
	stop        class = 0x020
	fricative   class = 0x040
	phoneme     class = 0x080
	boundary    class = 0x101
	affricate   class = 0x108
	consonant   class = 0x120
	phraseBreak class = 0x140
	closure     class = 0x201
	vowel       class = 0x202
	lateral     class = 0x240
	approximant class = 0x301
	laryngeal   class = 0x302
	frontEnding class = 0x340
	high        class = 0x401
	fortis      class = 0x540
)

// hasClass reports whether the symbol is in the class. Symbols of 128 and up are in none.
func (stage *Stage) hasClass(symbol byte, kind class) bool {
	return symbol < datapack.SymbolCount && stage.classes[kind>>8][symbol]&byte(kind) != 0
}

// inClass reports whether the node is in the class; a negative class is the symbols not in the class. A missing
// node is in neither.
func (stage *Stage) inClass(node *pipeline.Node, kind class) bool {
	if node == nil {
		return false
	}
	if kind < 0 {
		return !stage.hasClass(node.Symbol(), -kind)
	}
	return stage.hasClass(node.Symbol(), kind)
}

// A Context is what the stage notes about the current node and the nodes around it (prosody.md §2.3).
type Context struct {
	Symbol                                                  byte
	Affricate, Fricative, Nasal, Phoneme, Sonorant, Closure bool
	Stressed, Syllabic, Vowel                               bool
	Next, Following, AfterFollowing                         *pipeline.Node
	Previous, Preceding, BeforePreceding                    *pipeline.Node
	NextSymbol, FollowingSymbol, AfterFollowingSymbol       byte
	PreviousSymbol, PrecedingSymbol, BeforePrecedingSymbol  byte
	NextStressed, PrecedingSyllabic, FollowingSyllabic      bool
}

// noSymbol is the symbol a missing node has in the context.
const noSymbol = ' '

func symbolOf(node *pipeline.Node) byte {
	if node == nil {
		return noSymbol
	}
	return node.Symbol()
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

// previousPhoneme returns the last phoneme or segment node, of type 3 or 4, before the node in the window.
func (stage *Stage) previousPhoneme(node *pipeline.Node) *pipeline.Node {
	if node == nil {
		return nil
	}
	for node = stage.Pipeline.Previous(node); node != nil && node.Type() != pipeline.Phoneme && node.Type() != pipeline.Segment; node = stage.Pipeline.Previous(node) {
	}
	return node
}

func isWordMark(symbol byte) bool {
	return symbol == '&' || symbol == '%'
}

// nextSound returns the first phoneme node after the node in the window that isn't a word mark.
func (stage *Stage) nextSound(node *pipeline.Node) *pipeline.Node {
	if node == nil {
		return nil
	}
	for node = stage.Pipeline.Next(node); node != nil; node = stage.Pipeline.Next(node) {
		if node.Type() == pipeline.Phoneme && !isWordMark(node.Symbol()) {
			return node
		}
	}
	return nil
}

// previousSound returns the last phoneme or segment node before the node in the window that is neither a word
// mark nor in class break.
func (stage *Stage) previousSound(node *pipeline.Node) *pipeline.Node {
	if node == nil {
		return nil
	}
	for node = stage.Pipeline.Previous(node); node != nil; node = stage.Pipeline.Previous(node) {
		if (node.Type() == pipeline.Phoneme || node.Type() == pipeline.Segment) && !isWordMark(node.Symbol()) &&
			!stage.hasClass(node.Symbol(), phraseBreak) {
			return node
		}
	}
	return nil
}

// LoadContext notes the current node's classes and the nodes around it.
func (stage *Stage) LoadContext() {
	node := stage.cursor().Scan
	symbol := node.Symbol()
	context := &stage.Context
	context.Affricate = stage.hasClass(symbol, affricate)
	context.Fricative = stage.hasClass(symbol, fricative)
	context.Nasal = stage.hasClass(symbol, nasal)
	context.Phoneme = stage.hasClass(symbol, phoneme)
	context.Sonorant = stage.hasClass(symbol, sonorant)
	context.Closure = stage.hasClass(symbol, closure)
	context.Syllabic = stage.hasClass(symbol, syllabic)
	context.Vowel = stage.hasClass(symbol, vowel)
	context.Stressed = node.Flags()&stressedFlag != 0
	context.Symbol = symbol
	context.Next = stage.nextPhoneme(node)
	context.NextSymbol = symbolOf(context.Next)
	context.Following = stage.nextSound(node)
	context.FollowingSymbol = symbolOf(context.Following)
	context.AfterFollowing = stage.nextSound(context.Following)
	context.AfterFollowingSymbol = symbolOf(context.AfterFollowing)
	context.Previous = stage.previousPhoneme(node)
	context.PreviousSymbol = symbolOf(context.Previous)
	context.Preceding = stage.previousSound(node)
	context.PrecedingSymbol = symbolOf(context.Preceding)
	context.BeforePreceding = stage.previousSound(context.Preceding)
	context.BeforePrecedingSymbol = symbolOf(context.BeforePreceding)
	context.NextStressed = context.Next != nil && context.Next.Flags()&stressedFlag != 0
	context.PrecedingSyllabic = stage.hasClass(context.PrecedingSymbol, syllabic)
	context.FollowingSyllabic = stage.hasClass(context.FollowingSymbol, syllabic)
}

// A stress test of scanForClass.
const (
	byClass = iota
	findStress
	stopStress
)

// scanForClass looks at up to count phonemes after the current node going forward, or before it going back:
// it reports true at the first one in class find, and false at the first one not in class stop, or when it
// runs out. With findStress, a phoneme is found by its stress instead, and with stopStress it stops by it
// (prosody.md §2.2).
func (stage *Stage) scanForClass(forward bool, count int32, find, stop class, test int) bool {
	node := stage.cursor().Scan
	for ; count > 0; count-- {
		if forward {
			node = stage.nextPhoneme(node)
		} else {
			node = stage.previousPhoneme(node)
		}
		if node == nil {
			return false
		}
		if test == findStress {
			if found := hasStress(node, find); found && find > 0 || !found && find < 0 {
				return true
			}
		} else if stage.inClass(node, find) {
			return true
		}
		if test == stopStress {
			if found := hasStress(node, stop); found && stop < 0 || !found && stop > 0 {
				return false
			}
		} else if !stage.inClass(node, stop) {
			return false
		}
	}
	return false
}

// hasStress reports whether the node has stress 3, or stress 2 when the level's magnitude is 2.
func hasStress(node *pipeline.Node, level class) bool {
	if level < 0 {
		level = -level
	}
	return stress(node) == 3 || level == 2 && stress(node) == 2
}
