package datapack

import (
	"encoding/json"
	"math"
)

const (
	RateCount     = 26
	SlowRateCount = 9
)

// Prosody is the prosody stage's tables: each voice's pitch range, a Q15 share of the pitch's excursion, a pause's
// percent of its length at each rate, and at the slow rates the patterns of the pauses between words.
type Prosody struct {
	PitchRanges       [VoiceCount]int32
	PausePercents     [RateCount]int32
	SlowPausePatterns [SlowRateCount]int32
}

type prosodyDescription struct {
	PitchRanges       []int64 `json:"pitchRanges"`
	PausePercents     []int64 `json:"pausePercents"`
	SlowPausePatterns []int64 `json:"slowPausePatterns"`
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
	if err := copyValues(prosody.PausePercents[:], description.PausePercents, "pausePercents", 0, 10000); err != nil {
		return nil, err
	}
	if err := copyValues(prosody.SlowPausePatterns[:], description.SlowPausePatterns, "slowPausePatterns", 0, math.MaxUint16); err != nil {
		return nil, err
	}
	return prosody, nil
}
