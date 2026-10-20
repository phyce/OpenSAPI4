// Package preformatter formats the text TruVoice reads in its spreadsheet context (docs/truvoice/preformatter.md).
package preformatter

import (
	"bytes"
	"strings"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
)

const (
	lineLimit = 720
	width     = 77
	// none is a break's place before any: the scan compares places down to two before the line's start.
	none = -3
)

const (
	lineEnd      = "\r\n"
	paragraphEnd = "\r\r\n"
)

// Spreadsheet formats a text in the spreadsheet context (section 1): it reads the text's lines, writes each as
// section 2 has it and wraps it (section 1.3).
func Spreadsheet(words *datapack.Spreadsheet, text []byte) []byte {
	sheet := newSheet(words)
	return []byte(format(string(text), sheet.line))
}

// ending is how a piece of a text's line ends: where the reading cut the line, at a line feed or at the text's end.
type ending int

const (
	cutEnding ending = iota
	lineFeed
	textEnd
)

// format reads a text's lines (section 1.1), cuts those too long (section 1.2), writes each through transform,
// wraps it and ends it.
func format(text string, transform func(string) string) string {
	var output strings.Builder
	read(text, func(piece string, end ending) {
		switch {
		case end == cutEnding:
			output.WriteString(wrap(transform(piece)))
			output.WriteString(paragraphEnd)
		case piece != "" && end == lineFeed:
			output.WriteString(wrap(transform(piece)))
			output.WriteString(lineEnd)
		case piece != "":
			output.WriteString(wrap(transform(piece)))
			output.WriteString(paragraphEnd)
		case end == lineFeed:
			output.WriteString(lineEnd)
		}
	})
	return output.String()
}

// read reads a text's lines (section 1.1) and cuts those too long (section 1.2). It gives each piece, without its
// carriage returns unless it continues a line it cut, with how it ends.
func read(text string, handle func(piece string, end ending)) {
	position := 0
	continuation := false
	for position < len(text) {
		stop, full, newline := window(text, position)
		end := -1
		if continuation {
			end = cut(text, position, stop, full)
		} else if full {
			end = firstCut(text, position, stop)
		}
		if end >= 0 {
			piece := text[position:end]
			if !continuation {
				piece = strings.ReplaceAll(piece, "\r", "")
			}
			handle(piece, cutEnding)
			position = end
			continuation = true
			continue
		}
		piece := text[position:stop]
		if !continuation {
			piece = strings.ReplaceAll(piece, "\r", "")
		}
		if newline {
			handle(piece, lineFeed)
		} else {
			handle(piece, textEnd)
		}
		position = stop
		if newline {
			position++
		}
		continuation = false
	}
}

// window is the characters a line can take from start: up to a line feed or the text's end, at most lineLimit.
// It reports where they stop, whether there are lineLimit of them before the text's end and whether a line feed
// ends them.
func window(text string, start int) (stop int, full, newline bool) {
	stop = start
	for stop < len(text) && stop-start < lineLimit && text[stop] != '\n' {
		stop++
	}
	full = stop-start == lineLimit && stop < len(text)
	newline = !full && stop < len(text) && text[stop] == '\n'
	return stop, full, newline
}

// cut is where a line after a cut ends: after the window's last dot followed by a space, and the space after
// that, else after its last space, else at its end if it is full. -1 is no cut.
func cut(text string, start, stop int, full bool) int {
	for index := stop - 1; index >= start; index-- {
		if sentenceBreak(text, index) {
			if index+2 < len(text) && text[index+2] == ' ' {
				return index + 3
			}
			return index + 2
		}
	}
	if index := strings.LastIndexByte(text[start:stop], ' '); index >= 0 {
		return start + index + 1
	}
	if full {
		return stop
	}
	return -1
}

// firstCut is where the first window of a line that fills it ends (section 1.2): after the last dot followed by
// a space that it takes, else after the last space it takes, else at the window's end.
func firstCut(text string, start, stop int) int {
	sentence, space := -1, -1
	lastSentence, lastSpace := none, none
	for index := start; index < stop; index++ {
		if text[index] == ' ' {
			if lastSpace == index-1 {
				lastSpace = none
			} else {
				space = index + 1
				lastSpace = index
			}
		}
		if sentenceBreak(text, index) {
			if lastSentence == index-2 {
				lastSentence = none
			} else {
				sentence = index + 2
				lastSentence = index
			}
		}
	}
	switch {
	case sentence >= 0:
		return sentence
	case space >= 0:
		return space
	}
	return stop
}

func sentenceBreak(text string, index int) bool {
	return text[index] == '.' && index+1 < len(text) && text[index+1] == ' '
}

// wrap breaks a written line before width (section 1.3): where a line would go past it in a word with no space
// to break at, the character there becomes a space.
func wrap(line string) string {
	wrapped := []byte(line)
	start := 0
	for index := range wrapped {
		if index-start < width {
			continue
		}
		if wrapped[index] == ' ' {
			start = index + 1
			continue
		}
		if index+1 == len(wrapped) || wrapped[index+1] == ' ' {
			continue
		}
		if space := bytes.LastIndexByte(wrapped[start+1:index], ' '); space >= 0 {
			start += space + 2
		} else {
			wrapped[index] = ' '
			start = index + 1
		}
	}
	return string(wrapped)
}
