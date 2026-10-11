// Package whistler is the Microsoft Speech Synthesis Engine of SAPI 4: an engine speaks texts with one mode
// of a data pack, as the original does (docs/whistler/engine.md).
package whistler

import (
	"errors"
	"fmt"
	"sync"
	"unicode/utf16"

	"github.com/phyce/OpenSAPI4/whistler/analyser"
	"github.com/phyce/OpenSAPI4/whistler/datapack"
	"github.com/phyce/OpenSAPI4/whistler/effects"
	"github.com/phyce/OpenSAPI4/whistler/lexicon"
	"github.com/phyce/OpenSAPI4/whistler/posting"
	"github.com/phyce/OpenSAPI4/whistler/synthesis"
	"github.com/phyce/OpenSAPI4/whistler/text"
	"github.com/phyce/OpenSAPI4/whistler/units"
)

var (
	// ErrValueOutOfRange is TTSERR_VALUEOUTOFRANGE: a pitch or speed outside the mode's range.
	ErrValueOutOfRange = errors.New("whistler: value out of range")
	// ErrInvalidParameter is TTSERR_INVALIDPARAM: an attribute the engine doesn't have, or a thread priority
	// that isn't one of Windows' levels.
	ErrInvalidParameter = errors.New("whistler: invalid parameter")
	// ErrInvalidCharacter is TTSERR_INVALIDCHAR: a character set Speak doesn't take.
	ErrInvalidCharacter = errors.New("whistler: invalid character set")
	// ErrPaused pauses the engine when a sink returns it (Engine.Pause). Speak and Resume return it while the
	// engine is paused.
	ErrPaused = errors.New("whistler: paused")
	// ErrAlreadyPaused is TTSERR_ALREADYPAUSED: Pause on an engine that is paused.
	ErrAlreadyPaused = errors.New("whistler: already paused")
	// ErrNotPaused is TTSERR_NOTPAUSED: Resume on an engine that isn't paused.
	ErrNotPaused = errors.New("whistler: not paused")
)

// Data is what the engines of one data pack share. It loads each voice once, when an engine first needs it.
type Data struct {
	Modes        *datapack.Modes
	pack         *datapack.Pack
	prosody      *datapack.Prosody
	rules        *datapack.Rules
	dictionaries *datapack.Dictionaries
	text         *datapack.Text
	resamplers   *datapack.Resamplers
	converter    *posting.Converter

	userLexicon *lexicon.Lexicon

	lock   sync.Mutex
	voices map[string]*voice
}

type voice struct {
	data      *datapack.Voice
	synthesis *synthesis.Voice
}

func Load(pack *datapack.Pack) (*Data, error) {
	data := &Data{pack: pack, voices: map[string]*voice{}}
	var err error
	if data.Modes, err = pack.LoadModes(); err != nil {
		return nil, err
	}
	if data.prosody, err = pack.LoadProsody(); err != nil {
		return nil, err
	}
	if data.rules, err = pack.LoadRules(); err != nil {
		return nil, err
	}
	if data.dictionaries, err = pack.LoadDictionaries(); err != nil {
		return nil, err
	}
	if data.text, err = pack.LoadText(); err != nil {
		return nil, err
	}
	if data.resamplers, err = pack.LoadResamplers(); err != nil {
		return nil, err
	}
	data.userLexicon = lexicon.New(data.text)
	data.converter = posting.NewConverter(data.text.PhoneSet)
	return data, nil
}

func (data *Data) voice(name string) (*voice, error) {
	data.lock.Lock()
	defer data.lock.Unlock()
	if loaded, found := data.voices[name]; found {
		return loaded, nil
	}
	voiceData, err := data.pack.LoadVoice(name)
	if err != nil {
		return nil, err
	}
	loaded := &voice{data: voiceData, synthesis: synthesis.NewVoice(voiceData.Inventory)}
	data.voices[name] = loaded
	return loaded, nil
}

// Lexicon is the user lexicon every engine of the data looks words up in, as every engine of the original's
// process does (pronunciation.md §6.3). It starts empty; Read fills it from a file.
func (data *Data) Lexicon() *lexicon.Lexicon {
	return data.userLexicon
}

// Mode finds a mode an engine can be made with, by its name.
func (data *Data) Mode(name string) (*datapack.Mode, bool) {
	for index := range data.Modes.Modes {
		if mode := &data.Modes.Modes[index]; !mode.Hidden() && mode.Name == name {
			return mode, true
		}
	}
	return nil, false
}

// Engine speaks texts with one mode. Everything a text changes lasts into the next one, as in the original:
// pitch, speed, volume, the monotone flag, the address context, the effects and the synthesizer's state.
// An engine is not safe for concurrent use.
type Engine struct {
	mode         *datapack.Mode
	voice        *datapack.Voice
	tables       *datapack.Prosody
	preprocessor *text.Preprocessor
	userLexicon  *lexicon.Lexicon
	reader       *analyser.Reader
	analyser     *analyser.Analyser
	mapper       *units.Mapper
	synthesizer  *synthesis.Synthesizer
	effects      *effects.Effects
	monotone     *effects.Monotone
	converter    *posting.Converter
	format       Format

	basePitch      int
	pitch          int
	threadPriority int32
	addressContext bool
	textCount      int
	byteCount      int64

	// AttributeChanged, when set, is told of every change of pitch, speed or volume: from the methods that
	// set them and from the texts. Pitch changes come as the front end reads, speed and volume ones in
	// stream order.
	AttributeChanged func(attribute Attribute)

	stages       stages
	controls     []text.Control
	oneCharacter bool
	tagCount     int
	// transplanting is on while a prosody transplant replaces the text (engine.md §13.2).
	transplanting bool
	firstTag      int32
}

// NewEngine makes an engine for a mode that isn't hidden, in the voice's own format. Select finds the mode by
// its ID.
func (data *Data) NewEngine(mode *datapack.Mode) (*Engine, error) {
	return data.NewEngineAccepting(mode, nil)
}

// NewEngineAccepting makes an engine for a mode that isn't hidden, in the first format accept takes
// (interface.md §4).
func (data *Data) NewEngineAccepting(mode *datapack.Mode, accept AcceptFormat) (*Engine, error) {
	if mode.Hidden() {
		return nil, fmt.Errorf("mode %q is hidden: an engine can't be made with it", mode.Name)
	}
	return data.newEngine(mode, accept)
}

func (data *Data) newEngine(mode *datapack.Mode, accept AcceptFormat) (*Engine, error) {
	loaded, err := data.voice(mode.Voice)
	if err != nil {
		return nil, err
	}
	format, err := negotiate(loaded.data.SampleRate, accept)
	if err != nil {
		return nil, err
	}
	textAnalyser, err := analyser.New(data.rules, data.dictionaries, data.prosody)
	if err != nil {
		return nil, err
	}
	engine := &Engine{
		mode:         mode,
		voice:        loaded.data,
		tables:       data.prosody,
		preprocessor: text.New(data.text),
		userLexicon:  data.userLexicon,
		reader:       analyser.NewReader(data.dictionaries, data.prosody),
		analyser:     textAnalyser,
		mapper:       units.NewMapper(loaded.data, mode.Speed),
		synthesizer:  synthesis.NewSynthesizer(loaded.synthesis),
		effects:      effects.New(data.Modes, mode, loaded.synthesis.SampleRate()),
		monotone:     effects.NewMonotone(mode),
		converter:    data.converter,
		format:       format,
		basePitch:    mode.Pitch,
		pitch:        mode.Pitch,
	}
	engine.synthesizer.Effects = engine.effects
	engine.synthesizer.Resampler = synthesis.NewResampler(data.resamplers, loaded.data.SampleRate, format.SampleRate)
	engine.synthesizer.EightBit = format.Bits == 8
	engine.reader.UserLexicon = data.userLexicon
	return engine, nil
}

func (engine *Engine) Mode() *datapack.Mode {
	return engine.mode
}

// Format is the format the destination accepted. A resampled format's samples come at the voice's rate times
// the resampler's factor, which is not quite the format's rate: 8018.18 Hz for 8000 Hz, 16036.36 Hz for 16000 Hz
// and 11000 Hz for 11025 Hz (synthesis.md §13.2).
func (engine *Engine) Format() Format {
	return engine.format
}

func (engine *Engine) Pitch() uint16 {
	return uint16(engine.pitch)
}

// SetPitch sets the pitch in Hz: 0 gives the minimum, half the mode's own pitch, and 65535 the maximum,
// twice the mode's own. It refuses any other value outside that range.
func (engine *Engine) SetPitch(hertz uint16) error {
	minimum, maximum := engine.basePitch/2, 2*engine.basePitch
	value := int(hertz)
	switch {
	case hertz == 0:
		value = minimum
	case hertz == 0xFFFF:
		value = maximum
	case value > maximum || value < minimum:
		return ErrValueOutOfRange
	}
	engine.pitch = value
	engine.attributeChanged(PitchAttribute)
	return nil
}

func (engine *Engine) resetPitch() {
	engine.pitch = engine.basePitch
	engine.attributeChanged(PitchAttribute)
}

// Speed is the speed in words per minute.
func (engine *Engine) Speed() uint32 {
	return uint32(engine.mapper.Speed())
}

// SetSpeed sets the speed in words per minute: 0 gives the minimum, 30, and 0xFFFFFFFF the maximum, three
// times the mode's own speed. It refuses any other value outside that range.
func (engine *Engine) SetSpeed(wordsPerMinute uint32) error {
	if !engine.mapper.SetSpeed(wordsPerMinute) {
		return ErrValueOutOfRange
	}
	engine.attributeChanged(SpeedAttribute)
	return nil
}

func (engine *Engine) resetSpeed() {
	engine.mapper.ResetSpeed()
	engine.attributeChanged(SpeedAttribute)
}

// Volume is the volume in both halves, left channel in the high 16 bits, as the original reports it.
func (engine *Engine) Volume() uint32 {
	volume := uint32(engine.mapper.Volume())
	return volume<<16 | volume
}

// SetVolume sets the volume to the low 16 bits of value, the left channel; output is mono.
func (engine *Engine) SetVolume(value uint32) {
	engine.mapper.SetVolume(value)
	engine.attributeChanged(VolumeAttribute)
}

// Attribute reads an attribute as IAttributes::DWORDGet does: pitch, speed, volume or thread priority.
func (engine *Engine) Attribute(attribute Attribute) (uint32, error) {
	switch attribute {
	case PitchAttribute:
		return uint32(engine.Pitch()), nil
	case SpeedAttribute:
		return engine.Speed(), nil
	case VolumeAttribute:
		return engine.Volume(), nil
	case ThreadPriorityAttribute:
		return uint32(engine.threadPriority), nil
	}
	return 0, ErrInvalidParameter
}

// SetAttribute sets an attribute as IAttributes::DWORDSet does. A pitch is cut to its low 16 bits. A
// thread priority must be one of Windows' seven levels (-15, -2, -1, 0, 1, 2, 15); it is kept and changes
// nothing else.
func (engine *Engine) SetAttribute(attribute Attribute, value uint32) error {
	switch attribute {
	case PitchAttribute:
		return engine.SetPitch(uint16(value))
	case SpeedAttribute:
		return engine.SetSpeed(value)
	case VolumeAttribute:
		engine.SetVolume(value)
		return nil
	case ThreadPriorityAttribute:
		switch priority := int32(value); priority {
		case -15, -2, -1, 0, 1, 2, 15:
			engine.threadPriority = priority
			return nil
		}
	}
	return ErrInvalidParameter
}

// Inject applies the tags of an Inject string at once, outside any text (interface.md §7): \Pit=n\ (0 is
// the minimum), \Spd=n\, \Vol=n\ and \Rst\, which resets speed, volume and pitch but not the voice. It stops
// at the first character outside a tag other than a space, any other tag, a value refused, and after \Rst\.
func (engine *Engine) Inject(tags string) {
	engine.preprocessor.ReadInject(utf16.Encode([]rune(tags)), func(kind text.ControlKind, value uint32) bool {
		switch kind {
		case text.ControlPitch:
			return engine.SetPitch(uint16(value)) == nil
		case text.ControlSpeed:
			return engine.SetSpeed(value) == nil
		case text.ControlVolume:
			engine.SetVolume(value)
		case text.ControlReset:
			engine.resetSpeed()
			engine.SetVolume(0xFFFFFFFF)
			engine.resetPitch()
		}
		return true
	})
}

func (engine *Engine) attributeChanged(attribute Attribute) {
	if engine.AttributeChanged != nil {
		engine.AttributeChanged(attribute)
	}
}
