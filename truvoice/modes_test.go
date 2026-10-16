package truvoice

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/phyce/OpenSAPI4/truvoice/internal/conformance"
)

type goldenModeInfo struct {
	Engine         string `json:"engine"`
	Manufacturer   string `json:"manufacturer"`
	Product        string `json:"product"`
	Mode           string `json:"mode"`
	Name           string `json:"name"`
	Language       uint16 `json:"language"`
	Dialect        string `json:"dialect"`
	Speaker        string `json:"speaker"`
	Style          string `json:"style"`
	Gender         uint16 `json:"gender"`
	Age            uint16 `json:"age"`
	Features       uint32 `json:"features"`
	Interfaces     uint32 `json:"interfaces"`
	EngineFeatures uint32 `json:"engineFeatures"`
}

func goldenInfo(info ModeInfo) goldenModeInfo {
	return goldenModeInfo{
		Engine:         strings.ToUpper(info.EngineID),
		Manufacturer:   info.Manufacturer,
		Product:        info.ProductName,
		Mode:           strings.ToUpper(info.ModeID),
		Name:           info.ModeName,
		Language:       info.Language.ID,
		Dialect:        info.Language.Dialect,
		Speaker:        info.Speaker,
		Style:          info.Style,
		Gender:         info.Gender,
		Age:            info.Age,
		Features:       info.Features,
		Interfaces:     info.Interfaces,
		EngineFeatures: info.EngineFeatures,
	}
}

// wantedInfo is the mode a Find step asks for; "" is GUID_NULL.
func wantedInfo(golden goldenModeInfo) ModeInfo {
	nullIdentifier := func(identifier string) string {
		if identifier == "" {
			return "{00000000-0000-0000-0000-000000000000}"
		}
		return identifier
	}
	return ModeInfo{
		EngineID:       nullIdentifier(golden.Engine),
		Manufacturer:   golden.Manufacturer,
		ProductName:    golden.Product,
		ModeID:         nullIdentifier(golden.Mode),
		ModeName:       golden.Name,
		Language:       Language{ID: golden.Language, Dialect: golden.Dialect},
		Speaker:        golden.Speaker,
		Style:          golden.Style,
		Gender:         golden.Gender,
		Age:            golden.Age,
		Features:       golden.Features,
		Interfaces:     golden.Interfaces,
		EngineFeatures: golden.EngineFeatures,
	}
}

type enumeratorStep struct {
	Call        string          `json:"call"`
	Enumerator  int             `json:"enumerator"`
	Count       uint32          `json:"count"`
	Mode        string          `json:"mode"`
	Wanted      *goldenModeInfo `json:"wanted"`
	Rank        []uint32        `json:"rank"`
	Result      string          `json:"result"`
	Fetched     *uint32         `json:"fetched"`
	Modes       []string        `json:"modes"`
	Clone       *int            `json:"clone"`
	Information *goldenModeInfo `json:"information"`
}

type enumeratorCase struct {
	Number int              `json:"number"`
	Group  string           `json:"group"`
	Steps  []enumeratorStep `json:"steps"`
}

func loadData(t *testing.T) *Data {
	data, err := Load(conformance.Pack(t))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestModeList(t *testing.T) {
	data := loadData(t)
	encoded, err := os.ReadFile(conformance.Path(t, "golden", "truvoice", "modes.json"))
	if err != nil {
		t.Fatal(err)
	}
	var expected []goldenModeInfo
	if err := json.Unmarshal(encoded, &expected); err != nil {
		t.Fatal(err)
	}
	modes, complete := data.Enumerate().Next(^uint32(0))
	if complete || len(modes) != len(expected) {
		t.Fatalf("%d modes (complete %t), expected %d", len(modes), complete, len(expected))
	}
	for index, info := range modes {
		if got := goldenInfo(info); got != expected[index] {
			t.Errorf("mode %d:\n  %+v\nexpected\n  %+v", index, got, expected[index])
		}
	}
}

func resultFlag(ok bool) string {
	if ok {
		return "0x00000000"
	}
	return "0x00000001"
}

func rankOf(numbers []uint32) *Rank {
	if numbers == nil {
		return nil
	}
	return &Rank{numbers[0], numbers[1], numbers[2], numbers[3], numbers[4], numbers[5], numbers[6], numbers[7], numbers[8],
		numbers[9], numbers[10], numbers[11], numbers[12], numbers[13]}
}

func runEnumeratorCase(data *Data, golden *enumeratorCase) []string {
	enumerators := []*Enumerator{data.Enumerate()}
	var differences []string
	for index, step := range golden.Steps {
		describe := fmt.Sprintf("step %d %s on %d count %#x %s", index, step.Call, step.Enumerator, step.Count, step.Mode)
		if step.Enumerator >= len(enumerators) {
			return append(differences, describe+": no such enumerator")
		}
		enumerator := enumerators[step.Enumerator]
		result := "0x00000000"
		var information *ModeInfo
		switch step.Call {
		case "Next":
			modes, complete := enumerator.Next(step.Count)
			result = resultFlag(complete)
			identifiers := []string{}
			for _, info := range modes {
				identifiers = append(identifiers, strings.ToUpper(info.ModeID))
			}
			if step.Fetched == nil || int(*step.Fetched) != len(modes) || !slices.Equal(identifiers, append([]string{}, step.Modes...)) {
				differences = append(differences, fmt.Sprintf("%s: modes %v, expected %v", describe, identifiers, step.Modes))
			}
		case "Skip":
			result = resultFlag(enumerator.Skip(step.Count))
		case "Reset":
			enumerator.Reset()
		case "Clone":
			number := len(enumerators)
			enumerators = append(enumerators, enumerator.Clone())
			if step.Clone == nil || *step.Clone != number {
				differences = append(differences, fmt.Sprintf("%s: clone %d, expected %v", describe, number, step.Clone))
			}
		case "Select":
			engine, err := data.Select(step.Mode)
			switch {
			case errors.Is(err, ErrInvalidMode):
				result = "0x80040206"
			case err != nil:
				return append(differences, fmt.Sprintf("%s: %v", describe, err))
			default:
				found := engine.ModeInfo()
				information = &found
			}
		case "Find":
			found, err := enumerator.Find(wantedInfo(*step.Wanted), rankOf(step.Rank))
			switch {
			case errors.Is(err, ErrNoModeFound):
				result = "0x80004005"
			case err != nil:
				return append(differences, fmt.Sprintf("%s: %v", describe, err))
			default:
				information = &found
			}
		default:
			return append(differences, describe+": unknown call")
		}
		if result != step.Result {
			differences = append(differences, fmt.Sprintf("%s: result %s, expected %s", describe, result, step.Result))
		}
		switch {
		case information == nil && step.Information != nil:
			differences = append(differences, fmt.Sprintf("%s: no information, expected %+v", describe, *step.Information))
		case information != nil && step.Information == nil:
			differences = append(differences, fmt.Sprintf("%s: information %+v, expected none", describe, *information))
		case information != nil && goldenInfo(*information) != *step.Information:
			differences = append(differences, fmt.Sprintf("%s: information %+v, expected %+v", describe, goldenInfo(*information),
				*step.Information))
		}
	}
	return differences
}

// TestEnumerator replays the original's calls of ITTSEnum and ITTSFind on fresh enumerators: Next, Skip, Reset,
// Clone, Select with the mode information ModeGet gives, and Find.
func TestEnumerator(t *testing.T) {
	data := loadData(t)
	cases := conformance.ReadLines[enumeratorCase](t, "enumerator.jsonl.gz")
	failures, steps := 0, 0
	for number := range cases {
		golden := &cases[number]
		steps += len(golden.Steps)
		differences := runEnumeratorCase(data, golden)
		if len(differences) == 0 {
			continue
		}
		failures++
		if failures <= 20 {
			t.Errorf("case %d (%s):", golden.Number, golden.Group)
			for _, difference := range differences {
				t.Errorf("  %s", difference)
			}
		}
	}
	t.Logf("%d cases, %d calls: %d different", len(cases), steps, failures)
}
