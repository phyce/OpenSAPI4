// Package threadstate reads the TruVoice golden data that records calls of the original's functions with the
// speaking thread's state around them, and maps that state to the Go pipeline and stages and back, for the
// tests.
package threadstate

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/internal/conformance"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
)

// Size is the thread state's size.
const Size = 0x9184

// The places of the pipeline's state in the thread state.
const (
	SavedRate      = 0x1ae4
	NumberWord     = 0x1c1c
	ShortPhrase    = 0x1d54
	Levels         = 0x1f98
	Engine         = 0x20f4
	Pitch          = 0x20f8
	Speed          = 0x20fc
	Volume         = 0x2100
	Character      = 0x2108
	SampleRate     = 0x210c
	OutputVoice    = 0x210e
	ResetRequested = 0x211d
	ParsedReset    = 0x211e
	ResetValue     = 0x2120
	VisualEvents   = 0x2130
	PostedSpace    = 0x2132
	Current        = 0x213c
	Free           = 0x2140
	Cursors        = 0x2144
	CursorSize     = 0x44
	ListLast       = 0x22a0
	ListFirst      = 0x22c0
	FreeLast       = 0x22e8
	FreeFirst      = 0x2308
	Nodes          = 0x2328
	NodeSize       = 0x20
	NodeData       = 8
	NodeText       = 0x1c
)

// cursorValuesOffset is where a cursor's values start, after its seven pointers.
const cursorValuesOffset = 0x1c

// A Run is a run of bytes that changed, at an offset.
type Run struct {
	Offset int
	Data   []byte
}

func (run *Run) UnmarshalJSON(data []byte) error {
	var pair [2]json.RawMessage
	if err := json.Unmarshal(data, &pair); err != nil {
		return err
	}
	var text string
	if err := json.Unmarshal(pair[0], &run.Offset); err != nil {
		return err
	}
	if err := json.Unmarshal(pair[1], &text); err != nil {
		return err
	}
	var err error
	run.Data, err = hex.DecodeString(text)
	return err
}

// A Record is a recorded call of one of the original's functions or its return, with the thread state's runs
// that changed since the previous record, and the global variables' when they are recorded.
type Record struct {
	Kind      string   `json:"kind"`
	Function  string   `json:"function"`
	Site      string   `json:"site"`
	ECX       uint32   `json:"ecx"`
	EDX       uint32   `json:"edx"`
	Arguments []uint32 `json:"arguments"`
	EAX       uint32   `json:"eax"`
	Base      uint32   `json:"base"`
	Diff      []Run    `json:"diff"`
	Globals   []Run    `json:"globals"`
}

// A Case is a recorded run, with the words and pronunciations its user lexicon was given before its texts.
type Case struct {
	Name    string      `json:"name"`
	Lexicon [][2]string `json:"lexicon"`
	Records []Record    `json:"records"`
}

// State is the thread state and the global variables as the records bring them.
type State struct {
	Bytes   []byte
	Globals []byte
	Base    uint32
}

func NewState() *State {
	return &State{Bytes: make([]byte, Size)}
}

// Apply brings the state to a record: a record with a base starts a new thread state.
func (state *State) Apply(record *Record) {
	if record.Base != 0 {
		state.Base = record.Base
		clear(state.Bytes)
	}
	for _, run := range record.Diff {
		copy(state.Bytes[run.Offset:], run.Data)
	}
	for _, run := range record.Globals {
		if end := run.Offset + len(run.Data); end > len(state.Globals) {
			state.Globals = append(state.Globals, make([]byte, end-len(state.Globals))...)
		}
		copy(state.Globals[run.Offset:], run.Data)
	}
}

// A Call is a recorded call of one of the original's functions, with its return and the thread state and global
// variables at both.
type Call struct {
	Case                        *Case
	Record, Result              *Record
	Before, After               []byte
	GlobalsBefore, GlobalsAfter []byte
	Base                        uint32
}

func (call *Call) Argument(index int) int32 {
	return int32(call.Record.Arguments[index])
}

type Replayer func(call *Call) error

// Replay hands each recorded call of the functions given, in the golden file named, to their replayer, reports
// the calls that differ (a few per function) and returns how many calls each replayed. The cases are replayed in
// parallel.
func Replay(t *testing.T, file string, replayers map[string]Replayer) map[string]int {
	t.Helper()
	var lock sync.Mutex
	counts := map[string]int{}
	failures := map[string]int{}
	cases := make(chan Case)
	var workers sync.WaitGroup
	for range runtime.GOMAXPROCS(0) {
		workers.Go(func() {
			for golden := range cases {
				caseCounts, caseFailures, messages := replayCase(golden, replayers)
				lock.Lock()
				for function, count := range caseCounts {
					counts[function] += count
				}
				for function, count := range caseFailures {
					for _, message := range messages[function] {
						if failures[function] < 5 {
							t.Error(message)
						}
						failures[function]++
					}
					failures[function] += count - len(messages[function])
				}
				lock.Unlock()
			}
		})
	}
	conformance.EachLine(t, func(golden Case) { cases <- golden }, file)
	close(cases)
	workers.Wait()
	for function, count := range failures {
		t.Errorf("%s: %d of %d calls differ", function, count, counts[function])
	}
	return counts
}

func replayCase(golden Case, replayers map[string]Replayer) (map[string]int, map[string]int, map[string][]string) {
	counts := map[string]int{}
	failures := map[string]int{}
	messages := map[string][]string{}
	state := NewState()
	type open struct {
		call     *Call
		replayed bool
	}
	var stack []open
	for index := range golden.Records {
		record := &golden.Records[index]
		if record.Kind != "call" && record.Kind != "return" {
			continue
		}
		state.Apply(record)
		if record.Kind == "call" {
			entry := open{call: &Call{Case: &golden, Record: record, Base: state.Base}, replayed: replayers[record.Function] != nil}
			if entry.replayed {
				entry.call.Before = slices.Clone(state.Bytes)
				entry.call.GlobalsBefore = slices.Clone(state.Globals)
			}
			stack = append(stack, entry)
			continue
		}
		if len(stack) == 0 || stack[len(stack)-1].call.Record.Function != record.Function {
			messages["replay"] = append(messages["replay"], fmt.Sprintf("%s: record %d returns from %s, which isn't the call open", golden.Name, index, record.Function))
			failures["replay"]++
			return counts, failures, messages
		}
		entry := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		entry.call.Result = record
		if !entry.replayed {
			continue
		}
		entry.call.After = slices.Clone(state.Bytes)
		entry.call.GlobalsAfter = slices.Clone(state.Globals)
		counts[record.Function]++
		if err := replayers[record.Function](entry.call); err != nil {
			failures[record.Function]++
			if len(messages[record.Function]) < 5 {
				messages[record.Function] = append(messages[record.Function], fmt.Sprintf("%s: %s called at %s (record %d): %v", golden.Name, record.Function, entry.call.Record.Site, index, err))
			}
		}
	}
	return counts, failures, messages
}

func Dword(state []byte, offset int) uint32 {
	return binary.LittleEndian.Uint32(state[offset:])
}

func Bit(state []byte, offset, slot int) bool {
	return state[offset+slot>>3]&(1<<(slot&7)) != 0
}

func SetBit(state []byte, offset, slot int, value bool) {
	if value {
		state[offset+slot>>3] |= 1 << (slot & 7)
	} else {
		state[offset+slot>>3] &^= 1 << (slot & 7)
	}
}

// NodeOffsets are the state offsets of the pipeline's nodes: the pool's, then the sentinels'.
var NodeOffsets = sync.OnceValue(func() []int {
	offsets := make([]int, pipeline.NodeCount, pipeline.NodeCount+4)
	for index := range offsets {
		offsets[index] = Nodes + NodeSize*index
	}
	return append(offsets, ListFirst, ListLast, FreeFirst, FreeLast)
})

// nodeIndex returns the place among NodeOffsets of the node at a state offset, or -1.
func nodeIndex(offset int) int {
	if offset >= Nodes && offset < Nodes+pipeline.NodeCount*NodeSize && (offset-Nodes)%NodeSize == 0 {
		return (offset - Nodes) / NodeSize
	}
	if index := slices.Index(NodeOffsets()[pipeline.NodeCount:], offset); index >= 0 {
		return pipeline.NodeCount + index
	}
	return -1
}

// A Mapping maps a thread state at a base address to a Go pipeline. Text returns the text a node's text pointer
// stands for; without it, a node with a text gets a new one.
type Mapping struct {
	Pipeline *pipeline.Pipeline
	Base     uint32
	Text     func(pointer uint32) *pipeline.Text
}

// NodeAt returns the node a pointer in the thread state points to, or nil.
func (mapping Mapping) NodeAt(pointer uint32) *pipeline.Node {
	if index := nodeIndex(int(pointer) - int(mapping.Base)); pointer != 0 && index >= 0 {
		return mapping.Pipeline.Node(index)
	}
	return nil
}

// PointerTo returns a node's pointer in the thread state.
func (mapping Mapping) PointerTo(node *pipeline.Node) uint32 {
	if node == nil {
		return 0
	}
	return mapping.Base + uint32(NodeOffsets()[node.Index()])
}

// storePointer writes a node pointer, unless the state's points to the same node.
func (mapping Mapping) storePointer(state []byte, offset int, node *pipeline.Node) {
	if mapping.NodeAt(Dword(state, offset)) == node {
		return
	}
	binary.LittleEndian.PutUint32(state[offset:], mapping.PointerTo(node))
}

func cursorPointers(cursor *pipeline.Cursor) []**pipeline.Node {
	return []**pipeline.Node{&cursor.Start, &cursor.Done, &cursor.Scan, &cursor.First, &cursor.End, &cursor.Working[0], &cursor.Working[1]}
}

// cursorValues are a cursor's settings and types, in their order in the thread state.
func cursorValues(cursor *pipeline.Cursor) []any {
	return []any{&cursor.PhoneticInput, &cursor.Phrase, &cursor.Rate, &cursor.Pitch, &cursor.Attenuation, &cursor.Filter,
		&cursor.Flags, &cursor.InputFlags, &cursor.Types, &cursor.Voice}
}

// PipelineFields are the pipeline's values the stages share, with their places in the thread state.
func PipelineFields(state *pipeline.Pipeline) []Field {
	fields := []Field{At(Pitch, "pitch", &state.Pitch), At(Speed, "speed", &state.Speed),
		At(Volume, "volume", &state.Volume), At(Character, "character", &state.Character),
		At(ResetValue, "reset value", &state.ResetValue), At(SavedRate, "saved rate", &state.SavedRate),
		At(ShortPhrase, "short phrase", &state.ShortPhrase), At(ParsedReset, "parsed reset", &state.ParsedReset),
		At(NumberWord, "number word", &state.NumberWord),
		At(ResetRequested, "reset requested", &state.ResetRequested)}
	for track := range datapack.TrackCount {
		fields = append(fields, At(Levels+track, fmt.Sprintf("level %d", track), &state.Levels[track]))
	}
	return fields
}

// LoadPipeline gives the pipeline the thread state's nodes, cursors, free count and shared values.
func (mapping Mapping) LoadPipeline(state []byte) {
	target := mapping.Pipeline
	for index, offset := range NodeOffsets() {
		node := target.Node(index)
		node.Next, node.Previous = mapping.NodeAt(Dword(state, offset)), mapping.NodeAt(Dword(state, offset+4))
		copy(node.Data[:], state[offset+NodeData:])
		node.Text = nil
		if pointer := Dword(state, offset+NodeText); pointer != 0 && mapping.Text != nil {
			node.Text = mapping.Text(pointer)
		} else if pointer != 0 {
			node.Text = &pipeline.Text{}
		}
	}
	for stage := range target.Cursors {
		offset := Cursors + CursorSize*stage
		for index, field := range cursorPointers(&target.Cursors[stage]) {
			*field = mapping.NodeAt(Dword(state, offset+4*index))
		}
		for index, field := range cursorValues(&target.Cursors[stage]) {
			setValue(field, Dword(state, offset+cursorValuesOffset+4*index))
		}
	}
	target.Current = nil
	if offset := int(Dword(state, Current)) - int(mapping.Base) - Cursors; offset >= 0 && offset%CursorSize == 0 && offset/CursorSize < pipeline.StageCount {
		target.Current = &target.Cursors[offset/CursorSize]
	}
	target.Free = int(Dword(state, Free))
	mapping.LoadFields(PipelineFields(target), state)
	target.Synthesizer.OutputVoice = int(int16(binary.LittleEndian.Uint16(state[OutputVoice:])))
	target.SampleRate = int(binary.LittleEndian.Uint16(state[SampleRate:]))
	target.VisualEvents = binary.LittleEndian.Uint16(state[VisualEvents:]) != 0
	target.PostedSpace = binary.LittleEndian.Uint16(state[PostedSpace:]) != 0
	if Dword(state, Engine) != 0 && target.Engine == nil {
		target.Engine = &pipeline.Engine{}
	}
}

// StorePipeline writes the pipeline's nodes, cursors, current cursor, free count and shared values into the
// thread state. It writes a node pointer only where it points elsewhere than the state's.
func (mapping Mapping) StorePipeline(state []byte) {
	source := mapping.Pipeline
	for index, offset := range NodeOffsets() {
		node := source.Node(index)
		mapping.storePointer(state, offset, node.Next)
		mapping.storePointer(state, offset+4, node.Previous)
		copy(state[offset+NodeData:offset+NodeData+len(node.Data)], node.Data[:])
		if node.Text == nil {
			binary.LittleEndian.PutUint32(state[offset+NodeText:], 0)
		}
	}
	for stage := range source.Cursors {
		offset := Cursors + CursorSize*stage
		for index, field := range cursorPointers(&source.Cursors[stage]) {
			mapping.storePointer(state, offset+4*index, *field)
		}
		for index, field := range cursorValues(&source.Cursors[stage]) {
			binary.LittleEndian.PutUint32(state[offset+cursorValuesOffset+4*index:], valueOf(field))
		}
	}
	binary.LittleEndian.PutUint32(state[Free:], uint32(source.Free))
	current := uint32(0)
	for stage := range source.Cursors {
		if source.Current == &source.Cursors[stage] {
			current = mapping.Base + uint32(Cursors+CursorSize*stage)
		}
	}
	binary.LittleEndian.PutUint32(state[Current:], current)
	mapping.StoreFields(PipelineFields(source), state)
	binary.LittleEndian.PutUint16(state[OutputVoice:], uint16(source.Synthesizer.OutputVoice))
	binary.LittleEndian.PutUint16(state[PostedSpace:], 0)
	if source.PostedSpace {
		state[PostedSpace] = 1
	}
}

// A Field is a Go value with its place in the thread state, and its name when named. Its value is an *int32,
// *uint32, *byte, *bool or **pipeline.Node; a bool is a byte, True when true (1 when True is 0).
type Field struct {
	Offset int
	Name   string
	Value  any
	True   byte
}

// At returns the field of a value at an offset.
func At(offset int, name string, value any) Field {
	return Field{Offset: offset, Name: name, Value: value}
}

// FlagAt returns the field of a bool at an offset that is the byte trueValue when true.
func FlagAt(offset int, name string, value *bool, trueValue byte) Field {
	return Field{Offset: offset, Name: name, Value: value, True: trueValue}
}

// elsewhere is the node a field gets for a pointer to no node of the thread state, as one into the thread state an
// engine reset replaced. Storing it, or a node of another pipeline, as one a speaking thread that ended had,
// leaves the pointer.
var elsewhere = new(pipeline.Node)

// LoadFields gives the fields the thread state's values.
func (mapping Mapping) LoadFields(fields []Field, state []byte) {
	for _, field := range fields {
		switch value := field.Value.(type) {
		case *int32:
			*value = int32(Dword(state, field.Offset))
		case *uint32:
			*value = Dword(state, field.Offset)
		case *byte:
			*value = state[field.Offset]
		case *bool:
			*value = state[field.Offset] != 0
		case **pipeline.Node:
			pointer := Dword(state, field.Offset)
			if *value = mapping.NodeAt(pointer); *value == nil && pointer != 0 {
				*value = elsewhere
			}
		}
	}
}

// StoreFields writes the fields into the thread state.
func (mapping Mapping) StoreFields(fields []Field, state []byte) {
	for _, field := range fields {
		switch value := field.Value.(type) {
		case *int32:
			binary.LittleEndian.PutUint32(state[field.Offset:], uint32(*value))
		case *uint32:
			binary.LittleEndian.PutUint32(state[field.Offset:], *value)
		case *byte:
			state[field.Offset] = *value
		case *bool:
			state[field.Offset] = 0
			if *value {
				state[field.Offset] = max(field.True, 1)
			}
		case **pipeline.Node:
			switch node := *value; {
			case node == nil:
				binary.LittleEndian.PutUint32(state[field.Offset:], 0)
			case mapping.Pipeline.Node(node.Index()) == node:
				mapping.storePointer(state, field.Offset, node)
			}
		}
	}
}

// Names names the thread state's bytes the fields cover.
func Names(fields ...[]Field) map[int]string {
	names := map[int]string{PostedSpace: "posted space", Free: "free nodes"}
	for _, list := range fields {
		for _, field := range list {
			size := 1
			switch field.Value.(type) {
			case *int32, *uint32, **pipeline.Node:
				size = 4
			}
			for offset := range size {
				names[field.Offset+offset] = field.Name
			}
		}
	}
	return names
}

// Describe names a byte of the thread state, for the reports.
func Describe(offset int, names map[int]string) string {
	if offset >= Nodes && offset < Nodes+pipeline.NodeCount*NodeSize {
		return fmt.Sprintf("node %d byte %#x", (offset-Nodes)/NodeSize, (offset-Nodes)%NodeSize)
	}
	if offset >= Cursors && offset < Cursors+pipeline.StageCount*CursorSize {
		return fmt.Sprintf("cursor %d byte %#x", (offset-Cursors)/CursorSize, (offset-Cursors)%CursorSize)
	}
	if name, found := names[offset]; found {
		return name
	}
	return fmt.Sprintf("%04x", offset)
}

// Differences reports where a state differs from the expected one, naming the bytes by describe.
func Differences(state, expected []byte, describe func(offset int) string) error {
	var found []string
	for offset := range state {
		if state[offset] != expected[offset] {
			if len(found) == 8 {
				found = append(found, "...")
				break
			}
			found = append(found, fmt.Sprintf("%s is %d, expected %d", describe(offset), state[offset], expected[offset]))
		}
	}
	if found != nil {
		return fmt.Errorf("%s", strings.Join(found, "; "))
	}
	return nil
}

func setValue(field any, value uint32) {
	switch field := field.(type) {
	case *int32:
		*field = int32(value)
	case *uint32:
		*field = value
	}
}

func valueOf(field any) uint32 {
	switch field := field.(type) {
	case *int32:
		return uint32(*field)
	case *uint32:
		return *field
	}
	return 0
}
