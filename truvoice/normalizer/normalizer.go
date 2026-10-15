// Package normalizer is the TruVoice engine's normalizer (docs/truvoice/normalizer.md): it runs the data pack's
// program on the nodes the input stage made, which spells words as letters, says numbers, abbreviations and symbols
// as phonemes, and reads phonetic input.
package normalizer

import (
	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
)

// CallDepth is the number of calls the program can return from.
const CallDepth = 20

// A Stage is the normalizer's state.
type Stage struct {
	Pipeline *pipeline.Pipeline

	// The program's machine (normalizer.md §2): the place in the program it runs at, whether a condition comes
	// next, the bytes of actions left in the run and those it skips after, whether the run has yielded, the calls
	// it returns to and how many it is in, and its registers.
	Counter   int32
	Testing   bool
	Remaining byte
	Skip      byte
	Yielded   bool
	Calls     [CallDepth]Call
	Depth     int32
	Registers [datapack.NormalizerRegisters]byte

	// Entry is the list entry a match found last, and Inserted whether its replacement has been inserted.
	Entry    *datapack.ListEntry
	Inserted bool

	// Pending holds what a mark of phonetic input gives the next node (normalizer.md §4.2): its value, its second
	// value and the pending flag.
	Pending pipeline.Node

	tables  *datapack.Normalizer
	classes *datapack.Classes
}

// A Call is where a call returns to: the call's place, and the run's bytes left and to skip.
type Call struct {
	Counter   int32
	Remaining byte
	Skip      byte
}

// New returns the stage for the pipeline.
func New(tables *datapack.Normalizer, parameters *datapack.Parameters, pipeline *pipeline.Pipeline) *Stage {
	stage := &Stage{Pipeline: pipeline, tables: tables}
	if parameters != nil {
		stage.classes = &parameters.Classes
	}
	return stage
}

func (stage *Stage) cursor() *pipeline.Cursor {
	return &stage.Pipeline.Cursors[pipeline.Normalizer]
}

// Reset starts the program from its beginning, with no call to return from, and clears the number word flag
// (normalizer.md §3).
func (stage *Stage) Reset() {
	stage.Counter = 0
	stage.Testing = true
	stage.Yielded = false
	stage.Depth = 0
	stage.Pipeline.NumberWord = false
}

// The flag bits of a node the normalizer sets: phonetic input's pending flag, a word spelled with register 2 set,
// and a phoneme the ! mark gave its values.
const (
	pendingFlag   = 0x20
	spelledFlag   = 0x40
	exclaimedFlag = 0x80
)

// The places of a node's bytes in its data: a capital letter's mark and the hyphen a letter stands for.
const (
	capitalOffset = 0x14 - 8
	hyphenOffset  = 0x18 - 8
)

// The class of the symbols that start a word's phonemes and of those the program's marks may give values.
const (
	wordMarkClass = 0x08
	phonemeClass  = 0x80
)

// hasClass reports whether the symbol has a bit of the symbol classes' first plane. Symbols of 128 and up are in
// none.
func (stage *Stage) hasClass(symbol, bits byte) bool {
	return symbol < datapack.SymbolCount && stage.classes[0][symbol]&bits != 0
}

// Run runs the program until a condition needs a node past the window or the program yields, and hands the
// finished nodes on (normalizer.md §2.1). It reports whether it handed nodes on.
func (stage *Stage) Run() bool {
	cursor := stage.cursor()
	if !stage.Pipeline.Begin(cursor) {
		return false
	}
	if cursor.Working[0] == nil {
		cursor.Working[0] = cursor.Scan
	}
	program := stage.tables.Program
	for {
		if stage.Testing {
			passed, waiting := stage.test()
			if waiting {
				return stage.finish(false)
			}
			stage.Testing = false
			branch := program[stage.Counter+1]
			if passed {
				stage.Remaining, stage.Skip = branch>>4, branch&15
				stage.Counter += 2
			} else {
				stage.Remaining, stage.Skip = branch&15, 0
				stage.Counter += 2 + int32(branch>>4)
			}
		}
		for stage.Remaining != 0 {
			action := program[stage.Counter]
			if action&datapack.TransferBit != 0 {
				if action&datapack.CallBit != 0 {
					stage.Calls[stage.Depth] = Call{Counter: stage.Counter, Remaining: stage.Remaining, Skip: stage.Skip}
					stage.Depth++
				}
				stage.Counter = int32((int(action)<<8 | int(program[stage.Counter+1])) & datapack.TargetBits)
				stage.Remaining, stage.Skip = 1, 0
				break
			}
			if action == yieldAction && !stage.Yielded {
				stage.Yielded = true
				return stage.finish(true)
			}
			stage.act(action)
			stage.Counter++
			stage.Remaining--
		}
		stage.Testing = true
		stage.Counter += int32(stage.Skip)
	}
}

// operand reads the next byte of the program as an operand.
func (stage *Stage) operand() byte {
	stage.Counter++
	return stage.tables.Program[stage.Counter]
}

// actionOperand reads the next byte of the program as an operand of an action, which the run counts.
func (stage *Stage) actionOperand() byte {
	stage.Remaining--
	return stage.operand()
}

// stringOperand reads a two-byte offset into the string table and returns the string there.
func (stage *Stage) stringOperand(read func() byte) []byte {
	high := int(read())
	return stage.tables.String(high<<8 | int(read()))
}

// finish hands the finished nodes on. When the program waits for a node, they are those before the scan node,
// but for the control nodes, ']' and ' ' phonemes at the window's start, which wait with it; when it yields, all
// before the scan node. It reports whether it handed nodes on, or, when it yields, true.
func (stage *Stage) finish(yielding bool) bool {
	cursor := stage.cursor()
	if yielding {
		cursor.Done = cursor.Scan
	} else {
		cursor.Done = cursor.Start
		for cursor.Done != cursor.Scan {
			node := cursor.Done
			if node.Type() != pipeline.Control && (node.Type() != pipeline.Phoneme || node.Symbol() != ']' && node.Symbol() != ' ') {
				break
			}
			cursor.Done = stage.Pipeline.Next(node)
		}
	}
	return stage.Pipeline.Finish() || yielding
}

// insert puts a node of the type with the symbol before the scan node, or at the list's end when there is none,
// which then starts or ends the window. A node of a symbol that starts a word's phonemes takes what phonetic input
// left pending (normalizer.md §2.4).
func (stage *Stage) insert(nodeType int, symbol byte) {
	cursor := stage.cursor()
	pipe := stage.Pipeline
	var node *pipeline.Node
	if cursor.Scan == nil {
		node = pipe.NewNode(pipe.Last().Previous, true, nodeType, symbol)
		cursor.End = node
		if cursor.Start == nil {
			cursor.Start = node
		}
	} else {
		node = pipe.NewNode(cursor.Scan, false, nodeType, symbol)
	}
	pending := &stage.Pending
	if stage.hasClass(symbol, wordMarkClass) && pending.Flags()&pendingFlag != 0 {
		node.SetFlags(node.Flags() | pendingFlag)
		node.SetSecond(pending.Second())
		pending.SetSecond(0)
		node.SetValue(pending.Value())
		pending.SetValue(0)
		pending.SetFlags(pending.Flags() &^ pendingFlag)
	}
	if isUpper(symbol) {
		node.Data[capitalOffset] = 1
	}
}

// match looks for a word of the list that the character nodes from the first working node to the first node
// spell, lowercased when folding, and keeps the entry it finds (normalizer.md §2.5).
func (stage *Stage) match(list byte, folding bool) bool {
	cursor := stage.cursor()
	entries := stage.tables.Lists[list]
	for index := range entries {
		word := entries[index].Match
		position := 0
		cursor.Working[1] = cursor.Working[0]
		for {
			node := cursor.Working[1]
			symbol := node.Symbol()
			if folding && isUpper(symbol) {
				symbol += 'a' - 'A'
			}
			if node.Type() != pipeline.Character || symbol != word[position] {
				break
			}
			position++
			if position == len(word) || node == cursor.First {
				break
			}
			cursor.Working[1] = node.Next
		}
		if position == len(word) && cursor.Working[1] == cursor.First {
			stage.Entry, stage.Inserted = &entries[index], false
			if list == numberList || list == teensList || list == tensList {
				stage.Pipeline.NumberWord = true
			}
			return true
		}
	}
	return false
}

// The lists whose words are numbers: a match of one sets the number word flag.
const (
	numberList = 1
	teensList  = 13
	tensList   = 14
)

func isUpper(symbol byte) bool {
	return symbol >= 'A' && symbol <= 'Z'
}

func isLower(symbol byte) bool {
	return symbol >= 'a' && symbol <= 'z'
}

func isDigit(symbol byte) bool {
	return symbol >= '0' && symbol <= '9'
}

// isTransparent reports whether a control node's letter is one the program passes over: any but C, F, I, N and x.
func isTransparent(letter byte) bool {
	switch letter {
	case 'C', 'F', 'I', 'N', 'x':
		return false
	}
	return true
}
