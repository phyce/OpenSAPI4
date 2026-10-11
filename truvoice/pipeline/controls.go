package pipeline

const (
	muteAttenuation = 16
	silentVolume    = 0
	nodeResets      = 0x20
)

// attenuationVolumes[a] is the volume the engine reports after a command of attenuation a: 65535 · 10^(−a/10),
// truncated (pipeline.md §4.2).
var attenuationVolumes = [muteAttenuation]uint32{65535, 52056, 41349, 32845, 26089, 20723, 16461, 13075, 10386, 8250,
	6553, 5205, 4134, 3284, 2608, 2072}

// ExecuteControl runs the control command of the current cursor's Scan node, as far as the cursor's stage
// runs it, and reports whether the stage may go on to the next node: false only for a generated segment at the
// prosody stage (pipeline.md §4).
func (pipeline *Pipeline) ExecuteControl() bool {
	cursor := pipeline.Current
	node := cursor.Scan
	stage := pipeline.stageOf(cursor)
	if stage == Prosody && node.Type() == Generated {
		return false
	}
	if node.Type() != Control {
		return true
	}
	letter := node.Symbol()
	value := node.Value()
	if letter != 'i' && letter != 'z' && letter != 'n' {
		value &= 0xff
	}
	second := uint32(node.Second())
	engine := pipeline.Engine
	switch letter {
	case 'A':
		cursor.InputFlags = second<<8 | value
	case 'C':
		if stage == Normalizer {
			pipeline.NewNode(node, false, Phoneme, ']')
		}
	case 'I':
		cursor.PhoneticInput = int32(value)
	case 'N':
		cursor.Flags = second<<8 | value
	case 'P':
		cursor.Flush = int32(second<<8 + value)
	case 'V':
		cursor.Voice = int32(value)
		cursor.Pitch = pipeline.tables.Voices[cursor.Voice].Pitch
		cursor.Rate = pipeline.tables.Voices[cursor.Voice].Rate
		if stage == Parameters {
			voice := int16(value)
			pipeline.Synthesizer.OutputVoice = int(voice)
			pipeline.Speed = uint32(pipeline.tables.Voices[voice].Speed)
			pipeline.Pitch = uint32(cursor.Pitch)
			if engine != nil {
				engine.Voice = int32(voice)
				engine.Pitch = uint16(pipeline.Pitch)
				engine.Speed = pipeline.Speed
			}
		}
	case 'a':
		cursor.Attenuation = int32(value)
		if stage == Parameters {
			if value == muteAttenuation {
				pipeline.Volume = silentVolume
				cursor.Attenuation = 100
			} else {
				pipeline.Volume = attenuationVolumes[value]
				pipeline.Synthesizer.Mute = false
			}
			if engine != nil {
				engine.Volume = pipeline.Volume
				engine.Messages = append(engine.Messages, Message{Kind: AttributeChanged, Attribute: VolumeAttribute})
			}
		}
	case 'c':
		if stage == Prosody && engine != nil {
			pipeline.Character = value
			engine.Character = value
		}
	case 'f':
		if cursor.Filter == 0 {
			pipeline.SavedRate = cursor.Rate
		}
		cursor.Filter = int32(value)
		cursor.Rate = int32(second)
		if cursor.Filter == 0 {
			cursor.Rate = pipeline.SavedRate
		}
	case 'g', 's', 't':
		if stage == Prosody {
			node.Data[0] = node.Data[0]&^2 | Generated
		}
	case 'i':
		if stage != Parameters {
			break
		}
		if engine != nil {
			if value == 0 && node.Text != nil {
				engine.Messages = append(engine.Messages, Message{Kind: TextDone, Text: node.Text})
			}
			if node.Text != nil {
				engine.Queue = append(engine.Queue, Item{Event: &Event{Kind: Mark, Text: node.Text, Value: value}})
				if value == 0 {
					node.Text = nil
				}
			}
		}
		pipeline.resetFrom(node, value)
	case 'l':
		pipeline.Levels[value] = byte(second)
	case 'p':
		cursor.Pitch = int32(value * 2)
		if stage == Prosody {
			pipeline.Pitch = uint32(cursor.Pitch)
			if engine != nil {
				engine.Pitch = uint16(pipeline.Pitch)
				engine.Messages = append(engine.Messages, Message{Kind: AttributeChanged, Attribute: PitchAttribute})
			}
		}
	case 'r', 'v':
		cursor.Rate = int32(value)
		if stage == Prosody {
			pipeline.Speed = (value*8 + 50) / 10 * 10
			if engine != nil {
				engine.Speed = pipeline.Speed
				engine.Messages = append(engine.Messages, Message{Kind: AttributeChanged, Attribute: SpeedAttribute})
			}
		}
	case 'x':
		if stage == Output {
			pipeline.ResetValue = 0
			pipeline.ResetRequested = true
		} else if stage == Normalizer {
			pipeline.NewNode(node, true, Control, 'C')
		}
	case 'z':
		if stage != Parameters {
			break
		}
		if engine != nil && node.Text != nil {
			engine.Queue = append(engine.Queue, Item{Event: &Event{Kind: WordPosition, Text: node.Text, Value: value}})
		}
		pipeline.resetFrom(node, value)
	}
	return true
}

// resetFrom asks for the stages to be reset after a bookmark or word command whose node has flag 0x20.
func (pipeline *Pipeline) resetFrom(node *Node, value uint32) {
	if node.Flags()&nodeResets != 0 {
		pipeline.ResetValue = value
		pipeline.ResetRequested = true
	}
}
