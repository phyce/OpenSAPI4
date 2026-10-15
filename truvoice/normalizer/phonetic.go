package normalizer

import "github.com/phyce/OpenSAPI4/truvoice/pipeline"

// A phoneticLetter is how phonetic input writes a letter, in either case (normalizer.md §4.1): as the phoneme
// alone, or, before one of the letters of pairs, in either case, as the phoneme after it, that letter then removed.
type phoneticLetter struct {
	alone byte
	pairs string
}

var phoneticLetters = [26]phoneticLetter{
	{'o', "AoHoEaOwRrWfX@YI"}, {'B', ""}, {'C', "HC"}, {'D', "HxTt"}, {'e', "HeRkYA"}, {'F', ""}, {'G', ""},
	{'H', "HdWhXH"}, {'i', "HiR4X|YE"}, {'J', "HJ"}, {'K', ""}, {'L', "Xj"}, {'M', ""}, {'N', "G~X~"},
	{'o', "RgWOYy"}, {'P', ""}, {'Q', "Qq"}, {'R', "R3"}, {'S', "Hs"}, {'T', "HX"}, {'u', "HuLlMmNnRcWbXv"},
	{'V', ""}, {'W', ""}, {0, "Xp"}, {'Y', "R5"}, {'Z', "Hz"},
}

// The kinds of mark that phonetic input leaves pending for the next word, as the pending value (normalizer.md
// §4.2).
const (
	equalsMark      = 1
	graveMark       = 2
	apostropheMark  = 3
	asteriskMark    = 4
	hyphenMark      = 5
	backslashMark   = 6
	slashMark       = 7
	slashBackMark   = 8
	exclamationMark = 9
	colonMark       = 10
	hashMark        = 11
	braceMark       = 12
	dollarMark      = 13
	underscoreMark  = 17
	hashValue       = 8
	highestDigits   = 0x3f
)

// plainMarks are the marks left pending before the end of the phonetic input and removed elsewhere.
var plainMarks = map[byte]uint32{'*': asteriskMark, '=': equalsMark, '_': underscoreMark, ':': colonMark,
	'{': braceMark, '\\': backslashMark}

// phonetic reads the first node's symbol as phonetic input (normalizer.md §4): a letter, alone or with the
// letter after it, becomes a phoneme, a mark right before the end of the phonetic input is left pending for the
// next word's start, and a digit or two before it give the pending values their second value.
func (stage *Stage) phonetic() {
	cursor := stage.cursor()
	pipe := stage.Pipeline
	node := cursor.First
	next := node.Next
	ends := next.Type() == pipeline.Control && next.Symbol() == 'I' && next.Value() == 0
	afterPhoneme := node.Previous.Type() == pipeline.Phoneme
	pending := &stage.Pending
	pend := func(mark uint32) {
		pending.SetValue(mark)
		pending.SetFlags(pending.Flags() | pendingFlag)
		node.SetSymbol(0)
	}
	symbol := node.Symbol()
	switch symbol {
	case ' ', '(', ')', ',', '.', ';', '?':
	case '!', '"', '~':
		if ends && !afterPhoneme {
			pend(exclamationMark)
		} else {
			node.SetSymbol('"')
		}
	case '#':
		if ends && !afterPhoneme {
			pend(hashMark)
		} else {
			node.SetSymbol(' ')
			node.SetValue(hashValue)
		}
	case '$':
		if ends {
			pend(dollarMark)
		}
	case '%', '&':
		if pending.Flags()&pendingFlag != 0 {
			node.SetValue(pending.Value())
			node.SetSecond(pending.Second())
			pending.SetFlags(pending.Flags() &^ pendingFlag)
		}
	case '\'':
		if ends && afterPhoneme {
			node.SetSymbol('1')
		} else if ends {
			pending.SetValue(apostropheMark)
			pending.SetFlags(pending.Flags() | pendingFlag)
		}
	case '*', '=', '_', ':', '{', '\\':
		if ends {
			pend(plainMarks[symbol])
		} else {
			node.SetSymbol(0)
		}
	case '+':
		node.SetSymbol('[')
	case '-':
		if ends {
			pend(hyphenMark)
		} else {
			node.SetSymbol('1')
		}
	case '`':
		if ends && !afterPhoneme {
			pend(graveMark)
		} else {
			node.SetSymbol('2')
		}
	case '/':
		if ends {
			pend(slashMark)
		} else if after := next.Next; next.Symbol() == '\\' && after.Type() == pipeline.Control && after.Symbol() == 'I' && after.Value() == 0 {
			pend(slashBackMark)
			pipe.FreeNode(next, false)
		} else {
			node.SetSymbol(0)
		}
	case '<':
		if ends {
			pipe.Synthesizer.Hold = true
		}
		node.SetSymbol(0)
	case '>':
		if !ends {
			node.SetSymbol(0)
			return
		}
		previous := node.Previous
		previous.SetSymbol(0)
		previous.SetFlags(previous.Flags() &^ 7)
		node.SetSymbol(0)
		node.SetFlags(node.Flags() &^ 7)
		next.SetSymbol('C')
		cursor.PhoneticInput = 0
		pipe.Synthesizer.Hold = false
	case '|':
		node.SetSymbol('\\')
	case '}':
		node.SetSymbol(']')
	default:
		switch {
		case isDigit(symbol):
			stage.digits(node, next)
		case isUpper(symbol) || isLower(symbol):
			letter := phoneticLetters[(symbol|0x20)-'a']
			second := next.Symbol()
			for index := 0; index < len(letter.pairs); index += 2 {
				if second == letter.pairs[index] || second == letter.pairs[index]|0x20 {
					pipe.FreeNode(next, false)
					node.SetSymbol(letter.pairs[index+1])
					return
				}
			}
			node.SetSymbol(letter.alone)
		default:
			node.SetSymbol(0)
		}
	}
}

// digits reads a digit of phonetic input: before the end of the phonetic input it gives the pending values its
// value as their second value, and before another digit the second value its character code times ten, at most
// 63, the other digit then removed (normalizer.md §4.2). The digit itself is removed.
func (stage *Stage) digits(node, next *pipeline.Node) {
	pending := &stage.Pending
	switch {
	case isDigit(next.Symbol()):
		if after := next.Next; after.Type() == pipeline.Control && after.Symbol() == 'I' && after.Value() == 0 {
			pending.SetSecond(min(node.Symbol()*10, highestDigits))
			pending.SetFlags(pending.Flags() | pendingFlag)
		}
		stage.Pipeline.FreeNode(next, false)
	case next.Symbol() == 'I':
		if next.Type() == pipeline.Control && next.Value() == 0 {
			pending.SetSecond(node.Symbol() - '0')
			pending.SetFlags(pending.Flags() | pendingFlag)
		}
	}
	node.SetSymbol(0)
}
