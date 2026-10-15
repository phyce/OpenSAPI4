package normalizer

import (
	"bytes"
	"strings"

	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
)

// The program's conditions, by their byte (normalizer.md §2.2).
const (
	phoneticInputCondition = iota
	phraseCondition
	flagCondition
	phoneticSymbolCondition
	letterCondition
	digitCondition
	lowercaseCondition
	uppercaseCondition
	aheadCondition
	registerCondition
	registerAboveCondition
	symbolCondition
	listCondition
	nextControlCondition
	controlCondition
	trueCondition
	transparentCondition
	inCondition
	exactListCondition
)

// The classes the class conditions and the condition ahead test, by their byte.
const (
	letterClass       = 4
	digitClass        = 5
	lowercaseClass    = 6
	uppercaseClass    = 7
	controlClass      = 0xe
	fullStopClass     = 0x13
	aheadLimit        = 20
	flagsPerWord      = 16
	phoneticInputMode = 2
)

// The program's actions, by their byte (normalizer.md §2.3).
const (
	lettersAction = iota
	phonemesAction
	skipAction
	spellAction
	phonemizeAction
	replacementAction
	setAction
	incrementAction
	decrementAction
	rewindAction
	nextAction
	backAction
	deleteAction
	deleteSpanAction
	insertAction
	_
	matchAction
	yieldAction
	executeAction
	returnAction
	exclaimAction
	slashAction
	markAction
)

// inClass reports whether a symbol is in a class of the class conditions.
func inClass(class, symbol byte) bool {
	switch class {
	case letterClass:
		return isLower(symbol) || isUpper(symbol) || symbol == '\''
	case digitClass:
		return isDigit(symbol)
	case lowercaseClass:
		return isLower(symbol)
	case uppercaseClass:
		return isUpper(symbol)
	case controlClass:
		return int8(symbol) < ' '
	case fullStopClass:
		return symbol == '.'
	}
	return false
}

// test runs the condition at the program's place, and leaves the place at its last byte. It reports whether the
// condition passed, or that it waits for a node past the window.
func (stage *Stage) test() (passed, waiting bool) {
	cursor := stage.cursor()
	pipe := stage.Pipeline
	first := cursor.First
	switch stage.tables.Program[stage.Counter] {
	case phoneticInputCondition:
		return cursor.PhoneticInput != 0, false
	case phraseCondition:
		return cursor.Phrase == 0, false
	case flagCondition:
		if flag := stage.operand(); flag < flagsPerWord {
			return cursor.Flags&(1<<flag) != 0, false
		} else {
			return cursor.InputFlags&(1<<(flag-flagsPerWord)) != 0, false
		}
	case phoneticSymbolCondition:
		if cursor.PhoneticInput == phoneticInputMode {
			if first == cursor.End {
				return false, true
			}
			stage.phonetic()
		}
		symbol := cursor.First.Symbol()
		return isUpper(symbol) || isLower(symbol) || symbol >= '1' && symbol <= '5' ||
			strings.IndexByte(stage.tables.Punctuation, symbol) >= 0, false
	case letterCondition, digitCondition, lowercaseCondition, uppercaseCondition:
		return inClass(stage.tables.Program[stage.Counter], first.Symbol()), false
	case aheadCondition:
		return stage.ahead()
	case registerCondition:
		register := stage.operand()
		return stage.Registers[register] == stage.operand(), false
	case registerAboveCondition:
		register := stage.operand()
		return stage.Registers[register] > stage.operand(), false
	case symbolCondition:
		symbol := stage.operand()
		return first.Symbol() == symbol && symbol < 0x80, false
	case listCondition:
		return stage.match(stage.operand(), true), false
	case nextControlCondition:
		if first == cursor.End {
			return false, true
		}
		cursor.First = pipe.Next(first)
		return cursor.First.Type() == pipeline.Control, false
	case controlCondition:
		if first == nil {
			return false, true
		}
		return first.Type() == pipeline.Control, false
	case trueCondition:
		return true, false
	case transparentCondition:
		return isTransparent(first.Symbol()), false
	case inCondition:
		return bytes.IndexByte(stage.stringOperand(stage.operand), first.Symbol()) >= 0, false
	case exactListCondition:
		return stage.match(stage.operand(), false), false
	}
	return false, false
}

// ahead looks at the nodes after the first, passing over transparent control nodes and spaces, '~' and '`' that
// aren't in the class, for one in the class of the condition's operand: a control node that isn't transparent
// stands for the letter 'a'. It gives up after 20 nodes, and waits at the window's end.
func (stage *Stage) ahead() (passed, waiting bool) {
	cursor := stage.cursor()
	class := stage.tables.Program[stage.Counter+1]
	cursor.Working[1] = cursor.First
	for range aheadLimit {
		if cursor.Working[1] == cursor.End {
			return false, true
		}
		node := cursor.Working[1].Next
		cursor.Working[1] = node
		symbol := node.Symbol()
		if node.Type() == pipeline.Control {
			if !isTransparent(symbol) {
				passed = inClass(class, 'a')
				break
			}
			continue
		}
		if inClass(class, symbol) {
			passed = true
			break
		}
		if symbol != ' ' && symbol != '~' && symbol != '`' {
			break
		}
	}
	stage.Counter++
	return passed, false
}

// act runs the action at the program's place, but a goto, a call and a yield, and leaves the place at its last
// byte, with the run's count less its operands.
func (stage *Stage) act(action byte) {
	cursor := stage.cursor()
	pipe := stage.Pipeline
	switch action {
	case lettersAction, phonemesAction:
		for _, symbol := range stage.stringOperand(stage.actionOperand) {
			stage.insert(pipeline.Letter+int(action), symbol)
		}
	case skipAction:
		stage.advance()
	case spellAction:
		stage.spell()
		stage.advance()
	case phonemizeAction:
		for node := cursor.Scan; ; node = node.Next {
			if node.Type() != pipeline.Control {
				node.SetFlags(node.Flags()&^4 | pipeline.Phoneme)
			}
			if node == cursor.First {
				break
			}
		}
		stage.advance()
	case replacementAction:
		if stage.Entry != nil && !stage.Inserted {
			for index := range len(stage.Entry.Replacement) {
				stage.insert(pipeline.Phoneme, stage.Entry.Replacement[index])
			}
			stage.Inserted = true
		}
	case setAction:
		register := stage.actionOperand()
		stage.Registers[register] = stage.actionOperand()
	case incrementAction:
		stage.Registers[stage.actionOperand()]++
	case decrementAction:
		stage.Registers[stage.actionOperand()]--
	case rewindAction:
		cursor.Working[0] = cursor.Scan
		cursor.First = cursor.Scan
	case nextAction:
		cursor.First = pipe.Next(cursor.First)
	case backAction:
		if cursor.First == nil {
			cursor.First = cursor.End
		} else {
			cursor.First = cursor.First.Previous
		}
	case deleteAction:
		if cursor.Scan == cursor.First {
			next := pipe.Next(cursor.Scan)
			cursor.Working[0], cursor.Scan = next, next
		}
		cursor.First = pipe.FreeNode(cursor.First, true)
	case deleteSpanAction:
		for cursor.Scan != cursor.First {
			cursor.Scan = pipe.FreeNode(cursor.Scan, true)
		}
		cursor.Scan = pipe.FreeNode(cursor.Scan, true)
		cursor.Working[0], cursor.First = cursor.Scan, cursor.Scan
	case insertAction:
		pipe.NewNode(cursor.First, true, pipeline.Letter, stage.tables.Program[stage.Counter+1])
		cursor.First = cursor.First.Next
		stage.actionOperand()
	case matchAction:
		stage.match(stage.actionOperand(), true)
	case yieldAction:
		stage.Yielded = false
	case executeAction:
		pipe.ExecuteControl()
	case returnAction:
		stage.Depth--
		call := stage.Calls[stage.Depth]
		stage.Counter, stage.Remaining, stage.Skip = call.Counter+1, call.Remaining-1, call.Skip
	case exclaimAction, slashAction:
		stage.annotate(action)
	case markAction:
		cursor.Working[0] = cursor.First
	}
}

// advance moves the first node, the scan node and the first working node on to the node after the first.
func (stage *Stage) advance() {
	cursor := stage.cursor()
	next := stage.Pipeline.Next(cursor.First)
	cursor.First, cursor.Working[0], cursor.Scan = next, next, next
}

// spell makes the nodes from the scan node to the first a word of letters: capitals, an apostrophe '@' and a hyphen
// '&' (normalizer.md §2.3).
func (stage *Stage) spell() {
	cursor := stage.cursor()
	if stage.Registers[2] != 0 {
		cursor.Start.SetFlags(cursor.Start.Flags() | spelledFlag)
	}
	if cursor.Done != nil && cursor.Done.Symbol() == '-' && cursor.Scan.Previous.Symbol() == '&' {
		cursor.Scan.Previous.Data[hyphenOffset] = '-'
	}
	for node := cursor.Scan; ; node = node.Next {
		switch symbol := node.Symbol(); {
		case symbol == '\'':
			node.SetSymbol('@')
		case symbol == '-':
			node.SetSymbol('&')
			node.Data[hyphenOffset] = '-'
		case isLower(symbol):
			node.SetSymbol(symbol - ('a' - 'A'))
		case isUpper(symbol):
			node.Data[capitalOffset] = 1
		}
		node.SetFlags(node.Flags()&^5 | pipeline.Letter)
		if node == cursor.First {
			return
		}
	}
}

// annotate marks the scan node, a phoneme of phonetic input, with ! or not with /, and reads the values after the
// mark that follows it: a pitch and a duration, which the node takes when they are in range (normalizer.md §4.3).
func (stage *Stage) annotate(action byte) {
	cursor := stage.cursor()
	phoneme := cursor.Scan
	if action == exclaimAction {
		phoneme.SetFlags(phoneme.Flags() | exclaimedFlag)
	} else {
		phoneme.SetFlags(phoneme.Flags() &^ exclaimedFlag)
	}
	values := [2]int32{-1, -1}
	position := 0
	cursor.Working[1] = phoneme.Next
	var symbol byte
	for {
		cursor.Working[1] = cursor.Working[1].Next
		symbol = cursor.Working[1].Symbol()
		if isDigit(symbol) {
			digit := int32(symbol - '0')
			if values[position] < 0 {
				values[position] = digit
			} else {
				values[position] = values[position]*10 + digit
			}
			if values[position] <= 0xff {
				continue
			}
		}
		if symbol != ';' {
			break
		}
		if position++; position == len(values) {
			break
		}
	}
	pitch, duration := values[0], max(values[1], 0)
	if pitch < 0 {
		pitch = 0
	} else if pitch == 0 {
		pitch = 2
	}
	switch {
	case symbol == '!' && pitch <= 300 && (duration >= 40 && duration <= 160 || duration == 0) && stage.hasClass(phoneme.Symbol(), phonemeClass):
		if pitch != 0 {
			phoneme.SetSecond(byte((pitch - 100) >> 1))
		}
		if duration != 0 {
			cursor.Scan.SetValue(uint32(duration - 100))
		}
	case symbol == '/' && (pitch >= 50 || pitch <= 2) && pitch <= 200 && duration <= 60 && stage.hasClass(phoneme.Symbol(), phonemeClass):
		phoneme.SetSecond(byte(pitch >> 1))
		cursor.Scan.SetValue(uint32(duration))
	}
}
