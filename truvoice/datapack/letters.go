package datapack

import (
	"encoding/json"
	"fmt"
	"math"
)

// LetterRuleLetters is the number of letters that have letter-to-sound rules: '@' to '['.
const LetterRuleLetters = '[' - '@' + 1

// A LetterRule converts a letter, with the letters Before it, to Phonemes, both in the word's order, when its
// Masks match what follows the letter and the context If holds before the letters. JoinsPrefix joins a prefix to
// the stem, Weight is what the phonemes weigh towards the stress, 0 for nothing, and Flags tell the rules of the
// letters before what follows them (lexical.md §10).
type LetterRule struct {
	Before, Phonemes string
	JoinsPrefix      bool
	Weight           int8
	If               Context
	Masks, Flags     [2]uint32
}

type letterRuleDescription struct {
	Before      *string           `json:"before"`
	Phonemes    *string           `json:"phonemes"`
	JoinsPrefix bool              `json:"joinsPrefix"`
	Weight      int64             `json:"weight"`
	If          []json.RawMessage `json:"if"`
	Masks       []int64           `json:"masks"`
	Flags       []int64           `json:"flags"`
}

// alwaysHolds reports whether the rule holds whatever the letters: the rule a letter's list must end with.
func (rule *LetterRule) alwaysHolds() bool {
	return rule.Before == "" && rule.Masks == [2]uint32{} && (len(rule.If) == 0 || rule.If[0].End)
}

// parseLetterRules reads the rules of each letter into lexical.
func parseLetterRules(descriptions [][]letterRuleDescription, lexical *Lexical) error {
	if len(descriptions) != LetterRuleLetters {
		return fmt.Errorf("letterRules has %d lists, expected %d", len(descriptions), LetterRuleLetters)
	}
	for letter, rules := range descriptions {
		name := fmt.Sprintf("letterRules[%d]", letter)
		parsed := make([]LetterRule, len(rules))
		for index, rule := range rules {
			if err := parseLetterRule(rule, fmt.Sprintf("%s[%d]", name, index), &parsed[index]); err != nil {
				return err
			}
		}
		if len(parsed) == 0 || !parsed[len(parsed)-1].alwaysHolds() {
			return fmt.Errorf("%s doesn't end with a rule that always holds", name)
		}
		lexical.LetterRules[letter] = parsed
	}
	return nil
}

func parseLetterRule(description letterRuleDescription, name string, rule *LetterRule) error {
	if description.Before == nil {
		return fmt.Errorf("%s.before is missing", name)
	}
	if description.Phonemes == nil {
		return fmt.Errorf("%s.phonemes is missing", name)
	}
	rule.Before, rule.Phonemes, rule.JoinsPrefix = *description.Before, *description.Phonemes, description.JoinsPrefix
	for index := range len(rule.Before) {
		if rule.Before[index] == 0 || rule.Before[index] >= SymbolCount {
			return fmt.Errorf("%s.before[%d] is %d, expected a symbol from 1 to %d", name, index, rule.Before[index], SymbolCount-1)
		}
	}
	for index := range len(rule.Phonemes) {
		if rule.Phonemes[index] < ' ' || rule.Phonemes[index] >= SymbolCount-1 {
			return fmt.Errorf("%s.phonemes[%d] is %d, expected a symbol from 32 to 126", name, index, rule.Phonemes[index])
		}
	}
	if description.Weight < math.MinInt8 || description.Weight > 15 {
		return fmt.Errorf("%s.weight is %d, outside %d to 15", name, description.Weight, math.MinInt8)
	}
	rule.Weight = int8(description.Weight)
	if rule.JoinsPrefix && rule.Weight == 0 && rule.Phonemes == "" {
		return fmt.Errorf("%s joins the prefix but has no weight nor phonemes", name)
	}
	if description.If == nil {
		return fmt.Errorf("%s.if is missing", name)
	}
	var err error
	if rule.If, err = parseContext(description.If, name+".if"); err != nil {
		return err
	}
	if err := copyValues(rule.Masks[:], description.Masks, name+".masks", 0, math.MaxUint32); err != nil {
		return err
	}
	return copyValues(rule.Flags[:], description.Flags, name+".flags", 0, math.MaxUint32)
}
