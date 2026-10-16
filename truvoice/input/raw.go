package input

// rawPump is the most characters a raw pump gives the escape parser at once.
const rawPump = 400

// StartRaw starts feeding a text as it is (input.md §7): from its start, then a pump.
func (stage *Stage) StartRaw(text []byte) {
	stage.Position, stage.AllFed = 0, false
	stage.FeedRaw(text)
	stage.PumpRaw()
}

// TurnRaw is the input side each time round the loop that feeds a text as it is: it feeds more of the text if it
// isn't all fed and the input ring has more than 2,048 bytes free, or else pumps the ring if the escape parser is
// out of input and the ring isn't drained.
func (stage *Stage) TurnRaw(text []byte) {
	switch {
	case !stage.AllFed && stage.inputRoom() > feedRoom:
		stage.FeedRaw(text)
	case stage.Pipeline.OutOfInput && !stage.Drained:
		stage.PumpRaw()
	}
}

// FeedRaw writes more of a text into the input ring as it is, from Position, while the ring has room for ten
// characters, and the end of text once the text is all in.
func (stage *Stage) FeedRaw(text []byte) {
	start := stage.Position
	for stage.Position < int32(len(text)) && stage.inputRoom() >= inputReserve {
		stage.putInput(text[stage.Position])
		stage.Position++
	}
	if stage.Position >= int32(len(text)) {
		stage.putEndOfText()
		stage.AllFed = true
	}
	if stage.Position > start {
		stage.Drained = false
	}
}

// PumpRaw gives the escape parser up to 400 characters of the input ring as they are, without the tokenizer.
func (stage *Stage) PumpRaw() {
	count := 0
	for ; count < rawPump; count++ {
		character := stage.get()
		if character == nothing {
			break
		}
		stage.Put(byte(character))
	}
	if count > 0 {
		stage.Pipeline.OutOfInput = false
	}
}

// PutString gives the escape parser a string's characters and the end of text.
func (stage *Stage) PutString(text []byte) {
	for _, character := range text {
		stage.Put(character)
	}
	stage.Pipeline.OutOfInput = false
	for index := range len(endOfText) {
		stage.Put(endOfText[index])
	}
}
