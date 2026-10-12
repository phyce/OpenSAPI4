package parameters

import "github.com/phyce/OpenSAPI4/truvoice/datapack"

// RenderPhoneme renders the phoneme at the cursor: it loads its targets, sets the rules' flags, applies the
// rules, adjusts the targets for voicing, attenuation and the voice, and renders every track's segment
// (parameters.md §6).
func (generator *Generator) RenderPhoneme() {
	cursor := generator.cursor()
	generator.LoadTargets()
	generator.InitModes()
	current, previous, next := cursor.Scan, cursor.Done, cursor.First
	symbol, previousSymbol, nextSymbol := current.Symbol(), previous.Symbol(), next.Symbol()
	flags := &generator.Flags
	*flags = [datapack.FlagCount]bool{}
	flags[BeforeStressedFlag] = next.Flags()&stressedFlag != 0
	flags[VoicedFlag] = generator.has(symbol, 0, voicedClass)
	switch generator.PreviousCategory {
	case VowelCategory:
		flags[AfterVowelFlag] = true
	case ClosureCategory:
		flags[AfterClosureFlag] = true
	case SonorantCategory:
		flags[AfterSonorantFlag] = true
	default:
		flags[AfterFricativeFlag] = true
	}
	if flags[AfterClosureFlag] && generator.has(previousSymbol, 0, nasalClass) || flags[AfterVowelFlag] {
		flags[AfterSonorantFlag] = true
	}
	if generator.Category != VowelCategory {
		if generator.has(symbol, 0, stopClass) && (generator.has(nextSymbol, 2, closureClass) || nextSymbol == ' ' || current.Flags()&glottalFlag != 0) {
			flags[ClosedFlag] = true
		}
		_, nextPhoneme := generator.phoneme(nextSymbol)
		switch nextPhoneme.Place {
		case 0:
			flags[BeforePlace0Flag] = true
		case 1:
			flags[BeforePlace1Flag] = true
		case 3:
			flags[BeforePlace3Flag] = true
		case 2:
			flags[BeforePlace2Flag] = true
		}
	}
	generator.ApplyRules()
	segments := &generator.Segments
	if !generator.has(symbol, 0, voicedClass) {
		generator.UnvoicedPitch = segments[pitchTrack].Target
		segments[pitchTrack].Target = 0
		segments[pitchTrack].Mode &^= PullForward
	}
	if current.Second() == 0 {
		voicing := segments[voicingTrack].Target
		segments[voicingTrack].Target = 0
		segments[voicingTrack].Onset = 0
		segments[aspirationTrack].Target = voicing + 5
		segments[voicingTrack].BackTarget = 0
		segments[pitchTrack].Mode &^= PullForward
	}
	if !generator.has(previousSymbol, 0, voicedClass) || previous.Second() == 0 {
		segments[pitchTrack].Mode &^= PullBack
	}
	attenuation := cursor.Attenuation
	for _, track := range []int{voicingTrack, fricationTrack, aspirationTrack} {
		segment := &segments[track]
		segment.Target = max(segment.Target-attenuation, 0)
		segment.BackTarget = max(segment.BackTarget-attenuation, 0)
		segment.Onset = max(segment.Onset-attenuation, 0)
	}
	var voice int32
	if cursor.Voice != 0 {
		voice = cursor.Voice
		segments[fourthFormant].Target = min(segments[fourthFormant].Target, generator.voice().FourthFormantLimit)
	}
	segments[voiceTrack].Target |= voice << 4
	segments[attenuationTrack].Target |= attenuation << 4
	if generator.KeepEarlier {
		for track := range generator.Tracks {
			generator.Tracks[track].Earliest = generator.Tracks[track].Next
		}
		generator.KeepEarlier = false
	}
	if generator.Pipeline.VisualEvents && symbol != 'p' {
		generator.Pipeline.PostVisual(symbol)
	}
	for track := range segments {
		generator.RenderSegment(track)
	}
}
