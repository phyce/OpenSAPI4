// Package truvoice is the L&H TruVoice engine (docs/truvoice/engine.md): it converts texts as TextData does, speaks
// them with the speaking pipeline and gives the audio and the events, in stream order, to a sink.
package truvoice

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/lexicon"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
	"github.com/phyce/OpenSAPI4/truvoice/prosody"
	"github.com/phyce/OpenSAPI4/truvoice/text"
)

var (
	// ErrValueOutOfRange is the original's 0x8000FFFF: a pitch or speed outside the engine's range.
	ErrValueOutOfRange = errors.New("truvoice: value out of range")
	// ErrWaveFormatNotSupported is TTSERR_WAVEFORMATNOTSUPPORTED: the destination takes none of the engine's
	// formats.
	ErrWaveFormatNotSupported = errors.New("truvoice: wave format not supported")
	// ErrPaused pauses the engine when a sink returns it (Engine.Pause). Run and Resume return it while the
	// engine is paused.
	ErrPaused = errors.New("truvoice: paused")
	// ErrAlreadyPaused is TTSERR_ALREADYPAUSED: Pause on an engine that is paused.
	ErrAlreadyPaused = errors.New("truvoice: already paused")
	// ErrNotPaused is TTSERR_NOTPAUSED: Resume on an engine that isn't paused.
	ErrNotPaused = errors.New("truvoice: not paused")
)

// Data is what the engines of one data pack share: its tables, and what the original keeps for the whole process,
// the prosody stage's shared state, the voice \Rst\ goes back to and the user lexicon.
type Data struct {
	Modes       *datapack.Modes
	pipeline    *datapack.Pipeline
	synthesizer *datapack.Synthesizer
	parameters  *datapack.Parameters
	vowels      *datapack.Vowels
	input       *datapack.Input
	normalizer  *datapack.Normalizer
	lexical     *datapack.Lexical
	dictionary  *datapack.Dictionary
	prosody     *datapack.Prosody
	text        *datapack.Text
	engine      *datapack.Engine

	process     *text.Process
	shared      *prosody.Shared
	userLexicon *lexicon.Lexicon
}

func Load(pack *datapack.Pack) (*Data, error) {
	data := &Data{process: text.NewProcess(), shared: new(prosody.Shared)}
	var err error
	loads := []func(){
		func() { data.Modes, err = pack.LoadModes() },
		func() { data.pipeline, err = pack.LoadPipeline() },
		func() { data.synthesizer, err = pack.LoadSynthesizer() },
		func() { data.parameters, err = pack.LoadParameters() },
		func() { data.vowels, err = pack.LoadVowels() },
		func() { data.input, err = pack.LoadInput() },
		func() { data.normalizer, err = pack.LoadNormalizer() },
		func() { data.lexical, err = pack.LoadLexical() },
		func() { data.dictionary, err = pack.LoadDictionary() },
		func() { data.prosody, err = pack.LoadProsody() },
		func() { data.text, err = pack.LoadText() },
		func() { data.engine, err = pack.LoadEngine() },
	}
	for _, load := range loads {
		if load(); err != nil {
			return nil, err
		}
	}
	data.userLexicon = lexicon.New(data.text, data.engine, func(value []byte) []byte {
		return data.listing(append(bytes.Clone(value), 0), false)
	})
	return data, nil
}

// Lexicon is the user lexicon every engine of the data looks words up in, as every engine of the original's process
// does (engine.md §7). It starts empty; Read fills it from a file.
func (data *Data) Lexicon() *lexicon.Lexicon {
	return data.userLexicon
}

// Format is a PCM format: mono, SampleRate samples a second, and Bits 16 (signed, little-endian) or 8
// (unsigned).
type Format struct {
	SampleRate int
	Bits       int
}

// AcceptFormat is the audio destination's answer to a format the engine offers: nil when it takes the format.
type AcceptFormat func(format Format) error

// formats are the formats the engine offers, in order.
var formats = []Format{{11025, 16}, {11025, 8}, {8000, 16}, {8000, 8}}

// The attributes a new engine has (engine.md §2.5).
const (
	initialVolume = 0xffff
	initialGain   = 1
)

// Engine speaks texts with one of the ten voices. What a text changes lasts into the next one, as in the original:
// the attributes, the voice, the context and the doubling of the word marks' offsets. Each speaking thread starts
// a new pipeline. An engine is not safe for concurrent use.
type Engine struct {
	data      *Data
	format    Format
	state     pipeline.Engine
	converter *text.Converter
	textCount int

	// The window (engine.md §2.3, §5): the bytes of audio the sinks took, and of the queue's first buffer.
	given, taken int64
	// The speaking thread, while it runs or waits in a pause (engine.md §5.2): it runs on behind the held audio
	// until its queue is full, and then waits for the window.
	thread           *thread
	running          bool
	paused           bool
	runningOn        bool
	waitingForWindow bool

	// AttributeChanged, when set, is told of every change of pitch, speed or volume: from the methods that set
	// them, and from the texts in stream order.
	AttributeChanged func(attribute Attribute)
}

// NewEngine makes an engine for a voice, 0 to 9, in the engine's first format, 11025 Hz with 16 bits.
func (data *Data) NewEngine(voice int) (*Engine, error) {
	return data.NewEngineAccepting(voice, nil)
}

// NewEngineAccepting makes an engine for a voice, in the first format accept takes: 11025 Hz with 16 bits, then 8,
// then 8000 Hz with 16 and 8. A nil accept takes the first.
func (data *Data) NewEngineAccepting(voice int, accept AcceptFormat) (*Engine, error) {
	if voice < 0 || voice >= datapack.VoiceCount {
		return nil, fmt.Errorf("truvoice: voice %d, expected 0 to %d", voice, datapack.VoiceCount-1)
	}
	format, err := negotiate(accept)
	if err != nil {
		return nil, err
	}
	defaults := data.pipeline.Voices[voice]
	engine := &Engine{data: data, format: format}
	engine.state = pipeline.Engine{Pitch: uint16(defaults.Pitch), Speed: uint32(defaults.Speed), Volume: initialVolume,
		Voice: int32(voice), SampleRate: format.SampleRate, Gain: initialGain, Pronounce: engine.pronounce}
	engine.converter = text.New(data.text, data.Modes, data.process, int32(voice),
		text.Defaults{Pitch: uint16(defaults.Pitch), Speed: uint32(defaults.Speed), Volume: initialVolume})
	return engine, nil
}

func negotiate(accept AcceptFormat) (Format, error) {
	if accept == nil {
		return formats[0], nil
	}
	for _, format := range formats {
		if accept(format) == nil {
			return format, nil
		}
	}
	return Format{}, ErrWaveFormatNotSupported
}

func (engine *Engine) Format() Format {
	return engine.format
}
