package prosody

import (
	"strconv"

	"github.com/phyce/OpenSAPI4/whistler/datapack"
)

// Target is a pitch target: a value in steps of 1/48 octave and the time to the next target in
// tenths of a millisecond (0 for the last).
type Target struct {
	Flag  byte
	Value int16
	Time  int16
}

const (
	toneLimit      = 620
	fillInterval   = 2400
	fillMargin     = 2500
	closingGap     = 3100
	closingStep    = 3000
	defaultType    = 4
	declinationCap = 512
)

var toneRecords = map[byte][]int{
	'1': {0}, '2': {4}, '3': {2}, '4': {3}, '5': {1}, '7': {10, 5}, '8': {5}, '9': {6}, 'A': {13}, 'B': {17}, 'F': {18},
	'H': {4, 15}, 'I': {3, 15}, 'L': {30}, 'O': {29}, 'P': {0, 9}, 'T': {24}, 'U': {26}, 'X': {31}, 'Y': {22}, 'Z': {32},
	']': {21}, 'a': {10}, 'b': {16}, 'c': {8}, 'f': {0, 18}, 'h': {1, 9}, 'j': {7}, 'l': {11}, 'p': {2, 14}, 't': {23},
	'u': {25}, 'x': {27}, 'z': {28}, '(': {19}, ')': {20},
}

var sentenceTypes = map[byte]int{'S': 0, 'q': 2, 'W': 3}

func (analyser *analyser) assignEventTimes(sentence *Sentence) []Event {
	anchors := make([]byte, analyser.end+2)
	for cell := 1; cell < len(anchors) && cell < len(sentence.Anchors); cell++ {
		anchors[cell] = sentence.Anchors[cell]
	}
	queue := sentence.Events
	if len(queue) > 0 && queue[0].Kind == SentenceEvent && anchors[analyser.start] == 0 {
		cell := analyser.start
		for cell <= analyser.end && anchors[cell] == 0 {
			cell++
		}
		anchors[analyser.start] = 1
		anchors[cell]--
	}
	var events []Event
	head := 0
	time := 0
	for cell := analyser.start; cell <= analyser.end; cell++ {
		for ; anchors[cell] != 0 && head < len(queue); anchors[cell]-- {
			events = append(events, Event{Kind: queue[head].Kind, Value: queue[head].Value, Time: time})
			head++
		}
		if analyser.tier1[cell] != '-' {
			events = append(events, Event{Kind: PhoneEvent, Value: int(analyser.tier1[cell]), Time: time})
		}
		time += int(analyser.durations[cell])
	}
	return events
}

type target struct {
	flag  byte
	value int16
	time  uint16
}

type intonation struct {
	targets    []target
	baseline   int16
	cumulative uint16
}

func (intonation *intonation) last() *target {
	return &intonation.targets[len(intonation.targets)-1]
}

func (intonation *intonation) addTone(tone datapack.Tone, duration int16) {
	count := len(intonation.targets) - 1
	if count >= toneLimit {
		return
	}
	lastTime := int(intonation.targets[count].time)
	time := int(tone.TimeOffset) + int(intonation.cumulative)
	if tone.FromPhoneEnd {
		time += int(duration)
	}
	shift := 0
	if time < 0 {
		shift, time = -time, 0
	} else if lastTime > int(uint16(time)) {
		shift = (lastTime - int(uint16(time))) / 2
		if int(tone.Duration) < int(uint16(shift)) {
			return
		}
		time += shift
		if count >= 2 && uint16(time) < intonation.targets[count-1].time {
			return
		}
		intonation.targets[count].time = uint16(time)
	}
	lastTime = int(intonation.targets[count].time)
	if int(uint16(time)) > lastTime {
		slope := (int(intonation.targets[count].value) - int(intonation.baseline) - int(tone.Level)) / int(int16(time-lastTime))
		for anchor := lastTime; int(uint16(anchor))+fillMargin < int(uint16(time)); {
			anchor += fillInterval
			fill := target{flag: '0', time: uint16(anchor)}
			fill.value = int16(int(intonation.last().value) - int(uint16(int(fill.time)-lastTime))*slope)
			intonation.targets = append(intonation.targets, fill)
		}
		intonation.targets = append(intonation.targets, target{flag: tone.Mark, value: int16(int(intonation.baseline) + int(tone.Level)), time: uint16(time)})
	} else {
		intonation.last().flag = tone.Mark
	}
	intonation.baseline = int16(int(intonation.baseline) + int(tone.BaselineStep))
	second := int16(int(tone.SecondOffset) + int(tone.Level) + int(intonation.baseline))
	intonation.targets = append(intonation.targets, target{flag: ' ', value: second, time: uint16(int(tone.Duration) - int(uint16(shift)) + time)})
}

func (intonation *intonation) appendCopy(time int) {
	last := *intonation.last()
	flag := byte('0')
	if last.flag == '*' {
		flag = '*'
	}
	intonation.targets = append(intonation.targets, target{flag: flag, value: last.value, time: uint16(time)})
}

func (analyser *analyser) intonate(sentence *Sentence) []Target {
	durations := analyser.durations
	points := append([]Point{{}}, sentence.Points...)
	running := uint16(0)
	number := 0
	for cell := analyser.start; cell <= analyser.end; cell++ {
		if cell < len(sentence.PointIndex) && sentence.PointIndex[cell] != 0 {
			index := int(sentence.PointIndex[cell])
			for ; number >= 0 && number < index && number+1 < len(points); number++ {
				points[number+1].Time += int32(int16(running))
			}
			number = index
		}
		running += uint16(durations[cell])
	}
	intonation := &intonation{targets: []target{{}, {flag: '0'}}}
	sentenceType := defaultType
	for cell := analyser.start; cell <= analyser.end; cell++ {
		mark := analyser.tier5[cell]
		if records, found := toneRecords[mark]; found {
			for _, record := range records {
				intonation.addTone(analyser.tables.Tones[record], durations[cell])
			}
		} else if value, found := sentenceTypes[mark]; found {
			sentenceType = value
		}
		intonation.cumulative += uint16(durations[cell])
	}
	for _, point := range points[1:] {
		position := 1
		for position < len(intonation.targets) && point.Time >= int32(intonation.targets[position].time) {
			position++
		}
		explicit := target{flag: '*', value: point.Value, time: uint16(point.Time)}
		intonation.targets = append(intonation.targets[:position], append([]target{explicit}, intonation.targets[position:]...)...)
	}
	for int(intonation.cumulative)-int(intonation.last().time) > closingGap {
		intonation.appendCopy(int(intonation.last().time) + closingStep)
	}
	if intonation.cumulative > intonation.last().time {
		intonation.appendCopy(int(intonation.cumulative))
	}
	return analyser.decline(intonation, sentenceType)
}

func (analyser *analyser) decline(intonation *intonation, sentenceType int) []Target {
	durations := analyser.durations
	partial := uint16(0)
	for cell := analyser.start + 1; cell < analyser.end; cell++ {
		partial += uint16(durations[cell])
	}
	length := int(int16(uint16(durations[analyser.start]) + partial))
	divisor := 1
	for length > declinationCap {
		length /= 2
		divisor *= 2
	}
	quotient := int(uint16(int(partial) / divisor))
	perThousand := int(uint16(1000 / divisor))
	declination := analyser.tables.Declination[sentenceType]
	floor, rate := int(declination.Floor), int(declination.Slope)
	ceiling := int(uint16(100-floor)) * perThousand
	if quotient*rate > ceiling {
		rate = ceiling / quotient
	}
	baseline := int(intonation.baseline)
	if baseline > 0 && baseline > int(uint16(rate))*quotient/perThousand && quotient != 0 {
		rate = baseline*perThousand/quotient + 1
	}
	targets := intonation.targets
	count := len(targets) - 1
	times := make([]uint16, count+2)
	for index := 1; index <= count; index++ {
		times[index] = targets[index].time
	}
	times[count+1] = intonation.cumulative
	result := make([]Target, count+1)
	for index := 1; index <= count; index++ {
		value := int(targets[index].value)
		if targets[index].flag != '*' {
			position := int(times[index]) / divisor
			if length > int(uint16(position)) {
				value += int(int32(uint32(uint16(length-position))*uint32(uint16(rate))))/perThousand - baseline + floor
			} else {
				value += floor - baseline
			}
		}
		delta := int16(uint16(int(times[index+1])*10) - uint16(int(times[index])*10))
		if delta == 0 {
			delta = 1
		}
		result[index] = Target{Flag: targets[index].flag, Value: int16(value), Time: delta}
	}
	result[count].Time = 0
	return result[1:]
}

func (analyser *analyser) analysisText(targets []Target) []byte {
	start := analyser.start
	text := []byte{0x1B, '/', analyser.tier1[start], '['}
	text = strconv.AppendInt(text, int64(analyser.durations[start]), 10)
	accumulated := 0
	for _, target := range targets {
		text = append(text, '(')
		text = strconv.AppendInt(text, int64(uint16(accumulated)), 10)
		text = append(text, ',')
		text = strconv.AppendInt(text, int64(target.Value), 10)
		text = append(text, ')')
		accumulated += int(uint16(target.Time)) / 10
	}
	text = append(text, ']')
	for cell := start + 1; cell <= analyser.end; cell++ {
		if analyser.tier1[cell] == '-' {
			text = append(text, '-')
			continue
		}
		text = append(text, analyser.tier1[cell], '[')
		text = strconv.AppendInt(text, int64(analyser.durations[cell]), 10)
		text = append(text, ']')
	}
	return append(text, 0x1B, '/', 0x1B, 'E', ' ')
}
