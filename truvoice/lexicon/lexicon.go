// Package lexicon is the user lexicon of the TruVoice engine (docs/truvoice/engine.md §7): the words an application
// gives pronunciations, which every engine of the data looks up before its dictionary, the methods of ILexPronounce
// that add, get and remove them, and the file the original keeps them in.
package lexicon

import (
	"bytes"
	"errors"
	"slices"
	"sync"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/text"
)

var (
	// ErrInvalidArgument is E_INVALIDARG: a character set the method doesn't take.
	ErrInvalidArgument = errors.New("lexicon: invalid character set")
	// ErrInvalidWord is LEXERR_INVALIDTEXTCHAR: a word may hold only the letters A to Z and a to z.
	ErrInvalidWord = errors.New("lexicon: a word may hold only letters")
	// ErrNotInLexicon is LEXERR_NOTINLEX.
	ErrNotInLexicon = errors.New("lexicon: word not in the lexicon")
	// ErrInvalidPronunciation is LEXERR_INVALIDPRONCHAR: a pronunciation may hold only the symbols A to Z, a to z,
	// 1 to 5, @, |, ~ and &.
	ErrInvalidPronunciation = errors.New("lexicon: invalid pronunciation")
	// ErrAlreadyInLexicon is LEXERR_ALREADYINLEX: Add doesn't change a word the lexicon has.
	ErrAlreadyInLexicon = errors.New("lexicon: word already in the lexicon")
)

// Capacity is the most words the lexicon holds. Add gives a word it can't hold no error, and doesn't add it.
const Capacity = 5000

const pronunciationSymbols = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz12345@|~&"

type entry struct {
	word          []byte
	pronunciation []byte
}

// Lexicon is a user lexicon. It is safe for concurrent use: the engines read it while it changes.
type Lexicon struct {
	tables  *datapack.Text
	engine  *datapack.Engine
	phoneme func(text []byte) []byte

	lock    sync.Mutex
	entries []entry
	changed bool
}

// New makes an empty lexicon. phoneme converts a text, untagged, to engine phonemes as the engine's Phoneme does,
// with a NUL after them: Add takes a pronunciation in text through it.
func New(tables *datapack.Text, engine *datapack.Engine, phoneme func(text []byte) []byte) *Lexicon {
	return &Lexicon{tables: tables, engine: engine, phoneme: phoneme}
}

// Clone makes a lexicon with this one's words, unchanged, that converts pronunciations in text with phoneme: what
// a new process of the original has after reading the file this one's process saved.
func (lexicon *Lexicon) Clone(phoneme func(text []byte) []byte) *Lexicon {
	lexicon.lock.Lock()
	defer lexicon.lock.Unlock()
	return &Lexicon{tables: lexicon.tables, engine: lexicon.engine, phoneme: phoneme, entries: slices.Clone(lexicon.entries)}
}

// Add adds a word and its pronunciation as ILexPronounceW::Add does (engine.md §7.2). Word and pronunciation are
// UTF-16 code units, up to the first NUL, and the pronunciation is text, IPA or engine phonemes, as characterSet
// says. Spaces and quotation marks in the word go, but every second one of a run. The pronunciation is stored in
// engine phonemes, with each stress digit after the phoneme it stresses.
func (lexicon *Lexicon) Add(characterSet text.CharacterSet, word, pronunciation []uint16) error {
	if characterSet != text.Text && characterSet != text.IPA && characterSet != text.EnginePhonetic {
		return ErrInvalidArgument
	}
	converted := strip(text.ToANSI(lexicon.tables, wideString(word)))
	for _, character := range converted {
		if !isLetter(character) {
			return ErrInvalidWord
		}
	}
	if _, found := lexicon.find(converted); found {
		return ErrAlreadyInLexicon
	}
	var phonemes []byte
	switch characterSet {
	case text.EnginePhonetic:
		phonemes = text.ToANSI(lexicon.tables, wideString(pronunciation))
	case text.IPA:
		phonemes = text.IPAToEnginePhonemes(lexicon.tables, wideString(pronunciation))
	default:
		phonemes = lexicon.textPhonemes(strip(text.ToANSI(lexicon.tables, wideString(pronunciation))))
	}
	phonemes = strip(phonemes)
	for _, symbol := range phonemes {
		if bytes.IndexByte([]byte(pronunciationSymbols), symbol) < 0 {
			return ErrInvalidPronunciation
		}
	}
	lexicon.store(converted, phonemes)
	return nil
}

// textPhonemes converts a text to the phonemes of a pronunciation: the engine's phonemes for it, without a full stop
// at their end, then a final p, and then the word mark they start with.
func (lexicon *Lexicon) textPhonemes(value []byte) []byte {
	phonemes := cString(lexicon.phoneme(value))
	end := len(phonemes)
	if end > 0 && phonemes[end-1] == '.' {
		end--
	}
	if end > 0 && phonemes[end-1] == 'p' {
		end--
	}
	start := 0
	if end > 0 && (phonemes[0] == '&' || phonemes[0] == '%') {
		start = 1
	}
	return phonemes[start:end]
}

// AddANSI adds a word and its pronunciation as ILexPronounceA::Add does: they are code page 1252, and the
// pronunciation is text or engine phonemes; IPA gives ErrInvalidArgument. Otherwise it is Add.
func (lexicon *Lexicon) AddANSI(characterSet text.CharacterSet, word, pronunciation []byte) error {
	if characterSet != text.Text && characterSet != text.EnginePhonetic {
		return ErrInvalidArgument
	}
	return lexicon.Add(characterSet, text.ToWide(lexicon.tables, cString(word)), text.ToWide(lexicon.tables, cString(pronunciation)))
}

// Get returns a word's pronunciation, looked up in any case, as ILexPronounceW::Get gives it, without its NUL:
// engine phonemes for text and engine phonemes, or IPA. Unlike Add and Remove, it leaves the word's spaces and
// quotation marks.
func (lexicon *Lexicon) Get(characterSet text.CharacterSet, word []uint16) ([]uint16, error) {
	if characterSet != text.Text && characterSet != text.IPA && characterSet != text.EnginePhonetic {
		return nil, ErrInvalidArgument
	}
	phonemes, found := lexicon.find(text.ToANSI(lexicon.tables, wideString(word)))
	if !found {
		return nil, ErrNotInLexicon
	}
	if characterSet != text.IPA {
		return text.ToWide(lexicon.tables, phonemes), nil
	}
	converted := []uint16{}
	for _, symbol := range phonemes {
		if symbol >= ' ' && symbol <= '~' {
			converted = append(converted, lexicon.engine.IPA[symbol]...)
		}
	}
	return converted, nil
}

// GetANSI returns a word's pronunciation in engine phonemes, as ILexPronounceA::Get gives it: the word is code page
// 1252, and IPA gives ErrInvalidArgument. Otherwise it is Get.
func (lexicon *Lexicon) GetANSI(characterSet text.CharacterSet, word []byte) ([]byte, error) {
	if characterSet != text.Text && characterSet != text.EnginePhonetic {
		return nil, ErrInvalidArgument
	}
	phonemes, err := lexicon.Get(characterSet, text.ToWide(lexicon.tables, cString(word)))
	if err != nil {
		return nil, err
	}
	return text.ToANSI(lexicon.tables, phonemes), nil
}

// Remove removes a word, in any case, as ILexPronounceW::Remove does. Spaces and quotation marks in the word go as
// in Add.
func (lexicon *Lexicon) Remove(word []uint16) error {
	converted := upper(strip(text.ToANSI(lexicon.tables, wideString(word))))
	lexicon.lock.Lock()
	defer lexicon.lock.Unlock()
	index := lexicon.search(converted)
	if index < 0 {
		return ErrNotInLexicon
	}
	lexicon.entries = append(lexicon.entries[:index], lexicon.entries[index+1:]...)
	lexicon.changed = true
	return nil
}

// RemoveANSI removes a word given in code page 1252, as ILexPronounceA::Remove does.
func (lexicon *Lexicon) RemoveANSI(word []byte) error {
	return lexicon.Remove(text.ToWide(lexicon.tables, cString(word)))
}

// Pronunciation is the lexical stage's lookup (lexical.md §7): the pronunciation of a word in capitals, compared
// byte for byte.
func (lexicon *Lexicon) Pronunciation(word string) (string, bool) {
	lexicon.lock.Lock()
	defer lexicon.lock.Unlock()
	index := lexicon.search([]byte(word))
	if index < 0 {
		return "", false
	}
	return string(lexicon.entries[index].pronunciation), true
}

// Changed reports whether Add or Remove changed the lexicon since it was made or last written. The original saves
// its file when an application releases ILexPronounce after a change.
func (lexicon *Lexicon) Changed() bool {
	lexicon.lock.Lock()
	defer lexicon.lock.Unlock()
	return lexicon.changed
}

func (lexicon *Lexicon) find(word []byte) ([]byte, bool) {
	lexicon.lock.Lock()
	defer lexicon.lock.Unlock()
	index := lexicon.search(upper(word))
	if index < 0 {
		return nil, false
	}
	return lexicon.entries[index].pronunciation, true
}

// store adds a word in capitals, when the lexicon has room for it, and sorts the lexicon again. A word the lexicon
// has gets the new pronunciation: Add refuses such a word first, but two Adds of one word can reach here at once.
func (lexicon *Lexicon) store(word, pronunciation []byte) {
	lexicon.lock.Lock()
	defer lexicon.lock.Unlock()
	if len(lexicon.entries) >= Capacity {
		return
	}
	word = upper(word)
	if index := lexicon.search(word); index >= 0 {
		lexicon.entries[index].pronunciation = bytes.Clone(pronunciation)
	} else {
		lexicon.entries = append(lexicon.entries, entry{word: word, pronunciation: bytes.Clone(pronunciation)})
		sortEntries(lexicon.entries)
	}
	lexicon.changed = true
}

// strip removes spaces and quotation marks, but the character after each one it removes, which it doesn't look at.
func strip(value []byte) []byte {
	value = bytes.Clone(value)
	for index := 0; index < len(value); index++ {
		if value[index] == ' ' || value[index] == '"' {
			value = append(value[:index], value[index+1:]...)
		}
	}
	return value
}

func isLetter(character byte) bool {
	return character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z'
}

func upper(word []byte) []byte {
	raised := make([]byte, len(word))
	for index, character := range word {
		if character >= 'a' && character <= 'z' {
			character -= 'a' - 'A'
		}
		raised[index] = character
	}
	return raised
}

func cString(value []byte) []byte {
	if end := bytes.IndexByte(value, 0); end >= 0 {
		return value[:end]
	}
	return value
}

func wideString(value []uint16) []uint16 {
	for index, character := range value {
		if character == 0 {
			return value[:index]
		}
	}
	return value
}
