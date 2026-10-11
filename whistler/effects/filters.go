package effects

import (
	"github.com/phyce/OpenSAPI4/whistler/datapack"
	"github.com/phyce/OpenSAPI4/whistler/internal/numeric"
)

type postFilter struct {
	coefficients []float32
	history      []float32
}

func newPostFilter(coefficients []float32) *postFilter {
	return &postFilter{coefficients: coefficients, history: make([]float32, len(coefficients))}
}

func (filter *postFilter) apply(samples []float32) {
	coefficients := filter.coefficients
	history := filter.history
	for index, sample := range samples {
		history[0] = sample
		accumulator := float64(float64(coefficients[0]) * float64(sample))
		for k := len(coefficients) - 1; k >= 1; k-- {
			accumulator = float64(float64(coefficients[k])*float64(history[k])) + accumulator
			history[k] = history[k-1]
		}
		samples[index] = float32(accumulator)
	}
}

const (
	thousandth     = float32(0.001)
	flushThreshold = 0.001
)

type allpassLine struct {
	gain     float32
	buffer   []float32
	position int
}

type reverb struct {
	inputGain        float32
	dryGain          float32
	lengthModulation float32
	lines            []allpassLine
	randomValue      float32
	wet              []float32
}

func newReverb(preset datapack.ReverbPreset, sampleRate int) *reverb {
	samplesPerMillisecond := float32(float64(sampleRate) * float64(thousandth))
	reverb := &reverb{inputGain: preset.InputGain, dryGain: preset.DryGain, lengthModulation: preset.LengthModulation}
	for _, line := range preset.Lines {
		length := max(2, int(numeric.Truncate(float64(line.Delay)*float64(samplesPerMillisecond))))
		reverb.lines = append(reverb.lines, allpassLine{gain: line.Gain, buffer: make([]float32, length)})
	}
	return reverb
}

func (reverb *reverb) process(samples []float32) {
	if len(samples) == 0 {
		return
	}
	lengthScale := float32(1 - float64(float64(reverb.lengthModulation)*float64(reverb.randomValue)))
	if cap(reverb.wet) < len(samples) {
		reverb.wet = make([]float32, len(samples))
	}
	wet := reverb.wet[:len(samples)]
	if reverb.inputGain > 0 {
		for index, sample := range samples {
			wet[index] = float32(float64(reverb.inputGain) * float64(sample))
		}
	} else {
		clear(wet)
	}
	for index := range reverb.lines {
		reverb.lines[index].run(wet, lengthScale)
	}
	reverb.mix(samples, wet)
}

func (reverb *reverb) endFrame(random *numeric.Random) {
	reverb.randomValue = float32(random.Next()) * (1.0 / 4096)
}

func (line *allpassLine) run(data []float32, lengthScale float32) {
	end := int(numeric.Truncate(float64(len(line.buffer)) * float64(lengthScale)))
	gain := float64(line.gain)
	for index, input := range data {
		delayed := float64(line.buffer[line.position])
		value := float64(delayed*gain) + float64(input)
		stored := float32(value)
		if value > 0 {
			if !(float64(stored) >= flushThreshold) {
				stored = 0
			}
		} else if float64(stored) > -flushThreshold {
			stored = 0
		}
		line.buffer[line.position] = stored
		data[index] = float32(delayed - float64(float64(stored)*gain))
		line.position++
		if line.position >= end {
			line.position = 0
		}
	}
}

func (reverb *reverb) mix(samples []float32, wet []float32) {
	if !(reverb.dryGain > 0) {
		return
	}
	dryGain := float64(reverb.dryGain)
	for index, sample := range samples {
		value := float64(dryGain*float64(sample)) + float64(wet[index])
		switch {
		case !(value >= -32768):
			samples[index] = -32768
		case value > 32767:
			samples[index] = 32767
		default:
			samples[index] = float32(value)
		}
	}
}
