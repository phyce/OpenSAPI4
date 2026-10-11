package synthesis

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/phyce/OpenSAPI4/whistler/internal/conformance"
)

func TestResamplerFrames(t *testing.T) {
	tables, err := conformance.Pack(t).LoadResamplers()
	if err != nil {
		t.Fatal(err)
	}
	generator := rand.New(rand.NewPCG(1, 2))
	signal := make([]float32, 20000)
	for index := range signal {
		signal[index] = float32(generator.NormFloat64() * 8000)
	}
	for _, shape := range resamplerShapes {
		resampler := NewResampler(tables, shape.voiceRate, shape.outputRate)
		whole := resampler.Process(signal)
		expected := (len(signal) - shape.history) * shape.up / shape.down
		if len(whole) < expected-1 || len(whole) > expected+1 {
			t.Errorf("%d to %d Hz: %d samples from %d, expected about %d", shape.voiceRate, shape.outputRate, len(whole), len(signal), expected)
		}
		for range 20 {
			resampler.Reset()
			var pieces []float32
			for rest, first := signal, true; len(rest) > 0; first = false {
				length := min(len(rest), 1+generator.IntN(3000))
				if first {
					length = min(len(rest), shape.history+generator.IntN(3000))
				}
				pieces = append(pieces, resampler.Process(rest[:length])...)
				rest = rest[length:]
			}
			if !slices.Equal(pieces, whole) {
				t.Errorf("%d to %d Hz: frames of other lengths give other samples", shape.voiceRate, shape.outputRate)
				break
			}
		}
		resampler.Reset()
		short := shape.history / 2
		if first := resampler.Process(signal[:short]); len(first) != 0 {
			t.Errorf("%d to %d Hz: a first frame of %d samples gives %d", shape.voiceRate, shape.outputRate, short, len(first))
		}
		if rest := resampler.Process(signal[short:]); len(rest) <= len(whole) {
			t.Errorf("%d to %d Hz: after a short first frame, %d samples, without it %d", shape.voiceRate, shape.outputRate, len(rest), len(whole))
		}
	}
	if NewResampler(tables, 22050, 11025) != nil || NewResampler(tables, 8000, 16000) != nil {
		t.Error("a resampler for a pair the original has none for")
	}
}

func TestEightBitSamples(t *testing.T) {
	values := []float32{0, -0.6, 255.4, 255.5, 32767, -32768, 40000, -40000, float32(math.NaN()), -0.5, 127.5, -256.5}
	expected := []byte{0x80, 0x7F, 0x80, 0x81, 0xFF, 0x00, 0xFF, 0x00, 0x00, 0x80, 0x80, 0x7F}
	if actual := convertTo8Bit(values); !slices.Equal(actual, expected) {
		t.Errorf("%x, expected %x", actual, expected)
	}
}
