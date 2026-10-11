# OpenSAPI4

An open-source implementation of the Microsoft SAPI 4 voices (Microsoft Sam, Mike, Mary and all their variants), written in pure Go. The same text and settings give the same audio as the original engine, sample for sample, on Windows, Linux and macOS.

## Why

The SAPI 4 voices are from the late 1990s. They're robotic, they mispronounce things, and a lot of people love them. They're in countless videos, games, memes and old Windows memories.

Running them today is hard. You need 25-year-old 32-bit installers on Windows, or Wine everywhere else, and both get harder to keep working every year. OpenSAPI4 is a preservation project. It keeps the voices alive as a small native program that needs nothing installed and sounds exactly like the original, quirks included.

## Status

| | |
|---|---|
| Microsoft voices | Done: all 19 voices, every tag, event and setting |
| Command-line tools, HTTP and stream modes | In progress |
| Releases for Windows, Linux and macOS | In progress |
| L&H TruVoice voices | Planned |

The engine is checked against the original on more than 40,000 cases, and it's identical in every sample and event. Details are in [docs/plan.md](docs/plan.md).

## Voices

| Base voice | Voices |
|---|---|
| Mary | Mary, Mary (for Telephone), Mary in Hall, Mary in Stadium, Mary in Space, Female Whisper, RoboSoft Four, RoboSoft Five, RoboSoft Six |
| Mike | Mike, Mike (for Telephone), Mike in Hall, Mike in Stadium, Mike in Space, Male Whisper, RoboSoft One, RoboSoft Two, RoboSoft Three |
| Sam | Sam |

The telephone voices speak at 8000 Hz; the rest at 22050 Hz. `\Chr="Whisper"\` in a text makes any of them whisper, Sam included.

## The voice data

The engine needs the voices' data: the recorded speech, the pronunciation rules and the dictionaries. It reads them from a data pack converted from the original voice files ([docs/whistler/datapack.md](docs/whistler/datapack.md)). The data pack isn't in this repository and is never compiled into the program; it will come with the release downloads as a folder of its own.

## Using it from Go

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
// sink implements whistler.Sink: it gets the PCM (engine.Format()) and the events in order.
err = engine.SpeakString(`Hello, \Pit=150\ world.`, sink)
```

Packages: `github.com/phyce/OpenSAPI4/whistler` and `github.com/phyce/OpenSAPI4/whistler/datapack`.

## Layout

| Path | Contents |
|---|---|
| `docs/whistler/` | How the Microsoft engine behaves, and the data pack format |
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

## Tests

```sh
OPENSAPI4_CONFORMANCE=<conformance data folder> go test -count=1 ./...
```

The conformance tests compare every stage with the original engine's output. Their data and the data pack aren't in this repository; without `OPENSAPI4_CONFORMANCE` only the tests on synthetic data run.

## Trademarks

OpenSAPI4 is an independent project. It is not affiliated with, endorsed by or sponsored by Microsoft. Microsoft and Windows are trademarks of Microsoft Corporation, and TruVoice is a trademark of its owner. The names are used only to say which voices this project is compatible with.
