package datapack

import (
	"encoding/json"
	"fmt"
	"math"
)

// GridCount is the number of duration grids and GridSize the bytes of the smallest. BreakReductionCount is the
// number of break reductions and WordClassCount the number of word classes with function word rules.
const (
	RateCount           = 26
	SlowRateCount       = 9
	GridCount           = 38
	GridSize            = 160
	BreakReductionCount = 47
	WordClassCount      = 16
)

// Prosody is the prosody stage's tables: each voice's pitch range, a Q15 share of the pitch's excursion, a pause's
// percent of its length at each rate, at the slow rates the patterns of the pauses between words, each symbol's
// durations and a duration's percent at each rate. DurationGrids are the bytes of the class durations' grids,
// each at its offset in them; BreakReductions the frames a vowel after a glide loses before a break, and
// FunctionWordRules the rules for a function word's vowel, by the word's class.
type Prosody struct {
	PitchRanges       [VoiceCount]int32
	PausePercents     [RateCount]int32
	SlowPausePatterns [SlowRateCount]int32
	Durations         [SymbolCount]Duration
	DurationPercents  [RateCount]int32
	DurationGrids     []byte
	GridOffsets       [GridCount]int32
	BreakReductions   [BreakReductionCount]int32
	FunctionWordRules [WordClassCount][]FunctionWordRule
}

// A Duration is a symbol's inherent and shortest durations in frames.
type Duration struct {
	Inherent, Shortest int32
}

// A FunctionWordRule takes Frames from a function word's vowel when the nodes from Back before the vowel spell
// Symbols, the vowel meets the Conditions, a bit each, and every test passes.
type FunctionWordRule struct {
	Back       int32
	Symbols    string
	Conditions int32
	Tests      []FunctionWordTest
	Frames     int32
}

// A FunctionWordTest tests a node, or the sound after the vowel, by its Kind. Direction 1 takes the node
// Steps phonemes after the vowel, and 0 keeps the node before.
type FunctionWordTest struct {
	Kind, Direction, Steps, Value int32
}

// The kinds of test, of the test's node or of the sound after the vowel.
const (
	NodeInClass = iota
	NodeInWordClass
	NodeNotInWordClass
	SoundInClass
	SoundNotInClass
	SoundNotSymbol
)

type prosodyDescription struct {
	PitchRanges       []int64                         `json:"pitchRanges"`
	PausePercents     []int64                         `json:"pausePercents"`
	SlowPausePatterns []int64                         `json:"slowPausePatterns"`
	Durations         [][]int64                       `json:"durations"`
	DurationPercents  []int64                         `json:"durationPercents"`
	DurationGrids     []int64                         `json:"durationGrids"`
	GridOffsets       []int64                         `json:"gridOffsets"`
	BreakReductions   []int64                         `json:"breakReductions"`
	FunctionWordRules [][]functionWordRuleDescription `json:"functionWordRules"`
}

type functionWordRuleDescription struct {
	Back       *int64                        `json:"back"`
	Symbols    string                        `json:"symbols"`
	Conditions *int64                        `json:"conditions"`
	Tests      []functionWordTestDescription `json:"tests"`
	Frames     *int64                        `json:"frames"`
}

type functionWordTestDescription struct {
	Kind      *int64 `json:"kind"`
	Direction *int64 `json:"direction"`
	Steps     *int64 `json:"steps"`
	Value     *int64 `json:"value"`
}

func (pack *Pack) LoadProsody() (*Prosody, error) {
	return load(pack, "prosody.json", parseProsody)
}

func parseProsody(data []byte) (*Prosody, error) {
	var description prosodyDescription
	if err := json.Unmarshal(data, &description); err != nil {
		return nil, err
	}
	prosody := &Prosody{}
	if err := copyValues(prosody.PitchRanges[:], description.PitchRanges, "pitchRanges", 0, math.MaxInt16); err != nil {
		return nil, err
	}
	if err := copyValues(prosody.PausePercents[:], description.PausePercents, "pausePercents", 0, 10000); err != nil {
		return nil, err
	}
	if err := copyValues(prosody.SlowPausePatterns[:], description.SlowPausePatterns, "slowPausePatterns", 0, math.MaxUint16); err != nil {
		return nil, err
	}
	if len(description.Durations) != SymbolCount {
		return nil, fmt.Errorf("durations has %d values, expected %d", len(description.Durations), SymbolCount)
	}
	for symbol, values := range description.Durations {
		var pair [2]int32
		if err := copyValues(pair[:], values, fmt.Sprintf("durations[%d]", symbol), 0, math.MaxUint8); err != nil {
			return nil, err
		}
		prosody.Durations[symbol] = Duration{pair[0], pair[1]}
	}
	if err := copyValues(prosody.DurationPercents[:], description.DurationPercents, "durationPercents", 0, 1000); err != nil {
		return nil, err
	}
	if len(description.DurationGrids) < GridSize {
		return nil, fmt.Errorf("durationGrids has %d values, expected at least %d", len(description.DurationGrids), GridSize)
	}
	prosody.DurationGrids = make([]byte, len(description.DurationGrids))
	if err := copyValues(prosody.DurationGrids, description.DurationGrids, "durationGrids", 0, math.MaxUint8); err != nil {
		return nil, err
	}
	if err := copyValues(prosody.GridOffsets[:], description.GridOffsets, "gridOffsets", 0, int64(len(prosody.DurationGrids)-GridSize)); err != nil {
		return nil, err
	}
	if err := copyValues(prosody.BreakReductions[:], description.BreakReductions, "breakReductions", 0, math.MaxUint8); err != nil {
		return nil, err
	}
	if len(description.FunctionWordRules) != WordClassCount {
		return nil, fmt.Errorf("functionWordRules has %d classes, expected %d", len(description.FunctionWordRules), WordClassCount)
	}
	for wordClass, rules := range description.FunctionWordRules {
		for index, rule := range rules {
			parsed, err := parseFunctionWordRule(rule, fmt.Sprintf("functionWordRules[%d][%d]", wordClass, index))
			if err != nil {
				return nil, err
			}
			prosody.FunctionWordRules[wordClass] = append(prosody.FunctionWordRules[wordClass], parsed)
		}
	}
	return prosody, nil
}

func parseFunctionWordRule(description functionWordRuleDescription, name string) (FunctionWordRule, error) {
	rule := FunctionWordRule{Symbols: description.Symbols}
	if err := copyValue(&rule.Back, description.Back, name+".back", 0, math.MaxUint8); err != nil {
		return rule, err
	}
	if len(rule.Symbols) == 0 || len(rule.Symbols) > math.MaxUint8 {
		return rule, fmt.Errorf("%s.symbols has %d symbols, expected 1 to %d", name, len(rule.Symbols), math.MaxUint8)
	}
	for index := range len(rule.Symbols) {
		if rule.Symbols[index] >= SymbolCount {
			return rule, fmt.Errorf("%s.symbols[%d] is %d, expected a symbol below %d", name, index, rule.Symbols[index], SymbolCount)
		}
	}
	if err := copyValue(&rule.Conditions, description.Conditions, name+".conditions", 0, math.MaxInt8); err != nil {
		return rule, err
	}
	if err := copyValue(&rule.Frames, description.Frames, name+".frames", 0, math.MaxUint8); err != nil {
		return rule, err
	}
	if len(description.Tests) > math.MaxUint8 {
		return rule, fmt.Errorf("%s.tests has %d tests, expected at most %d", name, len(description.Tests), math.MaxUint8)
	}
	for index, test := range description.Tests {
		testName := fmt.Sprintf("%s.tests[%d]", name, index)
		var parsed FunctionWordTest
		if err := copyValue(&parsed.Kind, test.Kind, testName+".kind", NodeInClass, SoundNotSymbol); err != nil {
			return rule, err
		}
		if err := copyValue(&parsed.Direction, test.Direction, testName+".direction", 0, 1); err != nil {
			return rule, err
		}
		if err := copyValue(&parsed.Steps, test.Steps, testName+".steps", 0, int64(parsed.Direction)*math.MaxUint8); err != nil {
			return rule, err
		}
		highest := int64(math.MaxUint8)
		switch parsed.Kind {
		case NodeInClass, SoundInClass, SoundNotInClass:
			highest = ClassPlanes<<8 - 1
		case SoundNotSymbol:
			highest = SymbolCount - 1
		}
		if err := copyValue(&parsed.Value, test.Value, testName+".value", 0, highest); err != nil {
			return rule, err
		}
		rule.Tests = append(rule.Tests, parsed)
	}
	return rule, nil
}
