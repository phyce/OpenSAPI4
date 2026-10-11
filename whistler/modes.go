package whistler

import (
	"errors"
	"strings"

	"github.com/phyce/OpenSAPI4/whistler/datapack"
)

const (
	// EngineID is the original engine's CLSID.
	EngineID    = "{E0725551-286F-11D0-8E73-00A0C9083363}"
	ProductName = "Microsoft Speech Synthesis Engine"
	// NullModeID is GUID_NULL, the mode ID of the hidden modes.
	NullModeID = "{00000000-0000-0000-0000-000000000000}"
)

// ErrInvalidMode is TTSERR_INVALIDMODE: Select with a mode ID that isn't in the list.
var ErrInvalidMode = errors.New("whistler: invalid mode")

// Gender is GENDER_NEUTRAL, GENDER_FEMALE or GENDER_MALE.
type Gender uint16

const (
	GenderNeutral Gender = iota
	GenderFemale
	GenderMale
)

// Features are the TTSFEATURE flags.
type Features uint32

const (
	FeatureAnyWord Features = 1 << iota
	FeatureVolume
	FeatureSpeed
	FeaturePitch
	FeatureTagged
	FeatureIPAUnicode
	FeatureVisual
	FeatureWordPosition
	FeaturePCOptimized
	FeaturePhoneOptimized
	FeatureFixedAudio
	FeatureSingleInstance
	FeatureThreadSafe
	FeatureIPATextData
	FeaturePreferred
	FeatureTransplanted
	FeatureSAPI4
)

// Interfaces are the TTSI flags.
type Interfaces uint32

const (
	InterfaceLexPronounce Interfaces = 1 << iota
	InterfaceTTSAttributes
	InterfaceTTSCentral
	InterfaceTTSDialogs
	InterfaceAttributes
	InterfaceLexPronounce2
)

const (
	engineFeatures = FeatureAnyWord | FeatureVolume | FeatureSpeed | FeaturePitch | FeatureTagged | FeatureIPAUnicode | FeatureVisual |
		FeatureWordPosition | FeatureIPATextData | FeatureTransplanted | FeatureSAPI4
	engineInterfaces = InterfaceLexPronounce | InterfaceTTSAttributes | InterfaceTTSCentral | InterfaceTTSDialogs | InterfaceAttributes |
		InterfaceLexPronounce2
)

type Language struct {
	ID      uint16
	Dialect string
}

// ModeInfo is what the original reports of a mode, its TTSMODEINFO (interface.md §2).
type ModeInfo struct {
	EngineID       string
	Manufacturer   string
	ProductName    string
	ModeID         string
	ModeName       string
	Language       Language
	Speaker        string
	Style          string
	Gender         Gender
	Age            uint16
	Features       Features
	Interfaces     Interfaces
	EngineFeatures uint32
}

func modeInfo(mode *datapack.Mode) ModeInfo {
	info := ModeInfo{
		EngineID:     EngineID,
		Manufacturer: mode.Manufacturer,
		ProductName:  ProductName,
		ModeID:       modeID(mode),
		ModeName:     mode.Name,
		Language:     Language{ID: mode.Language, Dialect: mode.Dialect},
		Speaker:      mode.Speaker,
		Style:        mode.Style,
		Gender:       GenderMale,
		Age:          mode.Age,
		Features:     Features(mode.Features) | engineFeatures,
		Interfaces:   engineInterfaces,
	}
	if mode.Gender == "female" {
		info.Gender = GenderFemale
	}
	if mode.Preferred {
		info.Features |= FeaturePreferred
	}
	return info
}

func modeID(mode *datapack.Mode) string {
	if mode.Hidden() {
		return NullModeID
	}
	return mode.ModeID
}

// Select makes an engine for the first mode in the list with this mode ID, written
// {XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX} in either case, in the voice's own format. As in the original,
// NullModeID selects the first hidden mode (interface.md §3).
func (data *Data) Select(identifier string) (*Engine, error) {
	return data.SelectAccepting(identifier, nil)
}

// SelectAccepting is Select for an audio destination that chooses the format: the engine offers its formats
// in order and takes the first accept takes. When it takes none, Select fails with the error accept gave for
// the last one (interface.md §4).
func (data *Data) SelectAccepting(identifier string, accept AcceptFormat) (*Engine, error) {
	for index := range data.Modes.Modes {
		if mode := &data.Modes.Modes[index]; strings.EqualFold(modeID(mode), identifier) {
			return data.newEngine(mode, accept)
		}
	}
	return nil, ErrInvalidMode
}

// ModeInfo is what ModeGet reports: the information of the engine's mode.
func (engine *Engine) ModeInfo() ModeInfo {
	return modeInfo(engine.mode)
}

// Enumerator goes through the mode list as ITTSEnum does (interface.md §3). Hidden modes take a place in the
// list, which its position counts, but are never returned.
type Enumerator struct {
	modes    []datapack.Mode
	position uint32
}

// Enumerate gives an enumerator at the start of the list.
func (data *Data) Enumerate() *Enumerator {
	return &Enumerator{modes: data.Modes.Modes}
}

// Next returns up to count modes from the position on, moves past them and the hidden modes among them, and
// reports whether it returned count modes.
func (enumerator *Enumerator) Next(count uint32) ([]ModeInfo, bool) {
	total := uint32(len(enumerator.modes))
	if enumerator.position >= total {
		return nil, false
	}
	span := min(count, total-enumerator.position)
	var modes []ModeInfo
	for index := uint32(0); index < span; index++ {
		mode := &enumerator.modes[enumerator.position+index]
		if !mode.Hidden() {
			modes = append(modes, modeInfo(mode))
		} else if span < total-enumerator.position {
			span++
		}
	}
	enumerator.position += span
	return modes, uint32(len(modes)) == count
}

// Skip moves past count modes that aren't hidden. Where the list ends sooner, it moves to the end only if the
// position plus count (32 bits, wrapping) is past the end, and otherwise stays where it is.
func (enumerator *Enumerator) Skip(count uint32) bool {
	total := uint32(len(enumerator.modes))
	if count+enumerator.position > total {
		enumerator.position = total
		return false
	}
	index, passed := uint32(0), uint32(0)
	for passed < count {
		at := enumerator.position + index
		if at > total {
			return false
		}
		if at < total && !enumerator.modes[at].Hidden() {
			passed++
		}
		index++
	}
	enumerator.position += index
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
