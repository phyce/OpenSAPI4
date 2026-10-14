package pipeline

// The stages that have cursors, in the order nodes pass through them (pipeline.md §3).
const (
	Normalizer = iota
	Lexical
	Prosody
	Parameters
	Output
	StageCount
)

// typeMasks maps a node type to its bit in a cursor's Types.
var typeMasks = [8]uint32{0x10, 1, 2, 4, 8, 0x20, 0, 0}

// stageTypes are the node types each stage works on, set by its reset: normalizer 0-3, lexical 2-3, prosody 0,
// 3 and 4, parameters 4 and 5, output all.
var stageTypes = [StageCount]uint32{0x17, 6, 0x1c, 0x28, 0x3f}

// Settings are what the control commands set, from the defaults the escape parser keeps. Each cursor has its
// own copy, so that a stage applies a command when its nodes reach it (pipeline.md §3.1).
type Settings struct {
	PhoneticInput int32  // I
	Phrase        int32  // P
	Rate          int32  // r, v: the rate index
	Pitch         int32  // p: Hz
	Attenuation   int32  // a: dB
	Filter        int32  // f
	Flags         uint32 // N, F
	InputFlags    uint32 // A, D
	Voice         int32  // V
}

// A Cursor is a stage's window on the list: the nodes from Start to End the previous stage handed over, of
// which those before Done are finished, Scan the next node whose control command the stage hasn't run, and
// First the first node of the stage's Types. Working are two node pointers of the stage's own.
type Cursor struct {
	Start, Done, Scan, First, End *Node
	Working                       [2]*Node
	Settings
	Types uint32
}

// Takes reports whether the node is of one of the cursor's types.
func (cursor *Cursor) Takes(node *Node) bool {
	return cursor.Types&typeMasks[node.Type()] != 0
}

func (pipeline *Pipeline) stageOf(cursor *Cursor) int {
	for stage := range pipeline.Cursors {
		if cursor == &pipeline.Cursors[stage] {
			return stage
		}
	}
	return -1
}

// Begin makes the cursor current, moves First on to the first node of the cursor's types, and runs the
// control commands from Scan up to the first node of its types, until one says to stop. It reports whether
// the window has a node of its types (pipeline.md §3.2).
func (pipeline *Pipeline) Begin(cursor *Cursor) bool {
	pipeline.Current = cursor
	first := cursor.First
	for first != nil && !cursor.Takes(first) {
		first = pipeline.Next(first)
	}
	cursor.First = first
	for cursor.Scan != nil && !cursor.Takes(cursor.Scan) && pipeline.ExecuteControl() {
		scanned := cursor.Scan
		cursor.Scan = pipeline.Next(scanned)
		if cursor.Done == scanned {
			cursor.Done = cursor.Scan
		}
	}
	return first != nil
}

// Finish hands the current cursor's finished nodes, Start up to Done, to the next stage's cursor, or frees
// them at the output stage. A Done of nil means the whole window, which empties the cursor. Finish reports
// whether it handed anything over, and leaves no cursor current (pipeline.md §3.3).
func (pipeline *Pipeline) Finish() bool {
	cursor := pipeline.Current
	handed := cursor.Start != nil && cursor.Start != cursor.Done
	if handed {
		end := cursor.End
		if cursor.Done != nil {
			end = cursor.Done.Previous
		}
		if stage := pipeline.stageOf(cursor); stage == Output {
			for {
				cursor.Start = pipeline.FreeNode(cursor.Start, true)
				if cursor.Start == cursor.Done {
					break
				}
			}
		} else {
			next := &pipeline.Cursors[stage+1]
			next.End = end
			for _, field := range []**Node{&next.First, &next.Scan, &next.Done, &next.Start} {
				if *field == nil {
					*field = cursor.Start
				}
			}
		}
		if cursor.Done == nil {
			cursor.Start, cursor.Scan, cursor.First, cursor.End = nil, nil, nil, nil
		} else {
			cursor.Start = cursor.Done
		}
	}
	pipeline.Current = nil
	return handed
}
