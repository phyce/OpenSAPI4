package whistler

import (
	"errors"
	"fmt"
	"math"
	"runtime"
	"slices"
	"sync"
	"testing"

	"github.com/phyce/OpenSAPI4/whistler/internal/conformance"
	"github.com/phyce/OpenSAPI4/whistler/text"
)

type pauseCall struct {
	Call  string `json:"call"`
	Value uint32 `json:"value"`
	Text  string `json:"text"`
}

type callResult struct {
	Result string  `json:"result"`
	Value  *uint32 `json:"value"`
}

type pauseCase struct {
	resetCase
	Texts   []string     `json:"texts"`
	Paused  bool         `json:"paused"`
	Calls   []pauseCall  `json:"calls"`
	During  []string     `json:"during"`
	End     string       `json:"end"`
	Later   []string     `json:"later"`
	Results []callResult `json:"results"`
	Second  *secondPause `json:"second"`
}

// secondPause is a pause again after the first pause's calls: the sink takes Limit samples in all.
type secondPause struct {
	Limit  int64       `json:"limit"`
	Paused bool        `json:"paused"`
	Calls  []pauseCall `json:"calls"`
}

// pausingSink takes limit samples in all and pauses the engine in the Audio call that would go past them,
// having taken what fits; with every set, it also pauses at every every-th event.
type pausingSink struct {
	recorder *recorder
	limit    int64
	every    int
	events   int
	done     int
}

func (sink *pausingSink) Audio(data []byte) (int, error) {
	room := int(min(int64(len(data)), max(sink.limit-sink.recorder.samples, 0)*int64(sink.recorder.sampleSize)))
	taken, _ := sink.recorder.Audio(data[:room])
	if taken < len(data) {
		return taken, ErrPaused
	}
	return taken, nil
}

func (sink *pausingSink) Event(event Event) error {
	if event.Kind == TextDone {
		sink.done++
	}
	if err := sink.recorder.Event(event); err != nil {
		return err
	}
	sink.events++
	if sink.every > 0 && sink.events%sink.every == 0 {
		return ErrPaused
	}
	return nil
}

func makeCall(engine *Engine, call pauseCall) (error, uint32) {
	switch call.Call {
	case "PitchSet":
		return engine.SetPitch(uint16(call.Value)), 0
	case "SpeedSet":
		return engine.SetSpeed(call.Value), 0
	case "VolumeSet":
		engine.SetVolume(call.Value)
	case "PitchGet":
		return nil, uint32(engine.Pitch())
	case "SpeedGet":
		return nil, engine.Speed()
	case "VolumeGet":
		return nil, engine.Volume()
	case "Inject":
		engine.Inject(call.Text)
	default:
		return fmt.Errorf("call %q", call.Call), 0
	}
	return nil, 0
}

func runPauseCase(data *Data, golden *pauseCase, texts, during, later [][]uint16) ([]string, error) {
	mode, found := data.Mode(golden.Mode)
	if !found {
		return nil, fmt.Errorf("no mode %q", golden.Mode)
	}
	engine, err := data.NewEngineAccepting(mode, accepting(golden.Accept))
	if err != nil {
		return nil, err
	}
	recorder := newRecorder()
	recorder.sampleSize = engine.Format().Bits / 8
	engine.AttributeChanged = recorder.attributeChanged
	if pitch, found := golden.Attributes["pitch"]; found {
		engine.SetPitch(uint16(pitch))
	}
	if speed, found := golden.Attributes["speed"]; found {
		engine.SetSpeed(speed)
	}
	if volume, found := golden.Attributes["volume"]; found {
		engine.SetVolume(volume)
	}
	var differences []string
	sink := &pausingSink{recorder: recorder, limit: golden.Limit}
	for number, characters := range texts {
		err := engine.Speak(characters, true, text.CharacterSet(0), sink)
		switch {
		case errors.Is(err, ErrPaused):
		case err != nil:
			return nil, err
		case engine.Paused():
			differences = append(differences, fmt.Sprintf("text %d was spoken during the pause", number))
		}
	}
	if engine.Paused() != golden.Paused {
		differences = append(differences, fmt.Sprintf("paused by the sink %t, expected %t", engine.Paused(), golden.Paused))
	}
	if !engine.Paused() {
		if err := engine.Pause(); err != nil {
			return nil, err
		}
	}
	if err := engine.Pause(); !errors.Is(err, ErrAlreadyPaused) {
		differences = append(differences, fmt.Sprintf("pausing twice gave %v", err))
	}
	makeCalls := func(calls []pauseCall, results []callResult) {
		for index, call := range calls {
			err, value := makeCall(engine, call)
			want := results[index]
			if resultCode(err) != want.Result || want.Value != nil && value != *want.Value {
				wanted := "nothing"
				if want.Value != nil {
					wanted = fmt.Sprint(*want.Value)
				}
				differences = append(differences, fmt.Sprintf("call %d %+v gave %s and %d, expected %s and %s", index, call, resultCode(err), value,
					want.Result, wanted))
			}
		}
	}
	makeCalls(golden.Calls, golden.Results)
	for number, characters := range during {
		if err := engine.Speak(characters, true, text.CharacterSet(0), sink); !errors.Is(err, ErrPaused) {
			differences = append(differences, fmt.Sprintf("text %d queued during the pause gave %v", number, err))
		}
	}
	if second := golden.Second; second != nil {
		sink.limit = second.Limit
		if err := engine.Resume(sink); err != nil && !errors.Is(err, ErrPaused) {
			return nil, err
		}
		if engine.Paused() != second.Paused {
			differences = append(differences, fmt.Sprintf("paused again by the sink %t, expected %t", engine.Paused(), second.Paused))
		}
		if !engine.Paused() {
			if err := engine.Pause(); err != nil {
				return nil, err
			}
		}
		makeCalls(second.Calls, golden.Results[len(golden.Calls):])
	}
	switch golden.End {
	case "resume":
		sink.limit = math.MaxInt32
		if err := engine.Resume(sink); err != nil {
			return nil, err
		}
	case "reset":
		engine.Reset()
		for number := sink.done; number < len(texts)+len(during); number++ {
			recorder.events = append(recorder.events, canonical([]any{recorder.samples, "aborted", number}))
		}
	}
	if engine.Paused() {
		differences = append(differences, "still paused")
	}
	if err := engine.Resume(recorder); !errors.Is(err, ErrNotPaused) {
		differences = append(differences, fmt.Sprintf("resuming an engine that isn't paused gave %v", err))
	}
	for _, characters := range later {
		if err := engine.Speak(characters, true, text.CharacterSet(0), recorder); err != nil {
			return nil, err
		}
	}
	if recorder.blockLength > 0 {
		recorder.endBlock()
	}
	expected := &goldenCase{Samples: golden.Samples, Digest: golden.Digest, Blocks: golden.Blocks, Events: golden.Events, AttributeEvents: golden.AttributeEvents}
	differences = append(differences, compareCase(expected, recorder)...)
	volume := engine.Volume()
	if want := golden.AttributesAfter; engine.Pitch() != want.Pitch || engine.Speed() != want.Speed || volume != want.Volume {
		differences = append(differences, fmt.Sprintf("pitch, speed and volume %d %d %#x, expected %d %d %#x", engine.Pitch(), engine.Speed(), volume,
			want.Pitch, want.Speed, want.Volume))
	}
	return differences, nil
}

func TestPause(t *testing.T) {
	data := loadData(t)
	cases := conformance.ReadLines[pauseCase](t, "golden", "pause", "cases.jsonl.gz")
	type outcome struct {
		differences []string
		err         error
	}
	outcomes := make([]outcome, len(cases))
	numbers := make(chan int)
	var group sync.WaitGroup
	for range runtime.GOMAXPROCS(0) {
		group.Go(func() {
			for number := range numbers {
				golden := &cases[number]
				texts, during, later := decodeTexts(t, golden.Texts), decodeTexts(t, golden.During), decodeTexts(t, golden.Later)
				outcomes[number].differences, outcomes[number].err = runPauseCase(data, golden, texts, during, later)
			}
		})
	}
	for number := range cases {
		numbers <- number
	}
	close(numbers)
	group.Wait()
	failures, paused, again, reset := 0, 0, 0, 0
	for number, outcome := range outcomes {
		golden := &cases[number]
		if golden.Paused {
			paused++
		}
		if golden.Second != nil && golden.Second.Paused {
			again++
		}
		if golden.End == "reset" {
			reset++
		}
		if outcome.err == nil && len(outcome.differences) == 0 {
			continue
		}
		failures++
		if failures <= 20 {
			t.Errorf("case %d (%s, %s, %v, limit %d, %s, calls %+v)", golden.Number, golden.Group, golden.Mode, golden.Attributes, golden.Limit,
				golden.End, golden.Calls)
			if outcome.err != nil {
				t.Errorf("  %v", outcome.err)
			}
			for _, difference := range outcome.differences {
				t.Errorf("  %s", difference)
			}
		}
	}
	t.Logf("%d cases (%d paused by the sink, %d paused again, %d reset): %d different", len(cases), paused, again, reset, failures)
}

// TestPauseAnywhere pauses and resumes texts at many places, by the samples and by the events, with texts
// queued during the pauses: with no calls made, the sink gets what it gets without pausing.
func TestPauseAnywhere(t *testing.T) {
	data := loadData(t)
	texts := []string{
		`One two three \Spd=220\ four five. \Vol=40000\ Six \Mrk=7\ seven \Pau=300\ eight. \Pit=200\ Nine ten. \Chr="Whisper"\ Eleven.`,
		`Twelve \Rst\ thirteen. \Pau=50\`,
		`Fourteen fifteen.`,
	}
	speak := func(engine *Engine, step int64, every int) *recorder {
		pausing := &pausingSink{recorder: newRecorder(), limit: step, every: every}
		if step == 0 {
			pausing.limit = math.MaxInt32
		}
		resume := func(err error) {
			for errors.Is(err, ErrPaused) {
				if step > 0 {
					pausing.limit = pausing.recorder.samples + step
				}
				err = engine.Resume(pausing)
			}
			if err != nil {
				t.Fatal(err)
			}
		}
		err := engine.SpeakString(texts[0], pausing)
		if errors.Is(err, ErrPaused) {
			for _, queued := range texts[1:] {
				if err := engine.SpeakString(queued, pausing); !errors.Is(err, ErrPaused) {
					t.Fatalf("a text queued during a pause gave %v", err)
				}
			}
			resume(err)
		} else {
			resume(err)
			if err := engine.Pause(); err != nil {
				t.Fatal(err)
			}
			for _, queued := range texts[1:] {
				if err := engine.SpeakString(queued, pausing); !errors.Is(err, ErrPaused) {
					t.Fatalf("a text queued during a pause gave %v", err)
				}
			}
			resume(ErrPaused)
		}
		return pausing.recorder
	}
	mode, _ := data.Mode("Mary")
	newEngine := func() *Engine {
		engine, err := data.NewEngine(mode)
		if err != nil {
			t.Fatal(err)
		}
		return engine
	}
	expected := newRecorder()
	engine := newEngine()
	for _, value := range texts {
		if err := engine.SpeakString(value, expected); err != nil {
			t.Fatal(err)
		}
	}
	for _, every := range []int{0, 1, 3, 17} {
		for _, step := range []int64{0, 1, 97, 1000, 4096, 20000, 1 << 20} {
			if step == 0 && every == 0 {
				continue
			}
			got := speak(newEngine(), step, every)
			if got.samples != expected.samples || string(got.digest.Sum(nil)) != string(expected.digest.Sum(nil)) || !slices.Equal(got.events, expected.events) {
				t.Errorf("pausing every %d samples and every %d events: %d samples and %d events, expected %d and %d", step, every, got.samples,
					len(got.events), expected.samples, len(expected.events))
			}
		}
	}
	engine = newEngine()
	if err := engine.Resume(expected); !errors.Is(err, ErrNotPaused) {
		t.Errorf("resuming an engine that isn't paused gave %v", err)
	}
	engine.Reset()
	if err := engine.Pause(); err != nil {
		t.Fatal(err)
	}
	if err := engine.Pause(); !errors.Is(err, ErrAlreadyPaused) {
		t.Errorf("pausing twice gave %v", err)
	}
}
