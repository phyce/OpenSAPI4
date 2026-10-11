package rules

func (current *frame) count(tier1 *[Cells]byte, first int, stop int, amount int) {
	for cell := first; cell < stop; cell++ {
		current.histogram[tier1[cell]] += amount
	}
}

// parkTail copies cells start..end+1 to the top of the arrays, last first, while a procedure runs.
func (machine *Machine) parkTail(start int, end int) bool {
	if machine.tailTop-(end+2-start) < 0 {
		machine.fail()
		return false
	}
	for cell := end + 1; cell >= start; cell-- {
		machine.tailTop--
		machine.moveCell(cell, machine.tailTop)
		machine.State.Anchors[machine.tailTop] = machine.State.Anchors[cell]
		machine.State.Anchors[cell] = 0
	}
	return true
}

// restoreTail copies the parked tail back after end, up to its edge, and returns the new end.
func (machine *Machine) restoreTail(end int) int {
	for {
		previous := end
		end++
		if end >= Cells || machine.tailTop >= Cells {
			machine.fail()
			return previous
		}
		machine.moveCell(machine.tailTop, end)
		machine.State.Anchors[end] = machine.State.Anchors[machine.tailTop]
		machine.tailTop++
		if machine.tier(1)[end] == Edge {
			return previous
		}
	}
}

// shiftTail moves cells start..end+1 by delta cells and returns the new end. Anchors of cells that move down
// are added to the anchors at their destination.
func (machine *Machine) shiftTail(start int, end int, delta int) int {
	anchors := &machine.State.Anchors
	if delta < 1 {
		destination := start + delta
		for cell := start; cell <= end+1; cell++ {
			machine.moveCell(cell, destination)
			anchors[destination] += anchors[cell]
			anchors[cell] = 0
			destination++
		}
	} else {
		for cell := end + 1; cell >= start; cell-- {
			machine.moveCell(cell, cell+delta)
			anchors[cell+delta] = anchors[cell]
			anchors[cell] = 0
		}
	}
	return end + delta
}

// applyAction runs the action of rule number on the focus that just matched (rules.md §7).
func (machine *Machine) applyAction(number int, current *frame) bool {
	code := machine.code
	machine.outputTier = current.defaultOutput
	result := true
	length := machine.focusLength
	matchEnd := current.position + length
	tier1 := machine.tier(1)
	var advance int
	if code[machine.pc+1] == 0xFE {
		machine.pc += 2
		switch procedure := int(code[machine.pc]); procedure {
		case 1:
			advance = length - 1
		case 2:
			advance = length - 1
			current.position = stopCell
			current.rule = len(machine.module.Rules)
		default:
			current.count(tier1, current.position, matchEnd, -1)
			if !machine.parkTail(matchEnd, current.end) {
				current.fired = false
				return false
			}
			current.end = matchEnd - 1
			result = machine.Call(procedure, current.position, &current.end)
			newEnd := current.end
			if machine.tailTop <= newEnd {
				machine.failed = true
				current.fired = false
				return false
			}
			for cell := matchEnd - 1; newEnd < cell; cell-- {
				if machine.State.Anchors[cell] != 0 {
					machine.State.Anchors[newEnd] += machine.State.Anchors[cell]
				}
			}
			current.count(tier1, current.position, newEnd+1, 1)
			advance = 0
			if result {
				advance = newEnd - current.position
			}
			current.end = machine.restoreTail(newEnd)
		}
	} else {
		newLength := machine.module.Rule(number).OutputLength
		pad := 0
		machine.pc++
		if code[machine.pc] == 0xE9 {
			if newLength < length {
				pad = length - newLength
				newLength = length
			}
			machine.pc++
		}
		delta := newLength - length
		advance = newLength - 1
		if current.end+1+delta >= Cells {
			machine.fail()
			return false
		}
		current.count(tier1, current.position, matchEnd, -1)
		if delta != 0 {
			current.end = machine.shiftTail(matchEnd, current.end, delta)
		}
		for cell := matchEnd; cell < matchEnd+delta; cell++ {
			for tier := range machine.State.Tiers {
				machine.State.Tiers[tier][cell] = Empty
			}
		}
		write := current.position
		for code[machine.pc] != sectionEnd {
			switch operation := code[machine.pc]; operation {
			case 0xEC:
				for range pad {
					machine.tier(machine.outputTier)[write] = Empty
					write++
				}
			case 0xEE, 0xF0:
				machine.outputTier = current.defaultOutput
				machine.pc++
			case 0xEF, 0xF1:
				machine.outputTier = int(code[machine.pc+1])
				machine.pc++
			case 0xF8:
				machine.pc++
				machine.tier(machine.outputTier)[write] = machine.module.SeriesMaps[code[machine.pc]-1][machine.seriesRegister-1]
				write++
			case 0xFD:
				machine.pc++
				machine.tier(machine.outputTier)[write] = machine.variables[code[machine.pc]]
				write++
			default:
				machine.tier(machine.outputTier)[write] = operation
				write++
			}
			machine.pc++
		}
		current.count(tier1, current.position, write, 1)
	}
	if !current.keepLargest || current.advance < advance {
		current.advance = advance
	}
	return result
}
