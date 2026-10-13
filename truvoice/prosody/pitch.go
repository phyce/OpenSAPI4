package prosody

import "github.com/phyce/OpenSAPI4/truvoice/pipeline"

// The phrase's contours (prosody.md §3).
const (
	plainContour       = 0
	firstMarkContour   = 6
	secondMarkContour  = 7
	thirdMarkContour   = 8
	markPairContour    = 15
	markRiseContour    = 16
	exclamationContour = 19
)

// The phrase's kinds, the low byte of the cursor's phrase setting (prosody.md §3), and its high byte for a
// phrase that goes on from the one before.
const (
	fallingPhrase   = 1
	commaPhrase     = 3
	questionPhrase  = 4
	risingPhrase    = 5
	levelPhrase     = 6
	continuedPhrase = 0x100
)

// The kinds of consonant after the current phoneme (prosody.md §4.2).
const (
	voicelessFricative = 1
	voicelessStop      = 2
	voicedFricative    = 3
	otherFollowing     = 4
)

// The characters a node gets (pipeline.md §4).
const (
	whisper  = 1
	monotone = 2
)

// The pitches, in Hz.
const (
	lowestPitch   = 50
	highestPitch  = 500
	lowestFall    = 46
	fallStep      = 8
	raisedPercent = 108
)

// The word marks' values the pitch looks at.
const (
	raisingMark  = 12
	loweringMark = 17
)

// divide is a quotient truncated toward zero. The original divides by zero nowhere the recorded runs reach
// (prosody.md §8); the port gives 0 there.
func divide(dividend, divisor int32) int32 {
	if divisor == 0 {
		return 0
	}
	return dividend / divisor
}

// ResetPitch starts a sentence's pitch afresh.
func (stage *Stage) ResetPitch() {
	stage.FollowingConsonant = otherFollowing
	stage.Rise = false
	stage.Position = 0
	stage.Fall = 0
	stage.Nucleus = nil
	stage.AfterStress, stage.AtNucleus, stage.PhraseEnd, stage.WordEnd = false, false, false, false
	stage.Raised = false
}

// PitchContext notes where the current phoneme stands for its pitch: the phrase's nucleus, whether the phoneme
// is in its word's last syllable, in the phrase's last, and at the nucleus, and the consonant after it
// (prosody.md §4.2).
func (stage *Stage) PitchContext() {
	context := &stage.Context
	if stage.hasClass(context.Symbol, wordMark) {
		stage.WordEnd = false
		return
	}
	if stage.hasClass(context.Symbol, phraseBreak) {
		stage.Fall = 0
		stage.AfterStress, stage.AtNucleus, stage.PhraseEnd, stage.WordEnd = false, false, false, false
		stage.Nucleus = nil
		return
	}
	if !context.Syllabic {
		return
	}
	cursor := stage.cursor()
	if stage.Nucleus == nil {
		stage.findNucleus(cursor)
	}
	if stage.scanForClass(true, 9, phraseBreak, -syllabic, byClass) {
		stage.PhraseEnd = true
		stage.WordEnd = true
	} else if stage.scanForClass(true, 10, wordMark, -syllabic, byClass) {
		stage.WordEnd = true
	}
	if cursor.Scan == stage.Nucleus {
		stage.AtNucleus = true
	}
	node := context.Next
	stage.FollowingConsonant = otherFollowing
	if stage.inClass(node, sonorant) {
		node = stage.nextPhoneme(node)
		if stage.inClass(node, -sonorant) && stage.inClass(node, phoneme) {
			node = context.Next
		}
	}
	switch {
	case stage.inClass(node, voiced):
		if stage.inClass(node, fricative) || stage.inClass(node, affricate) {
			stage.FollowingConsonant = voicedFricative
		}
	case stage.inClass(node, fricative):
		stage.FollowingConsonant = voicelessFricative
	case stage.inClass(node, stop):
		stage.FollowingConsonant = voicelessStop
	}
}

// findNucleus finds the last phoneme of stress 2 or 3 from the current node up to the first in class break,
// within the phrase the stage has scanned; there is none without a break.
func (stage *Stage) findNucleus(cursor *pipeline.Cursor) {
	for node := cursor.Scan; ; {
		if node.Type() == pipeline.Phoneme {
			if stage.hasClass(node.Symbol(), phoneme) {
				if stress(node) >= 2 {
					stage.Nucleus = node
				}
			} else if stage.hasClass(node.Symbol(), phraseBreak) {
				return
			}
		}
		if node == cursor.First {
			break
		}
		if node = stage.Pipeline.Next(node); node == nil {
			break
		}
	}
	stage.Nucleus = nil
}

// AssignPitch gives the current phoneme its pitch, the node's second value in units of 2 Hz, and its character;
// at a word mark it notes the marks of values 12 and 17 (prosody.md §4.3).
func (stage *Stage) AssignPitch() {
	cursor := stage.cursor()
	node := cursor.Scan
	context := &stage.Context
	afterNext := byte(0)
	if after := stage.nextPhoneme(context.Next); after != nil {
		afterNext = after.Symbol()
	}
	if !context.Phoneme {
		if !stage.hasClass(node.Symbol(), wordMark) {
			return
		}
		switch node.Value() {
		case raisingMark:
			stage.Raised = true
		case loweringMark:
			stage.Lowered = true
		default:
			if stage.Lowered && stage.AfterStress && !stage.PhraseEnd {
				stage.Lowered = false
			}
		}
		return
	}
	stage.Position++
	node.SetCharacter(byte(stage.Pipeline.Character))
	switch stage.Pipeline.Character {
	case whisper:
		node.SetSecond(0)
		return
	case monotone:
		node.SetSecond(byte(cursor.Pitch >> 1))
		return
	}
	if stage.Fall != 0 {
		stage.Fall = max(stage.Fall-fallStep, lowestFall)
		node.SetSecond(byte(stage.Fall >> 1))
		return
	}
	pitchRange := stage.tables.PitchRanges[cursor.Voice]
	excursion := multiply(cursor.Pitch/3, pitchRange)
	if stage.Contour == exclamationContour {
		if stage.Position > stage.Marks[1] {
			excursion *= 4
		} else {
			excursion *= 6
		}
	}
	if stage.Phonemes == 0 {
		stage.Phonemes = 1
	}
	pitch := excursion + cursor.Pitch
	if drop, declining := stage.declination(cursor, excursion); declining {
		pitch -= drop
	}
	if stage.Raised {
		pitch = pitch * raisedPercent / 100
	}
	scaled := multiply(pitch, pitchRange)
	step := scaled>>2 - scaled>>4
	if !stage.AfterStress {
		if cursor.Phrase&^0xff == continuedPhrase || stage.Lowered {
			step -= step >> 2
		} else {
			step += step >> 4
		}
	}
	if stage.Nucleus != nil && stress(stage.Nucleus) == 3 {
		step <<= 3
	}
	following := context.Following
	if following != nil && stress(following) >= 2 && !stage.AtNucleus ||
		stage.Position == 1 && context.Vowel && context.Stressed ||
		stage.Marks[3] != 0 && stage.Position >= stage.Marks[3] && stage.PhraseEnd {
		stage.Rise = true
	}
	if following == stage.Nucleus && following != nil && byte(cursor.Phrase) != levelPhrase {
		pitch += step
		if stress(following) == 3 {
			pitch += 2 * step
		}
	}
	switch stress(node) {
	case 1:
		pitch += step >> 1
	case 2, 3:
		stage.AfterStress = true
		if node == stage.Nucleus && byte(cursor.Phrase) != levelPhrase {
			node.SetFlags(node.Flags() | nucleusFlag)
			stage.Rise = false
		} else {
			pitch += step
			if stress(node) == 3 {
				pitch += 8 * step
			}
		}
	}
	if stage.Rise {
		pitch += scaled >> 2
	}
	pitch = stage.consonantPitch(node, pitch, afterNext)
	if !stage.PhraseEnd {
		if stage.hasClass(context.Symbol, high) {
			pitch += 3
		}
		node.SetSecond(byte(min(max(pitch, lowestPitch), highestPitch) >> 1))
		return
	}
	if stage.Lowered {
		node.SetSecond(byte(min(max(pitch, lowestPitch), highestPitch) >> 1))
		return
	}
	switch byte(cursor.Phrase) {
	case fallingPhrase:
		pitch -= scaled >> 4
		if stage.AtNucleus && stage.hasClass(context.Symbol, vowel) && stage.hasClass(context.NextSymbol, phraseBreak) {
			pitch -= 22
		}
	case commaPhrase:
		pitch += scaled >> 2 >> 3
	case questionPhrase, risingPhrase:
		pitch = 2*cursor.Pitch - cursor.Pitch>>3
	}
	stage.Fall = min(max(pitch, lowestPitch), highestPitch)
	node.SetSecond(byte(stage.Fall >> 1))
}

// declination returns how far the phrase's contour has brought the pitch down at the current phoneme, from the
// excursion above the voice's pitch, and whether the contour declines at all.
func (stage *Stage) declination(cursor *pipeline.Cursor, excursion int32) (int32, bool) {
	position, phonemes := stage.Position, stage.Phonemes
	toward := func(mark int32) int32 {
		if position > mark {
			return divide((position-mark)*excursion, phonemes-mark)
		}
		return divide((mark-position)*excursion, mark)
	}
	first, second := stage.Marks[0], stage.Marks[1]
	switch stage.Contour {
	case plainContour:
		return divide(excursion*position, phonemes), true
	case firstMarkContour:
		if stage.Marks[0] == 0 {
			stage.Marks[0] = phonemes
		}
		return toward(stage.Marks[0]), true
	case secondMarkContour:
		if position > second {
			return divide((phonemes-position)*excursion, phonemes-second), true
		}
		return divide(excursion*position, second), true
	case thirdMarkContour:
		cursor.Phrase = levelPhrase
		if stage.Marks[2] == 0 {
			stage.Marks[2] = phonemes
		}
		return toward(stage.Marks[2]), true
	case markPairContour:
		cursor.Phrase = levelPhrase
		fallthrough
	case exclamationContour:
		switch {
		case position <= second:
			return divide(excursion*position, second), true
		case position > first:
			return divide((position-first)*excursion, phonemes-first), true
		}
		return divide((first-position)*excursion, first-second), true
	case markRiseContour:
		cursor.Phrase = commaPhrase
		switch {
		case position <= first:
			return divide((first-position)*excursion, first), true
		case position > second:
			return divide((phonemes-position)*excursion, phonemes-second), true
		}
		return divide((position-first)*excursion, second-first), true
	}
	return 0, false
}

// consonantPitch adjusts the pitch for the current phoneme's sound and the sounds around it.
func (stage *Stage) consonantPitch(node *pipeline.Node, pitch int32, afterNext byte) int32 {
	context := &stage.Context
	symbol := context.Symbol
	following := context.Following
	if !stage.hasClass(symbol, voiced) {
		if following != nil && stage.hasClass(following.Symbol(), vowel) &&
			(context.Preceding == nil || context.Preceding.Symbol() == ' ') && (symbol == 'T' || symbol == 'P' || symbol == 'K') {
			pitch += 20
		}
	} else if !context.Stressed && following != nil && !stage.hasClass(following.Symbol(), vowel) && stress(following) == 0 &&
		(!stage.hasClass(symbol, consonant) || !stage.hasClass(symbol, approximant) || !stage.hasClass(symbol, nasal)) ||
		node == stage.Nucleus {
		pitch -= 3
		if !context.Sonorant {
			pitch -= 5
		}
	}
	if symbol == 'N' && stage.hasClass(context.PrecedingSymbol, vowel) && context.Preceding.Flags()&stressedFlag == 0 &&
		(context.FollowingSymbol == 'T' || context.FollowingSymbol == 'q') && isWordMark(afterNext) &&
		(stage.hasClass(context.AfterFollowingSymbol, sonorant) ||
			stage.hasClass(context.AfterFollowingSymbol, vowel) && context.AfterFollowing.Flags()&stressedFlag != 0) {
		pitch -= 55
	}
	return pitch
}
