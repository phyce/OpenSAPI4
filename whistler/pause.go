package whistler

// Pause pauses the engine as AudioPause does (engine.md §14): it holds the next chunk of samples, and the
// stages run on until each waits behind the destination. The sink still gets the events before that chunk,
// each time the window thread drains the queue. A sink pauses the engine by returning ErrPaused; a chunk it
// took part of is held from there. While the engine is paused, the attribute methods and Inject apply where
// each stage waits, Speak queues its text, Resume goes on and Reset stops.
func (engine *Engine) Pause() error {
	destination := &engine.stages.destination
	if destination.paused {
		return ErrAlreadyPaused
	}
	destination.paused = true
	return nil
}

func (engine *Engine) Paused() bool {
	return engine.stages.destination.paused
}

// Resume goes on from a pause as AudioResume does (engine.md §14.3): the sink gets what the engine held, and
// each stage goes on from its wait, with the values the calls left. It returns as Speak does: when the sink
// has every text queued, ErrPaused when the sink pauses again, or the sink's error, which stops them all.
func (engine *Engine) Resume(sink Sink) error {
	destination := &engine.stages.destination
	if !destination.paused {
		return ErrNotPaused
	}
	destination.sink, destination.paused = sink, false
	engine.handOver()
	engine.runSynthesizer()
	engine.runVoice()
	engine.runFrontEnd()
	return engine.finish()
}

// Reset stops a paused engine as AudioReset does (engine.md §12, §14.3): what it held is dropped, each stage
// goes on from its wait as an aborted stage does, and the engine is no longer paused. Every text queued whose
// TextDone no sink had ends there; the caller sends its TextDataDone with TTSBNS_ABORTED. An engine that
// isn't paused has nothing to stop.
func (engine *Engine) Reset() {
	if engine.stages.destination.paused {
		engine.abort()
	}
}

// finish ends a Speak or a Resume: a stopped engine is aborted (engine.md §12.3), and a paused one stays at its
// waits. Otherwise the sink has everything and the queues are empty.
func (engine *Engine) finish() error {
	stages := &engine.stages
	if err := stages.destination.err; err != nil {
		engine.abort()
		return err
	}
	if stages.destination.paused {
		return ErrPaused
	}
	stages.destination = destination{}
	return nil
}

// abort is what AudioReset does to the stages (engine.md §12.1): each goes on from where it waits, doing
// what an aborted stage does, until all are idle. The sink gets nothing.
func (engine *Engine) abort() {
	stages := &engine.stages
	stages.frontEnd.aborted, stages.voice.aborted, stages.synthesizer.aborted = true, true, true
	engine.runSynthesizer()
	engine.runVoice()
	engine.runFrontEnd()
	stages.frontEnd.aborted, stages.voice.aborted, stages.synthesizer.aborted = false, false, false
	stages.destination = destination{}
}
