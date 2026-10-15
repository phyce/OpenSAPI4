package text

import (
	"encoding/binary"
	"strconv"

	"github.com/phyce/OpenSAPI4/truvoice/synthesizer"
)

const (
	// A tag's value or a \Vce\ key or value is read up to this many characters.
	longestValue = 19
	// The pause chunks of \Pau=\: at most ten, the first nine of 255 hundredths.
	pauseChunk     = 255
	pauseChunks    = 10
	mutedVolume    = 16
	lowestPitch    = 50
	highestPitch   = 400
	lowestSpeed    = 50
	highestSpeed   = 250
	unknownContext = `\Ctx="Unknown"\`
)

// tagState is what ConvertTags keeps while it reads one text: the text's length, which a long pause can cut,
// spell mode, the last volume and the last number read (the original's variable starts with whatever its stack
// holds; the port's with 0).
type tagState struct {
	length int
	spell  bool
	volume uint32
	number uint32
}

// convertTags turns the tags of a text into escape sequences (engine.md §1.3): it reads each tag by the three
// letters after its backslash, puts a space before what a tag writes when the character before the tag isn't a
// space, drops the tags it doesn't know, keeps \Ctx=...\ for applyContexts and spells the text while \RmS=1\ is in
// force. An unterminated tag, or a backslash in the last two characters, ends the text.
func (converter *Converter) convertTags(text []byte) []byte {
	state := &tagState{length: len(text), volume: 0xffff}
	var converted []byte
	for position := 0; position < state.length && position < len(text); {
		character := text[position]
		if character == backslash {
			if at(text, position+1) == backslash {
				converted = append(converted, backslash, backslash)
				position += 2
				continue
			}
			closing := position + 2
			for closing < state.length && at(text, closing) != backslash {
				closing++
			}
			if state.length <= closing {
				break
			}
			if closing-(position+1) >= 3 {
				converted = converter.convertTag(converted, text, position, closing, state)
			}
			position = closing + 1
			continue
		}
		switch {
		case !state.spell:
			converted = append(converted, character)
			position++
		case character == escape:
			for !isAlpha(at(text, position)) && position < len(text) {
				converted = append(converted, text[position])
				position++
			}
			converted = append(converted, at(text, position))
			position++
		case !isAlphanumeric(character):
			converted = append(converted, character)
			position++
		default:
			for isAlphanumeric(at(text, position)) {
				letter := isAlpha(text[position])
				converted = append(converted, text[position])
				position++
				if letter {
					converted = append(converted, '.')
				}
				converted = append(converted, ' ')
			}
			converted = append(converted, ',')
		}
	}
	return cString(converted)
}

// convertTag converts the tag from start to the backslash at closing.
func (converter *Converter) convertTag(converted, text []byte, start, closing int, state *tagState) []byte {
	name := lowered(text[start+1 : start+4])
	spaced := start > 1 && !isSpace(text[start-1])
	space := func() {
		if spaced {
			converted = append(converted, ' ')
		}
	}
	switch name {
	case "ctx":
		space()
		return append(converted, text[start:closing+1]...)
	case "emp":
		space()
		return append(converted, escape, '[', '0', 'e')
	case "rst":
		space()
		return converter.reset(converted, state)
	}
	if text[start+4] != '=' {
		return converted
	}
	valueStart := start + 5
	value := text[valueStart:closing]
	switch name {
	case "mrk":
		if !allDigits(value) {
			return converted
		}
		space()
		state.scan(value)
		if state.number != 0 {
			converted = append(converted, escape, '[')
			converted = appendBytes(converted, state.number)
			converted = append(converted, 'i', ' ')
			state.number = 0
		}
	case "chr":
		space()
		switch lowered(field(text, valueStart)) {
		case `"normal"`:
			converted = append(converted, escape, '[', '0', 'c')
		case `"whisper"`:
			converted = append(converted, escape, '[', '1', 'c')
		case `"monotone"`:
			converted = append(converted, escape, '[', '2', 'c')
		}
	case "pau":
		if !allDigits(value) {
			return converted
		}
		space()
		if len(value) >= 2 {
			converted = state.pause(converted, value)
		}
	case "pit":
		if !allDigits(value) {
			return converted
		}
		space()
		state.scan(value)
		switch state.number {
		case 0:
			state.number = lowestPitch
		case 0xffff:
			state.number = highestPitch
		}
		if state.number >= lowestPitch && state.number <= highestPitch {
			converted = append(converted, escape, '[')
			converted = strconv.AppendUint(converted, uint64(state.number>>1), 10)
			converted = append(converted, 'p')
		}
	case "spd":
		if !allDigits(value) {
			return converted
		}
		space()
		state.scan(value)
		switch state.number {
		case 0:
			state.number = lowestSpeed
		case 0xffffffff:
			state.number = highestSpeed
		}
		if state.number >= lowestSpeed && state.number <= highestSpeed {
			converted = append(converted, escape, '[')
			converted = strconv.AppendUint(converted, uint64(state.number), 10)
			converted = append(converted, 'r')
		}
	case "vol":
		if !allDigits(value) {
			return converted
		}
		space()
		if number, read := scanNumber(value); read {
			state.volume = number
		}
		converted = appendAttenuation(converted, state.volume)
	case "vce":
		space()
		converted = converter.findVoice(converted, text, valueStart, closing)
	case "rms":
		if closing == valueStart+1 {
			switch text[valueStart] {
			case '1':
				state.spell = true
			case '0':
				state.spell = false
			}
		}
	case "pro":
		space()
		if closing == valueStart+1 {
			switch text[valueStart] {
			case '1':
				converted = append(converted, escape, '[', '1', '4', 'F')
			case '0':
				converted = append(converted, escape, '[', '1', '4', 'N')
			}
		}
	case "prn":
		space()
		converted = converter.pronunciation(converted, text, valueStart, closing)
		state.number = 0
	}
	return converted
}

// reset writes \Rst\: the voice saved by the first \Vce\, if any, then flag 14 off, the unknown context, and the
// defaults' character, speed, pitch and volume. It ends spell mode.
func (converter *Converter) reset(converted []byte, state *tagState) []byte {
	state.spell = false
	converter.process.lock.Lock()
	saved := converter.process.savedVoice
	if saved != noVoice {
		converted = append(converted, escape, '[')
		converted = strconv.AppendUint(converted, uint64(saved), 10)
		converted = append(converted, 'V')
		converter.process.savedVoice = noVoice
	}
	converter.process.lock.Unlock()
	converted = append(converted, escape, '[', '1', '4', 'F')
	converted = append(converted, unknownContext...)
	converted = append(converted, escape, '[', byte(converter.Defaults.Character)+'0', 'c')
	converted = append(converted, escape, '[')
	converted = strconv.AppendUint(converted, uint64(converter.Defaults.Speed), 10)
	converted = append(converted, 'r', escape, '[')
	converted = strconv.AppendUint(converted, uint64(converter.Defaults.Pitch>>1), 10)
	converted = append(converted, 'p')
	return appendAttenuation(converted, converter.Defaults.Volume)
}

// pause writes \Pau=\'s hundredths of a second in chunks: the rest first, then up to nine of 255. The original
// prints each into a buffer of 4 bytes: a rest of 4 digits or more writes the digits after the fourth, and a
// NUL, over the text's length, which ends the text sooner (or, past its end, never; the port ends it there).
func (state *tagState) pause(converted, value []byte) []byte {
	state.scan(value)
	state.number /= 10
	var chunks []uint32
	for state.number > pauseChunk && len(chunks) < pauseChunks-1 {
		chunks = append(chunks, pauseChunk)
		state.number -= pauseChunk
	}
	chunks = append(chunks, state.number)
	for index := len(chunks) - 1; index >= 0; index-- {
		digits := strconv.AppendInt(nil, int64(int32(chunks[index])), 10)
		converted = append(converted, escape, '[')
		converted = append(converted, digits[:min(len(digits), 4)]...)
		converted = append(converted, 's')
		if len(digits) >= 4 {
			var length [4]byte
			binary.LittleEndian.PutUint32(length[:], uint32(state.length))
			copy(length[:], append(digits[4:], 0))
			state.length = int(int32(binary.LittleEndian.Uint32(length[:])))
		}
	}
	return converted
}

// scan reads a number as sscanf's %u does, wrapping at 32 bits; an empty value leaves the last one.
func (state *tagState) scan(value []byte) {
	if number, read := scanNumber(value); read {
		state.number = number
	}
}

func scanNumber(value []byte) (uint32, bool) {
	var number uint32
	for _, character := range value {
		number = number*10 + uint32(character-'0')
	}
	return number, len(value) > 0
}

func allDigits(value []byte) bool {
	for _, character := range value {
		if !isDigit(character) {
			return false
		}
	}
	return true
}

// field is a value from index up to a comma or a backslash, at most 19 characters.
func field(text []byte, index int) []byte {
	end := index
	for end < index+longestValue && at(text, end) != ',' && at(text, end) != backslash {
		end++
	}
	return text[index:min(end, len(text))]
}

// appendAttenuation writes a volume's attenuation: 16 for a volume below 80, which mutes, else the thread's.
func appendAttenuation(converted []byte, volume uint32) []byte {
	attenuation, audible := synthesizer.Attenuation(uint16(volume))
	if !audible {
		attenuation = mutedVolume
	}
	converted = append(converted, escape, '[')
	converted = strconv.AppendInt(converted, int64(attenuation), 10)
	return append(converted, 'a')
}

// preFormatted is whether a context sends untagged text through the pre-formatter.
func preFormatted(context int) bool {
	return context == AddressContext || context == EmailContext || context == SpreadsheetContext
}
