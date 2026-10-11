package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/internal/conformance"
	"github.com/phyce/OpenSAPI4/truvoice/synthesizer"
)

type goldenCursors struct {
	Position  int32 `json:"position"`
	End       int32 `json:"end"`
	Written   int32 `json:"written"`
	Lookahead int32 `json:"lookahead"`
	Done      byte  `json:"done"`
	Hold      byte  `json:"hold"`
	Ready     byte  `json:"ready"`
	Markers   int32 `json:"markers"`
	Bytes     int   `json:"bytes"`
}

// goldenScheduleState is what the original's scheduler reads and writes besides the stages.
type goldenScheduleState struct {
	Flags      string `json:"flags"`
	ResetValue uint32 `json:"resetValue"`
	Free       int    `json:"free"`
	Bytes      int    `json:"bytes"`
	Done       byte   `json:"done"`
	Hold       byte   `json:"hold"`
	Ready      byte   `json:"ready"`
	Markers    int32  `json:"markers"`
	Position   int32  `json:"position"`
	Highest    int32  `json:"highest"`
	Margin     int32  `json:"margin"`
	Waiting    int32  `json:"waiting"`
	Mute       byte   `json:"mute"`
	Gain       uint16 `json:"gain"`
	Rate       int16  `json:"rate"`
	Voice      int16  `json:"voice"`
	Rendering  uint16 `json:"rendering"`

	Pitch     uint32        `json:"pitch"`
	Speed     uint32        `json:"speed"`
	Volume    uint32        `json:"volume"`
	Character uint32        `json:"character"`
	Engine    *goldenEngine `json:"engine"`
}

// threadState is what the speaking thread keeps in the pipeline besides the scheduler: its copies of the
// attributes, the synthesizer's output-filter voice, mute flag, gain, rate and rendering flag, and the engine's
// attributes.
type threadState struct {
	Pitch, Speed, Volume, Character uint32
	Voice                           int16
	Mute                            byte
	Gain                            uint16
	Rate                            int16
	Rendering                       uint16
	Engine                          goldenEngine
}

func (state goldenScheduleState) thread() threadState {
	return threadState{Pitch: state.Pitch, Speed: state.Speed, Volume: state.Volume, Character: state.Character,
		Voice: state.Voice, Mute: state.Mute, Gain: state.Gain, Rate: state.Rate, Rendering: state.Rendering, Engine: *state.Engine}
}

func (replay *replay) threadState() threadState {
	pipeline := replay.pipeline
	synthesizer := pipeline.Synthesizer
	engine := pipeline.Engine
	return threadState{Pitch: pipeline.Pitch, Speed: pipeline.Speed, Volume: pipeline.Volume, Character: pipeline.Character,
		Voice: int16(synthesizer.OutputVoice), Mute: boolByte(synthesizer.Mute), Gain: synthesizer.Gain,
		Rate: int16(pipeline.SampleRate), Rendering: uint16(boolByte(synthesizer.Rendering)),
		Engine: goldenEngine{engine.Pitch, engine.Speed, engine.Volume, engine.Character, engine.Voice}}
}

func (replay *replay) compareThread(state goldenScheduleState) error {
	if state.Engine == nil {
		return fmt.Errorf("no engine recorded")
	}
	if replay.racy {
		replay.setEngine(state.Engine)
	}
	if actual, expected := replay.threadState(), state.thread(); actual != expected {
		return fmt.Errorf("thread %+v, expected %+v", actual, expected)
	}
	return nil
}

func (replay *replay) setEngine(attributes *goldenEngine) {
	engine := replay.pipeline.Engine
	engine.Pitch, engine.Speed, engine.Volume = attributes.Pitch, attributes.Speed, attributes.Volume
	engine.Character, engine.Voice = attributes.Character, attributes.Voice
}

// settingFields are the escape parser's settings as the records list them.
func (replay *replay) settingFields() []int32 {
	pipeline := replay.pipeline
	settings := pipeline.Settings
	return []int32{settings.PhoneticInput, settings.Flush, settings.Rate, pipeline.SettingsSpeed, settings.Pitch,
		settings.Attenuation, settings.Filter, int32(settings.Flags), int32(settings.InputFlags), settings.Voice}
}

func (replay *replay) allCursorFields() [][]int64 {
	var cursors [][]int64
	for stage := range replay.pipeline.Cursors {
		cursors = append(cursors, cursorFields(&replay.pipeline.Cursors[stage]))
	}
	return cursors
}

// attributeFields are, for each attribute, the places in the settings and in each cursor its setter writes.
var attributeFields = map[string]struct{ settings, cursor []int }{
	"pitch":  {[]int{4}, []int{10}},
	"speed":  {[]int{2, 3}, []int{9}},
	"volume": {[]int{5}, []int{11}},
	"voice":  {[]int{9}, []int{16}},
}

// applyAttributes runs the Go thread's ApplyAttributes and compares what it wrote with the setters the
// original called: each setter's settings and cursor fields, mute flag and voice as its record has them, and
// the fields of the attributes it didn't apply as they were.
func (replay *replay) applyAttributes(applied []goldenRecord) error {
	pipeline := replay.pipeline
	settingsBefore, cursorsBefore := replay.settingFields(), replay.allCursorFields()
	pipeline.ApplyAttributes()
	settings, cursors := replay.settingFields(), replay.allCursorFields()
	written := map[string]bool{}
	for _, record := range applied {
		fields, known := attributeFields[record.Attribute]
		if !known {
			return fmt.Errorf("attribute %q", record.Attribute)
		}
		var expectedSettings []int32
		if err := json.Unmarshal(record.Settings, &expectedSettings); err != nil {
			return err
		}
		muting := record.Attribute == "volume" && record.Mute != 0
		written[record.Attribute] = !muting
		if muting {
			continue
		}
		for _, field := range fields.settings {
			if settings[field] != expectedSettings[field] {
				return fmt.Errorf("after the %s %d: settings %v, expected %v", record.Attribute, record.Value, settings, expectedSettings)
			}
		}
		for stage := range cursors {
			for _, field := range fields.cursor {
				if cursors[stage][field] != record.Cursors[stage][field] {
					return fmt.Errorf("after the %s %d: cursor %d is %v, expected %v", record.Attribute, record.Value, stage, cursors[stage], record.Cursors[stage])
				}
			}
		}
	}
	for _, record := range applied {
		if record.Attribute == "volume" && boolByte(pipeline.Synthesizer.Mute) != record.Mute {
			return fmt.Errorf("after the volume %d: mute %v, expected %d", record.Value, pipeline.Synthesizer.Mute, record.Mute)
		}
		if record.Attribute == "voice" && int16(pipeline.Synthesizer.OutputVoice) != record.Voice {
			return fmt.Errorf("after the voice %d: output-filter voice %d, expected %d", record.Value, pipeline.Synthesizer.OutputVoice, record.Voice)
		}
	}
	for attribute, fields := range attributeFields {
		if written[attribute] {
			continue
		}
		for _, field := range fields.settings {
			if settings[field] != settingsBefore[field] {
				return fmt.Errorf("the Go thread applied the %s, the original didn't: settings %v, before %v", attribute, settings, settingsBefore)
			}
		}
		for stage := range cursors {
			for _, field := range fields.cursor {
				if cursors[stage][field] != cursorsBefore[stage][field] {
					return fmt.Errorf("the Go thread applied the %s, the original didn't: cursor %d is %v, before %v", attribute, stage, cursors[stage], cursorsBefore[stage])
				}
			}
		}
	}
	return nil
}

// scheduleFlags are the original's flags from 2110: the input's, the scheduler's, the output stage's result
// and the reset request.
type scheduleFlags struct {
	OutOfInput     bool
	HasWork        [Output]bool
	WaitingForRoom bool
	Retired        int
	ResetRequested bool
	ResetValue     uint32
	Free           int
	Bytes          int
	Done           bool
	Hold           bool
	Ready          bool
	Markers        int32
	Position       int32
}

func decodeScheduleState(raw json.RawMessage) (goldenScheduleState, scheduleFlags, error) {
	var state goldenScheduleState
	if err := json.Unmarshal(raw, &state); err != nil {
		return state, scheduleFlags{}, err
	}
	bytes, err := hex.DecodeString(state.Flags)
	if err != nil || len(bytes) != 16 {
		return state, scheduleFlags{}, fmt.Errorf("flags %q", state.Flags)
	}
	flags := scheduleFlags{OutOfInput: bytes[2] != 0, WaitingForRoom: bytes[7] != 0,
		Retired: int(bytes[8]) | int(bytes[9])<<8 | int(bytes[10])<<16 | int(bytes[11])<<24, ResetRequested: bytes[13] != 0,
		ResetValue: state.ResetValue, Free: state.Free, Bytes: state.Bytes, Done: state.Done != 0, Hold: state.Hold != 0,
		Ready: state.Ready != 0, Markers: state.Markers, Position: state.Position}
	for stage := range flags.HasWork {
		flags.HasWork[stage] = bytes[3+stage] != 0
	}
	return state, flags, nil
}

func (replay *replay) scheduleFlags() scheduleFlags {
	pipeline := replay.pipeline
	synthesizer := pipeline.Synthesizer
	return scheduleFlags{OutOfInput: pipeline.OutOfInput, HasWork: pipeline.HasWork, WaitingForRoom: pipeline.WaitingForRoom,
		Retired: pipeline.Retired, ResetRequested: pipeline.ResetRequested, ResetValue: pipeline.ResetValue, Free: pipeline.Free,
		Bytes: 2 * len(synthesizer.Samples), Done: synthesizer.Done, Hold: synthesizer.Hold, Ready: synthesizer.Ready,
		Markers: synthesizer.MarkersPassed, Position: synthesizer.Position}
}

// replayStages stands in for the stages the Go pipeline doesn't have yet. A stage's call replays the calls of
// the framework the original's stage made, from the records, and returns what it returned; the parameter
// generator's also writes the frames and the ring's cursors the synthesizer then reads.
type replayStages struct {
	replay   *replay
	position int
	end      int
	room     bool
	done     bool
	hold     bool
	calls    []string
	err      error
}

func (stages *replayStages) fail(format string, arguments ...any) {
	if stages.err == nil {
		stages.err = fmt.Errorf(format, arguments...)
	}
}

// skip returns the index after the synthesizer's step or the output stage's call at index, or index when the
// record isn't one.
func (stages *replayStages) skip(index int) int {
	record := stages.replay.records[index]
	switch {
	case record.Kind == "retire":
		end, err := stages.replay.exit(index, "retired")
		if err != nil {
			stages.fail("%v", err)
			return stages.end
		}
		if end+1 < stages.end && stages.replay.records[end+1].Kind == "step" {
			end++
		}
		return end + 1
	case record.Kind == "frame" || record.Kind == "cursors" || record.Kind == "event":
		return index + 1
	case record.Kind == "step" && (record.Stage == "synthesizer" || record.Stage == "output"):
		return index + 1
	}
	return index
}

func (stages *replayStages) run(stage string) int64 {
	stages.calls = append(stages.calls, stage)
	replay := stages.replay
	for stages.err == nil && stages.position < stages.end {
		if next := stages.skip(stages.position); next != stages.position {
			stages.position = next
			continue
		}
		record := replay.records[stages.position]
		if record.Kind == "step" {
			if record.Stage != stage {
				stages.fail("record %d: the Go scheduler ran the %s stage, the original the %s stage", stages.position, stage, record.Stage)
				return 0
			}
			if replay.pipeline.Free != record.Free {
				stages.fail("record %d: %d free nodes after the %s stage, expected %d", stages.position, replay.pipeline.Free, stage, record.Free)
			}
			stages.position++
			return *record.Result
		}
		end, err := replay.replayRecord(stages.position)
		if err != nil {
			stages.fail("record %d (%s): %v", stages.position, record.Kind, err)
			return 0
		}
		stages.position = end + 1
	}
	stages.fail("the Go scheduler ran the %s stage, the original none", stage)
	return 0
}

func (stages *replayStages) Input() bool      { return stages.run("input") != 0 }
func (stages *replayStages) Normalizer() bool { return stages.run("normalizer") != 0 }
func (stages *replayStages) Lexical() bool    { return stages.run("lexical") != 0 }
func (stages *replayStages) Prosody() bool    { return stages.run("prosody") != 0 }
func (stages *replayStages) Room() bool       { return stages.room }

func (stages *replayStages) Reset() {
	stages.replay.pipeline.Synthesizer.Lookahead = 2
}

// Parameters replays the parameter generator's step, then writes the frames the synthesizer converts in this
// step of the scheduler and sets the ring's cursors as the first of the synthesizer's steps found them.
func (stages *replayStages) Parameters() int {
	result := int(stages.run("parameters"))
	if stages.err != nil {
		return result
	}
	ring := stages.replay.pipeline.Synthesizer
	cursors := false
	for index := stages.position; index < stages.end; index++ {
		record := stages.replay.records[index]
		switch record.Kind {
		case "frame":
			tracks, err := hex.DecodeString(record.Tracks)
			if err != nil || len(tracks) != len(synthesizer.Frame{}) {
				stages.fail("record %d: tracks %q", index, record.Tracks)
				return result
			}
			slot := record.Position & (synthesizer.RingLength - 1)
			copy(ring.Frames[slot][:], tracks)
			ring.Markers[slot] = ring.Markers[slot] || record.Marker
			ring.Tones[slot] = ring.Tones[slot] || record.Tone
		case "cursors":
			if cursors {
				continue
			}
			cursors = true
			before := record.Before
			if ring.Position != before.Position && !ring.Rebase() {
				stages.fail("record %d: position %d, expected %d", index, ring.Position, before.Position)
				return result
			}
			ring.End, ring.Written, ring.Lookahead = before.End, before.Written, before.Lookahead
			ring.Done, ring.Hold = before.Done != 0, before.Hold != 0
		}
	}
	if !cursors {
		ring.Done, ring.Hold = stages.done, stages.hold
	}
	stages.prepareRetires(stages.position)
	return result
}

// prepareRetires sets, for the output stage's calls from index on, what the stages wrote that the output
// stage reads: the settings of its cursor, the types of its window's nodes, the nodes of its control commands
// and the flags of the nodes it frees. It stops at a stage's step.
func (stages *replayStages) prepareRetires(index int) {
	replay := stages.replay
	for ; index < stages.end && stages.err == nil; index++ {
		record := replay.records[index]
		if record.Kind == "step" && record.Stage != "synthesizer" && record.Stage != "output" {
			return
		}
		if record.Kind != "retire" {
			continue
		}
		end, err := replay.exit(index, "retired")
		if err != nil {
			stages.fail("%v", err)
			return
		}
		cursor := &replay.pipeline.Cursors[Output]
		fields := cursorFields(cursor)
		copy(fields[7:], record.Fields[7:])
		replay.setCursor(cursor, fields)
		node := replay.node(record.Fields[0])
		for position := range len(record.Types) {
			if node == nil {
				stages.fail("record %d: the window is shorter than its types", index)
				return
			}
			node.Data[0] = node.Data[0]&^typeBits | record.Types[position] - '0'
			node = node.Next
		}
		for nested := index + 1; nested < end; nested++ {
			switch nestedRecord := replay.records[nested]; nestedRecord.Kind {
			case "control":
				node, err := decodeNode(nestedRecord.Node)
				if err == nil {
					err = replay.setNode(node)
				}
				if err != nil {
					stages.fail("record %d: %v", nested, err)
					return
				}
			case "free":
				replay.pipeline.Node(nodeIndex(nestedRecord)).SetFlags(nestedRecord.FlagsBefore)
			}
		}
		index = end
	}
}

// replaySchedule runs the Go thread's attributes and scheduler for the original's call at index, with the
// engine's attributes as the original's thread found them and the setter calls it made before, and compares
// what they ran, reported and left.
func (replay *replay) replaySchedule(index int, stages *replayStages, applied []goldenRecord) (int, int, error) {
	pipeline := replay.pipeline
	synthesizer := pipeline.Synthesizer
	end, err := replay.exit(index, "scheduled")
	if err != nil {
		return index, 0, err
	}
	state, expected, err := decodeScheduleState(replay.records[index].State)
	if err != nil {
		return end, 0, err
	}
	pipeline.OutOfInput = expected.OutOfInput
	synthesizer.Hold = expected.Hold
	if state.Engine == nil {
		return end, 0, fmt.Errorf("no engine recorded")
	}
	replay.setEngine(state.Engine)
	if err := replay.applyAttributes(applied); err != nil {
		return end, 0, err
	}
	if err := replay.compareThread(state); err != nil {
		return end, 0, fmt.Errorf("after the attributes: %v", err)
	}
	if actual := replay.scheduleFlags(); actual != expected {
		return end, 0, fmt.Errorf("before: %+v, expected %+v", actual, expected)
	}
	exitState, expectedAfter, err := decodeScheduleState(replay.records[end].State)
	if err != nil {
		return end, 0, err
	}
	*stages = replayStages{replay: replay, position: index + 1, end: end,
		room: state.Margin+state.Highest-state.Position+state.Waiting < 256, done: expectedAfter.Done, hold: expectedAfter.Hold}
	stages.prepareRetires(index + 1)
	result := pipeline.Step()
	if stages.err != nil {
		return end, 0, stages.err
	}
	for position := stages.position; position < end; {
		next := stages.skip(position)
		if next == position {
			return end, 0, fmt.Errorf("record %d: the original ran the %s stage after %v, the Go scheduler didn't", position, replay.records[position].Stage, stages.calls)
		}
		position = next
	}
	var lastRetired *goldenRecord
	for position := index + 1; position < end; position++ {
		if replay.records[position].Kind == "retired" {
			lastRetired = &replay.records[position]
		}
	}
	if int64(result) != *replay.records[end].Result {
		return end, 0, fmt.Errorf("Step reported %d after %v, expected %d", result, stages.calls, *replay.records[end].Result)
	}
	if actual := replay.scheduleFlags(); actual != expectedAfter {
		return end, 0, fmt.Errorf("after %v: %+v, expected %+v", stages.calls, actual, expectedAfter)
	}
	if err := replay.compareThread(exitState); err != nil {
		return end, 0, fmt.Errorf("after %v: %v", stages.calls, err)
	}
	if lastRetired != nil {
		if actual := cursorFields(&pipeline.Cursors[Output]); !slices.Equal(actual, lastRetired.Fields) {
			return end, 0, fmt.Errorf("after %v: the output cursor is %v, expected %v", stages.calls, actual, lastRetired.Fields)
		}
	}
	return end, result, nil
}

// compareInitialized compares the Go pipeline after Initialize with the original's: the escape parser's
// settings and track levels, the track levels, the cursors, the scheduler's flags and the ring's cursors.
func (replay *replay) compareInitialized(record goldenRecord) error {
	pipeline := replay.pipeline
	actualSettings := replay.settingFields()
	var expectedSettings []int32
	if err := json.Unmarshal(record.Settings, &expectedSettings); err != nil {
		return err
	}
	if !slices.Equal(actualSettings, expectedSettings) {
		return fmt.Errorf("settings %v, expected %v", actualSettings, expectedSettings)
	}
	if actual := hex.EncodeToString(pipeline.SettingsLevels[:]); actual != record.SettingsLevels {
		return fmt.Errorf("the settings' track levels are %s, expected %s", actual, record.SettingsLevels)
	}
	if actual := hex.EncodeToString(pipeline.Levels[:]); actual != record.Levels {
		return fmt.Errorf("the track levels are %s, expected %s", actual, record.Levels)
	}
	for stage := range pipeline.Cursors {
		if actual := cursorFields(&pipeline.Cursors[stage]); !slices.Equal(actual, record.Cursors[stage]) {
			return fmt.Errorf("cursor %d is %v, expected %v", stage, actual, record.Cursors[stage])
		}
	}
	state, expected, err := decodeScheduleState(record.State)
	if err != nil {
		return err
	}
	if actual := replay.scheduleFlags(); actual != expected {
		return fmt.Errorf("%+v, expected %+v", actual, expected)
	}
	if err := replay.compareThread(state); err != nil {
		return err
	}
	ring := pipeline.Synthesizer
	actualRing := goldenCursors{Position: ring.Position, End: ring.End, Written: ring.Written, Lookahead: ring.Lookahead,
		Done: boolByte(ring.Done), Hold: boolByte(ring.Hold), Ready: boolByte(ring.Ready), Markers: ring.MarkersPassed,
		Bytes: 2 * len(ring.Samples)}
	if actualRing != *record.Ring {
		return fmt.Errorf("ring %+v, expected %+v", actualRing, *record.Ring)
	}
	return nil
}

func boolByte(value bool) byte {
	if value {
		return 1
	}
	return 0
}

// TestScheduler runs the Go speaking thread's start, attributes and hand-overs, and the Go scheduler, output
// stage and synthesizer, for every call of the original's scheduler in the recorded runs, with the other
// stages replayed from the records: it checks the attributes the thread applies, the stages the scheduler
// runs, what it reports and the flags, free nodes, ring cursors and output cursor it leaves, that the audio
// handed over is the original's PCM, and that the bookmarks and words come where the original's did.
func TestScheduler(t *testing.T) {
	pack := conformance.Pack(t)
	tables, err := pack.LoadPipeline()
	if err != nil {
		t.Fatal(err)
	}
	synthesizerTables, err := pack.LoadSynthesizer()
	if err != nil {
		t.Fatal(err)
	}
	cases := conformance.ReadLines[goldenCase](t, "pipeline.jsonl.gz")
	counts := map[string]int{}
	failures, steps, handOvers := 0, 0, 0
	for _, golden := range cases {
		err := replayScheduler(tables, synthesizerTables, golden, counts, &steps, &handOvers)
		if err != nil {
			failures++
			if failures <= 10 {
				t.Errorf("%s: %v", golden.Name, err)
			}
		}
	}
	if failures > 0 || steps == 0 {
		t.Errorf("%d of %d cases differ", failures, len(cases))
	}
	t.Logf("%d cases: %d scheduler steps, %d hand-overs", len(cases), steps, handOvers)
}

func replayScheduler(tables *datapack.Pipeline, synthesizerTables *datapack.Synthesizer, golden goldenCase, counts map[string]int, steps, handOvers *int) error {
	rate, bits := golden.Format[0], golden.Format[1]
	stages := &replayStages{}
	replay := &replay{texts: map[int]*Text{}, records: golden.Records, counts: counts, racy: golden.Racy}
	engine := &Engine{}
	replay.pipeline = New(tables, synthesizer.New(synthesizerTables, rate), stages, engine)
	stages.replay = replay
	var applied []goldenRecord
	for index := 0; index < len(golden.Records); index++ {
		record := golden.Records[index]
		switch record.Kind {
		case "initialized":
			state, _, err := decodeScheduleState(record.State)
			if err != nil {
				return fmt.Errorf("record %d: %v", index, err)
			}
			if state.Engine == nil {
				return fmt.Errorf("record %d: no engine recorded", index)
			}
			engine.SampleRate, engine.Gain = record.EngineRate, record.EngineGain
			replay.setEngine(state.Engine)
			replay.pipeline.Start()
			if err := replay.compareInitialized(record); err != nil {
				return fmt.Errorf("record %d: after Start: %v", index, err)
			}
		case "attribute":
			applied = append(applied, record)
		case "schedule":
			end, result, err := replay.replaySchedule(index, stages, applied)
			if err != nil {
				return fmt.Errorf("record %d: %v", index, err)
			}
			applied = nil
			if result&StepHandOver != 0 {
				replay.pipeline.HandOver()
				*handOvers++
			}
			*steps++
			index = end
		case "feed", "reset", "reset-nodes":
		default:
			return fmt.Errorf("record %d: a %s record outside the scheduler", index, record.Kind)
		}
	}
	return compareQueue(engine.Queue, golden, bits)
}

// compareQueue compares the audio the Go thread queued with the original's PCM, and the bookmarks and words
// queued with it with the ones the original reported, at the sample where each was queued. A bookmark of 0,
// a text's end, is reported as the text done, whose position the recordings don't keep.
func compareQueue(queue []Item, golden goldenCase, bits int) error {
	var pcm []byte
	var actual, expected []string
	position := 0
	for _, item := range queue {
		if item.Event == nil {
			pcm = synthesizer.AppendPCM(pcm, item.Samples, bits)
			position += len(item.Samples)
			continue
		}
		event := item.Event
		switch {
		case event.Kind == WordPosition:
			actual = append(actual, fmt.Sprint(position, " word ", event.Text.Number, " ", event.Value))
		case event.Kind == Mark && event.Value != 0:
			actual = append(actual, fmt.Sprint(position, " mark ", event.Text.Number, " ", event.Value))
		}
	}
	for _, event := range golden.Events {
		if event[0] != nil && (event[1] == "word" || event[1] == "mark") {
			expected = append(expected, fmt.Sprint(int(event[0].(float64)), " ", event[1], " ", int(event[2].(float64)), " ", uint32(event[3].(float64))))
		}
	}
	if digest := sha256.Sum256(pcm); hex.EncodeToString(digest[:]) != golden.Digest {
		return fmt.Errorf("the PCM differs (%d bytes)", len(pcm))
	}
	if !slices.Equal(actual, expected) {
		return fmt.Errorf("events %v, expected %v", actual, expected)
	}
	return nil
}
