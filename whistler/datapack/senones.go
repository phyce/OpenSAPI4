package datapack

import (
	"fmt"
	"math"
)

type Senones struct {
	MeanDurations []float32
	Gains         []float32
}

func (senones *Senones) MeanDuration(senone int) float32 {
	return senones.MeanDurations[senone-1]
}

func (senones *Senones) Gain(senone int) float32 {
	return senones.Gains[senone-1]
}

func parseSenones(data []byte) (*Senones, error) {
	input, err := newReader(data, "WSEN")
	if err != nil {
		return nil, err
	}
	count := input.count(input.remaining(8))
	values := input.float32s(2 * count)
	if err := input.finish(); err != nil {
		return nil, err
	}
	senones := &Senones{MeanDurations: make([]float32, count), Gains: make([]float32, count)}
	for index := range count {
		senones.MeanDurations[index] = values[2*index]
		senones.Gains[index] = values[2*index+1]
		if !isFinite(senones.MeanDurations[index]) || !isFinite(senones.Gains[index]) || senones.Gains[index] <= 0 {
			return nil, fmt.Errorf("senone %d values", index+1)
		}
	}
	return senones, nil
}

func isFinite(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}
