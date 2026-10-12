# TruVoice: parameter generator

How L&H TruVoice (`tv_enua.dll`) turns timed phonemes into frames for its synthesizer. For each phoneme or pause the parameter generator works out what each of the 22 tracks of a frame should do ([synthesizer.md](synthesizer.md) §4.1), and writes the phoneme's frames into the synthesizer's ring ([synthesizer.md](synthesizer.md) §5.1), rewriting the end of the previous phoneme's where the two blend.

This spec covers the tracks and how the generator writes them (section 2), the phonemes (section 3), the targets a phoneme starts from (section 4), the rules that change them (section 5), how a phoneme is rendered (section 6), the stage's resets (section 7), the per-phoneme rules, action 16 (section 8), and the actions that set the onsets and shapes, actions 1, 2, 3, 5, 6, 8 and 15 (section 9). The other actions, the vowel database and the stage's driver are still to come.

**In short.**
- The generator works track by track. For each phoneme it fills in a **segment** for each track: a mode, how many frames, an onset and a target, two shapes, and a value to pull the previous phoneme's last frames toward. Rendering the segment writes the phoneme's frames of that track.
- A segment's values are in the track's units (Hz for the formants and bandwidths, the frame byte for the amplitudes) and become frame bytes by a shift. Frame bytes wrap around; nothing is clamped.
- Transitions follow 31 fixed shapes: decaying weights from 255 down, one per frame.
- After each phoneme every track moves on by its segment's frames. The ring's written frames end where the slowest track is, and the frame before it is marked for the output stage.
- A phoneme's segments start from its targets in the data pack. Then the first rule that applies, of the rule set for the phoneme and the one before it, changes them and runs its actions. Voicing, the attenuation and the voice adjust them last.

**Exactness.** Ported from the original's code, and checked against it by replaying every call of its track functions recorded in 164 runs (the ten voices, both rates, every speed range, pitch and volume tags, whisper, monotone, emphasis, tones and generated segments, texts spoken one after another in one session, long texts with a long pause, 44 sentences chosen for their sounds, 11 for their clusters of consonants and 13 for the contexts of the per-phoneme rules and the actions): each of the 248,330 segments rendered (section 2.4), the stage's 136,539 direct writes into the ring (section 2.5), 11,275 advances (section 2.6) and 28,769 room checks and rebases (section 2.7) left the speaking thread's state byte-identical to the original's. So did the 11,275 phonemes rendered (section 6), with their target loading, their rules, the per-phoneme rules (section 8; 5,919 runs of action 16, 467 groupings of the previous phoneme) and the actions of section 9 (1,990 runs of action 1, 2,432 of action 2, 5,956 of action 3, 2,175 of action 5, 3,636 of action 6, 2,833 of action 8 and 1,375 of action 15), the other actions taken from the recording after checking that each started from the original's state, and the stage's 164 resets and 207 context resets (section 7).

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
| 1 | shift the aspiration's start (section 9.3) |
| 2 | shapes from the previous phoneme (section 9.4) |
| 3 | the voice onset (section 9.5) |
| 4 | vowel coarticulation |
| 5 | sonorant transitions (section 9.6) |
| 6 | obstruent voicing (section 9.7) |
| 7 | fricative amplitudes |
| 8 | consonant shapes (section 9.8) |
| 9 | stop closure and burst |
| 10 | finish the phoneme |
| 11 | locus weights |
| 12 | special loci |
| 13 | consonant targets and F0 |
| 14 | source boundaries |
| 15 | pull the aspiration back (section 9.3) |
| 16 | the per-phoneme rules (section 8) |

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

## 8. The per-phoneme rules

Action 16 runs the rules of the current phoneme, if it is one of `B D G J M m R W X Y Z j l s x z`; for the others it does nothing. In sounds, as the engine's visual events give them: `B` b, `D` d, `G` g, `J` the d of the affricate `J z` (dʒ), `M` m, `m` a syllabic m, `R` r, `W` w, `X` θ, `Y` j, `Z` z, `j` an l after a vowel, `l` a syllabic l, `s` ʃ, `x` ð, `z` ʒ.

**Notation.** `T9 = 250` sets track 9's segment target to 250 (section 2.3), in the track's units (section 2.2); `T9..12 = a, b, c, d` sets tracks 9 to 12 in order. `W9` is track 9's weight (section 4.1), and `b0`, `b1`, `b2` are the burst levels of tracks 0, 1 and 2 (section 4.2). Previous, current and next are the phonemes of section 3.4; the node before the previous one may be any node. A **vowel** is one of the 24 phonemes 0 to 23 (section 3.1); class vowel also has `p`. A node is **stressed** if it has flag bit 0x20. A node's **frames** are its value, as in section 4.1, step 9. Divisions are of unsigned numbers and truncate.

### 8.1 Classes

The rules test two more classes:

| Class | Name here | Phonemes |
|---|---|---|
| (0, 0x01) | syllabic | the vowels and `l m n` |
| (1, 0x20) | consonant | the consonants but `l m n Y`: `B C D F G H J K L M N P Q R S T V W X Z d h j q s t x z ~` |

### 8.2 The previous phoneme's group

`G`, `M`, `m` and `s` first sort the previous phoneme into a **group**:

| Group | Previous phoneme |
|---|---|
| 1 | `E A` |
| 2 | `y I \| i e a` |
| 3 | `U b f O` |
| 4 | `u v w o @` |
| 5 | `4 k 3 r g c 5` |
| 6 | `M B V P F W L j R H` and the pause |
| 7 | any other in class consonant but `d` |

Any other previous phoneme (`p l Y m n d`) leaves the group as it was. The group is 0 when the speaking thread starts, and no reset changes it.

### 8.3 The rules

**B.** After `S`, before `L`: `T3 = 70`.

**D.** It sets the burst levels and track 8:
- Before a phoneme in class consonant:
  - after `j`: `b1 = 60` if the next phoneme is in class closure but not nasal, is in class fricative but isn't `Z`, or is the pause; otherwise `b0 = 57`, unless it is `Z`;
  - else after the pause, before a nasal: `b1 = 55`;
  - else before `n`: `b0 = 50`; before `Z`: `b1 = 57`;
  - else `b0 = 127`; before a fricative `b1 = 54`, or 51 after a nasal; before others `b1 = 57`, or 55 after the pause.
- Before `p`: after `N`, `b1 = 55` and `T8..15 = 0, 250, 1834, 2622, 3469, 143, 366, 166`.
- Before another phoneme in class vowel, of place `P`:
  1. `P` = 0: `T8 = 62`. `P` = 1, after `S`: `T8 = 55`. `P` = 3: `T8 = 65`, or 50 if the next phoneme is `3` and either its node is stressed or it doesn't follow `j`.
  2. If the next node isn't stressed: after `j`, `b0, b1 = 53, 52`; else for `P` = 1, `b0, b1 = 45, 55`; for `P` = 0, `b0 = 127`, and `b1 = 55` after a phoneme in class closure but not nasal or in class fricative, 52 after others.
  3. If it is: after `S`, `b0..2 = 127, 58, 48`; else for `P` = 0, `b0, b1 = 0, 55`; for `P` = 1, `b0, b1 = 45, 55`.
- In every case then, before `R`, after `S`, with the current node stressed: `b1 = 60`.

**G.** It groups the previous phoneme. If that isn't the pause, `W9 = 32472` and track 9's `back` becomes 3. Then:
- before `o`: `T18 = 8`; before another phoneme in class vowel: `T18 = 0`;
- before a phoneme in class syllabic but not in class closure (that is `l`), by group: 1, `T3..7 = 62, 62, 80, 80, 80`; 2, `T3..7 = 62, 62, 80, 65, 60` and `T10, T11 = 2200, 2400`; 3, `T3..7 = 80, 75, 40, 72, 50` and `T10, T11 = 2050, 2250`; 4, `T10, T11 = 1964, 2164`; 5, `T3..7 = 65, 75, 40, 72, 50`;
- before others, by group: 3, `T10, T11 = 1600, 1800`; 4 and 5, `T10, T11 = 1684, 1884`.

**J.** `T0 = 0` if the previous phoneme isn't in class voiced, or if it is the pause and the next is `z`. After a phoneme in class voiced or the pause: `b0, b1 = 48, 64`.

**M, m.** They group the previous phoneme. Before a vowel nothing else changes. In group 1 or 2, or after `Y`: `W10, W11 = 16400, 26240`. Otherwise `W10, W11 = 26240, 9840`, and in group 5 or after `R`, `T11 = 1750`.

**R.** By the previous phoneme:
- `G`, before a phoneme that isn't a vowel: `T0 = 55`, `T10, T11 = 1140, 1400`;
- `K`, before a phoneme that isn't a vowel: `T0..7 = 0, 56, 51, 65, 0, 55, 60, 60`, `T10 = 1404`, `T12 = 2544`;
- `P`: before `e`, `T3 = 50` and `T9..12 = 458, 1300, 1816, 2968`; then before a phoneme in class vowel, `T10, T11 = 996, 1472` if its node isn't stressed, `T8 = 54` if it is;
- `X`: before `E`, `T10..15 = 1157, 1490, 3010, 54, 247, 230`;
- `s`: if the node before the previous one is `C`, `T10, T11 = 1292, 1760`;
- any other, before a phoneme that isn't a vowel, by that phoneme's place: 0, `T0 = 60`; 2, `T0 = 60` and `T10, T11 = 1140, 1440` (only vowels have place 0 or 2); any other place, `T10` becomes `T10 − Q15(T10, 8200)`, then that plus `Q15(F2, 8200)`, where `F2` is the next phoneme's F2 target in the data pack, and `T11 = T10 + 400`.

Then in every case, before `l`: `T9..12 = 376, 1032, 1671, 2757`.

**W.** After `K`: `T10 = 800`. Else after the pause, before `E`: `T15 = 250`.

**X.** After a phoneme in class vowel: `W10, W11 = 26240, 26240`. Before `R`: track 1's `back` becomes 0, its `back target` and `onset` 25, and its `forward` its `frames`.

**Y.** After `G`: `T10, T11 = 2036, 2592`.

**Z.**
1. `T1 = 64` before `I` or `a`; otherwise 69 after a phoneme not in class closure, 60 after one. Tracks 18 and 19 get `forward` 3.
2. Before a phoneme that isn't a vowel: after one not in class vowel, `T0 = 0`, `T4..7 = 50, 73, 70, 70`, `T18, T19 = 15, 15` and track 1's `forward` 3, and if the next phoneme is neither in class vowel nor the pause, track 17's mode becomes 7, `W17 = 16400` and `T17 = 80`; after one in class vowel, `T0 = 56`, `T4 = 60`, `T6, T7 = 67, 63`, `T9 = 296`, `T18, T19 = 8, 14`.
3. Before a phoneme in class closure and class voiced: `T18, T19 = 8, 15`.

**j.** By the previous phoneme:
- `A`: `T9..12 = 459, 810, 2682, 3621`; `E`: `378, 869, 2724, 3715`; `a`: `400, 873, 2602, 3588`; `b`: `391, 810, 2967, 3880`;
- `e`: `T9..15 = 422, 843, 2679, 3750, 78, 79, 107`;
- `y`: before a phoneme in class vowel, `T9..12 = 410, 1174, 2412, 3596`, and tracks 9 to 12 get weight 3280, `forward` 4 and `back` 2; before others, `T9..12 = 548, 900, 2768, 3344`, `W9..11 = 3280`, and track 11's `forward` becomes the node's frames.

**l.**
1. Before the pause: `T9..12 = 532, 790, 2646, 3377`, and tracks 9 to 12 get the node's frames as `forward`. Before others: `T9..15 = 340, 814, 2656, 3473, 40, 40, 120`, tracks 9 to 15 get `back` 3 and `forward` `frames · 55 / 100`, and `W9..12 = 3280`.
2. After `R`: tracks 9 to 12 get mode 6.

**s.**
1. It groups the previous phoneme. Before a phoneme that isn't a vowel, by group: 1, `T6, T7 = 64, 55` and `T15 = 184`; 2, `T4, T5 = 65, 65`, `T7 = 53` and `T11, T12 = 2304, 3024`; 3, `T3..5 = 60, 60, 60`, `T7 = 54` and `T10 = 1700`; 4, `T5..7 = 58, 48, 48` and `T12 = 3504`; 5, `T4 = 65`, `T7 = 54` and `T11, T12 = 2256, 3168`.
2. Before `@`: track 1's `forward` goes up by 3, its target down by 3, and its mode becomes 6; tracks 4 to 7 get mode 6 and their `forward` goes up by 3; then after `P`, tracks 4 and 5 get mode 4. Before `b`: track 1's `forward` becomes `frames / 2`.
3. After `C` (the affricate `C s`, tʃ): before the pause, `T1 = 60`, and if the node before the previous one isn't a vowel, `T10..12 = 2540, 2928, 3744`; before `R`, `T2 = 50`, `T7 = 60`, `T10, T11 = 1396, 1936`, and `T1 = 58` if the current node isn't stressed, else `T1 = 60` and `T4 = 66`.

**x.** After the pause: `T10 = 1550` and `T7 = 50`. Otherwise after a phoneme in class vowel: `W10, W11 = 26240, 26240`.

**z.** After a phoneme in class vowel: `W10, W11 = 26240, 26240`. Then after `J`, by the next phoneme:
- the pause: track 0's `onset` becomes 47 and its `forward` `frames / 4`;
- `k`: `T9..12` become tracks 9 to 12's `last` values (section 2.1), then as for `A`;
- `A a e i |`: `T1, T2` become tracks 1 and 2's `last` values;
- `y`: if the node before the previous one is the pause, `T9..12 = 254, 1613, 2088, 2735`.

## 9. Onsets and shapes: actions 1, 2, 3, 5, 6, 8 and 15

These actions set the consonants' and vowels' transitions: where the aspiration and the voicing start, the shapes and modes of the segments, and some targets. The notation is section 8's: a phoneme named alone is the current one, "after" names the previous phoneme and "before" the next. A track **moves on** by `n` frames: its next frame goes up by `n` and its segment's frames go down by `n`. `L0`, `L1`, … are the tracks' last values (section 2.1). The writers are section 2.5's, on the track named.

### 9.1 Classes

The actions test the syllabic class (section 8.1) and these:

| Class | Name here | Phonemes |
|---|---|---|
| (1, 0x08) | affricate | `J C` |
| (2, 0x04) | alveolar | `D T t q Z S N n` |
| (2, 0x08) | rhotic | `3 R` |
| (2, 0x20) | front | `4 A E U a e i k \|` |
| (2, 0x40) | lateral | `j l L` |
| (3, 0x01) | approximant | `j l L W Y R h` |
| (3, 0x04) | palatal | `Y J C z s` |
| (3, 0x20) | rounded | `O U b c g u w y W h` |
| (3, 0x40) | front-ending | `5 A E I y` |
| (4, 0x01) | high | `4 E U b c i u \| W Y h` |
| (4, 0x02) | r-coloured | `4 5 c g k r` |
| (4, 0x04) | round-ending | `O U b f` |

A **word mark** is a node of symbol `&` or `%`, which start a word.

### 9.2 Articulators

Each phoneme has an **articulator** in the data pack (`articulator`), and each articulator a forward shape (`articulatorShapes`):

| Articulator | Phonemes | Shape |
|---|---|---|
| 0 | the vowels, `j L W Y R p h` | 0 |
| 1 (lips) | `B P V F M m` | 6 |
| 2 (teeth) | `x X` | 8 |
| 3 (gums) | `l D T t q Z S N n` | 8 |
| 4 (palate) | `J C z s` | 10 |
| 5 | none | 10 |
| 6 (velum) | `G K ~` | 10 |
| 7 (glottis) | `d H Q` and the pause | 8 |

To give the **formant shapes** `s`: tracks 9 to 16 get `forward` `s`, then track 9's becomes `s / 2 + 1`.

### 9.3 The aspiration's start (actions 1 and 15)

**Action 1.**
1. After a phoneme in class sonorant, if the current one isn't in class closure: the aspiration shift becomes 1. If the current phoneme is in class fricative and track 1's next frame is 2 or more, that frame moves back 2 and track 1's segment gets 2 frames more. Then action 15.
2. Track 17's mode becomes 4, unless the current phoneme is `S` before the pause after a phoneme in class vowel.
3. Unless the current phoneme is `S`, track 0's mode becomes 5.

**Action 15** moves the start of the aspiration back by the aspiration shift `h`. With `n` track 2's next frame and `a` the aspiration, if `h ≤ n` and the voice isn't 8 (Wanda) at 8000 Hz:
1. `n` goes down by `h`. On track 2, `AddBack(n, 3, n − earliest, a − f)`, where `f` is frame `n`.
2. Track 2's segment gets `h` frames more and `forward` 3, and `L2 = a`.
3. If the current phoneme is the pause: on track 0, `AddBack(next, 10 − h, next − earliest, −4)`.
4. If the current phoneme is in class closure, track 2's mode becomes 5.

### 9.4 Shapes from the previous phoneme (action 2)

1. The formant shapes of the previous phoneme's articulator.
2. After an affricate, tracks 0 to 2 get mode 6 and tracks 3 to 8 mode 4. Else after a stop, tracks 1 to 8 get mode 4, and a current `p` after `T` makes `W1, W2 = 0, 0` and tracks 1 and 2's mode 6. After the others, tracks 0 to 8 get mode 6.
3. After a nasal, a current phoneme in class voiced: track 0's mode becomes 4.
4. After `J`, `B`, `D` or `G` whose node before is in class voiced: track 0's `onset` and `back target` become 52.

### 9.5 The voice onset (action 3)

1. The voice onset is written (below) when the current phoneme is `R`, `L` or `W` after a fricative in class voiced, or when the previous phoneme isn't in class voiced, the pause or `H`, and its node hasn't flag bit 0x40. (The original's code also leaves out a vowel with a stressed node after `D`, which is in class voiced anyway.)
2. If the current phoneme isn't in class vowel, the voice onset becomes 127.
3. After a phoneme in class stop or fricative other than `s`: track 1's segment loses the voicing shift's frames.
4. If the current phoneme isn't a vowel:
   - after an approximant, tracks 9 to 16 get `forward` 7, or 11 for a rounded phoneme after `Y`; then after a rhotic, track 11's `forward` becomes 9;
   - if it isn't labial and the previous phoneme is round-ending or front-ending: `W9..11 = 11480`;
   - if it is rounded and the previous phoneme palatal: track 10's `forward` goes up by 5.
5. `T3..8` become `L3..8` for a phoneme in class vowel or `n` after `T`; a phoneme of place 2 or 3 or one of `a l W L Y p` after `K`; `i` after `P`, and `R` after `P` before a phoneme in class vowel whose node is stressed; `R` or a phoneme in class vowel after `X`.
6. A current `p` after `P`, `T` or `K`: `T0 = 0`.
7. A current phoneme in class vowel: if its node isn't stressed, the node after it isn't a word mark and the next phoneme is in class vowel with a stressed node, `T0` goes down by 2. If its node is stressed, the node before it isn't a word mark and the previous phoneme is in class vowel with a node that isn't stressed, `T0` goes up by 2.
8. A current `R` or `L` whose node before is `P`: `T8 = 0`.

**Writing the voice onset.** With `v` the voice onset and `a` the aspiration:
1. If the previous node isn't stressed, `a` goes down by 3, to 0 at the lowest.
2. After a phoneme that isn't a stop:
   - `v = 1` if the current phoneme is syllabic, else track 2's frames / 2;
   - unless the previous phoneme is `X` and the current one `R` or in class vowel: on track 2, `AddBack(next, 3, next − earliest, a − L2)`.
3. After a stop:
   1. `v = 2` if the cursor's input flags (the `A` command, [pipeline.md](pipeline.md) §4) have bit 0x10.
   2. A rhotic after an alveolar: `a` goes up by 9.
   3. After `P`, `T` or `K`: `a = L2` for `y` after `P`, `L2 − 6` for `p`, `L2 − 2` for the others; 0 at the lowest.
   4. A `p` after a stop other than `P`, `T` and `K` (only `C` can come here): track 1 gets `v` frames of an undefined value (section 10) and moves on by `v`.
   5. With `m` the current node's frames minus 3: if `v > m`, comparing both as unsigned 32-bit numbers, `v` becomes `m`, or 1 if `m` is 0.
   6. **Burst.** If the previous node is stressed and the current phoneme is syllabic or of place 4, or for `r`, `g` or `c` after `K`: track 1 gets `Repeat(next, v, b)` and moves on by `v`. `b` is 60 after a stop other than `P`, `T` and `K`, and else `L1 − 2`, 0 at the lowest, except 0 for a vowel whose node is stressed after `P` or `T`, and `L1` for `i b u U L` and 55 for `O` after `K`. Then the pitch's silence `s` is `v` after `K`, `v − 3` for `L` after `P`, `v − 1` otherwise; if that is below 0, `v − 1`, 0 at the lowest; and at most track 17's frames − 1. Track 17 gets `Repeat(next, s, 0)` and moves on by `s`, and `L17` becomes the unvoiced pitch (section 6).
   7. Track 2's earliest frame becomes its next.
4. `v` goes down to track 2's frames and to track 0's frames at the most. If it is below 1, that's all.
5. Track 0: for `R` after `z` or `V`, `Repeat(next, v, L0)`. For `R` after `X`: if `v > 1`, `Repeat(next, v − 1, 0)` and frame `next + v − 1` becomes 57; if `v` is 1, frame `next + 1` becomes 57 and frame `next` stays; `L0 = 57`. For a vowel with `v = 1` after a phoneme that isn't a stop: frame `next` becomes 48, `L0 = 48`. Otherwise `Repeat(next, v, 0)`, `L0 = 0`.
6. For `R` or a phoneme in class vowel after `X`: `Repeat(next, v, L2)` on track 2, `Repeat(next, v, L1)` on track 1, and track 1 moves on by `v`. Otherwise `Repeat(next, v, a)` on track 2.
7. `L2 = a`, track 0's mode becomes 6, and tracks 2 and 0 move on by `v`.

### 9.6 Sonorant transitions (action 5)

1. A lateral after a phoneme in class sonorant that isn't nasal: `W9..11 = 29520`.
2. Unless the current phoneme is `l` or the next is the pause: tracks 9 to 16 get `forward` 9, or for an approximant 7, or 5 if the previous phoneme is one too.
3. A rhotic: tracks 10 to 12 get `forward` 6.
4. `n`: before the pause whose node after is the pause too, tracks 10 to 12 get `forward` 4; after `D`, track 10's mode becomes 6.
5. `H`: before an r-coloured phoneme, `T2 = 55`; before `g`, after `s` whose node before is `C`, `T2 = 67`.

### 9.7 Obstruent voicing (action 6)

1. A palatal: track 1's `forward` becomes 6, except for `s`: it goes down by 2 if the node after the current one is `|` or `@` and the node after that `N`; otherwise it becomes 6 before a front or rounded phoneme and goes down by 2 before the others. An affricate after a phoneme not in class voiced: `T0 = 0`.
2. Before a phoneme in class sonorant: the voicing shift becomes 2, or the next node's frames minus 1 if they are 2 or fewer (unsigned); then 0 for a stop, for `X` before `R` and for `S`. Unless the current phoneme is `s`, track 1's segment gets the voicing shift's frames more.
3. A stop in class voiced (`B D J G q`):
   1. `T4, T5 = 0, 0`, unless it is `D` before a phoneme in class vowel or `n`, or `J` before `z`, or the next node is stressed.
   2. `T0` goes down by 20, unless the next phoneme is in class vowel or is `Z`, or it is `G` before `Y` or `D` before `n`.
   3. `T0 = 0` after a phoneme not in class voiced, except for `D` before `|`, and for `q`. Otherwise, after a phoneme not in class sonorant, `T0` goes down by 30.
   4. The first that applies: after a phoneme in class voiced, other than `q`: `T0 = 50` and track 0's `forward` becomes the node's frames; `B` or `G` after `S`: `T0 = 0`; before a phoneme not in class vowel: nothing; `B`: `T0 = 50`; any but `D`: nothing; `D` before `|`: `T0 = 0`; `D` after the pause or `S`: `T4, T5 = 0, 0` if the previous phoneme is syllabic and the next isn't `p`, which never holds; `D` before a stressed node: `T0 = 43`.
4. Before `s` whose node after is `R`, with the current node stressed: `T4 = 60`, `b1 = 60`, `b2 = 0` and `T7 = 65`.
5. `F` after the pause: `T8 = 60`. `V`: `T8 = 56`.

### 9.8 Consonant shapes (action 8)

1. After a phoneme not in class closure, tracks 0 to 9 get mode 5; then for `T`, or `M` after `z`, track 9's mode becomes 7.
2. A phoneme in class voiced: track 0's mode becomes 6 for a stop, 4 for a nasal.
3. After a lateral: tracks 10 and 11 get mode 5.
4. The formant shapes: 5 after a high phoneme, else those of the current phoneme's articulator.

## 10. What the recorded runs don't reach

These follow the original's code, but no recorded run gets there:
- Segments of modes 2, 9, 17, 21 or above 21. Mode 3 came up 4 times.
- A negative `frames`, `back target`, `onset` or `target` in a segment.
- A Q15 product where dividing by 32767 instead of 32768 changes the result, as for `151 · 217 · 128`.
- `AddBack` taking a frame other than the first below 0, or a first frame plus `change` of exactly −1.
- A ring without room: the generator was at most 115 frames ahead of the synthesizer, margin included, because the synthesizer renders after every step of the generator.
- 169 of the 468 rules: 299 applied. No rule of sets 43 (`z` after a closure), 47 and 48 (`h`, which no recorded run produced) applied; set 53 is empty and the matrix never gives it.
- A stop burst's exceptions: `t`, `J` or `C` before a pause (the stages before split `J` and `C` into `J z` and `C s`), `K` before `t` or `Q` (section 4.2).
- An F4 target above the voice's limit (section 6): no target in the data pack is.
- Some of the per-phoneme rules (section 8.3), as the stages before don't produce their contexts from English text: `D` after the pause before a nasal, before `n`, or after `S` before `R` (the stages before turn `D R` into `J z R`); `G` before `l` (they write `j` after `G`); `R` after `G` or `K` before a phoneme that isn't a vowel, `R` before `l` and `l` after `R`; `z` after `J` before `y` with the pause before `J` (a word mark always comes between); `M` or `m` after `Y` or `R` before a phoneme that isn't a vowel. Nor does the group of `H` or `d`, which only come before vowels. Where a rule tests for a vowel and another for class vowel, they differ only for `p`, which comes only before a pause, after a stop or `~`: no recorded `p` came after or before `M m R Z j s x z`.
- Some contexts of the actions (section 9), for the same reason: a rounded phoneme after `Y` and a phoneme after `R`, neither a vowel, in the voice onset; `n` after `D` (the stages before write no `D n`), `n` after `T` and `l` after `K` (they write `q n` and `K L`), `H` before `g` (`H` only starts a phrase), `D` before `|` after a phoneme not in class voiced, and `s` before `|` (`|` only comes after `t` or `d` in a word); `R` after `P` before a stressed phoneme that isn't a vowel; a vowel with a stressed node right after another, no word mark between (the stages before put `W` or `Y` between most vowels); a syllabic `m`, `l` or `n` after a phoneme neither in class voiced nor a stop; `p` in step 4 of the voice onset, or after a vowel; `r`, `g` or `c` after a `K` whose node isn't stressed, and a vowel whose node isn't stressed after a `P` or `T` whose node is (the stages before mark a stressed syllable's consonant and vowel together); action 8 after `W`, `Y`, `R` or `h`, which only come before vowels. Action 3 never runs after `C`, as the stages before always write `s` after it and the rules don't run action 3 on `s`: so the burst's 60 isn't reached either. Nor does a previous node with flag bit 0x40 outside class voiced: the stages before set the bit only on vowels, approximants and voiced consonants. Nor does a stop after which the cursor's input flags have bit 0x10, which the escape parser's `A` command sets.
- Values that make a step change nothing. In the voice onset: the aspiration is always 51 when the voice onset is written, so taking 3 off never goes below 0; the only alveolar stop the writer comes to is `T`, whose aspiration step 3.3 replaces, so the 9 of step 3.2 never shows; track 1's last value is never below 2 when a burst follows `P`, `T` or `K`; `L17` already is the unvoiced pitch when the burst sets it; the voice onset is never above track 0's or track 2's frames in step 4; step 5's `v = 1` always holds for a vowel after a phoneme that isn't a stop, as every vowel is syllabic; and the track 8 target of `L` after `P` is already 0. In action 6: the test for `@` before `N`, as `s` goes down by 2 before `@` anyway; an affricate after a phoneme in class voiced, as `J` gets `T0 = 50` in step 3.4 anyway and `C`'s `T0` is already 0; `G` after `S` in step 3.4, as `G` keeps step 3.3's `T0 = 0`; and steps 3.2 and 3.3 but for `T0 = 0`, as step 3.3 or 3.4 then sets every voiced stop's `T0` to 0, 43 or 50, whatever the text.
- A node of fewer than 3 frames: the shortest phonemes were 3 frames long. So the voicing shift never came from the next node's frames, the voice onset's limit was never 0, and the pitch's silence never reached track 17's frames (section 9.5 and 9.7).
- Action 15 on a phoneme in class closure: the rules run it only on the pause, and action 1 only on a phoneme not in class closure. Its check that the aspiration shift is at most track 2's next frame, and action 1's that track 1's next frame is 2 or more: the next frames are always larger.

In these the original reads outside its tables, and the port does something else instead. No recorded run gets there either:
- A previous, current or next node whose symbol isn't one of the phonemes: the port takes the pause.
- A symbol of 128 or more in a node a rule tests: the port gives it no classes.
- A voice other than 0 to 9: the port takes voice 0's tracks and F4 limit.
- A `p` after `C` in the voice onset (section 9.5, step 3.4; the stages before write `C s`): the original writes into track 1 the low byte of its state's address, which differs from run to run. The port writes 0.
