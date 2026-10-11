# SAPI 4 Microsoft engine: the engine

How the Microsoft Speech Synthesis Engine (`msttssyn.dll`) joins its stages for one selected mode: what happens to each `TextData` call, how the front end walks the segments and sentences of a text, which tags travel inside a sentence's units and which are sent on their own, and where every event lands in the audio. The stages themselves are in [text.md](text.md) (preprocessing and the reader), [rules.md](rules.md) and [pronunciation.md](pronunciation.md) (transcription), [prosody.md](prosody.md) (analysis and the unit builder), [units.md](units.md) (frames), [synthesis.md](synthesis.md) and [effects.md](effects.md) (samples). The interface (modes, attributes, `Inject`, sinks, audio control) is in [interface.md](interface.md); what `AudioReset` does to a text being spoken is in section 12, and pausing in section 14.

## 1. Stages

| Stage | Takes | Gives | Keeps |
|---|---|---|---|
| Front end | texts | unit batches and standalone controls | pitch (current and default), the monotone flag, the address context, the reader's values, the rule modules' registers, the duration style; per text: the control table and "one character" |
| Voice | unit batches and controls | frames and controls | speed, volume |
| Synthesiser | frames and controls | samples and events | filter memory, noise position, effects, reverb, `rand` |

In the original each stage is a thread with a queue, and every message keeps its order through all of them. No stage reads another's state except through messages, so running the stages one after another for each message gives the same audio and the same events. The only exception is `AttribChanged` (section 8).

## 2. A `TextData` call

1. **`TextDataStarted`** is placed at the current audio position.
2. **Preprocessing** (text.md §2–7) with the address context the previous call left (text.md §3.2). The control table and the "one character" flag (the converted text is exactly one byte) belong to this call.
3. **Segments.** The final text is cut at every `ESC I<n>` (text.md §7). The first segment is read as it is. Every later one starts with a pitch change (section 3).
4. Each segment is **read** (section 4).
5. **`TextDataDone`** (flags 0) is placed at the audio position after the call's last sample.

The next call starts where this one ended: speed, volume, pitch, the monotone flag, the context, the effects and the synthesiser's state all carry on.

## 3. Pitch at a segment start

The digits after `ESC I`:
- **`0`:** the mode's default pitch. `AttribChanged(pitch)` is sent.
- **Anything else:** the digits are read as a 32-bit signed integer that wraps (the original's `atoi` of the number written with `%u`). Its low 16 bits are set as the pitch, by the same rules as `PitchSet` (interface.md §5):
  - 0 gives the minimum, half the default rounded down;
  - 65535 gives the maximum, twice the default;
  - any other value outside [minimum, maximum] is ignored, and no event is sent;
  - otherwise the value is set and `AttribChanged(pitch)` is sent.

  So `\Pit=65536\` and `\Pit=2147483648\` set the minimum, and `\Pit=4294967295\` the maximum.

The new pitch applies from the next sentence on: the analyser's pitch steps (prosody.md §10.1), and the monotone pitch (section 6).

## 4. Reading a segment

The reader works on the segment followed by `0x1A` (text.md §8). The position p starts at 0.

1. **Read** a sentence at p. Its **span** is p to p + consumed − 1. When the reader finds no sentence, the consumed length is the whole rest, `0x1A` included.
2. **Check** the span (section 5). It either goes on or ends the segment.
3. **Speak the sentence:** transcribe, analyse and build units (prosody.md §10), then post the batch (section 6). If the analysis fails (module A overflow, or nothing but pauses), nothing is posted, and the sentence's tags are lost.
4. **Move on:** p += consumed. The reader leaves the byte that ended the sentence (normally the CR) unread, so the next span starts with it. Read at p. If p has reached the end of the segment, the segment ends; the read still happens, and what it does to the reader's values counts. Otherwise check the span (step 2).

## 5. The span check

Walk the span, cut at the end of the segment:
- space and CR are skipped;
- `ESC` takes its command letter and everything up to the next space, that space included. An `ESC N<k>` counts one tag of the span, and the first one's k is the span's **first tag**;
- any other byte is a **word**. LF is a word too, so every span that starts at the previous sentence's CR LF has one.

**Goes on** if the span has a word and the read found a sentence.

**Ends the segment** otherwise. Then every `ESC N<k>` from the start of the span to the end of the segment is sent as a standalone control (section 7), and nothing else of the segment is read.

Only a segment's first span can have no word while its read found a sentence: a first sentence made only of markers. `\Emp\` or `\Prt=` followed by a paragraph break (so the sentence breaker cuts right after the marker) does that. The rest of the segment is then silent: its word positions and bookmarks all fire at once, its pauses still play, and its speed, volume and character changes still apply. This continues up to the next `\Pit=` or `\Rst\`, or the end of the text. The original confirms it: `\Emp\` + CR LF CR LF + `Hello world. Second sentence.` gives no audio and four word positions at sample 0.

## 6. Posting a sentence

The units are built with "no final silence" when the call's text is one character.

Walk the units in order:
- **`#`, `SIL` and phones** are copied. While the monotone flag is on, every point's pitch becomes the current pitch in Hz, as `float32` of the integer. The flag is tested at each unit, so a `\Chr="Monotone"\` tag takes effect from the unit after it.
- **A tag unit `\<k>`** writes control k (text.md §3.4):

  | Control | Records | Also |
  |---|---|---|
  | Chr | `\Chr=<name>\` | `"Normal"`: the monotone flag goes back to the mode's own. `"Monotone"`: the flag goes on. Other names leave it |
  | word | `\Wrd=<offset>\` | |
  | Mrk | `\Mrk=<n>\` | |
  | Pau | `#`, `\Pau=<n>\`, `#` | |
  | Rst | `\RST\` | the monotone flag goes back to the mode's own |
  | Spd, Vol | `\Spd=<n>\`, `\Vol=<n>\` | |
  | Pit | nothing | |
  | Btp, Etp, Btp text | a prosody transplant's records | section 13 |

  While a prosody transplant is on, the units and every tag but Btp, Etp and Btp text are dropped instead (section 13.2).

- **Unplaced tags.** After the last unit, the span's tags that the builder didn't place are added. Every tag unit written counts as placed. While fewer than the span's tag count are placed, the next one is numbered one after the last placed tag (or the span's first tag if none was placed) and written as above.

  The numbering ignores which tags were really skipped. When the reader got a marker twice (an unknown `\Prt=` value repeats the last escape, text.md §3.3), the builder places one, and the count asks for one more: the next number, which may belong to a later sentence or lie past the end of the table (section 9).

The batch goes to the voice. The voice applies `\Spd=` and `\Vol=` where they stand and resets speed and volume at `\RST\` (volume first, then speed). It sends the other tags on between the frames of the words around them (units.md §10).

## 7. Standalone controls

Each control from a span check (section 5) is sent on its own, in text order:

| Control | Message | Also |
|---|---|---|
| Mrk | bookmark n | |
| word | word position (offset) | |
| Pau | pause n ms | |
| Chr | character name | monotone flag as in section 6 |
| Rst | reset | monotone flag back to the mode's own |
| Spd, Vol | speed n, volume n | |
| Pit | pitch: reaches no stage that uses it | |
| Btp, Etp, Btp text | nothing | |

The voice applies speed and volume (an out-of-range speed is ignored, with no event). At a reset it resets the speed first, then the volume, and passes the reset on. The synthesiser plays pauses as silent frames, applies character changes and resets to its effects (effects.md §7), and passes word positions and bookmarks on.

## 8. Events

Positions are in samples of the mode's output, in the format the destination accepted (interface.md §4), counted from its first `TextData` call. Each event is placed where its message stands in the stream, so it fires when playback reaches it.

| Event | Position | Values |
|---|---|---|
| `TextDataStarted` | the call's first sample | the call |
| `Visual` | the first sample of every frame with samples (units.md §11) | IPA, engine phoneme, mouth |
| `WordPosition` | all samples before it | the call, the word's byte offset (text.md §3.2) |
| `BookMark` | all samples before it | the call, n |
| `TextDataDone` | after the call's last sample | the call, flags 0 |

Several events at one position come in stream order: for example a bookmark, then a word position, then the next word's `Visual`.

**`AttribChanged`** isn't tied to the audio.
- **Pitch:** sent by the front end as it reads: a successful `PitchSet`, a successful pitch at a segment start, every reset to the default.
- **Speed:** sent by the voice in stream order, for every successful set and every reset.
- **Volume:** sent by the voice in stream order, for every set, including resets.

Pitch and speed or volume come from different threads in the original, so their mutual order depends on timing. The checks compare the two sequences separately. The Go engine sends each when its stage reaches it.

## 9. Quirks

**Reproduce these.** Each one changes the audio or the events.
1. A first span with no word but a sentence read ends the segment (section 5).
2. A sentence whose analysis fails loses its tags: no word positions, bookmarks, pauses or changes from its span.
3. Unplaced tags are numbered on from the last placed one (section 6).
4. A word whose `ESC N` lands in the previous sentence's span has its tag there. If that sentence has no units, the word position is lost: `\Emp\. Hello world.` sends one word position, for "world".
5. `\Pit=` numbers wrap as described in section 3.

**Don't reproduce.** The original crashes, or reads or writes memory it doesn't own.
- **A tag number past the control table** (section 6). The original reads past the table and usually crashes. The Go engine writes nothing for it.
- **A full unit buffer.** The original makes room for the units plus two records per tag of the span. Each unplaced pause needs three records, so a sentence that ends in a pause, with fewer word tags than pauses to spare, writes past the buffer (`No.\Pau=300\`: 10 units and one pause need 13 records, room for 12). That usually goes unnoticed and the audio is right; sometimes it corrupts the heap. The Go engine has no limit.
- **Very long pauses.** The synthesiser allocates a frame's whole output at once, so `\Pau=4294967295\` (49 days) crashes the original. A 10-minute pause plays correctly. The Go engine doesn't allocate a pause at once (section 10).
- **A `\Chr=` name of 17 characters or more, quotes included, sent as a standalone control.** It overflows a 20-byte stack buffer in the original, which crashes. The Go engine sends it.

Prosody transplants have faults of their own (section 13.7).

## 10. For the Go engine

- One engine per selected mode, holding the front end, the voice and the synthesiser. Process-wide state in the original (the duration style, the rule modules' registers) belongs to the engine instance in Go; one instance per voice gives what one engine per process gives.
- Each stage has its queue, as in the original, and waits where section 12.2 says. A stage that posts runs the stages after it at once, so they take each message in turn and pass every wait until the destination stops taking samples (section 12.3). Samples and events go to the caller as they are made, so a long pause or text never has to be held at once. A silence frame comes in pieces of at most 16384 samples, and its length isn't limited to 32 bits.
- The faults of section 9 are handled as described there, and `AudioReset` as section 12 describes.

The package is `whistler`:
- `Load` reads what all engines of a data pack share: the modes, the prosody tables, the rule modules, the dictionaries and the text tables. Each voice is loaded once, when an engine first needs it.
- `Data.Enumerate` goes through the mode list as `ITTSEnum` does (`Next`, `Skip`, `Reset`, `Clone`; interface.md §3), each mode as a `ModeInfo`, its `TTSMODEINFO`. `Data.Select` makes an engine for the first mode with a mode ID: `NullModeID` selects the first hidden mode, and an ID that isn't listed gives `ErrInvalidMode` (`TTSERR_INVALIDMODE`). `Engine.ModeInfo` is `ModeGet`.
- `Data.Mode` finds a visible mode by its name, and `Data.NewEngine` makes an engine with it. It refuses a hidden mode.
- **The output format** (interface.md §4). `Select` and `NewEngine` take the voice's own format. `SelectAccepting` and `NewEngineAccepting` take the audio destination's answer to each format, an `AcceptFormat` function that returns nil for a format it takes, as `WaveFormatSet` does. The engine offers the formats in the original's order and takes the first one accepted. When none is, it fails with the error the function gave for the last. `Engine.Format` is the format taken, as a rate and a sample width.
- `Engine.Speak` is one `TextData` call: UTF-16 text, tagged or not, in a character set of text.md §2. Any other character set gives `ErrInvalidCharacter` (`TTSERR_INVALIDCHAR`) and changes nothing. It returns when the sink has everything: the audio in pieces, as PCM in the engine's format (16-bit little-endian or 8-bit unsigned samples), and every event of section 8 at its place in it. The sink's `Audio` says how many of the bytes it took. An error from the sink stops the text as section 12.3 says, and `Speak` returns it; `ErrPaused` from the sink pauses the engine (section 14.5). `SpeakString` speaks a Go string with tags.
- `Pitch`, `Speed`, `Volume`, `SetPitch`, `SetSpeed` and `SetVolume` are the `ITTSAttributes` methods (interface.md §5); `Volume` reports both halves, as `VolumeGet` does. `Attribute` and `SetAttribute` are `IAttributes::DWORDGet` and `DWORDSet`, for pitch, speed, volume and thread priority. A pitch or speed out of range gives `ErrValueOutOfRange` (`TTSERR_VALUEOUTOFRANGE`), any other attribute or an unknown thread priority `ErrInvalidParameter` (`TTSERR_INVALIDPARAM`). Real time, the string and memory attributes and `Phoneme` have no Go method: the original refuses them all, and a COM layer can return its codes itself.
- `Inject` is `ITTSCentral::Inject` (interface.md §7), for a Go string.
- `Data.Lexicon` is the user lexicon (package `lexicon`, pronunciation.md §6.3), which every engine of the data looks words up in: `Add`, `AddIPA`, `Remove`, `Pronunciation`, `PronunciationIPA`, and `Read` and `Write` for the original's file. `Engine.Pronounce` and `PronounceIPA` are `ILexPronounce::Get` (pronunciation.md §6.5) and, like `Speak`, change the front end's state.
- `AttributeChanged`, when set, is told of every change of pitch, speed or volume: from those methods, from `Inject`, and from the texts when each stage reaches it.
- An engine is not safe for concurrent use. The attribute methods and `Inject` act between texts as the original's do on an idle engine. Called from the sink while a text is spoken, a pitch applies from the next sentence whose units the front end writes, and a speed or volume from the next word the voice builds; in the original the place depends on how far its threads have run ahead (section 14.2). During a pause they apply where each stage waits, as in the original (section 14.5).
- **Left to a COM layer.** These parts of interface.md don't touch the engine's state, so the Go engine has no equivalent, and a COM layer around it does them itself:
  - checking pointers, windows and interface IDs (`E_INVALIDARG`, `E_NOINTERFACE`);
  - the ANSI interfaces' conversions, and their `E_FAIL` for an empty text (interface.md §6);
  - `Register` and `UnRegister`, with their process-wide keys (interface.md §8). The layer passes `AttributeChanged` and each `Visual` event to every sink it holds, with the IPA character 0 for an ANSI sink;
  - the device: claiming and releasing it, its `AudioStart` and `AudioStop`, `PosnGet` and `ToFileTime`;
  - the dialogs (interface.md §12);
  - the calls the original refuses: real time, the string and memory attributes, `Phoneme`.
- `Pause`, `Resume` and `Reset` are `AudioPause`, `AudioResume` and `AudioReset` (section 14.5), with `ErrAlreadyPaused` (`TTSERR_ALREADYPAUSED`) and `ErrNotPaused` (`TTSERR_NOTPAUSED`).

## 11. Checked

This spec is checked against the original engine through its SAPI 4 interface, with a fresh engine per case and one `TextData` call per text. The check compares the PCM, every positional event with its sample, and the two `AttribChanged` sequences. 3,322 of 3,357 cases are identical:
- the 69 corpus sentences on the five base voices;
- 40 multi-sentence texts on all 19 visible modes;
- 53 tag texts on 9 modes (every tag alone, at the edges, between sentences and inside words, wrapping numbers);
- the preprocessing's hand cases, tagged and untagged;
- IPA input, tagged and untagged;
- 10 series of texts on 4 modes (context, monotone, speed, volume, pitch and effects carried from text to text);
- 11 attribute settings on 4 modes;
- 16 texts that meet the faults of section 9, on the five base voices;
- 1,984 random cases of one to three random preprocessing and reader texts, on random modes;
- 15 texts on 3 modes whose sentences the reader cuts at each of its length limits (text.md §8.5): the source and clause limits, the weight limits of digits and symbols, and long phonetic input;
- 4 series on 3 modes that send `\Chr="Monotone"\` and `\Rst\` on their own and carry the monotone flag into later texts.

They hold 112,020 word positions and 2,236 bookmarks. The other 35 cases crash the original, each at one of the faults of section 9: 18 tags past the control table, 7 unit buffer overruns (the other 6 overruns went unnoticed and match) and 10 standalone `\Chr=` names. The check leaves out tagged texts the original can't run for other reasons: `\Chr=` names of 78 characters or more (a heap overflow in its tag scanner), pauses over 10 minutes, and texts whose preprocessing appends uninitialised bytes (text.md §10).

The Go engine is checked on those 3,357 cases, the 35 that crash the original included (there it does what section 9 says), and on a 10-minute pause on a plain mode and on a reverb mode. All 3,359 are identical: the PCM, every event with its sample, and both `AttribChanged` sequences. Since then the preprocessing group has 18 more hand cases, with soft hyphens and `ß` in tag names (text.md §3.3): its 290 cases are identical against the original, and all 3,395 cases are identical in Go.

The user lexicon (pronunciation.md §6) is checked against the original through its SAPI 4 interface: 789 engines, each with calls in order, the texts spoken between them, the lexicon file at the start and the one written at the end. They are identical in Go, except where a case meets the escape of pronunciation.md §7, at which point it stops.

The output formats (interface.md §4, synthesis.md §13) are checked against the original with destinations that accept only some formats. 1,780 of 1,783 cases are identical, and the other 3 crash the original at a tag past the control table. Compared: the formats offered (every field), `Select`'s outcome, the PCM in the accepted format, every event with its sample, and both `AttribChanged` sequences.
- **Each fallback format:** 8-bit at the voice's rate; 8000 and 16000 Hz, 16- and 8-bit, for the 22050 Hz voices; 11025 Hz, 16- and 8-bit, for the telephone voices. 1,476 cases on 9 modes, with plain, whispered, reverb and robot voices: hand texts, corpus sentences, series of texts, and attributes set.
- **Short first frames:** texts whose first frame gives no output, and the frames after one.
- **The order of the offers:** 10 destinations, five of which accept nothing, so `Select` fails.
- **Random texts:** 297, each in a random fallback format.

The Go engine is checked on these 1,783 cases too, 6,542 in all. All are identical: the format taken (or `Select` failing), the PCM in it, every event with its sample, and both `AttribChanged` sequences.

The engine `Select` gives for `GUID_NULL` is checked against the original through its SAPI 4 interface. The cases spoken on it are the cases above on Mary (for Telephone), the same base voice, and the attribute, series, standalone and phonetic cases: 367 in all. They are identical in Go: the PCM, every event with its sample, and both `AttribChanged` sequences.

## 12. `AudioReset`

`AudioReset` (interface.md §9) stops the destination and sets an abort flag in each stage. It then sends a reset marker down the stages, waits until the marker comes out at the end, and clears the flags.

### 12.1 What the stages do while aborted

- **Front end.**
  - A text it takes from its queue is dropped whole. It isn't preprocessed, so the address context stays as it was.
  - In the text it is reading, it checks the flag only after a sentence: once it has posted that sentence's units and read the next span (sending the span's controls on their own if the span has no words, section 5). It then leaves the segment.
  - Every later segment of that text is still visited. It gets its pitch change (section 3) and the span check of its first span. If that span is a sentence, the sentence is analysed and posted (section 6, so its tags still change the monotone flag), the next span is read and checked, and the segment is left.
- **Voice.**
  - It drops every unit batch it receives.
  - In a batch it had already started, it drops the frames of the remaining words but still handles every tag: speed, volume and resets are applied, and character names, resets, word positions, bookmarks and pauses are passed on.
  - Standalone controls are handled as usual.
- **Synthesiser.** It drops frames and pauses. Character changes and resets still change its effects.
- **Events.**
  - No event is placed any more.
  - Each text whose end the reset reaches gets `TextDataDone` with flags 1 (`TTSBNS_ABORTED`) at the destination's position, in order.
  - A text whose start hadn't reached the destination gets no `TextDataStarted`.
  - `AttribChanged` is still sent for every change above.

Nothing else is reset. Pitch, speed and volume, the monotone flag, the effects, the address context, the rule modules' registers and the synthesiser's state stay as the stages left them, and the next text starts from there. So tags of the text being stopped can still take effect: every pitch change of its later segments, standalone controls, the tags of every batch the voice had started, and the monotone flag of the sentences the front end had posted.

### 12.2 Where each stage stands

That depends on how far each thread had got when the flag was set, which is timing. The stages are joined by queues, and a producer waits before some of its posts.

**Front end.**
- It waits until the voice's queue is empty before posting a sentence's units, and before passing on a text's start or end.
- It posts standalone controls without waiting.

**Voice.**
- It waits until the synthesiser's queue is empty before each word's frames, and before passing on a standalone control (other than speed and volume, which it uses itself) or a text's start or end.
- It posts the tags inside a batch without waiting.

**Synthesiser.**
- It waits while the destination's queue is full: from 10 messages until it has been drained to 2.
- It waits after synthesising a frame, before posting the frame's `Visual` and its samples (none for a frame without samples), and before passing on a word position or bookmark.
- It posts character changes and a text's start and end without waiting.

**Window thread.**
- It drains the destination's queue into the destination, as far as the destination takes it, whenever the synthesiser wakes it:
  - after the 5th chunk of samples of each text;
  - when a chunk fills the queue;
  - when it takes a message while the queue is full;
  - after it passes on a text's end.

When the destination stops taking samples (it is full while it plays), the window thread holds the chunk that didn't fit. Every stage then runs on until it waits on a full queue: the synthesiser some frames past that chunk, the voice up to its next word, and the front end up to its next sentence. A stage is left at the first wait that comes after a post its consumer hasn't taken. So the voice's and the front end's places follow from the synthesiser's.

How far the synthesiser gets is a race with the window thread. **The throttle rule:** each time the synthesiser wakes the window thread, the window thread drains the queue before the synthesiser goes on. Under this rule a stop at a given sample always leaves the stages in the same places.

The original doesn't always do that. 48 cases were each run three times on it: 18 hand cases and 30 random texts of corpus sentences and tags, on random modes, with the destination stopping at random places.
- Every one of the 144 runs is explained by section 12.1 with some placement of the stages.
- 35 cases gave the same result in all three runs; the others varied.
- The throttle rule gives what the original gave in 112 runs. It covers at least one run of 43 cases and every run of 31 of the 35 repeatable ones. The wake-up after a text's end (section 14.4) is part of it; scored both ways on one set of runs, the rule gives 111 runs with it and 108 without.
- Two other rules match fewer runs: the window thread draining only when the synthesiser waits (65), and the queue refilled from empty behind the held chunk (87).

The format doesn't change any of this: a chunk is a frame's samples, whatever their rate and width. The 18 hand cases were run twice each on destinations that accept only another format (8000 Hz 8-bit and 16000 Hz 16-bit for the 22050 Hz voices, 11025 Hz 16-bit and 8-bit for the telephone voice), with the place the destination stops counted in bytes. All 72 runs are explained by section 12.1, and the throttle rule gives 60 of them.

### 12.3 The Go engine

`Speak` returns when the sink has everything, so a text spoken on an engine that isn't paused starts on empty queues. Texts are queued behind another only during a pause (section 14.5), and a stop then ends them all.

- **The stop.** An error from the sink stops the text. `Audio` returns how many of its bytes the sink took, so the destination can stop inside a piece, as a device's buffer fills inside a chunk. The chunk the destination holds is the frame of the `Audio` call that failed, unless the sink took all of its bytes. Then, and after an error from `Event`, it is the next frame with samples.
- **The stages run on.** From then on the sink gets nothing, but the stages run as the original's threads would have:
  - the window thread drains the destination's queue each time the synthesiser wakes it (the throttle rule, section 12.2), up to the chunk it holds;
  - the synthesiser, the voice and the front end each stop at their first wait after a post that their consumer hasn't taken, and from there do what section 12.1 says;
  - the synthesiser makes the frame it is on whole, even when that frame is a long pause.
- **What the caller gets.** `Speak` (or `Resume`) returns the sink's error. The `TextDataDone` with `TTSBNS_ABORTED`, for every text queued whose `TextDone` the sink hasn't had, is left to the caller, who knows where it goes: at the samples the sink took. The positions in later texts count on from there.
- **Stopping from elsewhere.** A caller that wants to stop a text from another goroutine makes its sink return an error on its next call.

The Go engine is checked with the stages placed by the throttle rule and every queue empty when the stopped text starts. There are 2,171 cases, 2,079 of them stopped:
- the 18 hand cases of section 12.2, cut at the text in which the stop falls (the texts queued after it are dropped);
- 1,500 random texts made of corpus sentences and tags, some after a text spoken whole and some with an attribute set first. The stops fall anywhere, at the end of a chunk, at 0 or at the end of the text. Each case ends with another sentence or another random text;
- 160 stops of four designed texts, each stopped at 0 and at the end of every chunk, so that the destination holds each of their chunks in turn. The texts hold standalone controls, later segments, pauses, character changes and resets, and a sentence whose analysis fails;
- the hand cases and the four sweeps again on destinations that accept only a fallback format (36 cases and 157 stops), and 300 more random texts, each on a destination that accepts only one fallback format.

All 2,171 cases are identical: the PCM, every event with its sample, both `AttribChanged` sequences, and the pitch, speed and volume at the end.

## 13. Prosody transplants

A `\Btp=` text can carry its own phones, with their durations and pitch, to be spoken in place of the text that follows the tag. This is the engine's TRANSPLANTED feature (interface.md §2). The original converts the IPA codes with the SAPI 4 runtime's phoneme converter (`STPhoneConv` in `speech.dll`), which it creates through COM. Without the runtime it crashes. The Go engine carries the converter's table in the data pack (13.4).

### 13.1 The text

`\Btp=n<attributes>data\`, followed by the text it replaces. The preprocessor (text.md §3.3) adds three controls: Btp (the tag's offset), the Btp text (everything from the `=` up to the next `\`, so `=n<attributes>data`), and n bytes after that `\`, Etp. Their markers stand where the tags were, so the analyser places them among the sentence's units like any tag, and they may also end up unplaced (section 6).

The Btp text is upper-cased first (A to Z only), so case doesn't matter anywhere in it.

**Attributes**, between `<` and `>`, separated by `,`:
- `RD`: durations are relative (13.3). Exactly these two characters.
- `RP`: pitches are relative. Exactly these two characters.
- `BP=n`: a pitch for the first point of the next phone that has a duration part. At least one character after the `=`, read with `atoi`.
- `EP=n`: a pitch for the end of the last phone (13.5). The same form.
- Anything else is ignored.

**Data**, from the first character after `>` that isn't a space, TAB, LF or CR: entries separated by `;`. An entry is one to three hexadecimal IPA codes separated by whitespace, then optionally `,` and a duration part: a duration and points `(position, P pitch, V amplitude)`.

Example: `\Btp=5<RD>68,80;2c8 25b,120 (50,P200);6c;2c8 6f,200;\hello` speaks "h ˈɛ l ˈo" for "hello".

### 13.2 State

**The flag.** A successful transplant turns it on. It goes off at an Etp control and at the start of each `TextData` call, not at the end of a sentence. While it is on, posting (section 6) drops every unit (phones, `#` and `SIL`) and every tag except Btp, Etp and Btp text. So the replaced words lose their word positions, and bookmarks, pauses, character changes, resets, speed and volume changes inside the replaced text are lost too.

**The batch's settings.** RD, RP, BP, EP and the Btp offset belong to one sentence's posting. They start off (BP and EP at 0, the offset unknown) and last to the end of the batch, so a second transplant in the same batch inherits RD, RP and EP and a BP that no phone used. A phone that uses BP sets it back to 0. EP is never cleared. A Btp control sets the offset to `atoi` of its value.

**The record buffer.** Posting writes 100-byte records:

| Offset | Field |
|---|---|
| 0x00 | name, NUL-terminated (a tag's text may run past 16 bytes) |
| 0x10 | duration code (float32) |
| 0x14 | point count (int32) |
| 0x18 | positions, 5 × float32 |
| 0x2C | pitches, 5 × float32 |
| 0x40 | amplitudes, 5 × float32 |

- The buffer starts zeroed, with room for the units plus two records per tag of the span.
- A unit is copied whole: name, duration code, and its one point (position 1.0, the pitch, amplitude 0), or none for `#` and `SIL`.
- A tag's record is its text written as a string.
- A pause is three records: `#`, then `\Pau=n\` with its duration code set to 0, then `#`.
- A record that was written and then taken back (by a failed transplant, or a `-` entry) stays in the buffer. A later record in the same place keeps every field it doesn't write.

**Growth.** A count starts at 0 for each batch. Each phone a transplant writes adds one (not `#` or `-` entries), and each unit or tag dropped while the flag is on takes one away. A failed transplant puts the count back as it was.
- When a transplant's write position reaches the buffer's room, the buffer grows by 5 records, zeroed.
- At an Etp control with the count above 0, the buffer grows by that many records, and that many records from the write position are zeroed.

### 13.3 A transplant

When posting reaches a Btp text control (placed or unplaced), and the text has a `<`:

1. Read the attributes.
2. Remember the write position and the count.
3. For each entry:
   1. **Codes.** As `sscanf("%x %x %x")`: up to three numbers, each after optional whitespace, with an optional sign, an optional `0X` and hexadecimal digits, kept in 32 bits (wrapping). Each code is kept to 16 bits. A code of 0 ends the string.
   2. **Conversion** with the converter (13.4). An invalid character fails the transplant.
   3. The converted name, at most 14 bytes, is written as the name of the record at the write position. Then, by its first character:
      - `#`: the record becomes `\Wrd=w\`, and the write position moves on (the count doesn't change). Here w is the Btp offset o plus the entry's position p in the Btp text (counting from the `=`): o + p if o ≥ 0, else 2p − o, with 32-bit wrapping, printed as a signed number. For the wide character sets the preprocessor stores the offset negated and doubled, so this is the entry's offset in the wide text. A negative w is a tag the voice can't read (units.md §10).
      - `-`: nothing more. The write position stays, so the next record overwrites this one.
      - anything else: a phone (4 below). Afterwards the write position moves on and the count goes up.
   4. **A phone.** Skip to the first `,` or `;`.
      - At a `;`, nothing more is written: the record keeps the duration code and point count already in its place (0 in a fresh buffer).
      - Otherwise skip that character and any space, CR, TAB or LF. Unless the next character is `(`:
        - the duration code is `atoi` of the text there: with RD, `f32(5 − n × f32(−0.01))`, so the phone lasts n % of its mean duration (units.md §6); without, `f32(−n)`, n milliseconds;
        - then skip to the next `;`, space, TAB, CR or LF, and over whitespace. **So points must be separated from a duration by whitespace:** `120(50,P200)` skips the point, `120 (50,P200)` keeps it.
      - The point count becomes 0. If BP isn't 0, point 0 becomes position 0 with the BP pitch (its amplitude stays as it was), the count becomes 1 and BP goes back to 0.
      - **Points.** While the next character is `(`:
        - A count of 5 or more becomes 4, so a sixth point overwrites the fifth.
        - The position is `f32(atof(text after "(") × f32(0.01))`. Above 1 it becomes 1; anything not at least 0 becomes 0.
        - If the count is above 0 and the position is below the previous point's position, the transplant fails.
        - From the next `,` in the text: skip it and any whitespace, take one letter and read `atof` after it. `P` sets the point's pitch, `V` its amplitude `f32(v × f32(0.01))`, and any other letter nothing. Skip to the next `,` or `)`, and repeat while it is a `,`. A point keeps whatever pitch or amplitude it doesn't set.
        - The count goes up by one. Skip to the next `(` or `;`.
   5. **Next entry:** after the next `;` from where reading stopped, and any space, CR, TAB or LF. The transplant ends at the end of the text, or when no `;` is left.
4. **Failure** puts the write position and the count back as they were in step 2. The records written stay in the buffer, and the flag doesn't change.
5. **Success** turns the flag on, then applies EP (13.5).

**Pitches.** A P, BP or EP value v is `f32(v)` Hz. With RP it is `f32(2^(v × 1/48) × pitch)`, where pitch is the current pitch in Hz (the one PitchSet and `\Pit=` set), and the product is rounded to double before float32. So v counts 1/48 octave (a quarter of a semitone). BP and EP are integers (`atoi`), P values are `atof`.

**Number formats**, as msvcrt reads them:
- `atoi`: optional whitespace and sign, decimal digits, wrapping in 32 bits.
- `atof`: optional whitespace and sign, digits with an optional `.`, and an optional exponent after `E` or `D`.

### 13.4 The converter

The converter gets the engine's phone set: a table from IPA strings to names, 71 entries. The data pack carries it.
- Stressed vowels, written ˈ plus the vowel, give `IY1`, `IH1`, `AE1`, `AA1`, `AH1` (ʌ and ə), `AO1`, `AY1` (ɑɪ), `EY1`, `EH1`, `ER1`, `OW1`, `AW1` (ɑʊ), `OY1` (ɔɪ), `UH1` and `UW1`.
- Unstressed vowels give the same names with `0`.
- Consonants: `TH`, `DH`, `SH`, `ZH`, `HH`, `NG`, `CH` (ʧ and tʃ), `JH` (ʤ and dʒ), `B`, `P`, `D`, `T` (t and ɾ), `G` (g and ɡ), `K`, `F`, `V`, `S`, `Z`, `L` (l and ɫ), `R` (r, ɻ and ɹ), `Y` (j), `W`, `M` and `N`.
- Others: `SIL` (`_`), `#` (space and `#`), and `-`, `.`, `?`, `!` and `,` (the characters themselves).

**Conversion.** From the start of the codes, take the longest IPA string in the table that the rest begins with, add its name (with no separator) and move past it. Matching is case-sensitive: `B` is not `b`. A character that begins no string is invalid, and the conversion fails.

Two or three codes can therefore give joined names: `6e 67` (n g) gives `NG`, a phone, and `62 28c` gives `BAH0`, which is none.

### 13.5 End pitch

After a successful transplant with EP not 0, take the last record before the write position whose name doesn't start with `\`. This is the transplant's last phone, or the record before the transplant if it wrote no phone. Number the 32-bit fields from the positions on, so that position slot i is at 0x18 + 4i and pitch slot k at 0x2C + 4k; slot 5 of the positions is the first pitch.
1. If position slot c, where c is the record's point count, isn't 1.0, the count goes up by one.
2. A count of 5 or more becomes 4.
3. Position slot `count` becomes 1.0, one past the last counted point.
4. The EP pitch goes into pitch slot k, where k is the point count of the record at the write position, normally 0.

So EP usually adds a point at position 0 with pitch 0, and sets the first point's pitch.

### 13.6 In the voice

- Transplanted phones come with their own duration codes and points. The voice's general interpolation of units.md §7 handles them, and their durations follow units.md §6.
- A batch can end with a phone: the transplant came last, or ran to the end of the sentence. The voice then reads the record after the batch for the last phone's word position (units.md §4): the record at the write position, zero in a fresh buffer, which is not a boundary.
- A name that isn't one of the voice's phones crashes the original (units.md §4). That covers `.`, `?`, `!`, `,` and joined names. The Go engine plays such a record as `SIL`.

### 13.7 Faults

**Don't reproduce.** The original reads memory it doesn't own, or calls `DebugBreak`:
- **Running out of text:** attributes with no `>`, a last entry with neither `,` nor `;` after its codes, or a point letter at the very end. The original reads past the Btp text; the Go engine treats the transplant as failed.
- **No data after `>`.** The original converts a stack word it never wrote. In every case seen the conversion failed and the transplant was taken back, as the Go engine does.
- **An entry with no hexadecimal code** (`;;` or `zz`): `DebugBreak`. The Go engine treats the transplant as failed.
- **A point with no `,` after it** (`(50)`). The original dereferences NULL and crashes. The Go engine treats the transplant as failed.
- **A name that isn't a phone** (13.6).
- **Growth during a transplant that then fails.** The original goes on writing through its pointer into the old buffer if the growth moved it. The Go engine keeps one buffer.
- **The record after a full buffer.** The record after the batch (13.6) or after a write position at the buffer's end lies outside the original's buffer. The Go engine reads it as zero.
- **Too many output periods.** Extreme pitches or durations can plan more output periods than the synthesiser's arrays hold (synthesis.md §14). The original writes past them and usually crashes. When it survives, its samples are the plan's, which the Go engine plays.

### 13.8 Checked

- **The converter**, against the runtime's own: 505,723 inputs, each with two buffer sizes, all identical. The inputs are every single code unit, every pair and triple of the table's characters and their case variants, the table's strings with a character before and after, and long and random strings.
- **Transplants**, against the original through its SAPI 4 interface:
  - 86 hand texts covering every branch above, on Mary, Sam, Mike (for Telephone) and Mary in Hall, and 12 of them on Mike with pitch 200 and speed 80. Four of them give a word's last phone five points that make the voice write past its pitches (units.md §7.3), and two depend on the records past the write position: the zeroing at Etp, the count a failed transplant puts back, and a stale record read after the batch;
  - 2 series of texts;
  - 1,000 random transplants in random contexts, on 8 modes.

  349 of the 364 hand cases and 944 of the 1,000 random ones are identical: the PCM, every event with its sample, and both `AttribChanged` sequences. The other 15 and 56 crash the original, each at one of the faults above. One case that reads past the Btp text survived in the last run and matched. The 14 transplants without phoneme data never crashed it and were taken back.
- **The rest of the engine** stays identical with transplants handled as above (NaN comparisons, division by zero, float32 overflow, the record after the batch): the other groups of section 11, with 300 random cases, give 1,678 of 1,708 identical and the same 30 crashes.

### 13.9 The Go engine

Package `posting` writes each batch into a byte buffer laid out as the original's (13.2): units copied whole, tags as their texts, transplants as 13.3 says, with the same growth. Records written and taken back stay, so later transplants and the record after the batch see what the original's would. The voice then reads the records back: a tag by its text (units.md §10), a phone with its duration code and points. The converter's phone set comes from the data pack.

The faults of 13.7 end the transplant as failed: running out of text, an entry with no code or no data, a point with no `,`, and a word boundary before any Btp offset. A write past the buffer makes it grow, and a phone record whose point count isn't 0 to 5 reads as a plain `SIL`. EP is left half done where it would reach outside the buffer.

The Go engine is also checked on the transplant groups of 13.8: the 86 hand texts and 2 series on four modes, 12 of them with pitch and speed set, and 1,000 random transplants. So are the converter, on the 505,723 inputs of 13.8, the C number readers (`atoi`, `atof`, `sscanf`), on 62,256 texts, and the general interpolation, on 4,000 cases of records as transplants make them. The Go engine is identical on all of them: the 4,759 engine cases (the PCM, every event with its sample, and both `AttribChanged` sequences), the converter, the number readers and the units cases.

## 14. `AudioPause` and `AudioResume`

`AudioPause` (interface.md §9) stops the device and releases it, and that is all it does. The stages keep running. The window thread holds the next chunk of samples it takes and nothing after it reaches the destination. Each time the window thread is woken (section 12.2), it still hands the destination the events queued before that chunk. Each stage runs on until it waits on a full queue, as it does behind a full destination. `AudioResume` claims the device again: the window thread hands over the held chunk, and the stages go on from their waits. Nothing is lost or made twice, so a pause without calls gives the audio and events of a text spoken without one.

### 14.1 Where the stages wait

The synthesiser's place follows from the held chunk, and the voice's and the front end's from the synthesiser's, as section 12.2 says. What matters for the calls is where in its work each stage waits:
- **Front end.** Before posting a sentence's units: the sentence is read and analysed, but its units aren't written yet. The pitch steps (prosody.md §10.1) and the monotone copy (section 6) are written after the wait. It also waits before passing on a text's start or end.
- **Voice.** After building a word's frames, before posting them, and before passing on a standalone control or a text's start or end. A standalone `\Rst\` has reset speed and volume before its wait.
- A stage that has handled everything it was given doesn't wait; it is idle.

### 14.2 Calls during a pause

The attribute calls (interface.md §5) and `Inject` (§7) write the stage's own values at once, from the application's thread. Pitch belongs to the front end, speed and volume to the voice. A waiting stage takes the new values up when it goes on:
- **Pitch** (`PitchSet`, or an injected `\Pit=` or `\Rst\`): the sentence the front end waits to post is written with the new pitch, and so is every later one, until a tag changes it again.
- **Speed and volume** (`SpeedSet`, `VolumeSet`, or an injected `\Spd=`, `\Vol=` or `\Rst\`): the word whose frames the voice waits to post keeps the old values. The next word it builds takes the new ones, and later tags in the batch change them as usual.
- **`PitchGet`, `SpeedGet` and `VolumeGet`** read the values as the stage holds them at its wait: the pitch after every change the front end has made, and the speed and volume after every tag of the batches the voice has handled.
- An idle stage takes the calls before anything queued after them.
- Results, ranges and `AttribChanged` are as on an idle engine (interface.md §5, §7, §8). A call's `AttribChanged` is sent at the call, and a tag's when its stage reaches it.

**Texts sent during a pause** are queued without claiming the device (interface.md §9). They are spoken after the texts before them, with the values the calls left. The stages take them at once, up to their waits. When nothing was held, the first chunk of the first such text is held, and the destination gets the events before it when the window thread is woken: after the text's fifth chunk, when the queue fills, or at the text's end. A text without samples is handed over whole.

### 14.3 Resume and reset

- **`AudioResume`** goes on from the held chunk, and each stage from its wait. If nothing is held (everything reached the destination before the pause), the stages are idle and the next text starts with the values the calls left.
- **`AudioReset` during a pause** un-pauses and stops (section 12). The stages are where the pause left them, with the calls made since already in their values, and with the texts queued during the pause taken up to their waits. From there each does what section 12.1 says. The destination keeps the events it was handed; every text whose `TextDataDone` it hadn't been handed gets one with `TTSBNS_ABORTED`. In the original, an `AudioReset` made at once after `TextData` comes before the front end has taken the text, which is then dropped whole.

### 14.4 Checked

This is checked against the original through its SAPI 4 interface. Each case queues the texts and pauses, either at once (before anything reached the destination) or once the destination stops taking samples and every stage waits behind it. It then makes the calls, queues the texts for the pause and waits 300 ms, so that the stages take them, calls `AudioResume` or `AudioReset`, and speaks another sentence.
- **26 hand cases:**
  - every call on its own and in groups;
  - `Inject` with `\Spd=`, `\Pit=`, `\Vol=` and `\Rst\`, and values out of range;
  - gets after tags;
  - a pause before anything was played, and one after the whole text;
  - a monotone sentence, segment pitch changes, a `\Pau=`, bookmarks and two texts;
  - `AudioReset` in place of `AudioResume`, also once the front end has finished;
  - on Mary, Sam, Mike, Mike in Hall, Mary (for Telephone) and RoboSoft Six.
- **9 of them again**, with a text queued during the pause: with `AudioResume` and with `AudioReset`, after a pause with nothing held, and a text of tags only.
- **100 random cases:** corpus sentences and tags on random modes, 0 to 4 random calls, the destination stopping anywhere or the pause at once, a text queued during the pause in about a third of them, and `AudioReset` in about a third.

There are 245 calls in all. Each case ran twice, which makes 270 runs.
- **Every run is explained** by some placement of the stages. That covers the PCM, every positional event with its sample, both `AttribChanged` sequences, each call's result and the value it read, and the pitch, speed and volume at the end.
- **Repeatability:** 126 of the 135 cases gave the same result both times.
- **The throttle rule** gives 241 runs. It covers at least one run of 125 cases and every run of 116 of the 126 repeatable ones.
- **The other rules** of section 12.2: the queue refilled behind the held chunk gives 213 runs, and the window thread draining only when the synthesiser waits gives 215.
- **The rest:** 13 runs, from 9 cases, are explained by a placement that none of the three rules gives, as with `AudioReset`.

The texts queued during the pause showed two things the first runs (without them) didn't: the synthesiser wakes the window thread after a text's end (section 12.2), and during a pause the destination gets only what a wake-up hands it.

**A second pause.** Some cases pause again after the calls. The destination is set to stop at a later place, `AudioResume` is called, and after 600 ms the destination has stopped again and the stages wait behind it. Then `AudioPause` is called, the second pause's calls are made, and the destination takes everything again. The second stop lies mostly inside what the first pause kept: the held chunk and the queue behind it. Under the throttle rule, at `AudioResume` the window thread hands over the held chunk, writing as much of it as the destination takes, and drains its queue up to the chunk that goes past the new stop. The synthesiser then goes on from its wait, but only once two messages or fewer are left.
- **Cases:** 12 hand cases and 100 random ones, with 302 calls. The hand cases include a pause at once, `AudioReset` after the second pause, a `\Pau=` and bookmarks, on Mary, Mary (for Telephone) and Sam. 110 cases paused again, 81 of them inside what the first pause kept, and in 39 the synthesiser was still at its first wait.
- **Every run is explained** by some placement of the stages at the two pauses: all 224 runs.
- **The throttle rule** at both pauses gives 180 runs, and every run of 83 of the 95 repeatable cases.
- **The other 44 runs:** in 24 the first pause's places are elsewhere, the same race as with one pause; in 20 it is the second's.
- **That race goes both ways.** The original stops the stages sooner than the rule in some runs and later in others. Three of the five repeatable cases among these gave other places when run again later, two of them the rule's. The window thread sets the synthesiser's thread priority from how full the queue is, so where the stages stop depends on scheduling. No other rule fits these runs better, so the Go engine keeps the throttle rule.

The 135 cases with one pause, run again alongside, were all explained again (270 runs), the throttle rule giving 244.

### 14.5 The Go engine

The stages run with their queues and waits (section 10): the front end analyses a sentence before its wait and writes the units after it, the voice builds its frames word by word and waits after each word's frames, and the synthesiser makes a frame it waits after only when it goes on. A pause holds them there, under the throttle rule:
- **Pausing.** A sink pauses the engine by returning `ErrPaused` from `Audio`, having taken what it wants of the piece, or from `Event`. `Pause` pauses an engine between texts, and fails with `ErrAlreadyPaused` on a paused one. The destination holds the chunk the sink didn't take all of, or after an `Event`, the next chunk with samples. The stages run on until each waits on a full queue, as behind a stop. What the synthesiser makes from then on is kept, with each event placed where it stands in the audio; each time the window thread is woken, the sink gets the events kept before the held chunk. `Speak` returns `ErrPaused`.
- **During the pause.** The attribute methods and `Inject` change the stages' values at once, which is where each stage waits (section 14.2). `Speak` queues its text and returns `ErrPaused`; the stages take the text at once, up to their waits. The events before the held chunk go to the sink the engine has, or if it has none (a pause between texts), to the sink of that `Speak`.
- **`Resume(sink)`** (`ErrNotPaused` when not paused) gives the sink what was kept, and the stages go on from their waits. It returns as `Speak` does. If the sink pauses again in what was kept, the chunk it pauses in (or after an event, the next one) is held, and the posts after it stay queued.
- **`Reset`** on a paused engine drops what was kept and aborts the stages from their waits (section 12.1). Every text queued whose `TextDone` no sink had ends there: the caller sends its `TextDataDone` with `TTSBNS_ABORTED`, as after a stop. On an engine that isn't paused, `Reset` does nothing.
- A pause keeps at most the destination queue's frames, but each whole: a long `\Pau=` held in a pause is held in memory.

The Go engine is checked in its own order: each text is spoken with its own `Speak`, the text the sink pauses in starts on empty queues, and the texts after it are queued during the pause. When the sink takes everything, the engine is paused after the texts, with nothing held. Then come the calls, the texts queued during the pause, `Resume` or `Reset`, and another text. A case with a second pause first resumes with a sink that takes a set number of samples in all and pauses again, at the places the throttle rule gives, and makes the second pause's calls. 1,833 cases:
- the 47 hand cases of section 14.4, 12 of them paused a second time;
- 1,200 random cases: 1 to 3 texts of corpus sentences and tags on random modes, some after an attribute set; the pause anywhere, at the end of a chunk, at 0, or with nothing held; 0 to 4 random calls; a text queued during the pause in about a third, and `Reset` in about a quarter;
- 150 more on destinations that accept only a fallback format;
- 118 pauses of four designed texts with fixed calls, each paused at 0 and at the end of every chunk, two with `Resume` and two with `Reset`, every third with a text queued during the pause;
- 300 random cases paused a second time, mostly inside what the first pause kept, often at the end of one of its chunks;
- 18 pauses of two designed texts, paused once in the middle and again at the end of every chunk the first pause kept, and at three places after it.

330 cases pause a second time, and in 293 of them the sink pauses the engine again. All 1,833 are identical: the PCM, every event with its sample, both `AttribChanged` sequences, each call's result and value read, and the pitch, speed and volume at the end. `TestPauseAnywhere` pauses and resumes three texts at many places, by samples and by events, again and again during what was kept, and gets what it gets without pausing. Seven of eight deliberate breaks of the second pause fail the tests. The eighth starts a text's chunk count again when the sink pauses inside what was kept. It can't show with two pauses: the window thread hands out nothing while it is stuck, so the count matters again only at a third pause. The original keeps that count in the synthesiser, which a pause doesn't touch.
