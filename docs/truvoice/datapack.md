# TruVoice: data pack

The files the Go engine reads instead of the TruVoice engine's own tables. A converter that is not part of this repo writes them from `tv_enua.dll`; the engine (`truvoice/datapack`) reads only these files. Like the Microsoft voices' pack ([../whistler/datapack.md](../whistler/datapack.md)), it is a separate download and is never added to this repo or to the app binary.

**Status.** Format version 1 holds the renderer's tables ([synthesizer.md](synthesizer.md) §3). The tables of the other stages are added with the parts that use them. Until the first release with TruVoice the format can change without a version bump; after it, any change that an older engine can't read raises `formatVersion`.

## 1. Layout

```
manifest.json
synthesizer.json
```

**Conventions.** JSON files are UTF-8. Every table value is an integer. Readers check every count and range when they load a file, so a damaged pack fails to load instead of failing during synthesis.

## 2. `manifest.json`

| Field | Meaning |
|---|---|
| `format` | `"opensapi4-truvoice"` |
| `formatVersion` | 1 |
| `files` | Every other file of the pack, mapped to the SHA-256 of its contents |

## 3. `synthesizer.json`

| Field | Values | Meaning |
|---|---|---|
| `halfCosine` | 256 words | The pulse's cosine table h (synthesizer.md §3.3): about `32760 · cos(π · i / 256)`, as the original stores it |
| `sourceShapes` | 32 words | The closing segment's shape, by source type (synthesizer.md §3.2) |
| `outputFilters` | 10 values, −100 to 100 | The output filter's percentage, by output-filter voice (synthesizer.md §3.6) |

Words are −32768 to 32767.
