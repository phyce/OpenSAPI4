package datapack

import (
	"encoding/json"
	"fmt"
	"slices"
)

const (
	FilterCount         = 10
	HighCharacterCount  = 64
	FirstHighCharacter  = 0xc0
	highCharacterLength = 2
	CharacterCount      = 256
)

// Input is the escape parser's tables: the letters it writes for each character from 0xC0 to 0xFF, one or two,
// the highest rate index each filter value allows, and the highest level of each track. And the TextIn
// tokenizer's: each character's token classes; the words it says for the symbols, by character, and before a
// negative number; the words it splits; "am" and how it writes it after a number; the months it writes out;
// the command it puts before an exclamation in quotes; the mark it puts before a word; the quote marks; and
// what it says at the start and the end of quoted text.
type Input struct {
	HighCharacters [HighCharacterCount][highCharacterLength]byte
	FilterRates    [FilterCount]int32
	LevelLimits    [TrackCount]int32

	TokenClasses           [CharacterCount]byte
	Symbols                [CharacterCount]string
	Minus                  string
	SplitWords             [][2]string
	Meridiem               [2]string
	Months                 [][2]string
	Exclamation, WordStart string
	QuoteMarks             string
	QuoteStart, QuoteEnd   string
}

type inputDescription struct {
	HighCharacters []string    `json:"highCharacters"`
	FilterRates    []int64     `json:"filterRates"`
	LevelLimits    []int64     `json:"levelLimits"`
	TokenClasses   []int64     `json:"tokenClasses"`
	Symbols        [][2]any    `json:"symbols"`
	Minus          string      `json:"minus"`
	SplitWords     [][2]string `json:"splitWords"`
	Meridiem       [2]string   `json:"meridiem"`
	Months         [][2]string `json:"months"`
	Exclamation    string      `json:"exclamation"`
	WordStart      string      `json:"wordStart"`
	QuoteMarks     string      `json:"quoteMarks"`
	QuoteStart     string      `json:"quoteStart"`
	QuoteEnd       string      `json:"quoteEnd"`
}

func (pack *Pack) LoadInput() (*Input, error) {
	return load(pack, "input.json", parseInput)
}

func parseInput(data []byte) (*Input, error) {
	var description inputDescription
	if err := json.Unmarshal(data, &description); err != nil {
		return nil, err
	}
	input := &Input{}
	if len(description.HighCharacters) != HighCharacterCount {
		return nil, fmt.Errorf("highCharacters has %d entries, expected %d", len(description.HighCharacters), HighCharacterCount)
	}
	for index, letters := range description.HighCharacters {
		if len(letters) < 1 || len(letters) > highCharacterLength {
			return nil, fmt.Errorf("highCharacters[%d] is %q, not one or two characters", index, letters)
		}
		for position := range len(letters) {
			if letters[position] < ' ' || letters[position] > '~' {
				return nil, fmt.Errorf("highCharacters[%d] is %q, not printable ASCII", index, letters)
			}
			input.HighCharacters[index][position] = letters[position]
		}
	}
	if err := copyValues(input.FilterRates[:], description.FilterRates, "filterRates", 0, MaximumRate); err != nil {
		return nil, err
	}
	if err := copyValues(input.LevelLimits[:], description.LevelLimits, "levelLimits", 0, 255); err != nil {
		return nil, err
	}
	if err := copyValues(input.TokenClasses[:], description.TokenClasses, "tokenClasses", 0, 255); err != nil {
		return nil, err
	}
	for index, symbol := range description.Symbols {
		character, isNumber := symbol[0].(float64)
		word, isString := symbol[1].(string)
		if !isNumber || !isString || character != float64(int(character)) || character < 0x80 || character > 0xff {
			return nil, fmt.Errorf("symbols[%d] is %v, not a character from 128 to 255 and its word", index, symbol)
		}
		if input.Symbols[int(character)] != "" {
			return nil, fmt.Errorf("symbols[%d]: character %d has a word already", index, int(character))
		}
		if err := checkWord(fmt.Sprintf("symbols[%d]", index), word); err != nil {
			return nil, err
		}
		input.Symbols[int(character)] = word
	}
	input.Minus, input.Exclamation, input.WordStart = description.Minus, description.Exclamation, description.WordStart
	input.SplitWords, input.Meridiem, input.Months = description.SplitWords, description.Meridiem, description.Months
	input.QuoteMarks, input.QuoteStart, input.QuoteEnd = description.QuoteMarks, description.QuoteStart, description.QuoteEnd
	words := [][2]string{{"minus", input.Minus}, {"exclamation", input.Exclamation}, {"wordStart", input.WordStart},
		{"quoteMarks", input.QuoteMarks}, {"quoteStart", input.QuoteStart}, {"quoteEnd", input.QuoteEnd},
		{"meridiem", input.Meridiem[0]}, {"meridiem", input.Meridiem[1]}}
	for _, pair := range slices.Concat(input.SplitWords, input.Months) {
		words = append(words, [2]string{"splitWords and months", pair[0]}, [2]string{"splitWords and months", pair[1]})
	}
	for _, word := range words {
		if err := checkWord(word[0], word[1]); err != nil {
			return nil, err
		}
	}
	return input, nil
}

// checkWord reports an error when a word of the tokenizer's is empty or isn't ASCII without NUL.
func checkWord(name, word string) error {
	if word == "" {
		return fmt.Errorf("%s has an empty word", name)
	}
	for position := range len(word) {
		if word[position] == 0 || word[position] > 0x7f {
			return fmt.Errorf("%s has %q, not ASCII without NUL", name, word)
		}
	}
	return nil
}
