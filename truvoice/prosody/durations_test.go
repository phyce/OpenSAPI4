package prosody_test

import (
	"testing"

	"github.com/phyce/OpenSAPI4/truvoice/prosody"
)

// TestDurations replays every recorded call of the durations and their parts.
func TestDurations(t *testing.T) {
	data := loadTestData(t)
	withState := func(run func(stage *prosody.Stage, call *call) error) replayer {
		return func(call *call) error {
			stage := data.load(call)
			resultError := run(stage, call)
			if err := compare(stage, call); err != nil {
				return err
			}
			return resultError
		}
	}
	result := func(duration func(stage *prosody.Stage, call *call) int32) replayer {
		return withState(func(stage *prosody.Stage, call *call) error {
			return expectResult(int32(call.Result.EAX), duration(stage, call))
		})
	}
	counts := replayDurations(t, map[string]replayer{
		"Prosody::AssignDuration": withState(func(stage *prosody.Stage, call *call) error {
			stage.AssignDuration()
			return nil
		}),
		"Prosody::MarkerDuration": withState(func(stage *prosody.Stage, call *call) error {
			stage.MarkerDuration()
			return nil
		}),
		"Prosody::SimpleDuration": result(func(stage *prosody.Stage, call *call) int32 {
			return stage.SimpleDuration()
		}),
		"Prosody::KlattBaseDuration": result(func(stage *prosody.Stage, call *call) int32 {
			return stage.KlattBaseDuration()
		}),
		"Prosody::SegmentDuration": result(func(stage *prosody.Stage, call *call) int32 {
			return stage.SegmentDuration()
		}),
		"Prosody::MergeGeminate": withState(func(stage *prosody.Stage, call *call) error {
			merged := stage.MergeGeminate()
			return expectResult(int32(call.Result.EAX&0xff), map[bool]int32{false: 0, true: 1}[merged])
		}),
		"Prosody::PauseDuration": result(func(stage *prosody.Stage, call *call) int32 {
			return stage.PauseDuration()
		}),
		"Prosody::EmphasisDuration": result(func(stage *prosody.Stage, call *call) int32 {
			return stage.EmphasisDuration(call.Argument(0))
		}),
		"Prosody::RateScale": result(func(stage *prosody.Stage, call *call) int32 {
			return stage.RateScale(call.Argument(0))
		}),
		"Prosody::VowelDuration": result(func(stage *prosody.Stage, call *call) int32 {
			return stage.VowelDuration()
		}),
		"Prosody::SonorantDuration": result(func(stage *prosody.Stage, call *call) int32 {
			return stage.SonorantDuration()
		}),
		"Prosody::FricativeDuration": result(func(stage *prosody.Stage, call *call) int32 {
			return stage.FricativeDuration()
		}),
		"Prosody::StopDuration": result(func(stage *prosody.Stage, call *call) int32 {
			return stage.StopDuration()
		}),
		"Prosody::FunctionWordVowelRule": result(func(stage *prosody.Stage, call *call) int32 {
			return stage.FunctionWordVowelRule(byte(call.Argument(0)), call.Argument(1) != 0, byte(call.Argument(2)) != 0)
		}),
	})
	if counts["Prosody::AssignDuration"] == 0 {
		t.Error("no duration recorded")
	}
	t.Logf("calls replayed: %v", counts)
}
