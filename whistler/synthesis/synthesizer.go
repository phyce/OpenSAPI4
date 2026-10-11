package synthesis

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/phyce/OpenSAPI4/whistler/datapack"
	"github.com/phyce/OpenSAPI4/whistler/internal/numeric"
)

type Voice struct {
	inventory  *datapack.Inventory
	sampleRate float32
	sine       []float32
	fade       []float32
	noise      []int8
}

func NewVoice(inventory *datapack.Inventory) *Voice {
	return &Voice{
		inventory:  inventory,
		sampleRate: float32(inventory.SampleRate),
		sine:       makeSineTable(inventory.TransformSize),
		fade:       makeFadeWindow(inventory.TransformSize),
		noise:      makeNoiseTable(inventory.SampleRate),
	}
}

func (voice *Voice) SampleRate() int {
	return voice.inventory.SampleRate
}

type Point struct {
	Time      float32
	Pitch     int16
	Amplitude float32
}

type Frame struct {
	Unit     int
	Duration float32
	Points   []Point
}

// Effects post-process the samples. Apply gets a frame's samples, in one piece or in several, and EndFrame
// follows its last piece.
type Effects interface {
	Whisper() bool
	Apply(samples []float32, silence bool)
	EndFrame()
}

type Synthesizer struct {
	voice         *Voice
	filterMemory  []float32
	noisePosition int
	Effects       Effects
	// Resampler, when set, takes the samples to the rate the destination accepted (synthesis.md §13.2).
	Resampler *Resampler
	// EightBit gives 8-bit unsigned samples instead of 16-bit little-endian ones (synthesis.md §13.1).
	EightBit bool
}

func NewSynthesizer(voice *Voice) *Synthesizer {
	return &Synthesizer{voice: voice, filterMemory: make([]float32, voice.inventory.PredictorOrder+1)}
}

func (synthesizer *Synthesizer) whisper() bool {
	return synthesizer.Effects != nil && synthesizer.Effects.Whisper()
}

func (synthesizer *Synthesizer) applyEffects(samples []float32, silence bool) {
	if synthesizer.Effects != nil {
		synthesizer.Effects.Apply(samples, silence)
	}
}

func (synthesizer *Synthesizer) endFrame() {
	if synthesizer.Effects != nil {
		synthesizer.Effects.EndFrame()
	}
}

var errCapacity = errors.New("frame needs more samples than its buffer holds")

const silencePieceLength = 1 << 14

// SynthesizeTo synthesizes a frame and gives its samples to output. A silent frame comes in pieces of at most
// 16384 samples, so a long pause is never held at once, and its length isn't limited to 32 bits. A piece can
// be empty after resampling.
func (synthesizer *Synthesizer) SynthesizeTo(frame Frame, output func(data []byte) error) error {
	if frame.Unit != 0 {
		samples, err := synthesizer.Synthesize(frame)
		if err != nil {
			return err
		}
		return output(samples)
	}
	count := int64(0)
	if value := float64(frame.Duration) * float64(synthesizer.voice.sampleRate); value > 0 && value < 1<<62 {
		count = int64(value)
	}
	clear(synthesizer.filterMemory)
	samples := make([]float32, min(count, silencePieceLength))
	for remaining := count; remaining > 0; {
		piece := samples[:min(remaining, silencePieceLength)]
		clear(piece)
		synthesizer.applyEffects(piece, true)
		if err := output(synthesizer.encode(piece)); err != nil {
			return err
		}
		remaining -= int64(len(piece))
	}
	synthesizer.endFrame()
	return nil
}

// Synthesize synthesizes a frame whole: its samples in the synthesizer's format.
func (synthesizer *Synthesizer) Synthesize(frame Frame) ([]byte, error) {
	if frame.Unit != 0 && len(frame.Points) < 2 {
		return nil, fmt.Errorf("frame of unit %d has %d points", frame.Unit, len(frame.Points))
	}
	byteCount := uint32(numeric.Truncate(math.Ceil(float64(synthesizer.voice.inventory.SampleRate)*float64(frame.Duration)*1.5) * 4.0))
	for {
		samples, err := synthesizer.synthesizeFrame(frame, int(byteCount>>2))
		if err == nil {
			synthesizer.endFrame()
			return synthesizer.encode(samples), nil
		}
		if !errors.Is(err, errCapacity) {
			return nil, err
		}
		if byteCount == 0 {
			return nil, fmt.Errorf("frame of unit %d has no buffer", frame.Unit)
		}
		byteCount <<= 1
	}
}

func (synthesizer *Synthesizer) synthesizeFrame(frame Frame, capacity int) ([]float32, error) {
	voice := synthesizer.voice
	sampleRate := float64(voice.sampleRate)
	duration := float64(frame.Duration)
	if frame.Unit == 0 {
		count := int(numeric.Truncate(duration * sampleRate))
		if count > capacity {
			return nil, errCapacity
		}
		clear(synthesizer.filterMemory)
		samples := make([]float32, max(count, 0))
		synthesizer.applyEffects(samples, true)
		return samples, nil
	}
	unit, err := synthesizer.decodeUnit(frame.Unit)
	if err != nil {
		return nil, err
	}
	positions := make([]float64, len(frame.Points))
	pitches := make([]float64, len(frame.Points))
	amplitudes := make([]float64, len(frame.Points))
	for index, point := range frame.Points {
		positions[index] = float64(float32(float64(point.Time) * duration * sampleRate))
		pitches[index] = float64(point.Pitch)
		amplitudes[index] = float64(point.Amplitude)
	}
	timeScale := float64(float32(sampleRate * duration / float64(len(unit.excitation))))
	plan := planOutputPeriods(unit.periods, timeScale, sampleRate, positions, pitches)
	count := 0
	for _, period := range plan {
		count += period.length
	}
	if count > capacity {
		return nil, errCapacity
	}
	samples := make([]float32, count)
	position := 0
	for _, period := range plan {
		output := samples[position : position+period.length]
		amplitude := float32(pointValue(float64(position), positions, amplitudes))
		source := unit.excitation[unit.starts[period.frame] : unit.starts[period.frame]+unit.lengths[period.frame]]
		copyPeriod(source, output, amplitude, period.reversed, voice.fade)
		position += period.length
	}
	position = 0
	for _, period := range plan {
		allPoleFilter(unit.coefficients[period.frame], synthesizer.filterMemory, samples[position:position+period.length])
		position += period.length
	}
	synthesizer.applyEffects(samples, false)
	return samples, nil
}

func pointValue(position float64, positions []float64, values []float64) float64 {
	segment := 1
	for segment < len(positions)-1 && position > positions[segment] {
		segment++
	}
	first := values[segment-1]
	origin := positions[segment-1]
	return float64((values[segment]-first)*(position-origin))/(positions[segment]-origin) + first
}

type plannedPeriod struct {
	frame    int
	reversed bool
	length   int
}

func planOutputPeriods(periods []int32, timeScale float64, sampleRate float64, positions []float64, pitches []float64) []plannedPeriod {
	var plan []plannedPeriod
	accumulator := 0.0
	var sourceTime float32
	frame := 0
	outputPosition := 0
	copies := 0
	reversed := false
	for frame < len(periods) {
		period := float64(periods[frame])
		var sourceLength, outputLength int
		unvoiced := !(period >= 0)
		if unvoiced {
			sourceLength = int(numeric.Truncate(-period + 0.5))
			outputLength = sourceLength
		} else {
			pitch := pointValue(float64(outputPosition), positions, pitches)
			sourceLength = int(numeric.Truncate(period + 0.5))
			outputLength = int(numeric.Truncate(sampleRate / pitch))
			if outputLength > 2*sourceLength {
				outputLength = 2 * sourceLength
			}
			if sourceLength > 2*outputLength {
				outputLength = sourceLength / 2
			}
		}
		half := float64(outputLength) * -0.5 / timeScale
		storedHalf := float64(float32(half))
		sourceEnd := float32(float64(sourceLength) + float64(sourceTime))
		skipped := accumulator-half > float64(sourceEnd)
		if !skipped {
			reversed = unvoiced && copies%2 == 1
			accumulator = float64(outputLength)/timeScale + accumulator
			plan = append(plan, plannedPeriod{frame: frame, reversed: reversed, length: outputLength})
			outputPosition += outputLength
			copies++
		}
		if accumulator-storedHalf > float64(sourceEnd) || copies >= 3 || reversed || skipped {
			frame++
			copies = 0
			sourceTime = sourceEnd
		}
	}
	return plan
}

func copyPeriod(source []float32, output []float32, amplitude float32, reversed bool, fade []float32) {
	switch {
	case reversed:
		for index := range output {
			output[index] = source[len(output)-1-index]
		}
	case len(source) == len(output):
		copy(output, source)
	default:
		resizePeriod(source, output, fade)
	}
	if amplitude != 1 && amplitude == amplitude {
		for index := range output {
			output[index] = float32(float64(amplitude) * float64(output[index]))
		}
	}
}

func resizePeriod(source []float32, output []float32, fade []float32) {
	if len(output) == 0 {
		return
	}
	clear(output)
	count := min(len(source), len(output))
	output[0] = source[0]
	step := float64(len(fade)) / float64(count)
	position := 0.5
	for index := 1; index < count; index++ {
		position += step
		weight := float64(fade[numeric.Truncate(position)])
		output[index] = float32(float64(float64(source[index])*weight) + float64(output[index]))
		output[len(output)-index] = float32(float64(weight*float64(source[len(source)-index])) + float64(output[len(output)-index]))
	}
}

func allPoleFilter(coefficients []float32, memory []float32, samples []float32) {
	order := len(coefficients) - 1
	for position, sample := range samples {
		memory[0] = sample
		for k := order; k >= 1; k-- {
			memory[0] = float32(float64(memory[0]) - float64(float64(coefficients[k])*float64(memory[k])))
			memory[k] = memory[k-1]
		}
		samples[position] = memory[0]
	}
}

// encode resamples, clips and converts a frame's samples (synthesis.md §13).
func (synthesizer *Synthesizer) encode(samples []float32) []byte {
	if synthesizer.Resampler != nil {
		samples = synthesizer.Resampler.Process(samples)
	}
	if synthesizer.EightBit {
		return convertTo8Bit(samples)
	}
	return convertTo16Bit(samples)
}

func clip(value float32) float32 {
	switch {
	case value > 32767:
		return 32767
	case !(value >= -32768):
		return -32768
	}
	return value
}

func convertTo16Bit(samples []float32) []byte {
	data := make([]byte, 2*len(samples))
	for index, value := range samples {
		binary.LittleEndian.PutUint16(data[2*index:], uint16(int16(math.RoundToEven(float64(clip(value))))))
	}
	return data
}

// convertTo8Bit rounds half up and then floors to 8 bits.
func convertTo8Bit(samples []float32) []byte {
	data := make([]byte, len(samples))
	for index, value := range samples {
		data[index] = byte(int32(math.Floor(float64(clip(value))+0.5))>>8) ^ 0x80
	}
	return data
}
