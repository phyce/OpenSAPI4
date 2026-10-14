package prosody

import (
	"strings"

	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
)

// stops are the stops in the order of their durations. A stop's durations are by row, 1 after a sound of its own
// word plus 2 in the phrase's last word, and by column, from the sounds around it (stopColumn); afterPauseColumn
// is never in rows 1 and 3, which hold 0 there.
const (
	stops            = "BCDGJKPT"
	stopRows         = 4
	stopColumns      = 21
	afterPauseColumn = 11
	otherColumn      = 20
)

var stopDurations = [len(stops)][stopRows][stopColumns]byte{
	{
		{7, 10, 10, 5, 8, 7, 7, 6, 7, 8, 9, 10, 7, 7, 5, 7, 6, 6, 6, 6, 7},
		{7, 7, 7, 5, 9, 7, 7, 6, 7, 9, 8, 0, 7, 7, 4, 7, 6, 6, 6, 6, 7},
		{9, 8, 9, 6, 10, 11, 7, 7, 7, 8, 9, 10, 8, 8, 4, 7, 8, 7, 8, 8, 11},
		{7, 7, 8, 5, 8, 11, 7, 7, 7, 8, 8, 0, 8, 9, 4, 7, 8, 7, 8, 8, 11},
	},
	{
		{8, 7, 8, 5, 6, 11, 5, 6, 6, 4, 6, 10, 3, 5, 3, 7, 5, 7, 8, 6, 11},
		{8, 7, 8, 5, 6, 11, 5, 6, 6, 3, 5, 0, 3, 5, 3, 7, 5, 7, 8, 6, 11},
		{8, 7, 8, 5, 6, 11, 5, 6, 6, 3, 6, 10, 3, 5, 3, 7, 6, 7, 8, 6, 11},
		{8, 7, 8, 5, 6, 11, 5, 6, 6, 4, 5, 0, 3, 5, 3, 7, 6, 7, 8, 6, 11},
	},
	{
		{8, 8, 8, 5, 6, 3, 5, 3, 7, 7, 7, 11, 7, 6, 4, 7, 6, 6, 6, 6, 3},
		{7, 8, 5, 4, 5, 3, 5, 3, 7, 5, 6, 0, 7, 6, 3, 7, 6, 6, 6, 6, 3},
		{8, 9, 8, 5, 6, 8, 5, 4, 7, 8, 8, 11, 8, 7, 5, 8, 6, 7, 6, 7, 8},
		{7, 9, 8, 5, 6, 8, 5, 4, 7, 5, 7, 0, 7, 7, 3, 8, 6, 7, 6, 7, 8},
	},
	{
		{9, 9, 8, 5, 7, 6, 6, 6, 6, 7, 7, 9, 5, 7, 4, 7, 7, 6, 7, 6, 6},
		{7, 7, 7, 5, 6, 6, 6, 6, 6, 6, 6, 0, 4, 7, 4, 7, 7, 6, 7, 6, 6},
		{10, 9, 7, 5, 7, 11, 6, 6, 6, 8, 7, 9, 6, 7, 5, 8, 8, 7, 9, 8, 11},
		{7, 6, 6, 5, 6, 11, 6, 6, 6, 7, 6, 0, 7, 8, 3, 8, 8, 7, 9, 8, 11},
	},
	{
		{8, 7, 8, 5, 6, 11, 5, 6, 6, 6, 5, 11, 2, 5, 3, 7, 6, 7, 6, 6, 11},
		{8, 7, 8, 5, 6, 11, 5, 6, 6, 4, 5, 0, 2, 4, 3, 7, 6, 7, 4, 6, 11},
		{8, 7, 8, 5, 6, 11, 5, 6, 6, 6, 8, 11, 2, 6, 3, 7, 6, 7, 6, 6, 11},
		{8, 7, 8, 5, 6, 11, 5, 6, 6, 6, 6, 0, 2, 5, 2, 7, 6, 7, 5, 6, 11},
	},
	{
		{8, 7, 8, 5, 7, 7, 4, 8, 8, 7, 7, 11, 6, 7, 6, 7, 7, 7, 8, 10, 7},
		{7, 7, 6, 5, 6, 7, 4, 8, 8, 6, 6, 0, 5, 8, 4, 7, 7, 7, 8, 10, 7},
		{9, 8, 7, 7, 8, 11, 5, 9, 9, 6, 8, 11, 6, 8, 5, 7, 8, 8, 8, 11, 11},
		{8, 7, 8, 5, 8, 11, 5, 9, 9, 6, 7, 0, 6, 8, 5, 7, 8, 8, 8, 11, 11},
	},
	{
		{10, 9, 8, 5, 8, 8, 5, 9, 9, 8, 8, 10, 6, 10, 6, 8, 6, 9, 7, 10, 8},
		{8, 7, 8, 5, 7, 8, 5, 9, 9, 8, 9, 0, 7, 10, 6, 8, 6, 9, 7, 10, 8},
		{10, 9, 9, 6, 8, 12, 6, 9, 10, 9, 8, 10, 7, 9, 7, 10, 7, 10, 7, 10, 12},
		{9, 7, 9, 6, 7, 12, 6, 9, 10, 7, 8, 0, 8, 10, 7, 10, 7, 10, 7, 10, 12},
	},
	{
		{8, 7, 8, 6, 6, 6, 4, 4, 6, 7, 6, 10, 6, 6, 5, 5, 6, 6, 8, 5, 6},
		{6, 7, 7, 5, 5, 6, 4, 4, 6, 5, 6, 0, 6, 7, 5, 5, 6, 6, 8, 5, 6},
		{7, 9, 8, 6, 6, 10, 5, 4, 6, 8, 6, 10, 6, 7, 5, 5, 6, 5, 11, 6, 10},
		{8, 7, 8, 5, 6, 10, 5, 4, 6, 7, 6, 0, 6, 8, 5, 5, 6, 5, 11, 6, 10},
	},
}

// releasedStops have release frames, by row as a fricative's, 1 after a sound of its own word plus 2 in the
// phrase's last word and 4 when unstressed, and by group: the groups of releaseGroups by the sound after the
// stop, otherGroup for any other, and afterPauseGroup after a pause, never in the odd rows.
const (
	releasedStops   = "PTK"
	releaseRows     = 8
	otherGroup      = 5
	afterPauseGroup = 6
)

var releaseGroups = [...]string{"4Ei|", "OUbcguwy", "Aaek", "3@Iforv", "LRWY"}

var stopReleases = [len(releasedStops)][releaseRows][afterPauseGroup + 1]byte{
	{
		{5, 3, 2, 3, 3, 1, 4},
		{5, 3, 3, 3, 3, 1, 0},
		{5, 3, 2, 2, 3, 2, 4},
		{3, 3, 2, 2, 3, 2, 0},
		{1, 2, 1, 2, 3, 1, 3},
		{2, 2, 2, 2, 3, 1, 0},
		{5, 2, 2, 2, 2, 2, 3},
		{2, 2, 2, 2, 3, 2, 0},
	},
	{
		{5, 4, 4, 4, 7, 1, 4},
		{5, 5, 4, 4, 3, 1, 0},
		{6, 5, 4, 3, 7, 1, 4},
		{5, 5, 4, 3, 9, 1, 0},
		{4, 2, 2, 2, 3, 1, 2},
		{3, 4, 2, 2, 3, 1, 0},
		{5, 3, 3, 2, 3, 1, 2},
		{4, 2, 2, 2, 3, 1, 0},
	},
	{
		{4, 4, 4, 3, 5, 1, 5},
		{5, 5, 4, 3, 3, 1, 0},
		{5, 5, 4, 3, 3, 1, 5},
		{5, 4, 5, 3, 3, 1, 0},
		{3, 3, 4, 3, 3, 1, 3},
		{3, 4, 3, 2, 3, 1, 0},
		{3, 3, 4, 2, 3, 1, 3},
		{3, 3, 3, 2, 3, 1, 0},
	},
}

// stopSounds are what a stop's duration depends on: the symbols around it, whether it is stressed, in the
// phrase's last word and at its word's start, after a pause or a word mark.
type stopSounds struct {
	symbol, previous, next, preceding, beforePreceding, following, afterFollowing byte
	stressed, lastWord, initial                                                   bool
}

func isStressed(node *pipeline.Node) bool {
	return node != nil && node.Flags()&stressedFlag != 0
}

// StopDuration is the duration in frames of a stop: its value for the sounds around it, whether it is in the
// phrase's last word and at its word's start, less what a cluster of consonants takes; P, T and K also get their
// release frames (prosody.md §6.11).
func (stage *Stage) StopDuration() int32 {
	context := &stage.Context
	node := stage.cursor().Scan
	symbol := context.Symbol
	sounds := stopSounds{symbol: symbol, previous: context.PreviousSymbol, next: context.NextSymbol,
		preceding: context.PrecedingSymbol, beforePreceding: context.BeforePrecedingSymbol,
		following: context.FollowingSymbol, afterFollowing: context.AfterFollowingSymbol}
	sounds.stressed = isStressed(node)
	sounds.lastWord = stage.scanForClass(true, 16, phraseBreak, -wordMark, byClass)
	sounds.initial = sounds.preceding == ' ' || isWordMark(sounds.previous)
	preceding, following := sounds.preceding, sounds.following
	row := int32(0)
	if !sounds.initial {
		row++
	}
	if sounds.lastWord {
		row += lastWordRows
	}
	column := stage.stopColumn(&sounds)
	duration := int32(stopDurations[strings.IndexByte(stops, symbol)][row][column])
	if symbol == 'D' && preceding == 'N' && following == 'p' && sounds.lastWord {
		duration = 5
	}
	if stage.hasClass(sounds.previous, nasal) && (stage.hasClass(sounds.next, stop) || stage.hasClass(sounds.next, fricative)) {
		switch {
		case symbol == 'P':
			duration -= 2
		case symbol == 'T' && !sounds.lastWord:
			duration--
		}
	}
	if among(symbol, releasedStops) {
		node.Data[releaseOffset] = stage.releaseFrames(&sounds)
	}
	if (stage.hasClass(preceding, vowel) || isGlideOrJ(preceding)) && stage.hasClass(following, vowel) &&
		!isStressed(stage.cursor().First) {
		switch symbol {
		case 'D':
			duration = 6
		case 'T':
			duration = 8
		}
	} else if symbol != 'C' && symbol != 'J' &&
		(stage.hasClass(preceding, consonant) && !stage.hasClass(preceding, nasal) || preceding == 'l' || preceding == 'n') {
		duration -= stage.clusterFrames(&sounds)
	}
	if (symbol == 'D' || symbol == 'T') && sounds.initial && preceding == 'S' && isGlideOrJ(following) {
		duration++
	}
	switch {
	case symbol == 'T' && following == 'p':
		duration = 9
	case (symbol == 'C' || symbol == 'J') && following == 'p':
		duration++
	}
	return max(duration, shortestByClass)
}

// stopColumn is the column of a stop's durations for the sounds around it.
func (stage *Stage) stopColumn(sounds *stopSounds) int32 {
	preceding, following := sounds.preceding, sounds.following
	switch {
	case preceding == ' ':
		return afterPauseColumn
	case stage.hasClass(preceding, vowel) || isGlideOrJ(preceding):
		switch {
		case among(following, "Ebiu"):
			return 0
		case isGlideOrJ(following):
			return 2
		case stage.hasClass(following, vowel) && following != 'p':
			return 1
		}
	case isGlideOrJ(following) && stage.hasClass(preceding, nasal):
		return 3
	case isGlideOrJ(following):
		return 4
	}
	switch following {
	case 'p':
		return 5
	case 'd':
		return 6
	case 'l':
		return 7
	case 'n':
		return 8
	}
	switch preceding {
	case 'C', 'K', 'P', 'T', 'q':
		return 9
	case 'F', 'H', 'S', 'X', 's':
		return 10
	case 'B', 'D', 'G', 'J', 't':
		return 12
	case 'V', 'Z', 'x', 'z':
		return 13
	case 'M', 'N', '~':
		if preceding == 'N' && sounds.beforePreceding == '|' && sounds.initial {
			return 8
		}
		return 14
	}
	switch following {
	case 'C', 'K', 'P', 'T', 'q':
		return 15
	case 'F', 'H', 'S', 'X', 's':
		return 16
	case 'B', 'D', 'G', 'J', 't':
		return 17
	case 'V', 'Z', 'x', 'z':
		return 18
	case 'M', 'N', '~':
		return 19
	}
	return otherColumn
}

// releaseFrames are the frames of a P, T or K's release, for the sounds around it. A stop between vowels has
// none when the phrase's first and last nodes are stressed.
func (stage *Stage) releaseFrames(sounds *stopSounds) byte {
	symbol, preceding, following := sounds.symbol, sounds.preceding, sounds.following
	cursor := stage.cursor()
	if stage.hasClass(preceding, vowel) && isStressed(cursor.Done) && stage.hasClass(following, vowel) &&
		isStressed(cursor.First) {
		return 0
	}
	group := otherGroup
	for index, symbols := range releaseGroups {
		if among(following, symbols) {
			group = index
			break
		}
	}
	if preceding == ' ' {
		group = afterPauseGroup
	}
	row := int32(0)
	if !sounds.initial {
		row++
	}
	if sounds.lastWord {
		row += lastWordRows
	}
	if !sounds.stressed {
		row += unstressedRow
	}
	frames := stopReleases[strings.IndexByte(releasedStops, symbol)][row][group]
	switch {
	case symbol == 'T' && following == 'l':
		frames = 2
	case following == 'N':
		frames--
	case isWordMark(sounds.next):
		switch {
		case symbol == 'T' && preceding == 'S' && stage.hasClass(following, vowel) && isStressed(stage.Context.Following):
			frames = 0
		case stage.hasClass(following, vowel) && symbol != 'K':
			frames = 1
		case isGlideOrJ(following):
			frames = 2
		}
	case preceding == ' ':
		if isGlideOrJ(following) {
			frames += 2
		}
	case preceding == 'S' && !isWordMark(sounds.previous):
		frames = 1
		if symbol == 'T' {
			frames = 0
		}
	}
	if symbol == 'P' && following == 'l' && frames != 0 {
		frames--
	}
	return frames
}

// clusterFrames are the frames a stop B, D, G, K, P or T loses after a consonant, l or n, in a cluster of
// consonants.
func (stage *Stage) clusterFrames(sounds *stopSounds) int32 {
	symbol, preceding, following := sounds.symbol, sounds.preceding, sounds.following
	beforePreceding, afterFollowing := sounds.beforePreceding, sounds.afterFollowing
	consonantAfter := stage.clusterConsonant(following)
	if consonantAfter && (stage.hasClass(beforePreceding, consonant) && !stage.hasClass(beforePreceding, nasal) ||
		stage.hasClass(afterFollowing, consonant)) {
		return 2
	}
	switch {
	case preceding == 'n' && consonantAfter && among(symbol, "PTK"):
		return 1
	case !isGlideOrJ(preceding) && stage.hasClass(beforePreceding, nasal):
		return 1
	case preceding == 'n' && consonantAfter:
		return 2
	case afterFollowing == 'l' || afterFollowing == 'n' ||
		stage.hasClass(following, consonant) && stage.clusterConsonant(afterFollowing):
		return 2
	case !consonantAfter && !stage.hasClass(beforePreceding, consonant) && beforePreceding != 'l' && beforePreceding != 'n':
		return 0
	}
	return 1
}

// clusterConsonant reports whether a symbol is a consonant but a glide or J, l or n.
func (stage *Stage) clusterConsonant(symbol byte) bool {
	return stage.hasClass(symbol, consonant) && !isGlideOrJ(symbol) || symbol == 'l' || symbol == 'n'
}
