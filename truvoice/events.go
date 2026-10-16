package truvoice

import "github.com/phyce/OpenSAPI4/truvoice/datapack"

// Sink receives what an engine makes of its texts, in stream order: the audio in pieces as the engine's window
// gives it to the destination, and each event where it stands in it. Audio gets PCM in the engine's format
// (Engine.Format) and returns how many bytes it took, all of them unless it returns an error. ErrPaused pauses the
// engine (Engine.Pause): the rest of the audio is held from the bytes it took, or from the next buffer after an
// event. Any other error resets the engine (Engine.Run).
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

// Event is a position in the audio. Sample counts the samples the engine gave before it, from its first text, in
// its format. Text is the number of the event's text, the texts queued before it; a Visual event belongs to no
// text, and its Text is −1.
type Event struct {
	Kind   EventKind
	Sample int64
	Text   int
	// Value is a word position's byte offset in the text (engine.md §1.2) or a bookmark's number.
	Value uint32
	// Visual is a Visual event's phoneme.
	Visual VisualPhoneme
	// Aborted is set on the TextDone of a text a reset ended (TTSBNS_ABORTED).
	Aborted bool
}

// VisualPhoneme is a phoneme a Visual event reports: the engine phoneme, its IPA character (0 for none) and its
// mouth shape (nil for none).
type VisualPhoneme struct {
	Engine byte
	IPA    uint16
	Mouth  *datapack.Mouth
}

// Attribute is a voice attribute, numbered as TTSATTR and TTSNSAC.
type Attribute uint32

const (
	PitchAttribute  Attribute = 1
	SpeedAttribute  Attribute = 2
	VolumeAttribute Attribute = 3
)
