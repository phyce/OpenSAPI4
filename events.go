package opensapi4

import (
	"github.com/phyce/OpenSAPI4/truvoice"
	"github.com/phyce/OpenSAPI4/whistler"
)

// Format is a PCM format: mono, SampleRate samples a second, and Bits 16 (signed, little-endian) or 8 (unsigned).
type Format struct {
	SampleRate int
	Bits       int
}

// Sink receives a speech's audio and events, in stream order. Audio gets PCM in the speech's format and returns how
// many bytes it took, all of them unless it returns an error. An error from either stops the speech.
type Sink interface {
	Audio(data []byte) (int, error)
	Event(event Event) error
}

// EventKind is an event's kind, numbered as the engines number theirs.
type EventKind int

const (
	TextStarted EventKind = iota + 1
	TextDone
	Visual
	WordPosition
	Bookmark
)

// Event is a position in the audio. Sample counts the samples before it, from the speech's start, in its format.
// Text is the number of the event's text, 0 for the request's Text and then 1, 2… for its Texts; TruVoice's Visual
// events belong to no text, and their Text is −1.
type Event struct {
	Kind   EventKind
	Sample int64
	Text   int
	// Value is a word position's offset in the text in bytes, two to a UTF-16 code unit, or a bookmark's number.
	Value uint32
	// Visual is a Visual event's phoneme.
	Visual VisualPhoneme
}

// VisualPhoneme is the phoneme a Visual event reports: the engine's phoneme, its IPA character (0 for none) and its
// mouth shape (nil for none).
type VisualPhoneme struct {
	Engine byte
	IPA    uint16
	Mouth  *Mouth
}

type Mouth struct {
	Height            byte
	Width             byte
	Upturn            byte
	JawOpen           byte
	UpperTeethVisible byte
	LowerTeethVisible byte
	TonguePosition    byte
	LipTension        byte
}

type microsoftSink struct {
	sink Sink
}

func (adapter microsoftSink) Audio(data []byte) (int, error) {
	return adapter.sink.Audio(data)
}

func (adapter microsoftSink) Event(event whistler.Event) error {
	converted := Event{Kind: EventKind(event.Kind), Sample: event.Sample, Text: event.Text, Value: event.Value}
	if event.Kind == whistler.Visual {
		mouth := Mouth(event.Visual.Mouth)
		converted.Visual = VisualPhoneme{Engine: event.Visual.Engine, IPA: event.Visual.IPA, Mouth: &mouth}
	}
	return adapter.sink.Event(converted)
}

type truVoiceSink struct {
	sink Sink
}

func (adapter truVoiceSink) Audio(data []byte) (int, error) {
	return adapter.sink.Audio(data)
}

func (adapter truVoiceSink) Event(event truvoice.Event) error {
	converted := Event{Kind: EventKind(event.Kind), Sample: event.Sample, Text: event.Text, Value: event.Value}
	if event.Kind == truvoice.Visual {
		converted.Visual = VisualPhoneme{Engine: event.Visual.Engine, IPA: event.Visual.IPA}
		if event.Visual.Mouth != nil {
			mouth := Mouth(*event.Visual.Mouth)
			converted.Visual.Mouth = &mouth
		}
	}
	return adapter.sink.Event(converted)
}
