package units

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
	"strconv"
	"strings"
	"testing"

	"github.com/phyce/OpenSAPI4/whistler/datapack"
	"github.com/phyce/OpenSAPI4/whistler/internal/conformance"
	"github.com/phyce/OpenSAPI4/whistler/synthesis"
)

var voiceNames = []string{"female", "female8", "male", "male8", "sam"}

func readGolden(t *testing.T, name string, value any) {
	t.Helper()
	data, err := os.ReadFile(conformance.Path(t, "golden", "units", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, value); err != nil {
		t.Fatal(err)
	}
}

func loadVoices(t *testing.T) map[string]*datapack.Voice {
	t.Helper()
	pack := conformance.Pack(t)
	voices := map[string]*datapack.Voice{}
	for _, name := range voiceNames {
		voices[name] = conformance.Voice(t, pack, name)
	}
	return voices
}

func TestSettings(t *testing.T) {
	var golden struct {
		Speeds map[string]struct {
			Threshold int      `json:"threshold"`
			Speeds    [][4]any `json:"speeds"`
			SetSpeed  [][3]any `json:"setSpeed"`
		} `json:"speeds"`
		VolumeFactorsSha256 string `json:"volumeFactorsSha256"`
	}
	readGolden(t, "settings.json", &golden)
	voice := loadVoices(t)["male"]
	for baseText, table := range golden.Speeds {
		base, _ := strconv.Atoi(baseText)
		mapper := NewMapper(voice, base)
		if mapper.threshold != table.Threshold {
			t.Errorf("base %d: threshold %d, expected %d", base, mapper.threshold, table.Threshold)
		}
		for _, row := range table.Speeds {
			speed := uint32(row[0].(float64))
			if !mapper.SetSpeed(speed) {
				t.Errorf("base %d: SetSpeed(%d) refused", base, speed)
				continue
			}
			actual := [3]any{mapper.slow, float64(math.Float32bits(mapper.scale)), float64(math.Float32bits(mapper.offset))}
			if actual != [3]any{row[1], row[2], row[3]} {
				t.Errorf("base %d speed %d: slow, scale, offset %v, expected %v", base, speed, actual, row[1:])
			}
		}
		for _, row := range table.SetSpeed {
			value := uint32(row[0].(float64))
			edge := NewMapper(voice, base)
			accepted := edge.SetSpeed(value)
			if accepted != row[1].(bool) || float64(edge.speed) != row[2].(float64) {
				t.Errorf("base %d: SetSpeed(%d) gives %v and speed %d, expected %v and %v", base, value, accepted, edge.speed, row[1], row[2])
			}
		}
	}
	factors := make([]byte, 0, 4*0x10000)
	for volume := range 0x10000 {
		factors = binary.LittleEndian.AppendUint32(factors, math.Float32bits(float32(float64(volume)*float64(volumeScale))))
	}
	if digest := sha256.Sum256(factors); hex.EncodeToString(digest[:]) != golden.VolumeFactorsSha256 {
		t.Error("volume factors differ")
	}
}

func TestVisual(t *testing.T) {
	var golden map[string]map[string][3]json.RawMessage
	readGolden(t, "visual.json", &golden)
	voices := loadVoices(t)
	for _, name := range voiceNames {
		voice := voices[name]
		silence := voice.Visual[voice.SilencePhone]
		for phone, phoneName := range voice.Phones {
			expected, found := golden[name][phoneName]
			if !found {
				if voice.HasTree(phone) || phoneName == "SIL" {
					t.Errorf("%s: %s is missing from the golden data", name, phoneName)
				} else if voice.Visual[phone] != silence {
					t.Errorf("%s: %s has no tree but not SIL's entry", name, phoneName)
				}
				continue
			}
			var ipa, engine uint16
			var mouth []int
			if json.Unmarshal(expected[0], &ipa) != nil || json.Unmarshal(expected[1], &engine) != nil || json.Unmarshal(expected[2], &mouth) != nil {
				t.Fatalf("%s %s: golden entry %s", name, phoneName, expected)
			}
			entry := voice.Visual[phone]
			actual := entry.Mouth
			shape := []int{int(actual.Height), int(actual.Width), int(actual.Upturn), int(actual.JawOpen), int(actual.UpperTeethVisible), int(actual.LowerTeethVisible),
				int(actual.TonguePosition), int(actual.LipTension)}
			if entry.IPA != ipa || uint16(entry.Engine) != engine || fmt.Sprint(shape) != fmt.Sprint(mouth) {
				t.Errorf("%s %s: %+v, expected %d %d %v", name, phoneName, entry, ipa, engine, mouth)
			}
		}
	}
}

func TestTrees(t *testing.T) {
	var golden map[string]map[string]struct {
		Sha256  string `json:"sha256"`
		Senones int    `json:"senones"`
	}
	readGolden(t, "trees.json", &golden)
	voices := loadVoices(t)
	for _, name := range voiceNames {
		voice := voices[name]
		checked := 0
		for phone, phoneName := range voice.Phones {
			if !voice.HasTree(phone) {
				continue
			}
			expected, found := golden[name][phoneName]
			if !found {
				t.Errorf("%s: %s is missing from the golden data", name, phoneName)
				continue
			}
			checked++
			senones := make([]byte, 0, 2*4*len(voice.Phones)*len(voice.Phones))
			distinct := map[int]bool{}
			for _, position := range []int{datapack.WordInitial, datapack.WordFinal, datapack.SinglePhoneWord, datapack.InsideWord} {
				for left := range voice.Phones {
					for right := range voice.Phones {
						senone := voice.Senone(phone, left, right, position)
						senones = binary.LittleEndian.AppendUint16(senones, uint16(senone))
						distinct[senone] = true
					}
				}
			}
			digest := sha256.Sum256(senones)
			if hex.EncodeToString(digest[:]) != expected.Sha256 || len(distinct) != expected.Senones {
				t.Errorf("%s %s: %d senones reached, expected %d; lookups differ: %v", name, phoneName, len(distinct), expected.Senones,
					hex.EncodeToString(digest[:]) != expected.Sha256)
			}
		}
		if checked != len(golden[name]) {
			t.Errorf("%s: checked %d phones, golden data has %d", name, checked, len(golden[name]))
		}
	}
}

type goldenCase struct {
	Name      string  `json:"name"`
	Voice     string  `json:"voice"`
	BaseSpeed int     `json:"baseSpeed"`
	Speed     *uint32 `json:"speed"`
	Volume    *uint32 `json:"volume"`
	Batches   []struct {
		Records   []json.RawMessage `json:"records"`
		Following RecordKind        `json:"following"`
		Items     []json.RawMessage `json:"items"`
	} `json:"batches"`
	State struct {
		Speed  int     `json:"speed"`
		Slow   bool    `json:"slow"`
		Scale  float64 `json:"scale"`
		Offset float64 `json:"offset"`
		Volume uint16  `json:"volume"`
	} `json:"state"`
}

// parseRecord reads a golden record: [name], [SIL, duration] or [phone, duration, pitch], or in the general
// cases [phone, duration bits, [[position bits, pitch bits, amplitude bits], ...]].
func parseRecord(data json.RawMessage, general bool) (Record, error) {
	var fields []json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || len(fields) == 0 {
		return Record{}, fmt.Errorf("record %s", data)
	}
	var name string
	if err := json.Unmarshal(fields[0], &name); err != nil {
		return Record{}, err
	}
	if general && len(fields) == 3 {
		var duration uint32
		var points [][3]uint32
		if json.Unmarshal(fields[1], &duration) != nil || json.Unmarshal(fields[2], &points) != nil {
			return Record{}, fmt.Errorf("record %s", data)
		}
		record := Record{Kind: PhoneRecord, Name: name, Duration: math.Float32frombits(duration)}
		for _, point := range points {
			record.Points = append(record.Points, Point{Position: math.Float32frombits(point[0]), Pitch: math.Float32frombits(point[1]),
				Amplitude: math.Float32frombits(point[2])})
		}
		return record, nil
	}
	numbers := make([]float64, len(fields)-1)
	for index, field := range fields[1:] {
		if err := json.Unmarshal(field, &numbers[index]); err != nil {
			return Record{}, err
		}
	}
	switch {
	case name == "#":
		return Record{Kind: BoundaryRecord}, nil
	case strings.HasPrefix(name, `\`):
		return Record{Kind: ControlRecord, Control: ParseTag(name)}, nil
	case len(numbers) == 1:
		return Record{Kind: PhoneRecord, Name: name, Duration: float32(numbers[0])}, nil
	case len(numbers) == 2:
		return Record{Kind: PhoneRecord, Name: name, Duration: float32(numbers[0]), Points: []Point{{Position: 1, Pitch: float32(numbers[1])}}}, nil
	}
	return Record{}, fmt.Errorf("record %s", data)
}

func describeItem(voice *datapack.Voice, item Item) string {
	if item.Frame == nil {
		return fmt.Sprintf("%+v", item.Control)
	}
	return fmt.Sprintf("%s unit %d %v %v", voice.Phones[item.Frame.Phone], item.Frame.Unit, item.Frame.Duration, item.Frame.Points)
}

// compareItem compares an item with a golden one. Frames are [name, unit, duration, [[position, pitch,
// amplitude], ...]], with every float as its float32 bits in the general cases.
func compareItem(voice *datapack.Voice, item Item, data json.RawMessage, general bool) error {
	if data[0] == '[' {
		var fields [4]json.RawMessage
		var name string
		var unit int
		var duration float64
		var points [][3]float64
		if json.Unmarshal(data, &fields) != nil || json.Unmarshal(fields[0], &name) != nil || json.Unmarshal(fields[1], &unit) != nil ||
			json.Unmarshal(fields[2], &duration) != nil || json.Unmarshal(fields[3], &points) != nil {
			return fmt.Errorf("golden frame %s", data)
		}
		float := func(value float64) float32 {
			if general {
				return math.Float32frombits(uint32(value))
			}
			return float32(value)
		}
		expected := Frame{Frame: synthesis.Frame{Unit: unit, Duration: float(duration)}}
		for _, point := range points {
			expected.Points = append(expected.Points, synthesis.Point{Time: float(point[0]), Pitch: int16(point[1]), Amplitude: float(point[2])})
		}
		if item.Frame == nil || voice.Phones[item.Frame.Phone] != name || item.Frame.Unit != unit || !sameBits(item.Frame.Duration, expected.Duration) ||
			!samePoints(item.Frame.Points, expected.Points) {
			return fmt.Errorf("%s, expected %s unit %d %v %v", describeItem(voice, item), name, unit, expected.Duration, expected.Points)
		}
		return nil
	}
	var fields struct {
		WordPosition *uint32 `json:"wordPosition"`
		Bookmark     *uint32 `json:"bookmark"`
		Character    *string `json:"character"`
		Reset        bool    `json:"reset"`
		Speed        *uint32 `json:"speed"`
		Volume       *uint32 `json:"volume"`
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	var expected Control
	switch {
	case fields.WordPosition != nil:
		expected = Control{Kind: WordPosition, Value: *fields.WordPosition}
	case fields.Bookmark != nil:
		expected = Control{Kind: Bookmark, Value: *fields.Bookmark}
	case fields.Character != nil:
		expected = Control{Kind: Character, Name: *fields.Character}
	case fields.Reset:
		expected = Control{Kind: Reset}
	case fields.Speed != nil:
		expected = Control{Kind: Speed, Value: *fields.Speed}
	case fields.Volume != nil:
		expected = Control{Kind: Volume, Value: *fields.Volume}
	default:
		return fmt.Errorf("golden item %s", data)
	}
	if item.Frame != nil || item.Control != expected {
		return fmt.Errorf("%s, expected %+v", describeItem(voice, item), expected)
	}
	return nil
}

// sameBits compares floats bit for bit, any NaN with any other: the original's NaNs carry its sign, and no
// stage reads it.
func sameBits(first float32, second float32) bool {
	return math.Float32bits(first) == math.Float32bits(second) || first != first && second != second
}

func samePoints(first []synthesis.Point, second []synthesis.Point) bool {
	if len(first) != len(second) {
		return false
	}
	for index := range first {
		if !sameBits(first[index].Time, second[index].Time) || first[index].Pitch != second[index].Pitch || !sameBits(first[index].Amplitude, second[index].Amplitude) {
			return false
		}
	}
	return true
}

func runCase(voice *datapack.Voice, golden goldenCase, general bool) error {
	mapper := NewMapper(voice, golden.BaseSpeed)
	if golden.Speed != nil && !mapper.SetSpeed(*golden.Speed) {
		return fmt.Errorf("speed %d refused", *golden.Speed)
	}
	if golden.Volume != nil {
		mapper.SetVolume(*golden.Volume)
	}
	for number, batch := range golden.Batches {
		records := make([]Record, len(batch.Records))
		for index, data := range batch.Records {
			record, err := parseRecord(data, general)
			if err != nil {
				return err
			}
			records[index] = record
		}
		items := mapper.Map(records, batch.Following)
		for index := range max(len(items), len(batch.Items)) {
			if index >= len(items) || index >= len(batch.Items) {
				return fmt.Errorf("batch %d: %d items, expected %d", number, len(items), len(batch.Items))
			}
			if err := compareItem(voice, items[index], batch.Items[index], general); err != nil {
				return fmt.Errorf("batch %d item %d: %w", number, index, err)
			}
		}
	}
	state := golden.State
	if mapper.speed != state.Speed || mapper.slow != state.Slow || !sameBits(mapper.scale, float32(state.Scale)) || !sameBits(mapper.offset, float32(state.Offset)) ||
		mapper.volume != state.Volume {
		return fmt.Errorf("state speed %d slow %v scale %v offset %v volume %d, expected %+v", mapper.speed, mapper.slow, mapper.scale, mapper.offset, mapper.volume, state)
	}
	return nil
}

func TestCases(t *testing.T) {
	runCases(t, "cases.jsonl.gz", false)
}

// TestGeneralCases runs records as prosody transplants make them through the general interpolation.
func TestGeneralCases(t *testing.T) {
	runCases(t, "general.jsonl.gz", true)
}

func runCases(t *testing.T, name string, general bool) {
	voices := loadVoices(t)
	file, err := os.Open(conformance.Path(t, "golden", "units", name))
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
	cases, frames, failures := 0, 0, 0
	for line := 0; scanner.Scan(); line++ {
		if testing.Short() && line%20 != 0 {
			continue
		}
		var golden goldenCase
		if err := json.Unmarshal(scanner.Bytes(), &golden); err != nil {
			t.Fatal(err)
		}
		cases++
		for _, batch := range golden.Batches {
			for _, item := range batch.Items {
				if item[0] == '[' {
					frames++
				}
			}
		}
		if err := runCase(voices[golden.Voice], golden, general); err != nil {
			failures++
			if failures <= 20 {
				t.Errorf("%s: %v", golden.Name, err)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d cases, %d frames, %d failed", cases, frames, failures)
	if failures > 20 {
		t.Errorf("%d more cases failed", failures-20)
	}
}

func TestParseTag(t *testing.T) {
	tags := map[string]Control{
		`\Wrd=12\`:          {Kind: WordPosition, Value: 12},
		`\WRD=4294967297\`:  {Kind: WordPosition, Value: 1},
		`\Wrd=-5\`:          {Kind: Unparsed},
		`\Wrd=12`:           {Kind: Unparsed},
		`\Wrd=12x\`:         {Kind: Unparsed},
		`\Wrd=\`:            {Kind: Unparsed},
		`\Mrk=7\`:           {Kind: Bookmark, Value: 7},
		`\Pau=0\`:           {Kind: Pause},
		`\Spd=80\`:          {Kind: Speed, Value: 80},
		`\Vol=65536\`:       {Kind: Volume, Value: 65536},
		`\Chr="Whisper"\`:   {Kind: Character, Name: `"Whisper"`},
		`\Chr="Whisper"`:    {Kind: Unparsed},
		`\Chr=Whisper\`:     {Kind: Unparsed},
		`\RST\`:             {Kind: Reset},
		`\Rst`:              {Kind: Unparsed},
		`\Pit=100\`:         {Kind: Unparsed},
		`\Ctx="Address"\`:   {Kind: Unparsed},
		`\Wrdx=1\`:          {Kind: Unparsed},
		`Wrd=1\`:            {Kind: Unparsed},
		`\Wrd<=1\`:          {Kind: Unparsed},
		`\Chr="a" "b"\`:     {Kind: Unparsed},
		`\chr="Monotone"\x`: {Kind: Character, Name: `"Monotone"`},
	}
	for text, expected := range tags {
		if control := ParseTag(text); control != expected {
			t.Errorf("%s: %+v, expected %+v", text, control, expected)
		}
	}
}

func TestUnusualRecords(t *testing.T) {
	voice := loadVoices(t)["male"]
	mapper := NewMapper(voice, 170)
	if items := mapper.Map(nil, BoundaryRecord); len(items) != 0 {
		t.Errorf("no records give %d items", len(items))
	}
	boundary := Record{Kind: BoundaryRecord}
	items := mapper.Map([]Record{boundary, {Kind: PhoneRecord, Name: "QQ", Duration: 0.1}, boundary, {Kind: PhoneRecord, Name: "SILb", Duration: 0.1}, boundary}, BoundaryRecord)
	for _, item := range items {
		if item.Frame == nil || item.Frame.Phone != voice.SilencePhone || item.Frame.Unit != 0 {
			t.Errorf("an unknown or treeless phone gives %s, expected SIL", describeItem(voice, item))
		}
	}
	plain := mapper.Map([]Record{boundary, {Kind: PhoneRecord, Name: "AA1", Duration: 1, Points: []Point{{Position: 1, Pitch: 120}}}, boundary}, BoundaryRecord)
	undefined := mapper.Map([]Record{boundary, {Kind: PhoneRecord, Name: "AA1", Duration: float32(math.NaN()), Points: []Point{{Position: 1, Pitch: 120}}}, boundary}, BoundaryRecord)
	if len(plain) != 1 || len(undefined) != 1 || !sameBits(plain[0].Frame.Duration, undefined[0].Frame.Duration) {
		t.Error("a NaN duration code doesn't count as 1.0")
	}
	if ending := mapper.Map([]Record{boundary, {Kind: PhoneRecord, Name: "AA1", Duration: 1, Points: []Point{{Position: 1, Pitch: 120}}}}, PhoneRecord); len(ending) != 1 || ending[0].Frame.Unit == 0 {
		t.Error("a batch ending with a phone gives no frame for it")
	}
}
