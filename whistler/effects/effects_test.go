package effects

import (
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/phyce/OpenSAPI4/whistler/datapack"
	"github.com/phyce/OpenSAPI4/whistler/internal/conformance"
	"github.com/phyce/OpenSAPI4/whistler/internal/numeric"
	"github.com/phyce/OpenSAPI4/whistler/synthesis"
)

func readGolden(t *testing.T, name string, value any) {
	t.Helper()
	data, err := os.ReadFile(conformance.Path(t, "golden", "effects", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, value); err != nil {
		t.Fatal(err)
	}
}

func loadModes(t *testing.T) (*datapack.Pack, *datapack.Modes) {
	t.Helper()
	pack := conformance.Pack(t)
	modes, err := pack.LoadModes()
	if err != nil {
		t.Fatal(err)
	}
	return pack, modes
}

func TestModes(t *testing.T) {
	_, modes := loadModes(t)
	var expected []struct {
		Name         string            `json:"name"`
		ModeID       *string           `json:"modeId"`
		Voice        string            `json:"voice"`
		Variant      int               `json:"variant"`
		Speed        int               `json:"speed"`
		Pitch        int               `json:"pitch"`
		Preferred    bool              `json:"preferred"`
		CharacterTag string            `json:"characterTag"`
		Effects      []datapack.Effect `json:"effects"`
	}
	readGolden(t, "modes.json", &expected)
	if len(modes.Modes) != len(expected) {
		t.Fatalf("%d modes, expected %d", len(modes.Modes), len(expected))
	}
	for index, want := range expected {
		mode := &modes.Modes[index]
		modeID := ""
		if want.ModeID != nil {
			modeID = *want.ModeID
		}
		actual := fmt.Sprintf("%s %s %s %d %d %d %v %s %v", mode.Name, mode.ModeID, mode.Voice, mode.Variant, mode.Speed, mode.Pitch, mode.Preferred, mode.CharacterTag, mode.Effects)
		golden := fmt.Sprintf("%s %s %s %d %d %d %v %s %v", want.Name, modeID, want.Voice, want.Variant, want.Speed, want.Pitch, want.Preferred, want.CharacterTag, want.Effects)
		if actual != golden {
			t.Errorf("mode %d:\n  %s\nexpected\n  %s", index, actual, golden)
		}
		found, ok := modes.Find(strings.ToLower(modeID))
		if ok != (modeID != "") || (ok && found != mode) {
			t.Errorf("Find(%q): %v %v", modeID, found, ok)
		}
	}
	if _, ok := modes.Find(""); ok {
		t.Error("Find of an empty mode id found a mode")
	}
	var variants []string
	for _, variant := range modes.Variants("male") {
		variants = append(variants, variant.Name)
	}
	if want := "Male Whisper, RoboSoft One, RoboSoft Two, RoboSoft Three, Mike in Hall, Mike in Stadium, Mike in Space"; strings.Join(variants, ", ") != want {
		t.Errorf("male variants %v", variants)
	}
}

func floatBits(value float32) string {
	return fmt.Sprintf("%08x", math.Float32bits(value))
}

func TestPresets(t *testing.T) {
	_, modes := loadModes(t)
	var expected []struct {
		Preset           int      `json:"preset"`
		SampleRate       int      `json:"sampleRate"`
		InputGain        string   `json:"inputGain"`
		DryGain          string   `json:"dryGain"`
		LengthModulation string   `json:"lengthModulation"`
		Lines            [][2]any `json:"lines"`
	}
	readGolden(t, "presets.json", &expected)
	if len(expected) != 2*len(modes.ReverbPresets) {
		t.Fatalf("%d presets, golden has %d", len(modes.ReverbPresets), len(expected)/2)
	}
	if math.Float32bits(thousandth) != 0x3A83126F {
		t.Errorf("thousandth is %08x", math.Float32bits(thousandth))
	}
	for _, want := range expected {
		reverb := newReverb(modes.ReverbPresets[want.Preset-1], want.SampleRate)
		lines := []string{}
		for _, line := range reverb.lines {
			lines = append(lines, fmt.Sprintf("%d %s", len(line.buffer), floatBits(line.gain)))
		}
		expectedLines := []string{}
		for _, line := range want.Lines {
			expectedLines = append(expectedLines, fmt.Sprintf("%v %v", line[0], line[1]))
		}
		actual := fmt.Sprintf("%s %s %s %v", floatBits(reverb.inputGain), floatBits(reverb.dryGain), floatBits(reverb.lengthModulation), lines)
		golden := fmt.Sprintf("%s %s %s %v", want.InputGain, want.DryGain, want.LengthModulation, expectedLines)
		if actual != golden {
			t.Errorf("preset %d at %d Hz: %s, expected %s", want.Preset, want.SampleRate, actual, golden)
		}
	}
}

func generatedBlock(state *uint32, size int, exponent any) []float32 {
	samples := make([]float32, size)
	for index := range samples {
		*state = *state*1664525 + 1013904223
		if exponent != nil {
			samples[index] = float32(math.Ldexp(float64(int32(*state)>>8), int(exponent.(float64))))
		}
	}
	return samples
}

func samplesDigest(samples []float32) string {
	digest := sha256.New()
	_ = binary.Write(digest, binary.LittleEndian, samples)
	return hex.EncodeToString(digest.Sum(nil))
}

func TestFilters(t *testing.T) {
	_, modes := loadModes(t)
	data, err := os.ReadFile(conformance.Path(t, "golden", "effects", "filters.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var run struct {
			Kind         string    `json:"kind"`
			Preset       int       `json:"preset"`
			SampleRate   int       `json:"sampleRate"`
			Coefficients []float32 `json:"coefficients"`
			Seed         uint32    `json:"seed"`
			Blocks       [][]any   `json:"blocks"`
			Outputs      []any     `json:"outputs"`
		}
		if err := json.Unmarshal([]byte(line), &run); err != nil {
			t.Fatal(err)
		}
		state := run.Seed
		random := numeric.NewRandom()
		var reverb *reverb
		var filter *postFilter
		if run.Kind == "reverb" {
			reverb = newReverb(modes.ReverbPresets[run.Preset-1], run.SampleRate)
		} else {
			filter = newPostFilter(run.Coefficients)
		}
		for index, block := range run.Blocks {
			samples := generatedBlock(&state, int(block[0].(float64)), block[1])
			actual := ""
			if reverb != nil {
				reverb.process(samples)
				reverb.endFrame(&random)
				actual = fmt.Sprintf("%s %v", samplesDigest(samples), reverb.randomValue)
			} else {
				if !block[2].(bool) {
					filter.apply(samples)
				}
				actual = samplesDigest(samples)
			}
			expected := fmt.Sprint(run.Outputs[index])
			if output, isList := run.Outputs[index].([]any); isList {
				expected = fmt.Sprintf("%s %v", output[0], float32(output[1].(float64)))
			}
			if actual != expected {
				t.Errorf("%s %d %d Hz %v, block %d: %s, expected %s", run.Kind, run.Preset, run.SampleRate, run.Coefficients, index, actual, expected)
				break
			}
		}
	}
}

type engineText struct {
	Items       []json.RawMessage `json:"items"`
	Counts      []int             `json:"counts"`
	Digest      string            `json:"sha256"`
	RandomValue *float64          `json:"randomValue"`
}

type engineRun struct {
	Name  string       `json:"name"`
	Mode  string       `json:"mode"`
	Texts []engineText `json:"texts"`
}

type voiceCache struct {
	pack   *datapack.Pack
	lock   sync.Mutex
	voices map[string]*synthesis.Voice
}

func (cache *voiceCache) voice(t *testing.T, name string) *synthesis.Voice {
	cache.lock.Lock()
	defer cache.lock.Unlock()
	if voice, found := cache.voices[name]; found {
		return voice
	}
	voice := synthesis.NewVoice(conformance.Voice(t, cache.pack, name).Inventory)
	cache.voices[name] = voice
	return voice
}

func (run *engineRun) check(t *testing.T, modes *datapack.Modes, voices *voiceCache) {
	index := slices.IndexFunc(modes.Modes, func(mode datapack.Mode) bool { return mode.Name == run.Mode })
	if index < 0 {
		t.Fatalf("no mode %q", run.Mode)
	}
	mode := &modes.Modes[index]
	voice := voices.voice(t, mode.Voice)
	effects := New(modes, mode, voice.SampleRate())
	synthesizer := synthesis.NewSynthesizer(voice)
	synthesizer.Effects = effects
	for number, text := range run.Texts {
		if testing.Short() && number > 1 {
			return
		}
		digest := sha256.New()
		counts := []int{}
		for _, item := range text.Items {
			if item[0] == '{' {
				var control struct {
					Character *string `json:"character"`
					Reset     bool    `json:"reset"`
				}
				if err := json.Unmarshal(item, &control); err != nil {
					t.Fatal(err)
				}
				if control.Character != nil {
					effects.Character(*control.Character)
				} else if control.Reset {
					effects.Reset()
				}
				continue
			}
			data, err := synthesizer.Synthesize(parseFrame(t, item))
			if err != nil {
				t.Fatalf("%s %s text %d: %v", run.Mode, run.Name, number, err)
			}
			counts = append(counts, len(data)/2)
			digest.Write(data)
		}
		actual := hex.EncodeToString(digest.Sum(nil))
		randomValue := "none"
		if effects.reverb != nil {
			randomValue = fmt.Sprint(effects.reverb.randomValue)
		}
		expectedRandomValue := "none"
		if text.RandomValue != nil {
			expectedRandomValue = fmt.Sprint(float32(*text.RandomValue))
		}
		if actual != text.Digest || !slices.Equal(counts, text.Counts) || randomValue != expectedRandomValue {
			t.Errorf("%s, %s, text %d: PCM %s, expected %s; random value %s, expected %s", run.Mode, run.Name, number, actual[:12], text.Digest[:12], randomValue, expectedRandomValue)
			for frame := range min(len(counts), len(text.Counts)) {
				if counts[frame] != text.Counts[frame] {
					t.Errorf("  frame %d: %d samples, expected %d", frame, counts[frame], text.Counts[frame])
					break
				}
			}
			return
		}
	}
}

func parseFrame(t *testing.T, item json.RawMessage) synthesis.Frame {
	var fields []json.RawMessage
	var unit int
	var duration float64
	var points [][3]float64
	if json.Unmarshal(item, &fields) != nil || len(fields) != 3 || json.Unmarshal(fields[0], &unit) != nil ||
		json.Unmarshal(fields[1], &duration) != nil || json.Unmarshal(fields[2], &points) != nil {
		t.Fatalf("frame %s does not parse", item)
	}
	frame := synthesis.Frame{Unit: unit, Duration: float32(duration)}
	for _, point := range points {
		frame.Points = append(frame.Points, synthesis.Point{Time: float32(point[0]), Pitch: int16(point[1]), Amplitude: float32(point[2])})
	}
	return frame
}

func TestSentences(t *testing.T) {
	pack, modes := loadModes(t)
	file, err := os.Open(conformance.Path(t, "golden", "effects", "sentences.jsonl.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(nil, 1<<26)
	var runs []*engineRun
	for scanner.Scan() {
		var run engineRun
		if err := json.Unmarshal(scanner.Bytes(), &run); err != nil {
			t.Fatal(err)
		}
		runs = append(runs, &run)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	voices := &voiceCache{pack: pack, voices: map[string]*synthesis.Voice{}}
	texts := 0
	for _, run := range runs {
		texts += len(run.Texts)
		t.Run(run.Mode+"/"+run.Name, func(t *testing.T) {
			t.Parallel()
			run.check(t, modes, voices)
		})
	}
	t.Logf("%d engine runs, %d texts", len(runs), texts)
}

func switchingModes() *datapack.Modes {
	whisper := []datapack.Effect{{Effect: datapack.EffectWhisper}, {Effect: datapack.EffectPostFilter, Coefficients: []float32{0.25, -0.5, 0.25}}}
	return &datapack.Modes{
		Modes: []datapack.Mode{
			{Name: "Base", ModeID: "{00000000-0000-0000-0000-000000000001}", Voice: "test"},
			{Name: "Whisper", ModeID: "{00000000-0000-0000-0000-000000000002}", Voice: "test", Variant: 1, CharacterTag: `\Chr="Whisper"\`, Effects: whisper},
			{Name: "Robot", ModeID: "{00000000-0000-0000-0000-000000000003}", Voice: "test", Variant: 2,
				Effects: []datapack.Effect{{Effect: datapack.EffectReverb, Preset: 1}, {Effect: datapack.EffectMonotone}}},
			{Name: "Other", ModeID: "{00000000-0000-0000-0000-000000000004}", Voice: "other"},
			{Name: "Other Echo", Voice: "other", Variant: 1, CharacterTag: `\Chr="Echo"\`, Effects: []datapack.Effect{{Effect: datapack.EffectReverb, Preset: 1}}},
		},
		ReverbPresets: []datapack.ReverbPreset{{InputGain: 1, DryGain: 1, Lines: []datapack.ReverbLine{{Delay: 1, Gain: 0.5}}}},
	}
}

func TestSwitching(t *testing.T) {
	modes := switchingModes()
	base := New(modes, &modes.Modes[0], 8000)
	steps := []struct {
		name    string
		whisper bool
	}{
		{`"whisper"`, true}, {`"normal"`, false}, {`"WhIsPeR"`, true}, {`"NORMAL"`, false}, {`whisper`, false},
		{`"echo"`, false}, {`"whiſper"`, false}, {`"whİsper"`, false}, {`"monotone"`, false},
	}
	for _, step := range steps {
		base.Character(step.name)
		if base.Whisper() != step.whisper || base.reverb != nil {
			t.Errorf("base mode after %s: whisper %v, reverb %v", step.name, base.Whisper(), base.reverb != nil)
		}
	}
	whisper := New(modes, &modes.Modes[1], 8000)
	whisper.postFilter.history[1] = 7
	whisper.Character(`"bogus"`)
	if !whisper.Whisper() || whisper.postFilter.history[1] != 7 {
		t.Error("an unknown name changed the whisper mode")
	}
	whisper.Character(`"normal"`)
	if !whisper.Whisper() || whisper.postFilter == nil || whisper.postFilter.history[1] != 0 {
		t.Error("normal did not restore the whisper mode with a zeroed history")
	}
	whisper.postFilter.history[1] = 7
	whisper.Reset()
	if !whisper.Whisper() || whisper.postFilter.history[1] != 0 {
		t.Error("reset did not restore the whisper mode with a zeroed history")
	}
	whisper.postFilter.history[1] = 7
	whisper.Character(`"whisper"`)
	if !whisper.Whisper() || whisper.postFilter.history[1] != 0 {
		t.Error("whisper on the whisper mode did not zero the history")
	}
	robot := New(modes, &modes.Modes[2], 8000)
	reverb := robot.reverb
	robot.Character(`"whisper"`)
	robot.Apply(make([]float32, 3), true)
	robot.EndFrame()
	if !robot.Whisper() || robot.postFilter == nil || robot.reverb != reverb || !robot.reverbOn {
		t.Error("whisper did not stack on the robot")
	}
	robot.Character(`"normal"`)
	if robot.Whisper() || robot.postFilter != nil || robot.reverb != reverb || !robot.reverbOn {
		t.Error("normal did not return the robot to its own effects")
	}
	other := New(modes, &modes.Modes[3], 8000)
	other.Character(`"echo"`)
	if other.reverb == nil || !other.reverbOn {
		t.Fatal("a tag did not create the reverb")
	}
	other.Character(`"normal"`)
	random := other.random
	other.Apply(make([]float32, 5), false)
	other.EndFrame()
	if other.reverb == nil || other.reverbOn || other.random != random {
		t.Error("normal did not switch the reverb off and keep it")
	}
	other.Character(`"echo"`)
	if !other.reverbOn {
		t.Error("the reverb did not come back")
	}
}

func TestMonotone(t *testing.T) {
	modes := switchingModes()
	steps := []struct {
		mode  int
		names []string
		on    []bool
	}{
		{0, []string{`"monotone"`, `"normal"`, `"MONOTONE"`, `"whisper"`, `"normal"`, `monotone`}, []bool{true, false, true, true, false, false}},
		{2, []string{`"normal"`, `"monotone"`, `"bogus"`, `"normal"`}, []bool{true, true, true, true}},
		{1, []string{`"monotone"`}, []bool{true}},
	}
	for _, step := range steps {
		monotone := NewMonotone(&modes.Modes[step.mode])
		if monotone.On() != (step.mode == 2) {
			t.Errorf("%s starts with monotone %v", modes.Modes[step.mode].Name, monotone.On())
		}
		for index, name := range step.names {
			monotone.Character(name)
			if monotone.On() != step.on[index] {
				t.Errorf("%s after %s: monotone %v", modes.Modes[step.mode].Name, name, monotone.On())
			}
		}
		monotone.Reset()
		if monotone.On() != (step.mode == 2) {
			t.Errorf("%s after reset: monotone %v", modes.Modes[step.mode].Name, monotone.On())
		}
	}
}
