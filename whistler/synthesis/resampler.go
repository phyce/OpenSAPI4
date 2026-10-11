package synthesis

import "github.com/phyce/OpenSAPI4/whistler/datapack"

// Resampler takes a voice's samples to another rate as the original does (synthesis.md §13.2): it puts up − 1
// zeros after each sample, filters with its taps and keeps every down-th sample. Each frame goes through it
// whole; Reset starts a text.
type Resampler struct {
	up      int
	down    int
	taps    []float32
	history []float32
	phase   int
	start   int
	buffer  []float32
}

type resamplerShape struct {
	voiceRate  int
	outputRate int
	up         int
	down       int
	history    int
	taps       func(tables *datapack.Resamplers) []float32
}

var resamplerShapes = []resamplerShape{
	{22050, 8000, 4, 11, 100, func(tables *datapack.Resamplers) []float32 { return tables.From22050To8000 }},
	{22050, 16000, 8, 11, 75, func(tables *datapack.Resamplers) []float32 { return tables.From22050To16000 }},
	{11025, 8000, 8, 11, 75, func(tables *datapack.Resamplers) []float32 { return tables.From22050To16000 }},
	{8000, 11025, 11, 8, 37, func(tables *datapack.Resamplers) []float32 { return tables.From8000To11025 }},
}

// NewResampler gives the resampler from a voice's rate to the rate the destination accepted, reset, or nil
// when the original has none for the pair.
func NewResampler(tables *datapack.Resamplers, voiceRate int, outputRate int) *Resampler {
	for _, shape := range resamplerShapes {
		if shape.voiceRate == voiceRate && shape.outputRate == outputRate {
			resampler := &Resampler{up: shape.up, down: shape.down, taps: shape.taps(tables), history: make([]float32, shape.history)}
			resampler.Reset()
			return resampler
		}
	}
	return nil
}

func (resampler *Resampler) Reset() {
	clear(resampler.history)
	resampler.phase = -1
	resampler.start = len(resampler.history)
}

// Process resamples a frame. A text's first frame of at most the history's length gives nothing, and the
// next one then sees part of the zeros the reset left.
func (resampler *Resampler) Process(samples []float32) []float32 {
	buffer := append(append(resampler.buffer[:0], resampler.history...), samples...)
	resampler.buffer = buffer
	up, down, taps := resampler.up, resampler.down, resampler.taps
	output := make([]float32, 0, (len(samples)+1)*up/down+1)
	phase := resampler.phase
	for index := resampler.start; index < len(samples); index++ {
		phase += up
		if phase < 0 {
			continue
		}
		for repeat := (phase + down) / down; repeat > 0; repeat-- {
			total := 0.0
			for tap, position := phase, index; tap < len(taps); tap, position = tap+up, position+1 {
				total += float64(float64(buffer[position]) * float64(taps[tap]))
			}
			output = append(output, float32(total))
			phase -= down
		}
	}
	resampler.phase = phase
	resampler.start = 0
	copy(resampler.history, buffer[len(samples):])
	return output
}
