package parameters

import (
	"strings"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
)

// The rules' flags (parameters.md §5.1).
const (
	VoicedFlag = iota
	AfterVowelFlag
	AfterClosureFlag
	AfterSonorantFlag
	AfterFricativeFlag
	ClosedFlag
	BeforePlace0Flag
	BeforePlace1Flag
	BeforePlace3Flag
	BeforePlace2Flag
	BeforeStressedFlag
)

// ApplyRules applies the first rule of the phoneme's rule set whose conditions pass: its assignments, then its
// actions (parameters.md §5).
func (generator *Generator) ApplyRules() {
	cursor := generator.cursor()
	current, _ := generator.phoneme(cursor.Scan.Symbol())
	previous, _ := generator.phoneme(cursor.Done.Symbol())
	for _, rule := range generator.tables.RuleSets[generator.tables.RuleMatrix[current][previous]] {
		if !generator.passes(rule.Conditions) {
			continue
		}
		generator.Assign(rule.Assignments)
		for _, action := range rule.Actions {
			generator.act(action)
		}
		return
	}
}

// ported are the rules' actions the port has (parameters.md §5.4).
var ported = [datapack.ActionCount + 1]func(generator *Generator){
	1:  (*Generator).ShiftAspiration,
	2:  (*Generator).ShapesFromPrevious,
	3:  (*Generator).VowelOnset,
	4:  (*Generator).VowelCoarticulation,
	5:  (*Generator).SonorantTransitions,
	6:  (*Generator).ObstruentVoicing,
	7:  (*Generator).FricativeAmplitudes,
	8:  (*Generator).ConsonantShapes,
	9:  (*Generator).StopClosureBurst,
	10: (*Generator).FinishPhoneme,
	11: (*Generator).LocusWeights,
	12: (*Generator).SpecialLoci,
	13: (*Generator).ConsonantTargetsAndPitch,
	15: (*Generator).PullAspirationBack,
	16: (*Generator).PhonemeRules,
}

func (generator *Generator) act(action int) {
	if run := ported[action]; run != nil {
		run(generator)
		return
	}
	generator.Actions.Run(generator, action)
}

func (generator *Generator) passes(conditions []datapack.Condition) bool {
	for _, condition := range conditions {
		var result bool
		switch condition.Kind {
		case datapack.FlagCondition:
			result = generator.Flags[condition.Flag]
		case datapack.ClassCondition:
			result = generator.has(generator.operand(condition.Phoneme), condition.Plane, condition.Bits)
		case datapack.SymbolCondition:
			result = strings.IndexByte(condition.Symbols, generator.operand(condition.Phoneme)) >= 0
		}
		if result != condition.Is {
			return false
		}
	}
	return true
}

// operand returns the symbol of the phoneme a condition tests.
func (generator *Generator) operand(phoneme int) byte {
	cursor := generator.cursor()
	var node *pipeline.Node
	switch phoneme {
	case datapack.CurrentPhoneme:
		node = cursor.Scan
	case datapack.NextPhoneme:
		node = cursor.First
	case datapack.AfterNextPhoneme:
		node = cursor.First.Next
	case datapack.BeforePreviousPhoneme:
		node = cursor.Done.Previous
	}
	return symbolOf(node)
}

// Assign runs a rule's assignments in order.
func (generator *Generator) Assign(assignments []datapack.Assignment) {
	for _, assignment := range assignments {
		value := generator.field(assignment.To)
		switch assignment.Operation {
		case datapack.Set:
			*value = assignment.Value
		case datapack.Add:
			*value += assignment.Value
		case datapack.Subtract:
			*value -= assignment.Value
		case datapack.Copy:
			*value = *generator.field(assignment.From)
		}
	}
}

func (generator *Generator) field(place datapack.Place) *int32 {
	segment := &generator.Segments[place.Track]
	switch place.Field {
	case datapack.TargetField:
		return &segment.Target
	case datapack.ModeField:
		return &segment.Mode
	case datapack.ForwardField:
		return &segment.Forward
	case datapack.BackField:
		return &segment.Back
	case datapack.OnsetField:
		return &segment.Onset
	case datapack.BackTargetField:
		return &segment.BackTarget
	case datapack.FramesField:
		return &segment.Frames
	case datapack.NextField:
		return &generator.Tracks[place.Track].Next
	case datapack.EarliestField:
		return &generator.Tracks[place.Track].Earliest
	case datapack.NextTargetField:
		return &generator.NextTargets[place.Track]
	case datapack.LastField:
		return &generator.Last[place.Track]
	case datapack.WeightField:
		return &generator.Weights[place.Track]
	case datapack.BurstLevelField:
		return &generator.BurstLevels[place.Track]
	case datapack.PreviousCategoryField:
		return &generator.PreviousCategory
	case datapack.CategoryField:
		return &generator.Category
	case datapack.PreviousFrontnessField:
		return &generator.PreviousFrontness
	case datapack.FrontnessField:
		return &generator.Frontness
	case datapack.AspirationShiftField:
		return &generator.AspirationShift
	case datapack.VoiceOnsetField:
		return &generator.VoiceOnset
	case datapack.BurstFramesField:
		return &generator.BurstFrames
	case datapack.ClosureFramesField:
		return &generator.ClosureFrames
	case datapack.FricativeAspirationField:
		return &generator.FricativeAspiration
	case datapack.AspirationField:
		return &generator.Aspiration
	case datapack.VowelOffsetField:
		return &generator.VowelOffset
	}
	return &generator.VowelTransition
}
