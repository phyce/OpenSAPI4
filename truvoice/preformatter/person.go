package preformatter

import "strings"

const (
	addressDelimiters = "@!%/:"
	nameSeparators    = " ,;"
)

// person is the name in a sender's value (section 3.7), and whether it was taken out of an address: a bracketed
// name after a word with `@`, the part before a bracket, or the part before a delimiter.
func person(value string) (name string, fromAddress bool) {
	value = strings.Trim(strings.ReplaceAll(value, "\t", " "), " ")
	fromAddress = true
	found := false
	opening := separatedAt(value, '(')
	if opening > 0 && value[opening-1] == ' ' {
		before := strings.TrimRight(value[:opening], " ")
		if strings.Contains(before[strings.LastIndexByte(before, ' ')+1:], "@") {
			rest := value[opening:]
			for index := 1; index < len(rest); index++ {
				if rest[index] == ')' && (index+1 == len(rest) || strings.IndexByte(nameSeparators, rest[index+1]) >= 0) {
					name = strings.Trim(rest[1:index], " ")
					found = name != ""
					break
				}
			}
		}
	}
	if !found {
		angle := separatedAt(value, '<')
		switch {
		case opening >= 0:
			name = beforeDelimiter(value[:opening])
		case angle >= 0:
			name = strings.Trim(value[:angle], " ")
			name = strings.TrimPrefix(name, "\"")
			name = strings.TrimSuffix(name, "\"")
		default:
			name = beforeDelimiter(value)
			fromAddress = name != value
		}
	}
	return title(collapse(strings.Trim(name, " "))), fromAddress
}

// people is the names in an Author: or names value (section 3.7): each name of its list as item takes it, the
// commas and semicolons between them kept, and whether the last was taken out of an address.
func people(value string) (string, bool) {
	value = strings.ReplaceAll(value, "\t", " ")
	var names strings.Builder
	fromAddress := false
	for {
		stop := strings.IndexAny(value, ",;")
		if stop < 0 {
			var name string
			name, fromAddress = item(value)
			names.WriteString(name)
			break
		}
		name, _ := item(value[:stop])
		names.WriteString(name + value[stop:stop+1])
		value = value[stop+1:]
	}
	return title(collapse(strings.Trim(names.String(), " "))), fromAddress
}

// item is one name of an Author: or names list, its spaces kept, and whether it was taken out of an address: a
// bracketed name after a spaced word with `@`, the part before a bracket or an angle bracket after a space, or the
// part before a delimiter, with a space after it if words follow the delimiter.
func item(value string) (string, bool) {
	leading := len(value) - len(strings.TrimLeft(value, " "))
	stripped := strings.Trim(value, " ")
	opening := -1
	for index := 1; index < len(stripped); index++ {
		if stripped[index] == '(' && stripped[index-1] == ' ' {
			opening = index
			break
		}
	}
	if opening > 0 {
		before := strings.TrimRight(stripped[:opening], " ")
		if strings.Contains(before[strings.LastIndexByte(before, ' ')+1:], "@") {
			rest := stripped[opening:]
			for index := 1; index < len(rest); index++ {
				if rest[index] == ')' && (index+1 == len(rest) || rest[index+1] == ' ') {
					if name := strings.Trim(rest[1:index], " "); name != "" {
						return strings.Repeat(" ", leading) + name, true
					}
					break
				}
			}
		}
		return beforeDelimiter(value[:leading+opening]), true
	}
	if angle := separatedAt(stripped, '<'); angle >= 0 {
		name := value[:leading+angle]
		quoted := strings.TrimSuffix(strings.TrimPrefix(strings.Trim(name, " "), "\""), "\"")
		return name[:len(name)-len(strings.TrimLeft(name, " "))] + quoted + name[len(strings.TrimRight(name, " ")):], true
	}
	name := beforeDelimiter(value)
	if name == value {
		return name, false
	}
	if strings.Contains(value[len(name):], " ") {
		name += " "
	}
	return name, true
}

// continued is a line that goes on with an Author: or names value (section 3.6): its first word as it is, the rest
// read as names, and whether the last was taken out of an address.
func continued(piece string) (string, bool) {
	if blankHeaderLine(strings.Trim(piece, " \t")) {
		return "", false
	}
	line := strings.TrimLeft(unquoted(piece), " \t")
	for index := 1; index < len(line); index++ {
		switch line[index] {
		case ' ', '\t', ':':
			names, fromAddress := people(line[index+1:])
			text := line[:index]
			if line[index] == ':' {
				text += ":"
			}
			if names != "" {
				text += " " + names
			}
			return text, fromAddress
		case ',', ';':
			names, fromAddress := people(line[index:])
			return line[:index] + names, fromAddress
		}
	}
	return line, false
}

// separatedAt is where character first comes at the value's start or after a space, comma or semicolon, or -1.
func separatedAt(value string, character byte) int {
	for index := 0; index < len(value); index++ {
		if value[index] == character && (index == 0 || strings.IndexByte(nameSeparators, value[index-1]) >= 0) {
			return index
		}
	}
	return -1
}

// beforeDelimiter is the value up to its first delimiter, or to an `at` after a space, comma or semicolon and
// before one or the value's end.
func beforeDelimiter(value string) string {
	stop := len(value)
	if index := strings.IndexAny(value, addressDelimiters); index >= 0 {
		stop = index
	}
	for index := 1; index+1 < len(value) && index < stop; index++ {
		if value[index:index+2] == "at" && strings.IndexByte(nameSeparators, value[index-1]) >= 0 &&
			(index+2 == len(value) || strings.IndexByte(nameSeparators, value[index+2]) >= 0) {
			stop = index
			break
		}
	}
	return value[:stop]
}

// title capitalizes each word's first letter and lowercases each other capital that doesn't follow a small letter,
// a word's first letter as capitalized. A word starts after a space, comma or semicolon, or after a dash that
// follows another character.
func title(name string) string {
	titled := []byte(name)
	var previous byte
	for index := range titled {
		character := name[index]
		if index == 0 || strings.IndexByte(nameSeparators, previous) >= 0 ||
			previous == '-' && index > 1 && strings.IndexByte(nameSeparators, titled[index-2]) < 0 {
			if small(character) {
				titled[index] = character - 'a' + 'A'
			}
			previous = titled[index]
			continue
		}
		if upper(character) && !small(previous) {
			titled[index] = character - 'A' + 'a'
		}
		previous = character
	}
	return string(titled)
}

func small(character byte) bool {
	return character >= 'a' && character <= 'z'
}
