# TruVoice: data pack

The files the Go engine reads instead of the TruVoice engine's own tables. A converter that is not part of this repo writes them from `tv_enua.dll`; the engine (`truvoice/datapack`) reads only these files. Like the Microsoft voices' pack ([../whistler/datapack.md](../whistler/datapack.md)), it is a separate download and is never added to this repo or to the app binary.

**Status.** Format version 1 holds the synthesizer's tables ([synthesizer.md](synthesizer.md) §3 and §4), the pipeline's ([pipeline.md](pipeline.md)) and the parameter generator's transition shapes, phonemes, classes, rules, articulators and action 4's tables ([parameters.md](parameters.md) §2.5, §3 to §5, §9.2 and §12) and its vowel database (§11), the prosody stage's pitch ranges, pauses, durations, duration grids and function words' rules ([prosody.md](prosody.md) §3.4, §4.3 and §6), the lexical stage's function words' lists, dictionary, affixes, contexts and letter-to-sound's rules ([lexical.md](lexical.md) §3.4, §6 and §8 to §11), the escape parser's and the TextIn tokenizer's tables ([input.md](input.md) §1 and §4), the normalizer's program and lists ([normalizer.md](normalizer.md) §2), what TextData converts texts with ([engine.md](engine.md) §1) and the modes (§3), what the visual events report ([engine.md](engine.md) §2.6), what Phoneme converts its phonemes with (§6), and the pre-formatter's words for the spreadsheet, address and e-mail contexts ([preformatter.md](preformatter.md)). The tables of the other stages are added with the parts that use them. Until the first release with TruVoice the format can change without a version bump; after it, any change that an older engine can't read raises `formatVersion`.

## 1. Layout

```
manifest.json
synthesizer.json
pipeline.json
parameters.json
vowels.json
prosody.json
lexical.json
dictionary.json
input.json
normalizer.json
text.json
modes.json
engine.json
preformatter.json
```

**Conventions.** JSON files are UTF-8. Every table value is an integer. Readers check every count and range when they load a file, so a damaged pack fails to load instead of failing during synthesis.

## 2. `manifest.json`

| Field | Meaning |
|---|---|
| `format` | `"opensapi4-truvoice"` |
| `formatVersion` | 1 |
| `files` | Every other file of the pack, mapped to the SHA-256 of its contents |

## 3. `synthesizer.json`

Words are −32768 to 32767. Section numbers are synthesizer.md's.

| Field | Values | Meaning |
|---|---|---|
| `halfCosine` | 256 words | The pulse's cosine table h (§3.3): about `32760 · cos(π · i / 256)`, as the original stores it |
| `sourceShapes` | 32 words | The closing segment's shape, by source type (§3.2) |
| `openPhases` | 32 values, 0 to 32767 | The open phase's length factor, by source type (§4.3 step 4) |
| `sourceAmplitudes` | 32 words | The pulse amplitude's factor, by source type |
| `phaseSteps` | 140 words | The pulse's phase step, by open phase length: about `102940 / i` |
| `amplitudeScales` | 18 values, 0 to 65535 | The pulse amplitude's second factor, by a voice's `amplitude` |
| `jitterDepths`, `shimmerDepths` | 16 values each | Jitter (0 to 32767) and shimmer (words) depths, by a voice's `jitter` and `shimmer` |
| `deviations` | 64 words | The random deviations jitter and shimmer draw from |
| `decibels` | 213 words | About `8192 · 10^((i − 200) / 20)` (§4.4) |
| `formantLevels` | 121 words | The level of a formant, by frequency in 32 Hz steps (§4.3 step 9) |
| `proximity` | 118 words | The level correction for two formants this many 32 Hz steps apart |
| `voices` | 10 objects | The voices, below |
| `rates` | 2 objects | The tables of each sample rate, named `"11025"` and `"8000"`, below |

`jitterDepths` and `deviations` must not allow a pitch period of 0: `8 · max(jitterDepths) · max(|deviations|)` stays below `32767 · 32767`.

A **voice** (§4.3 step 2) has:

| Field | Values | Meaning |
|---|---|---|
| `scales` | 7 values, −100 to 100 | Percentages added to F1, B1, F2, B2, F3, B3 and F4 |
| `bandwidth4` | 0 to 719 | B4 in Hz |
| `formant5`, `bandwidth5` | 0 to 32767, 0 to 719 | F5's lowest frequency and B5, in Hz |
| `jitter`, `shimmer` | 0 to 15 | Indexes into `jitterDepths` and `shimmerDepths` |
| `aspiration` | 0 to 107 | Added to the aspiration amplitude |
| `sourceType` | 0 to 31 | The glottal source's type |
| `amplitude` | 0 to 17 | Index into `amplitudeScales` |
| `sourceGain` | word | The pulse derivative's gain (block word 17) |
| `outputFilter` | −100 to 100 | The output filter's percentage when it is the output-filter voice (§3.6) |

A **rate** (§4.2, §4.3) has:

| Field | Values | Meaning |
|---|---|---|
| `resetBlock` | 40 words | The block after a reset |
| `sourceStage` | 3 words | A, B and −C of the source stage (block words 37–39) |
| `nasalZeroB` | word | The nasal zero's B factor |
| `radii`, `squaredRadii` | 180 words each | About `8192 · e^(−π · 4j / rate)` and `8192 · e^(−2π · 4j / rate)`, by bandwidth in 4 Hz steps |
| `cosines` | 700 words | About `16384 · cos(2π · 8i / rate)`, by frequency in 8 Hz steps |
| `nasalZeroGains` | 18 words | The nasal zero's gain, by FNZ in 16 Hz steps |

## 4. `pipeline.json`

| Field | Values | Meaning |
|---|---|---|
| `voices` | 10 objects | Each voice's own `pitch` (50 to 400 Hz), `rate` (rate index, 0 to 25) and `speed` (50 to 250 words a minute) ([pipeline.md](pipeline.md) §4) |
| `initialFrame` | 22 bytes | The frame the synthesizer's ring starts with ([synthesizer.md](synthesizer.md) §5.1), track by track |
| `generatedFrame` | 22 bytes | The track levels the escape parser starts with, track by track: a reset gives them to the parameter generator, whose generated segments `g` are made of them ([parameters.md](parameters.md) §17.5) |

## 5. `parameters.json`

| Field | Values | Meaning |
|---|---|---|
| `shapes` | 31 lists | The transition shapes ([parameters.md](parameters.md) §2.5): shape `i` is at most `i` weights, 1 to 255, the first 255. Shape 0 is empty, and shapes 23 to 30 have one weight fewer than their number |
| `phonemes` | 59 objects | The phonemes in their order ([parameters.md](parameters.md) §3.1), below |
| `classes` | 6 lists of 128 bytes | The class bytes of the symbols 0 to 127 ([parameters.md](parameters.md) §3.2): `classes[p][s]` is symbol `s`'s byte `p`. The prosody stage tests them too ([prosody.md](prosody.md) §2.1) |
| `voices` | 10 objects | Per voice: `tracks`, the targets of tracks 18 to 20, and `fourthFormantLimit`, the highest F4 target in Hz ([parameters.md](parameters.md) §4.1 and §6) |
| `categoryWeights` | 4 lists of 4 | The weight, Q15, for a phoneme's category after the previous one's: `categoryWeights[previous][current]` ([parameters.md](parameters.md) §4.1) |
| `defaultShapes` | 22 numbers, 0 to 30 | Each track's forward shape before the rules ([parameters.md](parameters.md) §4.1) |
| `ruleMatrix` | 59 lists of 59 | The rule set for each current phoneme and previous phoneme ([parameters.md](parameters.md) §5.2) |
| `ruleSets` | lists of rules | The rule sets, below |
| `articulatorShapes` | 8 numbers, 0 to 30 | Each articulator's forward shape ([parameters.md](parameters.md) §9.2) |
| `neutralFormants` | 3 numbers | The F1 to F3 in Hz that action 4 moves a vowel toward, the more the shorter it is ([parameters.md](parameters.md) §12.2) |
| `reductionWeights` | 16 numbers, 0 to 32767 | The Q15 weight of that move, by the vowel's frames: entry `min(10f, 255) / 16` |
| `liquids` | object | The formants' trajectories through the liquids ([parameters.md](parameters.md) §14.3), below |
| `segmentFrames` | 16 lists of 22 bytes | The frames of the generated segments `t`, by kind, track by track; 255 is filled in ([parameters.md](parameters.md) §17.5). Kinds 10 to 15 are the bytes after the original's table, which it reads for them |
| `toneLevels` | 16 bytes | What a tone's voicing goes down by, by its frequency in 160 Hz steps |

A phoneme:

| Field | Values | Meaning |
|---|---|---|
| `symbol` | one character | The phoneme's symbol |
| `formants` | 4 numbers | F1 to F4 targets, Hz |
| `bandwidths` | 3 numbers | B1 to B3 targets, Hz |
| `voicing` | 0 to 255 | Track 0's (AV) target |
| `amplitudes` | 8 bytes | Tracks 1 to 8's targets |
| `nasalZero` | number | Track 16's (FNZ) target, Hz |
| `frontness` | Q15 | The phoneme's frontness |
| `place` | 0 to 9 | Its place |
| `articulator` | 0 to 7 | Its articulator ([parameters.md](parameters.md) §9.2) |
| `offglide`, `endFrontness`, `transition`, `offset` | 7 numbers, Q15, byte, 0 to 32767 | Only the vowels with an offglide: the offglide's F1 to F4 and B1 to B3 in Hz, the frontness the vowel ends with, its transition, and the Q15 share of its frames that its start takes ([parameters.md](parameters.md) §12.5) |
| `burst` | 2 lists of 4 bytes | Only `B P D T G K`: AV, AF, AH and AB before a vowel, and otherwise |

A rule is `{"if": conditions, "set": assignments, "do": actions}` ([parameters.md](parameters.md) §5):
- a condition is `{"flag": f, "is": b}` for flag `f` (0 to 10), `{"phoneme": who, "plane": p, "bits": m, "is": b}` for class `(p, m)`, `m` one bit, or `{"phoneme": who, "in": symbols, "is": b}`, with `who` one of `current`, `next`, `afterNext` and `beforePrevious`;
- an assignment is `{"to": place, "set": n}`, `"add"` or `"subtract"` instead of `"set"`, or `{"to": place, "copy": place}`. A place is `[field, track]` for the fields with tracks (`burstLevel`: 0 to 2), or `[field]`;
- the actions are numbers from 1 to 16.

The liquids:
- `R`, `L` and `Y`: 4 objects each, for F1 to F4, with `rows` and `contexts`, one list of contexts for each row but the last. A context is `[symbol, symbols]`: the row is for that symbol before the liquid, or for `Y` that vowel, with any of the symbols after it;
- `rAfterVowel`: 3 lists of 1, 2 and 2 rows, F1 to F3 of `R` after a vowel; `rAcrossMark`: 4 lists of 1, 5, 4 and 4 rows, F1 to F4 of `R` after a word mark;
- `targetsAcrossMark`: 4 lists of 2, 7, 5 and 4 numbers, the F1 to F4 targets in Hz of a consonant before a word mark and `R`.

A row is 7 numbers: the kinds of its start, middle and end (0 to 2), its duration in percent (0 to 100), and the values of its start, middle and end in Hz.

## 6. `vowels.json`

The vowel database ([parameters.md](parameters.md) §11). Bytes are frame bytes of their track (parameters.md §2.2).

| Field | Values | Meaning |
|---|---|---|
| `contexts` | 128 numbers, −1 to 33 | Each symbol's context, −1 for none |
| `vowels` | 24 objects | The database's vowels in its order, `@ \| O b f U A E I y a e i v o w u 3 r g 5 k 4 c`, below |

A vowel:

| Field | Values | Meaning |
|---|---|---|
| `symbol` | one character | The vowel's symbol |
| `records` | lists of numbers | The records, ordered by left context, then right context. Each starts with its left and right context, 0 to 33. For `@` and `\|` it goes on with F1 to F4 start, middle and end bytes, the four timings (0 to 15) and the B1 to B3 end bytes: 21 numbers. For the others it goes on with F1 to F4 start and end bytes, an entry of each formant's trajectory table, and the B1 to B3 starts, middles and ends in Hz: 23 numbers |
| `trajectories` | 4 lists | Not for `@` and `\|`: F1 to F4's trajectory tables, whose entries are four point bytes and four timings (0 to 15) |
| `alternatives` | objects | Only some vowels: `{"record": i, "neighbour": n, "symbol": s, "values": v}`, an alternative of record `i`, used when the node before the previous phoneme (`n` is `beforePrevious`) or after the next one (`afterNext`) has symbol `s`. `v` is a record without its contexts |

## 7. `prosody.json`

| Field | Values | Meaning |
|---|---|---|
| `pitchRanges` | 10 numbers, 0 to 32767 | Each voice's pitch range, the Q15 share of the excursion its pitch moves by ([prosody.md](prosody.md) §4.3); 32766 for every voice |
| `pausePercents` | 26 numbers, 0 to 10000 | By rate, 0 to 25: the percent of a pause's length the durations give it, from 2200 at rate 0 to 30 at 19 and up. The slow pauses read the last ones after an `r` command ([prosody.md](prosody.md) §3.4) |
| `slowPausePatterns` | 9 numbers, 0 to 65535 | By rate, 0 to 8: the 16 slots of slow speech's pauses between words, a bit each ([prosody.md](prosody.md) §3.4) |
| `durations` | 128 pairs of numbers, 0 to 255 | By symbol: its inherent and shortest durations in frames ([prosody.md](prosody.md) §6). Below the space the pairs are bytes of other tables, which no phoneme reads |
| `durationPercents` | 26 numbers, 0 to 1000 | By rate, 0 to 25: the percent the durations scale by, from 344 at rate 0 to 65 at 25 ([prosody.md](prosody.md) §6.2 and §6.3) |
| `durationGrids` | 6240 numbers, 0 to 255 | The bytes of the class durations' grids, rows of 20 columns; a grid has 8 rows, but `M`'s and `N`'s, grids 29 and 30, have 12. A position before the first byte reads 0 ([prosody.md](prosody.md) §6.7) |
| `gridOffsets` | 38 numbers | Each grid's first byte in `durationGrids`, at least its size before its end. Grids 0 to 23 are the vowels' ([prosody.md](prosody.md) §6.7), 24 to 28 the fricatives' (§6.10) and 29 to 37 the sonorants' (§6.9). The grids lie in another order than this list's: 0 to 20, 22, 23, 21, then the others |
| `breakReductions` | 47 numbers, 0 to 255 | By a vowel's grid: the frames a vowel after a glide loses before a break, from 0 when unstressed and from 23 when stressed ([prosody.md](prosody.md) §6.7, step 19) |
| `functionWordRules` | 16 lists of rules | By word class: the rules for a function word's vowel ([prosody.md](prosody.md) §6.8). A rule is `{"back": b, "symbols": s, "conditions": c, "tests": t, "frames": f}`: `b` 0 to 255, `s` 1 to 255 symbols below 128, `c` bits 0 to 127, `f` 0 to 255. A test is `{"kind": k, "direction": d, "steps": n, "value": v}`: `k` 0 to 5, `d` 0 or 1, `n` 0 to 255 and 0 for direction 0, `v` a class `(p, b)` as `p · 256 + b` below 1536 for kinds 0, 3 and 4, a symbol below 128 for kind 5, and 0 to 255 for kinds 1 and 2. Classes 0 and 6 have none |

## 8. `lexical.json`

A **word** here is a string of symbols 1 to 127.

| Field | Values | Meaning |
|---|---|---|
| `functionWords` | Lists | The lists of function words, in the order the lexical stage searches them ([lexical.md](lexical.md) §3.4). A list is `{"words": w, "class": c, "exceptions": e}`: `w` a word, the list's words each between two `#`, which a word's spelling is searched in; `c` 0 to 255, the class its words get; `e`, which can be left out, a list of `{"word": s, "class": c}`, a spelling between two `#` that gets class `c` instead |
| `functionWordEndings` | Endings | The endings tested in turn when no list has a word's spelling: `{"ending": n, "class": c, "exceptions": e}`, `n` a word, the ending before the spelling's last `#`, with `c` and `e` as above |
| `suffixes`, `prefixes` | 27 numbers | The list of affixes the lookup tries for a word that ends, or starts, with each letter `@` to `Z` ([lexical.md](lexical.md) §9.1): an index into `affixLists`, or -1 for none |
| `affixLists` | Lists | The lists of affixes, each a list of indexes into `affixes`, tried in their order |
| `affixes` | Affixes | `{"letters": l, "if": c, "lookup": b, "class": k, "stress": s, "next": n, "silentE": e, "keepsI": i}`: `l` the affix's letters, `@` to `Z`, in reading order; `c` its context; `b` whether the stem it leaves is looked up; `k` 0 to 255, the word class a first suffix gives; `s` -128 to 127, its stress; `n` the index of the list tried after it; `e`, which can be left out for 0, 0 to 2, whether the stem may have lost a silent E (lexical.md §9.3); `i`, which can be left out for false, whether a final I stays |
| `stemConditions` | Contexts | `{"silentE": c, "noSilentE": c, "withoutE": c, "lostT": c}`, the contexts of a stem's end the lookup tests (lexical.md §9.3) |
| `reductionContext` | Context | The context letter-to-sound's vowel reduction tests (lexical.md §11.2) |
| `letterRules` | 28 lists of rules | Letter-to-sound's rules for each letter `@` to `[`, in the order it tries them (lexical.md §10.2). A rule is `{"before": b, "phonemes": p, "joinsPrefix": j, "weight": w, "if": c, "masks": [m, n], "flags": [f, g]}`: `b` the letters before the letter, symbols 1 to 127, in the word's order; `p` the phonemes that take their place, symbols 32 to 126, in the word's order; `j`, which can be left out for false, whether it joins the prefix to the stem; `w`, which can be left out for 0, -128 to 15, its weight; `c` its context; `m` and `n` 0 to 4294967295, the masks it tests following and ending with, and `f` and `g` the flags it sets them from. A rule that joins the prefix needs a weight or phonemes, and each list ends with a rule that holds whatever the letters: no letters before, masks 0 and an empty context |

A **context** ([lexical.md](lexical.md) §8) is a list of items: `"end"`, or a clause `{"test": t, "match": m}`, `t` 1 to 255 and `m`, which can be left out when empty, a list of elements: a string of one symbol 32 to 126, or `[p, b]`, a class, the bits `b`, 1 to 255, of class byte `p`, 0 to 5. Only tests whose high three bits are 0 or 7 have elements, and a sequence has at least one element fewer than its nodes. The items after a context's first end are those the original stores after it, which a clause that reads past its end can reach.

## 9. `dictionary.json`

A **word** here is a string of symbols 1 to 127, and in `homographs`, `nounBeforeColon` and `pastTense` words each between two `#`.

| Field | Values | Meaning |
|---|---|---|
| `words` | Entries | The dictionary's entries ([lexical.md](lexical.md) §6.1). An entry is `{"word": w, "class": c, "phonemes": p}` or `{"word": w, "class": c, "stressPattern": s}`: `w` a letter `A` to `Z` and up to 20 more of `@` and `A` to `Z`; `c` 0 to 15; `p` 1 to 15 symbols below 128, not only the digits `0`, `1` and `2`; `s` 0 to 255. The lookup searches the entries of the same first letter and length in their order here |
| `homographs` | A word | The homographs, each between two `#` ([lexical.md](lexical.md) §6.3) |
| `verbEndings` | Words | The endings after which a homograph's verb's entry is taken |
| `pluralEndings` | Words | The endings that make a homograph plural |
| `nounBeforeColon` | A word | The homograph, between two `#`, whose verb's entry a colon after it passes over |
| `pastTense` | A word | The word, between two `#`, whose first entry the forms of be and have, and `he`, `she` and `it`, pass over |


## 10. `input.json`

| Field | Values | Meaning |
|---|---|---|
| `highCharacters` | 64 strings | The letters the escape parser writes for each character from 192 to 255 in text ([input.md](input.md) §1.2): one or two characters from 32 to 126 |
| `filterRates` | 10 numbers, 0 to 25 | The highest rate index each filter value allows ([input.md](input.md) §1.4, `f`, `r` and `v`) |
| `levelLimits` | 22 numbers, 0 to 255 | The highest level the `l` command gives each track |
| `tokenClasses` | 256 numbers, 0 to 255 | Each character's token classes ([input.md](input.md) §4.1): bit 0x01 separator, 0x02 trailing, 0x04 letter, 0x08 digit, 0x10 (not read), 0x20 punctuation, 0x40 read again |
| `symbols` | 13 pairs | The symbols, each a character from 128 to 255 and the word the tokenizer says for it ([input.md](input.md) §4.4, rule 4) |
| `minus` | string | What a minus sign before a number says (rule 1) |
| `splitWords` | 2 pairs | The words the tokenizer splits and what they say (rule 5) |
| `meridiem` | pair | `am` and what it says after a number (rule 6) |
| `months` | 2 pairs | The months the tokenizer writes out and what they say (rule 7) |
| `exclamation` | string | The command the tokenizer puts before a quoted exclamation (rule 8) |
| `wordStart` | string | The token word start puts before a word ([input.md](input.md) §4.4) |
| `quoteMarks` | string | The quote marks of quote mode ([input.md](input.md) §4.6) |
| `quoteStart`, `quoteEnd` | strings | What quote mode says at the start and the end of quoted text |

The tokenizer's words are ASCII, without NUL; `exclamation` holds the escape character.

## 11. `normalizer.json`

| Field | Values | Meaning |
|---|---|---|
| `program` | Blocks | The normalizer's program ([normalizer.md](normalizer.md) §2), each block its bytes in hex: a condition and its operands, a branch byte and its two runs of actions. The program is the blocks one after another; a place is an offset in it |
| `strings` | Strings | The string table: the strings one after another, each followed by a NUL. A string operand is the offset of one of them |
| `punctuation` | string | The characters condition 3 takes besides the letters and the digits 1 to 5 |
| `lists` | 22 lists | The word lists, each a list of entries `[w, r]`: `w` a word, which a match spells, and `r` its replacement, which can be empty |

The strings, the punctuation and the lists' words and replacements are printable ASCII. A block's conditions and actions are those normalizer.md §2.2 and §2.3 list, with all their operands, and its runs end at its end: a register is below 12, a flag below 32, a list one of the lists, a class one of the classes condition 8 tests, a string offset one where a string starts, and a goto or a call goes to a block's start. Neither run of the last block is empty, and each ends in a goto or a return, so that the program never runs past its end.

## 12. `text.json`

| Field | Values | Meaning |
|---|---|---|
| `bestFit` | Pairs `[c, b]` | Code page 1252's best fit, recorded from Windows: each UTF-16 code unit `c` that `WideCharToMultiByte` turns into a byte other than `?`, and the byte `b` ([engine.md](engine.md) §1.1). Each `c` is listed once, 1 to 65535 but no surrogate; each `b` is 1 to 255 but `?` |
| `wide` | Pairs `[b, c]` | The bytes `MultiByteToWideChar` turns into a code unit other than their own value, recorded from Windows for code page 1252: the byte `b`, 1 to 255, each listed once, and the code unit `c`, 1 to 65535 but no surrogate. Every other byte is its own code unit. Phoneme converts its engine phonemes with them ([engine.md](engine.md) §6.1). The best fit gives each byte but `?` back from its code unit |
| `ipaRules` | Pairs `[s, p]` | The IPA sequences TextData reads ([engine.md](engine.md) §1.5): `s` one to three characters, each listed once, and `p` the engine phonemes it gives |

## 13. `modes.json`

| Field | Values | Meaning |
|---|---|---|
| `modes` | 10 objects | The engine's modes, in order ([engine.md](engine.md) §3.1): each mode's voice is its place |

A **mode** has its mode information: `engine` and `mode` (GUIDs), `manufacturer`, `product`, `name`, `language` (a language ID), `dialect`, `speaker`, `style`, `gender` (0 neutral, 1 female, 2 male), `age`, `features`, `interfaces` and `engineFeatures`. Each mode has a speaker and a name.

## 14. `engine.json`

| Field | Values | Meaning |
|---|---|---|
| `visuals` | Entries `[s, i, m]` | What a visual event reports for a phoneme symbol ([engine.md](engine.md) §2.6): `s` the symbol, `i` its IPA character (0 for none) and `m` its mouth shape, 8 values of 0 to 255 in the order of `TTSMOUTH` (height, width, upturn, jaw open, upper teeth, lower teeth, tongue, lip tension), or null for none |
| `ipa` | Pairs `[s, t]` | The IPA string Phoneme gives for a symbol of its listing ([engine.md](engine.md) §6.4): `s` the symbol, a printable ASCII character listed once, and `t` its string, one character or more, without NULs. A symbol not listed gives nothing |

Each symbol of `visuals` is a printable ASCII character, listed once, with an IPA character or a mouth shape. An IPA character is 1 to 65535 but no surrogate. A symbol not listed has neither.

## 15. `preformatter.json`

| Field | Values | Meaning |
|---|---|---|
| `spreadsheet` | Object | The words the pre-formatter writes in the spreadsheet context ([preformatter.md](preformatter.md) §2), below |
| `address` | Object | The words the pre-formatter knows in the address and e-mail contexts ([preformatter.md](preformatter.md) §3.4, §3.5, §3.7, §3.9), below |

The **spreadsheet** words:

| Field | Values | Meaning |
|---|---|---|
| `months` | 12 words | The months' names, January first (§2.1) |
| `denominators` | 9 words | The words of the denominators 1 to 9 (§2.2) |
| `teens` | 10 words | The words of the denominators 10 to 19 |
| `tens` | 8 words | The words of the denominators 20, 30 and so on to 90 |
| `ordinals` | 9 words | The words of the units 1 to 9 after other tens |
| `otherUnits` | 2 words | The words of a second character that isn't a digit: after `1`, and after another character |
| `minus` | Word | What a minus sign and an opening bracket are written as, with a space after it |
| `and` | Word | What a fraction writes after a line's first word |
| `exponent` | Word | What an exponent's `E` is written as, between spaces |

Each word is printable ASCII, one character or more.

The **address** words:

| Field | Values | Meaning |
|---|---|---|
| `functionWords` | 59 words | The function words, after which a dot ends no sentence and which a sentence's start skips |
| `abbreviations` | 168 words | The abbreviations, after whose dot no sentence starts and one space follows |
| `spacedAbbreviations` | 2 words | The abbreviations that get two spaces after their dot before a capitalized word, as other words do |
| `words` | Array of objects | The list words, written out ([preformatter.md](preformatter.md) §3.5), below |
| `numberWords` | 19 words | The words whose dot gets a lone dot after it before two spaces or more (§3.5) |
| `headers` | Array of objects | The e-mail headers ([preformatter.md](preformatter.md) §3.7), below |
| `messageStart` | Word | What a message header line writes where a message starts (§3.8) |
| `letters` | Object | The tables that write words in capitals of two letters or more ([preformatter.md](preformatter.md) §3.9), below |

Each function word, abbreviation and number word is one lower-case letter or more, `a` to `z`, and each spaced abbreviation is one of the abbreviations. The message start is printable ASCII, one character or more.

Each **list word**:

| Field | Values | Meaning |
|---|---|---|
| `word` | Word | The word, case and all: letters `A` to `Z` and `a` to `z` and `'`, one character or more; each word comes once |
| `expansion` | Word | What it is written as, printable ASCII, one character or more |
| `kind` | `abbreviation`, `title`, `plain` or `capital` | How its dot and the spaces after it are written; a capital word is written as a plain one, and only when no word in capitals stands near it |
| `dotted` | `true` or absent | Whether it is a list word only before a dot |
| `anyCase` | `true` or absent | Whether a word with a small letter and a capital after its first letter is this one when its first letter as it is and its other letters lowered are this word ([preformatter.md](preformatter.md) §3.5) |

A capital word's expansion is the word after a `~`.

Each **header**:

| Field | Values | Meaning |
|---|---|---|
| `name` | Word | The header's name, printable ASCII without spaces and colons, case and all; each name comes once |
| `outside` | Action or absent | What a line of it does outside a header block |
| `block` | Action or absent | What a line of it does in a header block |
| `after` | Action or absent | What a line of it does right after a header block |
| `outsideNames` | Count, 0 if absent | How many messages must have started for a line of it outside a header block to be read as names; 0 is never |
| `insideNames` | Count, 0 if absent | The same in and right after a header block |
| `continues` | `true` or absent | Whether a value of it that ends in a comma, when it isn't read as names, goes on in the next line as a dropped one (§3.7, §3.8) |
| `clearsAuthor` | `true` or absent | Whether a message it starts outside a header block has no `Author:` line yet (§3.8) |
| `quotedTwice` | `true` or absent | Whether in or right after a header block two `>` may quote a line of it (§3.7) |

An action is one of `dropped`, `block`, `message`, `sender`, `kept`, `date`, `subject` and `author` (§3.8). Where a header has no action and isn't read as names, a line of it isn't a header line.

The **letters** ([preformatter.md](preformatter.md) §3.9):

| Field | Values | Meaning |
|---|---|---|
| `twoLetters` | Words of 2 letters | The words of two capitals that are said, not spelled |
| `onsets` | Words of 2 letters | The consonant pairs that may start a word of three capitals |
| `codas` | Object | The pairs that may end a word of three capitals after a vowel: each pair's vowels |
| `firstPairs` | Words of 2 vowels | The vowel pairs that may start a word of three capitals |
| `lastPairs` | Words of 2 vowels | The vowel pairs that may end one |
| `spelled` | Words of 2 to 6 letters | The words that are always spelled |
| `markedSpelled` | Words of 2 or 3 letters | The words that are spelled when marked |
| `unkept` | Words of 3 letters | The words of three capitals that are never said |
| `kept` | Words of 3 to 5 letters | The words of three to five capitals that are always said |
| `capitalsOnly` | Words of 3 to 6 letters | The spelled and kept words that are spelled or said so only in capitals, not in a word with a small head ([preformatter.md](preformatter.md) §3.12) |
| `respelled` | Object | The words written otherwise: each word's writing |
| `heads` | Words of 3 letters | The first three capitals of a word of four that are said apart from the last |
| `tails` | Words of 3 letters | The last three capitals of a word of four that are said apart from the first |
| `openHeads` | Words of 3 letters | The first three capitals that may be said in a word of four with a soft start |
| `unkeptHeads` | Object | The first three capitals of a word of four that is said only before certain letters: each one's letters |
| `clusters` | Object | The consonant pairs that start a word of four, or follow the vowel that starts one, before certain consonants: each pair's consonants |
| `vowelCodas` | Words of 2 letters | The pairs that may end a word of four after a first pair, or its rest after a `Q`, `XO` or `XU` |
| `codaPairs` | Words of 2 letters | The pairs that may end a word of four after a vowel |
| `pieces` | Words of 4 or 6 letters | The first or last capitals of a word of five or more that are said apart from the rest |
| `longHeads` | Words of 3 letters | The first three capitals of a word of five or more that are said apart from the rest, when no piece is |
| `longTails` | Words of 2, 3 or 5 letters | The last capitals of a word of five or more that are said apart from the rest, when no piece or head is |
| `unsplit` | Words of 4 to 9 letters | The words that keep a piece, head or tail they start or end with from being said apart |
| `readings` | Object | The words read as abbreviations near a loud word: each word's reading |

Each word is capitals `A` to `Z`, listed once, and a vowel is `A`, `E`, `I`, `O` or `U`. A coda's pair is two capitals and its vowels one vowel or more; a respelled word is two to six capitals and a reading's word two to nine, and their writing printable ASCII, one character or more. An unkept head is three capitals and a cluster two, each mapped to one capital or more, none twice. No word is both kept and unkept, and each capitals-only word is spelled or kept.
