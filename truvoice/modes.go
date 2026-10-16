package truvoice

import (
	"errors"
	"slices"
	"strings"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
)

var (
	// ErrInvalidMode is TTSERR_INVALIDMODE: Select with a mode ID that isn't one of the ten.
	ErrInvalidMode = errors.New("truvoice: invalid mode")
	// ErrNoModeFound is Find's E_FAIL: every mode scores 0.
	ErrNoModeFound = errors.New("truvoice: no mode found")
)

type Language struct {
	ID      uint16
	Dialect string
}

// ModeInfo is what the engine reports of a mode, its TTSMODEINFO (engine.md §3.1).
type ModeInfo struct {
	EngineID       string
	Manufacturer   string
	ProductName    string
	ModeID         string
	ModeName       string
	Language       Language
	Speaker        string
	Style          string
	Gender         uint16
	Age            uint16
	Features       uint32
	Interfaces     uint32
	EngineFeatures uint32
}

func modeInfo(mode *datapack.Mode) ModeInfo {
	return ModeInfo{
		EngineID:       mode.Engine,
		Manufacturer:   mode.Manufacturer,
		ProductName:    mode.Product,
		ModeID:         mode.Mode,
		ModeName:       mode.Name,
		Language:       Language{ID: mode.Language, Dialect: mode.Dialect},
		Speaker:        mode.Speaker,
		Style:          mode.Style,
		Gender:         mode.Gender,
		Age:            mode.Age,
		Features:       mode.Features,
		Interfaces:     mode.Interfaces,
		EngineFeatures: mode.EngineFeatures,
	}
}

// Select makes an engine for the mode with this mode ID, written {XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX} in either
// case, in the engine's first format (engine.md §3.3).
func (data *Data) Select(identifier string) (*Engine, error) {
	return data.SelectAccepting(identifier, nil)
}

// SelectAccepting is Select for an audio destination that chooses the format, as NewEngineAccepting.
func (data *Data) SelectAccepting(identifier string, accept AcceptFormat) (*Engine, error) {
	for index := range data.Modes.Modes {
		if strings.EqualFold(data.Modes.Modes[index].Mode, identifier) {
			return data.NewEngineAccepting(index, accept)
		}
	}
	return nil, ErrInvalidMode
}

// ModeInfo is what ModeGet reports: the information of the engine's voice, which Select, \Vce\ in a text and
// Inject set.
func (engine *Engine) ModeInfo() ModeInfo {
	return modeInfo(&engine.data.Modes.Modes[engine.state.Voice])
}

// Enumerator goes through the ten modes as ITTSEnum does (engine.md §3.2).
type Enumerator struct {
	modes    []datapack.Mode
	position uint32
}

// Enumerate gives an enumerator at the start of the list.
func (data *Data) Enumerate() *Enumerator {
	return &Enumerator{modes: data.Modes.Modes[:]}
}

// Next returns up to count modes from the position on, moves past them, and reports whether it returned count
// modes.
func (enumerator *Enumerator) Next(count uint32) ([]ModeInfo, bool) {
	total := uint32(len(enumerator.modes))
	if enumerator.position >= total {
		return nil, false
	}
	span := min(count, total-enumerator.position)
	modes := make([]ModeInfo, span)
	for index := range modes {
		modes[index] = modeInfo(&enumerator.modes[enumerator.position+uint32(index)])
	}
	enumerator.position += span
	return modes, span == count
}

// Skip moves past count modes. When the position plus count (32 bits, wrapping) is past the end, it moves to the end
// and reports false.
func (enumerator *Enumerator) Skip(count uint32) bool {
	total := uint32(len(enumerator.modes))
	if enumerator.position+count > total {
		enumerator.position = total
		return false
	}
	enumerator.position += count
	return true
}

func (enumerator *Enumerator) Reset() {
	enumerator.position = 0
}

// Clone gives an enumerator at the same position.
func (enumerator *Enumerator) Clone() *Enumerator {
	clone := *enumerator
	return &clone
}

// Rank is a TTSMODEINFORANK: how much each field of the mode wanted counts in Find. Find reads only EngineID,
// Manufacturer, ProductName, ModeName, Speaker, Gender and Age.
type Rank struct {
	EngineID       uint32
	Manufacturer   uint32
	ProductName    uint32
	ModeID         uint32
	ModeName       uint32
	Language       uint32
	Dialect        uint32
	Speaker        uint32
	Style          uint32
	Gender         uint32
	Age            uint32
	Features       uint32
	Interfaces     uint32
	EngineFeatures uint32
}

// DefaultRank is the rank Find takes when it is given none.
var DefaultRank = Rank{ModeName: 4, Speaker: 3, Gender: 2, Age: 1}

const (
	fullScore  = 10
	genderSpan = 3
	ageSpan    = 6
)

// The ages Find knows, from youngest to oldest.
var ages = []uint16{1, 3, 6, 14, 30, 70}

// Find returns the mode that matches wanted best, by rank or by DefaultRank when rank is nil, and leaves the
// enumerator at the end of the list (engine.md §3.4). A mode scores 10 times the rank of each of the engine ID,
// manufacturer, product name, mode name and speaker it has (the names compared up to a NUL, ASCII letters' case
// aside), and the ranks of the gender and the age times how close the mode's are, out of 10, when wanted's are
// ones Find knows; scores are 32 bits and wrap. The first mode with the highest score wins, and when every mode
// scores 0, Find gives ErrNoModeFound.
func (enumerator *Enumerator) Find(wanted ModeInfo, rank *Rank) (ModeInfo, error) {
	if rank == nil {
		rank = &DefaultRank
	}
	enumerator.position = uint32(len(enumerator.modes))
	best, bestScore := -1, uint32(0)
	for index := range enumerator.modes {
		mode := modeInfo(&enumerator.modes[index])
		score := uint32(0)
		if rank.EngineID != 0 && strings.EqualFold(wanted.EngineID, mode.EngineID) {
			score = rank.EngineID * fullScore
		}
		for _, field := range []struct {
			rank           uint32
			wanted, actual string
		}{
			{rank.Manufacturer, wanted.Manufacturer, mode.Manufacturer},
			{rank.ProductName, wanted.ProductName, mode.ProductName},
			{rank.ModeName, wanted.ModeName, mode.ModeName},
			{rank.Speaker, wanted.Speaker, mode.Speaker},
		} {
			if field.rank != 0 && equalFoldASCII(field.wanted, field.actual) {
				score += field.rank * fullScore
			}
		}
		if wanted.Gender <= datapack.GenderMale {
			score += genderScore(wanted.Gender, mode.Gender) * rank.Gender
		}
		if slices.Contains(ages, wanted.Age) {
			score += ageScore(wanted.Age, mode.Age) * rank.Age
		}
		if score > bestScore {
			best, bestScore = index, score
		}
	}
	if best < 0 {
		return ModeInfo{}, ErrNoModeFound
	}
	return modeInfo(&enumerator.modes[best]), nil
}

// equalFoldASCII compares two names as _wcsicmp does in the C locale: up to a NUL, with only ASCII letters' case
// aside.
func equalFoldASCII(first, second string) bool {
	first, _, _ = strings.Cut(first, "\x00")
	second, _, _ = strings.Cut(second, "\x00")
	if len(first) != len(second) {
		return false
	}
	for index := range len(first) {
		if lowerASCII(first[index]) != lowerASCII(second[index]) {
			return false
		}
	}
	return true
}

func lowerASCII(character byte) byte {
	if character >= 'A' && character <= 'Z' {
		return character + 'a' - 'A'
	}
	return character
}

// genderScore is how close a mode's gender, female or male, is to the gender wanted: 10 for the same, 6 when neutral
// is wanted, 3 for the other.
func genderScore(wanted, actual uint16) uint32 {
	closeness := uint32(1)
	switch wanted {
	case actual:
		closeness = 3
	case datapack.GenderNeutral:
		closeness = 2
	}
	return closeness * fullScore / genderSpan
}

// ageScore is how close a mode's age is to the age wanted, out of 10, by their places among the ages Find knows.
func ageScore(wanted, actual uint16) uint32 {
	distance := slices.Index(ages, wanted) - slices.Index(ages, actual)
	if distance < 0 {
		distance = -distance
	}
	return uint32((ageSpan - distance) * fullScore / ageSpan)
}
