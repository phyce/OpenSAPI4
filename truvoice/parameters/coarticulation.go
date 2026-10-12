package parameters

// The classes only action 4's own steps test (parameters.md §12.1): of class byte 3 and 4.
const (
	highFrontClass = 0x08
	offglideClass  = 0x10
	neutralClass   = 0x40
)

// A vowel's targets after N, F1 to F4 and B1 to B3 in Hz (parameters.md §12.5): those of the vowels that have
// their own, and those of the others.
var (
	targetsAfterN = map[byte][7]int32{
		'4': {264, 1841, 2462, 3394, 68, 408, 188},
		'g': {296, 1606, 2534, 3468, 75, 408, 281},
		'k': {324, 1646, 2471, 3436, 128, 74, 55},
		'r': {354, 1059, 2709, 3289, 141, 196, 94},
		'u': {383, 1489, 2412, 3042, 149, 79, 60},
		'w': {387, 1461, 2554, 3521, 178, 156, 80},
	}
	otherTargetsAfterN = [7]int32{340, 1459, 2517, 3353, 125, 185, 135}
)

// VowelCoarticulation is action 4: the vowel database's record for the current vowel, or for a vowel it has
// none for, and for p, the vowel's own transitions (parameters.md §11 and §12).
func (generator *Generator) VowelCoarticulation() {
	if generator.fromDatabase() {
		return
	}
	cursor := generator.cursor()
	current := cursor.Scan
	symbol, previousSymbol, nextSymbol := current.Symbol(), cursor.Done.Symbol(), cursor.First.Symbol()
	segments := &generator.Segments
	switch {
	case symbol == 'u' && nextSymbol == 'j':
		segments[secondFormant].Target -= 150
	case generator.has(nextSymbol, 2, lateralClass):
		if generator.has(symbol, 2, frontClass) {
			generator.Offglide[1] -= 300
			segments[secondFormant].Target -= 300
		}
	case generator.has(previousSymbol, 2, rhoticClass):
		segments[thirdFormant].Target = (segments[thirdFormant].Target + 600) * 3 / 4
		generator.Offglide[2] = (generator.Offglide[2] + 600) * 3 / 4
		if generator.has(symbol, 2, frontClass) {
			segments[secondFormant].Target = (segments[secondFormant].Target*3 + 1400) / 4
			generator.Offglide[1] = (generator.Offglide[1]*3 + 1400) / 4
		}
	}
	if generator.has(nextSymbol, 0, stopClass) && generator.has(nextSymbol, 2, alveolarClass) && symbol == 'U' {
		generator.Offglide[1] += 400
	}
	if generator.has(previousSymbol, 3, highFrontClass) && generator.has(symbol, 3, roundedClass) {
		segments[secondFormant].Forward += 5
	}
	if !generator.has(symbol, 2, rhoticClass) && !generator.has(symbol, 4, neutralClass) {
		weight := generator.tables.ReductionWeights[min(int32(current.Value())*10, 255)/16]
		for formant, neutral := range generator.tables.NeutralFormants {
			target := &segments[firstFormant+formant].Target
			*target += multiply(neutral-*target, weight)
			generator.Offglide[formant] += multiply(neutral-generator.Offglide[formant], weight)
		}
	}
	switch {
	case generator.has(symbol, 1, reducedClass):
		generator.coarticulateReduced()
	case !generator.has(symbol, 3, offglideClass) || symbol == 'u' && nextSymbol == 'j':
		generator.coarticulateWithoutOffglide()
	default:
		generator.coarticulateWithOffglide()
	}
}

// coarticulateReduced is action 4's step for @ and | without a record, and for p (parameters.md §12.3).
func (generator *Generator) coarticulateReduced() {
	cursor := generator.cursor()
	symbol, previousSymbol := cursor.Scan.Symbol(), cursor.Done.Symbol()
	segments := &generator.Segments
	if symbol == 'p' {
		if generator.has(previousSymbol, 0, voicedClass) && generator.has(previousSymbol, 0, stopClass) {
			segments[voicingTrack].Target = 54
		}
		if previousSymbol == 'K' {
			segments[secondFormant].Target, segments[thirdFormant].Target = 1652, 1856
		}
	}
	if !stressed(cursor.Scan) {
		segments[voicingTrack].Target = max(segments[voicingTrack].Target-2, 0)
	}
	segments[thirdFormant].Target = multiply(generator.NextTargets[thirdFormant]+generator.Last[thirdFormant]+segments[thirdFormant].Target, 10824)
}

// coarticulateWithoutOffglide is action 4's step for 3, o and v without a record, and u before j
// (parameters.md §12.4).
func (generator *Generator) coarticulateWithoutOffglide() {
	cursor := generator.cursor()
	current, previous := cursor.Scan, cursor.Done
	segments := &generator.Segments
	for track := firstFormant; track <= fourthFormant; track++ {
		segments[track].Onset = generator.Last[track]
	}
	switch previous.Symbol() {
	case 'G':
		segments[firstFormant].Mode = Shaped
		fallthrough
	case 'K':
		segments[fourthFormant].Mode = Shaped
		generator.Weights[fourthFormant] = 0
	case 'M':
		if isVowel(symbolBefore(previous)) && !stressed(current) {
			segments[firstFormant].Back = int32(previous.Value()) + 1
			segments[firstFormant].Mode |= PullBack
		}
	case 'P':
		if current.Symbol() == 'o' {
			segments[secondGainTrack].Mode, segments[thirdGainTrack].Mode = PullBack|Flat, PullBack|Flat
		}
	default:
		if isVowel(previous.Symbol()) {
			generator.setModes(firstFormant, fourthFormant, Shaped)
			generator.setForward(firstFormant, fourthFormant, int32(current.Value()>>1))
		}
	}
	if !stressed(current) {
		segments[voicingTrack].Target -= 3
	}
}

// coarticulateWithOffglide is action 4's step for the vowels with an offglide without a record: each formant
// and bandwidth moves from its target to its offglide (parameters.md §12.5). The original divides by the node's
// frames, and faults for a node of none; the port leaves the vowel transition as it is (parameters.md §15).
func (generator *Generator) coarticulateWithOffglide() {
	cursor := generator.cursor()
	current, previous := cursor.Scan, cursor.Done
	symbol, previousSymbol := current.Symbol(), previous.Symbol()
	_, phoneme := generator.phoneme(symbol)
	segments := &generator.Segments
	transition := generator.VowelTransition
	if frames := uint32(current.Value()); frames != 0 {
		generator.VowelTransition = int32(uint32(transition*segments[firstFormant].Frames)/frames+uint32(transition)) >> 1
	}
	generator.VowelOffset = multiply(segments[firstFormant].Frames, phoneme.Vowel.Offset)
	afterN := previousSymbol == 'N'
	nasalTargets, found := targetsAfterN[symbol]
	if !found {
		nasalTargets = otherTargetsAfterN
	}
	afterVowel := isVowel(previousSymbol)
	reduces := generator.has(symbol, 4, rColouredClass) && !stressed(current)
	for track := firstFormant; track < nasalZeroTrack; track++ {
		index := track - firstFormant
		segment, position := &segments[track], &generator.Tracks[track]
		saved, next, earliest := *segment, position.Next, position.Earliest
		onset, target, offglide := segment.Onset, segment.Target, generator.Offglide[index]
		midpoint := &generator.Midpoints[index]
		segment.Mode = PullBack | Shaped
		if reduces {
			target -= (target - offglide) >> 2
		}
		*midpoint = (target + offglide) / 2
		if track == thirdFormant && symbol == 'U' {
			generator.VowelOffset -= 7
		}
		if generator.VowelOffset < 1 {
			*midpoint = target
		} else {
			if generator.VowelOffset >= segment.Frames {
				*midpoint = offglide
				generator.VowelOffset = segment.Frames
			}
			segment.Mode, segment.Onset = Shaped, target
			if afterN {
				segment.Onset = nasalTargets[index]
			}
			if afterVowel {
				segment.Onset = generator.Last[track]
			}
			if previousSymbol == 'M' && isVowel(symbolBefore(previous)) && !stressed(current) && track == firstFormant {
				segment.Back, segment.Mode = int32(previous.Value())+1, PullBack|Shaped
			}
			segment.Target, segment.Frames = *midpoint, generator.VowelOffset
			segment.Forward = min(generator.TransitionLimit, generator.VowelOffset)
			generator.RenderSegment(track)
			onset = segment.Onset
			segment.Frames = saved.Frames - generator.VowelOffset
			position.Next += generator.VowelOffset
		}
		position.Earliest = next
		segment.Target, segment.Onset, segment.BackTarget = offglide, *midpoint, *midpoint
		segment.Forward = min(generator.TransitionLimit, generator.VowelTransition)
		segment.Back = segment.Forward
		generator.RenderSegment(track)
		segment.Target, segment.Frames, position.Next = target, saved.Frames, next
		switch {
		case afterVowel:
			segment.Onset, segment.Mode = generator.Last[track], 0
		case afterN:
			segment.Onset, segment.Mode = nasalTargets[index], PullBack
			if track == firstFormant {
				segment.Onset = onset
			}
			if track >= secondFormant && track <= fourthFormant {
				segment.Mode = 0
			}
		case previousSymbol != 'M' || track != secondFormant:
			segment.Mode = PullBack | PullForward
		}
		position.Earliest, segment.Forward, segment.Back, segment.BackTarget = earliest, saved.Forward, saved.Back, saved.BackTarget
	}
	if previousSymbol == 'K' || previousSymbol == 'G' {
		segments[fourthFormant].Mode = Shaped
		generator.Weights[fourthFormant] = 0
	}
	if !stressed(current) {
		segments[voicingTrack].Target -= 3
	}
	if (phoneme.Place == 2 || phoneme.Place == 3) && previousSymbol == 'G' {
		segments[firstFormant].Mode = Shaped
	}
}
