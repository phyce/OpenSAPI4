// Package compatibility does what the sapi4out and sapi4limits tools that other programs call do: how they
// read their arguments, choose a voice, set it up and speak.
package compatibility

import (
	"fmt"
	"math"
	"math/rand/v2"

	"github.com/phyce/OpenSAPI4/internal/voicedata"
	"github.com/phyce/OpenSAPI4/internal/wave"
	"github.com/phyce/OpenSAPI4/truvoice"
	truvoicetext "github.com/phyce/OpenSAPI4/truvoice/text"
	"github.com/phyce/OpenSAPI4/whistler"
	"github.com/phyce/OpenSAPI4/whistler/text"
)

// Argument is a command-line argument as the tools get it.
type Argument struct {
	// ANSI is the argument as main's argv has it: on Windows in the ANSI code page, elsewhere as given.
	ANSI []byte
	// Text is the argument in UTF-16, as SAPI 4's ANSI interface converts a text with the ANSI code page.
	Text []uint16
}

// fallbackVoice is the mode the tools get for a name no mode has exactly: the one SAPI 4's Find picks when
// it is asked for nothing but a name.
const fallbackVoice = "Mary"

type Limits struct {
	Default, Minimum, Maximum uint32
}

// Voice is an engine set up as the tools set one up, with the pitch and speed limits they read from it.
type Voice struct {
	Name         string
	Pitch, Speed Limits
	engine       engine
	microsoft    *whistler.Engine
	truVoice     *truvoice.Engine
}

// engine is what the tools set of both engines.
type engine interface {
	Pitch() uint16
	SetPitch(hertz uint16) error
	Speed() uint32
	SetSpeed(wordsPerMinute uint32) error
}

// ModeNames are the names of the modes as SAPI 4's enumerator lists them: the Microsoft modes, then TruVoice's.
func ModeNames(voices *voicedata.Voices) ([]string, error) {
	var names []string
	if voices.Microsoft != nil {
		enumerator := voices.Microsoft.Enumerate()
		for {
			modes, complete := enumerator.Next(1)
			if !complete {
				break
			}
			names = append(names, modes[0].ModeName)
		}
	}
	if voices.TruVoice != nil {
		modes, err := voices.TruVoice.LoadModes()
		if err != nil {
			return nil, err
		}
		for _, mode := range modes.Modes {
			names = append(names, mode.Name)
		}
	}
	return names, nil
}

// Open makes an engine with the mode named exactly name, case included, a Microsoft mode or a TruVoice one, or
// with Mary when no mode is. It reads the limits as the tools do, by setting the minimum and then the maximum
// pitch and speed, which stay set.
func Open(voices *voicedata.Voices, name []byte) (*Voice, error) {
	voice, found, err := open(voices, string(name))
	if err == nil && !found {
		if voice, found, err = open(voices, fallbackVoice); err == nil && !found {
			err = fmt.Errorf("the voices have no %q", fallbackVoice)
		}
	}
	if err != nil {
		return nil, err
	}
	engine := voice.engine
	voice.Pitch.Default, voice.Speed.Default = uint32(engine.Pitch()), engine.Speed()
	engine.SetPitch(0)
	engine.SetSpeed(0)
	voice.Pitch.Minimum, voice.Speed.Minimum = uint32(engine.Pitch()), engine.Speed()
	engine.SetPitch(math.MaxUint16)
	engine.SetSpeed(math.MaxUint32)
	voice.Pitch.Maximum, voice.Speed.Maximum = uint32(engine.Pitch()), engine.Speed()
	return voice, nil
}

func open(voices *voicedata.Voices, name string) (*Voice, bool, error) {
	if voices.Microsoft != nil {
		if mode, found := voices.Microsoft.Mode(name); found {
			engine, err := voices.Microsoft.NewEngine(mode)
			if err != nil {
				return nil, false, err
			}
			return &Voice{Name: mode.Name, engine: engine, microsoft: engine}, true, nil
		}
	}
	if voices.TruVoice == nil {
		return nil, false, nil
	}
	modes, err := voices.TruVoice.LoadModes()
	if err != nil {
		return nil, false, err
	}
	for index, mode := range modes.Modes {
		if mode.Name == name {
			data, err := truvoice.Load(voices.TruVoice)
			if err != nil {
				return nil, false, err
			}
			engine, err := data.NewEngine(index)
			if err != nil {
				return nil, false, err
			}
			return &Voice{Name: mode.Name, engine: engine, truVoice: engine}, true, nil
		}
	}
	return nil, false, nil
}

// Format is the format of the voice's audio, its own.
func (voice *Voice) Format() wave.Format {
	if voice.microsoft != nil {
		return wave.Format(voice.microsoft.Format())
	}
	return wave.Format(voice.truVoice.Format())
}

// Speak speaks a text as the tools do, into a WAV file: it sets the pitch and the speed, where a value out of
// range changes nothing, and speaks the text with tags as one TextData call of the ANSI interface without a
// notification sink.
func (voice *Voice) Speak(pitch uint16, speed uint32, characters []uint16, writer *wave.Writer) error {
	voice.engine.SetPitch(pitch)
	voice.engine.SetSpeed(speed)
	if voice.microsoft != nil {
		return voice.microsoft.Speak(characters, true, text.ANSIText, microsoftSink{Writer: writer})
	}
	if _, err := voice.truVoice.Queue(characters, true, truvoicetext.Text, false); err != nil {
		return err
	}
	return voice.truVoice.Run(truVoiceSink{Writer: writer})
}

type microsoftSink struct {
	*wave.Writer
}

func (sink microsoftSink) Event(whistler.Event) error {
	return nil
}

type truVoiceSink struct {
	*wave.Writer
}

func (sink truVoiceSink) Event(truvoice.Event) error {
	return nil
}

// Atoi reads a number as the C runtime's atoi does: after white space and a sign, the digits up to the first
// other character, and the nearest 32-bit value when the number is outside that range.
func Atoi(value []byte) int32 {
	index := 0
	for index < len(value) && isSpace(value[index]) {
		index++
	}
	negative := false
	if index < len(value) && (value[index] == '+' || value[index] == '-') {
		negative = value[index] == '-'
		index++
	}
	limit := uint64(math.MaxInt32)
	if negative {
		limit++
	}
	var magnitude uint64
	for ; index < len(value) && value[index] >= '0' && value[index] <= '9'; index++ {
		magnitude = min(magnitude*10+uint64(value[index]-'0'), limit)
	}
	if negative {
		return int32(-int64(magnitude))
	}
	return int32(magnitude)
}

func isSpace(character byte) bool {
	return character == ' ' || character >= '\t' && character <= '\r'
}

// FileName is a name of 16 random letters, as the tools give their WAV files.
func FileName() string {
	const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	name := make([]byte, 16)
	for index := range name {
		name[index] = letters[rand.IntN(len(letters))]
	}
	return string(name)
}
