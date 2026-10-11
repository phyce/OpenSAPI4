package posting

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/phyce/OpenSAPI4/whistler/datapack"
	"github.com/phyce/OpenSAPI4/whistler/internal/conformance"
	"github.com/phyce/OpenSAPI4/whistler/units"
)

func TestConverter(t *testing.T) {
	text, err := conformance.Pack(t).LoadText()
	if err != nil {
		t.Fatal(err)
	}
	converter := NewConverter(text.PhoneSet)
	failures := 0
	rows := conformance.ReadLines[[3]json.RawMessage](t, "golden", "transplant", "converter.jsonl.gz")
	for _, row := range rows {
		var codes []uint16
		var valid bool
		var name string
		if json.Unmarshal(row[0], &codes) != nil || json.Unmarshal(row[1], &valid) != nil || json.Unmarshal(row[2], &name) != nil {
			t.Fatalf("golden row %s", row)
		}
		if converted, convertedValid := converter.Convert(codes); string(converted) != name || convertedValid != valid {
			if failures++; failures <= 20 {
				t.Errorf("%04x: %q %v, expected %q %v", codes, converted, convertedValid, name, valid)
			}
		}
	}
	t.Logf("%d inputs, %d failed", len(rows), failures)
}

func TestNumbers(t *testing.T) {
	failures := 0
	rows := conformance.ReadLines[[4]json.RawMessage](t, "golden", "transplant", "numbers.jsonl.gz")
	for _, row := range rows {
		var textValue, atofBits string
		var atoi int32
		var scan [2]json.RawMessage
		var count int
		var values []uint32
		if json.Unmarshal(row[0], &textValue) != nil || json.Unmarshal(row[1], &atoi) != nil || json.Unmarshal(row[2], &atofBits) != nil ||
			json.Unmarshal(row[3], &scan) != nil || json.Unmarshal(scan[0], &count) != nil || json.Unmarshal(scan[1], &values) != nil {
			t.Fatalf("golden row %s", row)
		}
		bits, err := hex.DecodeString(atofBits)
		if err != nil {
			t.Fatal(err)
		}
		text := cText(string(conformance.Bytes(textValue)) + "\x00")
		scannedCount, scanned := text.scanCodes(0)
		actual := fmt.Sprint(text.atoi(0), math.Float64bits(text.atof(0)), scannedCount, scanned)
		expected := fmt.Sprint(atoi, binary.LittleEndian.Uint64(bits), count, values)
		if actual != expected {
			if failures++; failures <= 20 {
				t.Errorf("%q: %s, expected %s", textValue, actual, expected)
			}
		}
	}
	t.Logf("%d texts, %d failed", len(rows), failures)
}

func TestRunningOutOfText(t *testing.T) {
	converter := NewConverter([]datapack.PhoneSetEntry{{IPA: []uint16{'h'}, Name: "HH"}})
	for _, value := range []string{"=5<RD", "=5<", "=5<>68", "=5<>68,80 (50"} {
		batch := NewBatch(converter, 4)
		if batch.Transplant(value, 100) || batch.written != 0 {
			t.Errorf("%q: the transplant succeeded or wasn't taken back", value)
		}
	}
}

func TestRecords(t *testing.T) {
	batch := NewBatch(NewConverter(nil), 3)
	batch.Copy("#", 0, nil)
	batch.Copy("AA1", 1.2, []units.Point{{Position: 1, Pitch: 120}})
	batch.Pause("300")
	records, following := batch.Records()
	kinds := []units.RecordKind{units.BoundaryRecord, units.PhoneRecord, units.BoundaryRecord, units.ControlRecord, units.BoundaryRecord}
	if len(records) != len(kinds) || following != units.PhoneRecord {
		t.Fatalf("%d records, following %v", len(records), following)
	}
	for index, record := range records {
		if record.Kind != kinds[index] {
			t.Errorf("record %d is %+v", index, record)
		}
	}
	if !slices.Equal(records[1].Points, []units.Point{{Position: 1, Pitch: 120}}) || records[1].Duration != 1.2 || records[3].Control != (units.Control{Kind: units.Pause, Value: 300}) {
		t.Errorf("records %+v", records)
	}
}
