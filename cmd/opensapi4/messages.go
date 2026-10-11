package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"

	opensapi4 "github.com/phyce/OpenSAPI4"
	"github.com/phyce/OpenSAPI4/internal/wave"
	"github.com/phyce/OpenSAPI4/whistler"
	"github.com/phyce/OpenSAPI4/whistler/datapack"
)

// request is a request as the stream and the HTTP server take it: the speech, with an ID to echo, the events to
// send, and in a stream, a WAV file to write the audio to instead.
type request struct {
	opensapi4.Request
	ID     json.RawMessage `json:"id,omitempty"`
	Events bool            `json:"events,omitempty"`
	Visual bool            `json:"visual,omitempty"`
	Output string          `json:"output,omitempty"`
	Format string          `json:"format,omitempty"`
}

func decodeRequest(content []byte) (request, error) {
	decoder := json.NewDecoder(bytes.NewReader(trimByteOrderMark(content)))
	decoder.DisallowUnknownFields()
	var decoded request
	if err := decoder.Decode(&decoded); err != nil {
		return request{}, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return request{}, errors.New("more than one JSON value")
	}
	return decoded, nil
}

type formatMessage struct {
	ID         json.RawMessage `json:"id,omitempty"`
	Type       string          `json:"type"`
	SampleRate int             `json:"sampleRate"`
	Bits       int             `json:"bits"`
	Channels   int             `json:"channels"`
}

type audioMessage struct {
	ID    json.RawMessage `json:"id,omitempty"`
	Type  string          `json:"type"`
	Data  []byte          `json:"data,omitempty"`
	Bytes int             `json:"bytes,omitempty"`
}

type eventMessage struct {
	ID       json.RawMessage `json:"id,omitempty"`
	Type     string          `json:"type"`
	Event    string          `json:"event"`
	Sample   int64           `json:"sample"`
	Text     int             `json:"text"`
	Offset   *uint32         `json:"offset,omitempty"`
	Bookmark *uint32         `json:"bookmark,omitempty"`
	IPA      string          `json:"ipa,omitempty"`
	Phoneme  *int            `json:"phoneme,omitempty"`
	Mouth    *mouth          `json:"mouth,omitempty"`
}

type mouth struct {
	Height            int `json:"height"`
	Width             int `json:"width"`
	Upturn            int `json:"upturn"`
	JawOpen           int `json:"jawOpen"`
	UpperTeethVisible int `json:"upperTeethVisible"`
	LowerTeethVisible int `json:"lowerTeethVisible"`
	TonguePosition    int `json:"tonguePosition"`
	LipTension        int `json:"lipTension"`
}

type doneMessage struct {
	ID      json.RawMessage `json:"id,omitempty"`
	Type    string          `json:"type"`
	Samples int64           `json:"samples"`
	Output  string          `json:"output,omitempty"`
}

type errorMessage struct {
	ID    json.RawMessage `json:"id,omitempty"`
	Type  string          `json:"type"`
	Error string          `json:"error"`
}

var eventNames = map[whistler.EventKind]string{
	whistler.TextStarted:  "textStarted",
	whistler.TextDone:     "textDone",
	whistler.WordPosition: "word",
	whistler.Bookmark:     "bookmark",
	whistler.Visual:       "visual",
}

const audioMessageSize = 32768

// messages writes a speech as JSON messages, one a line: its format, its audio, the events asked for, and done
// or an error. The audio is base64 in the message, or with raw, the message's bytes right after its line. When
// audio is set, the audio goes to that WAV file instead.
type messages struct {
	output     *bufio.Writer
	flush      func()
	raw        bool
	id         json.RawMessage
	events     bool
	visual     bool
	audio      *wave.Writer
	sampleSize int
	pending    []byte
	samples    int64
	// stop, when set, is checked before every piece of audio: an error stops the speech.
	stop func() error
}

func (writer *messages) write(message any) error {
	line, err := json.Marshal(message)
	if err != nil {
		return err
	}
	if _, err := writer.output.Write(append(line, '\n')); err != nil {
		return err
	}
	return nil
}

func (writer *messages) sendFormat(format whistler.Format) error {
	writer.sampleSize = format.Bits / 8
	return writer.write(formatMessage{ID: writer.id, Type: "format", SampleRate: format.SampleRate, Bits: format.Bits, Channels: 1})
}

func (writer *messages) Audio(data []byte) (int, error) {
	if writer.stop != nil {
		if err := writer.stop(); err != nil {
			return 0, err
		}
	}
	writer.samples += int64(len(data) / writer.sampleSize)
	if writer.audio != nil {
		return writer.audio.Audio(data)
	}
	writer.pending = append(writer.pending, data...)
	if len(writer.pending) >= audioMessageSize {
		if err := writer.sendAudio(); err != nil {
			return 0, err
		}
	}
	return len(data), nil
}

func (writer *messages) sendAudio() error {
	if len(writer.pending) == 0 {
		return nil
	}
	pending := writer.pending
	writer.pending = writer.pending[:0]
	if !writer.raw {
		return writer.writeAndFlush(audioMessage{ID: writer.id, Type: "audio", Data: pending})
	}
	if err := writer.write(audioMessage{ID: writer.id, Type: "audio", Bytes: len(pending)}); err != nil {
		return err
	}
	if _, err := writer.output.Write(pending); err != nil {
		return err
	}
	return writer.flushOutput()
}

func (writer *messages) writeAndFlush(message any) error {
	if err := writer.write(message); err != nil {
		return err
	}
	return writer.flushOutput()
}

func (writer *messages) flushOutput() error {
	if err := writer.output.Flush(); err != nil {
		return err
	}
	if writer.flush != nil {
		writer.flush()
	}
	return nil
}

func (writer *messages) Event(event whistler.Event) error {
	if event.Kind == whistler.Visual && !writer.visual || event.Kind != whistler.Visual && !writer.events {
		return nil
	}
	if err := writer.sendAudio(); err != nil {
		return err
	}
	message := eventMessage{ID: writer.id, Type: "event", Event: eventNames[event.Kind], Sample: event.Sample, Text: event.Text}
	switch event.Kind {
	case whistler.WordPosition:
		offset := event.Value / 2
		message.Offset = &offset
	case whistler.Bookmark:
		message.Bookmark = &event.Value
	case whistler.Visual:
		phoneme := int(event.Visual.Engine)
		message.IPA, message.Phoneme, message.Mouth = ipa(event.Visual), &phoneme, mouthOf(event.Visual.Mouth)
	}
	return writer.write(message)
}

func ipa(visual datapack.Visual) string {
	if visual.IPA == 0 {
		return ""
	}
	return string(rune(visual.IPA))
}

func mouthOf(shape datapack.Mouth) *mouth {
	return &mouth{Height: int(shape.Height), Width: int(shape.Width), Upturn: int(shape.Upturn), JawOpen: int(shape.JawOpen),
		UpperTeethVisible: int(shape.UpperTeethVisible), LowerTeethVisible: int(shape.LowerTeethVisible),
		TonguePosition: int(shape.TonguePosition), LipTension: int(shape.LipTension)}
}

// finish sends the rest of the audio and done, or the error that stopped the speech.
func (writer *messages) finish(err error, output string) error {
	if err == nil {
		err = writer.sendAudio()
	}
	if err != nil {
		return writer.writeAndFlush(errorMessage{ID: writer.id, Type: "error", Error: err.Error()})
	}
	return writer.writeAndFlush(doneMessage{ID: writer.id, Type: "done", Samples: writer.samples, Output: output})
}
