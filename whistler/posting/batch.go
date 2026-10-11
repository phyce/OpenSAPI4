package posting

import (
	"math"
	"strconv"

	"github.com/phyce/OpenSAPI4/whistler/units"
)

var (
	hundredth         = float32(0.01)
	negativeHundredth = float32(-0.01)
)

const (
	semitoneScale     = 1.0 / 48
	nameLength        = 14
	growth            = 5
	attributeStops    = ",>"
	entryWhitespace   = " \r\t\n"
	durationStops     = "; \t\r\n"
	durationSeparator = " \t\r\n"
)

// Batch is one sentence's posting: the record buffer and the settings prosody transplants keep until the end
// of the batch (engine.md §13.2).
type Batch struct {
	converter *Converter
	buffer    buffer
	written   int
	pending   int

	offset            int32
	offsetKnown       bool
	relativeDurations bool
	relativePitch     bool
	beginPitch        int32
	endPitch          int32
}

// NewBatch starts a batch with room for capacity records: the units plus two per tag of the span.
func NewBatch(converter *Converter, capacity int) *Batch {
	return &Batch{converter: converter, buffer: newBuffer(capacity)}
}

// slot is the record at the write position. Past the buffer the original writes over its heap; the buffer
// grows instead.
func (batch *Batch) slot() int {
	if batch.written >= batch.buffer.capacity {
		batch.buffer.grow(batch.written + 1)
	}
	return batch.written
}

// Copy writes a unit whole: its name, duration code and points.
func (batch *Batch) Copy(name string, duration float32, points []units.Point) {
	record := batch.slot()
	batch.buffer.zero(record, 1)
	copy(batch.buffer.data[record*recordSize:record*recordSize+16], name)
	batch.buffer.setFloat(record, durationOffset, duration)
	batch.buffer.setInteger(record, countOffset, int32(len(points)))
	for index, point := range points {
		batch.buffer.setFloat(record, positionsOffset+4*index, point.Position)
		batch.buffer.setFloat(record, pitchesOffset+4*index, point.Pitch)
		batch.buffer.setFloat(record, amplitudesOffset+4*index, point.Amplitude)
	}
	batch.written++
}

// Tag writes a tag's text.
func (batch *Batch) Tag(text string) {
	batch.buffer.setString(batch.slot(), text)
	batch.written++
}

// Pause writes a pause as #, \Pau=value\ and #.
func (batch *Batch) Pause(value string) {
	batch.Tag("#")
	batch.Tag(`\Pau=` + value + `\`)
	batch.buffer.setInteger(batch.written-1, durationOffset, 0)
	batch.Tag("#")
}

// Drop counts a unit or tag dropped while a transplant is on.
func (batch *Batch) Drop() {
	batch.pending--
}

// SetOffset keeps a \Btp= tag's offset for the word positions of the transplants after it.
func (batch *Batch) SetOffset(offset int32) {
	batch.offset, batch.offsetKnown = offset, true
}

// EndTransplant makes room for the records a transplant wrote beyond those it dropped.
func (batch *Batch) EndTransplant() {
	if batch.pending > 0 {
		batch.buffer.grow(batch.buffer.capacity + batch.pending)
		batch.buffer.zero(batch.written, batch.pending)
	}
}

// Records gives the records written, as the voice reads them, and the kind of the record after them.
func (batch *Batch) Records() ([]units.Record, units.RecordKind) {
	records := make([]units.Record, batch.written)
	for number := range records {
		records[number] = batch.buffer.record(number)
	}
	following := units.PhoneRecord
	if batch.written < batch.buffer.capacity {
		following = kindOf(batch.buffer.first(batch.written))
	}
	return records, following
}

func (batch *Batch) advance(phone bool) {
	batch.written++
	if phone {
		batch.pending++
	}
	if batch.written >= batch.buffer.capacity {
		batch.buffer.grow(batch.written + growth)
		batch.buffer.zero(batch.written, growth)
	}
}

// pitchValue is a BP, EP or P value in Hz: as it is, or with RP in steps of 1/48 octave from the pitch.
func (batch *Batch) pitchValue(value float64, pitch int) float32 {
	if batch.relativePitch {
		return float32(math.Pow(2, value*semitoneScale) * float64(pitch))
	}
	return float32(value)
}

// Transplant runs a \Btp= text (engine.md §13.3) and reports whether it succeeded. A failed transplant takes
// the write position back; the records it wrote stay in the buffer.
func (batch *Batch) Transplant(value string, pitch int) bool {
	text := cText(upper(value) + "\x00")
	if text.at(0) != '=' {
		return false
	}
	position := text.find(0, '<')
	if position < 0 {
		return false
	}
	written, pending := batch.written, batch.pending
	if !batch.readTransplant(text, position, pitch) {
		batch.written, batch.pending = written, pending
		return false
	}
	if batch.endPitch != 0 {
		batch.applyEndPitch(pitch)
	}
	return true
}

func (batch *Batch) readTransplant(text cText, position int, pitch int) (succeeded bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if _, isFault := recovered.(fault); !isFault {
				panic(recovered)
			}
			succeeded = false
		}
	}()
	return batch.readPhones(text, batch.readAttributes(text, position), pitch)
}

func (batch *Batch) readAttributes(text cText, position int) int {
	for {
		start := position + 1
		length := text.complementSpan(start, attributeStops)
		attribute := string(text[start : start+length])
		switch {
		case attribute == "RD":
			batch.relativeDurations = true
		case attribute == "RP":
			batch.relativePitch = true
		case length > 3 && attribute[:3] == "BP=":
			batch.beginPitch = text.atoi(start + 3)
		case length > 3 && attribute[:3] == "EP=":
			batch.endPitch = text.atoi(start + 3)
		}
		position = start + length
		if text.at(position) == '>' {
			return position + 1 + text.span(position+1, " \t\n\r")
		}
	}
}

// readPhones reads the entries after the attributes and reports whether they all converted.
func (batch *Batch) readPhones(text cText, position int, pitch int) bool {
	for {
		count, values := text.scanCodes(position)
		if count == 0 {
			panic(fault("DebugBreak: no phoneme code"))
		}
		if count < 0 {
			panic(fault("no phoneme data"))
		}
		codes := make([]uint16, len(values))
		for index, value := range values {
			codes[index] = uint16(value)
		}
		name, valid := batch.converter.Convert(codes)
		if !valid {
			return false
		}
		record := batch.slot()
		batch.buffer.setString(record, string(name[:min(len(name), nameLength)]))
		failed := false
		switch {
		case len(name) > 0 && name[0] == '#':
			if !batch.offsetKnown {
				panic(fault("a word boundary before any \\Btp= offset in the batch"))
			}
			offset := batch.offset + int32(position)
			if batch.offset < 0 {
				offset = 2*int32(position) - batch.offset
			}
			batch.buffer.setString(record, `\Wrd=`+strconv.Itoa(int(offset))+`\`)
			batch.advance(false)
		case len(name) > 0 && name[0] == '-':
		default:
			position, failed = batch.readPhone(text, position, record, pitch)
			batch.advance(true)
		}
		next := text.find(position, ';')
		if failed {
			return false
		}
		if next < 0 {
			return true
		}
		position = next + 1 + text.span(next+1, entryWhitespace)
		if text.at(position) == 0 {
			return true
		}
	}
}

// readPhone reads a phone's duration and points into its record. It reports a failure when a point lies
// before the one before it.
func (batch *Batch) readPhone(text cText, position int, record int, pitch int) (int, bool) {
	buffer := &batch.buffer
	position += text.complementSpan(position, ",;")
	if text.at(position) == ';' {
		return position, false
	}
	position += 1 + text.span(position+1, entryWhitespace)
	if text.at(position) != '(' {
		number := text.atoi(position)
		duration := float32(-float64(number))
		if batch.relativeDurations {
			duration = float32(5 - float64(float64(number)*float64(negativeHundredth)))
		}
		buffer.setFloat(record, durationOffset, duration)
		position += text.complementSpan(position, durationStops)
		position += text.span(position, durationSeparator)
	}
	buffer.setInteger(record, countOffset, 0)
	if batch.beginPitch != 0 {
		buffer.setFloat(record, positionsOffset, 0)
		buffer.setFloat(record, pitchesOffset, batch.pitchValue(float64(batch.beginPitch), pitch))
		batch.beginPitch = 0
		buffer.setInteger(record, countOffset, 1)
	}
	for text.at(position) == '(' {
		if buffer.integer(record, countOffset) >= maximumPoints {
			buffer.setInteger(record, countOffset, maximumPoints-1)
		}
		point := int(buffer.integer(record, countOffset))
		field := positionsOffset + 4*point
		buffer.setFloat(record, field, float32(text.atof(position+1)*float64(hundredth)))
		if stored := buffer.float(record, field); stored > 1 {
			buffer.setFloat(record, field, 1)
		} else if !(stored >= 0) {
			buffer.setFloat(record, field, 0)
		}
		if point != 0 && buffer.float(record, field) < buffer.float(record, field-4) {
			return position, true
		}
		position = text.find(position, ',')
		if position < 0 {
			panic(fault("a point without a comma"))
		}
		for {
			position++
			position += text.span(position, durationSeparator)
			letter := text.at(position)
			position++
			value := text.atof(position)
			switch letter {
			case 'P':
				buffer.setFloat(record, pitchesOffset+4*point, batch.pitchValue(value, pitch))
			case 'V':
				buffer.setFloat(record, amplitudesOffset+4*point, float32(value*float64(hundredth)))
			}
			position += text.complementSpan(position, ",)")
			if text.at(position) != ',' {
				break
			}
		}
		buffer.setInteger(record, countOffset, int32(point+1))
		position += text.complementSpan(position, "(;")
	}
	return position, false
}

// applyEndPitch puts EP on the last record before the write position that isn't a tag (engine.md §13.5). A
// read or write outside the buffer leaves the rest undone.
func (batch *Batch) applyEndPitch(pitch int) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if _, isFault := recovered.(fault); !isFault {
				panic(recovered)
			}
		}
	}()
	buffer := &batch.buffer
	record := batch.written - 1
	for buffer.first(record) == '\\' {
		record--
	}
	count := buffer.integer(record, countOffset)
	if buffer.float(record, positionsOffset+4*int(count)) != 1 {
		buffer.setInteger(record, countOffset, count+1)
	}
	if buffer.integer(record, countOffset) >= maximumPoints {
		buffer.setInteger(record, countOffset, maximumPoints-1)
	}
	buffer.setFloat(record, positionsOffset+4*int(buffer.integer(record, countOffset)), 1)
	slot := int32(0)
	if batch.written < buffer.capacity {
		slot = buffer.integer(batch.written, countOffset)
	}
	buffer.setFloat(record, pitchesOffset+4*int(slot), batch.pitchValue(float64(batch.endPitch), pitch))
}

// upper upper-cases A to Z only, as the original's _strupr does.
func upper(value string) string {
	characters := []byte(value)
	for index, character := range characters {
		if character >= 'a' && character <= 'z' {
			characters[index] = character - 'a' + 'A'
		}
	}
	return string(characters)
}
