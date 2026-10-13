package parameters

import (
	"fmt"
	"slices"
	"testing"

	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
)

// pointerOf returns a node's pointer in the thread state.
func pointerOf(node *pipeline.Node, base uint32) uint32 {
	if node == nil {
		return 0
	}
	return base + uint32(nodeOffsets()[node.Index()])
}

// TestStep replays every recorded call of the stage's step, its lookahead and pauses, and its generated segments.
func TestStep(t *testing.T) {
	data := loadTestData(t)
	counts := replay(t, map[string]replayer{
		"Parameters::Step": func(call *call) error {
			generator := data.load(call)
			if result, expected := generator.Step(), int(call.result.EAX&0xff); result != expected {
				return fmt.Errorf("returned %d, expected %d", result, expected)
			}
			image := slices.Clone(call.before)
			copy(image[stateCurrent:stateCurrent+4], call.after[stateCurrent:])
			return difference(generator, image, call.after, call.base)
		},
		"Parameters::NextNode": func(call *call) error {
			generator := data.load(call)
			if result, expected := pointerOf(generator.lookAhead(), call.base), call.result.EAX; result != expected {
				return fmt.Errorf("returned %08x, expected %08x", result, expected)
			}
			return compare(generator, call)
		},
		"Parameters::InsertPausePair": func(call *call) error {
			generator := data.load(call)
			if call.argument(1) != generator.Ahead {
				return fmt.Errorf("called for %d with %d ahead", call.argument(1), generator.Ahead)
			}
			at := nodeAt(generator.Pipeline, uint32(call.argument(0)), call.base)
			if result, expected := pointerOf(generator.insertPauses(at), call.base), call.result.EAX; result != expected {
				return fmt.Errorf("returned %08x, expected %08x", result, expected)
			}
			return compare(generator, call)
		},
		"Parameters::RenderGenerated": func(call *call) error {
			generator := data.load(call)
			if result, expected := generator.RenderGenerated(), int32(call.result.EAX); result != expected {
				return fmt.Errorf("returned %d, expected %d", result, expected)
			}
			return compare(generator, call)
		},
		"Parameters::FillTemplateFrames": func(call *call) error {
			generator := data.load(call)
			if byte(call.argument(0)) != generator.SegmentSymbol || call.argument(1) != generator.SegmentLeft || (byte(call.argument(3)) != 0) != generator.SegmentStarting {
				return fmt.Errorf("called with %v, the segment is %q, %d left, starting %t", call.record.Arguments, generator.SegmentSymbol, generator.SegmentLeft, generator.SegmentStarting)
			}
			if result, expected := generator.FillSegment(call.argument(2)), int32(call.result.EAX); result != expected {
				return fmt.Errorf("returned %d, expected %d", result, expected)
			}
			return compare(generator, call)
		},
	})
	t.Logf("calls replayed: %v", counts)
}
