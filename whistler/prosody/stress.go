package prosody

import "github.com/phyce/OpenSAPI4/whistler/datapack"

// AssignStress rewrites tier 3 with stress levels between module A groups 10 and 16 (prosody.md §4).
// The tiers are indexed by cell.
func AssignStress(tables *datapack.Prosody, tier1 []byte, tier3 []byte, start int, end int, multipleWords bool) {
	pending := byte(' ')
	for cell := start; cell <= end; cell++ {
		code := tier1[cell]
		if tables.PhoneClasses[code] > 0x60 {
			switch code {
			case ' ', '-':
				pending = ' '
			case '"':
				pending = 'Z'
			case '\'':
				pending = 'P'
			case '`':
				pending = 'S'
			}
		}
		switch {
		case tables.PhoneFeatures[tier3[cell]] < 0x80 || !multipleWords:
			tier3[cell] = pending
		case pending == 'P' && polysyllabic(tier1, start, end, cell):
			tier3[cell] = 'S'
		case pending == 'Z':
			tier3[cell] = 'Z'
		default:
			tier3[cell] = 'F'
		}
		if tables.PhoneClasses[code] == 'V' {
			pending = ' '
		}
	}
}
