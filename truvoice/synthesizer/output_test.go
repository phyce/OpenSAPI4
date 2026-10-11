package synthesizer

import (
	"bytes"
	"testing"

	"github.com/phyce/OpenSAPI4/truvoice/internal/conformance"
)

type goldenVolumeRun struct {
	First       int     `json:"first"`
	Last        int     `json:"last"`
	Mute        int     `json:"mute"`
	Attenuation int32   `json:"attenuation"`
	Stages      []int32 `json:"stages"`
}

type goldenPrecision struct {
	ControlWord int               `json:"controlWord"`
	Runs        []goldenVolumeRun `json:"runs"`
}

// TestAttenuation checks every volume against what the original stores for it, at each x87 precision.
func TestAttenuation(t *testing.T) {
	precisions := conformance.ReadLines[goldenPrecision](t, "volume.jsonl.gz")
	checked := 0
	for _, precision := range precisions {
		next := 0
		for _, run := range precision.Runs {
			if run.First != next || run.Last < run.First {
				t.Fatalf("control word %#x: a run from %d to %d after %d", precision.ControlWord, run.First, run.Last, next-1)
			}
			next = run.Last + 1
			for volume := run.First; volume <= run.Last; volume++ {
				attenuation, audible := Attenuation(uint16(volume))
				expectedAudible := run.Mute == 0
				if audible != expectedAudible || audible && attenuation != run.Attenuation {
					t.Fatalf("control word %#x, volume %d: %d %v, expected %d %v", precision.ControlWord, volume, attenuation, audible, run.Attenuation, expectedAudible)
				}
				for _, stage := range run.Stages {
					if audible && stage != attenuation || !audible && stage != -1 {
						t.Fatalf("control word %#x, volume %d: stage attenuation %d", precision.ControlWord, volume, stage)
					}
				}
				checked++
			}
		}
		if next != 0x10000 {
			t.Fatalf("control word %#x: the runs end at %d", precision.ControlWord, next-1)
		}
	}
	if checked == 0 {
		t.Fatal("no volumes")
	}
	t.Logf("%d precisions, %d volumes", len(precisions), checked)
}

func TestAppendPCM(t *testing.T) {
	samples := []int16{0, 1, -1, 255, 256, -256, 32767, -32768}
	if pcm := AppendPCM(nil, samples, 16); !bytes.Equal(pcm, []byte{0, 0, 1, 0, 0xff, 0xff, 0xff, 0, 0, 1, 0, 0xff, 0xff, 0x7f, 0, 0x80}) {
		t.Errorf("16-bit % x", pcm)
	}
	if pcm := AppendPCM([]byte{9}, samples, 8); !bytes.Equal(pcm, []byte{9, 128, 128, 127, 128, 129, 127, 255, 0}) {
		t.Errorf("8-bit % x", pcm)
	}
}
