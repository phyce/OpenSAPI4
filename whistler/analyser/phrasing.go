package analyser

import "github.com/phyce/OpenSAPI4/whistler/rules"

const phraseBreak = 0x19

func isClausePunctuation(code byte) bool {
	switch code {
	case ',', '.', ';', '?', '!', ':', '#', phraseBreak:
		return true
	}
	return false
}

func isPhrasePunctuation(code byte) bool {
	return isClausePunctuation(code) || code == '(' || code == ')'
}

// phrasingNatives are module B's natives (prosody.md §2.3, §2.4). They index the tiers from 0, so index i is
// cell i + 1, as the original does.
func (analyser *Analyser) phrasingNatives() map[int]rules.Native {
	return map[int]rules.Native{
		4: insertSubjectBreaks,
		5: insertPhraseBreaks,
		7: func(machine *rules.Machine, start int, end *int) bool {
			copyTier4ToTier1(machine.State, start, *end)
			return true
		},
		8: applyPartOfSpeech,
	}
}

type indexed struct {
	state *rules.State
}

func (tiers indexed) at(number int, index int) byte {
	return tiers.state.Tier(number)[index+1]
}

func (tiers indexed) set(number int, index int, code byte) {
	tiers.state.Tier(number)[index+1] = code
}

// countChunk counts the spaces minus the 0x11 bytes of tier 1 at indexes first..stop-1.
func (tiers indexed) countChunk(first int, stop int) int16 {
	count := 0
	for index := first; index < stop; index++ {
		switch tiers.at(1, index) {
		case ' ':
			count++
		case 0x11:
			count--
		}
	}
	return int16(count)
}

func copyTier4ToTier1(state *rules.State, start int, end int) {
	tiers := indexed{state}
	for index := start - 1; index < end; index++ {
		tiers.set(1, index, tiers.at(4, index))
	}
}

// insertSubjectBreaks is procedure 4: a phrase break before a verb chunk that follows a noun-phrase subject
// of three or more words, when three or more words follow.
func insertSubjectBreaks(machine *rules.Machine, start int, end *int) bool {
	tiers := indexed{machine.State}
	for index := start - 1; index < *end; index++ {
		if tiers.at(4, index) != 'V' || tiers.at(3, index) == 'S' {
			continue
		}
		left := index - 1
		for {
			if code := tiers.at(4, left); code != ' ' && code != rules.Empty {
				break
			}
			if left <= start-1 {
				break
			}
			if code := tiers.at(3, left); code == 'P' || code == 'S' {
				break
			}
			if isClausePunctuation(tiers.at(1, left)) {
				break
			}
			left--
		}
		wordsBefore := tiers.countChunk(left, index)
		right := index + 1
		for right < *end && !isClausePunctuation(tiers.at(1, right)) {
			right++
		}
		wordsAfter := tiers.countChunk(index, right)
		verb := index
		if tiers.at(1, index-1) == rules.Empty {
			for verb > start {
				verb--
				if tiers.at(1, verb-1) != rules.Empty {
					break
				}
			}
		}
		if tiers.at(1, verb-1) == ' ' && tiers.at(4, left) == 'N' && wordsBefore >= 3 && wordsAfter >= 3 {
			tiers.set(1, verb-1, phraseBreak)
		}
	}
	return true
}

// insertPhraseBreaks is procedure 5: '?' inside a word is protected as '%', a phrase break goes before each
// long prepositional phrase marked 'B', and a noun chunk after a "be" verb is joined to it. Its loops start
// one cell late, at index start.
func insertPhraseBreaks(machine *rules.Machine, start int, end *int) bool {
	tiers := indexed{machine.State}
	for index := start; index < *end-1; index++ {
		if tiers.at(1, index) == '?' && tiers.at(1, index+1) != ' ' {
			tiers.set(1, index, '%')
		}
	}
	for index := start; index < *end; {
		if tiers.at(1, index) != 'B' || tiers.at(4, index-1) != 'P' {
			index++
			continue
		}
		verbFound := false
		left := index - 2
		for left > start-1 && !isPhrasePunctuation(tiers.at(1, left)) {
			left--
		}
		chunk := left
		if tiers.at(1, left+1) == ' ' {
			chunk = left + 2
		}
		wordsBefore := tiers.countChunk(chunk, index)
		right := left + 1
		for right < *end && !isPhrasePunctuation(tiers.at(1, right)) {
			right++
		}
		wordsAfter := tiers.countChunk(index, right)
		words := 0
		for back := index - 1; back >= chunk && words < 3; back-- {
			if tiers.at(1, back) == ' ' {
				words++
			}
			switch tiers.at(3, back) {
			case 'V', 'v', 'R', 'r', 'x', 'X', 'M':
				verbFound = true
			}
		}
		if tiers.at(1, index-2) == ' ' && !verbFound && wordsBefore >= 2 && wordsAfter >= 3 {
			tiers.set(1, index-2, phraseBreak)
			position := index + 1
			words = 0
			for {
				if tiers.at(1, position) == ' ' {
					words++
				}
				position++
				if words >= 3 || position >= rules.Cells-1 {
					break
				}
			}
			index = position - 1
		}
		index++
	}
	for index := start; index < *end-1; index++ {
		if tiers.at(1, index) == '%' {
			tiers.set(1, index, '?')
		}
	}
	for index := *end - 2; index > start; index-- {
		if tiers.at(4, index) != 'N' || index-2 <= start {
			continue
		}
		limit := start - 1
		position := index
		if index > limit {
			for code := tiers.at(1, position); code != ' ' && code != phraseBreak; code = tiers.at(1, position) {
				position--
				if position <= limit {
					break
				}
			}
		}
		for {
			position--
			if position <= limit {
				break
			}
			if code := tiers.at(1, position); code == ' ' || code == phraseBreak {
				break
			}
		}
		position++
		if tiers.at(1, position) == 'G' {
			position++
		}
		if tiers.at(3, position) == 'X' && tiers.at(4, position) == 'V' {
			tiers.set(4, index, rules.Empty)
			tiers.set(4, index-1, rules.Empty)
		}
	}
	return true
}

// applyPartOfSpeech is procedure 8: it applies a 0x11 '@' x part-of-speech marker (text.md §8.2), which
// module C copied into tier 3, to the word after it. The index it finds is passed on as a cell number, one
// cell early, as in the original.
func applyPartOfSpeech(machine *rules.Machine, start int, end *int) bool {
	tiers := indexed{machine.State}
	stop := *end
	index := start - 1
	for index < stop && tiers.at(3, index) != 0x11 {
		index++
	}
	for index < stop && tiers.at(3, index) != '@' {
		index++
	}
	for {
		index++
		if !(index < stop && tiers.at(3, index) == rules.Empty) {
			break
		}
	}
	tag := tiers.at(3, index)
	index += 2
	word := index
	for word < stop {
		if code := tiers.at(3, word); code != ' ' && code != rules.Empty {
			break
		}
		word++
	}
	if word >= stop {
		word = index
	}
	target := word
	if tag == '@' {
		if tiers.at(3, target) == 'H' {
			limit := target
			for code := tiers.at(4, limit); code != '&' && code != ' '; code = tiers.at(4, limit) {
				limit++
			}
			for tiers.at(4, limit) == rules.Empty {
				limit++
			}
			if limit > target {
				copyTier4ToTier1(machine.State, target, limit)
			}
		}
		return true
	}
	if tiers.at(3, target) != 'H' {
		return writeTag(tiers, target, tag, *end)
	}
	digitIndex := target
	for {
		digitIndex++
		if !(digitIndex < stop && tiers.at(3, digitIndex) == rules.Empty) {
			break
		}
	}
	resolve := false
	switch digit := int(tiers.at(3, digitIndex)) - '0'; digit {
	case 0:
		if tag == '1' {
			tiers.set(3, target, rules.Empty)
			tiers.set(3, digitIndex, rules.Empty)
			return true
		}
		search := digitIndex + 1
		for search < stop && tiers.at(3, search) != tag && !(tiers.at(3, search) == 'v' && tag == 'V') {
			search++
		}
		if search != stop {
			tiers.set(3, target-1, tag)
			for position := target; position < *end; position++ {
				tiers.set(3, position, rules.Empty)
			}
			return writeTag(tiers, target, tag, *end)
		}
		machine.Call(9, target, end)
		if tag == '2' {
			return true
		}
		return writeTag(tiers, target, tag, *end)
	case 1, 2, 3, 8:
		resolve = tag == 'N'
	case 4:
		resolve = tag == 'J' || tag == 'N'
	case 5, 6:
		resolve = tag == 'J'
	case 7:
		resolve = tag == 'R'
	default:
		return writeTag(tiers, target, tag, *end)
	}
	if resolve {
		machine.Call(9, target, end)
		return true
	}
	return writeTag(tiers, target, tag, *end)
}

func writeTag(tiers indexed, target int, tag byte, end int) bool {
	tiers.set(3, target, tag)
	for position := target + 1; position < end; position++ {
		tiers.set(3, position, rules.Empty)
	}
	return true
}
