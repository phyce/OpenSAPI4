package text

import "unicode/utf16"

// PhoneticItem is what the engine phoneme parse reads: the code of a phonetic symbol (1 to 52), or an
// escape sequence with the space that ends it.
type PhoneticItem struct {
	Code   int
	Escape []byte
}

// findEnginePhoneme looks a 1- or 2-byte name up among the engine phoneme names, ignoring ASCII case;
// the first match wins (text.md §2.3).
func (preprocessor *Preprocessor) findEnginePhoneme(key []byte) int {
	lowered := lowerASCII(key)
	for index, phoneme := range preprocessor.tables.Phonemes[:len(preprocessor.tables.Phonemes)-1] {
		name := []byte(phoneme.EngineName)
		if len(lowered) > 0 && name[0] == lowered[0] && string(lowerASCII(name)) == string(lowered) {
			return index + 1
		}
	}
	return 0
}

// FindEnginePhoneme is the code of an engine phoneme name, 0 if there is none.
func (preprocessor *Preprocessor) FindEnginePhoneme(name string) int {
	return preprocessor.findEnginePhoneme([]byte(name))
}

// ParseEnginePhonemes reads engine phoneme names as the original's parser does (text.md §2.3), up to the
// first NUL: names of one or two letters, the longer where the aspirates after it pair up, escapes copied
// whole, and aa + ih, aa + uh and ao + ih joined into ay, aw and oy. It reports false where the original
// reports an error, keeping what it read before: a name it doesn't know, a stress mark followed by a
// boundary before any vowel, a stress mark left at the end, or an escape that runs to the end.
func (preprocessor *Preprocessor) ParseEnginePhonemes(names []byte) ([]PhoneticItem, bool) {
	find := func(name string) int { return preprocessor.findEnginePhoneme([]byte(name)) }
	primary, secondary, firstConsonant, hyphen := find("1"), find("2"), find("p"), find("-")
	source := append(append([]byte(nil), names...), 0, 0)
	var items []PhoneticItem
	pending := 0
	parsed := true
	for index := 0; source[index] != 0; index++ {
		if source[index] == escape {
			end := index
			for source[end] != 0 && source[end] != space {
				end++
			}
			items = append(items, PhoneticItem{Escape: append(append([]byte(nil), source[index:end]...), space)})
			if source[end] == 0 {
				parsed = false
				break
			}
			index = end
			continue
		}
		if source[index] == tab || source[index] == carriageReturn || source[index] == lineFeed {
			source[index] = space
		}
		single := preprocessor.findEnginePhoneme(source[index : index+1])
		pairKey := source[index : index+2]
		if pairKey[1] == 0 {
			pairKey = pairKey[:1]
		}
		pair := preprocessor.findEnginePhoneme(pairKey)
		if single == 0 && pair == 0 {
			parsed = false
			break
		}
		code := single
		if single == 0 || pair != 0 && evenAspirates(source, index+2) {
			code = pair
			index++
		}
		switch {
		case pending == 0:
			if code == primary || code == secondary {
				pending = code
			}
		case code < firstConsonant:
			pending = 0
		case code >= hyphen:
			return preprocessor.merge(items), false
		}
		items = append(items, PhoneticItem{Code: code})
	}
	return preprocessor.merge(items), parsed && pending == 0
}

func evenAspirates(source []byte, index int) bool {
	count := 0
	for index < len(source) && (source[index] == 'h' || source[index] == 'H') {
		count++
		index++
	}
	return count%2 == 0
}

func (preprocessor *Preprocessor) merge(items []PhoneticItem) []PhoneticItem {
	find := func(name string) int { return preprocessor.findEnginePhoneme([]byte(name)) }
	merges := map[[2]int]int{{find("aa"), find("ih")}: find("ay"), {find("aa"), find("uh")}: find("aw"), {find("ao"), find("ih")}: find("oy")}
	merged := make([]PhoneticItem, 0, len(items))
	for index := 0; index < len(items); index++ {
		item := items[index]
		if item.Escape == nil && index+1 < len(items) && items[index+1].Escape == nil {
			if code, found := merges[[2]int{item.Code, items[index+1].Code}]; found {
				item.Code = code
				index++
			}
		}
		merged = append(merged, item)
	}
	return merged
}

// convertEnginePhonemes turns engine phoneme names into the analyser's phone symbols between ESC / and
// ESC /, copying escapes (text.md §2.3).
func (preprocessor *Preprocessor) convertEnginePhonemes(tagged []byte) []byte {
	items, _ := preprocessor.ParseEnginePhonemes(tagged)
	output := []byte{escape, '/'}
	for _, item := range items {
		if item.Escape != nil {
			output = append(output, item.Escape...)
			continue
		}
		output = append(output, preprocessor.tables.Phonemes[item.Code-1].InternalSymbol...)
	}
	return append(output, escape, '/')
}

// IPAToEngineNames converts IPA without tags to engine phoneme names, up to the first U+0000, as the
// lexicon does (pronunciation.md §6.4). It reports false for a symbol the table doesn't have and for a
// backslash.
func (preprocessor *Preprocessor) IPAToEngineNames(ipa []uint16) ([]byte, bool) {
	var names []byte
	for _, unit := range ipa {
		if unit == 0 {
			break
		}
		code := preprocessor.ipaCode(unit)
		if code == 0 || unit == backslash {
			return names, false
		}
		names = append(names, preprocessor.tables.Phonemes[code-1].WideName...)
	}
	return names, true
}

// EngineNamesToIPA converts engine phoneme names to IPA as the lexicon does (pronunciation.md §6.4): what
// the parse reads, an error or not, with aw, ay and oy written as two symbols. Escapes are left out.
func (preprocessor *Preprocessor) EngineNamesToIPA(names []byte) []uint16 {
	items, _ := preprocessor.ParseEnginePhonemes(names)
	pairs := map[int][]rune{
		preprocessor.FindEnginePhoneme("aw"): {'ɑ', 'ʊ'},
		preprocessor.FindEnginePhoneme("ay"): {'ɑ', 'ɪ'},
		preprocessor.FindEnginePhoneme("oy"): {'ɔ', 'ɪ'},
	}
	var ipa []uint16
	for _, item := range items {
		switch pair, found := pairs[item.Code]; {
		case item.Escape != nil:
		case found:
			ipa = append(ipa, utf16.Encode(pair)...)
		default:
			ipa = append(ipa, preprocessor.tables.Phonemes[item.Code-1].IPA)
		}
	}
	return ipa
}
