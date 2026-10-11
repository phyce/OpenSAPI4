package datapack

import (
	"encoding/binary"
	"fmt"
	"math"
)

type Codebook struct {
	Rows    int
	Columns int
	Values  []float32
	Cosines []float64
}

func (codebook *Codebook) Row(index byte) []float32 {
	start := int(index) * codebook.Columns
	return codebook.Values[start : start+codebook.Columns]
}

type Inventory struct {
	SampleRate             int
	PredictorOrder         int
	TransformSize          int
	TransformExponent      int
	LineSpectralCodebooks  []Codebook
	FirstSpectrumCodebooks []Codebook
	DeltaSpectrumCodebooks []Codebook

	recordOffsets []uint32
	records       []byte
}

type UnitRecord struct {
	Periods             []int32
	LineSpectralIndices []byte
	Gains               []float32
	SpectrumIndices     []byte
}

func (inventory *Inventory) UnitCount() int {
	return len(inventory.recordOffsets) - 1
}

func (inventory *Inventory) Record(unit int) (UnitRecord, bool) {
	if unit <= 0 || unit >= inventory.UnitCount() {
		return UnitRecord{}, false
	}
	record, err := inventory.parseRecord(inventory.records[inventory.recordOffsets[unit]:inventory.recordOffsets[unit+1]])
	return record, err == nil
}

func parseInventory(data []byte) (*Inventory, error) {
	input, err := newReader(data, "WINV")
	if err != nil {
		return nil, err
	}
	header := input.int32s(7)
	if input.err != nil {
		return nil, input.err
	}
	inventory := &Inventory{
		SampleRate:        int(header[0]),
		PredictorOrder:    int(header[1]),
		TransformSize:     int(header[5]),
		TransformExponent: int(header[6]),
	}
	if inventory.SampleRate <= 0 || inventory.PredictorOrder <= 0 || inventory.PredictorOrder%2 != 0 {
		return nil, fmt.Errorf("rate %d, predictor order %d", inventory.SampleRate, inventory.PredictorOrder)
	}
	if inventory.TransformExponent < 2 || inventory.TransformExponent > 9 || inventory.TransformSize != 1<<inventory.TransformExponent {
		return nil, fmt.Errorf("transform size %d, exponent %d", inventory.TransformSize, inventory.TransformExponent)
	}
	counts := header[2:5]
	for _, count := range counts {
		if count < 0 || count > 64 {
			return nil, fmt.Errorf("%d codebooks", count)
		}
	}
	groups := [3][]Codebook{}
	for group, count := range counts {
		groups[group] = make([]Codebook, count)
		for index := range groups[group] {
			rows, columns := int(input.int32()), int(input.int32())
			if input.err == nil && (rows <= 0 || rows > 256 || columns <= 0 || columns > inventory.TransformSize) {
				return nil, fmt.Errorf("codebook of %d x %d", rows, columns)
			}
			values := input.float32s(rows * columns)
			for _, value := range values {
				if !isFinite(value) {
					return nil, fmt.Errorf("codebook value is not finite")
				}
			}
			groups[group][index] = Codebook{Rows: rows, Columns: columns, Values: values}
		}
	}
	inventory.LineSpectralCodebooks, inventory.FirstSpectrumCodebooks, inventory.DeltaSpectrumCodebooks = groups[0], groups[1], groups[2]
	for index := range inventory.LineSpectralCodebooks {
		codebook := &inventory.LineSpectralCodebooks[index]
		codebook.Cosines = input.float64s(len(codebook.Values))
	}
	unitCount := input.count(input.remaining(4) - 1)
	inventory.recordOffsets = input.uint32s(unitCount + 1)
	if input.err != nil {
		return nil, input.err
	}
	inventory.records = input.bytes(len(data) - input.position)
	if err := input.finish(); err != nil {
		return nil, err
	}
	if err := inventory.validate(); err != nil {
		return nil, err
	}
	return inventory, nil
}

func widthSum(codebooks []Codebook) int {
	total := 0
	for _, codebook := range codebooks {
		total += codebook.Columns
	}
	return total
}

func (inventory *Inventory) validate() error {
	if widthSum(inventory.LineSpectralCodebooks) != inventory.PredictorOrder {
		return fmt.Errorf("line spectral codebooks give %d values for order %d", widthSum(inventory.LineSpectralCodebooks), inventory.PredictorOrder)
	}
	for _, group := range [][]Codebook{inventory.FirstSpectrumCodebooks, inventory.DeltaSpectrumCodebooks} {
		if len(group) > 0 && widthSum(group) != inventory.TransformSize-2 {
			return fmt.Errorf("spectrum codebooks give %d values for size %d", widthSum(group), inventory.TransformSize)
		}
		for _, codebook := range group {
			if codebook.Columns%2 != 0 {
				return fmt.Errorf("spectrum codebook of odd width %d", codebook.Columns)
			}
		}
	}
	if len(inventory.FirstSpectrumCodebooks) == 0 {
		return fmt.Errorf("no first spectrum codebooks")
	}
	for _, codebook := range inventory.LineSpectralCodebooks {
		for _, cosine := range codebook.Cosines {
			if math.IsNaN(cosine) || math.IsInf(cosine, 0) {
				return fmt.Errorf("line spectral cosine is not finite")
			}
		}
	}
	if inventory.UnitCount() < 1 || inventory.recordOffsets[0] != 0 || inventory.recordOffsets[1] != 0 || int(inventory.recordOffsets[inventory.UnitCount()]) != len(inventory.records) {
		return fmt.Errorf("unit record offsets")
	}
	for unit := 1; unit < inventory.UnitCount(); unit++ {
		start, end := inventory.recordOffsets[unit], inventory.recordOffsets[unit+1]
		if end < start {
			return fmt.Errorf("unit %d record offsets", unit)
		}
		if _, err := inventory.parseRecord(inventory.records[start:end]); err != nil {
			return fmt.Errorf("unit %d: %w", unit, err)
		}
	}
	return nil
}

func (inventory *Inventory) parseRecord(data []byte) (UnitRecord, error) {
	if len(data) < 4 {
		return UnitRecord{}, errTruncated
	}
	frameCount := int(int32(binary.LittleEndian.Uint32(data)))
	if frameCount <= 0 || frameCount > len(data) {
		return UnitRecord{}, fmt.Errorf("%d frames", frameCount)
	}
	position := 4
	periods := make([]int32, frameCount)
	for frame := range periods {
		if position >= len(data) {
			return UnitRecord{}, errTruncated
		}
		escape := data[position]
		value := int32(int8(escape))
		if escape == 0x7F || escape == 0x80 {
			for position < len(data) && data[position] == escape {
				if position+1 >= len(data) {
					return UnitRecord{}, errTruncated
				}
				value += int32(int8(data[position+1]))
				position++
			}
		}
		position++
		if value == 0 || value >= int32(2*inventory.TransformSize) || value <= -int32(2*inventory.TransformSize) {
			return UnitRecord{}, fmt.Errorf("frame %d has period %d", frame, value)
		}
		periods[frame] = value
	}
	lineCount := len(inventory.LineSpectralCodebooks)
	if frameCount*lineCount > len(data)-position {
		return UnitRecord{}, errTruncated
	}
	lineIndices := data[position : position+frameCount*lineCount]
	for index, row := range lineIndices {
		if int(row) >= inventory.LineSpectralCodebooks[index%lineCount].Rows {
			return UnitRecord{}, fmt.Errorf("line spectral index %d out of range", row)
		}
	}
	position += frameCount * lineCount
	if 4*frameCount > len(data)-position {
		return UnitRecord{}, errTruncated
	}
	gains := make([]float32, frameCount)
	for frame := range gains {
		gains[frame] = math.Float32frombits(binary.LittleEndian.Uint32(data[position+4*frame:]))
		if !isFinite(gains[frame]) {
			return UnitRecord{}, fmt.Errorf("gain is not finite")
		}
	}
	position += 4 * frameCount
	spectrumStart := position
	for frame, period := range periods {
		if period <= 0 {
			continue
		}
		codebooks := inventory.FirstSpectrumCodebooks
		if len(inventory.DeltaSpectrumCodebooks) > 0 && frame > 0 && periods[frame-1] > 0 {
			codebooks = inventory.DeltaSpectrumCodebooks
		}
		if len(codebooks) > len(data)-position {
			return UnitRecord{}, errTruncated
		}
		for index, codebook := range codebooks {
			if int(data[position+index]) >= codebook.Rows {
				return UnitRecord{}, fmt.Errorf("spectrum index %d out of range", data[position+index])
			}
		}
		position += len(codebooks)
	}
	if position != len(data) {
		return UnitRecord{}, fmt.Errorf("%d bytes after the record", len(data)-position)
	}
	return UnitRecord{Periods: periods, LineSpectralIndices: lineIndices, Gains: gains, SpectrumIndices: data[spectrumStart:position]}, nil
}
