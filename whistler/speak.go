package whistler

import (
	"strings"
	"unicode/utf16"

	"github.com/phyce/OpenSAPI4/whistler/analyser"
	"github.com/phyce/OpenSAPI4/whistler/posting"
	"github.com/phyce/OpenSAPI4/whistler/prosody"
	"github.com/phyce/OpenSAPI4/whistler/synthesis"
	"github.com/phyce/OpenSAPI4/whistler/text"
	"github.com/phyce/OpenSAPI4/whistler/units"
)

const (
	escape         = 0x1B
	space          = ' '
	carriageReturn = '\r'
	tagCommand     = 'N'
)

var thousandth = float32(0.001)

// SpeakString speaks a text with tags, as SpeakText with the Unicode character set does.
func (engine *Engine) SpeakString(value string, sink Sink) error {
	return engine.Speak(utf16.Encode([]rune(value)), true, text.UnicodeText, sink)
}

// Speak speaks one text, as one TextData call of the original does (engine.md §2), and returns when the
// sink has everything. Characters are UTF-16 code units. A character set other than the four of text.md §2
// gives ErrInvalidCharacter and changes nothing. An error from the sink stops the text as AudioReset does
// (engine.md §12): the sink gets nothing more, what the stages still do carries into the next text, and
// Speak returns the error. When the sink pauses the engine (Pause), Speak returns ErrPaused, and Resume
// speaks the rest. While the engine is paused, Speak queues the text and returns ErrPaused; the events before
// the chunk the pause holds go to the sink the engine has, or this one if it has none, and Resume's sink gets
// the rest.
func (engine *Engine) Speak(characters []uint16, tagged bool, characterSet text.CharacterSet, sink Sink) error {
	switch characterSet {
	case text.UnicodeText, text.UnicodeIPA, text.ANSIText, text.ANSIPhonetic:
	default:
		return ErrInvalidCharacter
	}
	stages := &engine.stages
	if !stages.destination.paused || stages.destination.sink == nil {
		stages.destination.sink = sink
	}
	stages.frontEnd.texts = append(stages.frontEnd.texts, spokenText{characters: characters, tagged: tagged, characterSet: characterSet,
		number: engine.textCount})
	engine.textCount++
	engine.runFrontEnd()
	return engine.finish()
}

// atoi reads leading digits as msvcrt's atoi does for the numbers the front end prints with %u: a 32-bit
// value that wraps.
func atoi(digits []byte) int32 {
	var value uint32
	for _, character := range digits {
		if character < '0' || character > '9' {
			break
		}
		value = value*10 + uint32(character-'0')
	}
	return int32(value)
}

// escapeArgument copies an escape's argument as the original's span check does: the bytes after the
// command letter up to the next space, that space included, or to the end.
func escapeArgument(content []byte, index int) (command byte, argument []byte, next int) {
	if index+1 < len(content) {
		command = content[index+1]
	}
	index++
	if command != 0 {
		for character := command; character != space; {
			index++
			character = 0
			if index < len(content) {
				character = content[index]
			}
			if character == 0 {
				break
			}
			argument = append(argument, character)
		}
	}
	return command, argument, index
}

// checkSpan reports whether the sentence a read found is spoken (engine.md §5). When it isn't, every tag
// from the span to the end of the segment is sent on its own and the segment ends.
func (engine *Engine) checkSpan(read analyser.Read, content []byte, start int) bool {
	last := min(start+read.Consumed-1, len(content)-1)
	engine.tagCount = 0
	words := false
	for index := start; index <= last; {
		switch content[index] {
		case space, carriageReturn:
			index++
		case escape:
			command, argument, next := escapeArgument(content, index)
			if command == tagCommand {
				if engine.tagCount == 0 {
					engine.firstTag = atoi(argument)
				}
				engine.tagCount++
			}
			index = next
		default:
			words = true
			for index < len(content) && content[index] != space && content[index] != escape {
				index++
			}
		}
	}
	if words && read.Result != analyser.ReadNothingLeft {
		return true
	}
	for index := start; index < len(content); {
		if content[index] != escape {
			index++
			continue
		}
		command, argument, next := escapeArgument(content, index)
		if command == tagCommand {
			engine.sendControl(int(atoi(argument)))
		}
		index = next
	}
	return false
}

// sendControl sends a control on its own (engine.md §7). \Pit= and a tag the voice can't read pass through
// every stage unused.
func (engine *Engine) sendControl(number int) {
	if number < 0 || number >= len(engine.controls) {
		return
	}
	control := engine.controls[number]
	switch control.Kind {
	case text.ControlCharacter:
		engine.monotone.Character(control.Value)
	case text.ControlReset:
		engine.monotone.Reset()
	case text.ControlMark, text.ControlWord, text.ControlPause, text.ControlPitch, text.ControlSpeed, text.ControlVolume:
	default:
		return
	}
	engine.postToVoice(voiceMessage{kind: controlMessage, control: control})
}

// parseNumber reads a control's value as the voice reads the number of its tag: digits only, wrapping
// at 32 bits.
func parseNumber(value string) (uint32, bool) {
	if value == "" || strings.IndexByte(value, 0) >= 0 {
		return 0, false
	}
	var number uint32
	for index := range len(value) {
		if value[index] < '0' || value[index] > '9' {
			return 0, false
		}
		number = number*10 + uint32(value[index]-'0')
	}
	return number, true
}

// characterName reads a \Chr= control's value as the voice reads the tag: a quoted name, quotes included,
// that ends where the tag does.
func characterName(value string) (string, bool) {
	record := value + `\`
	if index := strings.IndexByte(record, 0); index >= 0 {
		record = record[:index]
	}
	if len(record) < 2 || record[0] != '"' {
		return "", false
	}
	closing := strings.IndexByte(record[1:], '"') + 1
	if closing == 0 || closing+1 >= len(record) || record[closing+1] != '\\' {
		return "", false
	}
	return record[:closing+1], true
}

func (engine *Engine) pauseFrame(milliseconds uint32) *units.Frame {
	duration := float32(float64(milliseconds) * float64(thousandth))
	return &units.Frame{Frame: synthesis.Frame{Duration: duration}, Phone: engine.voice.SilencePhone}
}

// analysedSentence is a sentence the front end has read and analysed, ready for its units to be written.
type analysedSentence struct {
	analysis    *prosody.Analysis
	transcribed []byte
}

// analyse transcribes and analyses a sentence the reader found. It reports false when the analysis fails.
func (engine *Engine) analyse(read analyser.Read) (analysedSentence, bool) {
	if !read.Sentence() {
		return analysedSentence{}, false
	}
	state := read.State
	engine.analyser.Transcribe(state)
	if !engine.analyser.Analyse(state) {
		return analysedSentence{}, false
	}
	analysis, kept := prosody.Analyze(engine.tables, state.Sentence())
	return analysedSentence{analysis: analysis, transcribed: state.Transcribed}, kept
}

// writeUnits builds a sentence's units with the pitch as it is now.
func (engine *Engine) writeUnits(sentence analysedSentence, oneCharacter bool) []prosody.Unit {
	built, _ := prosody.BuildUnits(engine.tables, sentence.analysis, sentence.transcribed, prosody.PitchSteps(engine.pitch), oneCharacter)
	return built
}

func (engine *Engine) buildUnits(read analyser.Read, oneCharacter bool) ([]prosody.Unit, bool) {
	sentence, analysed := engine.analyse(read)
	if !analysed {
		return nil, false
	}
	return engine.writeUnits(sentence, oneCharacter), true
}

// postUnits writes a sentence's units and tags into the records the voice reads (engine.md §6). While a
// prosody transplant is on, the units and every tag but the transplant's own are dropped (engine.md §13.2).
func (engine *Engine) postUnits(built []prosody.Unit) {
	batch := posting.NewBatch(engine.converter, len(built)+2*engine.tagCount)
	placed := 0
	tag := -1
	if engine.tagCount > 0 {
		tag = int(engine.firstTag) - 1
	}
	for position := 0; ; {
		if position < len(built) {
			unit := built[position]
			position++
			if unit.Kind != prosody.TagUnit {
				if engine.transplanting {
					batch.Drop()
				} else {
					engine.copyUnit(batch, unit)
				}
				continue
			}
			tag = unit.Tag
		} else if placed >= engine.tagCount {
			break
		} else {
			tag++
		}
		placed++
		engine.postControl(batch, tag)
	}
	records, following := batch.Records()
	engine.postToVoice(voiceMessage{kind: batchMessage, records: records, following: following})
}

// copyUnit writes a unit whole. A phone has one point, at its end, with its pitch, or the current pitch while
// the monotone flag is on.
func (engine *Engine) copyUnit(batch *posting.Batch, unit prosody.Unit) {
	var points []units.Point
	if unit.Kind == prosody.PhoneUnit {
		pitch := unit.Pitch
		if engine.monotone.On() {
			pitch = float32(engine.pitch)
		}
		points = []units.Point{{Position: 1, Pitch: pitch}}
	}
	batch.Copy(unit.Name, unit.Duration, points)
}

var tagPrefixes = map[text.ControlKind]string{text.ControlCharacter: `\Chr=`, text.ControlWord: `\Wrd=`, text.ControlMark: `\Mrk=`,
	text.ControlSpeed: `\Spd=`, text.ControlVolume: `\Vol=`}

// postControl writes control number's records into a sentence's batch, or runs it when it belongs to a
// prosody transplant (engine.md §13). A number past the control table writes nothing (engine.md §9).
func (engine *Engine) postControl(batch *posting.Batch, number int) {
	if number < 0 || number >= len(engine.controls) {
		return
	}
	control := engine.controls[number]
	switch {
	case control.Kind == text.ControlTransplantOffset:
		batch.SetOffset(atoi([]byte(control.Value)))
	case control.Kind == text.ControlTransplantEnd:
		batch.EndTransplant()
		engine.transplanting = false
	case control.Kind == text.ControlTransplant:
		if batch.Transplant(control.Value, engine.pitch) {
			engine.transplanting = true
		}
	case engine.transplanting:
		batch.Drop()
	case control.Kind == text.ControlPause:
		batch.Pause(control.Value)
	case control.Kind == text.ControlReset:
		engine.monotone.Reset()
		batch.Tag(`\RST\`)
	case tagPrefixes[control.Kind] != "":
		if control.Kind == text.ControlCharacter {
			engine.monotone.Character(control.Value)
		}
		batch.Tag(tagPrefixes[control.Kind] + control.Value + `\`)
	}
}
