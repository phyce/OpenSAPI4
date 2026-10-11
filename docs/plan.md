# OpenSAPI4: plan

**Goal:** a Go engine that works exactly like the SAPI 4 voices. The same text, voice, pitch and speed give the same audio as Microsoft Sam, Mike, Mary and L&H TruVoice, on Windows, Linux and macOS.

- **Users** install nothing and provide nothing. The voices come as a data pack, downloaded with the program.
- **No Microsoft or L&H executable code** ships: no DLLs, no installers, no emulator running them.
- **Exact:** for the same input the output is the same, sample for sample, and every event (text start and end, word positions, bookmarks, mouth shapes) is at the same sample.

## The voice data

The engine's code is written from the specs in [whistler/](whistler/). The engine's data can't be written: it is the voice. Sam's sound is a real person's recorded speech, and the front end's rules and dictionaries decide every pronunciation. So the data is converted from the original voice files into a format of our own, the data pack ([whistler/datapack.md](whistler/datapack.md)).

| | Ships? |
|---|---|
| Engine code (Go) | Yes, in this repo |
| Data pack | Yes, as its own folder in the release downloads. Never in this repo, never compiled into a binary |
| Microsoft or L&H DLLs, installers, `.vce` files as they are | Never |

## Done: the Microsoft voices

The 19 modes of the Microsoft engine and its three hidden whisper variants ([whistler/interface.md](whistler/interface.md) §2), with everything an application can ask of them through SAPI 4:
- speaking texts with every tag, as plain or phonetic text, in the four character sets;
- the events: text start and end, word positions, bookmarks and `Visual` (mouth shapes);
- pitch, speed and volume, `Inject`, and the attribute interfaces with their ranges and errors;
- the mode information and the enumerator;
- the user lexicon;
- prosody transplants;
- the 8-bit and resampled formats used when the audio destination refuses the voice's own;
- stopping a text (`AudioReset`), pausing and resuming.

Every part is checked against the original engine, on more than 40,000 cases, and is identical in every sample and event. Where the original's own result depends on how its threads are scheduled (where a stop or a pause lands), the Go engine gives its most common result ([whistler/engine.md](whistler/engine.md) §12, §14).

## Next

1. **Command-line tools**, in this repo:
   - *Done:* `sapi4out` and `sapi4limits` (`cmd/`), drop-in replacements for the tools of [TETYYS/SAPI4](https://github.com/TETYYS/SAPI4) that other projects call: the same arguments, the same output, the same WAV files. Compared with the originals on 2,492 command lines (every voice, odd voice names, pitches and speeds in and out of range, quoting, texts with tags and non-ASCII characters): identical, except that the original's voice list also has the TruVoice voices, and one text crashes the original ([whistler/engine.md](whistler/engine.md) §9; the audio it wrote before the crash matches);
   - *Done:* `opensapi4` ([command-line.md](command-line.md)): speaks with flags or JSON requests, lists the voices, and runs as a stream of JSON messages over standard input and output (audio, words, bookmarks, mouth shapes) or as an HTTP server. Package `opensapi4` gives the same requests to Go programs.
2. **Releases** for Windows, Linux and macOS, with the data pack. *Ready:* `scripts/release` builds them (eight systems, plus the data pack on its own), reproducibly, and checks them; the first release is still to be published.
3. **L&H TruVoice:** its ten voices. A separate engine, rule-based, with all its data inside its DLL.
4. **More voices:**
   - new modes of the existing voices, defined as data: a base voice with its own name, pitch, speed and effects (rooms, robot voices, whisper), as the shipped variants are;
   - later, new speakers recorded and built into the same format.
5. **Later, maybe:** the Microsoft concatenated engine (`mstts.dll`).
