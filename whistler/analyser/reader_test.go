package analyser

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/phyce/OpenSAPI4/whistler/internal/conformance"
	"github.com/phyce/OpenSAPI4/whistler/rules"
)

func compareRead(report *report, name string, read *Read, golden *conformance.TextRead) bool {
	equal := report.check(name, int(read.Result) == golden.Code && read.State.DurationStyle == golden.DurationStyle && !read.Overflowed,
		"result 0x%x, duration style %d, overflowed %t; expected 0x%x, %d", read.Result, read.State.DurationStyle, read.Overflowed, golden.Code, golden.DurationStyle)
	state := read.State
	if read.Sentence() {
		equal = report.check(name, read.Consumed == golden.Consumed && state.Start == golden.Start && state.End == golden.End && read.NaturalEnd == (golden.NaturalEnd == 1),
			"consumed %d, start %d, end %d, natural end %t; expected %d, %d, %d, %d", read.Consumed, state.Start, state.End, read.NaturalEnd,
			golden.Consumed, golden.Start, golden.End, golden.NaturalEnd) && equal
	}
	end := state.End
	tier1 := state.Tier(1)
	equal = report.check(name, bytes.Equal(tier1[1:end+1], conformance.Bytes(golden.Tier1)), "tier 1 %q, expected %q", tier1[1:end+1], golden.Tier1) && equal
	beyond := bytes.TrimRight(tier1[end+1:0x501], " ")
	equal = report.check(name, bytes.Equal(beyond, conformance.Bytes(golden.Beyond)), "past the end %q, expected %q", beyond, golden.Beyond) && equal
	equal = report.check(name, !slices.ContainsFunc(tier1[0x501:], func(character byte) bool { return character != 0 }) && tier1[0] == 0,
		"cell 0 or a cell past 0x500 written") && equal
	anchors := make([]int, end)
	for cell := range anchors {
		anchors[cell] = int(state.Anchors[cell+1])
	}
	anchorsBeyond := [][2]int{}
	for cell := end + 1; cell < rules.Cells; cell++ {
		if count := state.Anchors[cell]; count != 0 {
			anchorsBeyond = append(anchorsBeyond, [2]int{cell, int(count)})
		}
	}
	equal = report.check(name, slices.Equal(anchors, golden.Anchors) && slices.Equal(anchorsBeyond, append([][2]int{}, golden.AnchorsBeyond...)),
		"anchors %v past the end %v, expected %v %v", anchors, anchorsBeyond, golden.Anchors, golden.AnchorsBeyond) && equal
	events := make([][4]int64, len(state.Events))
	for index, event := range state.Events {
		events[index] = [4]int64{int64(event.Kind), int64(event.Offset), int64(event.Length), int64(event.Value)}
	}
	return report.check(name, slices.Equal(events, append([][4]int64{}, golden.Events...)), "events %v, expected %v", events, golden.Events) && equal
}

func TestReader(t *testing.T) {
	data := loadPack(t)
	cases := conformance.ReadLines[conformance.TextCase](t, "golden", "text", "texts.jsonl.gz")
	report := &report{t: t}
	reads, sentences, dropped, overflowed := 0, 0, 0, 0
	for _, textCase := range cases {
		if report.failures > 20 {
			t.FailNow()
		}
		reader := NewReader(data.dictionaries, data.prosody)
		analyser := data.analyser(t)
		for segmentNumber, segment := range textCase.Segments {
			name := fmt.Sprintf("%s segment %d", textCase.Name, segmentNumber+1)
			actual := reader.ReadSegment(conformance.Bytes(segment.Text))
			if !report.check(name, len(actual) == len(segment.Reads), "%d reads, expected %d", len(actual), len(segment.Reads)) {
				continue
			}
			for index := range actual {
				read, golden := &actual[index], &segment.Reads[index]
				readName := fmt.Sprintf("%s read %d", name, index+1)
				reads++
				if !compareRead(report, readName, read, golden) || !read.Sentence() || golden.Error != "" {
					continue
				}
				sentences++
				transcribed := analyser.Transcribe(read.State)
				if !transcribed {
					dropped++
				}
				report.check(readName, transcribed == (golden.Transcribed.Result == 0) && stateHash(read.State) == golden.Transcribed.Hash,
					"transcribed %t, result %d, or the state differs", transcribed, golden.Transcribed.Result)
				analysed := analyser.Analyse(read.State)
				if !analysed {
					overflowed++
				}
				report.check(readName, analysed == (golden.Analysis.Result == 0) && read.State.Start == golden.Analysis.Start && read.State.End == golden.Analysis.End &&
					stateHash(read.State) == golden.Analysis.Hash, "analysed %t, result %d, or the state differs", analysed, golden.Analysis.Result)
			}
		}
	}
	t.Logf("%d texts, %d reads, %d sentences: %d not transcribed, %d overflowing module A", len(cases), reads, sentences, dropped, overflowed)
	if sentences < 100000 {
		t.Errorf("only %d sentences", sentences)
	}
}

// TestDurationStyle reads ESC A with both styles in either case, and an ESC A after content, which ends
// the sentence before it. The FrontEnd itself sends only ESC A P.
func TestDurationStyle(t *testing.T) {
	data := loadPack(t)
	reader := NewReader(data.dictionaries, data.prosody)
	reader.durationStyle = 1
	expected := []struct {
		result   ReadResult
		consumed int
		style    int
		tier1    string
	}{
		{ReadNormal, 9, 0, "word "}, {ReadNothingLeft, 0, 0, ""}, {ReadNormal, 6, 1, "a "}, {ReadNothingLeft, 0, 1, ""},
		{ReadNormal, 5, 1, "word "}, {ReadNormal, 9, 0, "next "}, {ReadNothingLeft, 0, 0, ""},
		{ReadNormal, 6, 1, "x "}, {ReadNormal, 6, 1, "y "}, {ReadNothingLeft, 0, 1, ""},
	}
	var reads []Read
	for _, segment := range []string{"\x1bAF word \r\n", "\x1bAp a \r\n", "word \x1bAf next \r\n", "\x1bAP x \x1bAQ y \r\n"} {
		reads = append(reads, reader.ReadSegment([]byte(segment))...)
	}
	if len(reads) != len(expected) {
		t.Fatalf("%d reads", len(reads))
	}
	for index, read := range reads {
		want := expected[index]
		tier1 := string(read.State.Tier(1)[1 : read.State.End+1])
		if read.Result != want.result || read.Sentence() && read.Consumed != want.consumed || read.State.DurationStyle != want.style || tier1 != want.tier1 {
			t.Errorf("read %d: 0x%x, consumed %d, style %d, %q; expected %+v", index, read.Result, read.Consumed, read.State.DurationStyle, tier1, want)
		}
	}
}

func TestReaderOverflow(t *testing.T) {
	data := loadPack(t)
	reader := NewReader(data.dictionaries, data.prosody)
	segment := append(bytes.Repeat([]byte("\x1b\""), 800), "word \r\n"...)
	reads := reader.ReadSegment(segment)
	if len(reads) != 2 || !reads[0].Overflowed || reads[0].Sentence() || reads[0].State.End >= rules.Cells || reads[1].Result != ReadNothingLeft {
		t.Errorf("reads %+v", reads)
	}
}

// TestUnusualSegments reads random segments of escapes, markers, brackets, long words and line breaks, and
// transcribes and analyses every sentence; nothing may fail.
func TestUnusualSegments(t *testing.T) {
	data := loadPack(t)
	random := rand.New(rand.NewPCG(18, 9))
	pieces := []string{"\x1bAP ", "\x1bAF ", "\x1bN12 ", "\x1bN1,2,3 ", "\x1bH500 ", "\x1b\" ", "\x1b@N ", "\x1b[addr] ", "\x1b[\\addr] ", "\x1b/",
		"\x1b[x", "\x1bQ", "\x1bE ", "\x1b", "AIFF ", "Dr. ", "word ", "--- ", "aaaaaaaaaa ", ".......... ", "1234 ", "(+$%) ", "\r\n", " \r\n",
		"\t", ", ", "hh'aI ", "\x10 'h$ \x10 ", "\x1a", "\x00", "\xb0", "\x91"}
	var analyser *Analyser
	reader := NewReader(data.dictionaries, data.prosody)
	sentences := 0
	for number := range 3000 {
		if number%50 == 0 {
			analyser = data.analyser(t)
		}
		var segment []byte
		for range random.IntN(200) {
			segment = append(segment, pieces[random.IntN(len(pieces))]...)
		}
		if number%10 == 0 {
			segment = append(segment, bytes.Repeat([]byte(pieces[random.IntN(len(pieces))]), random.IntN(900))...)
		}
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("segment %d %q: %v", number, segment, recovered)
				}
			}()
			for _, read := range reader.ReadSegment(segment) {
				if read.State.End < 0 || read.State.End >= rules.Cells {
					t.Fatalf("segment %d: end %d", number, read.State.End)
				}
				if read.Sentence() {
					sentences++
					analyser.Transcribe(read.State)
					analyser.Analyse(read.State)
				}
			}
		}()
	}
	t.Logf("3000 segments, %d sentences", sentences)
}
