package prosody

import (
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/phyce/OpenSAPI4/whistler/datapack"
)

type UnitKind int

const (
	PhoneUnit UnitKind = iota
	BoundaryUnit
	SilenceUnit
	TagUnit
)

// Unit is a unit record for the voice (prosody.md §10.4). A phone has its name with the stress
// digit, its duration code and its pitch in Hz; a silence its length in seconds; a tag the index of
// its control.
type Unit struct {
	Kind     UnitKind
	Name     string
	Duration float32
	Pitch    float32
	Tag      int
}

const (
	tagEnd            = 2000000000
	phoneTimePadding  = 1900000000
	pointTerminator   = 0x7FFFFFFF
	startingPitch     = 110.0
	glottalStop       = 70
	stepFraction      = 0.020833333333333332
	ownPitchStep      = 95
	analysisStringEnd = 0x14
)

var (
	thousandth        = float32(0.001)
	phraseLengthening = float32(1.2)
	finalLengthening  = float32(1.3)
	finalSilence      = float32(0.36)
)

var (
	pitchStepExceptions = map[int]int{25: -47, 100: 47, 200: 95, 800: 191, 12800: 383}
	stepBase            = new(big.Int).Exp(big.NewInt(50), big.NewInt(48), nil)
)

// PitchSteps turns a voice pitch in Hz into the analyser's pitch steps (prosody.md §10.1): the exact
// trunc(48 × log2(Hz / 50)), found by comparing Hz⁴⁸ with 50⁴⁸ × 2ⁿ, except where the original is off by one.
func PitchSteps(hertz int) int {
	hertz = max(hertz, 1)
	if steps, found := pitchStepExceptions[hertz]; found {
		return steps
	}
	larger, smaller := new(big.Int).Exp(big.NewInt(int64(hertz)), big.NewInt(48), nil), stepBase
	sign := 1
	if larger.Cmp(smaller) < 0 {
		larger, smaller, sign = smaller, larger, -1
	}
	steps := larger.BitLen() - smaller.BitLen()
	if new(big.Int).Lsh(smaller, uint(steps)).Cmp(larger) > 0 {
		steps--
	}
	return sign * steps
}

func stepsToHertz(value float32, pitchSteps int) float64 {
	return math.Pow(2, (float64(value)-float64(ownPitchStep-pitchSteps))*stepFraction) * 50
}

type parsedAnalysis struct {
	phones    []byte
	durations []int
	pitch     []float32
}

type timedPoint struct {
	time  int
	value int
}

func readNumber(text []byte, position int) (value int, next int, found bool) {
	for position < len(text) && strings.IndexByte(" \t\n\r\v\f", text[position]) >= 0 {
		position++
	}
	negative := false
	if position < len(text) && (text[position] == '+' || text[position] == '-') {
		negative = text[position] == '-'
		position++
	}
	digits := position
	for position < len(text) && text[position] >= '0' && text[position] <= '9' {
		value = value*10 + int(text[position]-'0')
		position++
	}
	if negative {
		value = -value
	}
	return value, position, position > digits
}

func parseAnalysis(text []byte) parsedAnalysis {
	var points []timedPoint
	time, value := 0, 0
	for position := 0; position < len(text) && text[position] != ']'; position++ {
		if text[position] != '(' {
			continue
		}
		number, next, found := readNumber(text, position+1)
		if found {
			time = number
			if next < len(text) && text[next] == ',' {
				if number, _, found := readNumber(text, next+1); found {
					value = number
				}
			}
		}
		points = append(points, timedPoint{time, value})
	}
	points = append(points, timedPoint{pointTerminator, 0})
	var parsed parsedAnalysis
	var ends []int
	total := 0
	for position := 0; position < len(text) && text[position] > analysisStringEnd; position++ {
		if text[position] != '[' || position == 0 {
			continue
		}
		duration, _, _ := readNumber(text, position+1)
		total += duration
		parsed.phones = append(parsed.phones, text[position-1])
		parsed.durations = append(parsed.durations, duration)
		ends = append(ends, total)
	}
	index := 0
	for _, end := range ends {
		for index+2 < len(points) && points[index+1].time < end {
			index++
		}
		point, next := points[index], points[min(index+1, len(points)-1)]
		span := float32(float64(next.time) - float64(point.time))
		fraction := (float64(end) - float64(point.time)) / float64(span)
		pitch := float64((1-fraction)*float64(point.value)) + float64(float64(next.value)*fraction)
		parsed.pitch = append(parsed.pitch, float32(pitch))
	}
	return parsed
}

type tagTime struct {
	time  int
	value int
}

type builder struct {
	tables          *datapack.Prosody
	analysis        parsedAnalysis
	phoneStartTimes []int
	tags            []tagTime
	pitchSteps      int
	singleCharacter bool
}

// BuildUnits turns an analysis and the transcribed tier 1 into unit records (prosody.md §10.3),
// running the count pass and the write pass. It also reports how many marker tags it placed.
func BuildUnits(tables *datapack.Prosody, analysis *Analysis, transcribed []byte, pitchSteps int, singleCharacter bool) ([]Unit, int) {
	builder := newBuilder(tables, analysis, pitchSteps, singleCharacter)
	copied := append([]byte{}, transcribed...)
	builder.build(copied, false)
	units, _, placed := builder.build(copied, true)
	return units, placed
}

func newBuilder(tables *datapack.Prosody, analysis *Analysis, pitchSteps int, singleCharacter bool) *builder {
	builder := &builder{tables: tables, analysis: parseAnalysis(analysis.Text), pitchSteps: pitchSteps, singleCharacter: singleCharacter}
	for _, event := range analysis.Events {
		switch event.Kind {
		case MarkerEvent:
			builder.tags = append(builder.tags, tagTime{event.Time * 10, event.Value})
		case PhoneEvent:
			builder.phoneStartTimes = append(builder.phoneStartTimes, event.Time*10)
		}
	}
	builder.tags = append(builder.tags, tagTime{tagEnd, -1})
	finalLength := 0
	for finalLength < analysis.End && analysis.Tier1[finalLength+1] != 0 {
		finalLength++
	}
	for len(builder.phoneStartTimes) <= finalLength {
		builder.phoneStartTimes = append(builder.phoneStartTimes, phoneTimePadding)
	}
	return builder
}

func (builder *builder) phoneAt(index int) byte {
	if index < len(builder.analysis.phones) {
		return builder.analysis.phones[index]
	}
	return 0
}

func (builder *builder) hertz(index int) float64 {
	if index >= len(builder.analysis.pitch) {
		index = len(builder.analysis.pitch) - 1
	}
	if index < 0 {
		return startingPitch
	}
	return stepsToHertz(builder.analysis.pitch[index], builder.pitchSteps)
}

func (builder *builder) phoneStartTime(index int) int {
	if index >= len(builder.phoneStartTimes) {
		return phoneTimePadding
	}
	return builder.phoneStartTimes[index]
}

func phone(name string, pitch float64) Unit {
	return Unit{Kind: PhoneUnit, Name: name, Duration: 1, Pitch: float32(pitch)}
}

func (builder *builder) build(transcribed []byte, write bool) (units []Unit, count int, placed int) {
	emit := func(unit Unit) {
		count++
		if write {
			units = append(units, unit)
		}
	}
	durations := builder.analysis.durations
	inBoundary, afterR, stressed := false, false, false
	current, following := startingPitch, startingPitch
	final := 0
	for cursor := 0; cursor < len(transcribed) && transcribed[cursor] != 0; cursor++ {
		code := transcribed[cursor]
		finalCode := builder.phoneAt(final)
		if finalCode == '#' || finalCode == '?' && durations[final] != glottalStop {
			if !inBoundary {
				emit(Unit{Kind: BoundaryUnit, Name: "#"})
			}
			emit(Unit{Kind: SilenceUnit, Name: "SIL", Duration: float32(float64(durations[final]) * float64(thousandth))})
			emit(Unit{Kind: BoundaryUnit, Name: "#"})
			inBoundary = true
			final++
			cursor--
			continue
		}
		if finalCode == '?' {
			final++
			cursor--
			continue
		}
		switch {
		case code == finalCode:
			current = builder.hertz(final)
			if builder.phoneAt(final+1) != 0 {
				following = builder.hertz(final + 1)
			}
			final++
		case code == 'R':
			current = builder.hertz(final)
			final++
		case code == 't' && finalCode == 'd':
			code = 'R'
			transcribed[cursor] = code
			current = builder.hertz(final)
			final++
		case code == 0x91 && finalCode == '$' || code == '$' && finalCode == 'i':
			code = finalCode
			transcribed[cursor] = code
			current = builder.hertz(final)
			final++
		}
		next := byte(0)
		if cursor+1 < len(transcribed) {
			next = transcribed[cursor+1]
		}
		switch {
		case strings.IndexByte("#.,;():!", code) >= 0:
			if !inBoundary {
				emit(Unit{Kind: BoundaryUnit, Name: "#"})
			}
			if write && strings.IndexByte(",;:)", code) >= 0 {
				lengthen(units, count-2, phraseLengthening)
			}
		case code == 0x16 || code == ' ' || code == '?':
			if !inBoundary {
				emit(Unit{Kind: BoundaryUnit, Name: "#"})
				inBoundary = true
			}
		case strings.IndexByte("~`%-\x0e\x10M\x9d\x11\x19G", code) >= 0:
		case code == '"' || code == '\'':
			stressed = true
		case code == 't' && next == 'S' || code == 'd' && next == 'Z':
			current = following
			name := "CH"
			if code == 'd' {
				name = "JH"
			}
			emit(phone(name, following))
			final++
			inBoundary = false
			cursor++
		case code == '$' && next == 'r':
			current = following
			name := "ER0"
			if stressed {
				name = "ER1"
			}
			emit(phone(name, following))
			inBoundary, afterR, stressed = false, true, false
		default:
			entry := builder.tables.PhoneNames[code]
			if entry.Name == "" {
				continue
			}
			if !(afterR && entry.Name[0] == 'R') && entry.Name[0] != ' ' && entry.Name[0] != '#' {
				name := entry.Name
				if entry.Vowel {
					if code == '$' {
						stressed = false
					}
					name += "0"
					if stressed {
						name = entry.Name + "1"
					}
					stressed = false
				}
				emit(phone(name, current))
				inBoundary = false
			}
			afterR = len(entry.Name) > 1 && entry.Name[1] == 'R'
		}
		if inBoundary {
			for builder.phoneStartTime(final) >= builder.tags[placed].time {
				emit(Unit{Kind: TagUnit, Name: tagName(builder.tags[placed].value), Tag: builder.tags[placed].value})
				placed++
			}
		}
	}
	if !inBoundary {
		emit(Unit{Kind: BoundaryUnit, Name: "#"})
	}
	if !write || !builder.singleCharacter {
		emit(Unit{Kind: SilenceUnit, Name: "SIL", Duration: finalSilence})
	}
	emit(Unit{Kind: BoundaryUnit, Name: "#"})
	if write {
		lengthen(units, count-2, finalLengthening)
	}
	return units, count, placed
}

func tagName(value int) string {
	return `\` + strconv.Itoa(value)
}

func lengthen(units []Unit, first int, value float32) {
	marked := false
	for index := first; index >= 0; index-- {
		if strings.IndexByte("AEIOU", units[index+1].Name[0]) >= 0 {
			return
		}
		unit := &units[index]
		if marked && unit.Kind == BoundaryUnit {
			return
		}
		if unit.Kind == PhoneUnit {
			unit.Duration = value
			marked = true
		}
	}
}
