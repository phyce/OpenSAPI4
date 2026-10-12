package parameters

import (
	"strings"

	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
)

// vowels are the phonemes 0 to 23, the vowels without p, as the per-phoneme rules list them.
const vowels = "45AEIOUabcefgikruwy3@ov|"

// The class bits only the per-phoneme rules test (parameters.md §8.1): of class byte 0 and 1.
const (
	syllabicClass  = 0x01
	consonantClass = 0x20
)

// PhonemeRules is action 16: the rules of the phonemes B D G J M m R W X Y Z j l s x z (parameters.md §8).
func (generator *Generator) PhonemeRules() {
	switch generator.cursor().Scan.Symbol() {
	case 'B':
		generator.rulesForB()
	case 'D':
		generator.rulesForD()
	case 'G':
		generator.rulesForG()
	case 'J':
		generator.rulesForJ()
	case 'M', 'm':
		generator.rulesForM()
	case 'R':
		generator.rulesForR()
	case 'W':
		generator.rulesForW()
	case 'X':
		generator.rulesForX()
	case 'Y':
		generator.rulesForY()
	case 'Z':
		generator.rulesForZ()
	case 'j', 'l':
		generator.rulesForLowerJAndL()
	case 's':
		generator.rulesForLowerS()
	case 'x':
		generator.rulesForLowerX()
	case 'z':
		generator.rulesForLowerZ()
	}
}

func isVowel(symbol byte) bool {
	return strings.IndexByte(vowels, symbol) >= 0
}

func stressed(node *pipeline.Node) bool {
	return node.Flags()&stressedFlag != 0
}

func (generator *Generator) setTargets(first int, targets ...int32) {
	for index, target := range targets {
		generator.Segments[first+index].Target = target
	}
}

// groupPrevious sorts the previous phoneme into its group (parameters.md §8.2). A phoneme of none leaves the
// group as it was.
func (generator *Generator) groupPrevious() {
	previous := generator.cursor().Done.Symbol()
	for group, members := range []string{"EA", "yI|iea", "UbfO", "uvwo@", "4k3rgc5", "MBVPFWLjRH "} {
		if strings.IndexByte(members, previous) >= 0 {
			generator.PreviousGroup = int32(group + 1)
			return
		}
	}
	if generator.has(previous, 1, consonantClass) && previous != 'd' {
		generator.PreviousGroup = 7
	}
}

func (generator *Generator) rulesForB() {
	cursor := generator.cursor()
	if cursor.First.Symbol() == 'L' && cursor.Done.Symbol() == 'S' {
		generator.Segments[3].Target = 70
	}
}

func (generator *Generator) rulesForD() {
	cursor := generator.cursor()
	next := cursor.First
	previousSymbol, nextSymbol := cursor.Done.Symbol(), next.Symbol()
	bursts := &generator.BurstLevels
	switch {
	case generator.has(nextSymbol, 1, consonantClass):
		switch {
		case previousSymbol == 'j':
			if generator.has(nextSymbol, 2, closureClass) && !generator.has(nextSymbol, 0, nasalClass) ||
				generator.has(nextSymbol, 0, fricativeClass) && nextSymbol != 'Z' || nextSymbol == ' ' {
				bursts[1] = 60
			} else if nextSymbol != 'Z' {
				bursts[0] = 57
			}
		case previousSymbol == ' ' && generator.has(nextSymbol, 0, nasalClass):
			bursts[1] = 55
		case nextSymbol == 'n':
			bursts[0] = 50
		case nextSymbol == 'Z':
			bursts[1] = 57
		default:
			bursts[0] = 127
			if generator.has(nextSymbol, 0, fricativeClass) {
				bursts[1] = 54
				if generator.has(previousSymbol, 0, nasalClass) {
					bursts[1] = 51
				}
			} else {
				bursts[1] = 57
				if previousSymbol == ' ' {
					bursts[1] = 55
				}
			}
		}
	case !generator.has(nextSymbol, 2, vowelClass):
	case nextSymbol == 'p':
		if previousSymbol == 'N' {
			bursts[1] = 55
			generator.setTargets(bypassTrack, 0, 250, 1834, 2622, 3469, 143, 366, 166)
		}
	default:
		_, nextPhoneme := generator.phoneme(nextSymbol)
		place := nextPhoneme.Place
		switch place {
		case 0:
			generator.Segments[bypassTrack].Target = 62
		case 1:
			if previousSymbol == 'S' {
				generator.Segments[bypassTrack].Target = 55
			}
		case 3:
			generator.Segments[bypassTrack].Target = 65
			if nextSymbol == '3' && (stressed(next) || previousSymbol != 'j') {
				generator.Segments[bypassTrack].Target = 50
			}
		}
		if !stressed(next) {
			switch {
			case previousSymbol == 'j':
				bursts[0], bursts[1] = 53, 52
			case place == 1:
				bursts[0], bursts[1] = 45, 55
			case place == 0:
				bursts[0], bursts[1] = 127, 52
				if generator.has(previousSymbol, 2, closureClass) && !generator.has(previousSymbol, 0, nasalClass) ||
					generator.has(previousSymbol, 0, fricativeClass) {
					bursts[1] = 55
				}
			}
		} else {
			switch {
			case previousSymbol == 'S':
				bursts[0], bursts[1], bursts[2] = 127, 58, 48
			case place == 0:
				bursts[0], bursts[1] = 0, 55
			case place == 1:
				bursts[0], bursts[1] = 45, 55
			}
		}
	}
	if nextSymbol == 'R' && previousSymbol == 'S' && stressed(cursor.Scan) {
		bursts[1] = 60
	}
}

func (generator *Generator) rulesForG() {
	cursor := generator.cursor()
	generator.groupPrevious()
	if cursor.Done.Symbol() != ' ' {
		generator.Weights[firstFormant] = 32472
		generator.Segments[firstFormant].Back = 3
	}
	next := cursor.First.Symbol()
	switch {
	case next == 'o':
		generator.Segments[firstVoiceTrack].Target = 8
	case generator.has(next, 2, vowelClass):
		generator.Segments[firstVoiceTrack].Target = 0
	case !generator.has(next, 2, closureClass) && generator.has(next, 0, syllabicClass):
		switch generator.PreviousGroup {
		case 1:
			generator.setTargets(3, 62, 62, 80, 80, 80)
		case 2:
			generator.setTargets(3, 62, 62, 80, 65, 60)
			generator.setTargets(firstFormant+1, 2200, 2400)
		case 3:
			generator.setTargets(3, 80, 75, 40, 72, 50)
			generator.setTargets(firstFormant+1, 2050, 2250)
		case 4:
			generator.setTargets(firstFormant+1, 1964, 2164)
		case 5:
			generator.setTargets(3, 65, 75, 40, 72, 50)
		}
	default:
		switch generator.PreviousGroup {
		case 3:
			generator.setTargets(firstFormant+1, 1600, 1800)
		case 4, 5:
			generator.setTargets(firstFormant+1, 1684, 1884)
		}
	}
}

func (generator *Generator) rulesForJ() {
	cursor := generator.cursor()
	previous, next := cursor.Done.Symbol(), cursor.First.Symbol()
	if next == 'z' && previous == ' ' || !generator.has(previous, 0, voicedClass) {
		generator.Segments[voicingTrack].Target = 0
	}
	if generator.has(previous, 0, voicedClass) || previous == ' ' {
		generator.BurstLevels[0], generator.BurstLevels[1] = 48, 64
	}
}

func (generator *Generator) rulesForM() {
	cursor := generator.cursor()
	previous, next := cursor.Done.Symbol(), cursor.First.Symbol()
	generator.groupPrevious()
	if isVowel(next) {
		return
	}
	group := generator.PreviousGroup
	if group == 1 || group == 2 || previous == 'Y' {
		generator.Weights[firstFormant+1], generator.Weights[firstFormant+2] = 16400, 26240
		return
	}
	generator.Weights[firstFormant+1], generator.Weights[firstFormant+2] = 26240, 9840
	if group == 5 || previous == 'R' {
		generator.Segments[firstFormant+2].Target = 1750
	}
}

func (generator *Generator) rulesForR() {
	cursor := generator.cursor()
	nextNode := cursor.First
	next := nextNode.Symbol()
	segments := &generator.Segments
	switch cursor.Done.Symbol() {
	case 'G':
		if !isVowel(next) {
			segments[voicingTrack].Target = 55
			generator.setTargets(firstFormant+1, 1140, 1400)
		}
	case 'K':
		if !isVowel(next) {
			generator.setTargets(voicingTrack, 0, 56, 51, 65, 0, 55, 60, 60)
			segments[firstFormant+1].Target = 1404
			segments[fourthFormant].Target = 2544
		}
	case 'P':
		if next == 'e' {
			segments[3].Target = 50
			generator.setTargets(firstFormant, 458, 1300, 1816, 2968)
		}
		if generator.has(next, 2, vowelClass) {
			if !stressed(nextNode) {
				generator.setTargets(firstFormant+1, 996, 1472)
			} else {
				segments[bypassTrack].Target = 54
			}
		}
	case 'X':
		if next == 'E' {
			generator.setTargets(firstFormant+1, 1157, 1490, 3010, 54, 247, 230)
		}
	case 's':
		if symbolOf(cursor.Done.Previous) == 'C' {
			generator.setTargets(firstFormant+1, 1292, 1760)
		}
	default:
		if isVowel(next) {
			break
		}
		_, nextPhoneme := generator.phoneme(next)
		switch nextPhoneme.Place {
		case 0:
			segments[voicingTrack].Target = 60
		case 2:
			segments[voicingTrack].Target = 60
			generator.setTargets(firstFormant+1, 1140, 1440)
		default:
			second := &segments[firstFormant+1].Target
			*second -= multiply(*second, 8200)
			*second += multiply(nextPhoneme.Formants[1], 8200)
			segments[firstFormant+2].Target = *second + 400
		}
	}
	if next == 'l' {
		generator.setTargets(firstFormant, 376, 1032, 1671, 2757)
	}
}

func (generator *Generator) rulesForW() {
	cursor := generator.cursor()
	switch previous := cursor.Done.Symbol(); {
	case previous == 'K':
		generator.Segments[firstFormant+1].Target = 800
	case previous == ' ' && cursor.First.Symbol() == 'E':
		generator.Segments[firstBandwidth+2].Target = 250
	}
}

func (generator *Generator) rulesForX() {
	cursor := generator.cursor()
	if generator.has(cursor.Done.Symbol(), 2, vowelClass) {
		generator.Weights[firstFormant+1], generator.Weights[firstFormant+2] = 26240, 26240
	}
	if cursor.First.Symbol() == 'R' {
		frication := &generator.Segments[fricationTrack]
		frication.Back, frication.BackTarget, frication.Onset, frication.Forward = 0, 25, 25, frication.Frames
	}
}

func (generator *Generator) rulesForY() {
	if generator.cursor().Done.Symbol() == 'G' {
		generator.setTargets(firstFormant+1, 2036, 2592)
	}
}

func (generator *Generator) rulesForZ() {
	cursor := generator.cursor()
	previous, next := cursor.Done.Symbol(), cursor.First.Symbol()
	segments := &generator.Segments
	switch {
	case next == 'I' || next == 'a':
		segments[fricationTrack].Target = 64
	case !generator.has(previous, 2, closureClass):
		segments[fricationTrack].Target = 69
	default:
		segments[fricationTrack].Target = 60
	}
	segments[firstVoiceTrack].Forward, segments[firstVoiceTrack+1].Forward = 3, 3
	if !isVowel(next) {
		if !generator.has(previous, 2, vowelClass) {
			segments[voicingTrack].Target = 0
			generator.setTargets(4, 50, 73, 70, 70)
			generator.setTargets(firstVoiceTrack, 15, 15)
			segments[fricationTrack].Forward = 3
			if !generator.has(next, 2, vowelClass) && next != ' ' {
				segments[pitchTrack].Mode = defaultMode
				generator.Weights[pitchTrack] = 16400
				segments[pitchTrack].Target = 80
			}
		} else {
			segments[voicingTrack].Target = 56
			segments[4].Target = 60
			generator.setTargets(6, 67, 63)
			segments[firstFormant].Target = 296
			generator.setTargets(firstVoiceTrack, 8, 14)
		}
	}
	if generator.has(next, 2, closureClass) && generator.has(next, 0, voicedClass) {
		generator.setTargets(firstVoiceTrack, 8, 15)
	}
}

func (generator *Generator) rulesForLowerJAndL() {
	cursor := generator.cursor()
	current := cursor.Scan
	previous, next := cursor.Done.Symbol(), cursor.First.Symbol()
	segments := &generator.Segments
	if current.Symbol() == 'j' {
		switch previous {
		case 'A':
			generator.setTargets(firstFormant, 459, 810, 2682, 3621)
		case 'E':
			generator.setTargets(firstFormant, 378, 869, 2724, 3715)
		case 'a':
			generator.setTargets(firstFormant, 400, 873, 2602, 3588)
		case 'b':
			generator.setTargets(firstFormant, 391, 810, 2967, 3880)
		case 'e':
			generator.setTargets(firstFormant, 422, 843, 2679, 3750, 78, 79, 107)
		case 'y':
			if generator.has(next, 2, vowelClass) {
				generator.setTargets(firstFormant, 410, 1174, 2412, 3596)
				for track := firstFormant; track <= fourthFormant; track++ {
					generator.Weights[track] = 3280
					segments[track].Forward, segments[track].Back = 4, 2
				}
			} else {
				generator.setTargets(firstFormant, 548, 900, 2768, 3344)
				for track := firstFormant; track < fourthFormant; track++ {
					generator.Weights[track] = 3280
				}
				segments[firstFormant+2].Forward = int32(current.Value())
			}
		}
		return
	}
	if next == ' ' {
		generator.setTargets(firstFormant, 532, 790, 2646, 3377)
		for track := firstFormant; track <= fourthFormant; track++ {
			segments[track].Forward = int32(current.Value())
		}
	} else {
		generator.setTargets(firstFormant, 340, 814, 2656, 3473, 40, 40, 120)
		for track := firstFormant; track < nasalZeroTrack; track++ {
			segments[track].Forward, segments[track].Back = int32(current.Value()*55/100), 3
			if track <= fourthFormant {
				generator.Weights[track] = 3280
			}
		}
	}
	if previous == 'R' {
		for track := firstFormant; track <= fourthFormant; track++ {
			segments[track].Mode = Shaped
		}
	}
}

func (generator *Generator) rulesForLowerS() {
	cursor := generator.cursor()
	current := cursor.Scan
	previous, next := cursor.Done.Symbol(), cursor.First.Symbol()
	beforePrevious := symbolOf(cursor.Done.Previous)
	segments := &generator.Segments
	generator.groupPrevious()
	if !isVowel(next) {
		switch generator.PreviousGroup {
		case 1:
			generator.setTargets(6, 64, 55)
			segments[firstBandwidth+2].Target = 184
		case 2:
			segments[4].Target, segments[5].Target, segments[7].Target = 65, 65, 53
			generator.setTargets(firstFormant+2, 2304, 3024)
		case 3:
			generator.setTargets(3, 60, 60, 60)
			segments[7].Target = 54
			segments[firstFormant+1].Target = 1700
		case 4:
			generator.setTargets(5, 58, 48, 48)
			segments[fourthFormant].Target = 3504
		case 5:
			segments[4].Target, segments[7].Target = 65, 54
			generator.setTargets(firstFormant+2, 2256, 3168)
		}
	}
	switch next {
	case '@':
		frication := &segments[fricationTrack]
		frication.Forward += 3
		frication.Target -= 3
		frication.Mode = Shaped
		for track := 4; track <= 7; track++ {
			segments[track].Mode = Shaped
			segments[track].Forward += 3
		}
		if previous == 'P' {
			segments[4].Mode, segments[5].Mode = Flat, Flat
		}
	case 'b':
		segments[fricationTrack].Forward = int32(current.Value() >> 1)
	}
	if previous != 'C' {
		return
	}
	switch next {
	case ' ':
		segments[fricationTrack].Target = 60
		if !isVowel(beforePrevious) {
			generator.setTargets(firstFormant+1, 2540, 2928, 3744)
		}
	case 'R':
		segments[aspirationTrack].Target = 50
		segments[7].Target = 60
		generator.setTargets(firstFormant+1, 1396, 1936)
		if !stressed(current) {
			segments[fricationTrack].Target = 58
		} else {
			segments[fricationTrack].Target = 60
			segments[4].Target = 66
		}
	}
}

func (generator *Generator) rulesForLowerX() {
	previous := generator.cursor().Done.Symbol()
	if previous == ' ' {
		generator.Segments[firstFormant+1].Target = 1550
		generator.Segments[7].Target = 50
		return
	}
	if generator.has(previous, 2, vowelClass) {
		generator.Weights[firstFormant+1], generator.Weights[firstFormant+2] = 26240, 26240
	}
}

func (generator *Generator) rulesForLowerZ() {
	cursor := generator.cursor()
	previous := cursor.Done.Symbol()
	segments := &generator.Segments
	if generator.has(previous, 2, vowelClass) {
		generator.Weights[firstFormant+1], generator.Weights[firstFormant+2] = 26240, 26240
	}
	if previous != 'J' {
		return
	}
	switch cursor.First.Symbol() {
	case ' ':
		segments[voicingTrack].Onset = 47
		segments[voicingTrack].Forward = int32(cursor.Scan.Value() >> 2)
	case 'k':
		for track := firstFormant; track <= fourthFormant; track++ {
			segments[track].Target = generator.Last[track]
		}
		fallthrough
	case 'A', 'a', 'e', 'i', '|':
		segments[fricationTrack].Target = generator.Last[fricationTrack]
		segments[aspirationTrack].Target = generator.Last[aspirationTrack]
	case 'y':
		if symbolOf(cursor.Done.Previous) == ' ' {
			generator.setTargets(firstFormant, 254, 1613, 2088, 2735)
		}
	}
}
