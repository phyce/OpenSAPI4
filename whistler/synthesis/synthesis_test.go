package synthesis

import (
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/phyce/OpenSAPI4/whistler/internal/conformance"
)

var voiceNames = []string{"female", "female8", "male", "male8", "sam"}

func floatsDigest(values []float32) string {
	digest := sha256.New()
	_ = binary.Write(digest, binary.LittleEndian, values)
	return hex.EncodeToString(digest.Sum(nil))
}

func TestTables(t *testing.T) {
	data, err := os.ReadFile(conformance.Path(t, "golden", "synthesis", "tables.json"))
	if err != nil {
		t.Fatal(err)
	}
	var expected map[string]any
	if err := json.Unmarshal(data, &expected); err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{512, 256} {
		tables := expected[fmt.Sprintf("N%d", size)].(map[string]any)
		if actual := floatsDigest(makeSineTable(size)); actual != tables["sineTable"] {
			t.Errorf("sine table %d: %s", size, actual)
		}
		if actual := floatsDigest(makeFadeWindow(size)); actual != tables["fadeWindow"] {
			t.Errorf("fade window %d: %s", size, actual)
		}
	}
	for _, size := range []int{22050, 8000} {
		noise := makeNoiseTable(size)
		bytes := make([]byte, len(noise))
		for index, value := range noise {
			bytes[index] = byte(value)
		}
		digest := sha256.Sum256(bytes)
		if actual := hex.EncodeToString(digest[:]); actual != expected[fmt.Sprintf("noise%d", size)] {
			t.Errorf("noise table %d: %s", size, actual)
		}
	}
	noise := makeNoiseTable(8)
	if want := []int8{-2, -1, 12, -31, -66, 8, 37, -28}; fmt.Sprint(noise) != fmt.Sprint(want) {
		t.Errorf("noise table starts %v, expected %v", noise, want)
	}
}

type whisper struct{}

func (whisper) Whisper() bool         { return true }
func (whisper) Apply([]float32, bool) {}
func (whisper) EndFrame()             {}

func loadVoices(t *testing.T) map[string]*Voice {
	t.Helper()
	pack := conformance.Pack(t)
	voices := map[string]*Voice{}
	for _, name := range voiceNames {
		voices[name] = NewVoice(conformance.Voice(t, pack, name).Inventory)
	}
	return voices
}

func unitDigest(voice *Voice, unit int, effects Effects) (int, string, error) {
	synthesizer := NewSynthesizer(voice)
	synthesizer.Effects = effects
	decoded, err := synthesizer.decodeUnit(unit)
	if err != nil {
		return 0, "", err
	}
	digest := sha256.New()
	_ = binary.Write(digest, binary.LittleEndian, decoded.periods)
	for _, coefficients := range decoded.coefficients {
		_ = binary.Write(digest, binary.LittleEndian, coefficients)
	}
	_ = binary.Write(digest, binary.LittleEndian, decoded.excitation)
	_ = binary.Write(digest, binary.LittleEndian, uint32(synthesizer.noisePosition))
	return len(decoded.periods), hex.EncodeToString(digest.Sum(nil)), nil
}

func TestUnits(t *testing.T) {
	voices := loadVoices(t)
	for _, name := range voiceNames {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(conformance.Path(t, "golden", "synthesis", "units-"+name+".tsv"))
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			failures := 0
			for _, line := range lines {
				fields := strings.Split(line, "\t")
				unit, _ := strconv.Atoi(fields[0])
				for column, effects := range []Effects{nil, whisper{}} {
					frameCount, digest, err := unitDigest(voices[name], unit, effects)
					if err != nil || strconv.Itoa(frameCount) != fields[1] || digest != fields[2+column] {
						failures++
						if failures <= 5 {
							t.Errorf("unit %d (%v): %d frames, %s, %v", unit, effects, frameCount, digest, err)
						}
					}
				}
			}
			if failures > 0 {
				t.Errorf("%d of %d decodes differ", failures, 2*len(lines))
			}
		})
	}
}

type frameCase struct {
	Voice    string              `json:"voice"`
	Sentence string              `json:"sentence"`
	Speed    *int                `json:"speed"`
	Pitch    *int                `json:"pitch"`
	Frames   [][]json.RawMessage `json:"frames"`
	Counts   []int               `json:"counts"`
	Digest   string              `json:"sha256"`
}

func (item frameCase) frames(t *testing.T) []Frame {
	frames := make([]Frame, len(item.Frames))
	for index, fields := range item.Frames {
		var unit int
		var duration float64
		var points [][3]float64
		if json.Unmarshal(fields[0], &unit) != nil || json.Unmarshal(fields[1], &duration) != nil || json.Unmarshal(fields[2], &points) != nil {
			t.Fatalf("frame %d of %q does not parse", index, item.Sentence)
		}
		frame := Frame{Unit: unit, Duration: float32(duration)}
		for _, point := range points {
			frame.Points = append(frame.Points, Point{Time: float32(point[0]), Pitch: int16(point[1]), Amplitude: float32(point[2])})
		}
		frames[index] = frame
	}
	return frames
}

func (item frameCase) check(t *testing.T, voice *Voice) bool {
	synthesizer := NewSynthesizer(voice)
	digest := sha256.New()
	counts := []int{}
	for _, frame := range item.frames(t) {
		data, err := synthesizer.Synthesize(frame)
		if err != nil {
			t.Errorf("%q: %v", item.Sentence, err)
			return false
		}
		counts = append(counts, len(data)/2)
		digest.Write(data)
	}
	actual := hex.EncodeToString(digest.Sum(nil))
	if actual != item.Digest || fmt.Sprint(counts) != fmt.Sprint(item.Counts) {
		t.Errorf("%s %q speed %v pitch %v: PCM %s, expected %s", item.Voice, item.Sentence, value(item.Speed), value(item.Pitch), actual[:12], item.Digest[:12])
		for index := range min(len(counts), len(item.Counts)) {
			if counts[index] != item.Counts[index] {
				t.Errorf("  frame %d: %d samples, expected %d", index, counts[index], item.Counts[index])
				break
			}
		}
		return false
	}
	return true
}

func value(pointer *int) string {
	if pointer == nil {
		return "default"
	}
	return strconv.Itoa(*pointer)
}

func TestSentences(t *testing.T) {
	voices := loadVoices(t)
	for _, name := range voiceNames {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			file, err := os.Open(conformance.Path(t, "golden", "synthesis", "sentences-"+name+".jsonl.gz"))
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			reader, err := gzip.NewReader(file)
			if err != nil {
				t.Fatal(err)
			}
			scanner := bufio.NewScanner(reader)
			scanner.Buffer(nil, 1<<24)
			total, identical := 0, 0
			for scanner.Scan() {
				var item frameCase
				if err := json.Unmarshal(scanner.Bytes(), &item); err != nil {
					t.Fatal(err)
				}
				item.Voice = name
				total++
				if item.check(t, voices[name]) {
					identical++
				}
				if testing.Short() && total == 20 {
					break
				}
			}
			if err := scanner.Err(); err != nil {
				t.Fatal(err)
			}
			if total == 0 || identical != total {
				t.Errorf("%d of %d cases identical", identical, total)
			}
			t.Logf("%d cases identical", identical)
		})
	}
}

func TestSyntheticFrames(t *testing.T) {
	voices := loadVoices(t)
	data, err := os.ReadFile(conformance.Path(t, "golden", "synthesis", "synthetic.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var item frameCase
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			t.Fatal(err)
		}
		item.Sentence = "synthetic " + item.Voice
		item.check(t, voices[item.Voice])
	}
}

func TestInvalidFrames(t *testing.T) {
	pack := conformance.Pack(t)
	voice := NewVoice(conformance.Voice(t, pack, "female").Inventory)
	synthesizer := NewSynthesizer(voice)
	invalid := []Frame{
		{Unit: 2081, Duration: 0.1, Points: []Point{{Time: 0, Pitch: 170, Amplitude: 1}}},
		{Unit: 3334, Duration: 0.1, Points: []Point{{Time: 0, Pitch: 170, Amplitude: 1}, {Time: 1, Pitch: 170, Amplitude: 1}}},
		{Unit: -1, Duration: 0.1, Points: []Point{{Time: 0, Pitch: 170, Amplitude: 1}, {Time: 1, Pitch: 170, Amplitude: 1}}},
	}
	for _, frame := range invalid {
		if _, err := synthesizer.Synthesize(frame); err == nil {
			t.Errorf("unit %d with %d points: no error", frame.Unit, len(frame.Points))
		}
	}
	data, err := synthesizer.Synthesize(Frame{Unit: 0, Duration: -0.01})
	if err != nil || len(data) != 0 {
		t.Errorf("negative silence: %d bytes, %v", len(data), err)
	}
}
