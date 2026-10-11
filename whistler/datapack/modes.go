package datapack

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

const (
	EffectWhisper    = "whisper"
	EffectPostFilter = "postFilter"
	EffectReverb     = "reverb"
	EffectMonotone   = "monotone"
)

type Effect struct {
	Effect       string    `json:"effect"`
	Coefficients []float32 `json:"coefficients"`
	Preset       int       `json:"preset"`
}

type Mode struct {
	Name         string   `json:"name"`
	ModeID       string   `json:"modeId"`
	Voice        string   `json:"voice"`
	Variant      int      `json:"variant"`
	Manufacturer string   `json:"manufacturer"`
	Language     uint16   `json:"language"`
	Dialect      string   `json:"dialect"`
	Speaker      string   `json:"speaker"`
	Style        string   `json:"style"`
	Gender       string   `json:"gender"`
	Age          uint16   `json:"age"`
	Features     uint32   `json:"features"`
	Speed        int      `json:"speed"`
	Pitch        int      `json:"pitch"`
	Preferred    bool     `json:"preferred"`
	CharacterTag string   `json:"characterTag"`
	Effects      []Effect `json:"effects"`
}

func (mode *Mode) Hidden() bool {
	return mode.ModeID == ""
}

func (mode *Mode) HasEffect(name string) bool {
	return slices.ContainsFunc(mode.Effects, func(effect Effect) bool { return effect.Effect == name })
}

type ReverbLine struct {
	Delay float32 `json:"delay"`
	Gain  float32 `json:"gain"`
}

type ReverbPreset struct {
	InputGain        float32      `json:"inputGain"`
	DryGain          float32      `json:"dryGain"`
	LengthModulation float32      `json:"lengthModulation"`
	Lines            []ReverbLine `json:"lines"`
}

type Modes struct {
	Modes         []Mode         `json:"modes"`
	ReverbPresets []ReverbPreset `json:"reverbPresets"`
}

func (pack *Pack) LoadModes() (*Modes, error) {
	return load(pack, "modes.json", func(data []byte) (*Modes, error) {
		var modes Modes
		if err := json.Unmarshal(data, &modes); err != nil {
			return nil, err
		}
		if err := modes.validate(pack.Manifest.Voices); err != nil {
			return nil, err
		}
		return &modes, nil
	})
}

func (modes *Modes) Find(modeID string) (*Mode, bool) {
	for index := range modes.Modes {
		mode := &modes.Modes[index]
		if !mode.Hidden() && strings.EqualFold(mode.ModeID, modeID) {
			return mode, true
		}
	}
	return nil, false
}

func (modes *Modes) Variants(voice string) []*Mode {
	var variants []*Mode
	for index := range modes.Modes {
		if mode := &modes.Modes[index]; mode.Voice == voice && mode.Variant > 0 {
			variants = append(variants, mode)
		}
	}
	return variants
}

var modeIDPattern = regexp.MustCompile(`^\{[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}\}$`)

func (modes *Modes) validate(voices []string) error {
	if len(modes.Modes) == 0 {
		return fmt.Errorf("no modes")
	}
	for number, preset := range modes.ReverbPresets {
		if err := preset.validate(); err != nil {
			return fmt.Errorf("reverb preset %d: %w", number+1, err)
		}
	}
	variantCounts := map[string]int{}
	modeIDs := map[string]bool{}
	for index := range modes.Modes {
		mode := &modes.Modes[index]
		if err := mode.validate(voices, len(modes.ReverbPresets)); err != nil {
			return fmt.Errorf("mode %q: %w", mode.Name, err)
		}
		count, baseSeen := variantCounts[mode.Voice]
		switch {
		case mode.Variant == 0 && baseSeen:
			return fmt.Errorf("mode %q: second base mode of voice %s", mode.Name, mode.Voice)
		case mode.Variant > 0 && (!baseSeen || mode.Variant != count+1):
			return fmt.Errorf("mode %q: variant %d of voice %s out of order", mode.Name, mode.Variant, mode.Voice)
		}
		variantCounts[mode.Voice] = mode.Variant
		if !mode.Hidden() {
			key := strings.ToUpper(mode.ModeID)
			if modeIDs[key] {
				return fmt.Errorf("mode id %s is listed twice", mode.ModeID)
			}
			modeIDs[key] = true
		}
	}
	return nil
}

func (mode *Mode) validate(voices []string, presetCount int) error {
	switch {
	case mode.Name == "":
		return fmt.Errorf("no name")
	case !slices.Contains(voices, mode.Voice):
		return fmt.Errorf("unknown voice %q", mode.Voice)
	case mode.Variant < 0:
		return fmt.Errorf("variant %d", mode.Variant)
	case !mode.Hidden() && !modeIDPattern.MatchString(mode.ModeID):
		return fmt.Errorf("mode id %q", mode.ModeID)
	case mode.Gender != "female" && mode.Gender != "male":
		return fmt.Errorf("gender %q", mode.Gender)
	case mode.Speed < 1 || mode.Speed > 0xFFFE || mode.Pitch < 1 || mode.Pitch > 0xFFFE:
		return fmt.Errorf("speed %d, pitch %d", mode.Speed, mode.Pitch)
	case mode.Variant == 0 && (mode.Hidden() || len(mode.Effects) > 0 || mode.CharacterTag != ""):
		return fmt.Errorf("a base mode is hidden or has effects or a character tag")
	}
	reverbs := 0
	for _, effect := range mode.Effects {
		switch effect.Effect {
		case EffectWhisper, EffectMonotone:
			if effect.Coefficients != nil || effect.Preset != 0 {
				return fmt.Errorf("%s effect with values", effect.Effect)
			}
		case EffectPostFilter:
			if len(effect.Coefficients) == 0 || len(effect.Coefficients) >= 0x10000 || effect.Preset != 0 {
				return fmt.Errorf("post-filter of %d coefficients with preset %d", len(effect.Coefficients), effect.Preset)
			}
		case EffectReverb:
			reverbs++
			if reverbs > 1 {
				return fmt.Errorf("more than one reverb")
			}
			if effect.Preset < 1 || effect.Preset > presetCount || effect.Coefficients != nil {
				return fmt.Errorf("reverb preset %d of %d", effect.Preset, presetCount)
			}
		default:
			return fmt.Errorf("unknown effect %q", effect.Effect)
		}
	}
	return nil
}

func (preset *ReverbPreset) validate() error {
	if len(preset.Lines) == 0 || len(preset.Lines) > 4 {
		return fmt.Errorf("%d lines", len(preset.Lines))
	}
	if !(preset.LengthModulation >= 0 && preset.LengthModulation <= 0.125) {
		return fmt.Errorf("length modulation %g", preset.LengthModulation)
	}
	for _, line := range preset.Lines {
		if !(line.Delay > 0 && line.Delay <= 10000) {
			return fmt.Errorf("line of %g ms", line.Delay)
		}
	}
	return nil
}
