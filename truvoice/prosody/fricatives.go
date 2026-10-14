package prosody

import (
	"strings"

	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
)

// fricativeGrids are the fricatives with a duration grid, in the order of their grids from grid 24; any other
// symbol but the fixed fricatives has F's grid.
const (
	fricativeGrids     = "FXSsZ"
	firstFricativeGrid = 24
)

// fixedFricatives have no grid: their durations are fixedDurations', by row and column as in a grid's gridRows rows,
// with a 21st column, afterStopColumn, for column 3 after C, K, P or T. A duration of 0 is the fricative's own.
const (
	fixedFricatives = "HVdxz"
	afterStopColumn = gridColumns
	gridRows        = 8
)

var fixedDurations = [len(fixedFricatives)][gridRows][gridColumns + 1]byte{
	{
		{8, 8, 0, 6, 7, 6, 0, 7, 6, 0, 7, 0, 0, 0, 0, 0, 0, 0, 0, 9, 6},
		{8, 8, 0, 6, 5, 6, 0, 7, 6, 0, 7, 0, 0, 0, 0, 0, 0, 0, 0, 9, 6},
		{7, 8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 9, 0},
		{7, 8, 0, 6, 7, 6, 0, 6, 6, 0, 7, 0, 0, 0, 0, 0, 0, 0, 0, 9, 6},
		{6, 8, 0, 5, 7, 6, 0, 5, 6, 0, 7, 0, 0, 0, 0, 0, 0, 0, 0, 9, 5},
		{6, 8, 0, 5, 5, 6, 0, 5, 6, 0, 7, 0, 0, 0, 0, 0, 0, 0, 0, 9, 5},
		{7, 8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 9, 0},
		{7, 8, 0, 5, 6, 6, 0, 5, 6, 0, 7, 0, 0, 0, 0, 0, 0, 0, 0, 9, 5},
	},
	{
		{9, 8, 0, 9, 10, 6, 8, 5, 7, 8, 6, 5, 6, 6, 6, 8, 5, 6, 0, 5, 6},
		{9, 8, 0, 9, 10, 8, 8, 7, 7, 8, 6, 5, 6, 6, 6, 18, 5, 6, 0, 9, 6},
		{9, 9, 19, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 9, 0},
		{9, 9, 0, 10, 11, 8, 9, 7, 6, 7, 6, 7, 9, 6, 5, 7, 8, 8, 0, 9, 6},
		{8, 6, 0, 8, 10, 6, 7, 5, 5, 7, 6, 6, 6, 6, 6, 6, 7, 7, 0, 5, 6},
		{8, 5, 0, 8, 10, 6, 7, 5, 5, 7, 6, 6, 7, 6, 6, 6, 6, 5, 0, 7, 6},
		{8, 8, 19, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 7, 0},
		{8, 4, 0, 11, 8, 7, 8, 5, 5, 7, 6, 6, 8, 6, 6, 6, 7, 7, 0, 7, 6},
	},
	{
		{6, 8, 0, 0, 0, 7, 6, 0, 10, 5, 7, 0, 0, 0, 0, 0, 0, 0, 0, 9, 6},
		{6, 8, 0, 0, 0, 7, 6, 0, 6, 7, 7, 0, 0, 0, 0, 0, 0, 0, 0, 9, 6},
		{6, 9, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 9, 0},
		{6, 9, 0, 0, 0, 8, 7, 0, 6, 7, 8, 0, 0, 0, 0, 0, 0, 0, 0, 9, 6},
		{6, 7, 0, 0, 0, 6, 6, 0, 10, 6, 5, 0, 0, 0, 0, 0, 0, 0, 0, 6, 6},
		{6, 7, 0, 0, 0, 6, 6, 0, 5, 6, 5, 0, 0, 0, 0, 0, 0, 0, 0, 6, 6},
		{6, 7, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 7, 0},
		{6, 7, 0, 0, 0, 7, 6, 0, 5, 5, 5, 0, 0, 0, 0, 0, 0, 0, 0, 7, 6},
	},
	{
		{6, 7, 0, 6, 6, 6, 5, 7, 7, 5, 6, 7, 7, 0, 6, 8, 8, 8, 0, 5, 6},
		{6, 5, 0, 6, 6, 7, 7, 7, 7, 5, 6, 7, 7, 0, 6, 8, 8, 8, 0, 5, 6},
		{8, 7, 13, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 5, 0},
		{8, 7, 0, 6, 9, 7, 7, 8, 8, 6, 7, 8, 8, 0, 7, 8, 8, 8, 0, 5, 6},
		{5, 7, 0, 5, 5, 5, 5, 6, 6, 5, 5, 6, 6, 0, 5, 8, 7, 7, 0, 5, 6},
		{5, 5, 0, 5, 5, 5, 5, 6, 6, 5, 5, 6, 6, 0, 5, 7, 7, 7, 0, 5, 6},
		{5, 6, 15, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 5, 0},
		{5, 6, 0, 5, 6, 5, 5, 6, 6, 5, 5, 7, 7, 0, 6, 7, 7, 7, 0, 5, 6},
	},
	{
		{6, 9, 0, 0, 0, 8, 0, 0, 7, 0, 0, 8, 10, 0, 9, 9, 9, 7, 0, 9, 6},
		{6, 9, 0, 0, 0, 8, 0, 0, 7, 0, 0, 8, 10, 0, 9, 9, 9, 7, 0, 9, 6},
		{6, 11, 18, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 11, 0},
		{6, 11, 0, 0, 0, 8, 0, 0, 7, 0, 0, 9, 10, 0, 10, 10, 10, 7, 0, 11, 6},
		{6, 11, 0, 0, 0, 8, 0, 0, 7, 0, 0, 7, 9, 0, 8, 8, 8, 8, 0, 8, 6},
		{6, 11, 0, 0, 0, 7, 0, 0, 7, 0, 0, 7, 9, 0, 8, 8, 8, 6, 0, 8, 6},
		{6, 10, 18, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 10, 0},
		{6, 10, 0, 0, 0, 7, 0, 0, 7, 0, 0, 8, 9, 0, 9, 9, 8, 7, 0, 10, 6},
	},
}

// fricativeSounds are what a fricative's duration depends on: the symbols around it as the duration sees them,
// the fricative's stress and place, and its column.
type fricativeSounds struct {
	symbol, previous, preceding, beforePreceding, following, afterFollowing byte
	affricate, stressed, lastWord, medial                                   bool
	column                                                                  int32
}

// FricativeDuration is the duration in frames of a fricative, H or d: its grid's value, or a fixed one for H, V,
// d, x and z, for the sounds around it, whether it is stressed, in the phrase's last word and inside its word,
// with exceptions for some sounds around it (prosody.md §6.10).
func (stage *Stage) FricativeDuration() int32 {
	context := &stage.Context
	node := stage.cursor().Scan
	symbol, next := context.Symbol, context.NextSymbol
	sounds := fricativeSounds{symbol: symbol, previous: context.PreviousSymbol, preceding: context.PrecedingSymbol,
		beforePreceding: context.BeforePrecedingSymbol, following: context.FollowingSymbol,
		afterFollowing: context.AfterFollowingSymbol}
	sounds.affricate = symbol == 's' && sounds.preceding == 'C' || symbol == 'z' && sounds.preceding == 'J'
	stage.skipPause(&sounds, node)
	sounds.stressed = node.Flags()&stressedFlag != 0
	sounds.lastWord = stage.scanForClass(true, 16, phraseBreak, -wordMark)
	sounds.medial = !stage.hasClass(next, wordMark) && !among(sounds.following, "p ,") && !stage.hasClass(next, boundary)
	if pause := after(node, 2); symbol == 'S' && pause != nil && pause.Symbol() == ' ' && pause.Value() > 20 {
		sounds.lastWord = true
	}
	sounds.column = stage.fricativeColumn(&sounds)
	row := int32(0)
	if sounds.medial {
		row++
	}
	if sounds.lastWord {
		row += lastWordRows
	}
	if !sounds.stressed {
		row += unstressedRow
	}
	grid := firstFricativeGrid + max(strings.IndexByte(fricativeGrids, symbol), 0)
	duration, exception := stage.fricativeException(&sounds)
	fixed := strings.IndexByte(fixedFricatives, symbol)
	fromGrid := false
	switch {
	case exception:
	case fixed >= 0:
		duration = int32(fixedDurations[fixed][row][sounds.column])
	default:
		fromGrid = true
		column := sounds.column
		if column == afterStopColumn {
			column = 3
		}
		duration = stage.gridValue(grid, row*gridColumns+column)
	}
	switch {
	case symbol == 'V' && sounds.following == 'X':
		duration += 4
	case symbol == 'V' && stage.hasClass(sounds.following, voiced) && stage.hasClass(sounds.following, fricative):
		duration -= 2
	case symbol == 'F' && sounds.column == 0 && sounds.lastWord && isWordMark(sounds.previous):
		duration += 2
	}
	if duration == 0 && fromGrid {
		fallbackRow := int32(0)
		if !sounds.stressed {
			fallbackRow = unstressedRow
		}
		duration = stage.gridValue(grid, fallbackRow*gridColumns+3)
	}
	if duration == 0 {
		duration = stage.ownDuration(symbol, sounds.stressed)
	}
	return max(duration, shortestByClass)
}

// skipPause lets S and Z see a nasal or approximant past a pause after them, S a Z past it and Z an S past a pause
// before it, and an affricate's frication a nasal or L past a pause, before a vowel.
func (stage *Stage) skipPause(sounds *fricativeSounds, node *pipeline.Node) {
	symbol, afterFollowing := sounds.symbol, sounds.afterFollowing
	if sounds.following == ' ' && (symbol == 'S' || symbol == 'Z') &&
		(afterFollowing == 'M' || afterFollowing == 'N' || stage.hasClass(afterFollowing, approximant)) ||
		sounds.following == ' ' && symbol == 'S' && afterFollowing == 'Z' {
		sounds.following = afterFollowing
	}
	if symbol == 'Z' && sounds.preceding == ' ' && sounds.beforePreceding == 'S' {
		sounds.preceding = 'S'
	}
	if (symbol == 's' && sounds.previous == 'C' || symbol == 'z' && sounds.previous == 'J') && sounds.following == ' ' &&
		(stage.hasClass(afterFollowing, nasal) || afterFollowing == 'L') && stage.inClass(after(node, 3), vowel) {
		sounds.following = afterFollowing
	}
}

// fricativeColumn is the column of a fricative's grid for the sounds around it.
func (stage *Stage) fricativeColumn(sounds *fricativeSounds) int32 {
	preceding, following := sounds.preceding, sounds.following
	afterMark := stage.hasClass(sounds.previous, wordMark)
	switch {
	case preceding == ' ':
		return 0
	case stage.hasClass(preceding, vowel) && stage.hasClass(following, vowel) && afterMark:
		return 19
	case stage.hasClass(preceding, vowel) && stage.hasClass(following, vowel):
		return 1
	case sounds.lastWord && !sounds.medial:
		return 2
	}
	switch preceding {
	case 'B', 'D', 'G', 'J', 'Q', 'q', 't':
		if afterMark {
			return 6
		}
		return 5
	case 'C', 'K', 'P', 'T':
		if afterMark {
			return 4
		}
		return afterStopColumn
	case 'F', 'H', 'S', 'X', 's':
		return 7
	case 'L', 'R', 'W', 'Y', 'j', 'l':
		return 10
	case 'M', 'N', '~':
		if stage.hasClass(following, closure) && stage.hasClass(following, consonant) {
			return 13
		}
		return 9
	case 'V', 'Z', 'd', 'x', 'z':
		return 8
	case 'm', 'n':
		return 9
	}
	switch following {
	case 'B', 'D', 'G', 'J', 'Q', 'q', 't':
		return 12
	case 'C', 'K', 'P', 'T':
		return 11
	case 'F', 'H', 'S', 'X', 's':
		return 14
	case 'L', 'R', 'W', 'Y', 'j', 'l':
		return 17
	case 'M', 'N', 'm', 'n', '~':
		return 16
	case 'V', 'Z', 'x', 'z':
		return 15
	}
	return 3
}

// fricativeException is a fricative's duration when the sounds around it give one instead of its grid's or its
// fixed one, and whether they do.
func (stage *Stage) fricativeException(sounds *fricativeSounds) (int32, bool) {
	symbol, stressed, lastWord, medial := sounds.symbol, sounds.stressed, sounds.lastWord, sounds.medial
	preceding, following := sounds.preceding, sounds.following
	consonantBefore := stage.hasClass(sounds.beforePreceding, consonant)
	consonantAfter := stage.hasClass(sounds.afterFollowing, consonant)
	switch sounds.column {
	case 2:
		if symbol == 's' && sounds.affricate {
			return 16, true
		}
	case afterStopColumn:
		if symbol == 'S' && !stage.hasClass(preceding, voiced) && stage.hasClass(preceding, closure) &&
			!stage.hasClass(following, voiced) && stage.hasClass(following, consonant) && !stressed && !lastWord && !medial {
			return 6, true
		}
		if symbol == 's' && sounds.affricate && medial {
			return affricateFrication(stage.hasClass(following, consonant) && (consonantBefore || consonantAfter),
				stressed, lastWord), true
		}
	case 4:
		if symbol == 'V' && stage.hasClass(preceding, closure) && stage.hasClass(preceding, consonant) && consonantBefore &&
			medial {
			switch {
			case !stressed && lastWord:
				return 6, true
			case !stressed:
				return 5, true
			case lastWord:
				return 9, true
			}
			return 7, true
		}
	case 5:
		if symbol == 'z' && sounds.affricate && (consonantBefore || consonantAfter) {
			switch {
			case !stressed && lastWord:
				return 7, true
			case !stressed:
				return 6, true
			case medial:
				return 7, true
			}
			return 0, true
		}
	case 7:
		if symbol == 'F' && stage.hasClass(preceding, fricative) && stage.hasClass(preceding, consonant) &&
			stage.hasClass(sounds.beforePreceding, closure) && consonantBefore && !stressed && lastWord && medial {
			return 7, true
		}
	case 9:
		if symbol == 'F' && preceding == 'N' && !stage.hasClass(sounds.previous, wordMark) && medial &&
			stage.hasClass(following, consonant) && stressed && !lastWord {
			return 9, true
		}
	case 11, 12:
		switch {
		case symbol == 'S' && stage.hasClass(following, alveolar) && stage.hasClass(following, closure) && consonantAfter &&
			!lastWord:
			switch {
			case !stressed:
				return 6, true
			case medial:
				return 9, true
			}
			return 0, true
		case symbol == 'V' && stage.hasClass(following, closure) && stage.hasClass(following, consonant) && consonantAfter &&
			!stressed && !lastWord:
			return 5, true
		}
	case 13:
		switch {
		case symbol == 'S' && consonantAfter && preceding == 'N' && stressed && medial && !lastWord &&
			!stage.hasClass(sounds.previous, wordMark):
			return 5, true
		case symbol == 'z' && consonantAfter && !stressed && !medial:
			return 7, true
		}
	case 14:
		if symbol == 'F' && stage.hasClass(following, fricative) && medial && stage.hasClass(sounds.afterFollowing, fricative) &&
			!stressed && !lastWord {
			return 5, true
		}
	}
	return 0, false
}

// affricateFrication is the duration of the frication of C inside its word after the stop, in a cluster of
// consonants or not.
func affricateFrication(cluster, stressed, lastWord bool) int32 {
	switch {
	case cluster && stressed && lastWord:
		return 7
	case cluster && (stressed || lastWord):
		return 6
	case cluster:
		return 5
	case stressed:
		return 9
	case lastWord:
		return 8
	}
	return 7
}
