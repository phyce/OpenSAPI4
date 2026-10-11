package whistler

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"unicode/utf16"

	"github.com/phyce/OpenSAPI4/whistler/internal/conformance"
	"github.com/phyce/OpenSAPI4/whistler/lexicon"
	"github.com/phyce/OpenSAPI4/whistler/text"
)

type lexiconStep struct {
	Call          string  `json:"call"`
	Wide          bool    `json:"wide"`
	Lexicon       uint32  `json:"lexicon"`
	CharacterSet  uint32  `json:"characterSet"`
	Word          string  `json:"word"`
	Pronunciation string  `json:"pronunciation"`
	Sense         uint32  `json:"sense"`
	Size          uint32  `json:"size"`
	Text          string  `json:"text"`
	Result        string  `json:"result"`
	Returned      *string `json:"returned"`
}

type lexiconCase struct {
	Number          int               `json:"number"`
	Group           string            `json:"group"`
	Mode            string            `json:"mode"`
	File            *string           `json:"file"`
	Steps           []lexiconStep     `json:"steps"`
	Samples         int64             `json:"samples"`
	Digest          string            `json:"digest"`
	Blocks          []string          `json:"blocks"`
	Events          []json.RawMessage `json:"events"`
	AttributeEvents struct {
		Pitch       []string `json:"pitch"`
		SpeedVolume []string `json:"speedVolume"`
	} `json:"attributeEvents"`
	Written *string `json:"written"`
}

const (
	lexiconBuffer          = 65536
	lexiconUser            = 2
	lexiconLetterToSound   = 8
	resultSucceeded        = "0x00000000"
	resultAlreadyInLexicon = "0x00040806"
	resultNotImplemented   = "0x80004001"
	resultInvalidArgument  = "0x80070057"
	resultInvalidWord      = "0x80040801"
	resultInvalidSense     = "0x80040802"
	resultNotInLexicon     = "0x80040803"
	resultInvalidPhoneme   = "0x80040805"
	resultInvalidLexicon   = "0x80040809"
)

// lexiconResult is the HRESULT the original returns where the Go lexicon returns added and err.
func lexiconResult(added bool, err error) string {
	switch {
	case errors.Is(err, lexicon.ErrInvalidWord):
		return resultInvalidWord
	case errors.Is(err, lexicon.ErrInvalidPronunciation):
		return resultInvalidPhoneme
	case errors.Is(err, lexicon.ErrNotInLexicon):
		return resultNotInLexicon
	case err != nil:
		return err.Error()
	case !added:
		return resultAlreadyInLexicon
	}
	return resultSucceeded
}

// skippedResult is the result a call the Go API has no form for must have, or "" for a call Go makes. The
// checks come in the original's order; a call they stop changes nothing.
func skippedResult(step *lexiconStep, userLexicon *lexicon.Lexicon, preprocessor *text.Preprocessor) string {
	switch step.Call {
	case "QueryLexicons":
		return resultSucceeded
	case "ChangeSpelling":
		return resultNotImplemented
	case "AddTo", "RemoveFrom":
		if step.Lexicon != lexiconUser {
			return resultInvalidLexicon
		}
	}
	switch step.Call {
	case "Add", "AddTo":
		if step.Wide {
			if _, converted := preprocessor.IPAToEngineNames(utf16.Encode([]rune(step.Pronunciation))); !converted {
				return ""
			}
		}
		if step.CharacterSet != 1 {
			return resultInvalidArgument
		}
	case "Remove", "RemoveFrom":
		if step.Sense != 0 {
			return resultInvalidSense
		}
	case "Get", "GetFrom":
		switch {
		case step.CharacterSet != 1:
			return resultInvalidArgument
		case step.Sense != 0:
			return resultInvalidSense
		case step.Call == "GetFrom" && step.Lexicon&0xFF&(lexiconUser|lexiconLetterToSound) == 0 && lexicon.ValidWord(userLexicon.ANSI(step.Word)):
			return resultInvalidLexicon
		}
	}
	return ""
}

// escapeAccepted reports an ANSI Add whose pronunciation holds an escape and which the original accepted: it
// stores what it reads outside its table, which the Go lexicon refuses (pronunciation.md §7).
func escapeAccepted(step *lexiconStep) bool {
	return !step.Wide && (step.Call == "Add" || step.Call == "AddTo") && strings.ContainsRune(step.Pronunciation, 0x1B) &&
		step.Result == resultSucceeded
}

var errNotReproduced = errors.New("not reproduced")

// ansiString is a step's ANSI text, each character a byte, as a Go string of those bytes.
func ansiString(value string) string {
	encoded := make([]byte, 0, len(value))
	for _, character := range value {
		encoded = append(encoded, byte(character))
	}
	return string(encoded)
}

func runLexiconStep(engine *Engine, userLexicon *lexicon.Lexicon, step *lexiconStep, sink Sink) (string, *string, error) {
	pronunciation := step.Pronunciation
	if !step.Wide {
		pronunciation = ansiString(pronunciation)
	}
	switch step.Call {
	case "Speak":
		return "", nil, engine.Speak(utf16.Encode([]rune(step.Text)), true, text.UnicodeText, sink)
	case "Add", "AddTo":
		var added bool
		var err error
		if step.Wide {
			added, err = userLexicon.AddIPA(step.Word, pronunciation)
		} else {
			added, err = userLexicon.Add(step.Word, pronunciation)
		}
		return lexiconResult(added, err), nil, nil
	case "Remove", "RemoveFrom":
		return lexiconResult(true, userLexicon.Remove(step.Word)), nil, nil
	}
	var returned string
	var err error
	switch letterToSound := step.Call == "Get" || step.Lexicon&lexiconLetterToSound != 0; {
	case letterToSound && step.Wide:
		returned, err = engine.PronounceIPA(step.Word)
	case letterToSound:
		returned, err = engine.Pronounce(step.Word)
	case step.Wide:
		returned, err = userLexicon.PronunciationIPA(step.Word)
	default:
		returned, err = userLexicon.Pronunciation(step.Word)
	}
	if err != nil {
		return lexiconResult(true, err), nil, nil
	}
	return resultSucceeded, &returned, nil
}

func runLexiconCase(t *testing.T, data *Data, golden *lexiconCase) ([]string, error) {
	mode, found := data.Mode(golden.Mode)
	if !found {
		return nil, fmt.Errorf("no mode %q", golden.Mode)
	}
	engine, err := data.NewEngine(mode)
	if err != nil {
		return nil, err
	}
	userLexicon := lexicon.New(data.text)
	engine.userLexicon, engine.reader.UserLexicon = userLexicon, userLexicon
	var file []byte
	if golden.File != nil {
		if file, err = hex.DecodeString(*golden.File); err != nil {
			return nil, err
		}
		userLexicon.Read(bytes.NewReader(file))
	}
	sink := newRecorder()
	engine.AttributeChanged = sink.attributeChanged
	preprocessor := text.New(data.text)
	var differences []string
	for index := range golden.Steps {
		step := &golden.Steps[index]
		describe := fmt.Sprintf("step %d %s wide %t lexicon %#x %q %q", index, step.Call, step.Wide, step.Lexicon, step.Word, step.Pronunciation)
		if escapeAccepted(step) {
			return differences, errNotReproduced
		}
		if skipped := skippedResult(step, userLexicon, preprocessor); skipped != "" {
			if step.Result != skipped {
				differences = append(differences, fmt.Sprintf("%s: result %s, expected %s", describe, skipped, step.Result))
			}
			continue
		}
		result, returned, err := runLexiconStep(engine, userLexicon, step, sink)
		if err != nil {
			return differences, err
		}
		if step.Call == "Speak" || (step.Call == "Get" || step.Call == "GetFrom") && step.Size != lexiconBuffer {
			continue
		}
		if result != step.Result {
			differences = append(differences, fmt.Sprintf("%s: result %s, expected %s", describe, result, step.Result))
		}
		if (returned == nil) != (step.Returned == nil) || returned != nil && *returned != *step.Returned {
			differences = append(differences, fmt.Sprintf("%s: returned %s, expected %s", describe, show(returned), show(step.Returned)))
		}
	}
	if sink.blockLength > 0 {
		sink.endBlock()
	}
	speech := goldenCase{Samples: golden.Samples, Digest: golden.Digest, Blocks: golden.Blocks, Events: golden.Events, AttributeEvents: golden.AttributeEvents}
	differences = append(differences, compareCase(&speech, sink)...)
	expected := file
	if golden.Written != nil {
		if expected, err = hex.DecodeString(*golden.Written); err != nil {
			return differences, err
		}
	} else {
		expected = nil
	}
	written := file
	if userLexicon.Modified() {
		var output bytes.Buffer
		userLexicon.Write(&output)
		written = output.Bytes()
	}
	if (written == nil) != (expected == nil) || !bytes.Equal(written, expected) {
		differences = append(differences, fmt.Sprintf("file written %q, expected %q", written, expected))
	}
	return differences, nil
}

func show(value *string) string {
	if value == nil {
		return "nothing"
	}
	return fmt.Sprintf("%q", *value)
}

func TestLexicon(t *testing.T) {
	data := loadData(t)
	cases := conformance.ReadLines[lexiconCase](t, "golden", "lexicon", "cases.jsonl.gz")
	type outcome struct {
		differences []string
		err         error
	}
	outcomes := make([]outcome, len(cases))
	numbers := make(chan int)
	var group sync.WaitGroup
	for range runtime.GOMAXPROCS(0) {
		group.Go(func() {
			for number := range numbers {
				outcomes[number].differences, outcomes[number].err = runLexiconCase(t, data, &cases[number])
			}
		})
	}
	for number := range cases {
		numbers <- number
	}
	close(numbers)
	group.Wait()
	failures, notReproduced, steps, texts, gets := 0, 0, 0, 0, 0
	for number, outcome := range outcomes {
		golden := &cases[number]
		steps += len(golden.Steps)
		for _, step := range golden.Steps {
			switch step.Call {
			case "Speak":
				texts++
			case "Get", "GetFrom":
				if step.Returned != nil {
					gets++
				}
			}
		}
		if errors.Is(outcome.err, errNotReproduced) && len(outcome.differences) == 0 {
			notReproduced++
			continue
		}
		if outcome.err == nil && len(outcome.differences) == 0 {
			continue
		}
		failures++
		if failures <= 20 {
			t.Errorf("case %d (%s, %s):", golden.Number, golden.Group, golden.Mode)
			if outcome.err != nil {
				t.Errorf("  %v", outcome.err)
			}
			for index, difference := range outcome.differences {
				if index == 8 {
					t.Errorf("  … %d more", len(outcome.differences)-index)
					break
				}
				t.Errorf("  %s", difference)
			}
		}
	}
	t.Logf("%d cases, %d calls (%d texts, %d pronunciations): %d different, %d stopped at an escape the original accepts", len(cases), steps,
		texts, gets, failures, notReproduced)
	if notReproduced > 1 {
		t.Errorf("%d cases stopped at an escape", notReproduced)
	}
}

func TestLexiconPronunciations(t *testing.T) {
	data := loadData(t)
	mode, _ := data.Mode("Mary")
	engine, err := data.NewEngine(mode)
	if err != nil {
		t.Fatal(err)
	}
	userLexicon := data.Lexicon()
	if added, err := userLexicon.AddIPA("tomato", "təmˈɑto"); !added || err != nil {
		t.Fatalf("AddIPA: %t %v", added, err)
	}
	if names, err := userLexicon.Pronunciation("TOMATO"); names != "taxm1aatow" || err != nil {
		t.Errorf("Pronunciation: %q %v", names, err)
	}
	if names, err := engine.Pronounce("tomato"); names != "taxm1aatow" || err != nil {
		t.Errorf("Pronounce: %q %v", names, err)
	}
	if ipa, err := engine.PronounceIPA("butter"); ipa != "bˈʌɾɚ" || err != nil {
		t.Errorf("PronounceIPA before speaking: %q %v", ipa, err)
	}
}
