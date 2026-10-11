package units

import (
	"math"

	"github.com/phyce/OpenSAPI4/whistler/internal/numeric"
	"github.com/phyce/OpenSAPI4/whistler/synthesis"
)

// buildWord makes the frames of a word: each record's duration and gains (units.md §6, §7.1), the
// interpolation of its pitches and amplitudes (§7.2), and a slow pause after it when the next phone isn't SIL.
func (mapper *Mapper) buildWord(items []Item, records []Record, phones []phoneState, next int) []Item {
	voice := mapper.voice
	work := make([]workRecord, len(phones))
	for index, state := range phones {
		record, entry := &records[state.record], &work[index]
		entry.count = min(len(record.Points), maximumPoints)
		for point := range entry.count {
			entry.slots[position(point)] = record.Points[point].Position
			entry.slots[value(pitchPass, point)] = record.Points[point].Pitch
			entry.slots[value(amplitudePass, point)] = record.Points[point].Amplitude
		}
		amplitudes := entry.slots[value(amplitudePass, 0):value(amplitudePass, entry.count)]
		if state.phone == voice.SilencePhone {
			for point := range amplitudes {
				amplitudes[point] = 1
			}
			entry.duration = mapper.silenceDuration(record.Duration)
			continue
		}
		root := math.Sqrt(float64(voice.Senones.Gain(state.senone)))
		for point, amplitude := range amplitudes {
			switch {
			case amplitude == 0:
				amplitudes[point] = float32(root)
			case amplitude > 0:
				amplitudes[point] = float32(root * float64(amplitude))
			}
		}
		entry.duration = mapper.phoneDuration(record.Duration, voice.Senones.MeanDuration(state.senone))
	}
	endsInSilence := phones[len(phones)-1].phone == voice.SilencePhone
	mapper.interpolate(work, pitchPass, endsInSilence)
	mapper.interpolate(work, amplitudePass, endsInSilence)
	factor := float32(float64(mapper.volume) * float64(volumeScale))
	for index, state := range phones {
		entry := &work[index]
		unit := state.senone
		if state.alternative > voice.Trees.SenoneCount {
			unit = state.alternative
		}
		points := make([]synthesis.Point, entry.count)
		for point := range points {
			points[point] = synthesis.Point{Time: entry.slots[position(point)], Pitch: int16(numeric.Truncate(float64(entry.slots[value(pitchPass, point)]))),
				Amplitude: float32(float64(entry.slots[value(amplitudePass, point)]) * float64(factor))}
		}
		items = append(items, Item{Frame: &Frame{Frame: synthesis.Frame{Unit: unit, Duration: entry.duration, Points: points}, Phone: state.phone}})
	}
	if mapper.slow && next != voice.SilencePhone {
		items = append(items, Item{Frame: &Frame{Frame: synthesis.Frame{Duration: mapper.offset}, Phone: voice.SilencePhone}})
	}
	return items
}

func durationCode(code float32) float32 {
	if code == 0 || math.IsNaN(float64(code)) {
		return 1
	}
	return code
}

func (mapper *Mapper) phoneDuration(code float32, mean float32) float32 {
	code = durationCode(code)
	switch {
	case code > 5:
		return float32((float64(code) - 5) * float64(mean) * float64(thousandth))
	case code > 0:
		return float32(float64(float32(float64(mapper.scale)*float64(mean)*float64(code))) * float64(thousandth))
	default:
		return float32(float64(code) * float64(-thousandth))
	}
}

func (mapper *Mapper) silenceDuration(code float32) float32 {
	code = durationCode(code)
	switch {
	case code > 5:
		return float32(float64((float64(code)-5)*float64(mapper.scale)) + float64(mapper.offset))
	case code > 0:
		return float32(float64(float64(mapper.scale)*float64(code)) + float64(mapper.offset))
	default:
		return float32(-float64(mapper.scale) * float64(code) * 0.001)
	}
}
