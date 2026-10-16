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

	"github.com/phyce/OpenSAPI4/truvoice"
	truvoicetext "github.com/phyce/OpenSAPI4/truvoice/text"
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
	format Format
	audio  []byte
	events []Event
}

func (sink *recorder) Audio(data []byte) (int, error) {
	sink.audio = append(sink.audio, data...)
	return len(data), nil
}

func (sink *recorder) Event(event Event) error {
	sink.events = append(sink.events, event)
	return nil
}

func (sink *recorder) same(other *recorder) bool {
	return bytes.Equal(sink.audio, other.audio) && slices.EqualFunc(sink.events, other.events, func(first, second Event) bool {
		firstMouth, secondMouth := first.Visual.Mouth, second.Visual.Mouth
		first.Visual.Mouth, second.Visual.Mouth = nil, nil
		return first == second && (firstMouth == nil) == (secondMouth == nil) && (firstMouth == nil || *firstMouth == *secondMouth)
	})
}

func TestOpen(t *testing.T) {
	root := os.Getenv("OPENSAPI4_CONFORMANCE")
	if root == "" {
		t.Skip("OPENSAPI4_CONFORMANCE is not set")
	}
	if _, err := Open(t.TempDir()); err == nil {
		t.Error("a folder without voices opened")
	}
	folder := t.TempDir()
	if err := os.CopyFS(filepath.Join(folder, TruVoiceFolder), os.DirFS(filepath.Join(root, "datapack", TruVoiceFolder))); err != nil {
		t.Fatal(err)
	}
	voices, err := Open(folder)
	if err != nil {
		t.Fatal(err)
	}
	if voices.Microsoft() != nil || voices.TruVoice() == nil {
		t.Error("expected the TruVoice voices alone")
	}
	list, err := voices.List()
	if err != nil || len(list) != 10 {
		t.Fatalf("%d voices, %v, expected 10", len(list), err)
	}
	if _, err := voices.NewSpeech(Request{Text: "Hello."}); err == nil {
		t.Error("the default voice spoke without the Microsoft voices")
	}
	if _, err := voices.NewSpeech(Request{Voice: "Peter", Text: "Hello."}); err != nil {
		t.Error(err)
	}
}

func TestList(t *testing.T) {
	voices := openVoices(t)
	list, err := voices.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 29 {
		t.Fatalf("%d voices, expected 29", len(list))
	}
	expected := map[string]Voice{
		"Mary": {Name: "Mary", Engine: "microsoft", Gender: "female", Age: 30, Language: "en-US", SampleRate: 22050, Pitch: Range{169, 84, 338},
			Speed: Range{170, 30, 510}},
		"Mike (for Telephone)": {Name: "Mike (for Telephone)", Engine: "microsoft", Gender: "male", Age: 30, Language: "en-US", SampleRate: 8000,
			Pitch: Range{113, 56, 226}, Speed: Range{170, 30, 510}},
		"Sam": {Name: "Sam", Engine: "microsoft", Gender: "male", Age: 30, Language: "en-US", SampleRate: 22050, Pitch: Range{100, 50, 200},
			Speed: Range{150, 30, 450}},
		"RoboSoft One": {Name: "RoboSoft One", Engine: "microsoft", Gender: "male", Age: 30, Language: "en-US", SampleRate: 22050,
			Pitch: Range{75, 37, 150}, Speed: Range{130, 30, 390}},
		"Adult Male #1, American English (TruVoice)": {Name: "Adult Male #1, American English (TruVoice)", Engine: "truvoice", Gender: "male",
			Age: 30, Language: "en-US", SampleRate: 11025, Pitch: Range{85, 50, 400}, Speed: Range{150, 50, 250}},
		"Adult Male #6, American English (TruVoice)": {Name: "Adult Male #6, American English (TruVoice)", Engine: "truvoice", Gender: "male",
			Age: 30, Language: "en-US", SampleRate: 11025, Pitch: Range{89, 50, 400}, Speed: Range{120, 50, 250}},
		"Adult Female #2, American English (TruVoice)": {Name: "Adult Female #2, American English (TruVoice)", Engine: "truvoice",
			Gender: "female", Age: 30, Language: "en-US", SampleRate: 11025, Pitch: Range{152, 50, 400}, Speed: Range{150, 50, 250}},
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
	if list[0].Name != "Mary" || list[18].Name != "Mike in Space" || list[19].Speaker != "Peter" || list[28].Speaker != "Julia" {
		t.Errorf("order: %s first, %s, %s, %s last", list[0].Name, list[18].Name, list[19].Speaker, list[28].Speaker)
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
		{Voice: "Peter", Text: "Hello.", Pitch: number(401)},
		{Voice: "Peter", Text: "Hello.", Speed: number(49)},
		{Voice: "Peter", Text: "Hello.", Rate: 22050},
		{Voice: "Business", Text: "Hello."},
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
	sink := &recorder{format: speech.Format()}
	if err := speech.Speak(sink); err != nil {
		t.Fatal(err)
	}
	return sink
}

// TestSpeech checks that a request speaks as the engine does with the same settings.
func TestSpeech(t *testing.T) {
	voices := openVoices(t)
	const peter, wanda, julia = "Adult Male #1, American English (TruVoice)", "Adult Female #1, American English (TruVoice)",
		"Adult Female #2, American English (TruVoice)"
	for _, test := range []struct {
		request Request
		voice   string
		format  Format
		set     func(engine engine)
		texts   []string
		tagged  bool
		ipa     bool
	}{
		{request: Request{Text: "Hello."}, voice: "Mary", texts: []string{"Hello."}, tagged: true},
		{request: Request{Voice: "sam", Text: `Hello \Pit=150\ there.`}, voice: "Sam", texts: []string{`Hello \Pit=150\ there.`}, tagged: true},
		{request: Request{Voice: "Sam", Text: `Hello \Pit=150\ there.`, Tagged: new(bool)}, voice: "Sam", texts: []string{`Hello \Pit=150\ there.`}},
		{request: Request{Voice: "Mike", Text: "One.", Texts: []string{`\Chr="Whisper"\ Two.`, "Three."}, Pitch: number(0), Speed: number(200),
			Volume: number(30000)}, voice: "Mike", texts: []string{"One.", `\Chr="Whisper"\ Two.`, "Three."}, tagged: true,
			set: func(engine engine) {
				engine.SetPitch(0)
				engine.SetSpeed(200)
				engine.SetVolume(30000)
			}},
		{request: Request{Voice: "Sam", Text: "Hello.", Rate: 8000, Bits: 8}, voice: "Sam", format: Format{SampleRate: 8000, Bits: 8},
			texts: []string{"Hello."}, tagged: true},
		{request: Request{Voice: "Mary (for Telephone)", Text: "Hello.", Rate: 11025}, voice: "Mary (for Telephone)",
			format: Format{SampleRate: 11025, Bits: 16}, texts: []string{"Hello."}, tagged: true},
		{request: Request{Voice: "Sam", Text: "Hello.", Inject: `\Spd=300\`}, voice: "Sam", texts: []string{"Hello."}, tagged: true,
			set: func(engine engine) { engine.Inject(`\Spd=300\`) }},
		{request: Request{Voice: "Sam", Text: "həˈloʊ", IPA: true}, voice: "Sam", texts: []string{"həˈloʊ"}, tagged: true, ipa: true},
		{request: Request{Voice: "peter", Text: "Hello there, 12 people."}, voice: peter, texts: []string{"Hello there, 12 people."}, tagged: true},
		{request: Request{Voice: "adult female #1, american english (truvoice)", Text: `Hello \Pit=200\ there \Mrk=7\ again.`}, voice: wanda,
			texts: []string{`Hello \Pit=200\ there \Mrk=7\ again.`}, tagged: true},
		{request: Request{Voice: "Wanda", Text: `Hello \Pit=200\ there.`, Tagged: new(bool)}, voice: wanda, texts: []string{`Hello \Pit=200\ there.`}},
		{request: Request{Voice: "Julia", Text: "One.", Texts: []string{`\Vce=Speaker=Biff\ Two.`, "Three."}, Pitch: number(0), Speed: number(220),
			Volume: number(20000)}, voice: julia, texts: []string{"One.", `\Vce=Speaker=Biff\ Two.`, "Three."}, tagged: true,
			set: func(engine engine) {
				engine.SetPitch(0)
				engine.SetSpeed(220)
				engine.SetVolume(20000)
			}},
		{request: Request{Voice: "Julia", Text: "Hello.", Rate: 8000, Bits: 8}, voice: julia, format: Format{SampleRate: 8000, Bits: 8},
			texts: []string{"Hello."}, tagged: true},
		{request: Request{Voice: "Peter", Text: "Hello.", Bits: 8}, voice: peter, format: Format{SampleRate: 11025, Bits: 8},
			texts: []string{"Hello."}, tagged: true},
		{request: Request{Voice: "Peter", Text: "Hello.", Inject: `\Spd=220\\Chr=Monotone\`}, voice: peter, texts: []string{"Hello."}, tagged: true,
			set: func(engine engine) { engine.Inject(`\Spd=220\\Chr=Monotone\`) }},
		{request: Request{Voice: "Peter", Text: "həˈloʊ", IPA: true}, voice: peter, texts: []string{"həˈloʊ"}, tagged: true, ipa: true},
	} {
		got := speak(t, voices, test.request)
		choice, err := voices.find(test.voice)
		if err != nil {
			t.Fatal(err)
		}
		want := &recorder{}
		if test.format != (Format{}) && got.format != test.format {
			t.Errorf("%+v: %+v, expected %+v", test.request, got.format, test.format)
		}
		accept := func(format Format) error {
			if test.format == (Format{}) || format == test.format {
				return nil
			}
			return errFormatRefused
		}
		var settings engine
		var speakText func(characters []uint16) error
		if choice.microsoft != nil {
			engine, err := voices.Microsoft().NewEngineAccepting(choice.microsoft, func(format whistler.Format) error { return accept(Format(format)) })
			if err != nil {
				t.Fatal(err)
			}
			characterSet := text.UnicodeText
			if test.ipa {
				characterSet = text.UnicodeIPA
			}
			settings = engine
			speakText = func(characters []uint16) error {
				return engine.Speak(characters, test.tagged, characterSet, microsoftSink{sink: want})
			}
		} else {
			engine, err := voices.TruVoice().NewProcess().NewEngineAccepting(choice.truVoice, func(format truvoice.Format) error { return accept(Format(format)) })
			if err != nil {
				t.Fatal(err)
			}
			characterSet := truvoicetext.Text
			if test.ipa {
				characterSet = truvoicetext.IPA
			}
			settings = engine
			speakText = func(characters []uint16) error {
				return engine.Speak(characters, test.tagged, characterSet, truVoiceSink{sink: want})
			}
		}
		if test.set != nil {
			test.set(settings)
		}
		for _, value := range test.texts {
			if err := speakText(utf16.Encode([]rune(value))); err != nil {
				t.Fatal(err)
			}
		}
		if len(got.audio) == 0 || !got.same(want) {
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
		{Voice: "Peter", Text: `Hello \Mrk=1\ there, how are you today? \Rst\ Fine.`},
		{Voice: "Wanda", Text: "The quick brown fox jumps over the lazy dog. The end... Or is it?"},
		{Voice: "Amos", Text: `\Vce=Speaker=Julia\ Testing, one, two, three.`, Speed: number(200)},
		{Voice: "Julia", Text: "Whispering, 12345.", Rate: 8000, Bits: 8},
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
				index := (worker*3 + round) % len(requests)
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
				if !sink.same(expected[index]) {
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

// TestSeparateProcesses checks that what a TruVoice request's tags change for the whole process, the user lexicon and
// the voice \Rst\ goes back to, doesn't reach the next request, and that the voices' user lexicon reaches each one.
func TestSeparateProcesses(t *testing.T) {
	voices := openVoices(t)
	tomato := Request{Voice: "Peter", Text: "Tomato."}
	reset := Request{Voice: "Julia", Text: `\Rst\ Two.`}
	tomatoBefore, resetBefore := speak(t, voices, tomato), speak(t, voices, reset)
	pronounced := speak(t, voices, Request{Voice: "Peter", Text: `\Prn=Tomato=tomahto\ Tomato.`})
	speak(t, voices, Request{Voice: "Peter", Text: `\Vce=Speaker=Julia\ One.`})
	if bytes.Equal(pronounced.audio, tomatoBefore.audio) {
		t.Error(`\Prn= changed nothing`)
	}
	if !speak(t, voices, tomato).same(tomatoBefore) {
		t.Error(`a request's \Prn= reached the next request`)
	}
	if !speak(t, voices, reset).same(resetBefore) {
		t.Error(`a request's \Vce= reached the next request's \Rst\`)
	}
	if err := voices.TruVoice().Lexicon().Add(truvoicetext.Text, utf16.Encode([]rune("Tomato")), utf16.Encode([]rune("tomahto"))); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(speak(t, voices, tomato).audio, pronounced.audio) {
		t.Error("the voices' user lexicon didn't reach the request")
	}
}
