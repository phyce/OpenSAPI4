package prosody

import "strings"

func (analyser *analyser) markPhrasePositions() {
	marks := analyser.marks
	for cell := 1; cell < len(marks); cell++ {
		marks[cell] = ' '
	}
	current := byte(' ')
	lastDash := -1
	for cell := analyser.end; cell >= analyser.start; cell-- {
		code := analyser.tier1[cell]
		switch {
		case analyser.class(code) != 's':
			marks[cell] = current
		case code == ' ':
			looked := cell + 1
			if analyser.tier4[cell+1] == '~' || analyser.tier4[cell+1] == ' ' {
				looked = cell + 2
			}
			current = 'P'
			if analyser.tier4[looked] == '~' {
				current = 'F'
			}
			lastDash = analyser.markInitial(cell, lastDash)
		case code == '#':
			current = '#'
			lastDash = analyser.markInitial(cell, lastDash)
		case code == '-':
			current = ' '
			lastDash = cell
		default:
			marks[cell] = current
		}
	}
}

func (analyser *analyser) markInitial(cell int, lastDash int) int {
	if lastDash != -1 && cell < lastDash {
		for marked := cell + 1; marked <= lastDash; marked++ {
			analyser.marks[marked] = 'I'
		}
	}
	return -1
}

func polysyllabic(tier1 []byte, start int, end int, cell int) bool {
	if cell < end {
		for position := cell; tier1[position] != '#' && tier1[position] != ' '; {
			position++
			if tier1[position] == '-' {
				return true
			}
			if position >= end {
				break
			}
		}
	}
	if start < cell {
		for position := cell; tier1[position] != '#' && tier1[position] != ' '; {
			position--
			if tier1[position] == '-' {
				return true
			}
			if position <= start {
				break
			}
		}
	}
	return false
}

func (analyser *analyser) computeDurations() {
	for cell := analyser.start; cell <= analyser.end; cell++ {
		value := int(analyser.durations[cell])
		if value > 3000 {
			analyser.durations[cell] = int16(value - 3000)
			continue
		}
		analyser.durations[cell] = int16(analyser.cellDuration(cell, value) + value)
	}
}

var stopFloors = map[byte]int{'b': 155, 'd': 165, 'g': 130, 'k': 180, 'p': 180, 't': 140}

func (analyser *analyser) cellDuration(cell int, value int) int {
	tier1 := analyser.tier1
	code := tier1[cell]
	switch {
	case code == '#':
		switch {
		case cell == analyser.start || cell == analyser.end:
			return 15
		case analyser.tier4[cell] == '.':
			return 400
		case analyser.tier4[cell] == ',':
			return 200
		}
		return 150
	case code == '?':
		if tier1[cell-1] == '#' {
			return 15
		}
		return 70
	case analyser.feature(code) == 8:
		return value
	}
	inherent, minimum := int(analyser.tables.InherentDurations[code]), int(analyser.tables.MinimumDurations[code])
	if (code == 'i' || code == 'U' || code == 'I') && tier1[cell+1] == 'r' {
		inherent, minimum = 180, 100
	}
	mark := analyser.mark(cell)
	vowel := analyser.vowelLike(code)
	own := analyser.class(code)
	scale := 100
	if mark == '#' {
		if vowel {
			scale = 180
		} else if cell-1 > 1 {
			position := cell - 1
			for analyser.mark(position) == '#' && analyser.consonant(tier1[position]) {
				position--
			}
			if analyser.mark(position) == '#' && analyser.class(tier1[position]) == 'V' {
				scale = 180
			}
		}
	}
	if mark == '#' || mark == 'P' {
		if own == 'L' && analyser.class(tier1[cell-1]) == 'V' {
			scale = scale * 7 / 5
		}
	} else if vowel {
		scale = scale * 3 / 5
	}
	if vowel && mark != 'F' && mark != '#' && mark != 'P' {
		scale = scale * 17 / 20
	}
	stretch := scale
	if vowel && polysyllabic(tier1, analyser.start, analyser.end, cell) {
		stretch = scale * 4 / 5
	}
	if analyser.consonant(code) && mark != 'I' {
		stretch = stretch * 17 / 20
	}
	stress := analyser.tier3[cell]
	if stress == ' ' || stress == 'F' || stress == 'S' {
		if stress != 'S' {
			minimum /= 2
		}
		stretch = analyser.stressFactor(cell) * stretch / 100
	}
	if own == 'V' || analyser.feature(code)&1 != 0 {
		if factor := analyser.followingFactor(cell + 1); factor != 0 {
			if own == 'V' && mark != '#' && mark != 'P' {
				factor = factor*3/10 + 70
			}
			stretch = factor * stretch / 100
		}
	}
	if factor := analyser.clusterFactor(cell); factor != 100 {
		stretch = factor * stretch / 100
	}
	duration := int(int16((inherent-minimum)*stretch/100 + minimum))
	duration = max(duration, minimum)
	following := tier1[cell+1]
	if analyser.consonant(code) && analyser.tier2[cell] == '+' {
		duration = int(int16(duration * 18 / 10))
	}
	if analyser.feature(code) == 0x10 && (following == '#' || following == link) {
		if code == 'z' {
			duration = int(int16(duration * 16 / 10))
		} else {
			duration = int(int16(duration * 15 / 10))
		}
	}
	if floor, found := stopFloors[code]; found && (own == 'P' || own == 'Q') && (following == '#' || following == link) {
		duration = max(duration, floor)
	}
	if analyser.tier2[cell] == '*' {
		duration = int(int16(duration * 13 / 10))
	}
	if !analyser.multipleWords {
		duration = int(int16(duration * 11 / 10))
		if tier1[cell-1] == '#' {
			if code == 'm' {
				duration = 75
			} else if code == 'l' && following != 'A' {
				duration = 130
			}
		}
		return duration
	}
	switch stress {
	case ' ', 'F':
		if mark == '#' && code == '$' {
			duration = int(int16(duration * 75 / 100))
		}
	case 'P', 'Z':
		if strings.IndexByte("1FHcfl", analyser.tier5[cell]) >= 0 {
			duration = int(int16(duration * 90 / 100))
		} else {
			duration = int(int16(duration * 85 / 100))
		}
	case 'S':
		duration = int(int16(duration * 95 / 100))
	}
	if mark == 'P' || mark == '#' {
		duration = int(int16(duration * analyser.finalScale / 100))
	}
	duration = int(int16(duration * analyser.durationScale / 100))
	if tier1[cell] == link {
		tier1[cell] = '#'
		duration = 0
	}
	return duration
}

func (analyser *analyser) stressFactor(cell int) int {
	code := analyser.tier1[cell]
	factor := 70
	if analyser.vowelLike(code) && analyser.mark(cell) == ' ' {
		factor = 50
	}
	if analyser.feature(code)&1 != 0 && code != 'h' && analyser.class(analyser.tier1[cell+1]) == 'V' {
		factor = 40
	}
	return factor
}

func (analyser *analyser) skipBreaks(cell int) int {
	for analyser.tier1[cell] == '-' || analyser.tier1[cell] == '~' {
		cell++
	}
	return cell
}

func (analyser *analyser) followingFactor(cell int) int {
	cell = analyser.skipBreaks(cell)
	code := analyser.tier1[cell]
	if analyser.feature(code)&1 != 0 {
		return int(analyser.tables.AfterApproximantFactors[analyser.tier1[analyser.skipBreaks(cell+1)]])
	}
	return int(analyser.tables.FollowingFactors[code])
}

func (analyser *analyser) clusterFactor(cell int) int {
	tier1 := analyser.tier1
	own := analyser.class(tier1[cell])
	factor := 100
	position := analyser.skipBreaks(cell + 1)
	code := tier1[position]
	boundary := code == '#' || code == ' ' && (analyser.mark(cell) == '#' || analyser.mark(cell) == 'P')
	for analyser.feature(code) == 8 {
		position++
		code = tier1[position]
	}
	if own == 'V' && analyser.class(code) == 'V' {
		factor = 120
	} else if own <= 'Q' && analyser.class(code) <= 'Q' && !boundary {
		factor = 70
	}
	position = cell - 1
	for (tier1[position] == '~' || tier1[position] == '-') && position > 2 {
		position--
	}
	code = tier1[position]
	boundary = code == '#' || (code == ' ' || code == '=') && (analyser.mark(position-1) == '#' || analyser.mark(position-1) == 'P')
	for analyser.feature(code) == 8 {
		position--
		code = tier1[position]
	}
	previous := analyser.class(code)
	if previous == 'V' && own == 'V' {
		return 70
	}
	if previous <= 'Q' && own <= 'Q' && !boundary {
		if factor == 100 {
			return 70
		}
		return 50
	}
	return factor
}
