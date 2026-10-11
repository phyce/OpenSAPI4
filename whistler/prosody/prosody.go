package prosody

import "github.com/phyce/OpenSAPI4/whistler/datapack"

type EventKind byte

const (
	SentenceEvent EventKind = 1
	WordEvent     EventKind = 2
	PhoneEvent    EventKind = 4
	MarkerEvent   EventKind = 8
	PauseEvent    EventKind = 0x20
)

type Event struct {
	Kind  EventKind
	Value int
	Time  int
}

type Point struct {
	Time  int32
	Value int16
}

// Sentence is the analyser state after module A group 16. Every per-cell slice is indexed by cell
// and holds cells 0..End; cell 0 is not read.
type Sentence struct {
	Start         int
	End           int
	Tier1         []byte
	Tier2         []byte
	Tier3         []byte
	Tier4         []byte
	Tier5         []byte
	Transcribed   []byte
	Durations     []int16
	Points        []Point
	PointIndex    []int16
	Anchors       []byte
	Events        []Event
	MultipleWords bool
	DurationStyle int
}

// Analysis holds the analyser's results. Per-cell slices are indexed by cell.
type Analysis struct {
	Start     int
	End       int
	Tier1     []byte
	Durations []int16
	Marks     []byte
	Units     []int16
	Frames    []int16
	Closures  []byte
	Events    []Event
	Targets   []Target
	Text      []byte
}

const (
	edge      = 0x0E
	link      = 0x19
	rate      = 40
	warpLimit = 0xEFC
)

type analyser struct {
	tables        *datapack.Prosody
	start         int
	end           int
	tier1         []byte
	tier2         []byte
	tier3         []byte
	tier4         []byte
	tier5         []byte
	durations     []int16
	marks         []byte
	units         []int16
	frames        []int16
	closures      []byte
	multipleWords bool
	finalScale    int
	durationScale int
}

func cells(tier []byte, end int) []byte {
	copied := make([]byte, end+3)
	copy(copied[1:end+1], tier[1:end+1])
	copied[end+1] = copied[end]
	copied[end+2] = edge
	return copied
}

// Analyze runs steps 6 to 15 of the analysis (prosody.md §5 to §9): durations, analyser units,
// event times, intonation and the analysis string. It reports false when the sentence holds nothing
// but pauses and is dropped.
func Analyze(tables *datapack.Prosody, sentence *Sentence) (*Analysis, bool) {
	end := sentence.End
	analyser := &analyser{
		tables:        tables,
		start:         sentence.Start,
		end:           end,
		tier1:         cells(sentence.Tier1, end),
		tier2:         cells(sentence.Tier2, end),
		tier3:         cells(sentence.Tier3, end),
		tier4:         cells(sentence.Tier4, end),
		tier5:         cells(sentence.Tier5, end),
		durations:     make([]int16, end+3),
		marks:         make([]byte, end+3),
		units:         make([]int16, end+3),
		frames:        make([]int16, end+3),
		closures:      make([]byte, end+3),
		multipleWords: sentence.MultipleWords,
	}
	copy(analyser.durations[1:end+1], sentence.Durations[1:end+1])
	analyser.finalScale, analyser.durationScale = 100, 100
	if sentence.DurationStyle == 1 {
		analyser.finalScale, analyser.durationScale = 120, 107
	}
	analyser.markPhrasePositions()
	analyser.computeDurations()
	if analyser.onlyPauses() {
		return nil, false
	}
	for cell := analyser.start; cell <= analyser.end; cell++ {
		if analyser.tier1[cell] == ' ' {
			analyser.tier1[cell] = '-'
		}
	}
	if analyser.tier1[analyser.end] == '-' {
		analyser.end--
	}
	analyser.assignUnits()
	analyser.fitDurations()
	events := analyser.assignEventTimes(sentence)
	targets := analyser.intonate(sentence)
	return &Analysis{
		Start:     analyser.start,
		End:       analyser.end,
		Tier1:     analyser.tier1,
		Durations: analyser.durations,
		Marks:     analyser.marks,
		Units:     analyser.units,
		Frames:    analyser.frames,
		Closures:  analyser.closures,
		Events:    events,
		Targets:   targets,
		Text:      analyser.analysisText(targets),
	}, true
}

func (analyser *analyser) class(code byte) byte {
	return analyser.tables.PhoneClasses[code]
}

func (analyser *analyser) feature(code byte) byte {
	return analyser.tables.PhoneFeatures[code]
}

func (analyser *analyser) vowelLike(code byte) bool {
	return analyser.class(code) == 'V' || code == 'L' || code == 0xA4
}

func (analyser *analyser) consonant(code byte) bool {
	return analyser.class(code) <= 'Q'
}

func (analyser *analyser) mark(cell int) byte {
	if cell < 0 {
		return 0
	}
	return analyser.marks[cell]
}

func (analyser *analyser) onlyPauses() bool {
	index := analyser.start - 1
	for index < analyser.end {
		code := analyser.tier1[index+1]
		if code != '#' && code != '~' && code != ' ' {
			break
		}
		index++
	}
	return index == analyser.end
}
