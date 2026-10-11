# SAPI 4 Microsoft engine: effects

What the Microsoft Speech Synthesis Engine (`msttssyn.dll`) does for its variant modes: whisper, the post-filter, the reverb behind Hall, Stadium, Space and the RoboSoft voices, and monotone. Also how `\Chr=` and `\Rst\` switch them, and which state they keep. The record format is in [voice-format.md](voice-format.md) §3, and the synthesis the effects hook into is in [synthesis.md](synthesis.md).

**In short.**
- **Whisper** gives every frame noise excitation and ignores the pitch.
- **The post-filter** is a 3-tap high-pass, used only with whisper.
- **The reverb** is a chain of allpass delay lines mixed with the dry signal. Long lines give the rooms; one short, strongly fed-back line gives the metallic RoboSoft ring.
- **Monotone** flattens the pitch to the voice's current pitch.
- **When they run.** The effects work in place on each frame after the vocal-tract filter, and never change the number of samples, so events and their timing are unaffected.
- **Switching.** A mode's own variant switches its effects on when the engine starts. `\Chr=` and `\Rst\` switch them between two frames.
- **Lifetime.** The reverb's delay lines and its random numbers run for the whole life of the engine. Nothing clears them and no tail is flushed.

**Exactness.** With the arithmetic of [synthesis.md](synthesis.md) §2 and the rules below, the result is bit-identical to the original. This was checked against the original engine:
- 49 post-filter cases;
- 79 reverb cases: every preset at 22050 and 8000 Hz, plus custom presets;
- 8,156 sentence frames through all 17 shipped variants, including whisper on top of Hall, pauses, frames with no samples and buffer retries;
- 99 variant-switching states.

## 1. The effects

| Effect | Record (voice-format.md §3) | Shipped in | Section |
|---|---|---|---|
| Whisper | 1 | the five Whisper variants | 3 |
| FIR post-filter | 2 | the Whisper variants: {0.25, −0.5, 0.25} | 4 |
| Reverb | 4 | Hall (preset 3), Stadium (5), Space (6), RoboSoft (1, 2, 7) | 5 |
| Monotone | 5 | RoboSoft Six, Five, One, Two | 6 |
| All-pole post-filter, reverb loop filter | 3, 6 | none | 4, 5.6 |

The telephone voices (8000 Hz) have only their hidden whisper variant. Every rule here applies to them at their own rate.

## 2. Where the effects run

Per frame ([synthesis.md](synthesis.md) §1, step 7), in place over the frame's samples:

| Frame | Whisper | Post-filter | Reverb |
|---|---|---|---|
| Speech | during decoding (section 3) | after the vocal-tract filter | after the post-filter |
| Speech with no samples | during decoding | does nothing | runs: new length scale and a random number (5.3) |
| Silence: `SIL`, pauses, `\Pau=0\` | – | doesn't run; its history is untouched | runs on the zeros, so tails ring through pauses |

- **Buffer retries** ([synthesis.md](synthesis.md) §9). The retry happens before any effect, so the post-filter and the reverb run once per frame, on the attempt that succeeds. Only the noise position moves twice.
- **Clipping.** The reverb clips its output to [−32768, 32767] (5.5). The normal clip and 16-bit conversion follow ([synthesis.md](synthesis.md) §10).

## 3. Whisper

With whisper on:
- **Noise for every frame.** Every stored frame of every unit takes the unvoiced path of [voice-format.md](voice-format.md) §8: noise from the noise table times `gain · 0.02`. The noise position advances for every frame.
- **Voiced frames become unvoiced.** A voiced frame's period is negated, so the period plan treats it as unvoiced: its output period is the frame's own length, and the pitch contour doesn't apply. A repeat of it is played reversed ([synthesis.md](synthesis.md) §5–6).

So monotone and pitch changes have no audible effect under whisper; only duration and amplitude do. Whisper always comes with the FIR post-filter in the shipped variants.

## 4. Post-filters

Each post-filter has a history `h` of L floats, owned by the engine. It is zeroed every time the record is applied (section 7). `c` holds the record's L coefficients. Arithmetic follows [synthesis.md](synthesis.md) §2: products of two floats are exact, and sums are rounded to double.

**FIR (type 2)**, per sample x:
```
h[0] = x
acc = c[0]·x
for k = L−1 down to 1:
    acc = c[k]·h[k] + acc
    h[k] = h[k−1]
y = f32(acc)
```
Inside the loop, `h[k]` is the input from k samples ago. The shipped filter gives `y = f32((0.25·x[n] + 0.25·x[n−2]) − 0.5·x[n−1])`: a second-order high-pass with gain sin²(ω/2), 0 at DC and 1 at the Nyquist frequency.

**All-pole (type 3)**, per sample x:
```
acc = c[0]·x
for k = L−1 down to 1:
    h[k] = h[k−1]
    acc = c[k]·h[k] + acc
y = f32(acc)
h[0] = y
```
Inside the loop, `h[k]` is the output from k samples ago; `h[L−1]` is written but never read. Types 2 and 3 replace each other and share the history.

The history carries across frames, silence frames and texts. So the first two samples after a pause still see the last samples before it.

## 5. Reverb

### 5.1 Presets

Each preset has an input (wet) gain, a dry gain, one to four allpass lines (delay, gain) and a length modulation. All values are float32. The delays and the line lengths in samples are:

| Preset | Used by | Wet dB | Dry dB | Line delays, ms | Line dB | Samples at 22050 Hz | At 8000 Hz | Modulation |
|---|---|---|---|---|---|---|---|---|
| 1 | RoboSoft Five, Two | 7 | 9 | 10.0 | −0.5 | 220 | 80 | 0 |
| 2 | RoboSoft Four, Three | 0 | 0 | 10.6 | −10.498 | 233 | 84 | 0 |
| 3 | Hall | −11 | −4 | 30.6, 20.83, 14.85, 10.98 | −2.498, −2.2533, −2.7551, −2.5828 | 674, 459, 327, 242 | 244, 166, 118, 87 | 0 |
| 4 | (none) | −5 | −5 | 81.2, 55.3, 35.7, 21.96 | as preset 3 | 1790, 1219, 787, 484 | 649, 442, 285, 175 | 0 |
| 5 | Stadium | −3 | −5 | 162.4, 110.6, 71.4, 43.92 | as preset 3 | 3580, 2438, 1574, 968 | 1299, 884, 571, 351 | 0 |
| 6 | Space | −10 | 0 | 400.6 | −10.498 | 8833 | 3204 | 0 |
| 7 | RoboSoft Six, One | 6.5 | 9 | 10.0 | −0.5 | 220 | 80 | 0.05 |

The converter copies the presets into the data pack (section 9).

**What they sound like:**
- **Hall and Stadium** are four diffusing allpasses in series, decaying in about 0.7 s and 3.9 s.
- **Space** is one echo line: the direct sound at 0.906, then echoes every 400.6 ms at 0.288, 0.086, 0.026, ….
- **Presets 1 and 7** add a narrow resonance at multiples of about 100 Hz, roughly 18 dB above the rest: the metallic RoboSoft ring. Preset 7 moves it at random every frame, to between about 100 and 167 Hz.
- **Preset 2** is a mild comb with peaks every 94.6 Hz.

### 5.2 Setup

The reverb is created the first time a type 4 record is applied. For the shipped files that is always when the engine starts.
- **Gains.** `gain(d) = 0` if `d < −110` or d is NaN, otherwise `f32(10^(d·0.05))`, with d·0.05 in double. The converter computes them exactly and the data pack holds the float32 gains ([datapack.md](datapack.md) §3), so the engine computes no powers. Any accurate `pow` gives the same floats: the closest one lies 7.9·10⁻¹⁰ (relative) from a float32 rounding boundary. The resulting bits:

  | dB | 7 | 9 | 0 | −11 | −4 | −5 | −3 | −10 | 6.5 | −0.5 | −10.498 | −2.498 | −2.2533 | −2.7551 | −2.5828 |
  |---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
  | float32 | 400f4735 | 40346063 | 3f800000 | 3e904d1c | 3f21866c | 3f0ff59a | 3f353bef | 3ea1e89b | 40074368 | 3f71adf9 | 3e98e304 | 3f400463 | 3f4580ea | 3f3a6ab1 | 3f3e26cf |

- **Line lengths.** `samplesPerMillisecond = f32(rate · f32(0.001))`: 22.050001 at 22050 Hz, exactly 8 at 8000 Hz. Each line's length is `max(2, trunc(delay · samplesPerMillisecond))`, the product being exact.
- **Initial state.** Each line has a buffer of that many zeros and a position p = 0. The random value r starts at 0.

### 5.3 Per frame

For x = the frame's n samples (n may be 0):
```
if the preset has lines:
    lengthScale = f32(1 − modulation·r)
if n > 0:
    wet[t] = f32(wetGain·x[t]), or 0 if !(wetGain > 0)
    run every line in order over wet (5.4)
    mix wet into x (5.5)
r = f32(rand() / 4096)
```
The random number is drawn for every frame while the reverb is on, including frames with no samples. Processing in blocks makes no difference.

**Random numbers.** `rand` is the MSVC generator:
- `state = state·214013 + 2531011` (mod 2³²);
- the value is `(state >> 16) & 0x7FFF`.

Each engine has its own state, starting at 1 when the engine starts, and the reverb is its only user. The noise table's generator is separate ([synthesis.md](synthesis.md) §3). So frame k after the engine starts uses the (k−1)th value, and frame 1 uses r = 0. Only preset 7 is affected: its line lengths at 22050 Hz go 220, 219, 170, 202, ….

### 5.4 An allpass line

Each line has a gain g, a length, a buffer and a position p that carries from frame to frame:
```
end = trunc(length · lengthScale)
for each sample in:
    d = buffer[p]
    v = d·g + in                         (double)
    s = f32(v)
    if v > 0:  if !(s ≥ 0.001): s = +0
    else:      if s > −0.001:    s = +0   (NaN is kept)
    buffer[p] = s
    out = f32(d − s·g)
    p = p + 1; if p ≥ end: p = 0
```
- **`end`** must be computed as an exact double product before truncating. For example, 80 × f32(0.95) is 75.9999990, which truncates to 75; a float32 multiply would round it to 76 first.
- **A shrinking length (preset 7).** When the length shrinks below the current position, the line still processes one sample at that position before it wraps.

### 5.5 Mix and clip

```
if !(dryGain > 0): x is unchanged and the wet signal is dropped
else:
    v = dryGain·x + wet                  (double)
    x = −32768 if !(v ≥ −32768); 32767 if v > 32767; else f32(v)
```
The loud presets do clip. In a test sentence, RoboSoft Four clipped 164 samples and RoboSoft Six 48.

### 5.6 Loop filter (type 6, not shipped)

A type 6 record puts an FIR with taps `T[0..M−1]` inside every line, applied to the value stored in the buffer. Each line keeps a history `H` of M floats, zeroed when the record is applied:
```
shift H up by one; H[0] = s
acc = +0
for k = 0..M−1: acc = f32(H[k]·T[k] + acc)
buffer[p] = acc            (out still uses s)
```
It needs an existing reverb. `"Normal"` switches it off.

### 5.7 Lifetime

The reverb is never reset or re-created, and its preset never changes:
- **Applying another type 4** only switches the reverb back on.
- **Switched off** (by `"Normal"`, until the mode's own variant switches it back on), it doesn't run at all: no samples, no random number.
- **No tail is flushed.** It is cut at the end of a text's last frame, normally the final `SIL`, and continues under the next text's first frames, also after `AudioReset`. Space's 400.6 ms echo can outlast the final pause.

## 6. Monotone

- **Sources.** Record 5 of the mode's own variant (RoboSoft Six, Five, One, Two), or the name `"Monotone"` in `\Chr=`.
- **Effect.** Every pitch point of every phone unit the front end posts becomes the voice's current pitch in Hz, unquantised ([prosody.md](prosody.md) §10.5).
- **The current pitch** is the mode's default, which carries the variant's override (100, 120, 75 and 120 Hz for the four RoboSoft voices). It changes with `\Pit=`, `PitchSet` and `Inject`.
  - The original reads it as each unit is posted, so a `PitchSet` arriving mid-sentence can change the pitch mid-sentence, depending on timing.
  - The Go engine reads it once per sentence.
- **Timing.** `\Chr="Monotone"\` inside a sentence applies from the unit after the tag.
- **Variants reached by tag.** A record 5 in such a variant is never applied.

## 7. Switching

**At engine start** the mode's own variant applies its records: types 1, 2 and 4 in the back end, type 5 in the front end. A base mode has none.

**`\Chr=` in `TextData`.** Every tag gives `"Normal"` followed by each accepted name ([text.md](text.md) §3.3), even when the rest of the tag is malformed. The names reach the back end together, at the tag's position: after the last frame of the words before the tag, before the first frame after it.

| Name (case-insensitive) | Back end | Front end |
|---|---|---|
| `"Normal"` | whisper, post-filter and reverb off (the post-filter history is discarded; the reverb keeps its state), then the mode's own variant again. On a Whisper mode, whisper and the post-filter come back with a zeroed history; on a reverb mode, the reverb comes back as it was | monotone off, then the own variant's record 5 again |
| `"Monotone"` | – | monotone on |
| any other | applies the first variant of the same base voice whose character tag is `\Chr=` + the name with its quotes + `\` (case-insensitive), **on top** of the current effects: whisper, post-filter (zeroed history), reverb, loop filter | – |

- **Whisper on every mode.** Each shipped base voice has exactly one tagged variant, its Whisper. So `\Chr="Whisper"\` works on every mode, including Sam and the telephone voices, and stacks: Mary in Hall whispers in the hall.
- **The reverb and robot variants** have no character tag, so no tag can reach them, and no tag can create a reverb.
- **Lasting changes.** `\Chr="Normal"\` leaves a RoboSoft voice monotone and robotic, and a Whisper mode whispering.
- **Unknown names** do nothing beyond the leading `"Normal"`.
- **Case folding.** The back end compares names with `_stricmp`, so case folding is ASCII. The original uses the system locale's lowercase; a Turkish locale's dotless ı would break the match. The front end's `"Normal"` and `"Monotone"` compare as tag names do ([text.md](text.md) §3.3): `\Chr="Monotone"\` with a soft hyphen inside the name flattens the pitch, but `\Chr="Whisper"\` with one doesn't whisper.

**`\Rst\` in text** does the same as `"Normal"` in every stage, besides its speed, volume and pitch resets ([units.md](units.md) §10). `\Rst\` in `Inject` doesn't touch the effects.

## 8. State

Per engine instance:

| State | Starts at | Changed by | Reset by |
|---|---|---|---|
| Whisper, post-filter, reverb on/off | the own variant | `\Chr=` names | `"Normal"` and `\Rst\`, which re-apply the own variant |
| Post-filter history | zeros | speech frames with samples | zeroed whenever the record is applied, so by every `\Chr=` and `\Rst\` on a Whisper mode |
| Reverb lines (contents, positions) | zeros | every frame with samples | never |
| Reverb random value and generator | 0 and seed 1 | every frame while the reverb is on | never |
| Monotone | the own variant | `"Monotone"` | `"Normal"` and `\Rst\`, which re-apply the own variant |

- **Continuity.** Nothing here is reset by `TextData`, the end of a text or `AudioReset`. So, as in [synthesis.md](synthesis.md) §12, consecutive texts sound as one stream.
- **`AudioReset`.** Which `\Chr=` and `\Rst\` of the stopped text still take effect, and in which stage, is in [engine.md](engine.md) §12. The front end's monotone and the back end's effects can disagree afterwards, until the next `\Chr=` or `\Rst\`: the front end posts sentences whose batches the voice then drops.

## 9. For the data pack

- **Records.** The converter checks every variant record and rejects anything the shipped files don't contain:
  - only types 1, 2, 4 and 5;
  - exact sizes: types 1 and 5 have no data, type 2 is 4 + 4n bytes with n ≥ 1, type 4 is 4 bytes with a preset of 1 to 7;
  - at most one type 4 per variant;
  - a character tag that is empty or `\Chr="name"\`, with at most 78 characters inside the quotes.

  The original never checks sizes, and the cases it doesn't handle crash it or corrupt memory: a 0-tap filter, a preset outside 1–7, a failed reverb set-up.
- **Reverb presets.** The seven presets of section 5.1: the delays in milliseconds, the gains as linear float32 values (5.2) and the modulations ([datapack.md](datapack.md) §3).
- **Post-filter coefficients** come from the records themselves.

Constants in this spec: f32(0.001), 0.05, 10, −110, 0.001, −0.001, 1/4096, 214013, 2531011, 32767 and −32768.

## 10. Quirks

**Reproduce these.** Each one changes the audio.
1. The whisper post-filter's history crosses pauses and texts, and every `\Chr=` or `\Rst\` on a Whisper mode zeroes it.
2. Reverb tails ring through pauses and carry into the next text, also after `AudioReset`. No tail is flushed at the end.
3. The reverb draws a random number for every frame, including frames with no samples. Preset 7's line length follows that sequence, including the one-sample overrun when it shrinks.
4. The reverb clips before the normal clip.
5. `\Chr="Normal"\` doesn't undo the mode's own effects, and `\Chr="Whisper"\` stacks on them.

**Don't reproduce:**
- Malformed records (section 9): the converter rejects them.
- Very long standalone `\Chr=` names, which overflow a buffer in the original's front end. We treat any unknown name as doing nothing after the `"Normal"`.
- Locale-dependent case folding.
- The timing-dependent `PitchSet` mid-sentence under monotone.

## 11. Open questions

To be checked against the original engine:
1. **Tails across texts.** That reverb tails continue into the next text and after `AudioReset` hasn't been heard. Record Mary in Stadium speaking two `TextData` calls. *Answered:* texts that follow one another on reverb modes behave as described ([engine.md](engine.md) §11), and so do the texts after an `AudioReset` on Mary in Hall, Mary in Stadium and Mike in Hall, Stadium and Space (§12).
2. **The random sequence.** Confirm that RoboSoft Six and One count frames from `Select`, and that a second engine starts again from seed 1. The first half is answered: every corpus sentence spoken by a fresh engine matches counting from `Select` at seed 1. A second engine in the same process is still to be checked.
3. **The host's FPU settings.** The own variant's reverb gains are computed on the thread that selects the mode. A host running the FPU at 24-bit precision would change them, as with the tables in [synthesis.md](synthesis.md) §16.
