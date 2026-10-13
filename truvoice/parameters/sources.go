package parameters

import "strings"

// The class only action 14 tests (parameters.md §16.1): of class byte 5.
const fortisClass = 0x40

// SourceBoundaries is action 14: it sets how the sources' amplitudes, the voice's quality and the pitch go from the
// previous phoneme into the current one, and into a pause (parameters.md §16).
func (generator *Generator) SourceBoundaries() {
	generator.sourceOnsets()
	generator.voicingAfterVoiced()
	generator.voicingOfVoiced()
	generator.sourcesByPhoneme()
	generator.fricativeBeforePause()
	for _, track := range []int{pitchTrack, voicingTrack} {
		segment := &generator.Segments[track]
		segment.Back = min(segment.Back, generator.Tracks[track].Next-generator.Tracks[track].Earliest)
	}
	if generator.cursor().Scan.Symbol() == ' ' {
		generator.sourcesAtPause()
	}
	generator.sourceContexts()
}

// sourceOnsets starts the tracks other than the formants, the bandwidths, FNZ and the pitch halfway between their
// last values and their targets, the amplitudes no further than a limit below either (parameters.md §16.2).
func (generator *Generator) sourceOnsets() {
	cursor := generator.cursor()
	symbol, previousSymbol := cursor.Scan.Symbol(), cursor.Done.Symbol()
	for track := range generator.Segments {
		if track >= firstFormant && track <= pitchTrack {
			continue
		}
		segment := &generator.Segments[track]
		segment.Onset = (segment.Target + generator.Last[track]) / 2
		limit := int32(9)
		if symbol == ' ' {
			limit = 15
		}
		if track == aspirationTrack {
			limit = 0
		}
		if !generator.has(previousSymbol, 0, voicedClass) {
			limit = 10
		}
		if track > aspirationTrack {
			limit = 0
		}
		if track == voicingTrack && generator.has(symbol, 0, nasalClass) && previousSymbol == ' ' {
			limit = 20
		}
		if track < firstFormant {
			segment.Onset = max(segment.Onset, segment.Target-limit, generator.Last[track]-limit)
		}
		segment.BackTarget = segment.Onset
	}
}

// voicingAfterVoiced sets the voicing's onset and its pull on the previous phoneme after a voiced phoneme other
// than R (parameters.md §16.3).
func (generator *Generator) voicingAfterVoiced() {
	cursor := generator.cursor()
	previous := cursor.Done
	symbol, previousSymbol, nextSymbol := cursor.Scan.Symbol(), previous.Symbol(), cursor.First.Symbol()
	if !generator.has(previousSymbol, 0, voicedClass) || previousSymbol == 'R' {
		return
	}
	segments := &generator.Segments
	voicing := &segments[voicingTrack]
	if voicing.Target == 0 {
		voicing.Onset, voicing.BackTarget = 40, 40
	} else {
		voicing.BackTarget = voicing.Target
	}
	if (!generator.has(symbol, 0, voicedClass) || !generator.has(symbol, 0, sonorantClass)) && (symbol != ' ' || previousSymbol != 'z') {
		voicing.BackTarget -= 6
		if symbol != 'V' || !generator.has(nextSymbol, 2, vowelClass) {
			voicing.Onset = voicing.BackTarget
		}
	}
	if isVowel(previousSymbol) {
		voicing.Back = max(int32(previous.Value()/3), 2)
	} else {
		voicing.Back = int32(previous.Value() >> 2)
	}
	if !generator.has(symbol, 0, sonorantClass) || !generator.has(previousSymbol, 0, stopClass) || !generator.has(symbol, 2, vowelClass) {
		return
	}
	voicing.Onset = 60
	if (previousSymbol == 'G' || previousSymbol == 'D') && symbol == 'E' || symbol == 'u' || symbol == 'b' ||
		previousSymbol == 'G' && !generator.has(symbol, 4, highClass) && !generator.has(symbol, 2, frontClass) && symbol != 'p' && symbol != 'o' {
		voicing.Onset = 56
	}
	if previousSymbol == 'B' && generator.has(symbol, 4, highClass) && !generator.has(symbol, 2, frontClass) {
		for track := firstBandwidth; track <= firstBandwidth+1; track++ {
			segments[track].BackTarget, segments[track].Onset = 75, 75
		}
	}
}

// voicingOfVoiced sets the voicing's mode and transitions for a voiced phoneme (parameters.md §16.4).
func (generator *Generator) voicingOfVoiced() {
	cursor := generator.cursor()
	current, previous := cursor.Scan, cursor.Done
	symbol, previousSymbol, nextSymbol := current.Symbol(), previous.Symbol(), cursor.First.Symbol()
	if !generator.has(symbol, 0, voicedClass) {
		return
	}
	voicing := &generator.Segments[voicingTrack]
	if !generator.has(symbol, 0, stopClass) && !generator.has(symbol, 3, silenceClass) && !generator.has(nextSymbol, 2, closureClass) && nextSymbol != ' ' {
		if (!generator.has(previousSymbol, 0, stopClass) || generator.has(previousSymbol, 3, silenceClass) || generator.has(symbol, 2, closureClass) ||
			symbol == ' ') && previousSymbol != ' ' {
			voicing.Mode = PullBack | Shaped
		} else if symbol != 'L' || previousSymbol != 'P' {
			voicing.Mode = Shaped
		}
		if stressed(current) {
			voicing.Forward = int32(current.Value() >> 2)
		} else {
			voicing.Forward = int32(current.Value() >> 1)
		}
		voicing.Back = int32(previous.Value() >> 2)
	}
	if previousSymbol == 'B' {
		if generator.Last[voicingTrack] == 0 {
			voicing.Mode = Flat
		} else {
			voicing.Mode |= PullForward
			voicing.Forward, voicing.Onset = int32(current.Value()>>1), generator.Last[voicingTrack]
		}
	}
	if generator.has(symbol, 0, fricativeClass) && generator.has(nextSymbol, 0, fricativeClass) && !generator.has(nextSymbol, 0, voicedClass) {
		voicing.Target, voicing.Mode, voicing.Forward = 0, Shaped, int32(current.Value()/3)
	}
	if strings.IndexByte("DGJ", previousSymbol) >= 0 && generator.BeforePrevious == ' ' ||
		previousSymbol == 'G' && generator.has(generator.BeforePrevious, 1, consonantClass) && !generator.has(generator.BeforePrevious, 0, voicedClass) {
		voicing.Mode = Shaped
	}
}

// sourcesByPhoneme sets the sources of a few phonemes by the phonemes around them (parameters.md §16.5).
func (generator *Generator) sourcesByPhoneme() {
	cursor := generator.cursor()
	current, previous := cursor.Scan, cursor.Done
	symbol, previousSymbol, nextSymbol := current.Symbol(), previous.Symbol(), cursor.First.Symbol()
	segments, last := &generator.Segments, &generator.Last
	voicing, frication := &segments[voicingTrack], &segments[fricationTrack]
	switch symbol {
	case 'D':
		if nextSymbol == 'o' {
			voicing.Onset = 0
		}
		if !stressed(current) && generator.has(previousSymbol, 0, sonorantClass) && generator.has(nextSymbol, 0, sonorantClass) {
			segments[bypassTrack].BackTarget, segments[bypassTrack].Onset = 0, 0
		}
	case 'E':
		if previousSymbol == 'R' {
			voicing.Onset = 57
		}
	case 'F':
		if previousSymbol == ' ' {
			segments[bypassTrack].BackTarget, segments[bypassTrack].Onset = 15, 15
		}
	case 'H':
		if place := generator.place(nextSymbol); place == 0 || place == 1 {
			segments[aspirationTrack].Onset, segments[aspirationTrack].Forward = 10, 5
		}
	case 'K', 'P', 'T':
		if !isVowel(previousSymbol) {
			voicing.Onset, voicing.Mode = 0, Flat
		}
	case 'L':
		if previousSymbol == 'F' {
			voicing.Target, voicing.Mode, voicing.BackTarget = 57, PullBack|Flat, 57
			generator.Tracks[voicingTrack].Next -= 2
			voicing.Frames += 2
		}
	case 'R':
		if previousSymbol == 'z' {
			voicing.Onset, voicing.BackTarget = last[voicingTrack], last[voicingTrack]
		}
		if generator.has(previousSymbol, 0, fricativeClass) && !generator.has(previousSymbol, 0, voicedClass) {
			voicing.Back += 4
			voicing.BackTarget += 10
		}
		if previousSymbol == 'X' {
			voicing.BackTarget, voicing.Onset, frication.Onset = 57, 57, 0
		}
	case 'S':
		if previousSymbol == ' ' {
			frication.Mode, frication.Onset = Shaped, 16
		}
		frication.Back = int32(previous.Value() >> 2)
	case 'W':
		if nextSymbol == 'E' && previousSymbol == ' ' {
			segments[thirdFormant].Onset, segments[firstBandwidth+2].Onset = 2150, 250
		}
	case 'X':
		if nextSymbol == 'R' {
			frication.BackTarget, frication.Onset = 40, 40
		}
		if generator.has(previousSymbol, 2, vowelClass) {
			segments[firstFormant].Onset, segments[firstFormant].Mode = last[firstFormant], Shaped
		}
		if nextSymbol == 'R' {
			generator.setTargets(secondGainTrack, 0, 0, 0, 72, 80)
		}
	case 'Z':
		if generator.has(previousSymbol, 0, voicedClass) {
			voicing.Mode, voicing.Forward = PullBack|Shaped, 6
			if generator.has(previousSymbol, 2, vowelClass) {
				voicing.BackTarget, voicing.Onset = 52, 52
				if nextSymbol != ' ' {
					voicing.Back = int32(previous.Value() >> 2)
				}
			}
		}
		if nextSymbol != ' ' {
			frication.BackTarget, frication.Onset = 10, 10
			voicing.BackTarget, voicing.Onset = 42, 42
			if !generator.has(previousSymbol, 2, vowelClass) && !generator.has(nextSymbol, 2, vowelClass) {
				segments[pitchTrack].Back, segments[pitchTrack].Forward = 4, 4
			}
		}
	case 'o':
		if previousSymbol == 'B' {
			segments[secondFormant].Onset += 100
		}
	case 'p':
		switch previousSymbol {
		case 'D':
			generator.setTargets(firstVoiceTrack, 10, 15)
		case 'P', 'T', 'K':
			frication.Onset, frication.Forward, frication.Target = last[fricationTrack], frication.Frames, 42
		case 'G', 'B':
			voicing.Target, voicing.Onset, voicing.Mode = 50, 50, Shaped
		}
	case 'q':
		voicing.Mode, voicing.Target = Flat, 0
	case 's':
		if previousSymbol == 'C' && generator.has(nextSymbol, 2, vowelClass) || nextSymbol == '@' {
			for track := thirdGainTrack; track <= spareTrack; track++ {
				segments[track].BackTarget, segments[track].Onset = last[track], last[track]
			}
			if nextSymbol == ' ' {
				frication.Onset, frication.BackTarget, frication.Forward, frication.Back, frication.Target, frication.Mode = 35, 25, 6, 3, 66, PullBack|Shaped
			}
		}
	case 't':
		voicing.Mode = Flat
	case 'x':
		if previousSymbol == 'q' || !generator.has(previousSymbol, 0, voicedClass) &&
			(generator.has(previousSymbol, 0, fricativeClass) || generator.has(previousSymbol, 2, closureClass)) {
			voicing.Target, voicing.Mode = 0, Flat
		}
	case 'y':
		if previousSymbol == 'P' {
			for track := secondGainTrack; track <= spareTrack; track++ {
				segments[track].Target = last[track]
			}
		}
	case '~':
		if generator.has(previousSymbol, 2, frontClass) {
			segments[secondFormant].BackTarget = 2100
		}
	}
	if generator.has(symbol, 2, vowelClass) && current.Flags()&glottalFlag != 0 {
		voicing.Onset, voicing.Forward, voicing.Mode = 42, 4, Shaped
	}
	if previousSymbol == 'Z' {
		voice := &segments[firstVoiceTrack]
		voice.Onset, voice.Forward = last[firstVoiceTrack], int32(min(current.Value(), 5))
	}
}

// fricativeBeforePause ends the voicing of a voiced fricative before the pause, and writes that of Z and V
// directly (parameters.md §16.6).
func (generator *Generator) fricativeBeforePause() {
	cursor := generator.cursor()
	current, previous := cursor.Scan, cursor.Done
	symbol, previousSymbol, nextSymbol := current.Symbol(), previous.Symbol(), cursor.First.Symbol()
	if nextSymbol != ' ' {
		return
	}
	voicing := &generator.Segments[voicingTrack]
	afterNext := symbolAfter(cursor.First)
	if generator.has(symbol, 0, voicedClass) && generator.has(symbol, 0, fricativeClass) &&
		(!generator.has(afterNext, 3, approximantClass) && afterNext != 'M' && afterNext != 'N' || generator.has(nextSymbol, 0, markClass)) {
		switch {
		case symbol == 'Z' && strings.IndexByte("BDG", previousSymbol) >= 0:
			voicing.Target, voicing.Mode = 0, Flat
		case !generator.has(previousSymbol, 0, nasalClass|fricativeClass) && previousSymbol != 'j' && previousSymbol != 'l' && current.Value() > 6:
			frames := voicing.Frames
			voicing.Onset, voicing.Target, voicing.Frames, voicing.Mode = 60, 50, 4, Shaped
			generator.RenderSegment(voicingTrack)
			generator.Last[voicingTrack] = 50
			voicing.Frames = frames - 4
			generator.Tracks[voicingTrack].Next += 4
			voicing.Mode = Flat
		default:
			voicing.BackTarget, voicing.Back, voicing.Mode = 50, 4, PullBack|Flat
		}
		voicing.Target = 0
	}
	if symbol == 'Z' || symbol == 'V' {
		next := generator.Tracks[voicingTrack].Next
		half := int32(current.Value() >> 1)
		rest := int32(current.Value()) - half
		ring := generator.Ring(voicingTrack)
		ring.Fill(next, half, half, byte(generator.Last[voicingTrack]), 46)
		ring.Fill(next+half, rest, rest, 0, 0)
		voicing.Mode = 0
	}
}

// sourcesAtPause lets the voicing and the pitch fall into the pause and ends a few phonemes' sources there
// (parameters.md §16.8).
func (generator *Generator) sourcesAtPause() {
	cursor := generator.cursor()
	current, previous := cursor.Scan, cursor.Done
	previousSymbol, nextSymbol := previous.Symbol(), cursor.First.Symbol()
	previousFrames := previous.Value()
	beforeSymbol := generator.beforePreviousSymbol()
	segments, last := &generator.Segments, &generator.Last
	voicing, frication, pitch := &segments[voicingTrack], &segments[fricationTrack], &segments[pitchTrack]
	if generator.has(previousSymbol, 0, voicedClass) && previousSymbol != 'Z' && symbolAfter(current) != '&' {
		pitch.BackTarget, pitch.Back = 58, int32(previousFrames-previousFrames/3)
		pitch.Mode |= PullBack
		voicing.BackTarget, voicing.Back = 48, int32(previousFrames-previousFrames/3)
		voicing.Mode |= PullBack
		if nextSymbol == ' ' && (generator.has(previousSymbol, 0, nasalClass) || generator.has(previousSymbol, 3, approximantClass) ||
			generator.has(previousSymbol, 2, vowelClass) && previousSymbol != 'p' || previousSymbol == 'p' && generator.has(beforeSymbol, 3, approximantClass)) {
			pitch.BackTarget = 155
			voicing.Mode &= PullBack | Flat
			if generator.has(previousSymbol, 2, vowelClass) {
				voicing.BackTarget, voicing.Onset = 38, 38
			} else {
				voicing.BackTarget, voicing.Onset = 45, 45
			}
			voicing.Target = 0
			generator.Tracks[voicingTrack].Next += 3
			voicing.Back += 3
			voicing.Frames -= 3
			voicing.Forward = 4
			if previousSymbol == 'N' {
				if isVowel(beforeSymbol) {
					voicing.BackTarget, voicing.Back = 42, 8
				}
				first := &segments[firstFormant]
				first.Onset, first.Target, first.Mode = last[firstFormant], last[firstFormant]-10, Shaped
			}
			if previousSymbol == 'p' {
				pitch.Back = int32(previousFrames) + 2
				frication.Mode = Flat
			} else {
				pitch.Back = max(int32(previousFrames/6), 2)
			}
		}
	}
	switch previousSymbol {
	case 'V':
		voicing.Forward, frication.Forward = 2, 2
		voicing.Back, voicing.BackTarget, voicing.Onset = int32(previousFrames)-4, 0, 5
	case 'Z':
		voicing.Forward, frication.Forward = 2, 2
		frication.Onset, frication.BackTarget, frication.Back = 20, 20, int32(previousFrames/3)
		segments[aspirationTrack].Back = int32(previousFrames / 3)
		voicing.Onset, voicing.BackTarget = 0, 0
	case 'n':
		if nextSymbol == ' ' {
			second := &segments[secondFormant]
			second.BackTarget, second.Back = 892, 9
			second.Mode |= PullBack
			voicing.BackTarget, voicing.Back = last[voicingTrack], 8
			voicing.Mode |= PullBack
		}
	case 'p':
		frication.Mode, frication.Onset, frication.Target = Flat, 0, 0
		if strings.IndexByte("PTK", beforeSymbol) >= 0 {
			voicing.Mode, voicing.BackTarget, voicing.Target = Flat, 0, 0
		}
	case 'z':
		voicing.Forward, frication.Forward = 2, 2
		frication.Back = int32(previousFrames >> 1)
	}
	if last[voicingTrack] == 0 {
		voicing.Mode = Flat
	}
}

// sourceContexts sets the sources and formants of a few more contexts (parameters.md §16.9).
func (generator *Generator) sourceContexts() {
	cursor := generator.cursor()
	current, previous := cursor.Scan, cursor.Done
	symbol, previousSymbol, nextSymbol := current.Symbol(), previous.Symbol(), cursor.First.Symbol()
	beforeSymbol := generator.beforePreviousSymbol()
	segments := &generator.Segments
	voicing, frication, pitch := &segments[voicingTrack], &segments[fricationTrack], &segments[pitchTrack]
	if (previousSymbol == 'l' || previousSymbol == 'j') && !generator.has(symbol, 2, vowelClass) && symbol != 'R' && symbol != 'Y' {
		generator.shapeFromLast()
	}
	if generator.has(previousSymbol, 0, stopClass) && (beforeSymbol == 'M' || beforeSymbol == 'N') {
		nasalZero := &segments[nasalZeroTrack]
		nasalZero.Back, nasalZero.Mode, nasalZero.BackTarget = 2, PullBack|Flat, 250
	}
	if symbol == 'T' || symbol == 'q' {
		if previousSymbol == 'N' && !stressed(previous.Previous) && generator.has(beforeSymbol, 2, vowelClass) && isWordMark(symbolAfter(current)) &&
			(generator.has(nextSymbol, 0, sonorantClass) || generator.has(nextSymbol, 2, vowelClass) && stressed(cursor.First.Next)) {
			voicing.BackTarget, voicing.Back = 40, int32(previous.Value())
			voicing.Mode |= PullBack
		}
		if symbol == 'q' && (!generator.has(nextSymbol, 0, voicedClass) || nextSymbol == 'x') && generator.has(previousSymbol, 2, vowelClass) {
			pitch.Mode |= PullBack
			pitch.BackTarget = 40
			if generator.Last[pitchTrack] > 99 {
				pitch.BackTarget = 20
			}
			if generator.Last[pitchTrack] > 149 {
				pitch.BackTarget -= 20
			}
			pitch.Back = 4
		}
	}
	if previousSymbol == 's' {
		frication.Mode = Flat
	}
	if symbol == 'V' && generator.has(previousSymbol, 5, fortisClass) && generator.has(previousSymbol, 2, closureClass) && generator.has(nextSymbol, 2, vowelClass) {
		voicing.Forward = 4
		frication.Mode, frication.Forward = Shaped, 4
	}
	if symbol == 'R' && previousSymbol == 'B' && generator.has(nextSymbol, 2, vowelClass) {
		voicing.Mode = Flat
	}
	if symbol == 'Z' && generator.has(previousSymbol, 0, nasalClass) && generator.has(nextSymbol, 2, vowelClass) {
		voicing.Target, voicing.Mode = 53, Flat
		frication.Target, frication.Forward = 64, 2
		segments[aspirationTrack].Target, segments[thirdGainTrack].Target = 0, 0
		generator.setTargets(fourthGainTrack, 74, 74, 74)
		generator.setTargets(firstVoiceTrack, 10, 14)
		generator.setTargets(secondFormant, 1844, 2496)
		segments[firstBandwidth].Target = 70
	}
	if symbol == 'F' && previousSymbol == 'Z' && symbolBefore(previous) == 'E' && symbolAfter(current) == 'k' {
		frication.Target = 55
	}
}
