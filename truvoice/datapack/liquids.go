package datapack

import (
	"fmt"
	"strings"
)

// LiquidTrackCount is the number of formant tracks the liquids' tables have rows for, F1 to F4.
const LiquidTrackCount = 4

// The ranges of the liquids' rows: a point's kind, the duration in percent and a value in Hz.
const (
	highestPointKind = 2
	highestDuration  = 100
)

// The rows each track of the tables the generator indexes itself has, and the targets of the consonants before
// a word mark and R.
var (
	rAfterVowelRows       = [3]int{1, 2, 2}
	rAcrossMarkRows       = [LiquidTrackCount]int{1, 5, 4, 4}
	targetsAcrossMarkRows = [LiquidTrackCount]int{2, 7, 5, 4}
)

// Liquids are the tables of the formants' trajectories through the liquids R and L and through Y after a vowel
// (parameters.md §14.3). R, L and Y have a table for each of F1 to F4 whose rows are chosen by context; the rows
// of R after a vowel (F1 to F3) and of R after a word mark are chosen by the generator, which also takes a
// consonant's targets before a word mark and R from TargetsAcrossMark.
type Liquids struct {
	R, L, Y           [LiquidTrackCount]LiquidTrack
	RAfterVowel       [3][]LiquidRow
	RAcrossMark       [LiquidTrackCount][]LiquidRow
	TargetsAcrossMark [LiquidTrackCount][]int32
}

// A LiquidTrack is one formant's rows: each row but the last, the default, has the contexts it is chosen for.
type LiquidTrack struct {
	Rows     []LiquidRow
	Contexts [][]LiquidContext
}

// A LiquidContext chooses its row for the symbol Before and any of the symbols After.
type LiquidContext struct {
	Before byte
	After  string
}

// A LiquidRow gives a formant's start, middle and end. Each point's kind says what its value is added to: 0
// nothing, 1 the track's last value (the end: its target), 2 the lower of the two. Duration is the percent of
// the phoneme's frames to the middle.
type LiquidRow struct {
	Kinds    [3]int32
	Duration int32
	Values   [3]int32
}

// Row returns the row of the first context with before and after, else the default.
func (track *LiquidTrack) Row(before, after byte) *LiquidRow {
	for index, contexts := range track.Contexts {
		for _, context := range contexts {
			if context.Before == before && strings.IndexByte(context.After, after) >= 0 {
				return &track.Rows[index]
			}
		}
	}
	return &track.Rows[len(track.Rows)-1]
}

type liquidTrackDescription struct {
	Rows     [][]int64    `json:"rows"`
	Contexts [][][]string `json:"contexts"`
}

type liquidsDescription struct {
	R                 []liquidTrackDescription `json:"R"`
	L                 []liquidTrackDescription `json:"L"`
	Y                 []liquidTrackDescription `json:"Y"`
	RAfterVowel       [][][]int64              `json:"rAfterVowel"`
	RAcrossMark       [][][]int64              `json:"rAcrossMark"`
	TargetsAcrossMark [][]int64                `json:"targetsAcrossMark"`
}

func parseLiquids(liquids *Liquids, description liquidsDescription) error {
	for _, table := range []struct {
		name   string
		tracks []liquidTrackDescription
		target *[LiquidTrackCount]LiquidTrack
	}{{"R", description.R, &liquids.R}, {"L", description.L, &liquids.L}, {"Y", description.Y, &liquids.Y}} {
		name := "liquids." + table.name
		if len(table.tracks) != LiquidTrackCount {
			return fmt.Errorf("%s has %d tracks, expected %d", name, len(table.tracks), LiquidTrackCount)
		}
		for index, track := range table.tracks {
			trackName := fmt.Sprintf("%s[%d]", name, index)
			if len(track.Rows) != len(track.Contexts)+1 {
				return fmt.Errorf("%s has %d rows for %d contexts, expected one more", trackName, len(track.Rows), len(track.Contexts))
			}
			rows, err := parseLiquidRows(track.Rows, trackName+".rows")
			if err != nil {
				return err
			}
			table.target[index].Rows = rows
			table.target[index].Contexts = make([][]LiquidContext, len(track.Contexts))
			for row, contexts := range track.Contexts {
				contextsName := fmt.Sprintf("%s.contexts[%d]", trackName, row)
				if len(contexts) == 0 {
					return fmt.Errorf("%s is empty", contextsName)
				}
				for _, pair := range contexts {
					if len(pair) != 2 || len(pair[0]) != 1 || !printable(pair[0][0]) || pair[1] == "" ||
						strings.IndexFunc(pair[1], func(symbol rune) bool { return symbol > 0xff || !printable(byte(symbol)) }) >= 0 {
						return fmt.Errorf("%s has %q, expected a symbol and printable symbols", contextsName, pair)
					}
					table.target[index].Contexts[row] = append(table.target[index].Contexts[row], LiquidContext{pair[0][0], pair[1]})
				}
			}
		}
	}
	if len(description.RAfterVowel) != len(rAfterVowelRows) {
		return fmt.Errorf("liquids.rAfterVowel has %d tracks, expected %d", len(description.RAfterVowel), len(rAfterVowelRows))
	}
	for track, count := range rAfterVowelRows {
		rows, err := parseCountedLiquidRows(description.RAfterVowel[track], count, fmt.Sprintf("liquids.rAfterVowel[%d]", track))
		if err != nil {
			return err
		}
		liquids.RAfterVowel[track] = rows
	}
	if len(description.RAcrossMark) != LiquidTrackCount {
		return fmt.Errorf("liquids.rAcrossMark has %d tracks, expected %d", len(description.RAcrossMark), LiquidTrackCount)
	}
	if len(description.TargetsAcrossMark) != LiquidTrackCount {
		return fmt.Errorf("liquids.targetsAcrossMark has %d tracks, expected %d", len(description.TargetsAcrossMark), LiquidTrackCount)
	}
	for track := range LiquidTrackCount {
		rows, err := parseCountedLiquidRows(description.RAcrossMark[track], rAcrossMarkRows[track], fmt.Sprintf("liquids.rAcrossMark[%d]", track))
		if err != nil {
			return err
		}
		liquids.RAcrossMark[track] = rows
		liquids.TargetsAcrossMark[track] = make([]int32, targetsAcrossMarkRows[track])
		if err := copyValues(liquids.TargetsAcrossMark[track], description.TargetsAcrossMark[track], fmt.Sprintf("liquids.targetsAcrossMark[%d]", track), 0, highestFrequency); err != nil {
			return err
		}
	}
	return nil
}

func parseCountedLiquidRows(values [][]int64, count int, name string) ([]LiquidRow, error) {
	if len(values) != count {
		return nil, fmt.Errorf("%s has %d rows, expected %d", name, len(values), count)
	}
	return parseLiquidRows(values, name)
}

func parseLiquidRows(values [][]int64, name string) ([]LiquidRow, error) {
	rows := make([]LiquidRow, len(values))
	for index, row := range values {
		rowName := fmt.Sprintf("%s[%d]", name, index)
		if len(row) != 7 {
			return nil, fmt.Errorf("%s has %d values, expected 7", rowName, len(row))
		}
		if err := copyValues(rows[index].Kinds[:], row[:3], rowName+" kinds", 0, highestPointKind); err != nil {
			return nil, err
		}
		if err := copyValue(&rows[index].Duration, &row[3], rowName+" duration", 0, highestDuration); err != nil {
			return nil, err
		}
		if err := copyValues(rows[index].Values[:], row[4:], rowName+" values", -highestFrequency, highestFrequency); err != nil {
			return nil, err
		}
	}
	return rows, nil
}
