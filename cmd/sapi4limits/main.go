// sapi4limits lists the voices, or prints one voice's pitch and speed ranges. It takes the arguments of the
// sapi4limits tool that programs call alongside sapi4out, and gives the same output:
//
//	sapi4limits           the mode names, one a line
//	sapi4limits <voice>   the mode the name selects (sapi4out's rule), then "default minimum maximum" for
//	                      the pitch and then the speed
//
// Both start with two lines: "SAPI4 limits", then "Enumerating voices" or "Voice info for <voice>".
package main

import (
	"fmt"
	"os"

	"github.com/phyce/OpenSAPI4/internal/compatibility"
	"github.com/phyce/OpenSAPI4/internal/voicedata"
)

func main() {
	os.Exit(run())
}

func run() int {
	arguments := compatibility.Arguments()
	output := []byte("SAPI4 limits\r\n")
	defer func() {
		os.Stdout.Write(output)
	}()
	if len(arguments) == 2 {
		output = append(append(append(output, "Voice info for "...), arguments[1].ANSI...), "\r\n"...)
		voices, err := voicedata.Load()
		if err != nil {
			fmt.Fprintln(os.Stderr, "sapi4limits:", err)
			return 1
		}
		voice, err := compatibility.Open(voices, arguments[1].ANSI)
		if err != nil {
			fmt.Fprintln(os.Stderr, "sapi4limits:", err)
			return 1
		}
		output = fmt.Appendf(output, "%s\r\n%d %d %d\r\n%d %d %d\r\n", voice.Name,
			voice.Pitch.Default, voice.Pitch.Minimum, voice.Pitch.Maximum,
			voice.Speed.Default, voice.Speed.Minimum, voice.Speed.Maximum)
		return 0
	}
	output = append(output, "Enumerating voices\r\n"...)
	voices, err := voicedata.Load()
	var names []string
	if err == nil {
		names, err = compatibility.ModeNames(voices)
	}
	if err != nil {
		output = append(output, "(null)"...)
		fmt.Fprintln(os.Stderr, "sapi4limits:", err)
		return 1
	}
	for _, name := range names {
		output = append(append(output, name...), "\r\n"...)
	}
	return 0
}
