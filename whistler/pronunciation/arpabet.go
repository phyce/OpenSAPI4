package pronunciation

import (
	"strings"

	"github.com/phyce/OpenSAPI4/whistler/datapack"
)

// FromARPAbet converts a lexicon pronunciation, ARPAbet names with stress digits separated by spaces, to
// tier 1 phones (pronunciation.md §6.2). It reports false when a name has no phone; the word then goes to the
// rules.
func FromARPAbet(tables *datapack.Prosody, pronunciation string) ([]byte, bool) {
	phones := []byte{}
	for _, name := range strings.Split(pronunciation, " ") {
		if name == "" {
			continue
		}
		stress := byte('0')
		if last := name[len(name)-1]; last >= '0' && last <= '9' {
			stress = last
			name = name[:len(name)-1]
			switch stress {
			case '1':
				phones = append(phones, '\'')
			case '2':
				phones = append(phones, '`')
			}
		}
		var code byte
		if stress == '0' && name == "AH" {
			code = '$'
		} else {
			for _, candidate := range tables.PhoneCodes {
				if tables.PhoneNames[candidate].Name == name {
					code = candidate
					break
				}
			}
		}
		switch code {
		case 0:
			return nil, false
		case 'X':
			phones = append(phones, 'd', 'Z')
		case 'Q':
			phones = append(phones, 't', 'S')
		case 'e':
			phones = append(phones, 'e', 'r')
		default:
			phones = append(phones, code)
		}
	}
	return phones, true
}
