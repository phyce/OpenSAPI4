package datapack

import (
	"encoding/json"
	"fmt"
	"math"
)

const (
	VoiceCount       = 10
	HalfCosineLength = 256
	SourceTypeCount  = 32
)

type Synthesizer struct {
	HalfCosine    [HalfCosineLength]int16
	SourceShapes  [SourceTypeCount]int16
	OutputFilters [VoiceCount]int32
}

type synthesizerDescription struct {
	HalfCosine    []int64 `json:"halfCosine"`
	SourceShapes  []int64 `json:"sourceShapes"`
	OutputFilters []int64 `json:"outputFilters"`
}

func (pack *Pack) LoadSynthesizer() (*Synthesizer, error) {
	return load(pack, "synthesizer.json", parseSynthesizer)
}

func parseSynthesizer(data []byte) (*Synthesizer, error) {
	var description synthesizerDescription
	if err := json.Unmarshal(data, &description); err != nil {
		return nil, err
	}
	synthesizer := &Synthesizer{}
	if err := copyValues(synthesizer.HalfCosine[:], description.HalfCosine, "halfCosine", math.MinInt16, math.MaxInt16); err != nil {
		return nil, err
	}
	if err := copyValues(synthesizer.SourceShapes[:], description.SourceShapes, "sourceShapes", math.MinInt16, math.MaxInt16); err != nil {
		return nil, err
	}
	if err := copyValues(synthesizer.OutputFilters[:], description.OutputFilters, "outputFilters", -100, 100); err != nil {
		return nil, err
	}
	return synthesizer, nil
}

func copyValues[T int16 | int32](target []T, values []int64, name string, minimum, maximum int64) error {
	if len(values) != len(target) {
		return fmt.Errorf("%s has %d values, expected %d", name, len(values), len(target))
	}
	for index, value := range values {
		if value < minimum || value > maximum {
			return fmt.Errorf("%s[%d] is %d, outside %d to %d", name, index, value, minimum, maximum)
		}
		target[index] = T(value)
	}
	return nil
}
