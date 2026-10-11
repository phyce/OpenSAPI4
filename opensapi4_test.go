package opensapi4

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"unicode/utf16"

	"github.com/phyce/OpenSAPI4/whistler"
	"github.com/phyce/OpenSAPI4/whistler/text"
)

func openVoices(t *testing.T) *Voices {
	t.Helper()
	root := os.Getenv("OPENSAPI4_CONFORMANCE")
	if root == "" {
		t.Skip("OPENSAPI4_CONFORMANCE is not set")
	}
	voices, err := Open(filepath.Join(root, "datapack"))
	if err != nil {
		t.Fatal(err)
	}
	return voices
}

type recorder struct {
	audio  []byte
	events []whistler.Event
}

func (sink *recorder) Audio(data []byte) (int, error) {
	sink.audio = append(sink.audio, data...)
	return len(data), nil
}

func (sink *recorder) Event(event whistler.Event) error {
	sink.events = append(sink.events, event)
	return nil
}

func TestList(t *testing.T) {
	voices := openVoices(t)
	list, err := voices.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 19 {
		t.Fatalf("%d voices, expected 19", len(list))
	}
	expected := map[string]Voice{
		"Mary": {Name: "Mary", Gender: "female", Age: 30, Language: "en-US", SampleRate: 22050, Pitch: Range{169, 84, 338}, Speed: Range{170, 30, 510}},
		"Mike (for Telephone)": {Name: "Mike (for Telephone)", Gender: "male", Age: 30, Language: "en-US", SampleRate: 8000,
			Pitch: Range{113, 56, 226}, Speed: Range{170, 30, 510}},
		"Sam":          {Name: "Sam", Gender: "male", Age: 30, Language: "en-US", SampleRate: 22050, Pitch: Range{100, 50, 200}, Speed: Range{150, 30, 450}},
		"RoboSoft One": {Name: "RoboSoft One", Gender: "male", Age: 30, Language: "en-US", SampleRate: 22050, Pitch: Range{75, 37, 150}, Speed: Range{130, 30, 390}},
	}
	for _, voice := range list {
		want, found := expected[voice.Name]
		if !found {
			continue
		}
		voice.Speaker, voice.Style = "", ""
		if voice != want {
			t.Errorf("%+v, expected %+v", voice, want)
		}
		delete(expected, voice.Name)
	}
	if len(expected) != 0 {
		t.Errorf("missing %v", expected)
	}
	if list[0].Name != "Mary" || list[len(list)-1].Name != "Mike in Space" {
		t.Errorf("order: %s first, %s last", list[0].Name, list[len(list)-1].Name)
	}
}

func number(value int) *int {
	return &value
}

func TestRequestErrors(t *testing.T) {
	voices := openVoices(t)
	for _, request := range []Request{
		{Voice: "Nobody", Text: "Hello."},
		{Voice: "Sam"},
		{Voice: "Sam", Text: "Hello.", Pitch: number(201)},
		{Voice: "Sam", Text: "Hello.", Pitch: number(49)},
		{Voice: "Sam", Text: "Hello.", Pitch: number(-1)},
		{Voice: "Sam", Text: "Hello.", Speed: number(451)},
		{Voice: "Sam", Text: "Hello.", Speed: number(29)},
		{Voice: "Sam", Text: "Hello.", Volume: number(65536)},
		{Voice: "Sam", Text: "Hello.", Rate: 11025},
		{Voice: "Sam", Text: "Hello.", Bits: 24},
		{Voice: "Mary (for Telephone)", Text: "Hello.", Rate: 16000},
	} {
		_, err := voices.NewSpeech(request)
		var requestError *RequestError
		if !errors.As(err, &requestError) {
			t.Errorf("%+v: %v, expected a request error", request, err)
		}
	}
}

func speak(t *testing.T, voices *Voices, request Request) *recorder {
	t.Helper()
	speech, err := voices.NewSpeech(request)
	if err != nil {
		t.Fatal(err)
	}
	sink := &recorder{}
	if err := speech.Speak(sink); err != nil {
		t.Fatal(err)
	}
	return sink
}

// TestSpeech checks that a request speaks as the engine does with the same settings.
func TestSpeech(t *testing.T) {
	voices := openVoices(t)
	data := voices.Microsoft()
	for _, test := range []struct {
		request Request
		mode    string
		format  whistler.Format
		set     func(engine *whistler.Engine)
		texts   []string
		tagged  bool
		ipa     bool
	}{
		{request: Request{Text: "Hello."}, mode: "Mary", texts: []string{"Hello."}, tagged: true},
		{request: Request{Voice: "sam", Text: `Hello \Pit=150\ there.`}, mode: "Sam", texts: []string{`Hello \Pit=150\ there.`}, tagged: true},
		{request: Request{Voice: "Sam", Text: `Hello \Pit=150\ there.`, Tagged: new(bool)}, mode: "Sam", texts: []string{`Hello \Pit=150\ there.`}},
		{request: Request{Voice: "Mike", Text: "One.", Texts: []string{`\Chr="Whisper"\ Two.`, "Three."}, Pitch: number(0), Speed: number(200),
			Volume: number(30000)}, mode: "Mike", texts: []string{"One.", `\Chr="Whisper"\ Two.`, "Three."}, tagged: true,
			set: func(engine *whistler.Engine) {
				engine.SetPitch(0)
				engine.SetSpeed(200)
				engine.SetVolume(30000)
			}},
		{request: Request{Voice: "Sam", Text: "Hello.", Rate: 8000, Bits: 8}, mode: "Sam", format: whistler.Format{SampleRate: 8000, Bits: 8},
			texts: []string{"Hello."}, tagged: true},
		{request: Request{Voice: "Mary (for Telephone)", Text: "Hello.", Rate: 11025}, mode: "Mary (for Telephone)",
			format: whistler.Format{SampleRate: 11025, Bits: 16}, texts: []string{"Hello."}, tagged: true},
		{request: Request{Voice: "Sam", Text: "Hello.", Inject: `\Spd=300\`}, mode: "Sam", texts: []string{"Hello."}, tagged: true,
			set: func(engine *whistler.Engine) { engine.Inject(`\Spd=300\`) }},
		{request: Request{Voice: "Sam", Text: "həˈloʊ", IPA: true}, mode: "Sam", texts: []string{"həˈloʊ"}, tagged: true, ipa: true},
	} {
		got := speak(t, voices, test.request)
		mode, _ := data.Mode(test.mode)
		var accept whistler.AcceptFormat
		if test.format != (whistler.Format{}) {
			accept = func(format whistler.Format) error {
				if format == test.format {
					return nil
				}
				return errFormatRefused
			}
		}
		engine, err := data.NewEngineAccepting(mode, accept)
		if err != nil {
			t.Fatal(err)
		}
		if test.set != nil {
			test.set(engine)
		}
		characterSet := text.UnicodeText
		if test.ipa {
			characterSet = text.UnicodeIPA
		}
		want := &recorder{}
		for _, value := range test.texts {
			if err := engine.Speak(utf16.Encode([]rune(value)), test.tagged, characterSet, want); err != nil {
				t.Fatal(err)
			}
		}
		if len(got.audio) == 0 || !bytes.Equal(got.audio, want.audio) || len(got.events) != len(want.events) {
			t.Errorf("%+v: %d bytes and %d events, expected %d and %d", test.request, len(got.audio), len(got.events), len(want.audio), len(want.events))
		}
	}
}

// TestConcurrentSpeech speaks with many engines of the same voices at once, the voices loading while they do,
// and checks that each gives what it gives alone.
func TestConcurrentSpeech(t *testing.T) {
	requests := []Request{
		{Voice: "Sam", Text: `Hello \Mrk=1\ there, how are you today?`},
		{Voice: "Mary in Hall", Text: "The quick brown fox jumps over the lazy dog."},
		{Voice: "Mike (for Telephone)", Text: "Testing, one, two, three.", Speed: number(250)},
		{Voice: "RoboSoft Three", Text: `\Chr="Whisper"\ A secret.`},
		{Voice: "Female Whisper", Text: "Whispering, 12345.", Rate: 16000, Bits: 8},
	}
	alone := openVoices(t)
	expected := make([]*recorder, len(requests))
	for index, request := range requests {
		expected[index] = speak(t, alone, request)
	}
	voices := openVoices(t)
	failures := make(chan string, 64)
	var group sync.WaitGroup
	for worker := range 8 {
		group.Go(func() {
			for round := range 3 {
				index := (worker + round) % len(requests)
				speech, err := voices.NewSpeech(requests[index])
				if err != nil {
					failures <- err.Error()
					return
				}
				sink := &recorder{}
				if err := speech.Speak(sink); err != nil {
					failures <- err.Error()
					return
				}
				if !bytes.Equal(sink.audio, expected[index].audio) || !slices.Equal(sink.events, expected[index].events) {
					failures <- fmt.Sprintf("worker %d, request %d: different audio or events", worker, index)
				}
			}
		})
	}
	group.Wait()
	close(failures)
	for failure := range failures {
		t.Error(failure)
	}
}
