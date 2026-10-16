package prosody

import (
	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
)

// The word marks' values the scan notes, and the word class of a question word.
const (
	firstContourMark  = 6
	secondContourMark = 7
	thirdContourMark  = 8
	questionWord      = 13
)

// The phrase's kind of a phrase without an end of its own.
const unendedPhrase = 2

// The flag bit the lexical stage's rules set on some word marks, the one a break gives a vowel after it, and the
// stress bits of stress 1.
const (
	flaggedMark = 0x80
	glottalFlag = 0x40
	firstStress = 0x08
)

// originalOffset is the place in a node's data of byte 0x18: a flap's original phoneme. The rules also test it on
// a word mark.
const originalOffset = 0x10

const flap = 't'

// ScanPhrase scans the window for the end of the next phrase, unless the stage is walking one, and reports what
// ended it, or scanning when the window ran out first (prosody.md §3.2).
func (stage *Stage) ScanPhrase() int32 {
	if stage.ScanResult == walking {
		return walking
	}
	shared := stage.Shared
	cursor := stage.cursor()
	if stage.ScanResult != scanning {
		stage.Phonemes -= stage.Padding
		stage.Padding = 0
		shared.Pending, shared.Counted, shared.Scanned = 0, 0, 0
		shared.InPhrase = false
		stage.Words = 0
		stage.Marks = [4]int32{-1, -1, -1, 0}
		if stage.ScanResult == sentenceEnd || stage.ScanResult == commaEnd {
			stage.ResetPitch()
			stage.Phonemes = 0
			stage.SentenceWords = 0
			shared.Held = false
			stage.FirstPhrase = true
		}
	}
	for cursor.Done != nil && cursor.Done == cursor.First && cursor.Done.Type() != pipeline.Phoneme {
		stage.Pipeline.ExecuteControl()
		stage.Pipeline.ProsodyOff = false
		stage.PhraseLimit = phraseLimit
		next := stage.Pipeline.Next(cursor.Done)
		cursor.Done, cursor.Scan, cursor.First = next, next, next
	}
	shared.LastNode = cursor.First
	stage.ScanResult = scanning
	var last byte
	for node := cursor.First; node != nil && stage.ScanResult == scanning; node = cursor.First {
		last = node.Symbol()
		shared.Scanned++
		if node.Type() == pipeline.Phoneme && stage.hasClass(last, phoneme) {
			shared.InPhrase = true
			shared.Pending++
			stage.listPhoneme(node, last)
		} else {
			stage.scanNode(node, last)
		}
		if stage.ScanResult != scanning {
			break
		}
		if shared.Scanned < scanLimit && shared.Counted < stage.PhraseLimit {
			cursor.First = stage.Pipeline.Next(cursor.First)
			continue
		}
		cursor.First = stage.Pipeline.Previous(shared.LastNode)
		last = shared.LastNode.Symbol()
		if isWordMark(last) {
			stage.Words--
		}
		if !shared.Held && !stage.NoPhrasing {
			stage.PlacePhraseBreaks(0)
		}
		stage.ScanResult = brokenOff
	}
	stage.chooseContour(last)
	return stage.ScanResult
}

// listPhoneme lists a phoneme the scan counts, but a pause, and a vowel's stress after it: 2 for stress 1, 1 for
// stress 2 or 3 (engine.md §6.2).
func (stage *Stage) listPhoneme(node *pipeline.Node, symbol byte) {
	if !stage.Pipeline.Listing {
		return
	}
	if symbol != ' ' {
		stage.Pipeline.List(symbol)
	}
	if !stage.hasClass(symbol, vowel) {
		return
	}
	switch stress(node) {
	case 1:
		stage.Pipeline.List('2')
	case 2, 3:
		stage.Pipeline.List('1')
	}
}

// scanNode notes a node of the scan that isn't a phoneme.
func (stage *Stage) scanNode(node *pipeline.Node, symbol byte) {
	shared := stage.Shared
	shared.Counted += shared.Pending
	stage.Phonemes += shared.Pending
	shared.Pending = 0
	shared.LastNode = node
	switch node.Type() {
	case pipeline.Phoneme:
		if isWordMark(symbol) && stage.Words < MaximumWords {
			switch node.Value() {
			case firstContourMark:
				stage.Marks[0] = stage.Phonemes
			case secondContourMark:
				stage.Marks[1] = stage.Phonemes
			case thirdContourMark:
				stage.Marks[2] = stage.Phonemes
			case loweringMark:
				stage.Marks[3] = stage.Phonemes
			}
			stage.WordMarks[stage.Words] = node
			stage.Words++
			stage.Pipeline.List(symbol)
		} else if stage.hasClass(symbol, phraseBreak) {
			stage.endPhrase(node, symbol)
		}
	case pipeline.Control:
		stage.scanControl(node, symbol)
	}
}

// endPhrase ends the phrase at a break.
func (stage *Stage) endPhrase(node *pipeline.Node, symbol byte) {
	shared := stage.Shared
	switch {
	case symbol == '.' || symbol == '?' || symbol == '!':
		stage.ScanResult = sentenceEnd
		stage.SentencePhonemes = stage.Phonemes
		stage.Pipeline.List(symbol)
		for previous := stage.Pipeline.Previous(node); stage.inClass(previous, -syllabic); previous = stage.Pipeline.Previous(previous) {
			if previous.Type() == pipeline.Phoneme && stage.hasClass(previous.Symbol(), phoneme) {
				stage.Phonemes--
			}
		}
	case symbol == ',' && stage.NoPhrasing:
		stage.ScanResult = commaEnd
	}
	if !shared.Held && !stage.NoPhrasing && stage.cursor().Rate < fastRate && symbol != '!' {
		stage.PlacePhraseBreaks(symbol)
	}
	shared.InPhrase = false
	shared.Held = false
	stage.SentenceWords += stage.Words
	stage.Words = 0
}

// scanControl runs the scan's part of a control command.
func (stage *Stage) scanControl(node *pipeline.Node, letter byte) {
	shared := stage.Shared
	switch letter {
	case 'C':
		if !shared.Held && !stage.NoPhrasing {
			stage.PlacePhraseBreaks(0)
		}
		stage.ScanResult = continued
	case 'N':
		stage.Pipeline.ProsodyOff = node.Second()&prosodyOffFlag != 0
		stage.PhraseLimit = phraseLimit
		if stage.Pipeline.ProsodyOff {
			stage.PhraseLimit = flatPhraseLimit
		}
	case 'P':
		stage.NoPhrasing = node.Value() == 0
		if !shared.InPhrase {
			shared.Held = true
		} else if stage.Pipeline.Free > 3 {
			inserted := stage.Pipeline.NewNode(node, false, pipeline.Phoneme, '\\')
			shared.Scanned++
			stage.cursor().First = stage.Pipeline.Previous(inserted)
		}
	case 'r', 'v':
		stage.SlowSpeech = node.Value() < slowRates
	}
}

// prosodyOffFlag is the bit of an N command's second value, flag 0x2000, that turns the prosody off.
const prosodyOffFlag = 0x20

// chooseContour chooses the phrase's contour by its marks; last is the symbol the scan ended at.
func (stage *Stage) chooseContour(last byte) {
	first, second, third := stage.Marks[0], stage.Marks[1], stage.Marks[2]
	switch {
	case first < 0 && second < 0:
		stage.Contour = plainContour
		if third >= 0 {
			stage.Contour = thirdMarkContour
		}
	case second >= 0 && first < 0:
		stage.Contour = secondMarkContour
		if second == stage.Phonemes {
			stage.Contour = plainContour
		}
	case second >= 0 && second <= first:
		switch {
		case second != 0:
			stage.Contour = markPairContour
		case first != 0:
			stage.Contour = firstMarkContour
		default:
			stage.Contour = plainContour
		}
	case second >= 0:
		switch {
		case first != 0:
			stage.Contour = markRiseContour
		case second == stage.Phonemes:
			stage.Contour = plainContour
		default:
			stage.Contour = secondMarkContour
		}
	default:
		stage.Contour = plainContour
		if first != 0 {
			stage.Contour = firstMarkContour
		}
	}
	if last == '!' {
		stage.exclamationMarks()
	}
}

// exclamationMarks gives an exclamation contour 19, its first mark the phonemes before the phrase's last stressed
// vowel, less 1, and its second half of that.
func (stage *Stage) exclamationMarks() {
	cursor := stage.cursor()
	stage.Contour = exclamationContour
	stage.Marks[0] = stage.SentencePhonemes
	if node := cursor.First; node != nil {
		for node != cursor.Done {
			if stage.hasClass(node.Symbol(), vowel) && node.Flags()&stressedFlag != 0 {
				break
			}
			if stage.hasClass(node.Symbol(), phoneme) {
				stage.Marks[0]--
			}
			node = stage.Pipeline.Previous(node)
			if stage.Marks[0] == 0 || node == nil {
				break
			}
		}
	}
	marked := stage.Marks[0]
	stage.Marks[0] = marked - 1
	if stage.Marks[0] == 1 {
		stage.Marks[1] = 0
	} else {
		stage.Marks[1] = marked / 2
	}
}

// PlacePhraseBreaks ends the phrase whose end the scan found, the symbol end or 0: it breaks it where the words'
// classes say, and puts the P command of its kind before it; in slow speech it puts pauses between some of its
// words (prosody.md §3.3).
func (stage *Stage) PlacePhraseBreaks(end byte) {
	if stage.Words < 1 {
		return
	}
	last := int32(0)
	for at := int32(2); at < stage.Words-2; at++ {
		switch stage.breakBefore(at) {
		case at:
			stage.InsertPhraseBreak(last, ',', at)
			last = at
			at += 2
		case at + 1:
			stage.InsertPhraseBreak(last, ',', at+1)
			last = at + 1
			at += 3
		}
	}
	list := stage.Pipeline
	if list.Free < 3 {
		return
	}
	phrase := list.NewNode(stage.WordMarks[last], false, pipeline.Control, 'P')
	cursor := stage.cursor()
	if phrase.Next == cursor.Done {
		cursor.Done = phrase
	}
	switch end {
	case '.', '!':
		phrase.SetValue(fallingPhrase)
	case ',':
		phrase.SetValue(commaPhrase)
	case '?':
		question := stage.WordMarks[0]
		if question.Second() != questionWord {
			question = stage.WordMarks[last]
		}
		if question.Second() != questionWord {
			phrase.SetValue(questionPhrase)
			break
		}
		phrase.SetValue(fallingPhrase)
		if word := question.Next.Next; word.Type() == pipeline.Phoneme && stage.hasClass(word.Symbol(), phoneme) {
			word.SetFlags(word.Flags()&^stressBits | stressedFlag | firstStress)
		}
	default:
		phrase.SetValue(unendedPhrase)
	}
	stage.markContinued(phrase)
	if stage.SlowSpeech {
		stage.placeSlowPauses()
	}
}

// markContinued gives a P command the second value 1 unless it is the sentence's first.
func (stage *Stage) markContinued(phrase *pipeline.Node) {
	if stage.FirstPhrase {
		phrase.SetSecond(0)
		stage.FirstPhrase = false
	} else {
		phrase.SetSecond(1)
	}
}

// breakBefore returns the word mark before which the words' classes put a break, looking at the word marks from
// at − 2 to at + 1, or −1.
func (stage *Stage) breakBefore(at int32) int32 {
	beforePrevious, previous, mark, next := stage.WordMarks[at-2], stage.WordMarks[at-1], stage.WordMarks[at], stage.WordMarks[at+1]
	previousClass, class, nextClass := previous.Second(), mark.Second(), next.Second()
	flagged := mark.Flags()&flaggedMark != 0
	if previousClass == 0 && (class == 8 || class == 11 || class == 12 ||
		!flagged && (class == 9 || class == 6 && next.Previous.Symbol() != '~')) {
		return at
	}
	startsWord := mark.Symbol() == '%' && !flagged && previous.Symbol() == '&'
	if startsWord && previousClass != 6 && (class == 5 || class == 14 && nextClass == 5) {
		return at
	}
	word := mark.Next.Symbol()
	classBreak := class == 13 || beforePrevious.Second() != 10 && previousClass != 6 && previousClass != 15 &&
		(class == 10 && mark.Data[originalOffset] != '-' && word != 'T' && word != 't' && word != 'a' ||
			class == 2 && word != 'x' && word != 'B' ||
			class == 3 && (word == 'F' || word == 's'))
	switch {
	case startsWord && classBreak:
		if (class == 10 || class == 3) && nextClass == 14 || class == 10 && nextClass == 10 {
			return at + 1
		}
		return at
	case previousClass == 15 && !flagged && (class == 10 || class == 5 || class == 14):
		return at
	}
	return -1
}

// InsertPhraseBreak puts a break, the symbol, before the word mark at, and a P command of a comma's phrase before
// the word mark start, where the phrase it ends starts.
func (stage *Stage) InsertPhraseBreak(start int32, symbol byte, at int32) {
	list := stage.Pipeline
	if list.Free <= 2 {
		return
	}
	mark := stage.WordMarks[at]
	inserted := list.NewNode(mark, false, pipeline.Phoneme, symbol)
	for _, neighbour := range []*pipeline.Node{inserted.Next, inserted.Previous} {
		if neighbour.Symbol() == flap {
			neighbour.SetSymbol(neighbour.Data[originalOffset])
		}
	}
	stage.AdjustAtBreak(list.Previous(inserted), false)
	stage.AdjustAtBreak(list.Next(mark), true)
	phrase := list.NewNode(stage.WordMarks[start], false, pipeline.Control, 'P')
	phrase.SetValue(commaPhrase)
	stage.markContinued(phrase)
	if cursor := stage.cursor(); phrase.Next == cursor.Done {
		cursor.Done = phrase
	}
}

// AdjustAtBreak adjusts the phoneme before an inserted break, or the one after it: a stop before it gets a
// release, a vowel after it a glottal onset at pitch 0, and a 'd' after it becomes 'H'.
func (stage *Stage) AdjustAtBreak(node *pipeline.Node, after bool) {
	symbol := node.Symbol()
	cursor := stage.cursor()
	if !after {
		switch symbol {
		case 'P', 'T', 'K', 'B', 'D', 'G':
			if cursor.Rate < fastRate && !stage.hasClass(symbol, affricate) {
				stage.Pipeline.NewNode(node, true, pipeline.Phoneme, 'p')
				stage.SentencePhonemes++
			}
		}
		return
	}
	if stage.hasClass(symbol, vowel) && symbol != 'U' && cursor.Pitch == 0 {
		node.SetFlags(node.Flags() | glottalFlag)
		return
	}
	if symbol == 'd' {
		node.SetSymbol('H')
	}
}

// placeSlowPauses puts a pause, the symbol ')', before some of the phrase's words in slow speech (prosody.md
// §3.4).
func (stage *Stage) placeSlowPauses() {
	list := stage.Pipeline
	rate := stage.cursor().Rate
	var previousMark byte
	end := stage.WordMarks[stage.Words-1]
	for node := stage.WordMarks[0]; node != end; node = list.Next(node) {
		if list.Free < 4 {
			return
		}
		symbol := node.Symbol()
		switch {
		case node.Type() == pipeline.Control && symbol == 'r':
			rate = (int32(node.Value()&0xff) - 46) >> 3
		case node.Type() == pipeline.Control && symbol == 'v':
			rate = int32(node.Value() & 0xff)
		case node.Type() == pipeline.Phoneme && isWordMark(symbol):
			if symbol == '%' && previousMark == '&' && rate < slowRates && list.Previous(node).Symbol() != ',' && stage.slowPause(rate) {
				list.NewNode(node, false, pipeline.Phoneme, ')')
			}
			previousMark = symbol
		}
	}
}

// slowPause takes the next chance of a pause between words at the rate.
func (stage *Stage) slowPause(rate int32) bool {
	shared := stage.Shared
	slot := shared.SlowPauses & 0xf
	shared.SlowPauses++
	var pattern int32
	if rate < 0 {
		pattern = stage.tables.PausePercents[datapack.RateCount+rate]
	} else {
		pattern = stage.tables.SlowPausePatterns[rate]
	}
	return uint32(pattern)>>slot&1 != 0
}
