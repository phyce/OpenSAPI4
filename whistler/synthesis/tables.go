package synthesis

import (
	"math"

	"github.com/phyce/OpenSAPI4/whistler/internal/numeric"
)

func makeSineTable(size int) []float32 {
	table := make([]float32, 2*size)
	step := math.Pi / float64(size)
	for index := 1; index < size/2; index++ {
		value := float32(math.Sin(float64(index) * step))
		table[index] = value
		table[size-index] = value
		table[size+index] = -value
		table[2*size-index] = -value
	}
	table[size/2] = 1
	table[size*3/2] = -1
	return table
}

func makeFadeWindow(size int) []float32 {
	window := make([]float32, size)
	step := math.Pi / float64(size)
	for index := range window {
		window[index] = float32((math.Cos(float64(index)*step) + 1) * 0.5)
	}
	return window
}

func makeNoiseTable(size int) []int8 {
	generator := numeric.NewRandom()
	table := make([]int8, size)
	var cached float32
	haveCached := false
	for index := range table {
		value := cached
		if haveCached {
			haveCached = false
		} else {
			first := float64(float64(generator.Next()) * 3.0517578125e-05)
			second := float64(float64(generator.Next()) * 3.0517578125e-05)
			radius := math.Sqrt(math.Log(1.0-first) * -2.0)
			angle := second * (2 * math.Pi)
			value = float32(math.Cos(angle) * radius)
			cached = float32(math.Sin(angle) * radius)
			haveCached = true
		}
		scaled := float64(float64(value) * 50.0)
		if scaled > 0 {
			scaled += 0.5
		} else {
			scaled -= 0.5
		}
		table[index] = int8(numeric.Truncate(scaled))
	}
	return table
}
