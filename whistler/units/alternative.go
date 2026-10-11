package units

import (
	"slices"

	"github.com/phyce/OpenSAPI4/whistler/datapack"
)

type phoneState struct {
	record      int
	phone       int
	senone      int
	alternative int
}

func (mapper *Mapper) lookupPhones(records []Record, following RecordKind) []phoneState {
	var phones []phoneState
	for index, record := range records {
		if record.Kind == PhoneRecord {
			phones = append(phones, phoneState{record: index, phone: mapper.phoneNumber(record.Name)})
		}
	}
	for number := range phones {
		mapper.lookupSenone(records, phones, number, following)
	}
	return phones
}

func (mapper *Mapper) lookupSenone(records []Record, phones []phoneState, number int, following RecordKind) {
	state := &phones[number]
	if state.phone == mapper.voice.SilencePhone {
		state.senone = 0
		return
	}
	left, right := mapper.voice.SilencePhone, mapper.voice.SilencePhone
	if number > 0 {
		left = phones[number-1].phone
	}
	if number+1 < len(phones) {
		right = phones[number+1].phone
	}
	startsWord := state.record > 0 && records[state.record-1].Kind != PhoneRecord
	endsWord := following != PhoneRecord
	if state.record+1 < len(records) {
		endsWord = records[state.record+1].Kind != PhoneRecord
	}
	position := datapack.InsideWord
	switch {
	case startsWord && endsWord:
		position = datapack.SinglePhoneWord
	case startsWord:
		position = datapack.WordInitial
	case endsWord:
		position = datapack.WordFinal
	}
	state.senone = mapper.voice.Senone(state.phone, left, right, position)
}

type match struct {
	alternative *datapack.AlternativeUnit
	start       int
}

func (mapper *Mapper) selectAlternativeUnits(records []Record, phones []phoneState, following RecordKind) {
	alternatives := mapper.voice.AlternativeUnits
	if len(alternatives) == 0 {
		return
	}
	var matches []match
	claims := make([]int, len(phones))
	for start := range phones {
		for index := range alternatives {
			alternative := &alternatives[index]
			if !keyMatches(phones[start:], alternative.Key) {
				continue
			}
			matches = append(matches, match{alternative: alternative, start: start})
			first, length := alternative.Span()
			for position := start + first; position < start+first+length; position++ {
				if claims[position] == 0 {
					claims[position] = len(matches)
				}
			}
		}
	}
	var replaced []int
	for number, match := range matches {
		first, length := match.alternative.Span()
		begin := match.start + first
		if slices.ContainsFunc(claims[begin:begin+length], func(claim int) bool { return claim != number+1 }) {
			continue
		}
		replacement := 0
		for offset, code := range match.alternative.Codes {
			state := &phones[begin+offset]
			state.alternative = int(code & 0x1FFF)
			if code&0xC000 == 0xC000 {
				state.phone = match.alternative.ReplacementPhones[replacement]
				replacement++
				replaced = append(replaced, begin+offset)
			}
		}
	}
	for _, position := range replaced {
		for neighbour := max(position-1, 0); neighbour <= min(position+1, len(phones)-1); neighbour++ {
			mapper.lookupSenone(records, phones, neighbour, following)
		}
	}
}

func keyMatches(phones []phoneState, key []int) bool {
	if len(phones) < len(key) {
		return false
	}
	for index, phone := range key {
		if phones[index].phone != phone {
			return false
		}
	}
	return true
}
