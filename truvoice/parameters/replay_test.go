package parameters

import (
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/truvoice/internal/conformance"
	"github.com/phyce/OpenSAPI4/truvoice/internal/threadstate"
	"github.com/phyce/OpenSAPI4/truvoice/pipeline"
	"github.com/phyce/OpenSAPI4/truvoice/synthesizer"
)

// The parameter generator's golden data (golden/truvoice/parameters.jsonl.gz) records calls of the original's
// functions with the changes of the speaking thread's state around them. The replay rebuilds the state at every
// call and return (internal/threadstate), so that a Go function can start from the state the original's started
// from and leave what it left.

type (
	call     = threadstate.Call
	replayer = threadstate.Replayer
)

// The thread state's places the replay maps: the ring's, the generator's own and the settings.
const (
	stateDone           = 0x0
	stateWanted         = 0x38
	stateMarkers        = 0x14
	stateSegments       = 0x144
	segmentSize         = 0x1c
	stateFrames         = 0x3ac
	stateTones          = 0x1fbd
	stateCoarticulation = 0x2034
	stateSettings       = 0x1a28
	stateMute           = 0x2075
	stateSegmentFrame   = 0x1ff6
)

// ringOf returns the track whose ring an argument points to.
func ringOf(call *call, index int) (int, error) {
	offset := int64(call.Record.Arguments[index]) - int64(call.Base) - stateFrames
	if offset < 0 || offset%synthesizer.RingLength != 0 || offset/synthesizer.RingLength >= datapack.TrackCount {
		return 0, fmt.Errorf("argument %d isn't a ring: %08x", index, call.Record.Arguments[index])
	}
	return int(offset / synthesizer.RingLength), nil
}

// replay hands each recorded call of the functions given to their replayer, reports the calls that differ (a
// few per function) and returns how many calls each replayed.
func replay(t *testing.T, replayers map[string]replayer) map[string]int {
	t.Helper()
	return threadstate.Replay(t, "parameters.jsonl.gz", replayers)
}

// generatorFields lists the generator's values with their places in the thread state, and with their names
// when named.
func generatorFields(generator *Generator, named bool) []threadstate.Field {
	label := func(format string, arguments ...any) string {
		if !named {
			return ""
		}
		return fmt.Sprintf(format, arguments...)
	}
	ring := generator.Synthesizer
	fields := []threadstate.Field{threadstate.At(0x4, "position", &ring.Position), threadstate.At(0x8, "end", &ring.End),
		threadstate.At(0xc, "written", &ring.Written), threadstate.At(0x10, "furthest", &generator.Furthest),
		threadstate.At(0x1fe0, "lookahead", &ring.Lookahead), threadstate.At(0x1fe4, "margin", &generator.Margin),
		threadstate.At(stateWanted, "wanted", &generator.Wanted), threadstate.At(0x1ff0, "segment left", &generator.SegmentLeft),
		threadstate.At(0x200c, "tone frequency", &generator.ToneFrequency), threadstate.At(0x1e24, "previous category", &generator.PreviousCategory),
		threadstate.At(0x1e28, "category", &generator.Category), threadstate.At(0x1e2c, "previous frontness", &generator.PreviousFrontness),
		threadstate.At(0x1e30, "frontness", &generator.Frontness), threadstate.At(0x1e34, "aspiration shift", &generator.AspirationShift),
		threadstate.At(0x1e38, "voice onset", &generator.VoiceOnset), threadstate.At(0x1e3c, "burst frames", &generator.BurstFrames),
		threadstate.At(0x1e40, "voicing shift", &generator.VoicingShift), threadstate.At(0x1e44, "vowel transition", &generator.VowelTransition),
		threadstate.At(0x1e48, "vowel offset", &generator.VowelOffset), threadstate.At(0x1e50, "closure frames", &generator.ClosureFrames),
		threadstate.At(0x1e54, "fricative aspiration", &generator.FricativeAspiration), threadstate.At(0x1e58, "aspiration", &generator.Aspiration),
		threadstate.At(0x1fb8, "unvoiced pitch", &generator.UnvoicedPitch), threadstate.At(0x1fe8, "transition limit", &generator.TransitionLimit),
		threadstate.At(0x1fb0, "ahead", &generator.Ahead), threadstate.At(0x1fb4, "start", &generator.Start),
		threadstate.At(0x2010, "previous group", &generator.PreviousGroup), threadstate.At(0x2014, "vowel frames", &generator.VowelFrames),
		threadstate.At(0x1fdd, "keep earlier", &generator.KeepEarlier), threadstate.At(0x1fbc, "at reset", &generator.AtReset),
		threadstate.At(0x1fae, "generated done", &generator.GeneratedDone), threadstate.At(0x2035, "database loci", &generator.DatabaseLoci),
		threadstate.At(stateDone, "done", &ring.Done), threadstate.At(0x1ff5, "segment starting", &generator.SegmentStarting),
		threadstate.At(stateMute, "mute", &ring.Mute), threadstate.At(stateCoarticulation, "coarticulation", &generator.Coarticulation),
		threadstate.At(0x1fed, "before previous", &generator.BeforePrevious), threadstate.At(0x1fec, "after next", &generator.AfterNext),
		threadstate.At(0x1ff4, "segment symbol", &generator.SegmentSymbol)}
	for index := range generator.BurstLevels {
		fields = append(fields, threadstate.At(0x1e5c+4*index, label("burst level %d", index), &generator.BurstLevels[index]))
	}
	for index := range generator.Offglide {
		fields = append(fields, threadstate.At(0x1f24+4*index, label("offglide %d", index), &generator.Offglide[index]),
			threadstate.At(0x2018+4*index, label("midpoint %d", index), &generator.Midpoints[index]))
	}
	for track := range generator.Tracks {
		fields = append(fields, threadstate.At(0x3c+4*track, label("track %d next", track), &generator.Tracks[track].Next),
			threadstate.At(0x94+4*track, label("track %d earliest", track), &generator.Tracks[track].Earliest),
			threadstate.At(0x1e74+4*track, label("track %d last", track), &generator.Last[track]),
			threadstate.At(0x1ecc+4*track, label("track %d weight", track), &generator.Weights[track]),
			threadstate.At(0x1f40+4*track, label("track %d next target", track), &generator.NextTargets[track]),
			threadstate.At(stateSegmentFrame+track, label("segment frame %d", track), &generator.SegmentFrame[track]))
		segment := &generator.Segments[track]
		for index, value := range []*int32{&segment.Mode, &segment.Back, &segment.Forward, &segment.Frames,
			&segment.BackTarget, &segment.Onset, &segment.Target} {
			name := []string{"mode", "back", "forward", "frames", "back target", "onset", "target"}[index]
			fields = append(fields, threadstate.At(stateSegments+segmentSize*track+4*index, label("track %d segment %s", track, name), value))
		}
	}
	for flag := range generator.Flags {
		fields = append(fields, threadstate.At(0x1e68+flag, label("flag %d", flag), &generator.Flags[flag]))
	}
	return fields
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
	loadInto(generator, call.Before, call.Base, nil)
	return generator
}

// loadInto gives the generator, its pipeline and the ring the thread state's values. text returns the text a
// node's text pointer stands for; without it, a node with a text gets a new one.
func loadInto(generator *Generator, state []byte, base uint32, text func(pointer uint32) *pipeline.Text) {
	mapping := threadstate.Mapping{Pipeline: generator.Pipeline, Base: base, Text: text}
	mapping.LoadPipeline(state)
	mapping.LoadFields(generatorFields(generator, false), state)
	ring := generator.Synthesizer
	for slot := range synthesizer.RingLength {
		for track := range datapack.TrackCount {
			ring.Frames[slot][track] = state[stateFrames+synthesizer.RingLength*track+slot]
		}
		ring.Markers[slot] = threadstate.Bit(state, stateMarkers, slot)
		ring.Tones[slot] = threadstate.Bit(state, stateTones, slot)
	}
}

// store writes the generator's values, its pipeline's and the ring into the thread state.
func store(generator *Generator, state []byte, base uint32) {
	mapping := threadstate.Mapping{Pipeline: generator.Pipeline, Base: base}
	mapping.StorePipeline(state)
	mapping.StoreFields(generatorFields(generator, false), state)
	ring := generator.Synthesizer
	for slot := range synthesizer.RingLength {
		for track := range datapack.TrackCount {
			state[stateFrames+synthesizer.RingLength*track+slot] = ring.Frames[slot][track]
		}
		threadstate.SetBit(state, stateMarkers, slot, ring.Markers[slot])
		threadstate.SetBit(state, stateTones, slot, ring.Tones[slot])
	}
}

var stateNames = sync.OnceValue(func() map[int]string {
	generator := New(nil, nil, pipeline.New(nil, new(synthesizer.Synthesizer), nil, nil))
	return threadstate.Names(generatorFields(generator, true), threadstate.PipelineFields(generator.Pipeline))
})

// describe names a byte of the thread state, for the reports.
func describe(offset int) string {
	if offset >= stateFrames && offset < stateFrames+datapack.TrackCount*synthesizer.RingLength {
		return fmt.Sprintf("track %d frame %d", (offset-stateFrames)/synthesizer.RingLength, (offset-stateFrames)%synthesizer.RingLength)
	}
	return threadstate.Describe(offset, stateNames())
}

// difference reports where the state the generator leaves, written over image, differs from expected.
func difference(generator *Generator, image, expected []byte, base uint32) error {
	state := slices.Clone(image)
	store(generator, state, base)
	return differences(state, expected)
}

// differences reports where a state differs from the expected one.
func differences(state, expected []byte) error {
	return threadstate.Differences(state, expected, describe)
}

// compare reports where the state the generator leaves differs from the original's.
func compare(generator *Generator, call *call) error {
	return difference(generator, call.Before, call.After, call.Base)
}
