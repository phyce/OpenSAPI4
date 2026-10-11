package datapack

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
)

const (
	RuleEdge       = 0x0E
	RuleLastSymbol = 0xAF
	ruleTiers      = 5
	ruleStackSize  = 20
	ruleEnd        = 0xFF
)

type ProcedureKind int

const (
	NativeProcedure ProcedureKind = iota + 1
	GroupProcedure
	GroupTrueProcedure
	GroupFalseProcedure
)

type RuleGroup struct {
	First int
	Last  int
}

type Rule struct {
	Code          []byte
	MinimumLeft   int
	MinimumRight  int
	OutputLength  int
	SkipOnSuccess int
	BlockCount    int
	Required1     byte
	Required2     byte
}

func (rule *Rule) Mode() byte {
	return rule.Code[0]
}

type RuleSeries struct {
	Members [256]bool
	Indexes [256]byte
}

type RuleVariable struct {
	Class  int
	Series int
}

type RuleModule struct {
	Name       string
	Groups     map[int]RuleGroup
	Rules      []Rule
	Classes    [][256]bool
	Series     []RuleSeries
	SeriesMaps [][26]byte
	Variables  []RuleVariable
	Procedures map[int]ProcedureKind
}

// Rule returns rule number (from 1).
func (module *RuleModule) Rule(number int) *Rule {
	return &module.Rules[number-1]
}

type Rules struct {
	Modules map[string]*RuleModule
}

type ruleDescription struct {
	Code          string `json:"code"`
	MinimumLeft   int    `json:"minimumLeft"`
	MinimumRight  int    `json:"minimumRight"`
	OutputLength  int    `json:"outputLength"`
	SkipOnSuccess int    `json:"skipOnSuccess"`
	BlockCount    int    `json:"blockCount"`
	Required1     int    `json:"required1"`
	Required2     int    `json:"required2"`
}

type ruleGroupDescription struct {
	Group int `json:"group"`
	First int `json:"first"`
	Last  int `json:"last"`
}

type ruleVariableDescription struct {
	Class  int `json:"class"`
	Series int `json:"series"`
}

type procedureDescription struct {
	Number int    `json:"number"`
	Kind   string `json:"kind"`
}

type ruleModuleDescription struct {
	Name       string                    `json:"name"`
	Groups     []ruleGroupDescription    `json:"groups"`
	Rules      []ruleDescription         `json:"rules"`
	Classes    [][]int                   `json:"classes"`
	Series     [][][2]int                `json:"series"`
	SeriesMaps [][]int                   `json:"seriesMaps"`
	Variables  []ruleVariableDescription `json:"variables"`
	Procedures []procedureDescription    `json:"procedures"`
}

type rulesDescription struct {
	Modules []ruleModuleDescription `json:"modules"`
}

var procedureKinds = map[string]ProcedureKind{
	"native":     NativeProcedure,
	"group":      GroupProcedure,
	"groupTrue":  GroupTrueProcedure,
	"groupFalse": GroupFalseProcedure,
}

func (pack *Pack) LoadRules() (*Rules, error) {
	return load(pack, "rules.json", parseRules)
}

func parseRules(data []byte) (*Rules, error) {
	var description rulesDescription
	if err := json.Unmarshal(data, &description); err != nil {
		return nil, err
	}
	if len(description.Modules) == 0 {
		return nil, fmt.Errorf("no rule modules")
	}
	rules := &Rules{Modules: map[string]*RuleModule{}}
	for index := range description.Modules {
		module, err := parseRuleModule(&description.Modules[index])
		if err != nil {
			return nil, err
		}
		if _, found := rules.Modules[module.Name]; found {
			return nil, fmt.Errorf("rule module %s appears twice", module.Name)
		}
		rules.Modules[module.Name] = module
	}
	return rules, nil
}

func parseRuleModule(description *ruleModuleDescription) (*RuleModule, error) {
	module := &RuleModule{
		Name:       description.Name,
		Groups:     map[int]RuleGroup{},
		Procedures: map[int]ProcedureKind{},
	}
	if module.Name == "" {
		return nil, fmt.Errorf("rule module without a name")
	}
	for _, rule := range description.Rules {
		code, err := hex.DecodeString(rule.Code)
		if err != nil || len(code) == 0 {
			return nil, fmt.Errorf("module %s rule %d: code %q", module.Name, len(module.Rules)+1, rule.Code)
		}
		counts := []int{rule.MinimumLeft, rule.MinimumRight, rule.OutputLength, rule.SkipOnSuccess, rule.BlockCount}
		if err := checkValues("rule fields", counts, len(counts), 0, math.MaxUint8); err != nil {
			return nil, fmt.Errorf("module %s rule %d: %w", module.Name, len(module.Rules)+1, err)
		}
		required := []int{rule.Required1, rule.Required2}
		if err := checkValues("required symbols", required, len(required), RuleEdge, RuleLastSymbol); err != nil {
			return nil, fmt.Errorf("module %s rule %d: %w", module.Name, len(module.Rules)+1, err)
		}
		module.Rules = append(module.Rules, Rule{
			Code:          code,
			MinimumLeft:   rule.MinimumLeft,
			MinimumRight:  rule.MinimumRight,
			OutputLength:  rule.OutputLength,
			SkipOnSuccess: rule.SkipOnSuccess,
			BlockCount:    rule.BlockCount,
			Required1:     byte(rule.Required1),
			Required2:     byte(rule.Required2),
		})
	}
	if len(module.Rules) == 0 || len(module.Rules) > math.MaxUint16 {
		return nil, fmt.Errorf("module %s has %d rules", module.Name, len(module.Rules))
	}
	for _, group := range description.Groups {
		if _, found := module.Groups[group.Group]; found || group.Group < 1 || group.Group > math.MaxUint8 ||
			group.First < 1 || group.First > group.Last || group.Last > len(module.Rules) {
			return nil, fmt.Errorf("module %s group %d: rules %d to %d", module.Name, group.Group, group.First, group.Last)
		}
		module.Groups[group.Group] = RuleGroup{First: group.First, Last: group.Last}
	}
	for number, members := range description.Classes {
		var class [256]bool
		for _, symbol := range members {
			if symbol <= RuleEdge || symbol > RuleLastSymbol || class[symbol] {
				return nil, fmt.Errorf("module %s class %d: symbol %d", module.Name, number+1, symbol)
			}
			class[symbol] = true
		}
		module.Classes = append(module.Classes, class)
	}
	for number, members := range description.Series {
		var series RuleSeries
		for _, member := range members {
			symbol, index := member[0], member[1]
			if symbol <= RuleEdge || symbol > RuleLastSymbol || series.Members[symbol] || index < 1 || index > 26 {
				return nil, fmt.Errorf("module %s series %d: symbol %d, index %d", module.Name, number+1, symbol, index)
			}
			series.Members[symbol] = true
			series.Indexes[symbol] = byte(index)
		}
		module.Series = append(module.Series, series)
	}
	for number, values := range description.SeriesMaps {
		if err := checkValues("series map", values, 26, 0, math.MaxUint8); err != nil {
			return nil, fmt.Errorf("module %s map %d: %w", module.Name, number+1, err)
		}
		var seriesMap [26]byte
		for index, value := range values {
			seriesMap[index] = byte(value)
		}
		module.SeriesMaps = append(module.SeriesMaps, seriesMap)
	}
	for number, variable := range description.Variables {
		classVariable := variable.Class >= 1 && variable.Class <= len(module.Classes) && variable.Series == 0
		seriesVariable := variable.Series >= 1 && variable.Series <= len(module.Series) && variable.Class == 0
		if !classVariable && !seriesVariable {
			return nil, fmt.Errorf("module %s variable %d: class %d, series %d", module.Name, number+1, variable.Class, variable.Series)
		}
		module.Variables = append(module.Variables, RuleVariable{Class: variable.Class, Series: variable.Series})
	}
	for _, procedure := range description.Procedures {
		kind, known := procedureKinds[procedure.Kind]
		if _, found := module.Procedures[procedure.Number]; found || !known || procedure.Number < 3 || procedure.Number > math.MaxUint8 {
			return nil, fmt.Errorf("module %s procedure %d of kind %q", module.Name, procedure.Number, procedure.Kind)
		}
		if _, isGroup := module.Groups[procedure.Number]; kind != NativeProcedure && !isGroup {
			return nil, fmt.Errorf("module %s procedure %d runs a group that doesn't exist", module.Name, procedure.Number)
		}
		module.Procedures[procedure.Number] = kind
	}
	if len(module.Variables) > 32 {
		return nil, fmt.Errorf("module %s has %d variables", module.Name, len(module.Variables))
	}
	for number := 1; number <= len(module.Rules); number++ {
		parser := &ruleParser{module: module, number: number, code: module.Rule(number).Code}
		if err := parser.parse(); err != nil {
			return nil, err
		}
	}
	if err := module.validateBlocks(); err != nil {
		return nil, err
	}
	return module, nil
}

var (
	focusOperations   = operationSet(0xEA, 0xEB, 0xEC, 0xED, 0xEE, 0xEF, 0xF0, 0xF1, 0xF2, 0xF3, 0xF4, 0xF7, 0xF8, 0xF9, 0xFA, 0xFB, 0xFC, 0xFD)
	contextOperations = operationSet(0xEA, 0xEC, 0xED, 0xEE, 0xEF, 0xF0, 0xF1, 0xF2, 0xF3, 0xF4, 0xF5, 0xF6, 0xF7, 0xF9, 0xFA, 0xFB, 0xFC, 0xFD)
	testOperations    = operationSet(0xEA, 0xEE, 0xEF, 0xF0, 0xF1, 0xF2, 0xF3, 0xF9, 0xFA, 0xFB, 0xFC, 0xFD)
	countOperations   = operationSet(0xF2, 0xF3, 0xF9, 0xFA, 0xFB, 0xFC)
	forwardModes      = operationSet(1, 3, 5, 7, 13)
	ruleModes         = operationSet(1, 2, 3, 4, 5, 6, 7, 8, 13, 14, 16, 99, 100)
)

func operationSet(values ...byte) (set [256]bool) {
	for _, value := range values {
		set[value] = true
	}
	return set
}

func IsBlockMode(mode byte) bool {
	return mode == 13 || mode == 14 || mode == 16
}

type ruleParser struct {
	module   *RuleModule
	number   int
	code     []byte
	position int

	focusSeries []byte
	bound       [33]bool
	call        int
	stop        bool
}

func (parser *ruleParser) fail(format string, arguments ...any) error {
	return fmt.Errorf("module %s rule %d: %s", parser.module.Name, parser.number, fmt.Sprintf(format, arguments...))
}

func (parser *ruleParser) peek() (byte, error) {
	if parser.position >= len(parser.code) {
		return 0, parser.fail("ends early")
	}
	return parser.code[parser.position], nil
}

func (parser *ruleParser) next() (byte, error) {
	value, err := parser.peek()
	parser.position++
	return value, err
}

func (parser *ruleParser) operand(operation byte, value byte) error {
	limit := 0
	switch operation {
	case 0xEA, 0xFD:
		limit = len(parser.module.Variables)
	case 0xEF, 0xF1:
		limit = ruleTiers
	case 0xF8, 0xF9, 0xFA:
		limit = len(parser.module.Series)
	case 0xFB, 0xFC:
		limit = len(parser.module.Classes)
	default:
		return nil
	}
	if value < 1 || int(value) > limit {
		return parser.fail("operand %d of 0x%02x", value, operation)
	}
	return nil
}

func (parser *ruleParser) parse() error {
	mode, err := parser.next()
	if err != nil {
		return err
	}
	if !ruleModes[mode] {
		return parser.fail("mode %d", mode)
	}
	switch mode {
	case 99:
		for _, tier := range parser.code[1:min(3, len(parser.code))] {
			if tier < 1 || tier > ruleTiers {
				return parser.fail("directive tier %d", tier)
			}
		}
		parser.position = 3
		return parser.expectEnds(4)
	case 100:
		return parser.expectEnds(4)
	}
	focusStart := parser.position
	if err := parser.parseFocus(); err != nil {
		return err
	}
	if mode <= 4 && (parser.code[focusStart] >= 0xE9 || parser.code[focusStart] == RuleEdge) {
		return parser.fail("scan mode %d without a first literal", mode)
	}
	for range 2 {
		if err := parser.parseContext(); err != nil {
			return err
		}
	}
	if err := parser.parseAction(); err != nil {
		return err
	}
	if parser.position != len(parser.code) {
		return parser.fail("%d bytes after the action", len(parser.code)-parser.position)
	}
	if parser.call > 2 {
		if _, defined := parser.module.Procedures[parser.call]; !defined {
			return parser.fail("calls procedure %d, which doesn't exist", parser.call)
		}
	}
	return nil
}

func (parser *ruleParser) expectEnds(count int) error {
	for range count {
		value, err := parser.next()
		if err != nil {
			return err
		}
		if value != ruleEnd {
			return parser.fail("0x%02x where a section ends", value)
		}
	}
	if parser.position != len(parser.code) {
		return parser.fail("%d bytes after the rule", len(parser.code)-parser.position)
	}
	return nil
}

func (parser *ruleParser) parseTests(terminator byte) error {
	for {
		operation, err := parser.next()
		if err != nil {
			return err
		}
		if operation == terminator {
			return nil
		}
		if !testOperations[operation] {
			return parser.fail("test 0x%02x", operation)
		}
		value, err := parser.next()
		if err != nil {
			return err
		}
		if value == terminator {
			return parser.fail("test operand equals its list's terminator")
		}
		if err := parser.operand(operation, value); err != nil {
			return err
		}
	}
}

// repeatMaximum reads an f4 maximum. The interpreter skips an f4's test list only after matching it at least
// once, so a maximum of 0 would leave it reading the list as elements.
func (parser *ruleParser) repeatMaximum() error {
	maximum, err := parser.next()
	if err != nil {
		return err
	}
	if maximum == 0 {
		return parser.fail("f4 with a maximum of 0")
	}
	return nil
}

func (parser *ruleParser) parseCount() error {
	operands := make([]byte, 5)
	for index := range operands {
		value, err := parser.next()
		if err != nil {
			return err
		}
		operands[index] = value
	}
	if operands[4] != 0xED {
		return parser.fail("count not closed")
	}
	if countOperations[operands[2]] {
		return parser.operand(operands[2], operands[3])
	}
	return nil
}

func (parser *ruleParser) parseFocus() error {
	for {
		operation, err := parser.next()
		if err != nil {
			return err
		}
		switch {
		case operation == ruleEnd:
			return nil
		case !focusOperations[operation]:
			if operation == RuleEdge {
				return parser.fail("matches the edge")
			}
		case operation == 0xEB || operation == 0xEC:
		case operation == 0xED:
			if err := parser.parseCount(); err != nil {
				return err
			}
		case operation == 0xF4 || operation == 0xF7:
			minimum := byte(1)
			if operation == 0xF4 {
				if minimum, err = parser.next(); err != nil {
					return err
				}
				if err := parser.repeatMaximum(); err != nil {
					return err
				}
			}
			start := parser.position
			if err := parser.parseTests(operation); err != nil {
				return err
			}
			if minimum >= 1 {
				for position := start; position+1 < parser.position; position += 2 {
					if parser.code[position] == 0xFD {
						parser.bound[parser.code[position+1]] = true
					}
				}
			}
		default:
			value, err := parser.next()
			if err != nil {
				return err
			}
			if err := parser.operand(operation, value); err != nil {
				return err
			}
			switch operation {
			case 0xF8:
				parser.focusSeries = append(parser.focusSeries, value)
			case 0xFD:
				parser.bound[value] = true
			}
		}
	}
}

type markerUse struct {
	position int
	marker   byte
	operand  byte
}

func (parser *ruleParser) parseContext() error {
	start := parser.position
	starts := map[int]bool{}
	var markers []markerUse
	var groups []byte
	pushes := 0
	for {
		starts[parser.position] = true
		position := parser.position
		operation, err := parser.next()
		if err != nil {
			return err
		}
		switch {
		case operation == ruleEnd:
			if len(groups) != 0 {
				return parser.fail("a context group is not closed")
			}
			if pushes > ruleStackSize {
				return parser.fail("%d backtracking points in one context", pushes)
			}
			return parser.checkMarkers(start, position, starts, markers)
		case !contextOperations[operation]:
			if operation == RuleEdge {
				return parser.fail("matches the edge")
			}
		case operation == 0xEC:
		case operation == 0xED:
			if err := parser.parseCount(); err != nil {
				return err
			}
		case operation == 0xF4 || operation == 0xF7:
			if _, err := parser.next(); err != nil {
				return err
			}
			if operation == 0xF4 {
				if err := parser.repeatMaximum(); err != nil {
					return err
				}
			} else if _, err := parser.next(); err != nil {
				return err
			}
			if err := parser.parseTests(operation); err != nil {
				return err
			}
			if operation == 0xF7 {
				pushes++
			}
		case operation == 0xF5 || operation == 0xF6:
			value, err := parser.next()
			if err != nil {
				return err
			}
			markers = append(markers, markerUse{position: position, marker: operation, operand: value})
			switch {
			case value == 0:
				if len(groups) == 0 || groups[len(groups)-1] != operation {
					return parser.fail("a context group closes before it opens")
				}
				groups = groups[:len(groups)-1]
			case value == 1 || operation == 0xF5:
				groups = append(groups, operation)
				pushes++
			case len(groups) == 0 || groups[len(groups)-1] != 0xF6:
				groups = append(groups, operation)
			}
		default:
			value, err := parser.next()
			if err != nil {
				return err
			}
			if err := parser.operand(operation, value); err != nil {
				return err
			}
			if operation == 0xFD && len(groups) == 0 {
				parser.bound[value] = true
			}
		}
	}
}

// checkMarkers replays every marker search the interpreter can make in a context: each must land on an
// element of the context with the marker it looks for, so the interpreter never leaves the section.
func (parser *ruleParser) checkMarkers(start int, end int, starts map[int]bool, markers []markerUse) error {
	lands := func(position int, marker byte, operand byte) bool {
		return position >= start && position+1 < end && starts[position] &&
			parser.code[position] == marker && parser.code[position+1] == operand
	}
	for _, use := range markers {
		switch {
		case use.operand == 0:
		case use.marker == 0xF6 && use.operand == 1:
			for count := 2; count <= math.MaxUint8; count++ {
				found, landed := FindRuleMarker(parser.code, use.position+2, 0xF6, byte(count))
				if !found {
					if !lands(landed, 0xF6, 0) {
						return parser.fail("the search for alternative %d leaves the context", count)
					}
					break
				}
				if !lands(landed, 0xF6, byte(count)) {
					return parser.fail("the search for alternative %d leaves the context", count)
				}
			}
		default:
			if _, landed := FindRuleMarker(parser.code, use.position+1, use.marker, 0); !lands(landed, use.marker, 0) {
				return parser.fail("the search for the end of a group leaves the context")
			}
		}
	}
	return nil
}

func (parser *ruleParser) parseAction() error {
	first, err := parser.next()
	if err != nil {
		return err
	}
	if first == 0xFE {
		number, err := parser.next()
		if err != nil {
			return err
		}
		if number == 0 {
			return parser.fail("calls procedure 0")
		}
		parser.call = int(number)
		parser.stop = number == 2
		return parser.expectSectionEnd()
	}
	if first != 0xE9 {
		parser.position--
	}
	written := 0
	for {
		operation, err := parser.next()
		if err != nil {
			return err
		}
		switch operation {
		case ruleEnd:
			if written != parser.module.Rule(parser.number).OutputLength {
				return parser.fail("writes %d symbols, output length %d", written, parser.module.Rule(parser.number).OutputLength)
			}
			return nil
		case 0xEC:
		case 0xEE, 0xEF, 0xF0, 0xF1, 0xF8, 0xFD:
			value, err := parser.next()
			if err != nil {
				return err
			}
			switch operation {
			case 0xEF, 0xF1:
				if err := parser.operand(operation, value); err != nil {
					return err
				}
			case 0xF8:
				if err := parser.checkSeriesMap(value); err != nil {
					return err
				}
				written++
			case 0xFD:
				if value < 1 || int(value) > len(parser.module.Variables) || !parser.bound[value] {
					return parser.fail("writes variable %d, which may be unbound", value)
				}
				written++
			}
		default:
			if operation <= RuleEdge || operation > RuleLastSymbol {
				return parser.fail("writes 0x%02x", operation)
			}
			written++
		}
	}
}

func (parser *ruleParser) checkSeriesMap(seriesMap byte) error {
	if len(parser.focusSeries) != 1 || seriesMap < 1 || int(seriesMap) > len(parser.module.SeriesMaps) {
		return parser.fail("writes series map %d without one series in its focus", seriesMap)
	}
	series := &parser.module.Series[parser.focusSeries[0]-1]
	for symbol, member := range series.Members {
		if !member {
			continue
		}
		if value := parser.module.SeriesMaps[seriesMap-1][series.Indexes[symbol]-1]; value <= RuleEdge || value > RuleLastSymbol {
			return parser.fail("series map %d writes 0x%02x", seriesMap, value)
		}
	}
	return nil
}

func (parser *ruleParser) expectSectionEnd() error {
	value, err := parser.next()
	if err != nil {
		return err
	}
	if value != ruleEnd {
		return parser.fail("0x%02x after the action's call", value)
	}
	return nil
}

// validateBlocks follows every group the way its rule loop can run: a block must end at a rule that exists,
// and STOP may end only a forward scan.
func (module *RuleModule) validateBlocks() error {
	heads := map[int]bool{}
	for _, group := range module.Groups {
		pending := []int{group.First}
		for len(pending) > 0 {
			number := pending[len(pending)-1]
			pending = pending[:len(pending)-1]
			if number > group.Last || heads[number] {
				continue
			}
			heads[number] = true
			rule := module.Rule(number)
			mode := rule.Mode()
			switch {
			case mode == 13 || mode == 14:
				pending = append(pending, number+rule.BlockCount+1)
			case mode == 16:
				pending = append(pending, number+rule.BlockCount+1, number+rule.SkipOnSuccess+1)
			case mode == 3 || mode == 4 || mode == 7 || mode == 8:
				pending = append(pending, number+1, number+rule.SkipOnSuccess+1)
			default:
				pending = append(pending, number+1)
			}
			members := []int{number}
			if IsBlockMode(mode) {
				if number+rule.BlockCount > len(module.Rules) {
					return fmt.Errorf("module %s rule %d: block past the last rule", module.Name, number)
				}
				members = members[:0]
				for member := number; member <= number+rule.BlockCount; member++ {
					members = append(members, member)
				}
			}
			for _, member := range members {
				if !forwardModes[mode] && module.stops(member) {
					return fmt.Errorf("module %s rule %d stops a backward scan", module.Name, member)
				}
			}
		}
	}
	return nil
}

func (module *RuleModule) stops(number int) bool {
	code := module.Rule(number).Code
	if code[0] == 99 || code[0] == 100 {
		return false
	}
	parser := &ruleParser{module: module, number: number, code: code}
	if parser.parse() != nil {
		return false
	}
	return parser.stop
}

// FindRuleMarker is the interpreter's byte-wise search for the marker pair (marker, number) from position:
// a nested group is skipped by a search for its closing pair, and the search resumes three bytes after that.
// It reports whether the pair was found and where it stopped: at the pair, or at the closing (marker, 0)
// pair it reached first. A search that runs off the end stops at the end.
func FindRuleMarker(code []byte, position int, marker byte, number byte) (bool, int) {
	at := func(index int) byte {
		if index < 0 || index >= len(code) {
			return ruleEnd
		}
		return code[index]
	}
	current, following := at(position), at(position+1)
	after := position + 1
	for position < len(code) {
		nested := false
		if current == marker {
			if following == number {
				return true, position
			}
			nested = following == 1
		}
		if !nested && marker == 0xF5 && current == 0xF5 && following != 0 {
			nested = true
		}
		if nested {
			_, position = FindRuleMarker(code, after, marker, 0)
			position += 2
		}
		next := position + 1
		current = at(next)
		if current == marker && at(position+2) == 0 {
			return false, next
		}
		after = position + 2
		following = at(after)
		position = next
	}
	return false, len(code)
}
