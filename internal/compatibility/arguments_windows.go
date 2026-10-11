package compatibility

import (
	"syscall"
	"unsafe"
)

var (
	kernel32            = syscall.NewLazyDLL("kernel32.dll")
	getCommandLine      = kernel32.NewProc("GetCommandLineA")
	stringLength        = kernel32.NewProc("lstrlenA")
	moveMemory          = kernel32.NewProc("RtlMoveMemory")
	isLeadByte          = kernel32.NewProc("IsDBCSLeadByte")
	multiByteToWideChar = kernel32.NewProc("MultiByteToWideChar")
)

// Arguments are the command line's arguments as a program built with the Microsoft C runtime gets them in
// main: Windows' ANSI copy of the command line, split by the runtime's rules.
func Arguments() []Argument {
	parsed := parseCommandLine(commandLine(), func(character byte) bool {
		lead, _, _ := isLeadByte.Call(uintptr(character))
		return lead != 0
	})
	arguments := make([]Argument, len(parsed))
	for index, value := range parsed {
		arguments[index] = Argument{ANSI: value, Text: toUTF16(value)}
	}
	return arguments
}

func commandLine() []byte {
	pointer, _, _ := getCommandLine.Call()
	length, _, _ := stringLength.Call(pointer)
	line := make([]byte, length)
	if length > 0 {
		moveMemory.Call(uintptr(unsafe.Pointer(&line[0])), pointer, length)
	}
	return line
}

// toUTF16 converts with the ANSI code page.
func toUTF16(value []byte) []uint16 {
	if len(value) == 0 {
		return nil
	}
	converted := make([]uint16, len(value))
	count, _, _ := multiByteToWideChar.Call(0, 0, uintptr(unsafe.Pointer(&value[0])), uintptr(len(value)),
		uintptr(unsafe.Pointer(&converted[0])), uintptr(len(converted)))
	return converted[:count]
}
