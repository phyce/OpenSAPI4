package prosody

import "testing"

// TestPitch replays every recorded call of the context, the pitch's context, the pitch and its reset.
func TestPitch(t *testing.T) {
	data := loadTestData(t)
	plain := func(run func(stage *Stage)) replayer {
		return func(call *call) error {
			stage := data.load(call)
			run(stage)
			return compare(stage, call)
		}
	}
	counts := replay(t, map[string]replayer{
		"Prosody::LoadContext":     plain((*Stage).LoadContext),
		"Prosody::PitchContext":    plain((*Stage).PitchContext),
		"Prosody::AssignPitch":     plain((*Stage).AssignPitch),
		"Prosody::ResetPitchState": plain((*Stage).ResetPitch),
	})
	if counts["Prosody::AssignPitch"] == 0 {
		t.Error("no pitch recorded")
	}
	t.Logf("calls replayed: %v", counts)
}
