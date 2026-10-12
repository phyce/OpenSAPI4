package parameters

import (
	"strings"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
)

// The highest F3 of a liquid's points, and the F1 a point relative to the lower of the track's last value and its
// target gets below the lowest (parameters.md §14.3).
const (
	highestLiquidThirdFormant = 2950
	lowestLiquidFirstFormant  = 200
	raisedLiquidFirstFormant  = 204
)

// vowelContext is the right context of a consonant before a vowel when the phoneme after the vowel is in class
// vowel (parameters.md §14.1).
func vowelContext(symbol byte) byte {
	switch {
	case strings.IndexByte("3krg4", symbol) >= 0:
		return 'R'
	case strings.IndexByte("bfOU", symbol) >= 0:
		return 'W'
	case strings.IndexByte("@vow", symbol) >= 0:
		return ' '
	}
	return 'Y'
}

// lociFromDatabase looks the next phoneme, one of the database's vowels, up with the current consonant as its
// left context. When there is a record, F1 to F4's targets become the vowel's start, and it reports true
// (parameters.md §14.1).
func (generator *Generator) lociFromDatabase(vowel int) bool {
	cursor := generator.cursor()
	current, previous, next := cursor.Scan, cursor.Done, cursor.First
	symbol := current.Symbol()
	after := next.Next
	right, neighbour := symbolOf(after), symbolAfter(after)
	if isWordMark(right) {
		right, neighbour = symbolAfter(after), ' '
		if after.Next != nil && after.Next.Next != nil {
			neighbour = after.Next.Next.Symbol()
		}
	}
	left := symbol
	if left == 'n' {
		left = 'N'
	}
	if generator.has(right, 2, vowelClass) {
		right = vowelContext(next.Symbol())
	}
	following := current.Next
	followingSymbol, beyond := symbolOf(following), following.Next
	beyondSymbol := symbolOf(beyond)
	if generator.has(followingSymbol, 3, frontEndingClass) && isWordMark(beyondSymbol) && followingSymbol != 'E' &&
		followingSymbol != '5' && strings.IndexByte("RLWYdHt", symbolAfter(beyond)) < 0 {
		right = ' '
	} else if right == 'n' {
		right = 'N'
	}
	if isWordMark(beyondSymbol) && isMerged(beyond.Next) {
		right = symbolAfter(beyond)
	}
	if isWordMark(followingSymbol) && generator.has(left, 0, fricativeClass) && left != 'Z' && left != 'S' && stressed(beyond) &&
		(generator.has(beyondSymbol, 3, frontEndingClass) || generator.has(symbolAfter(beyond), 0, nasalClass) && symbol != 'X' &&
			(symbol != 's' || beyondSymbol != '3') && (symbol != 'V' || beyondSymbol != 'a')) {
		left = ' '
	}
	if generator.context(left) < 0 || generator.context(right) < 0 {
		return false
	}
	record := generator.findRecord(&generator.vowels.Vowels[vowel], left, right, false)
	if record == nil {
		return false
	}
	if alternative := record.Alternative; alternative != nil {
		named := previous.Symbol()
		if alternative.AfterNext {
			named = neighbour
		}
		if named == alternative.Symbol {
			record = &alternative.Record
		}
	}
	for formant := range datapack.FormantCount {
		track := firstFormant + formant
		generator.Segments[track].Target = Units(track, record.Start[formant])
	}
	return true
}

// group returns the place of the first of the groups that has the symbol, or the number of groups.
func group(symbol byte, groups ...string) int {
	for index, members := range groups {
		if strings.IndexByte(members, symbol) >= 0 {
			return index
		}
	}
	return len(groups)
}

// targetsBeforeLiquid sets F1 to F4's targets of a consonant before R or L that the vowel database has none for
// (parameters.md §14.2).
func (generator *Generator) targetsBeforeLiquid() {
	cursor := generator.cursor()
	symbol, previousSymbol := cursor.Scan.Symbol(), cursor.Done.Symbol()
	following := cursor.Scan.Next
	liquid, after := symbolOf(following), symbolAfter(following)
	liquids := &generator.tables.Liquids
	segments := &generator.Segments
	if isWordMark(liquid) && after == 'R' {
		targets := &liquids.TargetsAcrossMark
		segments[firstFormant].Target = targets[0][group(symbol, "K")]
		segments[secondFormant].Target = targets[1][group(symbol, "x", "X", "K", "jl", "PS", "s")]
		segments[thirdFormant].Target = targets[2][group(symbol, "GJZz", "P", "K", "XS")]
		segments[fourthFormant].Target = targets[3][group(symbol, "~", "Rz", "P")]
		return
	}
	switch {
	case symbol == 'z' && previousSymbol == 'J':
		symbol = 'D'
	case symbol == 's' && previousSymbol == 'C':
		symbol = 'T'
	}
	table := &liquids.R
	if liquid == 'L' {
		table = &liquids.L
	} else if after == 'l' {
		after = '|'
	}
	for index := range table {
		segments[firstFormant+index].Target = table[index].Row(symbol, after).Values[0]
	}
}

// liquidTrajectories writes F1 to F4 of R, of L after a stop and of Y after a vowel from the liquids' tables, and
// sets the bandwidths' segments (parameters.md §14.3).
func (generator *Generator) liquidTrajectories() {
	cursor := generator.cursor()
	current, previous := cursor.Scan, cursor.Done
	symbol, previousSymbol, nextSymbol := current.Symbol(), previous.Symbol(), cursor.First.Symbol()
	liquids := &generator.tables.Liquids
	segments := &generator.Segments
	frames := int32(current.Value())
	afterVowel := generator.has(previousSymbol, 2, vowelClass)
	switch {
	case afterVowel && symbol == 'R':
		var rows [3]int
		if strings.IndexByte("AbEIy", previousSymbol) >= 0 {
			generator.setTargets(firstBandwidth, 60, 100, 156)
			segments[secondFormant].Back = frames / 3
		} else {
			rows[1] = 1
			generator.setTargets(firstBandwidth, 94, 97, 130)
			segments[secondFormant].Back = frames >> 2
		}
		if strings.IndexByte("gckr34@|", previousSymbol) < 0 {
			rows[2] = 1
		}
		segments[firstFormant].Back, segments[thirdFormant].Back = frames>>1, frames>>1
		for index, row := range rows {
			generator.threePoints(firstFormant+index, &liquids.RAfterVowel[index][row])
		}
	case isWordMark(symbolBefore(current)) && symbol == 'R':
		if previousSymbol == ' ' {
			previousSymbol = generator.BeforePrevious
		}
		rows := [datapack.LiquidTrackCount]int{0, 4, group(previousSymbol, "GJZ", "P", "V"), group(previousSymbol, "~", "R", "P")}
		switch {
		case nextSymbol == 'E':
			rows[1] = 0
		case strings.IndexByte("jlK", previousSymbol) >= 0 || strings.IndexByte("ycg3", nextSymbol) >= 0:
			rows[1] = 1
		case previousSymbol == 'x':
			rows[1] = 2
		case previousSymbol == 'P':
			rows[1] = 3
		}
		for index, row := range rows {
			generator.threePoints(firstFormant+index, &liquids.RAcrossMark[index][row])
		}
		segments[fourthFormant].Mode = PullBack
	case afterVowel && symbol == 'Y':
		for index := range liquids.Y {
			generator.threePoints(firstFormant+index, liquids.Y[index].Row(previousSymbol, nextSymbol))
		}
	default:
		if previousSymbol == ' ' {
			previousSymbol = generator.BeforePrevious
		}
		switch {
		case previousSymbol == 'z' && generator.BeforePrevious == 'J':
			previousSymbol = 'D'
		case previousSymbol == 's' && generator.BeforePrevious == 'C':
			previousSymbol = 'T'
		}
		table := &liquids.R
		if symbol == 'L' {
			table = &liquids.L
		} else if nextSymbol == 'l' {
			nextSymbol = '|'
		}
		for index := range table {
			generator.threePoints(firstFormant+index, table[index].Row(previousSymbol, nextSymbol))
		}
		segments[fourthFormant].Mode = PullBack
	}
	generator.setModes(firstFormant, thirdFormant, PullBack)
	quarter := func(frames uint32) int32 {
		if frames < 8 {
			return 2
		}
		return int32(frames >> 2)
	}
	for track := firstBandwidth; track < nasalZeroTrack; track++ {
		segments[track].Mode, segments[track].Back, segments[track].Forward = Bridged, quarter(previous.Value()), quarter(current.Value())
	}
}

// threePoints writes a formant's track through the start, middle and end of a row of the liquids' tables, and
// sets its segment's onset and back target to the start (parameters.md §14.3).
func (generator *Generator) threePoints(track int, row *datapack.LiquidRow) {
	segment := &generator.Segments[track]
	var points [3]int32
	for index, kind := range row.Kinds {
		switch kind {
		case 1:
			base := generator.Last[track]
			if index == 2 {
				base = segment.Target
			}
			points[index] = base + row.Values[index]
		case 2:
			points[index] = min(generator.Last[track], segment.Target) + row.Values[index]
			if track == firstFormant && points[index] < lowestLiquidFirstFormant {
				points[index] = raisedLiquidFirstFormant
			}
		default:
			points[index] = row.Values[index]
		}
	}
	var bytes [3]byte
	for index := range points {
		if track == thirdFormant {
			points[index] = min(points[index], highestLiquidThirdFormant)
		}
		bytes[index] = byte(fromUnits(track, points[index]))
	}
	toMiddle := int32((uint32(row.Duration)*generator.cursor().Scan.Value() + 50) / 100)
	generator.writeThreePoints(track, bytes, toMiddle, points[1] == 0)
	segment.Onset, segment.BackTarget = points[0], points[0]
}

// writeThreePoints writes a formant's track for the current phoneme: after the voice onset, a frame of the start,
// then a ramp to the middle and one to the end, or without a middle one ramp from the start to the end
// (parameters.md §14.3).
func (generator *Generator) writeThreePoints(track int, points [3]byte, toMiddle int32, noMiddle bool) {
	start, middle, end := points[0], points[1], points[2]
	ring := generator.Ring(track)
	position := generator.Tracks[track].Next
	if generator.voiceOnsetCounts() {
		onset := generator.VoiceOnset
		last := fromUnits(track, generator.Last[track])
		if track == firstFormant && !(isVoicelessStop(generator.cursor().Done.Symbol()) && last != int32(start)) {
			last = int32(start)
		}
		ring.Fill(position, min(onset, 20), onset, byte(last), start)
		position += onset
		if onset == 1 {
			generator.advanceTrack(track, onset)
		}
	}
	generator.VowelFrames = int32(generator.cursor().Scan.Value())
	if generator.VoiceOnset != 0 && generator.VoiceOnset != noVoiceOnset {
		generator.VowelFrames -= generator.VoiceOnset
	}
	ring.Fill(position, 1, 1, start, start)
	position++
	if noMiddle {
		ring.Ramp(position, min(generator.VowelFrames-1, 20), start, end)
		return
	}
	toMiddle = max(toMiddle-1, 1)
	ring.Ramp(position, min(toMiddle, 20), start, middle)
	position += min(toMiddle, 20)
	if rest := generator.VowelFrames - toMiddle - 1; rest > 0 {
		ring.Ramp(position, min(rest, 20), middle, end)
	}
}
