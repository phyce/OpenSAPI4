package analyser

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/phyce/OpenSAPI4/whistler/datapack"
	"github.com/phyce/OpenSAPI4/whistler/internal/conformance"
	"github.com/phyce/OpenSAPI4/whistler/rules"
)

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

type packData struct {
	rules        *datapack.Rules
	dictionaries *datapack.Dictionaries
	prosody      *datapack.Prosody
}

func loadPack(t *testing.T) *packData {
	pack := conformance.Pack(t)
	data := &packData{}
	var err error
	if data.rules, err = pack.LoadRules(); err != nil {
		t.Fatal(err)
	}
	if data.dictionaries, err = pack.LoadDictionaries(); err != nil {
		t.Fatal(err)
	}
	if data.prosody, err = pack.LoadProsody(); err != nil {
		t.Fatal(err)
	}
	return data
}

func (data *packData) analyser(t *testing.T) *Analyser {
	analyser, err := New(data.rules, data.dictionaries, data.prosody)
	if err != nil {
		t.Fatal(err)
	}
	return analyser
}

func readState(read conformance.RuleRead) *State {
	anchors := make([]byte, len(read.Anchors))
	for index, count := range read.Anchors {
		anchors[index] = byte(count)
	}
	state := NewState(conformance.Bytes(read.Tier1), anchors, read.DurationStyle)
	copy(state.Tier(1)[read.End+1:], conformance.Bytes(read.Beyond))
	for _, anchor := range read.AnchorsBeyond {
		state.Anchors[anchor[0]] = byte(anchor[1])
	}
	return state
}

func stateHash(state *State) string {
	digest := sha256.New()
	for tier := range state.Tiers {
		digest.Write(state.Tiers[tier][:])
	}
	digest.Write(state.Anchors[:])
	return hex.EncodeToString(digest.Sum(nil))
}

func compareSnapshot(report *report, name string, state *State, golden *conformance.RuleSnapshot) bool {
	if !report.check(name, state.End == golden.End, "end %d, expected %d", state.End, golden.End) {
		return false
	}
	equal := true
	for number := 1; number <= 5; number++ {
		actual := state.Tier(number)[1 : state.End+1]
		expected := conformance.Bytes(golden.Tiers[number-1])
		equal = report.check(name, slices.Equal(actual, expected), "tier %d\n%q\nexpected\n%q", number, actual, expected) && equal
	}
	anchors := make([]int, state.End)
	for cell := range anchors {
		anchors[cell] = int(state.Anchors[cell+1])
	}
	return report.check(name, slices.Equal(anchors, golden.Anchors), "anchors %v, expected %v", anchors, golden.Anchors) && equal
}

func compareAnalysis(report *report, name string, state *State, golden *conformance.RuleAnalysis) {
	if !compareSnapshot(report, name, state, &golden.RuleSnapshot) {
		return
	}
	report.check(name, state.MultipleWords == (golden.SentenceFlag == 1), "sentence flag %t, expected %d", state.MultipleWords, golden.SentenceFlag)
	report.check(name, slices.Equal(state.Durations[1:state.End+1], golden.Durations), "durations %v, expected %v", state.Durations[1:state.End+1], golden.Durations)
	report.check(name, slices.Equal(state.PointIndex[1:state.End+1], golden.PointIndex), "point index %v, expected %v", state.PointIndex[1:state.End+1], golden.PointIndex)
	points := make([][3]int32, len(state.Points))
	for index, point := range state.Points {
		points[index] = [3]int32{point.Time, int32(point.Value), int32(point.Cell)}
	}
	report.check(name, slices.Equal(points, golden.Points), "points %v, expected %v", points, golden.Points)
}

func TestTranscriptionAndAnalysis(t *testing.T) {
	data := loadPack(t)
	texts := conformance.ReadLines[conformance.RuleText](t, "golden", "rules", "texts.jsonl.gz")
	report := &report{t: t}
	sentences, dropped, overflowed, modelErrors := 0, 0, 0, 0
	for _, text := range texts {
		analyser := data.analyser(t)
		for number, sentence := range text.Sentences {
			name := fmt.Sprintf("%s sentence %d", text.Name, number+1)
			sentences++
			state := readState(sentence.Read)
			transcribed := analyser.Transcribe(state)
			if sentence.Error != "" {
				modelErrors++
				analysed := analyser.Analyse(state)
				report.check(name, !transcribed || !analysed, "the model failed with %s, but the sentence went through", sentence.Error)
				continue
			}
			golden := sentence.Transcribed
			if !transcribed {
				dropped++
			}
			report.check(name, transcribed == (golden.Result == 0), "transcribed %t, result %d", transcribed, golden.Result)
			if compareSnapshot(report, name+" transcribed", state, &golden.RuleSnapshot) {
				report.check(name, stateHash(state) == golden.Hash, "the state after transcription differs past the sentence")
			}
			analysed := analyser.Analyse(state)
			if !analysed {
				overflowed++
			}
			report.check(name, analysed == (sentence.Analysis.Result == 0), "analysed %t, result %d", analysed, sentence.Analysis.Result)
			compareAnalysis(report, name+" analysed", state, sentence.Analysis)
			report.check(name, stateHash(state) == sentence.Analysis.Hash, "the state after analysis differs past the sentence")
		}
	}
	t.Logf("%d texts, %d sentences: %d not transcribed, %d overflowing module A, %d where the model fails", len(texts), sentences, dropped, overflowed, modelErrors)
}

func TestTranscribedCopy(t *testing.T) {
	data := loadPack(t)
	analyser := data.analyser(t)
	state := NewState([]byte("hello "), nil, 1)
	if !analyser.Transcribe(state) {
		t.Fatal("not transcribed")
	}
	if string(state.Transcribed) != string(state.Tier(1)[1:state.End+1]) || len(state.Transcribed) != state.End {
		t.Errorf("transcribed %q, tier 1 %q", state.Transcribed, state.Tier(1)[1:state.End+1])
	}
	if state.Tier(1)[0] != 0 || state.Tier(1)[rules.TailStart] != 0 {
		t.Errorf("cell 0 or the cell at the tail's start changed")
	}
}

// unusualText builds tier 1 the reader could leave: bytes 0x0e to 0xaf, with markers, annotations and
// symbols the rules treat specially, at any length up to the tiers' limit.
func unusualText(random *rand.Rand) []byte {
	pieces := [][]byte{
		[]byte("hello "), []byte("the "), []byte("1234 "), []byte("Dr. "), []byte("U.S.A. "), []byte("can't "), []byte("(s) "),
		[]byte("$5.25 "), []byte("3:45 "), []byte("w "), []byte("? "), []byte(", "), []byte("\x11@N "), []byte("\x11@V "),
		[]byte("\x11\"a "), []byte(" \x10 'h$-lO \x10 "), []byte("[+30]"), []byte("[-50]"), []byte("[x9]"), []byte("[200]"),
		[]byte("[\x1e10\x1d150\x1f]"), []byte("[i1]"), []byte("[\x1e-5\x1d-9\x1f\x1d40]"), []byte("[]"), []byte("~ "),
		[]byte("\x0e"), []byte("\x19"), []byte("\x16"), []byte("# "), []byte("\x91"), []byte("\xa4\x81 "), []byte("Abc-def "),
	}
	length := random.IntN(1280) + 1
	if random.IntN(4) == 0 {
		length = random.IntN(40) + 1
	}
	text := []byte{}
	for len(text) < length {
		if random.IntN(3) == 0 {
			text = append(text, byte(0x0E+random.IntN(0xAF-0x0E+1)))
		} else {
			text = append(text, pieces[random.IntN(len(pieces))]...)
		}
	}
	return text[:length]
}

func TestUnusualSentences(t *testing.T) {
	data := loadPack(t)
	random := rand.New(rand.NewPCG(5, 17))
	var analyser *Analyser
	dropped, overflowed := 0, 0
	for number := range 3000 {
		if number%50 == 0 {
			analyser = data.analyser(t)
		}
		text := unusualText(random)
		anchors := make([]byte, len(text))
		for range random.IntN(4) {
			anchors[random.IntN(len(anchors))] = byte(random.IntN(3) + 1)
		}
		state := NewState(text, anchors, random.IntN(2))
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("sentence %d %q: %v", number, text, recovered)
				}
			}()
			if !analyser.Transcribe(state) {
				dropped++
			}
			if !analyser.Analyse(state) {
				overflowed++
			}
		}()
		if state.End < 0 || state.End >= rules.Cells {
			t.Fatalf("sentence %d: end %d", number, state.End)
		}
	}
	t.Logf("3000 sentences: %d not transcribed, %d overflowing module A", dropped, overflowed)
}
