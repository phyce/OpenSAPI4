package prosody_test

import (
	"testing"

	"github.com/phyce/OpenSAPI4/truvoice/prosody"
)

// TestPitch replays every recorded call of the context, the pitch's context, the pitch and its reset.
func TestPitch(t *testing.T) {
	data := loadTestData(t)
	plain := func(run func(stage *prosody.Stage)) replayer {
		return func(call *call) error {
			stage := data.load(call)
			run(stage)
			return compare(stage, call)
		}
	}
	counts := replay(t, map[string]replayer{
		"Prosody::LoadContext":     plain((*prosody.Stage).LoadContext),
		"Prosody::PitchContext":    plain((*prosody.Stage).PitchContext),
		"Prosody::AssignPitch":     plain((*prosody.Stage).AssignPitch),
		"Prosody::ResetPitchState": plain((*prosody.Stage).ResetPitch),
	})
	if counts["Prosody::AssignPitch"] == 0 {
		t.Error("no pitch recorded")
	}
	t.Logf("calls replayed: %v", counts)
}
