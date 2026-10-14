package prosody

import "strings"

// sonorantGrids are the sonorants in the order of their duration grids, from grid 29; n has N's grid, and m M's.
const (
	sonorantGrids     = "MN~RYLWlj"
	firstSonorantGrid = 29
)

// The places of a sonorant in its word.
const (
	atWordEnd = iota
	atWordStart
	insideWord
)

func sonorantGrid(symbol byte) int {
	if symbol == 'n' {
		symbol = 'N'
	}
	return firstSonorantGrid + max(strings.IndexByte(sonorantGrids, symbol), 0)
}

// SonorantDuration is a sonorant's duration in frames: its grid's value for the sounds around it, whether it is
// stressed, in the phrase's last word and where it is in its word, with changes for clusters, merged nasals and
// a few sounds after it (prosody.md §6.9).
func (stage *Stage) SonorantDuration() int32 {
	context := &stage.Context
	symbol := context.Symbol
	preceding, beforePreceding := context.PrecedingSymbol, context.BeforePrecedingSymbol
	following, afterFollowing := context.FollowingSymbol, context.AfterFollowingSymbol
	if preceding == ' ' && (beforePreceding == 'S' || beforePreceding == 'Z') && symbol != '~' {
		preceding = beforePreceding
	}
	stressed := stage.cursor().Scan.Flags()&stressedFlag != 0
	lastWord := stage.scanForClass(true, 16, phraseBreak, -wordMark)
	beforeSilence := following == ' ' && afterFollowing == ' ' || among(following, ".!,?\\")
	place := insideWord
	switch {
	case preceding == ' ' && beforePreceding == ' ' || isWordMark(context.PreviousSymbol):
		place = atWordStart
	case isWordMark(context.NextSymbol) || beforeSilence:
		place = atWordEnd
	}
	column := stage.sonorantColumn(preceding, beforePreceding, following, beforeSilence)
	grid := sonorantGrid(symbol)
	nasalGrid := among(symbol, "MNmn")
	rows, row := int32(2), int32(min(place, 1))
	switch {
	case nasalGrid:
		rows, row = 3, int32(place)
	case isGlide(symbol):
		row = 1
		if place == atWordStart {
			row = 0
		}
	}
	if lastWord {
		row += rows
	}
	if !stressed {
		row += 2 * rows
	}
	duration := stage.gridValue(grid, row*gridColumns+column)
	if symbol == 'n' {
		switch {
		case place != atWordEnd && preceding == 'q' && lastWord:
			duration = 12
		case place != atWordEnd && preceding == 'q':
			duration = 10
		case place != atWordEnd:
			duration += 5
		case column == 4:
			duration = 23
		case preceding == 'q' && !lastWord:
			duration = 14
		}
	}
	if duration == 0 {
		fallbackRow := int32(0)
		if !stressed {
			fallbackRow = unstressedRow
		}
		duration = stage.gridOrDuration(grid, fallbackRow*gridColumns+1, symbol, stressed)
	}
	if !among(symbol, "lmn") && column != 5 && column != 7 && column != 13 {
		precedingConsonant := stage.hasClass(context.PrecedingSymbol, consonant)
		followingConsonant := stage.hasClass(following, consonant)
		if precedingConsonant && stage.hasClass(beforePreceding, consonant) ||
			followingConsonant && stage.hasClass(afterFollowing, consonant) || precedingConsonant && followingConsonant {
			duration--
		}
	}
	if symbol == 'N' && among(following, "Llj") {
		duration--
	}
	if stage.Geminate && (symbol == 'M' || symbol == 'N') && stage.hasClass(following, vowel) {
		duration += geminateNasalFrames(symbol, lastWord, context.Following.Flags()&stressedFlag != 0)
	}
	if symbol == 'n' && !lastWord && following != ' ' {
		duration = 9
	}
	if afterFollowing == '.' {
		duration += endingFrames(symbol, following)
	}
	if symbol == 'R' && (context.PreviousSymbol == '4' || context.PreviousSymbol == '5') && stage.hasClass(following, vowel) {
		duration = 3
	}
	return max(duration, shortestByClass)
}

// sonorantColumn is the column of a sonorant's grid for the sounds around it.
func (stage *Stage) sonorantColumn(preceding, beforePreceding, following byte, beforeSilence bool) int32 {
	if stage.hasClass(preceding, vowel) && (stage.hasClass(following, vowel) || stage.hasClass(following, approximant)) {
		switch {
		case !stage.hasClass(following, vowel):
			return 2
		case stage.hasClass(following, lax):
			return 1
		}
		return 0
	}
	switch {
	case preceding == ' ' && beforePreceding == ' ':
		return 3
	case beforeSilence:
		return 4
	case stage.hasClass(beforePreceding, nasal) && !stage.hasClass(preceding, nasal) && stage.hasClass(preceding, consonant):
		return 5
	}
	if preceding == 's' && beforePreceding == 'C' || preceding == 'z' && beforePreceding == 'J' {
		preceding = beforePreceding
	}
	switch preceding {
	case 'j':
		return 6
	case 'l':
		return 7
	case 'C', 'K', 'P', 'T':
		return 8
	case 'F', 'H', 'S', 'X', 's':
		return 9
	case 'B', 'D', 'G', 'J', 'Q', 'q', 't':
		return 10
	case 'V', 'Z', 'd', 'x', 'z':
		return 11
	case 'M', 'N', 'm', 'n', '~':
		return 12
	}
	switch following {
	case 'R':
		return 13
	case 'd':
		return 14
	case 'C', 'K', 'P', 'T':
		return 15
	case 'F', 'H', 'S', 'X', 's':
		return 16
	case 'B', 'D', 'G', 'J', 'Q', 'q', 't':
		return 17
	case 'V', 'Z', 'x', 'z':
		return 18
	case 'M', 'N', 'm', 'n', '~':
		return 19
	}
	return 1
}

// geminateNasalFrames are the frames a merged M or N gets before a vowel.
func geminateNasalFrames(symbol byte, lastWord, followingStressed bool) int32 {
	switch {
	case symbol == 'N' && (lastWord || followingStressed):
		return 5
	case symbol == 'N':
		return 3
	case lastWord && followingStressed:
		return 4
	case lastWord:
		return 6
	case followingStressed:
		return 5
	}
	return 8
}

// endingFrames are the frames a sonorant gains before a D or Z followed by a full stop.
func endingFrames(symbol, following byte) int32 {
	switch following {
	case 'D':
		switch symbol {
		case 'l', 'j':
			return 5
		case 'M':
			return 4
		case 'N', '~':
			return 3
		}
	case 'Z':
		switch symbol {
		case 'l', 'j', '~':
			return 4
		case 'M', 'N':
			return 2
		}
	}
	return 0
}
