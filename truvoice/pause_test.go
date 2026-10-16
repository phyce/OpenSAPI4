package truvoice

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"unicode/utf16"

	"github.com/phyce/OpenSAPI4/truvoice/internal/conformance"
	"github.com/phyce/OpenSAPI4/truvoice/text"
)

func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	data, err := Load(conformance.Pack(t))
	if err != nil {
		t.Fatal(err)
	}
	engine, err := data.NewEngine(0)
	if err != nil {
		t.Fatal(err)
	}
	return engine
}

// TestResetQueued resets an engine whose texts no speaking thread has taken, as AudioReset right after TextData
// does: each text with a notification sink is done and aborted, and nothing is left to speak.
func TestResetQueued(t *testing.T) {
	engine := newTestEngine(t)
	for _, notify := range []bool{true, false, true} {
		if _, err := engine.Queue(utf16.Encode([]rune("Hello there.")), true, text.Text, notify); err != nil {
			t.Fatal(err)
		}
	}
	sink := &recorder{numbers: map[int]int{0: 0, 2: 2}, limit: -1}
	engine.Reset(sink)
	if err := engine.Run(sink); err != nil {
		t.Fatal(err)
	}
	expected := []string{"aborted 0 0", "aborted 2 0"}
	if !slices.Equal(sink.positions, expected) || len(sink.pcm) > 0 {
		t.Errorf("events %v and %d bytes, expected %v and none", sink.positions, len(sink.pcm), expected)
	}
}

// pausingSink pauses the engine from Audio once it has taken a number of buffers, and counts the buffers it is
// given while the engine is paused. It keeps the items' events with their samples apart from the texts' starts
// and ends, which come at the position where the window holds the audio.
type pausingSink struct {
	engine      *Engine
	pauseAfter  int
	buffers     int
	whilePaused int
	pcm         []byte
	items       []string
	messages    []string
}

func (sink *pausingSink) Audio(data []byte) (int, error) {
	if sink.engine.Paused() {
		sink.whilePaused++
	}
	sink.pcm = append(sink.pcm, data...)
	sink.buffers++
	if sink.buffers == sink.pauseAfter {
		return len(data), sink.engine.Pause()
	}
	return len(data), nil
}

func (sink *pausingSink) Event(event Event) error {
	switch event.Kind {
	case TextStarted, TextDone:
		sink.messages = append(sink.messages, fmt.Sprint(event.Kind, event.Text, event.Aborted))
	default:
		sink.items = append(sink.items, fmt.Sprint(event.Kind, event.Sample, event.Value, event.Visual.Engine))
	}
	return nil
}

func speakPausing(t *testing.T, value string, pauseAfter int) *pausingSink {
	t.Helper()
	engine := newTestEngine(t)
	sink := &pausingSink{engine: engine, pauseAfter: pauseAfter}
	err := engine.SpeakString(value, sink)
	if pauseAfter == 0 {
		if err != nil {
			t.Fatal(err)
		}
		return sink
	}
	if !errors.Is(err, ErrPaused) {
		t.Fatalf("pause after %d buffers: Speak returned %v, expected ErrPaused", pauseAfter, err)
	}
	if err := engine.Resume(sink); err != nil || engine.Paused() {
		t.Fatalf("pause after %d buffers: Resume returned %v, paused %v", pauseAfter, err, engine.Paused())
	}
	return sink
}

// TestSinkPause pauses the engine from the sink's Audio, as AudioPause made while the window gives the destination
// a buffer: the sink gets no audio until Resume, Speak returns ErrPaused, also when the pause comes at the last
// buffer, after the thread has ended, and after Resume the audio and the events are those of a run without a pause.
func TestSinkPause(t *testing.T) {
	for _, value := range []string{"Hello there.", "Dr. Smith lives at 221 Baker St. in London, and he reads every evening."} {
		plain := speakPausing(t, value, 0)
		for _, pauseAfter := range []int{1, 3, plain.buffers} {
			paused := speakPausing(t, value, pauseAfter)
			switch {
			case paused.whilePaused > 0:
				t.Errorf("%q, pause after %d buffers: %d buffers given while paused", value, pauseAfter, paused.whilePaused)
			case !slices.Equal(paused.pcm, plain.pcm):
				t.Errorf("%q, pause after %d buffers: %d bytes of audio, expected %d", value, pauseAfter, len(paused.pcm), len(plain.pcm))
			case !slices.Equal(paused.items, plain.items):
				t.Errorf("%q, pause after %d buffers: events %v, expected %v", value, pauseAfter, paused.items, plain.items)
			case !slices.Equal(paused.messages, plain.messages):
				t.Errorf("%q, pause after %d buffers: texts %v, expected %v", value, pauseAfter, paused.messages, plain.messages)
			}
		}
	}
}
