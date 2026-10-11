# SAPI 4 Microsoft engine: synthesis

How the Microsoft Speech Synthesis Engine (`msttssyn.dll`) turns each **frame** ([units.md](units.md)) into samples: decoding the recorded unit ([voice-format.md](voice-format.md) §8), fitting it to the frame's duration and pitch, the vocal-tract filter, clipping, what is sent per frame, and which state carries from frame to frame. The variant effects (whisper, rooms, robot voices) are in [effects.md](effects.md); this spec marks where they hook in.

**In short.**
- A frame plays one recorded unit, or silence, for its duration, following the pitch and amplitude of its points.
- The unit decodes into one excitation period per stored frame (a pitch pulse or a block of noise), each with its own filter.
- Duration comes from repeating or skipping whole periods. Pitch comes from resizing each voiced period. There is no overlap-add.
- Every output period goes through its source frame's all-pole filter. The filter memory runs on from frame to frame, and every silence frame clears it.
- Output is 16-bit mono at the voice's own rate. When the audio destination refuses that, it is 8-bit, or at another rate through a resampler (section 13).
- Each frame that yields samples sends one `Visual` event and one block of audio.

**Exactness.** With the arithmetic of section 2, the result is bit-identical to the original. This was checked against the original engine:
- every unit of the five voices;
- about 31,000 sentence frames (69 sentences × 3 speeds × 5 voices, plus extra speeds);
- hand-made edge cases.

## 1. Pipeline

Per frame:

| # | Step | Section |
|---|---|---|
| 1 | Silence frame (unit 0): zeros, clear the filter memory | 8 |
| 2 | Decode the unit | 4 |
| 3 | Plan the output periods | 5 |
| 4 | Too many samples for the buffer: start again from step 2 | 9 |
| 5 | Build the excitation | 6 |
| 6 | Filter | 7 |
| 7 | Effects ([effects.md](effects.md) §2) | |
| 8 | Resample, when the destination took another rate | 13 |
| 9 | Clip and convert to 16 or 8 bits | 10, 13 |
| 10 | Send `Visual` and the samples | 11 |

## 2. Arithmetic

The original runs on the x87 FPU at 53-bit precision, rounding to nearest. That is Windows' default for a new thread, and the engine never changes it.

- **Precision.** Every calculation is in double precision (float64), evaluated left to right as written, unless the spec says otherwise.
- **`f32(x)`** rounds a double to float32 (to nearest, ties to even). Every array the engine keeps is float32: spectrum, excitation, coefficients, filter memory, samples. Storing into one is an `f32`.
- **Products** of two float32 values are exact in double; the rounding happens at the next addition or store.
- **`trunc(x)`** truncates toward zero to a 32-bit integer. NaN and ±∞ give 0.
- **Division by zero** gives ±∞ or NaN, as in IEEE arithmetic.
- **Comparisons** involving NaN are false, unless a rule says otherwise.

**In Go:**
- Do float32 arithmetic in float64. Write `float32(float64(a) - float64(b)*float64(c))`, never `a - b*c` on float32 values: Go rounds every float32 operation to float32.
- Keep products separate. Go may fuse `x*y + z` into one instruction on arm64 and other platforms. Write `float64(x*y) + z`: the explicit conversion forces the rounding.
- Handle NaN and ±∞ before converting to an integer. Go's conversion of an out-of-range float to int is platform-specific.

**Transcendental functions.**
- **The line spectral cosines come from the data pack** (section 3), not from `math.Cos`. The original uses the FPU's own cosine instruction, whose result has 64 significant bits. For about 3.5 % of the codebook values it differs in the last bit from a correctly rounded double, and some of those differences change a coefficient.
- **The sine table, fade window and noise table** can use Go's `math.Sin`, `math.Cos`, `math.Log` and `math.Sqrt`. Every value lies at least 9.7·10⁻¹³ (relative) away from a float32 rounding boundary, so any accurate library gives the same tables.

## 3. Tables

Per base voice, built when it loads (N is the transform size, p the predictor order: voice-format.md §8):

- **Sine table** s, 2N floats. `step = π / N` in double.
  - For 1 ≤ i < N/2: `s[i] = f32(sin(i · step))`.
  - The rest by symmetry: `s[N − i] = s[i]`, `s[N + i] = s[2N − i] = −s[i]`.
  - `s[0] = s[N] = 0`, `s[N/2] = 1`, `s[3N/2] = −1`.
- **Fade window** w, N floats: `w[i] = f32((cos(i · step) + 1) · 0.5)`. It falls from 1 to 0 across the window.
- **Noise table:** voice-format.md §8. The Go engine builds it from `rand` seed 1 for every voice.
- **Line spectral cosine table**, from the data pack: for every value f in the line spectral codebooks, `c(f) = −2·cos(2π·f)` as a double, computed as the original does:
  1. θ = f · 2π in double (2π itself a double);
  2. the cosine of θ rounded to 64 significant bits;
  3. times −2, rounded to 53 bits.

  The converter computes this with high-precision arithmetic.
  - On the development machine's CPU, 5 codebook values come out one unit in the last place different, because the FPU's argument reduction near π/2 is inexact. The converter stores the measured values.
  - None of the 5 changes a coefficient.

## 4. Decoding a unit

The format and the decoding steps are in voice-format.md §8. This section gives the exact arithmetic.

**Periods.** The escape-coded integers are exact as floats. A frame's length is `L = trunc(|period|)`, and the unit's length is `T = Σ L`.

**Filter coefficients**, per frame:
1. Take the p line spectral frequencies (float32). If any is not greater than the one before it, sort them ascending.
2. Look up `cE[j] = c(f[2j])` and `cO[j] = c(f[2j+1])` for j < p/2 (section 3).
3. Feed an impulse through two cascades of p/2 second-order sections, all in double, with every state value starting at 0:

   ```
   previous = 0
   for n = 0 .. p:
       x = 1 if n == 0 else 0
       inP = previous + x
       inQ = x − previous
       previous = x
       for j = 0 .. p/2 − 1:
           outP = (cE[j]·oneP[j] + twoP[j]) + inP
           outQ = (cO[j]·oneQ[j] + twoQ[j]) + inQ
           twoP[j] = oneP[j];  oneP[j] = inP
           twoQ[j] = oneQ[j];  oneQ[j] = inQ
           inP = outP;  inQ = outQ
       if n ≥ 1: a[n] = f32((inP + inQ) · 0.5)
   a[0] = 1
   ```

**Voiced excitation**, per frame:
1. **Spectrum S** (N floats). On the "first" codebooks, zero S and copy the bands in. On the "delta" codebooks, add each value: `S[k] = f32(v + S[k])`. The band layout is in voice-format.md §8.
2. **Pulse.** Run the inverse transform (below) on a copy of S.
3. **Gain:** `x[t] = f32(gain · x[t])`.
4. **Fold into the period.** With `m = min(N/2, L)`:
   - `e[0..m) = x[0..m)`;
   - `e[m..L) = 0`;
   - then for i < m: `e[L − m + i] = f32(x[N − m + i] + e[L − m + i])`.

**Unvoiced excitation.** `factor = gain · f32(0.02)` is kept in double, and is exact. Then `e[t] = f32(noise[position + t] · factor)`. The noise position is handled as in voice-format.md §8. It advances for every unvoiced frame of the unit, including frames the plan later skips.

**The inverse transform.** This is Sorensen's split-radix inverse real FFT (Sorensen, Jones, Heideman and Burrus, *Real-valued fast Fourier transform algorithms*, IEEE ASSP 1987). The input x is in half-complex order (x[k] the real part of bin k, x[N − k] its imaginary part), and the result is divided by N. The result equals the inverse DFT of voice-format.md §8 to within about 2·10⁻⁶. To get the original's rounding, follow this order exactly:

```
h = N/2;  r = f32(0.70710677)
if log2(N) > 1:
    span = 2N;  stride = 1
    repeat log2(N) − 1 times:
        step = span;  span = span/2;  q = span/4;  e = q/2;  stride = 2·stride
        start = 0
        loop:
            for i1 = start; i1 < N; i1 += step:
                i2 = i1+q;  i3 = i2+q;  i4 = i3+q
                t1 = x[i1] − x[i3]
                x[i1] = f32(x[i3] + x[i1])
                x[i2] = f32(x[i2] + x[i2])
                x[i3] = f32(t1 − (x[i4] + x[i4]))
                x[i4] = f32(t1 − x[i4]·(−2))
                if q == 1: continue
                i1 += e;  i2 += e;  i3 += e;  i4 += e
                t2 = (x[i4] + x[i3]) · r
                t1 = (x[i2] − x[i1]) · r
                x[i1] = f32(x[i1] + x[i2])
                x[i2] = f32(x[i4] − x[i3])
                x[i3] = f32((t2 + t1) · (−2))
                x[i4] = f32((t1 − f32(t2)) · 2)
            start = 2·step − span;  step = 4·step
            until not (start < N − 1)
        for j = 1 .. e − 1:
            a = j·stride
            cc1 = s[a + h];  ss1 = s[a];  cc3 = s[3a + h];  ss3 = s[3a]
            start = 0;  step = 2·span
            loop:
                for i = start; i < N; i += step:
                    i1 = i + j;      i2 = i1+q;  i3 = i2+q;  i4 = i3+q
                    i5 = i + q − j;  i6 = i5+q;  i7 = i6+q;  i8 = i7+q
                    t1 = x[i1] − x[i6];  x[i1] = f32(x[i1] + x[i6])
                    t2 = x[i5] − x[i2];  x[i5] = f32(x[i2] + x[i5])
                    t3 = x[i3] + x[i8];  x[i6] = f32(x[i8] − x[i3])
                    t4 = x[i4] + x[i7];  x[i2] = f32(x[i4] − x[i7])
                    u4 = f32(t2 − t3)
                    t1 = f32(t1);  t2 = f32(t2);  t4 = f32(t4)
                    t5 = f32(t1 − t4);  u1 = f32(t4 + t1);  u2 = f32(t3 + t2)
                    x[i3] = f32(cc1·t5 + ss1·u4)
                    x[i7] = f32(ss1·t5 − cc1·u4)
                    x[i4] = f32(cc3·u1 − ss3·u2)
                    x[i8] = f32(ss3·u1 + cc3·u2)
                start = 2·step − span;  step = 4·step
                until not (start < N − 1)
start = 0;  step = 4
loop:
    for i = start; i < N; i += step:
        (x[i], x[i+1]) = (f32(x[i+1] + x[i]), f32(x[i] − x[i+1]))
    start = 2·step − 2;  step = 4·step
    until not (start < N − 1)
swap x into bit-reversed order
x[i] = x[i] / N for every i
```

Notes on the algorithm:
- The indices of a butterfly are distinct, and no array element is read after the butterfly has written it.
- In the general butterfly, `u4` uses the unrounded t2 and t3. t3 is never rounded.
- The twiddle factors come only from the sine table. Its largest index is 5N/4.

## 5. Planning the output periods

**Inputs:**
- the decoded periods `period[0..F−1]`: voiced if ≥ 0, unvoiced if < 0;
- T (section 4) and the frame's duration d (float32);
- the voice's rate R as float32;
- the frame's points (units.md §7).

**Points.** For each point i:
- position `X_i = f32((time_i · d) · R)`, in output samples from the frame's start;
- pitch `P_i` = the frame's 16-bit pitch value;
- amplitude `A_i` (float32).

A phone frame has two to five points: two from the FrontEnd's records, up to five from a prosody transplant (units.md §7). Silence frames don't read their points.

**Interpolation** of a point value V at output sample position x:
1. Take the segment i = the first i in 1 .. count−2 with `x ≤ X_i`, else count − 1. With two points, i is always 1.
2. The value is `((V_i − V_{i−1}) · (x − X_{i−1})) / (X_i − X_{i−1}) + V_{i−1}`, in double.

So the pitch and amplitude are linear across the frame and continue on the same line past its end.

**Time scale:** `ts = f32(R · d / T)`.

**The plan** is a list of output periods `(source frame, reversed, length)`:

```
acc = 0 (double);  sourceTime = 0 (float32);  j = 0;  outPos = 0;  copies = 0
while j < F:
    if period[j] ≥ 0:                                   voiced
        f0 = pitch interpolated at outPos
        sourceLength = trunc(period[j] + 0.5)
        length = trunc(R / f0)
        if length > 2·sourceLength: length = 2·sourceLength
        if sourceLength > 2·length: length = sourceLength / 2      (integer division)
        unvoiced = false
    else:                                               unvoiced
        sourceLength = length = trunc(−period[j] + 0.5)
        unvoiced = true
    half = (length · (−0.5)) / ts
    sourceEnd = f32(sourceLength + sourceTime)
    skipped = acc − half > sourceEnd
    reversed = false
    if not skipped:
        reversed = unvoiced and copies is odd
        acc = length / ts + acc
        add (j, reversed, length) to the plan
        outPos += length;  copies += 1
    if acc − f32(half) > sourceEnd or copies ≥ 3 or reversed or skipped:
        j += 1;  copies = 0;  sourceTime = sourceEnd
```

**Precision.**
- `acc` stays a double throughout.
- The first test uses `half` as a double; the second uses `f32(half)`.
- f0 of 0 makes `R / f0` infinite, so `trunc` gives 0. The length then becomes `sourceLength / 2`. The same happens for a negative or NaN f0.

**Consequences:**
- **Time** follows the request by repeating or skipping whole periods:
  - a voiced period plays at most 3 times;
  - an unvoiced block plays at most twice, and the second copy is reversed in time.

  So speech slower than about 3× the recording comes out shorter than requested. Sam at 30 wpm gets about 90 % of the requested time; single frames can get as little as 53 %.
- **Pitch** is per period: each voiced period is resized to `R / f0` samples, but never beyond twice or half its recorded length.
- **Short frames.** A period plays only if it is at most twice the frame's length. A frame too short for any of its periods produces no samples at all (section 11).

## 6. Building the excitation

The output buffer holds `Σ length` floats. For each planned period in order:
- x is its output position, the sum of the lengths before it;
- the source is the excitation of frame j, starting at `Σ_{k<j} L_k`, with `sourceLength = L_j`.

1. **Amplitude:** `amp = f32(amplitude interpolated at x)` (section 5).
2. **Copy:**
   - reversed: `out[k] = source[length − 1 − k]`;
   - equal lengths: plain copy;
   - otherwise resize:

     ```
     out[0 .. length) = 0
     m = min(sourceLength, length)
     out[0] = source[0]
     step = N / m;  position = 0.5            (doubles)
     for k = 1 .. m − 1:
         position = position + step
         w = fade[trunc(position)]
         out[k] = f32(source[k]·w + out[k])
         out[length − k] = f32(w·source[sourceLength − k] + out[length − k])
     ```

     The resized period takes the start of the recorded period, fading out, and its end, fading in. Where the two overlap, they add, in the order shown.
3. **Scale:** if `amp ≠ 1` and amp is not NaN, `out[k] = f32(amp · out[k])` over the period.

## 7. Filtering

Each planned period is filtered in place with its source frame's coefficients a[1..p]. m is the filter memory (p + 1 floats), shared by all frames. Per sample:

```
m[0] = sample
for k = p down to 1:
    m[0] = f32(m[0] − a[k]·m[k])
    m[k] = m[k − 1]
sample = m[0]
```

After each sample, m[k] holds the output k samples back. The order (k = p first) and the float32 store after every step are part of the result.

## 8. Silence frames

Unit 0 is used by `SIL` words, slow-mode pauses and `\Pau=`:
- **Samples:** `n = trunc(d · R)` zeros (R as float32; the product is exact).
- **Filter memory:** set to zero, even when n = 0.
- **Points:** not read. A `SIL` frame's amplitude of 110 and the point slots beyond the count have no effect.
- **Effects:** the reverb still runs on the zeros, and draws its random number even for 0 samples. The post-filter doesn't run ([effects.md](effects.md) §2).
- **In Go:** the zeros come in pieces of at most 16384 samples, each through the reverb, which draws its random number after the last one. n is a 64-bit count, so a pause of any length plays without being held at once ([engine.md](engine.md) §9).

## 9. Buffer capacity

**The rule.** The original sizes its buffer at `capacity = ceil(1.5 · R · d)` samples, with R as an integer (the product is exact).
- If the plan needs more samples, the attempt is abandoned after decoding and planning, and the frame starts again from decoding with twice the capacity.
- The failed decode has already moved the noise position on, so the second decode reads different noise. The audio uses the second decode.

**Reproduce it:** if the plan's total exceeds `ceil(1.5 · R · d)`, decode again before going on.

**How often.** A frame's output is at most `R · d` plus half its last period, which is less than twice the capacity. So it happens at most once per frame. It happens only for very short phone frames whose one playable period is longer than 1.5 times the frame. Example: Sam at 450 wpm has a 21 ms `T` whose only played block is 729 samples, against a capacity of 702.

## 10. Clipping and samples

For each output float x:
1. **Clip:**
   - `x > 32767` gives 32767;
   - x not ≥ −32768 gives −32768. This includes NaN.
2. **Round** to the nearest integer, ties to even.
3. **Keep** the low 16 bits.

Samples are signed 16-bit little-endian, mono, at the voice's rate. Section 13 has the other formats; they clip the same way.

## 11. What a frame sends

- **A frame with samples:** one `Visual` event for its phone (units.md §11), then its samples, as one block.
- **A frame with 0 samples:** nothing, neither `Visual` nor audio. This happens for:
  - `\Pau=0\`;
  - Sam's 0 s slow-mode pauses;
  - phone frames too short for one period. At speeds of 30–510 wpm none occur in the test sentences; the shortest phone frame there is 75 samples;
  - in a resampled format, frames the resampler turns into no samples, such as a text's first frame when it is no longer than the resampler's history (section 13.2).

**Positions.**
- A `Visual` event fires at the first sample of its frame.
- A word position or bookmark between two frames fires at the total number of samples of all frames before it (units.md §10).

## 12. State

Per engine instance:

| State | Starts at | Changed by | Reset by |
|---|---|---|---|
| Filter memory | 0 | every output period | every silence frame |
| Noise position | 0 | every decoded unvoiced frame, including skipped ones and a decode repeated for capacity | never |
| Effect state | [effects.md](effects.md) §8 | | |
| Resampler (section 13) | reset | every frame | the start of every `TextData` call |

Nothing but the resampler is reset at a `TextData` call, at the start or end of a buffer, or by `AudioReset`. So consecutive `TextData` calls are synthesised as one continuous stream, and in the voice's own rate they also play as one. Speed, volume and the pitch carries are in units.md §12.

**`AudioReset`.** The synthesiser drops the frames it takes after the abort flag is set, but it has usually synthesised some frames past the stopping point by then, and how many depends on thread timing. [engine.md](engine.md) §12 says how far, and the Go engine follows it.

## 13. Output formats

The voice's own format is 16-bit mono at 22050 Hz, or 8000 Hz for the telephone voices. When the audio destination refuses it, the engine falls back to another rate, another sample width, or both (interface.md §4). Every format is mono.

**A frame's samples**, in order:
1. Synthesis (sections 4–8) and the effects. The reverb clips its own output ([effects.md](effects.md)); nothing else is clipped yet.
2. The resampler, when the accepted rate isn't the voice's.
3. Clipping (section 10).
4. The sample width: 16 bits (section 10) or 8 bits.

### 13.1 8-bit samples

Each clipped float x becomes the byte `((floor(x + 0.5) >> 8) & 0xFF) XOR 0x80`:
- x + 0.5 is computed in double (exact) and floored to an integer;
- `>> 8` is an arithmetic shift (floor division by 256);
- the XOR makes it unsigned, with silence at 0x80.

So x is rounded half up to an integer and then floored to 8 bits; there is no rounding at 8 bits and no dither. Examples: 0 → 0x80, −0.6 → 0x7F, 255.4 → 0x80, 255.5 → 0x81, 32767 → 0xFF, −32768 → 0x00.

### 13.2 Resamplers

The engine has one resampler for each pair of rates below. Any other pair has none.

| Voice → accepted | Up L | Down M | Taps N | History H | Real output rate |
|---|---|---|---|---|---|
| 22050 → 8000 Hz | 4 | 11 | 401 | 100 | 8018.18 Hz |
| 22050 → 16000 Hz | 8 | 11 | 601 | 75 | 16036.36 Hz |
| 8000 → 11025 Hz | 11 | 8 | 401 | 37 | 11000 Hz |
| 11025 → 8000 Hz | 8 | 11 | 601 | 75 | 8018.18 Hz |

- The taps are float32 tables in the original: symmetric lowpass filters at L times the voice's rate. The 11025 Hz one uses the 22050 → 16000 table. No shipped voice has 11025 Hz, so it is never used.
- **State:** `history`, H floats; `phase`, an integer; `start`, an integer.
- **Reset:** `history` all 0, `phase = −1`, `start = H`. At the start of every `TextData` call, before the call's first frame, even while `AudioReset` is under way.
- **Each frame** of n samples x goes through it whole. With b = `history` followed by x (H + n floats):
  ```
  for i = start .. n − 1:
      p = phase + L
      if p ≥ 0:
          repeat (p + M) div M times:      (once when L < M; once or twice for 8000 → 11025)
              y = Σ b[i + k] · taps[p + L·k] for k = 0, 1, … while p + L·k < N
              output f32(y)
              p = p − M
      phase = p
  start = 0
  history = b[n .. n + H − 1]
  ```
- **Arithmetic:** y starts at 0 and adds the products in order of k (the oldest sample first), in double. The products of two float32 values are exact. y is rounded to float32 once, at the end.

**Consequences.**
- **Rates.** The output rate is the voice's rate times L/M, not the rate the destination accepted: 8018.18 Hz for "8000", 16036.36 Hz for "16000" and 11000 Hz for "11025". At the accepted rate, the 22050 Hz voices play 0.23 % slow and 3.9 cents flat, and the telephone voices 0.23 % fast and 3.9 cents sharp.
- **Gain.** The DC gain is −0.48 dB at 22050 → 8000, +0.09 dB at 22050 → 16000 and −0.47 dB at 8000 → 11025.
- **Each text stands alone.** Output m of a text stands for input time 49.25 + 2.75·m at 22050 → 8000 (36.625 + 1.375·m at 22050 → 16000, 17.27 + 0.727·m at 8000 → 11025). So about 2 ms at a text's start only show as the filter's tail. The last input samples of a text never come out, because the next call's reset zeroes them: 50 to 52.75 samples at 22050 → 8000, 37.5 to 38.9 at 22050 → 16000, 18.8 to 19.5 at 8000 → 11025.
- **Frame boundaries don't matter**, with one exception. A text's first frame of at most H samples gives no output. The next frame then starts its loop at 0, so its outputs see H − n of the zeros the reset left. Examples: a text that is only `\Pau=3\` (66 samples at 22050 Hz), or `\Pau=2\\Pau=2\`. A pause before a sentence's first word doesn't count: the sentence's leading `SIL` comes first.
- **Frames with no output** send no `Visual` (section 11). Besides a text's short first frame, that is a frame of at most 2 samples at 22050 → 8000 and of 1 sample at 22050 → 16000, depending on the phase; no phone frame is that short.
- **Event positions** count the output samples. An event between two frames lands after the earlier frame's output. That output covers input up to about the filter's delay before the frame's end: 50 samples (2.3 ms) at 22050 → 8000.
- **Clipping comes after the resampler**, so it sees the synthesis output unclipped: values past ±32768, and NaN.

### 13.3 In Go

The synthesiser holds the resampler and the sample width the engine took when it was made (engine.md §10), and gives each frame as PCM bytes in that format. The synthesiser resets the resampler when it takes a text's start, as the original's does, so each `TextData` call stands alone however far the stages have run ahead. Every format case checked against the original engine is identical in Go (engine.md §11, §12.3).

A silence frame comes in pieces of at most 16384 samples (section 8). Each piece goes through the resampler in turn, which gives what the whole frame gives. A first piece can be at most H samples only when it is the whole frame. A piece the resampler turns into no samples sends nothing; the frame's `Visual` comes before its first sample, as for a whole frame.

## 14. Quirks

**Reproduce these.** Each one changes the audio.
1. Duration comes from whole periods: at most 3 copies of a voiced period and 2 of an unvoiced block (the second reversed). Very slow speech is shorter than requested.
2. Pitch changes are limited to half and double the recorded period.
3. There is no overlap-add. Resized periods are tapered with the fade window, and shortening them shifts the DC level: Mary's speech gets a negative offset of up to about 260.
4. The filter memory carries from frame to frame, and every silence frame cuts it. Speech that runs straight into a pause ends with a click.
5. The noise position runs on for the whole session, through skipped periods and repeated decodes.
6. The capacity rule (section 9).
7. A frame with no samples sends no `Visual`.
8. Clipping turns NaN into −32768. Rounding is half to even.
9. 8-bit samples are floored, not rounded (section 13.1).
10. The resampled rates are 0.23 % off the rate the destination accepted, and each `TextData` call is resampled on its own: its last 1–2 ms are lost, and a short first frame gives nothing (section 13.2).

**Don't reproduce:**
- What allocation failures do: crashes, a silently dropped frame, or retrying forever.
- **More output periods than the original's arrays.** It sizes its plan arrays for `2 · trunc(trunc(3 · p · d) + 3 · ts · F)` periods, where p is the frame's highest point pitch as an integer. Only extreme transplanted pitches and durations plan more (engine.md §13.7). The original then writes past its arrays and usually crashes; when it survives, its samples are the plan's. The Go engine plays the plan.
- Unit numbers outside the inventory, and frames with fewer than two points. The original crashes, or retries forever.
- A noise table that depends on the host thread's earlier use of `rand`, and is rebuilt from it when a voice is selected again after its last engine was released. The Go engine always starts from seed 1.

## 15. For the data pack

Per base voice:
- the inventory: codebooks and unit records (voice-format.md §8 and §11);
- the line spectral cosine table: `c(f)` as a float64 for every distinct value in the line spectral codebooks (section 3).

The sine table, fade window and noise table are generated, not stored.

Shared: the three resampler tables (section 13.2), as float32 ([datapack.md](datapack.md) §12).

Constants in this spec: f32(0.02), f32(0.70710677), 0.5, 1.5, 2, 3, 32767 and −32768.

## 16. Open questions

To be settled against the original engine:
1. **`rand` when a voice loads.** The noise table uses the loading thread's `rand` state. Seed 1 holds for a fresh thread loading its first voice. A host that loads several voices on one thread continues the sequence, and so does selecting a voice again after its last engine was released. Check which applies to the usual hosts. With one voice selected on a fresh thread, the original's audio matches seed 1.
2. **The full path.** The checks so far cover the synthesis stage on its own. Recordings through the COM interface should confirm the clicks before pauses and Mary's DC offset. *Answered:* through the COM interface the original's audio matches ours bit for bit. That covers the 69-sentence corpus on all five voices at three speeds and three pitches and on the 14 visible variants: 3,036 cases.
3. **Other CPUs.** Do AMD CPUs give the same 64-bit cosines for the codebook values near π/2? No known difference changes a coefficient.
4. **The host's FPU settings.** A host that changes the FPU's precision while a voice loads (Direct3D's 24-bit mode, for example) would change the tables built at load. The synthesis thread is not affected.
