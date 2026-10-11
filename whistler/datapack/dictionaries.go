package datapack

import (
	"encoding/json"
	"fmt"
	"math"
)

const (
	dictionaryWordLength  = 31
	dictionaryEntryLength = 64
)

type DictionaryField struct {
	Tier      int
	Separator byte
}

// Dictionary is one of the dictionaries module C looks up. An entry holds its fields in order; every field
// but the last ends at its separator.
type Dictionary struct {
	Procedure int
	Fields    []DictionaryField
	Words     map[string]string
}

type BuiltInException struct {
	Word          string
	Pronunciation string
}

type Dictionaries struct {
	Dictionaries      map[int]*Dictionary
	BuiltInExceptions []BuiltInException
}

type dictionaryFieldDescription struct {
	Tier      int    `json:"tier"`
	Separator string `json:"separator"`
}

type dictionaryDescription struct {
	Procedure int                          `json:"procedure"`
	Fields    []dictionaryFieldDescription `json:"fields"`
	Words     map[string]string            `json:"words"`
}

type builtInExceptionDescription struct {
	Word          string `json:"word"`
	Pronunciation string `json:"pronunciation"`
}

type dictionariesDescription struct {
	Dictionaries      []dictionaryDescription       `json:"dictionaries"`
	BuiltInExceptions []builtInExceptionDescription `json:"builtInExceptions"`
}

func (pack *Pack) LoadDictionaries() (*Dictionaries, error) {
	return load(pack, "dictionaries.json", parseDictionaries)
}

// byteString turns a JSON string whose characters U+0000 to U+00FF stand for bytes into those bytes.
func byteString(text string) (string, bool) {
	data := make([]byte, 0, len(text))
	for _, character := range text {
		if character > math.MaxUint8 {
			return "", false
		}
		data = append(data, byte(character))
	}
	return string(data), true
}

func parseDictionaries(data []byte) (*Dictionaries, error) {
	var description dictionariesDescription
	if err := json.Unmarshal(data, &description); err != nil {
		return nil, err
	}
	dictionaries := &Dictionaries{Dictionaries: map[int]*Dictionary{}}
	for index := range description.Dictionaries {
		dictionary, err := parseDictionary(&description.Dictionaries[index])
		if err != nil {
			return nil, err
		}
		if _, found := dictionaries.Dictionaries[dictionary.Procedure]; found {
			return nil, fmt.Errorf("dictionary %d appears twice", dictionary.Procedure)
		}
		dictionaries.Dictionaries[dictionary.Procedure] = dictionary
	}
	for _, exception := range description.BuiltInExceptions {
		if !printable(exception.Word) || !printable(exception.Pronunciation) {
			return nil, fmt.Errorf("built-in exception %q: %q", exception.Word, exception.Pronunciation)
		}
		dictionaries.BuiltInExceptions = append(dictionaries.BuiltInExceptions, BuiltInException(exception))
	}
	return dictionaries, nil
}

func printable(text string) bool {
	for _, character := range []byte(text) {
		if character < 0x20 || character > 0x7E {
			return false
		}
	}
	return text != ""
}

func parseDictionary(description *dictionaryDescription) (*Dictionary, error) {
	dictionary := &Dictionary{Procedure: description.Procedure, Words: make(map[string]string, len(description.Words))}
	if dictionary.Procedure < 3 || dictionary.Procedure > math.MaxUint8 || len(description.Fields) < 1 || len(description.Fields) > 3 {
		return nil, fmt.Errorf("dictionary %d with %d fields", dictionary.Procedure, len(description.Fields))
	}
	var tiers [ruleTiers + 1]bool
	for index, field := range description.Fields {
		last := index == len(description.Fields)-1
		separator, valid := byteString(field.Separator)
		if last {
			valid = valid && separator == ""
		} else {
			valid = valid && len(separator) == 1 && separator[0] >= 0x21 && separator[0] <= 0x7E
		}
		if field.Tier != 1 && field.Tier != 3 && field.Tier != 4 || tiers[field.Tier] || !valid {
			return nil, fmt.Errorf("dictionary %d: field %d writes tier %d, separator %q", dictionary.Procedure, index+1, field.Tier, field.Separator)
		}
		tiers[field.Tier] = true
		entry := DictionaryField{Tier: field.Tier}
		if !last {
			entry.Separator = separator[0]
		}
		dictionary.Fields = append(dictionary.Fields, entry)
	}
	for wordText, entryText := range description.Words {
		word, wordValid := byteString(wordText)
		entry, entryValid := byteString(entryText)
		if !wordValid || !entryValid || len(word) < 1 || len(word) > dictionaryWordLength || len(entry) > dictionaryEntryLength {
			return nil, fmt.Errorf("dictionary %d: word %q, entry %q", dictionary.Procedure, wordText, entryText)
		}
		for _, character := range []byte(word) {
			if character < 0x20 || character == 0xFF {
				return nil, fmt.Errorf("dictionary %d: word %q", dictionary.Procedure, wordText)
			}
		}
		position := 0
		for _, character := range []byte(entry) {
			if character <= RuleEdge || character > RuleLastSymbol {
				return nil, fmt.Errorf("dictionary %d: entry %q of %q", dictionary.Procedure, entryText, wordText)
			}
			if position < len(dictionary.Fields)-1 && character == dictionary.Fields[position].Separator {
				position++
			}
		}
		if position != len(dictionary.Fields)-1 {
			return nil, fmt.Errorf("dictionary %d: entry %q of %q lacks a field", dictionary.Procedure, entryText, wordText)
		}
		dictionary.Words[word] = entry
	}
	return dictionary, nil
}
