package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	opensapi4 "github.com/phyce/OpenSAPI4"
	"github.com/phyce/OpenSAPI4/internal/wave"
)

// stream speaks the requests on standard input, one JSON object a line, in order, and writes each as messages on
// standard output. A request that fails gives an error message, and the stream goes on.
func stream(arguments []string) int {
	flags, folder := newFlags("stream", "[-raw]")
	raw := flags.Bool("raw", false, "send audio as raw bytes after each audio message's line, instead of base64 in it")
	others, code, proceed := parseFlags(flags, arguments)
	if !proceed {
		return code
	}
	if len(others) != 0 {
		flags.Usage()
		return 2
	}
	voices, err := openVoices(*folder)
	if err != nil {
		return fail(err)
	}
	input := bufio.NewReader(os.Stdin)
	output := bufio.NewWriter(os.Stdout)
	for {
		line, readErr := input.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			if err := streamRequest(voices, line, output, *raw); err != nil {
				return fail(err)
			}
		}
		if readErr == io.EOF {
			return 0
		}
		if readErr != nil {
			return fail(readErr)
		}
	}
}

// streamRequest speaks one request, and returns an error only when the output fails.
func streamRequest(voices *opensapi4.Voices, line []byte, output *bufio.Writer, raw bool) error {
	writer := &messages{output: output, raw: raw}
	decoded, err := decodeRequest(line)
	if err != nil {
		var identified struct {
			ID json.RawMessage `json:"id"`
		}
		json.Unmarshal(trimByteOrderMark(line), &identified)
		writer.id = identified.ID
		return writer.finish(fmt.Errorf("read the request: %w", err), "")
	}
	writer.id, writer.events, writer.visual = decoded.ID, decoded.Events, decoded.Visual
	if decoded.Format != "" {
		return writer.finish(errors.New(`a stream takes no "format": it sends PCM, or writes a WAV file named by "output"`), "")
	}
	speech, err := voices.NewSpeech(decoded.Request)
	if err != nil {
		return writer.finish(err, "")
	}
	if err := writer.sendFormat(speech.Format()); err != nil {
		return err
	}
	if decoded.Output == "" {
		return writer.finish(speech.Speak(writer), "")
	}
	file, err := os.Create(decoded.Output)
	if err != nil {
		return writer.finish(err, "")
	}
	writer.audio = wave.NewWriter(file, wave.Format(speech.Format()))
	err = writer.audio.Start()
	if err == nil {
		err = speech.Speak(writer)
	}
	if err == nil {
		err = writer.audio.Finish()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return writer.finish(err, decoded.Output)
}
