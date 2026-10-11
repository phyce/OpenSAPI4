package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	opensapi4 "github.com/phyce/OpenSAPI4"
	"github.com/phyce/OpenSAPI4/internal/wave"
	"github.com/phyce/OpenSAPI4/whistler"
)

// speak speaks a text, from the arguments, the request's JSON or standard input, into a WAV file or standard
// output.
func speak(arguments []string) int {
	flags, folder := newFlags("speak", "[flags] [text]")
	voice := flags.String("voice", opensapi4.DefaultVoice, "the voice; opensapi4 voices lists them")
	pitch := flags.Int("pitch", 0, "the pitch in Hz, in the voice's range; 0 is the lowest (default the voice's own)")
	speed := flags.Int("speed", 0, "the speed in words per minute, in the voice's range; 0 is the slowest (default the voice's own)")
	volume := flags.Int("volume", 0xFFFF, "the volume, 0 to 65535")
	rate := flags.Int("rate", 0, "the sample rate in Hz: 22050 Hz voices also give 8000 and 16000, 8000 Hz voices 11025 (default the voice's own)")
	bits := flags.Int("bits", 16, "bits a sample, 8 or 16")
	untagged := flags.Bool("untagged", false, `read \ as text, not as the start of a tag such as \Pau=500\`)
	ipa := flags.Bool("ipa", false, "the text is IPA")
	inject := flags.String("inject", "", `tags applied before the text, such as "\Spd=200\ \Pit=150\"`)
	output := flags.String("output", "", "the WAV file to write, or - for standard output (default: standard output, when it isn't a terminal)")
	pcm := flags.Bool("pcm", false, "write raw PCM, without the WAV header")
	requestJSON := flags.String("json", "", `the request as JSON, as the stream takes it: inline ('{"voice": "Sam", "text": "Hello"}'), a file, or - for standard input; flags given with it override its fields`)
	others, code, proceed := parseFlags(flags, arguments)
	if !proceed {
		return code
	}
	var speechRequest opensapi4.Request
	if *requestJSON != "" {
		var err error
		if speechRequest, err = readRequest(*requestJSON); err != nil {
			return fail(err)
		}
	}
	flags.Visit(func(set *flag.Flag) {
		switch set.Name {
		case "voice":
			speechRequest.Voice = *voice
		case "pitch":
			speechRequest.Pitch = pitch
		case "speed":
			speechRequest.Speed = speed
		case "volume":
			speechRequest.Volume = volume
		case "rate":
			speechRequest.Rate = *rate
		case "bits":
			speechRequest.Bits = *bits
		case "untagged":
			tagged := !*untagged
			speechRequest.Tagged = &tagged
		case "ipa":
			speechRequest.IPA = *ipa
		case "inject":
			speechRequest.Inject = *inject
		}
	})
	if len(others) > 0 {
		speechRequest.Text, speechRequest.Texts = strings.Join(others, " "), nil
	}
	if speechRequest.Text == "" && len(speechRequest.Texts) == 0 && *requestJSON != "-" {
		if isTerminal(os.Stdin) {
			fmt.Fprintln(os.Stderr, "opensapi4: no text: give it as arguments, or on standard input")
			return 2
		}
		text, err := readAll(os.Stdin)
		if err != nil {
			return fail(err)
		}
		speechRequest.Text = text
	}
	toStandardOutput := *output == "" || *output == "-"
	if toStandardOutput && isTerminal(os.Stdout) {
		fmt.Fprintln(os.Stderr, "opensapi4: give the file to write with -output, or redirect the output")
		return 2
	}
	voices, err := openVoices(*folder)
	if err != nil {
		return fail(err)
	}
	speech, err := voices.NewSpeech(speechRequest)
	if err != nil {
		return fail(err)
	}
	if toStandardOutput {
		err = speakToStandardOutput(speech, *pcm)
	} else {
		err = speakToFile(speech, *output, *pcm)
	}
	if err != nil {
		return fail(err)
	}
	return 0
}

func readRequest(argument string) (opensapi4.Request, error) {
	var content []byte
	var err error
	switch trimmed := strings.TrimSpace(argument); {
	case argument == "-":
		content, err = io.ReadAll(os.Stdin)
	case strings.HasPrefix(trimmed, "{"):
		content = []byte(trimmed)
	default:
		content, err = os.ReadFile(argument)
	}
	if err != nil {
		return opensapi4.Request{}, err
	}
	decoded, err := decodeRequest(content)
	if err != nil {
		return opensapi4.Request{}, fmt.Errorf("read the request: %w", err)
	}
	if decoded.ID != nil || decoded.Events || decoded.Visual || decoded.Output != "" || decoded.Format != "" {
		return opensapi4.Request{}, errors.New(`speak takes "id", "events", "visual", "output" and "format" only in a stream; use -output and -pcm`)
	}
	return decoded.Request, nil
}

// pcmSink writes the audio as it comes.
type pcmSink struct {
	output *bufio.Writer
}

func (sink pcmSink) Audio(data []byte) (int, error) {
	return sink.output.Write(data)
}

func (sink pcmSink) Event(whistler.Event) error {
	return nil
}

// bufferSink keeps the audio, for a WAV file that can't be rewritten at the end.
type bufferSink struct {
	data []byte
}

func (sink *bufferSink) Audio(data []byte) (int, error) {
	sink.data = append(sink.data, data...)
	return len(data), nil
}

func (sink *bufferSink) Event(whistler.Event) error {
	return nil
}

func speakToStandardOutput(speech *opensapi4.Speech, pcm bool) error {
	output := bufio.NewWriterSize(os.Stdout, 65536)
	if pcm {
		if err := speech.Speak(pcmSink{output: output}); err != nil {
			return err
		}
		return output.Flush()
	}
	buffer := &bufferSink{}
	if err := speech.Speak(buffer); err != nil {
		return err
	}
	if _, err := output.Write(wave.Header(speech.Format(), uint32(len(buffer.data)))); err != nil {
		return err
	}
	if _, err := output.Write(buffer.data); err != nil {
		return err
	}
	return output.Flush()
}

func speakToFile(speech *opensapi4.Speech, path string, pcm bool) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	if pcm {
		output := bufio.NewWriterSize(file, 65536)
		err = speech.Speak(pcmSink{output: output})
		if err == nil {
			err = output.Flush()
		}
	} else {
		writer := wave.NewWriter(file, speech.Format())
		err = writer.Start()
		if err == nil {
			err = speech.Speak(writer)
		}
		if err == nil {
			err = writer.Finish()
		}
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}
