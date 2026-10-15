package text

import "strconv"

// insertWordMarks puts a word mark before each word of a text (engine.md §1.2). With the tagged flags it passes
// over tags, puts a word's mark before an \Emp\ tag in front of it and puts none while the text is in the address
// context; with other flags every run of characters that aren't spaces is a word.
func (converter *Converter) insertWordMarks(text []byte, flags uint32) []byte {
	length := len(text)
	var marked []byte
	position := 0
	if flags != Tagged {
		for position < length {
			for isSpace(at(text, position)) {
				marked = append(marked, text[position])
				position++
			}
			marked = converter.appendMark(marked, position)
			for !isSpace(at(text, position)) {
				if length <= position {
					return marked
				}
				marked = append(marked, text[position])
				position++
			}
		}
		return marked
	}
	address := converter.Context == AddressContext
	for position < length {
		for isSpace(at(text, position)) {
			marked = append(marked, text[position])
			position++
		}
		if at(text, position) == backslash && at(text, position+1) != backslash {
			name := position + 1
			if hasPrefixFold(text, name, "ctx=") {
				name += len("ctx=")
				address = hasPrefixFold(text, name, `"address"`)
			}
			if hasPrefixFold(text, name, "emp") {
				position = converter.markBeforeEmphasis(&marked, text, position)
				continue
			}
			marked = append(marked, backslash)
			closing := position + 1
			for at(text, closing) != backslash && closing < length {
				marked = append(marked, text[closing])
				closing++
			}
			marked = append(marked, at(text, closing))
			position = closing + 1
			continue
		}
		if length <= position {
			break
		}
		if !address {
			marked = converter.appendMark(marked, position)
		}
		for !isSpace(at(text, position)) {
			if length <= position {
				return marked
			}
			marked = append(marked, text[position])
			position++
		}
	}
	return marked
}

// markBeforeEmphasis puts the mark of the word after an \Emp\ tag at start before the tag, then the tag and any
// tags between it and the word, without their spaces, then the word, and returns where it ends. An \Emp\ tag with
// no word after it is dropped with what follows it. The original looks for the tag's closing backslash past the
// text's end; the port takes the end as its place.
func (converter *Converter) markBeforeEmphasis(marked *[]byte, text []byte, start int) int {
	length := len(text)
	position := start
	for at(text, position+1) != backslash && position+1 < length {
		position++
	}
	position += 2
	if length <= position {
		return length
	}
	for {
		for isSpace(at(text, position)) {
			position++
		}
		if at(text, position) == backslash && at(text, position+1) != backslash {
			for at(text, position+1) != backslash && position+1 < length {
				position++
			}
			position += 2
		} else if position < length {
			*marked = converter.appendMark(*marked, position)
			for ; start < position; start++ {
				if !isSpace(text[start]) {
					*marked = append(*marked, text[start])
				}
			}
			for !isSpace(at(text, start)) && start < length {
				*marked = append(*marked, text[start])
				start++
			}
			return start
		}
		if length <= position {
			return length
		}
	}
}

// appendMark appends a word mark: ESC[b0;b1;b2;b3z and a space, the bytes of the word's offset in the text, low
// first, the offset doubled for a wide text.
func (converter *Converter) appendMark(marked []byte, position int) []byte {
	offset := uint32(position)
	if converter.WideOffsets {
		offset *= 2
	}
	marked = append(marked, escape, '[')
	marked = appendBytes(marked, offset)
	return append(marked, 'z', ' ')
}

// appendBytes appends a value's four bytes, low first, in decimal with semicolons between them.
func appendBytes(text []byte, value uint32) []byte {
	for index := range 4 {
		text = strconv.AppendUint(text, uint64(value&0xff), 10)
		value >>= 8
		if index != 3 {
			text = append(text, ';')
		}
	}
	return text
}
