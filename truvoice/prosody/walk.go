package prosody

import "github.com/phyce/OpenSAPI4/truvoice/pipeline"

// What NextNode reports.
const (
	phraseDone  = 1 + iota // the phrase or the window has ended
	controlNode            // a node to run as a control command
	phonemeNode            // a node of type 3
	yieldNode              // the stage stops for the stages after it
)

// The walk's states: at the phrase's start, on a node, and on the phrase's last node.
const (
	walkStart = 1 + iota
	walkOn
	walkLast
)

// The nodes a run walks before it stops whenever the synthesizer is fewer than starvingFrames frames ahead.
const (
	walkCountdown  = 2
	starvingFrames = 33
)

// pauseMark is the value of a word mark that puts in a pause as long as its phonemes since the last one.
const pauseMark = 12

// NextNode moves the walk to the phrase's next node and reports what to do with it (prosody.md §5.1).
func (stage *Stage) NextNode() int {
	shared := stage.Shared
	cursor := stage.cursor()
	list := stage.Pipeline
	if stage.ScanResult != walking {
		shared.Walk = walkStart
		shared.SavedResult = stage.ScanResult
	}
	stage.ScanResult = walking
	if shared.Walk == walkLast {
		end := list.Next(cursor.First)
		stage.clearPauseStress(cursor.Done, end)
		cursor.First, cursor.Scan, cursor.Done = end, end, end
		stage.ScanResult = shared.SavedResult
		shared.Walk = walkStart
		return phraseDone
	}
	node := cursor.Scan
	if node.Type() == pipeline.Phoneme && node.Symbol() == ',' && !stage.ProsodyOff && !stage.NoPhrasing && shared.Walk != walkStart {
		end := list.Next(node)
		stage.clearPauseStress(cursor.Done, end)
		cursor.Done, cursor.Scan = node, end
		shared.Walk = walkStart
		return yieldNode
	}
	stage.Countdown--
	synthesizer := list.Synthesizer
	if stage.Countdown < 0 && synthesizer.Written-synthesizer.Lookahead-synthesizer.Position < starvingFrames {
		return yieldNode
	}
	if shared.Walk == walkOn {
		cursor.Scan = list.Next(node)
	} else {
		shared.Walk = walkOn
	}
	if cursor.Scan == cursor.First {
		shared.Walk = walkLast
	}
	if cursor.Scan == nil {
		cursor.First, cursor.Scan, cursor.Done = nil, nil, nil
		stage.ScanResult = shared.SavedResult
		shared.Walk = walkStart
		return phraseDone
	}
	if cursor.Scan.Type() == pipeline.Phoneme {
		return phonemeNode
	}
	return controlNode
}

// clearPauseStress clears the stress bits of the pauses from a node up to end.
func (stage *Stage) clearPauseStress(node, end *pipeline.Node) {
	for ; node != nil && node != end; node = stage.Pipeline.Next(node) {
		if node.Type() == pipeline.Segment && node.Symbol() == ' ' {
			node.SetFlags(node.Flags() &^ stressBits)
		}
	}
}

// ProcessPhoneme times the current node of type 3 and gives it its pitch, puts in the pauses at a break and at a
// word mark of value 12, and turns a phoneme into a segment (prosody.md §5.2).
func (stage *Stage) ProcessPhoneme() {
	stage.LoadContext()
	stage.ApplyPresets(false)
	stage.DurationAndPitch()
	cursor := stage.cursor()
	symbol := stage.Context.Symbol
	if stage.hasClass(symbol, wordMark) && cursor.Scan.Value() == pauseMark {
		stage.InsertPause(stage.SinceMark)
		stage.SinceMark = 0
	}
	if stage.hasClass(symbol, phraseBreak) && !(symbol == ']' && afterOpening(cursor.Scan)) {
		stage.InsertPause(0)
	}
	if node := cursor.Scan; stage.hasClass(node.Symbol(), phoneme) {
		node.SetFlags(node.Flags()&^pipeline.Phoneme | pipeline.Segment)
		stage.SinceMark++
		stage.ApplyPresets(true)
	}
}

// afterOpening reports whether the node follows, from the list's start, only a generated pause and a bookmark.
func afterOpening(node *pipeline.Node) bool {
	bookmark := node.Previous
	if bookmark == nil || bookmark.Type() != pipeline.Control || bookmark.Symbol() != 'i' {
		return false
	}
	pause := bookmark.Previous
	return pause != nil && pause.Type() == pipeline.Generated && pause.Symbol() == 's' &&
		pause.Previous != nil && pause.Previous.Type() == pipeline.Sentinel
}

// DurationAndPitch gives the current node its pitch and its duration.
func (stage *Stage) DurationAndPitch() {
	stage.PitchContext()
	stage.AssignPitch()
	stage.AssignDuration()
}

// The presets' bits: a duration, a pitch, and both relative to the stage's.
const (
	durationPreset = 1
	pitchPreset    = 2
	relativePreset = 4
)

// The node's flag of relative presets, and the presets' limits.
const (
	relativeFlag     = 0x80
	pausePresetLimit = 50
	shortestPreset   = 2
	longestPreset    = 60
	lowestPreset     = 25
	highestPreset    = 100
)

// ApplyPresets notes the current node's preset duration and pitch, its value and second value as phonetic input
// gives them, before the stage times it; after, it puts them back in place of the stage's (prosody.md §5.3).
func (stage *Stage) ApplyPresets(after bool) {
	node := stage.cursor().Scan
	shared := stage.Shared
	if !after {
		shared.PresetPitch = uint32(node.Second())
		shared.PresetNode = node
		stage.PresetDuration = int32(node.Value())
		shared.PresetSymbol = node.Symbol()
		if shared.PresetPitch&0x80 != 0 {
			shared.PresetPitch |= 0xff00
		}
		if stage.PresetDuration&0x80 != 0 {
			stage.PresetDuration |= 0xff00
		}
		stage.PreviousPresets = stage.Presets
		stage.Presets = 0
		if node.Flags()&relativeFlag != 0 {
			stage.Presets = relativePreset
		}
		if stage.PresetDuration != 0 {
			stage.Presets |= durationPreset
		}
		if shared.PresetPitch != 0 {
			stage.Presets |= pitchPreset
		}
		if shared.PresetSymbol == ' ' && stage.PresetDuration > pausePresetLimit {
			stage.PresetDuration = pausePresetLimit
		}
		return
	}
	if shared.PresetNode != node || shared.PresetSymbol != node.Symbol() {
		return
	}
	if stage.Presets&durationPreset != 0 {
		if stage.Presets&relativePreset != 0 {
			stage.PresetDuration = min(max(stage.PresetDuration+int32(node.Value()), shortestPreset), longestPreset)
		}
		node.SetValue(uint32(stage.PresetDuration))
	}
	if stage.Presets&pitchPreset == 0 {
		return
	}
	if stage.Presets&relativePreset == 0 {
		if shared.PresetPitch == 1 {
			shared.PresetPitch = 0
		}
	} else {
		shared.PresetPitch += uint32(node.Second())
		shared.PresetPitch = uint32(min(max(int32(shared.PresetPitch), lowestPreset), highestPreset))
	}
	node.SetSecond(byte(shared.PresetPitch))
}

// The pauses' lengths in frames, and the second value they get.
const (
	sentencePause  = 39
	commaPause     = 10
	markPause      = 4
	bracketPause   = 12
	otherPause     = 8
	shortestPause  = 30
	longestPause   = 80
	pauseLength    = 50
	fastPause      = 3
	flatPause      = 2
	pauseSecond    = 50
	slowPauseRates = 6
)

// InsertPause puts pauses before the current node: at a break, or as long as length when it is above 0 (prosody.md
// §5.4).
func (stage *Stage) InsertPause(length int32) {
	symbol := stage.Context.Symbol
	cursor := stage.cursor()
	if symbol == ')' && cursor.Rate >= slowPauseRates {
		return
	}
	if cursor.Rate >= fastRate || stage.ProsodyOff {
		switch {
		case length == 0 && symbol == ']':
			stage.newPause(bracketPause)
		case stage.ProsodyOff:
			stage.newPause(flatPause)
			stage.newPause(flatPause)
		default:
			stage.newPause(fastPause)
			stage.newPause(fastPause)
		}
		stage.LoadContext()
		return
	}
	split := true
	frames := min(max(length, shortestPause), longestPause)
	if length < 1 {
		switch symbol {
		case '!', '.', '?':
			frames = sentencePause
		case ',':
			split, frames = false, commaPause
		case '\\':
			split, frames = false, markPause
		case ']':
			split, frames = false, bracketPause
		default:
			split, frames = false, otherPause
		}
	}
	for frames > 3 {
		if split && frames < pauseLength {
			stage.newPause(frames / 3)
			stage.newPause(frames * 2 / 3)
		} else {
			stage.newPause(min(frames, pauseLength))
		}
		frames -= pauseLength
		if frames > 0 {
			frames = max(frames, 4)
		}
		if stage.Pipeline.Free < 3 {
			break
		}
	}
	stage.LoadContext()
}

// newPause puts a pause of the frames before the current node.
func (stage *Stage) newPause(frames int32) {
	pause := stage.Pipeline.NewNode(stage.cursor().Scan, false, pipeline.Segment, ' ')
	pause.SetValue(uint32(frames))
	pause.SetSecond(pauseSecond)
}
