package parameters

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
	"github.com/phyce/OpenSAPI4/truvoice/synthesizer"
)

// The parameter generator's golden data (golden/truvoice/parameters.jsonl.gz) records calls of the original's
// functions with the changes of the speaking thread's state around them. replay rebuilds the state at every
// call and return, so that a Go function can start from the state the original's started from and leave what
// it left.

const stateSize = 0x9184

// The thread state's places the replay maps: the ring's, the generator's own, the pipeline's nodes, the
// parameters cursor and the visual events.
const (
	stateDone           = 0x0
	stateWanted         = 0x38
	stateMarkers        = 0x14
	stateSegments       = 0x144
	segmentSize         = 0x1c
	stateFrames         = 0x3ac
	stateTones          = 0x1fbd
	stateCoarticulation = 0x2034
	stateLevels         = 0x1f98
	stateSettings       = 0x1a28
	stateSavedRate      = 0x1ae4
	stateShortPhrase    = 0x1d54
	stateMute           = 0x2075
	stateSegmentFrame   = 0x1ff6
	stateEngine         = 0x20f4
	statePitch          = 0x20f8
	stateSpeed          = 0x20fc
	stateVolume         = 0x2100
	stateSampleRate     = 0x210c
	stateOutputVoice    = 0x210e
	stateResetRequested = 0x211d
	stateParsedReset    = 0x211e
	stateResetValue     = 0x2120
	stateVisualEvents   = 0x2130
	statePostedSpace    = 0x2132
	stateCurrent        = 0x213c
	stateFree           = 0x2140
	stateCursors        = 0x2144
	cursorSize          = 0x44
	stateCursor         = 0x2210
	stateListLast       = 0x22a0
	stateListFirst      = 0x22c0
	stateFreeLast       = 0x22e8
	stateFreeFirst      = 0x2308
	stateNodes          = 0x2328
	nodeSize            = 0x20
	nodeData            = 8
	nodeText            = 0x1c
)

type diffRun struct {
	offset int
	data   []byte
}

func (run *diffRun) UnmarshalJSON(data []byte) error {
	var pair [2]json.RawMessage
	if err := json.Unmarshal(data, &pair); err != nil {
		return err
	}
	var text string
	if err := json.Unmarshal(pair[0], &run.offset); err != nil {
		return err
	}
	if err := json.Unmarshal(pair[1], &text); err != nil {
		return err
	}
	var err error
	run.data, err = hex.DecodeString(text)
	return err
}

type callRecord struct {
	Kind      string    `json:"kind"`
	Function  string    `json:"function"`
	Site      string    `json:"site"`
	ECX       uint32    `json:"ecx"`
	EDX       uint32    `json:"edx"`
	Arguments []uint32  `json:"arguments"`
	EAX       uint32    `json:"eax"`
	Base      uint32    `json:"base"`
	Diff      []diffRun `json:"diff"`
}

type goldenCase struct {
	Name    string       `json:"name"`
	Records []callRecord `json:"records"`
}

// A call is a recorded call of one of the original's functions, with its return and the thread state at both.
type call struct {
	record, result *callRecord
	before, after  []byte
	base           uint32
}

func (call *call) argument(index int) int32 {
	return int32(call.record.Arguments[index])
}

// ring returns the track whose ring an argument points to.
func (call *call) ring(index int) (int, error) {
	offset := int64(call.record.Arguments[index]) - int64(call.base) - stateFrames
	if offset < 0 || offset%synthesizer.RingLength != 0 || offset/synthesizer.RingLength >= datapack.TrackCount {
		return 0, fmt.Errorf("argument %d isn't a ring: %08x", index, call.record.Arguments[index])
	}
	return int(offset / synthesizer.RingLength), nil
}

type replayer func(call *call) error

// replay hands each recorded call of the functions given to their replayer, reports the calls that differ (a
// few per function) and returns how many calls each replayed. The cases are replayed in parallel.
func replay(t *testing.T, replayers map[string]replayer) map[string]int {
	t.Helper()
	var lock sync.Mutex
	counts := map[string]int{}
	failures := map[string]int{}
	cases := make(chan goldenCase)
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
	conformance.EachLine(t, func(golden goldenCase) { cases <- golden }, "parameters.jsonl.gz")
	close(cases)
	workers.Wait()
	for function, count := range failures {
		t.Errorf("%s: %d of %d calls differ", function, count, counts[function])
	}
	return counts
}

func replayCase(golden goldenCase, replayers map[string]replayer) (map[string]int, map[string]int, map[string][]string) {
	counts := map[string]int{}
	failures := map[string]int{}
	messages := map[string][]string{}
	state := make([]byte, stateSize)
	var base uint32
	type open struct {
		call     *call
		replayed bool
	}
	var stack []open
	for index := range golden.Records {
		record := &golden.Records[index]
		if record.Kind != "call" && record.Kind != "return" {
			continue
		}
		if record.Base != 0 {
			base = record.Base
			clear(state)
		}
		for _, run := range record.Diff {
			copy(state[run.offset:], run.data)
		}
		if record.Kind == "call" {
			entry := open{call: &call{record: record, base: base}, replayed: replayers[record.Function] != nil}
			if entry.replayed {
				entry.call.before = slices.Clone(state)
			}
			stack = append(stack, entry)
			continue
		}
		if len(stack) == 0 || stack[len(stack)-1].call.record.Function != record.Function {
			messages["replay"] = append(messages["replay"], fmt.Sprintf("%s: record %d returns from %s, which isn't the call open", golden.Name, index, record.Function))
			failures["replay"]++
			return counts, failures, messages
		}
		entry := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		entry.call.result = record
		if !entry.replayed {
			continue
		}
		counts[record.Function]++
		entry.call.after = state
		if err := replayers[record.Function](entry.call); err != nil {
			failures[record.Function]++
			if len(messages[record.Function]) < 5 {
				messages[record.Function] = append(messages[record.Function], fmt.Sprintf("%s: %s called at %s (record %d): %v", golden.Name, record.Function, entry.call.record.Site, index, err))
			}
		}
	}
	return counts, failures, messages
}

type dwordField struct {
	offset int
	name   string
	value  *int32
}

type boolField struct {
	offset int
	name   string
	value  *bool
}

type byteField struct {
	offset int
	name   string
	value  *byte
}

// generatorFields lists the generator's values with their places in the thread state, and with their names
// when named.
func generatorFields(generator *Generator, named bool) ([]dwordField, []boolField, []byteField) {
	label := func(format string, arguments ...any) string {
		if !named {
			return ""
		}
		return fmt.Sprintf(format, arguments...)
	}
	ring := generator.Synthesizer
	dwords := []dwordField{{0x4, "position", &ring.Position}, {0x8, "end", &ring.End}, {0xc, "written", &ring.Written},
		{0x10, "furthest", &generator.Furthest}, {0x1fe0, "lookahead", &ring.Lookahead}, {0x1fe4, "margin", &generator.Margin},
		{stateWanted, "wanted", &generator.Wanted}, {0x1ff0, "segment left", &generator.SegmentLeft},
		{0x200c, "tone frequency", &generator.ToneFrequency}, {stateSavedRate, "saved rate", &generator.Pipeline.SavedRate},
		{0x1e24, "previous category", &generator.PreviousCategory}, {0x1e28, "category", &generator.Category},
		{0x1e2c, "previous frontness", &generator.PreviousFrontness}, {0x1e30, "frontness", &generator.Frontness},
		{0x1e34, "aspiration shift", &generator.AspirationShift}, {0x1e38, "voice onset", &generator.VoiceOnset},
		{0x1e3c, "burst frames", &generator.BurstFrames}, {0x1e40, "voicing shift", &generator.VoicingShift},
		{0x1e44, "vowel transition", &generator.VowelTransition}, {0x1e48, "vowel offset", &generator.VowelOffset},
		{0x1e50, "closure frames", &generator.ClosureFrames}, {0x1e54, "fricative aspiration", &generator.FricativeAspiration},
		{0x1e58, "aspiration", &generator.Aspiration}, {0x1fb8, "unvoiced pitch", &generator.UnvoicedPitch},
		{0x1fe8, "transition limit", &generator.TransitionLimit}, {0x1fb0, "ahead", &generator.Ahead},
		{0x1fb4, "start", &generator.Start}, {0x2010, "previous group", &generator.PreviousGroup},
		{0x2014, "vowel frames", &generator.VowelFrames}}
	for index := range generator.BurstLevels {
		dwords = append(dwords, dwordField{0x1e5c + 4*index, label("burst level %d", index), &generator.BurstLevels[index]})
	}
	for index := range generator.Offglide {
		dwords = append(dwords, dwordField{0x1f24 + 4*index, label("offglide %d", index), &generator.Offglide[index]},
			dwordField{0x2018 + 4*index, label("midpoint %d", index), &generator.Midpoints[index]})
	}
	for track := range generator.Tracks {
		dwords = append(dwords, dwordField{0x3c + 4*track, label("track %d next", track), &generator.Tracks[track].Next},
			dwordField{0x94 + 4*track, label("track %d earliest", track), &generator.Tracks[track].Earliest},
			dwordField{0x1e74 + 4*track, label("track %d last", track), &generator.Last[track]},
			dwordField{0x1ecc + 4*track, label("track %d weight", track), &generator.Weights[track]},
			dwordField{0x1f40 + 4*track, label("track %d next target", track), &generator.NextTargets[track]})
		segment := &generator.Segments[track]
		for index, value := range []*int32{&segment.Mode, &segment.Back, &segment.Forward, &segment.Frames,
			&segment.BackTarget, &segment.Onset, &segment.Target} {
			name := []string{"mode", "back", "forward", "frames", "back target", "onset", "target"}[index]
			dwords = append(dwords, dwordField{stateSegments + segmentSize*track + 4*index, label("track %d segment %s", track, name), value})
		}
	}
	bools := []boolField{{0x1fdd, "keep earlier", &generator.KeepEarlier}, {0x1fbc, "at reset", &generator.AtReset},
		{0x1fae, "generated done", &generator.GeneratedDone}, {0x2035, "database loci", &generator.DatabaseLoci},
		{stateDone, "done", &ring.Done}, {0x1ff5, "segment starting", &generator.SegmentStarting},
		{stateShortPhrase, "short phrase", &generator.Pipeline.ShortPhrase}, {stateParsedReset, "parsed reset", &generator.Pipeline.ParsedReset},
		{stateMute, "mute", &ring.Mute}, {stateResetRequested, "reset requested", &generator.Pipeline.ResetRequested}}
	for flag := range generator.Flags {
		bools = append(bools, boolField{0x1e68 + flag, label("flag %d", flag), &generator.Flags[flag]})
	}
	bytes := []byteField{{stateCoarticulation, "coarticulation", &generator.Coarticulation},
		{0x1fed, "before previous", &generator.BeforePrevious}, {0x1fec, "after next", &generator.AfterNext},
		{0x1ff4, "segment symbol", &generator.SegmentSymbol}}
	for track := range datapack.TrackCount {
		bytes = append(bytes, byteField{stateSegmentFrame + track, label("segment frame %d", track), &generator.SegmentFrame[track]},
			byteField{stateLevels + track, label("level %d", track), &generator.Pipeline.Levels[track]})
	}
	return dwords, bools, bytes
}

func bit(state []byte, offset, slot int) bool {
	return state[offset+slot>>3]&(1<<(slot&7)) != 0
}

func setBit(state []byte, offset, slot int, value bool) {
	if value {
		state[offset+slot>>3] |= 1 << (slot & 7)
	} else {
		state[offset+slot>>3] &^= 1 << (slot & 7)
	}
}

func dword(state []byte, offset int) uint32 {
	return binary.LittleEndian.Uint32(state[offset:])
}

// nodeOffsets are the state offsets of the pipeline's nodes: the pool's, then the sentinels'.
var nodeOffsets = sync.OnceValue(func() []int {
	offsets := make([]int, pipeline.NodeCount, pipeline.NodeCount+4)
	for index := range offsets {
		offsets[index] = stateNodes + nodeSize*index
	}
	return append(offsets, stateListFirst, stateListLast, stateFreeFirst, stateFreeLast)
})

// nodeIndex returns the place among nodeOffsets of the node at a state offset, or -1.
func nodeIndex(offset int) int {
	if offset >= stateNodes && offset < stateNodes+pipeline.NodeCount*nodeSize && (offset-stateNodes)%nodeSize == 0 {
		return (offset - stateNodes) / nodeSize
	}
	if index := slices.Index(nodeOffsets()[pipeline.NodeCount:], offset); index >= 0 {
		return pipeline.NodeCount + index
	}
	return -1
}

// testData are the data pack's tables the replays need.
type testData struct {
	parameters *datapack.Parameters
	vowels     *datapack.Vowels
	pipeline   *datapack.Pipeline
}

func loadTestData(t *testing.T) *testData {
	t.Helper()
	pack := conformance.Pack(t)
	parameters, err := pack.LoadParameters()
	if err != nil {
		t.Fatal(err)
	}
	vowels, err := pack.LoadVowels()
	if err != nil {
		t.Fatal(err)
	}
	pipelineTables, err := pack.LoadPipeline()
	if err != nil {
		t.Fatal(err)
	}
	return &testData{parameters, vowels, pipelineTables}
}

// load returns a generator with the thread state's values when the call started.
func (data *testData) load(call *call) *Generator {
	generator := New(data.parameters, data.vowels, pipeline.New(data.pipeline, new(synthesizer.Synthesizer), nil, nil))
	loadInto(generator, call.before, call.base, nil)
	return generator
}

// loadInto gives the generator, its pipeline's nodes and cursor and the ring the thread state's values. text
// returns the text a node's text pointer stands for; without it, a node with a text gets a new one.
func loadInto(generator *Generator, state []byte, base uint32, text func(pointer uint32) *pipeline.Text) {
	dwords, bools, bytes := generatorFields(generator, false)
	for _, field := range dwords {
		*field.value = int32(dword(state, field.offset))
	}
	for _, field := range bools {
		*field.value = state[field.offset] != 0
	}
	for _, field := range bytes {
		*field.value = state[field.offset]
	}
	ring := generator.Synthesizer
	for slot := range synthesizer.RingLength {
		for track := range datapack.TrackCount {
			ring.Frames[slot][track] = state[stateFrames+synthesizer.RingLength*track+slot]
		}
		ring.Markers[slot] = bit(state, stateMarkers, slot)
		ring.Tones[slot] = bit(state, stateTones, slot)
	}
	pipelineState := generator.Pipeline
	for index, offset := range nodeOffsets() {
		target := pipelineState.Node(index)
		target.Next, target.Previous = nodeAt(pipelineState, dword(state, offset), base), nodeAt(pipelineState, dword(state, offset+4), base)
		copy(target.Data[:], state[offset+nodeData:])
		target.Text = nil
		if pointer := dword(state, offset+nodeText); pointer != 0 && text != nil {
			target.Text = text(pointer)
		} else if pointer != 0 {
			target.Text = &pipeline.Text{}
		}
	}
	for stage := range pipelineState.Cursors {
		offset := stateCursors + cursorSize*stage
		for index, field := range cursorPointers(&pipelineState.Cursors[stage]) {
			*field = nodeAt(pipelineState, dword(state, offset+4*index), base)
		}
		for index, field := range cursorValues(&pipelineState.Cursors[stage]) {
			setValue(field, dword(state, offset+cursorValuesOffset+4*index))
		}
	}
	pipelineState.Current = nil
	if offset := int(dword(state, stateCurrent)) - int(base) - stateCursors; offset >= 0 && offset%cursorSize == 0 && offset/cursorSize < pipeline.StageCount {
		pipelineState.Current = &pipelineState.Cursors[offset/cursorSize]
	}
	pipelineState.Free = int(dword(state, stateFree))
	for _, field := range attributes(generator) {
		*field.value = dword(state, field.offset)
	}
	ring.OutputVoice = int(int16(binary.LittleEndian.Uint16(state[stateOutputVoice:])))
	pipelineState.SampleRate = int(binary.LittleEndian.Uint16(state[stateSampleRate:]))
	pipelineState.VisualEvents = binary.LittleEndian.Uint16(state[stateVisualEvents:]) != 0
	pipelineState.PostedSpace = binary.LittleEndian.Uint16(state[statePostedSpace:]) != 0
	if dword(state, stateEngine) != 0 && pipelineState.Engine == nil {
		pipelineState.Engine = &pipeline.Engine{}
	}
}

// nodeAt returns the node a pointer in the thread state points to, or nil.
func nodeAt(pipelineState *pipeline.Pipeline, pointer, base uint32) *pipeline.Node {
	if index := nodeIndex(int(pointer) - int(base)); pointer != 0 && index >= 0 {
		return pipelineState.Node(index)
	}
	return nil
}

// cursorValuesOffset is where a cursor's values start, after its seven pointers.
const cursorValuesOffset = 0x1c

func cursorPointers(cursor *pipeline.Cursor) []**pipeline.Node {
	return []**pipeline.Node{&cursor.Start, &cursor.Done, &cursor.Scan, &cursor.First, &cursor.End, &cursor.Working[0], &cursor.Working[1]}
}

// cursorValues are a cursor's settings and types, in their order in the thread state.
func cursorValues(cursor *pipeline.Cursor) []any {
	return []any{&cursor.PhoneticInput, &cursor.Flush, &cursor.Rate, &cursor.Pitch, &cursor.Attenuation, &cursor.Filter,
		&cursor.Flags, &cursor.InputFlags, &cursor.Types, &cursor.Voice}
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

type attributeField struct {
	offset int
	value  *uint32
}

// attributes are the pipeline's attributes the control commands set, with the reset request's value.
func attributes(generator *Generator) []attributeField {
	pipelineState := generator.Pipeline
	return []attributeField{{statePitch, &pipelineState.Pitch}, {stateSpeed, &pipelineState.Speed},
		{stateVolume, &pipelineState.Volume}, {stateResetValue, &pipelineState.ResetValue}}
}

// store writes the generator's values, the pipeline's nodes, cursors and attributes and the ring into the thread
// state. It writes a pointer only where it points elsewhere than the state's.
func store(generator *Generator, state []byte, base uint32) {
	dwords, bools, bytes := generatorFields(generator, false)
	for _, field := range dwords {
		binary.LittleEndian.PutUint32(state[field.offset:], uint32(*field.value))
	}
	for _, field := range bools {
		state[field.offset] = 0
		if *field.value {
			state[field.offset] = 1
		}
	}
	for _, field := range bytes {
		state[field.offset] = *field.value
	}
	ring := generator.Synthesizer
	for slot := range synthesizer.RingLength {
		for track := range datapack.TrackCount {
			state[stateFrames+synthesizer.RingLength*track+slot] = ring.Frames[slot][track]
		}
		setBit(state, stateMarkers, slot, ring.Markers[slot])
		setBit(state, stateTones, slot, ring.Tones[slot])
	}
	pipelineState := generator.Pipeline
	pointer := func(offset int, node *pipeline.Node) {
		if nodeAt(pipelineState, dword(state, offset), base) == node {
			return
		}
		value := uint32(0)
		if node != nil {
			value = base + uint32(nodeOffsets()[node.Index()])
		}
		binary.LittleEndian.PutUint32(state[offset:], value)
	}
	for index, offset := range nodeOffsets() {
		node := pipelineState.Node(index)
		pointer(offset, node.Next)
		pointer(offset+4, node.Previous)
		copy(state[offset+nodeData:offset+nodeData+len(node.Data)], node.Data[:])
		if node.Text == nil {
			binary.LittleEndian.PutUint32(state[offset+nodeText:], 0)
		}
	}
	for stage := range pipelineState.Cursors {
		offset := stateCursors + cursorSize*stage
		for index, field := range cursorPointers(&pipelineState.Cursors[stage]) {
			pointer(offset+4*index, *field)
		}
		for index, field := range cursorValues(&pipelineState.Cursors[stage]) {
			binary.LittleEndian.PutUint32(state[offset+cursorValuesOffset+4*index:], valueOf(field))
		}
	}
	binary.LittleEndian.PutUint32(state[stateFree:], uint32(pipelineState.Free))
	for _, field := range attributes(generator) {
		binary.LittleEndian.PutUint32(state[field.offset:], *field.value)
	}
	binary.LittleEndian.PutUint16(state[stateOutputVoice:], uint16(ring.OutputVoice))
	binary.LittleEndian.PutUint16(state[statePostedSpace:], 0)
	if pipelineState.PostedSpace {
		state[statePostedSpace] = 1
	}
}

var stateNames = sync.OnceValue(func() map[int]string {
	names := map[int]string{statePostedSpace: "posted space"}
	dwords, bools, bytes := generatorFields(New(nil, nil, pipeline.New(nil, new(synthesizer.Synthesizer), nil, nil)), true)
	for _, field := range dwords {
		for offset := range 4 {
			names[field.offset+offset] = field.name
		}
	}
	for _, field := range bools {
		names[field.offset] = field.name
	}
	for _, field := range bytes {
		names[field.offset] = field.name
	}
	return names
})

// describe names a byte of the thread state, for the reports.
func describe(offset int) string {
	switch {
	case offset >= stateFrames && offset < stateFrames+datapack.TrackCount*synthesizer.RingLength:
		return fmt.Sprintf("track %d frame %d", (offset-stateFrames)/synthesizer.RingLength, (offset-stateFrames)%synthesizer.RingLength)
	case offset >= stateNodes && offset < stateNodes+pipeline.NodeCount*nodeSize:
		return fmt.Sprintf("node %d byte %#x", (offset-stateNodes)/nodeSize, (offset-stateNodes)%nodeSize)
	}
	if name, found := stateNames()[offset]; found {
		return name
	}
	return fmt.Sprintf("%04x", offset)
}

// difference reports where the state the generator leaves, written over image, differs from expected.
func difference(generator *Generator, image, expected []byte, base uint32) error {
	state := slices.Clone(image)
	store(generator, state, base)
	return differences(state, expected)
}

// differences reports where a state differs from the expected one.
func differences(state, expected []byte) error {
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

// compare reports where the state the generator leaves differs from the original's.
func compare(generator *Generator, call *call) error {
	return difference(generator, call.before, call.after, call.base)
}
