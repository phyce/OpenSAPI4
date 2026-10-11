// Package lexicon is the user lexicon of the Microsoft engine (pronunciation.md §6.3–6.5): words with
// pronunciations in ARPAbet, which every engine looks up before its built-in exceptions and its rules, the
// file the original keeps them in, and the conversions between ARPAbet, engine phoneme names and IPA.
package lexicon

import (
	"bytes"
	"errors"
	"io"
	"sort"
	"sync"
	"unicode/utf16"

	"github.com/phyce/OpenSAPI4/whistler/datapack"
	"github.com/phyce/OpenSAPI4/whistler/text"
)

var (
	// ErrInvalidWord is LEXERR_INVALIDTEXTCHAR: a word may hold only ASCII letters and - ' .
	ErrInvalidWord = errors.New("lexicon: a word may hold only letters and - ' .")
	// ErrInvalidPronunciation is LEXERR_INVALIDPRONCHAR: a pronunciation the engine can't read.
	ErrInvalidPronunciation = errors.New("lexicon: invalid pronunciation")
	// ErrNotInLexicon is LEXERR_NOTINLEX.
	ErrNotInLexicon = errors.New("lexicon: word not in the lexicon")
	// ErrInvalidFile is a user lexicon file the original rejects; the lexicon is then empty.
	ErrInvalidFile = errors.New("lexicon: invalid user lexicon file")
)

const (
	fileHeader = "[MSTTSUSERLEXVER100]\r\n"
	escape     = 0x1B
)

type entry struct {
	word          []byte
	pronunciation []byte
}

// Lexicon is a user lexicon. It is safe for concurrent use: the engines read it while it changes.
type Lexicon struct {
	tables       *datapack.Text
	preprocessor *text.Preprocessor

	lock     sync.Mutex
	entries  []entry
	modified bool
}

// New makes an empty lexicon.
func New(tables *datapack.Text) *Lexicon {
	return &Lexicon{tables: tables, preprocessor: text.New(tables)}
}

// Modified reports whether Add or Remove changed the lexicon since it was made or read. The original
// writes its file back only then.
func (lexicon *Lexicon) Modified() bool {
	lexicon.lock.Lock()
	defer lexicon.lock.Unlock()
	return lexicon.modified
}

// Add adds a word with its pronunciation in engine phoneme names (hh ax l 1ow), as ILexPronounceA::Add does.
// The word is converted to code page 1252 with best fit, as ILexPronounceW::Add converts it. It reports
// false, and changes nothing, if the lexicon has the word already, in any case: Remove it first to change
// its pronunciation. The pronunciation is stored in ARPAbet, upper case (pronunciation.md §6.4).
func (lexicon *Lexicon) Add(word, phonemes string) (bool, error) {
	return lexicon.add(lexicon.ANSI(word), cString([]byte(phonemes)))
}

// AddIPA adds a word with its pronunciation in IPA (həlˈo), as ILexPronounceW::Add does.
func (lexicon *Lexicon) AddIPA(word, ipa string) (bool, error) {
	names, converted := lexicon.preprocessor.IPAToEngineNames(utf16.Encode([]rune(ipa)))
	if !converted {
		return false, ErrInvalidPronunciation
	}
	return lexicon.add(lexicon.ANSI(word), names)
}

func (lexicon *Lexicon) add(word, names []byte) (bool, error) {
	if !ValidWord(word) {
		return false, ErrInvalidWord
	}
	if bytes.IndexByte(names, escape) >= 0 {
		return false, ErrInvalidPronunciation
	}
	arpabet, parsed := lexicon.EngineNamesToARPAbet(names)
	if !parsed {
		return false, ErrInvalidPronunciation
	}
	lexicon.lock.Lock()
	defer lexicon.lock.Unlock()
	index := 0
	for ; index < len(lexicon.entries); index++ {
		order := compare(word, lexicon.entries[index].word)
		if order == 0 {
			return false, nil
		}
		if order < 0 {
			break
		}
	}
	lexicon.modified = true
	lexicon.entries = append(lexicon.entries, entry{})
	copy(lexicon.entries[index+1:], lexicon.entries[index:])
	lexicon.entries[index] = entry{word: upper(word), pronunciation: upper(arpabet)}
	return true, nil
}

// Remove removes a word, in any case.
func (lexicon *Lexicon) Remove(word string) error {
	ansi := lexicon.ANSI(word)
	if !ValidWord(ansi) {
		return ErrInvalidWord
	}
	lexicon.lock.Lock()
	defer lexicon.lock.Unlock()
	for index, current := range lexicon.entries {
		order := compare(ansi, current.word)
		if order < 0 {
			break
		}
		if order == 0 {
			lexicon.modified = true
			lexicon.entries = append(lexicon.entries[:index], lexicon.entries[index+1:]...)
			return nil
		}
	}
	return ErrNotInLexicon
}

// Find returns the stored pronunciation of a word in ANSI, looked up ignoring case.
func (lexicon *Lexicon) Find(word []byte) ([]byte, bool) {
	lexicon.lock.Lock()
	defer lexicon.lock.Unlock()
	index := sort.Search(len(lexicon.entries), func(index int) bool { return compare(lexicon.entries[index].word, word) >= 0 })
	if index < len(lexicon.entries) && compare(lexicon.entries[index].word, word) == 0 {
		return lexicon.entries[index].pronunciation, true
	}
	return nil, false
}

// Pronunciation is a word's pronunciation in the lexicon, in engine phoneme names, as
// ILexPronounce2A::GetFrom with ILP2_USER gives it: stress marks just before their vowels, no separators.
func (lexicon *Lexicon) Pronunciation(word string) (string, error) {
	names, err := lexicon.pronunciation(word)
	return string(names), err
}

// PronunciationIPA is a word's pronunciation in the lexicon in IPA, as ILexPronounce2W::GetFrom gives it.
func (lexicon *Lexicon) PronunciationIPA(word string) (string, error) {
	names, err := lexicon.pronunciation(word)
	if err != nil {
		return "", err
	}
	return lexicon.IPA(names), nil
}

func (lexicon *Lexicon) pronunciation(word string) ([]byte, error) {
	ansi := lexicon.ANSI(word)
	if !ValidWord(ansi) {
		return nil, ErrInvalidWord
	}
	arpabet, found := lexicon.Find(ansi)
	if !found {
		return nil, ErrNotInLexicon
	}
	return ARPAbetToEngineNames(arpabet), nil
}

// IPA converts engine phoneme names to IPA as the wide interfaces do.
func (lexicon *Lexicon) IPA(names []byte) string {
	return string(utf16.Decode(lexicon.preprocessor.EngineNamesToIPA(names)))
}

// ANSI converts a word as the wide interfaces do: to code page 1252 with best fit, up to the first U+0000.
func (lexicon *Lexicon) ANSI(word string) []byte {
	return lexicon.preprocessor.ToANSI(utf16.Encode([]rune(word)))
}

// Read replaces the lexicon with the entries of a user lexicon file, as the original reads its file when its
// first engine starts (pronunciation.md §6.3). A file the original rejects leaves the lexicon empty and gives
// ErrInvalidFile, as does a file that can't be read, with that error.
func (lexicon *Lexicon) Read(file io.Reader) error {
	data, err := io.ReadAll(file)
	lexicon.lock.Lock()
	defer lexicon.lock.Unlock()
	lexicon.entries, lexicon.modified = nil, false
	if err != nil {
		return err
	}
	entries, read := lexicon.parseFile(cString(data))
	if !read {
		return ErrInvalidFile
	}
	lexicon.entries = entries
	return nil
}

func (lexicon *Lexicon) parseFile(data []byte) ([]entry, bool) {
	if !bytes.HasPrefix(data, []byte(fileHeader)) {
		return nil, false
	}
	count := bytes.Count(data, []byte{'\r'}) - 1
	var entries []entry
	previous := []byte{}
	position := len(fileHeader)
	for range count {
		tab := bytes.IndexByte(data[position:], '\t')
		if tab < 0 {
			return nil, false
		}
		word := data[position : position+tab]
		if !ValidWord(word) || compare(previous, word) >= 0 {
			return nil, false
		}
		previous = word
		position += tab + 1
		end := bytes.IndexByte(data[position:], '\r')
		if end < 0 || position+end+1 >= len(data) || data[position+end+1] != '\n' {
			return nil, false
		}
		pronunciation := data[position : position+end]
		if _, parsed := lexicon.preprocessor.ParseEnginePhonemes(ARPAbetToEngineNames(pronunciation)); !parsed {
			return nil, false
		}
		entries = append(entries, entry{word: bytes.Clone(word), pronunciation: bytes.Clone(pronunciation)})
		position += end + 2
	}
	return entries, position == len(data)
}

// Write writes the lexicon in the original's file format.
func (lexicon *Lexicon) Write(file io.Writer) error {
	lexicon.lock.Lock()
	output := []byte(fileHeader)
	for _, current := range lexicon.entries {
		output = append(append(append(append(output, current.word...), '\t'), current.pronunciation...), '\r', '\n')
	}
	lexicon.lock.Unlock()
	_, err := file.Write(output)
	return err
}

// EngineNamesToARPAbet converts engine phoneme names to the lexicon's ARPAbet (pronunciation.md §6.4): each
// phonetic symbol's ARPAbet name followed by a space, a vowel with the stress digit before it, or 0. It
// reports false where the parse fails. Names with an escape are not converted.
func (lexicon *Lexicon) EngineNamesToARPAbet(names []byte) ([]byte, bool) {
	items, parsed := lexicon.preprocessor.ParseEnginePhonemes(names)
	primary, secondary := lexicon.preprocessor.FindEnginePhoneme("1"), lexicon.preprocessor.FindEnginePhoneme("2")
	firstConsonant := lexicon.preprocessor.FindEnginePhoneme("p")
	arpabet := []byte{}
	stress := 0
	for _, item := range items {
		if item.Escape != nil {
			continue
		}
		if item.Code == primary || item.Code == secondary {
			stress = item.Code
			continue
		}
		arpabet = append(arpabet, lexicon.tables.Phonemes[item.Code-1].ARPAbetName...)
		if item.Code < firstConsonant {
			if stress == 0 {
				arpabet = append(arpabet, '0')
			} else {
				arpabet = append(arpabet, lexicon.tables.Phonemes[stress-1].ARPAbetName...)
				stress = 0
			}
		}
		arpabet = append(arpabet, ' ')
	}
	return arpabet, parsed
}

// ARPAbetToEngineNames converts the lexicon's ARPAbet to engine phoneme names (pronunciation.md §6.4): every
// name followed by a space, AH0 as ax, NG as nx, a final 1 written before the name, a final 0 dropped, the
// rest in lower case, with no separators. What follows the last space is left out.
func ARPAbetToEngineNames(arpabet []byte) []byte {
	names := []byte{}
	for {
		end := bytes.IndexByte(arpabet, ' ')
		if end < 0 {
			return names
		}
		token := arpabet[:end]
		switch {
		case equalFold(token, []byte("AH0")):
			names = append(names, "ax"...)
		case equalFold(token, []byte("NG")):
			names = append(names, "nx"...)
		case len(token) > 0:
			last := token[len(token)-1]
			if last == '1' {
				names = append(names, '1')
			}
			if last == '1' || last == '0' {
				token = token[:len(token)-1]
			}
			names = append(names, lower(token)...)
		}
		arpabet = arpabet[end+1:]
	}
}

// UnitsToARPAbet joins the names of the units that are phones, each followed by a space.
func UnitsToARPAbet(names []string) []byte {
	arpabet := []byte{}
	for _, name := range names {
		if name != "" && (name[0] == '#' || name[0] == '\\') || equalFold([]byte(name), []byte("SIL")) {
			continue
		}
		arpabet = append(append(arpabet, name...), ' ')
	}
	return arpabet
}

// ValidWord reports whether a word in ANSI holds only ASCII letters and - ' . (an empty word does).
func ValidWord(word []byte) bool {
	for _, character := range word {
		if !isLetter(character) && character != '-' && character != '\'' && character != '.' {
			return false
		}
	}
	return true
}

func isLetter(character byte) bool {
	return character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z'
}

// compare orders words as _stricmp does in the C locale.
func compare(first, second []byte) int {
	for index := 0; ; index++ {
		var left, right byte
		if index < len(first) {
			left = lowerByte(first[index])
		}
		if index < len(second) {
			right = lowerByte(second[index])
		}
		if left != right || left == 0 {
			return int(left) - int(right)
		}
	}
}

func equalFold(first, second []byte) bool {
	return len(first) == len(second) && compare(first, second) == 0
}

func lowerByte(character byte) byte {
	if character >= 'A' && character <= 'Z' {
		return character + 'a' - 'A'
	}
	return character
}

func lower(word []byte) []byte {
	lowered := make([]byte, len(word))
	for index, character := range word {
		lowered[index] = lowerByte(character)
	}
	return lowered
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

func cString(data []byte) []byte {
	if end := bytes.IndexByte(data, 0); end >= 0 {
		return data[:end]
	}
	return data
}
