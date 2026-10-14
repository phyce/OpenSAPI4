// Package lexicalstate maps the lexical stage's state to the speaking thread's state the TruVoice golden data
// records, and back, for the tests.
package lexicalstate

import (
	"sync"

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
		at(0x1c50, "alternating", &stage.Alternating)}
}

// Unported are the thread state's bytes of the stage's parts later parts port: the affix found and its stem's
// repair (0x1c24 to 0x1c28), and letter-to-sound's state (0x1c54 to 0x1c73). The tests take them from the
// records.
var Unported = [][2]int{{0x1c24, 0x1c29}, {0x1c54, 0x1c74}}

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
	return threadstate.Names(Fields(stage), threadstate.PipelineFields(stage.Pipeline))
})

// Describe names a byte of the thread state, for the reports.
func Describe(offset int) string {
	return threadstate.Describe(offset, Names())
}
