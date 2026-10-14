// Package lexicalstate maps the lexical stage's state to the speaking thread's state the TruVoice golden data
// records, and back, for the tests.
package lexicalstate

import (
	"encoding/binary"
	"sync"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/internal/threadstate"
	"github.com/phyce/OpenSAPI4/truvoice/lexical"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
	"github.com/phyce/OpenSAPI4/truvoice/synthesizer"
)

// Fields lists the stage's values with their places in the thread state.
func Fields(stage *lexical.Stage) []threadstate.Field {
	at := threadstate.At
	return []threadstate.Field{at(0x1c20, "filtered", &stage.Filtered), at(0x1c29, "stress pattern", &stage.StressPattern),
		at(0x1c2a, "emphasized", &stage.Emphasized), at(0x1c2b, "deemphasized", &stage.Deemphasized),
		at(0x1c2c, "word class", &stage.WordClass), at(0x1c2d, "affix stress", &stage.AffixStress),
		at(0x1c30, "previous word", &stage.PreviousWord), at(0x1c34, "previous end", &stage.PreviousEnd),
		at(0x1c38, "previous symbol", &stage.PreviousSymbol), at(0x1c3c, "word", &stage.Word),
		at(0x1c40, "first letter", &stage.FirstLetter), at(0x1c44, "last node", &stage.LastNode),
		at(0x1c48, "search", &stage.Search), at(0x1c4c, "searched", &stage.Searched),
		at(0x1c50, "alternating", &stage.Alternating), at(0x1c28, "tried e", &stage.TriedE),
		at(0x1c6c, "consonants", &stage.Consonants)}
}

// Unported are the thread state's bytes of the stage's parts later parts port: letter-to-sound's state (0x1c54
// to 0x1c73) but its consonants. The tests take them from the records.
var Unported = [][2]int{{0x1c54, 0x1c6c}, {0x1c70, 0x1c74}}

// AffixPointer is where the thread state keeps the stage's affix.
const AffixPointer = 0x1c24

// Affixes maps the stage's affix to its pointer in a run's thread states. The pointers point into the original's
// own tables, which the golden data doesn't record: the first pointer an affix is stored against becomes the
// affix's, and an affix stored against another's pointer differs from it. A pointer no affix has yet loads as
// an affix of its own, which stores as that pointer.
type Affixes struct {
	pointers map[*datapack.Affix]uint32
	affixes  map[uint32]*datapack.Affix
	loaded   uint32
}

var unknownAffix = new(datapack.Affix)

func NewAffixes() *Affixes {
	return &Affixes{pointers: map[*datapack.Affix]uint32{}, affixes: map[uint32]*datapack.Affix{}}
}

// Load gives the stage the thread state's affix.
func (affixes *Affixes) Load(stage *lexical.Stage, state []byte) {
	pointer := threadstate.Dword(state, AffixPointer)
	affixes.loaded = pointer
	stage.Affix = affixes.affixes[pointer]
	if stage.Affix == nil && pointer != 0 {
		stage.Affix = unknownAffix
	}
}

// Store writes the stage's affix into the thread state, mapping an affix without a pointer to the expected
// state's.
func (affixes *Affixes) Store(stage *lexical.Stage, state, expected []byte) {
	var pointer uint32
	switch affix := stage.Affix; affix {
	case nil:
	case unknownAffix:
		pointer = affixes.loaded
	default:
		var known bool
		if pointer, known = affixes.pointers[affix]; !known {
			pointer = threadstate.Dword(expected, AffixPointer)
			if _, taken := affixes.affixes[pointer]; pointer == 0 || taken {
				pointer = ^pointer
			} else {
				affixes.pointers[affix], affixes.affixes[pointer] = pointer, affix
			}
		}
	}
	binary.LittleEndian.PutUint32(state[AffixPointer:], pointer)
}

// TakeUnported copies the unported bytes of the expected state into the state.
func TakeUnported(state, expected []byte) {
	for _, span := range Unported {
		copy(state[span[0]:span[1]], expected[span[0]:span[1]])
	}
}

// Load gives the stage the thread state's values, but its pipeline's.
func Load(mapping threadstate.Mapping, stage *lexical.Stage, state []byte) {
	mapping.LoadFields(Fields(stage), state)
}

// Store writes the stage's values, but its pipeline's, into the thread state.
func Store(mapping threadstate.Mapping, stage *lexical.Stage, state []byte) {
	mapping.StoreFields(Fields(stage), state)
}

// Names names the thread state's bytes the stage's and its pipeline's values cover.
var Names = sync.OnceValue(func() map[int]string {
	stage := lexical.New(nil, nil, nil, pipeline.New(nil, new(synthesizer.Synthesizer), nil, nil))
	names := threadstate.Names(Fields(stage), threadstate.PipelineFields(stage.Pipeline))
	for offset := range 4 {
		names[AffixPointer+offset] = "affix"
	}
	return names
})

// Describe names a byte of the thread state, for the reports.
func Describe(offset int) string {
	return threadstate.Describe(offset, Names())
}
