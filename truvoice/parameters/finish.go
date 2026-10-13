package parameters

import "strings"

// The class only action 10 tests (parameters.md §13.1): of class byte 2.
const velarClass = 0x80

// FinishPhoneme is action 10, which every rule ends with: it sets the loci, the consonants' targets, the pitch
// and the source, adjusts some contexts and holds the segments to their limits (parameters.md §13).
func (generator *Generator) FinishPhoneme() {
	generator.LocusWeights()
	generator.SpecialLoci()
	generator.ConsonantTargetsAndPitch()
	generator.SourceBoundaries()
	generator.FinalAdjustments()
	cursor := generator.cursor()
	segments := &generator.Segments
	if cursor.Scan.Symbol() != 'T' || cursor.First.Symbol() != '3' {
		for track := secondFormant; track <= fourthFormant; track++ {
			segments[track].Onset = max(segments[track].Onset, segments[track-1].Onset+200)
			segments[track].BackTarget = max(segments[track].BackTarget, segments[track-1].BackTarget+200)
		}
	}
	for track := range segments {
		segments[track].Forward = min(segments[track].Forward, 30)
		segments[track].Back = min(segments[track].Back, 20)
	}
	if segments[fricationTrack].Target > 0 {
		segments[fricationTrack].Target += 7
	}
}

// LocusWeights sets the formants' weights and transitions by the phonemes on either side (parameters.md §13.2).
func (generator *Generator) LocusWeights() {
	cursor := generator.cursor()
	current, previous := cursor.Scan, cursor.Done
	symbol, previousSymbol, nextSymbol := current.Symbol(), previous.Symbol(), cursor.First.Symbol()
	segments, weights := &generator.Segments, &generator.Weights
	if symbol == 'p' && generator.has(previousSymbol, 0, stopClass) {
		segments[thirdFormant].Onset = generator.Last[thirdFormant]
	}
	if generator.Category > generator.PreviousCategory {
		if !generator.closingLoci() {
			return
		}
	} else if !generator.openingLoci() {
		return
	}
	switch previousSymbol {
	case 'M', 'm':
		weights[fourthFormant] = 29520
		switch {
		case !generator.has(symbol, 2, frontClass):
			weights[secondFormant] = 3280
		case symbol == '4':
			weights[fourthFormant] = 16400
			segments[fourthFormant].Forward = 5
			segments[fourthFormant].Mode |= PullBack
		}
	case 'N', 'n':
		if !isVowel(symbol) {
			segments[thirdFormant].Onset, segments[fourthFormant].Onset = 2600, 3400
		}
		weights[fourthFormant], weights[thirdFormant] = 0, 0
		if generator.has(symbol, 4, highClass) && !generator.has(symbol, 2, frontClass) && !isVowel(symbol) {
			segments[secondFormant].Onset, weights[secondFormant] = 1670, 0
		}
		if symbol == 'd' && previousSymbol == 'N' {
			weights[firstVoiceTrack], weights[firstVoiceTrack+1] = 6560, 6560
			segments[firstVoiceTrack+1].Forward, segments[firstVoiceTrack].Forward = 4, 4
		}
	case '~':
		if symbol == 'p' && nextSymbol == ' ' {
			for track := secondFormant; track <= nasalZeroTrack; track++ {
				segments[track].Target, segments[track].Mode = generator.Last[track], Flat
			}
		}
	}
	if generator.has(symbol, 0, syllabicClass) && generator.has(previousSymbol, 0, voicedClass) &&
		previous.Flags()&(stressedFlag|glottalFlag) == stressedFlag && current.Flags()&(stressedFlag|glottalFlag) == stressedFlag {
		weights[pitchTrack] = 16400
		segments[pitchTrack].Mode = PullBack | Shaped
	}
}

// closingLoci is LocusWeights' step for a category above the previous one; it reports whether the rest of
// LocusWeights runs (parameters.md §13.2).
func (generator *Generator) closingLoci() bool {
	cursor := generator.cursor()
	current, previous := cursor.Scan, cursor.Done
	symbol, previousSymbol, nextSymbol := current.Symbol(), previous.Symbol(), cursor.First.Symbol()
	segments, weights := &generator.Segments, &generator.Weights
	if previousSymbol != ' ' {
		if generator.has(symbol, 3, palatalClass) && !generator.has(symbol, 0, sonorantClass) {
			weights[secondFormant] = 26240
			segments[secondFormant].Forward += 3
		}
		if generator.has(previousSymbol, 2, closureClass) || !generator.has(symbol, 2, closureClass) {
			return false
		}
		if symbol == 'q' && generator.has(previousSymbol, 2, vowelClass) {
			weights[firstFormant] = 32472
			segments[firstFormant].Back = int32(previous.Value() - previous.Value()/3)
		} else {
			weights[firstFormant] = 16400
		}
		if !generator.has(symbol, 2, labialClass) && symbol != 'T' {
			weights[thirdFormant], weights[secondFormant] = 32766, 32766
		}
		if nextSymbol == 's' && symbolAfter(cursor.First) == ' ' {
			weights[thirdFormant], weights[secondFormant] = 3280, 3280
		}
		if symbol == 'K' {
			weights[firstFormant] = 32766
			if generator.has(previousSymbol, 3, offglideClass) {
				weights[secondFormant] = 6560
			}
		}
	}
	if symbol != '~' && symbol != 'm' && symbol != 'n' {
		return true
	}
	weights[firstFormant] = 24600
	generator.setModes(firstFormant, nasalZeroTrack, PullBack|Flat)
	switch {
	case generator.has(previousSymbol, 0, sonorantClass):
		weights[nasalZeroTrack], weights[thirdFormant] = 18040, 29520
		segments[firstBandwidth].Forward, segments[nasalZeroTrack].Forward = 10, 10
		switch symbol {
		case 'm':
			weights[fourthFormant] = 9840
			segments[fourthFormant].Forward, segments[fourthFormant].Mode = 9, PullBack|Shaped
		case '~':
			second, third := &segments[secondFormant], &segments[thirdFormant]
			switch {
			case generator.has(nextSymbol, 0, sonorantClass):
				second.Target, third.Target = 900, 2176
			case generator.has(previousSymbol, 2, frontClass):
				second.Forward, second.Target, third.Target = 7, 1200, 2500
			default:
				second.Forward = 7
				second.Target = (generator.Last[secondFormant] + second.Target) / 2
				third.Target = (generator.Last[thirdFormant] + third.Target) / 2
			}
		}
	case previousSymbol == ' ':
		weights[voicingTrack] = 13120
		segments[voicingTrack].Forward = 6
	}
	return true
}

// openingLoci is LocusWeights' step for a category at most the previous one; it reports whether the rest of
// LocusWeights runs (parameters.md §13.2).
func (generator *Generator) openingLoci() bool {
	cursor := generator.cursor()
	current, previous := cursor.Scan, cursor.Done
	symbol, previousSymbol, nextSymbol := current.Symbol(), previous.Symbol(), cursor.First.Symbol()
	segments, weights := &generator.Segments, &generator.Weights
	if previousSymbol != ' ' {
		switch {
		case symbol == 'R' && previousSymbol == 's' && symbolBefore(previous) == 'C':
			weights[thirdFormant], weights[secondFormant] = 32472, 32472
		case symbol == 'R':
			weights[thirdFormant], weights[secondFormant] = 24600, 24600
		case generator.has(previousSymbol, 3, palatalClass) && !generator.has(previousSymbol, 0, sonorantClass) && !isVowel(symbol):
			weights[secondFormant] = 6560
			segments[secondFormant].Forward += 3
		}
		if generator.has(symbol, 3, palatalClass) && generator.has(previousSymbol, 1, affricateClass) &&
			!(symbol == 's' && previousSymbol == 'C' && (generator.has(nextSymbol, 2, vowelClass) || nextSymbol == 'R')) &&
			!(symbol == 'z' && previousSymbol == 'J' && generator.place(nextSymbol) == frontPlace) {
			segments[thirdFormant].Target += 300
		}
		if generator.has(symbol, 2, vowelClass) && previousSymbol == 'S' {
			segments[fricationTrack].Mode = Flat
		}
		if generator.has(previousSymbol, 0, nasalClass) && !generator.has(symbol, 0, sonorantClass) {
			generator.setForward(secondFormant, fourthFormant, 5)
			generator.setForward(firstBandwidth, nasalZeroTrack, 2)
		}
		if generator.has(symbol, 2, closureClass) || !generator.has(previousSymbol, 2, closureClass) {
			return false
		}
		if generator.has(previousSymbol, 0, stopClass) && generator.has(previousSymbol, 0, voicedClass) && generator.has(symbol, 2, vowelClass) {
			segments[fricationTrack].Mode = PullBack | Flat
		}
		weights[firstFormant], weights[thirdFormant], weights[secondFormant] = 16400, 0, 0
		switch {
		case generator.has(previousSymbol, 2, labialClass) && !isVowel(symbol):
			if previousSymbol != 'P' || symbol != 'S' {
				weights[secondFormant], weights[thirdFormant] = 6560, 22960
			}
		case generator.has(previousSymbol, 2, alveolarClass) && !isVowel(symbol):
			if previousSymbol != 'T' {
				if !generator.has(previousSymbol, 0, nasalClass) && previousSymbol != 'D' {
					segments[secondFormant].Onset = 1600
				}
				segments[thirdFormant].Onset = 2620
			}
			if generator.has(symbol, 2, rhoticClass) && previousSymbol != 'D' {
				segments[thirdFormant].Onset = 2300
			}
		case generator.has(previousSymbol, 2, velarClass):
			weights[firstFormant] = 0
		}
	}
	if generator.has(previousSymbol, 0, nasalClass) && generator.has(symbol, 0, sonorantClass) {
		weights[firstFormant] = 8200
		if !isVowel(symbol) {
			generator.setModes(firstFormant, thirdFormant, Shaped)
		}
		if previousSymbol != '~' || symbol != 'p' || nextSymbol != ' ' {
			weights[nasalZeroTrack], weights[thirdFormant] = 24600, 3280
			segments[nasalZeroTrack].Forward = 9
		}
	}
	return true
}

func (generator *Generator) place(symbol byte) int32 {
	_, phoneme := generator.phoneme(symbol)
	return phoneme.Place
}

// SpecialLoci sets the targets and transitions of a few contexts (parameters.md §13.3).
func (generator *Generator) SpecialLoci() {
	cursor := generator.cursor()
	current, previous := cursor.Scan, cursor.Done
	symbol, previousSymbol, nextSymbol := current.Symbol(), previous.Symbol(), cursor.First.Symbol()
	segments := &generator.Segments
	if generator.has(previousSymbol, 2, rhoticClass) || generator.has(previousSymbol, 4, rColouredClass) {
		if symbol == '~' || symbol == 'K' && !generator.has(nextSymbol, 2, vowelClass) {
			segments[secondFormant].Target, segments[thirdFormant].Target = 1700, 1900
		}
		if previousSymbol == 'R' && generator.has(symbol, 2, vowelClass) {
			segments[thirdFormant].Forward = 5
			if symbol == 'E' {
				segments[voicingTrack].Forward = 6
			}
		}
	}
	switch {
	case symbol == 'R' && generator.has(previousSymbol, 2, labialClass):
		generator.Weights[secondFormant] = 6560
	case generator.has(previousSymbol, 3, frontEndingClass) && strings.IndexByte("Znqt", symbol) >= 0:
		segments[secondFormant].Target = 1900
	}
	if symbol == 'd' && nextSymbol == 'b' {
		segments[firstFormant].Target = 182
	}
	if symbol == 'B' && generator.has(previousSymbol, 2, vowelClass) && symbolAfter(current) == 'p' {
		segments[voicingTrack].Mode = Flat
	}
	if symbol == 'y' && previousSymbol == 'L' {
		segments[firstFormant].Back = 5
	}
	if symbol == 'J' {
		segments[fricationTrack].Target = 64
	}
}

// FinalAdjustments sets the transitions of the bandwidths and adjusts a few contexts (parameters.md §13.4).
func (generator *Generator) FinalAdjustments() {
	cursor := generator.cursor()
	current, previous := cursor.Scan, cursor.Done
	symbol, previousSymbol, nextSymbol := current.Symbol(), previous.Symbol(), cursor.First.Symbol()
	frames, previousFrames := current.Value(), previous.Value()
	segments := &generator.Segments
	beforeSymbol := generator.beforePreviousSymbol()
	if symbol == 'M' && previousSymbol == 'a' {
		switch {
		case generator.has(nextSymbol, 2, labialClass):
			generator.setBack(secondFormant, fourthFormant, 2)
			generator.setForward(secondFormant, fourthFormant, 3)
		case nextSymbol == 'p':
			generator.setBack(secondFormant, thirdFormant, 3)
			generator.setForward(thirdFormant, fourthFormant, 3)
			segments[secondFormant].Forward = 4
		}
	}
	if generator.Coarticulation != 0 && !generator.has(symbol, 2, vowelClass) {
		for track := firstFormant; track <= fourthFormant; track++ {
			segments[track].Mode &^= PullBack
			if !generator.has(symbol, 0, nasalClass) {
				segments[track].Onset = generator.Last[track]
			}
		}
		if symbol == 'L' && !isVowel(previousSymbol) {
			segments[firstFormant].Mode = Flat
		}
		generator.Coarticulation = 0
	}
	if isVowel(beforeSymbol) && (previousSymbol != 'Y' || !generator.has(beforeSymbol, 3, frontEndingClass)) {
		for track := firstFormant; track <= fourthFormant; track++ {
			segments[track].Back = int32(min(uint32(segments[track].Back), previousFrames))
		}
	}
	if generator.has(symbol, 0, stopClass) {
		for track := firstBandwidth; track < nasalZeroTrack; track++ {
			segments[track].Forward = segments[track].Frames - 1
		}
	}
	if symbol == 'n' || previousSymbol == 'n' {
		segments[nasalZeroTrack].Mode, segments[nasalZeroTrack].Forward, segments[nasalZeroTrack].Back = Bridged, 3, 3
		if previousSymbol == 'n' && symbol != 'R' {
			for track := firstFormant; track < nasalZeroTrack; track++ {
				segments[track].Mode |= PullBack
				segments[track].Back = int32(previousFrames / 3)
			}
		}
	}
	third := func(frames uint32) int32 {
		return max(int32(frames/3), 2)
	}
	switch {
	case !generator.has(previousSymbol, 2, vowelClass) || previousSymbol == 'p':
		if !generator.has(symbol, 2, vowelClass) || symbol == 'p' {
			break
		}
		for track := firstBandwidth; track < nasalZeroTrack; track++ {
			segment := &segments[track]
			segment.Back, segment.Forward, segment.Mode = third(previousFrames), 2, Bridge
			if frames/3 > 1 {
				segment.Forward = 3
			}
			if generator.Coarticulation != 0 {
				generator.Tracks[track].Next -= int32(frames)
				segment.Frames = int32(frames)
			}
		}
	default:
		for track := firstBandwidth; track < nasalZeroTrack; track++ {
			segment := &segments[track]
			segment.Mode |= PullBack
			segment.BackTarget = segment.Onset
			if segment.Mode == PullBack|Flat {
				segment.BackTarget = segment.Target
			}
			if !generator.has(symbol, 2, vowelClass) {
				segment.Back, segment.Forward, segment.Mode = third(previousFrames), third(frames), Bridged
				continue
			}
			segment.Back, segment.Forward, segment.Mode = 1, 0, Bridge
			if !generator.has(previousSymbol, 1, reducedClass) {
				segment.Back = third(previousFrames)
			}
			if !generator.has(symbol, 1, reducedClass) {
				segment.Forward = third(frames)
			}
			if generator.Coarticulation != 0 {
				generator.Tracks[track].Next -= int32(frames)
				segment.Frames = int32(frames)
			}
		}
		if symbol == ' ' {
			for index, value := range []int32{300, 188, 194} {
				segments[firstBandwidth+index].Onset, segments[firstBandwidth+index].BackTarget = value, value
			}
		}
	}
	if previousSymbol == 'R' && symbolBefore(previous) == 'p' {
		for track := firstFormant; track <= fourthFormant; track++ {
			segments[track].Mode &= PullForward | Flat
		}
	}
	if symbol == 'M' {
		if generator.has(previousSymbol, 2, vowelClass) {
			first := &segments[firstFormant]
			first.Mode, first.Forward = Ramp, int32(frames>>1)
			first.Onset, first.Target = generator.Last[firstFormant], generator.Last[firstFormant]-50
		}
		if generator.has(previousSymbol, 3, frontEndingClass) && generator.has(nextSymbol, 2, vowelClass) {
			second := &segments[secondFormant]
			second.Mode, second.Forward, second.Back = Bridged, int32(frames>>1), int32(previousFrames>>2)
		}
	}
	if segments[firstFormant].Target > 700 && generator.has(symbol, 2, vowelClass) {
		segments[voicingTrack].Target -= 3
	}
	if symbol == 'p' && previousSymbol == '~' {
		generator.setModes(secondGainTrack, spareTrack, Flat)
		generator.setTargets(voicingTrack, 55)
		generator.setTargets(secondGainTrack, 57, 0, 0, 55, 0)
		generator.setTargets(firstFormant, 250, 1568, 2208, 3600)
		second := &segments[secondFormant]
		second.Mode, second.Forward, second.Onset = PullBack|Shaped, 2, 1700
		frication, voicing := &generator.Tracks[fricationTrack], &generator.Tracks[voicingTrack]
		generator.Ring(fricationTrack).Repeat(frication.Next, 1, 58)
		segments[fricationTrack].Frames--
		frication.Next++
		generator.Ring(voicingTrack).Repeat(voicing.Next, 1, 51)
		voicing.Next++
		segments[voicingTrack].Frames--
	}
	if symbol == '~' {
		if generator.has(previousSymbol, 2, vowelClass) && nextSymbol == 'p' {
			generator.setTargets(firstFormant, 250, 1196, 2288)
		}
		if previousSymbol == 'i' {
			second := &segments[secondFormant]
			second.Mode, second.Back, second.BackTarget, second.Forward, second.Onset = PullBack|Shaped, 3, 1500, 3, 1500
		}
	}
	if symbol == 'a' {
		segments[voicingTrack].Target = 60
	}
}
