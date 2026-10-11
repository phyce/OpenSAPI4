# SAPI 4 Microsoft engine: voice files

The formats of the voice files the Microsoft Speech Synthesis Engine (`msttssyn.dll`) reads, and what every field means to the engine. The converter reads these files and writes our data pack; the Go engine reads only the data pack. Behaviour at the interface is in [interface.md](interface.md). How units are chosen, how they are stretched and pitched, and the effects are in [units.md](units.md), [synthesis.md](synthesis.md) and [effects.md](effects.md).

Where the original has a bug that the data could trigger, this spec says what correct behaviour is. Those cases are listed in "Quirks we don't reproduce". Parts of the format that no shipped file uses are described only as far as needed to reject them.

## 1. Files

The engine's folder holds five voices, each as a `.vce` file plus a `.cfg` file with the same name: `female`, `female8`, `male`, `male8`, `sam`. The `8` files are the 8000 Hz telephone voices.

Every file is an OLE compound file (`D0 CF 11 E0 A1 B1 1A E1`) with all streams in the root storage. Stream names are compared case-insensitively.

Byte order is little-endian everywhere except `TreeImage`, which is big-endian. Header strings are fixed UTF-16LE arrays. Phone names in the other streams are ASCII.

| Stream | File | Bytes (female / female8 / male / male8 / sam) | Contents |
|---|---|---|---|
| `VcHeader` | `.vce`, `.cfg` | 2296 | Voice header (section 2) |
| `CFG1`…`CFG7` | `.cfg` | 2856–2876 | Variants (section 3) |
| `PhoneFile` | `.vce` | 1856 / 1856 / 1856 / 1856 / 1456 | Phone set, text (section 4) |
| `TreeImage` | `.vce` | 112978 ×4 / 113335 | Context decision trees (section 5) |
| `Senone` | `.vce` | 93328 | Per-senone duration and energy (section 6) |
| `AltUnits` | `sam.vce` only | 328 | Special-case unit sequences (section 7) |
| `Inventory` | `.vce` | 1823046 / 1254816 / 1710079 / 1170628 / 1395733 | Unit inventory: LPC vocoder data (section 8) |
| `Cdb` | `.vce` | 920 / 920 / 917 / 917 / 917 | Not read by the engine (section 9) |

Identical bytes:
- `PhoneFile` and `TreeImage` are the same in female, female8, male and male8.
- `Senone` is shared by female and female8, and by male and male8.
- Sam differs in every stream.

## 2. `VcHeader`

The stream is 0x8f8 bytes.

| Offset | Type | Field | Meaning |
|---|---|---|---|
| 0x000 | GUID | signature | Header format, see below |
| 0x010 | WCHAR[262] | manufacturer | Reported as the mode's manufacturer |
| 0x21c | WCHAR[262] | modeName | Mode name |
| 0x428 | GUID | modeId | Mode GUID |
| 0x438 | WORD + WCHAR[64] | language | LANGID and dialect |
| 0x4ba | WCHAR[262] | speaker | Reported only |
| 0x6c6 | WCHAR[262] | style | Reported only |
| 0x8d2 | WORD | gender | 1 female, 2 male |
| 0x8d4 | WORD | age | Reported only |
| 0x8d6 | WORD | defaultSpeed | Words per minute |
| 0x8d8 | WORD | defaultPitch | Hz |
| 0x8da | WORD | | Padding, never read |
| 0x8dc | DWORD | features | 0x100 PC-optimised, 0x200 phone-optimised; 0x8000 see below |
| 0x8e0 | WAVEFORMATEX | waveFormat | 18 bytes: the voice's native output format |
| 0x8f2 | WORD | | Padding, never read |
| 0x8f4 | SHORT | variantCount | Number of `CFG` streams in this file |
| 0x8f6 | SHORT | preferenceRank | Used to pick PREFERRED modes (interface.md §2) |

Signatures:

| Signature | Meaning |
|---|---|
| `{42FF5880-91E0-11D1-B1CD-0000F803E456}` | Current format. variantCount and preferenceRank are valid |
| `{C77C50A3-2867-11D0-847B-444553540000}` | Older format (`sam.vce`). variantCount and preferenceRank count as 0 |
| `{11825730-70C8-11D1-B1CB-0000F803E456}` | Like the older format, but `TreeImage` uses format 2. No shipped file; the converter rejects it |

Any other signature, or a short read, means the file is not a voice.

`features & 0x8000` would make the engine refuse to load the voice's inventory. No shipped header sets it; the converter rejects it.

All ten headers share: manufacturer `Microsoft`, LANGID 0x0409 `US English`, style `Clear low-accented North American English native`, age 30, and PCM mono 16-bit wave formats.

| File | Signature | modeName | modeId | Gender | Speed | Pitch | Features | Rate | variantCount | Rank |
|---|---|---|---|---|---|---|---|---|---|---|
| female.vce | current | Mary | `C77C5170-2867-11D0-847B-444553540000` | 1 | 170 | 169 | 0x100 | 22050 | 0 | 1 |
| female8.vce | current | Mary (for Telephone) | `C77C51D0-…` | 1 | 170 | 169 | 0x200 | 8000 | 0 | 1 |
| male.vce | current | Mike | `C77C5160-…` | 2 | 170 | 113 | 0x100 | 22050 | 0 | 1 |
| male8.vce | current | Mike (for Telephone) | `C77C51C0-…` | 2 | 170 | 113 | 0x200 | 8000 | 0 | 1 |
| sam.vce | older | Sam | `C5C35D60-DA44-11D1-B1F1-0000F803E456` | 2 | 150 | 100 | 0x100 | 22050 | 0 | 0 |
| female.cfg | current | Female | Mary's | 1 | 170 | 169 | 0x100 | 22050 | 7 | 0 |
| female8.cfg | current | Female (for Telephone) | Mary (for Telephone)'s | 1 | 170 | 169 | 0x200 | 8000 | 1 | 0 |
| male.cfg | current | Male | Mike's | 2 | 170 | 113 | 0x100 | 22050 | 7 | 0 |
| male8.cfg | current | Male (for Telephone) | Mike (for Telephone)'s | 2 | 170 | 113 | 0x200 | 8000 | 1 | 0 |
| sam.cfg | current | Male | Mike's | 2 | 170 | 113 | 0x100 | 22050 | 1 | 0 |

Speakers: `Default Female Voice` (female files), `Default Male Voice` (male files and sam.cfg), `Second Male Voice` (sam.vce). `sam.cfg`'s header is a copy of Mike's; it only matters as the merge base of the hidden "Sam Whisper" variant (section 3).

## 3. `CFG<n>` streams: variants

A variant is a mode built from a base voice plus overrides and effect records. `CFG<n>` is stream n (1-based, no leading zeros) of a `.cfg` file.

| Offset | Type | Field | Meaning |
|---|---|---|---|
| 0x000 | VcHeader layout | overrides | The header fields are overrides (below), except signature (`{5B435000-9752-11D1-B1CE-0000F803E456}`), modeId, waveFormat, variantCount (0) and preferenceRank |
| 0x428 | GUID | parentModeId | In the modeId slot: the base voice's modeId |
| 0x8f6 | SHORT | preferenceRank | |
| 0x8f8 | DWORD | version | 1 |
| 0x8fc | GUID | modeId | The variant's own mode GUID. GUID_NULL = hidden mode (interface.md §2) |
| 0x90c | WCHAR[262] | characterTag | `\Chr="Whisper"\` or empty |
| 0xb18 | DWORD | recordCount | |
| 0xb1c | records | | `recordCount` × `{DWORD type; DWORD size; BYTE data[size]}` |

The parent must be a voice from a `.vce` file in the same folder.

**Merging.** The variant's mode starts from the `VcHeader` of the `.cfg` file that holds it (not the parent `.vce`), and each override replaces the base value unless it holds its "inherit" value:

| Field | Inherit value |
|---|---|
| manufacturer, modeName, speaker, style | empty string |
| language | LANGID 0xFFFF |
| gender, age, defaultSpeed, defaultPitch | 0xFFFF |
| features | 0xFFFFFFFF |

waveFormat always comes from the base. The variant's voice data (all other streams) comes from the parent `.vce`.

**Records.** The engine applies a record by `type & 0xFFFFFF`. The top bits only say which stage must understand the record when the mode list is built: 0x40000000 front end, 0x20000000 unit mapper, 0x10000000 back end; 0x80000000 means "optional, don't check". A record the stage doesn't understand still leaves the mode listed and has no effect. Record sizes are never checked by the original; the converter checks them ([effects.md](effects.md) §9).

| Type & 0xFFFFFF | Data | Effect |
|---|---|---|
| 1 | none | Whisper: every frame uses noise excitation; pitch is ignored |
| 2 | `DWORD n; float c[n]` (n ≥ 1) | FIR post-filter `y[t] = Σ c[k]·x[t−k]`. Replaces type 3 |
| 3 | `DWORD n; float c[n]` (n ≥ 1) | All-pole post-filter `y[t] = c[0]·x[t] + Σ_{k≥1} c[k]·y[t−k]`. Replaces type 2. Not shipped |
| 4 | `DWORD preset` (1–7) | Reverb preset. Hall 3, Stadium 5, Space 6; 1, 2 and 7 are the RoboSoft resonators; 4 is not shipped |
| 5 | none | Monotone: every pitch point is set to the current pitch |
| 6 | `DWORD n; float taps[n]` | Filter inside the reverb loop; needs a type 4 first. Not shipped |

The filters, reverb presets and noise are specified in [effects.md](effects.md) and [synthesis.md](synthesis.md).

**Shipped variants** (speed/pitch "–" = inherited):

| File | n | Mode | Speed / pitch | characterTag | Records |
|---|---|---|---|---|---|
| female.cfg | 1 | Female Whisper | – | Whisper | 0x90000001; 0x10000002 {3, 0.25, −0.5, 0.25} |
| female.cfg | 2 | Mary in Space | – | | 0x10000004 {6} |
| female.cfg | 3 | Mary in Hall | – | | 0x10000004 {3} |
| female.cfg | 4 | Mary in Stadium | – | | 0x10000004 {5} |
| female.cfg | 5 | RoboSoft Six | 130 / 100 | | 0x10000004 {7}; 0x40000005 |
| female.cfg | 6 | RoboSoft Five | 130 / 120 | | 0x10000004 {1}; 0x40000005 |
| female.cfg | 7 | RoboSoft Four | – | | 0x10000004 {2} |
| female8.cfg | 1 | Female Whisper (for Telephone), hidden | – | Whisper | 0x90000001; 0x10000002 {3, 0.25, −0.5, 0.25} |
| male.cfg | 1 | Male Whisper | – | Whisper | 0x10000001; 0x10000002 {3, 0.25, −0.5, 0.25} |
| male.cfg | 2 | RoboSoft One | 130 / 75 | | 0x10000004 {7}; 0x40000005 |
| male.cfg | 3 | RoboSoft Two | 130 / 120 | | 0x10000004 {1}; 0x40000005 |
| male.cfg | 4 | RoboSoft Three | – | | 0x10000004 {2} |
| male.cfg | 5 | Mike in Hall | – | | 0x10000004 {3} |
| male.cfg | 6 | Mike in Stadium | – | | 0x10000004 {5} |
| male.cfg | 7 | Mike in Space | – | | 0x10000004 {6} |
| male8.cfg | 1 | Male Whisper (for Telephone), hidden | – | Whisper | 0x90000001; 0x10000002 {3, 0.25, −0.5, 0.25} |
| sam.cfg | 1 | Sam Whisper, hidden (parent Sam) | – | Whisper | 0x90000001; 0x10000002 {3, 0.25, −0.5, 0.25} |

The mode GUIDs are in interface.md §2.

**When records apply.**
- Selecting a variant mode applies all its records when the engine starts. A base mode has none.
- `\Chr="<name>"\` in text looks for a variant of the same base voice whose characterTag is `\Chr="<name>"\` (case-insensitive) and applies its back-end records (types 1, 2, 3, 4, 6) **on top of** the current ones. For example, Mary in Hall plus `\Chr="Whisper"\` whispers in the hall. Its type 5 records are not applied. In `TextData`, every `\Chr=` tag first sends `"Normal"` (text.md §3.3), so a tag starts again from the mode's own effects: only names listed in the same tag (`\Chr="Whisper","Monotone"\`) add up.
- `\Chr="Monotone"\` turns on monotone directly, without a variant.
- `\Chr="Normal"\` and `\Rst\` clear every effect, then re-apply the selected mode's own records.
- A reverb, once created, keeps its preset and its delay contents for the life of the engine. A later type 4 record only switches it back on.

## 4. `PhoneFile`

ASCII text. Every line, including the last, ends in CR LF. Each line has five whitespace-separated fields:

| Field | Meaning |
|---|---|
| name | Phone name. Vowels end in a stress digit: 1 stressed, 0 unstressed |
| kind | 0 in every shipped line (a plain phone) |
| class | 0 consonant, noise or silence; 1 monophthong; 2 diphthong. Not used by the engine for kind 0 |
| base | Equals index in every shipped line |
| index | Phone number, 0-based. Lines are in index order |

Any line with kind ≠ 0 or base ≠ index is a feature no shipped file uses; the converter rejects it.

Phones of female, female8, male and male8 (58):

```
 0 +NOISE+   1 B    2 CH   3 D    4 DH   5 F    6 G    7 HH   8 JH   9 K
10 L        11 M   12 N   13 NG  14 P   15 R   16 S   17 SH  18 SIL 19 T
20 TH       21 V   22 W   23 Y   24 Z   25 ZH
26 AA1 27 AE1 28 AH1 29 AO1 30 AW1 31 AY1 32 EH1 33 ER1 34 EY1 35 IH1 36 IY1 37 OW1 38 OY1 39 UH1 40 UW1
41 AA0 42 AE0 43 AH0 44 AO0 45 AW0 46 AY0 47 EH0 48 ER0 49 EY0 50 IH0 51 IY0 52 OW0 53 OY0 54 UH0 55 UW0
56 SILb 57 SILe
```

Sam (52) has the same phones 0–44 but no AW0, AY0, EY0, OY0, SILb or SILe, so its unstressed vowels are numbered 45 EH0, 46 ER0, 47 IH0, 48 IY0, 49 OW0, 50 UH0, 51 UW0. Phone numbers are therefore per voice; the converter keeps each voice's own table.

The silence phone is the one named `SIL` (18 in every voice). `+NOISE+`, `SILb` and `SILe` have no units; the engine never produces them.

## 5. `TreeImage`

Decision trees that pick a senone for a phone from its context: the previous phone, the next phone and the phone's position in its word. A senone is one acoustic unit of the voice (one entry of `Senone` and of `Inventory`). How the engine fills in phone, context and position is in `units.md`.

All integers are **big-endian**. Strings are NUL-terminated ASCII. The stream is read in this order:

| # | Type | Field | Shipped value / meaning |
|---|---|---|---|
| 1 | int32 | featureBlockSize | > 0 (1720 female, 1707 sam). Only a size hint. 0 or negative selects layouts no shipped file uses; the converter rejects them |
| 2 | int32 | wordsPerMask (W) | 2: masks are 64 bits |
| 3 | string | featureFileName | Training path, ignored |
| 4 | int32 | featureCount | 71 |
| 5 | int32[5] | boundaryFeatures | 0..4, ignored |
| 6 | featureCount × {string; uint32[W]} | features | Named phone sets (`VOWEL`, `WDBNDRY_B`, …). Ignored by the lookup; the converter rejects any name starting with `STATEID` |
| 7 | string, string | treeDirectory, treeExtension | Ignored |
| 8 | int32 | phoneCount | Must equal the PhoneFile phone count |
| 9 | int8[phoneCount] | statesPerPhone | All 1. Anything else is rejected |
| 10 | int32[phoneCount+1] | senoneOffsets | Cumulative; senoneOffsets[0] = phoneCount. Phone p has senoneOffsets[p+1] − senoneOffsets[p] senones |
| 11 | int32[5] | header | senoneCount (3333); unused (3333); wordPositionIndependent (0); silencePhone (must be the PhoneFile's `SIL`); wordEndRightContext (−1) |
| 12 | int32 | questionCount | 3288 (sam 3324) |
| 13 | uint32[questionCount × 2W] | questions | Each question: left mask (W words), then right mask (W words) |
| 14 | int32, int32 | nodeTotal, questionListWordTotal | Totals, for allocation |
| 15 | phoneCount trees | trees | In phone order, below |

A mask's bit b is bit b%32 of word b/32 (word 0 first). Phone p is bit p+4. In the left mask, bits 0–3 are the word positions the question accepts: 1 word-initial, 2 word-final, 4 single-phone word, 8 inside a word.

Each tree is `int16 nodeCount; int16 leafCount` followed by nodeCount nodes. A tree with 0 nodes is empty (the noise and extra silence phones). Each node is `int16 yes; int16 no; int16 leaf`:
- **Internal node** (yes ≥ 0): followed by a list of uint16 question indices ending in 0xFFFF. yes and no are node indices within the tree, always later than the node.
- **Leaf** (yes = −1): no = leaf = the leaf number, 0 .. senones(p)−1.

**Lookup** for phone p with left context l, right context r and position:
1. If the name of l starts with `+` or `SIL` (3 characters, case-insensitive), use the silence phone instead. The same for r. (wordEndRightContext is −1 in every shipped file, so it never applies.)
2. pos = 1 word-initial, 2 word-final, 4 single-phone word, 8 inside a word.
3. Start at node 0. At an internal node, the answer is yes if **any** question in the node's list has pos in its left mask's position bits, bit l+4 set in its left mask and bit r+4 set in its right mask. Go to the yes or no node. Stop at a leaf.
4. The senone id is `senoneOffsets[p] − phoneCount + leaf + 1`: 1..3333 in every voice.

The shipped trees reach every senone, and every lookup over every phone, context and position returns a valid leaf.

## 6. `Senone`

Little-endian. One record per senone, in senone id order.

| Type | Field | Meaning |
|---|---|---|
| int32 | count | 3333 |
| count × 28-byte records: | | |
| char[8] | phone | Phone name, NUL-padded |
| int32 | indexInPhone | 1-based. Record k (0-based) is senone id k+1 = senoneOffsets[phone] − phoneCount + indexInPhone |
| float | meanDuration | Milliseconds. The base duration of the unit (`units.md`) |
| float | b | Not used by the engine |
| float | c | Used only as below |
| float | d | Used only as below |

The engine's amplitude factor for the unit is `sqrt(exp(c − d))`. (b is probably the duration's standard deviation and c, d log energies; the engine doesn't need to know.)

## 7. `AltUnits` (Sam only)

Fixed unit sequences for phrases Sam recorded specially: "Hello, I'm Merlin, your Windows assistant" and "Merlin". When the phones of an utterance contain one of these sequences, the matching phones are synthesised from extra inventory units (ids 3334–3365) instead of the tree's choice. Little-endian:

| Type | Field |
|---|---|
| uint32 | payloadSize (324) |
| for each phone p in PhoneFile order: | |
| int32 | entryCount |
| entryCount entries, below | |

Entry:

| Type | Field | Meaning |
|---|---|---|
| bytes, NUL-terminated | key | Phone numbers of the sequence. The entry is listed under its first phone |
| uint8 | flags | 1: the first key phone is part of the replaced span. 2: the last key phone is part of it. 4, 8: context lists follow |
| if flags & 4: uint8 n; uint16[n] | leftList | Leaf numbers allowed at the start. Not shipped; the converter rejects it |
| if flags & 8: uint8 n; uint16[n] | rightList | Leaf numbers allowed after the end. Not shipped; the converter rejects it |
| int16 n; uint16[n] | codes | One code per phone of the span |
| bytes, NUL-terminated | replacementPhones | Phone numbers used by replace and insert codes |

A code's low 13 bits are the unit id (0 = silence). The top bits:

| Bits | Meaning |
|---|---|
| 0xC000 | Replace this phone with the next replacement phone |
| 0x8000 | Insert the next replacement phone before this one. Not shipped; the converter rejects it |
| 0x4000 | Delete this phone. Not shipped; the converter rejects it |
| 0x2000 | Delete two phones. Not shipped; the converter rejects it |

Sam's two entries:
- Under HH, flags 3: `HH EH0 L OW1 SIL AY1 AE1 M M ER1 L IH0 N SIL Y AO1 R W IH1 N D OW0 Z AH0 S IH1 S T AH0 N T` with codes for units 3334–3362 (0 at both SILs). EH0 becomes AW1 and OW0 becomes EY1 (replacementPhones `AW1 EY1`).
- Under M, flags 0: `M ER1 L IH0 N`, with units 3363–3365 for ER1, L and IH0.

How matches are found and applied is in `units.md`.

## 8. `Inventory`

The recorded speech, coded as a pitch-synchronous LPC vocoder. Each unit is a list of frames; a frame is one pitch period (voiced) or one block of noise (unvoiced). A frame stores its length, a vocal-tract filter (line spectral frequencies, vector-quantised), a gain and, for voiced frames, the spectrum of one excitation pulse (vector-quantised). Little-endian.

### Header

| Offset | Type | Field | 22 kHz voices | 8 kHz voices |
|---|---|---|---|---|
| 0x00 | int32 | sampleRate | 22050 | 8000 |
| 0x04 | int32 | predictorOrder (p) | 20 | 12 |
| 0x08 | int32 | lineSpectralCodebookCount (K_L) | 6 | 4 |
| 0x0c | int32 | firstSpectrumCodebookCount (K_F) | 11 | 10 |
| 0x10 | int32 | deltaSpectrumCodebookCount (K_D) | 13 | 10 |
| 0x14 | int32 | transformSize (N) | 512 | 256 |
| 0x18 | int32 | transformSizeExponent | 9 | 8 |

N must be at most 512.

### Codebooks

K_L line spectral codebooks, then K_F "first" spectrum codebooks, then K_D "delta" spectrum codebooks. Each is `int32 rows; int32 columns; float values[rows × columns]`, row-major; row r is code vector r.

| | 22 kHz | 8 kHz |
|---|---|---|
| Line spectral | 256 rows; widths 3, 3, 3, 3, 4, 4 | 256 rows; widths 3, 3, 3, 3 |
| First | 128 rows; widths 10, 10, 16, 22, 28, 32, 36, 40, 60, 100, 156 | 128 rows; widths 12, 12, 16, 16, 16, 18, 24, 38, 46, 56 |
| Delta | 128 rows; widths 10, 10, 10, 12, 12, 14, 18, 32, 36, 40, 60, 100, 156 | as First |

The line spectral widths add up to p. The spectrum widths add up to N − 2.

### Unit table

| Type | Field | Meaning |
|---|---|---|
| int32 | lastUnit (U) | 3333 (sam 3365). Units are numbered 0..U |
| int32 | blobSize | Bytes to the end of the stream |
| int32[U+1] | offsets | Unit i starts at (end of this array) + offsets[i] and ends where unit i+1 starts; unit U ends at the end of the stream |
| bytes | records | |

Unit 0 is empty: it is silence and is never decoded. Units 1..3333 are the senones (id = unit number). Units above 3333 are reached only through `AltUnits`.

### Unit record

| Order | Type | Field | Meaning |
|---|---|---|---|
| 1 | int32 | frameCount (F) | 2..151 in the shipped data |
| 2 | F escape-coded integers | periods | > 0: voiced, a pitch period of that many samples. < 0: unvoiced, a block of −value samples. Never 0 |
| 3 | uint8[F][K_L] | lineSpectralIndices | Per frame, one row per line spectral codebook |
| 4 | float[F] | gains | Excitation gain per frame |
| 5 | uint8 per codebook, voiced frames only | spectrumIndices | In frame order. A voiced frame uses K_D delta indices if K_D > 0, it isn't the first frame and the previous frame is voiced; otherwise K_F first indices |

**Escape coding.** Read a signed byte b. If b = 127, the value is 127 plus following signed bytes, as long as each added byte was 127 (`v = 127; repeat { c = next; v += c } while c == 127`). If b = −128, the same with −128. Otherwise the value is b. Periods reach 595 samples.

### Decoding a unit

For each frame f, with L = |periods[f]|:

1. **Filter.** Concatenate the line spectral code vectors (codebook k, row index[f][k]) into p frequencies, in cycles per sample. If they are not strictly increasing, sort them ascending. Convert to predictor coefficients a[0..p] (a[0] = 1), in double precision (the exact arithmetic is in synthesis.md §4):
   - `P(z) = (1 + z⁻¹) · Π_j (1 − 2cos(2π·lsf[2j])·z⁻¹ + z⁻²)`
   - `Q(z) = (1 − z⁻¹) · Π_j (1 − 2cos(2π·lsf[2j+1])·z⁻¹ + z⁻²)`, j = 0 .. p/2 − 1
   - `A(z) = (P(z) + Q(z)) / 2`; a[k] is the coefficient of z⁻ᵏ, stored as float.
2. **Unvoiced excitation** (period < 0, or whisper is on): take L values from the noise table at the voice's noise position, `e[t] = noise[position + t] · (gain · 0.02)`, with `gain · 0.02` kept exact (synthesis.md §4). If position + L would reach the end of the table, start again from 0 first. Then advance the position by L. The position belongs to the engine instance and carries over between frames, units and utterances.
3. **Voiced excitation:**
   1. Spectrum S of N floats in half-complex order: S[k] is the real part of bin k (k = 0..N/2) and S[N−k] the imaginary part of bin k. Bins 0 and N/2 stay 0.
   2. With the first codebooks, set S to zero and then **set** the bands; with the delta codebooks, **add** the bands to S as the previous voiced frame of this unit left it.
   3. Bands: s starts at 1. For each codebook k, take its code vector v and h = width/2: `S[s+j] = v[j]` and `S[N−s−h+1+j] = v[h+j]` for j < h (or `+=` for delta). Then s += h.
   4. Pulse w = inverse real DFT of S: `w[t] = (1/N) · (S[0] + (−1)ᵗ·S[N/2] + 2·Σ_{k=1}^{N/2−1} (S[k]·cos(2πkt/N) − S[N−k]·sin(2πkt/N)))`, then w ×= gain. The original computes it with Sorensen's split-radix inverse FFT; synthesis.md §4 gives the order that reproduces its rounding.
   5. Fold the pulse into the period: m = min(L, N/2); `e[0..m) = w[0..m)`, `e[m..L) = 0`, then `e[L−m+i] += w[N−m+i]` for i < m. The pulse starts at the period start; its tail before time 0 wraps to the end of the period.
4. With whisper on, a voiced frame is treated as unvoiced from then on (its period is negated), so the pitch contour doesn't apply to it.

**Natural playback** (no time or pitch change): each frame's L excitation samples go through the all-pole filter `y[t] = e[t] − Σ_{k=1}^{p} a[k]·y[t−k]` with that frame's coefficients. The filter memory carries over between frames and units, and is cleared by silence. How the engine repeats, skips and resizes periods to follow the requested duration and pitch is in `synthesis.md`.

**Noise table.** One per base voice, sampleRate bytes long (one second), built when the voice is first loaded. For each pair of entries, draw u1 and u2 as `rand() / 32768` (MSVC `rand`: `state = state·214013 + 2531011`, result `(state >> 16) & 0x7FFF`), then `r = sqrt(−2·ln(1 − u1))`, `θ = 2π·u2`, and z = cos θ·r for the first entry and sin θ·r for the second (each rounded to float). Each entry is `v = 50·z`, rounded half away from zero, keeping only the low 8 bits as a signed byte (so values beyond ±127 wrap). The original uses whatever `rand` state the host thread has. The Go engine starts from seed 1, the state of a thread that hasn't used `rand`; with it the table starts −2, −1, 12, −31, −66, 8, 37, −28.

### Shipped totals

| Voice | Units | Frames (voiced / unvoiced) | Audio at natural timing |
|---|---|---|---|
| female | 3333 | 45 655 / 12 946 | 326.5 s |
| female8 | 3333 | 43 542 / 14 248 | 326.6 s |
| male | 3333 | 41 078 / 10 230 | 377.5 s |
| male8 | 3333 | 41 026 / 10 202 | 377.7 s |
| sam | 3365 | 29 809 / 6 919 | 346.4 s |

Every byte of every stream is accounted for by this layout.

## 9. `Cdb`

Never opened by the engine. It is text: one line per tone label with its count and the mean and standard deviation (Hz) at onset and offset, then a separator and a few numbers; probably training statistics. Sam's copy is byte-identical to Mike's. The converter ignores it. Since no stream holds prosody templates, any the engine uses are built into the engine (`prosody.md`).

## 10. Quirks we don't reproduce

- A missing `CFG<n>` stream makes the original read the next file's streams from the wrong file. The converter reads each file on its own.
- Record sizes and type 4 presets are not checked: preset 0 or above 7, or a type 2/3 with n = 0, crashes the original. The converter rejects them.
- An inventory that fails to load is stored as loaded and crashes later. `features & 0x8000` leaves a lock held. Both are rejected by the converter.
- An empty tree (noise, `SILb`, `SILe`) used for a unit would crash the original. The Go engine treats those phones as silence.
- The original's handling of `AltUnits` context lists, insert and delete codes is broken (wrong leaf indices, reads past its lists, a freed block reused). No shipped entry uses them, so the converter rejects them (units.md §5).

## 11. For the data pack

The format is in [datapack.md](datapack.md). What the converter keeps, per base voice (5): the phone table, the trees (questions, nodes, senone offsets), the senone durations and amplitude factors, Sam's alternative units, the inventory (codebooks and unit records, kept in this coding: it is already compact at 0.9–3 bits per sample), and the header values. Per mode (22, hidden ones included): the merged header values, the characterTag and the records. Shared streams are stored once. Each base voice also gets the line spectral cosine table (synthesis.md §3). The noise table is generated, not stored. The rest is ignored: `Cdb`, the feature names, the training paths, and every unused header field.

## 12. Open questions

- What `Senone` fields b, c and d mean on their own. The engine only uses `exp(c − d)`, so this doesn't affect the port.
- The second value of the `TreeImage` header (3333) is never used.
- Whether applications in practice leave `rand` untouched before selecting a voice (the noise table's seed). To be checked against the original engine's audio for unvoiced sounds (synthesis.md §16).
