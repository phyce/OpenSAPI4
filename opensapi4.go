// Package opensapi4 speaks with the voices of a voices folder, the folder of data packs that comes with the
// releases: it lists the voices, and speaks requests in the form the opensapi4 command takes them.
//
// The Microsoft voices' pack is the folder's microsoft folder, and L&H TruVoice's its truvoice folder. The engines
// themselves are packages whistler and truvoice.
package opensapi4

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf16"

	"github.com/phyce/OpenSAPI4/truvoice"
	truvoicedatapack "github.com/phyce/OpenSAPI4/truvoice/datapack"
	truvoicetext "github.com/phyce/OpenSAPI4/truvoice/text"
	"github.com/phyce/OpenSAPI4/whistler"
	"github.com/phyce/OpenSAPI4/whistler/datapack"
	"github.com/phyce/OpenSAPI4/whistler/text"
)

const (
	MicrosoftFolder = "microsoft"
	TruVoiceFolder  = "truvoice"
	DefaultVoice    = "Mary"
)

// Voices are the voices of a voices folder.
type Voices struct {
	microsoft *whistler.Data
	truVoice  *truvoice.Data
	choices   []choice

	listOnce sync.Once
	list     []Voice
	listErr  error
}

// choice is a voice of one of the engines: a Microsoft mode, or else a TruVoice voice's number.
type choice struct {
	name      string
	speaker   string
	microsoft *datapack.Mode
	truVoice  int
}

// Voice describes a voice: its engine (the name of its pack's folder, microsoft or truvoice), its mode information,
// its own format, and its pitch and speed with their ranges.
type Voice struct {
	Name       string `json:"name"`
	Engine     string `json:"engine"`
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
	// Voice is a voice's name; a name that matches none exactly may match one in another case, or the speaker of
	// one voice alone, such as TruVoice's Peter.
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
	// 8000 Hz voices 11025 Hz, TruVoice's 11025 Hz voices 8000 Hz, each in 16 or 8 bits.
	Rate int `json:"rate,omitempty"`
	Bits int `json:"bits,omitempty"`
	// Tagged reads tags such as \Spd=200\ in the texts; it is on unless set to false.
	Tagged *bool `json:"tagged,omitempty"`
	// IPA reads the texts as phonetic text in IPA.
	IPA bool `json:"ipa,omitempty"`
	// Inject is a string of tags applied before the texts, as SAPI 4's Inject applies them: \Pit=\, \Spd=\,
	// \Vol=\ and \Rst\, and for TruVoice also \Chr=\ and \Vce=\.
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
	format    Format
	texts     []string
	tagged    bool
	ipa       bool
	microsoft *whistler.Engine
	truVoice  *truvoice.Engine
}

// engine is what the two engines' requests set.
type engine interface {
	Pitch() uint16
	SetPitch(hertz uint16) error
	Speed() uint32
	SetSpeed(wordsPerMinute uint32) error
	SetVolume(value uint32)
	Inject(tags string)
}

// Open loads the voices of a voices folder: its microsoft folder, its truvoice folder, or both.
func Open(folder string) (*Voices, error) {
	voices := &Voices{}
	path := filepath.Join(folder, MicrosoftFolder)
	if found, err := present(path); err != nil {
		return nil, err
	} else if found {
		pack, err := datapack.Open(path)
		if err != nil {
			return nil, fmt.Errorf("no Microsoft voices in %s: %w", path, err)
		}
		if voices.microsoft, err = whistler.Load(pack); err != nil {
			return nil, fmt.Errorf("load the Microsoft voices from %s: %w", path, err)
		}
		for index := range voices.microsoft.Modes.Modes {
			if mode := &voices.microsoft.Modes.Modes[index]; !mode.Hidden() {
				voices.choices = append(voices.choices, choice{name: mode.Name, speaker: mode.Speaker, microsoft: mode})
			}
		}
	}
	path = filepath.Join(folder, TruVoiceFolder)
	if found, err := present(path); err != nil {
		return nil, err
	} else if found {
		pack, err := truvoicedatapack.Open(path)
		if err != nil {
			return nil, fmt.Errorf("no TruVoice voices in %s: %w", path, err)
		}
		if voices.truVoice, err = truvoice.Load(pack); err != nil {
			return nil, fmt.Errorf("load the TruVoice voices from %s: %w", path, err)
		}
		for index, mode := range voices.truVoice.Modes.Modes {
			voices.choices = append(voices.choices, choice{name: mode.Name, speaker: mode.Speaker, truVoice: index})
		}
	}
	if voices.microsoft == nil && voices.truVoice == nil {
		return nil, fmt.Errorf("no voices in %s: it has no %s or %s folder", folder, MicrosoftFolder, TruVoiceFolder)
	}
	return voices, nil
}

func present(path string) (bool, error) {
	_, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// Microsoft is the Microsoft voices' data, for what package whistler does beyond requests, such as the user
// lexicon; nil when the folder has none.
func (voices *Voices) Microsoft() *whistler.Data {
	return voices.microsoft
}

// TruVoice is the TruVoice voices' data, for what package truvoice does beyond requests, such as the user lexicon;
// nil when the folder has none. Each TruVoice speech starts with a copy of its user lexicon.
func (voices *Voices) TruVoice() *truvoice.Data {
	return voices.truVoice
}

// List describes the voices, in the order SAPI 4 lists them: the Microsoft voices, then TruVoice's. The first
// call loads every Microsoft voice.
func (voices *Voices) List() ([]Voice, error) {
	voices.listOnce.Do(func() {
		for _, choice := range voices.choices {
			voice, err := voices.describe(choice)
			if err != nil {
				voices.list, voices.listErr = nil, err
				return
			}
			voices.list = append(voices.list, voice)
		}
	})
	return voices.list, voices.listErr
}

func (voices *Voices) describe(choice choice) (Voice, error) {
	if mode := choice.microsoft; mode != nil {
		engine, err := voices.microsoft.NewEngine(mode)
		if err != nil {
			return Voice{}, fmt.Errorf("voice %s: %w", mode.Name, err)
		}
		return describe(engine, Voice{Name: mode.Name, Engine: MicrosoftFolder, Gender: mode.Gender, Age: int(mode.Age), Speaker: mode.Speaker,
			Style: mode.Style, Language: language(mode.Language), SampleRate: engine.Format().SampleRate}), nil
	}
	mode := &voices.truVoice.Modes.Modes[choice.truVoice]
	engine, err := voices.truVoice.NewEngine(choice.truVoice)
	if err != nil {
		return Voice{}, fmt.Errorf("voice %s: %w", mode.Name, err)
	}
	return describe(engine, Voice{Name: mode.Name, Engine: TruVoiceFolder, Gender: gender(mode.Gender), Age: int(mode.Age), Speaker: mode.Speaker,
		Style: mode.Style, Language: language(mode.Language), SampleRate: engine.Format().SampleRate}), nil
}

// describe reads a new engine's pitch and speed, and their ranges by setting the lowest and the highest.
func describe(engine engine, voice Voice) Voice {
	voice.Pitch.Default, voice.Speed.Default = int(engine.Pitch()), int(engine.Speed())
	engine.SetPitch(0)
	engine.SetSpeed(0)
	voice.Pitch.Minimum, voice.Speed.Minimum = int(engine.Pitch()), int(engine.Speed())
	engine.SetPitch(0xFFFF)
	engine.SetSpeed(0xFFFFFFFF)
	voice.Pitch.Maximum, voice.Speed.Maximum = int(engine.Pitch()), int(engine.Speed())
	return voice
}

func gender(value uint16) string {
	switch value {
	case 1:
		return "female"
	case 2:
		return "male"
	}
	return "neutral"
}

func language(identifier uint16) string {
	if identifier == 0x0409 {
		return "en-US"
	}
	return fmt.Sprintf("0x%04X", identifier)
}

// find finds a voice by its name, then by its name in another case, then by a speaker only one voice has.
func (voices *Voices) find(name string) (choice, error) {
	for _, choice := range voices.choices {
		if choice.name == name {
			return choice, nil
		}
	}
	for _, choice := range voices.choices {
		if strings.EqualFold(choice.name, name) {
			return choice, nil
		}
	}
	var found []choice
	for _, choice := range voices.choices {
		if strings.EqualFold(choice.speaker, name) {
			found = append(found, choice)
		}
	}
	if len(found) == 1 {
		return found[0], nil
	}
	return choice{}, requestError("no voice %q", name)
}

var errFormatRefused = errors.New("format refused")

// NewSpeech checks a request and makes its engine. An error in the request is a *RequestError. A TruVoice speech
// speaks as in a new process of the original, so that what another speech's texts change, its user lexicon
// included, doesn't change it.
func (voices *Voices) NewSpeech(request Request) (*Speech, error) {
	name := request.Voice
	if name == "" {
		name = DefaultVoice
	}
	choice, err := voices.find(name)
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
	if request.Bits != 0 && request.Bits != 8 && request.Bits != 16 {
		return nil, requestError("bits must be 8 or 16, not %d", request.Bits)
	}
	speech := &Speech{texts: texts, tagged: request.Tagged == nil || *request.Tagged, ipa: request.IPA}
	var settings engine
	if mode := choice.microsoft; mode != nil {
		var accept whistler.AcceptFormat
		if request.Rate != 0 || request.Bits != 0 {
			accept = func(format whistler.Format) error { return request.accept(Format(format)) }
		}
		speech.microsoft, err = voices.microsoft.NewEngineAccepting(mode, accept)
		if err == nil {
			speech.format, settings = Format(speech.microsoft.Format()), speech.microsoft
		}
	} else {
		var accept truvoice.AcceptFormat
		if request.Rate != 0 || request.Bits != 0 {
			accept = func(format truvoice.Format) error { return request.accept(Format(format)) }
		}
		speech.truVoice, err = voices.truVoice.NewProcess().NewEngineAccepting(choice.truVoice, accept)
		if err == nil {
			speech.format, settings = Format(speech.truVoice.Format()), speech.truVoice
		}
	}
	if errors.Is(err, errFormatRefused) || errors.Is(err, truvoice.ErrWaveFormatNotSupported) {
		bits := request.Bits
		if bits == 0 {
			bits = 16
		}
		rate := "its own rate"
		if request.Rate != 0 {
			rate = fmt.Sprintf("%d Hz", request.Rate)
		}
		return nil, requestError("%s can't give %d-bit audio at %s", choice.name, bits, rate)
	}
	if err != nil {
		return nil, fmt.Errorf("voice %s: %w", choice.name, err)
	}
	if err := setPitch(settings, request.Pitch, choice.name); err != nil {
		return nil, err
	}
	if err := setSpeed(settings, request.Speed, choice.name); err != nil {
		return nil, err
	}
	if request.Volume != nil {
		if *request.Volume < 0 || *request.Volume > 0xFFFF {
			return nil, requestError("volume must be 0 to 65535, not %d", *request.Volume)
		}
		settings.SetVolume(uint32(*request.Volume))
	}
	if request.Inject != "" {
		settings.Inject(request.Inject)
	}
	return speech, nil
}

// accept takes the format the request asks for: its rate, or any, and its bits, or 16.
func (request Request) accept(format Format) error {
	if (request.Rate == 0 || format.SampleRate == request.Rate) && (format.Bits == request.Bits || request.Bits == 0 && format.Bits == 16) {
		return nil
	}
	return errFormatRefused
}

func setPitch(engine engine, pitch *int, name string) error {
	if pitch == nil {
		return nil
	}
	if *pitch >= 0 && *pitch <= 0xFFFF && engine.SetPitch(uint16(*pitch)) == nil {
		return nil
	}
	current := engine.Pitch()
	engine.SetPitch(0)
	low := engine.Pitch()
	engine.SetPitch(0xFFFF)
	high := engine.Pitch()
	engine.SetPitch(current)
	return requestError("pitch %d is outside %s's range, %d to %d Hz", *pitch, name, low, high)
}

func setSpeed(engine engine, speed *int, name string) error {
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
	return requestError("speed %d is outside %s's range, %d to %d words per minute", *speed, name, low, high)
}

// Format is the format of the audio the sink gets.
func (speech *Speech) Format() Format {
	return speech.format
}

// Microsoft is the speech's engine, with the request's settings, for a Microsoft voice; nil for a TruVoice voice.
func (speech *Speech) Microsoft() *whistler.Engine {
	return speech.microsoft
}

// TruVoice is the speech's engine, with the request's settings, for a TruVoice voice; nil for a Microsoft voice.
func (speech *Speech) TruVoice() *truvoice.Engine {
	return speech.truVoice
}

// Speak speaks the texts into the sink, and returns when it has everything. An error from the sink stops the
// speech and is returned.
func (speech *Speech) Speak(sink Sink) error {
	for _, value := range speech.texts {
		characters := utf16.Encode([]rune(value))
		var err error
		if speech.microsoft != nil {
			characterSet := text.UnicodeText
			if speech.ipa {
				characterSet = text.UnicodeIPA
			}
			err = speech.microsoft.Speak(characters, speech.tagged, characterSet, microsoftSink{sink: sink})
		} else {
			characterSet := truvoicetext.Text
			if speech.ipa {
				characterSet = truvoicetext.IPA
			}
			err = speech.truVoice.Speak(characters, speech.tagged, characterSet, truVoiceSink{sink: sink})
		}
		if err != nil {
			return err
		}
	}
	return nil
}
