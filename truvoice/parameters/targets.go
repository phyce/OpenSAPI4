package parameters

import (
	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
)

// The class bits the generator itself tests (parameters.md §3.2): of class byte 0, 2 and 3.
const (
	sonorantClass  = 0x02
	voicedClass    = 0x04
	nasalClass     = 0x10
	stopClass      = 0x20
	fricativeClass = 0x40
	closureClass   = 0x01
	vowelClass     = 0x02
	labialClass    = 0x10
	silenceClass   = 0x02
)

// The phonemes' categories (parameters.md §3.3).
const (
	VowelCategory = iota
	SonorantCategory
	FricativeCategory
	ClosureCategory
)

// The places that matter to the generator itself (parameters.md §3.1).
const (
	fricativePlace = 6
	closurePlace   = 8
	pausePlace     = 9
)

// The first consonant and the first stop in the phonemes' order, and the frontness a reset starts from.
const (
	firstConsonant  = 30
	firstStop       = 34
	resetFrontness  = -13120
	stressedFlag    = 0x20
	glottalFlag     = 0x40
	releaseOffset   = 8
	noVoiceOnset    = 0x7f
	defaultMode     = PullBack | Shaped
	aspirationLevel = 0x33
	burstLevel      = 0x7f
)

// The tracks the generator handles by name.
const (
	voicingTrack     = 0
	fricationTrack   = 1
	aspirationTrack  = 2
	bypassTrack      = 8
	firstFormant     = 9
	fourthFormant    = 12
	firstBandwidth   = 13
	nasalZeroTrack   = 16
	pitchTrack       = 17
	firstVoiceTrack  = 18
	attenuationTrack = 20
	voiceTrack       = 21
)

func (generator *Generator) cursor() *pipeline.Cursor {
	return &generator.Pipeline.Cursors[pipeline.Parameters]
}

func symbolOf(node *pipeline.Node) byte {
	if node == nil {
		return 0
	}
	return node.Symbol()
}

// has reports whether a symbol has the bits of a class byte. Symbols of 0x80 and up have none.
func (generator *Generator) has(symbol byte, plane int, bits byte) bool {
	return symbol < datapack.SymbolCount && generator.tables.Classes[plane][symbol]&bits != 0
}

// phoneme returns the phoneme a symbol is, or the pause for a symbol that isn't one.
func (generator *Generator) phoneme(symbol byte) (int, *datapack.Phoneme) {
	index, found := generator.tables.Index(symbol)
	if !found {
		index = datapack.PauseIndex
	}
	return index, &generator.tables.Phonemes[index]
}

func (generator *Generator) category(symbol byte) int32 {
	switch {
	case generator.has(symbol, 2, vowelClass):
		return VowelCategory
	case generator.has(symbol, 2, closureClass):
		return ClosureCategory
	case generator.has(symbol, 0, sonorantClass):
		return SonorantCategory
	}
	return FricativeCategory
}

// voice returns the voice whose tracks the generator takes, the cursor's, or voice 0 for one out of range.
func (generator *Generator) voice() *datapack.VoiceTracks {
	voice := generator.cursor().Voice
	if voice < 0 || voice >= datapack.VoiceCount {
		voice = 0
	}
	return &generator.tables.Voices[voice]
}

// LoadTargets fills the segments with the phoneme's targets and default modes, shapes and frames, and notes
// what the rules and actions need of it and of its neighbours (parameters.md §4.1).
func (generator *Generator) LoadTargets() {
	cursor := generator.cursor()
	current, previous, next := cursor.Scan, cursor.Done, cursor.First
	symbol, previousSymbol, nextSymbol := current.Symbol(), previous.Symbol(), next.Symbol()
	_, phoneme := generator.phoneme(symbol)
	if _, previousPhoneme := generator.phoneme(previousSymbol); previousPhoneme.Vowel != nil && previousSymbol != 'u' && symbol != 'j' {
		generator.PreviousFrontness = previousPhoneme.Vowel.EndFrontness
	} else {
		generator.PreviousFrontness = generator.Frontness
	}
	generator.PreviousCategory = generator.Category
	generator.Frontness = phoneme.Frontness
	generator.FricativeAspiration = aspirationLevel
	generator.Aspiration = aspirationLevel
	generator.BurstLevels = [3]int32{}
	generator.Category = generator.category(symbol)
	segments := &generator.Segments
	for formant := range phoneme.Formants {
		segments[firstFormant+formant].Target = phoneme.Formants[formant]
	}
	for bandwidth := range phoneme.Bandwidths {
		segments[firstBandwidth+bandwidth].Target = phoneme.Bandwidths[bandwidth]
	}
	segments[voicingTrack].Target = phoneme.Voicing
	segments[pitchTrack].Target = int32(current.Second()) << 1
	for track := range generator.voice().Tracks {
		segments[firstVoiceTrack+track].Target = generator.voice().Tracks[track]
	}
	segments[voiceTrack].Target = 0
	for track := range phoneme.Amplitudes {
		segments[fricationTrack+track].Target = phoneme.Amplitudes[track]
	}
	if phoneme.Vowel != nil {
		generator.VowelTransition = phoneme.Vowel.Transition
		generator.Offglide = phoneme.Vowel.Offglide
	}
	if symbol == 'u' && nextSymbol == 'j' {
		generator.Frontness = 0
	}
	switch {
	case phoneme.Burst != nil:
		generator.loadBurst(phoneme, current, previous, next)
	case symbol == 't' || symbol == 'J' || symbol == 'C':
		generator.loadAffricate(symbol, nextSymbol)
	}
	segments[nasalZeroTrack].Target = phoneme.NasalZero
	weight := generator.tables.CategoryWeights[generator.PreviousCategory][generator.Category]
	for track := range segments {
		segments[track].Forward = generator.tables.DefaultShapes[track]
		segments[track].Mode = defaultMode
		segments[track].Frames = int32(current.Value())
		generator.Weights[track] = weight
	}
	segments[attenuationTrack].Mode = Flat
	segments[voiceTrack].Mode = Flat
	nextIndex, nextPhoneme := generator.phoneme(nextSymbol)
	for formant := range 3 {
		generator.NextTargets[firstFormant+formant] = nextPhoneme.Formants[formant]
	}
	if nextIndex >= firstStop {
		for bandwidth := range nextPhoneme.Bandwidths {
			generator.NextTargets[firstBandwidth+bandwidth] = nextPhoneme.Bandwidths[bandwidth]
		}
	}
}

// loadAffricate notes the burst of t, J and C before a phoneme that isn't a closure or a pause.
func (generator *Generator) loadAffricate(symbol, nextSymbol byte) {
	_, nextPhoneme := generator.phoneme(nextSymbol)
	if nextPhoneme.Place == closurePlace || nextPhoneme.Place == pausePlace {
		return
	}
	beforeFricative := nextPhoneme.Place == fricativePlace
	generator.BurstLevels = [3]int32{burstLevel, burstLevel, 0x33}
	if symbol == 'J' {
		generator.BurstLevels[2] = 0
	}
	if beforeFricative && symbol == 'C' {
		generator.BurstLevels[2] = 0
	}
	if beforeFricative && symbol != 't' {
		generator.BurstLevels[1] = 0x46
		if symbol == 'J' {
			generator.BurstLevels[1] = 0
		}
	}
	if beforeFricative && symbol == 'J' {
		generator.BurstLevels[0] = 0
	}
	generator.VoiceOnset = noVoiceOnset
	generator.BurstFrames = 1
	if symbol == 't' {
		generator.BurstFrames = noVoiceOnset
	}
}

// loadBurst notes the burst of B, P, D, T, G and K: its levels, which a closure or pause after it can cut, its
// voice onset and how many frames it lasts.
func (generator *Generator) loadBurst(phoneme *datapack.Phoneme, current, previous, next *pipeline.Node) {
	symbol, previousSymbol, nextSymbol := current.Symbol(), previous.Symbol(), next.Symbol()
	row := phoneme.Burst[1]
	if generator.has(nextSymbol, 2, vowelClass) {
		row = phoneme.Burst[0]
	}
	copy(generator.BurstLevels[:], row[:3])
	generator.Segments[bypassTrack].Target = row[3]
	if _, nextPhoneme := generator.phoneme(nextSymbol); nextPhoneme.Place == closurePlace || nextPhoneme.Place == pausePlace {
		if symbol != 'K' || !generator.has(nextSymbol, 0, nasalClass) && nextSymbol != 'Q' && nextSymbol != 't' {
			generator.BurstLevels = [3]int32{burstLevel, burstLevel, burstLevel}
			generator.Segments[bypassTrack].Target = 0
		}
	}
	generator.VoiceOnset = noVoiceOnset
	if !generator.has(symbol, 0, voicedClass) {
		generator.VoiceOnset = int32(current.Data[releaseOffset])
	}
	generator.BurstFrames = 0
	nextIsOralClosure := generator.has(nextSymbol, 2, closureClass) && !generator.has(nextSymbol, 0, nasalClass)
	previousIsOralClosure := generator.has(previousSymbol, 2, closureClass) && !generator.has(previousSymbol, 0, nasalClass)
	if nextIsOralClosure && !previousIsOralClosure {
		generator.BurstFrames = noVoiceOnset
		switch symbol {
		case 'D':
			generator.BurstFrames = 0
			if previousSymbol == 'j' && (nextSymbol == 'B' || nextSymbol == 'G') {
				generator.BurstFrames = 1
			}
		case 'K', 'P':
			if symbol == 'K' && nextSymbol == 'P' || symbol == 'P' && !generator.has(nextSymbol, 0, voicedClass) {
				generator.BurstLevels[1] = 0x3c
				generator.BurstLevels[2] = 0x30
				generator.BurstFrames = 1
			}
		}
		return
	}
	nextIsVowel := generator.has(nextSymbol, 2, vowelClass)
	switch {
	case symbol == 'K' && generator.has(nextSymbol, 0, fricativeClass):
		generator.BurstFrames = 1
	case symbol == 'G':
		generator.BurstFrames = 2
		if next.Flags()&stressedFlag == 0 && nextIsVowel {
			generator.BurstFrames = 1
		}
	case symbol == 'T' && generator.has(nextSymbol, 0, fricativeClass):
		generator.BurstFrames = noVoiceOnset
	case symbol == 'D':
		generator.BurstFrames = noVoiceOnset
		if !generator.has(nextSymbol, 0, sonorantClass) || nextIsVowel {
			generator.BurstFrames = 1
			if previousSymbol == ' ' {
				generator.BurstFrames = 2
			}
		}
	case !generator.has(symbol, 2, labialClass) && nextSymbol != 'p':
		generator.BurstFrames = 2
	default:
		generator.BurstFrames = 1
	}
}

// InitModes adjusts the default modes after the phoneme before: the pitch track's pull back always goes, the
// voicing's after an unvoiced phoneme, and the earlier frames of some tracks are kept (parameters.md §4.2).
func (generator *Generator) InitModes() {
	previousSymbol := generator.cursor().Done.Symbol()
	generator.Weights[pitchTrack] = 0
	generator.Segments[pitchTrack].Mode = Shaped
	if !generator.has(previousSymbol, 0, voicedClass) {
		generator.Segments[voicingTrack].Mode = Shaped
		generator.Tracks[pitchTrack].Earliest = generator.Tracks[pitchTrack].Next
	}
	if generator.has(previousSymbol, 3, silenceClass) {
		for formant := range 3 {
			generator.Weights[firstFormant+formant] = 0x7ffe
		}
	}
	if generator.has(previousSymbol, 0, stopClass) {
		generator.Tracks[fricationTrack].Earliest = generator.Tracks[fricationTrack].Next
	}
}

// Reset is the stage's reset: its own state, the ring's lookahead and margin, and the context (parameters.md
// §7).
func (generator *Generator) Reset() {
	generator.Coarticulation = 0
	generator.DatabaseLoci = false
	generator.PauseState = 0
	generator.EndReached = false
	generator.KeepEarlier = false
	generator.UnvoicedPitch = 0
	generator.TransitionLimit = 4
	generator.Margin = 3
	generator.Synthesizer.Lookahead = 2
	generator.GeneratedDone = true
	generator.EndState = 1
	generator.ResetContext()
}

// ResetContext forgets the phonemes before: every track starts at the ring's written frames, from the initial
// frame's values (parameters.md §7).
func (generator *Generator) ResetContext() {
	generator.Frontness = resetFrontness
	generator.PreviousFrontness = resetFrontness
	generator.AspirationShift = 0
	generator.PreviousCategory = VowelCategory
	generator.Category = VowelCategory
	generator.VoicingShift = 0
	generator.BurstFrames = 0
	generator.VowelOffset = 0
	generator.VowelTransition = 0
	generator.ClosureFrames = 0
	generator.VoiceOnset = 0
	generator.BurstLevels = [3]int32{}
	generator.Aspiration = 0
	generator.FricativeAspiration = 0
	written := generator.Synthesizer.Written
	initial := generator.Pipeline.InitialFrame()
	for track := range generator.Tracks {
		generator.Tracks[track] = Track{Next: written, Earliest: written - generator.Synthesizer.Lookahead}
		generator.Last[track] = Units(track, initial[track])
	}
	generator.BeforePrevious = ' '
	generator.Furthest = written
}
