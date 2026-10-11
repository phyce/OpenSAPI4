package analyser

import "github.com/phyce/OpenSAPI4/whistler/rules"

const (
	segmentSeparator = 0x1D
	pointOpen        = 0x1E
	pointClose       = 0x1F
	annotationClose  = ']'
	durationLimit    = 2700
	fixedDuration    = 3000
)

func isDigit(code byte) bool {
	return code >= '0' && code <= '9'
}

// accentNatives are module A's natives (prosody.md §3.6, §3.7).
func (analyser *Analyser) accentNatives() map[int]rules.Native {
	return map[int]rules.Native{
		3:  spreadWordTag,
		4:  clearProsodyTier,
		11: analyser.applyAnnotation,
		12: copyMarkToNextPhone,
		13: analyser.connectedSpeech,
	}
}

// spreadWordTag is procedure 3: every cell of the span gets the span's last tier 3 byte that is not '~'.
func spreadWordTag(machine *rules.Machine, start int, end *int) bool {
	tier3 := machine.State.Tier(3)
	tag := byte(rules.Empty)
	last := start
	for cell := start; cell <= *end; cell++ {
		if tier3[cell] != rules.Empty {
			tag = tier3[cell]
			last = cell
		} else {
			tier3[cell] = tag
		}
	}
	for cell := start; cell < last; cell++ {
		tier3[cell] = tag
	}
	return true
}

func clearProsodyTier(machine *rules.Machine, start int, end *int) bool {
	tier5 := machine.State.Tier(5)
	for cell := start; cell <= *end; cell++ {
		tier5[cell] = rules.Empty
	}
	return true
}

// copyMarkToNextPhone is procedure 12: the tier 4 byte of a '"', '\”, '`' or 'G' cell (unless '~') goes to
// the next cell whose tier 1 is not '~'.
func copyMarkToNextPhone(machine *rules.Machine, start int, end *int) bool {
	tier1, tier4 := machine.State.Tier(1), machine.State.Tier(4)
	for cell := start; cell < *end; cell++ {
		switch tier1[cell] {
		case '"', '\'', '`', 'G':
			if tier4[cell] == rules.Empty {
				continue
			}
			step := 1
			for cell+step < rules.Cells-1 && tier1[cell+step] == rules.Empty {
				step++
			}
			tier4[cell+step] = tier4[cell]
		}
	}
	return true
}

// connectedSpeech is procedure 13: group 15 when the duration style is P, else group 14.
func (analyser *Analyser) connectedSpeech(machine *rules.Machine, start int, end *int) bool {
	if analyser.state.DurationStyle != 0 {
		return machine.Call(15, start, end)
	}
	return machine.Call(14, start, end)
}

// applyAnnotation is procedure 11: it reads '[' segment {0x1d segment} ']' after a phone, sets the phone's
// duration or adds an explicit pitch point or a label, and deletes the span (prosody.md §3.7). The phone's
// cell is start - 1 in the group's numbering.
func (analyser *Analyser) applyAnnotation(machine *rules.Machine, start int, end *int) bool {
	tier1 := machine.State.Tier(1)
	phone := start - 1
	inParenthesis := false
	segment := start + 1
	if tier1[segment] != annotationClose {
		for segment < *end {
			cell := segment
			for cell < rules.Cells-1 {
				code := tier1[cell]
				if (code == segmentSeparator || code == annotationClose || code == pointOpen) && !inParenthesis {
					break
				}
				if cell > *end {
					break
				}
				if code == pointOpen {
					inParenthesis = true
				}
				cell++
				if tier1[cell] == pointClose {
					inParenthesis = false
				}
			}
			if cell > segment {
				if code := tier1[segment]; code == pointOpen || code == 'i' {
					analyser.parsePitch(tier1, phone, segment, cell-1)
				} else {
					analyser.parseDuration(tier1, phone, segment, cell-1)
				}
			}
			if tier1[cell] == pointOpen {
				cell--
			}
			cell++
			current := tier1[cell]
			if current == pointOpen {
				inParenthesis = true
			}
			segment = cell
			if current == annotationClose {
				break
			}
		}
	}
	*end = start - 1
	return true
}

// parseDuration reads [n] (a fixed duration, stored as n + 3000), [+n], [-n] or [xn] (added), clamped to
// ±2700, and stores it for the cell before the phone, as the original indexes it.
func (analyser *Analyser) parseDuration(tier1 *[rules.Cells]byte, phone int, first int, last int) {
	cell := first
	fixed, negative := true, false
	if !isDigit(tier1[cell]) {
		fixed = false
		negative = tier1[cell] == '-'
		cell++
	}
	var accumulated int32
	for position := cell; position <= last; position++ {
		accumulated = int32(tier1[position]) - '0' + accumulated*10
	}
	value := int64(accumulated)
	if negative {
		value = -value
	}
	value = min(max(value, -durationLimit), durationLimit)
	if fixed {
		value += fixedDuration
	}
	if index := phone - 1; index >= 0 {
		analyser.state.Durations[index] = int16(value)
	}
}

// signOf classifies the byte before a number: 1 a digit (no sign), 3 '-', 2 anything else ('+' or 'x').
func signOf(code byte) int {
	switch {
	case isDigit(code):
		return 1
	case code == '-':
		return 3
	}
	return 2
}

// parsePitch reads (t, f) into an explicit pitch point, or i<mark> into a tier 5 label.
func (analyser *Analyser) parsePitch(tier1 *[rules.Cells]byte, phone int, first int, last int) {
	state := analyser.state
	cell := first
	if tier1[cell] == 'i' {
		cell++
	}
	if mark := tier1[cell]; mark != pointOpen {
		state.Tier(5)[phone] = mark
		return
	}
	position := cell + 1
	sign := signOf(tier1[cell+1])
	if sign > 1 {
		position = cell + 2
	}
	code := tier1[position]
	var time int32
	lastDigit := position
	for {
		lastDigit = position
		time = int32(code) - '0' + time*10
		code = tier1[lastDigit+1]
		if code == segmentSeparator {
			break
		}
		position = lastDigit + 1
		if lastDigit+1 >= last || position >= rules.Cells-1 {
			break
		}
	}
	if sign == 3 {
		time = -time
	}
	position = lastDigit + 2
	sign = signOf(tier1[lastDigit+2])
	if sign > 1 {
		position = lastDigit + 3
	}
	code = tier1[position]
	var value int32
	for {
		position++
		value = int32(code) - '0' + value*10
		code = tier1[position]
		if code == pointClose || position >= last || position >= rules.Cells-1 {
			break
		}
	}
	if sign == 3 {
		value = -value
	}
	if len(state.Points) < explicitPointLimit {
		if value < 1 {
			value = 1
		}
		state.Points = append(state.Points, ExplicitPoint{Time: time, Value: int16(value), Cell: phone - 1})
		if index := phone - 1; index >= 0 {
			state.PointIndex[index] = int16(len(state.Points))
		}
	}
}
