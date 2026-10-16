package truvoice

import (
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

// Speak speaks one text, as one TextData call of the wide interface with a notification sink does, and returns when
// the speaking thread ends: when the sink has everything of the text, and of the texts queued before it.
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
// (engine.md §2.3). It returns when the thread ends, after the last text. An error from the sink ends the thread at
// once: the texts still queued are dropped, and Run returns the error.
func (engine *Engine) Run(sink Sink) error {
	if len(engine.state.Texts) == 0 {
		return nil
	}
	thread := engine.newThread()
	for thread.input.Turn() {
		thread.pipeline.ApplyAttributes()
		result := thread.pipeline.Step()
		if result&pipeline.StepHandOver != 0 {
			thread.pipeline.HandOver()
		}
		if result&pipeline.StepIdle != 0 {
			thread.input.Busy = false
		}
		if err := engine.deliver(sink); err != nil {
			engine.state.Texts = nil
			engine.state.Queue = engine.state.Queue[:0]
			return err
		}
	}
	engine.state.Flush()
	return engine.deliver(sink)
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
	data := engine.data
	thread := &thread{}
	thread.pipeline = pipeline.New(data.pipeline, synthesizer.New(data.synthesizer, engine.state.SampleRate), thread, &engine.state)
	thread.pipeline.Generator = parameters.New(data.parameters, data.vowels, thread.pipeline)
	thread.input = input.New(data.input, thread.pipeline)
	thread.normalizer = normalizer.New(data.normalizer, data.parameters, thread.pipeline)
	thread.lexical = lexical.New(data.lexical, data.dictionary, data.parameters, thread.pipeline)
	thread.prosody = prosody.New(data.prosody, data.parameters, thread.pipeline)
	thread.prosody.Shared = data.shared
	thread.pipeline.Start()
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
// message after the items flushed when it was posted (engine.md §2.3).
func (engine *Engine) deliver(sink Sink) error {
	state := &engine.state
	messages := state.Messages
	flushed := state.Flushed
	defer func() {
		state.Queue = state.Queue[:copy(state.Queue, state.Queue[flushed:])]
		state.Flushed = 0
		state.Messages = state.Messages[:0]
	}()
	for index, item := range state.Queue[:flushed] {
		for ; len(messages) > 0 && messages[0].Queued <= index; messages = messages[1:] {
			if err := engine.deliverMessage(messages[0], sink); err != nil {
				return err
			}
		}
		if err := engine.deliverItem(item, sink); err != nil {
			return err
		}
	}
	for _, message := range messages {
		if err := engine.deliverMessage(message, sink); err != nil {
			return err
		}
	}
	return nil
}

func (engine *Engine) deliverMessage(message pipeline.Message, sink Sink) error {
	switch {
	case message.Kind == pipeline.AttributeChanged:
		engine.attributeChanged(Attribute(message.Attribute))
	case message.Text == nil:
	case message.Kind == pipeline.TextStarted:
		return sink.Event(Event{Kind: TextStarted, Sample: engine.samples, Text: message.Text.Number})
	case message.Kind == pipeline.TextDone:
		return sink.Event(Event{Kind: TextDone, Sample: engine.samples, Text: message.Text.Number})
	}
	return nil
}

func (engine *Engine) deliverItem(item pipeline.Item, sink Sink) error {
	if item.Event == nil {
		data := synthesizer.AppendPCM(nil, item.Samples, engine.format.Bits)
		engine.samples += int64(len(item.Samples))
		_, err := sink.Audio(data)
		return err
	}
	event := Event{Sample: engine.samples, Text: -1, Value: item.Event.Value}
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
			return nil
		}
		event.Kind = Bookmark
	}
	return sink.Event(event)
}
