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
	stateMarkers        = 0x14
	stateSegments       = 0x144
	segmentSize         = 0x1c
	stateFrames         = 0x3ac
	stateTones          = 0x1fbd
	stateCoarticulation = 0x2034
	stateLevels         = 0x1f98
	stateSettings       = 0x1a28
	stateEngine         = 0x20f4
	stateSampleRate     = 0x210c
	stateVisualEvents   = 0x2130
	statePostedSpace    = 0x2132
	stateCursor         = 0x2210
	stateAttenuation    = 0x223c
	stateInputFlags     = 0x2248
	stateTypes          = 0x224c
	stateVoice          = 0x2250
	stateListLast       = 0x22a0
	stateListFirst      = 0x22c0
	stateFreeLast       = 0x22e8
	stateFreeFirst      = 0x2308
	stateNodes          = 0x2328
	nodeSize            = 0x20
	nodeData            = 8
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
// Actions are the calls of the rules' actions it made, directly or not.
type call struct {
	record, result *callRecord
	before, after  []byte
	base           uint32
	actions        []*call
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

// actionFunctions are the original's functions of the rules' actions 1 to 16.
var actionFunctions = [datapack.ActionCount + 1]string{"", "Parameters::ShiftAspirationBoundary",
	"Parameters::ShapesFromPrevious", "Parameters::VowelOnset", "Parameters::VowelCoarticulation",
	"Parameters::SonorantTransitions", "Parameters::ObstruentVoicing", "Parameters::FricativeAmplitudes",
	"Parameters::ConsonantShapes", "Parameters::StopClosureBurst", "Parameters::FinishPhoneme",
	"Parameters::LocusWeights", "Parameters::SpecialLoci", "Parameters::ConsonantTargetsAndF0",
	"Parameters::SourceBoundaries", "PullAspirationBack", "Parameters::PhonemeRules"}

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
	isAction := map[string]bool{}
	for _, function := range actionFunctions[1:] {
		isAction[function] = true
	}
	type open struct {
		call     *call
		replayed bool
	}
	var stack []open
	inAction := func() bool {
		return slices.ContainsFunc(stack, func(entry open) bool { return isAction[entry.call.record.Function] })
	}
	replayedOpen := func() bool {
		return slices.ContainsFunc(stack, func(entry open) bool { return entry.replayed })
	}
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
			if entry.replayed || isAction[record.Function] && replayedOpen() && !inAction() {
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
		if isAction[record.Function] && entry.call.before != nil {
			entry.call.after = slices.Clone(state)
			for _, ancestor := range stack {
				if ancestor.replayed {
					ancestor.call.actions = append(ancestor.call.actions, entry.call)
				}
			}
		}
		if !entry.replayed {
			continue
		}
		counts[record.Function]++
		if entry.call.after == nil {
			entry.call.after = state
		}
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
		{0x1e24, "previous category", &generator.PreviousCategory}, {0x1e28, "category", &generator.Category},
		{0x1e2c, "previous frontness", &generator.PreviousFrontness}, {0x1e30, "frontness", &generator.Frontness},
		{0x1e34, "aspiration shift", &generator.AspirationShift}, {0x1e38, "voice onset", &generator.VoiceOnset},
		{0x1e3c, "burst frames", &generator.BurstFrames}, {0x1e40, "voicing shift", &generator.VoicingShift},
		{0x1e44, "vowel transition", &generator.VowelTransition}, {0x1e48, "vowel offset", &generator.VowelOffset},
		{0x1e50, "closure frames", &generator.ClosureFrames}, {0x1e54, "fricative aspiration", &generator.FricativeAspiration},
		{0x1e58, "aspiration", &generator.Aspiration}, {0x1fb8, "unvoiced pitch", &generator.UnvoicedPitch},
		{0x1fe8, "transition limit", &generator.TransitionLimit}, {0x1fb0, "pause state", &generator.PauseState},
		{0x1fb4, "end state", &generator.EndState}, {0x2010, "previous group", &generator.PreviousGroup},
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
	bools := []boolField{{0x1fdd, "keep earlier", &generator.KeepEarlier}, {0x1fbc, "end reached", &generator.EndReached},
		{0x1fae, "generated done", &generator.GeneratedDone}}
	for flag := range generator.Flags {
		bools = append(bools, boolField{0x1e68 + flag, label("flag %d", flag), &generator.Flags[flag]})
	}
	bytes := []byteField{{stateCoarticulation, "coarticulation", &generator.Coarticulation}, {0x2035, "pitch mark", &generator.PitchMark},
		{0x1fed, "before previous", &generator.BeforePrevious}, {0x1fec, "after next", &generator.AfterNext}}
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
	loadInto(generator, call.before, call.base)
	return generator
}

// loadInto gives the generator, its pipeline's nodes and cursor and the ring the thread state's values.
func loadInto(generator *Generator, state []byte, base uint32) {
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
	node := func(pointer uint32) *pipeline.Node {
		if index := nodeIndex(int(pointer) - int(base)); pointer != 0 && index >= 0 {
			return pipelineState.Node(index)
		}
		return nil
	}
	for index, offset := range nodeOffsets() {
		target := pipelineState.Node(index)
		target.Next, target.Previous = node(dword(state, offset)), node(dword(state, offset+4))
		copy(target.Data[:], state[offset+nodeData:])
	}
	cursor := &pipelineState.Cursors[pipeline.Parameters]
	cursor.Start, cursor.Done, cursor.Scan = node(dword(state, stateCursor)), node(dword(state, stateCursor+4)), node(dword(state, stateCursor+8))
	cursor.First, cursor.End = node(dword(state, stateCursor+12)), node(dword(state, stateCursor+16))
	cursor.Attenuation, cursor.Voice = int32(dword(state, stateAttenuation)), int32(dword(state, stateVoice))
	cursor.InputFlags = dword(state, stateInputFlags)
	pipelineState.SampleRate = int(binary.LittleEndian.Uint16(state[stateSampleRate:]))
	pipelineState.VisualEvents = binary.LittleEndian.Uint16(state[stateVisualEvents:]) != 0
	pipelineState.PostedSpace = binary.LittleEndian.Uint16(state[statePostedSpace:]) != 0
	if dword(state, stateEngine) != 0 && pipelineState.Engine == nil {
		pipelineState.Engine = &pipeline.Engine{}
	}
}

// store writes the generator's values, the pipeline's node contents and the ring into the thread state.
func store(generator *Generator, state []byte) {
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
	for index, offset := range nodeOffsets() {
		node := generator.Pipeline.Node(index)
		copy(state[offset+nodeData:offset+nodeData+len(node.Data)], node.Data[:])
	}
	binary.LittleEndian.PutUint16(state[statePostedSpace:], 0)
	if generator.Pipeline.PostedSpace {
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
func difference(generator *Generator, image, expected []byte) error {
	state := slices.Clone(image)
	store(generator, state)
	var differences []string
	for offset := range state {
		if state[offset] != expected[offset] {
			if len(differences) == 8 {
				differences = append(differences, "...")
				break
			}
			differences = append(differences, fmt.Sprintf("%s is %d, expected %d", describe(offset), state[offset], expected[offset]))
		}
	}
	if differences != nil {
		return fmt.Errorf("%s", strings.Join(differences, "; "))
	}
	return nil
}

// compare reports where the state the generator leaves differs from the original's.
func compare(generator *Generator, call *call) error {
	return difference(generator, call.before, call.after)
}

// recordedActions stands for the rules' actions the port doesn't have: each takes the generator to the state
// the original's action left, after checking that the generator is in the state it started from. The
// recorded calls of the actions the port has are skipped: the state the next action starts from, or the
// state the call leaves, checks them.
type recordedActions struct {
	call     *call
	position int
	image    []byte
	err      error
}

func (recorded *recordedActions) Run(generator *Generator, action int) {
	if recorded.err != nil {
		return
	}
	recorded.skipPorted()
	if recorded.position == len(recorded.call.actions) {
		recorded.err = fmt.Errorf("action %d (%s) wasn't recorded", action, actionFunctions[action])
		return
	}
	actionCall := recorded.call.actions[recorded.position]
	recorded.position++
	if actionCall.record.Function != actionFunctions[action] {
		recorded.err = fmt.Errorf("action %d (%s) runs where the original ran %s", action, actionFunctions[action], actionCall.record.Function)
		return
	}
	if err := difference(generator, recorded.image, actionCall.before); err != nil {
		recorded.err = fmt.Errorf("before %s: %v", actionCall.record.Function, err)
		return
	}
	loadInto(generator, actionCall.after, actionCall.base)
	recorded.image = actionCall.after
}

func (recorded *recordedActions) skipPorted() {
	for recorded.position < len(recorded.call.actions) && isPorted(recorded.call.actions[recorded.position].record.Function) {
		recorded.position++
	}
}

func isPorted(function string) bool {
	action := slices.Index(actionFunctions[:], function)
	return action > 0 && ported[action] != nil
}

// withActions runs a Go function on the call's state with the recorded actions, and compares the state it
// leaves.
func (data *testData) withActions(call *call, run func(generator *Generator)) error {
	generator := data.load(call)
	recorded := &recordedActions{call: call, image: call.before}
	generator.Actions = recorded
	run(generator)
	if recorded.err != nil {
		return recorded.err
	}
	recorded.skipPorted()
	if recorded.position != len(call.actions) {
		return fmt.Errorf("%d of %d recorded actions ran, the next %s", recorded.position, len(call.actions), call.actions[recorded.position].record.Function)
	}
	return difference(generator, recorded.image, call.after)
}
