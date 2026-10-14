package datapack

import (
	"encoding/json"
	"fmt"
	"math"
)

// AffixLetters is the number of letters that have lists of affixes: '@' to 'Z'.
const AffixLetters = 'Z' - '@' + 1

// An Affix is a suffix or a prefix the lexical stage strips from a word: its Letters, in reading order, the
// Context the letters next to it must be in, whether the stem it leaves is looked up, the word class a suffix
// gives, its Stress, and the list of affixes tried after it. SilentE is 1 when the stem may have lost a silent E,
// 2 when it may have unless it ends as StemConditions.WithoutE says, else 0; KeepsI keeps a stem's final I.
type Affix struct {
	Letters string
	Context Context
	Lookup  bool
	Class   byte
	Stress  int8
	Next    *AffixList
	SilentE byte
	KeepsI  bool
}

// An AffixList is the affixes the lexical stage tries in turn.
type AffixList []*Affix

// StemConditions are the contexts of the end of a stem an affix left: SilentE and NoSilentE decide whether it
// lost a silent E, WithoutE that it had none before affixes of SilentE 2, and LostT that it lost a T.
type StemConditions struct {
	SilentE, NoSilentE, WithoutE, LostT Context
}

// A Context is a rule's condition on the nodes next to a place: its clauses, and the ends the clauses can pass
// over to clauses after them.
type Context []Clause

// A Clause is a test of the nodes next to a place, with the Elements it compares them with, or an end.
type Clause struct {
	End      bool
	Test     byte
	Elements []Element
}

// An Element is a symbol, or the symbols of a class when Symbol is 0: those with one of Bits in their class byte
// of Plane.
type Element struct {
	Symbol      byte
	Plane, Bits byte
}

type affixDescription struct {
	Letters string            `json:"letters"`
	If      []json.RawMessage `json:"if"`
	Lookup  bool              `json:"lookup"`
	Class   *int64            `json:"class"`
	Stress  *int64            `json:"stress"`
	Next    *int64            `json:"next"`
	SilentE *int64            `json:"silentE"`
	KeepsI  bool              `json:"keepsI"`
}

type stemConditionsDescription struct {
	SilentE   []json.RawMessage `json:"silentE"`
	NoSilentE []json.RawMessage `json:"noSilentE"`
	WithoutE  []json.RawMessage `json:"withoutE"`
	LostT     []json.RawMessage `json:"lostT"`
}

type clauseDescription struct {
	Test  *int64            `json:"test"`
	Match []json.RawMessage `json:"match"`
}

// parseAffixes reads the affixes, their lists, the lists of each letter and the stem conditions into lexical.
func parseAffixes(description *lexicalDescription, lexical *Lexical) error {
	affixes := make([]Affix, len(description.Affixes))
	lists := make([]AffixList, len(description.AffixLists))
	listAt := func(index int64, name string, none bool) (*AffixList, error) {
		if none && index == -1 {
			return nil, nil
		}
		if index < 0 || index >= int64(len(lists)) {
			return nil, fmt.Errorf("%s is %d, not a list of affixes", name, index)
		}
		return &lists[index], nil
	}
	for index, members := range description.AffixLists {
		for position, member := range members {
			if member < 0 || member >= int64(len(affixes)) {
				return fmt.Errorf("affixLists[%d][%d] is %d, not an affix", index, position, member)
			}
			lists[index] = append(lists[index], &affixes[member])
		}
	}
	for index, affix := range description.Affixes {
		name := fmt.Sprintf("affixes[%d]", index)
		parsed := &affixes[index]
		parsed.Letters, parsed.Lookup, parsed.KeepsI = affix.Letters, affix.Lookup, affix.KeepsI
		if parsed.Letters == "" {
			return fmt.Errorf("%s.letters is empty", name)
		}
		for position := range len(parsed.Letters) {
			if letter := parsed.Letters[position]; letter < '@' || letter > 'Z' {
				return fmt.Errorf("%s.letters[%d] is %d, expected a letter from @ to Z", name, position, letter)
			}
		}
		var err error
		if parsed.Context, err = parseContext(affix.If, name+".if"); err != nil {
			return err
		}
		var class, stress, silentE int32
		if err = copyValue(&class, affix.Class, name+".class", 0, math.MaxUint8); err != nil {
			return err
		}
		if err = copyValue(&stress, affix.Stress, name+".stress", math.MinInt8, math.MaxInt8); err != nil {
			return err
		}
		if affix.SilentE != nil {
			if err = copyValue(&silentE, affix.SilentE, name+".silentE", 0, 2); err != nil {
				return err
			}
		}
		parsed.Class, parsed.Stress, parsed.SilentE = byte(class), int8(stress), byte(silentE)
		if affix.Next == nil {
			return fmt.Errorf("%s.next is missing", name)
		}
		if parsed.Next, err = listAt(*affix.Next, name+".next", false); err != nil {
			return err
		}
	}
	for _, letters := range []struct {
		name   string
		lists  []int64
		target *[AffixLetters]*AffixList
	}{{"suffixes", description.Suffixes, &lexical.Suffixes}, {"prefixes", description.Prefixes, &lexical.Prefixes}} {
		if len(letters.lists) != AffixLetters {
			return fmt.Errorf("%s has %d lists, expected %d", letters.name, len(letters.lists), AffixLetters)
		}
		for letter, index := range letters.lists {
			list, err := listAt(index, fmt.Sprintf("%s[%d]", letters.name, letter), true)
			if err != nil {
				return err
			}
			letters.target[letter] = list
		}
	}
	lexical.Affixes = affixes
	stems := &description.StemConditions
	for _, stem := range []struct {
		name        string
		description []json.RawMessage
		target      *Context
	}{{"silentE", stems.SilentE, &lexical.StemConditions.SilentE}, {"noSilentE", stems.NoSilentE, &lexical.StemConditions.NoSilentE},
		{"withoutE", stems.WithoutE, &lexical.StemConditions.WithoutE}, {"lostT", stems.LostT, &lexical.StemConditions.LostT}} {
		if stem.description == nil {
			return fmt.Errorf("stemConditions.%s is missing", stem.name)
		}
		var err error
		if *stem.target, err = parseContext(stem.description, "stemConditions."+stem.name); err != nil {
			return err
		}
	}
	return nil
}

// parseContext reads a context's items: clauses, and "end".
func parseContext(items []json.RawMessage, name string) (Context, error) {
	context := Context{}
	for index, item := range items {
		itemName := fmt.Sprintf("%s[%d]", name, index)
		var end string
		if json.Unmarshal(item, &end) == nil {
			if end != "end" {
				return nil, fmt.Errorf("%s is %q, expected a clause or \"end\"", itemName, end)
			}
			context = append(context, Clause{End: true})
			continue
		}
		var description clauseDescription
		if err := json.Unmarshal(item, &description); err != nil {
			return nil, fmt.Errorf("%s: %v", itemName, err)
		}
		var test int32
		if err := copyValue(&test, description.Test, itemName+".test", 1, math.MaxUint8); err != nil {
			return nil, err
		}
		clause := Clause{Test: byte(test)}
		if kind := clause.Test & 0xe0; kind != 0 && kind != 0xe0 {
			if len(description.Match) > 0 {
				return nil, fmt.Errorf("%s.match isn't empty, but test %d compares no elements", itemName, test)
			}
		} else if nodes := int(test&3) + 2; test&0x10 != 0 && len(description.Match) < nodes-1 {
			return nil, fmt.Errorf("%s.match has %d elements, but test %d compares %d nodes", itemName, len(description.Match), test, nodes)
		}
		for position, element := range description.Match {
			parsed, err := parseElement(element, fmt.Sprintf("%s.match[%d]", itemName, position))
			if err != nil {
				return nil, err
			}
			clause.Elements = append(clause.Elements, parsed)
		}
		context = append(context, clause)
	}
	return context, nil
}

// parseElement reads an element: a symbol, as a string of one, or a class, as [plane, bits].
func parseElement(element json.RawMessage, name string) (Element, error) {
	var symbol string
	if json.Unmarshal(element, &symbol) == nil {
		if len(symbol) != 1 || symbol[0] < ' ' || symbol[0] >= SymbolCount-1 {
			return Element{}, fmt.Errorf("%s is %q, expected a symbol from 32 to %d", name, symbol, SymbolCount-2)
		}
		return Element{Symbol: symbol[0]}, nil
	}
	var class []int64
	if err := json.Unmarshal(element, &class); err != nil || len(class) != 2 {
		return Element{}, fmt.Errorf("%s isn't a symbol or [plane, bits]", name)
	}
	if class[0] < 0 || class[0] >= ClassPlanes || class[1] < 1 || class[1] > math.MaxUint8 {
		return Element{}, fmt.Errorf("%s is %v, expected a plane from 0 to %d and bits from 1 to 255", name, class, ClassPlanes-1)
	}
	return Element{Plane: byte(class[0]), Bits: byte(class[1])}, nil
}
