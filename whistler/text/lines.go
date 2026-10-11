package text

import "bytes"

const endOfText = 0x1A

// joinLines normalises line breaks to CR LF and removes single breaks; two or more breaks with only
// spaces, TABs and escapes between them leave one CR (text.md §4).
func joinLines(tagged []byte) []byte {
	text := append(append([]byte(nil), tagged...), 0, 0)
	for index := 0; text[index] != 0; index++ {
		switch {
		case text[index] == lineFeed:
			text[index] = carriageReturn
			if text[index+1] == carriageReturn {
				text[index+1] = lineFeed
				index++
			}
		case text[index] == carriageReturn && text[index+1] == lineFeed:
			index++
		}
	}
	seenBreak := false
	pending := -1
	for index := 0; text[index] != 0; index++ {
		switch character := text[index]; character {
		case tab, space:
		case lineFeed:
			text[index] = space
		case carriageReturn:
			if seenBreak {
				text[index] = space
				pending = -1
			} else {
				seenBreak = true
				pending = index
			}
		case escape:
			for text[index] != space && text[index] != tab && text[index] != carriageReturn && text[index] != lineFeed && text[index] != endOfText && text[index] != 0 {
				index++
			}
			if text[index] == 0 {
				index--
			}
		default:
			if pending >= 0 {
				text[pending] = space
				pending = -1
			}
			seenBreak = false
		}
	}
	return text[:bytes.IndexByte(text, 0)]
}
