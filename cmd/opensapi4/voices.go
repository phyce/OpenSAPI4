package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
)

func listVoices(arguments []string) int {
	flags, folder := newFlags("voices", "[-json]")
	asJSON := flags.Bool("json", false, "list them as JSON, with every detail")
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
	list, err := voices.List()
	if err != nil {
		return fail(err)
	}
	if *asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(list); err != nil {
			return fail(err)
		}
		return 0
	}
	table := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "VOICE\tGENDER\tRATE\tPITCH (Hz)\tSPEED (words/minute)")
	for _, voice := range list {
		fmt.Fprintf(table, "%s\t%s\t%d Hz\t%d (%d to %d)\t%d (%d to %d)\n", voice.Name, voice.Gender, voice.SampleRate,
			voice.Pitch.Default, voice.Pitch.Minimum, voice.Pitch.Maximum, voice.Speed.Default, voice.Speed.Minimum, voice.Speed.Maximum)
	}
	if err := table.Flush(); err != nil {
		return fail(err)
	}
	return 0
}
