// Package pronunciation holds module C's procedures written in code: the dictionary lookups, morphology
// and the tier copies (pronunciation.md, rules.md §9).
package pronunciation

import (
	"fmt"

	"github.com/phyce/OpenSAPI4/whistler/datapack"
	"github.com/phyce/OpenSAPI4/whistler/rules"
)

const (
	homographs  = 53
	verbs       = 54
	longestWord = 31
)

var dictionaryProcedures = []int{21, 22, 25, 26, 27, 34, 35, 36, homographs, verbs}

// Natives are module C's natives with the two flags morphology reads.
type Natives struct {
	dictionaries   map[int]*datapack.Dictionary
	homographFound bool
	verbFound      bool
}

func NewNatives(dictionaries *datapack.Dictionaries) (*Natives, error) {
	for _, procedure := range dictionaryProcedures {
		if dictionaries.Dictionaries[procedure] == nil {
			return nil, fmt.Errorf("the data pack has no dictionary %d", procedure)
		}
	}
	return &Natives{dictionaries: dictionaries.Dictionaries}, nil
}

// Procedures returns the natives by procedure number.
func (natives *Natives) Procedures() map[int]rules.Native {
	procedures := map[int]rules.Native{
		3:  numberSyllables,
		4:  clearTier4,
		23: restoreSpelling,
		30: natives.morphology,
		31: copyTier1,
		41: func(*rules.Machine, int, *int) bool { return false },
		42: expandOptionalSuffix,
		52: copyTier4,
	}
	for _, procedure := range dictionaryProcedures {
		procedures[procedure] = natives.lookup(procedure)
	}
	return procedures
}

func (natives *Natives) lookup(procedure int) rules.Native {
	dictionary := natives.dictionaries[procedure]
	return func(machine *rules.Machine, start int, end *int) bool {
		found := Lookup(dictionary, machine.State, start, end)
		switch procedure {
		case homographs:
			natives.homographFound = found
		case verbs:
			natives.verbFound = found
		}
		return found
	}
}

// Lookup looks tier 1 over start..*end up in the dictionary. On a match it writes the entry's fields from
// start into their tiers, pads tiers 1 to 4 with '~' up to the new end and moves *end there
// (pronunciation.md §3.2).
func Lookup(dictionary *datapack.Dictionary, state *rules.State, start int, end *int) bool {
	if start < 1 || *end < start || *end-start+1 > longestWord {
		return false
	}
	entry, found := dictionary.Words[string(state.Tier(1)[start:*end+1])]
	if !found {
		return false
	}
	last := [5]int{*end, *end, *end, *end, *end}
	position := 0
	for index, field := range dictionary.Fields {
		tier := state.Tier(field.Tier)
		cell := start
		final := index == len(dictionary.Fields)-1
		for position < len(entry) && (final || entry[position] != field.Separator) {
			tier[cell] = entry[position]
			cell++
			position++
		}
		position++
		last[field.Tier] = cell - 1
	}
	newEnd := start
	for tier := 1; tier <= 4; tier++ {
		newEnd = max(newEnd, last[tier])
	}
	for number := 1; number <= 4; number++ {
		tier := state.Tier(number)
		for cell := last[number] + 1; cell <= newEnd; cell++ {
			tier[cell] = rules.Empty
		}
	}
	*end = newEnd
	return true
}

// morphology is procedure 30: suffixes are split off (group 56) until a part is a homograph or a verb
// (group 57), then group 58 finishes the word, or group 59 restores it (pronunciation.md §4).
func (natives *Natives) morphology(machine *rules.Machine, start int, end *int) bool {
	natives.homographFound, natives.verbFound = false, false
	machine.ApplyGroup(57, start, end)
	found := natives.homographFound || natives.verbFound
	for !found {
		fired := machine.ApplyGroup(56, start, end)
		if fired {
			machine.ApplyGroup(57, start, end)
		}
		found = natives.homographFound || natives.verbFound
		if !fired {
			break
		}
	}
	if found {
		machine.ApplyGroup(58, start, end)
	} else {
		machine.ApplyGroup(59, start, end)
	}
	return found
}

// numberSyllables is procedure 3: the k-th '-' from the end of tier 1 gets the digit k in tier 4 (k ≤ 9).
func numberSyllables(machine *rules.Machine, start int, end *int) bool {
	tier1, tier4 := machine.State.Tier(1), machine.State.Tier(4)
	count := 1
	for cell := *end; cell >= start; cell-- {
		if tier1[cell] == '-' {
			if count < 10 {
				tier4[cell] = byte('0' + count)
			}
			count++
		}
	}
	return true
}

func clearTier4(machine *rules.Machine, start int, end *int) bool {
	tier4 := machine.State.Tier(4)
	for cell := start; cell <= *end; cell++ {
		tier4[cell] = ' '
	}
	return true
}

func restoreSpelling(machine *rules.Machine, start int, end *int) bool {
	tier1, tier2 := machine.State.Tier(1), machine.State.Tier(2)
	for cell := start; cell <= *end; cell++ {
		tier1[cell] = tier2[cell]
	}
	return true
}

func copyTier1(machine *rules.Machine, start int, end *int) bool {
	state := machine.State
	for cell := start; cell <= *end; cell++ {
		value := state.Tier(1)[cell]
		state.Tier(2)[cell], state.Tier(3)[cell], state.Tier(4)[cell] = value, value, value
	}
	return true
}

func copyTier4(machine *rules.Machine, start int, end *int) bool {
	tier1, tier4 := machine.State.Tier(1), machine.State.Tier(4)
	for cell := start; cell <= *end; cell++ {
		tier1[cell] = tier4[cell]
	}
	return true
}

// expandOptionalSuffix is procedure 42: "stem(suffix)rest" becomes "stemrest or stemsuffixrest", built in
// tier 2 and copied to tier 1. The rules call it only on a span holding "(s)" or "(es)".
func expandOptionalSuffix(machine *rules.Machine, start int, end *int) bool {
	tier1, tier2 := machine.State.Tier(1), machine.State.Tier(2)
	if start+2*(*end-start+1)+4 >= rules.Cells {
		return false
	}
	open := -1
	for cell := start; cell <= *end; cell++ {
		if tier1[cell] == '(' {
			open = cell
			break
		}
	}
	if open < 0 {
		return false
	}
	close := -1
	for cell := open; cell <= *end; cell++ {
		if tier1[cell] == ')' {
			close = cell
			break
		}
	}
	if close < 0 {
		return false
	}
	write := start
	put := func(value byte) {
		tier2[write] = value
		write++
	}
	stem := append([]byte(nil), tier1[start:open]...)
	suffix := append([]byte(nil), tier1[open+1:close]...)
	rest := append([]byte(nil), tier1[close+1:*end+1]...)
	for _, part := range [][]byte{stem, rest, []byte(" or "), stem, suffix, rest} {
		for _, value := range part {
			put(value)
		}
	}
	for cell := start; cell < write; cell++ {
		tier1[cell] = tier2[cell]
	}
	*end = write - 1
	return true
}
