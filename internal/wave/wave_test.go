package wave

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.wav")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := NewWriter(file, Format{SampleRate: 22050, Bits: 16})
	for _, piece := range [][]byte{{1, 2, 3, 4}, {5, 6}} {
		if taken, err := writer.Audio(piece); err != nil || taken != len(piece) {
			t.Fatalf("Audio took %d, %v", taken, err)
		}
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	file.Close()
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	expected, _ := hex.DecodeString("52494646" + "2c000000" + "57415645666d7420" + "12000000" + "0100" + "0100" +
		"22560000" + "44ac0000" + "0200" + "1000" + "0000" + "64617461" + "06000000" + "010203040506")
	if !bytes.Equal(written, expected) {
		t.Errorf("wrote %x, expected %x", written, expected)
	}
}

func TestWriterWithoutAudio(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.wav")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := NewWriter(file, Format{SampleRate: 8000, Bits: 16})
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	file.Close()
	if written, err := os.ReadFile(path); err != nil || len(written) != 0 {
		t.Errorf("wrote %x, %v; expected nothing", written, err)
	}
}

func TestWriterStarted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.wav")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := NewWriter(file, Format{SampleRate: 8000, Bits: 8})
	if err := writer.Start(); err != nil {
		t.Fatal(err)
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	file.Close()
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if expected := Header(Format{SampleRate: 8000, Bits: 8}, 0); !bytes.Equal(written, expected) {
		t.Errorf("wrote %x, expected %x", written, expected)
	}
}
