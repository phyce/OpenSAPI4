package datapack

import (
	"encoding/json"
	"fmt"
)

const (
	FilterCount         = 10
	HighCharacterCount  = 64
	FirstHighCharacter  = 0xc0
	highCharacterLength = 2
)

// Input is the escape parser's tables: the letters it writes for each character from 0xC0 to 0xFF, one or two,
// the highest rate index each filter value allows, and the highest level of each track.
type Input struct {
	HighCharacters [HighCharacterCount][highCharacterLength]byte
	FilterRates    [FilterCount]int32
	LevelLimits    [TrackCount]int32
}

type inputDescription struct {
	HighCharacters []string `json:"highCharacters"`
	FilterRates    []int64  `json:"filterRates"`
	LevelLimits    []int64  `json:"levelLimits"`
}

func (pack *Pack) LoadInput() (*Input, error) {
	return load(pack, "input.json", parseInput)
}

func parseInput(data []byte) (*Input, error) {
	var description inputDescription
	if err := json.Unmarshal(data, &description); err != nil {
		return nil, err
	}
	input := &Input{}
	if len(description.HighCharacters) != HighCharacterCount {
		return nil, fmt.Errorf("highCharacters has %d entries, expected %d", len(description.HighCharacters), HighCharacterCount)
	}
	for index, letters := range description.HighCharacters {
		if len(letters) < 1 || len(letters) > highCharacterLength {
			return nil, fmt.Errorf("highCharacters[%d] is %q, not one or two characters", index, letters)
		}
		for position := range len(letters) {
			if letters[position] < ' ' || letters[position] > '~' {
				return nil, fmt.Errorf("highCharacters[%d] is %q, not printable ASCII", index, letters)
			}
			input.HighCharacters[index][position] = letters[position]
		}
	}
	if err := copyValues(input.FilterRates[:], description.FilterRates, "filterRates", 0, MaximumRate); err != nil {
		return nil, err
	}
	if err := copyValues(input.LevelLimits[:], description.LevelLimits, "levelLimits", 0, 255); err != nil {
		return nil, err
	}
	return input, nil
}
