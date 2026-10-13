package parameters

import (
	"fmt"
	"slices"
	"testing"

	"github.com/phyce/OpenSAPI4/truvoice/internal/threadstate"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
)

// pointerOf returns a node's pointer in the thread state.
func pointerOf(node *pipeline.Node, base uint32) uint32 {
	return threadstate.Mapping{Base: base}.PointerTo(node)
}

// TestStep replays every recorded call of the stage's step, its lookahead and pauses, and its generated segments.
func TestStep(t *testing.T) {
	data := loadTestData(t)
	counts := replay(t, map[string]replayer{
		"Parameters::Step": func(call *call) error {
			generator := data.load(call)
			if result, expected := generator.Step(), int(call.Result.EAX&0xff); result != expected {
				return fmt.Errorf("returned %d, expected %d", result, expected)
			}
			image := slices.Clone(call.Before)
			copy(image[threadstate.Current:threadstate.Current+4], call.After[threadstate.Current:])
			return difference(generator, image, call.After, call.Base)
		},
		"Parameters::NextNode": func(call *call) error {
			generator := data.load(call)
			if result, expected := pointerOf(generator.lookAhead(), call.Base), call.Result.EAX; result != expected {
				return fmt.Errorf("returned %08x, expected %08x", result, expected)
			}
			return compare(generator, call)
		},
		"Parameters::InsertPausePair": func(call *call) error {
			generator := data.load(call)
			if call.Argument(1) != generator.Ahead {
				return fmt.Errorf("called for %d with %d ahead", call.Argument(1), generator.Ahead)
			}
			at := threadstate.Mapping{Pipeline: generator.Pipeline, Base: call.Base}.NodeAt(uint32(call.Argument(0)))
			if result, expected := pointerOf(generator.insertPauses(at), call.Base), call.Result.EAX; result != expected {
				return fmt.Errorf("returned %08x, expected %08x", result, expected)
			}
			return compare(generator, call)
		},
		"Parameters::RenderGenerated": func(call *call) error {
			generator := data.load(call)
			if result, expected := generator.RenderGenerated(), int32(call.Result.EAX); result != expected {
				return fmt.Errorf("returned %d, expected %d", result, expected)
			}
			return compare(generator, call)
		},
		"Parameters::FillTemplateFrames": func(call *call) error {
			generator := data.load(call)
			if byte(call.Argument(0)) != generator.SegmentSymbol || call.Argument(1) != generator.SegmentLeft || (byte(call.Argument(3)) != 0) != generator.SegmentStarting {
				return fmt.Errorf("called with %v, the segment is %q, %d left, starting %t", call.Record.Arguments, generator.SegmentSymbol, generator.SegmentLeft, generator.SegmentStarting)
			}
			if result, expected := generator.FillSegment(call.Argument(2)), int32(call.Result.EAX); result != expected {
				return fmt.Errorf("returned %d, expected %d", result, expected)
			}
			return compare(generator, call)
		},
	})
	t.Logf("calls replayed: %v", counts)
}
