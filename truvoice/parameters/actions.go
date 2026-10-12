package parameters

import (
	"strings"

	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
)

// The class bits only the actions test (parameters.md §9.1): of class byte 1, 2, 3 and 4.
const (
	affricateClass   = 0x08
	alveolarClass    = 0x04
	rhoticClass      = 0x08
	frontClass       = 0x20
	lateralClass     = 0x40
	approximantClass = 0x01
	palatalClass     = 0x04
	roundedClass     = 0x20
	frontEndingClass = 0x40
	highClass        = 0x01
	rColouredClass   = 0x02
	roundEndingClass = 0x04
)

// The tracks only the actions handle by name.
const (
	thirdGainTrack  = 4
	fourthGainTrack = 5
	spareTrack      = 7
	secondFormant   = 10
	thirdFormant    = 11
)

// The input flag that makes a stop's voice onset 2 frames, the place of the approximants, and the weights of F1
// to F3 after a vowel that ends front or round and before a lateral. unknownFill is what the port writes where
// the original writes the low byte of its state's address (parameters.md §9.5).
const (
	shortOnsetFlag    = 0x10
	approximantPlace  = 4
	frontEndingWeight = 0x2cd8
	lateralWeight     = 0x7350
	unknownFill       = 0
)

func symbolAfter(node *pipeline.Node) byte {
	if node == nil {
		return 0
	}
	return symbolOf(node.Next)
}

func symbolBefore(node *pipeline.Node) byte {
	if node == nil {
		return 0
	}
	return symbolOf(node.Previous)
}

func isWordMark(symbol byte) bool {
	return symbol == '&' || symbol == '%'
}

func isVoicelessStop(symbol byte) bool {
	return strings.IndexByte("PTK", symbol) >= 0
}

// articulatorShape returns the forward shape of the articulator of a phoneme (parameters.md §9.2).
func (generator *Generator) articulatorShape(symbol byte) int32 {
	_, phoneme := generator.phoneme(symbol)
	return generator.tables.ArticulatorShapes[phoneme.Articulator]
}

// setFormantShapes gives the formant, bandwidth and nasal zero tracks a forward shape, F1 half of it plus one.
func (generator *Generator) setFormantShapes(shape int32) {
	for track := firstFormant; track <= nasalZeroTrack; track++ {
		generator.Segments[track].Forward = shape
	}
	generator.Segments[firstFormant].Forward = shape/2 + 1
}

func (generator *Generator) setModes(first, last int, mode int32) {
	for track := first; track <= last; track++ {
		generator.Segments[track].Mode = mode
	}
}

func (generator *Generator) setForward(first, last int, shape int32) {
	for track := first; track <= last; track++ {
		generator.Segments[track].Forward = shape
	}
}

func (generator *Generator) setBack(first, last int, back int32) {
	for track := first; track <= last; track++ {
		generator.Segments[track].Back = back
	}
}

func (generator *Generator) setWeights(first, last int, weight int32) {
	for track := first; track <= last; track++ {
		generator.Weights[track] = weight
	}
}

// advanceTrack moves a track's next frame on by frames frames, taking them from its segment.
func (generator *Generator) advanceTrack(track int, frames int32) {
	generator.Tracks[track].Next += frames
	generator.Segments[track].Frames -= frames
}

// ShiftAspiration is action 1 (parameters.md §9.3).
func (generator *Generator) ShiftAspiration() {
	cursor := generator.cursor()
	symbol, previous := cursor.Scan.Symbol(), cursor.Done.Symbol()
	if generator.has(previous, 0, sonorantClass) && !generator.has(symbol, 2, closureClass) {
		generator.AspirationShift = 1
		if generator.has(symbol, 0, fricativeClass) && generator.Tracks[fricationTrack].Next >= 2 {
			generator.Segments[fricationTrack].Frames += 2
			generator.Tracks[fricationTrack].Next -= 2
		}
		generator.PullAspirationBack()
	}
	if symbol != 'S' || cursor.First.Symbol() != ' ' || !generator.has(previous, 2, vowelClass) {
		generator.Segments[pitchTrack].Mode = Flat
	}
	if symbol != 'S' {
		generator.Segments[voicingTrack].Mode = PullBack | Flat
	}
}

// PullAspirationBack is action 15: the aspiration starts the aspiration shift's frames earlier (parameters.md
// §9.3).
func (generator *Generator) PullAspirationBack() {
	cursor := generator.cursor()
	aspiration := &generator.Tracks[aspirationTrack]
	if generator.AspirationShift > aspiration.Next || cursor.Voice == 8 && generator.Pipeline.SampleRate == 8000 {
		return
	}
	aspiration.Next -= generator.AspirationShift
	ring := generator.Ring(aspirationTrack)
	ring.AddBack(aspiration.Next, 3, aspiration.Next-aspiration.Earliest, generator.Aspiration-int32(ring.At(aspiration.Next)))
	segment := &generator.Segments[aspirationTrack]
	segment.Frames += generator.AspirationShift
	generator.Last[aspirationTrack] = generator.Aspiration
	segment.Forward = 3
	symbol := cursor.Scan.Symbol()
	if symbol == ' ' {
		voicing := &generator.Tracks[voicingTrack]
		generator.Ring(voicingTrack).AddBack(voicing.Next, 10-generator.AspirationShift, voicing.Next-voicing.Earliest, -4)
	}
	if generator.has(symbol, 2, closureClass) {
		segment.Mode = PullBack | Flat
	}
}

// ShapesFromPrevious is action 2 (parameters.md §9.4).
func (generator *Generator) ShapesFromPrevious() {
	cursor := generator.cursor()
	symbol, previous := cursor.Scan.Symbol(), cursor.Done.Symbol()
	generator.setFormantShapes(generator.articulatorShape(previous))
	switch {
	case generator.has(previous, 1, affricateClass):
		generator.setModes(voicingTrack, aspirationTrack, Shaped)
		generator.setModes(aspirationTrack+1, bypassTrack, Flat)
	case generator.has(previous, 0, stopClass):
		generator.setModes(fricationTrack, bypassTrack, Flat)
		if previous == 'T' && symbol == 'p' {
			generator.Weights[aspirationTrack], generator.Weights[fricationTrack] = 0, 0
			generator.setModes(fricationTrack, aspirationTrack, Shaped)
		}
	default:
		generator.setModes(voicingTrack, bypassTrack, Shaped)
	}
	segments := &generator.Segments
	if generator.has(previous, 0, nasalClass) && generator.has(symbol, 0, voicedClass) {
		segments[voicingTrack].Mode = Flat
	}
	if strings.IndexByte("JBDG", previous) >= 0 && generator.has(symbolBefore(cursor.Done), 0, voicedClass) {
		segments[voicingTrack].Onset, segments[voicingTrack].BackTarget = 52, 52
	}
}

// VowelOnset is action 3 (parameters.md §9.5).
func (generator *Generator) VowelOnset() {
	cursor := generator.cursor()
	current, previous, next := cursor.Scan, cursor.Done, cursor.First
	symbol, previousSymbol, nextSymbol := current.Symbol(), previous.Symbol(), next.Symbol()
	segments := &generator.Segments
	vowel := generator.has(symbol, 2, vowelClass)
	afterVoicedFricative := (symbol == 'R' || symbol == 'L' || symbol == 'W') && generator.has(previousSymbol, 0, fricativeClass) && generator.has(previousSymbol, 0, voicedClass)
	afterUnvoiced := !generator.has(previousSymbol, 0, voicedClass) && previousSymbol != ' ' && previousSymbol != 'H' &&
		previous.Flags()&glottalFlag == 0 && !(stressed(current) && vowel && previousSymbol == 'D')
	if afterVoicedFricative || afterUnvoiced {
		generator.writeVoiceOnset()
	}
	if !vowel {
		generator.VoiceOnset = noVoiceOnset
	}
	if generator.has(previousSymbol, 0, stopClass|fricativeClass) && previousSymbol != 's' {
		segments[fricationTrack].Frames -= generator.VoicingShift
	}
	if !isVowel(symbol) {
		if generator.has(previousSymbol, 3, approximantClass) {
			shape := int32(7)
			if generator.has(symbol, 3, roundedClass) && previousSymbol == 'Y' {
				shape = 11
			}
			generator.setForward(firstFormant, nasalZeroTrack, shape)
			if generator.has(previousSymbol, 2, rhoticClass) {
				segments[thirdFormant].Forward = 9
			}
		}
		if !generator.has(symbol, 2, labialClass) && (generator.has(previousSymbol, 4, roundEndingClass) || generator.has(previousSymbol, 3, frontEndingClass)) {
			generator.setWeights(firstFormant, thirdFormant, frontEndingWeight)
		}
		if generator.has(symbol, 3, roundedClass) && generator.has(previousSymbol, 3, palatalClass) {
			segments[secondFormant].Forward += 5
		}
	}
	_, phoneme := generator.phoneme(symbol)
	switch previousSymbol {
	case 'T':
		if vowel || symbol == 'n' {
			generator.keepGains()
		}
	case 'K':
		if phoneme.Place == 3 || phoneme.Place == 2 || strings.IndexByte("alWLYp", symbol) >= 0 {
			generator.keepGains()
		}
	case 'P':
		if symbol == 'i' || symbol == 'R' && stressed(next) && generator.has(nextSymbol, 2, vowelClass) {
			generator.keepGains()
		}
	case 'X':
		if symbol == 'R' || vowel {
			generator.keepGains()
		}
	}
	if isVoicelessStop(previousSymbol) && symbol == 'p' {
		segments[voicingTrack].Target = 0
	}
	if vowel && !stressed(current) && !isWordMark(symbolAfter(current)) && generator.has(nextSymbol, 2, vowelClass) && stressed(next) {
		segments[voicingTrack].Target -= 2
	}
	if vowel && stressed(current) && !isWordMark(symbolBefore(current)) && generator.has(previousSymbol, 2, vowelClass) && !stressed(previous) {
		segments[voicingTrack].Target += 2
	}
	if (symbol == 'R' || symbol == 'L') && symbolBefore(current) == 'P' {
		segments[bypassTrack].Target = 0
	}
}

// keepGains makes the targets of tracks 3 to 8 the values they last had.
func (generator *Generator) keepGains() {
	for track := aspirationTrack + 1; track <= bypassTrack; track++ {
		generator.Segments[track].Target = generator.Last[track]
	}
}

// writeVoiceOnset writes the frames before the voicing starts on tracks 0 to 2, and after a stop its frication
// and the pitch's silence (parameters.md §9.5).
func (generator *Generator) writeVoiceOnset() {
	cursor := generator.cursor()
	current, previous := cursor.Scan, cursor.Done
	symbol, previousSymbol := current.Symbol(), previous.Symbol()
	segments, last := &generator.Segments, &generator.Last
	voicing, frication, aspiration, pitch := &generator.Tracks[voicingTrack], &generator.Tracks[fricationTrack], &generator.Tracks[aspirationTrack], &generator.Tracks[pitchTrack]
	if !stressed(previous) {
		generator.Aspiration = max(generator.Aspiration-3, 0)
	}
	voiceless := isVoicelessStop(previousSymbol)
	if !generator.has(previousSymbol, 0, stopClass) {
		generator.VoiceOnset = 1
		if !generator.has(symbol, 0, syllabicClass) {
			generator.VoiceOnset = segments[aspirationTrack].Frames / 2
		}
		if !((symbol == 'R' || generator.has(symbol, 2, vowelClass)) && previousSymbol == 'X') {
			generator.Ring(aspirationTrack).AddBack(aspiration.Next, 3, aspiration.Next-aspiration.Earliest, generator.Aspiration-last[aspirationTrack])
		}
	} else {
		if cursor.InputFlags&shortOnsetFlag != 0 {
			generator.VoiceOnset = 2
		}
		if generator.has(previousSymbol, 2, alveolarClass) && generator.has(symbol, 2, rhoticClass) {
			generator.Aspiration += 9
		}
		if voiceless {
			switch {
			case previousSymbol == 'P' && symbol == 'y':
				generator.Aspiration = last[aspirationTrack]
			case symbol == 'p':
				generator.Aspiration = last[aspirationTrack] - 6
			default:
				generator.Aspiration = last[aspirationTrack] - 2
			}
			generator.Aspiration = max(generator.Aspiration, 0)
		}
		if symbol == 'p' && !voiceless {
			generator.Ring(fricationTrack).Repeat(frication.Next, generator.VoiceOnset, unknownFill)
			generator.advanceTrack(fricationTrack, generator.VoiceOnset)
		}
		if limit := current.Value() - 3; uint32(generator.VoiceOnset) > limit {
			generator.VoiceOnset = int32(max(limit, 1))
		}
		_, phoneme := generator.phoneme(symbol)
		if stressed(previous) && (generator.has(symbol, 0, syllabicClass) || phoneme.Place == approximantPlace) ||
			previousSymbol == 'K' && strings.IndexByte("rgc", symbol) >= 0 {
			level := int32(60)
			if voiceless {
				level = max(last[fricationTrack]-2, 0)
				switch {
				case previousSymbol != 'K':
					if isVowel(symbol) && stressed(current) {
						level = 0
					}
				case symbol == 'O':
					level = 55
				case strings.IndexByte("ibuUL", symbol) >= 0:
					level = last[fricationTrack]
				}
			}
			onset := generator.VoiceOnset
			generator.Ring(fricationTrack).Repeat(frication.Next, onset, byte(level))
			generator.advanceTrack(fricationTrack, onset)
			silence := onset - 1
			switch {
			case previousSymbol == 'K':
				silence = onset
			case previousSymbol == 'P' && symbol == 'L':
				silence = onset - 3
			}
			if silence < 0 {
				silence = max(onset-1, 0)
			}
			if silence >= segments[pitchTrack].Frames {
				silence = segments[pitchTrack].Frames - 1
			}
			generator.Ring(pitchTrack).Repeat(pitch.Next, silence, 0)
			generator.advanceTrack(pitchTrack, silence)
			last[pitchTrack] = generator.UnvoicedPitch
		}
		aspiration.Earliest = aspiration.Next
	}
	generator.VoiceOnset = min(generator.VoiceOnset, segments[aspirationTrack].Frames, segments[voicingTrack].Frames)
	onset := generator.VoiceOnset
	if onset < 1 {
		return
	}
	voicingRing := generator.Ring(voicingTrack)
	switch {
	case symbol == 'R' && (previousSymbol == 'z' || previousSymbol == 'V'):
		voicingRing.Repeat(voicing.Next, onset, byte(last[voicingTrack]))
	case symbol == 'R' && previousSymbol == 'X':
		frames := onset
		if frames > 1 {
			frames--
			voicingRing.Repeat(voicing.Next, frames, 0)
		}
		voicingRing.Repeat(voicing.Next+frames, 1, 57)
		last[voicingTrack] = 57
	case isVowel(symbol) && onset == 1 && !generator.has(previousSymbol, 0, stopClass):
		voicingRing.Repeat(voicing.Next, 1, 48)
		last[voicingTrack] = 48
	default:
		voicingRing.Repeat(voicing.Next, onset, 0)
		last[voicingTrack] = 0
	}
	if (symbol == 'R' || generator.has(symbol, 2, vowelClass)) && previousSymbol == 'X' {
		generator.Ring(aspirationTrack).Repeat(aspiration.Next, onset, byte(last[aspirationTrack]))
		generator.Ring(fricationTrack).Repeat(frication.Next, onset, byte(last[fricationTrack]))
		generator.advanceTrack(fricationTrack, onset)
	} else {
		generator.Ring(aspirationTrack).Repeat(aspiration.Next, onset, byte(generator.Aspiration))
	}
	last[aspirationTrack] = generator.Aspiration
	segments[voicingTrack].Mode = Shaped
	generator.advanceTrack(aspirationTrack, onset)
	generator.advanceTrack(voicingTrack, onset)
}

// SonorantTransitions is action 5 (parameters.md §9.6).
func (generator *Generator) SonorantTransitions() {
	cursor := generator.cursor()
	current, previous, next := cursor.Scan, cursor.Done, cursor.First
	symbol, previousSymbol, nextSymbol := current.Symbol(), previous.Symbol(), next.Symbol()
	segments := &generator.Segments
	if generator.has(symbol, 2, lateralClass) && generator.has(previousSymbol, 0, sonorantClass) && !generator.has(previousSymbol, 0, nasalClass) {
		generator.setWeights(firstFormant, thirdFormant, lateralWeight)
	}
	if symbol != 'l' && nextSymbol != ' ' {
		shape := int32(9)
		if generator.has(symbol, 3, approximantClass) {
			shape = 7
			if generator.has(previousSymbol, 3, approximantClass) {
				shape = 5
			}
		}
		generator.setForward(firstFormant, nasalZeroTrack, shape)
	}
	if generator.has(symbol, 2, rhoticClass) {
		generator.setForward(secondFormant, fourthFormant, 6)
	}
	if symbol == 'n' {
		if nextSymbol == ' ' && symbolAfter(next) == ' ' {
			generator.setForward(secondFormant, fourthFormant, 4)
		}
		if previousSymbol == 'D' {
			segments[secondFormant].Mode = Shaped
		}
	}
	if symbol == 'H' {
		if generator.has(nextSymbol, 4, rColouredClass) {
			segments[aspirationTrack].Target = 55
		}
		if nextSymbol == 'g' && previousSymbol == 's' && symbolBefore(previous) == 'C' {
			segments[aspirationTrack].Target = 67
		}
	}
}

// ObstruentVoicing is action 6 (parameters.md §9.7).
func (generator *Generator) ObstruentVoicing() {
	cursor := generator.cursor()
	current, previous, next := cursor.Scan, cursor.Done, cursor.First
	symbol, previousSymbol, nextSymbol := current.Symbol(), previous.Symbol(), next.Symbol()
	segments := &generator.Segments
	if generator.has(symbol, 3, palatalClass) {
		after := symbolAfter(current)
		switch {
		case symbol != 's':
			segments[fricationTrack].Forward = 6
		case (after == '|' || after == '@') && symbolAfter(current.Next) == 'N':
			segments[fricationTrack].Forward -= 2
		case generator.has(nextSymbol, 2, frontClass) || generator.has(nextSymbol, 3, roundedClass):
			segments[fricationTrack].Forward = 6
		default:
			segments[fricationTrack].Forward -= 2
		}
		if generator.has(symbol, 1, affricateClass) && !generator.has(previousSymbol, 0, voicedClass) {
			segments[voicingTrack].Target = 0
		}
	}
	if generator.has(nextSymbol, 0, sonorantClass) {
		generator.VoicingShift = 2
		if next.Value() <= 2 {
			generator.VoicingShift = int32(next.Value()) - 1
		}
		if generator.has(symbol, 0, stopClass) || symbol == 'X' && nextSymbol == 'R' || symbol == 'S' {
			generator.VoicingShift = 0
		}
		if symbol != 's' {
			segments[fricationTrack].Frames += generator.VoicingShift
		}
	}
	if generator.has(symbol, 0, voicedClass) && generator.has(symbol, 0, stopClass) {
		generator.voicedStop()
	}
	if nextSymbol == 's' && symbolAfter(next) == 'R' && stressed(current) {
		segments[thirdGainTrack].Target = 60
		generator.BurstLevels[fricationTrack] = 60
		generator.BurstLevels[aspirationTrack] = 0
		segments[spareTrack].Target = 65
	}
	if symbol == 'F' && previousSymbol == ' ' {
		segments[bypassTrack].Target = 60
	}
	if symbol == 'V' {
		segments[bypassTrack].Target = 56
	}
}

// voicedStop is the part of action 6 for the voiced stops B D J G q.
func (generator *Generator) voicedStop() {
	cursor := generator.cursor()
	current, previous, next := cursor.Scan, cursor.Done, cursor.First
	symbol, previousSymbol, nextSymbol := current.Symbol(), previous.Symbol(), next.Symbol()
	segment := &generator.Segments[voicingTrack]
	nextVowel := generator.has(nextSymbol, 2, vowelClass)
	if !(symbol == 'D' && (nextVowel || nextSymbol == 'n')) && !(symbol == 'J' && nextSymbol == 'z') && !stressed(next) {
		generator.Segments[fourthGainTrack].Target, generator.Segments[thirdGainTrack].Target = 0, 0
	}
	if !nextVowel && !(symbol == 'G' && nextSymbol == 'Y') && nextSymbol != 'Z' && !(symbol == 'D' && nextSymbol == 'n') {
		segment.Target -= 20
	}
	previousVoiced := generator.has(previousSymbol, 0, voicedClass)
	switch {
	case !previousVoiced && !(symbol == 'D' && nextSymbol == '|') || symbol == 'q':
		segment.Target = 0
	case !generator.has(previousSymbol, 0, sonorantClass):
		segment.Target -= 30
	}
	switch {
	case symbol != 'q' && previousVoiced:
		segment.Target = 50
		segment.Forward = int32(current.Value())
	case (symbol == 'B' || symbol == 'G') && previousSymbol == 'S':
		segment.Target = 0
	case !nextVowel:
	case symbol == 'B':
		segment.Target = 50
	case symbol != 'D':
	case nextSymbol == '|':
		segment.Target = 0
	case previousSymbol == ' ' || previousSymbol == 'S':
		if generator.has(previousSymbol, 0, syllabicClass) && nextSymbol != 'p' {
			generator.Segments[fourthGainTrack].Target, generator.Segments[thirdGainTrack].Target = 0, 0
		}
	case stressed(next):
		segment.Target = 43
	}
}

// ConsonantShapes is action 8 (parameters.md §9.8).
func (generator *Generator) ConsonantShapes() {
	cursor := generator.cursor()
	symbol, previous := cursor.Scan.Symbol(), cursor.Done.Symbol()
	segments := &generator.Segments
	if !generator.has(previous, 2, closureClass) {
		generator.setModes(voicingTrack, firstFormant, PullBack|Flat)
		if symbol == 'T' || symbol == 'M' && previous == 'z' {
			segments[firstFormant].Mode = PullBack | Shaped
		}
	}
	if generator.has(symbol, 0, voicedClass) {
		switch {
		case generator.has(symbol, 0, stopClass):
			segments[voicingTrack].Mode = Shaped
		case generator.has(symbol, 0, nasalClass):
			segments[voicingTrack].Mode = Flat
		}
	}
	if generator.has(previous, 2, lateralClass) {
		segments[thirdFormant].Mode, segments[secondFormant].Mode = PullBack|Flat, PullBack|Flat
	}
	shape := int32(5)
	if !generator.has(previous, 4, highClass) {
		shape = generator.articulatorShape(symbol)
	}
	generator.setFormantShapes(shape)
}
