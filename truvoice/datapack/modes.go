package datapack

import (
	"encoding/json"
	"fmt"
)

// The genders of a mode, as SAPI numbers them.
const (
	GenderNeutral = 0
	GenderFemale  = 1
	GenderMale    = 2
)

// Mode is one of the engine's ten modes, as its mode information describes it. The mode's voice is its place in
// the list.
type Mode struct {
	Engine         string `json:"engine"`
	Manufacturer   string `json:"manufacturer"`
	Product        string `json:"product"`
	Mode           string `json:"mode"`
	Name           string `json:"name"`
	Language       uint16 `json:"language"`
	Dialect        string `json:"dialect"`
	Speaker        string `json:"speaker"`
	Style          string `json:"style"`
	Gender         uint16 `json:"gender"`
	Age            uint16 `json:"age"`
	Features       uint32 `json:"features"`
	Interfaces     uint32 `json:"interfaces"`
	EngineFeatures uint32 `json:"engineFeatures"`
}

type Modes struct {
	Modes [VoiceCount]Mode
}

type modesDescription struct {
	Modes []Mode `json:"modes"`
}

func (pack *Pack) LoadModes() (*Modes, error) {
	return load(pack, "modes.json", parseModes)
}

func parseModes(data []byte) (*Modes, error) {
	var description modesDescription
	if err := json.Unmarshal(data, &description); err != nil {
		return nil, err
	}
	if len(description.Modes) != VoiceCount {
		return nil, fmt.Errorf("modes has %d entries, expected %d", len(description.Modes), VoiceCount)
	}
	modes := &Modes{}
	for index, mode := range description.Modes {
		if mode.Speaker == "" || mode.Name == "" || mode.Gender > GenderMale {
			return nil, fmt.Errorf("modes[%d] has no speaker or name, or gender %d", index, mode.Gender)
		}
		modes.Modes[index] = mode
	}
	return modes, nil
}
