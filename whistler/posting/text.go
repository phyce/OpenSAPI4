package posting

import (
	"strconv"
	"strings"
)

// fault is something the original does that the Go engine doesn't: it reads past the transplant text or the
// record buffer, or calls DebugBreak (engine.md §13.7). The transplant then fails.
type fault string

func (fault fault) Error() string {
	return string(fault)
}

const whitespace = " \t\n\v\f\r"

// cText is an upper-cased \Btp= text followed by its NUL, read with the C library calls the original makes on
// it. Reading past the NUL is a fault.
type cText string

func (text cText) at(index int) byte {
	if index < 0 || index >= len(text) {
		panic(fault("reads past the end of the \\Btp= text"))
	}
	return text[index]
}

// span is strspn: how many characters from index are in accepted.
func (text cText) span(index int, accepted string) int {
	count := 0
	for character := text.at(index); character != 0 && strings.IndexByte(accepted, character) >= 0; character = text.at(index + count) {
		count++
	}
	return count
}

// complementSpan is strcspn: how many characters from index are neither in stops nor the NUL.
func (text cText) complementSpan(index int, stops string) int {
	count := 0
	for character := text.at(index); character != 0 && strings.IndexByte(stops, character) < 0; character = text.at(index + count) {
		count++
	}
	return count
}

// find is strchr: the index of the first character from index, or -1 at the NUL.
func (text cText) find(index int, character byte) int {
	for ; ; index++ {
		switch text.at(index) {
		case character:
			return index
		case 0:
			return -1
		}
	}
}

func isDigit(character byte) bool {
	return character >= '0' && character <= '9'
}

func (text cText) skipWhitespace(index int) int {
	for character := text.at(index); character != 0 && strings.IndexByte(whitespace, character) >= 0; character = text.at(index) {
		index++
	}
	return index
}

// atoi is msvcrt's atoi: whitespace, a sign and decimal digits, wrapping in 32 bits.
func (text cText) atoi(index int) int32 {
	index = text.skipWhitespace(index)
	negative := text.at(index) == '-'
	if character := text.at(index); character == '+' || character == '-' {
		index++
	}
	var value uint32
	for ; isDigit(text.at(index)); index++ {
		value = value*10 + uint32(text.at(index)-'0')
	}
	if negative {
		value = -value
	}
	return int32(value)
}

// atof is msvcrt's atof: whitespace, a sign, digits with an optional point, and an exponent after E or D.
func (text cText) atof(index int) float64 {
	index = text.skipWhitespace(index)
	start := index
	if character := text.at(index); character == '+' || character == '-' {
		index++
	}
	digits := 0
	for ; isDigit(text.at(index)); index++ {
		digits++
	}
	if text.at(index) == '.' {
		for index++; isDigit(text.at(index)); index++ {
			digits++
		}
	}
	if digits == 0 {
		return 0
	}
	number := string(text[start:index])
	if character := text.at(index); character == 'E' || character == 'D' {
		scan := index + 1
		sign := ""
		if character := text.at(scan); character == '+' || character == '-' {
			sign = string(character)
			scan++
		}
		exponent := scan
		for isDigit(text.at(scan)) {
			scan++
		}
		if scan > exponent {
			number += "e" + sign + string(text[exponent:scan])
		}
	}
	value, _ := strconv.ParseFloat(number, 64)
	return value
}

func hexDigit(character byte) (uint32, bool) {
	switch {
	case isDigit(character):
		return uint32(character - '0'), true
	case character >= 'A' && character <= 'F':
		return uint32(character-'A') + 10, true
	}
	return 0, false
}

// scanHex is one %x of msvcrt's sscanf: whitespace, a sign, an optional 0X and hexadecimal digits, wrapping in
// 32 bits. It reports false, and where the whitespace ended, when nothing converts.
func (text cText) scanHex(index int) (uint32, int, bool) {
	index = text.skipWhitespace(index)
	if text.at(index) == 0 {
		return 0, index, false
	}
	scan := index
	negative := false
	if character := text.at(scan); character == '+' || character == '-' {
		negative = character == '-'
		scan++
	}
	var value uint32
	digits := 0
	if text.at(scan) == '0' && text.at(scan+1) == 'X' {
		scan += 2
		digits = 1
	}
	for {
		digit, valid := hexDigit(text.at(scan))
		if !valid {
			break
		}
		value = value*16 + digit
		scan++
		digits++
	}
	if digits == 0 {
		return 0, index, false
	}
	if negative {
		value = -value
	}
	return value, scan, true
}

// scanCodes is sscanf(text, "%x %x %x"): up to three numbers. The count is -1 when the text ends before the
// first.
func (text cText) scanCodes(index int) (int, []uint32) {
	var values []uint32
	for len(values) < 3 {
		value, next, converted := text.scanHex(index)
		index = next
		if !converted {
			if len(values) == 0 && text.at(index) == 0 {
				return -1, nil
			}
			break
		}
		values = append(values, value)
	}
	return len(values), values
}
