package truvoice

import "github.com/phyce/OpenSAPI4/truvoice/pipeline"

// Pause pauses the engine as AudioPause does (engine.md §5). Called while the engine runs, from the sink's methods,
// it holds the audio from the next buffer, and the speaking thread runs on until its queue is full, as behind a
// destination that has stopped taking audio; a sink that returns ErrPaused does the same. Called while the engine
// doesn't run, it keeps the next speaking thread waiting before its first step. While the engine is paused, the
// attribute methods and Inject apply at the thread's next step, Speak queues its text and returns ErrPaused, Resume
// goes on and Reset stops.
func (engine *Engine) Pause() error {
	if engine.paused {
		return ErrAlreadyPaused
	}
	if engine.running {
		engine.pauseWindow()
	} else {
		engine.paused = true
	}
	return nil
}

func (engine *Engine) Paused() bool {
	return engine.paused
}

// Resume goes on from a pause as AudioResume does (engine.md §5.2): the window gives the sink the queue, and the
// speaking thread goes on from its wait, with the values the calls left. It returns as Run does. Called from the
// sink's methods, it only undoes their pause.
func (engine *Engine) Resume(sink Sink) error {
	if !engine.paused {
		return ErrNotPaused
	}
	engine.paused, engine.state.Holding, engine.runningOn = false, false, false
	if engine.running {
		return nil
	}
	if engine.thread == nil && len(engine.state.Texts) > 0 {
		engine.thread = engine.newThread()
	}
	return engine.run(sink)
}

// Reset stops the engine as AudioReset does (engine.md §5.3): the window drops what it held, the speaking thread
// ends, the texts queued are dropped, and the engine is no longer paused. The sink gets a TextDone with Aborted for
// each text that ends there: first the texts the thread hadn't all fed, in order, then those whose end was in the
// pipeline, then the text it was reading. Reset does nothing to an engine that has nothing to speak and isn't
// paused, or that runs: a sink stops a running engine by returning an error.
func (engine *Engine) Reset(sink Sink) {
	if !engine.running && (engine.paused || engine.thread != nil || len(engine.state.Texts) > 0) {
		engine.reset(sink)
	}
}

// reset is what AudioReset does to a paused or a running engine. The messages still posted, which a sink's error
// left, come after the TextDone of the texts the thread hadn't all fed. The sink's errors are ignored.
func (engine *Engine) reset(sink Sink) {
	state := &engine.state
	var queued, unfinished []*pipeline.Text
	for _, text := range state.Texts {
		if text.Text != nil {
			queued = append(queued, text.Text)
		}
	}
	if engine.thread != nil {
		unfinished = engine.thread.unfinished()
	}
	messages := state.Messages
	state.Texts, state.Queue, state.Flushed, state.Messages, state.Holding = nil, state.Queue[:0], 0, nil, false
	engine.thread, engine.paused, engine.runningOn, engine.waitingForWindow, engine.taken = nil, false, false, false, 0
	aborted := func(texts []*pipeline.Text) {
		for _, text := range texts {
			sink.Event(Event{Kind: TextDone, Sample: engine.position(), Text: text.Number, Aborted: true})
		}
	}
	aborted(queued)
	for _, message := range messages {
		switch {
		case message.Kind == pipeline.AttributeChanged:
			engine.attributeChanged(Attribute(message.Attribute))
		case message.Text == nil:
		case message.Kind == pipeline.TextStarted:
			sink.Event(Event{Kind: TextStarted, Sample: engine.position(), Text: message.Text.Number})
		case message.Kind == pipeline.TextDone:
			sink.Event(Event{Kind: TextDone, Sample: engine.position(), Text: message.Text.Number})
		}
	}
	aborted(unfinished)
}

// unfinished are the texts a reset ends in the speaking thread (engine.md §5.3): those whose end is a node in the
// pipeline, in the nodes' order, then the text it is reading, if that is all fed and its end isn't a node yet.
func (thread *thread) unfinished() []*pipeline.Text {
	var texts []*pipeline.Text
	speaking := thread.pipeline
	node := speaking.First().Next
	for range pipeline.NodeCount - speaking.Free {
		if node.Type() == pipeline.Control && node.Symbol() == 'i' && node.Value() == 0 && node.Text != nil {
			texts = append(texts, node.Text)
		}
		node = node.Next
	}
	if thread.input.EndUnread() && thread.input.Text != nil {
		texts = append(texts, thread.input.Text)
	}
	return texts
}
