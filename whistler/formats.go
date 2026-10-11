package whistler

// Format is a PCM format: mono, SampleRate samples a second, and Bits 16 (signed, little-endian) or 8
// (unsigned).
type Format struct {
	SampleRate int
	Bits       int
}

// AcceptFormat is the audio destination's answer to a format the engine offers, as IAudio::WaveFormatSet
// gives it: nil when the destination takes the format, otherwise why it doesn't.
type AcceptFormat func(format Format) error

// offeredFormats are the formats the engine offers for a voice's rate, in order (interface.md §4).
func offeredFormats(sampleRate int) []Format {
	formats := []Format{{sampleRate, 16}, {sampleRate, 8}}
	otherRate := 8000
	if sampleRate == 8000 {
		otherRate = 11025
	}
	formats = append(formats, Format{otherRate, 16}, Format{otherRate, 8})
	if otherRate == 8000 {
		formats = append(formats, Format{16000, 16}, Format{16000, 8})
	}
	return formats
}

// negotiate offers the formats in order and takes the first the destination accepts. When it accepts none,
// the error is the one it gave for the last offer. A nil accept takes the first, the voice's own format.
func negotiate(sampleRate int, accept AcceptFormat) (Format, error) {
	offers := offeredFormats(sampleRate)
	if accept == nil {
		return offers[0], nil
	}
	var err error
	for _, format := range offers {
		if err = accept(format); err == nil {
			return format, nil
		}
	}
	return Format{}, err
}
