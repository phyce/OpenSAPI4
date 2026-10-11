package synthesizer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"testing"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/internal/conformance"
)

// goldenStep is one call of the original's step: the samples it rendered, the frame it converted and its
// cursors before and after.
type goldenStep struct {
	samples, frame, cursors *goldenRecord
}

func splitSteps(t *testing.T, golden *goldenCase) (int, []goldenStep) {
	t.Helper()
	rate := 0
	var steps []goldenStep
	var pending goldenStep
	for index := range golden.Records {
		record := &golden.Records[index]
		switch record.Kind {
		case "reset":
			if index != 0 {
				t.Fatalf("%s: a reset at record %d", golden.Name, index)
			}
			rate = record.Rate
		case "samples":
			pending.samples = record
		case "frame":
			pending.frame = record
		case "cursors":
			pending.cursors = record
			steps = append(steps, pending)
			pending = goldenStep{}
		}
	}
	if rate == 0 || len(steps) == 0 || pending.samples != nil || pending.frame != nil {
		t.Fatalf("%s: no reset, no steps or records after the last step", golden.Name)
	}
	return rate, steps
}

// stops reports whether the original's loop stopped after a step that left these cursors.
func stops(cursors goldenCursors) bool {
	return cursors.Hold != 0 || cursors.Ready == 0 || cursors.Done != 0 || cursors.Bytes > HandOverBytes
}

func compareCursors(synthesizer *Synthesizer, expected goldenCursors) error {
	actual := goldenCursors{Position: synthesizer.Position, End: synthesizer.End, Written: synthesizer.Written,
		Lookahead: synthesizer.Lookahead, Done: boolByte(synthesizer.Done), Hold: boolByte(synthesizer.Hold),
		Ready: boolByte(synthesizer.Ready), Markers: synthesizer.MarkersPassed, Bytes: 2 * len(synthesizer.Samples)}
	if actual != expected {
		return fmt.Errorf("cursors %+v, expected %+v", actual, expected)
	}
	return nil
}

func boolByte(value bool) byte {
	if value {
		return 1
	}
	return 0
}

// TestSynthesizer replays the original's steps: before each run of its loop it sets what the other stages
// set (the frames, cursors, mute, output-filter voice and gain) and hands the buffer over where the original
// did, then checks the loop's steps, its samples and its hand-over, the whole PCM and that every event falls
// where a buffer was handed over.
func TestSynthesizer(t *testing.T) {
	tables, err := conformance.Pack(t).LoadSynthesizer()
	if err != nil {
		t.Fatal(err)
	}
	cases := conformance.ReadLines[goldenCase](t, "synthesizer.jsonl.gz")
	totalSteps, totalRuns, handOvers, failures := 0, 0, 0, 0
	for caseIndex := range cases {
		golden := &cases[caseIndex]
		err := replay(t, tables, golden, &totalSteps, &totalRuns, &handOvers)
		if err != nil {
			failures++
			if failures <= 10 {
				t.Errorf("%s: %v", golden.Name, err)
			}
		}
	}
	if failures > 0 || totalSteps == 0 {
		t.Errorf("%d of %d cases differ", failures, len(cases))
	}
	t.Logf("%d cases, %d steps in %d runs, %d hand-overs", len(cases), totalSteps, totalRuns, handOvers)
}

func replay(t *testing.T, tables *datapack.Synthesizer, golden *goldenCase, totalSteps, totalRuns, handOvers *int) error {
	rate, steps := splitSteps(t, golden)
	bits := golden.Format[1]
	synthesizer := New(tables, rate)
	synthesizer.Rendering = true
	var initial Frame
	if steps[0].frame == nil || steps[0].frame.Position != 0 {
		return fmt.Errorf("the first step converts no frame 0")
	}
	if tracks, err := hex.DecodeString(steps[0].frame.Tracks); err != nil || copy(initial[:], tracks) != len(initial) {
		return fmt.Errorf("tracks %q", steps[0].frame.Tracks)
	}
	synthesizer.ResetFrames(&initial)
	synthesizer.Lookahead = steps[0].cursors.Before.Lookahead
	if err := compareCursors(synthesizer, steps[0].cursors.Before); err != nil {
		return fmt.Errorf("after the reset: %v", err)
	}
	for _, step := range steps[:initialEnd] {
		if step.frame == nil || step.frame.Tracks != steps[0].frame.Tracks {
			return fmt.Errorf("the first %d frames aren't the initial frame", initialEnd)
		}
	}
	var pcm []byte
	handedOver := 0
	boundaries := map[int]bool{0: true}
	handOver := func() {
		if len(synthesizer.Samples) == 0 {
			return
		}
		pcm = AppendPCM(pcm, synthesizer.Samples, bits)
		handedOver += len(synthesizer.Samples)
		boundaries[handedOver] = true
		synthesizer.Samples = synthesizer.Samples[:0]
		*handOvers++
	}
	for start := 0; start < len(steps); {
		end := start + 1
		for end < len(steps) && !stops(steps[end-1].cursors.After) {
			end++
		}
		run := steps[start:end]
		before := run[0].cursors.Before
		if before.Bytes == 0 {
			handOver()
		}
		if synthesizer.Position != before.Position && !synthesizer.Rebase() {
			return fmt.Errorf("step %d: position %d, expected %d", start, synthesizer.Position, before.Position)
		}
		synthesizer.End, synthesizer.Written, synthesizer.Lookahead = before.End, before.Written, before.Lookahead
		synthesizer.Done, synthesizer.Hold = before.Done != 0, before.Hold != 0
		synthesizer.MarkersPassed = before.Markers
		if err := compareCursors(synthesizer, before); err != nil {
			return fmt.Errorf("step %d, before: %v", start, err)
		}
		var expected []int16
		for _, step := range run {
			if step.frame != nil {
				tracks, err := hex.DecodeString(step.frame.Tracks)
				if err != nil || len(tracks) != len(Frame{}) {
					return fmt.Errorf("step %d: tracks %q", start, step.frame.Tracks)
				}
				slot := step.frame.Position & (RingLength - 1)
				copy(synthesizer.Frames[slot][:], tracks)
				if synthesizer.Markers[slot] && !step.frame.Marker || synthesizer.Tones[slot] && !step.frame.Tone {
					return fmt.Errorf("step %d: frame %d's slot still has a marker or tone bit", start, step.frame.Position)
				}
				synthesizer.Markers[slot] = synthesizer.Markers[slot] || step.frame.Marker
				synthesizer.Tones[slot] = synthesizer.Tones[slot] || step.frame.Tone
			}
			if step.samples != nil {
				expected = append(expected, decodeWords(t, step.samples.Samples)...)
			}
		}
		apply := func(from int) {
			for _, step := range run[from:] {
				if step.frame != nil {
					synthesizer.Mute = step.frame.Mute != 0
					break
				}
			}
			for _, step := range run[from:] {
				if step.samples != nil {
					synthesizer.OutputVoice, synthesizer.Gain = step.samples.Filter, step.samples.Gain
					break
				}
			}
		}
		apply(0)
		var retireError error
		retire := func() {
			if !synthesizer.Ready {
				return
			}
			index := slices.IndexFunc(run, func(step goldenStep) bool {
				return step.cursors.After.Ready != 0 && step.cursors.After.Position == synthesizer.Position
			})
			if index < 0 || index == len(run)-1 {
				return
			}
			if err := compareCursors(synthesizer, run[index].cursors.After); err != nil && retireError == nil {
				retireError = fmt.Errorf("step %d, after: %v", start+index, err)
			}
			synthesizer.MarkersPassed = run[index+1].cursors.Before.Markers
			apply(index + 1)
		}
		renderedBefore := len(synthesizer.Samples)
		handedOverByRun := synthesizer.Run(retire)
		if retireError != nil {
			return retireError
		}
		after := run[len(run)-1].cursors.After
		synthesizer.MarkersPassed = after.Markers
		if err := compareCursors(synthesizer, after); err != nil {
			return fmt.Errorf("steps %d-%d, after: %v", start, end-1, err)
		}
		if !slices.Equal(synthesizer.Samples[renderedBefore:], expected) {
			return fmt.Errorf("steps %d-%d: the samples differ", start, end-1)
		}
		if end < len(steps) && after.Bytes > 0 {
			if expectedHandOver := steps[end].cursors.Before.Bytes == 0; handedOverByRun != expectedHandOver {
				return fmt.Errorf("steps %d-%d: hand-over %v, expected %v", start, end-1, handedOverByRun, expectedHandOver)
			}
		}
		*totalSteps += len(run)
		*totalRuns++
		start = end
	}
	handOver()
	if digest := sha256.Sum256(pcm); hex.EncodeToString(digest[:]) != golden.Digest {
		return fmt.Errorf("the PCM differs (%d bytes)", len(pcm))
	}
	for _, event := range golden.Events {
		if position, ok := event[0].(float64); ok && !boundaries[int(position)] {
			return fmt.Errorf("event %v isn't where a buffer was handed over", event)
		}
	}
	return nil
}
