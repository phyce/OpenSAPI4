package prosody

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"runtime/debug"
	"slices"
	"testing"

	"github.com/phyce/OpenSAPI4/whistler/datapack"
	"github.com/phyce/OpenSAPI4/whistler/internal/conformance"
)

type goldenInput struct {
	Start         int        `json:"start"`
	End           int        `json:"end"`
	Tiers         [5]string  `json:"tiers"`
	Transcribed   string     `json:"transcribed"`
	Durations     []int16    `json:"durations"`
	Points        [][2]int32 `json:"points"`
	PointIndex    []int16    `json:"pointIndex"`
	Anchors       []byte     `json:"anchors"`
	Events        [][2]int   `json:"events"`
	SentenceFlag  int        `json:"sentenceFlag"`
	DurationStyle int        `json:"durationStyle"`
}

type goldenStress struct {
	Start        int    `json:"start"`
	End          int    `json:"end"`
	SentenceFlag int    `json:"sentenceFlag"`
	Tier1        string `json:"tier1"`
	Tier3        string `json:"tier3"`
	Stressed     string `json:"stressed"`
}

type goldenAnalysis struct {
	Dropped   bool    `json:"dropped"`
	Start     int     `json:"start"`
	End       int     `json:"end"`
	Tier1     string  `json:"tier1"`
	Durations []int16 `json:"durations"`
	Marks     string  `json:"marks"`
	UnitIDs   []int16 `json:"unitIds"`
	Frames    []int16 `json:"frames"`
	Closures  []int   `json:"closures"`
}

type goldenIntonation struct {
	Events   [][3]int `json:"events"`
	Targets  [][3]int `json:"targets"`
	Text     string   `json:"text"`
	KeptRate bool     `json:"keptRate"`
}

type goldenBuild struct {
	PitchSteps     int               `json:"pitchSteps"`
	NoFinalSilence bool              `json:"noFinalSilence"`
	Units          []json.RawMessage `json:"units"`
	Count          int               `json:"count"`
	TagsPlaced     int               `json:"tagsPlaced"`
	Error          string            `json:"error"`
}

type goldenCase struct {
	Name       string            `json:"name"`
	Input      goldenInput       `json:"input"`
	Stress     *goldenStress     `json:"stress"`
	Analysis   goldenAnalysis    `json:"analysis"`
	Intonation *goldenIntonation `json:"intonation"`
	Units      []goldenBuild     `json:"units"`
}

func loadTables(t *testing.T) *datapack.Prosody {
	t.Helper()
	tables, err := conformance.Pack(t).LoadProsody()
	if err != nil {
		t.Fatal(err)
	}
	return tables
}

func loadCases(t *testing.T) []goldenCase {
	t.Helper()
	file, err := os.Open(conformance.Path(t, "golden", "prosody", "cases.jsonl.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(nil, 1<<24)
	var cases []goldenCase
	for scanner.Scan() {
		var golden goldenCase
		if err := json.Unmarshal(scanner.Bytes(), &golden); err != nil {
			t.Fatal(err)
		}
		cases = append(cases, golden)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return cases
}

func latin1(text string) []byte {
	data := make([]byte, 0, len(text))
	for _, character := range text {
		data = append(data, byte(character))
	}
	return data
}

func cellTier(text string) []byte {
	return append([]byte{0}, latin1(text)...)
}

func sentenceOf(input goldenInput) *Sentence {
	sentence := &Sentence{
		Start:         input.Start,
		End:           input.End,
		Tier1:         cellTier(input.Tiers[0]),
		Tier2:         cellTier(input.Tiers[1]),
		Tier3:         cellTier(input.Tiers[2]),
		Tier4:         cellTier(input.Tiers[3]),
		Tier5:         cellTier(input.Tiers[4]),
		Transcribed:   latin1(input.Transcribed),
		Durations:     append([]int16{0}, input.Durations...),
		PointIndex:    append([]int16{0}, input.PointIndex...),
		Anchors:       append([]byte{0}, input.Anchors...),
		MultipleWords: input.SentenceFlag != 0,
		DurationStyle: input.DurationStyle,
	}
	for _, point := range input.Points {
		sentence.Points = append(sentence.Points, Point{Time: point[0], Value: int16(point[1])})
	}
	for _, event := range input.Events {
		sentence.Events = append(sentence.Events, Event{Kind: EventKind(event[0]), Value: event[1]})
	}
	return sentence
}

type failures struct {
	t     *testing.T
	count int
}

func (failures *failures) check(name string, matches bool, format string, arguments ...any) {
	if matches {
		return
	}
	failures.count++
	if failures.count <= 20 {
		failures.t.Errorf("%s: %s", name, fmt.Sprintf(format, arguments...))
	}
}

func TestStress(t *testing.T) {
	tables := loadTables(t)
	checked := 0
	report := &failures{t: t}
	for _, golden := range loadCases(t) {
		stress := golden.Stress
		if stress == nil {
			continue
		}
		tier1, tier3 := cellTier(stress.Tier1), cellTier(stress.Tier3)
		AssignStress(tables, tier1, tier3, stress.Start, stress.End, stress.SentenceFlag != 0)
		report.check(golden.Name, string(tier3[1:]) == string(latin1(stress.Stressed)), "tier 3 %q, expected %q", tier3[1:], latin1(stress.Stressed))
		checked++
	}
	t.Logf("%d sentences", checked)
	if checked < 2000 {
		t.Errorf("only %d sentences with stress input", checked)
	}
}

func TestAnalyze(t *testing.T) {
	tables := loadTables(t)
	cases := loadCases(t)
	report := &failures{t: t}
	dropped, keptRate := 0, 0
	for _, golden := range cases {
		analysis, kept := Analyze(tables, sentenceOf(golden.Input))
		expected := golden.Analysis
		report.check(golden.Name, kept != expected.Dropped, "kept %v, dropped %v", kept, expected.Dropped)
		if !kept || expected.Dropped {
			dropped++
			continue
		}
		end := expected.End
		closures := make([]byte, len(expected.Closures))
		for index, closure := range expected.Closures {
			closures[index] = byte(closure)
		}
		report.check(golden.Name, analysis.Start == expected.Start && analysis.End == end, "cells %d..%d, expected %d..%d", analysis.Start, analysis.End, expected.Start, end)
		if analysis.End != end {
			continue
		}
		report.check(golden.Name, string(analysis.Tier1[1:end+1]) == string(latin1(expected.Tier1)), "tier 1 %q, expected %q", analysis.Tier1[1:end+1], latin1(expected.Tier1))
		report.check(golden.Name, slices.Equal(analysis.Durations[1:end+1], expected.Durations), "durations %v, expected %v", analysis.Durations[1:end+1], expected.Durations)
		report.check(golden.Name, string(analysis.Marks[1:end+1]) == string(latin1(expected.Marks)), "marks %q, expected %q", analysis.Marks[1:end+1], latin1(expected.Marks))
		report.check(golden.Name, slices.Equal(analysis.Units[1:end+1], expected.UnitIDs), "units %v, expected %v", analysis.Units[1:end+1], expected.UnitIDs)
		report.check(golden.Name, slices.Equal(analysis.Frames[1:end+1], expected.Frames), "frames %v, expected %v", analysis.Frames[1:end+1], expected.Frames)
		report.check(golden.Name, slices.Equal(analysis.Closures[1:end+1], closures), "closures %v, expected %v", analysis.Closures[1:end+1], closures)
		intonation := golden.Intonation
		if intonation == nil {
			report.check(golden.Name, false, "no intonation in the golden data")
			continue
		}
		events := make([][3]int, len(analysis.Events))
		for index, event := range analysis.Events {
			events[index] = [3]int{int(event.Kind), event.Value, event.Time}
		}
		targets := make([][3]int, len(analysis.Targets))
		for index, target := range analysis.Targets {
			targets[index] = [3]int{int(target.Flag), int(target.Value), int(target.Time)}
		}
		report.check(golden.Name, slices.Equal(events, intonation.Events), "events %v, expected %v", events, intonation.Events)
		report.check(golden.Name, slices.Equal(targets, intonation.Targets), "targets %v, expected %v", targets, intonation.Targets)
		report.check(golden.Name, string(analysis.Text) == string(latin1(intonation.Text)), "text %q, expected %q", analysis.Text, latin1(intonation.Text))
		if intonation.KeptRate {
			keptRate++
		}
	}
	t.Logf("%d sentences, %d dropped, %d keep the declination rate, %d differences", len(cases), dropped, keptRate, report.count)
	if len(cases) < 8000 || dropped < 100 || keptRate < 12 {
		t.Errorf("only %d sentences, %d dropped, %d keep the declination rate", len(cases), dropped, keptRate)
	}
}

func TestPitchSteps(t *testing.T) {
	data, err := os.ReadFile(conformance.Path(t, "golden", "prosody", "pitchsteps.json"))
	if err != nil {
		t.Fatal(err)
	}
	var steps []int
	if err := json.Unmarshal(data, &steps); err != nil {
		t.Fatal(err)
	}
	if len(steps) != 65535 {
		t.Fatalf("%d pitch steps", len(steps))
	}
	report := &failures{t: t}
	for index, expected := range steps {
		hertz := index + 1
		report.check(fmt.Sprintf("%d Hz", hertz), PitchSteps(hertz) == expected, "%d steps, expected %d", PitchSteps(hertz), expected)
	}
}

func unitJSON(unit Unit) string {
	var fields []any
	switch unit.Kind {
	case PhoneUnit:
		fields = []any{unit.Name, float64(unit.Duration), float64(unit.Pitch)}
	case SilenceUnit:
		fields = []any{unit.Name, float64(unit.Duration)}
	default:
		fields = []any{unit.Name}
	}
	data, _ := json.Marshal(fields)
	return string(data)
}

func canonicalJSON(data json.RawMessage) string {
	var fields []any
	if err := json.Unmarshal(data, &fields); err != nil {
		return string(data)
	}
	canonical, _ := json.Marshal(fields)
	return string(canonical)
}

func TestBuildUnits(t *testing.T) {
	tables := loadTables(t)
	report := &failures{t: t}
	builds, units, readPast := 0, 0, 0
	for _, golden := range loadCases(t) {
		if len(golden.Units) == 0 {
			continue
		}
		analysis, kept := Analyze(tables, sentenceOf(golden.Input))
		if !kept {
			report.check(golden.Name, false, "dropped")
			continue
		}
		transcribed := latin1(golden.Input.Transcribed)
		for _, build := range golden.Units {
			name := fmt.Sprintf("%s at %d steps", golden.Name, build.PitchSteps)
			if build.Error != "" {
				BuildUnits(tables, analysis, transcribed, build.PitchSteps, build.NoFinalSilence)
				readPast++
				continue
			}
			counter := newBuilder(tables, analysis, build.PitchSteps, build.NoFinalSilence)
			_, count, _ := counter.build(append([]byte{}, transcribed...), false)
			report.check(name, count == build.Count, "count pass %d, expected %d", count, build.Count)
			built, placed := BuildUnits(tables, analysis, transcribed, build.PitchSteps, build.NoFinalSilence)
			report.check(name, placed == build.TagsPlaced, "%d tags placed, expected %d", placed, build.TagsPlaced)
			actual := make([]string, len(built))
			for index, unit := range built {
				actual[index] = unitJSON(unit)
			}
			expected := make([]string, len(build.Units))
			for index, unit := range build.Units {
				expected[index] = canonicalJSON(unit)
			}
			if !slices.Equal(actual, expected) {
				first := 0
				for first < min(len(actual), len(expected)) && actual[first] == expected[first] {
					first++
				}
				report.check(name, false, "%d units, expected %d; first difference at %d: %v, expected %v", len(actual), len(expected), first,
					actual[first:min(first+3, len(actual))], expected[first:min(first+3, len(expected))])
			}
			builds++
			units += len(built)
		}
	}
	t.Logf("%d builds, %d units, %d differences; %d where the model reads past an array", builds, units, report.count, readPast)
	if builds < 16000 {
		t.Errorf("only %d builds", builds)
	}
}

func TestUnusualSentences(t *testing.T) {
	tables := loadTables(t)
	cases := loadCases(t)
	random := rand.New(rand.NewPCG(1, 2))
	for iteration := range 3000 {
		golden := cases[random.IntN(len(cases))]
		sentence := sentenceOf(golden.Input)
		end := sentence.End
		for _, tier := range [][]byte{sentence.Tier1, sentence.Tier2, sentence.Tier3, sentence.Tier4, sentence.Tier5} {
			for range random.IntN(end/2 + 1) {
				tier[1+random.IntN(end)] = byte(random.IntN(256))
			}
		}
		for range random.IntN(end/4 + 1) {
			cell := 1 + random.IntN(end)
			sentence.Durations[cell] = int16(random.IntN(65536) - 32768)
			sentence.Anchors[cell] = byte(random.IntN(256))
			sentence.PointIndex[cell] = int16(random.IntN(4) - 1)
		}
		for range random.IntN(4) {
			sentence.Points = append(sentence.Points, Point{Time: int32(random.Uint32()), Value: int16(random.IntN(65536) - 32768)})
			sentence.Events = append(sentence.Events, Event{Kind: []EventKind{SentenceEvent, WordEvent, MarkerEvent}[random.IntN(3)], Value: random.IntN(100)})
		}
		sentence.MultipleWords = random.IntN(2) == 0
		sentence.DurationStyle = random.IntN(2)
		transcribed := append([]byte{}, sentence.Transcribed...)
		for range random.IntN(len(transcribed)/2 + 1) {
			if len(transcribed) > 0 {
				transcribed[random.IntN(len(transcribed))] = byte(random.IntN(256))
			}
		}
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("iteration %d (%s): %v\n%s", iteration, golden.Name, recovered, debug.Stack())
				}
			}()
			AssignStress(tables, append([]byte{}, sentence.Tier1...), append([]byte{}, sentence.Tier3...), sentence.Start, end, sentence.MultipleWords)
			if analysis, kept := Analyze(tables, sentence); kept {
				BuildUnits(tables, analysis, transcribed, random.IntN(200)-40, random.IntN(2) == 0)
			}
		}()
	}
}

func TestDamagedAnalysisText(t *testing.T) {
	tables := loadTables(t)
	texts := []string{"", "]", "(", "(5", "(5,", "(,7)]#[", "\x1b/#[15(0,90)(0,95)]a[0]#[-20]\x1b/\x1bE ", "\x1b/#[15]a[10]\x10b[5]", "[[[", "a[", "a[x]"}
	for _, text := range texts {
		analysis := &Analysis{Start: 1, End: 2, Tier1: []byte{0, '#', 'a', 'a', edge}, Text: []byte(text)}
		for _, transcribed := range []string{"", "#a#", "Ra'tS$r", "?a?"} {
			units, _ := BuildUnits(tables, analysis, []byte(transcribed), 56, false)
			if len(units) < 2 || units[len(units)-1].Kind != BoundaryUnit {
				t.Errorf("%q, %q: units %v", text, transcribed, units)
			}
		}
	}
}
