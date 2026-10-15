// Package lexical is the TruVoice engine's lexical stage (docs/truvoice/lexical.md): it turns the words' letters
// into phonemes, marks their stress and their word class, and applies the post-lexical rules.
package lexical

import (
	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
)

// A Stage is the lexical stage's state.
type Stage struct {
	Pipeline *pipeline.Pipeline
	Lexicon  Lexicon

	// The words (lexical.md §2.3): the previous word's header and last node, the current word's header, first
	// letter and last node, the node the search for the next word has reached and the nodes it has passed.
	PreviousWord, PreviousEnd   *pipeline.Node
	Word, FirstLetter, LastNode *pipeline.Node
	Search                      *pipeline.Node
	Searched                    int32

	// The current word's conversion (lexical.md §3): whether it is emphasized or not, its stress pattern from
	// the dictionary, its word class and the stress its affix gives.
	Emphasized    bool
	Deemphasized  bool
	StressPattern byte
	WordClass     byte
	AffixStress   byte

	// The post-lexical pass (lexical.md §4): the words the filter has counted, whether a schwa may become a
	// barred i, and the previous word's last symbol.
	Filtered       int32
	Alternating    bool
	PreviousSymbol int32

	// The lookup by affixes (lexical.md §9): the suffix it found last, and whether a stem was looked up with a
	// silent E and not found.
	Affix  *datapack.Affix
	TriedE bool

	// Letter-to-sound (lexical.md §10): the letter it converts and the node it reached, its count of consonants,
	// which a context can test (lexical.md §8.1), the vowel it wrote last and whether that vowel may be reduced,
	// whether it converts the letters after a stem the lookup found, and what follows the letter, which its rules
	// test.
	Letter, Reached   *pipeline.Node
	Consonants        int32
	Vowel             *pipeline.Node
	Reducible         bool
	AfterStem         bool
	Following, Ending uint32

	tables     *datapack.Lexical
	dictionary *datapack.Dictionary
	classes    *datapack.Classes
}

// New returns the stage for the pipeline.
func New(tables *datapack.Lexical, dictionary *datapack.Dictionary, parameters *datapack.Parameters, pipeline *pipeline.Pipeline) *Stage {
	stage := &Stage{Pipeline: pipeline, tables: tables, dictionary: dictionary}
	if parameters != nil {
		stage.classes = &parameters.Classes
	}
	return stage
}

func (stage *Stage) cursor() *pipeline.Cursor {
	return &stage.Pipeline.Cursors[pipeline.Lexical]
}

// A class is a symbol class (lexical.md §1.2): bits of a class byte, the byte's plane times 256 plus the bits.
type class int32

const (
	syllabic    class = 0x001
	sonorant    class = 0x002
	voiced      class = 0x004
	wordMark    class = 0x008
	nasal       class = 0x010
	stop        class = 0x020
	fricative   class = 0x040
	phoneme     class = 0x080
	boundary    class = 0x101
	stressMark  class = 0x102
	neutral     class = 0x110
	consonant   class = 0x120
	phraseBreak class = 0x140
	closure     class = 0x201
	vowel       class = 0x202
	alveolar    class = 0x204
	front       class = 0x220
	velar       class = 0x280
	approximant class = 0x301
	palatal     class = 0x304
	rounded     class = 0x320
	yGlide      class = 0x340
	high        class = 0x401
	rColoured   class = 0x402
	reduced     class = 0x440
)

// hasClass reports whether the symbol is in the class. Symbols of 128 and up are in none.
func (stage *Stage) hasClass(symbol byte, kind class) bool {
	return symbol < datapack.SymbolCount && stage.classes[kind>>8][symbol]&byte(kind) != 0
}

// The places of a node's bytes in its data: the byte the stages before set on a word (lexical.md §1.1), the
// syllable's number, the stop a flap stands for and the word class.
const (
	markByteOffset  = 0x14 - 8
	syllableOffset  = 0x17 - 8
	originalOffset  = 0x18 - 8
	wordClassOffset = 0x19 - 8
)

// The flag bits of a node: the type's bit the stage clears to make a word's header a phoneme node, a word's
// code flag, its stress, and whether its syllable is stressed.
const (
	letterBit   = 4
	codeFlag    = 0x20
	stressBits  = 0x18
	stressShift = 3
	onsetFlag   = 0x20
)

// Reset starts the stage afresh, as the pipeline's reset does (lexical.md §5).
func (stage *Stage) Reset() {
	stage.Word, stage.FirstLetter, stage.LastNode = nil, nil, nil
	stage.PreviousWord, stage.PreviousEnd = nil, nil
	stage.Filtered = 0
	stage.PreviousSymbol = 0
	stage.Alternating = false
}
