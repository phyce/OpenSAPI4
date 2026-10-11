// Package posting writes a sentence's units and tags into the records the voice reads, as the original's
// front end does (engine.md §6), prosody transplants included (engine.md §13).
package posting

import (
	"bytes"
	"encoding/binary"
	"math"

	"github.com/phyce/OpenSAPI4/whistler/units"
)

const (
	recordSize       = 100
	durationOffset   = 0x10
	countOffset      = 0x14
	positionsOffset  = 0x18
	pitchesOffset    = 0x2C
	amplitudesOffset = 0x40
	maximumPoints    = 5
)

// buffer is the original's record buffer (engine.md §13.2): 100-byte records, zero when they are added, that
// keep whatever a write leaves in them. A tag's text runs on into the records after it.
type buffer struct {
	data     []byte
	capacity int
}

func newBuffer(capacity int) buffer {
	return buffer{data: make([]byte, capacity*recordSize), capacity: capacity}
}

// grow makes room for capacity records; the new ones are zero.
func (buffer *buffer) grow(capacity int) {
	if capacity > buffer.capacity {
		buffer.data = append(buffer.data, make([]byte, (capacity-buffer.capacity)*recordSize)...)
		buffer.capacity = capacity
	}
}

// offset is where a field lies, or a fault when it is outside the buffer.
func (buffer *buffer) offset(record int, field int, length int) int {
	offset := record*recordSize + field
	if record < 0 || offset < 0 || offset+length > len(buffer.data) {
		panic(fault("reaches outside the record buffer"))
	}
	return offset
}

func (buffer *buffer) float(record int, field int) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(buffer.data[buffer.offset(record, field, 4):]))
}

func (buffer *buffer) setFloat(record int, field int, value float32) {
	binary.LittleEndian.PutUint32(buffer.data[buffer.offset(record, field, 4):], math.Float32bits(value))
}

func (buffer *buffer) integer(record int, field int) int32 {
	return int32(binary.LittleEndian.Uint32(buffer.data[buffer.offset(record, field, 4):]))
}

func (buffer *buffer) setInteger(record int, field int, value int32) {
	binary.LittleEndian.PutUint32(buffer.data[buffer.offset(record, field, 4):], uint32(value))
}

func (buffer *buffer) first(record int) byte {
	return buffer.data[buffer.offset(record, 0, 1)]
}

// setString writes text and its NUL from the start of a record. A text longer than the buffer holds makes it
// grow, where the original writes past it.
func (buffer *buffer) setString(record int, text string) {
	if end := record*recordSize + len(text) + 1; end > len(buffer.data) {
		buffer.grow((end + recordSize - 1) / recordSize)
	}
	start := buffer.offset(record, 0, len(text)+1)
	copy(buffer.data[start:], text)
	buffer.data[start+len(text)] = 0
}

func (buffer *buffer) zero(record int, count int) {
	start := buffer.offset(record, 0, count*recordSize)
	clear(buffer.data[start : start+count*recordSize])
}

// name reads a record's name: the bytes up to the first NUL, which may lie in a later record.
func (buffer *buffer) name(record int) string {
	data := buffer.data[buffer.offset(record, 0, 1):]
	if end := bytes.IndexByte(data, 0); end >= 0 {
		data = data[:end]
	}
	return string(data)
}

// kindOf is what the voice makes of a record by its first byte: a boundary, a tag or, zero included, a phone.
func kindOf(first byte) units.RecordKind {
	switch first {
	case '#':
		return units.BoundaryRecord
	case '\\':
		return units.ControlRecord
	}
	return units.PhoneRecord
}

// record reads a record as the voice does. A phone whose point count isn't 0 to 5 is read as a plain SIL,
// where the original overruns its work record.
func (buffer *buffer) record(number int) units.Record {
	name := buffer.name(number)
	switch kind := kindOf(buffer.first(number)); kind {
	case units.BoundaryRecord:
		return units.Record{Kind: kind, Name: name}
	case units.ControlRecord:
		return units.Record{Kind: kind, Name: name, Control: units.ParseTag(name)}
	}
	count := buffer.integer(number, countOffset)
	if count < 0 || count > maximumPoints {
		return units.Record{Kind: units.PhoneRecord, Name: "SIL"}
	}
	record := units.Record{Kind: units.PhoneRecord, Name: name, Duration: buffer.float(number, durationOffset), Points: make([]units.Point, count)}
	for point := range record.Points {
		record.Points[point] = units.Point{Position: buffer.float(number, positionsOffset+4*point), Pitch: buffer.float(number, pitchesOffset+4*point),
			Amplitude: buffer.float(number, amplitudesOffset+4*point)}
	}
	return record
}
