// Package pipeline is the TruVoice engine's speaking pipeline (docs/truvoice/pipeline.md): the nodes the
// stages pass on, the stages' cursors, the control commands, the output stage and the scheduler.
package pipeline

import (
	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/synthesizer"
)

// Pipeline is the speaking thread's state, as far as the stages share it.
type Pipeline struct {
	nodes   [nodeSlots]Node
	Free    int
	Current *Cursor
	Cursors [StageCount]Cursor

	// Settings are the escape parser's current settings, SettingsSpeed its speed in words a minute and
	// SettingsLevels its track levels. A reset gives the settings to every cursor and the track levels to the
	// parameter generator.
	Settings       Settings
	SettingsSpeed  int32
	SettingsLevels [datapack.TrackCount]byte

	// The scheduler's state: the stages that have work, whether the escape parser's ring has run out, whether
	// the parameter generator waits for room in the frame ring, and what the output stage reported last.
	Stages         Stages
	HasWork        [Output]bool
	OutOfInput     bool
	WaitingForRoom bool
	Retired        int

	// Pitch, Speed, Volume and Character are the attributes the speaking thread last applied, which the
	// control commands also set. SavedRate is the rate index a filter command keeps, Levels the track levels.
	// SampleRate is the rate the synthesizer renders at.
	Pitch      uint32
	Speed      uint32
	Volume     uint32
	Character  uint32
	SavedRate  int32
	Levels     [datapack.TrackCount]byte
	SampleRate int

	// ResetRequested makes the next step reset the stages; ResetValue is what the request carries.
	ResetRequested bool
	ResetValue     uint32

	Synthesizer *synthesizer.Synthesizer
	Engine      *Engine
	tables      *datapack.Pipeline
}

// Engine is the engine the pipeline speaks for (nil when it converts text to phonemes): the attributes its
// API sets and reads, its sample rate and output gain, the queue that carries the audio and the events to the
// destination in order, and the messages the pipeline posts to the engine's window.
type Engine struct {
	Pitch      uint16
	Speed      uint32
	Volume     uint32
	Character  uint32
	Voice      int32
	SampleRate int
	Gain       uint32
	Queue      []Item
	Messages   []Message
}

// An Item is an event, or a buffer of audio, queued for the destination.
type Item struct {
	Event   *Event
	Samples []int16
}

type EventKind int

// The events' kinds. A mark of value 0 is a text's end.
const (
	Mark EventKind = iota
	Visual
	WordPosition
)

type Event struct {
	Kind  EventKind
	Text  *Text
	Value uint32
}

type MessageKind int

const (
	AttributeChanged MessageKind = iota
	TextDone
)

// The attributes of an AttributeChanged message, as SAPI numbers them.
const (
	PitchAttribute  = 1
	SpeedAttribute  = 2
	VolumeAttribute = 3
)

type Message struct {
	Kind      MessageKind
	Attribute int
	Text      *Text
}

// The attributes, gain and sample rate a new pipeline starts with (pipeline.md §8.1).
const (
	initialPitch      = 85
	initialSpeed      = 150
	initialVolume     = 0xffff
	initialGain       = 1
	initialSampleRate = 8000
)

// New returns a pipeline as the speaking thread constructs it on its stack: its nodes all zero, its attributes
// pitch 85, speed 150, full volume, normal character and voice 0, gain 1, rendering at 8000 Hz.
func New(tables *datapack.Pipeline, synthesizer *synthesizer.Synthesizer, stages Stages, engine *Engine) *Pipeline {
	pipeline := &Pipeline{Synthesizer: synthesizer, Stages: stages, Engine: engine, tables: tables,
		Pitch: initialPitch, Speed: initialSpeed, Volume: initialVolume, SampleRate: initialSampleRate}
	synthesizer.OutputVoice = 0
	synthesizer.Gain = initialGain
	for index := range pipeline.nodes {
		pipeline.nodes[index].index = index
	}
	for stage := range pipeline.Cursors {
		pipeline.Cursors[stage].Types = stageTypes[stage]
	}
	return pipeline
}
