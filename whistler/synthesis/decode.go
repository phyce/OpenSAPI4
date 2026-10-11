package synthesis

import (
	"cmp"
	"fmt"
	"math"
	"slices"
)

var noiseGainFactor = float64(float32(0.02))

type decodedUnit struct {
	periods      []int32
	lengths      []int
	starts       []int
	coefficients [][]float32
	excitation   []float32
}

type lineSpectralValue struct {
	frequency float32
	cosine    float64
}

func (synthesizer *Synthesizer) predictor(indices []byte) []float32 {
	inventory := synthesizer.voice.inventory
	order := inventory.PredictorOrder
	values := make([]lineSpectralValue, 0, order)
	for book, row := range indices {
		codebook := &inventory.LineSpectralCodebooks[book]
		start := int(row) * codebook.Columns
		for column := range codebook.Columns {
			values = append(values, lineSpectralValue{codebook.Values[start+column], codebook.Cosines[start+column]})
		}
	}
	for index := 1; index < order; index++ {
		if !(values[index].frequency > values[index-1].frequency) {
			slices.SortStableFunc(values, func(first, second lineSpectralValue) int {
				return cmp.Compare(first.frequency, second.frequency)
			})
			break
		}
	}
	half := order / 2
	firstP := make([]float64, half)
	secondP := make([]float64, half)
	firstQ := make([]float64, half)
	secondQ := make([]float64, half)
	coefficients := make([]float32, order+1)
	coefficients[0] = 1
	previous := 0.0
	for step := 0; step <= order; step++ {
		impulse := 0.0
		if step == 0 {
			impulse = 1
		}
		inputP := previous + impulse
		inputQ := impulse - previous
		previous = impulse
		for section := range half {
			outputP := float64(values[2*section].cosine*firstP[section]) + secondP[section] + inputP
			outputQ := float64(values[2*section+1].cosine*firstQ[section]) + secondQ[section] + inputQ
			secondP[section] = firstP[section]
			firstP[section] = inputP
			secondQ[section] = firstQ[section]
			firstQ[section] = inputQ
			inputP = outputP
			inputQ = outputQ
		}
		if step > 0 {
			coefficients[step] = float32((inputP + inputQ) * 0.5)
		}
	}
	return coefficients
}

func setSpectrumBand(spectrum []float32, vector []float32, start int, half int, size int, add bool) {
	imaginary := size - start - half + 1
	if add {
		for index := range half {
			spectrum[start+index] = float32(float64(vector[index]) + float64(spectrum[start+index]))
			spectrum[imaginary+index] = float32(float64(spectrum[imaginary+index]) + float64(vector[half+index]))
		}
		return
	}
	copy(spectrum[start:start+half], vector[:half])
	copy(spectrum[imaginary:imaginary+half], vector[half:2*half])
}

func foldPulseIntoPeriod(output []float32, length int, pulse []float32, size int) {
	count := min(size/2, length)
	copy(output[:count], pulse[:count])
	clear(output[count:length])
	tail := length - count
	for index := range count {
		output[tail+index] = float32(float64(pulse[size-count+index]) + float64(output[tail+index]))
	}
}

func (synthesizer *Synthesizer) decodeUnit(unit int) (*decodedUnit, error) {
	voice := synthesizer.voice
	inventory := voice.inventory
	record, found := inventory.Record(unit)
	if !found {
		return nil, fmt.Errorf("unit %d is not in the inventory", unit)
	}
	size := inventory.TransformSize
	frameCount := len(record.Periods)
	lineCount := len(inventory.LineSpectralCodebooks)
	decoded := &decodedUnit{
		periods:      slices.Clone(record.Periods),
		lengths:      make([]int, frameCount),
		starts:       make([]int, frameCount),
		coefficients: make([][]float32, frameCount),
	}
	total := 0
	for frame, period := range record.Periods {
		decoded.lengths[frame] = int(math.Abs(float64(period)))
		decoded.starts[frame] = total
		total += decoded.lengths[frame]
		decoded.coefficients[frame] = synthesizer.predictor(record.LineSpectralIndices[frame*lineCount : (frame+1)*lineCount])
	}
	decoded.excitation = make([]float32, total)
	whisper := synthesizer.whisper()
	spectrum := make([]float32, size)
	work := make([]float32, size)
	spectrumIndices := record.SpectrumIndices
	for frame, period := range record.Periods {
		length := decoded.lengths[frame]
		output := decoded.excitation[decoded.starts[frame] : decoded.starts[frame]+length]
		gain := float64(record.Gains[frame])
		if period <= 0 || whisper {
			factor := gain * noiseGainFactor
			if length+synthesizer.noisePosition >= len(voice.noise) {
				synthesizer.noisePosition = 0
			}
			for index := range output {
				output[index] = float32(float64(voice.noise[synthesizer.noisePosition+index]) * factor)
			}
			synthesizer.noisePosition += length
		} else {
			codebooks := inventory.FirstSpectrumCodebooks
			delta := len(inventory.DeltaSpectrumCodebooks) > 0 && frame != 0 && !(decoded.periods[frame-1] < 0)
			if delta {
				codebooks = inventory.DeltaSpectrumCodebooks
			} else {
				clear(spectrum)
			}
			start := 1
			for book := range codebooks {
				codebook := &codebooks[book]
				half := codebook.Columns / 2
				setSpectrumBand(spectrum, codebook.Row(spectrumIndices[book]), start, half, size, delta)
				start += half
			}
			spectrumIndices = spectrumIndices[len(codebooks):]
			copy(work, spectrum)
			inverseRealTransform(work, size, inventory.TransformExponent, voice.sine)
			for index := range work {
				work[index] = float32(gain * float64(work[index]))
			}
			foldPulseIntoPeriod(output, length, work, size)
		}
		if period > 0 && whisper {
			decoded.periods[frame] = -period
		}
	}
	return decoded, nil
}
