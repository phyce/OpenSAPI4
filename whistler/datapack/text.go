package datapack

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf16"
)

const (
	characterNameLength = 29
	phoneticSymbols     = 53
	phoneSetIPALength   = 10
	phoneSetNameLength  = 10
)

type TextContext struct {
	Name  string
	Value int
}

type PartOfSpeech struct {
	Value  string
	Escape string
}

// Phoneme is one phonetic symbol. EngineName is empty for a symbol that only the IPA conversion uses.
// ARPAbetName is what the lexicon stores for it (pronunciation.md §6.4): empty for a symbol that isn't a
// phone, the digit for a stress mark.
type Phoneme struct {
	EngineName     string
	WideName       string
	InternalSymbol string
	IPA            uint16
	ARPAbetName    string
}

// PhoneSetEntry is an IPA string of the phone set the engine gives the phoneme converter, and its phone name
// (engine.md §13.4).
type PhoneSetEntry struct {
	IPA  []uint16
	Name string
}

// Text holds the preprocessing tables (text.md §11) and the phone set of prosody transplants. The word lists are sets: the converter and the
// reader check that each is strictly ascending in signed byte order, so the original's binary search
// finds exactly their words.
type Text struct {
	SentenceClasses [256]byte
	CharacterNames  [256]string
	AccentFolding   [256]byte
	Titles          map[string]bool
	Abbreviations   map[string]bool
	Starters        map[string]bool
	NumberWords     map[string]bool
	CharacterStyles []string
	Contexts        []TextContext
	PartsOfSpeech   []PartOfSpeech
	Phonemes        []Phoneme
	BestFit         map[uint16]byte
	Upper           [256]bool
	Lower           [256]bool
	Alphabetic      [256]bool
	Alphanumeric    [256]bool
	ToLower         [256]byte
	ToUpper         [256]byte
	PhoneSet        []PhoneSetEntry
}

type textContextDescription struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

type partOfSpeechDescription struct {
	Value  string `json:"value"`
	Escape string `json:"escape"`
}

type phonemeDescription struct {
	EngineName     string  `json:"engineName"`
	WideName       string  `json:"wideName"`
	InternalSymbol string  `json:"internalSymbol"`
	IPA            int     `json:"ipa"`
	ARPAbetName    *string `json:"arpabetName"`
}

type textDescription struct {
	SentenceClasses []int                     `json:"sentenceClasses"`
	CharacterNames  []string                  `json:"characterNames"`
	AccentFolding   string                    `json:"accentFolding"`
	Titles          []string                  `json:"titles"`
	Abbreviations   []string                  `json:"abbreviations"`
	Starters        []string                  `json:"starters"`
	NumberWords     []string                  `json:"numberWords"`
	CharacterStyles []string                  `json:"characterStyles"`
	Contexts        []textContextDescription  `json:"contexts"`
	PartsOfSpeech   []partOfSpeechDescription `json:"partsOfSpeech"`
	Phonemes        []phonemeDescription      `json:"phonemes"`
	BestFit         [][2]json.RawMessage      `json:"bestFit"`
	Upper           string                    `json:"upper"`
	Lower           string                    `json:"lower"`
	Alphabetic      string                    `json:"alphabetic"`
	Alphanumeric    string                    `json:"alphanumeric"`
	ToLower         string                    `json:"toLower"`
	ToUpper         string                    `json:"toUpper"`
	PhoneSet        [][2]json.RawMessage      `json:"phoneSet"`
}

func (pack *Pack) LoadText() (*Text, error) {
	return load(pack, "text.json", parseText)
}

func parseText(data []byte) (*Text, error) {
	var description textDescription
	if err := json.Unmarshal(data, &description); err != nil {
		return nil, err
	}
	text := &Text{}
	if len(description.SentenceClasses) != 256 {
		return nil, fmt.Errorf("%d sentence character classes", len(description.SentenceClasses))
	}
	for character, class := range description.SentenceClasses {
		if class < 1 || class > 15 && class != 17 && class != 21 {
			return nil, fmt.Errorf("byte 0x%02x has sentence character class %d", character, class)
		}
		text.SentenceClasses[character] = byte(class)
	}
	if len(description.CharacterNames) != 256 {
		return nil, fmt.Errorf("%d character names", len(description.CharacterNames))
	}
	for character, nameText := range description.CharacterNames {
		name, valid := byteString(nameText)
		if !valid || len(name) > characterNameLength || name != "" && !printable(name) {
			return nil, fmt.Errorf("byte 0x%02x has the character name %q", character, nameText)
		}
		text.CharacterNames[character] = name
	}
	folding, valid := byteString(description.AccentFolding)
	if !valid || len(folding) != 0x80 {
		return nil, fmt.Errorf("accent folding of %d bytes", len(folding))
	}
	for index := range len(folding) {
		letter := folding[index]
		if letter != 0 && !isASCIILetter(letter) {
			return nil, fmt.Errorf("byte 0x%02x folds to 0x%02x", 0x80+index, letter)
		}
		text.AccentFolding[0x80+index] = letter
	}
	lists := []struct {
		name  string
		words []string
		set   *map[string]bool
	}{
		{"titles", description.Titles, &text.Titles},
		{"abbreviations", description.Abbreviations, &text.Abbreviations},
		{"starters", description.Starters, &text.Starters},
		{"number words", description.NumberWords, &text.NumberWords},
	}
	for _, list := range lists {
		set, err := wordSet(list.name, list.words)
		if err != nil {
			return nil, err
		}
		*list.set = set
	}
	for index, styleText := range description.CharacterStyles {
		style, valid := byteString(styleText)
		if !valid || !quoted(style) || index == 0 && style != `"Normal"` {
			return nil, fmt.Errorf("character style %d is %q", index, styleText)
		}
		text.CharacterStyles = append(text.CharacterStyles, style)
	}
	if len(text.CharacterStyles) == 0 {
		return nil, fmt.Errorf("no character styles")
	}
	for _, context := range description.Contexts {
		name, valid := byteString(context.Name)
		if !valid || !quoted(name) {
			return nil, fmt.Errorf("context %q", context.Name)
		}
		text.Contexts = append(text.Contexts, TextContext{Name: name, Value: context.Value})
	}
	for _, part := range description.PartsOfSpeech {
		value, valueValid := byteString(part.Value)
		escape, escapeValid := byteString(part.Escape)
		if !valueValid || !escapeValid || !quoted(value) || len(escape) != 4 || !strings.HasPrefix(escape, "\x1b@") || escape[3] != ' ' || !printable(escape[2:3]) {
			return nil, fmt.Errorf("part of speech %q: %q", part.Value, part.Escape)
		}
		text.PartsOfSpeech = append(text.PartsOfSpeech, PartOfSpeech{Value: value, Escape: escape})
	}
	if err := text.parsePhonemes(description.Phonemes); err != nil {
		return nil, err
	}
	if err := text.parseBestFit(description.BestFit); err != nil {
		return nil, err
	}
	if err := text.parseClassification(&description); err != nil {
		return nil, err
	}
	if err := text.parsePhoneSet(description.PhoneSet); err != nil {
		return nil, err
	}
	return text, nil
}

func isASCIILetter(character byte) bool {
	return character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z'
}

func quoted(text string) bool {
	return len(text) > 2 && text[0] == '"' && text[len(text)-1] == '"' && printable(text)
}

// wordSet checks that the words are strictly ascending in the order of the original's binary search
// (signed bytes, a prefix before the longer word), so that an exact match finds what the search finds.
func wordSet(name string, words []string) (map[string]bool, error) {
	set := make(map[string]bool, len(words))
	previous := ""
	for index, wordText := range words {
		word, valid := byteString(wordText)
		if !valid || word == "" || strings.IndexByte(word, 0) >= 0 || index > 0 && compareSigned(previous, word) >= 0 {
			return nil, fmt.Errorf("%s: word %d %q", name, index+1, wordText)
		}
		set[word] = true
		previous = word
	}
	return set, nil
}

func compareSigned(first string, second string) int {
	for index := 0; index < len(first) && index < len(second); index++ {
		if left, right := int8(first[index]), int8(second[index]); left != right {
			if left < right {
				return -1
			}
			return 1
		}
	}
	return len(first) - len(second)
}

func (text *Text) parsePhonemes(descriptions []phonemeDescription) error {
	if len(descriptions) != phoneticSymbols {
		return fmt.Errorf("%d phonetic symbols", len(descriptions))
	}
	for index, description := range descriptions {
		engineName, engineValid := byteString(description.EngineName)
		wideName, wideValid := byteString(description.WideName)
		symbol, symbolValid := byteString(description.InternalSymbol)
		searchable := index < phoneticSymbols-1
		valid := engineValid && wideValid && symbolValid && len(wideName) >= 1 && len(wideName) <= 2 && printable(wideName)
		if searchable {
			valid = valid && len(engineName) >= 1 && len(engineName) <= 2 && printable(engineName) && len(symbol) >= 1 && len(symbol) <= 2
		} else {
			valid = valid && engineName == "" && symbol == ""
		}
		for _, character := range []byte(symbol) {
			valid = valid && character >= 0x20 && character <= RuleLastSymbol
		}
		var arpabetName string
		if valid = valid && description.ARPAbetName != nil; valid {
			var arpabetValid bool
			arpabetName, arpabetValid = byteString(*description.ARPAbetName)
			valid = arpabetValid && len(arpabetName) <= 2 && (arpabetName == "" || printable(arpabetName) && !strings.Contains(arpabetName, " "))
		}
		ipa := description.IPA
		if !valid || ipa < 0 || ipa > 0xFFFF || utf16.IsSurrogate(rune(ipa)) {
			return fmt.Errorf("phonetic symbol %d: %+v", index+1, description)
		}
		text.Phonemes = append(text.Phonemes, Phoneme{EngineName: engineName, WideName: wideName, InternalSymbol: symbol, IPA: uint16(ipa),
			ARPAbetName: arpabetName})
	}
	return nil
}

func (text *Text) parseBestFit(pairs [][2]json.RawMessage) error {
	text.BestFit = make(map[uint16]byte, len(pairs))
	previous := 0
	for _, pair := range pairs {
		var code int
		var valueText string
		if json.Unmarshal(pair[0], &code) != nil || json.Unmarshal(pair[1], &valueText) != nil {
			return fmt.Errorf("best fit entry %s", pair)
		}
		value, valid := byteString(valueText)
		if !valid || len(value) != 1 || value == "?" || value[0] == 0 || code <= previous || code > 0xFFFF || utf16.IsSurrogate(rune(code)) {
			return fmt.Errorf("best fit of U+%04X is %q", code, valueText)
		}
		text.BestFit[uint16(code)] = value[0]
		previous = code
	}
	return nil
}

func (text *Text) parseClassification(description *textDescription) error {
	sets := []struct {
		name  string
		value string
		flags *[256]bool
	}{
		{"upper", description.Upper, &text.Upper},
		{"lower", description.Lower, &text.Lower},
		{"alphabetic", description.Alphabetic, &text.Alphabetic},
		{"alphanumeric", description.Alphanumeric, &text.Alphanumeric},
	}
	for _, set := range sets {
		members, valid := byteString(set.value)
		if !valid {
			return fmt.Errorf("%s characters %q", set.name, set.value)
		}
		for _, character := range []byte(members) {
			if set.flags[character] {
				return fmt.Errorf("%s characters list 0x%02x twice", set.name, character)
			}
			set.flags[character] = true
		}
	}
	mappings := []struct {
		name  string
		value string
		table *[256]byte
	}{
		{"lowercase", description.ToLower, &text.ToLower},
		{"uppercase", description.ToUpper, &text.ToUpper},
	}
	for _, mapping := range mappings {
		table, valid := byteString(mapping.value)
		if !valid || len(table) != 256 {
			return fmt.Errorf("%s mapping of %d bytes", mapping.name, len(table))
		}
		copy(mapping.table[:], table)
	}
	for character := range 256 {
		digit := character >= '0' && character <= '9'
		if text.Upper[character] && text.Lower[character] || text.Alphabetic[character] != (text.Upper[character] || text.Lower[character]) ||
			(text.Alphabetic[character] || digit) && !text.Alphanumeric[character] {
			return fmt.Errorf("byte 0x%02x is classified inconsistently", character)
		}
		if lowered := text.ToLower[character]; int(lowered) != character && !(text.Upper[character] && text.Lower[lowered]) {
			return fmt.Errorf("byte 0x%02x lowercases to 0x%02x", character, lowered)
		}
		if raised := text.ToUpper[character]; int(raised) != character && !(text.Lower[character] && text.Upper[raised]) {
			return fmt.Errorf("byte 0x%02x uppercases to 0x%02x", character, raised)
		}
	}
	return nil
}

// parsePhoneSet reads the phone set: IPA strings of 1 to 10 code units other than 0, each listed once, and
// names of 1 to 10 printable characters without spaces.
func (text *Text) parsePhoneSet(pairs [][2]json.RawMessage) error {
	seen := map[string]bool{}
	for _, pair := range pairs {
		var codes []int
		var nameText string
		if json.Unmarshal(pair[0], &codes) != nil || json.Unmarshal(pair[1], &nameText) != nil {
			return fmt.Errorf("phone set entry %s", pair)
		}
		name, valid := byteString(nameText)
		valid = valid && len(name) <= phoneSetNameLength && printable(name) && !strings.Contains(name, " ") && len(codes) >= 1 && len(codes) <= phoneSetIPALength
		ipa := make([]uint16, len(codes))
		for index, code := range codes {
			valid = valid && code > 0 && code <= 0xFFFF
			ipa[index] = uint16(code)
		}
		key := fmt.Sprint(ipa)
		if !valid || seen[key] {
			return fmt.Errorf("phone set entry %v %q", codes, nameText)
		}
		seen[key] = true
		text.PhoneSet = append(text.PhoneSet, PhoneSetEntry{IPA: ipa, Name: name})
	}
	return nil
}
