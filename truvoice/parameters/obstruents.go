package parameters

import (
	"strings"

	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
)

// The class bits only actions 7 and 9 test (parameters.md §10.1): of class byte 0 and 1.
const (
	markClass    = 0x08
	closingClass = 0x40
)

// The tracks only actions 7 and 9 handle by name.
const (
	secondGainTrack = 3
	fifthGainTrack  = 6
)

// The input flag that keeps the frication's amplitude, the byte of a node that marks a phoneme the stages before
// made of two, and the place of the front vowels.
const (
	fullFricationFlag = 0x01
	mergedOffset      = 0x10
	frontPlace        = 1
)

func isVoicedPlosive(symbol byte) bool {
	return strings.IndexByte("BDG", symbol) >= 0
}

func isMerged(node *pipeline.Node) bool {
	return node.Data[mergedOffset] == 1
}

// FricativeAmplitudes is action 7 (parameters.md §10.2).
func (generator *Generator) FricativeAmplitudes() {
	cursor := generator.cursor()
	current, previous, next := cursor.Scan, cursor.Done, cursor.First
	symbol, previousSymbol, nextSymbol := current.Symbol(), previous.Symbol(), next.Symbol()
	before, after := symbolBefore(current), symbolAfter(current)
	segments := &generator.Segments
	nextVowel := generator.has(nextSymbol, 2, vowelClass)
	previousVowel := generator.has(previousSymbol, 2, vowelClass)
	if previousSymbol == ' ' {
		generator.setModes(secondGainTrack, spareTrack, Flat)
	}
	voiced := generator.has(symbol, 0, voicedClass)
	if !voiced {
		voicing := &segments[voicingTrack]
		voicing.Target, voicing.Forward = 0, 3
		switch {
		case !isVowel(previousSymbol):
			voicing.Mode, voicing.Onset = Shaped, generator.Last[voicingTrack]
		case symbol == 'S':
			voicing.Mode = Flat
		default:
			voicing.Mode, voicing.Onset = PullBack|Shaped, 40
		}
	}
	if cursor.InputFlags&fullFricationFlag == 0 && !generator.has(symbol, 2, labialClass) && previousSymbol != 'J' {
		lowering := int32(12)
		if symbol == 'S' {
			lowering = 15
		}
		segments[fricationTrack].Target = max(segments[fricationTrack].Target-lowering, 0)
	}
	if !voiced {
		lower := false
		if nextVowel {
			lower = symbol != 'S' && symbol != 'X' && generator.has(nextSymbol, 2, frontClass)
		} else {
			lower = !(symbol == 'X' && nextSymbol == 'R') && !(previousSymbol == 'j' && symbol == 'F' && nextSymbol == 'S') &&
				!(symbol == 'S' && (nextSymbol == ' ' || generator.has(nextSymbol, 0, fricativeClass)))
		}
		if lower {
			segments[aspirationTrack].Target = generator.FricativeAspiration - 20
		}
	}
	if symbol == 'Z' {
		generator.amplitudesForZ()
	}
	if symbol == 'V' && previousVowel && (nextSymbol == 'M' || nextSymbol == 'N') {
		segments[fricationTrack].Target = 62
	}
	if symbol == 'Z' {
		if nextSymbol == 'X' {
			segments[aspirationTrack].Target = 50
		}
		if generator.has(previousSymbol, 0, nasalClass) && (nextSymbol == 'F' || nextSymbol == 'X') {
			segments[fricationTrack].Target, segments[fricationTrack].Mode = 69, Flat
		}
		if previousVowel && nextSymbol == 'F' {
			segments[firstVoiceTrack].Target = 17
		}
		if before == 'j' && nextSymbol == 'o' {
			segments[fricationTrack].Target, segments[firstVoiceTrack+1].Target = 63, 14
		}
	}
	if symbol == 'X' {
		if previousSymbol == 'Z' || previousSymbol == 'V' {
			segments[fricationTrack].Mode = Shaped
		}
		segments[bypassTrack].Target = 55
		if before == 'S' && nextSymbol == 'R' {
			segments[aspirationTrack].Target = 73
		}
		if before == 'S' && (nextSymbol == 'W' || nextSymbol == 'L') {
			segments[aspirationTrack].Target = 50
			segments[fifthGainTrack].Target, segments[spareTrack].Target = 75, 80
		}
		if nextSymbol == 'E' {
			segments[fifthGainTrack].Target = 70
		}
		if nextSymbol == 'w' {
			segments[aspirationTrack].Target, segments[spareTrack].Target = 65, 70
		}
	}
	if symbol == 'S' && nextSymbol == 'E' && previousSymbol == 'w' {
		segments[aspirationTrack].Target = 55
	}
	if symbol == 's' {
		generator.amplitudesForLowerS()
	}
	if symbol == 'z' {
		if previousVowel && nextSymbol == 'E' {
			segments[fricationTrack].Target = 72
		}
		if before == 'J' {
			switch nextSymbol {
			case '|':
				segments[fricationTrack].Target = 68
			case 'r':
				segments[fricationTrack].Target, segments[thirdGainTrack].Target = 65, 65
			case 'g':
				segments[fricationTrack].Target = 72
				segments[fourthGainTrack].Target, segments[fifthGainTrack].Target = 62, 70
			}
		}
	}
	if symbol == 'Z' && previousSymbol == ' ' && nextVowel {
		segments[firstVoiceTrack].Target = 12
	}
	if symbol == 'Z' && nextSymbol == ' ' && generator.has(previousSymbol, 0, voicedClass) &&
		(generator.has(previousSymbol, 2, closureClass) || generator.has(previousSymbol, 0, fricativeClass)) {
		generator.setTargets(firstVoiceTrack, 8, 14)
	}
	if symbol == 'z' && (before == 'J' || generator.has(before, 2, vowelClass)) && nextSymbol == 'S' {
		segments[thirdGainTrack].Target = 65
	}
	if symbol == 'Z' {
		if generator.has(before, 0, nasalClass) && (generator.has(after, 0, markClass) || generator.has(after, 1, closingClass) || after == ' ') {
			segments[thirdGainTrack].Target = 66
		}
		if isVoicedPlosive(previousSymbol) && nextVowel {
			segments[firstBandwidth].Target, segments[firstVoiceTrack+1].Target = 70, 14
		}
		if previousSymbol == 'E' && nextSymbol == 'F' && symbolAfter(next) == 'k' {
			segments[fricationTrack].Target = 55
		}
	}
	if symbol == 'F' && isMerged(current) {
		for track := secondGainTrack; track <= bypassTrack; track++ {
			if segments[track].Target > 3 {
				segments[track].Target -= 4
			}
		}
	}
	if symbol == 's' {
		if previousSymbol == 'N' && nextVowel {
			segments[fricationTrack].Target = 68
			segments[thirdGainTrack].Target, segments[fourthGainTrack].Target = 68, 68
		}
		if (generator.has(previousSymbol, 4, rColouredClass) || previousVowel || strings.IndexByte("Ljl", previousSymbol) >= 0) && nextSymbol == 'E' {
			segments[fricationTrack].Target, segments[aspirationTrack].Target = 72, 50
		}
		if generator.has(after, 4, rColouredClass) && after != 'r' && after != '4' {
			segments[fricationTrack].Target, segments[aspirationTrack].Target = 72, 50
		}
		if previousSymbol == 'K' && strings.IndexByte("g4k", nextSymbol) >= 0 {
			segments[fricationTrack].Target, segments[aspirationTrack].Target = 75, 50
		}
	}
	if symbol == 'V' {
		if previousSymbol == ' ' && nextVowel {
			segments[firstVoiceTrack+1].Target = 14
		}
		if nextVowel || after == 'L' && generator.has(symbolAfter(current.Next), 2, vowelClass) {
			segments[voicingTrack].Target = 45
		}
	}
	if symbol == 'F' && previousSymbol == 'f' && nextSymbol == ' ' {
		segments[aspirationTrack].Target = 45
	}
}

// amplitudesForZ is the part of action 7 for Z before a stop or a sonorant.
func (generator *Generator) amplitudesForZ() {
	cursor := generator.cursor()
	current, previous, next := cursor.Scan, cursor.Done, cursor.First
	previousSymbol, nextSymbol := previous.Symbol(), next.Symbol()
	segments := &generator.Segments
	if !generator.has(nextSymbol, 0, sonorantClass) {
		if !generator.has(nextSymbol, 0, stopClass) {
			return
		}
		generator.setTargets(fourthGainTrack, 74, 74, 74)
		switch {
		case isVoicedPlosive(previousSymbol) && isVoicedPlosive(nextSymbol):
			segments[voicingTrack].Forward = int32(current.Value() >> 1)
		case generator.has(nextSymbol, 0, voicedClass):
			segments[voicingTrack].Target = max(generator.Last[voicingTrack]-2, 50)
		}
		if generator.has(previousSymbol, 0, nasalClass) {
			segments[fricationTrack].Target, segments[fricationTrack].Mode = 69, Flat
		}
		return
	}
	if nextSymbol != 'd' {
		segments[voicingTrack].Target = max(generator.Last[voicingTrack]-2, 50)
	}
	if previousSymbol != ' ' {
		segments[firstVoiceTrack].Target = 10
		segments[firstVoiceTrack].Back = int32(min(previous.Value(), 5))
	}
	switch {
	case generator.has(previousSymbol, 2, vowelClass):
		segments[fricationTrack].Target, segments[fricationTrack].Forward = 64, 2
		generator.setTargets(secondGainTrack, 45, 45, 72, 72, 70)
	case previousSymbol == ' ':
		segments[fourthGainTrack].Target, segments[spareTrack].Target = 72, 72
	case nextSymbol == 'd' && generator.has(previousSymbol, 0, nasalClass):
		segments[fricationTrack].Target = 69
		generator.setTargets(fourthGainTrack, 74, 74, 74)
	default:
		generator.setTargets(fourthGainTrack, 68, 68, 68)
	}
}

// amplitudesForLowerS is the part of action 7 for s, alone or after C.
func (generator *Generator) amplitudesForLowerS() {
	cursor := generator.cursor()
	current, previous, next := cursor.Scan, cursor.Done, cursor.First
	nextSymbol := next.Symbol()
	segments := &generator.Segments
	if symbolBefore(current) != 'C' {
		if generator.has(nextSymbol, 2, vowelClass) {
			segments[aspirationTrack].Target = 48
		}
		generator.setTargets(thirdGainTrack, 65, 65, 60)
		segments[fricationTrack].Target = 66
	}
	if previous.Symbol() != 'C' {
		return
	}
	switch nextSymbol {
	case 'w', 'c', 'u':
		segments[thirdGainTrack].Target, segments[fourthGainTrack].Target = 68, 65
	case 'A', 'o':
		segments[thirdGainTrack].Target = 70
	case 'a', 'O':
		segments[thirdGainTrack].Target, segments[fourthGainTrack].Target = 70, 65
	case 'v':
		segments[thirdGainTrack].Target = 60
	case 'R':
		segments[fricationTrack].Target, segments[spareTrack].Target = 68, 55
	case 'E':
		if generator.has(symbolBefore(previous), 4, rColouredClass) {
			segments[fricationTrack].Target, segments[aspirationTrack].Target = 72, 50
		}
	}
}

// StopClosureBurst is action 9 (parameters.md §10.3).
func (generator *Generator) StopClosureBurst() {
	cursor := generator.cursor()
	current, previous, next := cursor.Scan, cursor.Done, cursor.First
	symbol, previousSymbol, nextSymbol := current.Symbol(), previous.Symbol(), next.Symbol()
	before, after := symbolBefore(current), symbolAfter(current)
	segments, levels := &generator.Segments, &generator.BurstLevels
	switch symbol {
	case 'B':
		generator.burstForB()
	case 'D':
		generator.burstForD()
	case 'G':
		generator.burstForG()
	case 'K':
		generator.burstForK()
	case 'P':
		generator.burstForP()
	case 'T':
		generator.burstForT()
	}
	if isVoicedPlosive(symbol) {
		switch {
		case nextSymbol == 'Z' && symbolAfter(next) == ' ':
			segments[voicingTrack].Target, levels[voicingTrack] = 55, 0
		case previousSymbol != ' ' && generator.has(previousSymbol, 0, voicedClass):
			segments[voicingTrack].Target = 42
		}
		if previousSymbol == 'Z' && isVoicedPlosive(symbolBefore(previous)) {
			segments[voicingTrack].Mode &= Shaped
		}
	}
	if symbol == 'B' && generator.has(previousSymbol, 2, vowelClass) && after == 'p' {
		segments[voicingTrack].Target, segments[voicingTrack].Mode, levels[voicingTrack] = 41, Flat, 43
	}
	if cursor.InputFlags&fullFricationFlag == 0 {
		levels[fricationTrack] = max(levels[fricationTrack], 0)
	}
	weakening := int32(0)
	if !stressed(current) && (symbol == 'C' && !(nextSymbol == 's' && symbolAfter(next) == 'R') || symbol == 'J') {
		weakening = 3
		if generator.has(previousSymbol, 0, nasalClass) {
			weakening = 6
		}
	}
	if symbol == 'C' && nextSymbol == 's' && (before == ' ' || isWordMark(before)) {
		switch vowel := symbolAfter(next); {
		case strings.IndexByte("3vAIkrOwcg4", vowel) >= 0:
			generator.setTargets(secondGainTrack, 0, 60, 0, 0, 0)
		case strings.IndexByte("iE5oefy", vowel) >= 0:
			generator.setTargets(secondGainTrack, 0, 0, 55, 0, 45)
		case vowel == 'b' || vowel == 'u':
			generator.setTargets(secondGainTrack, 0, 45, 0, 0, 0)
		}
	}
	frication := &generator.Tracks[fricationTrack]
	frames := segments[fricationTrack].Frames
	if generator.BurstFrames >= frames && generator.BurstFrames != burstLevel {
		generator.BurstFrames = frames >> 1
	}
	generator.BurstFrames = max(generator.BurstFrames, 1)
	generator.Ring(fricationTrack).AddBack(frication.Next, 3, frication.Next-frication.Earliest, -20)
	closure := frames
	if generator.BurstFrames < frames {
		closure = frames - generator.BurstFrames
		for track := voicingTrack; track <= aspirationTrack; track++ {
			if levels[track] != burstLevel {
				generator.writeClosure(track, closure, weakening)
			}
		}
	}
	nextVowel := generator.has(nextSymbol, 2, vowelClass)
	if symbol == 'P' && (after == 'R' || after == 'L') || symbol == 'T' && stressed(next) && nextVowel {
		bypass := &segments[bypassTrack]
		generator.Ring(bypassTrack).Repeat(generator.Tracks[bypassTrack].Next, closure, 0)
		bypass.Target = 62
		if symbol == 'P' {
			bypass.Target = 60
		}
		if symbol == 'T' && (before == ' ' || isWordMark(before)) && nextVowel {
			bypass.Target = 55
		}
		bypass.Mode = Flat
		generator.advanceTrack(bypassTrack, closure)
		generator.Last[bypassTrack] = 0
	}
	if symbol == 'B' && nextVowel {
		segments[bypassTrack].Target = 60
	}
	if symbol == 'T' && before == 'S' && nextVowel && isWordMark(after) {
		segments[bypassTrack].Target = 0
	}
	if nextSymbol == 'O' && previousSymbol == ' ' && symbol == 'T' {
		segments[bypassTrack].Target = 0
	}
	generator.ClosureFrames = closure
}

// writeClosure writes the closure's frames on track 0, 1 or 2, and makes the burst level the segment's target.
func (generator *Generator) writeClosure(track int, closure, weakening int32) {
	cursor := generator.cursor()
	current, previous, next := cursor.Scan, cursor.Done, cursor.First
	symbol, previousSymbol, nextSymbol := current.Symbol(), previous.Symbol(), next.Symbol()
	segment, position := &generator.Segments[track], &generator.Tracks[track]
	segment.Target = max(segment.Target-cursor.Attenuation, 0)
	target := byte(segment.Target)
	ring := generator.Ring(track)
	previousVowel := generator.has(previousSymbol, 2, vowelClass)
	written := track == voicingTrack
	switch {
	case track != voicingTrack:
	case previousSymbol == ' ':
		if symbol == 'B' && closure > 3 {
			ring.Repeat(position.Next, closure-3, 0)
			ring.Repeat(position.Next+closure-3, 3, target)
		} else {
			ring.Repeat(position.Next, closure, target)
			ring.PullForward(position.Next, closure>>1, closure, byte(segment.Target>>1))
		}
	case isVoicedPlosive(symbol) && nextSymbol == 'Z' && symbolAfter(next) == ' ':
		ring.Repeat(position.Next, closure, target)
		ring.AddBack(position.Next+closure, closure+2, closure+2, -12)
	case symbol == 'T' && previousVowel && generator.has(nextSymbol, 2, vowelClass):
		ring.PullBack(position.Next, 6, 6, 40)
		ring.Repeat(position.Next, closure, target)
	case (symbol == 'B' || symbol == 'D') && generator.has(previousSymbol, 0, voicedClass) &&
		!(previousSymbol == 'Z' && isVoicedPlosive(symbolBefore(previous))):
		value := generator.Last[voicingTrack]/4 - segment.Target/4 + segment.Target
		ring.PullBack(position.Next, int32(previous.Value()>>1), position.Next-position.Earliest, byte(value))
		ring.Repeat(position.Next, closure, target)
	default:
		written = false
	}
	if !written {
		if symbol == 'q' && (!generator.has(nextSymbol, 0, voicedClass) || nextSymbol == 'x') && previousVowel {
			shape := int32(previous.Value() >> 2)
			ring.PullBack(position.Next, shape, shape, 38)
		}
		ring.Repeat(position.Next, closure, target)
	}
	generator.advanceTrack(track, closure)
	generator.Last[track] = segment.Target
	levels := &generator.BurstLevels
	levels[track] = max(levels[track]-weakening, 0)
	silenced := strings.IndexByte("DGJ", symbol) >= 0 && previousSymbol == ' ' ||
		symbol == 'G' && generator.has(previousSymbol, 1, consonantClass) && !generator.has(previousSymbol, 0, voicedClass)
	if track == voicingTrack && silenced {
		segment.Target = 0
	} else {
		segment.Target = levels[track]
	}
	_, nextPhoneme := generator.phoneme(nextSymbol)
	if track == voicingTrack && (nextSymbol == 'p' ||
		stressed(next) && symbol == 'D' && previousSymbol != ' ' && nextPhoneme.Place == frontPlace) {
		segment.Mode = PullBack | Flat
	} else {
		segment.Mode = Flat
	}
}

func (generator *Generator) burstForB() {
	cursor := generator.cursor()
	previousSymbol, nextSymbol := cursor.Done.Symbol(), cursor.First.Symbol()
	levels := &generator.BurstLevels
	nextVowel := generator.has(nextSymbol, 2, vowelClass)
	switch {
	case nextSymbol == 'p':
		levels[voicingTrack], levels[aspirationTrack], levels[fricationTrack] = burstLevel, 0, 57
		if previousSymbol == 'S' {
			levels[fricationTrack] = 60
		}
	case nextVowel && previousSymbol == 'S':
		levels[voicingTrack], levels[fricationTrack] = burstLevel, 60
	case nextSymbol == 'Z' && previousSymbol != 'S':
		levels[fricationTrack] += 8
	case isVowel(nextSymbol):
		levels[fricationTrack] = 62
	case nextSymbol == 'X':
		levels[fricationTrack] = 68
	}
	if nextVowel && isVoicelessStop(previousSymbol) {
		generator.Segments[voicingTrack].Target = 0
	}
	if nextSymbol == 'k' {
		levels[fricationTrack], levels[aspirationTrack] = 68, 55
	}
}

func (generator *Generator) burstForD() {
	cursor := generator.cursor()
	current, previousSymbol, nextSymbol := cursor.Scan, cursor.Done.Symbol(), cursor.First.Symbol()
	after := symbolAfter(current)
	segments, levels := &generator.Segments, &generator.BurstLevels
	previousVowel := generator.has(previousSymbol, 2, vowelClass)
	switch {
	case strings.IndexByte("RLWY", nextSymbol) >= 0:
		if previousSymbol == 'S' && nextSymbol == 'R' && stressed(current) {
			generator.BurstFrames, levels[fricationTrack] = 2, 60
			generator.setTargets(fourthGainTrack, 65, 60, 60)
		} else {
			generator.BurstFrames = 1
			levels[voicingTrack], levels[fricationTrack] = burstLevel, 60
		}
	case isWordMark(after) && (generator.has(nextSymbol, 2, vowelClass) || nextSymbol == 'x' && previousSymbol == 'N'):
		levels[fricationTrack] = 42
	case nextSymbol == '|':
		levels[voicingTrack], levels[fricationTrack], levels[aspirationTrack] = 50, 60, 48
	case nextSymbol == 'X':
		levels[fricationTrack] = 68
	case nextSymbol == 'Z' && previousVowel:
		levels[fricationTrack], levels[aspirationTrack] = 70, 60
	case nextSymbol == 'M' && generator.has(after, 0, markClass):
		generator.BurstFrames, levels[fricationTrack] = 1, 65
		segments[fourthGainTrack].Target = 65
	case nextSymbol == 'w' && previousVowel:
		levels[fricationTrack] = 60
	}
}

func (generator *Generator) burstForG() {
	cursor := generator.cursor()
	previousSymbol, nextSymbol := cursor.Done.Symbol(), cursor.First.Symbol()
	levels := &generator.BurstLevels
	frication := &generator.Segments[fricationTrack]
	frication.Onset, frication.Target = 0, 0
	switch nextSymbol {
	case 'Z':
		levels[voicingTrack] = 50
	case 'R', 'L':
		levels[fricationTrack] = 66
	case 'X':
		levels[fricationTrack], levels[aspirationTrack] = 65, 57
	}
	previousVowel := generator.has(previousSymbol, 2, vowelClass)
	if previousVowel && generator.has(nextSymbol, 0, nasalClass) {
		levels[fricationTrack], levels[aspirationTrack] = 50, 40
	}
	if previousVowel && symbolAfter(cursor.Scan) == 'p' {
		levels[voicingTrack] = 46
	}
}

func (generator *Generator) burstForK() {
	cursor := generator.cursor()
	next, previousSymbol := cursor.First, cursor.Done.Symbol()
	nextSymbol := next.Symbol()
	segments, levels := &generator.Segments, &generator.BurstLevels
	segments[fricationTrack].Onset, segments[fricationTrack].Target = 0, 0
	previousVowel := generator.has(previousSymbol, 2, vowelClass)
	switch {
	case nextSymbol == 'p':
		levels[fricationTrack], levels[aspirationTrack] = 58, 48
		segments[bypassTrack].Target = 55
	case nextSymbol == 'A':
		levels[fricationTrack] = 60
	case nextSymbol == 'a':
		levels[fricationTrack] = 64
	case nextSymbol == 'k':
		levels[fricationTrack] = 64
		generator.setTargets(thirdGainTrack, 68, 55, 68)
	case nextSymbol == 'L':
		levels[fricationTrack] = 65
		segments[fifthGainTrack].Target, segments[spareTrack].Target = 67, 65
		if previousVowel && generator.has(symbolAfter(cursor.Scan), 0, markClass) {
			levels[fricationTrack] = 55
		}
	case nextSymbol == 'T':
		levels[fricationTrack], levels[aspirationTrack], generator.BurstFrames = 65, 62, 1
	case nextSymbol == 'e':
		levels[fricationTrack] = 68
		generator.setTargets(thirdGainTrack, 65, 55, 55)
		if previousVowel {
			levels[fricationTrack] = 62
		}
	case nextSymbol == '|':
		levels[fricationTrack] = 65
		generator.setTargets(thirdGainTrack, 70, 35, 55, 50)
	case strings.IndexByte("buOU", nextSymbol) >= 0:
		levels[fricationTrack] = 65
	case generator.has(nextSymbol, 2, vowelClass) && stressed(next):
		levels[fricationTrack]++
		levels[aspirationTrack]++
	case nextSymbol == 'R':
		levels[fricationTrack], segments[secondGainTrack].Target = 63, 63
	case nextSymbol == 'Y':
		generator.setTargets(thirdGainTrack, 35, 45, 70, 80)
	case nextSymbol == 'M' || nextSymbol == 'N':
		levels[fricationTrack] = burstLevel
	}
	if previousVowel && nextSymbol == 'W' {
		segments[secondGainTrack].Target, segments[spareTrack].Target = 60, 60
		segments[secondFormant].Target, levels[fricationTrack] = 1600, 65
	}
	if nextSymbol == 'w' {
		segments[secondGainTrack].Target, segments[fifthGainTrack].Target, levels[fricationTrack] = 60, 65, 60
	}
	if nextSymbol == 's' && previousVowel {
		levels[fricationTrack], levels[aspirationTrack] = 70, 55
		if strings.IndexByte("EaieA4k", symbolAfter(next)) >= 0 {
			generator.setTargets(secondGainTrack, 45, 75, 65, 75)
		}
	}
	if nextSymbol == 'i' {
		levels[fricationTrack] += 7
		for track := thirdGainTrack; track <= fifthGainTrack; track++ {
			segments[track].Target += 7
		}
	}
}

func (generator *Generator) burstForP() {
	cursor := generator.cursor()
	next, nextSymbol := cursor.First, cursor.First.Symbol()
	before := symbolBefore(cursor.Scan)
	segments, levels := &generator.Segments, &generator.BurstLevels
	switch {
	case nextSymbol == 'y':
		levels[fricationTrack] = 58
		levels[aspirationTrack] += 3
	case generator.has(nextSymbol, 2, vowelClass) && stressed(next):
		levels[fricationTrack] += 3
		levels[aspirationTrack] += 3
	case nextSymbol == 'S':
		levels[fricationTrack] = 60
	case nextSymbol == 'Y':
		segments[secondGainTrack].Target = 80
	}
	if !generator.has(nextSymbol, 0, voicedClass) && generator.has(nextSymbol, 0, stopClass|fricativeClass) {
		levels[fricationTrack], levels[aspirationTrack] = 70, 62
		if generator.has(nextSymbol, 2, alveolarClass) {
			segments[secondFormant].Target = 908
		}
	}
	if before == 'E' && nextSymbol == 'R' {
		levels[fricationTrack] = 65
		generator.setTargets(secondGainTrack, 0, 0, 0, 0, 0, 60)
	}
	if before == 'E' && nextSymbol == 's' {
		levels[fricationTrack], segments[secondGainTrack].Target = 65, 65
	}
	if nextSymbol == 'a' {
		generator.setTargets(secondGainTrack, 55, 65, 45)
		levels[fricationTrack], levels[aspirationTrack] = 67, 55
	}
}

func (generator *Generator) burstForT() {
	cursor := generator.cursor()
	current, previous, next := cursor.Scan, cursor.Done, cursor.First
	previousSymbol, nextSymbol := previous.Symbol(), next.Symbol()
	before := symbolBefore(current)
	segments, levels := &generator.Segments, &generator.BurstLevels
	nextVowel := generator.has(nextSymbol, 2, vowelClass)
	if nextVowel && (stressed(next) || nextSymbol == 'p') {
		switch {
		case nextSymbol == 'b':
			levels[fricationTrack] = 65
		case previousSymbol != 'N' || levels[fricationTrack] != 0:
			levels[fricationTrack] = 58
		}
		segments[bypassTrack].Target = 0
	}
	switch {
	case generator.has(nextSymbol, 4, rColouredClass):
		levels[fricationTrack], levels[aspirationTrack] = 65, 52
	case nextVowel:
		levels[fricationTrack], levels[aspirationTrack] = 63, 55
	}
	if previousSymbol == 'F' && nextSymbol == 'p' {
		levels[fricationTrack], levels[aspirationTrack] = 65, 55
	}
	if nextSymbol == 'n' {
		levels[fricationTrack], levels[aspirationTrack] = 60, 48
	}
	if generator.has(previousSymbol, 2, vowelClass) && nextSymbol == 'K' {
		levels[fricationTrack], levels[aspirationTrack], generator.BurstFrames = 68, 58, 1
	}
	if previousSymbol == 'S' && nextVowel {
		levels[fricationTrack], levels[aspirationTrack] = 65, 55
	}
	if nextSymbol == 'c' && (before == ' ' || isWordMark(before)) {
		segments[thirdGainTrack].Target, levels[fricationTrack] = 65, 60
	}
	if nextSymbol == 'r' {
		segments[secondGainTrack].Target = 40
		segments[thirdGainTrack].Target, segments[spareTrack].Target = 70, 70
	}
	if previousSymbol == 'N' && !stressed(previous.Previous) && generator.has(symbolBefore(previous), 2, vowelClass) &&
		isWordMark(symbolAfter(current)) && nextVowel && stressed(next.Next) {
		generator.BurstFrames = burstLevel
	}
}
