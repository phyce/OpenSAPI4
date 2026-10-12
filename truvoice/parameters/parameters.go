// Package parameters is the TruVoice engine's parameter generator (docs/truvoice/parameters.md): it turns the
// timed phonemes into frames of 22 tracks in the synthesizer's ring.
package parameters

import (
	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
	"github.com/phyce/OpenSAPI4/truvoice/synthesizer"
)

// Generator is the parameter generator's state. The frames it writes, and how far it has written, are the
// synthesizer's; the phonemes it renders are the pipeline's nodes at its cursor.
type Generator struct {
	Tracks   [datapack.TrackCount]Track
	Segments [datapack.TrackCount]Segment
	// Last is the value of each track's last written frame, in the track's units.
	Last [datapack.TrackCount]int32
	// Furthest is the furthest any track has written to, Margin the frames the ring keeps free beyond it.
	Furthest int32
	Margin   int32

	// What the generator knows of the phoneme it renders and the one before it (parameters.md §4), and the
	// rules' flags (§5.1).
	Category            int32
	PreviousCategory    int32
	Frontness           int32
	PreviousFrontness   int32
	AspirationShift     int32
	VoiceOnset          int32
	BurstFrames         int32
	VoicingShift        int32
	VowelTransition     int32
	VowelOffset         int32
	ClosureFrames       int32
	FricativeAspiration int32
	Aspiration          int32
	BurstLevels         [3]int32
	Flags               [datapack.FlagCount]bool
	Weights             [datapack.TrackCount]int32
	Offglide            [7]int32
	NextTargets         [datapack.TrackCount]int32

	// UnvoicedPitch is the pitch target of the last phoneme without voicing. KeepEarlier makes the next phoneme
	// leave the frames before it as they are.
	UnvoicedPitch int32
	KeepEarlier   bool

	// The stage's own state, which its reset sets (parameters.md §7).
	TransitionLimit int32
	PauseState      int32
	EndState        int32
	EndReached      bool
	GeneratedDone   bool
	Coarticulation  byte
	PitchMark       byte
	BeforePrevious  byte

	Actions     Actions
	Pipeline    *pipeline.Pipeline
	Synthesizer *synthesizer.Synthesizer
	tables      *datapack.Parameters
}

// Actions runs the rules' actions (parameters.md §5.4), which the port doesn't have yet.
type Actions interface {
	Run(generator *Generator, action int)
}

func New(tables *datapack.Parameters, pipeline *pipeline.Pipeline) *Generator {
	return &Generator{Pipeline: pipeline, Synthesizer: pipeline.Synthesizer, tables: tables}
}

func multiply(first, second int32) int32 {
	return first * second / 0x7fff
}
