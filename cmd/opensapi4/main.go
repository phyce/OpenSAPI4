// opensapi4 speaks with the SAPI 4 voices: into WAV files, as a stream of JSON messages over standard input and
// output, or as an HTTP server. Run it without arguments for its commands.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	opensapi4 "github.com/phyce/OpenSAPI4"
	"github.com/phyce/OpenSAPI4/internal/voicedata"
)

var version = "dev"

const usage = `opensapi4 speaks with the Microsoft SAPI 4 voices.

Commands:
  opensapi4 speak [flags] [text]   speak a text into a WAV file
  opensapi4 voices [-json]         list the voices
  opensapi4 stream [-raw]          speak JSON requests from standard input, one a line, as JSON messages
  opensapi4 serve [-address a]     speak over HTTP
  opensapi4 version                print the version

Run "opensapi4 <command> -h" for a command's flags. Every command takes -voices <folder>, the folder of the
voice data; by default the folder in OPENSAPI4_VOICES, or else the voices folder next to the program.
`

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(arguments []string) int {
	if len(arguments) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	command, rest := arguments[0], arguments[1:]
	switch command {
	case "speak":
		return speak(rest)
	case "voices":
		return listVoices(rest)
	case "stream":
		return stream(rest)
	case "serve":
		return serve(rest)
	case "version", "-version", "--version":
		fmt.Println("opensapi4", programVersion())
		return 0
	case "help", "-h", "-help", "--help":
		fmt.Print(usage)
		return 0
	}
	fmt.Fprintf(os.Stderr, "opensapi4: unknown command %q\n\n%s", command, usage)
	return 2
}

func programVersion() string {
	if version != "dev" {
		return version
	}
	if information, found := debug.ReadBuildInfo(); found && information.Main.Version != "" && information.Main.Version != "(devel)" {
		return information.Main.Version
	}
	return version
}

// newFlags makes a command's flag set, with the -voices flag every command takes.
func newFlags(name, arguments string) (*flag.FlagSet, *string) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "usage: opensapi4 %s %s\n\n", name, arguments)
		flags.PrintDefaults()
	}
	folder := flags.String("voices", "", "the folder of the voice data (default: $OPENSAPI4_VOICES, or the voices folder next to the program)")
	return flags, folder
}

// parseFlags parses a command's flags, before or after its other arguments (after --, everything is an
// argument), and gives the other arguments, or the exit code when the command should stop.
func parseFlags(flags *flag.FlagSet, arguments []string) ([]string, int, bool) {
	var others []string
	for {
		err := flags.Parse(arguments)
		if errors.Is(err, flag.ErrHelp) {
			return nil, 0, false
		}
		if err != nil {
			return nil, 2, false
		}
		rest := flags.Args()
		if consumed := len(arguments) - len(rest); consumed > 0 && arguments[consumed-1] == "--" || len(rest) == 0 {
			return append(others, rest...), 0, true
		}
		others = append(others, rest[0])
		arguments = rest[1:]
	}
}

func openVoices(folder string) (*opensapi4.Voices, error) {
	if folder == "" {
		var err error
		if folder, err = voicedata.Folder(); err != nil {
			return nil, err
		}
	}
	voices, err := opensapi4.Open(folder)
	if err != nil {
		return nil, fmt.Errorf("%w (keep the voices folder next to the program, or give its path with -voices or %s)", err,
			voicedata.EnvironmentVariable)
	}
	return voices, nil
}

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "opensapi4:", err)
	return 1
}

// isTerminal reports whether a file is a terminal (a character device), which takes no audio.
func isTerminal(file *os.File) bool {
	information, err := file.Stat()
	return err == nil && information.Mode()&os.ModeCharDevice != 0
}

func readAll(reader io.Reader) (string, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(trimByteOrderMark(content)), nil
}

func trimByteOrderMark(content []byte) []byte {
	if len(content) >= 3 && content[0] == 0xEF && content[1] == 0xBB && content[2] == 0xBF {
		return content[3:]
	}
	return content
}
