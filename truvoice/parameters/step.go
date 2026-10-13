package parameters

import (
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
	"github.com/phyce/OpenSAPI4/truvoice/synthesizer"
)

// What the stage found after the current phoneme (parameters.md §17.3).
const (
	AheadPhoneme   = iota // a phoneme
	AheadEnd              // an end command, which every text ends with
	AheadReset            // a reset command, or a bookmark once a reset is parsed
	AheadGenerated        // a generated segment
	AheadNothing          // ten nodes without a phoneme
)

// How far the stage is in starting the synthesizer after a reset (parameters.md §17.1).
const (
	StartUnheld   = iota // no reset sets it
	StartWaiting         // after a reset
	StartWritten         // a phoneme other than the pause is written
	StartReleased        // the synthesizer is released
	StartSpeaking        // a phoneme is written without a reset before
	StartEnded           // the synthesizer was released otherwise
)

// The pauses the stage puts in, by flag bits 3 and 4 of their nodes.
const (
	pauseKind   = 0x18
	leadInPause = 0x08
	breakPause  = 0x10
	finalPause  = 0x18
	resetsFlag  = 0x20
)

const (
	fastFlags           = 0x2000
	fastRate            = 25
	pauseFrames         = 4
	breakFrames         = 10
	defaultPitch        = 50
	nodesWithout        = 10
	startAhead          = 8
	farAhead            = 64
	whisperCharacter    = 1
	whisperPitch        = 100
	segmentFramesAtMost = 53
	toneLevel           = 60
	wideTone            = 180
	wideBandwidth       = 45
	punctuationClass    = 0x08
	noEnd               = -1
)

// isPause reports whether a node is a pause of the kind.
func isPause(node *pipeline.Node, kind uint32) bool {
	return node != nil && node.Type() == pipeline.Segment && node.Symbol() == ' ' && node.Flags()&pauseKind == kind
}

// ahead returns how far the ring's written frames are ahead of the synthesizer, beyond the lookahead.
func (generator *Generator) ahead() int32 {
	ring := generator.Synthesizer
	return ring.Written - ring.Position - ring.Lookahead
}

// release lets the synthesizer go on past its end.
func (generator *Generator) release() {
	generator.Synthesizer.End = noEnd
	generator.Synthesizer.Done = false
	if generator.Start == StartSpeaking {
		generator.Start = StartEnded
	}
}

// Step is the stage's step (parameters.md §17.2): it renders the next phoneme, or writes the next frames of a
// generated segment. It reports pipeline.ParametersFinished, ParametersWaiting or ParametersWorking.
func (generator *Generator) Step() int {
	stage := generator.Pipeline
	cursor := generator.cursor()
	ring := generator.Synthesizer
	stage.Begin(cursor)
	if cursor.Done == nil && cursor.Scan == nil && cursor.First == nil {
		return generator.finish(false)
	}
	if cursor.Flags&fastFlags == 0 && cursor.Rate < fastRate {
		generator.TransitionLimit, generator.Margin, ring.Lookahead = 30, 15, 12
	} else {
		generator.TransitionLimit, generator.Margin, ring.Lookahead = 4, 3, 2
	}
	if ring.End != noEnd && generator.ahead() <= startAhead && generator.Start == StartUnheld {
		generator.Start = StartWaiting
	}
	scan := cursor.Scan
	if generator.Ahead == AheadReset && isPause(scan, finalPause) {
		generator.AtReset = true
		return generator.finish(false)
	}
	if scan != nil && (generator.Ahead == AheadGenerated || scan.Type() != pipeline.Segment) {
		generator.Ahead = AheadGenerated
		if scan.Type() != pipeline.Segment || isPause(scan, finalPause) {
			return generator.stepGenerated()
		}
	}
	if scan != nil {
		generator.Wanted = int32(scan.Value())
		if !generator.Room(generator.Wanted) {
			generator.release()
			return generator.waitOrWork()
		}
		if generator.ahead() > farAhead {
			generator.release()
			stage.ShortPhrase = false
		}
		if cursor.Flush == 0 {
			generator.release()
		}
	}
	next := generator.lookAhead()
	if next == nil {
		return generator.finish(false)
	}
	cursor.First = next
	for node := stage.Next(next); node != nil; node = stage.Next(node) {
		if node.Type() == pipeline.Segment {
			generator.AfterNext = node.Symbol()
			break
		}
		if node.Type() != pipeline.Phoneme {
			generator.AfterNext = ' '
			break
		}
	}
	if cursor.Scan.Symbol() != ' ' {
		switch generator.Start {
		case StartUnheld:
			generator.Start = StartSpeaking
		case StartWaiting:
			generator.Start = StartWritten
		}
	}
	generator.RenderPhoneme()
	generator.Advance()
	if !generator.has(cursor.Scan.Symbol(), 0, voicedClass) {
		generator.Last[pitchTrack] = generator.UnvoicedPitch
	}
	if cursor.Scan.Character() == whisperCharacter {
		generator.UnvoicedPitch = whisperPitch
		generator.Last[pitchTrack] = whisperPitch
	}
	freed := false
	previous := cursor.Done
	if isPause(previous, leadInPause) {
		stage.FreeNode(previous, true)
		freed = true
	}
	if generator.Start == StartWritten && generator.ahead() > startAhead {
		ring.End = noEnd
		ring.Done = false
		generator.Start = StartReleased
	}
	generator.BeforePrevious = previous.Symbol()
	cursor.Done = cursor.Scan
	cursor.Scan = stage.Next(cursor.Scan)
	cursor.First = stage.Next(cursor.First)
	if isPause(cursor.Scan, finalPause) {
		generator.endAt(cursor.Scan)
	}
	generator.Rebase()
	return generator.finish(freed)
}

// endAt makes the synthesizer end where the final pause starts, and drops the pause when no phoneme or generated
// segment comes after it.
func (generator *Generator) endAt(pause *pipeline.Node) {
	stage := generator.Pipeline
	cursor := generator.cursor()
	ring := generator.Synthesizer
	ring.End = ring.Written - ring.Lookahead
	ring.Done = false
	node := stage.Next(pause)
	for node != nil && node.Type() != pipeline.Segment && node.Type() != pipeline.Generated {
		node = stage.Next(node)
	}
	if node != nil {
		return
	}
	stage.FreeNode(pause, false)
	cursor.Done, cursor.Scan, cursor.First = nil, nil, nil
	ring.Markers[uint8(ring.Written-1)] = false
	ring.Markers[uint8(ring.End-1)] = true
}

// finish hands the finished nodes on and reports whether the stage did anything.
func (generator *Generator) finish(freed bool) int {
	if generator.Pipeline.Finish() || freed {
		return pipeline.ParametersWorking
	}
	return pipeline.ParametersFinished
}

// waitOrWork hands the finished nodes on, and reports that the stage waits for room unless it handed some on.
func (generator *Generator) waitOrWork() int {
	if generator.Pipeline.Finish() {
		return pipeline.ParametersWorking
	}
	return pipeline.ParametersWaiting
}

// stepGenerated writes the next frames of the generated segment at the cursor, and moves on to the next node
// once it has written them all (parameters.md §17.5).
func (generator *Generator) stepGenerated() int {
	cursor := generator.cursor()
	left := generator.RenderGenerated()
	generator.Rebase()
	generator.KeepEarlier = true
	generator.Wanted = min(left, 1)
	if left == 0 {
		cursor.Done = cursor.Scan
		cursor.Scan = generator.Pipeline.Next(cursor.Scan)
		cursor.First = cursor.Scan
		generator.Ahead = AheadPhoneme
	}
	generator.Start = StartEnded
	return generator.waitOrWork()
}

// lookAhead puts a pause before the current phoneme when the previous node isn't a segment, finds what comes
// after the current phoneme, puts in pauses where that asks for them, and returns the next phoneme, or nil
// (parameters.md §17.3).
func (generator *Generator) lookAhead() *pipeline.Node {
	stage := generator.Pipeline
	cursor := generator.cursor()
	if scan := cursor.Scan; scan != nil && (cursor.Done == scan || cursor.Done.Type() != pipeline.Segment) {
		pause := stage.NewNode(scan, false, pipeline.Segment, ' ')
		cursor.Done = pause
		pause.SetValue(pauseFrames)
		pause.SetSecond(cursor.Scan.Second())
		pause.SetFlags(pause.Flags() | leadInPause)
	}
	count := 0
	var node *pipeline.Node
	if cursor.Scan != nil {
	search:
		for node = stage.Next(cursor.Scan); node != nil; node = stage.Next(node) {
			if node.Type() == pipeline.Segment {
				if generator.Ahead == AheadGenerated && isPause(node, finalPause) {
					return node
				}
				generator.Ahead = AheadPhoneme
				break
			}
			count++
			symbol := node.Symbol()
			switch node.Type() {
			case pipeline.Control:
				if symbol == 'C' {
					generator.Ahead = AheadEnd
					break search
				}
				if symbol == 'x' || stage.ParsedReset && symbol == 'i' {
					node.SetFlags(node.Flags() | resetsFlag)
					generator.Ahead = AheadReset
					break search
				}
			case pipeline.Generated:
				generator.Ahead = AheadGenerated
				break search
			case pipeline.Phoneme:
				if generator.has(symbol, 4, punctuationClass) && symbol != ']' {
					generator.release()
				}
			}
		}
	}
	if count >= nodesWithout {
		generator.Ahead = AheadNothing
	}
	switch {
	case generator.Ahead == AheadPhoneme || node == nil && generator.Ahead != AheadNothing:
		generator.Synthesizer.End = noEnd
	case generator.Ahead == AheadEnd || generator.Ahead == AheadReset:
		return generator.insertPauses(node)
	case generator.Ahead == AheadNothing:
		return generator.insertPauses(cursor.End)
	}
	return node
}

// insertPauses puts a pair of pauses after a node: a break of 4 frames before an end command, or long enough for
// the synthesizer to speak every frame before it, and a final pause of 4 frames (parameters.md §17.4). It returns
// the break, or the final pause when there is no current phoneme, which the break then becomes.
func (generator *Generator) insertPauses(at *pipeline.Node) *pipeline.Node {
	stage := generator.Pipeline
	cursor := generator.cursor()
	pitch := byte(defaultPitch)
	if cursor.Scan != nil {
		pitch = cursor.Scan.Second()
	}
	first := stage.NewNode(at, true, pipeline.Segment, ' ')
	if generator.Ahead == AheadEnd {
		first.SetValue(pauseFrames)
	} else {
		first.SetValue(uint32(generator.Synthesizer.Lookahead + breakFrames + generator.Margin))
	}
	first.SetFlags(first.Flags() | breakPause)
	first.SetSecond(pitch)
	second := stage.NewNode(first, true, pipeline.Segment, ' ')
	second.SetValue(pauseFrames)
	second.SetSecond(pitch)
	second.SetFlags(second.Flags() | finalPause)
	if cursor.Scan == nil {
		cursor.Scan = first
		first = stage.Next(first)
	}
	generator.Synthesizer.End = noEnd
	generator.Synthesizer.Done = false
	return first
}

// RenderGenerated starts the generated segment at the cursor when the last one is written, writes as many of its
// frames as the ring has room for, and returns how many are left (parameters.md §17.5).
func (generator *Generator) RenderGenerated() int32 {
	cursor := generator.cursor()
	ring := generator.Synthesizer
	if generator.GeneratedDone {
		if cursor.Scan.Type() == pipeline.Segment {
			cursor.Scan = generator.Pipeline.FreeNode(cursor.Scan, true)
		}
		generator.SegmentSymbol = cursor.Scan.Symbol()
		generator.SegmentLeft = int32(cursor.Scan.Value() & 0xff)
		generator.ResetContext()
		generator.Ahead = AheadGenerated
		generator.GeneratedDone = false
		generator.SegmentStarting = true
	}
	room := ring.Position - generator.Margin - generator.Furthest + synthesizer.RingLength
	if generator.SegmentLeft != 0 && room != 0 {
		generator.SegmentLeft = generator.FillSegment(room)
		if generator.SegmentStarting {
			ring.End = noEnd
			ring.Done = false
		}
		generator.SegmentStarting = false
	}
	if generator.SegmentLeft == 0 {
		generator.GeneratedDone = true
	}
	return generator.SegmentLeft
}

// FillSegment writes the generated segment's frame from the ring's written frames on, at most room frames and 53,
// and returns the frames left. A segment's first call makes its frame. Every track then starts after the frames
// written.
func (generator *Generator) FillSegment(room int32) int32 {
	ring := generator.Synthesizer
	if generator.SegmentStarting {
		generator.ToneFrequency = 0
		switch generator.SegmentSymbol {
		case 'g':
			generator.SegmentFrame = generator.Pipeline.Levels
		case 's':
			generator.SegmentFrame = generator.Pipeline.InitialFrame()
		case 't':
			generator.makeSegmentFrame()
		}
	}
	left := generator.SegmentLeft
	position := ring.Written
	for count := segmentFramesAtMost; left > 0 && room > 0 && count != 0; count-- {
		ring.Frames[uint8(position)] = synthesizer.Frame(generator.SegmentFrame)
		if generator.ToneFrequency != 0 {
			ring.Tones[uint8(position-1)] = true
		}
		left--
		room--
		position++
	}
	for track := range generator.Tracks {
		generator.Tracks[track] = Track{Next: position, Earliest: position - ring.Lookahead}
	}
	ring.Written = position
	generator.Furthest = position
	return left
}

// makeSegmentFrame makes the frame of a generated segment t from the data pack's frame of its kind, flag bits 3
// to 6 of its node: the voicing and frication, the formants of a tone, and the pitch.
func (generator *Generator) makeSegmentFrame() {
	scan := generator.cursor().Scan
	flags := scan.Flags()
	kind := flags >> 3 & 3
	if flags>>5&1 != 0 {
		kind += 4
	}
	if flags>>6&1 != 0 {
		kind += 8
	}
	template := &generator.tables.SegmentFrames[kind]
	tone := template[firstFormant] == 0xff
	if tone {
		generator.ToneFrequency = int32(scan.Second())
	}
	wide := true
	for track, value := range template {
		value := int32(value)
		if tone && track >= firstBandwidth && track < firstBandwidth+3 && wide {
			value = wideBandwidth
		}
		if value == 0xff {
			switch {
			case track >= firstFormant && track < fourthFormant:
				value = generator.ToneFrequency
				wide = generator.ToneFrequency > wideTone
			case track == pitchTrack:
				value = int32(scan.Second())
			case track == voicingTrack || track == fricationTrack:
				value = toneLevel - generator.cursor().Attenuation
				if generator.ToneFrequency != 0 {
					value -= int32(generator.tables.ToneLevels[generator.ToneFrequency>>4])
				}
				if value < 1 {
					value = 1
				}
			}
		}
		generator.SegmentFrame[track] = byte(value)
	}
}
