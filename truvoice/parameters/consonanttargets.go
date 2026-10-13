package parameters

import "strings"

// The class only action 13 tests (parameters.md §15.1): of class byte 4.
const dentalClass = 0x10

// The Q15 weight of the mean that makes t's targets, and the most frames action 13 leaves a segment
// (parameters.md §15).
const (
	meanWeight = 0x2a48
	mostFrames = 60
)

// ConsonantTargetsAndPitch is action 13: it sets the consonants' formant targets and transitions by the phonemes
// around them, and the pitch's small movements (parameters.md §15).
func (generator *Generator) ConsonantTargetsAndPitch() {
	generator.consonantTargets()
	generator.limitsAndOnsets()
	generator.targetsByPhoneme()
	generator.afterNasal()
	generator.nasalTransitions()
	generator.liquidTransitions()
	generator.fricativeTransitions()
	generator.afterStop()
	generator.formantOnsets()
	generator.pitchMovements()
	cursor := generator.cursor()
	previous := cursor.Done
	beforeSymbol := generator.beforePreviousSymbol()
	if (isVowel(beforeSymbol) || isWordMark(beforeSymbol)) && (previous.Symbol() != 'Y' || !generator.has(beforeSymbol, 3, frontEndingClass)) {
		for track := firstFormant; track <= fourthFormant; track++ {
			generator.Segments[track].Back = int32(min(uint32(generator.Segments[track].Back), previous.Value()))
		}
	}
}

// beforePreviousSymbol returns the symbol of the node before the previous one, or of the node before that if it is
// in class mark, the pause if there is none (parameters.md §13.4).
func (generator *Generator) beforePreviousSymbol() byte {
	before := generator.cursor().Done.Previous
	symbol := symbolOf(before)
	if generator.has(symbol, 0, markClass) {
		symbol = ' '
		if before.Previous != nil {
			symbol = before.Previous.Symbol()
		}
	}
	return symbol
}

// isLocusConsonant reports whether a symbol is one of B P D T G K, a fricative, H or d (parameters.md §15.2).
func (generator *Generator) isLocusConsonant(symbol byte) bool {
	return strings.IndexByte("PTKBDGHd", symbol) >= 0 || generator.has(symbol, 0, fricativeClass)
}

// holdLast makes F1 to F4 hold their last values.
func (generator *Generator) holdLast() {
	for track := firstFormant; track <= fourthFormant; track++ {
		generator.Segments[track].Target, generator.Segments[track].Mode = generator.Last[track], Flat
	}
}

// shapeFromLast makes F1 to F4 start from their last values.
func (generator *Generator) shapeFromLast() {
	for track := firstFormant; track <= fourthFormant; track++ {
		generator.Segments[track].Onset, generator.Segments[track].Mode = generator.Last[track], Shaped
	}
}

// continueFromLast makes the tracks start from their last values, with the whole segment for the shape.
func (generator *Generator) continueFromLast(first, last int) {
	for track := first; track <= last; track++ {
		segment := &generator.Segments[track]
		segment.BackTarget, segment.Onset, segment.Mode, segment.Forward = generator.Last[track], generator.Last[track], Shaped, segment.Frames
	}
}

// consonantTargets sets the targets of a consonant by the phonemes around it (parameters.md §15.2).
func (generator *Generator) consonantTargets() {
	cursor := generator.cursor()
	current, previous := cursor.Scan, cursor.Done
	symbol, previousSymbol, nextSymbol := current.Symbol(), previous.Symbol(), cursor.First.Symbol()
	following := symbolAfter(current)
	segments, last := &generator.Segments, &generator.Last
	if symbol == 't' {
		for track := firstFormant; track <= thirdFormant; track++ {
			segments[track].Target = multiply(last[track]+generator.NextTargets[track]+segments[track].Target, meanWeight)
		}
	}
	if generator.has(previousSymbol, 3, highFrontClass) && !generator.has(symbol, 0, nasalClass) && !generator.has(symbol, 4, dentalClass) &&
		!generator.has(symbol, 2, labialClass|velarClass) && !isVowel(symbol) {
		segments[secondFormant].Forward -= 2
	}
	generator.DatabaseLoci = false
	if generator.isLocusConsonant(symbol) || symbol == 'R' && previousSymbol != 's' ||
		symbol == 'L' && generator.has(symbolBefore(current), 0, stopClass) ||
		symbol == 'Y' && generator.has(previousSymbol, 2, vowelClass) && generator.has(following, 2, vowelClass) {
		if vowel := generator.vowels.Index(nextSymbol); vowel >= 0 {
			generator.DatabaseLoci = generator.lociFromDatabase(vowel)
		} else if nextSymbol == 'R' && symbol != 'P' && symbol != 's' || generator.has(symbol, 0, stopClass) && following == 'L' {
			generator.targetsBeforeLiquid()
		}
	}
	if symbol == 'T' && following == 'o' {
		segments[fricationTrack].Target = 68
		generator.setTargets(secondGainTrack, 50, 50, 70, 70)
	}
	if symbol == 'K' && strings.IndexByte("rgc", nextSymbol) >= 0 {
		generator.setTargets(secondFormant, 1080, 1680, 2896)
		segments[fricationTrack].Target, segments[secondGainTrack].Target = 62, 65
		generator.setTargets(firstBandwidth+1, 200, 200)
	}
	if symbol == 'K' && nextSymbol == 'g' {
		segments[fricationTrack].Target, segments[aspirationTrack].Target, segments[fifthGainTrack].Target = 65, 55, 70
	}
	if previousSymbol == 'K' && strings.IndexByte("rgc", symbol) >= 0 {
		generator.setTargets(secondGainTrack, 60, 60, 60)
	}
	if symbol == 'F' {
		segments[firstFormant].Target = 290
	}
	if isVoicelessStop(symbol) && generator.DatabaseLoci {
		segments[firstFormant].Target = min(segments[firstFormant].Target, 500)
	}
	if symbol == 'K' && nextSymbol == '|' {
		generator.setTargets(secondFormant, 1772, 2240)
	}
	if segments[voicingTrack].Target > 0 || segments[aspirationTrack].Target > 0 {
		segments[fourthFormant].Target = min(segments[fourthFormant].Target, generator.voice().FourthFormantLimit)
		for track := secondFormant; track <= fourthFormant; track++ {
			segments[track].Target = max(segments[track].Target, segments[track-1].Target+200)
		}
	}
}

// limitsAndOnsets holds the segments' transitions and frames to their limits, sets the onsets by the weights, and
// those of W after T, D or q (parameters.md §15.3).
func (generator *Generator) limitsAndOnsets() {
	cursor := generator.cursor()
	symbol, previousSymbol := cursor.Scan.Symbol(), cursor.Done.Symbol()
	segments := &generator.Segments
	for track := range segments {
		if track == nasalZeroTrack || generator.Coarticulation != 0 && track >= firstFormant && track < nasalZeroTrack {
			continue
		}
		segment := &segments[track]
		segment.Forward = min(segment.Forward, generator.TransitionLimit)
		segment.Back = segment.Forward
		segment.Frames = min(segment.Frames, mostFrames)
	}
	first := firstFormant
	if generator.Coarticulation != 0 && generator.has(symbol, 2, vowelClass) {
		first = nasalZeroTrack
	}
	for track := first; track <= pitchTrack; track++ {
		if previousSymbol == 'N' && generator.has(symbol, 2, vowelClass) && track >= secondFormant && track < nasalZeroTrack {
			continue
		}
		segment := &segments[track]
		weight := generator.Weights[track]
		segment.Onset = multiply(generator.Last[track], 0x7ffe-weight) + multiply(segment.Target, weight)
		segment.BackTarget = segment.Onset
	}
	if strings.IndexByte("TDq", previousSymbol) >= 0 && symbol == 'W' {
		for index, value := range []int32{1200, 2050, 2500} {
			segment := &segments[secondFormant+index]
			segment.BackTarget, segment.Onset = value, value
		}
		segments[fourthFormant].Forward = 3
	}
}

// targetsByPhoneme sets the targets and transitions of a few consonants by the phonemes around them
// (parameters.md §15.4).
func (generator *Generator) targetsByPhoneme() {
	cursor := generator.cursor()
	current, previous := cursor.Scan, cursor.Done
	symbol, previousSymbol, nextSymbol := current.Symbol(), previous.Symbol(), cursor.First.Symbol()
	following, before := symbolAfter(current), symbolBefore(current)
	afterBoundary := isWordMark(before) || before == ' '
	segments := &generator.Segments
	switch symbol {
	case 'C':
		if following == 's' && symbolAfter(current.Next) == ' ' {
			generator.setTargets(secondFormant, 1844, 2528)
		}
		fallthrough
	case 's':
		if previousSymbol != 'C' {
			break
		}
		if following == ' ' {
			generator.setTargets(secondFormant, 1844, 2528)
			segments[secondGainTrack].Target, segments[thirdGainTrack].Target, segments[fricationTrack].Target = 60, 70, 60
		} else if isVowel(generator.beforePreviousSymbol()) && !generator.has(nextSymbol, 2, vowelClass) && nextSymbol != 'R' {
			generator.holdLast()
		}
	case 'D':
		if isVowel(previousSymbol) {
			generator.shapeFromLast()
		}
	case 'F':
		if previousSymbol == ' ' {
			segments[bypassTrack].Forward, segments[fricationTrack].Forward = 7, 7
		}
		if generator.has(nextSymbol, 2, vowelClass) {
			segments[secondGainTrack].Target, segments[fricationTrack].Target = 0, 60
		}
	case 'G':
		if nextSymbol == 'p' {
			segments[secondFormant].Back = 7
		}
		if previousSymbol != ' ' && nextSymbol != 'Y' {
			back := int32(previous.Value() - previous.Value()>>2)
			segments[thirdFormant].Back, segments[secondFormant].Back = back, back
		}
		if nextSymbol != 'o' && (generator.has(nextSymbol, 2, frontClass) || !generator.has(nextSymbol, 4, highClass)) {
			segments[voicingTrack].Forward = 8
		}
		if nextSymbol == 'l' {
			generator.setTargets(firstFormant, 322, 1205, 2531, 3587)
		}
	case 'J':
		if !generator.has(previousSymbol, 0, voicedClass) {
			segments[voicingTrack].Forward = 8
		}
	case 'L':
		if isVowel(previousSymbol) {
			segments[firstFormant].Onset, segments[firstFormant].Mode = generator.Last[firstFormant], Shaped
		}
		if previousSymbol == 'E' {
			segments[secondFormant].Target = 1185
		}
	case 'P':
		if nextSymbol == 'E' && afterBoundary {
			segments[fricationTrack].Target, segments[secondGainTrack].Target, segments[fifthGainTrack].Target = 55, 62, 0
		}
	case 'T':
		if nextSymbol != 'E' && nextSymbol != 'e' {
			break
		}
		for track := secondGainTrack; track <= thirdGainTrack; track++ {
			if segments[track].Target > 3 {
				segments[track].Target -= 3
			}
		}
		if nextSymbol == 'e' {
			segments[secondGainTrack].Target = 50
			generator.setTargets(fourthGainTrack, 65, 0, 0)
		} else {
			segments[fricationTrack].Target = 62
			generator.setTargets(thirdGainTrack, 65, 65, 68, 68)
			if afterBoundary {
				generator.setTargets(fricationTrack, 65, 60)
				generator.setTargets(thirdGainTrack, 0, 60, 77, 77, 0)
			}
		}
	case 'Z':
		if generator.has(nextSymbol, 2, vowelClass) {
			generator.setForward(firstFormant, fourthFormant, 3)
		}
	case 'j':
		if previousSymbol == 'O' && following == 'D' {
			generator.setTargets(firstFormant, 376, 604)
			segments[fourthFormant].Target = 3744
			segments[secondFormant].Forward, segments[fourthFormant].Forward = 3, 3
		}
	}
}

// afterNasal sets the transitions of a phoneme after a nasal (parameters.md §15.5).
func (generator *Generator) afterNasal() {
	cursor := generator.cursor()
	current, previous := cursor.Scan, cursor.Done
	symbol, previousSymbol, nextSymbol := current.Symbol(), previous.Symbol(), cursor.First.Symbol()
	if !generator.has(previousSymbol, 0, nasalClass) {
		return
	}
	segments := &generator.Segments
	first, zero := &segments[firstFormant], &segments[nasalZeroTrack]
	beforeSymbol := generator.beforePreviousSymbol()
	switch previousSymbol {
	case 'M':
		if (!generator.has(symbol, 2, vowelClass) || isVowel(symbol) && (stressed(current) || !generator.has(nextSymbol, 0, nasalClass))) &&
			!generator.has(symbol, 1, consonantClass) {
			zero.Mode = Flat
		} else {
			zero.Onset, zero.Forward, zero.Mode = 380, 4, Shaped
		}
		switch {
		case isVowel(symbol):
			first.BackTarget, first.Mode = first.Onset, PullBack
		case isVowel(beforeSymbol):
			for track := firstFormant; track <= thirdFormant; track++ {
				segment := &segments[track]
				segment.BackTarget, segment.Back = segment.Onset, 3
				segment.Mode |= PullBack
			}
		default:
			first.Onset = generator.Last[firstFormant]
			if generator.has(symbol, 0, sonorantClass) {
				first.Mode |= PullBack
				segments[fourthFormant].Mode |= PullBack
				segments[secondFormant].Mode &= PullBack | Shaped
				segments[thirdFormant].Mode &= PullBack | Shaped
				for track := firstBandwidth; track < nasalZeroTrack; track++ {
					segments[track].Mode &= Shaped
				}
			}
		}
	case 'N':
		switch {
		case generator.has(symbol, 0, stopClass):
			zero.Mode = Flat
		case !generator.has(symbol, 2, vowelClass) || !stressed(current) && generator.has(nextSymbol, 0, nasalClass) ||
			symbol == '|' && generator.has(beforeSymbol, 2, vowelClass):
			zero.Forward, zero.Mode, zero.Onset = 4, Shaped, 400
		default:
			zero.Forward, zero.Mode = 4, Shaped
		}
		if generator.has(symbol, 0, sonorantClass) && !isVowel(symbol) {
			segments[fourthFormant].Back = 4
			segments[fourthFormant].Mode |= PullBack
			first.Back = 3
			for track := secondFormant; track < nasalZeroTrack; track++ {
				segments[track].Mode &= Shaped
			}
		}
		if !isVowel(symbol) {
			first.Mode |= PullBack
			first.BackTarget = first.Onset
		}
		if symbol == 'X' {
			first.Mode |= Shaped
		}
		if generator.has(symbol, 1, consonantClass) {
			segments[secondFormant].Mode &= Shaped
		}
	case 'n':
		if !isVowel(symbol) {
			segments[thirdFormant].Onset, segments[fourthFormant].Onset = 2600, 3600
		}
		if generator.has(previousSymbol, 4, highClass) && !generator.has(previousSymbol, 2, frontClass) && !isVowel(symbol) {
			segments[secondFormant].Onset = 1670
		}
	}
	if isVowel(beforeSymbol) && previous.Value() < 6 {
		for track := firstFormant; track <= fourthFormant; track++ {
			segments[track].Mode |= PullBack
			segments[track].Back = int32(previous.Value())
		}
	}
	if generator.has(symbol, 3, approximantClass) {
		zero.Mode, zero.Onset, zero.Forward = Shaped, 380, 4
	}
	if generator.has(symbol, 2, vowelClass) {
		if before := symbolBefore(current.Previous); before == ' ' || isWordMark(before) || before == 0 || generator.has(before, 2, vowelClass) {
			segments[secondFormant].Mode = 0
		}
	}
}

// nasalTransitions sets a nasal's transitions (parameters.md §15.5).
func (generator *Generator) nasalTransitions() {
	cursor := generator.cursor()
	current, previous := cursor.Scan, cursor.Done
	symbol, previousSymbol, nextSymbol := current.Symbol(), previous.Symbol(), cursor.First.Symbol()
	if !generator.has(symbol, 0, nasalClass) {
		return
	}
	segments, last := &generator.Segments, &generator.Last
	first, fourth := &segments[firstFormant], &segments[fourthFormant]
	if symbol == 'M' || symbol == 'N' {
		segments[thirdFormant].Mode, segments[secondFormant].Mode = Flat, Flat
		segments[nasalZeroTrack].Mode, segments[nasalZeroTrack].Back = PullBack|Flat, 4
	}
	if symbol == 'M' || symbol == 'N' && isVowel(previousSymbol) {
		fourth.Onset, fourth.Mode = last[fourthFormant], Shaped
	}
	if symbol == 'N' {
		switch nextSymbol {
		case 'e':
			second := &segments[secondFormant]
			second.Onset, second.Mode, second.Forward = 812, Shaped, second.Frames
		case 'b':
			third := &segments[thirdFormant]
			third.Onset, third.Mode, third.Forward = 2474, Shaped, third.Frames
		}
	}
	if generator.has(nextSymbol, 0, stopClass) && !generator.has(nextSymbol, 0, voicedClass) {
		first.Mode, first.Forward = Shaped, 2
	}
	if isVowel(previousSymbol) && isVowel(nextSymbol) && current.Value() < 6 {
		generator.holdLast()
	}
	if symbol == 'N' && (previousSymbol == ' ' || generator.has(previousSymbol, 2, vowelClass)) && generator.has(nextSymbol, 2, vowelClass) {
		segments[secondFormant].Target, segments[secondFormant].Mode = 876, Flat
		generator.Tracks[secondFormant].Earliest = generator.Tracks[secondFormant].Next
	}
	if (symbol == 'N' || symbol == '~') && (isVowel(previousSymbol) || generator.has(previousSymbol, 0, fricativeClass) ||
		generator.has(previousSymbol, 2, closureClass) && isVowel(nextSymbol)) {
		fall := int32(100)
		if current.Value() < 5 {
			fall = 50
		}
		first.Onset, first.Target = last[firstFormant], max(last[firstFormant]-fall, 250)
		first.Mode, first.Forward = Shaped, int32(current.Value()>>1)
	}
}

// liquidTransitions sets the formants' transitions of R, of L after a stop and of Y between phonemes in class
// vowel (parameters.md §15.6).
func (generator *Generator) liquidTransitions() {
	cursor := generator.cursor()
	current, previous, next := cursor.Scan, cursor.Done, cursor.First
	symbol, previousSymbol, nextSymbol := current.Symbol(), previous.Symbol(), next.Symbol()
	segments, last := &generator.Segments, &generator.Last
	switch symbol {
	case 'R':
		if nextSymbol == 'E' {
			segments[voicingTrack].Forward = 5
		}
		if previousSymbol == 'X' {
			segments[thirdFormant].Back = 3
		}
		frames := int32(current.Value())
		switch {
		case isVowel(previousSymbol):
			generator.rAfterVowel()
		case generator.has(previousSymbol, 2, vowelClass):
			generator.shapeFromLast()
		case generator.has(previousSymbol, 1, consonantClass) && previousSymbol != 's' || strings.IndexByte("ln ", previousSymbol) >= 0:
			generator.liquidTrajectories()
		case generator.has(previousSymbol, 2, closureClass):
			for track := firstFormant; track <= fourthFormant; track++ {
				segments[track].Onset, segments[track].Mode, segments[track].Forward = last[track], Shaped, frames
			}
			if isVowel(nextSymbol) {
				segments[firstFormant].Mode, segments[firstFormant].Target = Flat, last[firstFormant]
				if !generator.has(previousSymbol, 0, voicedClass) {
					segments[fourthFormant].Mode, segments[fourthFormant].Target = Flat, last[fourthFormant]
				}
			}
		case generator.has(previousSymbol, 0, fricativeClass):
			generator.setModes(firstFormant, fourthFormant, PullBack|Shaped)
			generator.setBack(firstFormant, fourthFormant, 3)
			if isVowel(nextSymbol) && !generator.has(previousSymbol, 0, voicedClass) {
				segments[fourthFormant].Mode, segments[fourthFormant].Target = Flat, last[fourthFormant]
			}
		default:
			segments[firstFormant].Mode, segments[firstFormant].Onset, segments[firstFormant].Forward = Shaped, last[firstFormant], frames
			generator.setModes(secondFormant, fourthFormant, Flat)
		}
		if generator.has(nextSymbol, 2, vowelClass) && !stressed(next) && !isWordMark(symbolBefore(current)) && previousSymbol == 'T' {
			generator.holdLast()
		}
	case 'L':
		if generator.has(symbolBefore(current), 0, stopClass) {
			generator.liquidTrajectories()
		}
	case 'Y':
		if generator.has(symbolAfter(current), 2, vowelClass) && generator.has(previousSymbol, 2, vowelClass) &&
			!generator.has(previousSymbol, 3, frontEndingClass) {
			generator.liquidTrajectories()
		}
	}
}

// rAfterVowel sets the formants of R after a vowel (parameters.md §15.6).
func (generator *Generator) rAfterVowel() {
	cursor := generator.cursor()
	segments, last := &generator.Segments, &generator.Last
	first, second, third, fourth := &segments[firstFormant], &segments[secondFormant], &segments[thirdFormant], &segments[fourthFormant]
	first.Onset, second.Onset, third.Onset = last[firstFormant]-10, last[secondFormant]-20, last[thirdFormant]-40
	fourth.Mode, fourth.Target = Flat, last[fourthFormant]
	for track := firstFormant; track <= thirdFormant; track++ {
		segments[track].Mode, segments[track].Forward = Shaped, int32(cursor.Scan.Value()>>1)
	}
	switch {
	case generator.has(cursor.First.Symbol(), 2, vowelClass):
		generator.liquidTrajectories()
	case !generator.DatabaseLoci:
		first.Target, second.Target, third.Target = last[firstFormant]-20, last[secondFormant]-40, last[thirdFormant]-110
	default:
		first.Target -= 20
		if first.Target > first.Onset {
			first.Target = first.Onset - 10
		}
		second.Target -= 40
		if second.Target > second.Onset {
			second.Target = second.Onset - 40
		}
		third.Target -= 110
		if third.Target > third.Onset {
			third.Target = third.Onset - 60
		}
		second.Target = max(second.Target, first.Target+200)
		third.Target = max(third.Target, second.Target+200)
	}
}

// fricativeTransitions sets the transitions of a fricative after a phoneme in class vowel and of a phoneme after a
// fricative, and a few targets of F (parameters.md §15.7).
func (generator *Generator) fricativeTransitions() {
	cursor := generator.cursor()
	current, previous := cursor.Scan, cursor.Done
	symbol, previousSymbol, nextSymbol := current.Symbol(), previous.Symbol(), cursor.First.Symbol()
	segments, last := &generator.Segments, &generator.Last
	switch {
	case generator.has(symbol, 0, fricativeClass) && generator.has(previousSymbol, 2, vowelClass):
		segments[voicingTrack].Back = 5
		switch {
		case generator.has(symbol, 4, dentalClass) || generator.has(symbol, 2, labialClass):
			generator.setBack(secondFormant, thirdFormant, 7)
			generator.setForward(secondFormant, thirdFormant, 2)
			segments[voicingTrack].Forward = 4
		case generator.has(symbol, 2, alveolarClass):
			generator.setBack(secondFormant, thirdFormant, 6)
			generator.setForward(secondFormant, thirdFormant, 2)
		}
		if isVowel(previousSymbol) && symbol == 'S' {
			for track := firstFormant; track <= fourthFormant; track++ {
				segments[track].Onset = last[track]
			}
		}
		if isVowel(previousSymbol) && symbol == 'F' && isVowel(nextSymbol) {
			for track := firstFormant; track <= fourthFormant; track++ {
				segments[track].Onset, segments[track].Forward = last[track], int32(current.Value())
			}
			for track := thirdGainTrack; track <= bypassTrack; track++ {
				segments[track].Target = 60
			}
		}
	case generator.has(previousSymbol, 0, fricativeClass):
		if generator.has(symbol, 2, vowelClass) && generator.Coarticulation == 0 {
			switch {
			case !generator.has(previousSymbol, 4, dentalClass) && !generator.has(previousSymbol, 2, labialClass):
				if generator.has(previousSymbol, 2, alveolarClass) {
					generator.setForward(secondGainTrack, spareTrack, 7)
				}
			case generator.has(previousSymbol, 0, voicedClass):
				segments[voicingTrack].Forward = 5
			}
			if previousSymbol == 's' {
				segments[fricationTrack].Forward = 5
			}
		}
		frames := previous.Value()
		back := int32(frames - frames>>2)
		if previousSymbol == 's' || symbol == 'R' && (previousSymbol == 'z' && generator.BeforePrevious == 'J' || previousSymbol == 's' && generator.BeforePrevious == 'C') {
			back = int32(frames)
		}
		for track := firstFormant; track <= fourthFormant; track++ {
			segments[track].BackTarget, segments[track].Back = segments[track].Onset, back
		}
	}
	if symbol == 'F' {
		if strings.IndexByte("4kU", nextSymbol) >= 0 {
			segments[thirdGainTrack].Target, segments[fourthGainTrack].Target, segments[bypassTrack].Target = 35, 35, 65
		}
		if generator.has(nextSymbol, 2, vowelClass) && generator.has(nextSymbol, 4, highClass) && generator.has(nextSymbol, 2, frontClass) {
			segments[bypassTrack].Target = 60
		}
		if previousSymbol == 'T' && nextSymbol == 'U' {
			segments[bypassTrack].Target = 55
		}
	}
}

// afterStop sets the transitions after a stop and after R (parameters.md §15.7).
func (generator *Generator) afterStop() {
	cursor := generator.cursor()
	current, previous := cursor.Scan, cursor.Done
	symbol, previousSymbol := current.Symbol(), previous.Symbol()
	segments := &generator.Segments
	if generator.has(previousSymbol, 0, stopClass) && (generator.Coarticulation == 0 || !generator.has(symbol, 2, vowelClass)) {
		switch previousSymbol {
		case 'B', 'P':
			segments[secondFormant].Mode &^= PullBack
			segments[thirdFormant].Mode &^= PullBack
			segments[voicingTrack].Forward = 2
			if previousSymbol == 'B' && generator.has(symbol, 2, vowelClass) || previousSymbol == 'P' && symbol == 'o' {
				generator.setForward(firstFormant, thirdFormant, 3)
			}
		case 'D':
			front, back := generator.has(symbol, 2, frontClass), strings.IndexByte("oub", symbol) >= 0
			if !front && !back {
				break
			}
			generator.setForward(secondFormant, thirdFormant, 5)
			if front {
				generator.setBack(secondFormant, thirdFormant, 0)
			}
			if back {
				segments[voicingTrack].Forward = 4
			}
			if generator.place(symbol) == 0 {
				segments[secondFormant].Onset = 1924
			}
		case 'G':
			segments[voicingTrack].Forward = 5
			generator.setBack(secondFormant, thirdFormant, 0)
			if place := generator.place(symbol); place == 2 || place == 3 {
				generator.setForward(firstFormant, fourthFormant, 5)
			} else {
				generator.setForward(secondFormant, thirdFormant, 7)
			}
			if symbol == 'o' || symbol == 'E' {
				segments[voicingTrack].Forward = 4
			}
		case 'K':
			segments[voicingTrack].Forward = 3
			if symbol == 'u' {
				segments[thirdFormant].Forward = 6
			}
		case 'T':
			if generator.place(symbol) <= 1 && generator.has(symbol, 2, frontClass) && generator.has(symbol, 4, highClass) && symbol != 'E' {
				segments[thirdFormant].BackTarget, segments[thirdFormant].Onset = 2700, 2700
			}
		}
	}
	if previousSymbol == 'R' {
		beforeSymbol := generator.beforePreviousSymbol()
		back := int32(previous.Value() + 1)
		if isVowel(symbol) && (isVowel(beforeSymbol) || generator.has(beforeSymbol, 0, fricativeClass) || generator.has(beforeSymbol, 2, closureClass)) {
			segments[fourthFormant].Mode, segments[fourthFormant].Back = PullBack, back
		}
		if generator.has(symbol, 2, vowelClass) && !stressed(current) && beforeSymbol == 'T' {
			generator.setModes(firstFormant, fourthFormant, PullBack)
			generator.setBack(firstFormant, fourthFormant, back)
		}
	}
}

// formantOnsets starts the formants of closures and of consonants before a phoneme in class vowel or R from their
// last values, and sets a few contexts (parameters.md §15.8).
func (generator *Generator) formantOnsets() {
	cursor := generator.cursor()
	current, previous := cursor.Scan, cursor.Done
	symbol, previousSymbol, nextSymbol := current.Symbol(), previous.Symbol(), cursor.First.Symbol()
	segments, last := &generator.Segments, &generator.Last
	first := &segments[firstFormant]
	if generator.has(symbol, 2, closureClass) && !generator.has(symbol, 0, nasalClass) {
		first.Onset, first.BackTarget, first.Mode, first.Forward = last[firstFormant], last[firstFormant], Shaped, first.Frames
		if generator.BurstFrames >= 1 && generator.BurstFrames != noVoiceOnset {
			first.Forward -= generator.BurstFrames
		}
		if isVowel(previousSymbol) && (!isWordMark(symbolBefore(current)) || !generator.has(previousSymbol, 3, frontEndingClass)) {
			generator.continueFromLast(secondFormant, fourthFormant)
		}
		if previousSymbol == 'j' && isVowel(generator.beforePreviousSymbol()) {
			generator.continueFromLast(firstFormant, fourthFormant)
		}
	}
	if generator.isLocusConsonant(symbol) && symbol != 'F' && (generator.has(nextSymbol, 2, vowelClass) || nextSymbol == 'R') {
		generator.continueFromLast(firstFormant, fourthFormant)
	}
	if previousSymbol == 'M' && generator.has(generator.BeforePrevious, 2, vowelClass) {
		frames := previous.Value()
		first.Back = int32(frames - frames>>1 + 1)
		if !generator.has(symbol, 2, vowelClass) || generator.Coarticulation == 0 {
			first.Mode, first.Forward = Bridged, first.Frames-1
		} else {
			first.Mode, first.Forward = Bridge, 0
		}
	}
	if symbol == 'X' && generator.has(symbolBefore(current), 2, vowelClass) && isWordMark(symbolAfter(current)) &&
		generator.has(symbolAfter(current.Next), 2, vowelClass) {
		generator.setTargets(firstFormant, 296, 1548)
		first.Forward, segments[secondFormant].Forward = 3, 3
		segments[bypassTrack].Target = 55
	}
	if symbol == 'Y' && generator.has(symbolBefore(current), 2, vowelClass) && generator.has(symbolAfter(current), 2, vowelClass) {
		generator.Ring(firstFormant).Ramp(generator.Tracks[firstFormant].Next, first.Frames+1,
			byte(fromUnits(firstFormant, last[firstFormant])), byte(fromUnits(firstFormant, first.Target)))
		first.Mode = 0
	}
}

// pitchMovements sets the pitch's transitions, and its movements at a glottal stop, at a stressed vowel before the
// pause and in a few contexts (parameters.md §15.9).
func (generator *Generator) pitchMovements() {
	cursor := generator.cursor()
	current, previous := cursor.Scan, cursor.Done
	symbol, previousSymbol, nextSymbol := current.Symbol(), previous.Symbol(), cursor.First.Symbol()
	previousFrames := previous.Value()
	segments := &generator.Segments
	pitch := &segments[pitchTrack]
	pitch.Forward = min(max(pitch.Frames, 7), generator.TransitionLimit)
	if generator.has(symbol, 2, vowelClass) && stressed(current) && generator.has(previousSymbol, 1, consonantClass) {
		pitch.Onset, pitch.BackTarget = generator.Last[pitchTrack], generator.Last[pitchTrack]
	}
	pitch.Back = 0
	if generator.has(symbol, 0, syllabicClass) && current.Flags()&(stressedFlag|glottalFlag) == stressedFlag &&
		previous.Flags()&(stressedFlag|glottalFlag) == stressedFlag && generator.has(previousSymbol, 0, voicedClass) {
		pitch.Back = min(int32(previousFrames), generator.Synthesizer.Lookahead)
	}
	if isVowel(previousSymbol) {
		for track := firstBandwidth; track < nasalZeroTrack; track++ {
			segments[track].BackTarget, segments[track].Back = segments[track].Onset, int32(previousFrames-previousFrames>>2)
		}
	}
	if current.Flags()&glottalFlag != 0 {
		generator.glottalPitch()
	}
	if generator.has(symbol, 2, vowelClass) && stressed(current) {
		generator.stressedPitch()
	}
	if generator.has(symbol, 2, vowelClass) && generator.has(symbol, 3, frontEndingClass) && !stressed(current) && nextSymbol == ' ' {
		pitch.Target = max(pitch.Target, 62)
	}
	if symbol == 'T' && nextSymbol == 'p' && (generator.has(previousSymbol, 2, vowelClass|lateralClass) || generator.has(previousSymbol, 0, nasalClass)) {
		pitch.BackTarget = 40
		if previousFrames < 10 {
			pitch.BackTarget = 20
		}
		pitch.Back = int32(previousFrames>>1 + 1)
		pitch.Mode |= PullBack
	}
	if symbol == ' ' && previousSymbol == 's' {
		frication := &segments[fricationTrack]
		frication.BackTarget, frication.Onset, frication.Back, frication.Forward = 25, 15, 9, 3
	}
}

// glottalPitch writes the pitch's dip at a glottal stop (parameters.md §15.9).
func (generator *Generator) glottalPitch() {
	pitch, track := &generator.Segments[pitchTrack], &generator.Tracks[pitchTrack]
	ring := generator.Ring(pitchTrack)
	if generator.Last[pitchTrack] > 0 {
		third := int32(generator.cursor().Done.Value() / 3)
		ring.PullBack(track.Next, third, third, 41)
	}
	onset := pitch.Onset
	for _, value := range []int32{40, onset - onset/4, onset} {
		ring.Repeat(track.Next, 1, byte(value>>1))
		track.Next++
	}
	pitch.Frames -= 3
	generator.Last[pitchTrack] = pitch.Onset
}

// stressedPitch holds a stressed vowel's pitch before the pause when it falls, after a short rise and before a
// short fall (parameters.md §15.9).
func (generator *Generator) stressedPitch() {
	cursor := generator.cursor()
	current, previous, next := cursor.Scan, cursor.Done, cursor.First
	if symbolOf(next.Next) != ' ' || next.Symbol() != ' ' && symbolAfter(next.Next) != ' ' {
		return
	}
	pitch, track := &generator.Segments[pitchTrack], &generator.Tracks[pitchTrack]
	target := pitch.Target
	if target >= pitch.Onset {
		return
	}
	glottal := current.Flags()&glottalFlag != 0
	turns := current.Value() > 10 && (generator.has(previous.Symbol(), 0, voicedClass) || previous.Symbol() == ' ') && !glottal
	if turns {
		frames := pitch.Frames
		pitch.Frames, pitch.Forward, pitch.Back, pitch.Target = 3, 3, 0, pitch.Onset+4
		generator.RenderSegment(pitchTrack)
		track.Next += 3
		pitch.Frames, pitch.Forward, pitch.Back = frames-3, frames-3, 0
		pitch.Onset, pitch.Target = pitch.Target, target
	}
	held := int32(2)
	if glottal {
		pitch.Onset += min(pitch.Onset/4, 10)
		pitch.BackTarget += pitch.BackTarget / 4
		held = pitch.Frames / 3
	}
	generator.Ring(pitchTrack).Repeat(track.Next, held, byte(pitch.Onset>>1))
	track.Next += held
	pitch.Frames -= held
	generator.Last[pitchTrack] = pitch.Onset
	if turns {
		frames, target := pitch.Frames, pitch.Target
		pitch.Frames, pitch.Forward, pitch.Back, pitch.Target = 3, 3, 0, pitch.Onset-4
		generator.RenderSegment(pitchTrack)
		track.Next += 3
		generator.Last[pitchTrack] = pitch.Target
		pitch.Frames, pitch.Forward = frames-3, frames-3
		pitch.Onset, pitch.Target = pitch.Target, target
	}
}
