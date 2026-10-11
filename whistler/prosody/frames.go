package prosody

func (analyser *analyser) findUnit(position int) (unit int, last int) {
	tier1 := analyser.tier1
	acrossDash := tier1[position+1] == '-'
	following := position + 2
	right := tier1[position+1]
	if acrossDash {
		following = position + 3
		right = tier1[position+2]
	}
	list, unit := analyser.tables.UnitPair(tier1[position], right, acrossDash)
	last = following - 1
	if list > 0 {
		lengths := analyser.tables.UnitLists[list]
		for length := len(lengths); length > 0; length-- {
			if analyser.end-following+1 < length {
				continue
			}
			if found, matched := lengths[length-1][string(tier1[following:following+length])]; matched {
				return found, following + length - 1
			}
		}
	}
	return unit, last
}

func (analyser *analyser) assignUnits() {
	for cell := analyser.start; cell <= analyser.end; cell++ {
		analyser.units[cell], analyser.frames[cell], analyser.closures[cell] = 0, 0, 0
	}
	for position := analyser.start; position < analyser.end; {
		first := position
		unit, last := analyser.findUnit(position)
		analyser.units[first] = int16(unit)
		frames := analyser.tables.UnitFrames[unit-1]
		phone := 0
		for cell := first; cell <= last; cell++ {
			if analyser.tier1[cell] == '-' {
				continue
			}
			count := frames[phone]
			analyser.frames[cell] += int16(count)
			analyser.closures[cell] = 0
			switch analyser.tier1[cell] {
			case '?', 'b', 'd', 'g', 'k', 'p', 't':
				analyser.closures[cell] = count
			}
			phone++
		}
		position = last
	}
}

func (analyser *analyser) fitDurations() {
	warpCount := 0
	for cell := analyser.start; cell <= analyser.end; cell++ {
		frames := int(analyser.frames[cell])
		if frames == 0 {
			continue
		}
		if analyser.durations[cell] <= 0 {
			analyser.durations[cell] = 1
		}
		onset, offset, middle := 0, 0, frames
		if analyser.tier1[cell] != '#' {
			if int(analyser.closures[cell]) > frames>>1 {
				analyser.closures[cell] = byte(frames >> 1)
			}
			closure := int(analyser.closures[cell])
			onset = int(int16((frames - closure) >> 2))
			if onset == 0 {
				onset = 1
			}
			offset = closure
			if closure == 0 {
				offset = onset
			}
			middle = int(int16(frames - onset - offset))
		}
		if middle > 0 && int16((int(int16(int(analyser.durations[cell])*10))-(onset+offset)*rate)/middle) < 0 {
			analyser.durations[cell] = int16(((onset+offset)*rate + 5) / 10)
		}
		if warpCount < warpLimit {
			for _, part := range []int{onset, middle, offset} {
				if part > 0 {
					warpCount++
				}
			}
			continue
		}
		analyser.durations[cell] = int16(frames * rate / 10)
	}
}
