package datapack

import (
	"encoding/json"
	"fmt"
	"math"
)

const (
	VoiceCount           = 10
	HalfCosineLength     = 256
	SourceTypeCount      = 32
	ScaleCount           = 7
	DepthCount           = 16
	DeviationCount       = 64
	PhaseStepCount       = 140
	AmplitudeScaleCount  = 18
	DecibelCount         = 213
	FormantLevelCount    = 121
	ProximityCount       = 118
	BlockLength          = 40
	BandwidthCount       = 180
	CosineCount          = 700
	NasalZeroGainCount   = 18
	StandardSampleRate   = 11025
	TelephoneSampleRate  = 8000
	maximumAspiration    = 107
	maximumBandwidthHz   = 4*BandwidthCount - 1
	maximumVoicePercent  = 100
	maximumOutputPercent = 100
)

type Voice struct {
	Scales       [ScaleCount]int32
	Bandwidth4   int32
	Formant5     int32
	Bandwidth5   int32
	Jitter       int32
	Shimmer      int32
	Aspiration   int32
	SourceType   int32
	Amplitude    int32
	SourceGain   int32
	OutputFilter int32
}

type Rate struct {
	ResetBlock     [BlockLength]int16
	SourceStage    [3]int16
	NasalZeroB     int32
	Radii          [BandwidthCount]int32
	SquaredRadii   [BandwidthCount]int32
	Cosines        [CosineCount]int32
	NasalZeroGains [NasalZeroGainCount]int32
}

type Synthesizer struct {
	HalfCosine       [HalfCosineLength]int16
	SourceShapes     [SourceTypeCount]int16
	OpenPhases       [SourceTypeCount]int32
	SourceAmplitudes [SourceTypeCount]int32
	PhaseSteps       [PhaseStepCount]int32
	AmplitudeScales  [AmplitudeScaleCount]int32
	JitterDepths     [DepthCount]int32
	ShimmerDepths    [DepthCount]int32
	Deviations       [DeviationCount]int32
	Decibels         [DecibelCount]int32
	FormantLevels    [FormantLevelCount]int32
	Proximity        [ProximityCount]int32
	Voices           [VoiceCount]Voice
	Standard         Rate
	Telephone        Rate
}

// ForSampleRate returns the tables of the sample rate: the telephone rate's at 8000 Hz, the standard rate's at
// any other.
func (synthesizer *Synthesizer) ForSampleRate(sampleRate int) *Rate {
	if sampleRate == TelephoneSampleRate {
		return &synthesizer.Telephone
	}
	return &synthesizer.Standard
}

type voiceDescription struct {
	Scales       []int64 `json:"scales"`
	Bandwidth4   *int64  `json:"bandwidth4"`
	Formant5     *int64  `json:"formant5"`
	Bandwidth5   *int64  `json:"bandwidth5"`
	Jitter       *int64  `json:"jitter"`
	Shimmer      *int64  `json:"shimmer"`
	Aspiration   *int64  `json:"aspiration"`
	SourceType   *int64  `json:"sourceType"`
	Amplitude    *int64  `json:"amplitude"`
	SourceGain   *int64  `json:"sourceGain"`
	OutputFilter *int64  `json:"outputFilter"`
}

type rateDescription struct {
	ResetBlock     []int64 `json:"resetBlock"`
	SourceStage    []int64 `json:"sourceStage"`
	NasalZeroB     *int64  `json:"nasalZeroB"`
	Radii          []int64 `json:"radii"`
	SquaredRadii   []int64 `json:"squaredRadii"`
	Cosines        []int64 `json:"cosines"`
	NasalZeroGains []int64 `json:"nasalZeroGains"`
}

type synthesizerDescription struct {
	HalfCosine       []int64                    `json:"halfCosine"`
	SourceShapes     []int64                    `json:"sourceShapes"`
	OpenPhases       []int64                    `json:"openPhases"`
	SourceAmplitudes []int64                    `json:"sourceAmplitudes"`
	PhaseSteps       []int64                    `json:"phaseSteps"`
	AmplitudeScales  []int64                    `json:"amplitudeScales"`
	JitterDepths     []int64                    `json:"jitterDepths"`
	ShimmerDepths    []int64                    `json:"shimmerDepths"`
	Deviations       []int64                    `json:"deviations"`
	Decibels         []int64                    `json:"decibels"`
	FormantLevels    []int64                    `json:"formantLevels"`
	Proximity        []int64                    `json:"proximity"`
	Voices           []voiceDescription         `json:"voices"`
	Rates            map[string]rateDescription `json:"rates"`
}

func (pack *Pack) LoadSynthesizer() (*Synthesizer, error) {
	return load(pack, "synthesizer.json", parseSynthesizer)
}

type tableCheck struct {
	name             string
	values           []int64
	copy             func([]int64, string, int64, int64) error
	minimum, maximum int64
}

func into[T int16 | int32](target []T) func([]int64, string, int64, int64) error {
	return func(values []int64, name string, minimum, maximum int64) error {
		return copyValues(target, values, name, minimum, maximum)
	}
}

func parseSynthesizer(data []byte) (*Synthesizer, error) {
	var description synthesizerDescription
	if err := json.Unmarshal(data, &description); err != nil {
		return nil, err
	}
	synthesizer := &Synthesizer{}
	checks := []tableCheck{
		{"halfCosine", description.HalfCosine, into(synthesizer.HalfCosine[:]), math.MinInt16, math.MaxInt16},
		{"sourceShapes", description.SourceShapes, into(synthesizer.SourceShapes[:]), math.MinInt16, math.MaxInt16},
		{"openPhases", description.OpenPhases, into(synthesizer.OpenPhases[:]), 0, math.MaxInt16},
		{"sourceAmplitudes", description.SourceAmplitudes, into(synthesizer.SourceAmplitudes[:]), math.MinInt16, math.MaxInt16},
		{"phaseSteps", description.PhaseSteps, into(synthesizer.PhaseSteps[:]), math.MinInt16, math.MaxInt16},
		{"amplitudeScales", description.AmplitudeScales, into(synthesizer.AmplitudeScales[:]), 0, math.MaxUint16},
		{"jitterDepths", description.JitterDepths, into(synthesizer.JitterDepths[:]), 0, math.MaxInt16},
		{"shimmerDepths", description.ShimmerDepths, into(synthesizer.ShimmerDepths[:]), math.MinInt16, math.MaxInt16},
		{"deviations", description.Deviations, into(synthesizer.Deviations[:]), math.MinInt16, math.MaxInt16},
		{"decibels", description.Decibels, into(synthesizer.Decibels[:]), math.MinInt16, math.MaxInt16},
		{"formantLevels", description.FormantLevels, into(synthesizer.FormantLevels[:]), math.MinInt16, math.MaxInt16},
		{"proximity", description.Proximity, into(synthesizer.Proximity[:]), math.MinInt16, math.MaxInt16},
	}
	for _, check := range checks {
		if err := check.copy(check.values, check.name, check.minimum, check.maximum); err != nil {
			return nil, err
		}
	}
	if err := checkPeriods(synthesizer); err != nil {
		return nil, err
	}
	if len(description.Voices) != VoiceCount {
		return nil, fmt.Errorf("voices has %d entries, expected %d", len(description.Voices), VoiceCount)
	}
	for index, voice := range description.Voices {
		if err := parseVoice(&synthesizer.Voices[index], voice, fmt.Sprintf("voices[%d]", index)); err != nil {
			return nil, err
		}
	}
	if len(description.Rates) != 2 {
		return nil, fmt.Errorf("rates has %d entries, expected %d and %d", len(description.Rates), StandardSampleRate, TelephoneSampleRate)
	}
	for sampleRate, rate := range map[int]*Rate{StandardSampleRate: &synthesizer.Standard, TelephoneSampleRate: &synthesizer.Telephone} {
		name := fmt.Sprint(sampleRate)
		rateDescription, found := description.Rates[name]
		if !found {
			return nil, fmt.Errorf("rates has no %s", name)
		}
		if err := parseRate(rate, rateDescription, "rates."+name); err != nil {
			return nil, err
		}
	}
	return synthesizer, nil
}

// checkPeriods makes sure jitter can't make a pitch period 0: the converter divides by it.
func checkPeriods(synthesizer *Synthesizer) error {
	var deepest, widest int64
	for _, depth := range synthesizer.JitterDepths {
		deepest = max(deepest, int64(depth))
	}
	for _, deviation := range synthesizer.Deviations {
		widest = max(widest, int64(deviation), -int64(deviation))
	}
	if 8*deepest*widest >= math.MaxInt16*math.MaxInt16 {
		return fmt.Errorf("jitterDepths and deviations allow a pitch period of 0")
	}
	return nil
}

func parseVoice(voice *Voice, description voiceDescription, name string) error {
	if err := copyValues(voice.Scales[:], description.Scales, name+".scales", -maximumVoicePercent, maximumVoicePercent); err != nil {
		return err
	}
	fields := []struct {
		name             string
		value            *int64
		target           *int32
		minimum, maximum int64
	}{
		{"bandwidth4", description.Bandwidth4, &voice.Bandwidth4, 0, maximumBandwidthHz},
		{"formant5", description.Formant5, &voice.Formant5, 0, math.MaxInt16},
		{"bandwidth5", description.Bandwidth5, &voice.Bandwidth5, 0, maximumBandwidthHz},
		{"jitter", description.Jitter, &voice.Jitter, 0, DepthCount - 1},
		{"shimmer", description.Shimmer, &voice.Shimmer, 0, DepthCount - 1},
		{"aspiration", description.Aspiration, &voice.Aspiration, 0, maximumAspiration},
		{"sourceType", description.SourceType, &voice.SourceType, 0, SourceTypeCount - 1},
		{"amplitude", description.Amplitude, &voice.Amplitude, 0, AmplitudeScaleCount - 1},
		{"sourceGain", description.SourceGain, &voice.SourceGain, math.MinInt16, math.MaxInt16},
		{"outputFilter", description.OutputFilter, &voice.OutputFilter, -maximumOutputPercent, maximumOutputPercent},
	}
	for _, field := range fields {
		if err := copyValue(field.target, field.value, name+"."+field.name, field.minimum, field.maximum); err != nil {
			return err
		}
	}
	return nil
}

func parseRate(rate *Rate, description rateDescription, name string) error {
	checks := []tableCheck{
		{"resetBlock", description.ResetBlock, into(rate.ResetBlock[:]), math.MinInt16, math.MaxInt16},
		{"sourceStage", description.SourceStage, into(rate.SourceStage[:]), math.MinInt16, math.MaxInt16},
		{"radii", description.Radii, into(rate.Radii[:]), math.MinInt16, math.MaxInt16},
		{"squaredRadii", description.SquaredRadii, into(rate.SquaredRadii[:]), math.MinInt16, math.MaxInt16},
		{"cosines", description.Cosines, into(rate.Cosines[:]), math.MinInt16, math.MaxInt16},
		{"nasalZeroGains", description.NasalZeroGains, into(rate.NasalZeroGains[:]), math.MinInt16, math.MaxInt16},
	}
	for _, check := range checks {
		if err := check.copy(check.values, name+"."+check.name, check.minimum, check.maximum); err != nil {
			return err
		}
	}
	return copyValue(&rate.NasalZeroB, description.NasalZeroB, name+".nasalZeroB", math.MinInt16, math.MaxInt16)
}

func copyValue(target *int32, value *int64, name string, minimum, maximum int64) error {
	if value == nil {
		return fmt.Errorf("%s is missing", name)
	}
	if *value < minimum || *value > maximum {
		return fmt.Errorf("%s is %d, outside %d to %d", name, *value, minimum, maximum)
	}
	*target = int32(*value)
	return nil
}

func copyValues[T byte | int16 | int32](target []T, values []int64, name string, minimum, maximum int64) error {
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
