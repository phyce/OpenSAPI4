package pipeline

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/phyce/OpenSAPI4/truvoice/internal/conformance"
	"github.com/phyce/OpenSAPI4/truvoice/synthesizer"
)

type goldenNode struct {
	Index int    `json:"index"`
	Data  string `json:"data"`
	Text  *int   `json:"text"`
}

type goldenEngine struct {
	Pitch     uint16 `json:"pitch"`
	Speed     uint32 `json:"speed"`
	Volume    uint32 `json:"volume"`
	Character uint32 `json:"character"`
	Voice     int32  `json:"voice"`
}

type goldenControlState struct {
	Pitch      uint32        `json:"pitch"`
	Speed      uint32        `json:"speed"`
	Volume     uint32        `json:"volume"`
	Character  uint32        `json:"character"`
	Voice      int16         `json:"voice"`
	Mute       byte          `json:"mute"`
	Reset      byte          `json:"reset"`
	ResetValue uint32        `json:"resetValue"`
	SavedRate  int32         `json:"savedRate"`
	Level      *byte         `json:"level"`
	Engine     *goldenEngine `json:"engine"`
}

type goldenRecord struct {
	Kind        string          `json:"kind"`
	At          int             `json:"at"`
	After       json.RawMessage `json:"after"`
	Type        uint32          `json:"type"`
	Symbol      byte            `json:"symbol"`
	Stale       *goldenNode     `json:"stale"`
	Node        json.RawMessage `json:"node"`
	Direction   uint32          `json:"direction"`
	Result      *int64          `json:"result"`
	FlagsBefore uint32          `json:"flagsBefore"`
	Flags       uint32          `json:"flags"`
	Cursor      *int            `json:"cursor"`
	Current     *int            `json:"current"`
	Window      *[2]int         `json:"window"`
	WindowAfter *[2]int         `json:"windowAfter"`
	Free        int             `json:"free"`
	Fields      []int64         `json:"fields"`
	Types       string          `json:"types"`
	Next        []int64         `json:"next"`
	State       json.RawMessage `json:"state"`
	Text        *int            `json:"text"`
	Value       uint32          `json:"value"`
	Stage       string          `json:"stage"`
	Nodes       []goldenNode    `json:"nodes"`
	Markers     int32           `json:"markers"`
	Position    int32           `json:"position"`
	Tracks      string          `json:"tracks"`
	Marker      bool            `json:"marker"`
	Tone        bool            `json:"tone"`
	Before      *goldenCursors  `json:"before"`

	Settings       json.RawMessage `json:"settings"`
	SettingsLevels string          `json:"settingsLevels"`
	Levels         string          `json:"levels"`
	Cursors        [][]int64       `json:"cursors"`
	Ring           *goldenCursors  `json:"ring"`
}

type goldenCase struct {
	Name    string         `json:"name"`
	Format  [2]int         `json:"format"`
	Digest  string         `json:"digest"`
	Records []goldenRecord `json:"records"`
}

// replay runs the framework calls of one recorded run on a Go pipeline. It mirrors the list by replaying
// every NewNode and FreeNode; the stages' own writes (node contents, cursor fields, the attributes) are taken
// from the records before each call.
type replay struct {
	pipeline *Pipeline
	texts    map[int]*Text
	records  []goldenRecord
	counts   map[string]int
}

func (replay *replay) node(reference int64) *Node {
	if reference < 0 {
		return nil
	}
	return replay.pipeline.Node(int(reference))
}

func reference(node *Node) int64 {
	if node == nil {
		return -1
	}
	return int64(node.Index())
}

func (replay *replay) text(identifier *int) *Text {
	if identifier == nil {
		return nil
	}
	if replay.texts[*identifier] == nil {
		replay.texts[*identifier] = &Text{Number: *identifier}
	}
	return replay.texts[*identifier]
}

func (replay *replay) setNode(golden goldenNode) error {
	data, err := hex.DecodeString(golden.Data)
	if err != nil || len(data) != 24 {
		return fmt.Errorf("node data %q", golden.Data)
	}
	if golden.Text == nil && slices.ContainsFunc(data[20:], func(value byte) bool { return value != 0 }) {
		return fmt.Errorf("node %d holds % x at 0x1c, not a text", golden.Index, data[20:])
	}
	node := replay.pipeline.Node(golden.Index)
	copy(node.Data[:], data)
	node.Text = replay.text(golden.Text)
	return nil
}

func (replay *replay) compareNode(golden goldenNode) error {
	data, err := hex.DecodeString(golden.Data)
	if err != nil || len(data) != 24 {
		return fmt.Errorf("node data %q", golden.Data)
	}
	node := replay.pipeline.Node(golden.Index)
	if !slices.Equal(node.Data[:], data[:20]) || node.Text != replay.text(golden.Text) {
		return fmt.Errorf("node %d is % x text %v, expected % x text %v", golden.Index, node.Data, node.Text, data[:20], golden.Text)
	}
	return nil
}

func decodeNode(raw json.RawMessage) (goldenNode, error) {
	var node goldenNode
	err := json.Unmarshal(raw, &node)
	return node, err
}

func (replay *replay) setCursor(cursor *Cursor, fields []int64) {
	cursor.Start, cursor.Done, cursor.Scan = replay.node(fields[0]), replay.node(fields[1]), replay.node(fields[2])
	cursor.First, cursor.End = replay.node(fields[3]), replay.node(fields[4])
	cursor.Working = [2]*Node{replay.node(fields[5]), replay.node(fields[6])}
	cursor.Settings = Settings{PhoneticInput: int32(fields[7]), Flush: int32(fields[8]), Rate: int32(fields[9]),
		Pitch: int32(fields[10]), Attenuation: int32(fields[11]), Filter: int32(fields[12]), Flags: uint32(fields[13]),
		InputFlags: uint32(fields[14]), Voice: int32(fields[16])}
	cursor.Types = uint32(fields[15])
}

// setTypes gives the cursor's nodes, from its start to its end, the types the stages gave them.
func (replay *replay) setTypes(cursor *Cursor, types string) error {
	node := cursor.Start
	for position := range len(types) {
		if node == nil {
			return fmt.Errorf("the window has %d nodes, expected %d", position, len(types))
		}
		node.Data[0] = node.Data[0]&^typeBits | types[position] - '0'
		if node == cursor.End {
			break
		}
		node = node.Next
	}
	return nil
}

func cursorFields(cursor *Cursor) []int64 {
	return []int64{reference(cursor.Start), reference(cursor.Done), reference(cursor.Scan), reference(cursor.First),
		reference(cursor.End), reference(cursor.Working[0]), reference(cursor.Working[1]), int64(cursor.PhoneticInput),
		int64(cursor.Flush), int64(cursor.Rate), int64(cursor.Pitch), int64(cursor.Attenuation), int64(cursor.Filter),
		int64(cursor.Flags), int64(cursor.InputFlags), int64(cursor.Types), int64(cursor.Voice)}
}

func (replay *replay) setCurrent(index *int, window *[2]int) {
	pipeline := replay.pipeline
	pipeline.Current = nil
	if index == nil || *index < 0 {
		return
	}
	pipeline.Current = &pipeline.Cursors[*index]
	if window != nil {
		pipeline.Current.Start, pipeline.Current.End = replay.node(int64(window[0])), replay.node(int64(window[1]))
	}
}

func (replay *replay) compareWindow(window *[2]int) error {
	current := replay.pipeline.Current
	if window == nil {
		return nil
	}
	if actual := [2]int{int(reference(current.Start)), int(reference(current.End))}; actual != *window {
		return fmt.Errorf("window %v, expected %v", actual, *window)
	}
	return nil
}

func (replay *replay) controlState() goldenControlState {
	pipeline := replay.pipeline
	state := goldenControlState{Pitch: pipeline.Pitch, Speed: pipeline.Speed, Volume: pipeline.Volume,
		Character: pipeline.Character, Voice: int16(pipeline.Synthesizer.OutputVoice), ResetValue: pipeline.ResetValue,
		SavedRate: pipeline.SavedRate}
	if pipeline.Synthesizer.Mute {
		state.Mute = 1
	}
	if pipeline.ResetRequested {
		state.Reset = 1
	}
	if engine := pipeline.Engine; engine != nil {
		state.Engine = &goldenEngine{engine.Pitch, engine.Speed, engine.Volume, engine.Character, engine.Voice}
	}
	return state
}

func (replay *replay) setControlState(state goldenControlState, node *Node) {
	pipeline := replay.pipeline
	pipeline.Pitch, pipeline.Speed, pipeline.Volume, pipeline.Character = state.Pitch, state.Speed, state.Volume, state.Character
	pipeline.Synthesizer.OutputVoice, pipeline.Synthesizer.Mute = int(state.Voice), state.Mute != 0
	pipeline.ResetRequested, pipeline.ResetValue, pipeline.SavedRate = state.Reset != 0, state.ResetValue, state.SavedRate
	if state.Level != nil {
		pipeline.Levels[node.Value()&0xff] = *state.Level
	}
	if state.Engine != nil {
		pipeline.Engine.Pitch, pipeline.Engine.Speed, pipeline.Engine.Volume = state.Engine.Pitch, state.Engine.Speed, state.Engine.Volume
		pipeline.Engine.Character, pipeline.Engine.Voice = state.Engine.Character, state.Engine.Voice
	}
}

func (replay *replay) compareControlState(raw json.RawMessage, node *Node) error {
	var expected goldenControlState
	if err := json.Unmarshal(raw, &expected); err != nil {
		return err
	}
	actual := replay.controlState()
	if expected.Level != nil {
		level := replay.pipeline.Levels[node.Value()&0xff]
		actual.Level = &level
		if *expected.Level != level {
			return fmt.Errorf("level %d, expected %d", level, *expected.Level)
		}
	}
	actualJSON, _ := json.Marshal(actual)
	expectedJSON, _ := json.Marshal(expected)
	if string(actualJSON) != string(expectedJSON) {
		return fmt.Errorf("state %s, expected %s", actualJSON, expectedJSON)
	}
	return nil
}

// exit finds the record that ends the call whose entry record is at index.
func (replay *replay) exit(index int, kind string) (int, error) {
	for end := index + 1; end < len(replay.records); end++ {
		if replay.records[end].Kind == kind {
			return end, nil
		}
	}
	return 0, fmt.Errorf("record %d: no %s", index, kind)
}

// prepare sets what the calls nested between index and end read that the stages wrote: the nodes their
// control commands run on, and the nodes they take from the free list.
func (replay *replay) prepare(index, end int) error {
	first := true
	for nested := index + 1; nested < end; nested++ {
		record := replay.records[nested]
		switch record.Kind {
		case "control":
			node, err := decodeNode(record.Node)
			if err != nil {
				return err
			}
			if err := replay.setNode(node); err != nil {
				return err
			}
			if first {
				var state goldenControlState
				if err := json.Unmarshal(record.State, &state); err != nil {
					return err
				}
				replay.setControlState(state, replay.pipeline.Node(node.Index))
				first = false
			}
		case "new":
			if err := replay.setNode(*record.Stale); err != nil {
				return err
			}
		case "free":
			replay.pipeline.Node(nodeIndex(record)).SetFlags(record.FlagsBefore)
		}
	}
	return nil
}

func nodeIndex(record goldenRecord) int {
	var index int
	json.Unmarshal(record.Node, &index)
	return index
}

// check compares what the calls nested between index and end left with what they recorded: the nodes they
// took, the events, the attributes, the free count.
func (replay *replay) check(index, end, queued int) error {
	var events []goldenRecord
	free := -1
	var state json.RawMessage
	var stateNode *Node
	for nested := index + 1; nested < end; nested++ {
		record := replay.records[nested]
		switch record.Kind {
		case "new":
			node, err := decodeNode(record.Node)
			if err != nil {
				return err
			}
			if err := replay.compareNode(node); err != nil {
				return fmt.Errorf("record %d: %v", nested, err)
			}
			free = record.Free
		case "free":
			if flags := replay.pipeline.Node(nodeIndex(record)).Flags(); flags != record.Flags {
				return fmt.Errorf("record %d: flags %#x, expected %#x", nested, flags, record.Flags)
			}
			free = record.Free
		case "event":
			events = append(events, record)
		case "controlled":
			node, err := decodeNode(record.Node)
			if err != nil {
				return err
			}
			if freed := replay.freedAfter(nested, end, node.Index); freed != nil {
				data, err := hex.DecodeString(node.Data)
				if err != nil || len(data) != 24 {
					return fmt.Errorf("node data %q", node.Data)
				}
				binary.LittleEndian.PutUint32(data, freed.Flags)
				node.Data = hex.EncodeToString(data)
			}
			if err := replay.compareNode(node); err != nil {
				return fmt.Errorf("record %d: %v", nested, err)
			}
			state, stateNode = record.State, replay.pipeline.Node(node.Index)
		}
	}
	if state != nil {
		if err := replay.compareControlState(state, stateNode); err != nil {
			return err
		}
	}
	if free >= 0 && replay.pipeline.Free != free {
		return fmt.Errorf("%d free nodes, expected %d", replay.pipeline.Free, free)
	}
	queue := replay.pipeline.Engine.Queue[queued:]
	if len(queue) != len(events) {
		return fmt.Errorf("%d events, expected %d", len(queue), len(events))
	}
	for position, event := range events {
		actual := queue[position].Event
		if int(actual.Kind) != int(event.Type) || actual.Text != replay.text(event.Text) || actual.Value != event.Value {
			return fmt.Errorf("event %+v, expected %+v", *actual, event)
		}
	}
	return nil
}

// freedAfter returns the record of the node's freeing after index and before end, if any: the output stage
// frees the nodes of the commands it runs.
func (replay *replay) freedAfter(index, end, node int) *goldenRecord {
	for nested := index + 1; nested < end; nested++ {
		if record := &replay.records[nested]; record.Kind == "free" && nodeIndex(*record) == node {
			return record
		}
	}
	return nil
}

func (replay *replay) checkList(nodes []goldenNode, stage string) error {
	if len(nodes) == 0 {
		return nil
	}
	node := replay.pipeline.Node(nodes[0].Index)
	for position, golden := range nodes {
		if node == nil || node.Index() != golden.Index {
			return fmt.Errorf("%s: node %d of %d is %d in the list, expected %d", stage, position, len(nodes), reference(node), golden.Index)
		}
		node = node.Next
	}
	return nil
}

func (replay *replay) run() error {
	for index := 0; index < len(replay.records); index++ {
		end, err := replay.replayRecord(index)
		if err != nil {
			return fmt.Errorf("record %d (%s): %v", index, replay.records[index].Kind, err)
		}
		index = end
	}
	return nil
}

// replayRecord replays the call whose entry record is at index and returns the index of its exit record.
func (replay *replay) replayRecord(index int) (int, error) {
	pipeline := replay.pipeline
	record := replay.records[index]
	replay.counts[record.Kind]++
	var err error
	switch record.Kind {
	case "reset-nodes":
		pipeline.ResetNodes()
		if pipeline.First().Next != pipeline.Last() || pipeline.Free != NodeCount {
			err = fmt.Errorf("the list isn't empty after a reset")
		}
	case "new":
		err = replay.replayNew(record)
	case "free":
		err = replay.replayFree(record)
	case "begin":
		index, err = replay.replayBegin(index)
	case "finish":
		index, err = replay.replayFinish(index)
	case "control":
		index, err = replay.replayControl(index)
	case "retire":
		index, err = replay.replayRetire(index)
	case "input":
		last := pipeline.Last()
		for position := len(record.Nodes) - 1; position >= 0 && err == nil; position-- {
			last = last.Previous
			if last.Index() != record.Nodes[position].Index {
				err = fmt.Errorf("input node %d is %d, expected %d", position, last.Index(), record.Nodes[position].Index)
			}
		}
	case "step":
		if pipeline.Free != record.Free {
			err = fmt.Errorf("%d free nodes after the %s step, expected %d", pipeline.Free, record.Stage, record.Free)
		}
	}
	return index, err
}

func (replay *replay) replayNew(record goldenRecord) error {
	pipeline := replay.pipeline
	replay.setCurrent(record.Cursor, record.Window)
	if taken := pipeline.nodes[freeFirst].Next.Index(); taken != record.Stale.Index {
		return fmt.Errorf("the free list's first node is %d, expected %d", taken, record.Stale.Index)
	}
	if err := replay.setNode(*record.Stale); err != nil {
		return err
	}
	node := pipeline.NewNode(replay.node(int64(record.At)), string(record.After) == "1", int(record.Type), record.Symbol)
	golden, err := decodeNode(record.Node)
	if err != nil {
		return err
	}
	if node.Index() != golden.Index {
		return fmt.Errorf("took node %d, expected %d", node.Index(), golden.Index)
	}
	if err := replay.compareNode(golden); err != nil {
		return err
	}
	if pipeline.Free != record.Free {
		return fmt.Errorf("%d free nodes, expected %d", pipeline.Free, record.Free)
	}
	return replay.compareWindow(record.WindowAfter)
}

func (replay *replay) replayFree(record goldenRecord) error {
	pipeline := replay.pipeline
	replay.setCurrent(record.Cursor, record.Window)
	node := pipeline.Node(nodeIndex(record))
	node.SetFlags(record.FlagsBefore)
	result := pipeline.FreeNode(node, record.Direction == 1)
	if reference(result) != *record.Result {
		return fmt.Errorf("returned %d, expected %d", reference(result), *record.Result)
	}
	if node.Flags() != record.Flags || pipeline.Free != record.Free {
		return fmt.Errorf("flags %#x and %d free nodes, expected %#x and %d", node.Flags(), pipeline.Free, record.Flags, record.Free)
	}
	return replay.compareWindow(record.WindowAfter)
}

func (replay *replay) replayBegin(index int) (int, error) {
	pipeline := replay.pipeline
	record := replay.records[index]
	end, err := replay.exit(index, "begun")
	if err != nil {
		return index, err
	}
	cursor := &pipeline.Cursors[*record.Cursor]
	replay.setCursor(cursor, record.Fields)
	if err := replay.setTypes(cursor, record.Types); err != nil {
		return end, err
	}
	if err := replay.prepare(index, end); err != nil {
		return end, err
	}
	queued := len(pipeline.Engine.Queue)
	result := pipeline.Begin(cursor)
	exit := replay.records[end]
	if (*exit.Result != 0) != result || pipeline.stageOf(pipeline.Current) != *exit.Current {
		return end, fmt.Errorf("Begin returned %v with cursor %d current, expected %d and %d", result, pipeline.stageOf(pipeline.Current), *exit.Result, *exit.Current)
	}
	if actual := cursorFields(cursor); !slices.Equal(actual, exit.Fields) {
		return end, fmt.Errorf("cursor %v, expected %v", actual, exit.Fields)
	}
	return end, replay.check(index, end, queued)
}

func (replay *replay) replayFinish(index int) (int, error) {
	pipeline := replay.pipeline
	record := replay.records[index]
	end, err := replay.exit(index, "finished")
	if err != nil {
		return index, err
	}
	stage := *record.Cursor
	cursor := &pipeline.Cursors[stage]
	replay.setCursor(cursor, record.Fields)
	if stage < Output {
		next := &pipeline.Cursors[stage+1]
		next.Start, next.Done, next.Scan = replay.node(record.Next[0]), replay.node(record.Next[1]), replay.node(record.Next[2])
		next.First, next.End = replay.node(record.Next[3]), replay.node(record.Next[4])
	}
	pipeline.Current = cursor
	for nested := index + 1; nested < end; nested++ {
		if handoff := replay.records[nested]; handoff.Kind == "handoff" {
			if err := replay.checkList(handoff.Nodes, handoff.Stage); err != nil {
				return end, err
			}
		}
	}
	if err := replay.prepare(index, end); err != nil {
		return end, err
	}
	queued := len(pipeline.Engine.Queue)
	result := pipeline.Finish()
	exit := replay.records[end]
	if (*exit.Result != 0) != result || pipeline.Current != nil || *exit.Current != -1 {
		return end, fmt.Errorf("Finish returned %v, expected %d", result, *exit.Result)
	}
	if actual := cursorFields(cursor); !slices.Equal(actual, exit.Fields) {
		return end, fmt.Errorf("cursor %v, expected %v", actual, exit.Fields)
	}
	if stage < Output {
		if actual := cursorFields(&pipeline.Cursors[stage+1])[:5]; !slices.Equal(actual, exit.Next) {
			return end, fmt.Errorf("next cursor %v, expected %v", actual, exit.Next)
		}
	}
	return end, replay.check(index, end, queued)
}

func (replay *replay) replayRetire(index int) (int, error) {
	pipeline := replay.pipeline
	record := replay.records[index]
	end, err := replay.exit(index, "retired")
	if err != nil {
		return index, err
	}
	cursor := &pipeline.Cursors[Output]
	replay.setCursor(cursor, record.Fields)
	if err := replay.setTypes(cursor, record.Types); err != nil {
		return end, err
	}
	pipeline.Synthesizer.MarkersPassed = record.Markers
	if err := replay.prepare(index, end); err != nil {
		return end, err
	}
	queued := len(pipeline.Engine.Queue)
	result := pipeline.Retire()
	exit := replay.records[end]
	if int64(result) != *exit.Result || pipeline.Synthesizer.MarkersPassed != exit.Markers || pipeline.Current != nil {
		return end, fmt.Errorf("Retire returned %d with %d markers left, expected %d and %d", result, pipeline.Synthesizer.MarkersPassed, *exit.Result, exit.Markers)
	}
	if actual := cursorFields(cursor); !slices.Equal(actual, exit.Fields) {
		return end, fmt.Errorf("cursor %v, expected %v", actual, exit.Fields)
	}
	return end, replay.check(index, end, queued)
}

func (replay *replay) replayControl(index int) (int, error) {
	pipeline := replay.pipeline
	record := replay.records[index]
	end, err := replay.exit(index, "controlled")
	if err != nil {
		return index, err
	}
	cursor := &pipeline.Cursors[*record.Cursor]
	replay.setCursor(cursor, record.Fields)
	pipeline.Current = cursor
	if err := replay.prepare(index-1, end); err != nil {
		return end, err
	}
	queued := len(pipeline.Engine.Queue)
	result := pipeline.ExecuteControl()
	exit := replay.records[end]
	if (*exit.Result != 0) != result {
		return end, fmt.Errorf("ExecuteControl returned %v, expected %d", result, *exit.Result)
	}
	if actual := cursorFields(cursor); !slices.Equal(actual, exit.Fields) {
		return end, fmt.Errorf("cursor %v, expected %v", actual, exit.Fields)
	}
	return end, replay.check(index-1, end+1, queued)
}

// TestFramework replays every call of the original's node, cursor and control functions in the recorded runs
// on a Go pipeline and compares what each call returned and left: the nodes taken from the free list, the
// list, the cursors, the attributes, the events and the free count after each stage.
func TestFramework(t *testing.T) {
	pack := conformance.Pack(t)
	tables, err := pack.LoadPipeline()
	if err != nil {
		t.Fatal(err)
	}
	cases := conformance.ReadLines[goldenCase](t, "pipeline.jsonl.gz")
	counts := map[string]int{}
	failures := 0
	for _, golden := range cases {
		replay := &replay{pipeline: New(tables, new(synthesizer.Synthesizer), nil, &Engine{}), texts: map[int]*Text{},
			records: golden.Records, counts: counts}
		if err := replay.run(); err != nil {
			failures++
			if failures <= 10 {
				t.Errorf("%s: %v", golden.Name, err)
			}
		}
	}
	if failures > 0 {
		t.Errorf("%d of %d cases differ", failures, len(cases))
	}
	t.Logf("%d cases: %d new, %d free, %d begin, %d finish, %d control, %d retire (outside other calls), %d steps",
		len(cases), counts["new"], counts["free"], counts["begin"], counts["finish"], counts["control"], counts["retire"],
		counts["step"])
}
