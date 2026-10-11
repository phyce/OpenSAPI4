# SAPI 4 Microsoft engine: units

How the Microsoft Speech Synthesis Engine (`msttssyn.dll`) turns the unit records of a sentence ([prosody.md](prosody.md) §10) into **frames**: which recorded unit each phone plays, how long it lasts, its pitch and amplitude points, how speed and volume apply, Sam's special phrase units, where tags and pauses land in the stream, and the `Visual` (lip-sync) events. The voice data is described in [voice-format.md](voice-format.md) §4–7. How a frame is played (period repeat and skip, pitch, gain, resampling) is in `synthesis.md`; the variant effects are in `effects.md`.

**What the synthesiser receives.** For each sentence, an ordered stream of:
- **Frames**, one per phone, one per pause, and in slow mode one extra pause after most words. A frame has a phone name, a unit number (0 = silence), a duration in seconds, and up to five points of (position in the frame, pitch in Hz, amplitude).
- **Events and controls** between frames: word positions, bookmarks, `\Pau=` pauses, `\Chr=` changes, `\Rst\`.

Speed changes durations only. Volume changes amplitudes only. Pitch is already in the unit records. The 8 kHz voices share their trees and senone statistics with the 22 kHz ones, so their frames are identical.

## 1. Pipeline

For each sentence's batch of unit records:

| # | Step | Section |
|---|---|---|
| 1 | Rename phones | 3 |
| 2 | Look up a tree leaf for every phone | 4 |
| 3 | Sam only: alternative units | 5 |
| 4 | Reset the pitch and amplitude carries | 7 |
| 5 | Walk the records in order. A word becomes frames (durations, gains, points, slow pause) and is sent. A tag is applied or sent when it is reached | 2, 6–10 |

## 2. Records and words

Each record is one of:
- **Boundary:** name `#`.
- **Tag:** name starting with `\` (prosody.md §10.5).
- **Phone:** anything else, `SIL` included. Its number comes from the voice's phone table (voice-format.md §4).

A **word** is a maximal run of consecutive phone records. A boundary or a tag ends it. The FrontEnd always writes `#`, `SIL`, `#`, so every `SIL` is a word on its own, and every batch starts and ends with `#`. A prosody transplant (engine.md §13) can put `SIL` inside a word and end a batch with a phone.

**Output order.** The output is in record order, one word at a time:
- the frames of a word are sent together, then its slow pause (section 8);
- a tag's message comes after the frames of every word before it and before the frames of every word after it.

## 3. Renaming

Every record whose name is exactly one of these is renamed:

| From | To |
|---|---|
| `DX` | `T` |
| `AW0` | `AW1` |
| `AY0` | `AY1` |
| `EY0` | `EY1` |
| `OY0` | `OY1` |

So Mary and Mike never use their `AW0`, `AY0`, `EY0` and `OY0` senones, and Sam, who has no such phones, never needs them.

## 4. Tree lookup

For every phone record of the batch, in order:
- **Left context:** the previous phone record, skipping boundaries and tags; `SIL` for the first phone.
- **Right context:** the next phone record, the same way; `SIL` for the last phone.
- **Word position:**

  | Record just before | Record just after | Position |
  |---|---|---|
  | boundary or tag | boundary or tag | single-phone word (4) |
  | boundary or tag | phone | word-initial (1) |
  | phone | boundary or tag | word-final (2) |
  | phone | phone | inside a word (8) |

  Before the first record counts as a phone. After the last record, the voice reads the next record of the front end's posting buffer (engine.md §13.6): a boundary if its name starts with `#` or `\`, otherwise (a zero record included) a phone. The FrontEnd's batches start and end with `#`, so the leading `SIL` is a single-phone word and neither edge is read; only a transplant can end a batch with a phone.

- **Leaf:** the tree lookup of voice-format.md §5, which maps context names starting with `+` or `SIL` to `SIL`.

So contexts cross word boundaries and pauses, and positions come from the neighbouring records only.

**Unit number** of a phone p with leaf l: `senoneOffsets[p] − phoneCount + l + 1` (1..3333), the unit of the same number in the inventory. `SIL` has no tree and always gets unit 0.

The FrontEnd never produces a name missing from the phone table, or `+NOISE+`, `SILb`, `SILe` (empty trees). A transplant can produce missing names (engine.md §13.4). The original crashes on them; the Go engine treats them as `SIL`.

## 5. Sam's alternative units

Only Sam has an `AltUnits` stream (voice-format.md §7). It runs after the tree lookup, on the whole batch.

1. **Phone string.** The phone numbers of every phone record in order, `SIL` included, with boundaries and tags left out. A match can therefore span words, pauses and tags.
2. **Matches.** For each start position i in ascending order, and for each entry listed under the phone at i in table order: if the entry's key equals the phones starting at i, the entry claims its span. The span is the key's positions, without the first one unless flag 1 is set, and without the last one unless flag 2 is set.
3. **Choice.** Each position keeps the first claim made on it: the earliest start, then the earlier entry. A match that kept every position of its span is applied. A match that lost any position is dropped.
4. **Applying a match.** Code k goes to the k-th position of the span. Unit `u = code & 0x1FFF`. If `code & 0xC000 == 0xC000`, the phone is replaced by the next of the entry's replacement phones (record name and phone number both change).
5. **New leaves.** After all replacements, every replaced phone and every phone next to one gets a new tree leaf (section 4), with its final neighbours as contexts.
6. **Frame unit.** A phone whose chosen unit is above 3333 plays that unit. Every other phone keeps its tree unit (`SIL` stays 0).

**Durations and gains still come from the tree leaf** (section 6), even for phones that play an alternative unit. A replaced phone and its neighbours use their new leaves.

The shipped entries:
- **"Merlin"** (`M ER1 L IH0 N`, flags 0): ER1, L and IH0 play units 3363, 3364 and 3365. "My name is Merlin." uses them.
- **The phrase** (`HH EH0 L OW1 SIL AY1 AE1 M M ER1 L IH0 N SIL Y AO1 R W IH1 N D OW0 Z AH0 S IH1 S T AH0 N T`, flags 3): units 3334–3362, with EH0 replaced by AW1 and OW0 by EY1. Inside the phrase it beats "Merlin", which starts later.

  With our front end the phrase never matches:
  - "Hello, I'm Merlin, your Windows assistant." gives `AY1 M` for "I'm", while the key has `AY1 AE1 M`.
  - After a comma pause, the glottal stop before a vowel-initial word becomes a second, 15 ms `SIL` (prosody.md §14, quirk 5). The key has one `SIL`.

  "Hello, I am Merlin, your Windows assistant." differs only by that second `SIL`, and the original produces it too: its audio for that sentence matches ours bit for bit on all five voices. So the phrase units never play from text. Only "Merlin" does.

**Converter checks.** The original's handling of every other `AltUnits` feature is broken: it reads past its lists, uses wrong indices and frees a moved block. No shipped entry uses one, so the converter rejects any entry that has:
- context lists (flags 4 or 8);
- insert (0x8000 alone), delete (0x4000 alone) or 0x2000 codes;
- a code count different from the span length;
- a replacement-phone count different from the number of replace codes;
- a replace code on the last position of the span (the original never gives the next phone a new leaf);
- a code whose unit is neither 0 on a `SIL` position nor an inventory unit above 3333;
- a replacement phone that is `SIL` or has no tree.

**Same entry back to back.** If the same entry is matched twice in a row, so that one span ends where the next begins, the original runs past the code list. The Go engine applies each match's codes on their own.

## 6. Durations

Let d be the record's duration field (float32), `scale` and `offset` the speed state (section 8), and `mean` the leaf's mean duration in milliseconds (voice-format.md §6). A d of 0 (or −0, or NaN) counts as 1.0.

| Record | d | Duration in seconds |
|---|---|---|
| phone | 0 < d ≤ 5 | `f32(f32(scale × mean × d) × f32(0.001))` |
| phone | d > 5 | `f32((d − 5) × mean × f32(0.001))`: not speed-scaled |
| phone | d < 0 | `f32(d × f32(−0.001))`, i.e. −d ms: not speed-scaled |
| `SIL` | 0 < d ≤ 5 | `f32(scale × d + offset)` |
| `SIL` | d > 5 | `f32((d − 5) × scale + offset)` |
| `SIL` | d < 0 | `f32(−scale × d × 0.001)`, with 0.001 a double: no offset |

**Precision.**
- Each expression is evaluated left to right in double precision (the original's x87 code runs at 53 bits), from float32 operands.
- `f32` rounds to float32. Each `f32` is a separate rounding.
- `f32(0.001)` is the float32 nearest 0.001.

The FrontEnd only sends phone codes 1.0, 1.2 and 1.3, and `SIL` lengths in seconds. So the other phone rows never apply, and the other `SIL` rows only for a pause of 0 s or longer than 5 s.

## 7. Pitch and amplitude points

Each phone record brings up to five points: a position in the phone (0 to 1), a pitch in Hz and an amplitude. A pitch or amplitude of 0 is unset. The FrontEnd gives each phone one point (position 1.0, its pitch P, amplitude 0) and `SIL` none. A prosody transplant (engine.md §13) gives up to five, with any positions and any mix of set and unset values. The voice fills in every unset value by interpolating in time across the word: first the pitches, then the amplitudes. The Go engine implements this general routine; section 7.4 gives what it comes to for the FrontEnd's records.

### 7.1 Gains

Before interpolating, each record gets its duration in seconds (section 6) and its amplitudes become gains:
- `SIL`: every point's amplitude becomes 1.0.
- Other phones: an amplitude of 0 becomes g, a positive one `f32(g × a)` in double, and a negative one stays. Here `g = f32(sqrt(gain))`, where gain is the leaf's float32 energy gain `f32(exp(c − d))` (voice-format.md §6) and sqrt is taken in double.

### 7.2 The interpolation

The voice keeps two **carries**:
- the pitch carry, an integer, where −1 means none;
- the amplitude carry, a float32, where −1.0 means none.

Both are none at the start of every batch, and become none again after a word whose last record is `SIL`. A carry that lands on −1 or −1.0 also counts as none.

For a word of n records, with `count[r]`, `position[r][j]`, `values[r][j]` (the pitches, then the amplitudes) and `duration[r]`, one pass is:

```
startTime = 0                    float32
time = 0                         double
phone = 0;  point = 0;  startPoint = 0
loop:
    if phone ≥ n − 1:
        if phone ≠ n − 1: stop
        if point ≥ count[n − 1] and count[n − 1] > 1: stop
    segmentStart = f32(time)

    # the next set value, from point `point` of record `phone` on
    found = false;  i = phone
    while i < n and not found:
        while point < count[i]:
            if values[i][point] ≠ 0: found = true; break
            point += 1
        if not found:
            if i + 1 ≠ n: point = 0
            time = time + duration[i];  i += 1
    if found:
        e = i;  k = point
        if position[e][k] ≠ 0:   endTime = f32(duration[e] · position[e][k] + time)
        elif e = 0:              endTime = f32(time)
        elif count[e − 1] = 0:   endTime = f32(duration[e − 1] + time)
        else:                    endTime = f32(position[e − 1][count[e − 1] − 1] · duration[e − 1] + time)
        endValue = values[e][k]
        if the carry is none: carry = values[e][k]           (pitch: trunc)
    else:
        e = n − 1;  k = count[n − 1]
        endTime = f32(time)
        if the carry is none: carry = 110 (pitch) or 110.0 (amplitude)
        endValue = carry                                     (pitch: as float32)

    # fill the span from (phone, startPoint) to (e, k)
    base = carry (pitch: as double)
    time = segmentStart;  storedTime = segmentStart;  lastWritten = none
    for r = phone .. e:
        if this is the pitch pass:
            if count[r] = 0:
                count[r] = 1;  point 0 = (0, 0, 0);  if r = e: k += 1
            elif position[r][0] ≠ 0:
                if count[r] = 5: count[r] = 4
                move the points one place up;  point 0 = (0, 0, 0);  count[r] += 1
                if r = e: k += 1
                time = storedTime
            if position[r][count[r] − 1] ≠ 1:
                if count[r] = 5: count[r] = 4
                point count[r] = (1, 0, 0);  count[r] += 1
        limit = k if r = e else count[r]
        reached = startPoint
        for j = startPoint .. limit − 1:
            moment = duration[r] · position[r][j] + time
            values[r][j] = f32(((moment − startTime) · (endValue − base)) / (endTime − startTime) + base)
            lastWritten = values[r][j];  reached = limit
        if reached = count[r]:
            time = duration[r] + time;  storedTime = f32(time)
        startPoint = 0
    if lastWritten isn't none: carry = lastWritten           (pitch: trunc)

    # step over the set values that follow
    point = k;  i = e
    while i < n:
        if count[i] = 0 or position[i][0] ≠ 0: break
        while point < count[i] and values[i][point] ≠ 0:
            carry = values[i][point]                         (pitch: trunc)
            startTime = f32(duration[i] · position[i][point] + time)
            point += 1
        if point < count[i] or position[i][point − 1] ≠ 1: break
        time = time + duration[i];  i += 1
        if i ≠ n: point = 0
    phone = i;  startPoint = point
```

So the pitch pass gives every phone a point at 0 and one at 1.0 (cutting a fifth point to make room), and both passes draw straight lines in time between the set values, through the phones without any. A value carried from the previous word starts the line.

### 7.3 Arithmetic

- `time` is a double. `f32` rounds to float32, and stored values (`startTime`, `endTime`, the filled values) are float32. Products and sums are in double.
- `trunc` is msvcrt `_ftol`: it truncates to a 64-bit integer and keeps the low 32 bits, so NaN, infinities and values beyond ±2^63 give 0.
- **Division** by zero follows IEEE: ±infinity, or NaN for 0/0. A span can end where it starts, for example at three points at position 1.0.
- **Comparisons.** The original's x87 compare sets the zero flag for unordered operands, so every test of a float against 0, 1 or none finds a NaN equal. So a NaN value counts as unset, a NaN `lastWritten` as none, and a NaN amplitude carry as none.
- Inside a span every value is unset by construction. If one weren't, the original would call `DebugBreak`; the Go engine keeps the value.
- **The work record** holds the positions, the pitches and the amplitudes back to back, five slots each, and the pitch pass can reach one slot past the pitches. Take a word's last phone with five points, the first not at 0, reached when no set pitch follows. The end point is its count, 5. Inserting the point at 0 cuts the fifth point and moves the end point to 6, so the pass fills slots up to 5: it reads position slot 5, which is pitch 0, and writes pitch slot 5, which is amplitude 0 (just cleared by the insertion). That amplitude becomes an interpolated pitch, in Hz, so the phone starts very loud. Only prosody transplants make such phones. Nothing reaches past the amplitudes.

### 7.4 What this gives for the FrontEnd's records

The FrontEnd's phones each have one point at position 1.0 and amplitude 0, so for them the routine reduces exactly to the rules below.

**A word of phones** P₀…Pₙ₋₁ with gains g₀…gₙ₋₁. Each phone k gets two points:

| Point | Position | Pitch | Amplitude |
|---|---|---|---|
| 0 | 0 | c_k | a_k |
| 1 | 1.0 | P_k | g_k |

where:
- c₀ = the pitch carry, or `trunc(P₀)` if none;
- a₀ = the amplitude carry, or g₀ if none;
- for k ≥ 1, c_k = `trunc(Pₖ₋₁)` and a_k = gₖ₋₁.

After the word, the pitch carry is `trunc(Pₙ₋₁)` and the amplitude carry is gₙ₋₁.

**A `SIL` word** gets two points, (0, c, a) and (1.0, c, a):
- c is the pitch carry, or 110 if none;
- a is the amplitude carry, or 110.0 if none.

Both carries are then reset.

Consequences:
- The leading `SIL` of a sentence has pitch 110 and amplitude 110.0.
- A pause inside a sentence keeps the pitch and gain the previous word ended on.
- The first phone after any pause is flat at `trunc(P)`.
- Each phone glides from where the previous one ended to its own pitch.

### 7.5 Frame values

- Pitches become 16-bit integers: `trunc`, keeping the low 16 bits.
- Amplitudes become `f32(a × v)`, where v is the volume factor (section 9).

## 8. Speed

**Base speed.** The mode's default speed, from the merged header (voice-format.md §3): 170 for Mary and Mike, 150 for Sam, 130 for RoboSoft One, Two, Five and Six.

**Threshold:** `threshold = trunc(base × f32(2/3))` in double. That gives 113, 100 and 86. f32(2/3) is slightly above 2/3, so 150 gives 100 rather than 99.

**Scale and offset**, recomputed whenever the speed changes:

```
slow = speed <= threshold
if slow:
    x = float64(threshold - speed) / (float64(threshold) - 20) + 1.5
    scale = f32(x)
    offset = f32((1.0/float64(speed) - float64(scale)/float64(base)) * 55)
else:
    scale = f32(float64(base) / float64(speed))
    offset = 0
```

With speeds of at least 30, x stays below 2.5 and the offset is never negative. The original's 2.5 cap and its unused "measured rate" mode are therefore omitted.

| Base (threshold) | Speed | Slow | scale | offset (s) |
|---|---|---|---|---|
| 170 (113) | 30 | yes | 2.39247322 | 1.05929792 |
| | 100 | yes | 1.63978493 | 0.019481346 |
| | 113 | yes | 1.5 | 0.00143154606 |
| | 114 | no | 1.4912281 | 0 |
| | 300 | no | 0.566666663 | 0 |
| 150 (100) | 30 | yes | 2.375 | 0.962499976 |
| | 100 | yes | 1.5 | 0 |
| | 101 | no | 1.48514855 | 0 |
| 130 (86) | 86 | yes | 1.5 | 0.00491949916 |

**Slow pause.** In slow mode, a pause frame follows every word whose next phone is not `SIL`, including the leading `SIL` word. The next phone is the first phone record after the word, skipping boundaries and tags; at the end of the batch it counts as `SIL`. The pause frame is:
- name `SIL`, unit 0;
- duration `offset`;
- no points, except that slot 0 holds position 1.0, the word's last frame pitch and amplitude 0.

A `\Pau=` between two words doesn't prevent it. For Sam at 100 wpm the offset is exactly 0, so its pause frames are empty.

**When a change applies.**
- A new speed applies from the next word the engine builds, whether it comes from `SpeedSet` or a `\Spd=` tag. Words already built keep the old speed.
- Speed persists across sentences, `TextData` calls and `AudioReset`. Only `SpeedSet`, `\Spd=` and `\Rst\` change it.
- The analyser never sees speed (prosody.md §11).

**`\Spd=n\` in text** follows `SpeedSet`'s rules (interface.md §5):
- 0 means 30;
- 4294967295 means three times base;
- a value outside [30, 3 × base] is ignored silently, with no `AttribChanged`.

The number is read as an unsigned 32-bit value that wraps, so 4294967296 is 0.

## 9. Volume

`volume` is the low 16 bits of the last value set. It starts at 0xFFFF for every voice, persists like speed, and `\Rst\` sets it back to 0xFFFF.

When a word starts, the engine computes the factor `v = f32(volume × f32(1/65535))` in double. 0xFFFF gives exactly 1.0, and 0x8000 gives 0.500007629. Every amplitude of the word's frames is multiplied by v (section 7). A `\Vol=` therefore applies from the next word.

`\Vol=n\` always succeeds and keeps the low 16 bits, so `\Vol=65536\` is silence.

## 10. Tags in the stream

The voice reads a tag record by its text: a keyword after `\`, up to `,`, `:`, `;`, `=`, `\` or `<`, in any case. A number must follow `=` with at least one digit, wrap in 32 bits and end with `\`. A quoted value runs from `="` to the next `"`, which `\` must follow. `\Rst` needs a `\` anywhere after it.

| Tag record | Effect here | Sent on |
|---|---|---|
| `\Wrd=n\` | | word position n |
| `\Mrk=n\` | | bookmark n |
| `\Pau=n\` | | pause n ms |
| `\Chr="x"\` | | character change; the name is lowercased, ASCII letters only (the original's `CharLowerA` depends on the locale; [effects.md](effects.md) §7) |
| `\Rst\` | volume reset, then speed reset | reset ([effects.md](effects.md) §7) |
| `\Spd=n\` | speed (section 8) | no |
| `\Vol=n\` | volume (section 9) | no |
| anything that fails to parse | | dropped |

**Controls outside sentences.** Tags in a stretch of text with no words reach this stage as standalone controls between sentences, and act the same way. The only difference is `\Rst\`: a standalone one resets speed first, then volume.

**`AttribChanged` for `\Rst\` in text.**
1. PITCH, sent when the text is read, ahead of the audio.
2. Then VOLUME, SPEED for a reset inside a sentence, or SPEED, VOLUME for a standalone one.

`Inject` sends SPEED, VOLUME, PITCH (interface.md §7). Every successful set sends its notification, even when the value is unchanged.

**Pause** (`\Pau=n\`). The synthesiser plays a silent frame:
- name `SIL`, unit 0;
- duration `f32(n × f32(0.001))`, computed in double;
- no points.

It is not scaled by speed or volume. `\Pau=0\` plays nothing and sends no `Visual`. The FrontEnd writes the tag as `#`, `\Pau=n\`, `#`, so the pause sits exactly between the frames of the words around it: after the previous word's slow pause, if there is one.

**Event positions.** A word position or bookmark fires when playback reaches its place in the stream, after all the audio of the frames before it. Its sample offset is therefore the real synthesised length of everything before it (`synthesis.md`).

## 11. Visual events

**When.** One `Visual` per frame that produces audio: phones, `SIL`, pauses and slow pauses. It is sent when playback reaches the start of that frame's audio. A frame that produces no samples sends none. It is never sent for audio flushed by a reset (interface.md §8).

**Values.**

| Value | W sink | A sink |
|---|---|---|
| IPA phoneme | the entry's IPA character | 0 |
| engine phoneme | the entry's engine character | the same |
| hints | 0 | 0 |
| mouth | the entry's mouth shape (`TTSMOUTH`) | the same |

**Which entry.** The table has 54 entries, each with a name, an IPA character, an engine character and one of 25 mouth shapes. The converter resolves each phone of each voice to an entry ahead of time. It takes the phone name's leading letters (n of them, so the stress digit is dropped), compares them case-insensitively with the first n characters of each entry name, and takes the first entry that matches.

This gives:
- **Stress:** stressed and unstressed vowels look the same.
- **AH:** AH0 and AH1 both get schwa (ə). The ʌ entry comes later, so it is never reached. The same happens to ER, which gets ɚ.
- **Diphthongs:** a diphthong's IPA value is the last character of its symbol: AW → ʊ, AY → ɪ, EY → ɪ, OW → ʊ, OY → i.
- **`SIL`** (pauses included): IPA 0, engine character space, mouth shape 1.

`SILb`, `SILe` and unknown names find no entry, and the original then reads outside its table. They never reach a frame; the Go engine would use the `SIL` entry. The original passes a pointer into its own writable mouth table. The Go engine passes a copy.

## 12. State

Per engine instance:
- **Speed:** current speed, base, threshold, scale, offset, slow.
- **Volume.**
- **Carries:** the pitch and amplitude carries (reset every sentence).

The original resets nothing else at buffer starts, buffer ends or `AudioReset`.

## 13. Quirks

**Reproduce these.** Each one changes the frames or events.
1. The leading `SIL` gets pitch 110 and amplitude 110.0. Every other `SIL` keeps the previous word's last pitch and amplitude.
2. Pitch carries are truncated to integers, and the first phone after a pause is flat.
3. A duration of 0 means 1.0. Phone codes above 5 and below 0 are not speed-scaled. A negative `SIL` code gets no offset.
4. Slow pause frames follow every word not followed by `SIL`, including the leading `SIL`. Sam's are empty at 100 wpm.
5. Alternative units take their durations and gains from tree leaves.
6. Speed and volume change at word boundaries and survive `AudioReset`. Invalid `\Spd=` values are ignored silently, and `\Vol=` keeps 16 bits.
7. `Visual`:
   - one per frame with audio;
   - prefix matching (AH → ə, ER → ɚ);
   - diphthongs send their last IPA character;
   - A sinks get IPA 0.
8. The order of `AttribChanged` for `\Rst\` (section 10).
9. A batch that ends with a phone reads the record after it in the front end's buffer (section 4).
10. A NaN equals everything in the interpolation's tests, and the pitch pass can write one slot past a phone's pitches, into its first amplitude (section 7.3).

**Don't reproduce:**
- crashes on unknown or empty-tree phones;
- every `AltUnits` path no shipped entry uses (section 5);
- the race between `SpeedSet` on the caller's thread and a word being built (the Go engine applies a change between words);
- the out-of-bounds `Visual` entry and the shared writable mouth table;
- the unused "measured rate" speed mode and the unreachable 2.5 cap.

## 14. For the data pack

Per base voice:
- the phone table and trees (voice-format.md §11);
- per senone, the mean duration and the float32 energy gain `exp(c − d)`. Computed in double, no gain lies near a float32 rounding boundary, so any correct `exp` gives the same values;
- Sam's alternative units, after the checks of section 5;
- per phone, its `Visual` entry: IPA character, engine character and the 8 mouth-shape bytes.

Constants in this spec: 110 and 110.0, 2/3, 20, 1.5, 55, the float32 0.001 and 1/65535, and 5.

## 15. Open questions

The answered ones were settled against the original's audio and `Visual` events, through its COM interface:
1. **Sam's phrase.** Does "Hello, I am Merlin, your Windows assistant." give one `SIL` or two after "Hello,"? This decides whether the phrase units ever play. *Answered:* two, so they never play (section 5).
2. **Frames.** Compare our frames field by field with the engine's: "Hello world." on Mike at 170, 100 and 300 wpm, and "My name is Merlin." on Sam. *Answered:* the audio and every `Visual` position match, which fixes each frame's sample count. These sentences, and the 69-sentence corpus on every voice at three speeds and three pitches, are bit-identical.
3. **`Visual` count.** One per frame for "Hello." on each voice. *Answered:* one per frame with samples, at the frame's first sample, on every voice and corpus sentence.
4. **`AudioReset`** with tags in the discarded text: the original still applies the `\Spd=`, `\Vol=` and `\Rst\` it finds there. *Answered:* those of every batch the voice had started, and every standalone one ([engine.md](engine.md) §12).

Answered by synthesis.md:
- The synthesiser never reads point slots beyond the count, or a `SIL` frame's points (§8).
- Amplitude 0 plays silence, apart from the filter's decaying tail of the sound before it (§6–7).
- The engine's threads run at 53-bit precision (§2).
- Resampled output can also lose a `Visual` to a short frame: a text's first frame, when it is no longer than the resampler's history (§13.2).
