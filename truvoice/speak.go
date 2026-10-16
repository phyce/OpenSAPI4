package truvoice

import (
	"errors"
	"io"
	"unicode/utf16"

	"github.com/phyce/OpenSAPI4/truvoice/input"
	"github.com/phyce/OpenSAPI4/truvoice/lexical"
	"github.com/phyce/OpenSAPI4/truvoice/normalizer"
	"github.com/phyce/OpenSAPI4/truvoice/parameters"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
	"github.com/phyce/OpenSAPI4/truvoice/prosody"
	"github.com/phyce/OpenSAPI4/truvoice/synthesizer"
	"github.com/phyce/OpenSAPI4/truvoice/text"
)

// SpeakString speaks a text with tags, as Speak does.
func (engine *Engine) SpeakString(value string, sink Sink) error {
	return engine.Speak(utf16.Encode([]rune(value)), true, text.Text, sink)
}

// Speak speaks one text, as one TextData call of the wide interface with a notification sink does, and returns as
// Run does: when the speaking thread ends, when the sink has everything of the text and of the texts queued before
// it. While the engine is paused, Speak queues the text and returns ErrPaused.
func (engine *Engine) Speak(characters []uint16, tagged bool, characterSet text.CharacterSet, sink Sink) error {
	if _, err := engine.Queue(characters, tagged, characterSet, true); err != nil {
		return err
	}
	return engine.Run(sink)
}

// Queue converts a text as TextData of the wide interface does (engine.md §1) and queues it for the next speaking
// thread, which Run runs. Characters are UTF-16 code units. A text with notify has a notification sink: it gets word
// marks, and its events (but the Visual ones) go to the sink. Queue returns the text's number, the number of texts
// queued before it, and text.ErrInvalidCharacter for a character set TextData doesn't take, which queues nothing.
func (engine *Engine) Queue(characters []uint16, tagged bool, characterSet text.CharacterSet, notify bool) (int, error) {
	engine.converter.Voice = engine.state.Voice
	converted, err := engine.converter.Wide(characters, characterSet, flags(tagged), notify)
	if err != nil {
		return 0, err
	}
	return engine.queue(converted, notify), nil
}

// QueueANSI converts a text as TextData of the ANSI interface does: characters are code page 1252's, and IPA gives
// text.ErrInvalidCharacter. Otherwise it is Queue.
func (engine *Engine) QueueANSI(characters []byte, tagged bool, characterSet text.CharacterSet, notify bool) (int, error) {
	engine.converter.Voice = engine.state.Voice
	converted, err := engine.converter.ANSI(characters, characterSet, flags(tagged), notify)
	if err != nil {
		return 0, err
	}
	return engine.queue(converted, notify), nil
}

func flags(tagged bool) uint32 {
	if tagged {
		return text.Tagged
	}
	return 0
}

func (engine *Engine) queue(converted []byte, notify bool) int {
	number := engine.textCount
	engine.textCount++
	queued := pipeline.QueuedText{Characters: converted}
	if notify {
		queued.Text = &pipeline.Text{Number: number}
	}
	engine.state.Texts = append(engine.state.Texts, queued)
	return number
}

// Run runs a speaking thread, which speaks the queued texts with a new pipeline (pipeline.md §8), and gives the
// audio and the events to the sink in stream order, as the engine's window does when the thread flushes its queue
// (engine.md §2.3). It returns when the thread ends, after the last text. When the sink pauses the engine, Run
// returns ErrPaused, and Resume goes on (engine.md §5). Any other error from the sink resets the engine as Reset
// does, after which Run returns the error.
func (engine *Engine) Run(sink Sink) error {
	if engine.thread == nil && len(engine.state.Texts) > 0 {
		engine.thread = engine.newThread()
	}
	if engine.paused {
		return ErrPaused
	}
	return engine.run(sink)
}

// run goes round the speaking thread's loop (pipeline.md §8.2) until it ends, or waits in a pause: at once when
// the engine was paused before it ran, and after the hand-over that finds the queue full when the sink paused it.
func (engine *Engine) run(sink Sink) error {
	engine.running = true
	defer func() { engine.running = false }()
	if err := engine.deliver(sink); err != nil {
		engine.reset(sink)
		return err
	}
	for thread := engine.thread; thread != nil; thread = engine.thread {
		if engine.paused && !engine.runningOn {
			return ErrPaused
		}
		if thread.input.Turn() {
			thread.pipeline.ApplyAttributes()
			result := thread.pipeline.Step()
			if result&pipeline.StepHandOver != 0 && thread.pipeline.HandOver() {
				engine.runningOn, engine.waitingForWindow = false, true
			}
			if result&pipeline.StepIdle != 0 {
				thread.input.Busy = false
			}
		} else {
			engine.thread = nil
			engine.state.Flush()
		}
		if err := engine.deliver(sink); err != nil {
			engine.reset(sink)
			return err
		}
	}
	if engine.paused {
		return ErrPaused
	}
	return nil
}

// thread is a speaking thread's pipeline and stages.
type thread struct {
	pipeline   *pipeline.Pipeline
	input      *input.Stage
	normalizer *normalizer.Stage
	lexical    *lexical.Stage
	prosody    *prosody.Stage
}

func (engine *Engine) newThread() *thread {
	thread := engine.data.newThread(&engine.state, engine.state.SampleRate)
	thread.pipeline.Start()
	return thread
}

// newThread makes a pipeline and its stages for an engine's state, or for none.
func (data *Data) newThread(state *pipeline.Engine, sampleRate int) *thread {
	thread := &thread{}
	thread.pipeline = pipeline.New(data.pipeline, synthesizer.New(data.synthesizer, sampleRate), thread, state)
	thread.pipeline.Generator = parameters.New(data.parameters, data.vowels, thread.pipeline)
	thread.input = input.New(data.input, thread.pipeline)
	thread.normalizer = normalizer.New(data.normalizer, data.parameters, thread.pipeline)
	thread.lexical = lexical.New(data.lexical, data.dictionary, data.parameters, thread.pipeline)
	thread.prosody = prosody.New(data.prosody, data.parameters, thread.pipeline)
	thread.prosody.Shared = data.shared
	return thread
}

func (thread *thread) Input() bool      { return thread.input.MakeRecords() }
func (thread *thread) Normalizer() bool { return thread.normalizer.Run() }
func (thread *thread) Lexical() bool    { return thread.lexical.Step() }
func (thread *thread) Prosody() bool    { return thread.prosody.Run() }

func (thread *thread) Reset() {
	thread.input.Reset()
	thread.normalizer.Reset()
	thread.lexical.Reset()
	thread.prosody.Reset()
}

// deliver gives the sink what the window flushed and the messages posted since the last time, in order: each
// message after the items flushed when it was posted (engine.md §2.3). While the window holds the audio, it gives
// no buffer, only the events before the first one, and every message (engine.md §5.2). After an error from the
// sink, the messages it didn't give stay posted.
func (engine *Engine) deliver(sink Sink) error {
	state := &engine.state
	messages := state.Messages
	state.Messages = nil
	given := 0
	err := func() error {
		for ; given < state.Flushed; given++ {
			for ; len(messages) > 0 && messages[0].Queued <= given; messages = messages[1:] {
				if err := engine.deliverMessage(messages[0], sink); err != nil {
					messages = messages[1:]
					return err
				}
			}
			item := state.Queue[given]
			if state.Holding && item.Event == nil {
				break
			}
			whole, err := engine.deliverItem(item, sink)
			if err != nil {
				if whole {
					given++
				}
				return err
			}
			if !whole {
				break
			}
		}
		for ; len(messages) > 0; messages = messages[1:] {
			if err := engine.deliverMessage(messages[0], sink); err != nil {
				messages = messages[1:]
				return err
			}
		}
		return nil
	}()
	state.Messages = append(messages, state.Messages...)
	state.Queue = state.Queue[:copy(state.Queue, state.Queue[given:])]
	state.Flushed -= given
	if len(state.Queue) == 0 {
		engine.waitingForWindow = false
	}
	return err
}

// pauseWindow pauses the engine where the window gives the sink its items: the window holds the audio from the
// next buffer, and the speaking thread runs on until its queue is full, unless it already waits for the window.
func (engine *Engine) pauseWindow() {
	engine.paused, engine.state.Holding = true, true
	engine.runningOn = !engine.waitingForWindow
}

// accept handles a sink's error: ErrPaused pauses the engine, any other ends the delivery.
func (engine *Engine) accept(err error) error {
	if errors.Is(err, ErrPaused) {
		engine.pauseWindow()
		return nil
	}
	return err
}

func (engine *Engine) deliverMessage(message pipeline.Message, sink Sink) error {
	switch {
	case message.Kind == pipeline.AttributeChanged:
		engine.attributeChanged(Attribute(message.Attribute))
	case message.Text == nil:
	case message.Kind == pipeline.TextStarted:
		return engine.accept(sink.Event(Event{Kind: TextStarted, Sample: engine.position(), Text: message.Text.Number}))
	case message.Kind == pipeline.TextDone:
		return engine.accept(sink.Event(Event{Kind: TextDone, Sample: engine.position(), Text: message.Text.Number}))
	}
	return nil
}

// deliverItem gives the sink an item, and reports whether it took all of it. A buffer the sink paused in is held
// from the bytes it took.
func (engine *Engine) deliverItem(item pipeline.Item, sink Sink) (bool, error) {
	if item.Event == nil {
		data := synthesizer.AppendPCM(nil, item.Samples, engine.format.Bits)[engine.taken:]
		taken, err := sink.Audio(data)
		taken = min(max(taken, 0), len(data))
		engine.given += int64(taken)
		if err == nil && taken < len(data) {
			err = io.ErrShortWrite
		}
		if err = engine.accept(err); err != nil || taken < len(data) {
			engine.taken += int64(taken)
			return false, err
		}
		engine.taken = 0
		return true, nil
	}
	event := Event{Sample: engine.position(), Text: -1, Value: item.Event.Value}
	if item.Event.Text != nil {
		event.Text = item.Event.Text.Number
	}
	switch item.Event.Kind {
	case pipeline.Visual:
		symbol := byte(item.Event.Value)
		visual := VisualPhoneme{Engine: symbol}
		if symbol < byte(len(engine.data.engine.Visuals)) {
			visual.IPA = engine.data.engine.Visuals[symbol].IPA
			visual.Mouth = engine.data.engine.Visuals[symbol].Mouth
		}
		event.Kind, event.Value, event.Visual = Visual, 0, visual
	case pipeline.WordPosition:
		event.Kind = WordPosition
	case pipeline.Mark:
		if item.Event.Value == 0 {
			return true, nil
		}
		event.Kind = Bookmark
	}
	return true, engine.accept(sink.Event(event))
}

// position is where the audio stands: the samples the sinks took, in the engine's format.
func (engine *Engine) position() int64 {
	return engine.given * 8 / int64(engine.format.Bits)
}
