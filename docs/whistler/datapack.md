# SAPI 4 Microsoft engine: data pack

The files the Go engine reads instead of the original voice files and the original engine's own tables. A converter that is not part of this repo writes them from the original files; the engine (`whistler/datapack`) reads only these files and never parses compound files or `msttssyn.dll`. The data pack is a separate download and is never added to this repo or to the app binary.

**Status.** Format version 1 holds the voice data (phones, trees, senones, the `Visual` entry of each phone, Sam's alternative units and the unit inventory with its line spectral cosines), the modes with their effects and the reverb presets, the prosody tables, the three rule modules, the dictionaries, the text tables and the resamplers' taps. Until the first release the format can change without a version bump; after it, any change that an older engine can't read raises `formatVersion`.

## 1. Layout

```
manifest.json
modes.json
prosody.json
rules.json
dictionaries.json
text.json
resamplers.bin
voices/<voice>/voice.json
voices/<voice>/trees.bin
voices/<voice>/senones.bin
voices/<voice>/inventory.bin
```

`<voice>` is `female`, `female8`, `male`, `male8` or `sam`. Files with identical contents are stored once, and `voice.json` names the file each voice uses: female8, male and male8 use female's `trees.bin`, and female8 and male8 use the senones of female and male. The five voices take about 8 MB.

**Conventions.**
- Every binary file is little-endian. It starts with a 4-byte ASCII magic and a uint32 format version (1). Its sections follow without padding or alignment, and the file ends exactly where its last section does.
- `float32` and `float64` are IEEE 754 values, stored bit for bit.
- JSON files are UTF-8. File names in them are relative to the pack root, with `/` separators. A name that is not a plain relative path, or that the manifest doesn't list, is an error. A float32 value is written as the shortest decimal of its double value and read as the nearest float32, which gives it back exactly.
- A **byte string** (dictionary words and entries, the text tables' strings) is a JSON string whose characters U+0000 to U+00FF stand for the bytes 0x00 to 0xFF. A character above U+00FF is an error.
- Readers check every count, offset and index when they load a file, so a damaged pack fails to load instead of failing during synthesis.

## 2. `manifest.json`

| Field | Meaning |
|---|---|
| `format` | `"opensapi4-microsoft"` |
| `formatVersion` | 1 |
| `voices` | The voice names, in the order above |
| `files` | Every other file of the pack, mapped to the SHA-256 of its contents |

## 3. `modes.json`

| Field | Meaning |
|---|---|
| `modes` | The 22 modes of interface.md §2, in the original's list order: the five base voices, then the variants of female.cfg, female8.cfg, male.cfg, male8.cfg and sam.cfg |
| `reverbPresets` | The reverb presets of effects.md §5.1; preset n is entry n − 1 |

**A mode** carries its final values: a variant's header values are already merged over its `.cfg` file's header (voice-format.md §3).

| Field | Meaning |
|---|---|
| `name` | The mode name |
| `modeId` | The mode GUID, `{XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX}`, unique. Null for a hidden variant, which the mode list never shows (its mode ID is `GUID_NULL`) |
| `voice` | The base voice whose data the mode uses |
| `variant` | 0 for the base mode, k for the voice's kth variant. A voice's base mode comes before its variants, which are numbered from 1 without gaps in list order |
| `manufacturer`, `language`, `dialect`, `speaker`, `style`, `age` | Reported only. `language` is the LANGID |
| `gender` | `"female"` or `"male"` |
| `features` | The header's features: 0x100 PC-optimised, 0x200 telephone-optimised |
| `speed`, `pitch` | The default speed in words per minute and pitch in Hz, 1–65534 |
| `preferred` | Whether the mode is PREFERRED (interface.md §2) |
| `characterTag` | `\Chr="name"\` or empty |
| `effects` | The variant's records in order (below). A base mode has none and no character tag |

A hidden variant keeps the values merged over its `.cfg` file's header, which for Sam Whisper is a copy of Mike's (speed 170, pitch 113). Only the first hidden variant's are ever read, when `Select` is given `GUID_NULL` (interface.md §3). Otherwise a hidden variant is only reached through its character tag, and the voice keeps the selected mode's values.

**Effects.** One object per record. A record's top bits (voice-format.md §3) don't change what the engine does and are dropped.

| `effect` | Record type | Other fields |
|---|---|---|
| `"whisper"` | 1 | none |
| `"postFilter"` | 2, the FIR post-filter | `coefficients`: 1 to 65535 values |
| `"reverb"` | 4 | `preset`: 1 to the number of presets. At most one per mode |
| `"monotone"` | 5 | none |

**A reverb preset.**

| Field | Meaning |
|---|---|
| `inputGain`, `dryGain` | The linear gains |
| `lengthModulation` | 0 to 0.125, so the line length scale stays positive (effects.md §5.3: the random value is below 8) |
| `lines` | 1 to 4 allpass lines, each `{delay, gain}`: the delay in milliseconds, above 0 and at most 10,000 (which bounds the memory a damaged pack can ask for), and the linear gain |

The gains are `f32(10^(d·0.05))` of the decibel values (effects.md §5.2). The converter computes each one exactly and checks that it lies at least 2⁻⁴⁰ (relative) away from a float32 rounding boundary, so the engine computes no powers. The line lengths in samples depend on the voice's rate and are computed at load (effects.md §5.2).

## 4. `voice.json`

| Field | Meaning |
|---|---|
| `phones` | The phone names, by phone number (voice-format.md §4): 58 for Mary and Mike, 52 for Sam. At most 60 |
| `silencePhone` | The number of `SIL` (18) |
| `sampleRate` | 22050, or 8000 for the telephone voices |
| `trees`, `senones`, `inventory` | The files this voice uses |
| `visual` | The `Visual` entry of each phone (below) |
| `alternativeUnits` | Sam's alternative units (below); empty for the other voices |

**Alternative units** (voice-format.md §7, units.md §5). One object per entry, in the original's order: by the entry's first phone, then as listed under it.

| Field | Meaning |
|---|---|
| `key` | The phone numbers of the sequence |
| `flags` | 1: the first key phone is part of the replaced span; 2: the last one is. No other bits |
| `codes` | One code per phone of the span: the low 13 bits are the unit, the top bits 0xC000 mean "replace this phone with the next replacement phone". No other top bits. The unit is 0 on a `SIL` key phone, otherwise an inventory unit above the senone count. The last code doesn't replace |
| `replacementPhones` | The phones the replace codes use, in order; one per replace code. Always phones with a tree, so never `SIL` |

**Visual entries** (units.md §11). One object per phone, by phone number:

| Field | Meaning |
|---|---|
| `ipa` | The IPA character as a UTF-16 code unit, not a surrogate; 0 for `SIL` |
| `engine` | The engine phoneme, an ASCII character from 0x20 to 0x7E |
| `mouth` | The eight `TTSMOUTH` bytes in order: height, width, upturn, jaw open, upper teeth visible, lower teeth visible, tongue position, lip tension |

The converter resolves each phone the way the original resolves a frame's name (units.md §11). The other phones without a tree (`+NOISE+`, `SILb`, `SILe`) are played as `SIL` (units.md §4), so they carry `SIL`'s entry.

## 5. `trees.bin`

The context decision trees of voice-format.md §5, with node links made absolute and the masks as 64-bit integers.

| Type | Field | Meaning |
|---|---|---|
| char[4] | magic | `WTRE` |
| uint32 | version | 1 |
| uint32 | phoneCount | Equals the voice's phone count |
| uint32 | senoneCount | 3333 |
| uint32 | questionCount | |
| uint32 | nodeCount | Nodes of all trees together |
| uint32 | listLength | Question numbers of all nodes together |
| int32[phoneCount] | roots | The node where phone p's tree starts; −1 for a phone without a tree (`+NOISE+`, `SIL`, `SILb`, `SILe`) |
| uint32[phoneCount] | firstSenones | The senone number of phone p's leaf 0 |
| uint32[phoneCount] | senoneCounts | How many senones phone p has. 0 exactly when it has no tree |
| questionCount × {uint64 left; uint64 right} | questions | Bit b of a mask is bit b of the integer. In both masks, phone p is bit p + 4. Bits 0–3 of the left mask are the word positions the question accepts: 1 word-initial, 2 word-final, 4 single-phone word, 8 inside a word |
| nodeCount × {int32 yes; int32 no; int32 leaf; uint32 first; uint32 count} | nodes | An internal node has yes and no (node numbers, both greater than its own) and its questions `lists[first .. first+count)`, count ≥ 1; its leaf is −1. A leaf has yes = no = −1, its leaf number in `leaf` and first = count = 0 |
| uint16[listLength] | lists | Question numbers |

**Lookup** (voice-format.md §5): start at `roots[p]`. At an internal node the answer is yes if any of its questions has the position's bit and bit l+4 set in its left mask, and bit r+4 set in its right mask; go to `yes` or `no` and stop at a leaf. The unit is `firstSenones[p] + leaf`. Every tree reaches each of its phone's senones, and every leaf number is below `senoneCounts[p]`.

## 6. `senones.bin`

| Type | Field | Meaning |
|---|---|---|
| char[4] | magic | `WSEN` |
| uint32 | version | 1 |
| uint32 | count | 3333 |
| count × {float32 meanDuration; float32 gain} | senones | By senone number, from 1 |

`meanDuration` is the `Senone` stream's mean duration in milliseconds. `gain` is `exp(c − d)` rounded to float32 (voice-format.md §6, units.md §14). The converter computes it in double and checks that it lies at least 2⁻⁵⁰ (relative) away from a float32 rounding boundary, so any correct `exp` gives the same value. Both are finite, and the gain is positive.

## 7. `inventory.bin`

| Type | Field | Meaning |
|---|---|---|
| char[4] | magic | `WINV` |
| uint32 | version | 1 |
| int32 × 7 | header | sampleRate, predictorOrder p, lineSpectralCodebookCount, firstSpectrumCodebookCount, deltaSpectrumCodebookCount, transformSize N, its exponent (voice-format.md §8) |
| codebooks | | The line spectral, first and delta codebooks in that order, each `int32 rows; int32 columns; float32 values[rows × columns]` |
| float64 per line spectral codebook value | cosines | For each line spectral codebook in order, `c(f)` for each of its values in order (synthesis.md §3) |
| uint32 | unitCount | The units, from 0. 3334 (Sam 3366) |
| uint32[unitCount + 1] | offsets | Unit u's record is `records[offsets[u] .. offsets[u+1])`. Unit 0 (silence) is empty |
| bytes | records | The unit records, in the coding of voice-format.md §8 |

Checks the reader makes:
- N is 2ᵏ with 2 ≤ k ≤ 9, and p is even;
- the line spectral widths add up to p, and each set of spectrum widths to N − 2; spectrum widths are even;
- every codebook has 1–256 rows; every value and cosine is finite;
- every record decodes exactly to its end: at least one frame, every period nonzero and shorter than 2N samples (the fade window has N entries; synthesis.md §6), every codebook index below its codebook's row count, every gain finite.

**The cosines.** `c(f) = −2·cos(2π·f)`, computed as the original's FPU does: θ = f · 2π in double, the cosine rounded to 64 significant bits, then the product with −2 rounded to 53 bits (synthesis.md §3). The converter computes the cosine to 60 digits and rounds it that way, and stores the 5 values where the development machine's FPU gives a different last bit as measured. A value that appears more than once has the same cosine everywhere.

## 8. `prosody.json`

The tables of the analyser's prosody stages and the unit builder's phone names ([prosody.md](prosody.md)). Every voice uses them.

| Field | Meaning |
|---|---|
| `phoneClasses` | 256 bytes: the class letter of each code (prosody.md §1) |
| `phoneFeatures` | 256 bytes: the feature bits of each code (prosody.md §1) |
| `inherentDurations` | 256 int16 values: the inherent duration of each code in milliseconds |
| `minimumDurations` | 256 bytes: the minimum duration of each code in milliseconds |
| `followingFactors`, `afterApproximantFactors` | 256 bytes each: the direct and after-approximant following-phone factors (prosody.md §5.3) |
| `unitClasses` | 256 bytes: the analyser unit class of each code, 0–41 (prosody.md §6.1) |
| `unitPairs` | 42 rows of 42 uint16 values, by the left and right unit class |
| `unitLists` | The string lists the pair values refer to (below) |
| `unitFrames` | The frame count of each phone of each analyser unit, from unit 1: 2 to 5 bytes per unit |
| `tones` | The 33 tone records (prosody.md §8.1): `mark` (a byte), `level`, `secondOffset`, `timeOffset` and `baselineStep` (int16), `duration` (uint16), `fromPhoneEnd` |
| `declination` | Per sentence type 0–4, `{floor, slope}` (uint16) |
| `phoneNames` | The unit builder's phone names (prosody.md §10.3): `{code, name, vowel}`, in the original's table order, which the ARPAbet conversion also uses (pronunciation.md §6.2). A name is 1 to 4 printable ASCII characters, each code appears once, and a code without an entry has no name |

The per-code tables hold all 256 codes as the original reads them. Some of them overlap the next table, so codes that no tier holds carry neighbouring data.

**A unit list** is `{list, lengths}`. `list` is the list number, 1 or more. `lengths[n − 1]` holds the strings of n phones (n is 1 to 3) as `{phones, unit}`, with `phones` the n phone codes. A string appears at most once per length.

**Pair values** (prosody.md §6.2). Bit 15 is never set, and no value has both 0x2000 and 0x4000. For a pair value v:
- **Pair case** (no `-` between the two phones): the list is `v & 0x1FFF`. The unit when no string matches is that number, plus 0x561 if v has 0x2000 and the number is 1–0xA6.
- **Across a `-`:** the list is `v & 0x1FFF`, plus 0x561 if that is 1–0xA6. The unit when no string matches is the list, or `v & 0x1FFF` if v has 0x4000.
- **Default unit:** a unit of 0 becomes `v & 0x1FFF` of the pair (`#`, `t`).

Checks the reader makes:
- every table has its count and every value fits its type;
- unit classes are below 42;
- every unit has 2 to 5 phones;
- every pair's unit when no string matches, in both cases, and the default unit, is a unit of two phones;
- a string of n phones gives a unit of n + 2 phones;
- there are 33 tone records and 5 declination entries.

## 9. `rules.json`

The three rule modules of [rules.md](rules.md), as `{"modules": [...]}` with one object per module.

| Field | Meaning |
|---|---|
| `name` | `"A"`, `"B"` or `"C"` |
| `groups` | `{group, first, last}`: the group's first and last rule, 1 ≤ first ≤ last ≤ the rule count |
| `rules` | The rules in order, rule r at entry r − 1 (below) |
| `classes` | Class k at entry k − 1: its member symbols |
| `series` | Series k at entry k − 1: `[symbol, index]` per member, with the index 1–26 |
| `seriesMaps` | Map k at entry k − 1: the 26 output symbols for the indexes 1–26 |
| `variables` | Variable v at entry v − 1: `{class}` or `{series}`, the set its first binding must belong to. At most 32 |
| `procedures` | `{number, kind}`: kind `native`, or `group`, `groupTrue`, `groupFalse` for a procedure that runs group `number` and returns whether a rule fired, always true or always false. Numbers are 3–255 |

**A rule** is `{code, minimumLeft, minimumRight, outputLength, skipOnSuccess, blockCount, required1, required2}`. `code` is the rule's bytecode in hexadecimal, from its mode byte to the `ff` that ends its action, exactly as the original stores it (rules.md §3). The other fields are the rule's entries in the tables of rules.md §2. The bytecode of all rules together is every byte of the original's code area, so the engine needs no offsets.

Members of a block (rules.md §5) keep their own mode byte and fields; only their bytecode after the mode byte, `required1` and `outputLength` are used.

Checks the reader makes, so the interpreter never reads outside a rule or the tables:
- classes and series hold only symbols 0x0f–0xaf, so never the edge; a series index is 1–26;
- `required1` and `required2` are symbols 0x0e–0xaf;
- the mode is 1–8, 13, 14, 16, 99 or 100. The rules use no other mode, so modes 9–12 and 15 are rejected;
- a directive is `63 in out ff ff ff ff` with tiers 1–5, and a no-op `64 ff ff ff ff`;
- every other rule parses into its four sections and ends exactly after its action:
  - every operand is in range: classes, series and variables that exist, tiers 1–5, procedures that exist;
  - test lists hold only test opcodes, and no operand in a list equals the list's terminator;
  - no `f4` has a maximum of 0 (rules.md §3);
  - no focus or context matches the edge 0x0e as a literal;
  - modes 1–4 start their focus with a literal below 0xe9;
  - each context opens and closes its groups in order and has at most 20 backtracking points (`f5 k`, `f6 01`, `f7`);
  - every marker search the interpreter can make in a context (rules.md §6.3) lands on an element of that context with the marker it looks for: from each `f5 k` and each `f6 k` with k ≥ 2 to the closing pair, and from each `f6 01` to alternatives 2, 3, … until the closing pair;
  - an action writes only symbols 0x0f–0xaf, writes as many as `outputLength`, writes a series map only after a focus with exactly one series, writes a variable only if a part of the rule that always matches binds it, and calls no procedure 0;
- following every group's rule loop from its first rule (the next rule, the rule after a block, the rule after a skip on success): every block it reaches ends at a rule that exists, and STOP (`fe 02`) is reached only in a forward scan (modes 1, 3, 5, 7, 13).

The reader doesn't know which procedures are natives of which module; the analyser checks that it has every native a module lists, and every group it runs itself.

## 10. `dictionaries.json`

| Field | Meaning |
|---|---|
| `dictionaries` | One object per dictionary of pronunciation.md §3.1 (below) |
| `builtInExceptions` | The 160 built-in exceptions in the original's order, `{word, pronunciation}`, as stored: printable ASCII, the pronunciation in ARPAbet (pronunciation.md §6.1) |

**A dictionary** is `{procedure, fields, words}`:
- `procedure`: the module C procedure that looks it up, 3–255, each once;
- `fields`: the fields of an entry in order, `{tier, separator}`. Tiers are 1, 3 or 4, each at most once. Every field but the last ends at its one-character separator, a printable ASCII character; the last has none and runs to the end of the entry;
- `words`: the words, each mapped to its entry, both byte strings as the original stores them.

A word is 1 to 31 bytes from 0x20 to 0xfe. An entry is at most 64 bytes from 0x0f to 0xaf and holds the separators of its fields in order.

**What the converter guarantees.** A lookup in the engine is an exact match of the focus bytes against `words` (pronunciation.md §3.2). The converter makes sure the original's own search finds exactly these words and no others:
- every byte of the dictionary areas is accounted for, and each of the ten lookups finds every word;
- each word appears once, and each key belongs to one spelling;
- in the dictionaries with packed keys (all but the main one), no stored key holds a letter code that several bytes share, or the all-ones code that a byte without a code produces. So no other byte string can match a stored key.

Words that the original's byte check rejects (bytes below 0x21, or 0x20 outside the phrases, or 0xff) can't be in the list, because the converter only keeps words that pass it.

## 11. `text.json`

The tables of the Microsoft preprocessing (text.md §2–6 and §11), and the phone set of prosody transplants. Byte strings follow the convention of section 1.

| Field | Meaning |
|---|---|
| `sentenceClasses` | The sentence breaker's class of each byte (text.md §6.1), 256 values from 1–15, 17 and 21 |
| `characterNames` | The name of each byte (text.md §5.4), 256 byte strings of at most 29 printable ASCII characters, many empty |
| `accentFolding` | A byte string of 128 bytes: the ASCII letter that bytes 0x80–0xff fold to, or 0 for none |
| `titles`, `abbreviations`, `starters`, `numberWords` | The sentence breaker's word lists (87, 1415, 96 and 10 words) in the original's order |
| `characterStyles` | The `\Chr=` names with their quotes, `"Normal"` first; only `"Normal"` is used (text.md §3.3) |
| `contexts` | `{name, value}`: the `\Ctx=` names with their quotes, and their values; 2 is the address context |
| `partsOfSpeech` | `{value, escape}`: the `\Prt=` values with their quotes, and the escape each writes (`ESC @x ␠`) |
| `phonemes` | Phonetic symbol k at entry k − 1, 53 in all: `{engineName, wideName, internalSymbol, ipa, arpabetName}` (text.md §2.2–2.3). `arpabetName` is what the lexicon stores for the symbol (pronunciation.md §6.4): at most 2 printable characters, no space, empty for a symbol that isn't a phone, the digit for a stress mark |
| `bestFit` | `[code, byte]` pairs: the UTF-16 code units that code page 1252's best fit turns into something other than `?`, ascending (text.md §2.1) |
| `upper`, `lower`, `alphabetic`, `alphanumeric` | The bytes Windows classifies so in code page 1252 (text.md §12) |
| `toLower`, `toUpper` | Byte strings of 256 bytes: each byte's lowercase and uppercase form |
| `phoneSet` | `[ipa, name]` pairs: the phone set the engine gives the SAPI 4 phoneme converter for prosody transplants (engine.md §13.4), 71 in the original's order. `ipa` is a list of UTF-16 code units, `name` a byte string |

**Phonetic symbols.** Symbols 1–52 have an engine name and an internal symbol of 1 or 2 bytes; symbol 53 (`ɫ`, written `l`) has neither and serves only the IPA conversion. Names and IPA codes can repeat (`ao` is symbols 1 and 8): a search takes the first. An internal symbol holds bytes 0x20–0xaf other than ESC, and an IPA code is 0 (none) or a UTF-16 code unit that is not a surrogate.

**Checks the reader makes.**
- Each word list is strictly ascending in signed byte order, a word before any longer word it starts. The original searches the lists with a binary search in that order that needs an exact length (text.md §6.2), so an exact match in the engine finds the same words. No word is empty or holds a NUL.
- `bestFit` maps each code unit once, to one byte other than `?`, and holds no surrogate or U+0000.
- No byte is both upper and lower; `alphabetic` is upper and lower together; every alphabetic byte and digit is alphanumeric (Windows adds ¹ ² ³). A byte lowercases to itself or, if it is upper, to a lower byte, and the same the other way round.
- Each `phoneSet` IPA string has 1 to 10 code units, none of them 0, and appears once: the converter keeps the first name of a string, and only those are stored. Each name has 1 to 10 printable characters and no space.

The converter checks more: the classes are the runtime classes, and `isdigit` is 0–9 only.

## 12. `resamplers.bin`

The tap tables of the resamplers (synthesis.md §13.2), as the original stores them.

| Type | Field | Meaning |
|---|---|---|
| char[4] | magic | `WRSM` |
| uint32 | version | 1 |
| 3 × {uint32 count; float32 taps[count]} | tables | In this order: 22050 → 8000 Hz (401 taps), 22050 → 16000 Hz (601), 8000 → 11025 Hz (401) |

The 11025 → 8000 Hz resampler uses the second table: the original holds an identical copy of it, which the converter checks. The reader checks each count and that every tap is finite. The rates, the factors and the history lengths are in synthesis.md §13.2, not in the file.

## 13. Generated at load, not stored

- The sine table and fade window (synthesis.md §3).
- The noise table: `rand` from seed 1 (voice-format.md §8).

## 14. The converter

The converter is not part of this repo. It writes the pack from the original engine's files: the five `.vce` files, the five `.cfg` files and the engine's own tables (the reverb presets, `Visual` tables, prosody tables, rule modules, dictionaries, built-in exceptions, text tables, phone set and resamplers' taps), plus the code page 1252 tables recorded from Windows. It first checks the eleven original files against the pinned SHA-256 values. It rejects every feature that voice-format.md and effects.md say no shipped file uses, so the engine never has to support them:
- a `VcHeader` with another signature, or with `features & 0x8000`;
- `PhoneFile` lines with kind ≠ 0 or out of order;
- `TreeImage` layouts with flags, masks of other than 64 bits, `STATEID` features, more than one state per phone, word-position-independent trees, a silence phone other than `SIL`, a word-end right context, links back to earlier nodes, or trees that don't reach every senone;
- `Senone` records whose phone and index don't match the trees;
- `AltUnits` entries with any feature units.md §5 lists, or with a replacement phone that is `SIL` or has no tree;
- a phone whose name matches no `Visual` entry;
- inventory records that don't decode exactly, and periods of 2N samples or more;
- `.vce` files with embedded variants, and variants whose parent isn't a `.vce` voice;
- variant records other than types 1, 2, 4 and 5, records whose size doesn't match their type, a reverb preset outside 1–7, more than one reverb in a variant, and character tags other than empty or `\Chr="name"\` with 1 to 78 characters and no quote or backslash;
- reverb presets with more than four lines, a delay outside the range above, or a modulation above 0.125;
- analyser unit lists whose strings are not unique and ascending as unsigned bytes (so an exact match finds what the original's binary search finds), strings of more than three phones, a string whose unit is outside the id range of its length, and pair values with bit 15 or both flags, or whose unit when no string matches is not a unit of two phones;
- rule modules that fail any check of section 9 or these (every byte of the code area belongs to one rule, output lengths match, marker searches land where the structure says), class or series values other than 0 and 1, natives other than the ones the analyser implements, and missing entry groups;
- dictionaries that fail section 10's checks and guarantees, and built-in exceptions that don't use the phone table of `prosody.json`;
- text tables that fail section 11's checks.

It also checks the visible modes against the list the original engine reports (names, GUIDs, speakers, styles, gender, age, PC or telephone, PREFERRED).

`Cdb`, feature names, training paths and unused header fields are not converted.
