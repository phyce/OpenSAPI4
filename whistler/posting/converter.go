package posting

import (
	"encoding/binary"

	"github.com/phyce/OpenSAPI4/whistler/datapack"
)

// converterOutput is how many characters the front end's buffer gives the converter for a name, its NUL
// included.
const converterOutput = 0x13

// Converter turns IPA codes into phone names as the SAPI 4 runtime's phoneme converter does with the engine's
// phone set (engine.md §13.4).
type Converter struct {
	names   map[string]string
	longest int
}

func NewConverter(phoneSet []datapack.PhoneSetEntry) *Converter {
	converter := &Converter{names: make(map[string]string, len(phoneSet))}
	for _, entry := range phoneSet {
		key := ipaKey(entry.IPA)
		if _, found := converter.names[key]; !found {
			converter.names[key] = entry.Name
		}
		converter.longest = max(converter.longest, len(entry.IPA))
	}
	return converter
}

func ipaKey(codes []uint16) string {
	key := make([]byte, 2*len(codes))
	for index, code := range codes {
		binary.LittleEndian.PutUint16(key[2*index:], code)
	}
	return string(key)
}

// Convert converts the codes up to the first 0: at each position the longest IPA string of the phone set,
// its name added with no separator. It reports false when a code begins no string. Names that don't fit the
// front end's buffer, and every name after the first of them, are left out of the result.
func (converter *Converter) Convert(codes []uint16) ([]byte, bool) {
	for end, code := range codes {
		if code == 0 {
			codes = codes[:end]
			break
		}
	}
	var name []byte
	valid, fits, written := true, true, 0
	for position := 0; position < len(codes); {
		matched := ""
		length := min(converter.longest, len(codes)-position)
		for ; length > 0; length-- {
			if found, exists := converter.names[ipaKey(codes[position:position+length])]; exists {
				matched = found
				break
			}
		}
		if length == 0 {
			valid = false
			position++
			continue
		}
		if fits = fits && written < converterOutput && len(matched) < converterOutput-written; fits {
			name = append(name, matched...)
		}
		written += len(matched)
		position += length
	}
	return name, valid
}
