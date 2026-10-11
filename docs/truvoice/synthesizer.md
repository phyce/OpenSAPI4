# TruVoice: synthesizer

How L&H TruVoice (`tv_enua.dll`) turns its stream of parameter frames into samples. The synthesizer is a cascade/parallel formant synthesizer in the Klatt tradition, written entirely in 16- and 32-bit integer arithmetic. It works in two halves:

- the **converter** turns each 10 ms frame of 22 parameter bytes into a **coefficient block** of 40 words (section 4, to come);
- the **renderer** turns a block into one frame of samples (section 3).

The block is the boundary between them (section 2). This version of the spec covers the renderer.

**In short.**
- A frame is `rate / 100` samples: 110 at 11025 Hz, 80 at 8000 Hz. Both rates are synthesized directly; nothing is resampled.
- The source is a glottal pulse built from a cosine table, differentiated, plus aspiration noise. Each pitch period's length and amplitude come from the block, which holds two of each and alternates between them.
- The cascade branch runs the source through the source stage, the formant resonators from F5 (11025 Hz only) down to F2, a fixed nasal pole, a nasal zero and F1. The parallel branch runs the noise through F5 (11025 Hz only) to F2 and a bypass.
- F1, F2, F3, the nasal zero and the cascade's output gain change only at the start of a pitch period. Everything else changes at the frame.
- The two branches are mixed, smoothed and passed through a one-tap output filter that depends on the voice, then multiplied by the gain (1 to 3) and saturated.
- The renderer's state (filter memories, the noise generator, the pitch period clock) runs on from frame to frame and from text to text; only a reset clears it (section 3.1).

**Exactness.** With the arithmetic of section 1, the renderer is bit-identical to the original. This was checked against the original engine on every block of 74 recorded runs: 35,754 blocks and 3,460,050 samples, covering the ten voices at both rates, texts spoken one after another in one session, pitch and speed extremes, whisper and monotone, voice changes, gains 2 and 3 and muted output.

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

The output-filter voice and the gain are not in the block: the engine passes them with it (section 5, to come). The output-filter voice follows the selected voice, but on a voice change it switches when the change reaches the parameter generator: in the recorded runs, 12 frames (the converter's lookahead) before the new voice's frames reach the renderer.
