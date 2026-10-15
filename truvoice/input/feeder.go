package input

import (
	"strings"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
)

// The feeder's limits (input.md §5): the lines it feeds at a time, the characters and the characters but spaces
// and tabs after which it breaks a line, the room it keeps in the input ring, the sizes of text from which it
// lowercases lines in capitals and makes lines of punctuation dots, and the lines short enough to end with a dot.
const (
	feedLines      = 10
	lineLength     = 1000
	lineCharacters = 200
	inputReserve   = 10
	lowercasedText = 41
	dottedText     = 131
	shortLine      = 40
	rightQuote     = 0x92
)

// The characters that end a short line without a dot, and what the feeder writes at a text's end: a bookmark of
// value 0, which marks the end, and a continue.
const (
	lineEnds  = ".!?,;"
	endOfText = "\x1b[0i\x1b[C  "
)

// A line is a piece of the text the feeder writes at once (input.md §5.1): where it starts and how many characters
// it has, how many of those aren't spaces or tabs, whether it has no lowercase letter and none of the characters
// 128 to 255, whether it has no letter, digit, symbol or escape sequence, and whether a line end ended it.
type line struct {
	start, length, characters int32
	capitals, punctuation     bool
	ended                     bool
}

// Feed writes more of a text into the input ring, from Position (input.md §5): up to ten lines of it while the
// ring has room for them, and the end of text after its last.
func (stage *Stage) Feed(text []byte) {
	start := stage.Position
	lines := stage.readLines(text)
	if fed := stage.writeLines(text, lines); fed > 0 {
		stage.AfterLineEnd = lines[fed-1].ended
	}
	if stage.Position >= int32(len(text)) {
		stage.putEndOfText()
		stage.AllFed = true
	}
	if stage.Position > start {
		stage.Drained = false
	}
}

// readLines reads up to ten lines of the text from Position, and moves Position past them (input.md §5.1).
func (stage *Stage) readLines(text []byte) []line {
	lines := make([]line, 0, feedLines)
	current := newLine(stage.Position)
	for stage.Position < int32(len(text)) {
		character := text[stage.Position]
		switch {
		case character == '\n' || character == '\r':
			stage.endLine(text, &current)
		case current.characters > lineCharacters || current.length > lineLength:
			stage.breakLine(text, &current)
		default:
			stage.readCharacter(text, &current)
			continue
		}
		lines = append(lines, current)
		if len(lines) == feedLines {
			return lines
		}
		current = newLine(stage.Position)
	}
	current.trim(text, " \x00\t")
	if current.length > 0 {
		lines = append(lines, current)
	}
	return lines
}

func newLine(start int32) line {
	return line{start: start, capitals: true, punctuation: true}
}

// endLine ends a line at a line end, which a line end of the other kind can follow, and takes its trailing spaces
// and tabs off.
func (stage *Stage) endLine(text []byte, current *line) {
	pair := byte('\r')
	if text[stage.Position] == '\r' {
		pair = '\n'
	}
	stage.Position++
	if byteAt(text, int(stage.Position)) == pair {
		stage.Position++
	}
	current.ended = true
	current.trim(text, " \t")
}

// breakLine ends a line that has grown too long before its last space. The original looks for the space back to
// the line's start and before it: without a space after the start, the port ends the line where it is.
func (stage *Stage) breakLine(text []byte, current *line) {
	space := stage.Position
	for space > current.start && text[space] != ' ' {
		space--
	}
	if space > current.start {
		current.length -= stage.Position - space
		stage.Position = space
	}
}

// readCharacter adds the next character to a line, or the next escape sequence, up to its letter.
func (stage *Stage) readCharacter(text []byte, current *line) {
	place := int(stage.Position)
	character := text[place]
	if character == escape && byteAt(text, place+1) == '[' {
		current.punctuation = false
		if byteAt(text, place+2) == '1' && byteAt(text, place+3) == 'I' {
			current.capitals = false
		}
		for place < len(text) {
			place++
			if isLetter(text[place-1]) {
				break
			}
		}
		current.length += int32(place) - stage.Position
		stage.Position = int32(place)
		return
	}
	stage.Position++
	current.length++
	if character != ' ' && character != '\t' {
		current.characters++
	}
	if character >= 'a' && character <= 'z' || character >= firstHighCharacter {
		current.capitals = false
	}
	if isLetter(character) || isDigit(character) || character >= datapack.FirstHighCharacter || stage.tables.Symbols[character] != "" {
		current.punctuation = false
	}
}

// trim takes the characters given off the line's end.
func (current *line) trim(text []byte, characters string) {
	for current.length > 0 && strings.IndexByte(characters, text[current.start+current.length-1]) >= 0 {
		current.length--
	}
}

// writeLines writes lines into the input ring (input.md §5.2), and returns how many it wrote. It stops at a line
// the ring has no room for, and moves Position back to it.
func (stage *Stage) writeLines(text []byte, lines []line) int {
	for index, current := range lines {
		afterLineEnd := stage.AfterLineEnd
		if index > 0 {
			afterLineEnd = lines[index-1].ended
		}
		switch {
		case current.punctuation && current.length > 0 && len(text) >= dottedText:
			stage.putInput('.')
		case current.length > 0:
			if stage.inputRoom()-inputReserve <= current.length {
				stage.Position = current.start
				return index
			}
			stage.writeLine(text, current)
			if spoken, last := current.spoken(text); spoken > 0 && spoken < shortLine && afterLineEnd &&
				strings.IndexByte(lineEnds, last) < 0 {
				stage.putInput('.')
			}
		}
		if current.ended {
			stage.putInput('\n')
		}
	}
	return len(lines)
}

// writeLine writes a line's characters into the input ring, lowercased if it is in capitals and the text has 41
// characters or more, but for its escape sequences.
func (stage *Stage) writeLine(text []byte, current line) {
	lowercase := current.capitals && len(text) >= lowercasedText
	end := current.start + current.length
	for place := current.start; place < end; place++ {
		character := text[place]
		switch {
		case !lowercase:
			stage.putInput(character)
		case character != escape || byteAt(text, int(place)+1) != '[':
			if character >= 'A' && character <= 'Z' {
				character += 'a' - 'A'
			}
			stage.putInput(character)
		default:
			for ; place < end; place++ {
				stage.putInput(text[place])
				if isLetter(text[place]) {
					break
				}
			}
		}
	}
}

// spoken returns how many of a line's characters are outside its escape sequences and the spaces after them, and
// the last of those that isn't a space or a tab, or 0.
func (current line) spoken(text []byte) (int32, byte) {
	commands, last := int32(0), byte(0)
	at := func(place int32) byte { return byteAt(text, int(current.start+place)) }
	for place := int32(0); place < current.length; {
		if character := at(place); character != escape || at(place+1) != '[' {
			if character != ' ' && character != '\t' {
				last = character
			}
			place++
			continue
		}
		for place < current.length {
			if at(place) == escape && at(place+1) == '[' {
				place += 2
				commands += 2
				for place < current.length {
					place++
					commands++
					if isLetter(at(place - 1)) {
						break
					}
				}
			} else if at(place) == ' ' {
				place++
				commands++
			} else {
				break
			}
		}
	}
	return current.length - commands, last
}

// inputRoom is the room the input ring has left: 4,095 bytes less those it holds.
func (stage *Stage) inputRoom() int32 {
	return ringRoom(stage.InputRead, stage.InputWritten)
}

// putInput writes a character into the input ring, unless it has room for fewer than ten. A right single quote
// goes in as an apostrophe.
func (stage *Stage) putInput(character byte) {
	if stage.inputRoom() < inputReserve {
		return
	}
	if character == rightQuote {
		character = '\''
	}
	stage.Input[stage.InputWritten] = character
	stage.InputWritten = (stage.InputWritten + 1) % inputRingSize
}

// putEndOfText writes the end of text into the input ring, unless it has room for fewer than ten characters: as
// many of its characters as leave room for nine.
func (stage *Stage) putEndOfText() {
	if stage.inputRoom() < inputReserve {
		return
	}
	for index := range len(endOfText) {
		stage.putInput(endOfText[index])
	}
}

func isDigit(character byte) bool {
	return character >= '0' && character <= '9'
}
