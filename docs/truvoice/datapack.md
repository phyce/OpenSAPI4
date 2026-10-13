# TruVoice: data pack

The files the Go engine reads instead of the TruVoice engine's own tables. A converter that is not part of this repo writes them from `tv_enua.dll`; the engine (`truvoice/datapack`) reads only these files. Like the Microsoft voices' pack ([../whistler/datapack.md](../whistler/datapack.md)), it is a separate download and is never added to this repo or to the app binary.

**Status.** Format version 1 holds the synthesizer's tables ([synthesizer.md](synthesizer.md) §3 and §4), the pipeline's ([pipeline.md](pipeline.md)) and the parameter generator's transition shapes, phonemes, classes, rules, articulators and action 4's tables ([parameters.md](parameters.md) §2.5, §3 to §5, §9.2 and §12) and its vowel database (§11), and the prosody stage's pitch ranges ([prosody.md](prosody.md) §4.3). The tables of the other stages are added with the parts that use them. Until the first release with TruVoice the format can change without a version bump; after it, any change that an older engine can't read raises `formatVersion`.

## 1. Layout

```
manifest.json
synthesizer.json
pipeline.json
parameters.json
vowels.json
prosody.json
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
