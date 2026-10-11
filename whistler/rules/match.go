package rules

import "github.com/phyce/OpenSAPI4/whistler/datapack"

func isCountTest(operation byte) bool {
	switch operation {
	case 0xF2, 0xF3, 0xF9, 0xFA, 0xFB, 0xFC:
		return true
	}
	return false
}

func maximumCount(value byte) int {
	if value == 0xDC {
		return unbounded
	}
	return int(value)
}

func (machine *Machine) inClass(class byte, symbol byte) bool {
	return machine.module.Classes[class-1][symbol]
}

func (machine *Machine) inSeries(series byte, symbol byte) bool {
	return machine.module.Series[series-1].Members[symbol]
}

func (machine *Machine) simpleTest(operation byte, operand byte, symbol byte) bool {
	switch operation {
	case 0xF2:
		return operand != symbol
	case 0xF3:
		return operand == symbol
	case 0xF9:
		return !machine.inSeries(operand, symbol)
	case 0xFA:
		return machine.inSeries(operand, symbol)
	case 0xFB:
		return !machine.inClass(operand, symbol)
	}
	return machine.inClass(operand, symbol)
}

// variableAccepts tells whether a first binding of variable to symbol passes its type.
func (machine *Machine) variableAccepts(variable byte, symbol byte) bool {
	declared := machine.module.Variables[variable-1]
	if declared.Class != 0 {
		return machine.inClass(byte(declared.Class), symbol)
	}
	return machine.inSeries(byte(declared.Series), symbol)
}

func skipEmpty(tier *[Cells]byte, position int, step int) int {
	for tier[position] == Empty {
		position += step
	}
	return position
}

// matchFocus matches the focus from position (rules.md §6.1) and returns where the match ends, or 0.
func (machine *Machine) matchFocus(position int, end int) int {
	code := machine.code
	tier := machine.tier(machine.inputTier)
	skipping := machine.skip != 0
	focusCount := 0
	for {
		operation := code[machine.pc]
		switch {
		case operation == 0xEA:
			if skipping {
				position = skipEmpty(tier, position, 1)
			}
			machine.current = tier[position]
			if value := machine.variables[code[machine.pc+1]]; value != 0 && machine.current == value {
				return 0
			}
		case operation == 0xEB:
			position = end + 1
			machine.skip, skipping = 1, true
			machine.pc++
			continue
		case operation == 0xEC:
			position = skipEmpty(tier, position, 1)
			machine.current = tier[position]
			machine.skip, skipping = 1, true
			machine.pc++
			continue
		case operation == 0xED:
			last := position - 1
			if skipping {
				position = skipEmpty(tier, position, 1)
			}
			machine.current = tier[position]
			maximum := maximumCount(code[machine.pc+2])
			test, operand := code[machine.pc+3], code[machine.pc+4]
			if isCountTest(test) {
				focusCount = 0
				for focusCount < maximum {
					machine.current = tier[position]
					if machine.current == Edge || !machine.simpleTest(test, operand, machine.current) {
						break
					}
					last = position
					position = skipEmpty(tier, position+1, 1)
					focusCount++
				}
			}
			if focusCount < int(code[machine.pc+1]) {
				return 0
			}
			machine.skip, skipping = 1, true
			machine.pc += 6
			position = last + 1
			continue
		case operation == 0xEE || operation == 0xF0:
			machine.inputTier = machine.savedFocusTier
			tier = machine.tier(machine.inputTier)
			machine.skip, skipping = 0, false
			machine.pc += 2
			continue
		case operation == 0xEF || operation == 0xF1:
			machine.savedFocusTier = machine.inputTier
			machine.inputTier = int(code[machine.pc+1])
			tier = machine.tier(machine.inputTier)
			machine.skip, skipping = 0, false
			machine.pc += 2
			continue
		case isCountTest(operation):
			if skipping {
				position = skipEmpty(tier, position, 1)
			}
			machine.current = tier[position]
			if machine.current == Edge || !machine.simpleTest(operation, code[machine.pc+1], machine.current) {
				return 0
			}
		case operation == 0xF4:
			start := machine.pc
			last := position - 1
			if skipping {
				position = skipEmpty(tier, position, 1)
			}
			machine.current = tier[position]
			maximum := maximumCount(code[machine.pc+2])
			focusCount = 0
			if maximum != 0 {
				for {
					machine.pc = start + 3
					if tier[position] == Edge {
						break
					}
					matched := machine.matchTestList(0xF4, position)
					tier = machine.tier(machine.inputTier)
					if !matched {
						break
					}
					last = position
					position = skipEmpty(tier, position+1, 1)
					focusCount++
					machine.current = tier[position]
					if focusCount >= maximum {
						break
					}
				}
			}
			if focusCount < int(code[start+1]) {
				return 0
			}
			for code[machine.pc] != 0xF4 {
				machine.pc++
			}
			machine.skip, skipping = 1, true
			position = last + 1
			machine.pc++
			continue
		case operation == 0xF7:
			if skipping {
				position = skipEmpty(tier, position, 1)
			}
			machine.current = tier[position]
			machine.pc++
			if tier[position] == Edge || !machine.matchTestList(0xF7, position) {
				return 0
			}
			position++
			machine.skip, skipping = 1, true
			machine.pc++
			tier = machine.tier(machine.inputTier)
			continue
		case operation == 0xF8:
			if skipping {
				position = skipEmpty(tier, position, 1)
			}
			machine.pc++
			machine.current = tier[position]
			series := code[machine.pc]
			if !machine.inSeries(series, machine.current) {
				return 0
			}
			machine.seriesRegister = machine.module.Series[series-1].Indexes[machine.current]
			position++
			machine.skip, skipping = 1, true
			machine.pc++
			continue
		case operation == 0xFD:
			if skipping {
				position = skipEmpty(tier, position, 1)
			}
			machine.current = tier[position]
			variable := code[machine.pc+1]
			if machine.variables[variable] == 0 {
				if !machine.variableAccepts(variable, machine.current) {
					return 0
				}
				machine.dirty = true
				machine.variables[variable] = machine.current
			} else if machine.current != machine.variables[variable] {
				return 0
			}
		case operation == sectionEnd:
			return position
		default:
			if skipping {
				position = skipEmpty(tier, position, 1)
			}
			machine.current = tier[position]
			if machine.current != operation {
				return 0
			}
			position++
			machine.skip, skipping = 1, true
			machine.pc++
			continue
		}
		skipping = true
		position++
		machine.skip = 1
		machine.pc += 2
	}
}

// matchTestList applies a test list from machine.pc to the symbol at position (rules.md §6.2).
func (machine *Machine) matchTestList(terminator byte, position int) bool {
	code := machine.code
	pc := machine.pc
	symbol := machine.current
	saved := machine.savedTestTier
	currentTier := machine.inputTier
	for {
		operation := code[pc]
		switch {
		case operation == terminator:
			return true
		case operation == 0xEA:
			if value := machine.variables[code[pc+1]]; value != 0 && value == symbol {
				return false
			}
		case operation == 0xEE || operation == 0xF0:
			symbol = machine.tier(saved)[position]
			currentTier = saved
			machine.inputTier = saved
			machine.current = symbol
		case operation == 0xEF || operation == 0xF1:
			next := int(code[pc+1])
			symbol = machine.tier(next)[position]
			saved = currentTier
			machine.savedTestTier = currentTier
			currentTier = next
			machine.inputTier = next
			machine.current = symbol
		case isCountTest(operation):
			if !machine.simpleTest(operation, code[pc+1], symbol) {
				return false
			}
		case operation == 0xFD:
			variable := code[pc+1]
			if machine.variables[variable] == 0 {
				if !machine.variableAccepts(variable, symbol) {
					return false
				}
				machine.dirty = true
				machine.variables[variable] = symbol
			} else if symbol != machine.variables[variable] {
				return false
			}
		}
		pc += 2
		machine.pc = pc
	}
}

func (machine *Machine) skipContext(tier *[Cells]byte) {
	if machine.skip != 0 {
		machine.contextPosition = skipEmpty(tier, machine.contextPosition, machine.skip)
	}
}

func (machine *Machine) findMarker(marker byte, number byte) bool {
	found, landed := datapack.FindRuleMarker(machine.code, machine.pc, marker, number)
	machine.pc = landed
	return found
}

func (machine *Machine) push() *stackEntry {
	machine.depth++
	return &machine.stack[machine.depth]
}

// contextElement matches one context element at machine.contextPosition (rules.md §6.3) and leaves
// machine.pc on the element's last byte.
func (machine *Machine) contextElement() bool {
	code := machine.code
	start := machine.pc
	tier := machine.tier(machine.inputTier)
	operation := code[machine.pc]
	matched := true
	switch {
	case operation == 0xEA:
		machine.skipContext(tier)
		machine.pc++
		machine.current = tier[machine.contextPosition]
		value := machine.variables[code[machine.pc]]
		matched = !(value != 0 && machine.current == value)
	case operation == 0xEC:
		position := skipEmpty(tier, machine.contextPosition, machine.direction)
		machine.contextPosition = position - machine.direction
		machine.skip = machine.direction
		machine.current = tier[position]
		return true
	case operation == 0xED:
		last := machine.contextPosition - machine.direction
		machine.skipContext(tier)
		machine.current = tier[machine.contextPosition]
		maximum := maximumCount(code[machine.pc+2])
		test, operand := code[machine.pc+3], code[machine.pc+4]
		count := 0
		if isCountTest(test) {
			for count < maximum {
				machine.current = tier[machine.contextPosition]
				if machine.current == Edge || !machine.simpleTest(test, operand, machine.current) {
					break
				}
				position := skipEmpty(tier, machine.contextPosition+machine.direction, machine.direction)
				count++
				last = machine.contextPosition
				machine.contextPosition = position
			}
		}
		matched = int(code[machine.pc+1]) <= count
		machine.pc += 5
		machine.contextPosition = last
		machine.skip = machine.direction
		return matched
	case operation == 0xEE || operation == 0xF0:
		machine.inputTier = machine.savedContextTier
		machine.pc++
		machine.contextPosition -= machine.direction
		machine.skip = 0
		return true
	case operation == 0xEF || operation == 0xF1:
		machine.savedContextTier = machine.inputTier
		machine.inputTier = int(code[machine.pc+1])
		machine.pc++
		machine.contextPosition -= machine.direction
		machine.skip = 0
		return true
	case operation == 0xF2 || operation == 0xF3:
		machine.skipContext(tier)
		machine.pc++
		machine.current = tier[machine.contextPosition]
		matched = machine.current != Edge && machine.simpleTest(operation, code[machine.pc], machine.current)
	case operation == 0xF4:
		last := machine.contextPosition - machine.direction
		machine.skipContext(tier)
		machine.current = tier[machine.contextPosition]
		maximum := maximumCount(code[machine.pc+2])
		count := 0
		if maximum != 0 {
			position := machine.contextPosition
			for {
				machine.pc = start + 3
				if machine.tier(machine.inputTier)[position] == Edge || !machine.matchTestList(0xF4, position) {
					break
				}
				last = machine.contextPosition
				position = machine.contextPosition + machine.direction
				machine.contextPosition = position
				for machine.tier(machine.inputTier)[position] == Empty {
					position += machine.direction
					machine.contextPosition = position
				}
				count++
				machine.current = machine.tier(machine.inputTier)[position]
				if count >= maximum {
					break
				}
			}
		}
		machine.contextPosition = last
		matched = int(code[start+1]) <= count
		for code[machine.pc] != 0xF4 {
			machine.pc++
		}
	case operation == 0xF5:
		machine.pc++
		if code[machine.pc] != 0 {
			entry := machine.push()
			entry.kind, entry.skip, entry.tier, entry.position, entry.pc = 0xF5, machine.skip, machine.inputTier, machine.contextPosition, start+2
			machine.findMarker(0xF5, 0)
			machine.pc++
		}
		machine.contextPosition -= machine.direction
		return true
	case operation == 0xF6:
		machine.pc++
		switch value := code[machine.pc]; {
		case value == 1:
			entry := machine.push()
			entry.kind, entry.skip, entry.tier, entry.position, entry.count, entry.pc = 0xF6, machine.skip, machine.inputTier, machine.contextPosition, 1, start+2
		case value >= 2:
			machine.findMarker(0xF6, 0)
			machine.pc++
		}
		machine.contextPosition -= machine.direction
		return true
	case operation == 0xF7:
		return machine.repeatElement(start, tier)
	case operation >= 0xF9 && operation <= 0xFC:
		machine.skipContext(tier)
		machine.current = tier[machine.contextPosition]
		if machine.current == Edge {
			matched = false
			machine.pc = start
			if operation == 0xFB {
				machine.pc = start + 1
			}
		} else {
			machine.pc++
			matched = machine.simpleTest(operation, code[machine.pc], machine.current)
		}
	case operation == 0xFD:
		machine.skipContext(tier)
		machine.pc++
		machine.current = tier[machine.contextPosition]
		variable := code[machine.pc]
		if value := machine.variables[variable]; value != 0 {
			matched = machine.current == value
		} else if machine.variableAccepts(variable, machine.current) {
			machine.dirty = true
			machine.variables[variable] = machine.current
		} else {
			matched = false
		}
	default:
		machine.skipContext(tier)
		machine.current = tier[machine.contextPosition]
		matched = machine.current == operation
	}
	machine.skip = machine.direction
	return matched
}

// repeatElement is f7 m n tests f7 in a context: m symbols at first, one more each time a later element
// fails (a lazy repeat).
func (machine *Machine) repeatElement(start int, tier *[Cells]byte) bool {
	code := machine.code
	machine.pc = start + 3
	matched := true
	if !machine.repeatFresh {
		machine.skipContext(tier)
		position := machine.contextPosition
		machine.current = tier[position]
		machine.repeatFresh = true
		if tier[position] == Edge {
			matched = false
		} else {
			matched = machine.matchTestList(0xF7, position)
		}
		entry := &machine.stack[machine.depth]
		entry.position = machine.contextPosition + machine.direction
		entry.skip = machine.direction
		if !matched {
			machine.depth--
		}
		machine.skip = machine.direction
		return matched
	}
	entry := machine.push()
	entry.kind, entry.count, entry.tier, entry.pc = 0xF7, 0, machine.inputTier, start
	entryTier := machine.inputTier
	if code[start+1] != 0 {
		for matched {
			matched = false
			position := machine.contextPosition
			if machine.skip != 0 {
				position = skipEmpty(machine.tier(entryTier), position, machine.skip)
			}
			machine.current = machine.tier(entryTier)[position]
			entry.count++
			machine.pc = start + 3
			machine.contextPosition = position
			if machine.tier(entryTier)[position] != Edge {
				matched = machine.matchTestList(0xF7, position)
				entryTier = machine.inputTier
			}
			machine.contextPosition += machine.direction
			machine.skip = machine.direction
			if entry.count >= code[start+1] {
				break
			}
		}
	}
	entry.skip = machine.skip
	if entry.count == 0 {
		pc := start + 3
		for code[pc] != 0xF7 {
			pc += 2
		}
		machine.pc = pc
	}
	entry.position = machine.contextPosition
	if !matched {
		machine.depth--
	}
	machine.contextPosition -= machine.direction
	return matched
}

// runElements matches elements from machine.pc up to the section end: the loop of a resumed backtracking
// point.
func (machine *Machine) runElements(matched bool) bool {
	for machine.code[machine.pc] != sectionEnd && matched {
		matched = machine.contextElement()
		machine.pc++
		machine.contextPosition += machine.direction
	}
	return matched
}

// matchContext matches a context from machine.contextPosition in machine.direction, backtracking through
// its optional groups, choices and lazy repeats (rules.md §6.3).
func (machine *Machine) matchContext() bool {
	code := machine.code
	machine.depth = 0
	matched := true
	machine.skip = machine.direction
	machine.repeatFresh = true
	for {
		machine.pc++
		if code[machine.pc] == sectionEnd || !matched {
			break
		}
		matched = machine.contextElement()
		machine.contextPosition += machine.direction
	}
	if matched && code[machine.pc] == sectionEnd {
		return true
	}
	for {
		if machine.depth < 1 {
			return matched
		}
		entry := &machine.stack[machine.depth]
		machine.contextPosition = entry.position
		entry.count++
		machine.pc = entry.pc
		machine.inputTier = entry.tier
		machine.skip = entry.skip
		switch entry.kind {
		case 0xF5:
			machine.depth--
			matched = machine.runElements(true)
			if matched && code[machine.pc] == sectionEnd {
				return true
			}
		case 0xF6:
			if !machine.findMarker(0xF6, entry.count) {
				machine.pc += 2
				machine.depth--
				if matched {
					return matched
				}
				continue
			}
			matched = true
			machine.pc += 2
			for code[machine.pc] != sectionEnd && matched {
				matched = machine.contextElement()
				machine.contextPosition += machine.direction
				machine.pc++
			}
			if matched && code[machine.pc] == sectionEnd {
				return true
			}
		case 0xF7:
			if code[machine.pc+2] < entry.count {
				machine.depth--
				if matched {
					return matched
				}
				continue
			}
			machine.repeatFresh = false
			matched = machine.runElements(true)
			if matched && code[machine.pc] == sectionEnd {
				return true
			}
		default:
			if matched {
				return matched
			}
		}
	}
}
