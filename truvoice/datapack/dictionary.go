package datapack

import (
	"encoding/json"
	"fmt"
	"strings"
)

// The dictionary's limits: the letters a word has after its first, and an entry's phonemes with their stress
// digits.
const (
	DictionaryLetters  = 20
	DictionaryPhonemes = 15
)

// Dictionary is the lexical stage's dictionary: its entries, and the homographs whose verb's entry the words
// before them choose.
type Dictionary struct {
	// Entries holds each first letter's entries, A to Z, by the number of letters after the first, each list in
	// the order the lookup searches it.
	Entries [26][][]DictionaryEntry
	// Homographs are the words, each between two #, whose verb's entry depends on the words before them.
	Homographs string
	// VerbEndings are the endings after which a homograph's stem takes its verb's entry, and PluralEndings
	// those that make it plural, for the words before to choose.
	VerbEndings   []string
	PluralEndings []string
	// NounBeforeColon is the homograph, between two #, that is a noun before a colon, and PastTense the word,
	// between two #, whose first entry the forms of be and have, and he, she and it, skip.
	NounBeforeColon string
	PastTense       string
}

// A DictionaryEntry is a word's entry: its letters after the first, its class, and its phonemes with their
// stress digits, or its stress pattern when it has none.
type DictionaryEntry struct {
	Letters       string
	Class         byte
	Phonemes      string
	StressPattern byte
}

type dictionaryDescription struct {
	Words           []dictionaryWordDescription `json:"words"`
	Homographs      string                      `json:"homographs"`
	VerbEndings     []string                    `json:"verbEndings"`
	PluralEndings   []string                    `json:"pluralEndings"`
	NounBeforeColon string                      `json:"nounBeforeColon"`
	PastTense       string                      `json:"pastTense"`
}

type dictionaryWordDescription struct {
	Word          string  `json:"word"`
	Class         *int64  `json:"class"`
	Phonemes      *string `json:"phonemes"`
	StressPattern *int64  `json:"stressPattern"`
}

func (pack *Pack) LoadDictionary() (*Dictionary, error) {
	return load(pack, "dictionary.json", parseDictionary)
}

func parseDictionary(data []byte) (*Dictionary, error) {
	var description dictionaryDescription
	if err := json.Unmarshal(data, &description); err != nil {
		return nil, err
	}
	dictionary := &Dictionary{}
	if len(description.Words) == 0 {
		return nil, fmt.Errorf("words is empty")
	}
	for index, word := range description.Words {
		name := fmt.Sprintf("words[%d]", index)
		entry, err := parseDictionaryWord(word, name)
		if err != nil {
			return nil, err
		}
		buckets := &dictionary.Entries[word.Word[0]-'A']
		for len(*buckets) <= len(entry.Letters) {
			*buckets = append(*buckets, nil)
		}
		(*buckets)[len(entry.Letters)] = append((*buckets)[len(entry.Letters)], entry)
	}
	dictionary.Homographs = description.Homographs
	if err := checkWords(dictionary.Homographs, "homographs"); err != nil {
		return nil, err
	}
	var err error
	if dictionary.VerbEndings, err = parseEndings(description.VerbEndings, "verbEndings"); err != nil {
		return nil, err
	}
	if dictionary.PluralEndings, err = parseEndings(description.PluralEndings, "pluralEndings"); err != nil {
		return nil, err
	}
	dictionary.NounBeforeColon = description.NounBeforeColon
	if err := checkWords(dictionary.NounBeforeColon, "nounBeforeColon"); err != nil {
		return nil, err
	}
	dictionary.PastTense = description.PastTense
	if err := checkWords(dictionary.PastTense, "pastTense"); err != nil {
		return nil, err
	}
	return dictionary, nil
}

func parseDictionaryWord(word dictionaryWordDescription, name string) (DictionaryEntry, error) {
	var entry DictionaryEntry
	if len(word.Word) == 0 || len(word.Word) > DictionaryLetters+1 || word.Word[0] < 'A' || word.Word[0] > 'Z' ||
		strings.ContainsFunc(word.Word[1:], func(letter rune) bool { return letter < '@' || letter > 'Z' }) {
		return entry, fmt.Errorf("%s.word is %q, expected a letter A to Z and up to %d of @ and A to Z", name, word.Word, DictionaryLetters)
	}
	entry.Letters = word.Word[1:]
	var class int32
	if err := copyValue(&class, word.Class, name+".class", 0, 15); err != nil {
		return entry, err
	}
	entry.Class = byte(class)
	switch {
	case word.Phonemes != nil && word.StressPattern != nil:
		return entry, fmt.Errorf("%s has phonemes and a stress pattern", name)
	case word.Phonemes != nil:
		entry.Phonemes = *word.Phonemes
		if err := checkLetters(entry.Phonemes, name+".phonemes"); err != nil {
			return entry, err
		}
		if len(entry.Phonemes) > DictionaryPhonemes {
			return entry, fmt.Errorf("%s.phonemes has %d symbols, more than %d", name, len(entry.Phonemes), DictionaryPhonemes)
		}
		if strings.Trim(entry.Phonemes, "012") == "" {
			return entry, fmt.Errorf("%s.phonemes has only stress digits", name)
		}
	default:
		var pattern int32
		if err := copyValue(&pattern, word.StressPattern, name+".stressPattern", 0, 255); err != nil {
			return entry, err
		}
		entry.StressPattern = byte(pattern)
	}
	return entry, nil
}

func parseEndings(endings []string, name string) ([]string, error) {
	if len(endings) == 0 {
		return nil, fmt.Errorf("%s is empty", name)
	}
	for index, ending := range endings {
		if err := checkLetters(ending, fmt.Sprintf("%s[%d]", name, index)); err != nil {
			return nil, err
		}
	}
	return endings, nil
}

// checkWords checks that a text is words of symbols, each between two #.
func checkWords(text, name string) error {
	if err := checkLetters(text, name); err != nil {
		return err
	}
	if len(text) < 3 || text[0] != '#' || text[len(text)-1] != '#' || strings.Contains(text, "##") {
		return fmt.Errorf("%s is %q, expected words each between two #", name, text)
	}
	return nil
}
