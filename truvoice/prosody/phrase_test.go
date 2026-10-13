package prosody

import (
	"fmt"
	"testing"

	"github.com/phyce/OpenSAPI4/truvoice/internal/threadstate"
)

// TestPhrase replays every recorded call of the phrase's scan, its breaks and the stage's reset.
func TestPhrase(t *testing.T) {
	data := loadTestData(t)
	counts := replay(t, map[string]replayer{
		"Prosody::Reset": func(call *call) error {
			stage := data.load(call)
			stage.Reset()
			stage.cursor().Types = 0x1c
			return compare(stage, call)
		},
		"Prosody::ScanPhrase": func(call *call) error {
			stage := data.load(call)
			result := stage.ScanPhrase()
			if err := compare(stage, call); err != nil {
				return err
			}
			return expectResult(int32(call.Result.EAX), result)
		},
		"Prosody::PlacePhraseBreaks": func(call *call) error {
			stage := data.load(call)
			stage.PlacePhraseBreaks(byte(call.Argument(0)))
			return compare(stage, call)
		},
		"Prosody::InsertPhraseBreak": func(call *call) error {
			stage := data.load(call)
			stage.InsertPhraseBreak(call.Argument(0), byte(call.Argument(1)), call.Argument(2))
			return compare(stage, call)
		},
		"Prosody::AdjustAtBreak": func(call *call) error {
			stage := data.load(call)
			mapping := threadstate.Mapping{Pipeline: stage.Pipeline, Base: call.Base}
			stage.AdjustAtBreak(mapping.NodeAt(call.Record.Arguments[0]), call.Argument(1) != 0)
			return compare(stage, call)
		},
	})
	if counts["Prosody::ScanPhrase"] == 0 {
		t.Error("no scan recorded")
	}
	t.Logf("calls replayed: %v", counts)
}

// TestWalk replays every recorded call of the stage's run, its walk, its nodes, their presets and pauses, with
// the durations the original gave.
func TestWalk(t *testing.T) {
	data := loadTestData(t)
	withDurations := func(run func(stage *Stage, call *call) error) replayer {
		return func(call *call) error {
			stage := data.load(call)
			recorded := takeDurations(stage, call)
			resultError := run(stage, call)
			if err := recorded.check(stage); err != nil {
				return err
			}
			return resultError
		}
	}
	counts := replay(t, map[string]replayer{
		"Prosody::NextNode": func(call *call) error {
			stage := data.load(call)
			result := stage.NextNode()
			if err := compare(stage, call); err != nil {
				return err
			}
			return expectResult(int32(call.Result.EAX), int32(result))
		},
		"Prosody::ApplyPresets": func(call *call) error {
			stage := data.load(call)
			stage.ApplyPresets(call.Argument(0) != 0)
			return compare(stage, call)
		},
		"Prosody::InsertPause": func(call *call) error {
			stage := data.load(call)
			stage.InsertPause(call.Argument(0))
			return compare(stage, call)
		},
		"Prosody::DurationAndPitch": withDurations(func(stage *Stage, call *call) error {
			stage.DurationAndPitch()
			return nil
		}),
		"Prosody::ProcessPhoneme": withDurations(func(stage *Stage, call *call) error {
			stage.ProcessPhoneme()
			return nil
		}),
		"Prosody::Run": withDurations(func(stage *Stage, call *call) error {
			handed := stage.Run()
			return expectResult(int32(call.Result.EAX&0xff), map[bool]int32{false: 0, true: 1}[handed])
		}),
	})
	if counts["Prosody::Run"] == 0 {
		t.Error("no run recorded")
	}
	t.Logf("calls replayed: %v", counts)
}

func expectResult(expected, result int32) error {
	if result != expected {
		return fmt.Errorf("result %d, expected %d", result, expected)
	}
	return nil
}
