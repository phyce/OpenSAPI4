package pipeline

import (
	"slices"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/synthesizer"
)

// prebufferedItems is how many items may wait in the queue while the speaking thread prebuffers.
const prebufferedItems = 6

// The engine's output gains the speaking thread takes.
const (
	lowestGain  = 1
	highestGain = 4
)

// Start prepares the pipeline for a speaking thread of its engine: it takes the engine's sample rate and, if
// it is 1 to 4, its output gain, has the synthesizer render, turns the visual events on, starts prebuffering and
// initializes the pipeline (pipeline.md §8.1).
func (pipeline *Pipeline) Start() {
	engine := pipeline.Engine
	pipeline.SampleRate = engine.SampleRate
	if engine.Gain >= lowestGain && engine.Gain <= highestGain {
		pipeline.Synthesizer.Gain = uint16(engine.Gain)
	}
	pipeline.Synthesizer.Rendering = true
	pipeline.VisualEvents = true
	pipeline.Prebuffering = true
	pipeline.Initialize()
}

// ApplyAttributes applies each of the engine's attributes that differs from the pipeline's copy, and copies
// it, as the speaking thread does before each step (pipeline.md §8.3).
func (pipeline *Pipeline) ApplyAttributes() {
	engine := pipeline.Engine
	if pitch := uint32(engine.Pitch); pipeline.Pitch != pitch {
		pipeline.Pitch = pitch
		pipeline.SetPitch(pitch)
	}
	if pipeline.Speed != engine.Speed {
		pipeline.Speed = engine.Speed
		pipeline.SetSpeed(engine.Speed)
	}
	if pipeline.Volume != engine.Volume {
		pipeline.Volume = engine.Volume
		pipeline.SetVolume(engine.Volume)
	}
	pipeline.Character = engine.Character
	if int32(pipeline.Synthesizer.OutputVoice) != engine.Voice {
		voice := int16(engine.Voice)
		pipeline.Synthesizer.OutputVoice = int(voice)
		pipeline.SetVoice(int32(voice))
	}
}

// SetPitch makes pitch, in Hz, the escape parser's and every cursor's pitch.
func (pipeline *Pipeline) SetPitch(pitch uint32) {
	pipeline.Settings.Pitch = int32(pitch)
	for stage := range pipeline.Cursors {
		pipeline.Cursors[stage].Pitch = int32(pitch)
	}
}

// SetSpeed makes speed, in words a minute, the escape parser's speed, and its rate index (speed − 46) / 8 the
// escape parser's and every cursor's rate.
func (pipeline *Pipeline) SetSpeed(speed uint32) {
	pipeline.SettingsSpeed = int32(speed)
	rate := int32((speed - 46) >> 3)
	pipeline.Settings.Rate = rate
	for stage := range pipeline.Cursors {
		pipeline.Cursors[stage].Rate = rate
	}
}

// SetVolume mutes the synthesizer for a volume below 80. Otherwise it unmutes it and makes the volume's
// attenuation the escape parser's and every cursor's.
func (pipeline *Pipeline) SetVolume(volume uint32) {
	attenuation, audible := synthesizer.Attenuation(uint16(volume))
	if !audible {
		pipeline.Synthesizer.Mute = true
		return
	}
	pipeline.Synthesizer.Mute = false
	pipeline.Settings.Attenuation = attenuation
	for stage := range pipeline.Cursors {
		pipeline.Cursors[stage].Attenuation = attenuation
	}
}

// SetVoice makes voice, if it is one of the ten, the escape parser's and every cursor's voice.
func (pipeline *Pipeline) SetVoice(voice int32) {
	if uint32(voice) >= datapack.VoiceCount {
		return
	}
	pipeline.Settings.Voice = voice
	for stage := range pipeline.Cursors {
		pipeline.Cursors[stage].Voice = voice
	}
}

// HandOver queues the audio buffer for the destination, after the events the control commands queued, and
// empties the buffer. While prebuffering, it flushes the queue only when more than six items were waiting, and
// then stops prebuffering; after, it always does (pipeline.md §8.4).
func (pipeline *Pipeline) HandOver() {
	engine := pipeline.Engine
	waiting := len(engine.Queue) - engine.Flushed
	samples := pipeline.Synthesizer.Samples
	engine.Queue = append(engine.Queue, Item{Samples: slices.Clone(samples)})
	pipeline.Synthesizer.Samples = samples[:0]
	if !pipeline.Prebuffering || waiting > prebufferedItems {
		engine.Flush()
		pipeline.Prebuffering = false
	}
}
