package units

import (
	"math"

	"github.com/phyce/OpenSAPI4/whistler/internal/numeric"
)

const (
	maximumPoints = 5
	pitchPass     = 0
	amplitudePass = 1
	noPitch       = -1
	noAmplitude   = float32(-1)
)

// workRecord is a phone of the word being built: its duration in seconds and its points. The slots hold
// the positions, the pitches and the amplitudes (as gains) back to back, as the original's work record
// does, so position slot 5 is pitch 0 and pitch slot 5 is amplitude 0 (units.md §7.3).
type workRecord struct {
	duration float32
	count    int
	slots    [3 * maximumPoints]float32
}

func position(point int) int {
	return point
}

func value(pass int, point int) int {
	return maximumPoints*(pass+1) + point
}

func (record *workRecord) clearPoint(point int) {
	record.slots[position(point)] = 0
	record.slots[value(pitchPass, point)] = 0
	record.slots[value(amplitudePass, point)] = 0
}

// same is the original's float equality test: its x87 compare reports an unordered result as equal, so a NaN
// equals everything (units.md §7.3).
func same(left float64, right float64) bool {
	return left == right || math.IsNaN(left) || math.IsNaN(right)
}

func unset(value float32) bool {
	return same(float64(value), 0)
}

func (mapper *Mapper) carrying(pass int) bool {
	if pass == pitchPass {
		return mapper.pitchCarry != noPitch
	}
	return !same(float64(mapper.amplitudeCarry), float64(noAmplitude))
}

func (mapper *Mapper) carry(pass int) float64 {
	if pass == pitchPass {
		return float64(mapper.pitchCarry)
	}
	return float64(mapper.amplitudeCarry)
}

func (mapper *Mapper) setCarry(pass int, value float32) {
	if pass == pitchPass {
		mapper.pitchCarry = numeric.Truncate(float64(value))
		return
	}
	mapper.amplitudeCarry = value
}

func (mapper *Mapper) setDefaultCarry(pass int) {
	if pass == pitchPass {
		mapper.pitchCarry = defaultPitch
		return
	}
	mapper.amplitudeCarry = defaultAmplitude
}

func (mapper *Mapper) clearCarry(pass int) {
	if pass == pitchPass {
		mapper.pitchCarry = noPitch
		return
	}
	mapper.amplitudeCarry = noAmplitude
}

// interpolate fills in every unset pitch (or amplitude) of a word by drawing straight lines in time between
// the set ones, starting from the value carried from the previous word (units.md §7.2). The pitch pass also
// gives every phone a point at 0 and one at 1.0. Products are converted before they are added, so that no
// platform fuses them.
func (mapper *Mapper) interpolate(work []workRecord, pass int, endsInSilence bool) {
	count := len(work)
	last := count - 1
	var startTime float32
	point, phone, startPoint := 0, 0, 0
	time := 0.0
	for {
		if phone >= last {
			if phone != last {
				break
			}
			if lastCount := work[last].count; point >= lastCount && lastCount > 1 {
				break
			}
		}
		found := false
		segmentStart := float32(time)
		index := phone
		for index < count {
			record := &work[index]
			for point < record.count && unset(record.slots[value(pass, point)]) {
				point++
			}
			if found = point < record.count; found {
				break
			}
			if index+1 != count {
				point = 0
			}
			time = time + float64(record.duration)
			index++
		}
		var endTime float32
		var endValue float64
		endPhone, endPoint := last, work[last].count
		if found {
			record := &work[index]
			switch {
			case !unset(record.slots[position(point)]):
				endTime = float32(float64(float64(record.duration)*float64(record.slots[position(point)])) + time)
			case index == 0:
				endTime = float32(time)
			case work[index-1].count == 0:
				endTime = float32(float64(work[index-1].duration) + time)
			default:
				previous := &work[index-1]
				endTime = float32(float64(float64(previous.slots[position(previous.count-1)])*float64(previous.duration)) + time)
			}
			endValue = float64(record.slots[value(pass, point)])
			if !mapper.carrying(pass) {
				mapper.setCarry(pass, record.slots[value(pass, point)])
			}
			endPhone, endPoint = index, point
		} else {
			endTime = float32(time)
			if !mapper.carrying(pass) {
				mapper.setDefaultCarry(pass)
			}
			endValue = float64(float32(mapper.carry(pass)))
		}
		time = float64(segmentStart)
		storedTime := segmentStart
		lastWritten := float32(-1)
		for current := phone; current <= endPhone; current++ {
			record := &work[current]
			if pass == pitchPass {
				if record.count == 0 {
					record.count = 1
					record.clearPoint(0)
					if current == endPhone {
						endPoint++
					}
				} else if !unset(record.slots[position(0)]) {
					record.count = min(record.count, maximumPoints-1)
					for slot := record.count; slot > 0; slot-- {
						record.slots[position(slot)] = record.slots[position(slot-1)]
						record.slots[value(pitchPass, slot)] = record.slots[value(pitchPass, slot-1)]
						record.slots[value(amplitudePass, slot)] = record.slots[value(amplitudePass, slot-1)]
					}
					record.clearPoint(0)
					record.count++
					if current == endPhone {
						endPoint++
					}
					time = float64(storedTime)
				}
				if !same(float64(record.slots[position(record.count-1)]), 1) {
					record.count = min(record.count, maximumPoints-1)
					record.clearPoint(record.count)
					record.slots[position(record.count)] = 1
					record.count++
				}
			}
			limit := record.count
			if current == endPhone {
				limit = endPoint
			}
			reached := startPoint
			if startPoint < limit {
				base := mapper.carry(pass)
				for slot := startPoint; slot < limit; slot++ {
					if !unset(record.slots[value(pass, slot)]) {
						continue
					}
					moment := float64(float64(record.duration)*float64(record.slots[position(slot)])) + time
					rise := float64((moment - float64(startTime)) * (endValue - base))
					record.slots[value(pass, slot)] = float32(rise/(float64(endTime)-float64(startTime)) + base)
					lastWritten = record.slots[value(pass, slot)]
				}
				reached = limit
			}
			startPoint = 0
			if reached == record.count {
				time = float64(record.duration) + time
				storedTime = float32(time)
			}
		}
		if !same(float64(lastWritten), -1) {
			mapper.setCarry(pass, lastWritten)
		}
		point, index = endPoint, endPhone
		for index < count {
			record := &work[index]
			if record.count == 0 || !unset(record.slots[position(0)]) {
				break
			}
			for point < record.count && !unset(record.slots[value(pass, point)]) {
				mapper.setCarry(pass, record.slots[value(pass, point)])
				startTime = float32(float64(float64(record.duration)*float64(record.slots[position(point)])) + time)
				point++
			}
			if point < record.count || !same(float64(record.slots[position(point-1)]), 1) {
				break
			}
			index++
			if index != count {
				point = 0
			}
			time = time + float64(record.duration)
		}
		phone, startPoint = index, point
	}
	if endsInSilence {
		mapper.clearCarry(pass)
	}
}
