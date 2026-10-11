package datapack

import (
	"encoding/json"
	"fmt"
)

const (
	TrackCount   = 22
	MinimumPitch = 50
	MaximumPitch = 400
	MaximumRate  = 25
	MinimumSpeed = 50
	MaximumSpeed = 250
)

// VoiceDefaults are a voice's own pitch in Hz, rate index and speed in words a minute.
type VoiceDefaults struct {
	Pitch int32
	Rate  int32
	Speed int32
}

type Pipeline struct {
	Voices         [VoiceCount]VoiceDefaults
	InitialFrame   [TrackCount]byte
	GeneratedFrame [TrackCount]byte
}

type voiceDefaultsDescription struct {
	Pitch *int64 `json:"pitch"`
	Rate  *int64 `json:"rate"`
	Speed *int64 `json:"speed"`
}

type pipelineDescription struct {
	Voices         []voiceDefaultsDescription `json:"voices"`
	InitialFrame   []int64                    `json:"initialFrame"`
	GeneratedFrame []int64                    `json:"generatedFrame"`
}

func (pack *Pack) LoadPipeline() (*Pipeline, error) {
	return load(pack, "pipeline.json", parsePipeline)
}

func parsePipeline(data []byte) (*Pipeline, error) {
	var description pipelineDescription
	if err := json.Unmarshal(data, &description); err != nil {
		return nil, err
	}
	pipeline := &Pipeline{}
	if len(description.Voices) != VoiceCount {
		return nil, fmt.Errorf("voices has %d entries, expected %d", len(description.Voices), VoiceCount)
	}
	for index, voice := range description.Voices {
		target := &pipeline.Voices[index]
		for _, field := range []struct {
			name             string
			value            *int64
			target           *int32
			minimum, maximum int64
		}{
			{"pitch", voice.Pitch, &target.Pitch, MinimumPitch, MaximumPitch},
			{"rate", voice.Rate, &target.Rate, 0, MaximumRate},
			{"speed", voice.Speed, &target.Speed, MinimumSpeed, MaximumSpeed},
		} {
			name := fmt.Sprintf("voices[%d].%s", index, field.name)
			if field.value == nil {
				return nil, fmt.Errorf("%s is missing", name)
			}
			if *field.value < field.minimum || *field.value > field.maximum {
				return nil, fmt.Errorf("%s is %d, outside %d to %d", name, *field.value, field.minimum, field.maximum)
			}
			*field.target = int32(*field.value)
		}
	}
	if err := copyValues(pipeline.InitialFrame[:], description.InitialFrame, "initialFrame", 0, 255); err != nil {
		return nil, err
	}
	if err := copyValues(pipeline.GeneratedFrame[:], description.GeneratedFrame, "generatedFrame", 0, 255); err != nil {
		return nil, err
	}
	return pipeline, nil
}
