package pipeline

// What Retire reports.
const (
	// RetireWaiting: it stopped at a segment whose marker frame the synthesizer hasn't passed.
	RetireWaiting = iota
	// RetireStopped: a control command said to stop.
	RetireStopped
	// RetireDone: it went through the window up to its last node.
	RetireDone
)

// Retire is the output stage: it frees the nodes the synthesizer has spoken. It goes through its window,
// runs the control commands, passes one segment for each marker frame the synthesizer passed, and frees the
// nodes before the segment it stops at (pipeline.md §5).
func (pipeline *Pipeline) Retire() int {
	cursor := &pipeline.Cursors[Output]
	if pipeline.Begin(cursor) {
		for cursor.Scan != nil && cursor.Scan != cursor.End {
			switch cursor.Scan.Type() {
			case Control:
				if !pipeline.ExecuteControl() {
					return pipeline.retireUpTo(cursor.Scan, RetireStopped)
				}
			case Segment:
				if pipeline.Synthesizer.MarkersPassed == 0 {
					return pipeline.retireUpTo(cursor.Scan, RetireWaiting)
				}
				pipeline.Synthesizer.MarkersPassed--
			}
			cursor.Scan = pipeline.Next(cursor.Scan)
		}
		cursor.First, cursor.Done = cursor.Scan, cursor.Scan
	}
	pipeline.Finish()
	return RetireDone
}

func (pipeline *Pipeline) retireUpTo(node *Node, result int) int {
	cursor := &pipeline.Cursors[Output]
	cursor.First, cursor.Done = node, node
	pipeline.Finish()
	return result
}
