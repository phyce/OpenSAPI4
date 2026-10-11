// Package rules runs the analyser's compiled rewrite rules (rules.md): one interpreter for the three
// modules, on the shared tiers.
package rules

import (
	"fmt"

	"github.com/phyce/OpenSAPI4/whistler/datapack"
)

const (
	Cells      = 0x582
	Edge       = 0x0E
	Empty      = 0x7E
	TailStart  = 0x501
	stopCell   = 0x500
	unbounded  = 2200
	sectionEnd = 0xFF
)

// State is the analyser's five tiers and the anchor count of each cell. Cells are numbered from 1; cell 0
// and the cells from 0x501 up start as 0, the others as spaces.
type State struct {
	Tiers   [5][Cells]byte
	Anchors [Cells]byte
}

func NewState() *State {
	state := &State{}
	for tier := range state.Tiers {
		for cell := 1; cell < TailStart; cell++ {
			state.Tiers[tier][cell] = ' '
		}
	}
	return state
}

// Tier returns tier number (1 to 5).
func (state *State) Tier(number int) *[Cells]byte {
	return &state.Tiers[number-1]
}

// Native is a procedure written in code. It gets the focus span and may move its end.
type Native func(machine *Machine, start int, end *int) bool

type stackEntry struct {
	kind     byte
	count    byte
	pc       int
	tier     int
	skip     int
	position int
}

type frame struct {
	rule          int
	position      int
	end           int
	histogram     [256]int
	advance       int
	keepLargest   bool
	fired         bool
	defaultInput  int
	defaultOutput int
}

type abort struct{}

// Machine is one module's interpreter. Its registers are globals in the original, so they last from one
// group run and one sentence to the next.
type Machine struct {
	State *State

	module            *datapack.RuleModule
	natives           map[int]Native
	checkEveryAttempt bool

	failed           bool
	overflowed       bool
	tailTop          int
	code             []byte
	pc               int
	inputTier        int
	outputTier       int
	savedFocusTier   int
	savedContextTier int
	savedTestTier    int
	skip             int
	focusLength      int
	contextPosition  int
	direction        int
	current          byte
	seriesRegister   byte
	variables        [33]byte
	dirty            bool
	repeatFresh      bool
	depth            int
	stack            [24]stackEntry
}

// NewMachine makes the interpreter of a module with its natives. checkEveryAttempt gives module A's
// behaviour: the tail check before every rule attempt, and an abort when a procedure leaves the text at
// the tail.
func NewMachine(module *datapack.RuleModule, state *State, natives map[int]Native, checkEveryAttempt bool) (*Machine, error) {
	for number, kind := range module.Procedures {
		if _, implemented := natives[number]; implemented != (kind == datapack.NativeProcedure) {
			return nil, fmt.Errorf("module %s procedure %d: native %t, implemented %t", module.Name, number, kind == datapack.NativeProcedure, implemented)
		}
	}
	for number := range natives {
		if _, listed := module.Procedures[number]; !listed {
			return nil, fmt.Errorf("module %s has no procedure %d", module.Name, number)
		}
	}
	return &Machine{
		State:             state,
		module:            module,
		natives:           natives,
		checkEveryAttempt: checkEveryAttempt,
		tailTop:           TailStart,
		savedFocusTier:    1,
		savedContextTier:  1,
		savedTestTier:     1,
		direction:         1,
		repeatFresh:       true,
	}, nil
}

func (machine *Machine) Module() *datapack.RuleModule {
	return machine.module
}

// Begin clears the error flags before a module runs; withTail also puts the tail back at its start.
func (machine *Machine) Begin(withTail bool) {
	machine.failed = false
	machine.overflowed = false
	if withTail {
		machine.tailTop = TailStart
	}
}

// Failed reports module B's or C's error flag, set when the text reached the tail.
func (machine *Machine) Failed() bool {
	return machine.failed
}

// Overflowed reports module A's overflow flag.
func (machine *Machine) Overflowed() bool {
	return machine.overflowed
}

func (machine *Machine) tier(number int) *[Cells]byte {
	return &machine.State.Tiers[number-1]
}

// Call runs procedure number on cells start..*end: a group, or a native.
func (machine *Machine) Call(number int, start int, end *int) bool {
	var result bool
	switch machine.module.Procedures[number] {
	case datapack.GroupProcedure:
		result = machine.ApplyGroup(number, start, end)
	case datapack.GroupTrueProcedure:
		machine.ApplyGroup(number, start, end)
		result = true
	case datapack.GroupFalseProcedure:
		machine.ApplyGroup(number, start, end)
	case datapack.NativeProcedure:
		result = machine.natives[number](machine, start, end)
	}
	if machine.checkEveryAttempt && machine.tailTop <= *end {
		panic(abort{})
	}
	return result
}

// Enter calls procedure number as an entry point of module A. It reports whether the module aborted because
// a procedure left the text at the tail; the tiers are then left as the abort found them.
func (machine *Machine) Enter(number int, start int, end *int) (result bool, aborted bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if _, isAbort := recovered.(abort); !isAbort {
				panic(recovered)
			}
			machine.overflowed = true
			aborted = true
		}
	}()
	return machine.Call(number, start, end), false
}

// fail sets the module's error where the original would write past its tiers.
func (machine *Machine) fail() {
	if machine.checkEveryAttempt {
		machine.overflowed = true
	} else {
		machine.failed = true
	}
}

func (machine *Machine) moveCell(from int, to int) {
	for tier := range machine.State.Tiers {
		machine.State.Tiers[tier][to] = machine.State.Tiers[tier][from]
	}
}

func (machine *Machine) openText(start int, end int) {
	for cell := end; cell >= start; cell-- {
		machine.moveCell(cell, cell+1)
		machine.State.Anchors[cell+1] = machine.State.Anchors[cell]
	}
	for tier := range machine.State.Tiers {
		machine.State.Tiers[tier][end+2] = Edge
		machine.State.Tiers[tier][start] = Edge
	}
	machine.State.Anchors[end+2] = 0
	machine.State.Anchors[start] = 0
}

func (machine *Machine) closeText(first int, end int) {
	for cell := first; cell <= end; cell++ {
		machine.moveCell(cell, cell-1)
		machine.State.Anchors[cell-1] = machine.State.Anchors[cell]
	}
}

// ApplyGroup runs group number on cells start..*end (rules.md §4) and reports whether a rule fired.
func (machine *Machine) ApplyGroup(group int, start int, end *int) bool {
	if start < 0 || *end+2 >= Cells || *end < start-1 {
		machine.fail()
		return false
	}
	first := start + 1
	machine.openText(start, *end)
	*end++
	current := &frame{end: *end, defaultInput: 1, defaultOutput: 1}
	machine.outputTier, machine.inputTier = 1, 1
	machine.dirty = false
	machine.seriesRegister = 0
	machine.variables = [33]byte{}
	tier1 := machine.tier(1)
	for cell := first; cell <= current.end; cell++ {
		current.histogram[tier1[cell]]++
	}
	current.histogram[Edge] = 2
	rules := machine.module.Groups[group]
	current.rule = rules.First - 1
	for current.rule < rules.Last {
		if machine.failed {
			*end = current.end - 1
			machine.closeText(first, current.end)
			return false
		}
		current.rule++
		machine.runRule(current, first)
	}
	*end = current.end - 1
	machine.closeText(first, current.end)
	return current.fired
}

func (machine *Machine) required(current *frame, rule *datapack.Rule) bool {
	return current.histogram[rule.Required1] != 0 && current.histogram[rule.Required2] != 0
}

func (machine *Machine) runRule(current *frame, first int) {
	number := current.rule
	rule := machine.module.Rule(number)
	mode := rule.Mode()
	if mode > 100 {
		mode -= 100
	}
	if mode > 99 {
		return
	}
	if mode == 99 {
		current.defaultInput = int(rule.Code[1])
		current.defaultOutput = int(rule.Code[2])
		machine.inputTier = current.defaultInput
		machine.outputTier = current.defaultOutput
		return
	}
	if mode < 1 || mode > 16 {
		return
	}
	if mode <= 8 && !machine.required(current, rule) {
		return
	}
	left, right := rule.MinimumLeft, rule.MinimumRight
	skipOnSuccess := func() {
		current.rule += machine.module.Rule(current.rule).SkipOnSuccess
	}
	switch mode {
	case 1, 3:
		literal := rule.Code[1]
		current.position = first + left
		for current.position <= current.end-right {
			if machine.tier(machine.inputTier)[current.position] != literal {
				current.position++
				continue
			}
			current.advance = 0
			machine.code, machine.pc = rule.Code, 2
			machine.skip = 1
			fired := machine.tryRule(number, current.position+1, current)
			if mode == 3 {
				if fired {
					skipOnSuccess()
					return
				}
				current.position++
			} else {
				current.position += current.advance + 1
			}
		}
	case 2, 4:
		literal := rule.Code[1]
		current.position = current.end - right + 1
		for first+left <= current.position {
			if machine.tier(machine.inputTier)[current.position] == literal {
				machine.code, machine.pc = rule.Code, 2
				machine.skip = 1
				if machine.tryRule(number, current.position+1, current) && mode == 4 {
					skipOnSuccess()
					return
				}
			}
			current.position--
		}
	case 5, 7:
		current.position = first + left
		for current.position <= current.end-right {
			current.advance = 0
			machine.skip = 0
			machine.code, machine.pc = rule.Code, 1
			fired := machine.tryRule(number, current.position, current)
			if mode == 7 {
				if fired {
					skipOnSuccess()
					return
				}
				current.position++
			} else {
				current.position += current.advance + 1
			}
		}
	case 6, 8:
		current.position = current.end - right + 1
		for first+left <= current.position {
			machine.skip = 0
			machine.code, machine.pc = rule.Code, 1
			if machine.tryRule(number, current.position, current) && mode == 8 {
				skipOnSuccess()
				return
			}
			current.position--
		}
	case 13:
		current.keepLargest = true
		current.position = first + left
		for current.position <= current.end-right {
			fired := false
			current.advance = -2
			for member := 0; member <= machine.module.Rule(current.rule).BlockCount && !fired; member++ {
				target := current.rule + member
				targetRule := machine.module.Rule(target)
				if current.histogram[targetRule.Required1] != 0 {
					machine.code, machine.pc = targetRule.Code, 1
					machine.skip = 0
					fired = machine.tryRule(target, current.position, current)
				}
			}
			if fired {
				current.position += current.advance + 1
			} else {
				current.position++
			}
		}
		current.rule += machine.module.Rule(current.rule).BlockCount
		current.keepLargest = false
	case 14, 16:
		current.position = current.end - right + 1
		anyFired := false
		for first+left <= current.position {
			if mode == 16 && anyFired {
				break
			}
			fired := false
			position := current.position
			for member := 0; member <= machine.module.Rule(current.rule).BlockCount && !fired; member++ {
				target := current.rule + member
				targetRule := machine.module.Rule(target)
				if current.histogram[targetRule.Required1] != 0 {
					machine.code, machine.pc = targetRule.Code, 1
					machine.skip = 0
					fired = machine.tryRule(target, current.position, current)
					position = current.position
					anyFired = anyFired || fired
				}
			}
			current.position = position - 1
		}
		if mode == 16 && anyFired {
			skipOnSuccess()
		} else {
			current.rule += machine.module.Rule(current.rule).BlockCount
		}
	}
}

func (machine *Machine) tryRule(number int, position int, current *frame) bool {
	if machine.checkEveryAttempt && machine.tailTop-current.end < 2 {
		machine.overflowed = true
		current.fired = false
		return false
	}
	if machine.failed {
		return false
	}
	result := false
	if matchEnd := machine.matchFocus(position, current.end); matchEnd != 0 {
		machine.focusLength = matchEnd - current.position
		machine.direction = -1
		machine.contextPosition = current.position - 1
		if machine.matchContext() {
			machine.direction = 1
			machine.contextPosition = matchEnd
			if machine.matchContext() && machine.applyAction(number, current) {
				current.fired = true
				result = true
			}
		}
	}
	if machine.dirty {
		machine.dirty = false
		machine.variables = [33]byte{}
	}
	machine.inputTier = current.defaultInput
	return result
}
