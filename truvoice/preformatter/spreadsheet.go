package preformatter

import (
	"slices"
	"strings"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
)

const (
	escapeStart        = "\x1b["
	fractionMarks      = "!,.:;?"
	numberSpaces       = " \t\n\v\f\r"
	longestMonthNumber = 3
)

// sheet is the spreadsheet context's words and the phrases made of them.
type sheet struct {
	words *datapack.Spreadsheet
	// minus is "minus ", written for a dash and a bracket; spokenExponent is what a minus line writes for E-,
	// "E minus ".
	minus, spokenExponent string
	// exponent and negativeExponent are " times 10 to the " and " times 10 to the minus ".
	exponent, negativeExponent string
	// andDash and dashAnd are " and -" and "- and ".
	andDash, dashAnd string
}

func newSheet(words *datapack.Spreadsheet) *sheet {
	minus := words.Minus + " "
	exponent := " " + words.Exponent + " "
	return &sheet{words: words, minus: minus, spokenExponent: "E " + minus, exponent: exponent,
		negativeExponent: exponent + minus, andDash: " " + words.And + " -", dashAnd: "- " + words.And + " "}
}

// line writes a line (section 2): the escape sequences it starts with as they are, then the rest.
func (sheet *sheet) line(line string) string {
	end := escapes(line)
	return line[:end] + sheet.transform(line[end:])
}

// escapes is where the escape sequences a line starts with end, with the spaces before and between them: each
// is ESC [ up to a letter. One without a letter takes the rest of the line.
func escapes(line string) int {
	index, end := 0, 0
	for index < len(line) {
		if line[index] == ' ' {
			index++
			continue
		}
		if !strings.HasPrefix(line[index:], escapeStart) {
			break
		}
		final := index + len(escapeStart)
		for final < len(line) && !letter(line[final]) {
			final++
		}
		if final == len(line) {
			return final
		}
		index = final + 1
		end = index
	}
	return end
}

// transform writes a line's date, or its fraction, minus signs, brackets and exponents (sections 2.1 to 2.5).
func (sheet *sheet) transform(line string) string {
	slash := strings.IndexByte(line, '/')
	if slash >= 0 && strings.IndexByte(line[slash+1:], '/') >= 0 {
		return sheet.date(line, slash)
	}
	if slash >= 0 {
		line = sheet.fraction(line, slash)
	}
	if strings.IndexByte(line, '-') >= 0 {
		switch {
		case speaksMinus(line):
			line = sheet.minusLine(line)
		case exponentDash(line):
			line = strings.ReplaceAll(line, "E-", sheet.negativeExponent)
			line = strings.ReplaceAll(line, "E+", sheet.exponent)
			line = strings.ReplaceAll(line, sheet.spokenExponent, sheet.negativeExponent)
		default:
			return line
		}
	}
	return sheet.exponents(sheet.brackets(line))
}

// date writes the month before a line's first slash as its name, and the slashes after it as commas (section
// 2.1). A month of more than three characters leaves the line as it is.
func (sheet *sheet) date(line string, slash int) string {
	start := strings.LastIndexByte(line[:slash], ' ') + 1
	token := line[start:slash]
	month := number(token)
	if month < 1 || month > datapack.MonthCount || len(token) > longestMonthNumber {
		return line
	}
	return line[:start] + sheet.words.Months[month-1] + " " + strings.ReplaceAll(line[slash+1:], "/", ", ")
}

// fraction writes a line's fraction (section 2.2): the denominator after its slash as a word, and "and" after the
// line's first word.
func (sheet *sheet) fraction(line string, slash int) string {
	numerator := ""
	if slash > 0 {
		start := slash - 1
		for start > 0 && line[start-1] != ' ' {
			start--
		}
		numerator = line[start:slash]
	}
	if dash := strings.IndexByte(numerator, '-'); dash >= 0 {
		numerator = numerator[dash+1:]
	}
	end := strings.IndexByte(line[slash+1:], ' ')
	if end < 0 {
		end = len(line)
	} else {
		end += slash + 1
	}
	token := line[slash+1 : end]
	value := number(numerator)
	if value == 0 || number(token) == 0 {
		return line
	}
	denominator := strings.TrimRight(token, fractionMarks)
	marks := []byte(token[len(denominator):])
	slices.Reverse(marks)
	spoken := "/" + denominator + string(marks)
	if len(denominator) <= 2 {
		spoken = sheet.denominator(denominator)
		if value != 1 {
			spoken += "s"
		}
		spoken += string(marks)
	}
	output := line[:slash] + " " + spoken
	if end+1 < len(line) {
		output += " " + line[end+1:]
	}
	first := len(line) - len(strings.TrimLeft(line, " "))
	if space := strings.IndexByte(line[first:], ' '); space >= 0 && first+space < slash {
		space += first
		if line[space-1] != '-' && line[space+1] != '-' {
			output = output[:space+1] + sheet.words.And + " " + output[space+1:]
		}
	}
	for strings.Contains(output, sheet.andDash) || strings.Contains(output, sheet.dashAnd) {
		output = strings.ReplaceAll(output, sheet.andDash, " -")
		output = strings.ReplaceAll(output, sheet.dashAnd, "- ")
	}
	return output
}

// denominator is the word of a denominator of one or two characters.
func (sheet *sheet) denominator(token string) string {
	words := sheet.words
	if len(token) == 1 {
		return words.Denominators[token[0]-'1']
	}
	tens, units := token[0], token[1]
	switch {
	case tens == '1' && digit(units):
		return words.Teens[units-'0']
	case tens == '1':
		return words.OtherUnits[0]
	case units == '0':
		return words.Tens[tens-'2']
	case digit(units):
		return string(tens) + "0 " + words.Ordinals[units-'1']
	}
	return string(tens) + "0 " + words.OtherUnits[1]
}

// speaksMinus is whether a line's first dash is a minus sign (section 2.3): the first character but spaces, or
// between characters below A.
func speaksMinus(line string) bool {
	index := strings.IndexByte(line, '-')
	if strings.Trim(line[:index], " ") == "" {
		return true
	}
	return index+1 < len(line) && low(line[index-1]) && low(line[index+1])
}

// minusLine writes a line's every dash as "minus", and its runs of spaces after the leading ones as one.
func (sheet *sheet) minusLine(line string) string {
	leading := len(line) - len(strings.TrimLeft(line, " "))
	output := []byte(line[:leading])
	for index := leading; index < len(line); index++ {
		switch character := line[index]; character {
		case '-':
			if len(output) > 0 && output[len(output)-1] != ' ' {
				output = append(output, ' ')
			}
			output = append(output, sheet.minus...)
		case ' ':
			if !afterSpace(output, leading) {
				output = append(output, ' ')
			}
		default:
			output = append(output, character)
		}
	}
	return string(output)
}

// exponentDash is whether a line's first dash is an exponent's: after an E after a digit or a dot, and not last.
func exponentDash(line string) bool {
	index := strings.IndexByte(line, '-')
	return index > 1 && line[index-1] == 'E' && (digit(line[index-2]) || line[index-2] == '.') && index+1 < len(line)
}

// brackets writes a line's opening brackets as "minus ", drops its closing ones and writes its runs of spaces
// after the leading ones as one, if it has both (section 2.4).
func (sheet *sheet) brackets(line string) string {
	if strings.IndexByte(line, '(') < 0 || strings.IndexByte(line, ')') < 0 {
		return line
	}
	leading := len(line) - len(strings.TrimLeft(line, " "))
	output := []byte(line[:leading])
	for index := leading; index < len(line); index++ {
		switch character := line[index]; {
		case character == '(':
			output = append(output, sheet.minus...)
		case character == ')':
		case character != ' ' || !afterSpace(output, leading):
			output = append(output, character)
		}
	}
	return string(output)
}

// exponents writes a line's exponents (section 2.5), if its first E starts E+ or "E minus ".
func (sheet *sheet) exponents(line string) string {
	index := strings.IndexByte(line, 'E')
	if index < 0 || !strings.HasPrefix(line[index:], "E+") && !strings.HasPrefix(line[index:], sheet.spokenExponent) {
		return line
	}
	line = strings.ReplaceAll(line, "E+", sheet.exponent)
	return strings.ReplaceAll(line, sheet.spokenExponent, sheet.negativeExponent)
}

// afterSpace is whether a space written next would follow one written after the leading ones.
func afterSpace(output []byte, leading int) bool {
	return len(output) > leading && output[len(output)-1] == ' '
}

// number reads a number as the original does (section 2): spaces, a sign and decimal digits, in 16 bits.
func number(text string) uint16 {
	index := 0
	for index < len(text) && strings.IndexByte(numberSpaces, text[index]) >= 0 {
		index++
	}
	negative := false
	if index < len(text) && (text[index] == '+' || text[index] == '-') {
		negative = text[index] == '-'
		index++
	}
	var value uint16
	for ; index < len(text) && digit(text[index]); index++ {
		value = value*10 + uint16(text[index]-'0')
	}
	if negative {
		value = -value
	}
	return value
}

func digit(character byte) bool {
	return '0' <= character && character <= '9'
}

func letter(character byte) bool {
	return 'A' <= character && character <= 'Z' || 'a' <= character && character <= 'z'
}

// low is whether a character is below A as a signed byte: below A, or 128 and up.
func low(character byte) bool {
	return character < 'A' || character >= 0x80
}
