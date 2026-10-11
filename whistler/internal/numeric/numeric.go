package numeric

import "math"

type Random struct {
	state uint32
}

func NewRandom() Random {
	return Random{state: 1}
}

func (generator *Random) Next() int32 {
	generator.state = generator.state*214013 + 2531011
	return int32(generator.state>>16) & 0x7FFF
}

func Truncate(value float64) int32 {
	if math.IsNaN(value) || math.Abs(value) >= 1<<63 {
		return 0
	}
	return int32(int64(value))
}
