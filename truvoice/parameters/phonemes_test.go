package parameters

import (
	"encoding/binary"
	"fmt"
	"maps"
	"slices"
	"sync"
	"testing"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
)

// TestPhonemes replays every recorded call of the stage's resets, the target loading, the rules, the actions
// the port has and the rendering of a phoneme, with the other actions taken from the records.
func TestPhonemes(t *testing.T) {
	data := loadTestData(t)
	var lock sync.Mutex
	fired := map[[2]int]int{}
	roles := map[string]int{}
	phonemeRules := map[string]int{}
	note := func(generator *Generator) {
		cursor := generator.cursor()
		current, _ := generator.phoneme(cursor.Scan.Symbol())
		previous, _ := generator.phoneme(cursor.Done.Symbol())
		set := data.parameters.RuleMatrix[current][previous]
		lock.Lock()
		defer lock.Unlock()
		for number, rule := range data.parameters.RuleSets[set] {
			if generator.passes(rule.Conditions) {
				fired[[2]int{set, number}]++
				break
			}
		}
		for role, node := range map[string]*pipeline.Node{"previous": cursor.Done, "current": cursor.Scan, "next": cursor.First,
			"after next": cursor.First.Next, "before previous": cursor.Done.Previous} {
			switch _, found := data.parameters.Index(symbolOf(node)); {
			case node == nil:
				roles[role+" missing"]++
			case !found:
				roles[fmt.Sprintf("%s %q type %d", role, node.Symbol(), node.Type())]++
			}
		}
	}
	plain := func(run func(generator *Generator)) replayer {
		return func(call *call) error {
			generator := data.load(call)
			run(generator)
			return compare(generator, call)
		}
	}
	counts := replay(t, map[string]replayer{
		"Parameters::Reset": func(call *call) error {
			generator := data.load(call)
			generator.Reset()
			image := slices.Clone(call.before)
			copy(image[stateLevels:stateLevels+datapack.TrackCount], image[stateSettings:])
			binary.LittleEndian.PutUint32(image[stateTypes:], 0x28)
			return difference(generator, image, call.after)
		},
		"Parameters::ResetContext": plain((*Generator).ResetContext),
		"Parameters::LoadTargets":  plain((*Generator).LoadTargets),
		"Parameters::InitModes":    plain((*Generator).InitModes),
		"Parameters::ApplyPhonemeRules": func(call *call) error {
			note(data.load(call))
			return data.withActions(call, (*Generator).ApplyRules)
		},
		"Parameters::RenderPhoneme": func(call *call) error {
			return data.withActions(call, (*Generator).RenderPhoneme)
		},
		"Parameters::PhonemeRules": func(call *call) error {
			generator := data.load(call)
			cursor := generator.cursor()
			lock.Lock()
			phonemeRules[string(cursor.Scan.Symbol())]++
			lock.Unlock()
			generator.PhonemeRules()
			return compare(generator, call)
		},
		"Parameters::ClassifyPrevVowel":       plain((*Generator).groupPrevious),
		"Parameters::ShiftAspirationBoundary": plain((*Generator).ShiftAspiration),
		"Parameters::ShapesFromPrevious":      plain((*Generator).ShapesFromPrevious),
		"Parameters::VowelOnset":              plain((*Generator).VowelOnset),
		"Parameters::SonorantTransitions":     plain((*Generator).SonorantTransitions),
		"Parameters::ObstruentVoicing":        plain((*Generator).ObstruentVoicing),
		"Parameters::ConsonantShapes":         plain((*Generator).ConsonantShapes),
		"PullAspirationBack":                  plain((*Generator).PullAspirationBack),
	})
	t.Logf("calls replayed: %v", counts)
	ruleCount, unreached := 0, []int{}
	for set, rules := range data.parameters.RuleSets {
		ruleCount += len(rules)
		if !slices.ContainsFunc(slices.Collect(maps.Keys(fired)), func(rule [2]int) bool { return rule[0] == set }) {
			unreached = append(unreached, set)
		}
	}
	t.Logf("rules fired: %d of %d; rule sets none fired in: %v; phonemes that aren't: %v", len(fired), ruleCount, unreached, roles)
	t.Logf("per-phoneme rules by phoneme: %v", phonemeRules)
}
