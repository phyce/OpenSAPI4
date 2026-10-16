// Package text is what the TruVoice engine's TextData makes of a text before it queues it for the speaking thread
// (docs/truvoice/engine.md §1): the text in code page 1252, the word marks, the tags turned into escape sequences,
// the contexts and the characters 128 and up a tagged text loses.
package text

import (
	"errors"
	"sync"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
)

// CharacterSet is a text's character set, as SAPI numbers them.
type CharacterSet uint32

const (
	Text           CharacterSet = 0
	IPA            CharacterSet = 1
	EnginePhonetic CharacterSet = 2
)

// Tagged is the flags of a text whose tags are read; any other flags leave them as text.
const Tagged = 1

// The contexts of \Ctx="..."\.
const (
	NoContext          = 0
	AddressContext     = 1
	EmailContext       = 4
	SpreadsheetContext = 6
)

const (
	escape    = 0x1b
	backslash = '\\'
	noVoice   = -1
)

// ErrInvalidCharacter is TTSERR_INVALIDCHAR: a character set TextData doesn't take.
var ErrInvalidCharacter = errors.New("truvoice: invalid character set")

// Process is what TextData keeps for the whole process: the voice \Rst\ goes back to, which the first \Vce\
// saves.
type Process struct {
	lock       sync.Mutex
	savedVoice int32
}

func NewProcess() *Process {
	return &Process{savedVoice: noVoice}
}

// Defaults are what \Rst\ goes back to: the mode's pitch in Hz and speed in words a minute, its volume and its
// character.
type Defaults struct {
	Pitch     uint16
	Speed     uint32
	Volume    uint32
	Character uint32
}

// Pronunciation is a \Prn=\ tag's word and pronunciation, which the text refers to by its number. Phonetic is
// set for a pronunciation in engine phonemes (written after //).
type Pronunciation struct {
	Word          string
	Pronunciation string
	Phonetic      bool
}

// Converter converts the texts of one engine. What it reads and changes of the engine is in its fields.
type Converter struct {
	tables  *datapack.Text
	modes   *datapack.Modes
	process *Process

	// Context is the engine's context: \Ctx="..."\ sets it, and it lasts from text to text. A text in the
	// address, e-mail or spreadsheet context goes through the original's pre-formatter, which isn't ported.
	Context int
	// WideOffsets doubles the word marks' offsets, as for a text of UTF-16 code units. A text through the ANSI
	// interface clears it, and a text with word marks sets it after them.
	WideOffsets bool
	// Voice is the engine's voice, which the first \Vce\ in the process saves for \Rst\.
	Voice    int32
	Defaults Defaults
	// Pronunciations are the \Prn=\ tags' records, in order: the text refers to the first as 1.
	Pronunciations []Pronunciation
}

func New(tables *datapack.Text, modes *datapack.Modes, process *Process, voice int32, defaults Defaults) *Converter {
	return &Converter{tables: tables, modes: modes, process: process, WideOffsets: true, Voice: voice, Defaults: defaults}
}

// Wide converts a text as TextData of the wide interface does: characters are UTF-16 code units, the whole
// SDATA, so a NUL the caller counts is one of them. It returns the bytes the engine queues, NULs included, and
// ErrInvalidCharacter for a character set other than the three.
func (converter *Converter) Wide(characters []uint16, characterSet CharacterSet, flags uint32, sink bool) ([]byte, error) {
	switch characterSet {
	case Text, EnginePhonetic:
		return converter.convert(converter.ansi(characters), characterSet, flags, sink), nil
	case IPA:
		return converter.finish(append(cString(converter.ipa(characters, flags == Tagged)), 0), characterSet, flags), nil
	}
	return nil, ErrInvalidCharacter
}

// ANSI converts a text as TextData of the ANSI interface does: it takes text and engine phonemes, clears
// WideOffsets and converts the bytes as Wide does their code units. Code page 1252's bytes come back from UTF-16
// as they were.
func (converter *Converter) ANSI(text []byte, characterSet CharacterSet, flags uint32, sink bool) ([]byte, error) {
	if characterSet != Text && characterSet != EnginePhonetic {
		return nil, ErrInvalidCharacter
	}
	converter.WideOffsets = false
	return converter.convert(text, characterSet, flags, sink), nil
}

// convert queues text or engine phonemes in code page 1252: the text with a NUL after it, or the phonemes between
// the phonetic commands in a buffer 9 bytes longer, then the word marks and the tags.
func (converter *Converter) convert(text []byte, characterSet CharacterSet, flags uint32, sink bool) []byte {
	var queued []byte
	if characterSet == Text {
		queued = append(append([]byte{}, text...), 0)
	} else {
		queued = make([]byte, len(text)+len(phoneticStart)+len(phoneticEnd)+1)
		copy(queued, phoneticStart)
		copy(queued[len(phoneticStart):], text)
		copy(queued[len(cString(queued)):], phoneticEnd+"\x00")
	}
	if characterSet == Text && sink {
		queued = append(cString(converter.insertWordMarks(cString(queued), flags)), 0)
		converter.WideOffsets = true
	}
	return converter.finish(queued, characterSet, flags)
}

// finish reads a tagged text's tags and contexts, and pre-formats an untagged text in the address, e-mail or
// spreadsheet context.
func (converter *Converter) finish(queued []byte, characterSet CharacterSet, flags uint32) []byte {
	if flags == Tagged {
		return append(converter.applyContexts(converter.convertTags(cString(queued)), characterSet), 0)
	}
	if characterSet == Text && preFormatted(converter.Context) {
		return append(converter.preFormat(blank(cString(queued))), 0)
	}
	return queued
}

func (converter *Converter) ansi(characters []uint16) []byte {
	return ToANSI(converter.tables, characters)
}

// ToANSI converts code units to code page 1252 as WideCharToMultiByte does: best fit, '?' for the rest, NUL to NUL.
func ToANSI(tables *datapack.Text, characters []uint16) []byte {
	converted := make([]byte, len(characters))
	for index, character := range characters {
		switch value, found := tables.BestFit[character]; {
		case character == 0:
		case found:
			converted[index] = value
		default:
			converted[index] = '?'
		}
	}
	return converted
}

// ToWide converts code page 1252 to code units as MultiByteToWideChar does.
func ToWide(tables *datapack.Text, text []byte) []uint16 {
	converted := make([]uint16, len(text))
	for index, character := range text {
		converted[index] = tables.Wide[character]
	}
	return converted
}

// cString is text up to its first NUL.
func cString(text []byte) []byte {
	for index, character := range text {
		if character == 0 {
			return text[:index]
		}
	}
	return text
}

// at is the byte at index, or NUL past the end.
func at(text []byte, index int) byte {
	if index < len(text) {
		return text[index]
	}
	return 0
}

// The C locale's classes: bytes 128 and up are in none.
func isSpace(character byte) bool {
	return character == ' ' || character >= '\t' && character <= '\r'
}

func isAlpha(character byte) bool {
	return character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z'
}

func isDigit(character byte) bool {
	return character >= '0' && character <= '9'
}

func isAlphanumeric(character byte) bool {
	return isAlpha(character) || isDigit(character)
}

func lower(character byte) byte {
	if character >= 'A' && character <= 'Z' {
		return character + 'a' - 'A'
	}
	return character
}

// hasPrefixFold is whether text at index starts with literal, ASCII case aside, as _strnicmp compares them.
func hasPrefixFold(text []byte, index int, literal string) bool {
	for position := range len(literal) {
		if lower(at(text, index+position)) != literal[position] {
			return false
		}
	}
	return true
}

func lowered(text []byte) string {
	converted := make([]byte, len(text))
	for index, character := range text {
		converted[index] = lower(character)
	}
	return string(converted)
}
