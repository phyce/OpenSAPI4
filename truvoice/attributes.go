package truvoice

// The engine's ranges of pitch, in Hz, and speed, in words a minute (engine.md §2.5).
const (
	lowestPitch      = 50
	highestPitch     = 400
	highestPitchSign = 0xffff
	lowestSpeed      = 50
	highestSpeed     = 250
	highestSpeedSign = 0xffffffff
	volumeBits       = 0xffff
)

// Pitch is the engine's pitch in Hz.
func (engine *Engine) Pitch() uint16 {
	return engine.state.Pitch
}

// SetPitch sets the pitch: 0 is the lowest, 50 Hz, 65535 the highest, 400 Hz, and other values out of that range
// give ErrValueOutOfRange. The texts queued take it at the speaking thread's next step.
func (engine *Engine) SetPitch(hertz uint16) error {
	switch hertz {
	case 0:
		hertz = lowestPitch
	case highestPitchSign:
		hertz = highestPitch
	}
	if hertz < lowestPitch || hertz > highestPitch {
		return ErrValueOutOfRange
	}
	engine.state.Pitch = hertz
	engine.attributeChanged(PitchAttribute)
	return nil
}

// Speed is the engine's speed in words a minute.
func (engine *Engine) Speed() uint32 {
	return engine.state.Speed
}

// SetSpeed sets the speed: 0 is the lowest, 50 words a minute, 0xFFFFFFFF the highest, 250, and other values out
// of that range give ErrValueOutOfRange.
func (engine *Engine) SetSpeed(wordsPerMinute uint32) error {
	switch wordsPerMinute {
	case 0:
		wordsPerMinute = lowestSpeed
	case highestSpeedSign:
		wordsPerMinute = highestSpeed
	}
	if wordsPerMinute < lowestSpeed || wordsPerMinute > highestSpeed {
		return ErrValueOutOfRange
	}
	engine.state.Speed = wordsPerMinute
	engine.attributeChanged(SpeedAttribute)
	return nil
}

// Volume is the engine's volume, the same in both channels: its low word in both words.
func (engine *Engine) Volume() uint32 {
	volume := engine.state.Volume
	return volume<<16 | volume
}

// SetVolume sets the volume to the value's low word, the left channel's; the high word is left out.
func (engine *Engine) SetVolume(value uint32) {
	engine.state.Volume = value & volumeBits
	engine.attributeChanged(VolumeAttribute)
}

func (engine *Engine) attributeChanged(attribute Attribute) {
	if engine.AttributeChanged != nil {
		engine.AttributeChanged(attribute)
	}
}
