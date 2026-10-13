# OpenSAPI4

An open-source implementation of the Microsoft SAPI 4 voices (Microsoft Sam, Mike, Mary and all their variants), written in pure Go. The same text and settings give the same audio as the original engine, sample for sample, on Windows, Linux and macOS.

## Why

The SAPI 4 voices are from the late 1990s. They're robotic, they mispronounce things, and a lot of people love them. They're in countless videos, games, memes and old Windows memories.

Running them today is hard. You need 25-year-old 32-bit installers on Windows, or Wine everywhere else, and both get harder to keep working every year. OpenSAPI4 is a preservation project. It keeps the voices alive as a small native program that needs nothing installed and sounds exactly like the original, quirks included.

## Status

| | |
|---|---|
| Microsoft voices | Done: all 19 voices, every tag, event and setting |
| Command-line tools, HTTP and stream modes | Done |
| Releases for Windows, Linux and macOS | Done: [v0.1.0](https://github.com/phyce/OpenSAPI4/releases/tag/v0.1.0) |
| L&H TruVoice voices | In progress |
| AT&T FlexTalk voices | Planned |

The engine is checked against the original on more than 40,000 cases, and it's identical in every sample and event. Details are in [docs/plan.md](docs/plan.md).

## Voices

| Base voice | Voices |
|---|---|
| Mary | Mary, Mary (for Telephone), Mary in Hall, Mary in Stadium, Mary in Space, Female Whisper, RoboSoft Four, RoboSoft Five, RoboSoft Six |
| Mike | Mike, Mike (for Telephone), Mike in Hall, Mike in Stadium, Mike in Space, Male Whisper, RoboSoft One, RoboSoft Two, RoboSoft Three |
| Sam | Sam |

The telephone voices speak at 8000 Hz; the rest at 22050 Hz. `\Chr="Whisper"\` in a text makes any of them whisper, Sam included.

## Download

Get the archive for your system from the [releases](https://github.com/phyce/OpenSAPI4/releases), unpack it, and run the programs from its folder. They need nothing installed; keep the `voices` folder next to them.

| System | Archive |
|---|---|
| Windows | `windows-x64`, `windows-x86` (32-bit), `windows-arm64` |
| Linux | `linux-x64`, `linux-arm64`, `linux-armv7` (such as a Raspberry Pi with a 32-bit system) |
| macOS | `macos-arm64` (Apple silicon), `macos-x64` (Intel) |

The programs aren't signed. On macOS, allow them once with `xattr -dr com.apple.quarantine <the unpacked folder>`. On Windows, SmartScreen may ask the first time.

`opensapi4-voices` has only the voice data, for programs that use the Go package.

## Command line

```sh
opensapi4 speak -voice Sam -output hello.wav "Hello, I am Microsoft Sam."
opensapi4 voices
opensapi4 serve
```

- `opensapi4 speak` speaks a text into a WAV file, with flags or a JSON request.
- `opensapi4 stream` speaks JSON requests from standard input, one a line, and sends the audio and events (words, bookmarks, mouth shapes) as JSON messages on standard output.
- `opensapi4 serve` speaks over HTTP: `GET /voices`, `GET` or `POST /speak`, and a page to try the voices at `http://127.0.0.1:5040`.
- `sapi4out` and `sapi4limits` are drop-in replacements for the tools of [TETYYS/SAPI4](https://github.com/TETYYS/SAPI4): the same arguments, the same output, the same WAV files.

Everything is in [docs/command-line.md](docs/command-line.md).

## The voice data

The engine needs the voices' data: the recorded speech, the pronunciation rules and the dictionaries. It reads them from a data pack converted from the original voice files ([docs/whistler/datapack.md](docs/whistler/datapack.md)). The data pack isn't in this repository and is never compiled into the program; it comes with the release downloads as the `voices` folder.

## Using it from Go

```go
voices, err := opensapi4.Open("voices")
if err != nil {
	return err
}
speech, err := voices.NewSpeech(opensapi4.Request{Voice: "Sam", Text: `Hello, \Pit=150\ world.`})
if err != nil {
	return err
}
// sink implements whistler.Sink: it gets the PCM (speech.Format()) and the events in order.
err = speech.Speak(sink)
```

Package `github.com/phyce/OpenSAPI4` lists the voices and speaks requests as the command line takes them. For everything else (the user lexicon, pausing, attribute events, the format negotiation), use the engine directly:

```go
pack, err := datapack.Open("voices/microsoft")
if err != nil {
	return err
}
data, err := whistler.Load(pack)
if err != nil {
	return err
}
mode, _ := data.Mode("Sam")
engine, err := data.NewEngine(mode)
if err != nil {
	return err
}
err = engine.SpeakString(`Hello, \Pit=150\ world.`, sink)
```

## Layout

| Path | Contents |
|---|---|
| `docs/command-line.md` | The command-line tools, the stream and the HTTP server |
| `docs/whistler/` | How the Microsoft engine behaves, and the data pack format |
| `docs/truvoice/` | How the TruVoice engine behaves, and its data pack format (in progress) |
| `cmd/opensapi4`, `cmd/sapi4out`, `cmd/sapi4limits` | The command-line tools |
| `scripts/release` | Builds the release archives |
| `scripts/check-release` | Runs a release archive's programs and checks what they give |
| `opensapi4.go` | Package `opensapi4`: the voices of a voices folder, and requests |
| `whistler` | The engine: speaks texts with one voice, joining the stages below |
| `whistler/datapack` | Reads and checks the data pack |
| `whistler/text` | The preprocessing: character sets, tags and the control table, line breaks, special tokens, sentences and segments |
| `whistler/analyser` | The sentence reader, and a sentence's analyser state: transcription and analysis by the rule modules |
| `whistler/rules` | Runs the compiled rewrite rules |
| `whistler/pronunciation` | Dictionary lookups, morphology and the lexicons' ARPAbet conversion |
| `whistler/lexicon` | The user lexicon |
| `whistler/prosody` | Turns the analyser state into durations, pitch targets and unit records |
| `whistler/posting` | Writes each sentence's unit records, prosody transplants included |
| `whistler/units` | Turns unit records into frames, controls and settings |
| `whistler/synthesis` | Turns frames into samples: 16- or 8-bit, at the voice's rate or resampled |
| `whistler/effects` | Whisper, the post-filter, the reverb, `\Chr=` and `\Rst\` switching |
| `truvoice/datapack` | Reads and checks the TruVoice data pack |
| `truvoice/synthesizer` | TruVoice's formant synthesizer: parameter frames to coefficient blocks to samples, its frame ring, steps and audio buffer |
| `truvoice/pipeline` | TruVoice's pipeline: the nodes its stages pass on, the stages' cursors, the text's control commands, the output stage, the scheduler and the speaking thread's start, attributes and hand-overs |
| `truvoice/parameters` | TruVoice's parameter generator (in progress): its tracks and the segments it renders into the frame ring, the phonemes' targets, the rules, the per-phoneme rules, the actions that set the onsets and shapes, the fricatives' amplitudes and the stops' closures, the vowels' formants with the vowel database, the loci and final adjustments that finish a phoneme, the consonants' targets and transitions with the liquids, the pitch's small movements, and the rendering of a phoneme |

## Building releases

```sh
go run ./scripts/release -version v0.1.0 -voices <voices folder>
```

Builds the programs for every system in the table above, packs each with the voice data, and writes the archives and their `SHA256SUMS` to `dist/`. The voice data isn't in this repository, so releases are built where it is. The script checks the data against its manifest, speaks with every voice, and runs the programs of the archive for the computer it runs on. The same commit and data always give the same archives.

When a release is published, the Release workflow downloads its archives on Windows (x64, x86, arm64), Linux (x64, arm64) and macOS (Apple silicon, Intel), and checks each with `scripts/check-release`: every voice speaks three texts, `sapi4out` and `sapi4limits` run, and what they give must match the sums in `scripts/check-release/speech.sha256`. To check an archive by hand:

```sh
go run ./scripts/check-release -archive dist/opensapi4-v0.1.0-linux-x64.tar.gz
```

## Tests

```sh
OPENSAPI4_CONFORMANCE=<conformance data folder> go test -count=1 ./...
```

The conformance tests compare every stage with the original engine's output. Their data and the data pack aren't in this repository; without `OPENSAPI4_CONFORMANCE` only the tests on synthetic data run.

## Trademarks

OpenSAPI4 is an independent project. It is not affiliated with, endorsed by or sponsored by Microsoft. Microsoft and Windows are trademarks of Microsoft Corporation, and TruVoice, AT&T and FlexTalk are trademarks of their owners. The names are used only to say which voices this project is compatible with.
