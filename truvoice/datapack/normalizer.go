package datapack

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
)

const (
	// NormalizerRegisters are the normalizer's registers, NormalizerFlags the flags its flag condition tests: the
	// N flags, then the A flags.
	NormalizerRegisters = 12
	NormalizerFlags     = 32

	// TransferBit marks a goto or a call in a run of actions, CallBit a call; TargetBits are the place they go to
	// in the program, with the byte after.
	TransferBit = 0x80
	CallBit     = 0x40
	TargetBits  = 0x3fff

	returnAction = 0x13
)

// Normalizer is the normalizer's program (normalizer.md §2): its bytes, the string table its operands point into,
// a string ending at the first NUL from where they point, the characters its phonetic-symbol condition takes,
// and its word lists.
type Normalizer struct {
	Program     []byte
	Strings     []byte
	Punctuation string
	Lists       [][]ListEntry
}

// A ListEntry is a word of a list and the replacement a match of it gives.
type ListEntry struct {
	Match, Replacement string
}

type normalizerDescription struct {
	Program     []string      `json:"program"`
	Strings     []string      `json:"strings"`
	Punctuation string        `json:"punctuation"`
	Lists       [][][2]string `json:"lists"`
}

// The kinds of the operands of the program's conditions and actions.
type operandKind int

const (
	valueOperand operandKind = iota
	registerOperand
	listOperand
	flagOperand
	stringOperand
	classOperand
)

// The operands of each condition and each action, by their byte. The original's machine has an action 15, which
// the program doesn't use and the port leaves out (normalizer.md §5).
var conditionOperands = [...][]operandKind{
	{}, {}, {flagOperand}, {}, {}, {}, {}, {}, {classOperand}, {registerOperand, valueOperand},
	{registerOperand, valueOperand}, {valueOperand}, {listOperand}, {}, {}, {}, {}, {stringOperand}, {listOperand}}

var actionOperands = [...][]operandKind{
	{stringOperand}, {stringOperand}, {}, {}, {}, {}, {registerOperand, valueOperand}, {registerOperand},
	{registerOperand}, {}, {}, {}, {}, {}, {valueOperand}, nil, {listOperand}, {}, {}, {}, {}, {}, {}}

// classes are the classes the condition ahead tests (normalizer.md §2.2).
var classes = map[int]bool{4: true, 5: true, 6: true, 7: true, 0xe: true, 0x13: true}

func (pack *Pack) LoadNormalizer() (*Normalizer, error) {
	return load(pack, "normalizer.json", parseNormalizer)
}

func parseNormalizer(data []byte) (*Normalizer, error) {
	var description normalizerDescription
	if err := json.Unmarshal(data, &description); err != nil {
		return nil, err
	}
	normalizer := &Normalizer{Punctuation: description.Punctuation}
	starts := map[int]bool{}
	for index, text := range description.Strings {
		if err := checkProgramString(fmt.Sprintf("strings[%d]", index), text); err != nil {
			return nil, err
		}
		starts[len(normalizer.Strings)] = true
		normalizer.Strings = append(append(normalizer.Strings, text...), 0)
	}
	if err := checkProgramString("punctuation", description.Punctuation); err != nil {
		return nil, err
	}
	for index, list := range description.Lists {
		if len(list) == 0 {
			return nil, fmt.Errorf("lists[%d] is empty", index)
		}
		entries := make([]ListEntry, len(list))
		for position, entry := range list {
			name := fmt.Sprintf("lists[%d][%d]", index, position)
			if entry[0] == "" {
				return nil, fmt.Errorf("%s has an empty word", name)
			}
			if err := checkProgramString(name, entry[0]+entry[1]); err != nil {
				return nil, err
			}
			entries[position] = ListEntry{Match: entry[0], Replacement: entry[1]}
		}
		normalizer.Lists = append(normalizer.Lists, entries)
	}
	blocks := map[int]bool{}
	var targets [][2]int
	for index, text := range description.Program {
		block, err := hex.DecodeString(text)
		if err != nil {
			return nil, fmt.Errorf("program[%d]: %w", index, err)
		}
		blocks[len(normalizer.Program)] = true
		found, err := normalizer.checkBlock(block, starts, index == len(description.Program)-1)
		if err != nil {
			return nil, fmt.Errorf("program[%d]: %w", index, err)
		}
		for _, target := range found {
			targets = append(targets, [2]int{index, target})
		}
		normalizer.Program = append(normalizer.Program, block...)
	}
	if len(normalizer.Program) == 0 {
		return nil, fmt.Errorf("program is empty")
	}
	for _, target := range targets {
		if !blocks[target[1]] {
			return nil, fmt.Errorf("program[%d] goes to %#x, which starts no block", target[0], target[1])
		}
	}
	return normalizer, nil
}

// checkBlock checks that a block of the program at an offset is a condition with its operands, a branch byte and
// its two runs of whole actions, that its operands are in range, and, for the last block, that neither run is
// empty or ends but in a goto or a return. It returns the places its gotos and calls go to.
func (normalizer *Normalizer) checkBlock(block []byte, starts map[int]bool, last bool) ([]int, error) {
	if len(block) == 0 || int(block[0]) >= len(conditionOperands) {
		return nil, fmt.Errorf("doesn't start with a condition")
	}
	place, err := normalizer.checkOperands(block, 1, conditionOperands[block[0]], starts)
	if err != nil {
		return nil, err
	}
	if place >= len(block) {
		return nil, fmt.Errorf("has no branch byte")
	}
	branch := block[place]
	place++
	var targets []int
	for _, length := range []int{int(branch >> 4), int(branch & 15)} {
		end := place + length
		final := -1
		for place < end {
			final = place
			action := block[place]
			if action&TransferBit != 0 {
				if place+1 >= len(block) {
					return nil, fmt.Errorf("has a goto at %d without its second byte", place)
				}
				targets = append(targets, (int(action)<<8|int(block[place+1]))&TargetBits)
				place += 2
				continue
			}
			if int(action) >= len(actionOperands) || actionOperands[action] == nil {
				return nil, fmt.Errorf("has an unknown action %d at %d", action, place)
			}
			if place, err = normalizer.checkOperands(block, place+1, actionOperands[action], starts); err != nil {
				return nil, err
			}
		}
		if place != end {
			return nil, fmt.Errorf("has a run of actions ending at %d, not %d", place, end)
		}
		if last && (final < 0 || !isGoto(block[final]) && block[final] != returnAction) {
			return nil, fmt.Errorf("is the last and has a run that doesn't end in a goto or a return")
		}
	}
	if place != len(block) {
		return nil, fmt.Errorf("has %d bytes after its actions", len(block)-place)
	}
	return targets, nil
}

func isGoto(action byte) bool {
	return action&TransferBit != 0 && action&CallBit == 0
}

// checkOperands checks the operands of a condition or an action from place, and returns the place after them.
func (normalizer *Normalizer) checkOperands(block []byte, place int, kinds []operandKind, starts map[int]bool) (int, error) {
	for _, kind := range kinds {
		size := 1
		if kind == stringOperand {
			size = 2
		}
		if place+size > len(block) {
			return place, fmt.Errorf("ends inside an operand")
		}
		value := int(block[place])
		switch kind {
		case registerOperand:
			if value >= NormalizerRegisters {
				return place, fmt.Errorf("has register %d, expected below %d", value, NormalizerRegisters)
			}
		case listOperand:
			if value >= len(normalizer.Lists) {
				return place, fmt.Errorf("has list %d, expected below %d", value, len(normalizer.Lists))
			}
		case flagOperand:
			if value >= NormalizerFlags {
				return place, fmt.Errorf("has flag %d, expected below %d", value, NormalizerFlags)
			}
		case classOperand:
			if !classes[value] {
				return place, fmt.Errorf("has class %d, expected 4 to 7, 14 or 19", value)
			}
		case stringOperand:
			if value = value<<8 | int(block[place+1]); !starts[value] {
				return place, fmt.Errorf("has string offset %d, which starts no string", value)
			}
		}
		place += size
	}
	return place, nil
}

// String returns the string at an offset of the string table.
func (normalizer *Normalizer) String(offset int) []byte {
	table := normalizer.Strings[offset:]
	return table[:slices.Index(table, 0)]
}

// checkProgramString reports an error when a string of the normalizer's isn't printable ASCII.
func checkProgramString(name, text string) error {
	for position := range len(text) {
		if text[position] < ' ' || text[position] > '~' {
			return fmt.Errorf("%s has %q, not printable ASCII", name, text)
		}
	}
	return nil
}
