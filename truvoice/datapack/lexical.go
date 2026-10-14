package datapack

import (
	"encoding/json"
	"fmt"
	"math"
)

// Lexical is the lexical stage's tables: the lists of function words it looks a word up in, in order, and the
// endings it tests a word no list has for.
type Lexical struct {
	FunctionWords       []FunctionWordList
	FunctionWordEndings []FunctionWordEnding
}

// A FunctionWordList is a list of words, each between two #, which give a word in it Class, but the Exceptions.
type FunctionWordList struct {
	Words      string
	Class      int32
	Exceptions []WordException
}

// A FunctionWordEnding gives a word that ends in Ending after a letter at least Class, but the Exceptions.
type FunctionWordEnding struct {
	Ending     string
	Class      int32
	Exceptions []WordException
}

// A WordException gives the word Word, between two #, Class.
type WordException struct {
	Word  string
	Class int32
}

type lexicalDescription struct {
	FunctionWords       []functionWordListDescription `json:"functionWords"`
	FunctionWordEndings []functionWordListDescription `json:"functionWordEndings"`
}

type functionWordListDescription struct {
	Words      string                     `json:"words"`
	Ending     string                     `json:"ending"`
	Class      *int64                     `json:"class"`
	Exceptions []wordExceptionDescription `json:"exceptions"`
}

type wordExceptionDescription struct {
	Word  string `json:"word"`
	Class *int64 `json:"class"`
}

func (pack *Pack) LoadLexical() (*Lexical, error) {
	return load(pack, "lexical.json", parseLexical)
}

func parseLexical(data []byte) (*Lexical, error) {
	var description lexicalDescription
	if err := json.Unmarshal(data, &description); err != nil {
		return nil, err
	}
	lexical := &Lexical{}
	if len(description.FunctionWords) == 0 {
		return nil, fmt.Errorf("functionWords is empty")
	}
	for index, list := range description.FunctionWords {
		name := fmt.Sprintf("functionWords[%d]", index)
		var parsed FunctionWordList
		var err error
		parsed.Words = list.Words
		if err = checkLetters(parsed.Words, name+".words"); err != nil {
			return nil, err
		}
		if parsed.Class, parsed.Exceptions, err = parseWordClass(list, name); err != nil {
			return nil, err
		}
		lexical.FunctionWords = append(lexical.FunctionWords, parsed)
	}
	for index, ending := range description.FunctionWordEndings {
		name := fmt.Sprintf("functionWordEndings[%d]", index)
		var parsed FunctionWordEnding
		var err error
		parsed.Ending = ending.Ending
		if err = checkLetters(parsed.Ending, name+".ending"); err != nil {
			return nil, err
		}
		if parsed.Class, parsed.Exceptions, err = parseWordClass(ending, name); err != nil {
			return nil, err
		}
		lexical.FunctionWordEndings = append(lexical.FunctionWordEndings, parsed)
	}
	return lexical, nil
}

func parseWordClass(description functionWordListDescription, name string) (int32, []WordException, error) {
	var class int32
	if err := copyValue(&class, description.Class, name+".class", 0, math.MaxUint8); err != nil {
		return 0, nil, err
	}
	var exceptions []WordException
	for index, exception := range description.Exceptions {
		exceptionName := fmt.Sprintf("%s.exceptions[%d]", name, index)
		parsed := WordException{Word: exception.Word}
		if err := checkLetters(parsed.Word, exceptionName+".word"); err != nil {
			return 0, nil, err
		}
		if err := copyValue(&parsed.Class, exception.Class, exceptionName+".class", 0, math.MaxUint8); err != nil {
			return 0, nil, err
		}
		exceptions = append(exceptions, parsed)
	}
	return class, exceptions, nil
}

// checkLetters checks that a text is a C string of symbols: 1 to 127.
func checkLetters(text, name string) error {
	if len(text) == 0 {
		return fmt.Errorf("%s is empty", name)
	}
	for index := range len(text) {
		if text[index] == 0 || text[index] >= SymbolCount {
			return fmt.Errorf("%s[%d] is %d, expected a symbol from 1 to %d", name, index, text[index], SymbolCount-1)
		}
	}
	return nil
}
