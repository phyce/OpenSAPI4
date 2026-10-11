package units

import (
	"github.com/phyce/OpenSAPI4/whistler/datapack"
	"github.com/phyce/OpenSAPI4/whistler/internal/numeric"
	"github.com/phyce/OpenSAPI4/whistler/synthesis"
)

type RecordKind int

const (
	PhoneRecord RecordKind = iota
	BoundaryRecord
	ControlRecord
)

// Record is one of the front end's records (units.md §2). A phone has its duration code and up to five
// points.
type Record struct {
	Kind     RecordKind
	Name     string
	Duration float32
	Points   []Point
	Control  Control
}

// Point is a phone's pitch and amplitude at a position in it, from 0 to 1 (units.md §7). A pitch or
// amplitude of 0 is unset.
type Point struct {
	Position  float32
	Pitch     float32
	Amplitude float32
}

type ControlKind int

const (
	WordPosition ControlKind = iota + 1
	Bookmark
	Pause
	Character
	Reset
	Speed
	Volume
	// Unparsed is a tag the voice can't read. It is passed on, and no stage uses it.
	Unparsed
)

type Control struct {
	Kind  ControlKind
	Value uint32
	Name  string
}

type Frame struct {
	synthesis.Frame
	Phone int
}

type Item struct {
	Frame   *Frame
	Control Control
}

const (
	minimumSpeed     = 30
	defaultPitch     = 110
	defaultAmplitude = float32(110)
)

var (
	slowSpeedFraction = float32(2.0 / 3.0)
	volumeScale       = float32(1.0 / 65535)
	thousandth        = float32(0.001)
)

var renamedPhones = map[string]string{"DX": "T", "AW0": "AW1", "AY0": "AY1", "EY0": "EY1", "OY0": "OY1"}

type Mapper struct {
	voice     *datapack.Voice
	phones    map[string]int
	baseSpeed int
	threshold int

	speed  int
	slow   bool
	scale  float32
	offset float32
	volume uint16

	pitchCarry     int32
	amplitudeCarry float32
}

func NewMapper(voice *datapack.Voice, baseSpeed int) *Mapper {
	mapper := &Mapper{
		voice:     voice,
		phones:    map[string]int{},
		baseSpeed: baseSpeed,
		threshold: int(numeric.Truncate(float64(baseSpeed) * float64(slowSpeedFraction))),
		speed:     baseSpeed,
		volume:    0xFFFF,
	}
	for phone, name := range voice.Phones {
		if voice.HasTree(phone) {
			mapper.phones[name] = phone
		}
	}
	mapper.updateSpeedScale()
	return mapper
}

func (mapper *Mapper) Speed() int {
	return mapper.speed
}

func (mapper *Mapper) Volume() uint16 {
	return mapper.volume
}

func (mapper *Mapper) maximumSpeed() uint32 {
	return uint32(numeric.Truncate(float64(mapper.baseSpeed) * 3))
}

func (mapper *Mapper) SetSpeed(value uint32) bool {
	switch {
	case value == 0xFFFFFFFF:
		mapper.speed = int(mapper.maximumSpeed())
	case value == 0:
		mapper.speed = minimumSpeed
	case value < minimumSpeed || value > mapper.maximumSpeed():
		return false
	default:
		mapper.speed = int(value)
	}
	mapper.updateSpeedScale()
	return true
}

func (mapper *Mapper) ResetSpeed() {
	mapper.speed = mapper.baseSpeed
	mapper.updateSpeedScale()
}

func (mapper *Mapper) SetVolume(value uint32) {
	mapper.volume = uint16(value)
}

func (mapper *Mapper) updateSpeedScale() {
	mapper.slow = mapper.speed <= mapper.threshold
	if mapper.slow {
		mapper.scale = float32(float64(mapper.threshold-mapper.speed)/(float64(mapper.threshold)-20) + 1.5)
		mapper.offset = float32((1/float64(mapper.speed) - float64(mapper.scale)/float64(mapper.baseSpeed)) * 55)
		return
	}
	mapper.scale = float32(float64(mapper.baseSpeed) / float64(mapper.speed))
	mapper.offset = 0
}

func (mapper *Mapper) phoneNumber(name string) int {
	if renamed, found := renamedPhones[name]; found {
		name = renamed
	}
	if phone, found := mapper.phones[name]; found {
		return phone
	}
	return mapper.voice.SilencePhone
}

// Map turns a batch of records into frames and controls. following is the kind of the record after the
// batch, which the voice reads when the batch ends with a phone (units.md §4).
func (mapper *Mapper) Map(records []Record, following RecordKind) []Item {
	mapping := mapper.Begin(records, following)
	var items []Item
	for {
		next, _, more := mapping.Next()
		if !more {
			return items
		}
		items = append(items, next...)
	}
}

// Mapping is a batch the voice turns into frames one record at a time, as the original does (units.md §1):
// a speed or volume set between two words applies from the later one.
type Mapping struct {
	mapper  *Mapper
	records []Record
	phones  []phoneState
	index   int
	first   int
}

// Begin starts a batch: the phones' tree lookups, Sam's alternative units and the carries' reset.
func (mapper *Mapper) Begin(records []Record, following RecordKind) *Mapping {
	phones := mapper.lookupPhones(records, following)
	mapper.selectAlternativeUnits(records, phones, following)
	mapper.clearCarry(pitchPass)
	mapper.clearCarry(amplitudePass)
	return &Mapping{mapper: mapper, records: records, phones: phones}
}

// Next maps the batch's next tag or word: a tag's items, or a word's frames, which it reports as a word.
// It reports no more at the end of the batch.
func (mapping *Mapping) Next() (items []Item, word bool, more bool) {
	mapper, records := mapping.mapper, mapping.records
	for mapping.index < len(records) {
		switch records[mapping.index].Kind {
		case ControlRecord:
			mapping.index++
			return mapper.applyControl(nil, records[mapping.index-1].Control), false, true
		case BoundaryRecord:
			mapping.index++
		default:
			count := 1
			for mapping.index+count < len(records) && records[mapping.index+count].Kind == PhoneRecord {
				count++
			}
			next := mapper.voice.SilencePhone
			if mapping.first+count < len(mapping.phones) {
				next = mapping.phones[mapping.first+count].phone
			}
			items = mapper.buildWord(nil, records, mapping.phones[mapping.first:mapping.first+count], next)
			mapping.first += count
			mapping.index += count
			return items, true, true
		}
	}
	return nil, false, false
}

func (mapper *Mapper) applyControl(items []Item, control Control) []Item {
	switch control.Kind {
	case WordPosition, Bookmark:
		return append(items, Item{Control: control})
	case Pause:
		duration := float32(float64(control.Value) * float64(thousandth))
		return append(items, Item{Frame: &Frame{Frame: synthesis.Frame{Duration: duration}, Phone: mapper.voice.SilencePhone}})
	case Character:
		return append(items, Item{Control: Control{Kind: Character, Name: lowercase(control.Name)}})
	case Reset:
		mapper.SetVolume(0xFFFF)
		mapper.ResetSpeed()
		return append(items, Item{Control: Control{Kind: Volume, Value: uint32(mapper.volume)}}, Item{Control: Control{Kind: Speed, Value: uint32(mapper.speed)}},
			Item{Control: Control{Kind: Reset}})
	case Speed:
		if mapper.SetSpeed(control.Value) {
			return append(items, Item{Control: Control{Kind: Speed, Value: uint32(mapper.speed)}})
		}
	case Volume:
		mapper.SetVolume(control.Value)
		return append(items, Item{Control: Control{Kind: Volume, Value: uint32(mapper.volume)}})
	case Unparsed:
		return append(items, Item{Control: control})
	}
	return items
}

func lowercase(name string) string {
	characters := []byte(name)
	for index, character := range characters {
		if character >= 'A' && character <= 'Z' {
			characters[index] = character + 'a' - 'A'
		}
	}
	return string(characters)
}
