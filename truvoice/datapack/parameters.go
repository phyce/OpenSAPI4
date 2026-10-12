package datapack

import (
	"encoding/json"
	"fmt"
	"math"
	"math/bits"
)

const (
	// ShapeCount is the number of transition shapes.
	ShapeCount = 31
	// PhonemeCount is the number of phonemes the parameter generator knows, PauseIndex the pause's place among
	// them.
	PhonemeCount = 59
	PauseIndex   = 58
	// ClassPlanes is the number of class bytes of a symbol, SymbolCount the symbols that have them.
	ClassPlanes = 6
	SymbolCount = 128
	// CategoryCount is the number of phoneme categories, FlagCount the number of the rules' flags and
	// ActionCount the number of the rules' actions.
	CategoryCount = 4
	FlagCount     = 11
	ActionCount   = 16
)

// A Phoneme is one of the parameter generator's phonemes (parameters.md §3.1). Formants and Bandwidths are in
// Hz, Voicing is track 0's target, Amplitudes tracks 1 to 8's, NasalZero track 16's in Hz. Vowel is set for the
// vowels with an offglide, Burst for the stops with a burst.
type Phoneme struct {
	Symbol     byte
	Formants   [4]int32
	Bandwidths [3]int32
	Voicing    int32
	Amplitudes [8]int32
	NasalZero  int32
	Frontness  int32
	Place      int32
	Vowel      *Vowel
	Burst      *[2][4]int32
}

// Vowel is what a vowel has besides a phoneme's targets: its offglide's formants and bandwidths in Hz, the
// frontness it ends with and its transition.
type Vowel struct {
	Offglide     [7]int32
	EndFrontness int32
	Transition   int32
}

// VoiceTracks are a voice's targets of tracks 18 to 20 and the highest F4 target it allows.
type VoiceTracks struct {
	Tracks             [3]int32
	FourthFormantLimit int32
}

// The phonemes a rule's condition can test (parameters.md §5.2).
const (
	CurrentPhoneme = iota
	NextPhoneme
	AfterNextPhoneme
	BeforePreviousPhoneme
)

type ConditionKind int

const (
	FlagCondition ConditionKind = iota
	ClassCondition
	SymbolCondition
)

// A Condition is one test of a rule: a flag's value, whether a phoneme has a class bit (Bits of the class byte
// Plane), or whether it is one of Symbols. It passes when the test's result is Is.
type Condition struct {
	Kind    ConditionKind
	Flag    int
	Phoneme int
	Plane   int
	Bits    byte
	Symbols string
	Is      bool
}

type Operation int

const (
	Set Operation = iota
	Add
	Subtract
	Copy
)

// Field is a value of the parameter generator a rule can change (parameters.md §5.3). The fields before
// BurstLevelField have one per track, BurstLevelField one for each of tracks 0 to 2.
type Field int

const (
	TargetField Field = iota
	ModeField
	ForwardField
	BackField
	OnsetField
	BackTargetField
	FramesField
	NextField
	EarliestField
	NextTargetField
	LastField
	WeightField
	BurstLevelField
	PreviousCategoryField
	CategoryField
	PreviousFrontnessField
	FrontnessField
	AspirationShiftField
	VoiceOnsetField
	BurstFramesField
	ClosureFramesField
	FricativeAspirationField
	AspirationField
	VowelOffsetField
	VowelTransitionField
	fieldCount
)

var fieldNames = [fieldCount]string{"target", "mode", "forward", "back", "onset", "backTarget", "frames", "next",
	"earliest", "nextTarget", "last", "weight", "burstLevel", "previousCategory", "category", "previousFrontness",
	"frontness", "aspirationShift", "voiceOnset", "burstFrames", "closureFrames", "fricativeAspiration",
	"aspiration", "vowelOffset", "vowelTransition"}

// A Place is a field, and its track for the fields that have one.
type Place struct {
	Field Field
	Track int
}

// An Assignment sets, adds to or subtracts from a field Value, or copies another field, From, to it.
type Assignment struct {
	Operation Operation
	To        Place
	Value     int32
	From      Place
}

// A Rule is applied when all its conditions pass: its assignments, then its actions.
type Rule struct {
	Conditions  []Condition
	Assignments []Assignment
	Actions     []int
}

// Parameters are the parameter generator's tables. Shapes are the transition shapes: shape i has at most i
// weights, 1 to 255, the first 255. Classes are the symbols' class bytes. CategoryWeights are a weight for
// each category of a phoneme and of the one before it, DefaultShapes each track's forward shape. RuleMatrix is
// the rule set for each phoneme and the one before it.
type Parameters struct {
	Shapes          [ShapeCount][]byte
	Phonemes        [PhonemeCount]Phoneme
	Classes         [ClassPlanes][SymbolCount]byte
	Voices          [VoiceCount]VoiceTracks
	CategoryWeights [CategoryCount][CategoryCount]int32
	DefaultShapes   [TrackCount]int32
	RuleMatrix      [PhonemeCount][PhonemeCount]int
	RuleSets        [][]Rule
	indexes         [256]int
}

// Index returns the phoneme a symbol is, and whether it is one.
func (parameters *Parameters) Index(symbol byte) (int, bool) {
	index := parameters.indexes[symbol]
	return index, index >= 0
}

type phonemeDescription struct {
	Symbol       string    `json:"symbol"`
	Formants     []int64   `json:"formants"`
	Bandwidths   []int64   `json:"bandwidths"`
	Voicing      *int64    `json:"voicing"`
	Amplitudes   []int64   `json:"amplitudes"`
	NasalZero    *int64    `json:"nasalZero"`
	Frontness    *int64    `json:"frontness"`
	Place        *int64    `json:"place"`
	Offglide     []int64   `json:"offglide"`
	EndFrontness *int64    `json:"endFrontness"`
	Transition   *int64    `json:"transition"`
	Burst        [][]int64 `json:"burst"`
}

type voiceTracksDescription struct {
	Tracks             []int64 `json:"tracks"`
	FourthFormantLimit *int64  `json:"fourthFormantLimit"`
}

type conditionDescription struct {
	Flag    *int64  `json:"flag"`
	Phoneme *string `json:"phoneme"`
	Plane   *int64  `json:"plane"`
	Bits    *int64  `json:"bits"`
	In      *string `json:"in"`
	Is      *bool   `json:"is"`
}

type assignmentDescription struct {
	To       []json.RawMessage `json:"to"`
	Set      *int64            `json:"set"`
	Add      *int64            `json:"add"`
	Subtract *int64            `json:"subtract"`
	Copy     []json.RawMessage `json:"copy"`
}

type ruleDescription struct {
	If  []conditionDescription  `json:"if"`
	Set []assignmentDescription `json:"set"`
	Do  []int64                 `json:"do"`
}

type parametersDescription struct {
	Shapes          [][]int64                `json:"shapes"`
	Phonemes        []phonemeDescription     `json:"phonemes"`
	Classes         [][]int64                `json:"classes"`
	Voices          []voiceTracksDescription `json:"voices"`
	CategoryWeights [][]int64                `json:"categoryWeights"`
	DefaultShapes   []int64                  `json:"defaultShapes"`
	RuleMatrix      [][]int64                `json:"ruleMatrix"`
	RuleSets        [][]ruleDescription      `json:"ruleSets"`
}

// The ranges the tables' values must be in.
const (
	highestFrequency = 10000
	highestByte      = 255
	highestPlace     = 9
)

func (pack *Pack) LoadParameters() (*Parameters, error) {
	return load(pack, "parameters.json", parseParameters)
}

func parseParameters(data []byte) (*Parameters, error) {
	var description parametersDescription
	if err := json.Unmarshal(data, &description); err != nil {
		return nil, err
	}
	parameters := &Parameters{}
	if len(description.Shapes) != ShapeCount {
		return nil, fmt.Errorf("shapes has %d entries, expected %d", len(description.Shapes), ShapeCount)
	}
	for index, weights := range description.Shapes {
		name := fmt.Sprintf("shapes[%d]", index)
		if len(weights) > index {
			return nil, fmt.Errorf("%s has %d weights, at most %d allowed", name, len(weights), index)
		}
		if len(weights) > 0 && weights[0] != 255 {
			return nil, fmt.Errorf("%s starts with %d, expected 255", name, weights[0])
		}
		parameters.Shapes[index] = make([]byte, len(weights))
		if err := copyValues(parameters.Shapes[index], weights, name, 1, 255); err != nil {
			return nil, err
		}
	}
	if err := parsePhonemes(parameters, description.Phonemes); err != nil {
		return nil, err
	}
	if len(description.Classes) != ClassPlanes {
		return nil, fmt.Errorf("classes has %d planes, expected %d", len(description.Classes), ClassPlanes)
	}
	for plane, values := range description.Classes {
		if err := copyValues(parameters.Classes[plane][:], values, fmt.Sprintf("classes[%d]", plane), 0, highestByte); err != nil {
			return nil, err
		}
	}
	if len(description.Voices) != VoiceCount {
		return nil, fmt.Errorf("voices has %d entries, expected %d", len(description.Voices), VoiceCount)
	}
	for index, voice := range description.Voices {
		name := fmt.Sprintf("voices[%d]", index)
		if err := copyValues(parameters.Voices[index].Tracks[:], voice.Tracks, name+".tracks", 0, highestByte); err != nil {
			return nil, err
		}
		if err := copyValue(&parameters.Voices[index].FourthFormantLimit, voice.FourthFormantLimit, name+".fourthFormantLimit", 0, highestFrequency); err != nil {
			return nil, err
		}
	}
	if len(description.CategoryWeights) != CategoryCount {
		return nil, fmt.Errorf("categoryWeights has %d rows, expected %d", len(description.CategoryWeights), CategoryCount)
	}
	for previous, weights := range description.CategoryWeights {
		if err := copyValues(parameters.CategoryWeights[previous][:], weights, fmt.Sprintf("categoryWeights[%d]", previous), math.MinInt16, math.MaxInt16); err != nil {
			return nil, err
		}
	}
	if err := copyValues(parameters.DefaultShapes[:], description.DefaultShapes, "defaultShapes", 0, ShapeCount-1); err != nil {
		return nil, err
	}
	if err := parseRules(parameters, description); err != nil {
		return nil, err
	}
	return parameters, nil
}

func parsePhonemes(parameters *Parameters, descriptions []phonemeDescription) error {
	if len(descriptions) != PhonemeCount {
		return fmt.Errorf("phonemes has %d entries, expected %d", len(descriptions), PhonemeCount)
	}
	for symbol := range parameters.indexes {
		parameters.indexes[symbol] = -1
	}
	for index, description := range descriptions {
		name := fmt.Sprintf("phonemes[%d]", index)
		phoneme := &parameters.Phonemes[index]
		if len(description.Symbol) != 1 || !printable(description.Symbol[0]) {
			return fmt.Errorf("%s.symbol is %q, expected one printable character", name, description.Symbol)
		}
		phoneme.Symbol = description.Symbol[0]
		if parameters.indexes[phoneme.Symbol] >= 0 {
			return fmt.Errorf("%s.symbol %q is also phonemes[%d]'s", name, description.Symbol, parameters.indexes[phoneme.Symbol])
		}
		parameters.indexes[phoneme.Symbol] = index
		for _, field := range []struct {
			name             string
			values           []int64
			target           []int32
			minimum, maximum int64
		}{
			{"formants", description.Formants, phoneme.Formants[:], 0, highestFrequency},
			{"bandwidths", description.Bandwidths, phoneme.Bandwidths[:], 0, highestFrequency},
			{"amplitudes", description.Amplitudes, phoneme.Amplitudes[:], 0, highestByte},
		} {
			if err := copyValues(field.target, field.values, name+"."+field.name, field.minimum, field.maximum); err != nil {
				return err
			}
		}
		for _, field := range []struct {
			name             string
			value            *int64
			target           *int32
			minimum, maximum int64
		}{
			{"voicing", description.Voicing, &phoneme.Voicing, 0, highestByte},
			{"nasalZero", description.NasalZero, &phoneme.NasalZero, 0, highestFrequency},
			{"frontness", description.Frontness, &phoneme.Frontness, math.MinInt16, math.MaxInt16},
			{"place", description.Place, &phoneme.Place, 0, highestPlace},
		} {
			if err := copyValue(field.target, field.value, name+"."+field.name, field.minimum, field.maximum); err != nil {
				return err
			}
		}
		if description.Offglide != nil || description.EndFrontness != nil || description.Transition != nil {
			phoneme.Vowel = &Vowel{}
			if err := copyValues(phoneme.Vowel.Offglide[:], description.Offglide, name+".offglide", 0, highestFrequency); err != nil {
				return err
			}
			if err := copyValue(&phoneme.Vowel.EndFrontness, description.EndFrontness, name+".endFrontness", math.MinInt16, math.MaxInt16); err != nil {
				return err
			}
			if err := copyValue(&phoneme.Vowel.Transition, description.Transition, name+".transition", 0, highestByte); err != nil {
				return err
			}
		}
		if description.Burst != nil {
			if len(description.Burst) != 2 {
				return fmt.Errorf("%s.burst has %d rows, expected 2", name, len(description.Burst))
			}
			phoneme.Burst = &[2][4]int32{}
			for row, values := range description.Burst {
				if err := copyValues(phoneme.Burst[row][:], values, fmt.Sprintf("%s.burst[%d]", name, row), 0, highestByte); err != nil {
					return err
				}
			}
		}
	}
	if index, _ := parameters.Index(' '); index != PauseIndex {
		return fmt.Errorf("the pause is phonemes[%d], expected phonemes[%d]", index, PauseIndex)
	}
	return nil
}

func printable(symbol byte) bool {
	return symbol >= ' ' && symbol < SymbolCount-1
}

func parseRules(parameters *Parameters, description parametersDescription) error {
	parameters.RuleSets = make([][]Rule, len(description.RuleSets))
	for set, rules := range description.RuleSets {
		parameters.RuleSets[set] = make([]Rule, len(rules))
		for number, rule := range rules {
			name := fmt.Sprintf("ruleSets[%d][%d]", set, number)
			parsed := &parameters.RuleSets[set][number]
			for index, condition := range rule.If {
				parsedCondition, err := parseCondition(condition, fmt.Sprintf("%s.if[%d]", name, index))
				if err != nil {
					return err
				}
				parsed.Conditions = append(parsed.Conditions, parsedCondition)
			}
			for index, assignment := range rule.Set {
				parsedAssignment, err := parseAssignment(assignment, fmt.Sprintf("%s.set[%d]", name, index))
				if err != nil {
					return err
				}
				parsed.Assignments = append(parsed.Assignments, parsedAssignment)
			}
			for index, action := range rule.Do {
				if action < 1 || action > ActionCount {
					return fmt.Errorf("%s.do[%d] is %d, outside 1 to %d", name, index, action, ActionCount)
				}
				parsed.Actions = append(parsed.Actions, int(action))
			}
		}
	}
	if len(description.RuleMatrix) != PhonemeCount {
		return fmt.Errorf("ruleMatrix has %d rows, expected %d", len(description.RuleMatrix), PhonemeCount)
	}
	for current, row := range description.RuleMatrix {
		if len(row) != PhonemeCount {
			return fmt.Errorf("ruleMatrix[%d] has %d entries, expected %d", current, len(row), PhonemeCount)
		}
		for previous, set := range row {
			if set < 0 || set >= int64(len(parameters.RuleSets)) {
				return fmt.Errorf("ruleMatrix[%d][%d] is %d, outside 0 to %d", current, previous, set, len(parameters.RuleSets)-1)
			}
			parameters.RuleMatrix[current][previous] = int(set)
		}
	}
	return nil
}

var phonemeRoles = []string{"current", "next", "afterNext", "beforePrevious"}

func parseCondition(description conditionDescription, name string) (Condition, error) {
	condition := Condition{}
	if description.Is == nil {
		return condition, fmt.Errorf("%s.is is missing", name)
	}
	condition.Is = *description.Is
	if description.Flag != nil {
		if description.Phoneme != nil || description.Plane != nil || description.Bits != nil || description.In != nil {
			return condition, fmt.Errorf("%s tests a flag and a phoneme", name)
		}
		if *description.Flag < 0 || *description.Flag >= FlagCount {
			return condition, fmt.Errorf("%s.flag is %d, outside 0 to %d", name, *description.Flag, FlagCount-1)
		}
		condition.Kind, condition.Flag = FlagCondition, int(*description.Flag)
		return condition, nil
	}
	if description.Phoneme == nil {
		return condition, fmt.Errorf("%s has neither a flag nor a phoneme", name)
	}
	condition.Phoneme = -1
	for index, role := range phonemeRoles {
		if role == *description.Phoneme {
			condition.Phoneme = index
		}
	}
	if condition.Phoneme < 0 {
		return condition, fmt.Errorf("%s.phoneme is %q, expected one of %q", name, *description.Phoneme, phonemeRoles)
	}
	switch {
	case description.In != nil && description.Plane == nil && description.Bits == nil:
		if *description.In == "" {
			return condition, fmt.Errorf("%s.in is empty", name)
		}
		for _, symbol := range []byte(*description.In) {
			if !printable(symbol) {
				return condition, fmt.Errorf("%s.in has %q, expected printable characters", name, symbol)
			}
		}
		condition.Kind, condition.Symbols = SymbolCondition, *description.In
	case description.In == nil && description.Plane != nil && description.Bits != nil:
		if *description.Plane < 0 || *description.Plane >= ClassPlanes {
			return condition, fmt.Errorf("%s.plane is %d, outside 0 to %d", name, *description.Plane, ClassPlanes-1)
		}
		if *description.Bits < 1 || *description.Bits > highestByte || bits.OnesCount64(uint64(*description.Bits)) != 1 {
			return condition, fmt.Errorf("%s.bits is %d, expected one bit of a byte", name, *description.Bits)
		}
		condition.Kind, condition.Plane, condition.Bits = ClassCondition, int(*description.Plane), byte(*description.Bits)
	default:
		return condition, fmt.Errorf("%s needs either in, or plane and bits", name)
	}
	return condition, nil
}

func parsePlace(description []json.RawMessage, name string) (Place, error) {
	place := Place{Field: -1}
	if len(description) == 0 || len(description) > 2 {
		return place, fmt.Errorf("%s has %d entries, expected a field and a track, or a field", name, len(description))
	}
	var field string
	if err := json.Unmarshal(description[0], &field); err != nil {
		return place, fmt.Errorf("%s's field: %w", name, err)
	}
	for index, candidate := range fieldNames {
		if candidate == field {
			place.Field = Field(index)
		}
	}
	if place.Field < 0 {
		return place, fmt.Errorf("%s's field is %q, which isn't a field", name, field)
	}
	tracks := 0
	switch {
	case place.Field < BurstLevelField:
		tracks = TrackCount
	case place.Field == BurstLevelField:
		tracks = 3
	}
	if tracks == 0 {
		if len(description) != 1 {
			return place, fmt.Errorf("%s's field %s has no tracks", name, field)
		}
		return place, nil
	}
	if len(description) != 2 {
		return place, fmt.Errorf("%s's field %s needs a track", name, field)
	}
	if err := json.Unmarshal(description[1], &place.Track); err != nil {
		return place, fmt.Errorf("%s's track: %w", name, err)
	}
	if place.Track < 0 || place.Track >= tracks {
		return place, fmt.Errorf("%s's track is %d, outside 0 to %d", name, place.Track, tracks-1)
	}
	return place, nil
}

func parseAssignment(description assignmentDescription, name string) (Assignment, error) {
	assignment := Assignment{}
	var err error
	if assignment.To, err = parsePlace(description.To, name+".to"); err != nil {
		return assignment, err
	}
	operations := 0
	for _, operation := range []struct {
		value     *int64
		operation Operation
	}{{description.Set, Set}, {description.Add, Add}, {description.Subtract, Subtract}} {
		if operation.value == nil {
			continue
		}
		operations++
		if *operation.value < math.MinInt32 || *operation.value > math.MaxInt32 {
			return assignment, fmt.Errorf("%s's value %d is outside 32 bits", name, *operation.value)
		}
		assignment.Operation, assignment.Value = operation.operation, int32(*operation.value)
	}
	if description.Copy != nil {
		operations++
		assignment.Operation = Copy
		if assignment.From, err = parsePlace(description.Copy, name+".copy"); err != nil {
			return assignment, err
		}
	}
	if operations != 1 {
		return assignment, fmt.Errorf("%s has %d of set, add, subtract and copy, expected one", name, operations)
	}
	return assignment, nil
}
