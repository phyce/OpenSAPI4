package synthesizer

import "github.com/phyce/OpenSAPI4/truvoice/datapack"

// Frame is one 10 ms frame of the parameter generator's 22 tracks (synthesizer.md §4.1).
type Frame [22]byte

const (
	trackVoicing           = 0
	trackFrication         = 1
	trackAspiration        = 2
	trackParallel2         = 3
	trackParallel3         = 4
	trackParallel4         = 5
	trackParallel5         = 6
	trackBypass            = 8
	trackF1                = 9
	trackF2                = 10
	trackF3                = 11
	trackF4                = 12
	trackB1                = 13
	trackB2                = 14
	trackB3                = 15
	trackNasalZero         = 16
	trackPitch             = 17
	trackVoice             = 21
	minimumBandwidth       = 22
	maximumF2              = 255
	maximumAspiration      = 107
	f4Gap                  = 20
	maximumPhaseStep       = 139
	unitGain               = 0x2000
	maximumF1Gain          = 0x7ff
	maximumParallelLevel   = 160
	maximumParallel5Level  = 154
	jitterSeed             = 0x55
	silentFramesBeforeMute = 2
	voicingLevel           = 140
	aspirationLevel        = 105
	bypassLevel            = 37
)

// Converter turns frames into coefficient blocks. Its state carries over from frame to frame.
type Converter struct {
	tables       *datapack.Synthesizer
	rate         *datapack.Rate
	sampleRate   int32
	levels       []int32
	block        Block
	random       int32
	silentFrames int32
}

// NewConverter returns a converter for the sample rate, reset.
func NewConverter(tables *datapack.Synthesizer, sampleRate int) *Converter {
	levels := make([]int32, 0, len(tables.Decibels)+1+len(tables.FormantLevels)+len(tables.Proximity))
	levels = append(levels, tables.Decibels[:]...)
	levels = append(levels, 0)
	levels = append(levels, tables.FormantLevels[:]...)
	levels = append(levels, tables.Proximity[:]...)
	converter := &Converter{tables: tables, levels: levels}
	converter.Reset(sampleRate)
	return converter
}

// Reset sets the converter's block and state for the sample rate (synthesizer.md §4.2).
func (converter *Converter) Reset(sampleRate int) {
	converter.rate = converter.tables.ForSampleRate(sampleRate)
	converter.sampleRate = int32(int16(sampleRate))
	converter.block = converter.rate.ResetBlock
	converter.random = jitterSeed
	converter.silentFrames = 0
}

// Block returns the block the converter wrote last.
func (converter *Converter) Block() *Block {
	return &converter.block
}

// entry reads a table at an index computed from a frame, held to the table (synthesizer.md §4.4).
func entry(values []int32, index int32) int32 {
	return values[min(max(index, 0), int32(len(values)-1))]
}

func scale(value, percent int32) int32 {
	return value + percent*value/100
}

func multiply(first, second int32) int32 {
	return first * second / 0x7fff
}

func distance(value int32) int32 {
	if value <= 2 {
		return 0
	}
	return value
}

func (converter *Converter) nextDeviation() int32 {
	register := converter.random
	register = register>>1 + (register&1<<1^register&2)<<6
	converter.random = register
	return converter.tables.Deviations[register&0x3f]
}

// resonator returns a resonator's B, −C and gain for a frequency index and a bandwidth index.
func (converter *Converter) resonator(frequency, bandwidth int32) (int16, int16, int32) {
	squared := entry(converter.rate.SquaredRadii[:], bandwidth)
	product := entry(converter.rate.Radii[:], bandwidth) * entry(converter.rate.Cosines[:], frequency)
	return int16(product >> 12), int16(squared * 4), squared - product>>13 + unitGain
}

// Convert writes the frame's coefficients into the block and returns it (synthesizer.md §4.3). It raises the
// frame's bandwidths to their minimum. A tone frame has its formants in 10 Hz units; mute says the volume is off.
func (converter *Converter) Convert(frame *Frame, tone, mute bool) *Block {
	for track := trackB1; track <= trackB3; track++ {
		frame[track] = max(frame[track], minimumBandwidth)
	}
	var tracks [len(Frame{})]int32
	for index, value := range frame {
		tracks[index] = int32(value)
	}
	tables, rate, block := converter.tables, converter.rate, &converter.block
	voice := &tables.Voices[tracks[trackVoice]>>4]
	f1 := scale(tracks[trackF1], voice.Scales[0])
	b1 := scale(tracks[trackB1], voice.Scales[1])
	f2 := min(scale(tracks[trackF2], voice.Scales[2]), maximumF2)
	b2 := scale(tracks[trackB2], voice.Scales[3])
	f3 := scale(tracks[trackF3], voice.Scales[4])
	b3 := scale(tracks[trackB3], voice.Scales[5])
	f4 := max(scale(tracks[trackF4], voice.Scales[6]), f3+f4Gap)
	f5 := max(voice.Formant5, (f4+25)*16)
	voicing, frication := tracks[trackVoicing], tracks[trackFrication]
	aspiration := min(tracks[trackAspiration]+voice.Aspiration, maximumAspiration)

	if voicing == 0 && frication == 0 && aspiration == 0 {
		converter.silentFrames++
	} else {
		converter.silentFrames = 0
	}
	pitch := int32(0)
	if voicing != 0 {
		pitch = tracks[trackPitch] * 2
	}
	if pitch == 0 {
		block[blockSource] = 0
		block[blockPeriods], block[blockPeriods+1] = 0, 0
		block[blockAmplitudes], block[blockAmplitudes+1] = 0, 0
	} else {
		switch pitch {
		case 69, 74, 79, 84, 90:
			pitch++
		}
		depth := multiply(pitch, tables.JitterDepths[voice.Jitter])
		first := converter.nextDeviation()
		second := converter.nextDeviation()
		for index, deviation := range [2]int32{first, second} {
			frequency := int32(int16(pitch + multiply(depth, deviation)*8))
			block[blockPeriods+index] = int16(2 * converter.sampleRate / frequency / 2)
		}
		block[blockSource] = int16(voice.SourceType)
		period := int32(block[blockPeriods])
		step := multiply(multiply(period, voice.Amplitude<<11), tables.OpenPhases[voice.SourceType])
		block[blockPhaseIncrement] = int16(entry(tables.PhaseSteps[:], min(step, maximumPhaseStep)))
		amplitude := int16(entry(converter.levels, voicingLevel+voicing))
		amplitude = int16(multiply(int32(amplitude), tables.SourceAmplitudes[voice.SourceType]))
		amplitude = int16(multiply(int32(amplitude), tables.AmplitudeScales[voice.Amplitude]) >> 3)
		amplitude *= block[blockPeriods] >> 2
		shimmer := tables.ShimmerDepths[voice.Shimmer]
		block[blockAmplitudes] = amplitude + int16(multiply(multiply(int32(amplitude), shimmer), second)<<3)
		block[blockAmplitudes+1] = amplitude + int16(multiply(multiply(int32(amplitude), shimmer), converter.nextDeviation())<<3)
	}
	if mute {
		block[blockSource] |= muteBit
	}
	if converter.silentFrames > silentFramesBeforeMute {
		block[blockSource] |= muteBit
		converter.silentFrames = silentFramesBeforeMute
	}

	block[blockAspirationGain] = int16(entry(converter.levels, aspirationLevel+aspiration))
	block[blockSourceStageGain], block[blockSourceStageB], block[blockSourceStageC] = rate.SourceStage[0], rate.SourceStage[1], rate.SourceStage[2]
	half := converter.sampleRate / 2
	if f5 > half {
		block[blockF5Gain], block[blockF5B], block[blockF5C] = unitGain, 0, 0
	} else {
		b, c, gain := converter.resonator(f5>>3, voice.Bandwidth5>>2)
		block[blockF5B], block[blockF5C], block[blockF5Gain] = b, c, int16(gain)
	}
	if f4*16 >= half {
		f4 = (half - 10) >> 4
	}
	f1Index, f2Index, f3Index := f1>>1, f2+63, 2*f3
	if tone {
		f1Index, f2Index, f3Index = 10*f1>>3, 10*f2>>3, 10*f3>>3
	}
	b, c, f4Gain := converter.resonator(2*f4, voice.Bandwidth4>>2)
	block[blockF4B], block[blockF4C], block[blockF4Gain] = b, c, int16(f4Gain)
	b, c, f3Gain := converter.resonator(f3Index, b3>>1)
	block[blockF3B], block[blockF3C], block[blockF3Gain] = b, c, int16(f3Gain)
	b, c, f2Gain := converter.resonator(f2Index, b2>>1)
	block[blockF2B], block[blockF2C], block[blockF2Gain] = b, c, int16(f2Gain)
	b, c, f1Gain := converter.resonator(f1Index, b1>>1)
	block[blockF1B], block[blockF1C], block[blockCascadeGain] = b, c, int16(min(f1Gain, maximumF1Gain)<<4)
	block[blockSourceGain] = int16(voice.SourceGain)
	nasalZero := tracks[trackNasalZero]
	block[blockNasalZeroB] = int16(rate.NasalZeroB * entry(rate.Cosines[:], (4*nasalZero+192)>>3) >> 12)
	block[blockNasalZeroGain] = int16(entry(rate.NasalZeroGains[:], nasalZero>>2)) << 1

	if frication < 1 {
		block[blockParallel5Gain], block[blockParallel4Gain], block[blockParallel3Gain] = 0, 0, 0
		block[blockParallel2Gain], block[blockBypassGain] = 0, 0
		return block
	}
	level1, level2, level3 := f1>>3, f2>>2+16, f3>>1
	if tone {
		level1, level2, level3 = 10*f1>>5, 10*f2>>5, 10*f3>>5
	}
	formantLevel1, formantLevel2 := entry(tables.FormantLevels[:], level1), entry(tables.FormantLevels[:], level2)
	common := 2*formantLevel1 + 2*formantLevel2 - 235
	near12 := entry(tables.Proximity[:], distance(level2-level1))
	near23 := entry(tables.Proximity[:], distance(level3-level2-2))
	near34 := entry(tables.Proximity[:], distance(f4>>1-level3-5))
	level := entry(converter.levels, max(2*formantLevel1-formantLevel2-14+2*near12+near23+tracks[trackParallel2]+frication, 0))
	block[blockParallel2Gain] = int16(min(level, maximumParallelLevel) * f2Gain >> 11)
	level = converter.levels[min(max(common+2*near23+near34+tracks[trackParallel3]+frication+22, 0), maximumParallelLevel)]
	block[blockParallel3Gain] = -int16(level * f3Gain >> 11)
	level = converter.levels[min(max(common+2*near34+tracks[trackParallel4]+frication+17, 0), maximumParallelLevel)]
	block[blockParallel4Gain] = int16(level * f4Gain >> 11)
	if converter.sampleRate == telephoneRate {
		block[blockParallel4Gain] >>= 1
	}
	level = converter.levels[min(max(common+tracks[trackParallel5]+frication+16, 0), maximumParallel5Level)]
	block[blockParallel5Gain] = -int16(level * int32(block[blockF5Gain]) >> 11)
	block[blockBypassGain] = int16(entry(converter.levels, bypassLevel+tracks[trackBypass]+frication)) << 2
	return block
}
