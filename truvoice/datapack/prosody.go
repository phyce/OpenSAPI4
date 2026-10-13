package datapack

import (
	"encoding/json"
	"math"
)

// Prosody is the prosody stage's tables: each voice's pitch range, a Q15 share of the pitch's excursion.
type Prosody struct {
	PitchRanges [VoiceCount]int32
}

type prosodyDescription struct {
	PitchRanges []int64 `json:"pitchRanges"`
}

func (pack *Pack) LoadProsody() (*Prosody, error) {
	return load(pack, "prosody.json", parseProsody)
}

func parseProsody(data []byte) (*Prosody, error) {
	var description prosodyDescription
	if err := json.Unmarshal(data, &description); err != nil {
		return nil, err
	}
	prosody := &Prosody{}
	if err := copyValues(prosody.PitchRanges[:], description.PitchRanges, "pitchRanges", 0, math.MaxInt16); err != nil {
		return nil, err
	}
	return prosody, nil
}
