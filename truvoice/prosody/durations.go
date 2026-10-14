package prosody

import (
	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
)

// A duration's limits in frames, and the shortest duration of a vowel, a sonorant, a fricative or a stop.
const (
	shortestDuration = 2
	longestDuration  = 55
	shortestByClass  = 3
)

// The rate whose base durations take its percent and that the rate's scaling leaves alone, and the slowest rate
// that merges a pair of consonants.
const (
	baseRate    = 13
	mergingRate = 7
)

// The input flag that times the phonemes by their shortest durations alone.
const shortestTimingFlag = 0x10

// The value of a word mark that emphasizes its word.
const emphasisMark = 10

// The emphasis of a word: none, a content word's and a function word's.
const (
	noEmphasis = iota
	contentEmphasis
	functionEmphasis
)

// The word classes of a content word and a verb.
const (
	contentWord = 0
	verb        = 6
)

// The places in a node's data of byte 0x10, a stop's release frames, and byte 0x18, set on a phoneme merged from
// two.
const (
	releaseOffset = 0x08
	mergedOffset  = 0x10
)

// AssignDuration gives the current node its duration in frames (prosody.md §6.1).
func (stage *Stage) AssignDuration() {
	context := &stage.Context
	node := stage.cursor().Scan
	durations := stage.durations(context.Symbol)
	stage.Inherent = durations.Inherent * 10
	stage.Shortest = durations.Shortest * 10
	stage.Percents[0] = 100
	switch {
	case !context.Phoneme:
		stage.MarkerDuration()
	case stage.PresetDuration != 0:
		node.SetValue(uint32(stage.PresetDuration))
	case stage.cursor().InputFlags&shortestTimingFlag != 0:
		node.SetValue(uint32(stage.SimpleDuration()))
	default:
		node.SetValue(uint32(stage.KlattBaseDuration()))
		node.SetValue(uint32(stage.SegmentDuration()))
	}
}

// durations returns a symbol's inherent and shortest durations; symbols of 128 and up have none.
func (stage *Stage) durations(symbol byte) datapack.Duration {
	if symbol >= datapack.SymbolCount {
		return datapack.Duration{}
	}
	return stage.tables.Durations[symbol]
}

// MarkerDuration notes a word mark's emphasis, and gives any other node that isn't a phoneme no duration.
func (stage *Stage) MarkerDuration() {
	node := stage.cursor().Scan
	if !stage.hasClass(node.Symbol(), wordMark) {
		node.SetValue(0)
		return
	}
	switch {
	case node.Value() != emphasisMark:
		stage.Emphasis = noEmphasis
	case node.Second() != contentWord && node.Second() != verb:
		stage.Emphasis = functionEmphasis
	default:
		stage.Emphasis = contentEmphasis
	}
}

// SimpleDuration is a phoneme's duration by its shortest one alone (prosody.md §6.1).
func (stage *Stage) SimpleDuration() int32 {
	node := stage.cursor().Scan
	shortest := stage.durations(node.Symbol()).Shortest
	if node.Symbol() == ' ' {
		return max(shortest>>2, 4)
	}
	duration := shortest * 17 / 20
	if node.Flags()&stressedFlag == 0 {
		duration >>= 1
	}
	return max(duration, shortestDuration)
}

// KlattBaseDuration is a phoneme's base duration: its shortest one and a percent of the rest, by the phoneme's
// place in its phrase and word and the sounds around it (prosody.md §6.2). A syllabic phoneme has none.
func (stage *Stage) KlattBaseDuration() int32 {
	context := &stage.Context
	symbol := context.Symbol
	if stage.hasClass(symbol, syllabic) {
		return 0
	}
	if stage.Inherent == stage.Shortest {
		return stage.durations(symbol).Inherent
	}
	count := 0
	if !stage.ProsodyOff && stage.scanForClass(true, 6, phraseBreak, -syllabic) {
		count = 1
		stage.Percents[count] = 160
		if symbol == 'q' {
			stage.Percents[count] = 150
		}
	}
	if stage.scanForClass(false, 7, syllabic, -wordMark) {
		count++
		stage.Percents[count] = 85
	}
	if stage.Inherent <= stage.Shortest {
		return stage.Inherent / 10
	}
	preceding := context.PrecedingSymbol
	percent, byFollowing := int32(100), true
	if !stage.hasClass(symbol, laryngeal) && !stage.hasClass(preceding, syllabic) &&
		!stage.hasClass(preceding, affricate) && !stage.hasClass(preceding, laryngeal) {
		percent = 90
		if stage.hasClass(symbol, approximant) {
			percent = 60
		}
		if !context.Stressed {
			byFollowing = false
			percent -= 20
			if context.Closure && stage.hasClass(preceding, closure) &&
				stage.articulator(symbol) == stage.articulator(preceding) {
				percent = 40
			}
		}
	}
	if byFollowing && !context.Sonorant && !context.Affricate && context.Following != nil {
		following := context.FollowingSymbol
		if !stage.hasClass(following, syllabic) && preceding != 'S' && !stage.hasClass(following, laryngeal) {
			percent = 70
			if context.Fricative && (stage.hasClass(following, closure) ||
				stage.hasClass(following, fricative) && !stage.hasClass(preceding, affricate)) {
				percent = 30
			}
		}
	}
	stage.Percents[count+1] = percent
	product := int32(100)
	for index := count + 1; index > 0; index-- {
		product = product * stage.Percents[index] / 100
	}
	if stage.cursor().Rate == baseRate {
		product = product / 2 * stage.tables.DurationPercents[baseRate] / 100 * 2
	}
	stage.Percents[0] = product
	duration := (stage.Inherent-stage.Shortest)*product/100 + stage.Shortest
	if context.Stressed && preceding == 'S' && symbol == 'q' &&
		(context.FollowingSymbol == 'L' || context.FollowingSymbol == 'R') {
		duration = duration * 50 / 100
	}
	return duration / 10
}

// articulator returns the articulator of a phoneme's symbol.
func (stage *Stage) articulator(symbol byte) int32 {
	index, found := stage.parameters.Index(symbol)
	if !found {
		index = datapack.PauseIndex
	}
	return stage.parameters.Phonemes[index].Articulator
}

// SegmentDuration is a phoneme's duration from its base one (prosody.md §6.3): it merges a pair of consonants,
// times the phoneme by its class, adds a stop's release before it, and scales the duration by emphasis, the
// phrase and the rate, to between 2 and 55 frames.
func (stage *Stage) SegmentDuration() int32 {
	context := &stage.Context
	beforePreceding, following := context.BeforePrecedingSymbol, context.FollowingSymbol
	node := stage.cursor().Scan
	duration := int32(node.Value())
	if stage.MergeGeminate() {
		stage.LoadContext()
		duration = int32(node.Value())
	}
	inFunctionWord := stage.Geminate && stage.inFunctionWord(beforePreceding)
	symbol := context.Symbol
	afterFunctionMark := symbolOf(node.Previous) == '%'
	switch {
	case stage.hasClass(symbol, vowel):
		duration = stage.VowelDuration()
	case stage.hasClass(symbol, stop) && symbol != 'q':
		extra := int32(0)
		if stage.Geminate && !afterFunctionMark && !inFunctionWord {
			extra = stage.geminateStopFrames(symbol, beforePreceding, following)
		}
		duration = stage.StopDuration() + extra
	case stage.hasClass(symbol, fricative) || symbol == 'H' || symbol == 'd':
		extra := int32(0)
		if stage.Geminate && !afterFunctionMark {
			extra = stage.geminateFricativeFrames(symbol)
		}
		duration = stage.FricativeDuration() + extra
	case stage.hasClass(symbol, sonorant) && symbol != 'h':
		duration = stage.SonorantDuration()
	case symbol == ' ':
		duration = stage.PauseDuration()
	case symbol == 'q':
		if stage.hasClass(context.FollowingSymbol, nasal) {
			duration = 7
		}
	case symbol == flap:
		duration = 3
	}
	if preceding := context.PrecedingSymbol; preceding == 'P' || preceding == 'T' || preceding == 'K' {
		duration += int32(context.Preceding.Data[releaseOffset])
	}
	duration = stage.EmphasisDuration(duration)
	if stage.Contour == exclamationContour {
		switch stage.SentenceWords {
		case 1:
			duration = duration * 144 / 100
		case 2:
			duration = duration * 123 / 100
		}
	}
	if stage.cursor().Rate != baseRate {
		duration = stage.RateScale(duration)
	}
	return min(max(duration, shortestDuration), longestDuration)
}

// inFunctionWord reports whether the phoneme before the previous one, or the symbol when there is none, is a
// function word's mark, or comes after one in the list with no content word's mark between.
func (stage *Stage) inFunctionWord(symbol byte) bool {
	previous := stage.previousPhoneme(stage.Context.Previous)
	if previous != nil {
		symbol = previous.Symbol()
	}
	for symbol != '&' {
		if symbol == '%' {
			return true
		}
		if previous == nil || previous.Previous == nil {
			return false
		}
		previous = previous.Previous
		symbol = previous.Symbol()
	}
	return false
}

func isGlide(symbol byte) bool {
	return symbol == 'R' || symbol == 'L' || symbol == 'W' || symbol == 'Y'
}

// geminateStopFrames are the frames a stop merged from a pair gets, by the sounds before and after the pair.
func (stage *Stage) geminateStopFrames(symbol, beforePreceding, following byte) int32 {
	vowelBefore := stage.hasClass(beforePreceding, vowel)
	vowelAfter := stage.hasClass(following, vowel)
	switch symbol {
	case 'P', 'K':
		switch {
		case !vowelBefore:
			return 6
		case isGlide(following):
			if symbol == 'K' {
				return 9
			}
			return 6
		case !vowelAfter:
			return 6
		case !stage.hasClass(following, high):
			if symbol == 'P' {
				return 10
			}
			return 7
		}
		if symbol == 'P' {
			return 8
		}
	case 'D':
		if vowelBefore && vowelAfter {
			return 7
		}
	case 'B':
		switch {
		case (vowelBefore || isGlide(beforePreceding)) && vowelAfter && stage.hasClass(following, high):
			return 8
		case !vowelBefore:
			return 6
		case vowelAfter:
			return 8
		case isGlide(following):
			return 5
		}
	}
	return 6
}

// geminateFricativeFrames are the frames a fricative merged from a pair gets.
func (stage *Stage) geminateFricativeFrames(symbol byte) int32 {
	switch symbol {
	case 'F':
		return 8
	case 'x':
		return 6
	case 'S':
		if stage.hasClass(stage.Context.FollowingSymbol, vowel) && stage.hasClass(stage.Context.PrecedingSymbol, vowel) {
			return 6
		}
		return 5
	}
	return 4
}

// MergeGeminate merges the current consonant with the sound before it in its word when the two are said as one,
// at a rate of 7 or more, and reports whether it did (prosody.md §6.4).
func (stage *Stage) MergeGeminate() bool {
	context := &stage.Context
	node := stage.cursor().Scan
	before := stage.previousSound(node)
	previous := symbolOf(before)
	symbol := context.Symbol
	stage.Geminate = false
	if !stage.hasClass(symbol, consonant) || stage.cursor().Rate < mergingRate {
		return false
	}
	pair := symbol == previous && node.Flags()&glottalFlag == 0 ||
		(previous == 'S' || previous == 'Z') && symbol == 's' || previous == 'Z' && symbol == 'S' ||
		previous == 'V' && symbol == 'F' || previous == 'L' && symbol == 'j' || previous == 'B' && symbol == 'P'
	if !pair || symbol == 'S' && previous == 'S' && isWordMark(context.PreviousSymbol) &&
		stage.hasClass(context.NextSymbol, consonant) {
		return false
	}
	stage.Geminate = (symbol == previous || previous == 'B' && symbol == 'P' || previous == 'Z' && symbol == 'S') &&
		(stage.hasClass(symbol, sonorant) || stage.hasClass(symbol, fricative) ||
			stage.hasClass(symbol, stop) && symbol != 'C' && symbol != 'J' && symbol != 'q')
	duration := int32(before.Value()) + int32(node.Value())
	if previous == 'V' && symbol == 'F' {
		duration -= 4
	}
	node.SetValue(uint32(min(duration, longestDuration)))
	node.Data[mergedOffset] = 1
	node.SetSecond(before.Second())
	stage.Pipeline.FreeNode(before, false)
	return true
}

// EmphasisDuration lengthens a phoneme of stress 3, and a phoneme of an emphasized word by its kind (prosody.md
// §6.5).
func (stage *Stage) EmphasisDuration(duration int32) int32 {
	context := &stage.Context
	if stress(stage.cursor().Scan) == 3 {
		duration = duration * 130 / 100
	}
	if stage.Emphasis == noEmphasis {
		return duration
	}
	switch {
	case context.Syllabic && context.Stressed:
		duration = duration * 200 / 100
	case context.Sonorant && context.Stressed && !context.Syllabic:
		duration = duration * 150 / 100
	case context.Syllabic || context.Sonorant || context.Fricative:
		duration = duration * 125 / 100
	}
	if stage.Emphasis == functionEmphasis {
		duration = duration * 150 / 100
	}
	return duration
}

// RateScale scales a duration by the rate's percent of 91 % of it, to between the phoneme's shortest duration
// and 55 frames.
func (stage *Stage) RateScale(duration int32) int32 {
	shortest := stage.Shortest / 10
	scaled := stage.tables.DurationPercents[stage.cursor().Rate] * (duration * 91 / 100) / 100
	if scaled < shortest {
		return shortest
	}
	return min(scaled, longestDuration)
}

// PauseDuration is a pause's duration: the rate's pause percent of its base duration, in steps of 4 frames and
// at least 4. A pause node holds at most 50 frames: while the pool has 4 nodes free, the pause's first 50s go in
// pause nodes of their own before it (prosody.md §6.6).
func (stage *Stage) PauseDuration() int32 {
	cursor := stage.cursor()
	node := cursor.Scan
	duration := stage.tables.PausePercents[cursor.Rate] / 4 * int32(node.Value()) / 100 * 4
	duration = max(duration, 4)
	value := duration
	if duration > pauseLength {
		for stage.Pipeline.Free >= 4 {
			pause := stage.Pipeline.NewNode(node, false, pipeline.Segment, ' ')
			duration -= pauseLength
			pause.SetValue(pauseLength)
			pause.SetSecond(pauseSecond)
			if duration <= pauseLength {
				break
			}
		}
		value = min(duration, pauseLength)
	}
	node.SetValue(uint32(value))
	if symbolOf(node.Next) != '&' {
		node.SetSecond(pauseSecond)
	}
	stage.LoadContext()
	return duration
}
