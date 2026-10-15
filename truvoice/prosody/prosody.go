// Package prosody is the TruVoice engine's prosody stage (docs/truvoice/prosody.md): it ends the phrases, puts
// in their pauses, and gives each phoneme its duration and pitch.
package prosody

import (
	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
)

// MaximumWords is how many word marks the scan keeps for a phrase.
const MaximumWords = 50

// A Stage is the prosody stage's state.
type Stage struct {
	Pipeline *pipeline.Pipeline
	Shared   *Shared

	// The phrase's scan (prosody.md §3): what ended the last one, its word marks and the sentence's before it,
	// and the phonemes: the phrase's, the sentence's up to its end, and the padding added to a phrase without an
	// end of its own. The phrase ends at PhraseLimit phonemes. SlowSpeech, NoPhrasing and the pipeline's ProsodyOff
	// are the commands' settings, and FirstPhrase is set until the sentence's first P command is put in.
	ScanResult       int32
	Words            int32
	SentenceWords    int32
	WordMarks        [MaximumWords]*pipeline.Node
	Phonemes         int32
	SentencePhonemes int32
	Padding          int32
	PhraseLimit      int32
	SlowSpeech       bool
	NoPhrasing       bool
	FirstPhrase      bool

	// Contour is the phrase's pitch contour and Marks the positions of its word marks of values 6, 7, 8 and 17
	// (prosody.md §3.5).
	Contour int32
	Marks   [4]int32

	// The walk (prosody.md §5): the nodes it may take before it lets the synthesizer catch up, the phonemes
	// since the last word mark of value 12, and the current node's presets (§5.3).
	Countdown       int32
	SinceMark       int32
	Presets         uint32
	PreviousPresets uint32
	PresetDuration  int32

	// The pitch (prosody.md §4): Position is the phrase's phonemes given a pitch so far, Fall the pitch of the
	// phrase's final fall once it has started, Nucleus the phrase's last stressed vowel before a break, and
	// FollowingConsonant the kind of consonant after the current phoneme. The flags say whether the pitch rises
	// toward the nucleus, whether the current phoneme is in the word's last syllable and in the phrase's,
	// whether it is the nucleus, whether a stressed syllable has come, and whether word marks of value 12 and 17
	// have come.
	Position           int32
	Fall               int32
	Nucleus            *pipeline.Node
	FollowingConsonant int32
	Rise               bool
	WordEnd            bool
	PhraseEnd          bool
	AtNucleus          bool
	AfterStress        bool
	Raised             bool
	Lowered            bool

	// The durations (prosody.md §6): the current phoneme's inherent and shortest durations in tenths of a frame,
	// the percents of the difference its base duration takes and their product, the emphasis of the word, and
	// whether the phoneme was merged from a pair of the same sound.
	Inherent int32
	Shortest int32
	Percents [4]int32
	Emphasis int32
	Geminate bool

	Context Context

	tables     *datapack.Prosody
	parameters *datapack.Parameters
	classes    *datapack.Classes
}

// New returns the stage for the pipeline, with the process's shared state.
func New(tables *datapack.Prosody, parameters *datapack.Parameters, pipeline *pipeline.Pipeline) *Stage {
	stage := &Stage{Pipeline: pipeline, Shared: &processShared, tables: tables, parameters: parameters}
	if parameters != nil {
		stage.classes = &parameters.Classes
	}
	return stage
}

func (stage *Stage) cursor() *pipeline.Cursor {
	return &stage.Pipeline.Cursors[pipeline.Prosody]
}

// The flag bits of a phoneme's node: its stress, 0 to 3, whether its syllable is stressed, and whether it is
// the phrase's nucleus.
const (
	stressBits   = 0x18
	stressShift  = 3
	stressedFlag = 0x20
	nucleusFlag  = 0x80
)

func stress(node *pipeline.Node) uint32 {
	return node.Flags() & stressBits >> stressShift
}

func multiply(first, second int32) int32 {
	return first * second / 0x7fff
}

// The scan's results (prosody.md §3.2): what ended the phrase, or scanning while the window has run out first,
// or walking while the stage walks the phrase.
const (
	scanning = iota
	continued
	walking
	sentenceEnd
	brokenOff
	commaEnd
)

// The phrase's limits: the phonemes by default, without prosody and while the parameter generator is short of
// work, and the nodes a scan looks at.
const (
	phraseLimit      = 180
	flatPhraseLimit  = 15
	shortPhraseLimit = 70
	scanLimit        = 240
)

// Rates of fastRate and up are fast speech, below slowRates slow speech.
const (
	fastRate  = 25
	slowRates = datapack.SlowRateCount
)

// Reset starts the stage afresh, as the pipeline's reset does (prosody.md §7).
func (stage *Stage) Reset() {
	stage.Marks = [4]int32{-1, -1, -1, 0}
	stage.ScanResult = sentenceEnd
	stage.NoPhrasing = false
	stage.SlowSpeech = false
	stage.PhraseLimit = phraseLimit
	stage.Pipeline.ProsodyOff = false
	stage.Pipeline.ShortPhrase = false
	stage.SinceMark = 0
	stage.Lowered = false
	stage.Emphasis = 0
	stage.Padding = 0
	stage.PreviousPresets = 0
	stage.Presets = 0
}

// Run is one run of the stage: it scans the next phrase unless it is walking one, and walks the phrase's nodes
// until the phrase or the window ends, or the synthesizer needs the stages after it. It reports whether it handed
// nodes on or stopped for the synthesizer (prosody.md §5.1).
func (stage *Stage) Run() bool {
	stage.Shared.Lock()
	defer stage.Shared.Unlock()
	cursor := stage.cursor()
	stage.Pipeline.Begin(cursor)
	stage.Countdown = walkCountdown
	if !stage.Pipeline.ProsodyOff && stage.Pipeline.ShortPhrase && cursor.Rate < fastRate {
		stage.PhraseLimit = shortPhraseLimit
	}
	result := stage.ScanPhrase()
	switch result {
	case continued, brokenOff:
		stage.Padding = stage.Phonemes>>2 + 4
		stage.Phonemes += stage.Padding
		fallthrough
	case sentenceEnd, commaEnd:
		cursor.Scan = cursor.Done
		fallthrough
	case walking:
		if !stage.Pipeline.ProsodyOff {
			stage.Pipeline.ShortPhrase = false
		}
	}
	yielded := false
walk:
	for result != scanning {
		switch stage.NextNode() {
		case controlNode:
			stage.Pipeline.ExecuteControl()
		case phonemeNode:
			stage.ProcessPhoneme()
		case yieldNode:
			yielded = true
			break walk
		case phraseDone:
			break walk
		}
	}
	return stage.Pipeline.Finish() || yielded
}
