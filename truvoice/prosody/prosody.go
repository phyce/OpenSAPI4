// Package prosody is the TruVoice engine's prosody stage (docs/truvoice/prosody.md): it ends the phrases, puts
// in their pauses, and gives each phoneme its duration and pitch.
package prosody

import (
	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
)

// A Stage is the prosody stage's state.
type Stage struct {
	Pipeline *pipeline.Pipeline

	// Phonemes is how many phonemes the phrase has, Contour its pitch contour and Marks the positions of its
	// first word marks of values 6, 7, 8 and 17 (prosody.md §3).
	Phonemes int32
	Contour  int32
	Marks    [4]int32

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

	Context Context

	tables  *datapack.Prosody
	classes *datapack.Classes
}

// New returns the stage for the pipeline.
func New(tables *datapack.Prosody, classes *datapack.Classes, pipeline *pipeline.Pipeline) *Stage {
	return &Stage{Pipeline: pipeline, tables: tables, classes: classes}
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
