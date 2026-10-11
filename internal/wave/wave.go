// Package wave writes PCM as WAV files the way SAPI 4's audio file destination does: a RIFF file with an
// 18-byte fmt chunk (WAVEFORMATEX) and one data chunk.
package wave

import (
	"encoding/binary"
	"io"

	"github.com/phyce/OpenSAPI4/whistler"
)

const HeaderSize = 46

// Header is the header of a WAV file with dataSize bytes of PCM in format.
func Header(format whistler.Format, dataSize uint32) []byte {
	blockAlign := uint16(format.Bits / 8)
	header := make([]byte, 0, HeaderSize)
	header = append(header, "RIFF"...)
	header = binary.LittleEndian.AppendUint32(header, HeaderSize-8+dataSize)
	header = append(header, "WAVEfmt "...)
	header = binary.LittleEndian.AppendUint32(header, 18)
	header = binary.LittleEndian.AppendUint16(header, 1)
	header = binary.LittleEndian.AppendUint16(header, 1)
	header = binary.LittleEndian.AppendUint32(header, uint32(format.SampleRate))
	header = binary.LittleEndian.AppendUint32(header, uint32(format.SampleRate)*uint32(blockAlign))
	header = binary.LittleEndian.AppendUint16(header, blockAlign)
	header = binary.LittleEndian.AppendUint16(header, uint16(format.Bits))
	header = binary.LittleEndian.AppendUint16(header, 0)
	header = append(header, "data"...)
	return binary.LittleEndian.AppendUint32(header, dataSize)
}

// Writer writes an engine's audio as a WAV file. It is a whistler.Sink. As SAPI 4's audio file destination,
// it writes the header with the first audio, so a text with no audio leaves the file empty; Finish writes the
// sizes into the header.
type Writer struct {
	output  io.WriteSeeker
	format  whistler.Format
	started bool
	size    uint32
}

func NewWriter(output io.WriteSeeker, format whistler.Format) *Writer {
	return &Writer{output: output, format: format}
}

func (writer *Writer) Audio(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	if !writer.started {
		if _, err := writer.output.Write(Header(writer.format, 0)); err != nil {
			return 0, err
		}
		writer.started = true
	}
	written, err := writer.output.Write(data)
	writer.size += uint32(written)
	return written, err
}

func (writer *Writer) Event(whistler.Event) error {
	return nil
}

func (writer *Writer) Finish() error {
	if !writer.started {
		return nil
	}
	if _, err := writer.output.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, err := writer.output.Write(Header(writer.format, writer.size))
	return err
}
