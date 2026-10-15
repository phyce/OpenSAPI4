# TruVoice: synthesizer

How L&H TruVoice (`tv_enua.dll`) turns its stream of parameter frames into samples. The synthesizer is a cascade/parallel formant synthesizer in the Klatt tradition, written entirely in 16- and 32-bit integer arithmetic. It works in two halves:

- the **converter** turns each 10 ms frame of 22 parameter bytes into a **coefficient block** of 40 words (section 4);
- the **renderer** turns a block into one frame of samples (section 3).

The block is the boundary between them (section 2). Section 5 is how the engine drives them, one frame per step, and what it does with the samples.

**In short.**
- A frame is `rate / 100` samples: 110 at 11025 Hz, 80 at 8000 Hz. Both rates are synthesized directly; nothing is resampled.
- The source is a glottal pulse built from a cosine table, differentiated, plus aspiration noise. Each pitch period's length and amplitude come from the block, which holds two of each and alternates between them.
- The cascade branch runs the source through the source stage, the formant resonators from F5 (11025 Hz only) down to F2, a fixed nasal pole, a nasal zero and F1. The parallel branch runs the noise through F5 (11025 Hz only) to F2 and a bypass.
- F1, F2, F3, the nasal zero and the cascade's output gain change only at the start of a pitch period. Everything else changes at the frame.
- The two branches are mixed, smoothed and passed through a one-tap output filter that depends on the voice, then multiplied by the gain (1 to 3) and saturated.
- The renderer's state (filter memories, the noise generator, the pitch period clock) runs on from frame to frame and from text to text; only a reset clears it (section 3.1).
- The converter scales the frame's formants and bandwidths by the voice, computes each resonator's coefficients from tables, the two pitch periods and pulse amplitudes from the pitch with random jitter and shimmer, and the parallel branch's gains from the formant levels. Its state (the jitter generator, a count of silent frames) also runs on until a reset.
- Each step renders the block converted last, then converts the next frame, once the parameter generator has written enough frames past it. The samples collect in a buffer that is handed over each time the frame the parameter generator marks for a phoneme or a pause comes up, and at the latest every 55 frames or so (section 5).

**Exactness.** With the arithmetic of section 1, the synthesizer is bit-identical to the original. This was checked against the original engine on 83 recorded runs, covering the ten voices at both rates, 16- and 8-bit output, texts spoken one after another in one session, pitch and speed extremes, whisper and monotone, voice changes, tones and generated segments, gains 2 and 3 and muted output:
- the converter and the renderer on every frame: 39,317 frames, blocks and 3.8 million samples;
- the step and the loop on each of the original's 44,827 steps (5,540 loops, 3,343 hand-overs), the whole output of each run, and the positions of its 4,278 events;
- the volume conversion on every volume.

## 1. Arithmetic

- Integers are 16-bit (**words**) or 32-bit. Every product and sum is computed in 32 bits and wraps on overflow (two's complement). Nothing is computed in floating point.
- `x >> n` is an arithmetic shift (rounds toward −∞). `x / 100` truncates toward zero.
- `s16(x)` is the low 16 bits of x as a signed word. A value stored in a word field is `s16` of what was computed, unless the spec saturates it.
- `sat(x)` is 32767 when x ≥ 32767, **−32768 when x ≤ −32767**, and x otherwise. The original's limit is asymmetric: −32767 becomes −32768.
- A word read from the block or the state is sign-extended when it takes part in arithmetic, except the gain (section 3.8).
- `−w` of a word is `s16(−w)`: the negation of −32768 is −32768.

**In Go:** use `int16` and `int32` throughout, in the order the formulas are written. Go's signed integer overflow wraps, which is exactly the original's arithmetic; Go's `>>` on signed values is arithmetic and its `/` truncates. Mind Go's operator precedence: `*`, `>>` and `&` bind equally and tighter than `+`, unlike C.

## 2. The coefficient block

Forty words. Words named B and C are a resonator's feedback coefficients: B multiplies the previous output, C the one before. The block stores C negated: the renderer uses `C = −w`.

| Word | Name | Read by the renderer as |
|---|---|---|
| 0 | source | bits 0–4: the source type (0–31); bit 7: mute the frame. The other bits are ignored |
| 1 | phase step | the pulse's phase increment (section 3.3) |
| 2, 3 | | not read |
| 4, 5 | F5 B, −C | cascade F5 and parallel F5 (11025 Hz only) |
| 6, 7 | F4 B, −C | cascade F4 and parallel F4 |
| 8, 9 | F3 B, −C | latched: cascade F3 and parallel F3 |
| 10, 11 | F2 B, −C | latched: cascade F2 and parallel F2 |
| 12, 13 | nasal pole B, −C | the nasal pole |
| 14, 15 | F1 B, −C | latched: cascade F1 |
| 16 | aspiration gain | the noise added to the source |
| 17 | source gain | the pulse derivative's gain |
| 18 | | not read |
| 19 | first gain | A of the first resonator after the source stage: cascade F5 at 11025 Hz, cascade F4 at 8000 Hz |
| 20 | parallel F5 gain | A of parallel F5 (11025 Hz only) |
| 21 | F5 gain | A of cascade F4 at 11025 Hz (not read at 8000 Hz) |
| 22 | parallel F4 gain | A of parallel F4 |
| 23 | F4 gain | A of cascade F3 |
| 24 | parallel F3 gain | A of parallel F3 |
| 25 | F3 gain | latched: A of cascade F2 |
| 26 | parallel F2 gain | A of parallel F2 |
| 27 | F2 gain | latched: A of the nasal pole |
| 28 | bypass gain | the parallel branch's bypass |
| 29 | nasal zero gain | latched: A of cascade F1 |
| 30, 31 | periods | the two pitch period lengths, in samples minus one (section 3.2) |
| 32 | nasal zero B | latched: the nasal zero |
| 33 | nasal zero C | the nasal zero |
| 34, 35 | amplitudes | the two pulse amplitudes |
| 36 | cascade gain | latched: the cascade branch's output gain |
| 37, 38, 39 | source stage A, B, −C | the source stage |

Each resonator's normalising gain A is written with the formant it belongs to but applied at the input of the next resonator in the cascade: the F5 gain feeds cascade F4, the F4 gain cascade F3, and so on, and the nasal zero's gain feeds F1. The renderer only sees which word feeds which stage, as listed.

**Latched** words are taken into the renderer's state at the start of each pitch period (section 3.2) and keep their value until the next one, even across frames. All other words act from the first sample of the frame.

The converter writes every word except 2, 3, 12, 13, 18, 19 and 33, which keep the reset block's values (section 4.2), and it writes word 1 only for voiced frames.

## 3. The renderer

### 3.1 State and reset

The renderer keeps, all words unless said otherwise:
- the period clock: `countdown`, `halfCountdown`, `periodState`;
- the pulse: `segment`, `family`, `step` (phase increment), the 32-bit `shape`, `phase` and `amplitude`, and `previousPulse`, `derivative`;
- the noise generator: `noise` and `previousNoise`;
- the latched coefficients: F1 B and C, the cascade gain, F2 B, C and gain, F3 B, C and gain, the nasal zero's B and gain;
- two memories, `y1` (the last output) and `y2` (the one before), for each of the cascade stages (source stage, F5, F4, F3, F2, nasal pole, F1) and the parallel resonators (F5, F4, F3, F2);
- `previousParallel`, `previousMix` and the 32-bit `previousOutput`.

**Reset** sets all of them to 0, except `noise`, which becomes 0xAAAA (−21846), and sets the frame length to `rate / 100`. The engine resets the renderer when its speaking thread starts and when its pipeline is reset; texts spoken by a running thread share the state.

### 3.2 A frame

Per block, before the first sample:
- `type = word0 & 31`; `family` is the second family when `type ≥ 15`, otherwise the first; `shape = sourceShapes[type]` from the data pack.
- `step = s16((word1 · 0x517D) >> 15)`.
- Local copies `P = [word30, word31]` and `V = [word34, word35]`. They swap at every period start (below), and start again from the block's order in the next frame.

Then for each of the frame's samples, in order:

1. **Period clock.**
   - If `P[0] = 0` and `countdown < 0`: `periodState = 0`, `halfCountdown = 0`, `countdown = 0`.
   - Otherwise `countdown −= 1`. If now `countdown < 0`, a period starts: latch the words marked latched in section 2; `countdown = P[0]`; `halfCountdown = P[0] >> 1`; swap `P[0]` and `P[1]`; `periodState = −1`.
   - Otherwise `halfCountdown −= 1` and `periodState = 1`.

   A period of `P[0]` therefore lasts `P[0] + 1` samples. When `P[0]` is 0 (an unvoiced frame) a period starts at every sample, which keeps the latched coefficients current and the pulse at 0. The first branch only matters after a negative period, which the recorded runs never contain.
2. **Pulse** `g` (section 3.3).
3. **Derivative.** `derivative = sat(((g − previousPulse) · 8 · word17) >> 15)`; `previousPulse = g`.
4. **Noise.** `x = previousNoise ^ noise` (both sign-extended); `previousNoise = noise`; `n = s16(((x & 0x3FF) << 6) | ((x >> 7) & 0x1F8))`; `noise = n`, or 0xAAAA when n is 0. Call the new value `noise`.
5. **Cascade branch** (section 3.4), giving `cascade`.
6. **Half noise.** If `countdown ≠ 0` and `halfCountdown < 0`: `noise = noise >> 1` for the parallel branch. In a voiced period this halves the frication noise in the period's second half.
7. **Parallel branch** (section 3.5), giving `parallel`.
8. **Mix and output** (section 3.6).

### 3.3 The pulse

The pulse has an opening segment (a raised cosine), a closing segment (shaped by `shape`) and a closed phase. `segment` is 0 while opening, 2 while closing and 3 when closed. `h` is the data pack's half-cosine table, 256 words.

- **Period start** (`periodState = −1`): `family` is set from the block; `amplitude = V[0]`, then swap `V[0]` and `V[1]`; `shape` and `step` are set from the block; `segment = 0`; `phase = 0`; `g = 0`.
- **No pulse** (`periodState = 0`, or `segment` is 3): `g = 0`.
- **Otherwise** `phase += step` (32-bit, `step` sign-extended), and:
  - If `phase > 0xFFFF`: `phase = phase & 0xFFFF`. If the pulse was closing, `segment = 3` and `g = 0` (the recorded runs never reach this). Otherwise `segment = 2` and the closing formula below gives `g`.
  - Otherwise, if opening: `half = amplitude >> 1`; `g = s16(half − (((c − d) · half) >> 15))`.
  - Otherwise (closing) the closing formula gives `g`.

  with the interpolated cosine at the phase: `b = (phase >> 8) & 0xFF`, `c = h[b]`, `d = s16((h[|128 − b|] · (phase & 0xFF)) >> 15)`.

**Closing formula.** `p = (c − d) · shape`; `v = (p − (shape << 15)) >> 15`.
- First family: `w = (v + 0x7FFF) << 16`. If w is negative (bit 15 of `v + 0x7FFF` is set), `segment = 3` and `g = 0`; otherwise `g = s16((amplitude · (w >> 16)) >> 15)`.
- Second family: `w = ((v << 15) + (p & 0x7FFF)) · 2 + 0x7FFFFF`. If w is negative, `segment = 3` and `g = 0`; otherwise `g = s16((((w >> 8) & 0xFFFF) · amplitude) >> 15)`.

All of these are 32-bit wrapping operations; `w` relies on the wrap.

### 3.4 The cascade branch

A **cascade stage** with input x and coefficients A, B, C computes `y = sat((C · y2 + 2 · (A · x + B · y1)) >> 15)`, then `y2 = y1`, `y1 = y`, and outputs y.

1. Source stage: `x = s16(s16((word16 · noise) >> 15) + derivative)`; `u = stage(x, word37, word38, −word39)`.
2. At 11025 Hz: F5 `stage(u, word19, word4, −word5)`, then F4 `stage(·, word21, word6, −word7)`. At 8000 Hz: F4 `stage(u, word19, word6, −word7)`, and no F5.
3. F3: `stage(·, word23, F3 B, F3 C)` with the latched F3 coefficients.
4. F2: `stage(·, F3 gain, F2 B, F2 C)`, latched.
5. Nasal pole: `stage(·, F2 gain, word12, −word13)`, with the latched F2 gain. Call its output `q`, and its memories before this sample `q1` (`y1`) and `q2` (`y2`).
6. Nasal zero: `z = s16((((q1 · B) >> 14) & ~1) + q + ((word33 · q2) >> 15))` with the latched nasal zero B.
7. F1: `stage(z, nasal zero gain, F1 B, F1 C)`, latched.
8. Output: `cascade = sat(((F1 output · cascade gain) >> 14) & ~1)` with the latched cascade gain.

### 3.5 The parallel branch

A **parallel resonator** with input x computes the 32-bit `v = (A · x + 2 · B · y1 + C · y2) >> 15`, then `y2 = y1`, `y1 = s16(v)` (not saturated), and outputs v. The input of all of them is the noise after step 6.

1. At 11025 Hz: `t = sat(F5 resonator(noise, word20, word4, −word5))`. At 8000 Hz: `t = 0`.
2. `t = sat(s16(t) + F4 resonator(noise, word22, word6, −word7))`.
3. `t = sat(s16(t) + F3 resonator(noise, word24, F3 B, F3 C))`, latched F3 coefficients.
4. `t = sat(s16(t) + F2 resonator(noise, word26, F2 B, F2 C))`, latched F2 coefficients.
5. Bypass: `t = s16(((word28 · noise) >> 15) + t)`. This sum wraps instead of saturating.
6. `parallel = s16((t · 0x184D) >> 15)`.

### 3.6 Mix and output

1. `m = s16((parallel − previousParallel) + cascade − ((previousMix · 0x1D71) >> 15))`; `previousParallel = parallel`; `previousMix = m`. The parallel branch enters as a first difference.
2. If the block's mute bit is set, `m = 0` (after `previousMix` is stored).
3. `sample = sat((m − (filter · previousOutput) / 100) · gain · 8)`, where `filter` is the data pack's output filter of the output-filter voice (a percentage, −85 to 26) and `gain` is the engine's gain, 1 to 3, read as an unsigned word. `previousOutput = m`.
4. The sample is the next sample of the frame.

The output-filter voice and the gain are not in the block: the engine passes them with it (section 5.2). The output-filter voice follows the selected voice, but on a voice change it switches when the change reaches the parameter generator: in the recorded runs, 12 frames (the converter's lookahead) before the new voice's frames reach the renderer.

## 4. The converter

### 4.1 Frames

A frame is 22 bytes, one per **track**, written by the parameter generator for every 10 ms. The converter reads them as unsigned numbers `t0` to `t21`:

| Track | Name | Read as |
|---|---|---|
| 0 | AV | voicing amplitude; 0 means unvoiced |
| 1 | AF | frication amplitude; 0 switches the parallel branch off |
| 2 | AH | aspiration amplitude |
| 3, 4, 5, 6 | | offsets of the parallel F2, F3, F4 and F5 gains |
| 8 | AB | the bypass's amplitude |
| 9 | F1 | 4 Hz units |
| 10 | F2 | 504 Hz + 8 Hz units |
| 11, 12 | F3, F4 | 16 Hz units |
| 13, 14, 15 | B1, B2, B3 | 2 Hz units |
| 16 | FNZ | the nasal zero, 192 Hz + 4 Hz units |
| 17 | F0 | 2 Hz units |
| 21 | voice | bits 4–7: the voice (0–9) whose entry in the data pack applies |

Tracks 7, 18, 19 and 20 and the low bits of track 21 are not read. In a **tone frame**, F1, F2 and F3 are in 10 Hz units instead; the frame's tone bit in the ring says which frames are tone frames (section 5.1).

### 4.2 State and reset

The converter keeps:
- the **block** it writes (section 2): a frame overwrites the words listed in section 4.3, the others keep their values;
- `random`, a 32-bit register for jitter and shimmer;
- `silent`, a count of silent frames.

**Reset** sets the block to the data pack's reset block for the rate, `random = 0x55` and `silent = 0`. The engine resets the converter together with the renderer, and converts a frame before it renders one, so the reset block itself is never rendered.

The rate selects the data pack's tables: those for 8000 Hz at 8000 Hz, those for 11025 Hz at any other rate. `R` below is the rate's tables and `rate` the sample rate.

### 4.3 A frame

`q(a, b)` is `(a · b) / 32767`: the 32-bit product (it wraps), divided with truncation. `scale(x, p)` is `x + (p · x) / 100`. `V` is the voice's entry in the data pack's `voices`. Indexes computed from the frame follow section 4.4.

1. **Bandwidth floor.** Each of t13, t14 and t15 below 22 becomes 22, also in the frame itself.
2. **Voice.** `V = voices[t21 >> 4]`, and:
   - F1 = scale(t9, V.scales[0]), B1 = scale(t13, V.scales[1]), F2 = min(scale(t10, V.scales[2]), 255), B2 = scale(t14, V.scales[3]), F3 = scale(t11, V.scales[4]), B3 = scale(t15, V.scales[5]), F4 = max(scale(t12, V.scales[6]), F3 + 20), all in the tracks' units;
   - F5 = max(V.formant5, 16 · (F4 + 25)), in Hz;
   - AH = min(t2 + V.aspiration, 107).
3. **Silence.** If t0, t1 and AH are all 0, `silent += 1`; otherwise `silent = 0`.
4. **Source.** `f = 2 · t17`, the pitch in Hz, or 0 when t0 is 0.
   - If f is 0, words 0, 30, 31, 34 and 35 become 0.
   - Otherwise:
     1. If f is 74, 84 or 90, `f += 1`. (The original also tests 69 and 79, which an even f never is.)
     2. **Jitter.** `depth = q(f, jitterDepths[V.jitter])`. Draw two deviations, d1 and d2 (below). Word 30 is `s16((2 · rate / p) / 2)` with `p = s16(f + 8 · q(depth, d1))`, word 31 the same with d2.
     3. Word 0 = V.sourceType.
     4. Word 1 = `phaseSteps[min(q(q(word30, V.amplitude << 11), openPhases[V.sourceType]), 139)]`.
     5. **Amplitude.** `a = levels[140 + t0]`; `a = s16(q(a, sourceAmplitudes[V.sourceType]))`; `a = s16(q(a, amplitudeScales[V.amplitude]) >> 3)`; `a = s16((word30 >> 2) · a)`.
     6. **Shimmer.** With `s = shimmerDepths[V.shimmer]`: word 34 = `s16(a + (q(q(a, s), d2) << 3))`, reusing the second jitter deviation; draw d3; word 35 = `s16(a + (q(q(a, s), d3) << 3))`.

   A **deviation** is drawn by stepping the register, `random = (random >> 1) + ((((random & 1) << 1) ^ (random & 2)) << 6)`, and is `deviations[random & 63]`. A voiced frame draws three. The data pack's limits make sure p is never 0.
5. **Mute.** If the engine's mute flag is set (section 5.4), `word0 |= 0x80`. If `silent > 2`: `word0 |= 0x80` and `silent = 2`. Silence is muted from its third frame on, but only in voices whose aspiration offset is 0 (Peter and Melvin): in the others AH is never 0.
6. Word 16 = `levels[105 + AH]`; word 17 = V.sourceGain; words 37, 38 and 39 = R.sourceStage.
7. **Resonators.** The resonator for a frequency index i (8 Hz steps) and a bandwidth index j (4 Hz steps) has `P = R.radii[j] · R.cosines[i]`, `B = s16(P >> 12)`, `−C = s16(4 · R.squaredRadii[j])` and the 32-bit gain `G = R.squaredRadii[j] − (P >> 13) + 0x2000`.
   - F5: if `F5 > rate / 2`, there is none: word 21 = 0x2000 and words 4 and 5 become 0. Otherwise `i = F5 >> 3`, `j = V.bandwidth5 >> 2`; words 4 and 5 are B and −C, word 21 is `s16(G)`.
   - If `16 · F4 ≥ rate / 2`, `F4 = (rate / 2 − 10) >> 4`. (F5 was computed with F4 before this.)
   - F4: `i = 2 · F4`, `j = V.bandwidth4 >> 2`: words 6, 7 and 23 are B, −C and `s16(G4)`.
   - F3: `i = 2 · F3`, or `(10 · F3) >> 3` in a tone frame; `j = B3 >> 1`: words 8, 9 and 25.
   - F2: `i = F2 + 63`, or `(10 · F2) >> 3` in a tone frame; `j = B2 >> 1`: words 10, 11 and 27.
   - F1: `i = F1 >> 1`, or `(10 · F1) >> 3` in a tone frame; `j = B1 >> 1`: words 14 and 15 are B and −C, word 36 is `s16(min(G1, 0x7FF) << 4)`.
8. **Nasal zero.** Word 32 = `s16((R.nasalZeroB · R.cosines[(4 · t16 + 192) >> 3]) >> 12)`; word 29 = `s16(R.nasalZeroGains[t16 >> 2] << 1)`.
9. **Parallel branch.** If t1 is 0, words 20, 22, 24, 26 and 28 become 0. Otherwise:
   - Formant level indexes, in 32 Hz steps: `k1 = F1 >> 3`, `k2 = (F2 >> 2) + 16`, `k3 = F3 >> 1`; in a tone frame `k1 = (10 · F1) >> 5`, `k2 = (10 · F2) >> 5`, `k3 = (10 · F3) >> 5`.
   - `L1 = formantLevels[k1]`, `L2 = formantLevels[k2]`, `L = 2 · L1 + 2 · L2 − 235`.
   - Proximity corrections: `x12 = proximity[d(k2 − k1)]`, `x23 = proximity[d(k3 − k2 − 2)]`, `x34 = proximity[d((F4 >> 1) − k3 − 5)]`, where `d(n)` is n, or 0 when n ≤ 2, and F4 is the value after step 7's limit.
   - Word 26 (F2): `n = max(2 · L1 − L2 − 14 + 2 · x12 + x23 + t3 + t1, 0)`; word 26 = `s16((min(levels[n], 160) · G2) >> 11)`. Here the level is limited, not the index.
   - Word 24 (F3): `n = L + 2 · x23 + x34 + t4 + t1 + 22`, limited to 0–160; word 24 = `s16(−((levels[n] · G3) >> 11))`.
   - Word 22 (F4): `n = L + 2 · x34 + t5 + t1 + 17`, limited to 0–160; word 22 = `s16((levels[n] · G4) >> 11)`, then at 8000 Hz `word22 >> 1`.
   - Word 20 (F5): `n = L + t6 + t1 + 16`, limited to 0–154; word 20 = `s16(−((levels[n] · word21) >> 11))`, with word 21 as written in step 7.
   - Word 28 (bypass): `s16(levels[37 + t8 + t1] << 2)`.

**Not reached by the recorded runs:** a tone frame with frication, so the tone indexes of step 9 come from the code alone, and an index past the decibel table's end.

### 4.4 Tables and indexes

The tables are the data pack's (datapack.md). An index computed from a voice's entry always lies within its table: the data pack's limits make sure of it. Indexes computed from a frame:

- **`levels`** is the data pack's `decibels` (213 values, about `8192 · 10^((i − 200) / 20)`), followed by the value 0, the `formantLevels` and the `proximity` table: 453 values. The original reads its decibel table without a limit, and this is what follows it in its memory: an AV of 73 or more reads 0 and then the formant levels.
- In the recorded runs, every other index stays within its table. The original would read unrelated memory past a table; the port takes the nearest entry instead (the first for an index below 0, the last past the end), and does the same past the end of `levels`.

## 5. The step and the output

The synthesizer is the engine's last stage. The parameter generator hands it frames through a ring; the synthesizer converts and renders them one at a time, in steps, and collects the samples in a buffer that the engine hands to the audio destination.

### 5.1 The frame ring

The ring holds 256 frames. Each **slot** holds a frame (section 4.1), a **marker** bit and a **tone** bit. A frame's slot is its position modulo 256. Positions are counted by four 32-bit signed numbers:

- `position`: the next frame to convert;
- `written`: the frames written so far;
- `end`: the frame at which speech stops for now, or −1 for none;
- `lookahead`: how many written frames the synthesizer leaves alone, because the parameter generator may still change them.

The synthesizer also keeps three flags, `done`, `hold` and `ready` (a converted block waits to be rendered), and `passed`, a count of the marker bits it has met.

The parameter generator writes the frames and their bits, sets `written` and a marker bit after each phoneme or pause ([parameters.md](parameters.md) §2.6), and sets `end`, `lookahead` and `done` ([parameters.md](parameters.md) §17). Two operations on the ring are shared:

- **Reset**, when the engine resets its pipeline: clear every marker and tone bit; fill slots 0 to 11 with the parameter generator's initial frame; `position = 0`, `written = 12`, `end = 10`, `passed = 0`, `done` cleared. The parameter generator's own reset sets `lookahead = 2`. A session therefore starts with 10 copies of the initial frame, 100 ms.
- **Rebase**, when the parameter generator asks for it: if `position > 2048`, subtract 2048 from `position`, `written` and `end` (unless it is −1). The slots stay where they are. The parameter generator moves its own positions back by the same amount ([parameters.md](parameters.md) §2.7). The recorded runs contain one rebase, at position 2056; the limit of 2048 comes from the code.

### 5.2 A step

1. If `hold` and `done` are clear and `ready` is set:
   - if rendering is on, render the block (section 3) with the output-filter voice and the gain;
   - clear `ready`;
   - if `position = end`, set `done`.
2. If `lookahead + position < written` and `ready` is clear, convert the frame at `position`:
   - if its marker bit is set, clear it and add 1 to `passed`;
   - read its tone bit, then clear it;
   - convert it (section 4.3) with the engine's mute flag (section 5.4);
   - add 1 to `position` and set `ready`.

The first step after a reset only converts, so the block of section 4.2's reset is never rendered. Once `position` reaches `end`, the frame before it is the last one rendered until the parameter generator moves `end` on and clears `done`.

**Rendering** is on whenever the engine speaks. The engine also runs its pipeline without rendering, to turn text into phonemes.

**Hold** is set and cleared by the escape parser's commands `H`, `C` and `x` ([input.md](input.md) §1.4). The recorded runs never step with it set. Nor do they ever step while `done` is set: the loop below doesn't.

### 5.3 The loop and the buffer

Each time the engine's scheduler runs the synthesizer ([pipeline.md](pipeline.md) §6), the synthesizer runs a loop of steps:

1. If `done` is set, go to 3.
2. Repeat:
   1. If `hold` is set, stop.
   2. Step.
   3. If `ready` is clear, stop.
   4. If `passed` isn't 0, run the output stage and note a hand-over.
   5. If the buffer holds more than 12,000 bytes, note a hand-over and stop.
   6. If `done` is set, stop.
3. Run the output stage once more.

The **buffer** collects the rendered samples, 2 bytes each. When the loop has noted a hand-over, the engine's speaking thread hands the whole buffer to the audio queue and empties it ([pipeline.md](pipeline.md) §8.4). Otherwise the buffer keeps its samples for the next loop. Without markers a chunk is therefore 55 frames (12,100 bytes) at 11025 Hz and 76 frames (12,160 bytes) at 8000 Hz. The buffer the engine allocates holds 13,500 bytes.

The parameter generator marks one frame for each phoneme and pause. The **output stage** ([pipeline.md](pipeline.md) §5) frees the pipeline's nodes in step with the audio, a phoneme or pause node for each passed marker. The events are queued before that: the parameter generator's stage queues a word or bookmark event as soon as it reaches the command ([pipeline.md](pipeline.md) §4.1), while the buffer still holds samples rendered before it. So an event's position in the audio is where the last hand-over before it ended: every event falls on a hand-over boundary, never inside a chunk.

### 5.4 Volume, gain and the output format

**Volume.** Whenever the volume has changed, the engine applies it before it runs its scheduler again ([pipeline.md](pipeline.md) §8.3):

- A volume below 80 sets the mute flag, which mutes every frame converted from then on (section 4.3, step 5). The attenuation stays as it was.
- Any other volume clears the mute flag and sets the attenuation to `min(15, trunc(−10 · log10(volume / 65535)))` dB. The parameter generator subtracts the attenuation from its amplitudes ([parameters.md](parameters.md) §6).

The original computes the logarithm with x87 floating point. For every volume, at 24-, 53- and 64-bit x87 precision and in 64-bit floating point, the attenuation is the same. The value closest to an integer is 2.8·10⁻⁶ away from it. The thresholds are:

| Attenuation (dB) | Volumes |
|---|---|
| 0 | 52057–65535 |
| 1 | 41350–52056 |
| 2 | 32846–41349 |
| 3 | 26090–32845 |
| 4 | 20724–26089 |
| 5 | 16462–20723 |
| 6 | 13076–16461 |
| 7 | 10387–13075 |
| 8 | 8251–10386 |
| 9 | 6554–8250 |
| 10 | 5206–6553 |
| 11 | 4135–5205 |
| 12 | 3285–4134 |
| 13 | 2609–3284 |
| 14 | 2073–2608 |
| 15 | 80–2072 |
| mute | 0–79 |

**Gain.** The engine copies its gain (1 to 3, set with Inject `\gain=n\`) to the synthesizer when its speaking thread starts ([pipeline.md](pipeline.md) §8.1); a later Inject waits for the next session. The renderer multiplies by it (section 3.6).

**Output format.** The samples are 16-bit little-endian. In an 8-bit format each sample becomes one unsigned byte, `(sample >> 8) + 128`.
