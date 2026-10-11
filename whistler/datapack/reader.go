package datapack

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

var errTruncated = errors.New("file ends early")

type reader struct {
	data     []byte
	position int
	err      error
}

func newReader(data []byte, magic string) (*reader, error) {
	input := &reader{data: data}
	if string(input.bytes(len(magic))) != magic {
		return nil, fmt.Errorf("not a %s file", magic)
	}
	if version := input.uint32(); version != FormatVersion {
		return nil, fmt.Errorf("%s version %d, expected %d", magic, version, FormatVersion)
	}
	return input, input.err
}

func (input *reader) bytes(count int) []byte {
	if input.err != nil {
		return nil
	}
	if count < 0 || count > len(input.data)-input.position {
		input.err = errTruncated
		return nil
	}
	value := input.data[input.position : input.position+count]
	input.position += count
	return value
}

func (input *reader) uint32() uint32 {
	value := input.bytes(4)
	if value == nil {
		return 0
	}
	return binary.LittleEndian.Uint32(value)
}

func (input *reader) int32() int32 {
	return int32(input.uint32())
}

func (input *reader) count(limit int) int {
	value := input.uint32()
	if input.err == nil && uint64(value) > uint64(limit) {
		input.err = fmt.Errorf("count %d is larger than %d", value, limit)
		return 0
	}
	return int(value)
}

func (input *reader) remaining(elementSize int) int {
	return (len(input.data) - input.position) / elementSize
}

func (input *reader) uint16s(count int) []uint16 {
	data := input.bytes(2 * count)
	values := make([]uint16, len(data)/2)
	for index := range values {
		values[index] = binary.LittleEndian.Uint16(data[2*index:])
	}
	return values
}

func (input *reader) uint32s(count int) []uint32 {
	data := input.bytes(4 * count)
	values := make([]uint32, len(data)/4)
	for index := range values {
		values[index] = binary.LittleEndian.Uint32(data[4*index:])
	}
	return values
}

func (input *reader) int32s(count int) []int32 {
	data := input.bytes(4 * count)
	values := make([]int32, len(data)/4)
	for index := range values {
		values[index] = int32(binary.LittleEndian.Uint32(data[4*index:]))
	}
	return values
}

func (input *reader) uint64s(count int) []uint64 {
	data := input.bytes(8 * count)
	values := make([]uint64, len(data)/8)
	for index := range values {
		values[index] = binary.LittleEndian.Uint64(data[8*index:])
	}
	return values
}

func (input *reader) float32s(count int) []float32 {
	data := input.bytes(4 * count)
	values := make([]float32, len(data)/4)
	for index := range values {
		values[index] = math.Float32frombits(binary.LittleEndian.Uint32(data[4*index:]))
	}
	return values
}

func (input *reader) float64s(count int) []float64 {
	data := input.bytes(8 * count)
	values := make([]float64, len(data)/8)
	for index := range values {
		values[index] = math.Float64frombits(binary.LittleEndian.Uint64(data[8*index:]))
	}
	return values
}

func (input *reader) finish() error {
	if input.err != nil {
		return input.err
	}
	if input.position != len(input.data) {
		return fmt.Errorf("%d bytes after the end", len(input.data)-input.position)
	}
	return nil
}
