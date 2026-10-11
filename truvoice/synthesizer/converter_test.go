package synthesizer

import (
	"encoding/hex"
	"testing"

	"github.com/phyce/OpenSAPI4/truvoice/internal/conformance"
)

func TestConverter(t *testing.T) {
	tables, err := conformance.Pack(t).LoadSynthesizer()
	if err != nil {
		t.Fatal(err)
	}
	cases := conformance.ReadLines[goldenCase](t, "synthesizer.jsonl.gz")
	frames, failures, tones := 0, 0, 0
	for _, golden := range cases {
		var converter *Converter
		for index, record := range golden.Records {
			switch record.Kind {
			case "reset":
				converter = NewConverter(tables, record.Rate)
			case "frame":
				if converter == nil {
					t.Fatalf("%s: frame before a reset", golden.Name)
				}
				var frame Frame
				tracks, err := hex.DecodeString(record.Tracks)
				if err != nil || len(tracks) != len(frame) {
					t.Fatalf("%s record %d: tracks %q", golden.Name, index, record.Tracks)
				}
				copy(frame[:], tracks)
				expected := decodeWords(t, record.Block)
				actual := converter.Convert(&frame, record.Tone, record.Mute != 0)
				frames++
				if record.Tone {
					tones++
				}
				for word := range expected {
					if actual[word] != expected[word] {
						failures++
						if failures <= 10 {
							t.Errorf("%s record %d: word %d is %d, expected %d (tracks %s)", golden.Name, index, word, actual[word], expected[word], record.Tracks)
						}
						break
					}
				}
			}
		}
	}
	if failures > 0 || frames == 0 {
		t.Errorf("%d of %d blocks differ", failures, frames)
	}
	t.Logf("%d cases, %d frames, %d tone frames", len(cases), frames, tones)
}
