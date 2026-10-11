package whistler

import (
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"testing"

	"github.com/phyce/OpenSAPI4/whistler/internal/conformance"
	"github.com/phyce/OpenSAPI4/whistler/text"
)

type resetCase struct {
	Number          int               `json:"number"`
	Group           string            `json:"group"`
	Mode            string            `json:"mode"`
	Attributes      map[string]uint32 `json:"attributes"`
	Accept          [][2]int          `json:"accept"`
	Before          []string          `json:"before"`
	Text            string            `json:"text"`
	Limit           int64             `json:"limit"`
	After           []string          `json:"after"`
	Stopped         bool              `json:"stopped"`
	Samples         int64             `json:"samples"`
	Digest          string            `json:"digest"`
	Blocks          []string          `json:"blocks"`
	Events          []json.RawMessage `json:"events"`
	AttributeEvents struct {
		Pitch       []string `json:"pitch"`
		SpeedVolume []string `json:"speedVolume"`
	} `json:"attributeEvents"`
	AttributesAfter struct {
		Pitch  uint16 `json:"pitch"`
		Speed  uint32 `json:"speed"`
		Volume uint32 `json:"volume"`
	} `json:"attributesAfter"`
}

func runResetCase(data *Data, golden *resetCase, before [][]uint16, stopped []uint16, after [][]uint16) ([]string, error) {
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
	for _, characters := range before {
		if err := engine.Speak(characters, true, text.CharacterSet(0), recorder); err != nil {
			return nil, err
		}
	}
	sink := &stoppingSink{recorder: recorder, limit: recorder.samples + golden.Limit}
	err = engine.Speak(stopped, true, text.CharacterSet(0), sink)
	var differences []string
	switch {
	case errors.Is(err, errStopped):
		recorder.events = append(recorder.events, canonical([]any{recorder.samples, "aborted", len(before)}))
	case err != nil:
		return nil, err
	}
	if (err != nil) != golden.Stopped {
		differences = append(differences, fmt.Sprintf("the text returned %v, expected stopped %t", err, golden.Stopped))
	}
	for _, characters := range after {
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

func TestReset(t *testing.T) {
	data := loadData(t)
	cases := conformance.ReadLines[resetCase](t, "golden", "reset", "cases.jsonl.gz")
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
				before := decodeTexts(t, golden.Before)
				after := decodeTexts(t, golden.After)
				stopped := decodeTexts(t, []string{golden.Text})[0]
				outcomes[number].differences, outcomes[number].err = runResetCase(data, golden, before, stopped, after)
			}
		})
	}
	for number := range cases {
		numbers <- number
	}
	close(numbers)
	group.Wait()
	failures, stopped := 0, 0
	for number, outcome := range outcomes {
		golden := &cases[number]
		if golden.Stopped {
			stopped++
		}
		if outcome.err == nil && len(outcome.differences) == 0 {
			continue
		}
		failures++
		if failures <= 20 {
			t.Errorf("case %d (%s, %s, %v, limit %d): %q", golden.Number, golden.Group, golden.Mode, golden.Attributes, golden.Limit,
				string(utf16Runes(decodeTexts(t, []string{golden.Text})[0])))
			if outcome.err != nil {
				t.Errorf("  %v", outcome.err)
			}
			for _, difference := range outcome.differences {
				t.Errorf("  %s", difference)
			}
		}
	}
	t.Logf("%d cases (%d stopped): %d different", len(cases), stopped, failures)
}
