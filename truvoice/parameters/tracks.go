package parameters

import (
	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/synthesizer"
)

// A Track is where the parameter generator is on one of the 22 tracks: Next is the frame it writes next, and
// Earliest the first frame it may still change (parameters.md §2.1).
type Track struct {
	Next     int32
	Earliest int32
}

// A Segment is what the parameter generator writes on one track for a phoneme, in the track's units
// (parameters.md §2.3). Back is the shape of the pull on the frames before Next, or the frames a bridge reaches
// back; Forward the shape from Onset to Target, or the frames a ramp or a bridge reaches forward.
type Segment struct {
	Mode       int32
	Back       int32
	Forward    int32
	Frames     int32
	BackTarget int32
	Onset      int32
	Target     int32
}

// The segments' modes (parameters.md §2.4). The modes with PullBack first pull the frames before Next.
const (
	PullBack    = 1
	PullForward = 2
	Flat        = 4
	Shaped      = 6
	Ramp        = 8
	Bridge      = 16
	Bridged     = 20
)

// trackScales are the tracks' units: a frame's byte b on the track means b·2^shift + offset (parameters.md §2.2).
var trackScales = [datapack.TrackCount]struct{ shift, offset int32 }{
	9: {2, 0}, 10: {3, 500}, 11: {4, 0}, 12: {4, 0}, 13: {1, 0}, 14: {1, 0}, 15: {1, 0}, 16: {2, 192}, 17: {1, 0}}

// Units returns a frame's byte on the track in the track's units.
func Units(track int, value byte) int32 {
	return int32(value)<<trackScales[track].shift + trackScales[track].offset
}

func fromUnits(track int, units int32) int32 {
	return (units - trackScales[track].offset) >> trackScales[track].shift
}

// A Ring is one track of the synthesizer's frame ring: frame p of the track is byte p mod 256.
type Ring struct {
	frames *[synthesizer.RingLength]synthesizer.Frame
	track  int
	shapes *[datapack.ShapeCount][]byte
}

func (generator *Generator) Ring(track int) Ring {
	return Ring{&generator.Synthesizer.Frames, track, &generator.tables.Shapes}
}

func (ring Ring) At(position int32) byte {
	return ring.frames[uint8(position)][ring.track]
}

func (ring Ring) Set(position int32, value byte) {
	ring.frames[uint8(position)][ring.track] = value
}

// weights returns a shape's weights. The original has 31 shapes and reads outside its table for the others
// (parameters.md §14); the port gives them none.
func (ring Ring) weights(shape int32) []byte {
	if shape < 0 || int(shape) >= len(ring.shapes) {
		return nil
	}
	return ring.shapes[shape]
}

func weighted(difference int32, weight byte) byte {
	return byte(multiply(difference, int32(weight)<<7))
}

// Fill writes frames frames from position on: while the shape lasts, onset moved toward target by the shape's
// weights, then target. A negative count writes the whole shape (parameters.md §2.5).
func (ring Ring) Fill(position, shape, frames int32, onset, target byte) {
	for _, weight := range ring.weights(shape) {
		if frames == 0 {
			return
		}
		frames--
		ring.Set(position, weighted(int32(onset)-int32(target), weight)+target)
		position++
	}
	for ; frames > 0; frames-- {
		ring.Set(position, target)
		position++
	}
}

// PullBack moves up to frames frames before position toward value, by the shape's weights after its first.
func (ring Ring) PullBack(position, shape, frames int32, value byte) {
	weights := ring.weights(shape)
	for index := 1; index < len(weights) && frames != 0; index++ {
		position--
		frame := ring.At(position)
		ring.Set(position, frame+weighted(int32(value)-int32(frame), weights[index]))
		frames--
	}
}

// PullForward moves up to frames frames from position on toward value, by the shape's weights.
func (ring Ring) PullForward(position, shape, frames int32, value byte) {
	for _, weight := range ring.weights(shape) {
		if frames == 0 {
			return
		}
		frame := ring.At(position)
		ring.Set(position, frame+weighted(int32(value)-int32(frame), weight))
		position++
		frames--
	}
}

// Bridge rewrites the frames from back before position up to forward after it as a straight line from the
// first of them to the frame after them.
func (ring Ring) Bridge(position, back, forward int32) {
	frames := back + forward
	start := position - back
	last := ring.At(position + forward)
	first := ring.At(start)
	difference := int32(last) - int32(first)
	for index := range max(frames, 0) {
		ring.Set(start+index, byte(index*difference/frames)+first)
	}
}

// Ramp writes frames frames from position on, a straight line from start, which it doesn't reach, to end,
// which the last frame is.
func (ring Ring) Ramp(position, frames int32, start, end byte) {
	difference := int32(end) - int32(start)
	for index := range max(frames, 0) {
		ring.Set(position+index, byte((index+1)*difference/frames)+start)
	}
}

// Repeat writes value to frames frames from position on.
func (ring Ring) Repeat(position, frames int32, value byte) {
	for index := range max(frames, 0) {
		ring.Set(position+index, value)
	}
}

// AddBack adds a share of change to up to frames frames before position, by the shape's weights after its
// first. The change is held so that the frame before position doesn't go below 0; on the amplitude tracks no
// frame does.
func (ring Ring) AddBack(position, shape, frames, change int32) {
	position--
	if first := int32(ring.At(position)); first+change < 0 {
		change = -first
	}
	weights := ring.weights(shape)
	for index := 1; index < len(weights) && frames != 0; index++ {
		value := multiply(change, int32(weights[index])<<7) + int32(ring.At(position))
		if value < 1 && ring.track < 3 {
			value = 0
		}
		ring.Set(position, byte(value))
		position--
		frames--
	}
}

// RenderSegment writes the track's segment from the track's next frame on, as its mode says (parameters.md
// §2.4). The pulls on earlier frames stop at the track's earliest frame.
func (generator *Generator) RenderSegment(track int) {
	segment := &generator.Segments[track]
	mode := uint32(segment.Mode)
	backTarget := byte(fromUnits(track, max(segment.BackTarget, 0)))
	onset := byte(fromUnits(track, max(segment.Onset, 0)))
	target := byte(fromUnits(track, segment.Target))
	next := generator.Tracks[track].Next
	room := next - generator.Tracks[track].Earliest
	ring := generator.Ring(track)
	if mode&PullBack != 0 && room < 1 {
		mode &^= PullBack
	}
	if mode&PullBack != 0 {
		if mode > PullBack|Shaped {
			return
		}
		ring.PullBack(next, segment.Back, room, backTarget)
		mode &^= PullBack
	}
	switch mode {
	case PullForward:
		ring.PullForward(next, segment.Forward, segment.Frames, onset)
	case Flat:
		ring.Fill(next, 0, segment.Frames, 0, target)
	case Shaped:
		ring.Fill(next, segment.Forward, segment.Frames, onset, target)
	case Ramp:
		ring.Ramp(next, segment.Forward, onset, target)
	case Bridge:
		ring.Bridge(next, segment.Back, segment.Forward)
	case Bridged:
		ring.Fill(next, segment.Forward, segment.Frames, onset, target)
		ring.Bridge(next, segment.Back, segment.Forward)
	}
}

// Advance moves every track's next frame on by its segment's frames, notes each track's last value, moves
// the earliest frames on to the lookahead before the next, and makes the lowest next frame the ring's
// written frames, marking the frame before it (parameters.md §2.6).
func (generator *Generator) Advance() {
	lowest, furthest := int32(0x1000), int32(-1)
	for track := range generator.Tracks {
		next := generator.Tracks[track].Next + generator.Segments[track].Frames
		generator.Tracks[track].Next = next
		generator.Last[track] = Units(track, generator.Ring(track).At(next-1))
		lowest, furthest = min(lowest, next), max(furthest, next)
		generator.Tracks[track].Earliest = max(generator.Tracks[track].Earliest, next-generator.Synthesizer.Lookahead)
	}
	generator.Synthesizer.Markers[uint8(lowest-1)] = true
	generator.Synthesizer.Written = lowest
	generator.Furthest = furthest
}

// Room reports whether the ring has room for frames more frames beyond the furthest written, keeping the
// margin free.
func (generator *Generator) Room(frames int32) bool {
	return generator.Margin+generator.Furthest-generator.Synthesizer.Position+frames < synthesizer.RingLength
}

// Rebase moves the ring's frame counters and the tracks' back by the synthesizer's RebaseFrames once the
// synthesizer has passed them, and reports whether it did.
func (generator *Generator) Rebase() bool {
	if !generator.Synthesizer.Rebase() {
		return false
	}
	for track := range generator.Tracks {
		generator.Tracks[track].Next -= synthesizer.RebaseFrames
		generator.Tracks[track].Earliest -= synthesizer.RebaseFrames
	}
	generator.Furthest -= synthesizer.RebaseFrames
	return true
}
