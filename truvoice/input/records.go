package input

import "github.com/phyce/OpenSAPI4/truvoice/pipeline"

const (
	wordNodes     = 10
	bracketsFlag  = 0x20
	stressedFlag  = 0x40
	phoneticFlags = 3
)

// The flag bits a command of three values takes from its first value (input.md §2.3).
const (
	recordedBits = 0x18
	resetBit     = 0x20
	thirdBit     = 0x40
)

// MakeRecords is the input stage's step: it makes the nodes of the next word from the command ring, at the end of
// the list, and adds them to the normalizer's window. A word is its characters up to the spaces after them, at most
// ten nodes. It reports whether the ring had a character (input.md §2.2).
func (stage *Stage) MakeRecords() bool {
	if stage.Pending == nothing {
		if stage.Pending = stage.take(); stage.Pending == nothing {
			return false
		}
	}
	spaced := false
	for made := 0; made < wordNodes; made++ {
		character := stage.Pending
		if character == ' ' {
			spaced = true
		} else if spaced {
			break
		}
		phoneticBrackets := stage.InputFlags&bracketsFlag != 0 && stage.Flags&phoneticFlags == 0
		switch {
		case character == escape:
			stage.makeCommand()
		case character == '[' && phoneticBrackets:
			node := stage.appendNode(pipeline.Control, 'I')
			node.SetValue(1)
			if stage.InputFlags&stressedFlag != 0 {
				node.SetValue(2)
			}
		case character == ']' && phoneticBrackets:
			stage.appendNode(pipeline.Control, 'I')
		default:
			stage.appendNode(pipeline.Character, byte(character))
		}
		if stage.Pending = stage.take(); stage.Pending == nothing {
			break
		}
	}
	return true
}

// makeCommand makes the node of a command the escape parser wrote (input.md §2.3).
func (stage *Stage) makeCommand() {
	node := stage.appendNode(pipeline.Control, escape)
	node.SetSymbol(byte(stage.take()))
	letter := node.Symbol()
	switch stage.take() {
	case 1:
		node.SetValue(uint32(stage.take()))
		node.SetSecond(0)
		stage.giveText(node)
	case 2:
		node.SetValue(uint32(stage.take()))
		node.SetSecond(byte(stage.take()))
		flags := uint32(node.Second())<<8 | node.Value()&0xff
		switch letter {
		case 'A':
			stage.InputFlags = flags
		case 'N':
			stage.Flags = flags
		}
	case 3:
		bits := uint32(stage.take())
		flags := node.Flags()&^recordedBits | bits&3<<3
		if bits&4 != 0 {
			flags |= resetBit
		}
		if bits&8 != 0 {
			flags |= thirdBit
		}
		node.SetFlags(flags)
		node.SetValue(uint32(stage.take()))
		node.SetSecond(byte(stage.take()))
	case 4:
		value := uint32(stage.take())
		value |= uint32(stage.take()) << 8
		value |= uint32(stage.take()) << 16
		value |= uint32(stage.take()) << 24
		node.SetValue(value)
		stage.giveText(node)
	default:
		node.SetValue(0)
		node.SetSecond(0)
	}
}

// giveText gives a bookmark's or a word's node the text being read.
func (stage *Stage) giveText(node *pipeline.Node) {
	if letter := node.Symbol(); letter == 'i' || letter == 'z' {
		node.Text = stage.Text
	}
}

// appendNode makes a node at the end of the list and adds it to the normalizer's window.
func (stage *Stage) appendNode(nodeType int, symbol byte) *pipeline.Node {
	node := stage.Pipeline.NewNode(stage.Pipeline.Last(), false, nodeType, symbol)
	cursor := &stage.Pipeline.Cursors[pipeline.Normalizer]
	cursor.End = node
	for _, field := range []**pipeline.Node{&cursor.First, &cursor.Scan, &cursor.Done, &cursor.Start} {
		if *field == nil {
			*field = node
		}
	}
	return node
}
