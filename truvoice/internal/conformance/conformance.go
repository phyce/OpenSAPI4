// Package conformance finds the TruVoice golden data and data pack for the tests: the folder
// OPENSAPI4_CONFORMANCE names. Tests skip without it.
package conformance

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/phyce/OpenSAPI4/truvoice/datapack"
)

const EnvironmentVariable = "OPENSAPI4_CONFORMANCE"

func Path(t testing.TB, elements ...string) string {
	t.Helper()
	rootPath := os.Getenv(EnvironmentVariable)
	if rootPath == "" {
		t.Skip(EnvironmentVariable + " is not set")
	}
	return filepath.Join(append([]string{rootPath}, elements...)...)
}

func Pack(t testing.TB) *datapack.Pack {
	t.Helper()
	pack, err := datapack.Open(Path(t, "datapack", "truvoice"))
	if err != nil {
		t.Fatal(err)
	}
	return pack
}

// ReadLines reads a gzip-compressed file of JSON lines from the golden folder.
func ReadLines[T any](t testing.TB, elements ...string) []T {
	t.Helper()
	file, err := os.Open(Path(t, append([]string{"golden", "truvoice"}, elements...)...))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(nil, 1<<28)
	var lines []T
	for scanner.Scan() {
		var line T
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return lines
}
