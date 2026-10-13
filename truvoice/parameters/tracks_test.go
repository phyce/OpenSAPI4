package parameters

import (
	"fmt"
	"testing"
)

// TestTracks replays every recorded call of the track renderer, the ring writers the stage's actions call, the
// advance after a phoneme and the ring's room and rebase.
func TestTracks(t *testing.T) {
	data := loadTestData(t)
	onRing := func(write func(ring Ring, call *call) error) replayer {
		return func(call *call) error {
			track, err := ringOf(call, 0)
			if err != nil {
				return err
			}
			generator := data.load(call)
			if err := write(generator.Ring(track), call); err != nil {
				return err
			}
			return compare(generator, call)
		}
	}
	counts := replay(t, map[string]replayer{
		"RenderTrackSegment": func(call *call) error {
			generator := data.load(call)
			generator.RenderSegment(int(call.Argument(0)))
			return compare(generator, call)
		},
		"ShapeFill": onRing(func(ring Ring, call *call) error {
			ring.Fill(call.Argument(1), call.Argument(2), call.Argument(3), byte(call.Argument(4)), byte(call.Argument(5)))
			return nil
		}),
		"ShapePullBackward": onRing(func(ring Ring, call *call) error {
			ring.PullBack(call.Argument(1), call.Argument(2), call.Argument(3), byte(call.Argument(4)))
			return nil
		}),
		"ShapePullForward": onRing(func(ring Ring, call *call) error {
			ring.PullForward(call.Argument(1), call.Argument(2), call.Argument(3), byte(call.Argument(4)))
			return nil
		}),
		"LinearRamp": onRing(func(ring Ring, call *call) error {
			ring.Ramp(call.Argument(1), call.Argument(2), byte(call.Argument(3)), byte(call.Argument(4)))
			return nil
		}),
		"FillFrames": onRing(func(ring Ring, call *call) error {
			ring.Repeat(call.Argument(1), call.Argument(2), byte(call.Argument(3)))
			return nil
		}),
		"ShapeAddBackward": onRing(func(ring Ring, call *call) error {
			if int(call.Argument(1)) != ring.track {
				return fmt.Errorf("the track argument is %d, the ring's track %d", call.Argument(1), ring.track)
			}
			ring.AddBack(call.Argument(2), call.Argument(3), call.Argument(4), call.Argument(5))
			return nil
		}),
		"Parameters::AdvanceTracks": func(call *call) error {
			generator := data.load(call)
			generator.Advance()
			return compare(generator, call)
		},
		"Frames::Control": func(call *call) error {
			generator := data.load(call)
			var result bool
			switch call.Argument(0) {
			case 1:
				result = generator.Room(call.Argument(1))
			case 2:
				generator.Rebase()
				result = true
			default:
				return fmt.Errorf("operation %d", call.Argument(0))
			}
			if expected := call.Result.EAX&0xff != 0; result != expected {
				return fmt.Errorf("returned %t, expected %t", result, expected)
			}
			return compare(generator, call)
		},
	})
	t.Logf("calls replayed: %v", counts)
}
