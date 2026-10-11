package whistler

import (
	"encoding/hex"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"sync"
	"testing"

	"github.com/phyce/OpenSAPI4/whistler/internal/conformance"
	"github.com/phyce/OpenSAPI4/whistler/text"
)

type attributeStep struct {
	Call       string   `json:"call"`
	ID         uint32   `json:"id"`
	Value      uint32   `json:"value"`
	Text       string   `json:"text"`
	Result     string   `json:"result"`
	Written    *uint32  `json:"written"`
	Attributes []string `json:"attributes"`
}

type attributeCase struct {
	Number   int             `json:"number"`
	Group    string          `json:"group"`
	Mode     string          `json:"mode"`
	Steps    []attributeStep `json:"steps"`
	Samples  int64           `json:"samples"`
	Digest   string          `json:"digest"`
	TextEnds []int64         `json:"textEnds"`
}

var attributeNames = map[Attribute]string{PitchAttribute: "pitch", SpeedAttribute: "speed", VolumeAttribute: "volume"}

// resultCode is the HRESULT the original returns where the Go engine returns err.
func resultCode(err error) string {
	switch {
	case err == nil:
		return "0x00000000"
	case errors.Is(err, ErrValueOutOfRange):
		return "0x8000FFFF"
	case errors.Is(err, ErrInvalidParameter):
		return "0x80070057"
	}
	return err.Error()
}

type textEndSink struct {
	*recorder
	ends []int64
}

func (sink *textEndSink) Event(event Event) error {
	if event.Kind == TextDone {
		sink.ends = append(sink.ends, event.Sample)
	}
	return sink.recorder.Event(event)
}

// splitThreads keeps the order of pitch changes and of speed and volume changes, which come from different
// threads in the original.
func splitThreads(attributes []string) ([]string, []string) {
	var pitch, speedVolume []string
	for _, attribute := range attributes {
		if attribute == "pitch" {
			pitch = append(pitch, attribute)
		} else {
			speedVolume = append(speedVolume, attribute)
		}
	}
	return pitch, speedVolume
}

func runAttributeCase(t *testing.T, data *Data, golden *attributeCase) ([]string, error) {
	mode, found := data.Mode(golden.Mode)
	if !found {
		return nil, fmt.Errorf("no mode %q", golden.Mode)
	}
	engine, err := data.NewEngine(mode)
	if err != nil {
		return nil, err
	}
	var changed []string
	engine.AttributeChanged = func(attribute Attribute) {
		changed = append(changed, attributeNames[attribute])
	}
	sink := &textEndSink{recorder: newRecorder()}
	var differences []string
	for index, step := range golden.Steps {
		changed = nil
		var value uint32
		var err error
		getting := false
		switch step.Call {
		case "PitchGet":
			value, getting = uint32(engine.Pitch()), true
		case "SpeedGet":
			value, getting = engine.Speed(), true
		case "VolumeGet":
			value, getting = engine.Volume(), true
		case "PitchSet":
			err = engine.SetPitch(uint16(step.Value))
		case "SpeedSet":
			err = engine.SetSpeed(step.Value)
		case "VolumeSet":
			engine.SetVolume(step.Value)
		case "DWORDGet":
			value, err = engine.Attribute(Attribute(step.ID))
			getting = err == nil
		case "DWORDSet":
			err = engine.SetAttribute(Attribute(step.ID), step.Value)
		case "Inject":
			engine.Inject(string(utf16Runes(decodeTexts(t, []string{step.Text})[0])))
		case "Speak":
			if err := engine.Speak(decodeTexts(t, []string{step.Text})[0], true, text.UnicodeText, sink); err != nil {
				return differences, err
			}
		default:
			return differences, fmt.Errorf("unknown call %q", step.Call)
		}
		describe := fmt.Sprintf("step %d %s id %#x value %#x", index, step.Call, step.ID, step.Value)
		if step.Call != "Speak" && resultCode(err) != step.Result {
			differences = append(differences, fmt.Sprintf("%s: result %s, expected %s", describe, resultCode(err), step.Result))
		}
		if getting && (step.Written == nil || *step.Written != value) {
			differences = append(differences, fmt.Sprintf("%s: read %d, expected %v", describe, value, step.Written))
		}
		equal := slices.Equal(changed, step.Attributes)
		if step.Call == "Speak" {
			pitch, speedVolume := splitThreads(changed)
			wantPitch, wantSpeedVolume := splitThreads(step.Attributes)
			equal = slices.Equal(pitch, wantPitch) && slices.Equal(speedVolume, wantSpeedVolume)
		}
		if !equal {
			differences = append(differences, fmt.Sprintf("%s: changed %v, expected %v", describe, changed, step.Attributes))
		}
	}
	if digest := hex.EncodeToString(sink.digest.Sum(nil)); sink.samples != golden.Samples || digest != golden.Digest {
		differences = append(differences, fmt.Sprintf("%d samples %s, expected %d %s", sink.samples, digest[:16], golden.Samples, golden.Digest[:16]))
	}
	if !slices.Equal(sink.ends, golden.TextEnds) {
		differences = append(differences, fmt.Sprintf("texts end at %v, expected %v", sink.ends, golden.TextEnds))
	}
	return differences, nil
}

func TestAttributes(t *testing.T) {
	data := loadData(t)
	cases := conformance.ReadLines[attributeCase](t, "golden", "attributes", "cases.jsonl.gz")
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
				outcomes[number].differences, outcomes[number].err = runAttributeCase(t, data, &cases[number])
			}
		})
	}
	for number := range cases {
		numbers <- number
	}
	close(numbers)
	group.Wait()
	failures, steps := 0, 0
	for number, outcome := range outcomes {
		golden := &cases[number]
		steps += len(golden.Steps)
		if outcome.err == nil && len(outcome.differences) == 0 {
			continue
		}
		failures++
		if failures <= 20 {
			t.Errorf("case %d (%s, %s):", golden.Number, golden.Group, golden.Mode)
			if outcome.err != nil {
				t.Errorf("  %v", outcome.err)
			}
			for _, difference := range outcome.differences {
				t.Errorf("  %s", difference)
			}
		}
	}
	t.Logf("%d cases, %d calls: %d different", len(cases), steps, failures)
}
