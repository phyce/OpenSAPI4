package datapack

import (
	"encoding/json"
	"fmt"
	"unicode/utf16"
)

const (
	mouthValues = 8
	symbolCount = 128
)

// Mouth is a mouth shape, as SAPI's TTSMOUTH describes it.
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

// Visual is what a visual event reports for a phoneme symbol: its IPA character, 0 for none, and its mouth shape,
// nil for none.
type Visual struct {
	IPA   uint16
	Mouth *Mouth
}

// Engine is what the engine reports beside the audio: the visual event of each symbol below 128.
type Engine struct {
	Visuals [symbolCount]Visual
}

type engineDescription struct {
	Visuals [][3]json.RawMessage `json:"visuals"`
}

func (pack *Pack) LoadEngine() (*Engine, error) {
	return load(pack, "engine.json", parseEngine)
}

func parseEngine(data []byte) (*Engine, error) {
	var description engineDescription
	if err := json.Unmarshal(data, &description); err != nil {
		return nil, err
	}
	engine := &Engine{}
	listed := map[byte]bool{}
	for index, entry := range description.Visuals {
		var symbol string
		var ipa int
		var mouth []int
		if json.Unmarshal(entry[0], &symbol) != nil || json.Unmarshal(entry[1], &ipa) != nil || json.Unmarshal(entry[2], &mouth) != nil {
			return nil, fmt.Errorf("visuals[%d] is not a symbol, an IPA character and a mouth shape", index)
		}
		if len(symbol) != 1 || symbol[0] < ' ' || symbol[0] > '~' || listed[symbol[0]] {
			return nil, fmt.Errorf("visuals[%d]: symbol %q is not a new printable character", index, symbol)
		}
		if ipa < 0 || ipa > 0xffff || utf16.IsSurrogate(rune(ipa)) {
			return nil, fmt.Errorf("visuals[%d]: IPA character %d", index, ipa)
		}
		if mouth != nil && len(mouth) != mouthValues {
			return nil, fmt.Errorf("visuals[%d]: the mouth shape has %d values, expected %d", index, len(mouth), mouthValues)
		}
		if ipa == 0 && mouth == nil {
			return nil, fmt.Errorf("visuals[%d] has neither an IPA character nor a mouth shape", index)
		}
		listed[symbol[0]] = true
		visual := &engine.Visuals[symbol[0]]
		visual.IPA = uint16(ipa)
		if mouth == nil {
			continue
		}
		var shape [mouthValues]byte
		for position, value := range mouth {
			if value < 0 || value > 0xff {
				return nil, fmt.Errorf("visuals[%d]: mouth value %d", index, value)
			}
			shape[position] = byte(value)
		}
		visual.Mouth = &Mouth{shape[0], shape[1], shape[2], shape[3], shape[4], shape[5], shape[6], shape[7]}
	}
	return engine, nil
}
