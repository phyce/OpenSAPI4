// Package analyser is the text analyser's sentence state and its passes: transcription by rule modules C
// and B, and analysis by module A with the stress levels (text.md §9, prosody.md §2–4).
package analyser

import (
	"fmt"

	"github.com/phyce/OpenSAPI4/whistler/datapack"
	"github.com/phyce/OpenSAPI4/whistler/pronunciation"
	"github.com/phyce/OpenSAPI4/whistler/prosody"
	"github.com/phyce/OpenSAPI4/whistler/rules"
)

const (
	TranscriptionFailed = -101
	AnalysisOverflowed  = -200
	explicitPointLimit  = 0x280
)

var entryGroups = map[string][]int{"A": {5, 10, 14, 15, 16}, "B": {3, 6, 9, 16}, "C": {50, 56, 57, 58, 59}}

type ExplicitPoint struct {
	Time  int32
	Value int16
	Cell  int
}

// State is one sentence in the analyser: the tiers and anchors, the events the reader queued, and what
// the annotations set.
type State struct {
	rules.State
	Start         int
	End           int
	DurationStyle int
	Events        []Event
	Durations     [rules.Cells]int16
	PointIndex    [rules.Cells]int16
	Points        []ExplicitPoint
	MultipleWords bool
	Transcribed   []byte
}

// NewState holds tier 1 and the anchors the reader left over cells 1..len(tier1).
func NewState(tier1 []byte, anchors []byte, durationStyle int) *State {
	state := &State{State: *rules.NewState(), Start: 1, End: len(tier1), DurationStyle: durationStyle}
	copy(state.Tier(1)[1:], tier1)
	copy(state.Anchors[1:], anchors)
	return state
}

// Sentence gives the state after Analyse as the prosody stage reads it.
func (state *State) Sentence() *prosody.Sentence {
	cells := state.End + 1
	sentence := &prosody.Sentence{
		Start:         state.Start,
		End:           state.End,
		Tier1:         clone(state.Tier(1)[:cells]),
		Tier2:         clone(state.Tier(2)[:cells]),
		Tier3:         clone(state.Tier(3)[:cells]),
		Tier4:         clone(state.Tier(4)[:cells]),
		Tier5:         clone(state.Tier(5)[:cells]),
		Transcribed:   state.Transcribed,
		Durations:     clone(state.Durations[:cells]),
		PointIndex:    clone(state.PointIndex[:cells]),
		Anchors:       clone(state.Anchors[:cells]),
		MultipleWords: state.MultipleWords,
		DurationStyle: state.DurationStyle,
	}
	for _, point := range state.Points {
		sentence.Points = append(sentence.Points, prosody.Point{Time: point.Time, Value: point.Value})
	}
	for _, event := range state.Events {
		sentence.Events = append(sentence.Events, prosody.Event{Kind: event.Kind, Value: int(event.Value)})
	}
	return sentence
}

func clone[T any](cells []T) []T {
	return append([]T(nil), cells...)
}

// Analyser runs the three rule modules. Their registers last from one sentence to the next, as in the
// original.
type Analyser struct {
	tables  *datapack.Prosody
	moduleA *rules.Machine
	moduleB *rules.Machine
	moduleC *rules.Machine
	state   *State
}

func New(ruleModules *datapack.Rules, dictionaries *datapack.Dictionaries, tables *datapack.Prosody) (*Analyser, error) {
	for name, groups := range entryGroups {
		module := ruleModules.Modules[name]
		if module == nil {
			return nil, fmt.Errorf("the data pack has no rule module %s", name)
		}
		for _, group := range groups {
			if _, found := module.Groups[group]; !found {
				return nil, fmt.Errorf("rule module %s has no group %d", name, group)
			}
		}
	}
	natives, err := pronunciation.NewNatives(dictionaries)
	if err != nil {
		return nil, err
	}
	analyser := &Analyser{tables: tables}
	machines := []struct {
		machine **rules.Machine
		name    string
		natives map[int]rules.Native
	}{
		{&analyser.moduleA, "A", analyser.accentNatives()},
		{&analyser.moduleB, "B", analyser.phrasingNatives()},
		{&analyser.moduleC, "C", natives.Procedures()},
	}
	for _, entry := range machines {
		machine, err := rules.NewMachine(ruleModules.Modules[entry.name], nil, entry.natives, entry.name == "A")
		if err != nil {
			return nil, err
		}
		*entry.machine = machine
	}
	return analyser, nil
}

func (analyser *Analyser) use(state *State) {
	analyser.state = state
	for _, machine := range []*rules.Machine{analyser.moduleA, analyser.moduleB, analyser.moduleC} {
		machine.State = &state.State
	}
}

func (state *State) clearAnnotations() {
	state.Durations = [rules.Cells]int16{}
	state.PointIndex = [rules.Cells]int16{}
	state.Points = nil
}

// Transcribe runs module C group 50 and module B groups 16, 6 and 3 on the sentence. It reports false when a
// module fails or only pauses are left, and the sentence is dropped (text.md §9).
func (analyser *Analyser) Transcribe(state *State) bool {
	analyser.use(state)
	state.clearAnnotations()
	for cell := state.Start; cell <= state.End; cell++ {
		state.Tier(2)[cell] = state.Tier(1)[cell]
		state.Tier(3)[cell], state.Tier(4)[cell], state.Tier(5)[cell] = ' ', ' ', ' '
	}
	transcribed := false
	analyser.moduleC.Begin(true)
	analyser.moduleC.ApplyGroup(50, state.Start, &state.End)
	if !analyser.moduleC.Failed() {
		analyser.moduleB.Begin(true)
		for _, group := range []int{16, 6, 3} {
			analyser.moduleB.ApplyGroup(group, state.Start, &state.End)
		}
		if !analyser.moduleB.Failed() {
			for cell := state.Start; cell <= state.End; cell++ {
				if code := state.Tier(1)[cell]; code != '#' && code != rules.Empty && code != ' ' {
					transcribed = true
					break
				}
			}
		}
	}
	state.clearAnnotations()
	state.Transcribed = append([]byte(nil), state.Tier(1)[1:state.End+1]...)
	return transcribed
}

// Analyse runs module A group 5 (the sentence flag), group 10, the stress levels and group 16. It reports
// false when module A overflows; the sentence is then dropped. The sentence's end after module A is in
// state.End.
func (analyser *Analyser) Analyse(state *State) bool {
	analyser.use(state)
	machine := analyser.moduleA
	machine.Begin(false)
	start, end := state.Start, state.End
	defer func() { state.End = end }()
	multipleWords, aborted := machine.Enter(5, start, &end)
	if aborted {
		return false
	}
	state.MultipleWords = multipleWords
	if _, aborted = machine.Enter(10, start, &end); aborted {
		return false
	}
	prosody.AssignStress(analyser.tables, state.Tier(1)[:], state.Tier(3)[:], start, end, multipleWords)
	if _, aborted = machine.Enter(16, start, &end); aborted {
		return false
	}
	return !machine.Overflowed()
}
