// check-release unpacks a release archive and checks its programs on the computer it runs on: every voice speaks
// a few texts, sapi4out and sapi4limits run, and the SHA-256 sums of what they give must be the ones in
// speech.sha256. The release workflow runs it on each system the archives are for.
//
//	go run ./scripts/check-release -archive opensapi4-v0.1.0-linux-x64.tar.gz [-write]
package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

type speech struct {
	name      string
	arguments []string
}

var speeches = []speech{
	{name: "plain", arguments: []string{"Hello, this is Microsoft Speech, version four, speaking in 1998."}},
	{name: "tags", arguments: []string{`\Pit=120\ Testing \Spd=220\ one, two, three? \Emp\ Really! \Pau=300\ OK.`}},
	{name: "8 kHz 8-bit", arguments: []string{"-rate", "8000", "-bits", "8", "The quick brown fox jumps over the lazy dog."}},
}

type result struct {
	label string
	sum   string
}

func main() {
	archive := flag.String("archive", "", "the release archive to check")
	sums := flag.String("sums", filepath.Join("scripts", "check-release", "speech.sha256"), "the file of expected sums")
	write := flag.Bool("write", false, "write the sums file instead of checking against it")
	flag.Parse()
	if *archive == "" || flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}
	if err := check(*archive, *sums, *write); err != nil {
		fmt.Fprintln(os.Stderr, "check-release:", err)
		os.Exit(1)
	}
}

func check(archive, sums string, write bool) error {
	folder, err := os.MkdirTemp("", "opensapi4-check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(folder)
	name := filepath.Base(archive)
	if strings.HasSuffix(name, ".zip") {
		err = unzip(archive, folder)
	} else {
		err = untar(archive, folder)
	}
	if err != nil {
		return err
	}
	results, err := run(filepath.Join(folder, strings.TrimSuffix(strings.TrimSuffix(name, ".zip"), ".tar.gz")), folder)
	if err != nil {
		return err
	}
	if write {
		var text strings.Builder
		for _, result := range results {
			fmt.Fprintf(&text, "%s  %s\n", result.sum, result.label)
		}
		if err := os.WriteFile(sums, []byte(text.String()), 0o644); err != nil {
			return err
		}
		fmt.Printf("wrote %d sums to %s\n", len(results), sums)
		return nil
	}
	expected, err := readSums(sums)
	if err != nil {
		return err
	}
	failures := 0
	for index, result := range results {
		if index >= len(expected) || expected[index] != result {
			fmt.Printf("different: %s (%s)\n", result.label, result.sum)
			failures++
		}
	}
	if len(expected) != len(results) {
		return fmt.Errorf("%s gave %d outputs, expected %d", name, len(results), len(expected))
	}
	if failures > 0 {
		return fmt.Errorf("%s: %d of %d outputs are different", name, failures, len(results))
	}
	fmt.Printf("%s: all %d outputs are the expected ones\n", name, len(results))
	return nil
}

func run(programFolder, workFolder string) ([]result, error) {
	program := func(name string) string {
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		return filepath.Join(programFolder, name)
	}
	listed, err := output(program("opensapi4"), "", "voices", "-json")
	if err != nil {
		return nil, err
	}
	var voices []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(listed, &voices); err != nil {
		return nil, err
	}
	var results []result
	for _, voice := range voices {
		for _, speech := range speeches {
			audio, err := output(program("opensapi4"), "", append([]string{"speak", "-voice", voice.Name}, speech.arguments...)...)
			if err != nil {
				return nil, err
			}
			results = append(results, result{label: voice.Name + ": " + speech.name, sum: sum(audio)})
		}
	}
	for _, arguments := range [][]string{nil, {"Sam"}} {
		printed, err := output(program("sapi4limits"), "", arguments...)
		if err != nil {
			return nil, err
		}
		results = append(results, result{label: strings.Join(append([]string{"sapi4limits"}, arguments...), " "), sum: sum(printed)})
	}
	printed, err := output(program("sapi4out"), workFolder, "Sam", "100", "150", "Hello world.")
	if err != nil {
		return nil, err
	}
	audio, err := os.ReadFile(filepath.Join(workFolder, strings.TrimSpace(string(printed))))
	if err != nil {
		return nil, err
	}
	results = append(results, result{label: "sapi4out Sam 100 150", sum: sum(audio)})
	return results, nil
}

func output(program, directory string, arguments ...string) ([]byte, error) {
	command := exec.Command(program, arguments...)
	command.Dir = directory
	command.Env = slices.DeleteFunc(os.Environ(), func(variable string) bool {
		return strings.HasPrefix(strings.ToUpper(variable), "OPENSAPI4_VOICES=")
	})
	var printedErrors bytes.Buffer
	command.Stderr = &printedErrors
	data, err := command.Output()
	description := filepath.Base(program) + " " + strings.Join(arguments, " ")
	if err != nil {
		return nil, fmt.Errorf("%s: %w: %s", description, err, bytes.TrimSpace(printedErrors.Bytes()))
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("%s gave nothing", description)
	}
	return data, nil
}

func sum(data []byte) string {
	value := sha256.Sum256(data)
	return hex.EncodeToString(value[:])
}

func readSums(path string) ([]result, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var results []result
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		value, label, found := strings.Cut(scanner.Text(), "  ")
		if !found {
			return nil, fmt.Errorf("%s: a line without a sum: %q", path, scanner.Text())
		}
		results = append(results, result{label: label, sum: value})
	}
	return results, scanner.Err()
}

func unzip(archive, folder string) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer reader.Close()
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		source, err := file.Open()
		if err != nil {
			return err
		}
		err = extract(folder, file.Name, file.Mode(), source)
		source.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func untar(archive, folder string) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer file.Close()
	decompressed, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	reader := tar.NewReader(decompressed)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		if err := extract(folder, header.Name, header.FileInfo().Mode(), reader); err != nil {
			return err
		}
	}
}

func extract(folder, name string, fileMode os.FileMode, source io.Reader) error {
	if !filepath.IsLocal(name) {
		return fmt.Errorf("the archive has a file outside its folder: %s", name)
	}
	path := filepath.Join(folder, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	destination, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fileMode.Perm()|0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(destination, source); err != nil {
		destination.Close()
		return err
	}
	return destination.Close()
}
