package effects

import (
	"github.com/phyce/OpenSAPI4/internal/locale"
	"github.com/phyce/OpenSAPI4/whistler/datapack"
	"github.com/phyce/OpenSAPI4/whistler/internal/numeric"
)

type Effects struct {
	mode       *datapack.Mode
	variants   []*datapack.Mode
	presets    []datapack.ReverbPreset
	sampleRate int

	whisper    bool
	postFilter *postFilter
	reverb     *reverb
	reverbOn   bool
	random     numeric.Random
}

func New(modes *datapack.Modes, mode *datapack.Mode, sampleRate int) *Effects {
	effects := &Effects{
		mode:       mode,
		variants:   modes.Variants(mode.Voice),
		presets:    modes.ReverbPresets,
		sampleRate: sampleRate,
		random:     numeric.NewRandom(),
	}
	effects.apply(mode)
	return effects
}

func (effects *Effects) Whisper() bool {
	return effects.whisper
}

func (effects *Effects) Apply(samples []float32, silence bool) {
	if !silence && effects.postFilter != nil {
		effects.postFilter.apply(samples)
	}
	if effects.reverbOn {
		effects.reverb.process(samples)
	}
}

// EndFrame draws the reverb's next random value, once per frame while the reverb is on.
func (effects *Effects) EndFrame() {
	if effects.reverbOn {
		effects.reverb.endFrame(&effects.random)
	}
}

func (effects *Effects) Character(name string) {
	if equalFold(name, `"normal"`) {
		effects.Reset()
		return
	}
	tag := `\Chr=` + name + `\`
	for _, variant := range effects.variants {
		if equalFold(variant.CharacterTag, tag) {
			effects.apply(variant)
			return
		}
	}
}

func (effects *Effects) Reset() {
	effects.whisper = false
	effects.postFilter = nil
	effects.reverbOn = false
	effects.apply(effects.mode)
}

func (effects *Effects) apply(mode *datapack.Mode) {
	for _, effect := range mode.Effects {
		switch effect.Effect {
		case datapack.EffectWhisper:
			effects.whisper = true
		case datapack.EffectPostFilter:
			effects.postFilter = newPostFilter(effect.Coefficients)
		case datapack.EffectReverb:
			if effects.reverb == nil {
				effects.reverb = newReverb(effects.presets[effect.Preset-1], effects.sampleRate)
			}
			effects.reverbOn = true
		}
	}
}

type Monotone struct {
	own bool
	on  bool
}

func NewMonotone(mode *datapack.Mode) *Monotone {
	own := mode.HasEffect(datapack.EffectMonotone)
	return &Monotone{own: own, on: own}
}

func (monotone *Monotone) On() bool {
	return monotone.on
}

// Character takes a name of the front end's, which compares as lstrcmpiA does.
func (monotone *Monotone) Character(name string) {
	switch locale.Key([]byte(name)) {
	case `"normal"`:
		monotone.Reset()
	case `"monotone"`:
		monotone.on = true
	}
}

func (monotone *Monotone) Reset() {
	monotone.on = monotone.own
}

func equalFold(first string, second string) bool {
	if len(first) != len(second) {
		return false
	}
	for index := range len(first) {
		if lower(first[index]) != lower(second[index]) {
			return false
		}
	}
	return true
}

func lower(character byte) byte {
	if character >= 'A' && character <= 'Z' {
		return character + 'a' - 'A'
	}
	return character
}
