package whistler

import (
	"github.com/phyce/OpenSAPI4/whistler/text"
	"github.com/phyce/OpenSAPI4/whistler/units"
)

type voiceMessageKind int

const (
	textStartMessage voiceMessageKind = iota + 1
	textEndMessage
	batchMessage
	controlMessage
)

// voiceMessage is what the front end posts to the voice: a text's start or end, a sentence's batch, or a
// control sent on its own.
type voiceMessage struct {
	kind      voiceMessageKind
	text      int
	records   []units.Record
	following units.RecordKind
	control   text.Control
}

// voiceStage is where the voice stands: the batch it maps word by word, and the frames or message it waits
// to post. It waits until the synthesizer's queue is empty before each word's frames and before passing on
// a control or a text's start or end; the tags inside a batch it posts without waiting (engine.md §12.2).
type voiceStage struct {
	queue   []voiceMessage
	mapping *units.Mapping
	waiting bool
	post    []synthesizerItem
	// aborted drops every batch it takes; the batch it is mapping still has its tags handled (engine.md §12.1).
	aborted bool
}

// runVoice runs the voice until it waits on the synthesizer's queue or has nothing left.
func (engine *Engine) runVoice() {
	voice := &engine.stages.voice
	for {
		switch {
		case voice.waiting:
			if len(engine.stages.synthesizer.queue) > 0 {
				return
			}
			voice.waiting = false
			engine.postToSynthesizer(voice.post...)
		case voice.mapping != nil:
			items, word, more := voice.mapping.Next()
			switch {
			case !more:
				voice.mapping = nil
			case word:
				voice.post = voice.post[:0]
				for _, item := range items {
					voice.post = append(voice.post, synthesizerItem{kind: frameItem, frame: item.Frame})
				}
				voice.waiting = true
			default:
				for _, item := range items {
					engine.voiceTag(item)
				}
			}
		case len(voice.queue) > 0:
			message := voice.queue[0]
			voice.queue = voice.queue[1:]
			switch message.kind {
			case batchMessage:
				if !voice.aborted {
					voice.mapping = engine.mapper.Begin(message.records, message.following)
				}
			case controlMessage:
				engine.voiceControl(message.control)
			case textStartMessage:
				voice.wait(synthesizerItem{kind: textStartItem, text: message.text})
			case textEndMessage:
				voice.wait(synthesizerItem{kind: textEndItem, text: message.text})
			}
		default:
			return
		}
	}
}

func (voice *voiceStage) wait(item synthesizerItem) {
	voice.waiting, voice.post = true, append(voice.post[:0], item)
}

// voiceTag handles a tag inside a batch: speed and volume are the voice's own, anything else goes on to the
// synthesizer without waiting.
func (engine *Engine) voiceTag(item units.Item) {
	if item.Frame != nil {
		engine.postToSynthesizer(synthesizerItem{kind: frameItem, frame: item.Frame})
		return
	}
	switch control := item.Control; control.Kind {
	case units.Speed:
		engine.attributeChanged(SpeedAttribute)
	case units.Volume:
		engine.attributeChanged(VolumeAttribute)
	case units.WordPosition:
		engine.postToSynthesizer(synthesizerItem{kind: passItem, event: Event{Kind: WordPosition, Value: control.Value}})
	case units.Bookmark:
		engine.postToSynthesizer(synthesizerItem{kind: passItem, event: Event{Kind: Bookmark, Value: control.Value}})
	case units.Character:
		engine.postToSynthesizer(synthesizerItem{kind: characterItem, name: control.Name})
	case units.Reset:
		engine.postToSynthesizer(synthesizerItem{kind: resetItem})
	default:
		engine.postToSynthesizer(synthesizerItem{kind: passItem})
	}
}

// voiceControl handles a control sent on its own (engine.md §7). The voice uses \Spd= and \Vol= itself; a
// \Rst\ resets speed and volume before the voice waits to pass it on.
func (engine *Engine) voiceControl(control text.Control) {
	value, parsed := parseNumber(control.Value)
	switch {
	case control.Kind == text.ControlSpeed && parsed:
		engine.SetSpeed(value)
		return
	case control.Kind == text.ControlVolume && parsed:
		engine.SetVolume(value)
		return
	}
	name, named := characterName(control.Value)
	item := synthesizerItem{kind: passItem}
	switch {
	case control.Kind == text.ControlMark && parsed:
		item.event = Event{Kind: Bookmark, Value: value}
	case control.Kind == text.ControlWord && parsed:
		item.event = Event{Kind: WordPosition, Value: value}
	case control.Kind == text.ControlPause && parsed:
		item = synthesizerItem{kind: frameItem, frame: engine.pauseFrame(value)}
	case control.Kind == text.ControlCharacter && named:
		item = synthesizerItem{kind: characterItem, name: name}
	case control.Kind == text.ControlReset:
		engine.resetSpeed()
		engine.SetVolume(0xFFFFFFFF)
		item = synthesizerItem{kind: resetItem}
	}
	engine.stages.voice.wait(item)
}

// postToSynthesizer puts items on the synthesizer's queue and runs the synthesizer.
func (engine *Engine) postToSynthesizer(items ...synthesizerItem) {
	engine.stages.synthesizer.queue = append(engine.stages.synthesizer.queue, items...)
	engine.runSynthesizer()
}
