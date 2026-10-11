// Package synthesizer is TruVoice's formant synthesizer (docs/truvoice/synthesizer.md).
package synthesizer

import "github.com/phyce/OpenSAPI4/truvoice/datapack"

// Block is a frame's coefficient block, the converter's output and the renderer's input
// (synthesizer.md §2).
type Block [40]int16

const (
	blockSource           = 0
	blockPhaseIncrement   = 1
	blockF5B              = 4
	blockF5C              = 5
	blockF4B              = 6
	blockF4C              = 7
	blockF3B              = 8
	blockF3C              = 9
	blockF2B              = 10
	blockF2C              = 11
	blockNasalPoleB       = 12
	blockNasalPoleC       = 13
	blockF1B              = 14
	blockF1C              = 15
	blockAspirationGain   = 16
	blockSourceGain       = 17
	blockFirstGain        = 19
	blockParallel5Gain    = 20
	blockF5Gain           = 21
	blockParallel4Gain    = 22
	blockF4Gain           = 23
	blockParallel3Gain    = 24
	blockF3Gain           = 25
	blockParallel2Gain    = 26
	blockF2Gain           = 27
	blockBypassGain       = 28
	blockNasalZeroGain    = 29
	blockPeriods          = 30
	blockNasalZeroB       = 32
	blockNasalZeroC       = 33
	blockAmplitudes       = 34
	blockCascadeGain      = 36
	blockSourceStageGain  = 37
	blockSourceStageB     = 38
	blockSourceStageC     = 39
	sourceTypeMask        = 0x1f
	muteBit               = 0x80
	secondFamilyFirstType = 15
	secondFamily          = 0x4000
	noiseSeed             = -0x5556
	telephoneRate         = 8000
)

type resonator struct {
	previous       int16
	beforePrevious int16
}

// latched holds the coefficients the renderer takes from the block at the start of each pitch period.
type latched struct {
	f1B, f1C, cascadeGain              int16
	f2B, f2C, f2Gain, f3B, f3C, f3Gain int16
	nasalZeroB, nasalZeroGain          int16
}

// Renderer turns coefficient blocks into samples. Its state carries over from block to block.
type Renderer struct {
	tables          *datapack.Synthesizer
	telephone       bool
	samplesPerFrame int16

	countdown        int16
	halfCountdown    int16
	pulseSegment     int16
	previousMix      int16
	phaseIncrement   int16
	pulseFamily      int16
	periodState      int16
	previousParallel int16
	noise            int16
	previousNoise    int16
	previousPulse    int16
	derivative       int16
	latched          latched
	cascade5         resonator
	cascade4         resonator
	cascade3         resonator
	cascade2         resonator
	nasalPole        resonator
	cascade1         resonator
	sourceStage      resonator
	parallel5        resonator
	parallel4        resonator
	parallel3        resonator
	parallel2        resonator
	pulseShape       int32
	phase            int32
	pulseAmplitude   int32
	previousOutput   int32
}

// NewRenderer returns a renderer for the sample rate, reset.
func NewRenderer(tables *datapack.Synthesizer, sampleRate int) *Renderer {
	renderer := &Renderer{tables: tables}
	renderer.Reset(sampleRate)
	return renderer
}

// Reset clears the renderer's state for the sample rate (synthesizer.md §3.1).
func (renderer *Renderer) Reset(sampleRate int) {
	*renderer = Renderer{tables: renderer.tables, telephone: int16(sampleRate) == telephoneRate,
		samplesPerFrame: int16(sampleRate) / 100, noise: noiseSeed}
}

func saturate(value int32) int16 {
	if value >= 0x7fff {
		return 0x7fff
	}
	if value <= -0x7fff {
		return -0x8000
	}
	return int16(value)
}

func (stage *resonator) cascade(input, a, b, c int32) int16 {
	value := (c*int32(stage.beforePrevious) + (a*input+b*int32(stage.previous))*2) >> 15
	stage.beforePrevious = stage.previous
	stage.previous = saturate(value)
	return stage.previous
}

func (stage *resonator) parallel(input, a, b, c int32) int32 {
	value := (a*input + b*int32(stage.previous)*2 + c*int32(stage.beforePrevious)) >> 15
	stage.beforePrevious = stage.previous
	stage.previous = int16(value)
	return value
}

// Render appends a frame's samples, rendered from the block with the output-filter voice and gain, to samples.
func (renderer *Renderer) Render(block *Block, outputVoice int, gain uint16, samples []int16) []int16 {
	periods := [2]int16{block[blockPeriods], block[blockPeriods+1]}
	amplitudes := [2]int16{block[blockAmplitudes], block[blockAmplitudes+1]}
	sourceType := block[blockSource] & sourceTypeMask
	family := int16(0)
	if sourceType >= secondFamilyFirstType {
		family = secondFamily
	}
	shape := int32(renderer.tables.SourceShapes[sourceType])
	increment := int16(int32(block[blockPhaseIncrement]) * 0x517d >> 15)
	f5B, f5C := int32(block[blockF5B]), int32(-block[blockF5C])
	f4B, f4C := int32(block[blockF4B]), int32(-block[blockF4C])
	nasalPoleB, nasalPoleC := int32(block[blockNasalPoleB]), int32(-block[blockNasalPoleC])
	sourceStageC := int32(-block[blockSourceStageC])
	filter := renderer.tables.OutputFilters[outputVoice]
	for count := renderer.samplesPerFrame; ; {
		if periods[0] == 0 && renderer.countdown < 0 {
			renderer.periodState, renderer.halfCountdown, renderer.countdown = 0, 0, 0
		} else if renderer.countdown--; renderer.countdown < 0 {
			renderer.latched = latched{
				f1B: block[blockF1B], f1C: -block[blockF1C], cascadeGain: block[blockCascadeGain],
				f2B: block[blockF2B], f2C: -block[blockF2C], f2Gain: block[blockF2Gain],
				f3B: block[blockF3B], f3C: -block[blockF3C], f3Gain: block[blockF3Gain],
				nasalZeroB: block[blockNasalZeroB], nasalZeroGain: block[blockNasalZeroGain],
			}
			renderer.countdown = periods[0]
			renderer.halfCountdown = periods[0] >> 1
			periods[0], periods[1] = periods[1], periods[0]
			renderer.periodState = -1
		} else {
			renderer.halfCountdown--
			renderer.periodState = 1
		}

		var pulse int16
		switch {
		case renderer.periodState < 0:
			renderer.pulseFamily = family
			renderer.pulseAmplitude = int32(amplitudes[0])
			amplitudes[0], amplitudes[1] = amplitudes[1], amplitudes[0]
			renderer.pulseShape = shape
			renderer.phaseIncrement = increment
			renderer.pulseSegment = 0
			renderer.phase = 0
		case renderer.periodState == 0 || renderer.pulseSegment&1 != 0:
		default:
			pulse = int16(renderer.nextPulse())
		}

		difference := int32(pulse) - int32(renderer.previousPulse)
		renderer.previousPulse = pulse
		renderer.derivative = saturate(difference * 8 * int32(block[blockSourceGain]) >> 15)

		noiseBits := int32(renderer.previousNoise) ^ int32(renderer.noise)
		renderer.previousNoise = renderer.noise
		if next := int16(noiseBits&0x3ff<<6 | noiseBits>>7&0x1f8); next == 0 {
			renderer.noise = noiseSeed
		} else {
			renderer.noise = next
		}
		noise := renderer.noise

		input := int16(int32(block[blockAspirationGain])*int32(noise)>>15) + renderer.derivative
		cascade := renderer.sourceStage.cascade(int32(input), int32(block[blockSourceStageGain]), int32(block[blockSourceStageB]), sourceStageC)
		if renderer.telephone {
			cascade = renderer.cascade4.cascade(int32(cascade), int32(block[blockFirstGain]), f4B, f4C)
		} else {
			cascade = renderer.cascade5.cascade(int32(cascade), int32(block[blockFirstGain]), f5B, f5C)
			cascade = renderer.cascade4.cascade(int32(cascade), int32(block[blockF5Gain]), f4B, f4C)
		}
		cascade = renderer.cascade3.cascade(int32(cascade), int32(block[blockF4Gain]), int32(renderer.latched.f3B), int32(renderer.latched.f3C))
		cascade = renderer.cascade2.cascade(int32(cascade), int32(renderer.latched.f3Gain), int32(renderer.latched.f2B), int32(renderer.latched.f2C))
		poleTwoBack := int32(renderer.nasalPole.beforePrevious)
		pole := renderer.nasalPole.cascade(int32(cascade), int32(renderer.latched.f2Gain), nasalPoleB, nasalPoleC)
		zero := int16(int32(renderer.nasalPole.beforePrevious)*int32(renderer.latched.nasalZeroB)>>14&^1 +
			int32(pole) + int32(block[blockNasalZeroC])*poleTwoBack>>15)
		cascade = renderer.cascade1.cascade(int32(zero), int32(renderer.latched.nasalZeroGain), int32(renderer.latched.f1B), int32(renderer.latched.f1C))
		cascadeOutput := saturate(int32(cascade) * int32(renderer.latched.cascadeGain) >> 14 &^ 1)

		if renderer.countdown != 0 && renderer.halfCountdown < 0 {
			noise >>= 1
		}
		var parallel int16
		if !renderer.telephone {
			parallel = saturate(renderer.parallel5.parallel(int32(noise), int32(block[blockParallel5Gain]), f5B, f5C))
		}
		parallel = saturate(int32(parallel) + renderer.parallel4.parallel(int32(noise), int32(block[blockParallel4Gain]), f4B, f4C))
		parallel = saturate(int32(parallel) + renderer.parallel3.parallel(int32(noise), int32(block[blockParallel3Gain]), int32(renderer.latched.f3B), int32(renderer.latched.f3C)))
		parallel = saturate(int32(parallel) + renderer.parallel2.parallel(int32(noise), int32(block[blockParallel2Gain]), int32(renderer.latched.f2B), int32(renderer.latched.f2C)))
		parallel = int16(int32(int16(int32(block[blockBypassGain])*int32(noise)>>15+int32(parallel))) * 0x184d >> 15)
		mix := parallel - renderer.previousParallel + int16(int32(cascadeOutput)-int32(renderer.previousMix)*0x1d71>>15)
		renderer.previousParallel = parallel
		renderer.previousMix = mix
		if block[blockSource]&muteBit != 0 {
			mix = 0
		}
		samples = append(samples, saturate((int32(mix)-filter*renderer.previousOutput/100)*int32(gain)<<3))
		renderer.previousOutput = int32(mix)

		if count--; count <= 0 {
			return samples
		}
	}
}

// nextPulse advances the glottal pulse's phase and returns its value (synthesizer.md §3.3).
func (renderer *Renderer) nextPulse() int32 {
	renderer.phase += int32(renderer.phaseIncrement)
	var cosine, slope int32
	if renderer.phase > 0xffff {
		renderer.phase &= 0xffff
		if renderer.pulseSegment&2 != 0 {
			renderer.pulseSegment = 3
			return 0
		}
		renderer.pulseSegment = 2
		cosine, slope = renderer.interpolate(renderer.phase)
	} else {
		cosine, slope = renderer.interpolate(renderer.phase)
		if renderer.pulseSegment&2 == 0 {
			half := renderer.pulseAmplitude >> 1
			return half - (cosine-slope)*half>>15
		}
	}
	product := (cosine - slope) * renderer.pulseShape
	value := (product - renderer.pulseShape<<15) >> 15
	if renderer.pulseFamily&secondFamily != 0 {
		value = (value<<15+product&0x7fff)*2 + 0x7fffff
		if value < 0 {
			renderer.pulseSegment = 3
			return 0
		}
		return value >> 8 & 0xffff * renderer.pulseAmplitude >> 15
	}
	value = (value + 0x7fff) << 16
	if value < 0 {
		renderer.pulseSegment = 3
		return 0
	}
	return renderer.pulseAmplitude * (value >> 16) >> 15
}

// interpolate returns the half-cosine at the phase's high byte and the correction for its low byte.
func (renderer *Renderer) interpolate(phase int32) (cosine, slope int32) {
	index := phase >> 8 & 0xff
	distance := 0x80 - index
	if distance < 0 {
		distance = -distance
	}
	cosine = int32(renderer.tables.HalfCosine[index])
	slope = int32(int16(int32(renderer.tables.HalfCosine[distance]) * (phase & 0xff) >> 15))
	return cosine, slope
}
