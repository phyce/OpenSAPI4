package lexical_test

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/internal/conformance"
	"github.com/phyce/OpenSAPI4/truvoice/internal/lexicalstate"
	"github.com/phyce/OpenSAPI4/truvoice/internal/threadstate"
	"github.com/phyce/OpenSAPI4/truvoice/lexical"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
	"github.com/phyce/OpenSAPI4/truvoice/synthesizer"
)

// The lexical stage's golden data (golden/truvoice/lexical.jsonl.gz, dictionary.jsonl.gz, lexicon.jsonl.gz,
// affixes.jsonl.gz, letters.jsonl.gz and postlexical.jsonl.gz) records calls of the original's functions with the
// changes of the speaking thread's state around them. The replay rebuilds the state at every call and return
// (internal/threadstate, internal/lexicalstate), so that a Go function can start from the state the original's
// started from and leave what it left. The stage's parts that later parts port run as the original's recorded
// calls: each starts from the state the Go stage reached, checked against the original's, and leaves what the
// original's left.

type (
	call     = threadstate.Call
	replayer = threadstate.Replayer
)

// recorded are the functions of the parts later parts port.
var recorded = []string{"Lexical::PostLexicalRules"}

func replay(t *testing.T, replayers map[string]replayer) map[string]int {
	t.Helper()
	guarded := map[string]replayer{}
	for function, replayer := range replayers {
		guarded[function] = func(call *call) (err error) {
			defer func() {
				if failure := recover(); failure != nil {
					err = fmt.Errorf("panic: %v", failure)
				}
			}()
			return replayer(call)
		}
	}
	counts := map[string]int{}
	for _, file := range []string{"lexical.jsonl.gz", "dictionary.jsonl.gz", "lexicon.jsonl.gz", "affixes.jsonl.gz", "letters.jsonl.gz", "postlexical.jsonl.gz"} {
		for function, count := range threadstate.Replay(t, file, guarded, recorded...) {
			counts[function] += count
		}
	}
	return counts
}

// testData are the data pack's tables the replays need, and each run's pointers to the affixes.
type testData struct {
	lexical    *datapack.Lexical
	dictionary *datapack.Dictionary
	parameters *datapack.Parameters
	pipeline   *datapack.Pipeline
	affixes    sync.Map
}

func loadTestData(t *testing.T) *testData {
	t.Helper()
	pack := conformance.Pack(t)
	lexicalTables, err := pack.LoadLexical()
	if err != nil {
		t.Fatal(err)
	}
	dictionary, err := pack.LoadDictionary()
	if err != nil {
		t.Fatal(err)
	}
	parameters, err := pack.LoadParameters()
	if err != nil {
		t.Fatal(err)
	}
	pipelineTables, err := pack.LoadPipeline()
	if err != nil {
		t.Fatal(err)
	}
	return &testData{lexical: lexicalTables, dictionary: dictionary, parameters: parameters, pipeline: pipelineTables}
}

// recordedRules stand in for the stage's parts later parts port, with a call's recorded inner calls.
type recordedRules struct {
	stage    *lexical.Stage
	mapping  threadstate.Mapping
	affixes  *lexicalstate.Affixes
	calls    []*call
	position int
	err      error
}

// userLexicon is a run's user lexicon: its words in capitals, the first pronunciation given for each.
type userLexicon map[string]string

func (lexicon userLexicon) Pronunciation(word string) (string, bool) {
	pronunciation, found := lexicon[word]
	return pronunciation, found
}

// load returns a stage with the thread state's values when the call started and the run's user lexicon, whose
// rules are the call's recorded inner calls.
func (data *testData) load(call *call) (*lexical.Stage, *recordedRules) {
	stage := lexical.New(data.lexical, data.dictionary, data.parameters, pipeline.New(data.pipeline, new(synthesizer.Synthesizer), nil, nil))
	if len(call.Case.Lexicon) > 0 {
		lexicon := userLexicon{}
		for _, entry := range call.Case.Lexicon {
			word := strings.ToUpper(entry[0])
			if _, found := lexicon[word]; !found {
				lexicon[word] = entry[1]
			}
		}
		stage.Lexicon = lexicon
	}
	mapping := threadstate.Mapping{Pipeline: stage.Pipeline, Base: call.Base}
	mapping.LoadPipeline(call.Before)
	lexicalstate.Load(mapping, stage, call.Before)
	affixes, _ := data.affixes.LoadOrStore(call.Case, lexicalstate.NewAffixes())
	rules := &recordedRules{stage: stage, mapping: mapping, affixes: affixes.(*lexicalstate.Affixes), calls: call.Inner}
	rules.affixes.Load(stage, call.Before)
	stage.Rules = rules
	return stage, rules
}

// differences reports where the stage's state, written over the state it started from, differs from the
// expected one.
func (rules *recordedRules) differences(before, expected []byte) error {
	state := slices.Clone(before)
	rules.mapping.StorePipeline(state)
	lexicalstate.Store(rules.mapping, rules.stage, state)
	rules.affixes.Store(rules.stage, state, expected)
	return threadstate.Differences(state, expected, lexicalstate.Describe)
}

// take checks that the original made the call next, from the state the Go stage has reached, and gives the
// stage the state the call left.
func (rules *recordedRules) take(function string) *call {
	if rules.err != nil {
		return nil
	}
	if rules.position == len(rules.calls) {
		rules.err = fmt.Errorf("the Go stage called %s, the original didn't", function)
		return nil
	}
	call := rules.calls[rules.position]
	rules.position++
	if call.Record.Function != function {
		rules.err = fmt.Errorf("the Go stage called %s, the original %s", function, call.Record.Function)
		return nil
	}
	if err := rules.differences(call.Before, call.Before); err != nil {
		rules.err = fmt.Errorf("before %s: %v", function, err)
		return nil
	}
	rules.mapping.LoadPipeline(call.After)
	lexicalstate.Load(rules.mapping, rules.stage, call.After)
	return call
}

func (rules *recordedRules) PostLexicalRules(node *pipeline.Node) *pipeline.Node {
	pointer := rules.mapping.PointerTo(node)
	call := rules.take("Lexical::PostLexicalRules")
	if call == nil {
		return node
	}
	if argument := uint32(call.Argument(0)); argument != pointer && rules.err == nil {
		rules.err = fmt.Errorf("the post-lexical rules ran on %s, expected %s", describeNode(rules.mapping, pointer), describeNode(rules.mapping, argument))
	}
	return rules.mapping.NodeAt(call.Result.EAX)
}

func describeNode(mapping threadstate.Mapping, pointer uint32) string {
	if node := mapping.NodeAt(pointer); node != nil {
		return fmt.Sprintf("node %d", node.Index())
	}
	return fmt.Sprintf("%#x", pointer)
}

// finish reports the first difference of a replayed call: an inner call the Go stage didn't make like the
// original, the result, and the state the call left.
func (rules *recordedRules) finish(call *call, result error) error {
	if rules.err != nil {
		return rules.err
	}
	if rules.position < len(rules.calls) {
		return fmt.Errorf("the original called %s, the Go stage didn't", rules.calls[rules.position].Record.Function)
	}
	if result != nil {
		return result
	}
	return rules.differences(call.Before, call.After)
}

func expectFlag(call *call, result bool) error {
	if expected := call.Result.EAX&0xff != 0; result != expected {
		return fmt.Errorf("result %t, expected %t", result, expected)
	}
	return nil
}

// TestStage replays every recorded run of the stage's step, word conversion, post-lexical pass and reset, with
// the original's post-lexical rules.
func TestStage(t *testing.T) {
	data := loadTestData(t)
	counts := replay(t, map[string]replayer{
		"Lexical::Step": func(call *call) error {
			stage, rules := data.load(call)
			return rules.finish(call, expectFlag(call, stage.Step()))
		},
		"Lexical::ConvertWord": func(call *call) error {
			stage, rules := data.load(call)
			return rules.finish(call, expectFlag(call, stage.ConvertWord()))
		},
		"Lexical::PostLexicalWord": func(call *call) error {
			stage, rules := data.load(call)
			node := stage.PostLexicalWord()
			var result error
			if pointer := rules.mapping.PointerTo(node); pointer != call.Result.EAX {
				result = fmt.Errorf("returned %s, expected %s", describeNode(rules.mapping, pointer), describeNode(rules.mapping, call.Result.EAX))
			}
			return rules.finish(call, result)
		},
		"Lexical::Reset": func(call *call) error {
			stage, rules := data.load(call)
			stage.Reset()
			stage.Pipeline.Cursors[pipeline.Lexical].Types = 6
			return rules.finish(call, nil)
		},
	})
	t.Logf("calls replayed: %v", counts)
}

// TestLookup replays every recorded lookup of a word, whole and by its affixes.
func TestLookup(t *testing.T) {
	data := loadTestData(t)
	counts := replay(t, map[string]replayer{
		"Lexical::MorphLookup": func(call *call) error {
			stage, rules := data.load(call)
			return rules.finish(call, expectFlag(call, stage.Lookup()))
		},
	})
	t.Logf("calls replayed: %v", counts)
}

// TestLetterToSound replays every recorded conversion of a word's letters by rules.
func TestLetterToSound(t *testing.T) {
	data := loadTestData(t)
	counts := replay(t, map[string]replayer{
		"Lexical::LetterToSound": func(call *call) error {
			stage, rules := data.load(call)
			stage.LetterToSound()
			return rules.finish(call, nil)
		},
	})
	t.Logf("calls replayed: %v", counts)
}

// TestReduceVowel replays every recorded reduction of the vowel letter-to-sound wrote last.
func TestReduceVowel(t *testing.T) {
	data := loadTestData(t)
	counts := replay(t, map[string]replayer{
		"Lexical::ReduceVowel": func(call *call) error {
			stage, rules := data.load(call)
			stage.ReduceVowel(call.Argument(0)&0xff != 0)
			return rules.finish(call, nil)
		},
	})
	t.Logf("calls replayed: %v", counts)
}

// TestFunctionWords replays every recorded change of a function word's stress and every reduction of its vowels.
func TestFunctionWords(t *testing.T) {
	data := loadTestData(t)
	counts := replay(t, map[string]replayer{
		"Lexical::FunctionWordDestress": func(call *call) error {
			stage, rules := data.load(call)
			stage.FunctionWordStress()
			return rules.finish(call, nil)
		},
		"Lexical::FunctionWordReduceVowels": func(call *call) error {
			stage, rules := data.load(call)
			stage.FunctionWordReductions()
			return rules.finish(call, nil)
		},
	})
	t.Logf("calls replayed: %v", counts)
}

// TestPostLexicalRules replays every recorded run of the post-lexical rules. The rules for consonants come in a
// later part: a run on a consonant only counts whether it matches.
func TestPostLexicalRules(t *testing.T) {
	data := loadTestData(t)
	var matched, left atomic.Int64
	counts := replay(t, map[string]replayer{
		"Lexical::PostLexicalRules": func(call *call) error {
			stage, rules := data.load(call)
			node := rules.mapping.NodeAt(uint32(call.Argument(0)))
			symbol := node.Symbol()
			consonant := symbol < datapack.SymbolCount && data.parameters.Classes[1][symbol]&0x20 != 0
			var result error
			if pointer := rules.mapping.PointerTo(stage.PostLexicalRules(node)); pointer != call.Result.EAX {
				result = fmt.Errorf("returned %s, expected %s", describeNode(rules.mapping, pointer), describeNode(rules.mapping, call.Result.EAX))
			}
			err := rules.finish(call, result)
			if !consonant {
				return err
			}
			if err != nil {
				left.Add(1)
			} else {
				matched.Add(1)
			}
			return nil
		},
	})
	t.Logf("calls replayed: %v; consonants matched %d, left %d", counts, matched.Load(), left.Load())
}

// TestUserLexicon replays every recorded user lexicon lookup.
func TestUserLexicon(t *testing.T) {
	data := loadTestData(t)
	counts := replay(t, map[string]replayer{
		"Lexical::UserLexiconLookup": func(call *call) error {
			stage, rules := data.load(call)
			return rules.finish(call, expectFlag(call, stage.UserLexiconLookup()))
		},
	})
	t.Logf("calls replayed: %v", counts)
}

// TestDictionary replays every recorded dictionary lookup.
func TestDictionary(t *testing.T) {
	data := loadTestData(t)
	counts := replay(t, map[string]replayer{
		"Lexical::DictionaryLookup": func(call *call) error {
			stage, rules := data.load(call)
			return rules.finish(call, expectFlag(call, stage.DictionaryLookup()))
		},
	})
	t.Logf("calls replayed: %v", counts)
}
