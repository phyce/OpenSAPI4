package datapack_test

import (
	"math"
	"slices"
	"testing"

	"github.com/phyce/OpenSAPI4/whistler/datapack"
	"github.com/phyce/OpenSAPI4/whistler/internal/conformance"
)

func TestResamplers(t *testing.T) {
	resamplers, err := conformance.Pack(t).LoadResamplers()
	if err != nil {
		t.Fatal(err)
	}
	tables := []struct {
		name    string
		taps    []float32
		up      int
		count   int
		decibel float64
	}{
		{"22050 to 8000 Hz", resamplers.From22050To8000, 4, 401, -0.48},
		{"22050 to 16000 Hz", resamplers.From22050To16000, 8, 601, 0.09},
		{"8000 to 11025 Hz", resamplers.From8000To11025, 11, 401, -0.47},
	}
	for _, table := range tables {
		total := 0.0
		for _, tap := range table.taps {
			total += float64(tap)
		}
		gain := math.Round(2000*math.Log10(total/float64(table.up))) / 100
		symmetric := slices.Equal(table.taps, reversed(table.taps))
		if len(table.taps) != table.count || !symmetric || gain != table.decibel {
			t.Errorf("%s: %d taps, symmetric %t, DC gain %v dB", table.name, len(table.taps), symmetric, gain)
		}
	}
}

func reversed(values []float32) []float32 {
	copied := slices.Clone(values)
	slices.Reverse(copied)
	return copied
}

func TestDamagedResamplers(t *testing.T) {
	tables := func(counts ...int) []byte {
		data := littleEndian("WRSM", uint32(1))
		for _, count := range counts {
			data = append(data, littleEndian(uint32(count), make([]float32, count))...)
		}
		return data
	}
	load := func(data []byte) error {
		pack, err := datapack.Open(writePack(t, map[string][]byte{"resamplers.bin": data}))
		if err != nil {
			return err
		}
		_, err = pack.LoadResamplers()
		return err
	}
	valid := tables(401, 601, 401)
	if err := load(valid); err != nil {
		t.Fatalf("valid resamplers: %v", err)
	}
	notFinite := slices.Clone(valid)
	copy(notFinite[12:], littleEndian(float32(math.NaN())))
	damaged := map[string][]byte{
		"two tables":          tables(401, 601),
		"a table of 400 taps": tables(400, 601, 401),
		"a fourth table":      tables(401, 601, 401, 1),
		"truncated":           valid[:len(valid)-1],
		"a NaN tap":           notFinite,
		"version 2":           append(littleEndian("WRSM", uint32(2)), valid[8:]...),
	}
	for name, data := range damaged {
		if err := load(data); err == nil {
			t.Errorf("%s: loaded", name)
		} else {
			t.Logf("%s: %v", name, err)
		}
	}
}
