package datapack

import (
	"encoding/json"
	"fmt"
)

const (
	MonthCount  = 12
	TeenCount   = 10
	TensCount   = 8
	DigitsCount = 9
)

// PreFormatter is the words the pre-formatter writes and reads.
type PreFormatter struct {
	Spreadsheet Spreadsheet
	Address     Address
}

// Spreadsheet is the words the spreadsheet context writes: the months; a fraction's denominators of one digit,
// 1 to 9, of 10 to 19, of 20, 30 and so on to 90, and the units after the tens; the words for a second character
// that isn't a digit, after 1 and after another; the words for minus and and, and the words for an exponent.
type Spreadsheet struct {
	Months       [MonthCount]string
	Denominators [DigitsCount]string
	Teens        [TeenCount]string
	Tens         [TensCount]string
	Ordinals     [DigitsCount]string
	OtherUnits   [2]string
	Minus        string
	And          string
	Exponent     string
}

// Address is the words the address and e-mail contexts read: the function words, after which a dot ends no
// sentence, and the abbreviations, after which no sentence starts, with those whose dot keeps a word's spaces.
type Address struct {
	FunctionWords       []string
	Abbreviations       []string
	SpacedAbbreviations []string
}

type preFormatterDescription struct {
	Spreadsheet *spreadsheetDescription `json:"spreadsheet"`
	Address     *addressDescription     `json:"address"`
}

type addressDescription struct {
	FunctionWords       []string `json:"functionWords"`
	Abbreviations       []string `json:"abbreviations"`
	SpacedAbbreviations []string `json:"spacedAbbreviations"`
}

type spreadsheetDescription struct {
	Months       []string `json:"months"`
	Denominators []string `json:"denominators"`
	Teens        []string `json:"teens"`
	Tens         []string `json:"tens"`
	Ordinals     []string `json:"ordinals"`
	OtherUnits   []string `json:"otherUnits"`
	Minus        string   `json:"minus"`
	And          string   `json:"and"`
	Exponent     string   `json:"exponent"`
}

func (pack *Pack) LoadPreFormatter() (*PreFormatter, error) {
	return load(pack, "preformatter.json", parsePreFormatter)
}

func parsePreFormatter(data []byte) (*PreFormatter, error) {
	var description preFormatterDescription
	if err := json.Unmarshal(data, &description); err != nil {
		return nil, err
	}
	if description.Spreadsheet == nil {
		return nil, fmt.Errorf("spreadsheet is missing")
	}
	if description.Address == nil {
		return nil, fmt.Errorf("address is missing")
	}
	words := description.Spreadsheet
	preFormatter := &PreFormatter{}
	spreadsheet := &preFormatter.Spreadsheet
	lists := []struct {
		name   string
		words  []string
		target []string
	}{
		{"months", words.Months, spreadsheet.Months[:]},
		{"denominators", words.Denominators, spreadsheet.Denominators[:]},
		{"teens", words.Teens, spreadsheet.Teens[:]},
		{"tens", words.Tens, spreadsheet.Tens[:]},
		{"ordinals", words.Ordinals, spreadsheet.Ordinals[:]},
		{"otherUnits", words.OtherUnits, spreadsheet.OtherUnits[:]},
	}
	for _, list := range lists {
		if len(list.words) != len(list.target) {
			return nil, fmt.Errorf("spreadsheet %s has %d words, expected %d", list.name, len(list.words), len(list.target))
		}
		for index, word := range list.words {
			if !printableWord(word) {
				return nil, fmt.Errorf("spreadsheet %s[%d] is %q, not a word of printable ASCII", list.name, index, word)
			}
		}
		copy(list.target, list.words)
	}
	singles := []struct {
		name   string
		word   string
		target *string
	}{
		{"minus", words.Minus, &spreadsheet.Minus},
		{"and", words.And, &spreadsheet.And},
		{"exponent", words.Exponent, &spreadsheet.Exponent},
	}
	for _, single := range singles {
		if !printableWord(single.word) {
			return nil, fmt.Errorf("spreadsheet %s is %q, not a word of printable ASCII", single.name, single.word)
		}
		*single.target = single.word
	}
	address, err := parseAddress(description.Address)
	if err != nil {
		return nil, err
	}
	preFormatter.Address = *address
	return preFormatter, nil
}

func parseAddress(description *addressDescription) (*Address, error) {
	lists := []struct {
		name  string
		words []string
	}{
		{"functionWords", description.FunctionWords},
		{"abbreviations", description.Abbreviations},
		{"spacedAbbreviations", description.SpacedAbbreviations},
	}
	for _, list := range lists {
		if len(list.words) == 0 {
			return nil, fmt.Errorf("address %s is empty", list.name)
		}
		for index, word := range list.words {
			if !lowerWord(word) {
				return nil, fmt.Errorf("address %s[%d] is %q, not a word of lower case letters", list.name, index, word)
			}
		}
	}
	abbreviations := map[string]bool{}
	for _, word := range description.Abbreviations {
		abbreviations[word] = true
	}
	for _, word := range description.SpacedAbbreviations {
		if !abbreviations[word] {
			return nil, fmt.Errorf("address spaced abbreviation %q isn't an abbreviation", word)
		}
	}
	return &Address{FunctionWords: description.FunctionWords, Abbreviations: description.Abbreviations,
		SpacedAbbreviations: description.SpacedAbbreviations}, nil
}

func lowerWord(word string) bool {
	if word == "" {
		return false
	}
	for index := 0; index < len(word); index++ {
		if word[index] < 'a' || word[index] > 'z' {
			return false
		}
	}
	return true
}

func printableWord(word string) bool {
	if word == "" {
		return false
	}
	for index := 0; index < len(word); index++ {
		if !printable(word[index]) {
			return false
		}
	}
	return true
}
