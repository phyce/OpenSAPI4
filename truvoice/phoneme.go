package truvoice

import (
	"errors"
	"unicode/utf16"

	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
	"github.com/phyce/OpenSAPI4/truvoice/text"
)

// ErrInvalidArgument is E_INVALIDARG: a character set Phoneme doesn't give.
var ErrInvalidArgument = errors.New("truvoice: invalid argument")

// PhonemeString converts a text with tags to engine phonemes, as Phoneme does.
func (engine *Engine) PhonemeString(value string) string {
	characters := utf16.Encode([]rune(value))
	phonemes, _ := engine.Phoneme(append(characters, 0), true, text.EnginePhonetic)
	return string(utf16.Decode(phonemes[:len(phonemes)-1]))
}

// Phoneme converts a text to phonemes as Phoneme of the wide interface does (engine.md §6): characters are UTF-16
// code units, the whole SDATA. The phonemes are engine phonemes, or IPA, each word's mark before it, a vowel's
// stress after it and the punctuation that ends a sentence; with tagged, each tag goes over as it is. Phoneme
// returns the phonemes with a NUL after them, which the SDATA counts, and ErrInvalidArgument for a character set
// other than IPA and engine phonemes. It doesn't use the engine's state: it speaks with voice 0 at its defaults.
func (engine *Engine) Phoneme(characters []uint16, tagged bool, characterSet text.CharacterSet) ([]uint16, error) {
	data := engine.data
	if characterSet != text.IPA && characterSet != text.EnginePhonetic {
		return nil, ErrInvalidArgument
	}
	converted := append(text.ToANSI(data.text, characters), 0)
	listing := data.listing(converted, tagged)
	if characterSet == text.EnginePhonetic {
		return text.ToWide(data.text, listing), nil
	}
	return data.ipa(listing, tagged, len(cString(converted))), nil
}

// PhonemeANSI converts a text to engine phonemes as Phoneme of the ANSI interface does: characters are code page
// 1252's, and a character set other than engine phonemes gives ErrInvalidArgument. Otherwise it is Phoneme.
func (engine *Engine) PhonemeANSI(characters []byte, tagged bool, characterSet text.CharacterSet) ([]byte, error) {
	if characterSet != text.EnginePhonetic {
		return nil, ErrInvalidArgument
	}
	phonemes, err := engine.Phoneme(text.ToWide(engine.data.text, characters), tagged, characterSet)
	if err != nil {
		return nil, err
	}
	return text.ToANSI(engine.data.text, phonemes), nil
}

// listing speaks a text in code page 1252, with its NUL and what follows it, on a pipeline of its own that renders
// nothing and lists the phonemes, and returns the listing with a NUL after it (engine.md §6.2).
func (data *Data) listing(converted []byte, tagged bool) []byte {
	thread := data.newThread(nil, pipeline.InitialSampleRate)
	thread.pipeline.Listing = true
	thread.pipeline.Initialize()
	if tagged {
		thread.listTagged(converted)
	} else {
		thread.listRaw(cString(converted))
	}
	return append(thread.pipeline.Listed, 0)
}

// listRaw feeds a text as it is, and steps the pipeline until it is idle.
func (thread *thread) listRaw(text []byte) {
	thread.input.StartRaw(text)
	for {
		thread.input.TurnRaw(text)
		if thread.pipeline.Step()&pipeline.StepIdle != 0 {
			return
		}
	}
}

// listTagged lists a tagged text (engine.md §6.3). The text before each tag goes to the escape parser, and the
// pipeline steps until it is idle; then the tag goes into the listing as it is, from its backslash up to the next,
// and a space after it too. Two backslashes are one in the text. The text after the last tag is fed as it is.
func (thread *thread) listTagged(converted []byte) {
	length := len(cString(converted))
	var piece []byte
	for index := 0; index < length; {
		switch {
		case converted[index] != '\\':
			piece = append(piece, converted[index])
			index++
		case byteAt(converted, index+1) == '\\':
			piece = append(piece, '\\')
			index += 2
		default:
			end := index + 2
			for end < length && converted[end] != '\\' {
				end++
			}
			if len(piece) > 0 {
				thread.input.PutString(piece)
				piece = nil
				for thread.pipeline.Step()&pipeline.StepIdle == 0 {
				}
			}
			for ; index <= end; index++ {
				thread.pipeline.Listed = append(thread.pipeline.Listed, byteAt(converted, index))
			}
			if byteAt(converted, index) == ' ' {
				thread.pipeline.Listed = append(thread.pipeline.Listed, ' ')
				index++
			}
		}
	}
	if len(piece) > 0 {
		thread.listRaw(piece)
	}
}

// ipa converts a listing to IPA (engine.md §6.4): each symbol to its IPA string, any other byte to nothing. In a
// tagged text, a backslash and what follows it up to the next backslash go over as they are, the next backslash
// looked for no further than the text's length. The original reads what follows the listing in memory there; the
// port reads NULs.
func (data *Data) ipa(listing []byte, tagged bool, length int) []uint16 {
	var converted []uint16
	for index := 0; index < len(listing); {
		symbol := listing[index]
		if tagged && symbol == '\\' {
			end := index + 2
			for end < length && byteAt(listing, end) != '\\' {
				end++
			}
			for ; index <= end; index++ {
				converted = append(converted, uint16(byteAt(listing, index)))
			}
			continue
		}
		index++
		if symbol >= ' ' && symbol <= '~' {
			converted = append(converted, data.engine.IPA[symbol]...)
		}
	}
	return append(converted, 0)
}

func cString(value []byte) []byte {
	for index, character := range value {
		if character == 0 {
			return value[:index]
		}
	}
	return value
}

func byteAt(value []byte, index int) byte {
	if index < len(value) {
		return value[index]
	}
	return 0
}
