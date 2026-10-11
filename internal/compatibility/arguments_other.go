//go:build !windows

package compatibility

import (
	"os"
	"unicode/utf16"
)

// Arguments are the command line's arguments as given.
func Arguments() []Argument {
	arguments := make([]Argument, len(os.Args))
	for index, value := range os.Args {
		arguments[index] = Argument{ANSI: []byte(value), Text: utf16.Encode([]rune(value))}
	}
	return arguments
}
