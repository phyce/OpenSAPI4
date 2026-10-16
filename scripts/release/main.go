// release builds the release archives: for each platform, the programs with the voice data, and the voice data on
// its own, with a file of their SHA-256 sums. The archives are the same for the same commit and voice data, and the
// voice data's archive for the same voice data, so programs that download it can keep its sum from one release to
// the next.
//
//	go run ./scripts/release -version v0.1.0 -voices <voices folder> [-output dist]
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	opensapi4 "github.com/phyce/OpenSAPI4"
	truvoicedatapack "github.com/phyce/OpenSAPI4/truvoice/datapack"
	"github.com/phyce/OpenSAPI4/whistler/datapack"
)

type platform struct {
	system       string
	architecture string
	arm          string
	name         string
}

var platforms = []platform{
	{system: "windows", architecture: "amd64", name: "windows-x64"},
	{system: "windows", architecture: "386", name: "windows-x86"},
	{system: "windows", architecture: "arm64", name: "windows-arm64"},
	{system: "linux", architecture: "amd64", name: "linux-x64"},
	{system: "linux", architecture: "arm64", name: "linux-arm64"},
	{system: "linux", architecture: "arm", arm: "7", name: "linux-armv7"},
	{system: "darwin", architecture: "amd64", name: "macos-x64"},
	{system: "darwin", architecture: "arm64", name: "macos-arm64"},
}

var programs = []string{"opensapi4", "sapi4out", "sapi4limits"}

// voicesTime is the time of the files in the voice data's archive.
var voicesTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

var versionPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`)

// entry is a file of an archive: its path in the archive, and where its content is.
type entry struct {
	name       string
	source     string
	executable bool
}

func main() {
	version := flag.String("version", "", "the version, such as v0.1.0")
	voices := flag.String("voices", "", "the voices folder to ship, holding the microsoft and truvoice folders")
	output := flag.String("output", "dist", "the folder to write the archives to; it must be empty or new")
	allowChanges := flag.Bool("allow-changes", false, "build from a working tree with changes that aren't committed")
	flag.Parse()
	if err := release(*version, *voices, *output, *allowChanges); err != nil {
		fmt.Fprintln(os.Stderr, "release:", err)
		os.Exit(1)
	}
}

func release(version, voices, output string, allowChanges bool) error {
	if !versionPattern.MatchString(version) {
		return fmt.Errorf("-version must be like v1.2.3, not %q", version)
	}
	if voices == "" {
		return errors.New("-voices is needed: the voices folder to ship")
	}
	changes, err := command("git", "status", "--porcelain")
	if err != nil {
		return err
	}
	if changes != "" && !allowChanges {
		return fmt.Errorf("the working tree has changes; commit them, or give -allow-changes:\n%s", changes)
	}
	committed, err := command("git", "log", "-1", "--format=%ct")
	if err != nil {
		return err
	}
	seconds, err := strconv.ParseInt(committed, 10, 64)
	if err != nil {
		return fmt.Errorf("the commit time %q: %w", committed, err)
	}
	modified := time.Unix(seconds, 0).UTC()
	voiceFiles, err := checkVoices(voices)
	if err != nil {
		return err
	}
	if err := makeEmptyFolder(output); err != nil {
		return err
	}
	builds, err := os.MkdirTemp("", "opensapi4-release-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(builds)

	documents := []entry{{name: "README.md", source: "README.md"}, {name: "command-line.md", source: filepath.Join("docs", "command-line.md")}}
	var archives []string
	for _, target := range platforms {
		folder := "opensapi4-" + version + "-" + target.name
		entries := slices.Clone(documents)
		for _, program := range programs {
			executable := program
			if target.system == "windows" {
				executable += ".exe"
			}
			built := filepath.Join(builds, target.name, executable)
			fmt.Printf("build %s %s\n", target.name, program)
			if err := build(target, version, program, built); err != nil {
				return err
			}
			entries = append(entries, entry{name: executable, source: built, executable: true})
		}
		entries = append(entries, voiceFiles...)
		for index := range entries {
			entries[index].name = folder + "/" + entries[index].name
		}
		archive := folder + ".tar.gz"
		write := writeTarGzip
		if target.system == "windows" {
			archive, write = folder+".zip", writeZip
		}
		if err := write(filepath.Join(output, archive), entries, modified); err != nil {
			return err
		}
		archives = append(archives, archive)
	}
	voicesArchive := "opensapi4-voices-" + version + ".zip"
	if err := writeZip(filepath.Join(output, voicesArchive), voiceFiles, voicesTime); err != nil {
		return err
	}
	archives = append(archives, voicesArchive)
	if err := checkHostArchive(output, version); err != nil {
		return err
	}
	return writeSums(output, archives)
}

func command(name string, arguments ...string) (string, error) {
	run := exec.Command(name, arguments...)
	run.Stderr = os.Stderr
	result, err := run.Output()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w", name, strings.Join(arguments, " "), err)
	}
	return strings.TrimSpace(string(result)), nil
}

// checkVoices checks both packs of the voice data against their manifests' sums, and that every voice speaks, and
// gives their files: each pack's manifest and the files it lists, nothing else.
func checkVoices(folder string) ([]entry, error) {
	microsoft, err := datapack.Open(filepath.Join(folder, opensapi4.MicrosoftFolder))
	if err != nil {
		return nil, err
	}
	truVoice, err := truvoicedatapack.Open(filepath.Join(folder, opensapi4.TruVoiceFolder))
	if err != nil {
		return nil, err
	}
	var files []entry
	for pack, sums := range map[string]map[string]string{opensapi4.MicrosoftFolder: microsoft.Manifest.Files, opensapi4.TruVoiceFolder: truVoice.Manifest.Files} {
		packFolder := filepath.Join(folder, pack)
		files = append(files, entry{name: "voices/" + pack + "/manifest.json", source: filepath.Join(packFolder, "manifest.json")})
		for name, sum := range sums {
			if !filepath.IsLocal(filepath.FromSlash(name)) || path.Clean(name) != name {
				return nil, fmt.Errorf("the %s voice data's manifest lists %q", pack, name)
			}
			source := filepath.Join(packFolder, filepath.FromSlash(name))
			content, err := os.ReadFile(source)
			if err != nil {
				return nil, err
			}
			if digest := sha256.Sum256(content); hex.EncodeToString(digest[:]) != sum {
				return nil, fmt.Errorf("the %s voice data's %s doesn't match its manifest", pack, name)
			}
			files = append(files, entry{name: "voices/" + pack + "/" + name, source: source})
		}
	}
	slices.SortFunc(files, func(first, second entry) int { return strings.Compare(first.name, second.name) })
	voices, err := opensapi4.Open(folder)
	if err != nil {
		return nil, err
	}
	list, err := voices.List()
	if err != nil {
		return nil, err
	}
	for _, voice := range list {
		speech, err := voices.NewSpeech(opensapi4.Request{Voice: voice.Name, Text: "Testing, one, two, three."})
		if err != nil {
			return nil, err
		}
		counter := &audioCounter{}
		if err := speech.Speak(counter); err != nil || counter.bytes == 0 {
			return nil, fmt.Errorf("voice %s gives no audio: %v", voice.Name, err)
		}
	}
	fmt.Printf("voice data: %d files, %d voices\n", len(files), len(list))
	return files, nil
}

func makeEmptyFolder(folder string) error {
	existing, err := os.ReadDir(folder)
	if err == nil && len(existing) > 0 {
		return fmt.Errorf("the output folder %s isn't empty", folder)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.MkdirAll(folder, 0o755)
}

func build(target platform, version, program, built string) error {
	run := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w -X main.version="+version, "-o", built, "./cmd/"+program)
	run.Env = append(os.Environ(), "GOOS="+target.system, "GOARCH="+target.architecture, "GOARM="+target.arm, "CGO_ENABLED=0")
	run.Stdout, run.Stderr = os.Stdout, os.Stderr
	if err := run.Run(); err != nil {
		return fmt.Errorf("build %s for %s: %w", program, target.name, err)
	}
	return nil
}

func mode(file entry) os.FileMode {
	if file.executable {
		return 0o755
	}
	return 0o644
}

func writeZip(archive string, entries []entry, modified time.Time) error {
	return writeArchive(archive, func(output io.Writer) error {
		writer := zip.NewWriter(output)
		for _, file := range entries {
			header := &zip.FileHeader{Name: file.name, Method: zip.Deflate, Modified: modified}
			header.SetMode(mode(file))
			destination, err := writer.CreateHeader(header)
			if err != nil {
				return err
			}
			if err := copyFile(destination, file.source); err != nil {
				return err
			}
		}
		return writer.Close()
	})
}

func writeTarGzip(archive string, entries []entry, modified time.Time) error {
	return writeArchive(archive, func(output io.Writer) error {
		compressor, err := gzip.NewWriterLevel(output, gzip.BestCompression)
		if err != nil {
			return err
		}
		writer := tar.NewWriter(compressor)
		for _, file := range entries {
			information, err := os.Stat(file.source)
			if err != nil {
				return err
			}
			header := &tar.Header{Name: file.name, Mode: int64(mode(file)), Size: information.Size(), ModTime: modified, Typeflag: tar.TypeReg,
				Format: tar.FormatPAX}
			if err := writer.WriteHeader(header); err != nil {
				return err
			}
			if err := copyFile(writer, file.source); err != nil {
				return err
			}
		}
		if err := writer.Close(); err != nil {
			return err
		}
		return compressor.Close()
	})
}

func writeArchive(archive string, write func(output io.Writer) error) error {
	file, err := os.Create(archive)
	if err != nil {
		return err
	}
	err = write(file)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write %s: %w", archive, err)
	}
	fmt.Println("wrote", archive)
	return nil
}

func copyFile(destination io.Writer, source string) error {
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(destination, file)
	return err
}

// checkHostArchive unpacks the archive for this computer, if there is one, and speaks with its programs, which
// must find the voices next to them: Sam with opensapi4 and sapi4out, which must give the same file, and Peter.
func checkHostArchive(output, version string) error {
	var host *platform
	for index := range platforms {
		if platforms[index].system == runtime.GOOS && platforms[index].architecture == runtime.GOARCH {
			host = &platforms[index]
		}
	}
	if host == nil {
		fmt.Println("no archive for this computer to check")
		return nil
	}
	folder, err := os.MkdirTemp("", "opensapi4-check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(folder)
	name := "opensapi4-" + version + "-" + host.name
	if host.system == "windows" {
		err = unzip(filepath.Join(output, name+".zip"), folder)
	} else {
		err = untar(filepath.Join(output, name+".tar.gz"), folder)
	}
	if err != nil {
		return err
	}
	programFolder := filepath.Join(folder, name)
	extension := ""
	if host.system == "windows" {
		extension = ".exe"
	}
	speak := exec.Command(filepath.Join(programFolder, "opensapi4"+extension), "speak", "-voice", "Sam", "-output", filepath.Join(folder, "check.wav"),
		"Testing, one, two, three.")
	speakTruVoice := exec.Command(filepath.Join(programFolder, "opensapi4"+extension), "speak", "-voice", "Peter", "-output",
		filepath.Join(folder, "truvoice.wav"), "Testing, one, two, three.")
	compatible := exec.Command(filepath.Join(programFolder, "sapi4out"+extension), "Sam", "100", "150", "Testing, one, two, three.")
	compatible.Dir = folder
	var printed bytes.Buffer
	compatible.Stdout = &printed
	for _, run := range []*exec.Cmd{speak, speakTruVoice, compatible} {
		run.Env = withoutVoicesVariable(os.Environ())
		run.Stderr = os.Stderr
		if err := run.Run(); err != nil {
			return fmt.Errorf("check %s: %w", run.Path, err)
		}
	}
	first, err := os.ReadFile(filepath.Join(folder, "check.wav"))
	if err != nil {
		return err
	}
	second, err := os.ReadFile(filepath.Join(folder, strings.TrimSpace(printed.String())))
	if err != nil {
		return err
	}
	if len(first) < 10000 || !bytes.Equal(first, second) {
		return fmt.Errorf("check %s: opensapi4 gave %d bytes and sapi4out %d, expected the same", name, len(first), len(second))
	}
	truVoice, err := os.ReadFile(filepath.Join(folder, "truvoice.wav"))
	if err != nil {
		return err
	}
	if len(truVoice) < 10000 {
		return fmt.Errorf("check %s: Peter gave %d bytes", name, len(truVoice))
	}
	fmt.Printf("checked %s: %d bytes of audio from Sam, %d from Peter\n", name, len(first), len(truVoice))
	return nil
}

func withoutVoicesVariable(environment []string) []string {
	return slices.DeleteFunc(slices.Clone(environment), func(variable string) bool {
		return strings.HasPrefix(strings.ToUpper(variable), "OPENSAPI4_VOICES=")
	})
}

func unzip(archive, folder string) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer reader.Close()
	for _, file := range reader.File {
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
	decompressor, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	reader := tar.NewReader(decompressor)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := extract(folder, header.Name, header.FileInfo().Mode(), reader); err != nil {
			return err
		}
	}
}

func extract(folder, name string, fileMode os.FileMode, source io.Reader) error {
	if !filepath.IsLocal(filepath.FromSlash(name)) {
		return fmt.Errorf("the archive has %q", name)
	}
	destination := filepath.Join(folder, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fileMode.Perm())
	if err != nil {
		return err
	}
	_, err = io.Copy(file, source)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}

func writeSums(output string, archives []string) error {
	var sums strings.Builder
	slices.Sort(archives)
	for _, archive := range archives {
		content, err := os.ReadFile(filepath.Join(output, archive))
		if err != nil {
			return err
		}
		digest := sha256.Sum256(content)
		fmt.Fprintf(&sums, "%s  %s\n", hex.EncodeToString(digest[:]), archive)
	}
	if err := os.WriteFile(filepath.Join(output, "SHA256SUMS"), []byte(sums.String()), 0o644); err != nil {
		return err
	}
	fmt.Print("SHA256SUMS:\n", sums.String())
	return nil
}

type audioCounter struct {
	bytes int
}

func (counter *audioCounter) Audio(data []byte) (int, error) {
	counter.bytes += len(data)
	return len(data), nil
}

func (counter *audioCounter) Event(opensapi4.Event) error {
	return nil
}
