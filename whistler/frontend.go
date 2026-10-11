package whistler

import (
	"github.com/phyce/OpenSAPI4/whistler/analyser"
	"github.com/phyce/OpenSAPI4/whistler/text"
)

// stages are the original's threads (engine.md §1), each with the queue it reads, and the destination's queue
// after them (engine.md §12.2). A stage that posts runs the stages after it at once, so the stages take each
// message in turn, as one thread would, until one waits on a full queue.
type stages struct {
	frontEnd    frontEnd
	voice       voiceStage
	synthesizer synthesizerStage
	destination destination
}

type spokenText struct {
	characters   []uint16
	tagged       bool
	characterSet text.CharacterSet
	number       int
}

type frontEndStep int

const (
	takeText frontEndStep = iota
	prepareText
	nextSegment
	nextSentence
)

// frontEnd is where the front end stands in its texts (engine.md §2, §4): what it does next, and whether
// it waits to post a text's start or end or a sentence's units. It waits until the voice's queue is empty.
type frontEnd struct {
	texts    []spokenText
	text     spokenText
	step     frontEndStep
	segments []text.Segment
	segment  int
	content  []byte
	reading  analyser.Segment
	position int
	read     analyser.Read

	waiting  bool
	post     voiceMessage
	sentence analysedSentence
	analysed bool
	aborted  bool
}

// runFrontEnd runs the front end until it waits on the voice's queue or has no text left.
func (engine *Engine) runFrontEnd() {
	frontEnd := &engine.stages.frontEnd
	for {
		if frontEnd.waiting {
			if len(engine.stages.voice.queue) > 0 {
				return
			}
			frontEnd.waiting = false
			engine.frontEndPost()
			continue
		}
		if !engine.frontEndStep() {
			return
		}
	}
}

// frontEndStep runs the front end up to its next wait. It reports false when it has no text left.
func (engine *Engine) frontEndStep() bool {
	frontEnd := &engine.stages.frontEnd
	switch frontEnd.step {
	case takeText:
		if len(frontEnd.texts) == 0 {
			return false
		}
		frontEnd.text, frontEnd.texts = frontEnd.texts[0], frontEnd.texts[1:]
		engine.transplanting = false
		frontEnd.wait(voiceMessage{kind: textStartMessage, text: frontEnd.text.number})
		frontEnd.step = prepareText
	case prepareText:
		spoken := frontEnd.text
		frontEnd.segments, frontEnd.segment = nil, -1
		if !frontEnd.aborted {
			prepared := engine.preprocessor.Prepare(spoken.characters, spoken.tagged, spoken.characterSet, engine.addressContext)
			engine.addressContext = prepared.AddressContext
			engine.controls, engine.oneCharacter = prepared.Controls, prepared.OneCharacter
			frontEnd.segments = text.Segments(prepared.Final)
		}
		frontEnd.step = nextSegment
	case nextSegment:
		frontEnd.segment++
		if frontEnd.segment >= len(frontEnd.segments) {
			frontEnd.wait(voiceMessage{kind: textEndMessage, text: frontEnd.text.number})
			frontEnd.step = takeText
			return true
		}
		segment := frontEnd.segments[frontEnd.segment]
		switch {
		case segment.Pitch == nil:
		case string(segment.Pitch) == "0":
			engine.resetPitch()
		default:
			engine.SetPitch(uint16(atoi(segment.Pitch)))
		}
		frontEnd.content, frontEnd.reading, frontEnd.position = segment.Text, analyser.NewSegment(segment.Text), 0
		frontEnd.read = engine.reader.ReadSentence(frontEnd.reading, 0)
		if engine.checkSpan(frontEnd.read, frontEnd.content, 0) {
			engine.waitSentence()
			frontEnd.step = nextSentence
		}
	case nextSentence:
		frontEnd.position += frontEnd.read.Consumed
		frontEnd.read = engine.reader.ReadSentence(frontEnd.reading, frontEnd.position)
		if frontEnd.position >= len(frontEnd.content) {
			frontEnd.step = nextSegment
			return true
		}
		if engine.checkSpan(frontEnd.read, frontEnd.content, frontEnd.position) && !frontEnd.aborted {
			engine.waitSentence()
		} else {
			frontEnd.step = nextSegment
		}
	}
	return true
}

func (frontEnd *frontEnd) wait(message voiceMessage) {
	frontEnd.waiting, frontEnd.post, frontEnd.analysed = true, message, false
}

// waitSentence analyses the sentence read and waits to post its units (engine.md §14.1). A sentence whose
// analysis fails posts nothing, and its tags are lost, but the front end still waits for the voice first.
func (engine *Engine) waitSentence() {
	frontEnd := &engine.stages.frontEnd
	frontEnd.sentence, frontEnd.analysed = engine.analyse(frontEnd.read)
	frontEnd.waiting, frontEnd.post = true, voiceMessage{kind: batchMessage}
}

// frontEndPost is the front end's post after its wait. A sentence's units are written only now, with the
// pitch as it is now.
func (engine *Engine) frontEndPost() {
	frontEnd := &engine.stages.frontEnd
	if frontEnd.post.kind != batchMessage {
		engine.postToVoice(frontEnd.post)
		return
	}
	if frontEnd.analysed {
		engine.postUnits(engine.writeUnits(frontEnd.sentence, engine.oneCharacter))
	}
}

// postToVoice puts a message on the voice's queue and runs the voice.
func (engine *Engine) postToVoice(message voiceMessage) {
	engine.stages.voice.queue = append(engine.stages.voice.queue, message)
	engine.runVoice()
}
