package lexical_test

import (
	"fmt"
	"slices"
	"sync"
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
// started from and leave what it left.

type (
	call     = threadstate.Call
	replayer = threadstate.Replayer
)

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
		for function, count := range threadstate.Replay(t, file, guarded) {
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

// A stageRun is a stage that runs a recorded call: the mapping of its state and its run's affixes.
type stageRun struct {
	stage   *lexical.Stage
	mapping threadstate.Mapping
	affixes *lexicalstate.Affixes
}

// load returns a stage with the thread state's values when the call started and the run's user lexicon.
func (data *testData) load(call *call) (*lexical.Stage, *stageRun) {
	stage := lexical.New(data.lexical, data.dictionary, data.parameters, pipeline.New(data.pipeline, new(synthesizer.Synthesizer), nil, nil))
	stage.Lexicon = lexicalstate.NewUserLexicon(call.Case.Lexicon)
	mapping := threadstate.Mapping{Pipeline: stage.Pipeline, Base: call.Base}
	mapping.LoadPipeline(call.Before)
	lexicalstate.Load(mapping, stage, call.Before)
	affixes, _ := data.affixes.LoadOrStore(call.Case, lexicalstate.NewAffixes())
	run := &stageRun{stage: stage, mapping: mapping, affixes: affixes.(*lexicalstate.Affixes)}
	run.affixes.Load(stage, call.Before)
	return stage, run
}

func describeNode(mapping threadstate.Mapping, pointer uint32) string {
	if node := mapping.NodeAt(pointer); node != nil {
		return fmt.Sprintf("node %d", node.Index())
	}
	return fmt.Sprintf("%#x", pointer)
}

// finish reports the first difference of a replayed call: the result, then the state the call left, written over
// the state it started from.
func (run *stageRun) finish(call *call, result error) error {
	if result != nil {
		return result
	}
	state := slices.Clone(call.Before)
	run.mapping.StorePipeline(state)
	lexicalstate.Store(run.mapping, run.stage, state)
	run.affixes.Store(run.stage, state, call.After)
	return threadstate.Differences(state, call.After, lexicalstate.Describe)
}

func expectFlag(call *call, result bool) error {
	if expected := call.Result.EAX&0xff != 0; result != expected {
		return fmt.Errorf("result %t, expected %t", result, expected)
	}
	return nil
}

// TestStage replays every recorded run of the stage's step, word conversion, post-lexical pass and reset.
func TestStage(t *testing.T) {
	data := loadTestData(t)
	counts := replay(t, map[string]replayer{
		"Lexical::Step": func(call *call) error {
			stage, run := data.load(call)
			return run.finish(call, expectFlag(call, stage.Step()))
		},
		"Lexical::ConvertWord": func(call *call) error {
			stage, run := data.load(call)
			return run.finish(call, expectFlag(call, stage.ConvertWord()))
		},
		"Lexical::PostLexicalWord": func(call *call) error {
			stage, run := data.load(call)
			node := stage.PostLexicalWord()
			var result error
			if pointer := run.mapping.PointerTo(node); pointer != call.Result.EAX {
				result = fmt.Errorf("returned %s, expected %s", describeNode(run.mapping, pointer), describeNode(run.mapping, call.Result.EAX))
			}
			return run.finish(call, result)
		},
		"Lexical::Reset": func(call *call) error {
			stage, run := data.load(call)
			stage.Reset()
			stage.Pipeline.Cursors[pipeline.Lexical].Types = 6
			return run.finish(call, nil)
		},
	})
	t.Logf("calls replayed: %v", counts)
}

// TestLookup replays every recorded lookup of a word, whole and by its affixes.
func TestLookup(t *testing.T) {
	data := loadTestData(t)
	counts := replay(t, map[string]replayer{
		"Lexical::MorphLookup": func(call *call) error {
			stage, run := data.load(call)
			return run.finish(call, expectFlag(call, stage.Lookup()))
		},
	})
	t.Logf("calls replayed: %v", counts)
}

// TestLetterToSound replays every recorded conversion of a word's letters by rules.
func TestLetterToSound(t *testing.T) {
	data := loadTestData(t)
	counts := replay(t, map[string]replayer{
		"Lexical::LetterToSound": func(call *call) error {
			stage, run := data.load(call)
			stage.LetterToSound()
			return run.finish(call, nil)
		},
	})
	t.Logf("calls replayed: %v", counts)
}

// TestReduceVowel replays every recorded reduction of the vowel letter-to-sound wrote last.
func TestReduceVowel(t *testing.T) {
	data := loadTestData(t)
	counts := replay(t, map[string]replayer{
		"Lexical::ReduceVowel": func(call *call) error {
			stage, run := data.load(call)
			stage.ReduceVowel(call.Argument(0)&0xff != 0)
			return run.finish(call, nil)
		},
	})
	t.Logf("calls replayed: %v", counts)
}

// TestFunctionWords replays every recorded change of a function word's stress and every reduction of its vowels.
func TestFunctionWords(t *testing.T) {
	data := loadTestData(t)
	counts := replay(t, map[string]replayer{
		"Lexical::FunctionWordDestress": func(call *call) error {
			stage, run := data.load(call)
			stage.FunctionWordStress()
			return run.finish(call, nil)
		},
		"Lexical::FunctionWordReduceVowels": func(call *call) error {
			stage, run := data.load(call)
			stage.FunctionWordReductions()
			return run.finish(call, nil)
		},
	})
	t.Logf("calls replayed: %v", counts)
}

// TestPostLexicalRules replays every recorded run of the post-lexical rules.
func TestPostLexicalRules(t *testing.T) {
	data := loadTestData(t)
	counts := replay(t, map[string]replayer{
		"Lexical::PostLexicalRules": func(call *call) error {
			stage, run := data.load(call)
			node := run.mapping.NodeAt(uint32(call.Argument(0)))
			var result error
			if pointer := run.mapping.PointerTo(stage.PostLexicalRules(node)); pointer != call.Result.EAX {
				result = fmt.Errorf("returned %s, expected %s", describeNode(run.mapping, pointer), describeNode(run.mapping, call.Result.EAX))
			}
			return run.finish(call, result)
		},
	})
	t.Logf("calls replayed: %v", counts)
}

// TestUserLexicon replays every recorded user lexicon lookup.
func TestUserLexicon(t *testing.T) {
	data := loadTestData(t)
	counts := replay(t, map[string]replayer{
		"Lexical::UserLexiconLookup": func(call *call) error {
			stage, run := data.load(call)
			return run.finish(call, expectFlag(call, stage.UserLexiconLookup()))
		},
	})
	t.Logf("calls replayed: %v", counts)
}

// TestDictionary replays every recorded dictionary lookup.
func TestDictionary(t *testing.T) {
	data := loadTestData(t)
	counts := replay(t, map[string]replayer{
		"Lexical::DictionaryLookup": func(call *call) error {
			stage, run := data.load(call)
			return run.finish(call, expectFlag(call, stage.DictionaryLookup()))
		},
	})
	t.Logf("calls replayed: %v", counts)
}
