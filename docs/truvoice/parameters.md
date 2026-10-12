# TruVoice: parameter generator

How L&H TruVoice (`tv_enua.dll`) turns timed phonemes into frames for its synthesizer. For each phoneme or pause the parameter generator works out what each of the 22 tracks of a frame should do ([synthesizer.md](synthesizer.md) §4.1), and writes the phoneme's frames into the synthesizer's ring ([synthesizer.md](synthesizer.md) §5.1), rewriting the end of the previous phoneme's where the two blend.

This spec covers the tracks and how the generator writes them (section 2), the phonemes (section 3), the targets a phoneme starts from (section 4), the rules that change them (section 5), how a phoneme is rendered (section 6) and the stage's resets (section 7). The rules' actions, the vowel database and the stage's driver are still to come.

**In short.**
- The generator works track by track. For each phoneme it fills in a **segment** for each track: a mode, how many frames, an onset and a target, two shapes, and a value to pull the previous phoneme's last frames toward. Rendering the segment writes the phoneme's frames of that track.
- A segment's values are in the track's units (Hz for the formants and bandwidths, the frame byte for the amplitudes) and become frame bytes by a shift. Frame bytes wrap around; nothing is clamped.
- Transitions follow 31 fixed shapes: decaying weights from 255 down, one per frame.
- After each phoneme every track moves on by its segment's frames. The ring's written frames end where the slowest track is, and the frame before it is marked for the output stage.
- A phoneme's segments start from its targets in the data pack. Then the first rule that applies, of the rule set for the phoneme and the one before it, changes them and runs its actions. Voicing, the attenuation and the voice adjust them last.

**Exactness.** Ported from the original's code, and checked against it by replaying every call of its track functions recorded in 151 runs (the ten voices, both rates, every speed range, pitch and volume tags, whisper, monotone, emphasis, tones and generated segments, texts spoken one after another in one session, long texts with a long pause, 44 sentences chosen for their sounds and 11 for their clusters of consonants): each of the 232,915 segments rendered (section 2.4), the stage's 129,350 direct writes into the ring (section 2.5), 10,575 advances (section 2.6) and 26,936 room checks and rebases (section 2.7) left the speaking thread's state byte-identical to the original's. So did the 10,575 phonemes rendered (section 6), with their target loading and rules, the rules' actions taken from the recording after checking that each started from the original's state, and the stage's 151 resets and 194 context resets (section 7).

## 1. Arithmetic

All arithmetic is in 32-bit signed integers. Divisions truncate toward zero; shifts of negative numbers are arithmetic. The **Q15 product** of `a` and `b` is `a · b / 32767`. A value written into a frame is its low 8 bits.

## 2. Tracks

### 2.1 The tracks

The generator writes the synthesizer's ring ([synthesizer.md](synthesizer.md) §5.1): frame `p` of track `k` is byte `k` of slot `p mod 256`. For each track it keeps:
- `next`: the frame it writes next;
- `earliest`: the first frame it may still change. Frames before it may already be with the synthesizer;
- `last`: the value of the frame before `next`, in the track's units (section 2.2).

It also keeps `furthest`, the highest `next` of the tracks, and a `margin` of frames it keeps free in the ring.

### 2.2 Units

A track's values are in its units; a frame holds them as bytes:

| Track | Units | Byte of a value `v` | Value of a byte `b` |
|---|---|---|---|
| 9 (F1) | Hz | `v >> 2` | `b << 2` |
| 10 (F2) | Hz | `(v − 500) >> 3` | `(b << 3) + 500` |
| 11, 12 (F3, F4) | Hz | `v >> 4` | `b << 4` |
| 13, 14, 15 (B1, B2, B3), 17 (F0) | Hz | `v >> 1` | `b << 1` |
| 16 (FNZ) | Hz | `(v − 192) >> 2` | `(b << 2) + 192` |
| the others | the byte | `v` | `b` |

### 2.3 Segments

A **segment** is what the generator writes on one track for a phoneme. It has seven numbers:

| Field | Meaning |
|---|---|
| `mode` | How the segment is written (section 2.4) |
| `back` | The shape of the pull on the frames before `next`, or how many frames a bridge reaches back |
| `forward` | The shape from the onset to the target, or how many frames a ramp or a bridge reaches forward |
| `frames` | The segment's length in frames |
| `back target` | The value the pull moves the frames before `next` toward |
| `onset` | The value the segment starts from |
| `target` | The value the segment moves to |

### 2.4 Rendering a segment

To render track `k`'s segment:

1. Convert `back target` and `onset`, both raised to 0 if negative, and `target` to bytes (section 2.2); keep their low 8 bits.
2. `room = next − earliest`. If `mode` is odd and `room < 1`, make it even.
3. Write by the mode, from `next` on (the writers are in section 2.5):

| Mode | Writes |
|---|---|
| 1 | pull back: `PullBack(next, back, room, back target)` |
| 2 | `PullForward(next, forward, frames, onset)` |
| 3 | pull back, then as mode 2 |
| 4 | `Fill(next, shape 0, frames, 0, target)`: `frames` frames of the target |
| 5 | pull back, then as mode 4 |
| 6 | `Fill(next, forward, frames, onset, target)` |
| 7 | pull back, then as mode 6 |
| 8 | `Ramp(next, forward, onset, target)` |
| 16 | `Bridge(next, back, forward)` |
| 20 | as mode 6, then as mode 16 |
| any other | nothing |

So modes 9, 17 and 21 write nothing, unless step 2 made them 8, 16 or 20. `next` doesn't move here: the advance does (section 2.6).

### 2.5 The ring writers

The writers work on one track of the ring. Shapes are lists of weights from the data pack ([datapack.md](datapack.md) §5): shape `s` has at most `s` weights, the first 255; shape 0 has none. Moving a frame byte `f` toward a byte `v` by a weight `w` gives the low 8 bits of `f + Q15(v − f, w · 128)`.

- `Fill(p, s, n, onset, target)`: frame `p + i` is `target` moved toward `onset` by weight `i` of the shape, for each weight while `i < n`; then frames up to `p + n − 1` are `target`. A negative `n` writes all the shape's weights and nothing more.
- `PullBack(p, s, n, v)`: frame `p − i` moves toward `v` by weight `i`, for each weight after the first, at most `n` frames.
- `PullForward(p, s, n, v)`: frame `p + i` moves toward `v` by weight `i`, for each weight, at most `n` frames.
- `Bridge(p, b, f)`: with `n = b + f`, `first` frame `p − b` and `last` frame `p + f`: frame `p − b + i` becomes `first + i · (last − first) / n`, for `i` from 0 to `n − 1`. The last frame stays.
- `Ramp(p, n, start, end)`: frame `p + i` becomes `start + (i + 1) · (end − start) / n`, for `i` from 0 to `n − 1`: the line starts after `start` and ends on `end`.
- `Repeat(p, n, v)`: frames `p` to `p + n − 1` become `v`.
- `AddBack(p, s, n, change)`: if the frame before `p` plus `change` is below 0, `change` becomes minus that frame. Then frame `p − i` becomes its value plus `Q15(change, w · 128)`, for each weight `w` after the first, at most `n` frames; on the amplitude tracks (0, 1, 2) a result of 0 or less is 0. The result's low 8 bits are written.

The generator's other functions call the writers too; their sections will say where. For a negative count, `Ramp` and `Repeat` don't stop in the original; the recorded runs never give one.

### 2.6 Advancing

After the generator has rendered a phoneme's segments, every track moves on:
1. `next` moves on by the segment's `frames`;
2. `last` becomes the value of the frame before the new `next`;
3. `earliest` becomes `next − lookahead` ([synthesizer.md](synthesizer.md) §5.1) if that is later.

Then the lowest `next` of the tracks becomes the ring's `written`, the highest becomes `furthest`, and the frame before `written` gets its marker bit. The marked frame of each phoneme or pause is thus the last frame every track has written for it, which the output stage counts ([pipeline.md](pipeline.md) §5).

### 2.7 Room and rebase

- The ring has **room** for `n` frames when `margin + furthest − position + n < 256`, with the synthesizer's `position`.
- **Rebase**: the synthesizer's rebase ([synthesizer.md](synthesizer.md) §5.1). If it moved its counters back, every track's `next` and `earliest` and `furthest` move back by 2048 too.

## 3. Phonemes

### 3.1 The phonemes

The generator knows 59 phonemes, numbered in this order:

| Numbers | Phonemes |
|---|---|
| 0–18 | the vowels with an offglide: `4 5 A E I O U a b c e f g i k r u w y` |
| 19–23 | `3 @ o v \|` |
| 24–29 | `j l L W Y R` |
| 30–33 | `p d h H` |
| 34–44 | `B P D T t J C G K Q q` |
| 45–52 | `V F x X Z S z s` |
| 53–57 | `M m N n ~` |
| 58 | the pause, a space |

The data pack gives each phoneme ([datapack.md](datapack.md) §5) its F1 to F4 targets (`formants`), B1 to B3 (`bandwidths`) and FNZ (`nasalZero`) in Hz, its AV target (`voicing`) and its tracks 1 to 8 (`amplitudes`), a Q15 `frontness`, and a `place`:

| Place | Phonemes |
|---|---|
| 0 | `4 E U` |
| 1 | `A a e i k \|` |
| 2 | `O b c g u w y` |
| 3 | `5 I f r 3 @ o v` |
| 4 | `j l L W Y R` |
| 5 | `p` |
| 6 | `V F x X Z S z s` |
| 7 | `d h H` |
| 8 | `B P D T t J C G K Q q M m N n ~` |
| 9 | the pause |

The vowels with an offglide also have the offglide's F1 to F4 and B1 to B3 in Hz (`offglide`), the frontness they end with (`endFrontness`) and a `transition`. `B P D T G K` have a `burst`: AV, AF, AH and AB before a vowel, and the same otherwise.

### 3.2 Classes

Every symbol below 128 has six **class bytes** in the data pack; their bits sort the symbols into classes. Class `(p, b)` is bit `b` of class byte `p`. The rules can test any class (section 5.2). The generator itself tests these:

| Class | Name here | Phonemes |
|---|---|---|
| (0, 0x02) | sonorant | the vowels, `p`, `j l L W Y R`, `d h H`, the nasals `M m N n ~` |
| (0, 0x04) | voiced | the vowels, `p`, `j l L W Y R d h`, `B D t J G Q q V x Z z`, the nasals |
| (0, 0x10) | nasal | `M m N n ~` |
| (0, 0x20) | stop | `B P D T J C G K q` |
| (0, 0x40) | fricative | `V F x X Z S z s` |
| (2, 0x01) | closure | `B P D T t J C G K Q q` and the nasals |
| (2, 0x02) | vowel | the vowels and `p` |
| (2, 0x10) | labial | `W h B P V F M m` |
| (3, 0x02) | silence | `d H Q` and the pause |

### 3.3 Categories

A symbol's **category** is 0 (vowel) if it is in class vowel, else 3 (closure) if it is in class closure, else 1 (sonorant) if it is in class sonorant, else 2. So category 2 is the fricatives and the pause.

### 3.4 The phonemes around

The generator renders the node its cursor scans ([pipeline.md](pipeline.md) §3): the **current** phoneme. The cursor's done node is the **previous** phoneme and its first node the **next**; all three are segments, phonemes or pauses. The rules also test the node after the next one and the node before the previous one, whatever their type: punctuation and word marks too, and a sentinel, whose symbol is 0.

## 4. Targets

### 4.1 Loading the targets

A phoneme starts from its targets. `P` is the current phoneme's entry in the data pack, `N` the next's:

1. **Frontness.** If the previous phoneme has an offglide and isn't `u`, and the current one isn't `j`, the previous frontness becomes the previous phoneme's `endFrontness`; otherwise it becomes the frontness, still the previous phoneme's. Then the frontness becomes `P.frontness`.
2. **Category.** The previous category becomes the category, and the category the current phoneme's (section 3.3).
3. The aspiration and the fricative aspiration become 51, the three burst levels 0.
4. **Targets.** Tracks 9 to 12 (F1 to F4) get `P.formants`, 13 to 15 (B1 to B3) `P.bandwidths`, 0 (AV) `P.voicing`, 1 to 8 `P.amplitudes`, 17 (F0) twice the node's second value, 18 to 20 the voice's `tracks` (the data pack's `voices`, for the cursor's voice), 21 zero.
5. A vowel with an offglide: the vowel transition becomes `P.transition`, the offglide `P.offglide`.
6. `u` before `j`: the frontness becomes 0.
7. `B P D T G K` get their burst (section 4.2), `t J C` theirs (section 4.3).
8. Track 16 (FNZ) gets `P.nasalZero`.
9. Every track's segment gets the forward shape of the data pack's `defaultShapes`, mode 7 and the node's value as its frames; every track's weight becomes `categoryWeights[previous category][category]`. Tracks 20 and 21 then get mode 4.
10. The next targets of F1 to F3 become `N.formants`; if the next phoneme is number 34 (`B`) or later, those of B1 to B3 become `N.bandwidths`.

The phonemes' other values are what the rules and actions use: the voice onset and burst frames, the burst levels, the frontness, the offglide, the next targets.

### 4.2 Stop bursts

For `B P D T G K`:
1. The burst levels (AV, AF, AH) and track 8's (AB) target become the burst's row before a vowel when the next phoneme is in class vowel, else its other row.
2. Before a phoneme of place 8 or 9: the burst levels become 127 and AB's target 0, except for `K` before a nasal, `Q` or `t`.
3. The **voice onset** becomes 127, or, for a phoneme not in class voiced, the node's byte 0x10.
4. The **burst frames** become 0. Then, if the next phoneme is in class closure but not nasal, and the previous one isn't such a phoneme too:
   - `D`: 1 after `j` and before `B` or `G`, else they stay 0;
   - `K` before `P`, and `P` before a phoneme not in class voiced: the burst levels of AF and AH become 60 and 48, and the burst frames 1;
   - otherwise 127.
5. Otherwise, the first that applies:
   - `K` before a fricative: 1;
   - `G`: 1 before a vowel whose node hasn't flag bit 0x20, else 2;
   - `T` before a fricative: 127;
   - `D`: 127 before a phoneme in class sonorant but not vowel, else 2 after the pause and 1 after anything else;
   - a phoneme not in class labial, not before `p`: 2;
   - else 1.

### 4.3 Affricate bursts

For `t J C`, unless the next phoneme is of place 8 or 9: with `f` true when it is of place 6,
- the burst levels become 127, 127 and 51; AH's becomes 0 for `J`, and for `C` when `f`; when `f`, AF's becomes 70 for `C` and 0 for `J`, and AV's 0 for `J`;
- the voice onset becomes 127, and the burst frames 127 for `t` and 1 for the others.

### 4.4 Modes

After loading the targets, with the previous phoneme:
1. Track 17's weight becomes 0 and its mode 6.
2. After a phoneme not in class voiced: track 0's mode becomes 6, and track 17's earliest frame its next (section 2.1), so that its frames before stay.
3. After one in class silence: the weights of tracks 9 to 11 become 32766.
4. After one in class stop: track 1's earliest frame becomes its next.

## 5. Rules

### 5.1 Flags

Before the rules, the generator sets eleven flags; each is cleared first:

| Flag | Set when |
|---|---|
| 0 | the current phoneme is in class voiced |
| 1 | the previous category is 0 |
| 2 | the previous category is 3 |
| 3 | the previous category is 1, or it is 3 and the previous phoneme is nasal, or it is 0 |
| 4 | the previous category is 2 |
| 5 | the category isn't 0, the current phoneme is in class stop, and either the next one is in class closure or the pause, or the current node has flag bit 0x40 |
| 6, 7, 8, 9 | the category isn't 0 and the next phoneme is of place 0, 1, 3, 2 |
| 10 | the next node has flag bit 0x20 |

### 5.2 Rule sets and conditions

The data pack has 54 **rule sets** and a matrix that gives the set for each current phoneme and previous phoneme (`ruleMatrix[current][previous]`). Each set is a list of rules; the generator applies the first rule whose conditions all hold, and no other. A rule's conditions, in order, each with the result it wants (`is`):
- a flag's value;
- whether a phoneme is in a class (`plane` and `bits`);
- whether a phoneme is one of a list of symbols (`in`).

The phoneme is the current one, the next, the one after the next or the one before the previous (section 3.4); no rule in the data pack tests the one before the previous. A set may have no rule that applies; then nothing is done.

### 5.3 Assignments

A rule first runs its **assignments** in order: each sets a field to a number, adds a number to it, subtracts one from it, or copies another field into it. The fields:

| Field | Value |
|---|---|
| `target`, `mode`, `forward`, `back`, `onset`, `backTarget`, `frames` | a track's segment (section 2.3) |
| `next`, `earliest`, `last` | a track's next and earliest frames and last value (section 2.1) |
| `nextTarget` | a track's next target (section 4.1) |
| `weight` | a track's weight (section 4.1) |
| `burstLevel` | the burst level of track 0, 1 or 2 |
| `previousCategory`, `category`, `previousFrontness`, `frontness` | section 4.1 |
| `aspirationShift`, `voiceOnset`, `burstFrames`, `closureFrames`, `fricativeAspiration`, `aspiration`, `vowelOffset`, `vowelTransition` | values the actions use |

### 5.4 Actions

A rule then runs its **actions**, numbers 1 to 16, in order. Each is a step of its own, which the following parts of this spec describe:

| Action | Step |
|---|---|
| 1 | shift the aspiration boundary |
| 2 | shapes from the previous phoneme |
| 3 | vowel onset |
| 4 | vowel coarticulation |
| 5 | sonorant transitions |
| 6 | obstruent voicing |
| 7 | fricative amplitudes |
| 8 | consonant shapes |
| 9 | stop closure and burst |
| 10 | finish the phoneme |
| 11 | locus weights |
| 12 | special loci |
| 13 | consonant targets and F0 |
| 14 | source boundaries |
| 15 | pull the aspiration back |
| 16 | the per-phoneme rules |

## 6. Rendering a phoneme

To render the current phoneme, the generator:
1. loads its targets and sets its modes (section 4);
2. sets the flags (section 5.1) and applies the rules (sections 5.2 to 5.4);
3. **Voicing.** For a phoneme not in class voiced: the unvoiced pitch becomes track 17's target, which becomes 0, and the 2 is taken out of track 17's mode (6 becomes 4). For a node whose second value is 0: track 2's target becomes track 0's plus 5; track 0's target, onset and back target become 0; the 2 is taken out of track 17's mode. When the previous phoneme isn't in class voiced or its node's second value is 0, the 1, the pull back, is taken out of track 17's mode;
4. **Attenuation.** With the cursor's attenuation `a` in dB: the targets, onsets and back targets of tracks 0, 1 and 2 go down by `a`, to 0 at the lowest;
5. **Voice.** With a voice other than 0: track 12's target goes down to the voice's `fourthFormantLimit` at the most. Track 21's target gets `voice << 4` and track 20's `a << 4`, both by a bitwise or;
6. if the flag to keep the earlier frames is set (the stage's driver sets it), every track's earliest frame becomes its next, and the flag is cleared;
7. **Visual event.** If the visual events are on ([pipeline.md](pipeline.md) §8.1) and the phoneme isn't `p`, and the pipeline speaks for an engine, the generator queues a visual event with the phoneme's symbol, sign-extended, for the destination. A pause is queued only if the last visual event wasn't one;
8. renders the 22 segments, tracks 0 to 21 (section 2.4).

## 7. Resets

**The stage's reset**, with the pipeline's ([pipeline.md](pipeline.md) §7):
1. The stage's own state: its pause and end states 0 and 1, the end and keep-earlier flags off, the generated-segment flag on, two bytes the actions use 0, the unvoiced pitch 0, the transition limit 4. Its cursor's types are 4 and 5, and the track levels the escape parser's (the pipeline does both).
2. The ring's lookahead becomes 2 and the margin 3.
3. The context is reset.

**The context's reset**, also when a generated segment starts:
1. The frontness and the previous frontness become −13120; both categories, the aspiration shift, the voicing shift, the burst frames, the vowel offset and transition, the closure frames, the voice onset, the burst levels, the aspiration and the fricative aspiration 0.
2. Every track's next frame becomes the ring's `written`, its earliest frame `written − lookahead`, and its last value the initial frame's value for the track ([pipeline.md](pipeline.md) §7), in the track's units.
3. The phoneme before the previous one is the pause, and `furthest` becomes `written`.

## 8. What the recorded runs don't reach

These follow the original's code, but no recorded run gets there:
- Segments of modes 2, 9, 17, 21 or above 21. Mode 3 came up 4 times.
- A negative `frames`, `back target`, `onset` or `target` in a segment.
- A Q15 product where dividing by 32767 instead of 32768 changes the result, as for `151 · 217 · 128`.
- `AddBack` taking a frame other than the first below 0, or a first frame plus `change` of exactly −1.
- A ring without room: the generator was at most 115 frames ahead of the synthesizer, margin included, because the synthesizer renders after every step of the generator.
- 193 of the 468 rules: 275 applied. No rule of sets 43 (`z` after a closure), 47 and 48 (`h`, which no recorded run produced) applied; set 53 is empty and the matrix never gives it.
- A stop burst's exceptions: `t`, `J` or `C` before a pause (the stages before split `J` and `C` into `J z` and `C s`), `K` before `t` or `Q` (section 4.2).
- An F4 target above the voice's limit (section 6): no target in the data pack is.

In these the original reads outside its tables, and the port does something else instead. No recorded run gets there either:
- A previous, current or next node whose symbol isn't one of the phonemes: the port takes the pause.
- A symbol of 128 or more in a node a rule tests: the port gives it no classes.
- A voice other than 0 to 9: the port takes voice 0's tracks and F4 limit.
