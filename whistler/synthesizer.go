package whistler

import "github.com/phyce/OpenSAPI4/whistler/units"

type synthesizerItemKind int

const (
	frameItem synthesizerItemKind = iota + 1
	// passItem is a word position or bookmark, or a tag no stage uses, passed on after a wait.
	passItem
	characterItem
	resetItem
	textStartItem
	textEndItem
)

type synthesizerItem struct {
	kind  synthesizerItemKind
	frame *units.Frame
	event Event
	name  string
	text  int
}

// synthesizerStage is where the synthesizer stands. It waits while the destination's queue is full: after a
// frame, before posting its Visual and samples, and before passing on a word position, bookmark or other
// tag. It posts character changes and a text's start and end without waiting, and wakes the window thread
// after a text's end (engine.md §12.2).
type synthesizerStage struct {
	queue   []synthesizerItem
	waiting bool
	item    synthesizerItem
	// text is the text whose items it takes.
	text int
	// aborted drops frames, pauses and events; character changes and resets still change the effects
	// (engine.md §12.1).
	aborted bool
}

// runSynthesizer runs the synthesizer until it waits on the destination's queue or has nothing left. A
// frame it waits after is made when it goes on: nothing it waits for changes what the frame gives.
func (engine *Engine) runSynthesizer() {
	synthesizer, destination := &engine.stages.synthesizer, &engine.stages.destination
	for {
		if synthesizer.waiting {
			if synthesizer.aborted {
				synthesizer.waiting = false
				if synthesizer.item.kind == frameItem {
					_ = engine.synthesizer.SynthesizeTo(synthesizer.item.frame.Frame, func([]byte) error { return nil })
				}
				continue
			}
			if destination.full {
				return
			}
			synthesizer.waiting = false
			engine.postItem(synthesizer.item)
			continue
		}
		if len(synthesizer.queue) == 0 {
			return
		}
		item := synthesizer.queue[0]
		synthesizer.queue = synthesizer.queue[1:]
		if synthesizer.aborted {
			switch item.kind {
			case characterItem:
				engine.effects.Character(item.name)
			case resetItem:
				engine.effects.Reset()
			}
			continue
		}
		engine.take()
		switch item.kind {
		case frameItem, passItem:
			synthesizer.waiting, synthesizer.item = true, item
		case characterItem:
			engine.effects.Character(item.name)
			engine.post(1, false)
		case resetItem:
			engine.effects.Reset()
		case textStartItem:
			synthesizer.text = item.text
			destination.chunks = 0
			if engine.synthesizer.Resampler != nil {
				engine.synthesizer.Resampler.Reset()
			}
			engine.event(Event{Kind: TextStarted})
			engine.post(1, false)
		case textEndItem:
			engine.event(Event{Kind: TextDone})
			engine.post(1, false)
			engine.drain()
		}
	}
}

// postItem makes a frame and posts its Visual event and samples, or passes an event on.
func (engine *Engine) postItem(item synthesizerItem) {
	destination := &engine.stages.destination
	if item.kind == passItem {
		if item.event.Kind != 0 {
			engine.event(item.event)
		}
		engine.post(1, false)
		return
	}
	made := false
	err := engine.synthesizer.SynthesizeTo(item.frame.Frame, func(data []byte) error {
		if len(data) == 0 {
			return nil
		}
		if !made {
			made = true
			engine.event(Event{Kind: Visual, Visual: engine.voice.Visual[item.frame.Phone]})
		}
		engine.audio(data)
		return nil
	})
	if err != nil && destination.err == nil {
		destination.err = err
	}
	if made {
		engine.post(2, true)
	}
}
