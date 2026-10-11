package synthesizer

import (
	"encoding/binary"
	"encoding/hex"
	"testing"

	"github.com/phyce/OpenSAPI4/truvoice/internal/conformance"
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

type goldenRecord struct {
	Kind     string        `json:"kind"`
	Rate     int           `json:"rate"`
	Filter   int           `json:"filter"`
	Gain     uint16        `json:"gain"`
	Block    string        `json:"block"`
	Samples  string        `json:"samples"`
	Position int32         `json:"position"`
	Tracks   string        `json:"tracks"`
	Marker   bool          `json:"marker"`
	Tone     bool          `json:"tone"`
	Mute     byte          `json:"mute"`
	Before   goldenCursors `json:"before"`
	After    goldenCursors `json:"after"`
}

type goldenCase struct {
	Name    string         `json:"name"`
	Format  [2]int         `json:"format"`
	Digest  string         `json:"digest"`
	Events  [][]any        `json:"events"`
	Records []goldenRecord `json:"records"`
}

func decodeWords(t *testing.T, text string) []int16 {
	t.Helper()
	data, err := hex.DecodeString(text)
	if err != nil {
		t.Fatal(err)
	}
	words := make([]int16, len(data)/2)
	for index := range words {
		words[index] = int16(binary.LittleEndian.Uint16(data[2*index:]))
	}
	return words
}

func TestRenderer(t *testing.T) {
	tables, err := conformance.Pack(t).LoadSynthesizer()
	if err != nil {
		t.Fatal(err)
	}
	cases := conformance.ReadLines[goldenCase](t, "synthesizer.jsonl.gz")
	blocks, failures := 0, 0
	for _, golden := range cases {
		var renderer *Renderer
		for index, record := range golden.Records {
			switch record.Kind {
			case "reset":
				renderer = NewRenderer(tables, record.Rate)
			case "samples":
				if renderer == nil {
					t.Fatalf("%s: samples before a reset", golden.Name)
				}
				var block Block
				copy(block[:], decodeWords(t, record.Block))
				expected := decodeWords(t, record.Samples)
				actual := renderer.Render(&block, record.Filter, record.Gain, nil)
				blocks++
				if len(actual) != len(expected) {
					failures++
					t.Errorf("%s record %d: %d samples, expected %d", golden.Name, index, len(actual), len(expected))
					continue
				}
				for sample := range expected {
					if actual[sample] != expected[sample] {
						failures++
						if failures <= 10 {
							t.Errorf("%s record %d: sample %d is %d, expected %d", golden.Name, index, sample, actual[sample], expected[sample])
						}
						break
					}
				}
			}
		}
	}
	if failures > 0 || blocks == 0 {
		t.Errorf("%d of %d blocks differ", failures, blocks)
	}
	t.Logf("%d cases, %d blocks", len(cases), blocks)
}
