// Package opensapi4 speaks with the voices of a voices folder, the folder of data packs that comes with the
// releases: it lists the voices, and speaks requests in the form the opensapi4 command takes them.
//
// The Microsoft voices' pack is the folder's microsoft folder. The engine itself is package whistler.
package opensapi4

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf16"

	"github.com/phyce/OpenSAPI4/whistler"
	"github.com/phyce/OpenSAPI4/whistler/datapack"
	"github.com/phyce/OpenSAPI4/whistler/text"
)

const (
	MicrosoftFolder = "microsoft"
	DefaultVoice    = "Mary"
)

// Voices are the voices of a voices folder.
type Voices struct {
	microsoft *whistler.Data

	listOnce sync.Once
	list     []Voice
	listErr  error
}

// Voice describes a voice: its mode information, its own format, and its pitch and speed with their ranges.
type Voice struct {
	Name       string `json:"name"`
	Gender     string `json:"gender"`
	Age        int    `json:"age"`
	Speaker    string `json:"speaker"`
	Style      string `json:"style"`
	Language   string `json:"language"`
	SampleRate int    `json:"sampleRate"`
	// Pitch is in Hz, Speed in words per minute.
	Pitch Range `json:"pitch"`
	Speed Range `json:"speed"`
}

type Range struct {
	Default int `json:"default"`
	Minimum int `json:"minimum"`
	Maximum int `json:"maximum"`
}

// Request is a text to speak and how. Only Voice and Text or Texts are needed; the rest default to the
// voice's own settings.
type Request struct {
	// Voice is a voice's name; a name that matches none exactly may match one in another case.
	Voice string `json:"voice,omitempty"`
	Text  string `json:"text,omitempty"`
	// Texts are spoken after Text, one after another, by the same engine into the same audio, so a setting a
	// tag changes in one lasts into the next.
	Texts []string `json:"texts,omitempty"`
	// Pitch in Hz and Speed in words per minute must be in the voice's ranges; 0 gives the minimum, as in
	// SAPI 4.
	Pitch *int `json:"pitch,omitempty"`
	Speed *int `json:"speed,omitempty"`
	// Volume is 0 to 65535, the default.
	Volume *int `json:"volume,omitempty"`
	// Rate and Bits ask for a format other than the voice's own: 22050 Hz voices also give 8000 and 16000 Hz,
	// 8000 Hz voices 11025 Hz, each in 16 or 8 bits.
	Rate int `json:"rate,omitempty"`
	Bits int `json:"bits,omitempty"`
	// Tagged reads tags such as \Spd=200\ in the texts; it is on unless set to false.
	Tagged *bool `json:"tagged,omitempty"`
	// IPA reads the texts as phonetic text in IPA.
	IPA bool `json:"ipa,omitempty"`
	// Inject is a string of tags applied before the texts, as SAPI 4's Inject applies them: \Pit=\, \Spd=\,
	// \Vol=\ and \Rst\.
	Inject string `json:"inject,omitempty"`
}

// RequestError is a request that can't be spoken as it is: an unknown voice, a value out of range, no text.
type RequestError struct {
	message string
}

func (err *RequestError) Error() string {
	return err.message
}

func requestError(format string, arguments ...any) error {
	return &RequestError{message: fmt.Sprintf(format, arguments...)}
}

// Speech is a request ready to speak: an engine with the request's settings, and its texts.
type Speech struct {
	engine       *whistler.Engine
	texts        []string
	tagged       bool
	characterSet text.CharacterSet
}

// Open loads the voices of a voices folder.
func Open(folder string) (*Voices, error) {
	path := filepath.Join(folder, MicrosoftFolder)
	pack, err := datapack.Open(path)
	if err != nil {
		return nil, fmt.Errorf("no Microsoft voices in %s: %w", path, err)
	}
	data, err := whistler.Load(pack)
	if err != nil {
		return nil, fmt.Errorf("load the Microsoft voices from %s: %w", path, err)
	}
	return &Voices{microsoft: data}, nil
}

// Microsoft is the Microsoft voices' data, for what package whistler does beyond requests, such as the user
// lexicon.
func (voices *Voices) Microsoft() *whistler.Data {
	return voices.microsoft
}

// List describes the voices, in the order SAPI 4 lists them. The first call loads every voice.
func (voices *Voices) List() ([]Voice, error) {
	voices.listOnce.Do(func() {
		for index := range voices.microsoft.Modes.Modes {
			mode := &voices.microsoft.Modes.Modes[index]
			if mode.Hidden() {
				continue
			}
			voice, err := voices.describe(mode)
			if err != nil {
				voices.list, voices.listErr = nil, err
				return
			}
			voices.list = append(voices.list, voice)
		}
	})
	return voices.list, voices.listErr
}

func (voices *Voices) describe(mode *datapack.Mode) (Voice, error) {
	engine, err := voices.microsoft.NewEngine(mode)
	if err != nil {
		return Voice{}, fmt.Errorf("voice %s: %w", mode.Name, err)
	}
	voice := Voice{
		Name:       mode.Name,
		Gender:     mode.Gender,
		Age:        int(mode.Age),
		Speaker:    mode.Speaker,
		Style:      mode.Style,
		Language:   language(mode.Language),
		SampleRate: engine.Format().SampleRate,
		Pitch:      Range{Default: int(engine.Pitch())},
		Speed:      Range{Default: int(engine.Speed())},
	}
	engine.SetPitch(0)
	engine.SetSpeed(0)
	voice.Pitch.Minimum, voice.Speed.Minimum = int(engine.Pitch()), int(engine.Speed())
	engine.SetPitch(0xFFFF)
	engine.SetSpeed(0xFFFFFFFF)
	voice.Pitch.Maximum, voice.Speed.Maximum = int(engine.Pitch()), int(engine.Speed())
	return voice, nil
}

func language(identifier uint16) string {
	if identifier == 0x0409 {
		return "en-US"
	}
	return fmt.Sprintf("0x%04X", identifier)
}

func (voices *Voices) mode(name string) (*datapack.Mode, error) {
	if mode, found := voices.microsoft.Mode(name); found {
		return mode, nil
	}
	for index := range voices.microsoft.Modes.Modes {
		if mode := &voices.microsoft.Modes.Modes[index]; !mode.Hidden() && strings.EqualFold(mode.Name, name) {
			return mode, nil
		}
	}
	return nil, requestError("no voice %q", name)
}

var errFormatRefused = errors.New("format refused")

// NewSpeech checks a request and makes its engine. An error in the request is a *RequestError.
func (voices *Voices) NewSpeech(request Request) (*Speech, error) {
	name := request.Voice
	if name == "" {
		name = DefaultVoice
	}
	mode, err := voices.mode(name)
	if err != nil {
		return nil, err
	}
	texts := request.Texts
	if request.Text != "" {
		texts = append([]string{request.Text}, texts...)
	}
	if len(texts) == 0 {
		return nil, requestError("no text")
	}
	var accept whistler.AcceptFormat
	if request.Rate != 0 || request.Bits != 0 {
		if request.Bits != 0 && request.Bits != 8 && request.Bits != 16 {
			return nil, requestError("bits must be 8 or 16, not %d", request.Bits)
		}
		accept = func(format whistler.Format) error {
			if (request.Rate == 0 || format.SampleRate == request.Rate) && (format.Bits == request.Bits || request.Bits == 0 && format.Bits == 16) {
				return nil
			}
			return errFormatRefused
		}
	}
	engine, err := voices.microsoft.NewEngineAccepting(mode, accept)
	if errors.Is(err, errFormatRefused) {
		bits := request.Bits
		if bits == 0 {
			bits = 16
		}
		rate := "its own rate"
		if request.Rate != 0 {
			rate = fmt.Sprintf("%d Hz", request.Rate)
		}
		return nil, requestError("%s can't give %d-bit audio at %s", mode.Name, bits, rate)
	}
	if err != nil {
		return nil, fmt.Errorf("voice %s: %w", mode.Name, err)
	}
	if err := setPitch(engine, request.Pitch); err != nil {
		return nil, err
	}
	if err := setSpeed(engine, request.Speed); err != nil {
		return nil, err
	}
	if request.Volume != nil {
		if *request.Volume < 0 || *request.Volume > 0xFFFF {
			return nil, requestError("volume must be 0 to 65535, not %d", *request.Volume)
		}
		engine.SetVolume(uint32(*request.Volume))
	}
	if request.Inject != "" {
		engine.Inject(request.Inject)
	}
	speech := &Speech{engine: engine, texts: texts, tagged: request.Tagged == nil || *request.Tagged, characterSet: text.UnicodeText}
	if request.IPA {
		speech.characterSet = text.UnicodeIPA
	}
	return speech, nil
}

func setPitch(engine *whistler.Engine, pitch *int) error {
	if pitch == nil {
		return nil
	}
	if *pitch >= 0 && *pitch <= 0xFFFF && engine.SetPitch(uint16(*pitch)) == nil {
		return nil
	}
	low, high := engine.Mode().Pitch/2, 2*engine.Mode().Pitch
	return requestError("pitch %d is outside %s's range, %d to %d Hz", *pitch, engine.Mode().Name, low, high)
}

func setSpeed(engine *whistler.Engine, speed *int) error {
	if speed == nil {
		return nil
	}
	if *speed >= 0 && int64(*speed) <= 0xFFFFFFFF && engine.SetSpeed(uint32(*speed)) == nil {
		return nil
	}
	current := engine.Speed()
	engine.SetSpeed(0)
	low := engine.Speed()
	engine.SetSpeed(0xFFFFFFFF)
	high := engine.Speed()
	engine.SetSpeed(current)
	return requestError("speed %d is outside %s's range, %d to %d words per minute", *speed, engine.Mode().Name, low, high)
}

// Format is the format of the audio the sink gets.
func (speech *Speech) Format() whistler.Format {
	return speech.engine.Format()
}

// Engine is the speech's engine, with the request's settings.
func (speech *Speech) Engine() *whistler.Engine {
	return speech.engine
}

// Speak speaks the texts into the sink, and returns when it has everything. An error from the sink stops the
// speech and is returned.
func (speech *Speech) Speak(sink whistler.Sink) error {
	for _, value := range speech.texts {
		if err := speech.engine.Speak(utf16.Encode([]rune(value)), speech.tagged, speech.characterSet, sink); err != nil {
			return err
		}
	}
	return nil
}
