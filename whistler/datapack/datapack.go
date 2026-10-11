package datapack

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

const (
	Format        = "opensapi4-microsoft"
	FormatVersion = 1
)

type Manifest struct {
	Format        string            `json:"format"`
	FormatVersion int               `json:"formatVersion"`
	Voices        []string          `json:"voices"`
	Files         map[string]string `json:"files"`
}

type Pack struct {
	rootPath string
	Manifest Manifest

	lock   sync.Mutex
	loaded map[string]any
}

func Open(rootPath string) (*Pack, error) {
	data, err := os.ReadFile(filepath.Join(rootPath, "manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("read data pack manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse data pack manifest: %w", err)
	}
	if manifest.Format != Format {
		return nil, fmt.Errorf("data pack format is %q, expected %q", manifest.Format, Format)
	}
	if manifest.FormatVersion != FormatVersion {
		return nil, fmt.Errorf("data pack format version is %d, expected %d", manifest.FormatVersion, FormatVersion)
	}
	return &Pack{rootPath: rootPath, Manifest: manifest, loaded: map[string]any{}}, nil
}

func (pack *Pack) readFile(name string) ([]byte, error) {
	if _, listed := pack.Manifest.Files[name]; !listed {
		return nil, fmt.Errorf("data pack file %s is not in the manifest", name)
	}
	if !filepath.IsLocal(filepath.FromSlash(name)) || path.Clean(name) != name {
		return nil, fmt.Errorf("data pack file name %s is not a plain relative path", name)
	}
	data, err := os.ReadFile(filepath.Join(pack.rootPath, filepath.FromSlash(name)))
	if err != nil {
		return nil, fmt.Errorf("read data pack file: %w", err)
	}
	return data, nil
}

func load[T any](pack *Pack, name string, parse func([]byte) (*T, error)) (*T, error) {
	pack.lock.Lock()
	defer pack.lock.Unlock()
	if value, found := pack.loaded[name]; found {
		typed, matches := value.(*T)
		if !matches {
			return nil, fmt.Errorf("data pack file %s is used as two different kinds of data", name)
		}
		return typed, nil
	}
	data, err := pack.readFile(name)
	if err != nil {
		return nil, err
	}
	value, err := parse(data)
	if err != nil {
		return nil, fmt.Errorf("data pack file %s: %w", name, err)
	}
	pack.loaded[name] = value
	return value, nil
}

type AlternativeUnit struct {
	Key               []int    `json:"key"`
	Flags             int      `json:"flags"`
	Codes             []uint16 `json:"codes"`
	ReplacementPhones []int    `json:"replacementPhones"`
}

func (alternative *AlternativeUnit) Span() (first int, length int) {
	first = 1
	if alternative.Flags&1 != 0 {
		first = 0
	}
	end := len(alternative.Key) - 1
	if alternative.Flags&2 != 0 {
		end = len(alternative.Key)
	}
	return first, end - first
}

type Mouth struct {
	Height            byte
	Width             byte
	Upturn            byte
	JawOpen           byte
	UpperTeethVisible byte
	LowerTeethVisible byte
	TonguePosition    byte
	LipTension        byte
}

type Visual struct {
	IPA    uint16
	Engine byte
	Mouth  Mouth
}

type visualDescription struct {
	IPA    int   `json:"ipa"`
	Engine int   `json:"engine"`
	Mouth  []int `json:"mouth"`
}

type voiceDescription struct {
	Phones           []string            `json:"phones"`
	SilencePhone     int                 `json:"silencePhone"`
	SampleRate       int                 `json:"sampleRate"`
	Trees            string              `json:"trees"`
	Senones          string              `json:"senones"`
	Inventory        string              `json:"inventory"`
	Visual           []visualDescription `json:"visual"`
	AlternativeUnits []AlternativeUnit   `json:"alternativeUnits"`
}

type Voice struct {
	Name             string
	Phones           []string
	SilencePhone     int
	SampleRate       int
	Trees            *Trees
	Senones          *Senones
	Inventory        *Inventory
	Visual           []Visual
	AlternativeUnits []AlternativeUnit

	contextPhones []int
}

func (pack *Pack) LoadVoice(name string) (*Voice, error) {
	if !slices.Contains(pack.Manifest.Voices, name) {
		return nil, fmt.Errorf("data pack has no voice %q", name)
	}
	data, err := pack.readFile("voices/" + name + "/voice.json")
	if err != nil {
		return nil, err
	}
	var description voiceDescription
	if err := json.Unmarshal(data, &description); err != nil {
		return nil, fmt.Errorf("parse voice %s: %w", name, err)
	}
	trees, err := load(pack, description.Trees, parseTrees)
	if err != nil {
		return nil, err
	}
	senones, err := load(pack, description.Senones, parseSenones)
	if err != nil {
		return nil, err
	}
	inventory, err := load(pack, description.Inventory, parseInventory)
	if err != nil {
		return nil, err
	}
	visual, err := parseVisual(description.Visual)
	if err != nil {
		return nil, fmt.Errorf("voice %s: %w", name, err)
	}
	voice := &Voice{
		Name:             name,
		Phones:           description.Phones,
		SilencePhone:     description.SilencePhone,
		SampleRate:       description.SampleRate,
		Trees:            trees,
		Senones:          senones,
		Inventory:        inventory,
		Visual:           visual,
		AlternativeUnits: description.AlternativeUnits,
	}
	if err := voice.validate(); err != nil {
		return nil, fmt.Errorf("voice %s: %w", name, err)
	}
	voice.contextPhones = make([]int, len(voice.Phones))
	for phone, phoneName := range voice.Phones {
		voice.contextPhones[phone] = phone
		if strings.HasPrefix(phoneName, "+") || len(phoneName) >= 3 && strings.EqualFold(phoneName[:3], "SIL") {
			voice.contextPhones[phone] = voice.SilencePhone
		}
	}
	return voice, nil
}

func (voice *Voice) HasTree(phone int) bool {
	return voice.Trees.Roots[phone] != -1
}

func (voice *Voice) Senone(phone int, left int, right int, position int) int {
	return voice.Trees.Lookup(phone, voice.contextPhones[left], voice.contextPhones[right], position)
}

func parseVisual(descriptions []visualDescription) ([]Visual, error) {
	visual := make([]Visual, len(descriptions))
	for phone, description := range descriptions {
		if description.IPA < 0 || description.IPA > 0xFFFF || description.IPA >= 0xD800 && description.IPA < 0xE000 {
			return nil, fmt.Errorf("phone %d Visual IPA character %d", phone, description.IPA)
		}
		if description.Engine < 0x20 || description.Engine > 0x7E {
			return nil, fmt.Errorf("phone %d Visual engine phoneme %d", phone, description.Engine)
		}
		if len(description.Mouth) != 8 {
			return nil, fmt.Errorf("phone %d Visual mouth has %d values", phone, len(description.Mouth))
		}
		mouth := make([]byte, 8)
		for index, value := range description.Mouth {
			if value < 0 || value > 0xFF {
				return nil, fmt.Errorf("phone %d Visual mouth value %d", phone, value)
			}
			mouth[index] = byte(value)
		}
		visual[phone] = Visual{
			IPA:    uint16(description.IPA),
			Engine: byte(description.Engine),
			Mouth:  Mouth{mouth[0], mouth[1], mouth[2], mouth[3], mouth[4], mouth[5], mouth[6], mouth[7]},
		}
	}
	return visual, nil
}

func (voice *Voice) validate() error {
	phoneCount := len(voice.Phones)
	if phoneCount == 0 || phoneCount+4 > 64 {
		return fmt.Errorf("%d phones", phoneCount)
	}
	if voice.SilencePhone < 0 || voice.SilencePhone >= phoneCount || voice.Phones[voice.SilencePhone] != "SIL" {
		return fmt.Errorf("silence phone %d is not SIL", voice.SilencePhone)
	}
	if voice.Trees.PhoneCount() != phoneCount {
		return fmt.Errorf("trees are for %d phones, the voice has %d", voice.Trees.PhoneCount(), phoneCount)
	}
	if len(voice.Senones.MeanDurations) != voice.Trees.SenoneCount {
		return fmt.Errorf("%d senones, the trees have %d", len(voice.Senones.MeanDurations), voice.Trees.SenoneCount)
	}
	if voice.Inventory.UnitCount() <= voice.Trees.SenoneCount {
		return fmt.Errorf("inventory has %d units for %d senones", voice.Inventory.UnitCount(), voice.Trees.SenoneCount)
	}
	if voice.Inventory.SampleRate != voice.SampleRate {
		return fmt.Errorf("inventory rate %d, voice rate %d", voice.Inventory.SampleRate, voice.SampleRate)
	}
	if len(voice.Visual) != phoneCount {
		return fmt.Errorf("%d Visual entries for %d phones", len(voice.Visual), phoneCount)
	}
	for _, alternative := range voice.AlternativeUnits {
		first, length := alternative.Span()
		if alternative.Flags&^3 != 0 || length < 1 || len(alternative.Codes) != length {
			return fmt.Errorf("alternative unit with key %v, flags %d and %d codes", alternative.Key, alternative.Flags, len(alternative.Codes))
		}
		for _, phone := range slices.Concat(alternative.Key, alternative.ReplacementPhones) {
			if phone < 0 || phone >= phoneCount {
				return fmt.Errorf("alternative unit phone %d", phone)
			}
		}
		replacements := 0
		for position, code := range alternative.Codes {
			if code&0xE000 != 0 && code&0xE000 != 0xC000 {
				return fmt.Errorf("alternative unit code 0x%04x", code)
			}
			unit := int(code & 0x1FFF)
			silence := unit == 0 && alternative.Key[first+position] == voice.SilencePhone
			if !silence && (unit <= voice.Trees.SenoneCount || unit >= voice.Inventory.UnitCount()) {
				return fmt.Errorf("alternative unit code 0x%04x plays neither silence nor an alternative unit", code)
			}
			if code&0xC000 == 0xC000 {
				replacements++
			}
		}
		if alternative.Codes[length-1]&0xC000 == 0xC000 {
			return fmt.Errorf("alternative unit replaces the last phone of its span")
		}
		if replacements != len(alternative.ReplacementPhones) {
			return fmt.Errorf("alternative unit has %d replace codes and %d replacement phones", replacements, len(alternative.ReplacementPhones))
		}
		for _, phone := range alternative.ReplacementPhones {
			if phone == voice.SilencePhone || voice.Trees.Roots[phone] == -1 {
				return fmt.Errorf("alternative unit replacement phone %d is SIL or has no tree", phone)
			}
		}
	}
	return nil
}
