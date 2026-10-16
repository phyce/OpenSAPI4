package truvoice

import (
	"unicode/utf16"

	"github.com/phyce/OpenSAPI4/truvoice/text"
)

// Inject changes the engine's attributes with tags, as the original's Inject does (engine.md §4): \Pit=\, \Spd=\ and
// \Vol=\ set the pitch, speed and volume, \Chr=\ the character, \Vce=Speaker=\ the voice with its pitch and speed,
// and \Rst\ goes back to the pitch, speed, volume and character the engine started with and to no context; a string
// that is exactly \gain=1\, \gain=2\ or \gain=3\ sets the gain of the next speaking thread. Pitch, speed, volume and
// \Rst\ tell AttributeChanged, the voice and the character don't. The texts queued take the changes at the speaking
// thread's next step.
func (engine *Engine) Inject(tags string) {
	state := &engine.state
	engine.converter.Inject(utf16.Encode([]rune(tags)), func(kind text.InjectKind, value uint32) {
		switch kind {
		case text.InjectGain:
			state.Gain = value
		case text.InjectReset:
			defaults := engine.converter.Defaults
			state.Speed = defaults.Speed
			state.Volume = defaults.Volume
			state.Pitch = defaults.Pitch
			state.Character = defaults.Character
			engine.converter.Context = text.NoContext
			engine.attributeChanged(PitchAttribute)
			engine.attributeChanged(SpeedAttribute)
			engine.attributeChanged(VolumeAttribute)
		case text.InjectPitch:
			state.Pitch = uint16(value)
			engine.attributeChanged(PitchAttribute)
		case text.InjectSpeed:
			state.Speed = value
			engine.attributeChanged(SpeedAttribute)
		case text.InjectVolume:
			state.Volume = value
			engine.attributeChanged(VolumeAttribute)
		case text.InjectCharacter:
			state.Character = value
		case text.InjectVoice:
			voice := engine.data.pipeline.Voices[value]
			state.Voice = int32(value)
			state.Pitch = uint16(voice.Pitch)
			state.Speed = uint32(voice.Speed)
		}
	})
}
