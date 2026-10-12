package datapack

import (
	"encoding/json"
	"fmt"
)

// ShapeCount is the number of transition shapes.
const ShapeCount = 31

// Parameters are the parameter generator's tables. Shapes are the transition shapes: shape i has at most i
// weights, 1 to 255, the first 255.
type Parameters struct {
	Shapes [ShapeCount][]byte
}

type parametersDescription struct {
	Shapes [][]int64 `json:"shapes"`
}

func (pack *Pack) LoadParameters() (*Parameters, error) {
	return load(pack, "parameters.json", parseParameters)
}

func parseParameters(data []byte) (*Parameters, error) {
	var description parametersDescription
	if err := json.Unmarshal(data, &description); err != nil {
		return nil, err
	}
	parameters := &Parameters{}
	if len(description.Shapes) != ShapeCount {
		return nil, fmt.Errorf("shapes has %d entries, expected %d", len(description.Shapes), ShapeCount)
	}
	for index, weights := range description.Shapes {
		name := fmt.Sprintf("shapes[%d]", index)
		if len(weights) > index {
			return nil, fmt.Errorf("%s has %d weights, at most %d allowed", name, len(weights), index)
		}
		if len(weights) > 0 && weights[0] != 255 {
			return nil, fmt.Errorf("%s starts with %d, expected 255", name, weights[0])
		}
		parameters.Shapes[index] = make([]byte, len(weights))
		if err := copyValues(parameters.Shapes[index], weights, name, 1, 255); err != nil {
			return nil, err
		}
	}
	return parameters, nil
}
