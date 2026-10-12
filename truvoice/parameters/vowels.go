package parameters

import (
	"strings"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
)

// The class of the reduced vowels (parameters.md §11.1), the node byte some word marks have set, and the
// limits the vowel database's writers use.
const (
	reducedClass       = 0x10
	markByteOffset     = 0x0c
	highFirstFormant   = 650
	bandwidthLimit     = 200
	shortVowelFrames   = 8
	smoothedDifference = 200
	lowestFirstFormant = 250
)

// fromDatabase is the start of action 4. When the vowel database has a record for the current vowel in its
// context, the vowel's formant tracks follow it and fromDatabase reports true (parameters.md §11.2).
func (generator *Generator) fromDatabase() bool {
	cursor := generator.cursor()
	generator.Coarticulation = 0
	vowel := generator.vowels.Index(cursor.Scan.Symbol())
	if vowel < 0 {
		return false
	}
	if generator.has(cursor.Done.Symbol(), 2, vowelClass) {
		generator.VoiceOnset = noVoiceOnset
	}
	if !generator.vowelFromDatabase(vowel) {
		return false
	}
	generator.Coarticulation = 1
	generator.VoiceOnset = noVoiceOnset
	return true
}

func (generator *Generator) context(symbol byte) int32 {
	if symbol >= datapack.SymbolCount {
		return -1
	}
	return generator.vowels.Contexts[symbol]
}

// endingContext is the left context of a phoneme in class vowel, startingContext the right context of the
// current vowel before one (parameters.md §11.3).
func endingContext(symbol byte) byte {
	switch {
	case strings.IndexByte("AEIaeiy|", symbol) >= 0:
		return 'Y'
	case strings.IndexByte("OUbuf", symbol) >= 0:
		return 'W'
	case strings.IndexByte("34kcgr5", symbol) >= 0:
		return 'R'
	}
	return ' '
}

func startingContext(symbol byte) byte {
	switch {
	case strings.IndexByte("3krg4c5", symbol) >= 0:
		return 'R'
	case strings.IndexByte("bfOU", symbol) >= 0:
		return 'W'
	case strings.IndexByte("@vow", symbol) >= 0:
		return ' '
	}
	return 'Y'
}

// variant is the symbol the database tries in place of a context it has no record for. z has J only as the
// right context of the current vowel.
func variant(symbol byte, ofVowel bool) byte {
	switch symbol {
	case 'D':
		return 't'
	case 't':
		return 'D'
	case 'L':
		return 'l'
	case 'l':
		return 'j'
	case 'j':
		return 'L'
	case 'X':
		return 'x'
	case 'x':
		return 'X'
	case 'z':
		if ofVowel {
			return 'J'
		}
	}
	return symbol
}

func isLateral(symbol byte) bool {
	return symbol == 'j' || symbol == 'l' || symbol == 'L'
}

func lateralVariant(symbol byte) byte {
	switch symbol {
	case 'L':
		return 'j'
	case 'l':
		return 'L'
	}
	return 'l'
}

// databaseContexts returns the symbols the database looks the current vowel up by on its left and right, and
// the vowel it looks up (parameters.md §11.3).
func (generator *Generator) databaseContexts() (left, right, symbol byte) {
	cursor := generator.cursor()
	current, after, before := cursor.Scan, cursor.Scan.Next, cursor.Scan.Previous
	symbol, left, right = current.Symbol(), cursor.Done.Symbol(), cursor.First.Symbol()
	afterSymbol, beyondAfter, beforeSymbol := after.Symbol(), symbolAfter(after), before.Symbol()
	if generator.has(left, 2, vowelClass) {
		left = endingContext(left)
	} else if left == 'n' {
		left = 'N'
	}
	if generator.has(right, 2, vowelClass) {
		right = startingContext(symbol)
	}
	if generator.has(symbol, 3, frontEndingClass) && symbol != '5' && symbol != 'E' && isWordMark(afterSymbol) &&
		strings.IndexByte("RLWYdHt", beyondAfter) < 0 && (symbol != 'I' || stressed(current)) {
		right = ' '
	} else if right == 'n' {
		right = 'N'
	}
	if isWordMark(afterSymbol) && beyondAfter == 'N' && symbol != '@' && symbol != '|' && symbol != '5' && stressed(current) {
		right = ' '
	}
	if symbol == 'E' && left == 'M' && right == ' ' && beyondAfter == 'M' {
		right = 'M'
	}
	if isWordMark(afterSymbol) && isMerged(after.Next) {
		right = beyondAfter
	}
	if isWordMark(beforeSymbol) && generator.has(left, 0, fricativeClass) && left != 'Z' && left != 'S' && stressed(current) {
		if generator.has(symbol, 3, frontEndingClass) && symbol != 'I' {
			left = ' '
		} else if generator.has(afterSymbol, 0, nasalClass) && symbol != 'e' && left != 'X' && (left != 's' || symbol != '3') &&
			(left != 'V' || symbol != 'a') {
			left = ' '
		}
	}
	if symbol == 'v' && afterSymbol == 'N' && beforeSymbol == '&' {
		left = ' '
	}
	if isWordMark(beforeSymbol) && before.Data[markByteOffset] == 1 {
		left = ' '
	}
	if isWordMark(afterSymbol) && after.Data[markByteOffset] == 1 {
		right = ' '
	}
	if generator.has(symbol, 1, reducedClass) && !stressed(current) && afterSymbol == 'q' && generator.AfterNext == ' ' {
		right = 'T'
	}
	if symbol == 'E' && left == 'Z' && right == 'Y' && afterSymbol == 'Y' && beyondAfter == '@' {
		symbol = 'i'
	}
	return left, right, symbol
}

// findRecord looks a vowel up by its contexts, then by their variants (parameters.md §11.4): the current vowel,
// or the next one for a consonant (parameters.md §14.1).
func (generator *Generator) findRecord(entry *datapack.VowelEntry, left, right byte, ofVowel bool) *datapack.VowelRecord {
	leftContext, rightContext := generator.context(left), generator.context(right)
	record := entry.Find(leftContext, rightContext)
	tryLeft, tryRight := left, right
	for pass := 0; pass < 2 && record == nil; pass++ {
		tryRight = variant(tryRight, ofVowel)
		if tryRight != right && generator.context(tryRight) >= 0 {
			rightContext = generator.context(tryRight)
			record = entry.Find(leftContext, rightContext)
		}
		if record == nil {
			if isLateral(tryRight) {
				tryRight = lateralVariant(right)
				rightContext = generator.context(tryRight)
				record = entry.Find(leftContext, rightContext)
			}
			if record == nil && tryLeft == left {
				tryRight, rightContext = right, generator.context(right)
				tryLeft = variant(tryLeft, false)
				if tryLeft != left && generator.context(tryLeft) >= 0 {
					leftContext = generator.context(tryLeft)
					record = entry.Find(leftContext, rightContext)
				}
			}
		}
		if record == nil && isLateral(left) {
			tryLeft = lateralVariant(left)
			leftContext = generator.context(tryLeft)
			record = entry.Find(leftContext, rightContext)
		}
	}
	return record
}

// vowelFromDatabase writes the current vowel's formant tracks from its record in the vowel database, and
// reports whether there was one (parameters.md §11).
func (generator *Generator) vowelFromDatabase(vowel int) bool {
	cursor := generator.cursor()
	left, right, symbol := generator.databaseContexts()
	if symbol != cursor.Scan.Symbol() {
		vowel = generator.vowels.Index(symbol)
	}
	generator.VowelFrames = int32(cursor.Scan.Value())
	if generator.VoiceOnset != 0 && generator.VoiceOnset != noVoiceOnset {
		generator.VowelFrames -= generator.VoiceOnset
	}
	generator.VowelFrames = max(generator.VowelFrames, 1)
	if generator.context(left) < 0 || generator.context(right) < 0 {
		return false
	}
	entry := &generator.vowels.Vowels[vowel]
	record := generator.findRecord(entry, left, right, true)
	if record == nil {
		return false
	}
	if alternative := record.Alternative; alternative != nil {
		neighbour := symbolBefore(cursor.Done)
		if alternative.AfterNext {
			neighbour = symbolAfter(cursor.First)
		}
		if neighbour == alternative.Symbol {
			record = &alternative.Record
		}
	}
	afterVowel := isVowel(cursor.Done.Symbol())
	length := generator.VowelFrames - 1
	if datapack.Reduced(symbol) {
		for formant := range datapack.FormantCount {
			track := firstFormant + formant
			bandwidth := byte(0)
			if formant < 3 {
				bandwidth = record.BandwidthEnds[formant]
			}
			timing := ((record.Timing[formant]+3)*length + 10) * 5 / 100
			generator.writeReducedTrajectory(track, record.Start[formant], record.Middle[formant], record.End[formant], timing, bandwidth)
			generator.trajectoryBoundary(track, record.Start[formant], record.End[formant], bandwidth, afterVowel)
		}
		return true
	}
	var bandwidths [3]byte
	for formant := range datapack.FormantCount {
		track := firstFormant + formant
		trajectory := &entry.Trajectories[formant][record.Trajectories[formant]]
		if formant < 3 {
			for part := range bandwidths {
				bandwidths[part] = byte(min(record.Bandwidths[part][formant], bandwidthLimit) / 2)
			}
		}
		timing := trajectory.Timing
		times := [4]int32{((timing[0]+10)*length + 50) / 100, ((timing[1]+10)*length*2 + 50) / 100,
			((timing[2]+25)*length*2 + 50) / 100, ((timing[3]*2+65)*length + 50) / 100}
		start, end := record.Start[formant], record.End[formant]
		generator.writeTrajectory(track, trajectory.Points, times, bandwidths, start, end, afterVowel)
		generator.trajectoryBoundary(track, start, end, bandwidths[2], afterVowel)
		frames := generator.Segments[track].Frames
		ring, position := generator.Ring(track), generator.Tracks[track].Next+1
		switch {
		case frames < 5:
			ring.Ramp(position, frames-1, start, end)
		case frames < 8:
			points := trajectory.Points
			mean := byte((int32(points[0]) + int32(points[1]) + int32(points[2]) + int32(points[3])) / 4)
			half := (frames - 1) / 2
			ring.Ramp(position, half, start, mean)
			ring.Ramp(position+half, frames-half-1, mean, end)
		}
	}
	return true
}

// afterFrontY reports whether the previous phoneme is Y after a front-ending phoneme.
func (generator *Generator) afterFrontY() bool {
	previous := generator.cursor().Done
	return previous.Symbol() == 'Y' && generator.has(symbolBefore(previous), 3, frontEndingClass)
}

func (generator *Generator) voiceOnsetCounts() bool {
	return generator.VoiceOnset > 0 && generator.VoiceOnset != noVoiceOnset
}

// writeTrajectory writes a formant track and its bandwidth from a record of a vowel other than @ and |
// (parameters.md §11.5).
func (generator *Generator) writeTrajectory(track int, points [4]byte, times [4]int32, bandwidths [3]byte, start, end byte, afterVowel bool) {
	cursor := generator.cursor()
	previous, current := cursor.Done, cursor.Scan
	previousSymbol := previous.Symbol()
	frames := generator.VowelFrames
	last := generator.Last[track]
	lastByte := byte(fromUnits(track, last))
	startUnits, endUnits := Units(track, start), Units(track, end)
	var units [4]int32
	for index, point := range points {
		units[index] = Units(track, point)
	}
	if track == firstFormant && (startUnits > highFirstFormant || max(units[0], units[1], units[2], units[3]) > highFirstFormant ||
		endUnits > highFirstFormant) {
		generator.Segments[voicingTrack].Target -= 3
	}
	held := false
	if (afterVowel || previousSymbol == 'u') && (!stressed(previous) && stressed(current) ||
		stressed(previous) == stressed(current) && previous.Value() < current.Value()) {
		startUnits, start, held = last, lastByte, true
	}
	ring := generator.Ring(track)
	position := generator.Tracks[track].Next
	if generator.voiceOnsetCounts() {
		onset := generator.VoiceOnset
		from := lastByte
		if track == firstFormant && !(isVoicelessStop(previousSymbol) && lastByte != start) {
			from = start
		}
		ring.Fill(position, min(onset, 20), onset, from, start)
		position += onset
		if onset == 1 {
			generator.advanceTrack(track, onset)
		}
	}
	approximant := generator.has(previousSymbol, 3, approximantClass)
	if frames < shortVowelFrames && !approximant {
		halfway := func(from, to int32) byte {
			return byte(fromUnits(track, from+(to-from)/2))
		}
		points = [4]byte{halfway(startUnits, units[0]), halfway(units[0], units[1]), halfway(units[1], units[2]), halfway(units[2], units[3])}
		end = halfway(units[3], endUnits)
	}
	times[3] = max(times[3], frames-3)
	for index := 2; index >= 0; index-- {
		if times[index+1] < times[index] {
			times[index] = times[index+1] - 1
		}
	}
	afterFrontY := generator.afterFrontY()
	if track == firstFormant && afterFrontY {
		start = byte(fromUnits(track, units[0]+(generator.Last[firstFormant]-units[0])/2))
	}
	if held && max(last-units[0], units[0]-last) > smoothedDifference {
		start = byte(fromUnits(track, units[0]+(last-units[0])/2))
	}
	ring.Fill(position, 1, 1, start, start)
	if afterFrontY {
		shape := int32(previous.Value()) + 1
		ring.PullBack(position, shape, shape, start)
	}
	position++
	if times[0] > 0 && !approximant {
		count := min(times[0], 20)
		from := start
		if count == 1 {
			from = points[0]
		}
		ring.Fill(position, count, count, from, points[0])
		position += count
	}
	if span := times[1] - times[0]; span > 0 {
		from := points[0]
		if approximant {
			from, span = start, times[1]
		}
		ring.Ramp(position, span, from, points[1])
		position += span
		if afterFrontY {
			generator.Tracks[track].Next += times[0]
			segment := &generator.Segments[track]
			segment.Back = times[0] + 1
			if start != lastByte {
				segment.Back = int32(previous.Value()) + 2 + times[0]
			}
			segment.BackTarget = units[0]
			segment.Frames -= times[0]
		}
	} else if approximant {
		ring.Ramp(position, times[1], start, points[1])
		position += times[1]
	}
	for index := 1; index < 3; index++ {
		if span := times[index+1] - times[index]; span > 0 {
			ring.Ramp(position, span, points[index], points[index+1])
			position += span
		}
	}
	if span := frames - times[3] - 1; span > 0 {
		ring.Ramp(position, min(span, 20), points[3], end)
	}
	if track == fourthFormant {
		return
	}
	bandwidth := track + 4
	bandwidthRing, lastBandwidth := generator.Ring(bandwidth), byte(generator.Last[bandwidth]>>1)
	position = generator.Tracks[bandwidth].Next
	first, second := (frames+1)/2, (frames+3)/4
	third := frames - first - second
	if generator.voiceOnsetCounts() {
		onset := generator.VoiceOnset
		switch count := min(onset, 20); {
		case count < 4 && frames > 4 && frames < shortVowelFrames:
			bandwidthRing.Fill(position, 3, 3, lastBandwidth, bandwidths[0])
			position += 3
			first += 3 - onset
		case frames < shortVowelFrames:
			bandwidthRing.Fill(position, count, onset, lastBandwidth, bandwidths[0])
			position += onset
		default:
			bandwidthRing.Fill(position, count+3, onset+3, lastBandwidth, bandwidths[0])
			position += onset + 3
		}
		generator.advanceTrack(bandwidth, onset)
	} else if frames >= shortVowelFrames {
		bandwidthRing.Fill(position, 3, 3, lastBandwidth, bandwidths[0])
		position += 3
	}
	if frames >= shortVowelFrames {
		first -= 3
	}
	bandwidthRing.Fill(position, min(first, 20), first, bandwidths[0], bandwidths[1])
	position += first
	if second > 0 {
		bandwidthRing.Fill(position, min(second, 20), second, bandwidths[1], bandwidths[2])
		position += second
	}
	if third > 0 {
		bandwidthRing.Fill(position, min(third, 20), third, bandwidths[2], bandwidths[2])
	}
	generator.advanceTrack(bandwidth, frames)
}

// writeReducedTrajectory writes a formant track and its bandwidth from a record of @ or | (parameters.md
// §11.6).
func (generator *Generator) writeReducedTrajectory(track int, start, middle, end byte, timing int32, bandwidthEnd byte) {
	cursor := generator.cursor()
	frames := generator.VowelFrames
	ring := generator.Ring(track)
	position := generator.Tracks[track].Next
	if generator.voiceOnsetCounts() {
		onset := generator.VoiceOnset
		if track == firstFormant && onset == 1 {
			ring.Fill(position, 1, onset, start, start)
		} else {
			ring.Fill(position, min(onset, 20), onset, byte(fromUnits(track, generator.Last[track])), start)
		}
		position += onset
		if onset == 1 {
			generator.advanceTrack(track, onset)
		}
	}
	if generator.afterFrontY() {
		shape := int32(cursor.Done.Value()) + 1
		ring.PullBack(position, shape, shape, start)
	}
	ring.Fill(position, 1, 1, start, start)
	position++
	timing = max(timing, 1)
	ring.Ramp(position, min(timing, 20), start, middle)
	position += min(timing, 20)
	if rest := min(frames-timing-1, 20); rest > 0 {
		if cursor.Scan.Symbol() == '@' && cursor.First.Symbol() == 'j' {
			ring.Fill(position, rest, rest, middle, middle)
		} else {
			ring.Ramp(position, rest, middle, end)
		}
	}
	if track == firstFormant && max(Units(track, start), Units(track, middle), Units(track, end)) > highFirstFormant {
		generator.Segments[voicingTrack].Target -= 3
	}
	if track == fourthFormant {
		return
	}
	bandwidth := track + 4
	bandwidthRing, lastBandwidth := generator.Ring(bandwidth), byte(generator.Last[bandwidth]>>1)
	if generator.voiceOnsetCounts() {
		onset := generator.VoiceOnset
		bandwidthRing.Fill(generator.Tracks[bandwidth].Next, min(onset, 20), onset, lastBandwidth, lastBandwidth)
		generator.advanceTrack(bandwidth, onset)
	}
	position = generator.Tracks[bandwidth].Next
	first := (frames + 1) / 2
	bandwidthRing.Ramp(position, min(first, 20), lastBandwidth, bandwidthEnd)
	if rest := frames - first; rest > 0 {
		bandwidthRing.Fill(position+first, min(rest, 20), rest, bandwidthEnd, bandwidthEnd)
	}
	generator.advanceTrack(bandwidth, frames)
}

// trajectoryBoundary sets a formant track's segment, and its bandwidth's, after the database's writers
// (parameters.md §11.7).
func (generator *Generator) trajectoryBoundary(track int, start, end, bandwidthEnd byte, afterVowel bool) {
	cursor := generator.cursor()
	previous, current := cursor.Done, cursor.Scan
	previousSymbol := previous.Symbol()
	beforePrevious := symbolBefore(previous)
	if generator.has(beforePrevious, 0, markClass) {
		beforePrevious = ' '
		if node := previous.Previous.Previous; node != nil {
			beforePrevious = node.Symbol()
		}
	}
	segment := &generator.Segments[track]
	previousFrames := int32(previous.Value())
	mode := int32(0)
	if !afterVowel && (track == firstFormant || strings.IndexByte("mn~BDG", previousSymbol) < 0) && !isVoicelessStop(previousSymbol) {
		mode = PullBack
		switch previousSymbol {
		case 'L', 'W':
			if track == firstFormant {
				segment.Back = previousFrames + 1
			}
		case 'M', 'N':
			if previousSymbol == 'M' && track == secondFormant {
				mode = 0
			}
			if track == firstFormant {
				segment.Back = previousFrames
			}
		case 'R':
			switch {
			case track == fourthFormant:
				switch {
				case (generator.has(beforePrevious, 0, fricativeClass) || generator.has(beforePrevious, 2, closureClass)) &&
					!generator.has(beforePrevious, 0, voicedClass):
					segment.Back = previousFrames
				case strings.IndexByte("u4cg", beforePrevious) >= 0:
					segment.Back = previousFrames >> 1
				default:
					mode = 0
				}
			case track == firstFormant && generator.has(beforePrevious, 0, nasalClass):
				segment.Back = previousFrames + 1
			default:
				segment.Back = previousFrames >> 1
			}
		case 'Y':
			if generator.has(beforePrevious, 2, vowelClass) && !generator.has(beforePrevious, 3, frontEndingClass) {
				mode = 0
			}
		case 'j':
			if track == firstFormant && generator.has(beforePrevious, 2, vowelClass) || track == thirdFormant {
				segment.Back = previousFrames
			}
		}
		if previousSymbol == 'L' && generator.has(generator.BeforePrevious, 0, stopClass) {
			segment.Back = max(previousFrames/3, 2)
		}
	}
	startUnits, endUnits := Units(track, start), Units(track, end)
	switch {
	case generator.has(previousSymbol, 2, vowelClass):
		mode, segment.Back, segment.Forward = Bridge, 1, 0
		if !generator.has(previousSymbol, 1, reducedClass) {
			segment.Back = max(previousFrames/3, 2)
		}
		if !generator.has(current.Symbol(), 1, reducedClass) {
			segment.Forward = max(int32(current.Value())/3, 2)
		}
	case generator.Tracks[track].Next != generator.Tracks[nasalZeroTrack].Next && generator.VoiceOnset == noVoiceOnset:
		mode, segment.Onset = PullBack, startUnits
	case track == firstFormant && previousSymbol == 'j':
		segment.BackTarget = startUnits
		if generator.Last[firstFormant] > startUnits+100 {
			segment.BackTarget = startUnits - 100
		}
		segment.BackTarget = max(segment.BackTarget, lowestFirstFormant)
	default:
		segment.Onset, segment.BackTarget = startUnits, startUnits
	}
	if track == firstFormant {
		if previousSymbol == 't' {
			segment.BackTarget -= 100
			if generator.Last[firstFormant] > segment.BackTarget {
				segment.BackTarget -= 50
			}
			segment.BackTarget = max(segment.BackTarget, lowestFirstFormant)
		}
		if previousSymbol == 'R' && generator.has(beforePrevious, 0, nasalClass) {
			segment.BackTarget -= 20
		}
	}
	if previousSymbol == 'R' && generator.has(beforePrevious, 2, vowelClass) && track != fourthFormant {
		mode = 0
	}
	segment.Mode, segment.Target = mode, endUnits
	generator.Last[track] = endUnits
	if track == fourthFormant {
		return
	}
	bandwidthUnits := int32(bandwidthEnd) << 1
	bandwidth := &generator.Segments[track+4]
	bandwidth.Mode, bandwidth.Onset, bandwidth.BackTarget, bandwidth.Target = 0, bandwidthUnits, bandwidthUnits, bandwidthUnits
	generator.Last[track+4] = bandwidthUnits
}
