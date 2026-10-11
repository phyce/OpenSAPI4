package whistler

import "github.com/phyce/OpenSAPI4/whistler/datapack"

// Sink receives what an engine makes of a text, in stream order: the audio in pieces as it is made, and each
// event where it stands in it. Audio gets PCM in the engine's format (Engine.Format) and returns how many
// bytes it took, all of them unless it returns an error. An error stops the text (Engine.Speak), and
// ErrPaused pauses the engine (Engine.Pause): the rest of the samples are held from the bytes it took, or
// from the next piece after an event.
type Sink interface {
	Audio(data []byte) (int, error)
	Event(event Event) error
}

type EventKind int

const (
	TextStarted EventKind = iota + 1
	TextDone
	Visual
	WordPosition
	Bookmark
)

// Event is a position in the audio. Sample counts the samples the engine made before it, from its first
// text, in its format, and Text the texts before its own.
type Event struct {
	Kind   EventKind
	Sample int64
	Text   int
	// Value is a word position's byte offset in the text (engine.md §8) or a bookmark's number.
	Value uint32
	// Visual is a Visual event's phone: its IPA character, engine phoneme and mouth shape.
	Visual datapack.Visual
}

// Attribute is a voice attribute, numbered as TTSATTR and, for the three that change, as TTSNSAC.
type Attribute uint32

const (
	PitchAttribute          Attribute = 1
	SpeedAttribute          Attribute = 2
	VolumeAttribute         Attribute = 3
	ThreadPriorityAttribute Attribute = 0x1018
)
