# TruVoice: data pack

The files the Go engine reads instead of the TruVoice engine's own tables. A converter that is not part of this repo writes them from `tv_enua.dll`; the engine (`truvoice/datapack`) reads only these files. Like the Microsoft voices' pack ([../whistler/datapack.md](../whistler/datapack.md)), it is a separate download and is never added to this repo or to the app binary.

**Status.** Format version 1 holds the synthesizer's tables ([synthesizer.md](synthesizer.md) §3 and §4) and the pipeline's ([pipeline.md](pipeline.md)). The tables of the other stages are added with the parts that use them. Until the first release with TruVoice the format can change without a version bump; after it, any change that an older engine can't read raises `formatVersion`.

## 1. Layout

```
manifest.json
synthesizer.json
pipeline.json
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
| `generatedFrame` | 22 bytes | The frame a generated segment starts from, track by track |
