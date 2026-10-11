package datapack

import "fmt"

// Resamplers holds the resamplers' tap tables (synthesis.md §13.2). The 11025 → 8000 Hz resampler uses
// From22050To16000.
type Resamplers struct {
	From22050To8000  []float32
	From22050To16000 []float32
	From8000To11025  []float32
}

var resamplerTapCounts = [3]int{401, 601, 401}

func (pack *Pack) LoadResamplers() (*Resamplers, error) {
	return load(pack, "resamplers.bin", parseResamplers)
}

func parseResamplers(data []byte) (*Resamplers, error) {
	input, err := newReader(data, "WRSM")
	if err != nil {
		return nil, err
	}
	var tables [3][]float32
	for index, expected := range resamplerTapCounts {
		count := input.count(input.remaining(4))
		tables[index] = input.float32s(count)
		if input.err != nil {
			return nil, input.err
		}
		if count != expected {
			return nil, fmt.Errorf("resampler table %d has %d taps, expected %d", index+1, count, expected)
		}
		for tap, value := range tables[index] {
			if !isFinite(value) {
				return nil, fmt.Errorf("resampler table %d tap %d is not finite", index+1, tap)
			}
		}
	}
	if err := input.finish(); err != nil {
		return nil, err
	}
	return &Resamplers{From22050To8000: tables[0], From22050To16000: tables[1], From8000To11025: tables[2]}, nil
}
