package datapack

import (
	"encoding/json"
	"fmt"
)

const (
	// VowelCount is the number of vowels the vowel database has, ContextCount the number of contexts it tells
	// apart on each side of a vowel.
	VowelCount   = 24
	ContextCount = 34
	// FormantCount is the number of formant tracks a record has a trajectory for, F1 to F4; the first three
	// have a bandwidth track.
	FormantCount = 4
)

// The lengths of a record of the vowel database in vowels.json, with its two contexts.
const (
	reducedRecordLength = 21
	fullRecordLength    = 23
	highestBandwidth    = 1000
	highestTiming       = 15
)

// Vowels is the vowel database (parameters.md §11): for each vowel, a record of how its formants move for each
// pair of contexts it has one for. Contexts are the symbols' contexts, -1 for a symbol that has none.
type Vowels struct {
	Contexts [SymbolCount]int32
	Vowels   [VowelCount]VowelEntry
	indexes  [SymbolCount]int
}

// A VowelEntry is one vowel's records, and for the vowels other than @ and | the trajectories their records
// choose from, one table per formant.
type VowelEntry struct {
	Symbol       byte
	Records      []VowelRecord
	Trajectories [FormantCount][]Trajectory
	records      [ContextCount * ContextCount]int32
}

// A VowelRecord is how a vowel's formants move between a left and a right context. Start, Middle and End are
// frame bytes of the formant tracks (parameters.md §2.2). The records of @ and | have a Middle, the frames to
// it (Timing) and their bandwidths' ends as frame bytes; the others have a trajectory of each formant and the
// starts, middles and ends of their bandwidths in Hz.
type VowelRecord struct {
	Left, Right   int32
	Start, End    [FormantCount]byte
	Middle        [FormantCount]byte
	Timing        [FormantCount]int32
	BandwidthEnds [3]byte
	Trajectories  [FormantCount]int32
	Bandwidths    [3][3]int32
	Alternative   *Alternative
}

// An Alternative replaces a record when the node before the previous phoneme, or with AfterNext the node after
// the next one, is Symbol.
type Alternative struct {
	AfterNext bool
	Symbol    byte
	Record    VowelRecord
}

// A Trajectory is four points of a formant, as frame bytes, and the timing of each.
type Trajectory struct {
	Points [4]byte
	Timing [4]int32
}

// Index returns a symbol's place among the vowels, or -1.
func (vowels *Vowels) Index(symbol byte) int {
	if symbol >= SymbolCount {
		return -1
	}
	return vowels.indexes[symbol]
}

// Find returns the vowel's record for a left and a right context, or nil.
func (entry *VowelEntry) Find(left, right int32) *VowelRecord {
	if left < 0 || left >= ContextCount || right < 0 || right >= ContextCount {
		return nil
	}
	if number := entry.records[left*ContextCount+right]; number > 0 {
		return &entry.Records[number-1]
	}
	return nil
}

type alternativeDescription struct {
	Record    *int64  `json:"record"`
	Neighbour *string `json:"neighbour"`
	Symbol    *string `json:"symbol"`
	Values    []int64 `json:"values"`
}

type vowelDescription struct {
	Symbol       string                   `json:"symbol"`
	Records      [][]int64                `json:"records"`
	Alternatives []alternativeDescription `json:"alternatives"`
	Trajectories [][][]int64              `json:"trajectories"`
}

type vowelsDescription struct {
	Contexts []int64            `json:"contexts"`
	Vowels   []vowelDescription `json:"vowels"`
}

func (pack *Pack) LoadVowels() (*Vowels, error) {
	return load(pack, "vowels.json", parseVowels)
}

func parseVowels(data []byte) (*Vowels, error) {
	var description vowelsDescription
	if err := json.Unmarshal(data, &description); err != nil {
		return nil, err
	}
	vowels := &Vowels{}
	if err := copyValues(vowels.Contexts[:], description.Contexts, "contexts", -1, ContextCount-1); err != nil {
		return nil, err
	}
	if len(description.Vowels) != VowelCount {
		return nil, fmt.Errorf("vowels has %d entries, expected %d", len(description.Vowels), VowelCount)
	}
	for symbol := range vowels.indexes {
		vowels.indexes[symbol] = -1
	}
	for index, entryDescription := range description.Vowels {
		name := fmt.Sprintf("vowels[%d]", index)
		if len(entryDescription.Symbol) != 1 || !printable(entryDescription.Symbol[0]) || vowels.indexes[entryDescription.Symbol[0]] >= 0 {
			return nil, fmt.Errorf("%s has symbol %q", name, entryDescription.Symbol)
		}
		entry := &vowels.Vowels[index]
		entry.Symbol = entryDescription.Symbol[0]
		vowels.indexes[entry.Symbol] = index
		if err := parseVowelEntry(entry, entryDescription, name); err != nil {
			return nil, err
		}
	}
	return vowels, nil
}

// Reduced reports whether a vowel's records are those of @ and |.
func Reduced(symbol byte) bool {
	return symbol == '@' || symbol == '|'
}

func parseVowelEntry(entry *VowelEntry, description vowelDescription, name string) error {
	if Reduced(entry.Symbol) != (description.Trajectories == nil) {
		return fmt.Errorf("%s has %d trajectory tables", name, len(description.Trajectories))
	}
	if description.Trajectories != nil {
		if len(description.Trajectories) != FormantCount {
			return fmt.Errorf("%s has %d trajectory tables, expected %d", name, len(description.Trajectories), FormantCount)
		}
		for formant, table := range description.Trajectories {
			entry.Trajectories[formant] = make([]Trajectory, len(table))
			for number, values := range table {
				tableName := fmt.Sprintf("%s.trajectories[%d][%d]", name, formant, number)
				if len(values) != 8 {
					return fmt.Errorf("%s has %d values, expected 8", tableName, len(values))
				}
				trajectory := &entry.Trajectories[formant][number]
				if err := copyValues(trajectory.Points[:], values[:4], tableName, 0, highestByte); err != nil {
					return err
				}
				if err := copyValues(trajectory.Timing[:], values[4:], tableName, 0, highestTiming); err != nil {
					return err
				}
			}
		}
	}
	entry.Records = make([]VowelRecord, len(description.Records))
	previous := int32(-1)
	for number, values := range description.Records {
		recordName := fmt.Sprintf("%s.records[%d]", name, number)
		if len(values) < 2 {
			return fmt.Errorf("%s has %d values", recordName, len(values))
		}
		record := &entry.Records[number]
		if err := copyValue(&record.Left, &values[0], recordName+" left", 0, ContextCount-1); err != nil {
			return err
		}
		if err := copyValue(&record.Right, &values[1], recordName+" right", 0, ContextCount-1); err != nil {
			return err
		}
		position := record.Left*ContextCount + record.Right
		if position <= previous {
			return fmt.Errorf("%s is out of order", recordName)
		}
		previous = position
		entry.records[position] = int32(number + 1)
		if err := entry.parseRecord(record, values[2:], recordName); err != nil {
			return err
		}
	}
	for number, alternative := range description.Alternatives {
		alternativeName := fmt.Sprintf("%s.alternatives[%d]", name, number)
		if alternative.Record == nil || *alternative.Record < 0 || *alternative.Record >= int64(len(entry.Records)) ||
			entry.Records[*alternative.Record].Alternative != nil {
			return fmt.Errorf("%s has no record or a wrong one", alternativeName)
		}
		if alternative.Neighbour == nil || *alternative.Neighbour != "beforePrevious" && *alternative.Neighbour != "afterNext" {
			return fmt.Errorf("%s has no neighbour or a wrong one", alternativeName)
		}
		if alternative.Symbol == nil || len(*alternative.Symbol) != 1 || !printable((*alternative.Symbol)[0]) {
			return fmt.Errorf("%s has no symbol or a wrong one", alternativeName)
		}
		replacement := &Alternative{AfterNext: *alternative.Neighbour == "afterNext", Symbol: (*alternative.Symbol)[0]}
		if err := entry.parseRecord(&replacement.Record, alternative.Values, alternativeName); err != nil {
			return err
		}
		entry.Records[*alternative.Record].Alternative = replacement
	}
	return nil
}

func (entry *VowelEntry) parseRecord(record *VowelRecord, values []int64, name string) error {
	if Reduced(entry.Symbol) {
		if len(values) != reducedRecordLength-2 {
			return fmt.Errorf("%s has %d values, expected %d", name, len(values)+2, reducedRecordLength)
		}
		for _, part := range []struct {
			target []byte
			values []int64
			name   string
		}{{record.Start[:], values[0:4], "start"}, {record.Middle[:], values[4:8], "middle"}, {record.End[:], values[8:12], "end"},
			{record.BandwidthEnds[:], values[16:19], "bandwidth ends"}} {
			if err := copyValues(part.target, part.values, name+" "+part.name, 0, highestByte); err != nil {
				return err
			}
		}
		return copyValues(record.Timing[:], values[12:16], name+" timing", 0, highestTiming)
	}
	if len(values) != fullRecordLength-2 {
		return fmt.Errorf("%s has %d values, expected %d", name, len(values)+2, fullRecordLength)
	}
	if err := copyValues(record.Start[:], values[0:4], name+" start", 0, highestByte); err != nil {
		return err
	}
	if err := copyValues(record.End[:], values[4:8], name+" end", 0, highestByte); err != nil {
		return err
	}
	for formant := range FormantCount {
		trajectoryName := fmt.Sprintf("%s trajectory %d", name, formant)
		if err := copyValue(&record.Trajectories[formant], &values[8+formant], trajectoryName, 0, int64(len(entry.Trajectories[formant])-1)); err != nil {
			return err
		}
	}
	for part, partName := range []string{"bandwidth starts", "bandwidth middles", "bandwidth ends"} {
		if err := copyValues(record.Bandwidths[part][:], values[12+3*part:15+3*part], name+" "+partName, 0, highestBandwidth); err != nil {
			return err
		}
	}
	return nil
}
