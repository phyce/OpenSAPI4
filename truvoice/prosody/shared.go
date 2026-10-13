package prosody

import (
	"sync"

	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
)

// Shared is the stage's state that the original keeps in global variables, so that every speaking thread of the
// process shares it (prosody.md §3.1). A stage runs with it locked.
type Shared struct {
	// The scan: the nodes it has looked at, whether a phoneme has come since the last break, the last node that
	// isn't a phoneme, the phonemes since that node, the phonemes it has counted toward the phrase's limit, and
	// whether a P command came before any phoneme.
	Scanned  int32
	InPhrase bool
	LastNode *pipeline.Node
	Pending  int32
	Counted  int32
	Held     bool

	// The walk, and the scan's result it puts back when the phrase's walk ends.
	Walk        int32
	SavedResult int32

	// The presets of the current node (prosody.md §5.3): its symbol, its pitch and the node.
	PresetSymbol byte
	PresetPitch  uint32
	PresetNode   *pipeline.Node

	// SlowPauses counts the slow speech's chances of a pause between words (prosody.md §3.4).
	SlowPauses uint32

	sync.Mutex
}

var processShared Shared
