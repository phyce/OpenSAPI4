package pronunciation

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"testing"

	"github.com/phyce/OpenSAPI4/whistler/datapack"
	"github.com/phyce/OpenSAPI4/whistler/internal/conformance"
	"github.com/phyce/OpenSAPI4/whistler/rules"
)

const lookupStart = 10

type report struct {
	t        *testing.T
	failures int
}

func (report *report) check(name string, condition bool, format string, arguments ...any) bool {
	if !condition {
		report.failures++
		if report.failures <= 20 {
			report.t.Errorf("%s: %s", name, fmt.Sprintf(format, arguments...))
		}
	}
	return condition
}

func lookupState(word []byte) (*rules.State, int) {
	state := rules.NewState()
	end := lookupStart + len(word) - 1
	for cell := 1; cell < rules.TailStart; cell++ {
		state.Tier(5)[cell] = '5'
	}
	for cell := end + 1; cell < rules.TailStart; cell++ {
		state.Tier(1)[cell], state.Tier(2)[cell], state.Tier(3)[cell], state.Tier(4)[cell] = ' ', 'y', 'c', 'd'
	}
	for offset, value := range word {
		cell := lookupStart + offset
		state.Tier(1)[cell], state.Tier(2)[cell] = value, value
		state.Tier(3)[cell], state.Tier(4)[cell] = '3', '4'
	}
	return state, end
}

func TestLookups(t *testing.T) {
	dictionaries, err := conformance.Pack(t).LoadDictionaries()
	if err != nil {
		t.Fatal(err)
	}
	natives, err := NewNatives(dictionaries)
	if err != nil {
		t.Fatal(err)
	}
	lookups := conformance.ReadLines[conformance.RuleLookup](t, "golden", "rules", "lookups.jsonl.gz")
	report := &report{t: t}
	found := 0
	for _, golden := range lookups {
		name := fmt.Sprintf("dictionary %d %q", golden.Procedure, golden.Word)
		state, end := lookupState(conformance.Bytes(golden.Word))
		spanEnd := end
		result := natives.lookup(golden.Procedure)(&rules.Machine{State: state}, lookupStart, &spanEnd)
		if result {
			found++
		}
		report.check(name, result == golden.Found && spanEnd == golden.End, "found %t, end %d; expected %t, %d", result, spanEnd, golden.Found, golden.End)
		if golden.Flag != nil {
			flag := natives.homographFound
			if golden.Procedure == verbs {
				flag = natives.verbFound
			}
			report.check(name, flag == (*golden.Flag == 1), "flag %t, expected %d", flag, *golden.Flag)
		}
		last := max(end, golden.End) + 2
		for number := 1; number <= 5; number++ {
			actual := state.Tier(number)[lookupStart-1 : last+1]
			expected := conformance.Bytes(golden.Tiers[number-1])
			report.check(name, slices.Equal(actual, expected), "tier %d %q, expected %q", number, actual, expected)
		}
	}
	if len(lookups) < 100000 {
		t.Errorf("only %d lookups", len(lookups))
	}
	t.Logf("%d lookups, %d found", len(lookups), found)
}

// moduleC holds module C's interpreter for one text: its registers last from one sentence to the next.
type moduleC struct {
	machine *rules.Machine
}

func newModuleC(t *testing.T, module *datapack.RuleModule, natives *Natives) *moduleC {
	machine, err := rules.NewMachine(module, nil, natives.Procedures(), false)
	if err != nil {
		t.Fatal(err)
	}
	return &moduleC{machine: machine}
}

func readState(read conformance.RuleRead) *rules.State {
	state := rules.NewState()
	copy(state.Tier(1)[1:], conformance.Bytes(read.Tier1))
	for index, count := range read.Anchors {
		state.Anchors[index+1] = byte(count)
	}
	copy(state.Tier(1)[read.End+1:], conformance.Bytes(read.Beyond))
	for _, anchor := range read.AnchorsBeyond {
		state.Anchors[anchor[0]] = byte(anchor[1])
	}
	return state
}

// transcribe runs the first pass of the transcription (text.md §9): tier 2 = tier 1, tiers 3 to 5 blank,
// then group 50.
func (module *moduleC) transcribe(state *rules.State, end int) (int, bool) {
	machine := module.machine
	machine.State = state
	machine.Begin(true)
	for cell := 1; cell <= end; cell++ {
		state.Tier(2)[cell] = state.Tier(1)[cell]
		state.Tier(3)[cell], state.Tier(4)[cell], state.Tier(5)[cell] = ' ', ' ', ' '
	}
	machine.ApplyGroup(50, 1, &end)
	return end, !machine.Failed()
}

func compareSnapshot(report *report, name string, state *rules.State, end int, golden *conformance.RuleSnapshot) {
	if !report.check(name, end == golden.End, "end %d, expected %d", end, golden.End) {
		return
	}
	for number := 1; number <= 5; number++ {
		actual := state.Tier(number)[1 : end+1]
		expected := conformance.Bytes(golden.Tiers[number-1])
		report.check(name, slices.Equal(actual, expected), "tier %d\n%q\nexpected\n%q", number, actual, expected)
	}
	anchors := make([]int, end)
	for cell := range anchors {
		anchors[cell] = int(state.Anchors[cell+1])
	}
	report.check(name, slices.Equal(anchors, golden.Anchors), "anchors %v, expected %v", anchors, golden.Anchors)
}

func TestModuleC(t *testing.T) {
	pack := conformance.Pack(t)
	rulesData, err := pack.LoadRules()
	if err != nil {
		t.Fatal(err)
	}
	dictionaries, err := pack.LoadDictionaries()
	if err != nil {
		t.Fatal(err)
	}
	texts := conformance.ReadLines[conformance.RuleText](t, "golden", "rules", "texts.jsonl.gz")
	report := &report{t: t}
	sentences, failed := 0, 0
	for _, text := range texts {
		natives, err := NewNatives(dictionaries)
		if err != nil {
			t.Fatal(err)
		}
		module := newModuleC(t, rulesData.Modules["C"], natives)
		for number, sentence := range text.Sentences {
			name := fmt.Sprintf("%s sentence %d", text.Name, number+1)
			state := readState(sentence.Read)
			end, transcribed := module.transcribe(state, sentence.Read.End)
			sentences++
			switch {
			case sentence.Error != "":
				report.check(name, !transcribed, "transcribed where the model failed with %s", sentence.Error)
			case sentence.AfterC == nil:
				failed++
				report.check(name, !transcribed, "transcribed where module C failed")
			default:
				report.check(name, transcribed, "module C failed")
				compareSnapshot(report, name, state, end, sentence.AfterC)
			}
		}
	}
	t.Logf("%d texts, %d sentences, %d where module C fails", len(texts), sentences, failed)
}

func TestARPAbet(t *testing.T) {
	tables, err := conformance.Pack(t).LoadProsody()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(conformance.Path(t, "golden", "rules", "arpabet.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Pronunciation string  `json:"pronunciation"`
		Phones        *string `json:"phones"`
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	report := &report{t: t}
	unknown := 0
	for _, golden := range cases {
		phones, converted := FromARPAbet(tables, string(conformance.Bytes(golden.Pronunciation)))
		if golden.Phones == nil {
			unknown++
			report.check(golden.Pronunciation, !converted, "converted to %q", phones)
			continue
		}
		expected := conformance.Bytes(*golden.Phones)
		report.check(golden.Pronunciation, converted && slices.Equal(phones, expected), "%q (%t), expected %q", phones, converted, expected)
	}
	if len(cases) < 700 {
		t.Errorf("only %d cases", len(cases))
	}
	t.Logf("%d pronunciations, %d with an unknown name", len(cases), unknown)
}

func TestNumberSyllables(t *testing.T) {
	state := rules.NewState()
	word := []byte("#a-b-c-d-e-f-g-h-i-j-k-l#")
	copy(state.Tier(1)[1:], word)
	end := len(word)
	numberSyllables(&rules.Machine{State: state}, 1, &end)
	expected := []byte("      9 8 7 6 5 4 3 2 1  ")
	if actual := state.Tier(4)[1 : end+1]; !slices.Equal(actual, expected) {
		t.Errorf("tier 4 %q, expected %q", actual, expected)
	}
}
