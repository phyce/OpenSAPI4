package compatibility

// parseCommandLine splits a command line into main's argv as the Microsoft C runtime does. isLeadByte tells
// the code page's lead bytes, each copied with the byte after it.
//
// The program name ends at a space or tab outside quotes, and its quotes are dropped. Each argument after it
// ends at a space or tab outside quotes. Backslashes are literal except before a quote: 2n of them and a quote
// give n backslashes and start or end quoting, 2n+1 of them and a quote give n backslashes and a literal
// quote. Inside quotes, two quotes give one literal quote and quoting goes on.
func parseCommandLine(line []byte, isLeadByte func(byte) bool) [][]byte {
	at := func(index int) byte {
		if index < len(line) {
			return line[index]
		}
		return 0
	}
	position := 0
	var program []byte
	insideQuotes := false
	for {
		character := at(position)
		position++
		if character == '"' {
			insideQuotes = !insideQuotes
			continue
		}
		if character == 0 || !insideQuotes && (character == ' ' || character == '\t') {
			break
		}
		program = append(program, character)
		if isLeadByte(character) {
			program = append(program, at(position))
			position++
		}
	}
	arguments := [][]byte{program}
	insideQuotes = false
	for {
		for at(position) == ' ' || at(position) == '\t' {
			position++
		}
		if at(position) == 0 {
			return arguments
		}
		argument := []byte{}
		for {
			copyCharacter := true
			backslashes := 0
			for at(position) == '\\' {
				position++
				backslashes++
			}
			if at(position) == '"' {
				if backslashes%2 == 0 {
					if insideQuotes && at(position+1) == '"' {
						position++
					} else {
						copyCharacter = false
						insideQuotes = !insideQuotes
					}
				}
				backslashes /= 2
			}
			for ; backslashes > 0; backslashes-- {
				argument = append(argument, '\\')
			}
			character := at(position)
			if character == 0 || !insideQuotes && (character == ' ' || character == '\t') {
				break
			}
			if copyCharacter {
				if isLeadByte(character) {
					argument = append(argument, character)
					position++
				}
				argument = append(argument, at(position))
			}
			position++
		}
		arguments = append(arguments, argument)
	}
}
