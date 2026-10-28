package datapack

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
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
// sentence, and the abbreviations, after which no sentence starts, with those whose dot keeps a word's spaces; the
// words written out and the number words, after which a dot before two spaces is a dot and a lone dot; the e-mail
// headers and the words written where a message starts; and the tables that write words in capitals.
type Address struct {
	FunctionWords       []string
	Abbreviations       []string
	SpacedAbbreviations []string
	Words               []Word
	NumberWords         []string
	Headers             []Header
	MessageStart        string
	Letters             Letters
}

// Letters is how the address context writes a word in capitals of two letters or more, letters A to Z: the words of two
// letters it says; the onsets, two consonants that start a word, and the codas, two letters that end one after the
// vowels they map to; the first and last vowel pairs, which start and end a word; the words it spells letter by letter,
// and those it spells only after a ' or ~ or two quotes; the words of three letters it never says and the words of
// three to five letters it always says whatever their letters; the spelled and said words that are so only in capitals,
// not in a word with small letters; and the words it writes otherwise, with what it writes. For words of four letters:
// the heads and tails, three letters said apart from a fourth before or after them; the heads that open a word with a
// soft start; the unkept heads, said only before the letters they map to; the clusters, two consonants that start a
// word, or follow the vowel that starts one, before the letters they map to; the vowel codas, two letters that end a
// word after a first pair, or the three letters after a first Q or X; and the coda pairs, two letters that end a word
// after one vowel. For words of five letters or more: the pieces, four letters or more said apart from the rest before
// or after them; the long heads and tails, three letters (two or five for a tail) said apart from the rest after or
// before them; and the unsplit words, which keep a piece, head or tail they start or end with from being said apart.
// And the readings, the words it reads as an abbreviation near a word in capitals, with what it reads.
type Letters struct {
	TwoLetters    []string
	Onsets        []string
	Codas         map[string]string
	FirstPairs    []string
	LastPairs     []string
	Spelled       []string
	MarkedSpelled []string
	Unkept, Kept  []string
	CapitalsOnly  []string
	Respelled     map[string]string
	Heads, Tails  []string
	OpenHeads     []string
	UnkeptHeads   map[string]string
	Clusters      map[string]string
	VowelCodas    []string
	CodaPairs     []string
	Pieces        []string
	LongHeads     []string
	LongTails     []string
	Unsplit       []string
	Readings      map[string]string
}

// Word is a word the address context writes out, in one case: its expansion and how the dot after it is spaced. A
// dotted word is written out only before a dot; an any-case word also with capitals after its first letter, the
// others small or not; an any-first-case word, in small letters, also so with a capital first letter.
type Word struct {
	Word, Expansion string
	Kind            WordKind
	Dotted          bool
	AnyCase         bool
	AnyFirstCase    bool
}

// WordKind is how the dot after a word written out is spaced.
type WordKind int

const (
	// AbbreviationWord's dot stays before a capital, and before two spaces or more.
	AbbreviationWord WordKind = iota
	// TitleWord's dot goes before one space.
	TitleWord
	// PlainWord's dot and spaces stay.
	PlainWord
	// CapitalWord is written as a plain word; its expansion is the word after a ~, and it is written out only when
	// no capitals word stands near it.
	CapitalWord
)

var wordKinds = map[string]WordKind{"abbreviation": AbbreviationWord, "title": TitleWord, "plain": PlainWord,
	"capital": CapitalWord}

// Header is an e-mail header's name and what the e-mail reading does with a line of it outside a header block, in
// one and right after one. A line of it is read as names once OutsideNames messages have started outside a block,
// InsideNames in or right after one; 0 is never. A value of it that ends in a comma goes on in the next line if
// Continues is set; a message it starts has no Author: line if ClearsAuthor is; and in or right after a header block
// two `>` may quote it if QuotedTwice is.
type Header struct {
	Name                                 string
	Outside, Block, After                HeaderAction
	OutsideNames, InsideNames            int
	Continues, ClearsAuthor, QuotedTwice bool
}

// HeaderAction is what the e-mail reading does with a header line.
type HeaderAction int

const (
	NoHeader HeaderAction = iota
	// DroppedHeader drops the line.
	DroppedHeader
	// BlockHeader drops the line and starts a header block.
	BlockHeader
	// MessageHeader writes the message start's words and starts a header block.
	MessageHeader
	// SenderHeader writes the name in the line's value and starts a header block.
	SenderHeader
	// KeptHeader writes the line.
	KeptHeader
	// DateHeader writes the line, never as a short line.
	DateHeader
	// SubjectHeader writes the line; in a header block, the line after it goes on.
	SubjectHeader
	// AuthorHeader writes the name in the line's value and starts a header block.
	AuthorHeader
	// NameHeader writes the name in the line's value.
	NameHeader
)

var headerActions = map[string]HeaderAction{"dropped": DroppedHeader, "block": BlockHeader, "message": MessageHeader,
	"sender": SenderHeader, "kept": KeptHeader, "date": DateHeader, "subject": SubjectHeader, "author": AuthorHeader}

type preFormatterDescription struct {
	Spreadsheet *spreadsheetDescription `json:"spreadsheet"`
	Address     *addressDescription     `json:"address"`
}

type addressDescription struct {
	FunctionWords       []string            `json:"functionWords"`
	Abbreviations       []string            `json:"abbreviations"`
	SpacedAbbreviations []string            `json:"spacedAbbreviations"`
	Words               []wordDescription   `json:"words"`
	NumberWords         []string            `json:"numberWords"`
	Headers             []headerDescription `json:"headers"`
	MessageStart        string              `json:"messageStart"`
	Letters             *lettersDescription `json:"letters"`
}

type lettersDescription struct {
	TwoLetters    []string          `json:"twoLetters"`
	Onsets        []string          `json:"onsets"`
	Codas         map[string]string `json:"codas"`
	FirstPairs    []string          `json:"firstPairs"`
	LastPairs     []string          `json:"lastPairs"`
	Spelled       []string          `json:"spelled"`
	MarkedSpelled []string          `json:"markedSpelled"`
	CapitalsOnly  []string          `json:"capitalsOnly"`
	Unkept        []string          `json:"unkept"`
	Kept          []string          `json:"kept"`
	Respelled     map[string]string `json:"respelled"`
	Heads         []string          `json:"heads"`
	Tails         []string          `json:"tails"`
	OpenHeads     []string          `json:"openHeads"`
	UnkeptHeads   map[string]string `json:"unkeptHeads"`
	Clusters      map[string]string `json:"clusters"`
	VowelCodas    []string          `json:"vowelCodas"`
	CodaPairs     []string          `json:"codaPairs"`
	Pieces        []string          `json:"pieces"`
	LongHeads     []string          `json:"longHeads"`
	LongTails     []string          `json:"longTails"`
	Unsplit       []string          `json:"unsplit"`
	Readings      map[string]string `json:"readings"`
}

type wordDescription struct {
	Word         string `json:"word"`
	Expansion    string `json:"expansion"`
	Kind         string `json:"kind"`
	Dotted       bool   `json:"dotted"`
	AnyCase      bool   `json:"anyCase"`
	AnyFirstCase bool   `json:"anyFirstCase"`
}

type headerDescription struct {
	Name         string `json:"name"`
	Outside      string `json:"outside"`
	Block        string `json:"block"`
	After        string `json:"after"`
	OutsideNames int    `json:"outsideNames"`
	InsideNames  int    `json:"insideNames"`
	Continues    bool   `json:"continues"`
	ClearsAuthor bool   `json:"clearsAuthor"`
	QuotedTwice  bool   `json:"quotedTwice"`
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
		{"numberWords", description.NumberWords},
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
	if !printableWord(description.MessageStart) {
		return nil, fmt.Errorf("address messageStart is %q, not a word of printable ASCII", description.MessageStart)
	}
	words, err := parseWords(description.Words)
	if err != nil {
		return nil, err
	}
	headers, err := parseHeaders(description.Headers)
	if err != nil {
		return nil, err
	}
	letters, err := parseLetters(description.Letters)
	if err != nil {
		return nil, err
	}
	return &Address{FunctionWords: description.FunctionWords, Abbreviations: description.Abbreviations,
		SpacedAbbreviations: description.SpacedAbbreviations, Words: words, NumberWords: description.NumberWords,
		Headers: headers, MessageStart: description.MessageStart, Letters: *letters}, nil
}

func parseLetters(description *lettersDescription) (*Letters, error) {
	if description == nil {
		return nil, fmt.Errorf("address letters is missing")
	}
	lists := []struct {
		name       string
		words      []string
		shortest   int
		longest    int
		vowelsOnly bool
	}{
		{"twoLetters", description.TwoLetters, 2, 2, false},
		{"onsets", description.Onsets, 2, 2, false},
		{"firstPairs", description.FirstPairs, 2, 2, true},
		{"lastPairs", description.LastPairs, 2, 2, true},
		{"spelled", description.Spelled, 2, 6, false},
		{"markedSpelled", description.MarkedSpelled, 2, 3, false},
		{"capitalsOnly", description.CapitalsOnly, 3, 6, false},
		{"unkept", description.Unkept, 3, 3, false},
		{"kept", description.Kept, 3, 5, false},
		{"heads", description.Heads, 3, 3, false},
		{"tails", description.Tails, 3, 3, false},
		{"openHeads", description.OpenHeads, 3, 3, false},
		{"vowelCodas", description.VowelCodas, 2, 2, false},
		{"codaPairs", description.CodaPairs, 2, 2, false},
		{"pieces", description.Pieces, 4, 6, false},
		{"longHeads", description.LongHeads, 3, 3, false},
		{"longTails", description.LongTails, 2, 5, false},
		{"unsplit", description.Unsplit, 4, 9, false},
	}
	for _, list := range lists {
		if len(list.words) == 0 {
			return nil, fmt.Errorf("address letters %s is empty", list.name)
		}
		seen := map[string]bool{}
		for index, word := range list.words {
			if len(word) < list.shortest || len(word) > list.longest || !capitalWord(word) ||
				list.vowelsOnly && strings.Trim(word, vowelLetters) != "" {
				return nil, fmt.Errorf("address letters %s[%d] is %q", list.name, index, word)
			}
			if seen[word] {
				return nil, fmt.Errorf("address letters %s has %q twice", list.name, word)
			}
			seen[word] = true
		}
	}
	for _, word := range description.Kept {
		if slices.Contains(description.Unkept, word) {
			return nil, fmt.Errorf("address letters %q is kept and unkept", word)
		}
	}
	for _, word := range description.CapitalsOnly {
		if !slices.Contains(description.Spelled, word) && !slices.Contains(description.Kept, word) {
			return nil, fmt.Errorf("address letters capitals-only %q is neither spelled nor kept", word)
		}
	}
	if len(description.Codas) == 0 {
		return nil, fmt.Errorf("address letters codas is empty")
	}
	for pair, after := range description.Codas {
		if len(pair) != 2 || !capitalWord(pair) || after == "" || strings.Trim(after, vowelLetters) != "" {
			return nil, fmt.Errorf("address letters coda %q after %q", pair, after)
		}
	}
	for _, table := range []struct {
		name   string
		words  map[string]string
		length int
	}{{"unkeptHeads", description.UnkeptHeads, 3}, {"clusters", description.Clusters, 2}} {
		if len(table.words) == 0 {
			return nil, fmt.Errorf("address letters %s is empty", table.name)
		}
		for word, after := range table.words {
			if len(word) != table.length || !capitalWord(word) || !capitalWord(after) || !distinctLetters(after) {
				return nil, fmt.Errorf("address letters %s %q before %q", table.name, word, after)
			}
		}
	}
	for _, table := range []struct {
		name    string
		words   map[string]string
		longest int
	}{{"respelled", description.Respelled, 6}, {"readings", description.Readings, 9}} {
		if len(table.words) == 0 {
			return nil, fmt.Errorf("address letters %s is empty", table.name)
		}
		for word, written := range table.words {
			if len(word) < 2 || len(word) > table.longest || !capitalWord(word) || !printableWord(written) {
				return nil, fmt.Errorf("address letters %s %q is %q", table.name, word, written)
			}
		}
	}
	return &Letters{TwoLetters: description.TwoLetters, Onsets: description.Onsets, Codas: description.Codas,
		FirstPairs: description.FirstPairs, LastPairs: description.LastPairs, Spelled: description.Spelled,
		MarkedSpelled: description.MarkedSpelled, Unkept: description.Unkept, Kept: description.Kept,
		CapitalsOnly: description.CapitalsOnly, Respelled: description.Respelled, Heads: description.Heads,
		Tails: description.Tails, OpenHeads: description.OpenHeads, UnkeptHeads: description.UnkeptHeads,
		Clusters: description.Clusters, VowelCodas: description.VowelCodas, CodaPairs: description.CodaPairs,
		Pieces: description.Pieces, LongHeads: description.LongHeads, LongTails: description.LongTails,
		Unsplit: description.Unsplit, Readings: description.Readings}, nil
}

func distinctLetters(word string) bool {
	for index := 1; index < len(word); index++ {
		if strings.IndexByte(word[:index], word[index]) >= 0 {
			return false
		}
	}
	return true
}

func parseWords(descriptions []wordDescription) ([]Word, error) {
	if len(descriptions) == 0 {
		return nil, fmt.Errorf("address words is empty")
	}
	words := make([]Word, 0, len(descriptions))
	seen := map[string]bool{}
	for index, description := range descriptions {
		if !letterWord(description.Word) {
			return nil, fmt.Errorf("address word %d is %q, not a word of letters and apostrophes", index,
				description.Word)
		}
		if seen[description.Word] {
			return nil, fmt.Errorf("address word %q comes twice", description.Word)
		}
		seen[description.Word] = true
		if !printableWord(description.Expansion) {
			return nil, fmt.Errorf("address word %q's expansion is %q, not printable ASCII", description.Word,
				description.Expansion)
		}
		kind, known := wordKinds[description.Kind]
		if !known {
			return nil, fmt.Errorf("address word %q's kind is %q, not a kind", description.Word, description.Kind)
		}
		if kind == CapitalWord && description.Expansion != "~"+description.Word {
			return nil, fmt.Errorf("address capital word %q's expansion is %q, not the word after a ~",
				description.Word, description.Expansion)
		}
		words = append(words, Word{Word: description.Word, Expansion: description.Expansion, Kind: kind,
			Dotted: description.Dotted, AnyCase: description.AnyCase, AnyFirstCase: description.AnyFirstCase})
	}
	return words, nil
}

func parseHeaders(descriptions []headerDescription) ([]Header, error) {
	if len(descriptions) == 0 {
		return nil, fmt.Errorf("address headers is empty")
	}
	headers := make([]Header, 0, len(descriptions))
	seen := map[string]bool{}
	for index, description := range descriptions {
		if !printableWord(description.Name) || strings.ContainsAny(description.Name, ": ") {
			return nil, fmt.Errorf("address header %d's name is %q, not a word of printable ASCII without a colon",
				index, description.Name)
		}
		if seen[description.Name] {
			return nil, fmt.Errorf("address header %q comes twice", description.Name)
		}
		seen[description.Name] = true
		header := Header{Name: description.Name, OutsideNames: description.OutsideNames,
			InsideNames: description.InsideNames, Continues: description.Continues,
			ClearsAuthor: description.ClearsAuthor, QuotedTwice: description.QuotedTwice}
		actions := []struct {
			state  string
			action string
			target *HeaderAction
		}{
			{"outside", description.Outside, &header.Outside},
			{"block", description.Block, &header.Block},
			{"after", description.After, &header.After},
		}
		for _, action := range actions {
			if action.action == "" {
				continue
			}
			value, known := headerActions[action.action]
			if !known {
				return nil, fmt.Errorf("address header %q's %s is %q, not an action", description.Name, action.state,
					action.action)
			}
			*action.target = value
		}
		if header.OutsideNames < 0 || header.InsideNames < 0 {
			return nil, fmt.Errorf("address header %q has a negative message count", description.Name)
		}
		if header.Outside == NoHeader && header.Block == NoHeader && header.After == NoHeader &&
			header.OutsideNames == 0 && header.InsideNames == 0 {
			return nil, fmt.Errorf("address header %q does nothing", description.Name)
		}
		headers = append(headers, header)
	}
	return headers, nil
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

const vowelLetters = "AEIOU"

func capitalWord(word string) bool {
	for index := 0; index < len(word); index++ {
		if word[index] < 'A' || word[index] > 'Z' {
			return false
		}
	}
	return word != ""
}

func letterWord(word string) bool {
	if word == "" {
		return false
	}
	for index := 0; index < len(word); index++ {
		character := word[index] | 0x20
		if (character < 'a' || character > 'z') && word[index] != '\'' {
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
