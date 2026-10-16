package input

import "github.com/phyce/OpenSAPI4/truvoice/pipeline"

// The room the input ring must have for the speaking thread to feed it more of a text.
const feedRoom = 2048

// Turn is the speaking thread's input side each time round its loop (input.md §6). When the text being fed is
// all in the input ring, the escape parser is out of input and the ring is drained, it takes the engine's next
// text, feeds it and pumps the ring into the escape parser. Otherwise it feeds more of the text if the ring has
// more than 2,048 bytes free, or pumps the ring if the escape parser is out of input. A pump makes the thread
// busy; the thread is no longer busy after a step that reports it idle. Turn returns false when the thread ends:
// with no text to take, while it isn't busy.
func (stage *Stage) Turn() bool {
	switch {
	case stage.AllFed && stage.Pipeline.OutOfInput && stage.Drained:
		engine := stage.Pipeline.Engine
		if len(engine.Texts) == 0 {
			return stage.Busy
		}
		next := engine.Texts[0]
		stage.feeding, stage.Position = next.Characters, 0
		stage.AllFed, stage.AfterLineEnd = false, true
		stage.Text = next.Text
		engine.Post(pipeline.Message{Kind: pipeline.TextStarted, Text: next.Text})
		stage.feedText()
		stage.Pump(true)
		stage.Busy = true
	case !stage.AllFed && stage.inputRoom() > feedRoom:
		stage.feedText()
	case stage.Pipeline.OutOfInput:
		stage.Pump(false)
		stage.Busy = true
	}
	return true
}

// feedText feeds more of the text, and takes it off the engine's queue once it is all fed.
func (stage *Stage) feedText() {
	stage.Feed(stage.feeding)
	if engine := stage.Pipeline.Engine; stage.AllFed {
		engine.Texts = engine.Texts[1:]
	}
}
