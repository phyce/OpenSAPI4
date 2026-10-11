// sapi4out speaks a text into a WAV file. It takes the arguments of the sapi4out tool of TETYYS/SAPI4 and
// gives the same output, so programs that call that tool can call this one:
//
//	sapi4out <voice> <pitch> <speed> <text>
//
// It writes the WAV file in the current folder, named with 16 random letters and no extension, and prints its
// name. The voice is a mode name, case included; any other name gives Mary. The text may have tags.
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/phyce/OpenSAPI4/internal/compatibility"
	"github.com/phyce/OpenSAPI4/internal/voicedata"
	"github.com/phyce/OpenSAPI4/internal/wave"
	"github.com/phyce/OpenSAPI4/whistler/text"
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
	data, err := voicedata.LoadMicrosoft()
	if err != nil {
		fmt.Fprintln(os.Stderr, "sapi4out:", err)
		return 1
	}
	voice, err := compatibility.Open(data, arguments[1].ANSI)
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
	engine := voice.Engine
	engine.SetPitch(uint16(compatibility.Atoi(arguments[2].ANSI)))
	engine.SetSpeed(uint32(compatibility.Atoi(arguments[3].ANSI)))
	writer := wave.NewWriter(file, engine.Format())
	err = engine.Speak(arguments[4].Text, true, text.ANSIText, writer)
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
