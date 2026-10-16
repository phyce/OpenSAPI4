// sapi4out speaks a text into a WAV file. It takes the arguments of the sapi4out tool of TETYYS/SAPI4 and
// gives the same output, so programs that call that tool can call this one:
//
//	sapi4out <voice> <pitch> <speed> <text>
//
// It writes the WAV file in the current folder, named with 16 random letters and no extension, and prints its
// name. The voice is a mode name, case included, of a Microsoft voice or a TruVoice one; any other name gives
// Mary. The text may have tags.
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/phyce/OpenSAPI4/internal/compatibility"
	"github.com/phyce/OpenSAPI4/internal/voicedata"
	"github.com/phyce/OpenSAPI4/internal/wave"
)

func main() {
	os.Exit(run())
}

func run() int {
	arguments := compatibility.Arguments()
	if len(arguments) < 5 {
		fmt.Fprintln(os.Stderr, "usage: sapi4out <voice> <pitch> <speed> <text>")
		return 2
	}
	voices, err := voicedata.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "sapi4out:", err)
		return 1
	}
	voice, err := compatibility.Open(voices, arguments[1].ANSI)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sapi4out:", err)
		return 1
	}
	name, file, err := createFile()
	defer fmt.Print(name + "\r\n")
	if err != nil {
		fmt.Fprintln(os.Stderr, "sapi4out:", err)
		return 1
	}
	defer file.Close()
	writer := wave.NewWriter(file, voice.Format())
	err = voice.Speak(uint16(compatibility.Atoi(arguments[2].ANSI)), uint32(compatibility.Atoi(arguments[3].ANSI)),
		arguments[4].Text, writer)
	if err == nil {
		err = writer.Finish()
	}
	if err == nil {
		err = file.Close()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "sapi4out:", err)
		return 1
	}
	return 0
}

// createFile makes a new file in the current folder with a random name.
func createFile() (string, *os.File, error) {
	for {
		name := compatibility.FileName()
		file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		return name, file, err
	}
}
