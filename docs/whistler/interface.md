# SAPI 4 Microsoft engine: interface behaviour

How the Microsoft Speech Synthesis Engine (`msttssyn.dll`, the "Whistler" engine behind Mary, Mike and Sam) behaves at its interface: modes, voice attributes, text input, tags, events and audio control. It describes observable behaviour only, so the Go engine can match it. Later specs cover what happens inside: `voice-format.md` (the `.vce` and `.cfg` files), `text.md`, `pronunciation.md`, `prosody.md`, `units.md`, `synthesis.md`, `effects.md`.

Where the original has a bug (crashes, leaks, wrong return codes), this spec says what correct behaviour is and the Go engine does that instead. Those cases are listed in "Quirks we don't reproduce".

## 1. Engine identity

| | |
|---|---|
| Engine CLSID | `{E0725551-286F-11D0-8E73-00A0C9083363}` |
| Manufacturer | `Microsoft` |
| Product name | `Microsoft Speech Synthesis Engine` |
| Language | 0x0409, dialect `US English` |
| Style | `Clear low-accented North American English native` |
| Age | 30 |
| Interfaces reported | ILexPronounce, ITTSAttributes, ITTSCentral, ITTSDialogs, IAttributes, ILexPronounce2 |
| Features reported | ANYWORD, VOLUME, SPEED, PITCH, TAGGED, IPAUNICODE, VISUAL, WORDPOSITION, IPATEXTDATA, TRANSPLANTED, SAPI4, plus PCOPTIMIZED or PHONEOPTIMIZED, plus PREFERRED on the four base Mary/Mike voices |

## 2. Modes

A mode is a voice the application can select. There are 19 visible modes and 3 hidden ones.

### Visible modes

| # | Mode name | Mode GUID | Gender | Base voice | Output | Pitch Hz (min–max) | Speed wpm (min–max) |
|---|---|---|---|---|---|---|---|
| 0 | Mary | `{C77C5170-2867-11D0-847B-444553540000}` | female | Mary | 22050 Hz | 169 (84–338) | 170 (30–510) |
| 1 | Mary (for Telephone) | `{C77C51D0-2867-11D0-847B-444553540000}` | female | Mary, telephone | 8000 Hz | 169 (84–338) | 170 (30–510) |
| 2 | Mike | `{C77C5160-2867-11D0-847B-444553540000}` | male | Mike | 22050 Hz | 113 (56–226) | 170 (30–510) |
| 3 | Mike (for Telephone) | `{C77C51C0-2867-11D0-847B-444553540000}` | male | Mike, telephone | 8000 Hz | 113 (56–226) | 170 (30–510) |
| 4 | Sam | `{C5C35D60-DA44-11D1-B1F1-0000F803E456}` | male | Sam | 22050 Hz | 100 (50–200) | 150 (30–450) |
| 5 | Female Whisper | `{8E127230-726E-11D1-B1CB-0000F803E456}` | female | Mary, variant 1 | 22050 Hz | 169 (84–338) | 170 (30–510) |
| 6 | Mary in Space | `{D97E9F61-0FB9-11D2-B201-0000F803E456}` | female | Mary, variant 2 | 22050 Hz | 169 (84–338) | 170 (30–510) |
| 7 | Mary in Hall | `{D97E9F62-0FB9-11D2-B201-0000F803E456}` | female | Mary, variant 3 | 22050 Hz | 169 (84–338) | 170 (30–510) |
| 8 | Mary in Stadium | `{D97E9F60-0FB9-11D2-B201-0000F803E456}` | female | Mary, variant 4 | 22050 Hz | 169 (84–338) | 170 (30–510) |
| 9 | RoboSoft Six | `{D97E9F68-0FB9-11D2-B201-0000F803E456}` | female | Mary, variant 5 | 22050 Hz | 100 (50–200) | 130 (30–390) |
| 10 | RoboSoft Five | `{D97E9F67-0FB9-11D2-B201-0000F803E456}` | female | Mary, variant 6 | 22050 Hz | 120 (60–240) | 130 (30–390) |
| 11 | RoboSoft Four | `{D97E9F66-0FB9-11D2-B201-0000F803E456}` | female | Mary, variant 7 | 22050 Hz | 169 (84–338) | 170 (30–510) |
| 12 | Male Whisper | `{8E127232-726E-11D1-B1CB-0000F803E456}` | male | Mike, variant 1 | 22050 Hz | 113 (56–226) | 170 (30–510) |
| 13 | RoboSoft One | `{B674FFB0-A161-11D1-9019-006097B010E2}` | male | Mike, variant 2 | 22050 Hz | 75 (37–150) | 130 (30–390) |
| 14 | RoboSoft Two | `{B674FFB0-A161-11D1-9019-006097B010E3}` | male | Mike, variant 3 | 22050 Hz | 120 (60–240) | 130 (30–390) |
| 15 | RoboSoft Three | `{B674FFB0-A161-11D1-9019-006097B010E6}` | male | Mike, variant 4 | 22050 Hz | 113 (56–226) | 170 (30–510) |
| 16 | Mike in Hall | `{B674FFB0-A161-11D1-9019-006097B010E5}` | male | Mike, variant 5 | 22050 Hz | 113 (56–226) | 170 (30–510) |
| 17 | Mike in Stadium | `{B674FFB0-A161-11D1-9019-006097B010E4}` | male | Mike, variant 6 | 22050 Hz | 113 (56–226) | 170 (30–510) |
| 18 | Mike in Space | `{B674FFB0-A161-11D1-9019-006097B010E7}` | male | Mike, variant 7 | 22050 Hz | 113 (56–226) | 170 (30–510) |

- Output is always 16-bit mono PCM at the rate shown, unless the audio destination refuses it (section 4).
- The order above is the order the engine enumerates them in. Applications store the mode GUID.
- Speaker names: Mary's modes report `Default Female Voice`, Mike's `Default Male Voice`, Sam `Second Male Voice`.
- PREFERRED is set on Mary, Mary (for Telephone), Mike and Mike (for Telephone).
- Every mode reports the engine ID and product name of section 1, language 0x0409 `US English`, age 30, gender 1 (female) or 2 (male), interfaces 0x3F and engine features 0. Its features are 0x1A0FF (ANYWORD to WORDPOSITION, IPATEXTDATA, TRANSPLANTED, SAPI4), plus 0x100 (PCOPTIMIZED) for a 22050 Hz voice or 0x200 (PHONEOPTIMIZED) for a telephone one, plus 0x4000 if PREFERRED.

### Hidden variants

Three more variants exist with a null mode GUID: `Female Whisper (for Telephone)`, `Male Whisper (for Telephone)` and `Sam Whisper`. They never appear in the mode list, but `Select` with `GUID_NULL` selects the first of them, Female Whisper (for Telephone) (section 3). They exist for the `\Chr="Whisper"\` tag, which switches the current voice to its whisper variant, so that tag also works on the telephone voices and Sam.

### What a variant is

Every mode is a base voice (Mary, Mary telephone, Mike, Mike telephone, Sam) plus an optional variant. A variant changes some header values (name, speaker, pitch, speed, features; empty strings and all-ones numbers mean "same as the base voice") and carries a list of processing records for the pipeline stages (whisper, reverb rooms, robot voices). The output format always comes from the base voice. The variant format and records are in `voice-format.md` §3, their signal processing in `effects.md`.

### How the original builds the list

For reference, since it decides what our data pack has to contain:
1. It reads every `*.vce` file in its own folder, in name order. Each is one base mode.
2. If at least one `.vce` was found, it reads every `*.cfg` file in name order. Each `.cfg` holds numbered variants of a base mode that must already be listed. Variants whose parent is missing are skipped.
3. PREFERRED goes to the modes with the highest preference rank in each of four groups (PC male, PC female, telephone male, telephone female), and only if at least one `.cfg` exists.

There is no registry input. Our Go engine reads the converted data pack instead and produces the same list.

## 3. Selecting a mode

- `Select` takes the first mode in the list with the given GUID, hidden modes included. So `GUID_NULL` selects Female Whisper (for Telephone), and `ModeGet` then reports it: mode ID `GUID_NULL`, features 0x1A2FF. Any other GUID that isn't listed fails with `TTSERR_INVALIDMODE`.
- The application must give an audio destination. The engine needs it to support both `IAudio` and `IAudioDest`. For our Go engine the "destination" is the PCM sink.
- Each selected mode gets its own engine instance. Instances of the same base voice share the voice's unit inventory (read-only data).
- Enumeration. The enumerator's position counts every place in the list, the hidden modes included: 22 places, in the order of section 2, with the hidden modes among the variants (Female Whisper (for Telephone) after RoboSoft Four, the other two at the end).
  - `Next(count)` at or past the end returns nothing and S_FALSE. Otherwise it looks at the next `count` places, or as many as are left. A hidden mode among them is passed over and adds one more place, while places are left. It returns the modes found, moves past every place it looked at, and returns S_OK if it returned exactly `count` modes, else S_FALSE. So `Next(0)` returns S_OK, except at the end.
  - Without a `fetched` pointer (allowed only for a count of 1), `Next` returns S_OK whenever it started before the end, even if only hidden modes were left. The Go enumerator is the form with `fetched`.
  - `Skip(count)`: if the position plus `count`, a 32-bit sum that wraps, is past the end, it moves to the end and returns S_FALSE. Otherwise it walks on until it has passed `count` modes that aren't hidden and stops just after the last of them (S_OK). If the list ends first, it returns S_FALSE and doesn't move. With the shipped files, `Skip(20)` from the start fails and stays at the start, `Skip(23)` fails and moves to the end, and `Skip(0xFFFFFFFF)` after the first mode fails and stays.
  - `Reset` goes back to the start. `Clone` returns an enumerator at the same position, which then moves on its own.
  - The ANSI interface shares the position. Its `Next` converts the strings to the ANSI code page, which leaves these ASCII names as they are.

## 4. Output format

The engine asks the audio destination for the base voice's format and falls back if it's refused. The first format the destination accepts wins:

| Base voice | Order tried |
|---|---|
| 22050 Hz voices | 22050 Hz 16-bit, 22050 Hz 8-bit, 8000 Hz 16-bit, 8000 Hz 8-bit, 16000 Hz 16-bit, 16000 Hz 8-bit |
| 8000 Hz (telephone) voices | 8000 Hz 16-bit, 8000 Hz 8-bit, 11025 Hz 16-bit, 11025 Hz 8-bit |

- **Each offer** is a call to `IAudio::WaveFormatSet` with an 18-byte `WAVEFORMATEX`: PCM, mono, with the average bytes per second and the block alignment that go with its rate and width. The first is the voice header's own format; each later one is that structure with the rate and width changed.
- **If nothing is accepted**, `Select` fails with the error the destination gave for the last offer.
- **The accepted format** stays for the engine's life. The synthesiser resamples or converts to 8 bits as synthesis.md §13 says. Event positions are counted in samples of the accepted format.

The output is checked against the original engine with destinations that accept the native format, and for the other formats with destinations that accept only that one ([engine.md](engine.md) §11).

## 5. Voice attributes

Each mode starts at its default pitch and speed (section 2) and at full volume. Attributes are absolute values. There are no relative attributes.

### Pitch (Hz)

- `PitchSet(0)` sets the minimum, half the default rounded down.
- `PitchSet(0xFFFF)` sets the maximum, twice the default.
- Any other value must lie between those two, both included. Outside that range the call fails with `TTSERR_VALUEOUTOFRANGE` and the pitch doesn't change. Values are never clamped.
- `PitchGet` returns the stored value, which is the minimum or maximum after 0 or 0xFFFF.
- `PitchSet` reads only the low 16 bits of its argument, so 65686 sets 150 Hz.
- Internally pitch is converted to a step of 1/48 octave above 50 Hz: `step = 48 × log2(Hz / 50)`, truncated toward zero (so negative below 50 Hz). `prosody.md` uses this.

### Speed (words per minute)

- `SpeedSet(0)` sets 30.
- `SpeedSet(0xFFFFFFFF)` sets three times the default.
- Any other value must lie in [30, 3 × default], both included, else `TTSERR_VALUEOUTOFRANGE` and no change. Never clamped.
- A speed at or below two thirds of the default switches the voice's duration model to its slow mode (`units.md`). The text analyser never sees the speed (`prosody.md` §11).

### Volume

- Only the low 16 bits (left channel) of the value are used, as a linear gain of `volume / 65535`. The right channel is ignored, since output is mono.
- Any value is accepted. `VolumeGet` returns the low word repeated in both halves, so the default reads 0xFFFFFFFF.

### Other attributes

- Real time: Get and Set return `TTSERR_NOTSUPPORTED` (Get writes 0).
- `IAttributes::DWORDGet`/`DWORDSet` support ids 1 (pitch, the value is cut to 16 bits, so 0x10000 sets the minimum), 2 (speed), 3 (volume) and 0x1018 (thread priority). Every other id, real time (0) included, and every String/Memory call, returns `TTSERR_INVALIDPARAM`. A failed `DWORDGet` writes nothing.
- Thread priority only affects scheduling, not the audio. It starts at 0. Only −15, −2, −1, 0, 1, 2 and 15 are valid; anything else fails with `TTSERR_INVALIDPARAM` and leaves it unchanged. Setting it sends no `AttribChanged`. The Go engine keeps it and reports it, and changes nothing else.

### When changes take effect

A change applies to text the engine hasn't processed yet. Audio that is already synthesised is not changed. Speed and volume apply from the next word the engine builds, and are kept across `TextData` calls and `AudioReset` (units.md §8–9). During a pause they apply from where the stages wait: a new pitch from the sentence the front end waits to post, a new speed or volume from the word after the one the voice waits to post. The Get calls read the values as the stages hold them there ([engine.md](engine.md) §14).

## 6. Text input (`TextData`)

- Character sets accepted: `CHARSET_TEXT` and `CHARSET_IPAPHONETIC`. Anything else, `CHARSET_ENGINEPHONETIC` included, fails with `TTSERR_INVALIDCHAR`, before anything else is checked. The ANSI interface converts text from the system code page and then behaves the same (it passes the character set plus 4, which marks the text as having come from ANSI; that only matters for word offsets, section 8 and text.md §2). The Unicode interface takes those two sets too.
- `TTSDATAFLAG_TAGGED` turns on tag parsing for this text. In untagged text a backslash is an ordinary character and is read out as "backslash" (text.md §3.3, §5).
- A null text is an empty string. The size is in bytes and is rounded down to whole characters. The text ends at its first NUL (text.md §2.1).
- The ANSI interface converts exactly `size` bytes. A size of 0 gives no characters, and the call fails with `E_FAIL`.
- After the character set: an unreadable text gives `TTSERR_INVALIDPARAM`, a sink given with an interface other than ITTSBufNotifySink `E_NOINTERFACE`, and a device that can't be claimed its error. A text that fails is not queued.
- An optional buffer notify sink receives the per-text events (section 8).
- Calls queue. Each call is processed after the previous one, and its events arrive in order. The call returns as soon as the text is queued.

`Phoneme` (text to phonemes without speaking) returns `TTSERR_NOTSUPPORTED`.

## 7. Tags

### Vocabulary

Tag names are compared as Windows' `lstrcmpiA` compares them on a code page 1252 system: case is ignored, soft hyphens (U+00AD) don't count, and `ß` matches `ss` (text.md §3.3). So `\Spd=200\` with a soft hyphen inside `Spd` is still `\Spd=200\`. A tag starts with `\`, has an optional `=value`, and ends with `\`. The engine knows these tags. In text, unknown tags are dropped with their contents. The exact parsing, including how malformed tags swallow text, is in text.md §3.

| Tag | Value | In `TextData` (tagged) | In `Inject` |
|---|---|---|---|
| `\Chr=` | one or more quoted strings separated by commas: `"Normal"`, `"Monotone"` or a variant's character tag (`"Whisper"`) | goes back to the mode's own effects, then adds each name's effects in order (voice-format.md §3); `"Monotone"` flattens the pitch. Effects of an earlier `\Chr=` tag don't carry over | ignored, stops parsing |
| `\Wrd=` | number | ignored | ignored, stops parsing |
| `\Com=` | anything | ignored | ignored, stops parsing |
| `\Ctx=` | quoted string | `"Address"` marks the sentence as an address for the rules (it lasts to the end of that sentence); the other known contexts end it (text.md §3.3, §8.2) | ignored, stops parsing |
| `\Emp\` | none | emphasis mark for the rules on what follows (`prosody.md`) | ignored, stops parsing |
| `\Eng\` | anything | ignored | ignored, stops parsing |
| `\Eow\` | none | ignored | ignored, stops parsing |
| `\Fs\`, `\Hs\` | none | ignored | ignored, stops parsing |
| `\Mrk=` | number | bookmark event (section 8) | ignored, stops parsing |
| `\Pau=` | number (ms) | silence of that length | ignored, stops parsing |
| `\Pit=` | number (Hz) | pitch from here on; `\Pit=0\` means the default. Always ends the sentence (text.md §7) | sets pitch now (0 means the minimum) |
| `\Prn=` | IPA string | ignored | ignored, stops parsing |
| `\Pro=` | number | ignored | ignored, stops parsing |
| `\Prt=` | quoted `N`, `V`, `Adj`, `Adv` or `PP` | part of speech for the rules; `"Adv"` is taken as a verb, as in the original. Anything else is ignored | ignored, stops parsing |
| `\Rst\` | none | resets speed, volume, pitch and voice to the mode's defaults, and ends any `\Ctx="Address"`; ends the sentence | resets speed, volume, pitch (not the effects), then stops parsing |
| `\Spd=` | number (wpm) | speed from here on | sets speed now |
| `\Vce=` | voice attributes | ignored | ignored, stops parsing |
| `\Vol=` | number | volume from here on | sets volume now |
| `\Btp=`, `\Tp=` | number, then optionally `<attributes>` and phoneme data | a prosody transplant: the phones, durations and pitch given replace those of the next n characters (engine.md §13). Without `<` no effect. `Tp` is another name for `Btp` (text.md §3.3) | ignored, stops parsing |
| `\Etp=` | number | no effect: a transplant's end comes from its n (text.md §3.3) | ignored, stops parsing |

How `\Pau=`, `\Spd=`, `\Vol=`, `\Mrk=` and emphasis change the speech and the events is in `prosody.md`. Note that `\Pit=0\` means different things in text and in `Inject`. That difference is real behaviour and we keep it.

### `Inject`

`Inject` applies tags immediately, outside the text queue:
- The string is first converted to code page 1252 with best fit, as a text is (text.md §2.1). It ends at its first NUL.
- Only `\Pit=n\`, `\Spd=n\`, `\Vol=n\` and `\Rst\` do anything. Values follow section 5, so an out-of-range value is rejected. `\Pit=n\` takes the low 16 bits of n.
- Numbers are decimal digits, wrapping modulo 2³². Leading zeros are allowed; a sign, a space or anything else in their place stops parsing.
- Keywords are read as in a text (text.md §3.3), up to 79 bytes.
- Spaces (U+0020 only) between tags are skipped. Any other character outside a tag, tabs and line breaks included, stops parsing.
- Parsing stops at the first other tag, a missing `=`, a value that isn't a number, or a rejected value. Tags before it stay applied.
- After each number, one character is skipped without being looked at; it is meant to be the closing backslash. So `\Spd=200x` works, `\Spd=200\ \Vol=100\` and `\Spd=200\\Vol=100\` set both, and `\Spd=200\Vol=100\` stops at `V`.
- `\Rst\` resets speed, volume and pitch, in that order, and not the effects. Then parsing stops: everything after `\Rst\` is ignored.
- The call returns success whenever the string is readable, even if parsing stopped.

## 8. Events

Two kinds of sinks receive events. ITTSNotifySink is registered on the engine with `Register`. ITTSBufNotifySink is passed per `TextData` call.

| Event | Sink | When | Values |
|---|---|---|---|
| `TextDataStarted` | buffer | when playback reaches the start of that text's audio | audio position |
| `WordPosition` | buffer | when playback reaches the start of each word | audio position, byte offset of the word in the text as passed (characters × 2 for Unicode text, × 1 for ANSI) |
| `BookMark` | buffer | when playback reaches a `\Mrk=n\` | audio position, n |
| `TextDataDone` | buffer | when playback reaches the end of that text's audio | audio position, flags 0 |
| `Visual` | engine | for every synthesised frame that has audio (one per phone or pause, units.md §11), when playback reaches its start | audio position, IPA phoneme (0 for an A sink), engine phoneme, hints 0, mouth shape (`TTSMOUTH`) from a fixed table per phoneme |
| `AudioStart` / `AudioStop` | engine | when the audio destination starts or stops the device | the destination's total byte count |
| `AttribChanged` | engine | after every successful pitch, speed or volume change (from the API, `Inject` or a tag), even if the value didn't change; not tied to audio | `TTSNSAC_PITCH`, `TTSNSAC_SPEED`, `TTSNSAC_VOLUME`; `\Rst\` in `Inject` sends speed, volume, pitch in that order, in text pitch first (units.md §10) |

- The position events are bookmarks placed in the audio stream, so they fire when the audio is played, not when it is synthesised. Where each one lands is in [engine.md](engine.md) §8. The "audio position" is the destination's position (in bytes of output audio).
- For our Go engine (which writes PCM rather than playing it), this means each event belongs at an exact sample offset in the output. The engine reports `(sample offset, event)` pairs, which is enough to rebuild the original timing.
- `Visual` is not sent for audio that was flushed by a reset.
- All events reach the application on the thread that selected the mode, through that thread's message loop. The original never calls an application sink from its worker threads.
- The ITTSNotifySink2 events (`Error`, `Warning`, `VisualFuture`) are never sent.

### Registering sinks

- `Register` accepts only ITTSNotifySinkW and ITTSNotifySinkA. Anything else fails with `E_NOINTERFACE`.
- It returns a key. Keys are unique within the process and start at 1.
- `UnRegister` with an unknown key fails with `TTSERR_INVALIDKEY`.
- There is no limit on the number of sinks.

## 9. Audio control

| Call | Behaviour |
|---|---|
| `AudioPause` | Stops the device and releases it. The engine holds the next chunk of audio and gives the destination nothing after it (the events before it still reach the destination when the window thread is woken), while the stages run on until each waits on a full queue ([engine.md](engine.md) §14). Attribute calls and `Inject` made while paused apply from where each stage waits. Texts sent while paused are queued without claiming the device. Pausing twice fails with `TTSERR_ALREADYPAUSED` |
| `AudioResume` | Claims the device again and goes on from the held chunk, each stage from its wait; with no text queued it starts the device and releases it again. If the claim fails, `AudioResume` returns its error, and the engine is no longer paused. Fails with `TTSERR_NOTPAUSED` when not paused |
| `AudioReset` | Stops and flushes the device, throws away all queued text and audio, and also un-pauses. Returns after the pipeline is empty. Every text that hadn't finished gets `TextDataDone` with `TTSBNS_ABORTED`. Pitch, speed, volume and the synthesiser's state are not reset (synthesis.md §12), and tags of the stopped text can still take effect ([engine.md](engine.md) §12) |
| `PosnGet`, `ToFileTime` | Passed straight to the audio destination |
| `ModeGet` | Returns the selected mode's information, as the enumerator reports it (section 2); for `GUID_NULL` the hidden mode's (section 3) |

The device is claimed on the first `TextData` and released when the last queued text is done.

## 10. Errors

| Code | Value | When |
|---|---|---|
| `TTSERR_INVALIDMODE` | 0x80040206 | Select with an unknown mode |
| `TTSERR_INVALIDCHAR` | 0x80040208 | TextData with an unsupported character set |
| `TTSERR_INVALIDKEY` | 0x80040209 | UnRegister with an unknown key |
| `TTSERR_NOTPAUSED` | 0x80040501 | AudioResume when not paused |
| `TTSERR_ALREADYPAUSED` | 0x80040502 | AudioPause when paused |
| `TTSERR_VALUEOUTOFRANGE` (`E_UNEXPECTED`) | 0x8000FFFF | Pitch or speed outside its range |
| `TTSERR_NOTSUPPORTED` (`E_NOTIMPL`) | 0x80004001 | Phoneme, RealTimeGet/Set, TranslateDlg |
| `TTSERR_INVALIDPARAM` (`E_INVALIDARG`) | 0x80070057 | Bad pointers and windows, unsupported IAttributes ids |
| `E_NOINTERFACE` | 0x80004002 | Wrong sink interface in TextData or Register |
| `E_FAIL` | 0x80004005 | ANSI TextData with a size of 0 |

## 11. Quirks we don't reproduce

The original has these bugs. The Go engine behaves correctly instead:
- `Inject` reads a value that isn't a number into a 4-byte variable, so a value of 4 characters or more overwrites the stack. The Go engine stops parsing there, as the original does for a shorter one.
- `Inject` with a string that ends right after a number skips the terminating NUL and reads on past the string. The Go engine stops at the end.
- The ANSI `Next` crashes without a `fetched` pointer and frees the wrong block.
- Notify-sink references leak when an engine is destroyed with text still queued.

## 12. Dialogs

`ITTSDialogs` shows Windows dialog boxes and changes nothing in the engine:
- `AboutDlg` shows an About box with the engine's version, titled with the given title or a default one. `GeneralDlg` shows the same box.
- `LexiconDlg` opens the lexicon dialog of the Speech Tools (`STLexDlg`, an external component) on the engine's lexicon interfaces (pronunciation.md §6), and returns its result.
- `TranslateDlg` returns `TTSERR_NOTSUPPORTED`.
- With no window, `AboutDlg`, `GeneralDlg` and `LexiconDlg` do nothing and return S_OK. A window that isn't one, or an unreadable title, gives `TTSERR_INVALIDPARAM`.

The Go engine has no dialogs.
