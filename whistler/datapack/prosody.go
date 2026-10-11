package datapack

import (
	"encoding/json"
	"fmt"
	"math"
)

const (
	UnitClassCount   = 42
	ToneCount        = 33
	SentenceTypes    = 5
	pairFlag         = 0x2000
	dashFlag         = 0x4000
	acrossDashLimit  = 0xA6
	acrossDashOffset = 0x561
)

type Tone struct {
	Mark         byte
	Level        int16
	SecondOffset int16
	Duration     uint16
	FromPhoneEnd bool
	TimeOffset   int16
	BaselineStep int16
}

type Declination struct {
	Floor uint16
	Slope uint16
}

type PhoneName struct {
	Name  string
	Vowel bool
}

type Prosody struct {
	PhoneClasses            [256]byte
	PhoneFeatures           [256]byte
	InherentDurations       [256]int16
	MinimumDurations        [256]byte
	FollowingFactors        [256]byte
	AfterApproximantFactors [256]byte
	UnitClasses             [256]byte
	UnitPairs               [UnitClassCount][UnitClassCount]uint16
	UnitLists               map[int][]map[string]int
	UnitFrames              [][]byte
	Tones                   [ToneCount]Tone
	Declination             [SentenceTypes]Declination
	PhoneNames              [256]PhoneName
	PhoneCodes              []byte

	defaultUnit int
}

type unitStringDescription struct {
	Phones []int `json:"phones"`
	Unit   int   `json:"unit"`
}

type unitListDescription struct {
	List    int                       `json:"list"`
	Lengths [][]unitStringDescription `json:"lengths"`
}

type toneDescription struct {
	Mark         int  `json:"mark"`
	Level        int  `json:"level"`
	SecondOffset int  `json:"secondOffset"`
	Duration     int  `json:"duration"`
	FromPhoneEnd bool `json:"fromPhoneEnd"`
	TimeOffset   int  `json:"timeOffset"`
	BaselineStep int  `json:"baselineStep"`
}

type declinationDescription struct {
	Floor int `json:"floor"`
	Slope int `json:"slope"`
}

type phoneNameDescription struct {
	Code  int    `json:"code"`
	Name  string `json:"name"`
	Vowel bool   `json:"vowel"`
}

type prosodyDescription struct {
	PhoneClasses            []int                    `json:"phoneClasses"`
	PhoneFeatures           []int                    `json:"phoneFeatures"`
	InherentDurations       []int                    `json:"inherentDurations"`
	MinimumDurations        []int                    `json:"minimumDurations"`
	FollowingFactors        []int                    `json:"followingFactors"`
	AfterApproximantFactors []int                    `json:"afterApproximantFactors"`
	UnitClasses             []int                    `json:"unitClasses"`
	UnitPairs               [][]int                  `json:"unitPairs"`
	UnitLists               []unitListDescription    `json:"unitLists"`
	UnitFrames              [][]int                  `json:"unitFrames"`
	Tones                   []toneDescription        `json:"tones"`
	Declination             []declinationDescription `json:"declination"`
	PhoneNames              []phoneNameDescription   `json:"phoneNames"`
}

func (pack *Pack) LoadProsody() (*Prosody, error) {
	return load(pack, "prosody.json", parseProsody)
}

func parseProsody(data []byte) (*Prosody, error) {
	var description prosodyDescription
	if err := json.Unmarshal(data, &description); err != nil {
		return nil, err
	}
	prosody := &Prosody{}
	byteTables := []struct {
		name   string
		values []int
		table  *[256]byte
	}{
		{"phone classes", description.PhoneClasses, &prosody.PhoneClasses},
		{"phone features", description.PhoneFeatures, &prosody.PhoneFeatures},
		{"minimum durations", description.MinimumDurations, &prosody.MinimumDurations},
		{"following factors", description.FollowingFactors, &prosody.FollowingFactors},
		{"after-approximant factors", description.AfterApproximantFactors, &prosody.AfterApproximantFactors},
		{"unit classes", description.UnitClasses, &prosody.UnitClasses},
	}
	for _, table := range byteTables {
		if err := checkValues(table.name, table.values, 256, 0, math.MaxUint8); err != nil {
			return nil, err
		}
		for code, value := range table.values {
			table.table[code] = byte(value)
		}
	}
	if err := checkValues("inherent durations", description.InherentDurations, 256, math.MinInt16, math.MaxInt16); err != nil {
		return nil, err
	}
	for code, value := range description.InherentDurations {
		prosody.InherentDurations[code] = int16(value)
	}
	if err := prosody.parseUnits(&description); err != nil {
		return nil, err
	}
	if err := prosody.parseIntonation(&description); err != nil {
		return nil, err
	}
	for _, entry := range description.PhoneNames {
		if entry.Code < 1 || entry.Code > math.MaxUint8 || prosody.PhoneNames[entry.Code].Name != "" {
			return nil, fmt.Errorf("phone name code %d", entry.Code)
		}
		if len(entry.Name) < 1 || len(entry.Name) > 4 {
			return nil, fmt.Errorf("phone name %q", entry.Name)
		}
		for _, character := range []byte(entry.Name) {
			if character < 0x21 || character > 0x7E {
				return nil, fmt.Errorf("phone name %q", entry.Name)
			}
		}
		prosody.PhoneNames[entry.Code] = PhoneName{Name: entry.Name, Vowel: entry.Vowel}
		prosody.PhoneCodes = append(prosody.PhoneCodes, byte(entry.Code))
	}
	return prosody, nil
}

func checkValues(name string, values []int, count int, minimum int, maximum int) error {
	if len(values) != count {
		return fmt.Errorf("%s: %d values, expected %d", name, len(values), count)
	}
	for index, value := range values {
		if value < minimum || value > maximum {
			return fmt.Errorf("%s: value %d at %d", name, value, index)
		}
	}
	return nil
}

func (prosody *Prosody) parseUnits(description *prosodyDescription) error {
	for code, unitClass := range prosody.UnitClasses {
		if unitClass >= UnitClassCount {
			return fmt.Errorf("unit class %d of code %d", unitClass, code)
		}
	}
	if len(description.UnitPairs) != UnitClassCount {
		return fmt.Errorf("%d unit pair rows", len(description.UnitPairs))
	}
	for left, row := range description.UnitPairs {
		if err := checkValues("unit pairs", row, UnitClassCount, 0, math.MaxUint16); err != nil {
			return err
		}
		for right, value := range row {
			if value&0x8000 != 0 || value&(pairFlag|dashFlag) == pairFlag|dashFlag {
				return fmt.Errorf("unit pair value 0x%04x", value)
			}
			prosody.UnitPairs[left][right] = uint16(value)
		}
	}
	prosody.UnitFrames = make([][]byte, len(description.UnitFrames))
	for index, frames := range description.UnitFrames {
		if len(frames) < 2 || len(frames) > 5 {
			return fmt.Errorf("unit %d has %d phones", index+1, len(frames))
		}
		if err := checkValues("unit frames", frames, len(frames), 0, math.MaxUint8); err != nil {
			return err
		}
		prosody.UnitFrames[index] = make([]byte, len(frames))
		for phone, count := range frames {
			prosody.UnitFrames[index][phone] = byte(count)
		}
	}
	prosody.defaultUnit = int(prosody.UnitPairs[prosody.UnitClasses['#']][prosody.UnitClasses['t']] & 0x1FFF)
	for left := range UnitClassCount {
		for right := range UnitClassCount {
			for _, acrossDash := range []bool{false, true} {
				if _, unit := prosody.pairUnits(prosody.UnitPairs[left][right], acrossDash); prosody.UnitPhones(unit) != 2 {
					return fmt.Errorf("unit pair %d, %d falls back to unit %d, which is not a two-phone unit", left, right, unit)
				}
			}
		}
	}
	prosody.UnitLists = map[int][]map[string]int{}
	for _, list := range description.UnitLists {
		if _, found := prosody.UnitLists[list.List]; found || list.List < 1 || len(list.Lengths) > 3 {
			return fmt.Errorf("unit list %d with %d lengths", list.List, len(list.Lengths))
		}
		lengths := make([]map[string]int, len(list.Lengths))
		for index, entries := range list.Lengths {
			lengths[index] = map[string]int{}
			for _, entry := range entries {
				if err := checkValues("unit string", entry.Phones, index+1, 0, math.MaxUint8); err != nil {
					return fmt.Errorf("unit list %d: %w", list.List, err)
				}
				phones := make([]byte, len(entry.Phones))
				for position, phone := range entry.Phones {
					phones[position] = byte(phone)
				}
				if _, found := lengths[index][string(phones)]; found || prosody.UnitPhones(entry.Unit) != index+3 {
					return fmt.Errorf("unit list %d: string %v gives unit %d", list.List, entry.Phones, entry.Unit)
				}
				lengths[index][string(phones)] = entry.Unit
			}
		}
		prosody.UnitLists[list.List] = lengths
	}
	return nil
}

func (prosody *Prosody) parseIntonation(description *prosodyDescription) error {
	if len(description.Tones) != ToneCount {
		return fmt.Errorf("%d tone records, expected %d", len(description.Tones), ToneCount)
	}
	for index, tone := range description.Tones {
		signed := []int{tone.Level, tone.SecondOffset, tone.TimeOffset, tone.BaselineStep}
		if err := checkValues("tone record", signed, len(signed), math.MinInt16, math.MaxInt16); err != nil {
			return err
		}
		if tone.Mark < 0 || tone.Mark > math.MaxUint8 || tone.Duration < 0 || tone.Duration > math.MaxUint16 {
			return fmt.Errorf("tone record %d: mark %d, duration %d", index, tone.Mark, tone.Duration)
		}
		prosody.Tones[index] = Tone{
			Mark:         byte(tone.Mark),
			Level:        int16(tone.Level),
			SecondOffset: int16(tone.SecondOffset),
			Duration:     uint16(tone.Duration),
			FromPhoneEnd: tone.FromPhoneEnd,
			TimeOffset:   int16(tone.TimeOffset),
			BaselineStep: int16(tone.BaselineStep),
		}
	}
	if len(description.Declination) != SentenceTypes {
		return fmt.Errorf("%d declination entries, expected %d", len(description.Declination), SentenceTypes)
	}
	for index, entry := range description.Declination {
		if err := checkValues("declination", []int{entry.Floor, entry.Slope}, 2, 0, math.MaxUint16); err != nil {
			return err
		}
		prosody.Declination[index] = Declination{Floor: uint16(entry.Floor), Slope: uint16(entry.Slope)}
	}
	return nil
}

func (prosody *Prosody) UnitPhones(unit int) int {
	if unit < 1 || unit > len(prosody.UnitFrames) {
		return 0
	}
	return len(prosody.UnitFrames[unit-1])
}

func (prosody *Prosody) UnitPair(left byte, right byte, acrossDash bool) (list int, fallback int) {
	return prosody.pairUnits(prosody.UnitPairs[prosody.UnitClasses[left]][prosody.UnitClasses[right]], acrossDash)
}

func (prosody *Prosody) pairUnits(value uint16, acrossDash bool) (list int, fallback int) {
	list = int(value & 0x1FFF)
	fallback = list
	if acrossDash {
		if list > 0 && list <= acrossDashLimit {
			list += acrossDashOffset
		}
		fallback = list
		if value&dashFlag != 0 {
			fallback = int(value & 0x1FFF)
		}
	} else if value&pairFlag != 0 && fallback > 0 && fallback <= acrossDashLimit {
		fallback += acrossDashOffset
	}
	if fallback == 0 {
		fallback = prosody.defaultUnit
	}
	return list, fallback
}
