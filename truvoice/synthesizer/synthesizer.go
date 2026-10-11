package synthesizer

import "github.com/phyce/OpenSAPI4/truvoice/datapack"

const (
	// RingLength is the number of frames the ring between the parameter generator and the synthesizer holds.
	RingLength = 256
	// HandOverBytes is how full the audio buffer gets before the synthesizer stops to hand it over.
	HandOverBytes = 12000
	// RebaseFrames is how far the frame counters move back once the position passes it.
	RebaseFrames  = 2048
	initialFrames = 12
	initialEnd    = 10
	noEnd         = -1
)

// Synthesizer is the engine's last stage (synthesizer.md §5): it converts the frames of the ring and renders
// them, one frame per step. The parameter generator writes the frames, Written, End, Lookahead and Done;
// Samples is the audio buffer the engine hands over.
type Synthesizer struct {
	Frames  [RingLength]Frame
	Markers [RingLength]bool
	Tones   [RingLength]bool

	Position  int32
	End       int32
	Written   int32
	Lookahead int32

	Done          bool
	Hold          bool
	Ready         bool
	Rendering     bool
	Mute          bool
	MarkersPassed int32
	Gain          uint16
	OutputVoice   int
	Samples       []int16

	converter *Converter
	renderer  *Renderer
}

// New returns a synthesizer for the sample rate, reset, with an empty ring.
func New(tables *datapack.Synthesizer, sampleRate int) *Synthesizer {
	return &Synthesizer{End: noEnd, converter: NewConverter(tables, sampleRate), renderer: NewRenderer(tables, sampleRate)}
}

// Reset resets the converter and the renderer for the sample rate and drops the converted block.
func (synthesizer *Synthesizer) Reset(sampleRate int) {
	synthesizer.converter.Reset(sampleRate)
	synthesizer.renderer.Reset(sampleRate)
	synthesizer.Ready = false
	synthesizer.Hold = false
}

// ResetFrames empties the ring and starts it with 12 copies of the initial frame, of which the first 10 are
// spoken (synthesizer.md §5.1).
func (synthesizer *Synthesizer) ResetFrames(initial *Frame) {
	synthesizer.Markers = [RingLength]bool{}
	synthesizer.Tones = [RingLength]bool{}
	for position := range initialFrames {
		synthesizer.Frames[position] = *initial
	}
	synthesizer.Position, synthesizer.Written, synthesizer.End = 0, initialFrames, initialEnd
	synthesizer.MarkersPassed = 0
	synthesizer.Done = false
}

// Rebase moves the frame counters back by RebaseFrames once the position has passed it, and reports whether it
// did. The ring's slots stay where they are.
func (synthesizer *Synthesizer) Rebase() bool {
	if synthesizer.Position <= RebaseFrames {
		return false
	}
	synthesizer.Position -= RebaseFrames
	if synthesizer.End != noEnd {
		synthesizer.End -= RebaseFrames
	}
	synthesizer.Written -= RebaseFrames
	return true
}

// Step renders the converted block, unless held or done, and then converts the next frame if the lookahead
// allows (synthesizer.md §5.2).
func (synthesizer *Synthesizer) Step() {
	if !synthesizer.Hold && !synthesizer.Done && synthesizer.Ready {
		if synthesizer.Rendering {
			synthesizer.Samples = synthesizer.renderer.Render(synthesizer.converter.Block(), synthesizer.OutputVoice, synthesizer.Gain, synthesizer.Samples)
		}
		synthesizer.Ready = false
		if synthesizer.Position == synthesizer.End {
			synthesizer.Done = true
		}
	}
	if synthesizer.Lookahead+synthesizer.Position >= synthesizer.Written || synthesizer.Ready {
		return
	}
	slot := synthesizer.Position & (RingLength - 1)
	if synthesizer.Markers[slot] {
		synthesizer.Markers[slot] = false
		synthesizer.MarkersPassed++
	}
	tone := synthesizer.Tones[slot]
	synthesizer.Tones[slot] = false
	synthesizer.converter.Convert(&synthesizer.Frames[slot], tone, synthesizer.Mute)
	synthesizer.Position++
	synthesizer.Ready = true
}

// Run steps until no block is ready, speech is done or held, or the buffer is full, and reports whether the
// buffer is to be handed over. retire runs the output stage: after a step that passed a marker, and at the
// end (synthesizer.md §5.3).
func (synthesizer *Synthesizer) Run(retire func()) bool {
	handOver := false
	for !synthesizer.Done && !synthesizer.Hold {
		synthesizer.Step()
		if !synthesizer.Ready {
			break
		}
		full := 2*len(synthesizer.Samples) > HandOverBytes
		if synthesizer.MarkersPassed != 0 {
			retire()
			handOver = true
		} else if full {
			handOver = true
		}
		if full {
			break
		}
	}
	retire()
	return handOver
}
