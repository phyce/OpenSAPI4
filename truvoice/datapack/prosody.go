package datapack

import (
	"encoding/json"
	"fmt"
	"math"
)

const (
	RateCount     = 26
	SlowRateCount = 9
)

// Prosody is the prosody stage's tables: each voice's pitch range, a Q15 share of the pitch's excursion, a pause's
// percent of its length at each rate, at the slow rates the patterns of the pauses between words, each symbol's
// durations and a duration's percent at each rate.
type Prosody struct {
	PitchRanges       [VoiceCount]int32
	PausePercents     [RateCount]int32
	SlowPausePatterns [SlowRateCount]int32
	Durations         [SymbolCount]Duration
	DurationPercents  [RateCount]int32
}

// A Duration is a symbol's inherent and shortest durations in frames.
type Duration struct {
	Inherent, Shortest int32
}

type prosodyDescription struct {
	PitchRanges       []int64   `json:"pitchRanges"`
	PausePercents     []int64   `json:"pausePercents"`
	SlowPausePatterns []int64   `json:"slowPausePatterns"`
	Durations         [][]int64 `json:"durations"`
	DurationPercents  []int64   `json:"durationPercents"`
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
	if len(description.Durations) != SymbolCount {
		return nil, fmt.Errorf("durations has %d values, expected %d", len(description.Durations), SymbolCount)
	}
	for symbol, values := range description.Durations {
		var pair [2]int32
		if err := copyValues(pair[:], values, fmt.Sprintf("durations[%d]", symbol), 0, math.MaxUint8); err != nil {
			return nil, err
		}
		prosody.Durations[symbol] = Duration{pair[0], pair[1]}
	}
	if err := copyValues(prosody.DurationPercents[:], description.DurationPercents, "durationPercents", 0, 1000); err != nil {
		return nil, err
	}
	return prosody, nil
}
