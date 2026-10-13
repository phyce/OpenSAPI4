# TruVoice: parameter generator

How L&H TruVoice (`tv_enua.dll`) turns timed phonemes into frames for its synthesizer. For each phoneme or pause the parameter generator works out what each of the 22 tracks of a frame should do ([synthesizer.md](synthesizer.md) §4.1), and writes the phoneme's frames into the synthesizer's ring ([synthesizer.md](synthesizer.md) §5.1), rewriting the end of the previous phoneme's where the two blend.

This spec covers the tracks and how the generator writes them (section 2), the phonemes (section 3), the targets a phoneme starts from (section 4), the rules that change them (section 5), how a phoneme is rendered (section 6), the stage's resets (section 7), the per-phoneme rules, action 16 (section 8), the actions that set the onsets and shapes, actions 1, 2, 3, 5, 6, 8 and 15 (section 9), those that set the fricatives' amplitudes and the stops' closures, actions 7 and 9 (section 10), and action 4, which sets the vowels' formants: from the vowel database (section 11), and without a record by its own steps (section 12), action 10, which finishes every phoneme: its loci, special loci, final adjustments and limits (section 13), action 13's helpers: a consonant's loci from the vowel database, and the formants through the liquids (section 14), action 13 itself, which sets the consonants' targets and transitions and the pitch's small movements (section 15), action 14, which sets how the sources go from one phoneme into the next and into a pause (section 16), and the stage's step, which finds the next phoneme, puts in pauses and writes the generated segments (section 17).

**In short.**
- The generator works track by track. For each phoneme it fills in a **segment** for each track: a mode, how many frames, an onset and a target, two shapes, and a value to pull the previous phoneme's last frames toward. Rendering the segment writes the phoneme's frames of that track.
- A segment's values are in the track's units (Hz for the formants and bandwidths, the frame byte for the amplitudes) and become frame bytes by a shift. Frame bytes wrap around; nothing is clamped.
- Transitions follow 31 fixed shapes: decaying weights from 255 down, one per frame.
- After each phoneme every track moves on by its segment's frames. The ring's written frames end where the slowest track is, and the frame before it is marked for the output stage.
- A phoneme's segments start from its targets in the data pack. Then the first rule that applies, of the rule set for the phoneme and the one before it, changes them and runs its actions. Voicing, the attenuation and the voice adjust them last.
- A vowel's formants follow a database of trajectories, chosen by the vowel and the phonemes around it. For the few contexts it lacks, they go from the vowel's targets toward its offglide.
- Every phoneme ends with action 10: it sets the formants' weights and onsets by the phonemes on either side and the bandwidths' transitions, adjusts a few contexts, and holds the segments to their limits.
- A consonant before a vowel takes its formant targets from the start of the vowel database's record for that vowel. `R`, `L` and `Y` follow trajectories from tables, chosen by the phonemes around them.
- Action 13 also holds the transitions to their limits, starts the formants of most consonants from where the previous phoneme left them, and writes the pitch's small movements: a dip at a glottal stop, and a rise and fall on a stressed vowel before a pause.
- Action 14 starts the sources' amplitudes between their last values and their targets, shapes the voicing at the boundary, ends the voicing of a voiced fricative before a pause, and lets the voicing and the pitch fall into a pause.
- The stage's step renders one phoneme at a time. It looks past the next phoneme for a text's end, a reset or a generated segment and puts in pauses before them, and holds the synthesizer at the end of a text until more comes. A generated segment, a tone or a fixed frame, it writes as the ring has room.

**Exactness.** Ported from the original's code, and checked against it by replaying every call of its functions recorded in 204 runs (the ten voices, both rates, every speed range, pitch and volume tags, whisper, monotone, emphasis, tones and generated segments, texts spoken one after another in one session, long texts with a long pause, 44 sentences chosen for their sounds, 11 for their clusters of consonants and 53 for the contexts of the per-phoneme rules, the actions and the stage's step, two of these with the input flags changed and seven spoken as phonemes (section 12)): each of the 351,468 segments rendered (section 2.4), the stage's 175,467 direct writes into the ring (section 2.5), 15,828 advances (section 2.6) and 41,201 room checks and rebases (section 2.7) left the speaking thread's state byte-identical to the original's. So did the 15,828 phonemes rendered (section 6), with their target loading, their rules and all their actions: the per-phoneme rules (section 8; 7,888 runs of action 16, 777 groupings of the previous phoneme), the actions of section 9 (2,604 runs of action 1, 3,393 of action 2, 8,256 of action 3, 3,054 of action 5, 4,684 of action 6, 3,913 of action 8 and 2,045 of action 15), those of section 10 (2,297 runs of action 7 and 2,355 of action 9), action 4 (sections 11 and 12: 5,202 runs, 4,693 with a record of the vowel database, 219 of a vowel with an offglide without one, 22 of a vowel without an offglide, 15 of `@` and `|` and 253 of `p`), action 10 (section 13: 15,828 runs, its loci, special loci and final adjustments also each on their own), action 13 (sections 14 and 15: 15,828 runs, with 3,089 lookups of a consonant's loci in the vowel database and 743 runs of the liquids also on their own) and action 14 (section 16: 15,828 runs, also on its own). So did the stage's 204 resets and 255 context resets (section 7), and its 28,659 steps (section 17): their 25,351 lookaheads, with 237 lead-in pauses and 222 pairs of pauses (220 before an end command, two after ten nodes without a phoneme), and 69 runs of writing a generated segment, with 63 fills, for 27 tones, the nine other kinds of `t` that the escape sequences give, seven silences and one segment of the track levels; each also on its own.

The stage was also checked as a whole, in the pipeline, in 228 recorded runs: these, and the pipeline's runs with commands, volumes, voices and attributes changed while speaking ([pipeline.md](pipeline.md)). The Go speaking thread started each run as the original's, and the Go scheduler, parameter generator, output stage and synthesizer ran all 110,544 of the original's scheduler steps, with what the stages before the generator wrote taken from the recordings. Each step ran the same stages, from the same state (9,025 input steps, 19,114 normalizer, 21,480 lexical and 25,973 prosody steps), reported the same, and left the speaking thread's whole state the same: the frames, the nodes, the cursors and the stage's own. The stage ran 34,634 steps, 39 of them waiting for room. The audio, 19,136,740 samples in 16,930 hand-overs, was the original's PCM in every run, and each of the 4,053 words and 31 bookmarks came at the sample where the original reported it.

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
| 4 | vowel coarticulation: the vowel database (section 11), and without a record its own steps (section 12) |
| 5 | sonorant transitions (section 9.6) |
| 6 | obstruent voicing (section 9.7) |
| 7 | fricative amplitudes (section 10.2) |
| 8 | consonant shapes (section 9.8) |
| 9 | stop closure and burst (section 10.3) |
| 10 | finish the phoneme (section 13) |
| 11 | the loci (section 13.2) |
| 12 | the special loci (section 13.3) |
| 13 | the consonants' targets and the pitch (section 15) |
| 14 | the sources (section 16) |
| 15 | pull the aspiration back (section 9.3) |
| 16 | the per-phoneme rules (section 8) |

## 6. Rendering a phoneme

To render the current phoneme, the generator:
1. loads its targets and sets its modes (section 4);
2. sets the flags (section 5.1) and applies the rules (sections 5.2 to 5.4);
3. **Voicing.** For a phoneme not in class voiced: the unvoiced pitch becomes track 17's target, which becomes 0, and the 2 is taken out of track 17's mode (6 becomes 4). For a node whose second value is 0: track 2's target becomes track 0's plus 5; track 0's target, onset and back target become 0; the 2 is taken out of track 17's mode. When the previous phoneme isn't in class voiced or its node's second value is 0, the 1, the pull back, is taken out of track 17's mode;
4. **Attenuation.** With the cursor's attenuation `a` in dB: the targets, onsets and back targets of tracks 0, 1 and 2 go down by `a`, to 0 at the lowest;
5. **Voice.** With a voice other than 0: track 12's target goes down to the voice's `fourthFormantLimit` at the most. Track 21's target gets `voice << 4` and track 20's `a << 4`, both by a bitwise or;
6. if the flag to keep the earlier frames is set (the stage's step sets it after a generated segment, section 17.5), every track's earliest frame becomes its next, and the flag is cleared;
7. **Visual event.** If the visual events are on ([pipeline.md](pipeline.md) §8.1) and the phoneme isn't `p`, and the pipeline speaks for an engine, the generator queues a visual event with the phoneme's symbol, sign-extended, for the destination. A pause is queued only if the last visual event wasn't one;
8. renders the 22 segments, tracks 0 to 21 (section 2.4).

## 7. Resets

**The stage's reset**, with the pipeline's ([pipeline.md](pipeline.md) §7):
1. The stage's own state: ahead 0 and start 1 (section 17.1), at reset and the keep-earlier flag off, the last generated segment written, two bytes the actions use 0, the unvoiced pitch 0, the transition limit 4, and wanted 0 (in the original the ring's reset clears it). Its cursor's types are 4 and 5, and the track levels the escape parser's (the pipeline does both).
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
   4. A `p` after a stop other than `P`, `T` and `K` (only `C` can come here): track 1 gets `v` frames of an undefined value (section 18) and moves on by `v`.
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

## 10. Fricatives and stops: actions 7 and 9

Action 7 sets the amplitudes around the fricatives. Action 9 sets the stops' burst levels and writes their closures. The notation is that of sections 8 and 9. The node **before** and the node **after** a phoneme are its neighbours in the list, of any type (section 3.4). The text of every recorded run had a word command (`z`, [pipeline.md](pipeline.md) §4.1) before each written word. So after a word's last phoneme comes that command, and the word marks `&` and `%` come right after a phoneme only inside a written word that the stages before split into several, as `100,000,000` or `bad-man`.

### 10.1 Classes

| Class | Name here | Symbols |
|---|---|---|
| (0, 0x08) | mark | `& % [` |
| (1, 0x40) | closing | `! ) , . ? \ ]` |

A **merged** phoneme is one whose node has byte 0x18 set to 1: the stages before made it of two consonants, as the `F` of "five five" from `V F`. The **weakening** `w` is 0, except for a node that isn't stressed: 3 for `J`, and for `C` unless the next phoneme is `s` with `R` the node after it; 6 instead of 3 after a nasal.

### 10.2 Fricative amplitudes (action 7)

1. After the pause: tracks 3 to 7 get mode 4.
2. A phoneme not in class voiced: `T0 = 0` and track 0's `forward` 3. After a phoneme that isn't a vowel, track 0's mode becomes 6 and its `onset` `L0`. After a vowel: mode 4 for `S`, else mode 7 and `onset` 40.
3. Unless the cursor's input flags have bit 0x01, which they have by default ([pipeline.md](pipeline.md) §3.1): for a phoneme not labial after a phoneme other than `J`, `T1` goes down by 12, or 15 for `S`, to 0 at the lowest.
4. A phoneme not in class voiced: `T2` becomes the fricative aspiration minus 20 — before a phoneme in class vowel, if that one is front and the current one is neither `S` nor `X`; before the others, unless it is `X` before `R`, `F` after `j` before `S`, or `S` before the pause or a fricative.
5. `Z` before a stop: `T5..7 = 74, 74, 74`. Then between two of `B D G`, track 0's `forward` becomes the node's frames / 2; else before one in class voiced, `T0 = L0 − 2`, 50 at the lowest. After a nasal, `T1 = 69` and track 1's mode 4.
6. `Z` before a phoneme in class sonorant:
   1. Unless that is `d`: `T0 = L0 − 2`, 50 at the lowest.
   2. Unless the previous phoneme is the pause: `T18 = 10`, and track 18's `back` becomes the previous node's frames, 5 at the most.
   3. After a phoneme in class vowel: `T1 = 64`, track 1's `forward` 2, `T3..7 = 45, 45, 72, 72, 70`. After the pause: `T5, T7 = 72, 72`. After a nasal before `d`: `T1 = 69`, `T5..7 = 74, 74, 74`. After the others: `T5..7 = 68, 68, 68`.
7. `V` after a phoneme in class vowel before `M` or `N`: `T1 = 62`.
8. `Z`: before `X`, `T2 = 50`; after a nasal before `F` or `X`, `T1 = 69` and track 1's mode 4; after a phoneme in class vowel before `F`, `T18 = 17`; with `j` the node before it, before `o`, `T1 = 63` and `T19 = 14`.
9. `X`: after `Z` or `V`, track 1's mode becomes 6. `T8 = 55`. With `S` the node before it: before `R`, `T2 = 73`; before `W` or `L`, `T2 = 50` and `T6, T7 = 75, 80`. Before `E`, `T6 = 70`. Before `w`, `T2 = 65` and `T7 = 70`.
10. `S` after `w` before `E`: `T2 = 55`.
11. `s`:
    - unless `C` is the node before it: `T2 = 48` before a phoneme in class vowel, then `T4..6 = 65, 65, 60` and `T1 = 66`;
    - after `C`, by the next phoneme: `w`, `c` or `u`, `T4, T5 = 68, 65`; `A` or `o`, `T4 = 70`; `a` or `O`, `T4, T5 = 70, 65`; `v`, `T4 = 60`; `R`, `T1 = 68` and `T7 = 55`; `E`, if the node before `C` is r-coloured, `T1, T2 = 72, 50`.
12. `z`: after a phoneme in class vowel before `E`, `T1 = 72`. With `J` the node before it: before `|`, `T1 = 68`; before `r`, `T1 = 65` and `T4 = 65`; before `g`, `T1 = 72` and `T5, T6 = 62, 70`.
13. `Z` after the pause before a phoneme in class vowel: `T18 = 12`.
14. `Z` before the pause after a phoneme in class voiced and in class closure or fricative: `T18, T19 = 8, 14`.
15. `z` before `S`, with `J` or a node in class vowel before it: `T4 = 65`.
16. `Z`: with a nasal the node before it and a node in class mark or closing or the pause after it, `T4 = 66`. After `B`, `D` or `G` before a phoneme in class vowel, `T13 = 70` and `T19 = 14`. After `E` before `F` whose node after is `k`, `T1 = 55`.
17. A merged `F`: each of `T3..8` above 3 goes down by 4.
18. `s`: after `N` before a phoneme in class vowel, `T1 = 68` and `T4, T5 = 68, 68`. Before `E`, after a phoneme that is r-coloured, in class vowel or one of `L j l`: `T1, T2 = 72, 50`. With an r-coloured node after it other than `r` and `4`: `T1, T2 = 72, 50`. After `K` before `g`, `4` or `k`: `T1, T2 = 75, 50`.
19. `V`: after the pause before a phoneme in class vowel, `T19 = 14`. Before a phoneme in class vowel, or with `L` the node after it and a node in class vowel after that: `T0 = 45`.
20. `F` after `f` before the pause: `T2 = 45`.

### 10.3 Stop closure and burst (action 9)

The rules run action 9 on `B P D T J C G K`, and on some `q` and `t`. First, for `B D G K P T`, the burst levels and some targets, by the phoneme:

**B.** The first that applies: before `p`, `b0, b1, b2 = 127, 57, 0`, and `b1 = 60` after `S`; before a phoneme in class vowel after `S`, `b0, b1 = 127, 60`; before `Z` after a phoneme other than `S`, `b1` goes up by 8; before a vowel, `b1 = 62`; before `X`, `b1 = 68`. Then before a phoneme in class vowel after `P`, `T` or `K`: `T0 = 0`. Before `k`: `b1, b2 = 68, 55`.

**D.** The first that applies:
- before `R`, `L`, `W` or `Y`: for a stressed `D` after `S` before `R`, burst frames 2, `b1 = 60` and `T5..7 = 65, 60, 60`; else burst frames 1 and `b0, b1 = 127, 60`;
- with `&` or `%` the node after it, before a phoneme in class vowel, or before `x` after `N`: `b1 = 42`;
- before `|`: `b0, b1, b2 = 50, 60, 48`;
- before `X`: `b1 = 68`;
- before `Z` after a phoneme in class vowel: `b1, b2 = 70, 60`;
- before `M` with a node in class mark after it: burst frames 1, `b1 = 65` and `T5 = 65`;
- before `w` after a phoneme in class vowel: `b1 = 60`.

**G.** `T1 = 0` and track 1's `onset` 0. Before `Z`, `b0 = 50`; before `R` or `L`, `b1 = 66`; before `X`, `b1, b2 = 65, 57`. After a phoneme in class vowel: before a nasal, `b1, b2 = 50, 40`; with `p` the node after it, `b0 = 46`.

**K.** `T1 = 0` and track 1's `onset` 0. The first that applies, by the next phoneme:
- `p`: `b1, b2 = 58, 48` and `T8 = 55`;
- `A`: `b1 = 60`; `a`: `b1 = 64`;
- `k`: `b1 = 64` and `T4..6 = 68, 55, 68`;
- `L`: `b1 = 65` and `T6, T7 = 67, 65`; then `b1 = 55` after a phoneme in class vowel with a node in class mark after the current one;
- `T`: `b1, b2 = 65, 62` and burst frames 1;
- `e`: `b1 = 68`, or 62 after a phoneme in class vowel, and `T4..6 = 65, 55, 55`;
- `|`: `b1 = 65` and `T4..7 = 70, 35, 55, 50`;
- `b`, `u`, `O` or `U`: `b1 = 65`;
- in class vowel with a stressed node: `b1` and `b2` go up by 1;
- `R`: `b1 = 63` and `T3 = 63`;
- `Y`: `T4..7 = 35, 45, 70, 80`;
- `M` or `N`: `b1 = 127`.

Then: after a phoneme in class vowel before `W`, `T3, T7 = 60, 60`, `T10 = 1600` and `b1 = 65`. Before `w`: `T3, T6 = 60, 65` and `b1 = 60`. Before `s` after a phoneme in class vowel: `b1, b2 = 70, 55`, and `T3..6 = 45, 75, 65, 75` when the node after `s` is one of `E a i e A 4 k`. Before `i`: `b1` and `T4..6` go up by 7.

**P.** The first that applies: before `y`, `b1 = 58` and `b2` goes up by 3; before a phoneme in class vowel with a stressed node, `b1` and `b2` go up by 3; before `S`, `b1 = 60`; before `Y`, `T3 = 80`. Then, before a phoneme in class stop or fricative but not in class voiced: `b1, b2 = 70, 62`, and `T10 = 908` if it is alveolar. With `E` the node before it: before `R`, `b1 = 65`, `T3..7 = 0, 0, 0, 0, 0` and `T8 = 60`; before `s`, `b1 = 65` and `T3 = 65`. Before `a`: `T3..5 = 55, 65, 45` and `b1, b2 = 67, 55`.

**T.**
1. Before a phoneme in class vowel with a stressed node, or `p`: `b1 = 65` before `b`; else `b1 = 58`, unless it comes after `N` with `b1` 0. `T8 = 0`.
2. Before an r-coloured phoneme, `b1, b2 = 65, 52`; before another in class vowel, `b1, b2 = 63, 55`. So step 1's `b1` never shows.
3. After `F` before `p`: `b1, b2 = 65, 55`. Before `n`: `b1, b2 = 60, 48`. After a phoneme in class vowel before `K`: `b1, b2 = 68, 58` and burst frames 1. After `S` before a phoneme in class vowel: `b1, b2 = 65, 55`.
4. Before `c`, with the pause, `&` or `%` the node before it: `T4 = 65` and `b1 = 60`. Before `r`: `T3 = 40` and `T4, T7 = 70, 70`.
5. With `&` or `%` the node after it, after `N` whose node before is in class vowel and not stressed, before a phoneme in class vowel whose node after is stressed: burst frames 127.

Then, for every phoneme action 9 runs on:
1. `B`, `D` or `G`: before `Z` whose node after is the pause, `T0 = 55` and `b0 = 0`; else after a phoneme in class voiced, not the pause, `T0 = 42`. After `Z` with `B`, `D` or `G` the node before it, track 0's mode loses bit 0x01.
2. `B` after a phoneme in class vowel with `p` the node after it: `T0 = 41`, track 0's mode 4, `b0 = 43`.
3. Unless the cursor's input flags have bit 0x01: `b1` goes up to 0 at the least.
4. `C` before `s`, with the pause, `&` or `%` the node before it, by the node after `s`: for `3 v A I k r O w c g 4`, `T3..7 = 0, 60, 0, 0, 0`; for `i E 5 o e f y`, `T3..7 = 0, 0, 55, 0, 45`; for `b` or `u`, `T3..7 = 0, 45, 0, 0, 0`.
5. **Burst frames.** With `f` track 1's frames: burst frames of `f` or more, other than 127, become `f / 2`; then they are 1 at the least. On track 1, `AddBack(next, 3, next − earliest, −20)`.
6. **Closure.** If the burst frames are fewer than `f`, the closure is `c = f −` the burst frames, and each of tracks 0, 1 and 2 whose burst level isn't 127 gets it (below). Otherwise `c = f`.
7. `P` with `R` or `L` the node after it, or `T` before a phoneme in class vowel with a stressed node: track 8 gets `Repeat(next, c, 0)`; `T8 = 60` for `P` and 62 for `T`, or 55 for a `T` before a phoneme in class vowel with the pause, `&` or `%` the node before it; track 8's mode becomes 4, it moves on by `c`, and `L8 = 0`.
8. `B` before a phoneme in class vowel: `T8 = 60`. `T` before a phoneme in class vowel, with `S` the node before it and `&` or `%` the node after it: `T8 = 0`. `T` after the pause before `O`: `T8 = 0`.
9. The closure frames become `c`.

**Writing the closure** on track `k`, with `v` its next frame and `bk` its burst level:
1. `Tk` goes down by the cursor's attenuation, to 0 at the lowest.
2. On track 0, the first that applies:
   - after the pause: for `B` with `c > 3`, `Repeat(v, c − 3, 0)` and `Repeat(v + c − 3, 3, T0)`; otherwise `Repeat(v, c, T0)` and `PullForward(v, c / 2, c, T0 / 2)`;
   - `B`, `D` or `G` before `Z` whose node after is the pause: `Repeat(v, c, T0)` and `AddBack(v + c, c + 2, c + 2, −12)`;
   - `T` after a phoneme in class vowel before one in class vowel: `PullBack(v, 6, 6, 40)` and `Repeat(v, c, T0)`;
   - `B` or `D` after a phoneme in class voiced, unless that is `Z` with `B`, `D` or `G` the node before it: `PullBack(v, n / 2, v − earliest, L0 / 4 − T0 / 4 + T0)` with `n` the previous node's frames, and `Repeat(v, c, T0)`.
3. On tracks 1 and 2, and on track 0 if none applies: for `q` after a phoneme in class vowel, before one not in class voiced or `x`, `PullBack(v, n, n, 38)` with `n` the previous node's frames / 4; then `Repeat(v, c, Tk)`.
4. The track moves on by `c`, and `Lk = Tk`.
5. `bk` goes down by the weakening, to 0 at the lowest.
6. `Tk = bk`; but `T0 = 0` for `D`, `G` or `J` after the pause, and for `G` after a consonant not in class voiced.
7. The track's mode becomes 4. Track 0's becomes 5 instead before `p`, and for a `D` not after the pause before a phoneme of place 1 with a stressed node.

## 11. The vowel database: action 4's first step

Action 4 sets a vowel's formants. It first looks the vowel up in the **vowel database**: for each of 24 vowels, records of how F1 to F4 and B1 to B3 move, one for each pair of contexts the vowel has one for. When it finds a record, it writes the vowel's formant and bandwidth tracks from it and ends. This section covers that step; section 12 what action 4 does without a record, and for `p`. The notation is that of sections 8 to 10. A track **moves on** by `n` frames as in section 9. A value's **byte** on a track is the frame byte it gives (section 2.2), and a byte's **value** the reverse.

### 11.1 The database

The database is in the data pack's `vowels.json` ([datapack.md](datapack.md) §6). Its vowels are `@ | O b f U A E I y a e i v o w u 3 r g 5 k 4 c`, the phonemes numbered 0 to 23 (section 3.1): all the vowels but `p`. Each symbol below 128 has a **context**, 0 to 33, or none: the consonants and the pause have one, `H` and `d` the same, and so do the symbols below 32 and 127.

The **reduced** vowels are class (1, 0x10): `@ p |`.

A vowel's records are found by a left and a right context. The records of `@` and `|` give, for each formant, a start, a middle and an end byte, the **timing** of the middle (0 to 15), and for B1 to B3 an end byte. The other vowels have four **trajectory tables**, one for each formant, whose entries are four points (bytes) and four timings (0 to 15). Their records give, for each formant, a start and an end byte and an entry of its trajectory table, and for B1 to B3 a start, a middle and an end in Hz. Some of these records have an **alternative**: another such record, used instead when the node before the previous phoneme, or for some the node after the next one, has a given symbol.

### 11.2 The step

1. The coarticulation flag becomes 0. If the current phoneme isn't one of the database's vowels, the step ends.
2. After a phoneme in class vowel, the voice onset becomes 127.
3. The contexts and the vowel frames (section 11.3). If either symbol has no context, the step ends.
4. The record (section 11.4). If there is none, the step ends. If the record has an alternative and the neighbour it names has its symbol, the alternative is the record.
5. Each formant's track and bandwidth are written from the record (sections 11.5 and 11.6), and their segments set (section 11.7), F1 first.
6. The coarticulation flag becomes 1 and the voice onset 127, and action 4 ends.

The **phoneme after the next** is a symbol the stage's step keeps (section 17.2, step 8): that of the first phoneme after the next one, past word marks and punctuation, or a space if another node comes first.

### 11.3 The contexts

The database looks the vowel up by a left symbol `l` and a right symbol `r`, at first the previous and the next phoneme. With `A` the node after the current one, `A'` the node after `A` and `B` the node before the current one:
1. A previous phoneme in class vowel gives `l`: `Y` for `A E I a e i y |`, `W` for `O U b u f`, `R` for `3 4 k c g r 5`, else a space (`@ v o w p`). Otherwise an `l` of `n` becomes `N`.
2. A next phoneme in class vowel gives `r` by the current vowel: `R` for `3 k r g 4 c 5`, `W` for `b f O U`, a space for `@ v o w`, else `Y`.
3. For `A`, `I` or `y` (front-ending, but not `5` or `E`) with a word mark `A`, unless `A'` is one of `R L W Y d H t` or it is `I` whose node isn't stressed: `r` becomes a space. Otherwise an `r` of `n` becomes `N`.
4. A word mark `A` with `A'` being `N`, for a stressed vowel other than `@ | 5`: `r` becomes a space.
5. `E` with `l` being `M`, `r` a space and `A'` being `M`: `r` becomes `M`.
6. A word mark `A` whose node after is merged (section 10.1): `r` becomes `A'`.
7. A word mark `B`, `l` a fricative other than `Z` and `S`, and a stressed current node: `l` becomes a space for a front-ending vowel other than `I`. For the others it does if `A` is a nasal, unless the vowel is `e`, `l` is `X`, or it is `3` with `l` being `s`, or `a` with `l` being `V`.
8. `v` with `A` being `N` and `B` being `&`: `l` becomes a space.
9. A word mark `B` whose node has byte 0x14 set to 1: `l` becomes a space. Such a word mark `A`: `r` becomes a space. (The stages before set the byte to 1 on some word marks, as at a phrase's start and between spelled letters, and to 2 on those inside numbers.)
10. A reduced vowel whose node isn't stressed, with `A` being `q` and the phoneme after the next a space: `r` becomes `T`.
11. `E` with `l` being `Z`, `r` being `Y`, `A` being `Y` and `A'` being `@`: the vowel looked up is `i` instead.

The **vowel frames** `d` are the current node's frames, less the voice onset unless that is 0 or 127, and 1 at the least. Below, `n = d − 1`.

### 11.4 Finding the record

The record is the vowel's for the contexts of `l` and `r`. Without one, the database tries **variants**: `D` and `t` of each other, `L` gives `l`, `l` gives `j`, `j` gives `L`, `X` and `x` of each other, and on the right also `z` gives `J`. The **lateral** of `L` is `j`, of `l` it is `L`, and of the others `l`. Each lookup uses the contexts last set for each side. With `l'` and `r'` the symbols tried, at first `l` and `r`, it does this twice while there is no record:
1. `r'` becomes its variant. If that isn't `r`, its context is set and the record looked up.
2. If none, and `r'` is `j`, `l` or `L`: `r'` becomes the lateral of `r`, its context is set and the record looked up.
3. If none, and `l'` is still `l`: `r'` becomes `r` again and its context is set; `l'` becomes its variant, and if that isn't `l` its context is set and the record looked up.
4. If none, and `l` is `j`, `l` or `L`: `l'` becomes the lateral of `l`, its context is set and the record looked up.

### 11.5 Vowels other than `@` and `|`

For each formant `k`, on track `9 + k`, with the record's start byte `s` and end byte `e` and its entry of trajectory table `k`, points `p1` to `p4` and timings `a`, `b`, `c` and `f`:
- the **times** are `t1 = ((a + 10) · n + 50) / 100`, `t2 = ((b + 10) · 2n + 50) / 100`, `t3 = ((c + 25) · 2n + 50) / 100` and `t4 = ((2f + 65) · n + 50) / 100`;
- for F1 to F3, the bandwidth's start, middle and end bytes are the record's B start, middle and end, each 200 Hz at the most, halved. F4 has no bandwidth track.

The track is written (below) and its segments set (section 11.7). Then, with `x` the track's next frame and `g` its segment's frames: if `g` is 4 or less, `Ramp(x + 1, g − 1, s, e)`; if it is 5 to 7, with `m = (p1 + p2 + p3 + p4) / 4` and `h = (g − 1) / 2`, `Ramp(x + 1, h, s, m)` and `Ramp(x + 1 + h, g − h − 1, m, e)`.

**Writing the track.** With `S`, `P1` to `P4` and `E` the values of `s`, the points and `e`, `L` the track's last value and `b` its byte, and the voice onset `v`, which **counts** when it is above 0 and not 127:
1. On track 9: if any of `S`, `P1` to `P4` and `E` is above 650, `T0` goes down by 3.
2. **Held start.** After one of the database's vowels, if the previous node isn't stressed and the current one is, or both or neither are and the previous node has fewer frames: `S = L` and `s = b`.
3. With `x` the track's next frame: if `v` counts, `Fill(x, min(v, 20), v, o, s)`, where `o` is `b`, but on track 9 `s` unless the previous phoneme is `P`, `T` or `K` and `b` isn't `s`. `x` moves on by `v`, and if `v` is 1 the track moves on by 1.
4. For a vowel of fewer than 8 vowel frames, not after an approximant, the points become halfway points: `p1` the byte of `S + (P1 − S) / 2`, `p2` of `P1 + (P2 − P1) / 2`, `p3` of `P2 + (P3 − P2) / 2`, `p4` of `P3 + (P4 − P3) / 2`, and `e` the byte of `P4 + (E − P4) / 2`.
5. `t4` becomes `d − 3` if that is more. Then if `t4 < t3`, `t3 = t4 − 1`; if `t3 < t2`, `t2 = t3 − 1`; if `t2 < t1`, `t1 = t2 − 1`.
6. After a **front `Y`**, a `Y` whose node before is front-ending: on track 9, `s` becomes the byte of `P1 + (L9 − P1) / 2`. With a held start where `L` and `P1` are more than 200 apart: `s` becomes the byte of `P1 + (L − P1) / 2`.
7. `Fill(x, 1, 1, s, s)`; after a front `Y`, `PullBack(x, w + 1, w + 1, s)` with `w` the previous node's frames. `x` moves on by 1.
8. Not after an approximant, if `t1 > 0`: with `u = min(t1, 20)`, `Fill(x, u, u, o, p1)` with `o` being `p1` if `u` is 1, else `s`. `x` moves on by `u`.
9. If `t2 > t1`: `Ramp(x, t2 − t1, p1, p2)`, or after an approximant `Ramp(x, t2, s, p2)`, and `x` moves on by those frames. After a front `Y`, the track's next frame goes up by `t1`, its segment's frames down by `t1`, its back target becomes `P1` and its back `t1 + 1`, or `w + 2 + t1` if `s` isn't `b`. If `t2 ≤ t1`, after an approximant: `Ramp(x, t2, s, p2)`, and `x` moves on by `t2`.
10. If `t3 > t2`: `Ramp(x, t3 − t2, p2, p3)`, `x` moving on. If `t4 > t3`: `Ramp(x, t4 − t3, p3, p4)`, `x` moving on.
11. If `d − t4 − 1 > 0`: `Ramp(x, min(d − t4 − 1, 20), p4, e)`.
12. For F1 to F3, on the bandwidth's track `13 + k`, with `x` its next frame, `b'` its last value halved as a byte, and the bandwidth bytes `B1`, `B2` and `B3` (start, middle and end): `q1 = (d + 1) / 2`, `q2 = (d + 3) / 4` and `q3 = d − q1 − q2`.
    1. If `v` counts, with `u = min(v, 20)`: for `u < 4` and `d` of 5 to 7, `Fill(x, 3, 3, b', B1)`, `x` moves on by 3 and `q1` by `3 − v`; else for `d < 8`, `Fill(x, u, v, b', B1)` and `x` moves on by `v`; else `Fill(x, u + 3, v + 3, b', B1)` and `x` moves on by `v + 3`. The bandwidth's track moves on by `v`.
    2. If `v` doesn't count and `d` is 8 or more: `Fill(x, 3, 3, b', B1)`, and `x` moves on by 3.
    3. If `d` is 8 or more, `q1` goes down by 3.
    4. `Fill(x, min(q1, 20), q1, B1, B2)`, `x` moving on by `q1`; if `q2 > 0`, `Fill(x, min(q2, 20), q2, B2, B3)`, `x` moving on by `q2`; if `q3 > 0`, `Fill(x, min(q3, 20), q3, B3, B3)`.
    5. The bandwidth's track moves on by `d`.

### 11.6 `@` and `|`

For each formant `k`, on track `9 + k`, with the record's start, middle and end bytes `s`, `m` and `e`, its timing `τ`, `t = ((τ + 3) · n + 10) · 5 / 100` and 1 at the least, `v`, `b` and `b'` as in section 11.5 and `x` the track's next frame:
1. If `v` counts: on track 9 with `v` being 1, `Fill(x, 1, 1, s, s)`; otherwise `Fill(x, min(v, 20), v, b, s)`. `x` moves on by `v`, and if `v` is 1 the track moves on by 1.
2. After a front `Y`: `PullBack(x, w + 1, w + 1, s)` with `w` the previous node's frames.
3. `Fill(x, 1, 1, s, s)`, `x` moving on by 1; `Ramp(x, min(t, 20), s, m)`, `x` moving on by `min(t, 20)`.
4. With `u = min(d − t − 1, 20)`, if `u > 0`: for `@` before `j`, `Fill(x, u, u, m, m)`; otherwise `Ramp(x, u, m, e)`.
5. On track 9: if the value of `s`, `m` or `e` is above 650, `T0` goes down by 3.
6. For F1 to F3, on the bandwidth's track with `x` its next frame and the record's B end byte `B3`: if `v` counts, `Fill(x, min(v, 20), v, b', b')`, the bandwidth's track moves on by `v` and `x` with it. With `q = (d + 1) / 2`: `Ramp(x, min(q, 20), b', B3)`, and if `d − q > 0`, `Fill(x + q, min(d − q, 20), d − q, B3, B3)`. The bandwidth's track moves on by `d`.

### 11.7 The segments

After each formant's track is written, its segment is set, with `S` and `E` the values of the record's start and end bytes, `w` the previous node's frames, and `N` the node before the previous phoneme, or the node before that if `N` is in class mark (a space if there is none):
1. The mode is 0. Unless the previous phoneme is one of the database's vowels, or `m n ~ B D G` on tracks 10 to 12, or `P T K`, it is 1 instead, and by the previous phoneme:
   - `L` or `W`: on track 9, `back = w + 1`;
   - `M` or `N`: on track 9, `back = w`; for `M`, the mode is 0 on track 10;
   - `R`: on track 12, `back = w` if `N` is in class fricative or closure but not voiced; else `back = w / 2` if `N` is `u`, `4`, `c` or `g`; else the mode is 0. On track 9 with `N` a nasal, `back = w + 1`. On the others, `back = w / 2`;
   - `Y`: the mode is 0 if `N` is in class vowel but not front-ending;
   - `j`: `back = w` on track 11, and on track 9 if `N` is in class vowel.
   
   Then for `L` after a phoneme in class stop (the phoneme before the previous one): `back = w / 3`, 2 at the least.
2. After a phoneme in class vowel: the mode is 16, `back = 1` after a reduced one, else `w / 3`, 2 at the least, and `forward = 0` for a reduced current one, else its node's frames / 3, 2 at the least. Otherwise, if the track's next frame isn't track 16's and the voice onset is 127: the mode is 1 and `onset = S`. Otherwise, on track 9 after `j`: `back target = S`, or `S − 100` if `L9` is above `S + 100`, and 250 at the least. Otherwise `onset` and `back target` are `S`.
3. On track 9 after `t`: `back target` goes down by 100, and by 50 more if `L9` is then above it; 250 at the least. On track 9 after `R` with `N` a nasal: `back target` goes down by 20.
4. After `R` with `N` in class vowel, on tracks 9 to 11: the mode is 0.
5. The segment gets the mode and `target = E`, and the track's last value is `E`. For F1 to F3 the bandwidth's segment gets mode 0, and its `onset`, `back target` and `target` and its track's last value are the value of the bandwidth's end byte.

## 12. Action 4 without a record

When the vowel database has no record (section 11.2), action 4 goes on with the steps below. That is so for `p`, which isn't one of the database's vowels, and for a vowel whose contexts the database has no record for. In English text that is rare, as in `maharaja`, `concur`, `voluminous` or `Tecumseh`; the vowels without an offglide, `3 o v`, and `u` before `j` always have one, and only phonetic input (the escape sequence `ESC[1I`, which takes the engine's phonemes as they are) gets them here. The notation is that of sections 8 to 11. `O9` to `O15` are the **offglide** of tracks 9 to 15, which loading the targets sets for a vowel with an offglide (section 4.1); `M9` to `M15` are the **midpoints**, values the generator keeps for the same tracks; `X11` is track 11's next target (section 4.1).

### 12.1 Classes

| Class | Name here | Phonemes |
|---|---|---|
| (3, 0x08) | high front | `E U Y` |
| (3, 0x10) | with an offglide | the vowels with an offglide (section 3.1) |
| (4, 0x40) | neutral | `3 @ l m n \|` |

### 12.2 The first steps

1. **F2 and F3.** For `u` before `j`: `T10` goes down by 150. Otherwise, before a phoneme in class lateral: for a phoneme in class front, `O10` and `T10` go down by 300. Otherwise, after a phoneme in class rhotic: `T11 = (T11 + 600) · 3 / 4` and `O11 = (O11 + 600) · 3 / 4`, and for a phoneme in class front also `T10 = (3 · T10 + 1400) / 4` and `O10 = (3 · O10 + 1400) / 4`.
2. `U` before an alveolar stop (in classes stop and alveolar: `D T q`): `O10` goes up by 400.
3. A phoneme in class rounded after one in class high front: track 10's `forward` goes up by 5.
4. **Toward neutral.** A phoneme in neither class rhotic nor class neutral: with `f` its node's frames, `r` the data pack's `reductionWeights[min(10f, 255) / 16]` and `n` the data pack's `neutralFormants` of F1 to F3, on tracks 9, 10 and 11 in order: `T = T + Q15(n − T, r)` and `O = O + Q15(n − O, r)`.
5. Then `@`, `p` and `|` (class reduced) go on with section 12.3; the vowels without an offglide and `u` before `j` with section 12.4; the other vowels with section 12.5.

### 12.3 `@`, `p` and `|`

1. `p` after a phoneme in classes stop and voiced (`B D G J q`): `T0 = 54`. `p` after `K`: `T10, T11 = 1652, 1856`.
2. A current node that isn't stressed: `T0` goes down by 2, to 0 at the lowest.
3. `T11 = Q15(X11 + L11 + T11, 10824)`.

### 12.4 Vowels without an offglide

1. Tracks 9 to 12 get `onset = L`, the track's last value.
2. By the previous phoneme:
   - `G`: track 9 gets mode 6, and the step goes on as for `K`;
   - `K`: track 12 gets mode 6, and `W12 = 0`;
   - `M`: if the node before it is a vowel and the current node isn't stressed, track 9 gets `back` the previous node's frames plus 1, and the 1, the pull back, in its mode;
   - `P`: for `o`, tracks 3 and 4 get mode 5;
   - a vowel: tracks 9 to 12 get mode 6 and `forward` the current node's frames / 2.
3. A current node that isn't stressed: `T0` goes down by 3.

### 12.5 Vowels with an offglide

Each formant and bandwidth is written here in two segments, from its start to a midpoint and from there to its offglide. With `g` track 9's segment frames and `f` the current node's frames:
1. The vowel transition `t` becomes `(t · g / f + t) >> 1`, the division of unsigned numbers.
2. The vowel offset `v` becomes `Q15(g, offset)`, with the phoneme's `offset` in the data pack.
3. On each of tracks 9 to 15 in order, with `T`, `O` and `M` its target, offglide and midpoint, `s` its segment's `onset`, `x` and `e` the track's next and earliest frames, and `frames`, `forward`, `back` and `back target` its segment's:
   1. The segment gets mode 7. For a phoneme in class r-coloured whose node isn't stressed, `T` becomes `T − ((T − O) >> 2)`.
   2. `M = (T + O) / 2`. On track 11, for `U`, `v` goes down by 7.
   3. **The start.** If `v` is below 1: `M = T`. Otherwise:
      1. if `v` is the segment's `frames` or more, `M = O` and `v` becomes `frames`;
      2. the segment gets mode 6 and `onset = T`, but after `N` the vowel's target after `N` (below), and after a vowel `L`. After `M` whose node before is a vowel, with the current node not stressed, track 9's segment gets `back` the previous node's frames plus 1 and mode 7;
      3. with `target = M`, `frames = v` and `forward = min(l, v)`, `l` the transition limit (section 7), the segment is rendered (section 2.4), and `s` becomes its `onset`;
      4. the track's next frame goes up by `v`, and the segment's frames are the original's less `v`.
   4. **The end.** The track's earliest frame becomes `x`. The segment gets `target = O`, `onset` and `back target` `M`, and `forward` and `back` `min(l, t)`, and it is rendered.
   5. **The phoneme's own segment.** The segment gets back its target `T` and its frames, and the track its next frame `x`. After a vowel: `onset = L` and mode 0. After `N`: `onset` the vowel's target after `N`, but `s` on track 9, and mode 0 on tracks 10 to 12, 1 on the others. After another phoneme: mode 3, but after `M` track 10 keeps the mode it has.
   6. The track gets back its earliest frame `e`, the segment its `forward`, `back` and `back target`.
4. After `K` or `G`: track 12 gets mode 6, and `W12 = 0`.
5. A current node that isn't stressed: `T0` goes down by 3.
6. A phoneme of place 2 or 3 after `G`: track 9 gets mode 6.

**Targets after `N`**, in Hz:

| Vowel | F1 | F2 | F3 | F4 | B1 | B2 | B3 |
|---|---|---|---|---|---|---|---|
| `4` | 264 | 1841 | 2462 | 3394 | 68 | 408 | 188 |
| `g` | 296 | 1606 | 2534 | 3468 | 75 | 408 | 281 |
| `k` | 324 | 1646 | 2471 | 3436 | 128 | 74 | 55 |
| `r` | 354 | 1059 | 2709 | 3289 | 141 | 196 | 94 |
| `u` | 383 | 1489 | 2412 | 3042 | 149 | 79 | 60 |
| `w` | 387 | 1461 | 2554 | 3521 | 178 | 156 | 80 |
| the others | 340 | 1459 | 2517 | 3353 | 125 | 185 | 135 |

## 13. Finishing a phoneme: action 10

Every rule ends with action 10. It runs, in order, the loci (section 13.2), the special loci (section 13.3), action 13 (the consonants' targets and the pitch), action 14 (the sources), the final adjustments (section 13.4) and the limits (section 13.5). Action 13 is in sections 14 and 15, action 14 in section 16. The notation is that of sections 8 and 9. `f` is the current node's frames and `g` the previous node's.

### 13.1 Classes

Action 10 tests the classes of sections 3.2, 8.1, 9.1, 10.1 and 12.1, the reduced class (section 11.1) and this one:

| Class | Name here | Phonemes |
|---|---|---|
| (2, 0x80) | velar | `G K ~` |

### 13.2 The loci

1. `p` after a stop: track 11's `onset` becomes `L11`.
2. If the category (section 3.3) is above the previous phoneme's, step 3 runs, otherwise step 4. Both can end the loci early.
3. **Toward a closure.**
   1. Unless after the pause:
      1. A palatal phoneme not in class sonorant (`J C z s`): `W10 = 26240`, and track 10's `forward` goes up by 3.
      2. Unless the current phoneme is in class closure and the previous one isn't, the loci end.
      3. `q` after a phoneme in class vowel: `W9 = 32472`, and track 9's `back` becomes `g − g / 3`. Otherwise `W9 = 16400`.
      4. A phoneme neither labial nor `T`: `W10, W11 = 32766, 32766`.
      5. Before `s` whose node after is the pause: `W10, W11 = 3280, 3280`.
      6. `K`: `W9 = 32766`, and after a vowel with an offglide `W10 = 6560`.
   2. `~`, `m` and `n`: `W9 = 24600` and tracks 9 to 16 get mode 5. Then after a phoneme in class sonorant: `W11 = 29520`, `W16 = 18040` and tracks 13 and 16 get `forward` 10; for `m`, `W12 = 9840` and track 12 gets `forward` 9 and mode 7; for `~` before a phoneme in class sonorant, `T10, T11 = 900, 2176`, and before others track 10's `forward` becomes 7 and `T10, T11 = 1200, 2500` after a front phoneme, else `T10 = (L10 + T10) / 2` and `T11 = (L11 + T11) / 2`. After the pause instead: `W0 = 13120` and track 0's `forward` becomes 6.
4. **Toward an opening, or the same category.** Unless after the pause:
   1. `R`: `W10, W11 = 32472, 32472` after `s` whose node before is `C`, else `W10, W11 = 24600, 24600`. Any phoneme but `R` and the vowels, after a palatal phoneme not in class sonorant: `W10 = 6560` and track 10's `forward` goes up by 3.
   2. A palatal phoneme after an affricate: `T11` goes up by 300, but not for `s` after `C` before a phoneme in class vowel or `R`, nor for `z` after `J` before a phoneme of place 1.
   3. A phoneme in class vowel after `S`: track 1's mode becomes 4.
   4. A phoneme not in class sonorant after a nasal: tracks 10 to 12 get `forward` 5 and tracks 13 to 16 `forward` 2.
   5. Unless the previous phoneme is in class closure and the current one isn't, the loci end.
   6. A phoneme in class vowel after a stop in class voiced: track 1's mode becomes 5.
   7. `W9, W10, W11 = 16400, 0, 0`. Then, for a phoneme that isn't a vowel: after a labial, `W10, W11 = 6560, 22960`, except for `S` after `P`; after an alveolar other than `T`, track 11's `onset` becomes 2620, and track 10's 1600 unless the previous phoneme is a nasal or `D`; and `R` after an alveolar other than `D` gets track 11's `onset` 2300. After a velar: `W9 = 0`.
   8. Also after the pause: a phoneme in class sonorant after a nasal makes `W9 = 8200`; if it isn't a vowel, tracks 9 to 11 get mode 6; and unless it is `p` after `~` before the pause, `W11 = 3280`, `W16 = 24600` and track 16 gets `forward` 9.
5. By the previous phoneme:
   - `M` or `m`: `W12 = 29520`. A phoneme that isn't front then makes `W10 = 3280`; `4` makes `W12 = 16400`, and track 12 gets `forward` 5 and the 1, the pull back, in its mode.
   - `N` or `n`: a phoneme that isn't a vowel makes track 11's `onset` 2600 and track 12's 3400. `W11, W12 = 0, 0`. A high phoneme that is neither front nor a vowel (`W Y h`): track 10's `onset` becomes 1670 and `W10 = 0`. `d` after `N`: `W18, W19 = 6560, 6560`, and tracks 18 and 19 get `forward` 4.
   - `~`: `p` before the pause makes tracks 10 to 16 get `target = L` and mode 4.
6. A syllabic phoneme after a phoneme in class voiced, with both nodes stressed and without flag bit 0x40: `W17 = 16400` and track 17 gets mode 7.

### 13.3 The special loci

1. After a rhotic or r-coloured phoneme: `~`, and `K` before a phoneme not in class vowel, make `T10, T11 = 1700, 1900`. After `R`, a phoneme in class vowel gets track 11's `forward` 5, and `E` also track 0's `forward` 6.
2. `R` after a labial: `W10 = 6560`. Otherwise, after a front-ending phoneme, `Z`, `n`, `q` and `t` make `T10 = 1900`.
3. `d` before `b`: `T9 = 182`.
4. `B` after a phoneme in class vowel, with the node after it `p`: track 0's mode becomes 4.
5. `y` after `L`: track 9's `back` becomes 5.
6. `J`: `T1 = 64`.

### 13.4 The final adjustments

`t(x)` is `x / 3`, 2 at the least.
1. `M` after `a`: before a labial, tracks 10 to 12 get `back` 2 and `forward` 3. Before `p`, tracks 10 and 11 get `back` 3, track 10 gets `forward` 4 and tracks 11 and 12 `forward` 3.
2. With the coarticulation flag set (section 11.2), a phoneme not in class vowel: tracks 9 to 12 lose the 1 of their mode, and unless the phoneme is a nasal they get `onset = L`. `L` after a phoneme that isn't a vowel: track 9's mode becomes 4. The flag becomes 0.
3. Let `b` be the node before the previous one, or the node before `b` if `b` is in class mark (the pause if there is none). If `b` is a vowel, unless the previous phoneme is `Y` and `b` is front-ending: tracks 9 to 12 get `back` at most `g`, compared as unsigned numbers.
4. A stop: tracks 13 to 15 get `forward` their segment's `frames − 1`.
5. `n`, or after `n`: track 16 gets mode 20, `forward` 3 and `back` 3. After `n`, unless the phoneme is `R`: tracks 9 to 15 get the 1 in their mode and `back = g / 3`.
6. The bandwidths, tracks 13 to 15:
   - After `p` or a phoneme not in class vowel, a phoneme in class vowel other than `p` gets mode 16, `back = t(g)`, and `forward` 3 if `f / 3` is above 1, else 2.
   - After a phoneme in class vowel other than `p`: the mode gets the 1, and `back target` becomes the segment's `target` if the mode now is 5, else its `onset`. Then a phoneme not in class vowel gets mode 20, `back = t(g)` and `forward = t(f)`. One in class vowel gets mode 16, `back` 1 after a reduced phoneme, else `t(g)`, and `forward` 0 if it is reduced, else `t(f)`. The pause then gets `onset` and `back target` 300 on track 13, 188 on track 14 and 194 on track 15.

   Where mode 16 is set with the coarticulation flag set, the track's next frame goes down by `f` and its segment gets `f` frames.
7. After `R` whose node before is `p`: tracks 9 to 12 keep only the bits 2 and 4 of their mode.
8. `M`: after a phoneme in class vowel, track 9 gets mode 8, `forward = f / 2`, `onset = L9` and `T9 = L9 − 50`. After a front-ending phoneme, before a phoneme in class vowel: track 10 gets mode 20, `forward = f / 2` and `back = g / 4`.
9. A phoneme in class vowel with `T9` above 700: `T0` goes down by 3.
10. `p` after `~`: tracks 3 to 7 get mode 4, `T0 = 55`, `T3..7 = 57, 0, 0, 55, 0` and `T9..12 = 250, 1568, 2208, 3600`, and track 10 gets mode 7, `forward` 2 and `onset` 1700. Then on track 1 `Repeat(next, 1, 58)` and on track 0 `Repeat(next, 1, 51)` (section 2.5): each of the two tracks' next frame goes up by 1 and its segment's frames down by 1.
11. `~`: after a phoneme in class vowel, before `p`: `T9..11 = 250, 1196, 2288`. After `i`: track 10 gets mode 7, `back` 3, `back target` 1500, `forward` 3 and `onset` 1500.
12. `a`: `T0 = 60`.

### 13.5 The limits

1. Unless the phoneme is `T` before `3`: on tracks 10, 11 and 12 in order, the segment's `onset` becomes at least the previous track's `onset` plus 200, and its `back target` at least the previous track's `back target` plus 200.
2. Every track's `forward` becomes 30 at the most and its `back` 20 at the most.
3. `T1` above 0 goes up by 7.

## 14. Consonants before a vowel or a liquid: action 13's helpers

Action 13 (section 15) sets the consonants' targets and the pitch's small movements. It calls two helpers, which this section covers: the **loci from the vowel database**, which take a consonant's formant targets from the vowel after it, and the **liquids**, which write the formants through `R` and `L` and through `Y` after a vowel, and set the targets of a consonant before `R` or `L`. The notation is that of sections 8 to 13. `f` is the current node's frames and `w` the previous node's. The **phoneme before the previous one** is a symbol the stage's step keeps (section 17.2, step 13).

### 14.1 The loci from the vowel database

Action 13 calls this step for a consonant before one of the vowel database's vowels (section 11.1). It looks that vowel up with the consonant as its left context. With `V` the next phoneme, `A` the node after `V`, `C` the node after the current one (`V`, or a word mark before it) and `C'` the node after `C`:
1. The left symbol `l` is the current phoneme, `N` for `n`. The right symbol `r` is `A` and the **neighbour** the node after `A`; but if `A` is a word mark, `r` is the node after it and the neighbour the node after that, or a space if there is none.
2. An `r` in class vowel becomes, by `V`: `R` for `3 k r g 4`, `W` for `b f O U`, a space for `@ v o w`, else `Y`. (Section 11.3 has `c` and `5` with `R`; here they give `Y`.)
3. If `C` is front-ending but not `E` or `5`, and `C'` is a word mark whose node after isn't one of `R L W Y d H t`: `r` becomes a space. Otherwise an `r` of `n` becomes `N`.
4. If `C'` is a word mark whose node after is merged (section 10.1): `r` becomes the symbol of that node.
5. If `C` is a word mark, `l` a fricative other than `Z` and `S`, and `C'` stressed: `l` becomes a space if `C'` is front-ending, or if the node after `C'` is a nasal, unless the current phoneme is `X`, or `s` with `C'` being `3`, or `V` with `C'` being `a`.
6. If `l` or `r` has no context, there is no record. Otherwise the record is found as in section 11.4, but `z` has no variant. If the record has an alternative and the neighbour it names has its symbol, the alternative is the record: the previous phoneme, or for those named by the node after the next one, the neighbour of step 1.
7. With a record, `T9` to `T12` become the values of its start bytes (section 2.2), and the step reports that it found one.

### 14.2 The targets before a liquid

Action 13 calls this step for a consonant before `R` or `L` that the vowel database has no record for. With `C` the node after the current one and `A` the node after `C`, and the **group** of a symbol among lists of symbols the number of the first list that has it, or the number of lists:
- If `C` is a word mark and `A` is `R`, the targets come from the four lists of the data pack's targets across a mark ([datapack.md](datapack.md) §5), by the current phoneme's group: `T9` among `K`; `T10` among `x`, `X`, `K`, `j l`, `P S` and `s`; `T11` among `G J Z z`, `P`, `K` and `X S`; `T12` among `~`, `R z` and `P`.
- Otherwise the consonant is the current phoneme, but `D` for `z` after `J` and `T` for `s` after `C`. With the table of `L` if `C` is `L`, else the table of `R`, where an `A` of `l` is taken as `|`: `T9` to `T12` become the start values of the rows of F1 to F4 for the consonant and `A` (section 14.3).

### 14.3 The liquids

The data pack's liquid tables ([datapack.md](datapack.md) §5) have **rows**, which give a formant's start, middle and end, each a value in Hz and a kind, and a **duration**, the percent of the phoneme's frames to the middle. The tables of `R`, `L` and `Y` have rows for each of F1 to F4, chosen by context: every row but the last has contexts, which are a symbol and a list of symbols. The row for symbols `a` and `b` is the first that has a context of `a` with `b` in its list, or else the last row.

Action 13 calls the step below for `R` in some contexts, for `L` whose node before is in class stop, and for `Y` after a phoneme in class vowel that isn't front-ending, with a node in class vowel after it. Each track from 9 takes a row (writing a track, below), F1 first:
1. **`R` after a phoneme in class vowel**: tracks 9 to 11 take the rows of `R` after a vowel. F1 takes row 0. After `A b E I y`, F2 takes row 0, `T13`, `T14` and `T15` become 60, 100 and 156, and track 10's `back = f / 3`; otherwise F2 takes row 1, they become 94, 97 and 130, and track 10's `back = f / 4`. F3 takes row 0 after `g c k r 3 4 @ |`, else row 1. Tracks 9 and 11 get `back = f / 2`.
2. **`R` after a word mark** (the node before it): with `p` the previous phoneme, or the phoneme before the previous one if `p` is the pause, tracks 9 to 12 take the rows of `R` across a mark. F1 takes row 0. F2 takes row 0 before `E`, else row 1 after `j l K` or before `y c g 3`, else row 2 after `x`, row 3 after `P` and row 4 after the others. F3 takes the row of `p`'s group among `G J Z`, `P` and `V`, and F4 among `~`, `R` and `P`. Track 12's mode becomes 1.
3. **`Y`**: tracks 9 to 12 take the rows of `Y` for the previous phoneme and the next one.
4. **Otherwise** (`R` or `L`): with `p` as in step 2, but `D` if it is `z` and the phoneme before the previous one is `J`, and `T` if it is `s` and that phoneme is `C`: tracks 9 to 12 take the rows of `L` for an `L`, else of `R`, for `p` and the next phoneme, a next `l` taken as `|` for `R`. Track 12's mode becomes 1.
5. Tracks 9 to 11 get mode 1. Tracks 13 to 15 get mode 20, `back = w / 4` and `forward = f / 4`, each 2 for fewer than 8 frames.

**Writing a track.** A point of kind 0 is its value. Of kind 1, it is its value plus the track's last value, or for the end plus the segment's `target`. Of kind 2, it is its value plus the lower of the two, and on track 9 it becomes 204 if that is below 200. On track 11 each point is 2950 at the most. With `s`, `m` and `e` the bytes of the points (section 2.2), `t = (d · f + 50) / 100` for the row's duration `d`, `x` the track's next frame, `b` its last value's byte, not cut to 8 bits, and the voice onset `v`, which counts as in section 11.5:
1. If `v` counts: `Fill(x, min(v, 20), v, o, s)`, with `o` being `b`, but on track 9 `s` unless the previous phoneme is `P`, `T` or `K` and `b` isn't `s`. `x` moves on by `v`, and if `v` is 1 the track moves on by 1.
2. The vowel frames `d'` (section 11.3) become `f`, less `v` unless `v` is 0 or 127; here they aren't raised to 1.
3. `Fill(x, 1, 1, s, s)`, and `x` moves on by 1.
4. With a middle of value 0: `Ramp(x, min(d' − 1, 20), s, e)`. Otherwise, with `n = t − 1`, 1 at the least: `Ramp(x, min(n, 20), s, m)`, `x` moves on by `min(n, 20)`, and if `d' − n − 1 > 0`, `Ramp(x, min(d' − n − 1, 20), m, e)`.
5. The segment's `onset` and `back target` become the start's value.

## 15. The consonants' targets and the pitch: action 13

Action 10 runs action 13 after its special loci (section 13); no rule in the data pack runs it directly. It sets a consonant's formant targets by the phonemes around it, partly with the helpers of section 14, holds the transitions to their limits, sets the onsets from the weights, sets the transitions of many contexts, and writes the pitch's small movements: a dip at a glottal stop, and a rise and fall on a stressed vowel before the pause. The notation is that of sections 8 to 14. `f` is the current node's frames and `g` the previous node's, and `b` is the node before the previous one as in section 13.4, step 3. There is a **boundary** before the current node if the node before it is a word mark or the pause. A **locus consonant** is one of `B P D T G K`, a fricative, `H` or `d`.

### 15.1 Classes

Action 13 tests the classes of sections 3.2, 8.1, 9.1, 10.1, 12.1 and 13.1, and this one:

| Class | Name here | Phonemes |
|---|---|---|
| (4, 0x10) | dental | `X x` |

### 15.2 The targets

1. `t`: `T9`, `T10` and `T11` each become the Q15 product of `L + N + T` and 10824, with `N` the track's next target (section 4.1, step 10).
2. After a high front phoneme, a phoneme that is none of a nasal, dental, labial, velar or a vowel: track 10's `forward` goes down by 2.
3. **The loci.** The database loci flag becomes 0. For a locus consonant, `R` not after `s`, `L` whose node before is in class stop, and `Y` after a phoneme in class vowel with a node in class vowel after it: if the next phoneme is one of the vowel database's vowels (section 11.1), the loci from the vowel database run (section 14.1), and the flag becomes 1 if they found a record. Otherwise, before `R` but for `P` and `s`, and for a stop whose node after is `L`, the targets before a liquid run (section 14.2).
4. `T` whose node after is `o`: `T1 = 68` and `T3..6 = 50, 50, 70, 70`.
5. `K` before `r`, `g` or `c`: `T10..12 = 1080, 1680, 2896`, `T1 = 62`, `T3 = 65` and `T14, T15 = 200, 200`. `K` before `g` then also gets `T1 = 65`, `T2 = 55` and `T6 = 70`.
6. `r`, `g` or `c` after `K`: `T3..5 = 60, 60, 60`.
7. `F`: `T9 = 290`.
8. `P`, `T` or `K` with the database loci flag set: `T9` becomes 500 at the most.
9. `K` before `|`: `T10, T11 = 1772, 2240`.
10. If `T0` or `T2` is above 0: `T12` becomes the voice's F4 limit at the most (section 6), and then `T10`, `T11` and `T12` in order become at least the target of the track before plus 200.

### 15.3 The limits and the onsets

1. Every track but 16, and but 9 to 15 with the coarticulation flag set (section 11.2): `forward` becomes the transition limit (section 7) at the most, `back` becomes `forward`, and `frames` becomes 60 at the most.
2. Tracks 9 to 17, or only 16 and 17 for a phoneme in class vowel with the coarticulation flag set, and not 10 to 15 for a phoneme in class vowel after `N`: `onset` and `back target` become the sum of the Q15 products of `L` and `32766 − W`, and of `T` and `W`.
3. `W` after `T`, `D` or `q`: tracks 10 to 12 get `onset` and `back target` 1200, 2050 and 2500, and track 12 gets `forward` 3.

### 15.4 By phoneme

- `C` whose node after is `s`, with the pause the node after that: `T10, T11 = 1844, 2528`. Then as `s`.
- `s`, and `C`, after `C`: with the pause the node after it, `T10, T11 = 1844, 2528`, `T3, T4 = 60, 70` and `T1 = 60`. Otherwise, if `b` is a vowel and the next phoneme is neither in class vowel nor `R`, tracks 9 to 12 get `target = L` and mode 4.
- `D` after a vowel: tracks 9 to 12 get `onset = L` and mode 6.
- `F`: after the pause, tracks 1 and 8 get `forward` 7. Before a phoneme in class vowel, `T3 = 0` and `T1 = 60`.
- `G`: before `p`, track 10's `back` becomes 7. Unless after the pause or before `Y`, tracks 10 and 11 get `back = g − g / 4`. Before a phoneme other than `o` that is front, or that isn't high, track 0's `forward` becomes 8. Before `l`, `T9..12 = 322, 1205, 2531, 3587`.
- `J` after a phoneme not in class voiced: track 0's `forward` becomes 8.
- `L`: after a vowel, track 9 gets `onset = L9` and mode 6. After `E`, `T10 = 1185`.
- `P` before `E`, with a boundary before it: `T1 = 55`, `T3 = 62` and `T6 = 0`.
- `T` before `E` or `e`: `T3` and `T4` above 3 go down by 3. Before `e`, `T3 = 50` and `T5..7 = 65, 0, 0`. Before `E`, `T1 = 62` and `T4..7 = 65, 65, 68, 68`; then with a boundary before it, `T1, T2 = 65, 60` and `T4..8 = 0, 60, 77, 77, 0`.
- `Z` before a phoneme in class vowel: tracks 9 to 12 get `forward` 3.
- `j` after `O`, whose node after is `D`: `T9, T10 = 376, 604` and `T12 = 3744`, and tracks 10 and 12 get `forward` 3.

### 15.5 The nasals

1. **After a nasal.**
   - After `M`: track 16 gets mode 4 if the phoneme isn't a consonant and either isn't in class vowel, or is a vowel whose node is stressed or that comes before a phoneme that isn't a nasal. Otherwise it gets mode 6, `onset` 380 and `forward` 4. Then a vowel gives track 9 `back target = onset` and mode 1. Otherwise, with `b` a vowel, tracks 9 to 11 get `back target = onset`, `back` 3 and the 1 in their mode. Otherwise track 9 gets `onset = L9`, and a phoneme in class sonorant gives tracks 9 and 12 the 1 in their mode, makes tracks 10 and 11 keep only the bits 1, 2 and 4 of theirs, and tracks 13 to 15 only the bits 2 and 4.
   - After `N`: track 16 gets mode 4 for a stop. Otherwise it gets mode 6 and `forward` 4, and also `onset` 400 for a phoneme not in class vowel, one whose node isn't stressed before a nasal, and `|` with `b` in class vowel. A phoneme in class sonorant that isn't a vowel: track 12 gets `back` 4 and the 1 in its mode, track 9 gets `back` 3, and tracks 10 to 15 keep only the bits 2 and 4 of their mode. A phoneme that isn't a vowel: track 9 gets the 1 in its mode and `back target = onset`. `X`: track 9 gets the 2 and the 4 in its mode. A consonant: track 10 keeps only the bits 2 and 4 of its mode.
   - After `n`: a phoneme that isn't a vowel gets `onset` 2600 on track 11 and 3600 on track 12. The original then gives track 10 `onset` 1670 if `n` is high and not front, as in section 13.2, step 5; `n` isn't high.
   - With `b` a vowel and `g` below 6: tracks 9 to 12 get the 1 in their mode and `back = g`.
   - An approximant: track 16 gets mode 6, `onset` 380 and `forward` 4.
   - A phoneme in class vowel whose node two before it is the pause, a word mark, a sentinel or in class vowel: track 10 gets mode 0.
2. **A nasal.**
   - `M` and `N`: tracks 10 and 11 get mode 4, and track 16 gets mode 5 and `back` 4.
   - `M`, and `N` after a vowel: track 12 gets `onset = L12` and mode 6.
   - `N` before `e`: track 10 gets `onset` 812, mode 6 and `forward` its `frames`. `N` before `b`: track 11 gets `onset` 2474, mode 6 and `forward` its `frames`.
   - Before a stop not in class voiced: track 9 gets mode 6 and `forward` 2.
   - Between two vowels, with `f` below 6: tracks 9 to 12 get `target = L` and mode 4.
   - `N` after the pause or a phoneme in class vowel, before a phoneme in class vowel: `T10 = 876`, track 10 gets mode 4, and its earliest frame becomes its next (section 2.1).
   - `N` and `~` after a vowel or a fricative, or after a phoneme in class closure before a vowel: track 9 gets `onset = L9`, mode 6, `forward = f / 2` and `T9 = L9 − 100`, or `L9 − 50` for `f` below 5, 250 at the least.

### 15.6 The liquids

1. `R`: before `E`, track 0's `forward` becomes 5; after `X`, track 11's `back` becomes 3. Then by the previous phoneme:
   - **A vowel**: tracks 9 to 11 get `onset` `L9 − 10`, `L10 − 20` and `L11 − 40`, mode 6 and `forward = f / 2`, and track 12 gets `target = L12` and mode 4. Before a phoneme in class vowel, the liquids follow (section 14.3). Otherwise `T9..11 = L9 − 20, L10 − 40, L11 − 110` without the database loci flag. With it, `T9`, `T10` and `T11` go down by 20, 40 and 110, and become `onset − 10`, `onset − 40` and `onset − 60` where they are above their `onset`; then `T10` and `T11` in order become at least the target of the track before plus 200.
   - **`p`**: tracks 9 to 12 get `onset = L` and mode 6.
   - **A consonant other than `s`, or `l`, `n` or the pause**: the liquids (section 14.3).
   - **Another phoneme in class closure** (`m`): tracks 9 to 12 get `onset = L`, mode 6 and `forward = f`. Before a vowel, track 9 gets `target = L9` and mode 4, and after a phoneme not in class voiced so does track 12 with `L12`.
   - **Another fricative** (`s`): tracks 9 to 12 get mode 7 and `back` 3. Before a vowel, after a phoneme not in class voiced, track 12 gets `target = L12` and mode 4.
   - **The others** (`Y`): track 9 gets mode 6, `onset = L9` and `forward = f`; tracks 10 to 12 get mode 4.

   Then, after `T` before a phoneme in class vowel whose node isn't stressed, unless the node before is a word mark: tracks 9 to 12 get `target = L` and mode 4.
2. `L` whose node before is in class stop: the liquids.
3. `Y` whose node after is in class vowel, after a phoneme in class vowel that isn't front-ending: the liquids.

### 15.7 The fricatives and the stops

1. A fricative after a phoneme in class vowel: track 0's `back` becomes 5. A dental or labial one gives tracks 10 and 11 `back` 7 and `forward` 2, and track 0 `forward` 4; an alveolar one gives tracks 10 and 11 `back` 6 and `forward` 2. `S` after a vowel: tracks 9 to 12 get `onset = L`. `F` between vowels: tracks 9 to 12 get `onset = L` and `forward = f`, and `T4..8` become 60.
2. Otherwise, after a fricative: for a phoneme in class vowel without the coarticulation flag, after an alveolar fricative that is neither dental nor labial, tracks 3 to 7 get `forward` 7; after a dental or labial one in class voiced, track 0 gets `forward` 5; and after `s`, track 1 gets `forward` 5. Then tracks 9 to 12 get `back target = onset` and `back = g − g / 4`, or `back = g` after `s`, and for `R` after `z` with `J` the phoneme before the previous one or after `s` with `C` that phoneme.
3. `F`: before `4`, `k` or `U`, `T4, T5 = 35, 35` and `T8 = 65`. Before a phoneme in class vowel that is high and front, `T8 = 60`. After `T` before `U`, `T8 = 55`.
4. After a stop, without the coarticulation flag or for a phoneme not in class vowel:
   - `B` and `P`: tracks 10 and 11 lose the 1 of their mode, and track 0 gets `forward` 2. A phoneme in class vowel after `B`, and `o` after `P`, give tracks 9 to 11 `forward` 3.
   - `D`: a front phoneme, `o`, `u` or `b` gives tracks 10 and 11 `forward` 5; a front one also gives them `back` 0, and `o`, `u` and `b` give track 0 `forward` 4. One of place 0 also gets `onset` 1924 on track 10.
   - `G`: track 0 gets `forward` 5 and tracks 10 and 11 `back` 0. A phoneme of place 2 or 3 gives tracks 9 to 12 `forward` 5, the others tracks 10 and 11 `forward` 7. `o` and `E` give track 0 `forward` 4.
   - `K`: track 0 gets `forward` 3, and `u` gives track 11 `forward` 6.
   - `T`: a front and high phoneme of place 0 or 1 other than `E` gets `onset` and `back target` 2700 on track 11.
5. After `R`: a vowel, with `b` a vowel, a fricative or in class closure, gives track 12 mode 1 and `back = g + 1`. A phoneme in class vowel whose node isn't stressed, with `b` being `T`, gives tracks 9 to 12 mode 1 and `back = g + 1`.

### 15.8 The onsets

1. A phoneme in class closure that isn't a nasal: track 9 gets `onset` and `back target` `L9`, mode 6 and `forward` its `frames`, less the burst frames (section 4.2) unless they are below 1 or 127. After a vowel, unless the node before is a word mark and the vowel is front-ending, tracks 10 to 12 get `back target` and `onset` `L`, mode 6 and `forward` their `frames`. After `j` with `b` a vowel, tracks 9 to 12 get the same.
2. A locus consonant other than `F` before a phoneme in class vowel or `R`: tracks 9 to 12 get `back target` and `onset` `L`, mode 6 and `forward` their `frames`.
3. After `M`, with the phoneme before the previous one in class vowel: track 9 gets `back = g − g / 2 + 1`, and mode 20 and `forward` its `frames − 1`; but a phoneme in class vowel with the coarticulation flag set gets mode 16 and `forward` 0.
4. `X` whose node before is in class vowel, with a word mark after it and a node in class vowel after that: `T9, T10 = 296, 1548`, tracks 9 and 10 get `forward` 3, and `T8 = 55`.
5. `Y` between nodes in class vowel: on track 9, `Ramp(next, n + 1, l, t)` (section 2.5), with `n` the segment's `frames` and `l` and `t` the bytes of `L9` and `T9` (section 2.2). Track 9 gets mode 0.

### 15.9 The pitch

On track 17:
1. `forward` becomes the segment's `frames`, 7 at the least and the transition limit at the most.
2. A phoneme in class vowel whose node is stressed, after a consonant: `onset` and `back target` become `L17`.
3. `back` becomes 0; for a syllabic phoneme after a phoneme in class voiced, both nodes stressed and without flag bit 0x40, it becomes `g`, the ring's lookahead ([synthesizer.md](synthesizer.md) §5.1) at the most.
4. After a vowel: tracks 13 to 15 get `back target = onset` and `back = g − g / 4`.
5. **A glottal stop**, a current node with flag bit 0x40: if `L17` is above 0, `PullBack(next, s, s, 41)` with `s = g / 3`. Then with `o` the segment's `onset`, frames `next` to `next + 2` become the bytes (section 2.2) of 40, `o − o / 4` and `o`, the track's next frame moves on by 3, the segment's `frames` go down by 3, and `L17` becomes `o`.
6. **A stressed vowel before the pause**: for a phoneme in class vowel whose node is stressed, with the pause the node after the next one and either the next phoneme or the node after that the pause, if the segment's `target` is below its `onset`:
   1. With `f` above 10, after a phoneme in class voiced or the pause, without flag bit 0x40 on the current node, the pitch **rises**: the segment is rendered (section 2.4) with `frames` and `forward` 3, `back` 0 and `target = onset + 4`, and the track's next frame moves on by 3. Then the segment gets `frames` and `forward` its `frames` less 3, `back` 0, `onset` the target it was rendered with, and its own target back.
   2. The pitch is **held** for `h` frames: `h = 2`, but with flag bit 0x40 on the current node, the `onset` goes up by a quarter, 10 at the most, the `back target` by a quarter, and `h` is the segment's `frames / 3`. `Repeat(next, h, o)` for the byte `o` of the `onset`; the next frame moves on by `h`, the segment's `frames` go down by `h`, and `L17` becomes the `onset`.
   3. Where it rose, the pitch **falls**: the segment is rendered with `frames` and `forward` 3, `back` 0 and `target = onset − 4`, the next frame moves on by 3, and `L17` becomes that target. Then the segment gets `frames` and `forward` its `frames` less 3, `onset` that target, and its own target back.
7. A front-ending phoneme in class vowel whose node isn't stressed, before the pause: `T17` becomes 62 at the least.
8. `T` before `p` after a phoneme in class vowel, a lateral or a nasal: `back target` 40, or 20 for `g` below 10, `back = g / 2 + 1`, and the 1 in the mode.

### 15.10 The last steps

1. The pause after `s`: track 1 gets `back target` 25, `onset` 15, `back` 9 and `forward` 3.
2. If `b` is a vowel or a word mark, unless the previous phoneme is `Y` and `b` front-ending: tracks 9 to 12 get `back` at most `g`, compared as unsigned numbers.

## 16. The sources: action 14

Action 10 runs action 14 after action 13 (section 13); no rule in the data pack runs it directly. It sets how the sources go from the previous phoneme into the current one: the onsets of tracks 0 to 8 and 18 to 21, the voicing's onset, transitions and pull on the previous phoneme, and in a few contexts the frication, the aspiration, the formants and the pitch. Before the pause it ends the voicing of a voiced fricative, and at the pause it lets the voicing and the pitch fall. The notation is that of section 15: `f` is the current node's frames and `g` the previous node's, `b` is the node before the previous one as in section 13.4, step 3, and the phoneme before the previous one is the stage's (section 7).

### 16.1 Classes

Action 14 tests the classes of sections 3.2, 8.1, 9.1 and 10.1, and this one:

| Class | Name here | Phonemes |
|---|---|---|
| (5, 0x40) | fortis | `C F H K P S T s Y` |

### 16.2 The onsets

Each track but 9 to 17 gets `onset = (L + T) / 2`, a division of signed numbers that truncates toward 0. On tracks 0 to 8 the `onset` then becomes at least `T − m` and at least `L − m`, with the limit `m`:
- 0 on tracks 3 to 8;
- on tracks 0 to 2 after a phoneme not in class voiced, 10, or 20 on track 0 for a nasal after the pause;
- otherwise 0 on track 2, and on tracks 0 and 1 15 for the pause and 9 for the others.

Then every one of these tracks gets `back target = onset`.

### 16.3 The voicing after a voiced phoneme

After a phoneme in class voiced other than `R`, on track 0:
1. With `T0` 0, `onset` and `back target` become 40; otherwise `back target` becomes `T0`.
2. Unless the current phoneme is in class voiced and in class sonorant, or is the pause after `z`: `back target` goes down by 6, and `onset` becomes the same, but for `V` before a phoneme in class vowel.
3. `back` becomes `g / 3`, 2 at the least, after a vowel, and `g / 4` after the others.
4. For a phoneme in class sonorant and in class vowel after a stop: `onset` becomes 60, or 56 for `E` after `G` or `D`, for `u` and `b`, and after `G` for a phoneme that is neither high nor front, other than `p` and `o`. A high phoneme that isn't front after `B` also gives tracks 13 and 14 `onset` and `back target` 75.

### 16.4 The voicing of a voiced phoneme

For a phoneme in class voiced, on track 0:
1. If it is neither a stop nor in class silence, and the next phoneme is neither in class closure nor the pause: mode 7 if the previous phoneme isn't the pause and either isn't a stop, is in class silence, or the current phoneme is in class closure; otherwise mode 6, but `L` after `P` keeps its mode. `forward` becomes `f / 4` if the current node is stressed, else `f / 2`, and `back` becomes `g / 4`.
2. After `B`: mode 4 if `L0` is 0; otherwise the 2 in the mode, `forward = f / 2` and `onset = L0`.
3. A fricative before a fricative not in class voiced: `T0 = 0`, mode 6 and `forward = f / 3`.
4. After `D`, `G` or `J` with the phoneme before the previous one the pause, and after `G` with that phoneme a consonant not in class voiced: mode 6.

### 16.5 By phoneme

- `D`: before `o`, track 0 gets `onset` 0. If its node isn't stressed, between two phonemes in class sonorant, track 8 gets `onset` and `back target` 0.
- `E` after `R`: track 0 gets `onset` 57.
- `F` after the pause: track 8 gets `onset` and `back target` 15.
- `H` before a phoneme of place 0 or 1: track 2 gets `onset` 10 and `forward` 5.
- `K`, `P` and `T` after a phoneme that isn't a vowel: track 0 gets `onset` 0 and mode 4.
- `L` after `F`: `T0 = 57`, and track 0 gets mode 5, `back target` 57 and 2 frames more, its next frame moving back by 2.
- `R`: after `z`, track 0 gets `onset` and `back target` `L0`. After a fricative not in class voiced, track 0's `back` goes up by 4 and its `back target` by 10. After `X`, track 0 gets `onset` and `back target` 57, and track 1 `onset` 0.
- `S`: after the pause, track 1 gets mode 6 and `onset` 16. Track 1 gets `back = g / 4`.
- `W` before `E` after the pause: track 11 gets `onset` 2150 and track 15 `onset` 250.
- `X`: before `R`, track 1 gets `onset` and `back target` 40, and `T3..7 = 0, 0, 0, 72, 80`. After a phoneme in class vowel, track 9 gets `onset = L9` and mode 6.
- `Z`: after a phoneme in class voiced, track 0 gets mode 7 and `forward` 6, and after one in class vowel also `onset` and `back target` 52, and `back = g / 4` unless the next phoneme is the pause. Unless the next phoneme is the pause, track 1 gets `onset` and `back target` 10 and track 0 42, and between two phonemes not in class vowel, track 17 gets `back` and `forward` 4.
- `o` after `B`: track 10's `onset` goes up by 100.
- `p`: after `D`, `T18, T19 = 10, 15`; after `P`, `T` or `K`, track 1 gets `onset = L1`, `forward` its `frames` and `T1 = 42`; after `G` or `B`, track 0 gets `onset` 50, mode 6 and `T0 = 50`.
- `q`: track 0 gets mode 4 and `T0 = 0`.
- `s` after `C` before a phoneme in class vowel, and `s` before `@`: tracks 4 to 7 get `onset` and `back target` `L`. Then the original gives track 1 `onset` 35, `back target` 25, `forward` 6, `back` 3, mode 7 and `T1 = 66` if the next phoneme is the pause, which it can't be there.
- `t`: track 0 gets mode 4.
- `x` after `q`, or after a phoneme not in class voiced that is a fricative or in class closure: track 0 gets mode 4 and `T0 = 0`.
- `y` after `P`: `T3..7` become `L3..7`.
- `~` after a front phoneme: track 10 gets `back target` 2100.

Then:
- A phoneme in class vowel whose node has flag bit 0x40: track 0 gets `onset` 42, `forward` 4 and mode 6.
- After `Z`: track 18 gets `onset = L18` and `forward = f`, 5 at the most.

### 16.6 A voiced fricative before the pause

Before the pause:
1. A phoneme in class voiced that is a fricative, with the node after the pause neither an approximant nor `M` or `N` (or with the pause in class mark, which it isn't):
   - `Z` after `B`, `D` or `G`: track 0 gets mode 4.
   - After a phoneme that is neither a nasal nor a fricative, nor `j` or `l`, with `f` above 6: track 0's segment is rendered (section 2.4) with `onset` 60, `T0 = 50`, `frames` 4 and mode 6. `L0` becomes 50, the track's next frame moves on by 4, and the segment gets its own `frames` less 4 and mode 4.
   - Otherwise: track 0 gets `back target` 50, `back` 4 and mode 5.

   Then `T0 = 0`.
2. `Z` and `V`: with `h = f / 2` and `l` the low byte of `L0`, on track 0 `Fill(next, h, h, l, 46)` and `Fill(next + h, f − h, f − h, 0, 0)` (section 2.5). Track 0 gets mode 0, so that rendering it writes nothing (section 2.4); its next frame stays.

### 16.7 The limits

Tracks 17 and 0 get `back` at most `next − earliest` (section 2.1).

### 16.8 The pause

For the pause:
1. After a phoneme in class voiced other than `Z`, unless the node after the current one is `&`: track 17 gets `back target` 58 and track 0 `back target` 48, both get `back = g − g / 3` and the 1 in their mode. Then if the next phoneme is the pause too, after a nasal, an approximant, a phoneme in class vowel other than `p`, or `p` with `b` an approximant:
   1. Track 17 gets `back target` 155.
   2. Track 0 keeps only the bits 1 and 4 of its mode, gets `onset` and `back target` 38 after a phoneme in class vowel and 45 after the others, and `T0 = 0`. Its next frame moves on by 3, its `back` goes up by 3, its `frames` go down by 3, and it gets `forward` 4.
   3. After `N`: with `b` a vowel, track 0 gets `back target` 42 and `back` 8. Track 9 gets `onset = L9`, `T9 = L9 − 10` and mode 6.
   4. After `p`, track 17 gets `back = g + 2` and track 1 mode 4. After the others, track 17 gets `back = g / 6`, 2 at the least.
2. By the previous phoneme:
   - `V`: tracks 0 and 1 get `forward` 2, and track 0 gets `back = g − 4`, `back target` 0 and `onset` 5.
   - `Z`: tracks 0 and 1 get `forward` 2. Track 1 gets `onset` and `back target` 20 and `back = g / 3`, track 2 gets `back = g / 3`, and track 0 `onset` and `back target` 0.
   - `n` before the pause: track 10 gets `back target` 892 and `back` 9, track 0 `back target = L0` and `back` 8, and both the 1 in their mode.
   - `p`: track 1 gets mode 4, `onset` 0 and `T1 = 0`. With `b` being `P`, `T` or `K`, track 0 gets mode 4, `back target` 0 and `T0 = 0`.
   - `z`: tracks 0 and 1 get `forward` 2, and track 1 gets `back = g / 2`.
3. With `L0` 0, track 0 gets mode 4.

### 16.9 Other contexts

1. After `l` or `j`, a phoneme that is neither in class vowel nor `R` or `Y`: tracks 9 to 12 get `onset = L` and mode 6.
2. After a stop with `b` being `M` or `N`: track 16 gets `back` 2, mode 5 and `back target` 250.
3. `T` and `q` after `N` whose node before isn't stressed, with `b` in class vowel and a word mark the node after: before a phoneme in class sonorant, or in class vowel with the node after the next one stressed, track 0 gets `back target` 40, `back = g` and the 1 in its mode.
4. `q` after a phoneme in class vowel, before a phoneme not in class voiced or `x`: track 17 gets the 1 in its mode, `back` 4 and `back target` 40, or 20 if `L17` is above 99, or 0 if it is above 149.
5. After `s`: track 1 gets mode 4.
6. `V` after a fortis phoneme in class closure, before a phoneme in class vowel: track 0 gets `forward` 4, and track 1 mode 6 and `forward` 4.
7. `R` after `B` before a phoneme in class vowel: track 0 gets mode 4.
8. `Z` after a nasal before a phoneme in class vowel: track 0 gets mode 4 and `T0 = 53`, track 1 `forward` 2 and `T1 = 64`, and `T2 = 0`, `T4 = 0`, `T5..7 = 74, 74, 74`, `T18, T19 = 10, 14`, `T10, T11 = 1844, 2496` and `T13 = 70`.
9. `F` after `Z`, with the node before `Z` being `E` and the node after `F` being `k`: `T1 = 55`.

## 17. The stage's step

The scheduler runs the stage's step, asks whether the ring has room (section 2.7) for the frames the stage waits for, and resets the stage with the pipeline (section 7; [pipeline.md](pipeline.md) §6 and §7). A step renders the phoneme at the cursor, or writes the next frames of a generated segment, and reports **finished** (it had nothing to do), **waiting** (for room in the ring) or **working**. Before it renders a phoneme, it finds the next one and puts in the pauses that what comes next asks for. The previous, current and next phonemes are the cursor's done, scan and first nodes (section 3.4), and a node's **frames** are its value.

### 17.1 State and classes

Besides the state of section 7, the step keeps:
- **ahead**, what it found after the current phoneme (section 17.3): 0 a phoneme, 1 an end command, 2 a reset, 3 a generated segment, 4 nothing;
- **start**, how far it is in starting the synthesizer after a reset: 1 after the reset, 2 once it has rendered a phoneme other than the pause, 3 once it has let the synthesizer go on (section 17.2, step 12), and 5 after a generated segment. The original's code also has 0, which no reset leaves, and 4, which only follows 0;
- **at reset**, set when it stops at the pause before a reset;
- **wanted**, the frames it waits for room for. The scheduler checks the room for them ([pipeline.md](pipeline.md) §6);
- the **generated segment** it writes (section 17.5): its symbol, the frames still to write, whether it has written none yet, its frame of 22 bytes, and a tone's frequency in 10 Hz units.

The step **releases** the synthesizer: it sets the ring's `end` to −1 and clears its `done` ([synthesizer.md](synthesizer.md) §5.1), and start 4 becomes 5. The stage is `written − position − lookahead` frames **ahead** of the synthesizer, with the ring's counters.

It tests one more class:

| Class | Name here | Symbols |
|---|---|---|
| (4, 0x08) | punctuation | `! , . ? \ ]` |

### 17.2 The step

1. Begin the cursor ([pipeline.md](pipeline.md) §3.2). If its done, scan and first are all empty, go to step 16.
2. **Speed.** If the cursor's flags (the `N` command) haven't bit 0x2000 and its rate index is below 25, the transition limit becomes 30, the margin 15 and the ring's `lookahead` 12; otherwise 4, 3 and 2.
3. If `end` isn't −1, the stage is 8 frames ahead of the synthesizer at most, and start is 0, start becomes 1.
4. If ahead is 2 and the current node is a final pause (section 17.4), set at reset and go to step 16: the stage stops there until the reset.
5. **A generated segment.** If there is a current node and either ahead is 3 or the node isn't a segment: ahead becomes 3, and if the node isn't a segment, or is a final pause, write the generated segment (section 17.5) and stop.
6. **Room.** If there is a current node, wanted becomes its frames.
   1. Without room for them (section 2.7), release the synthesizer, finish the cursor ([pipeline.md](pipeline.md) §3.3) and report working if it handed nodes on, else waiting.
   2. If the stage is more than 64 frames ahead of the synthesizer, release it and clear prosody's flag that ends a phrase sooner.
   3. If the cursor's phrase setting (the `P` command) is 0, release the synthesizer.
7. **Look ahead** (section 17.3). If it returns no node, go to step 16. The node it returns becomes the next one.
8. **The phoneme after the next** becomes the symbol of the first segment after the next node, past nodes of type 3 (word marks and punctuation), or a space if a node of another type comes first. If the window ends first, it stays as it was.
9. If the current phoneme isn't the pause, start 0 becomes 4 and start 1 becomes 2.
10. Render the current phoneme (section 6) and advance (section 2.6). For a phoneme not in class voiced, track 17's last value `L17` becomes the unvoiced pitch. For a whispered node (its character, byte 0x1a, which prosody sets, is 1), both the unvoiced pitch and `L17` become 100.
11. If the previous node is a lead-in pause (section 17.4), free it, going forward ([pipeline.md](pipeline.md) §2.3).
12. If start is 2 and the stage is more than 8 frames ahead of the synthesizer, `end` becomes −1, `done` is cleared and start becomes 3.
13. **Move on.** The phoneme before the previous one becomes the previous node's symbol, the previous node becomes the current one, the current node the one after it, and the next node the one after it.
14. **The end.** If the new current node is a final pause, `end` becomes `written − lookahead` and `done` is cleared: the synthesizer stops before the frames the stage may still change. Then if neither a segment nor a generated segment comes after the pause in the window, free the pause, going back, empty the cursor's done, scan and first, clear the ring's marker bit before `written` and set the one before `end`.
15. Rebase (section 2.7).
16. Finish the cursor, and report working if it handed nodes on or step 11 freed a pause, else finished.

### 17.3 Looking ahead

1. **A lead-in pause.** If there is a current node, and the previous node is the current one (as at a window's first segment) or isn't a segment, put a lead-in pause before the current node (section 17.4). It becomes the previous node.
2. Go through the nodes after the current one, counting those that aren't segments:
   - a **segment**: if ahead is 3 and the segment is a final pause, return it, ahead unchanged. Otherwise ahead becomes 0, and the search ends;
   - an **end command** `C`, which ends every text, after its last bookmark: ahead becomes 1, and the search ends;
   - a **reset**: the `x` command, or a bookmark `i` once the escape parser has parsed a reset. The node gets flag bit 0x20, so that it resets the stages when the output stage reaches it ([pipeline.md](pipeline.md) §4.1). Ahead becomes 2, and the search ends;
   - a **generated segment**: ahead becomes 3, and the search ends;
   - a node of type 3 in class punctuation, other than `]`: release the synthesizer.
3. If 10 nodes or more were counted, ahead becomes 4.
4. Return:
   - with ahead 0, or without a node where the search ended and ahead other than 4: that node, or none. `end` becomes −1;
   - with ahead 1 or 2: the first of a pair of pauses put after the node where the search ended (section 17.4);
   - with ahead 4: the first of a pair of pauses put after the window's last node;
   - with ahead 3: the generated segment.

### 17.4 Pauses

The step puts in pauses: segment nodes taken from the free list ([pipeline.md](pipeline.md) §2.3) with the pause's symbol, and flag bits 3 and 4 for their kind. A pause's second value, its pitch, is the current node's, or 50 without one.

- A **lead-in** pause (bits 3 and 4 = 1) has 4 frames. It is the previous phoneme of the phoneme after it: it is never rendered, and step 11 of section 17.2 frees it.
- A **pair** of pauses goes after a node. The first, a **break** (2), has 4 frames before an end command, and `lookahead + margin + 10` otherwise, so that the synthesizer speaks every frame before it. The second, the **final** pause (3), has 4 frames: when the stage gets to it, the synthesizer stops (section 17.2, step 14). Without a current node, the break becomes the current node and the final pause is returned. Putting in a pair sets `end` to −1 and clears `done`.

### 17.5 Generated segments

A **generated segment** is a node of type 5 ([pipeline.md](pipeline.md) §2.1): one frame for a number of frames, the low byte of its value. Its symbol says which frame: `g` the track levels, which the `l` command sets ([pipeline.md](pipeline.md) §4), `s` the initial frame (silence), and `t` the data pack's `segmentFrames` of the **kind** in flag bits 3 to 6 of its node: `(bits 3–4) + 4 · (bit 5) + 8 · (bit 6)`. The original's table has 10 kinds; kinds 10 to 15 read the bytes after it, which the data pack keeps.

**Writing it.** The step:
1. If the last generated segment is written: if the current node is a segment (a final pause), free it, going forward, and take the node after it. Note the node's symbol and frames, reset the context (section 7), set ahead to 3 and note that none of the segment's frames is written.
2. With frames left and `room = position − margin − furthest + 256` other than 0: **fill** (below) and keep the frames left. On the segment's first fill, `end` becomes −1 and `done` is cleared.
3. With no frames left, the segment is written.
4. Rebase (section 2.7), and set the flag that makes the next phoneme keep the earlier frames (section 6). Wanted becomes 1, or 0 when no frames are left.
5. With no frames left, the previous node becomes the current one, and the current and next nodes the one after it. Ahead becomes 0.
6. Start becomes 5. Finish the cursor, and report working if it handed nodes on, else waiting.

**Filling.** On the segment's first fill, the tone's frequency becomes 0 and the frame is made: for `g` the track levels, for `s` the initial frame ([pipeline.md](pipeline.md) §7), and for `t` the bytes of its kind, in which 255 is filled in, track by track:
- if byte 9 of the kind is 255, the segment is a **tone**: its frequency is the node's second value, and tracks 13 to 15 become 45 while the **wide** flag is set, which starts set;
- a 255 on tracks 9 to 11 becomes the tone's frequency, and the wide flag becomes whether the frequency is above 180;
- a 255 on track 17 becomes the node's second value;
- a 255 on tracks 0 and 1 becomes `60 − a`, with the cursor's attenuation `a`, less `toneLevels[f >> 4]` if the tone's frequency `f` isn't 0, and 1 at the least;
- the other bytes stay as they are, 255 too.

Then from `written` on, for as many frames as are left, `room` and 53 allow, each slot gets the frame, and while the tone's frequency isn't 0, the slot before it gets its tone bit. Every track's next frame becomes the frame after the last one written and its earliest frame that less `lookahead`, and `written` and `furthest` become it too. The fill returns the frames still left.

## 18. What the recorded runs don't reach

These follow the original's code, but no recorded run gets there:
- Segments of modes 2, 9, 17, 21 or above 21. Mode 3 came up 4 times.
- A negative `frames`, `back target`, `onset` or `target` in a segment.
- A Q15 product where dividing by 32767 instead of 32768 changes the result, as for `151 · 217 · 128`.
- `AddBack` taking a frame other than the first below 0, or a first frame plus `change` of exactly −1.
- A ring without room, so step 6.1 of section 17.2: the generator was at most 115 frames ahead of the synthesizer, margin included, because the synthesizer renders after every step of the generator and is released once the generator is more than 64 frames ahead. Only a held synthesizer ([synthesizer.md](synthesizer.md) §5.2) could leave no room, and the original never finished speaking a text that held it that long.
- 112 of the 468 rules: 356 applied. No rule of set 43 (`z` after a closure) applied; set 53 is empty and the matrix never gives it.
- A stop burst's exceptions: `t`, `J` or `C` before a pause (the stages before split `J` and `C` into `J z` and `C s`), `K` before `t` or `Q` (section 4.2).
- An F4 target above the voice's limit (section 6): no target in the data pack is.
- Some of the per-phoneme rules (section 8.3), as the stages before don't produce their contexts from English text: `D` after the pause before a nasal, before `n`, or after `S` before `R` (the stages before turn `D R` into `J z R`); `G` before `l` (they write `j` after `G`); `R` after `G` or `K` before a phoneme that isn't a vowel, `R` before `l` and `l` after `R`; `z` after `J` before `y` with the pause before `J` (a word mark always comes between); `M` or `m` after `Y` or `R` before a phoneme that isn't a vowel. Nor does the group of `H` or `d`, which only come before vowels. Where a rule tests for a vowel and another for class vowel, they differ only for `p`, which comes only before a pause, after a stop or `~`, but anywhere in phonetic input: no recorded `p` came after or before `m Z j s x z`, nor after `R`; one came after `M`, in phonetic input.
- Some contexts of the actions (section 9), for the same reason: a rounded phoneme after `Y` and a phoneme after `R`, neither a vowel, in the voice onset; `n` after `D` (the stages before write no `D n`), `n` after `T` and `l` after `K` (they write `q n` and `K L`), `H` before `g` (`H` only starts a phrase), `D` before `|` after a phoneme not in class voiced, and `s` before `|` (`|` only comes after `t` or `d` in a word); `R` after `P` before a stressed phoneme that isn't a vowel; a vowel with a stressed node right after another, no word mark between (the stages before put `W` or `Y` between most vowels); a syllabic `m`, `l` or `n` after a phoneme neither in class voiced nor a stop; `p` in step 4 of the voice onset, or after a vowel; `r`, `g` or `c` after a `K` whose node isn't stressed, and a vowel whose node isn't stressed after a `P` or `T` whose node is (the stages before mark a stressed syllable's consonant and vowel together). Action 3 never runs after `C`, as the stages before always write `s` after it and the rules don't run action 3 on `s`: so the burst's 60 isn't reached either. Nor does a previous node with flag bit 0x40 outside class voiced: the stages before set the bit only on vowels, approximants and voiced consonants.
- Some contexts of actions 7 and 9 (section 10): `D` before `R`, `L` or `Y` (the stages before turn `D R` into `J z R`; no recorded text gave `D L` or `D Y`), and so a stressed `D` after `S` before `R`; `B` before `p` or `Z` after `S`; `T` before `n` (they write `q n`); `D` with a word mark after it before `x` after `N`; `T` before `c` inside a word, or with the pause the node before it (a word's first phoneme has its word mark before it); `L` before `s` (`L` only comes before a vowel). Nor did any recorded text have `C s` before `c` or `u`, `C s` before `5` at the start of a word, or `s` after `K` before `g`.
- Values that make a step change nothing. In the voice onset: the aspiration is always 51 when the voice onset is written, so taking 3 off never goes below 0; the only alveolar stop the writer comes to is `T`, whose aspiration step 3.3 replaces, so the 9 of step 3.2 never shows; track 1's last value is never below 2 when a burst follows `P`, `T` or `K`; `L17` already is the unvoiced pitch when the burst sets it; the voice onset is never above track 0's or track 2's frames in step 4; step 5's `v = 1` always holds for a vowel after a phoneme that isn't a stop, as every vowel is syllabic; and the track 8 target of `L` after `P` is already 0. In action 6: the test for `@` before `N`, as `s` goes down by 2 before `@` anyway; an affricate after a phoneme in class voiced, as `J` gets `T0 = 50` in step 3.4 anyway and `C`'s `T0` is already 0; `G` after `S` in step 3.4, as `G` keeps step 3.3's `T0 = 0`; and steps 3.2 and 3.3 but for `T0 = 0`, as step 3.3 or 3.4 then sets every voiced stop's `T0` to 0, 43 or 50, whatever the text. In action 7: `T1` never went below 1 in step 3, and no merged `F` had a target of 3 or 4. In action 9: `b1` was never below 0 in step 3 of the shared steps; burst frames of `f` or more came only with `f` of 3 or less, and the burst frames never were `f`; a `B` after the pause before a phoneme in class vowel always had a closure of more than 3 frames, and the other stops after the pause, like `G` after a consonant not in class voiced, already have `T0 = 0` (action 6), so their closure writes zeros on track 0 and step 6 of writing it changes nothing; and track 0's mode never had bit 0x01 for `B`, `D` or `G` after `Z` with `B`, `D` or `G` the node before it.
- Steps that change nothing whatever the text: `T`'s closure on track 0 (section 10.3, writing the closure, step 2), as `T`'s `b0` is 127 in both burst rows and nothing changes it; step 1 of `T`'s burst levels but for `T8 = 0`, as step 2 sets `b1` before every phoneme in class vowel; `P`'s `b1 = 60` before `S`, which the next step replaces; action 7's `T19 = 14` for `V` after the pause, as every rule of `V` sets it before its actions; the test for the pause in step 1 of action 9's shared steps, as the pause isn't in class voiced; the test for an r-coloured phoneme in step 18 of action 7, as they are all in class vowel; and the count of `c + 2` frames in the closure's `AddBack`, as shape `c + 2` has fewer weights after its first.
- Some contexts of the vowel database (section 11). In the contexts (section 11.3): `e`, `i`, `u`, `|`, `4` or `5` right before another vowel, and a current `4`, `5` or `v` right before one (the stages before put `Y`, `W` or `R` between most vowels, as in `here is`); step 3 for `5`, with `A'` being `H`, `W`, `Y` or `t`, or for `I` whose node isn't stressed; step 4 for `@`, `|` or `5`; `E` after `M` with the pause next and `M` after the node after it (step 5); step 7 after `X`, or after `V` before `a`, and for a vowel that isn't front-ending, whose one recorded case gave the same record either way (a word mark comes right before a vowel only in hyphenated words and before short words such as `in` or `over`, mostly after `S` or `Z`); a reduced vowel before `q` before the pause (step 10; after a reduced vowel `q` only came before `L`). In the lookup (section 11.4): a variant `D` or `t` that finds a different record than the lookups after it, a record found in the second pass or by a lookup after one was found, and the lateral of `j`. An alternative named by the node after the next one that applies: 19 such records came up, and none applied. On the segments (section 11.7): `R` on track 12 with `N` one of `u 4 c g`, and a syllabic `m` before a vowel on tracks 10 to 12.
- Values that make a step of the vowel database change nothing: the times' `+ 50` never rounded a time across a boundary; no count the writers take 20 at the most of was above 20; a held start's `L` and `P1` were never exactly 200 apart; after `j`, `L9` never was exactly `S + 100`, and `S − 100` always was below 250; every symbol looked up had a context; and a full vowel's track always had a write right after its first frame.
- Steps of the vowel database that change nothing whatever the text: step 5 of writing a track (section 11.5), as with the data pack's trajectories the times are in order for any vowel length; the test for `u` in the held start, as `u` is one of the database's vowels; the test for 650 rather than above it on track 9, as F1 values are multiples of 4; a `Fill` or `Ramp` of 0 frames, which writes nothing; the shape of a `Fill` from `s` to `s`; a frame more for the first `Fill` of a reduced vowel and for the bandwidths' first `Fill`, which the next write covers; the test that a variant's context is 0 or more rather than above 0, as no variant has context 0; and the space for the node before a word mark before the previous phoneme, which always exists.
- Values that make a step of section 12 change nothing: the F2 and F3 targets and offglides in step 1 of section 12.2 were always multiples of 4, so its divisions by 4 never round; after `M`, track 9's mode already had the 1 that section 12.4 gives it; and where step 3.1 of section 12.5 gives a segment mode 7 and nothing changes it after (a start below 1 frame, track 10 after `M`), it already had it.
- Steps of section 12 that change nothing whatever the text: step 1 of section 12.5, as track 9's segment has the node's frames, so that the vowel transition stays as it is; step 3.3.1 of section 12.5, as tracks 9 to 15 have the node's frames and every vowel's `offset` is below 32767, so that `v` is always below them; and the original's test of the coarticulation flag after section 12.3's steps, as the flag is 0 there.
- Values that make a step of action 10 change nothing (section 13): track 12's mode already had the 1 that `4` after `M` or `m` gives it (section 13.2, step 5); in step 3 of section 13.4, tracks 9 to 12 never had a `back` above the previous node's frames but for `Y` after a front-ending vowel, which the step leaves out, so that taking the node before a mark changed nothing either; in step 6, after a phoneme in class vowel, the bandwidths' `back target` already was their `onset`, and none had mode 4; `T9` was never exactly 701 in step 9, nor `T1` exactly 1 in step 3 of section 13.5.
- `R` after `D` in action 10's loci (section 13.2, step 4.7): the stages before turn `D R` into `J z R`, from phonetic input too.
- Steps of action 10 no text reaches: `L` with the coarticulation flag set after a phoneme that isn't a vowel (section 13.4, step 2), as the flag is set only by a vowel's action 4, every rule ends with action 10, which clears it for a phoneme not in class vowel, and `p`'s action 4 clears it. The original also has steps whose conditions can't hold together, which section 13 leaves out: in step 4.7 of section 13.2, after a labial, a front phoneme that isn't a vowel would get `W10, W11 = 16400, 6560`; in step 4.8, a phoneme with an offglide that isn't a vowel would get mode 6 on tracks 12 to 16 and mode 2 on tracks 9 to 11; in step 6 of section 13.4, after a phoneme in class vowel, `j` before a vowel after a phoneme not in class vowel would get `T9 = L9`. The original there also sets the bandwidths' `back` to `g / 4` before both cases set it again.
- Contexts of action 13's helpers (section 14) that no recorded text gave: an alternative named by the node after the next one, with that node after a word mark (section 14.1, step 1); `H` or `t` after a front-ending vowel and a word mark (step 3); `R` after a word mark before `3`, or with the pause as the previous phoneme (section 14.3, step 2); and a consonant before a word mark and `R` that is `S`, `Z` or `R` (section 14.2): the stages before put a pause after `S` and `Z` there. A point of kind 2 below 200 (writing a track): only `Y`'s rows have kind 2, they take at most 60 off, and the lower of F1's last value and target was 288 or more at every recorded `Y`.
- Steps of action 13's helpers no text reaches. Action 13 doesn't call them for `n` (section 14.1, step 1), for `s` before `R`, so not for `s` after `C` (sections 14.2 and 14.3, step 4), nor for `P`, `s`, `j`, `l` or `~` before a word mark and `R` (section 14.2). The stages before put `R` after `g`, `4` and `5` before a vowel (section 14.1, step 2), drop an `R` after `k`, `c`, `r` and `3` and write `Y 3` for one after `y` (section 14.3, step 1), and write `@ j` for a syllabic `l` after `R` (sections 14.2 and 14.3), from phonetic input too. Nor do two word marks come in a row before `R`. The voice onset in writing a track (steps 1 and 2) is always 127: `R`, `L` and `Y` are in class voiced, so loading their targets makes it 127 (section 4.1), action 3 keeps it 127 for a phoneme not in class vowel (section 9.5), and no rule sets it.
- Values that make a step of action 13's helpers change nothing: in step 5 of section 14.3, a node of exactly 8 frames gives 2 either way; the `+ 50` of `t`, as with the data pack's durations `d · f` never ends in 49, so that `+ 51` would give the same `t`; the shape of the `Fill` from `s` to `s`, and a frame more of it, which the next ramp writes over; no ramp of writing a track was longer than 20 frames, and a ramp of 0 frames writes nothing.
- Contexts of action 13 (section 15) that no recorded text gave: a segment of more than 60 frames (section 15.3, step 1); in step 10 of section 15.2, `T0` or `T2` of exactly 1, and `T12` above the voice's F4 limit or below `T11 + 200`; `T3` or `T4` of 3 or 4 for `T` before `E` or `e` (section 15.4); `X`, `x` or `C` after `U` or `Y` (section 15.2, step 2; after `E`, sections 15.7 and 15.8 set track 10's `forward` again); `D` after a front-ending vowel with a word mark before it, not before a phoneme in class vowel or `R` (section 15.4; otherwise section 15.8 sets tracks 9 to 12 again); a vowel after `M` without the coarticulation flag, with a phoneme not in class vowel before the `M` (section 15.5; otherwise track 9 already had mode 1 and its `back target` was its `onset`, or section 15.8, step 3 sets its mode again); a sentinel two nodes before a phoneme in class vowel after a nasal (section 15.5); `R` after `s` before a phoneme that isn't a vowel (section 15.6); `R` after a fricative other than `s` with `C` the phoneme before the previous one (section 15.7, step 2); `E` after `T` without the coarticulation flag (section 15.7, step 4); `L17` of 0 or 1 at a glottal stop (it was 50 or more); and at a stressed vowel before the pause, `f` of exactly 11, or an `onset` whose quarter is below 10 with flag bit 0x40 (the lowest was 128).
- Values that make a step of action 13 change nothing: after `N`, track 9's `back target` already was its `onset` for a phoneme that isn't a vowel, and `X` already had the 2 and the 4 in track 9's mode, or came before a phoneme in class vowel, so that section 15.8, step 2 sets the mode again; the modes of the one phoneme in class sonorant that came after `M` with `b` not a vowel (`Y`, twice) already were what section 15.5 makes them; `R` after `s` already had mode 7 on tracks 9 to 12 (section 15.6); `X`'s `T8` already was 55 in step 4 of section 15.8 (the per-phoneme rules set it, section 8.3).
- Steps of action 13 no text reaches. The stages before write `| j` or `@ j` for a syllabic `l` after `G`, so not `G` before `l` (section 15.4): a syllabic `l` stays only after `t`. They write `@ M` for a syllabic `m` before `R`, so not `R` after `m` (section 15.6), and `C s R` for `T R` inside a word and `q R` across words, so not `R` after `T` (section 15.6) nor a phoneme after `R` with `b` being `T` (section 15.7, step 5); from phonetic input too. `R` after a vowel with the database loci flag set (section 15.6): the flag needs one of the vowel database's vowels next, and before those the liquids run. The original's 1670 after `n` (section 15.5): `n` isn't high.
- Steps of action 13 that change nothing whatever the text: the 1 that track 12 gets for a phoneme in class sonorant after `N`, which the same step takes away again (section 15.5); track 11's `back` 3 for `R` after `X`, and tracks 9 to 12's `back` 3 for `R` after `s` (section 15.6), which section 15.7, step 2 sets again; `Z`'s `forward` 3 before a phoneme in class vowel (section 15.4), which section 15.8, step 2 sets again; the test for `s` after `C` for `R` in section 15.7, step 2, as `back = g` after `s` anyway; the test that the fricative before `R` isn't in class voiced (section 15.6), as only `s` comes there; the place of a front and high phoneme after `T` (section 15.7, step 4), as none has place 2, and that the previous phoneme there is a stop, as only stops have steps there; the 40 Hz of the glottal dip, as 41 gives the same byte; and in the rise of section 15.9, step 6, a fourth frame, which the hold writes over, a `back` of 1, as shapes 0 and 1 pull nothing, and the segment's `forward` and `back` after the rise, which the fall sets again.
- Contexts of action 14 (section 16) that no recorded text gave: `c` after `G` (section 16.3, step 4); a voiced fricative of exactly 6 frames before the pause, after a phoneme that is neither a nasal nor a fricative, nor `j` or `l` (section 16.6); a pause with a word mark right after it, after a phoneme in class voiced other than `Z` (section 16.8, step 1): of the voiced phonemes only `Z` came right before such a pause, in hyphenated words, and `love-`, `breathe-`, `beige-`, `bell-`, `sun-`, `bee-` and `bug-` don't get one; and `L17` of 99, 100, 149 or 150 at `q` (section 16.9, step 4; it was 92, or 154 to 336).
- Values that make a step of action 14 change nothing: track 0's mode already had the 2 after `B` (section 16.4, step 2), and already was 6 for `z` after `J` with the pause before it (step 4) and for every phoneme after `P` in step 1, so that `L` keeping its mode there changes nothing; the pause's `T0` already was 0 in step 1.2 of section 16.8; and for `n` before the pause, tracks 0 and 10 already had the 1 in their mode (section 16.8, step 2).
- Steps of action 14 no text reaches: track 1's values for `s` before the pause (section 16.5), as `s` there comes before a phoneme in class vowel or `@`.
- Steps of action 14 that change nothing whatever the text: the original's test for the pause in step 1 of section 16.4, as the pause isn't in class voiced, and its test there that a previous stop is in class silence, as no stop is; `p` left out in step 4 of section 16.3, as section 16.5 sets `p`'s `onset` after `G` again; the test for a phoneme in class vowel with the node after the next one stressed in step 3 of section 16.9, as every phoneme in class vowel is in class sonorant; the test that the pause is in class mark (section 16.6), as it isn't; `Z`'s mode 4 and `T0 = 0` after `B`, `D` or `G` before the pause (section 16.6), as step 2 gives `Z` mode 0 and `T0` becomes 0 anyway; and track 1's mode 4 after `p` in step 1.4 of section 16.8, which step 2 gives it again.
- Contexts of the stage's step (section 17) that no recorded run had: a reset ahead, and so the stop at its final pause (section 17.2, step 4, and section 17.3, step 2): a reset makes the original drop the rest of the text without reporting it done ([pipeline.md](pipeline.md) §9); a segment or a generated segment after a final pause in the window (section 17.2, step 14), so that no step began at a final pause: each pair of pauses went after the end command at a text's end, or after the window's last node; a pair of pauses without a current node, which the lookahead puts in only with ahead 4 left from the step before, and so the pause pitch of 50 (section 17.4); and the kinds 10 to 15 of a generated segment `t` (section 17.5): the escape sequences `ESC[10t` to `ESC[15t` gave none.
- Values that make a step of the stage's step change nothing: wherever the step released the synthesizer, `end` already was −1 (section 17.1), and in step 6.3 of section 17.2 `done` was clear too; in step 12 the lookahead had already set `end` to −1, and at a final pause (step 14) `done` was clear; prosody's flag that ends a phrase sooner was always clear when the stage was more than 64 frames ahead (step 6.2); the first phoneme rendered after a reset never was the pause (step 9); in step 1, a cursor without a done node never had a current or a next one; a lookahead that found nothing always began with ahead 0 (section 17.3, step 4); a `]` ahead (section 17.3, step 2) only came right before the end command, whose pair of pauses sets `end` and clears `done` anyway; no rebase was due while a generated segment was written (the recorded runs rebase once); and the room at a fill (section 17.5) was always 179 frames or more.
- Steps of the stage's step no text reaches: start 0 and 4 (section 17.1), as every thread resets the stage before its first step; and ahead 3 at a segment, so the final pause the lookahead returns at once (section 17.3, step 2) and the final pause a generated segment frees (section 17.5, step 1): ahead is 3 only from a lookahead that found a generated segment before any segment until that segment is written, and the current node then is that segment.
- Steps of the stage's step that change nothing whatever the text: ahead becomes 3 both in step 5 of section 17.2 and when a generated segment starts (section 17.5, step 1), where either would do; and a 255 on track 12 of a segment `t`'s frame would stay 255, but no kind of the data pack has one there.
- A node of fewer than 2 frames: the shortest phonemes were 2 frames long, and 3 without the input flag 0x10 (the escape sequence `ESC[5A` sets it). The pitch's silence never reached track 17's frames (section 9.5).
- Action 15 on a phoneme in class closure: the rules run it only on the pause, and action 1 only on a phoneme not in class closure. Its check that the aspiration shift is at most track 2's next frame, and action 1's that track 1's next frame is 2 or more: the next frames are always larger.

In these the original reads outside its tables, and the port does something else instead. No recorded run gets there either:
- A previous, current or next node whose symbol isn't one of the phonemes: the port takes the pause.
- A symbol of 128 or more in a node a rule tests: the port gives it no classes, and no context in the vowel database.
- A voice other than 0 to 9: the port takes voice 0's tracks and F4 limit.
- A transition shape above 30 (section 2.5): the original has 31, and reads a null pointer for shape 31 and its articulator map as pointers above it. The port gives such a shape no weights. Action 9 asks for shape `c + 2` for `B`, `D` or `G` before `Z` and a pause (section 10.3), so for a closure of 29 frames or more there, the glottal dip for shape `g / 3` (section 15.9, step 5), so for a previous node of 93 frames or more, and action 14 for shapes `f / 2` and `f − f / 2` (section 16.6, step 2), so for `Z` or `V` of 61 frames or more before the pause.
- A previous `Y` of 30 frames or more after a front-ending phoneme, for the vowel database's `PullBack` (sections 11.5 and 11.6), which asks for shape `w + 1`.
- A `p` after `C` in the voice onset (section 9.5, step 3.4; the stages before write `C s`): the original writes into track 1 the low byte of its state's address, which differs from run to run. The port writes 0.
- A vowel node of 0 frames in section 12.5: the original divides by 0 and faults; the port leaves the vowel transition as it is.

For `@` and `|` without a record after `M` whose node before is a vowel, with the current node not stressed (section 12.3), the original then compares a value it never set in that call with 9; for `p` the value is 3. At 9, track 9's `back` would become the previous node's frames plus 1 and its mode would get the 1. The port takes it as not 9, as it was in the recorded runs that have this context.
