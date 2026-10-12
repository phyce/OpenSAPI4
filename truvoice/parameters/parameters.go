// Package parameters is the TruVoice engine's parameter generator (docs/truvoice/parameters.md): it turns the
// timed phonemes into frames of 22 tracks in the synthesizer's ring.
package parameters

import (
	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/synthesizer"
)

// Generator is the parameter generator's state. The frames it writes, and how far it has written, are the
// synthesizer's.
type Generator struct {
	Tracks   [datapack.TrackCount]Track
	Segments [datapack.TrackCount]Segment
	// Last is the value of each track's last written frame, in the track's units.
	Last [datapack.TrackCount]int32
	// Furthest is the furthest any track has written to, Margin the frames the ring keeps free beyond it.
	Furthest int32
	Margin   int32

	Synthesizer *synthesizer.Synthesizer
	tables      *datapack.Parameters
}

func New(tables *datapack.Parameters, synthesizer *synthesizer.Synthesizer) *Generator {
	return &Generator{Synthesizer: synthesizer, tables: tables}
}

func multiply(first, second int32) int32 {
	return first * second / 0x7fff
}
