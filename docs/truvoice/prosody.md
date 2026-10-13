# TruVoice: prosody

How L&H TruVoice (`tv_enua.dll`) gives each phoneme its duration and its pitch. The prosody stage works on the lexical stage's phonemes, the word marks before the words and the punctuation ([pipeline.md](pipeline.md) §3). It finds where its phrases end, puts in the pauses between them, and turns each phoneme into a segment for the parameter generator ([parameters.md](parameters.md)): a node of type 4 whose value is its duration in frames and whose second value is its pitch, in units of 2 Hz.

This spec covers the arithmetic (section 1), the phonemes the stage sees and what it notes about them (section 2), what the pitch takes from the phrase (section 3) and the pitch (section 4). How the stage finds the phrases and puts in the pauses, its step, and the durations will follow.

**In short.**
- A phrase's pitch starts a third of the voice's pitch above it, and comes down over the phrase's phonemes. Phonetic input can place word marks that make it come down to a mark and rise from it instead: the phrase's **contour**.
- The phrase's **nucleus** is its last stressed vowel before a break. The syllables before it and the stressed ones go up; the nucleus carries the phrase's accent.
- In the phrase's last syllable the pitch moves as the phrase's kind says: down at a sentence's end, up a little at a comma, up at a question. From there every phoneme is 8 Hz lower than the one before, down to 46 Hz.
- Small changes for the sounds: a voiceless stop starting a word before a vowel is 20 Hz higher, an unstressed voiced sound before a consonant 3 Hz lower (8 if it isn't a sonorant), a high vowel or glide 3 Hz higher, and an `N` before a `T` or a glottal stop at a word's end 55 Hz lower.
- A whispered phoneme gets pitch 0, a monotone one the voice's pitch. Otherwise the pitch stays between 50 and 500 Hz.

**Exactness.** Ported from the original's code, and checked against it by replaying every call of its functions recorded in 296 runs: the parameter generator's and the pipeline's runs ([parameters.md](parameters.md), [pipeline.md](pipeline.md)), and 68 more for the phrases and the pitch (questions, exclamations, lists, long sentences without punctuation, emphasis, `\Pro=0\`, pitch, character and voice changes within a text, slow and fast speech, 30 with the word marks phonetic input places (section 3) and 4 with long runs of consonants in phonetic input). Each of the 30,967 contexts (section 2.3), the 28,339 pitch contexts (section 4.2), the 28,339 pitches (section 4.3) and the 1,040 resets of the pitch (section 4.4) left the speaking thread's state byte-identical to the original's.

## 1. Arithmetic

All arithmetic is in 32-bit signed integers. Divisions truncate toward zero; shifts of negative numbers are arithmetic. The **Q15 product** of `a` and `b` is `a · b / 32767`, as in [parameters.md](parameters.md) §1. In a quotient such as `E · (p − m) / (N − m)` the product comes first.

## 2. Phonemes

### 2.1 Nodes and classes

The stage's window ([pipeline.md](pipeline.md) §3) holds control commands (type 0), the lexical stage's nodes (type 3) and the segments the stage has made (type 4). The nodes of type 3 are the phonemes, a word mark before each word, `%` or `&`, and the punctuation. A phoneme node's flag bits 3 and 4 are its **stress**, 0 to 3; 3 is an emphasized word's. Flag bit 0x20 the lexical stage sets on a stressed syllable's consonants and vowel: such a node is **stressed**, as in [parameters.md](parameters.md) §8. Flag bit 0x80 marks a phrase's nucleus (section 4.3).

The stage tests the symbols' classes ([parameters.md](parameters.md) §3.2):

| Class | Name here | Symbols |
|---|---|---|
| (0, 0x01) | syllabic | the vowels but `p`, and `l m n` |
| (0, 0x02) | sonorant | the vowels, `p`, `j l L W Y R`, `d h H`, the nasals `M m N n ~` |
| (0, 0x04) | voiced | the vowels, `p`, `j l L W Y R d h`, `B D t J G Q q V x Z z`, the nasals |
| (0, 0x08) | word mark | `% & [` |
| (0, 0x10) | nasal | `M m N n ~` |
| (0, 0x20) | stop | `B P D T J C G K q` |
| (0, 0x40) | fricative | `V F x X Z S z s` |
| (0, 0x80) | phoneme | every phoneme and the pause, a space |
| (1, 0x08) | affricate | `C J` |
| (1, 0x20) | consonant | every phoneme but the vowels, `p`, `Y`, `l`, `m`, `n` and the pause |
| (1, 0x40) | break | `! ) , . ? \ ]` |
| (2, 0x01) | closure | `B P D T t J C G K Q q` and the nasals |
| (2, 0x02) | vowel | the vowels and `p` |
| (3, 0x01) | approximant | `L R W Y h j l` |
| (4, 0x01) | high | `4 E U W Y b c h i u \|` |

A class can also be **negated**: the symbols not in it. A missing node is in no class, negated or not.

### 2.2 Walking the window

The stage walks the nodes of its window ([pipeline.md](pipeline.md) §2.3: there is no node after the window's end or before its start). From a node:

- its **next phoneme** is the first node of type 3 after it;
- its **previous phoneme** is the last node of type 3 or 4 before it;
- its **next sound** is the first node of type 3 after it that isn't a word mark, `%` or `&`;
- its **previous sound** is the last node of type 3 or 4 before it that is neither a word mark, `%` or `&`, nor in class break.

There is none when the window has none.

**Scanning** looks at up to `n` phonemes from the cursor's scan node, the **current** node: its next phonemes going forward, its previous phonemes going back. With a class to find and a class to stop at, it reports a find at the first phoneme in the class to find, and stops without one at the first phoneme not in the class to stop at; the scan stops without a find after `n` phonemes or at the window's end. (The durations also scan by stress.)

### 2.3 The context

Before it times a node and gives it its pitch, the stage notes about the current node:
1. its symbol, and whether it is in class affricate, fricative, nasal, phoneme, sonorant, closure, syllabic and vowel, and whether it is stressed;
2. its **next** node, its next phoneme; its **following** node, its next sound, and the following node's next sound, the **after following**; its **previous** node, its previous phoneme; its **preceding** node, its previous sound, and the preceding node's previous sound, the **before preceding**. Each one's symbol is noted too, a space when there is none;
3. whether the next node is stressed, whether the preceding symbol is in class syllabic, and whether the following symbol is.

## 3. The phrase

The stage scans a phrase before it times its phonemes. The pitch takes from the scan:

- the phrase's **phonemes** `N`, a count of the phrase's nodes in class phoneme;
- the **marks** `m6`, `m7`, `m8` and `m17`: for the first word marks of the phrase whose value is 6, 7, 8 and 17, the number of phonemes before each; −1 without one, but 0 for `m17`;
- the phrase's **contour**: 0, 6, 7, 8, 15, 16 or 19;
- the phrase's **kind**, the prosody cursor's phrase setting ([pipeline.md](pipeline.md) §3.1), which the `P` command nodes the scan puts in set. Its low byte is 1 for a falling phrase (at `.`, at `!`, or a question that starts with a question word), 2 for a phrase without an end of its own, 3 for a comma's or one the scan breaks off, 4 for any other question; the text's own `P` commands can set other values, and the pitch sets 3 or 6 (section 4.3). Its high byte is 1 for a phrase that goes on from the one before in the same sentence.

A word mark's value is 0 but in phonetic input. When input flag 0x20 is set (`ESC[6A`), `[` starts phonetic input, in its second mode while input flag 0x40 is set, as it is by default, and `]` ends it. In that mode a symbol right before the `]` gives the next word mark its value: `\` 6, `/` 7, `/\` 8, `{` 12, `_` 17. Without a mark of value 6, 7 or 8 the contour is 0; a phrase that ends at `!` has contour 19.

## 4. The pitch

### 4.1 The pitch's state

For a phrase the pitch keeps:
- the **position** `p`, the phonemes given a pitch so far;
- the **fall** `F`, the pitch where the phrase's final fall is, 0 before it starts;
- the **nucleus**, a node or none;
- the **following consonant**: 1 for a voiceless fricative, 2 a voiceless stop, 3 a voiced fricative or affricate, 4 anything else;
- the flags **rise**, **word end**, **phrase end**, **at nucleus**, **after stress**, **raised** and **lowered**.

Nothing reads the following consonant or word end; the stage keeps them all the same.

### 4.2 The pitch's context

Before a node's pitch, with the context of section 2.3:
1. At a word mark (class word mark), word end is cleared. Done.
2. At a break (class break), the fall becomes 0, after stress, at nucleus, phrase end and word end are cleared, and there is no nucleus. Done.
3. Only a syllabic node goes on.
4. **The nucleus.** Without one, the stage looks for it, from the current node to the end of the phrase the scan found, the cursor's first node, or the window's end. Each phoneme of stress 2 or 3 (a node of type 3 in class phoneme) becomes the nucleus. At the first node of type 3 in class break the search ends. If it reaches the phrase's end or the window's end before a break, there is no nucleus, whatever it found.
5. **The phrase's end.** If a scan forward over 9 phonemes finds class break before it meets class syllabic, phrase end and word end are set. Otherwise, if a scan forward over 10 phonemes finds class word mark before class syllabic, word end is set.
6. If the current node is the nucleus, at nucleus is set.
7. **The following consonant** is 4, unless the node `c` says otherwise: `c` is the next node, but if `c` is in class sonorant, `c` becomes its next phoneme, and goes back to the next node if that phoneme is in class phoneme but not in class sonorant. If `c` is in class voiced, the following consonant is 3 when `c` is in class fricative or affricate. Otherwise it is 1 when `c` is in class fricative, else 2 when it is in class stop. (So after a next node in class sonorant it is always 4: no sonorant is a fricative, an affricate or a stop.)

### 4.3 A phoneme's pitch

`B` is the prosody cursor's pitch in Hz, and `R` the pitch range of the cursor's voice, which the data pack gives ([datapack.md](datapack.md) §7). Then for the current node:

1. **A node that isn't a phoneme** (not in class phoneme): a word mark of value 12 sets raised, one of value 17 sets lowered; any other word mark clears lowered after stress outside the phrase's end (after stress set and phrase end clear). Done.
2. `p` goes up by 1. The node's character, byte 0x1a, becomes the speaking thread's character ([pipeline.md](pipeline.md) §1). A whispered node (character 1) gets pitch 0, a monotone one (2) the byte `B >> 1`. Done.
3. **Within the final fall** (`F` isn't 0): `F` goes down by 8, to 46 at the least, and the node gets `F >> 1`. Done.
4. **The excursion** `E` is the Q15 product of `B / 3` and `R`. With contour 19 it is 4 times that once `p` is above `m7`, 6 times before. If `N` is 0 it becomes 1.
5. **The declination** `D` by the contour, with `m` reached when `p` passes it: toward `m` is `E · (m − p) / m` up to `m` and `E · (p − m) / (N − m)` after it.

   | Contour | `D` |
   |---|---|
   | 0 | `E · p / N` |
   | 6 | toward `m6`; `m6` becomes `N` if it is 0 |
   | 7 | `E · p / m7` up to `m7`, `E · (N − p) / (N − m7)` after it |
   | 8 | toward `m8`; `m8` becomes `N` if it is 0. The phrase's kind becomes 6 |
   | 15 | as 19. The phrase's kind becomes 6 |
   | 16 | `E · (m6 − p) / m6` up to `m6`, `E · (p − m6) / (m7 − m6)` up to `m7`, `E · (N − p) / (N − m7)` after it. The phrase's kind becomes 3 |
   | 19 | `E · p / m7` up to `m7`, `E · (m6 − p) / (m6 − m7)` up to `m6`, `E · (p − m6) / (N − m6)` after it |

   The pitch `P` is `B + E − D`; with another contour, `B + E`. Raised makes it `P · 108 / 100`.
6. **The step.** `S` is the Q15 product of `P` and `R`, and the step `s = (S >> 2) − (S >> 4)`. While after stress is clear, the step loses `s >> 2` in a phrase that goes on from the one before or when lowered is set, and gains `s >> 4` otherwise. With a nucleus of stress 3 it is `s << 3`.
7. **Rise** is set when the following node has stress 2 or 3 and at nucleus is clear; or `p` is 1 and the current node is a stressed vowel (class vowel); or `m17` isn't 0, `p` is `m17` or more and phrase end is set.
8. If the following node is the nucleus and the phrase's kind isn't 6, `P` gains `s`, and `2s` more for stress 3.
9. By the current node's stress: at 1, `P` gains `s >> 1`. At 2 or 3, after stress is set; then if the node is the nucleus and the phrase's kind isn't 6, the node gets flag bit 0x80 and rise is cleared; otherwise `P` gains `s`, and `8s` more for stress 3.
10. With rise set, `P` gains `S >> 2`.
11. **The sounds.**
    - A phoneme not in class voiced gains 20 if it is `T`, `P` or `K`, the following node is in class vowel, and there is no preceding node or it is a space.
    - A phoneme in class voiced loses 3, and 5 more if it isn't in class sonorant, when it is the nucleus, or when it isn't stressed and has a following node that is neither in class vowel nor of stress 1 to 3. (The original also requires it not to be at once in class consonant, approximant and nasal, which no symbol is.)
    - `N` loses 55 when the preceding symbol is in class vowel and its node isn't stressed, the following symbol is `T` or `q`, the next node's next phoneme is a word mark, and the after following symbol is in class sonorant, or in class vowel with a stressed node (the vowels are sonorants, so the second never adds a case).
12. **Outside the phrase's last syllable** (phrase end clear), a phoneme in class high gains 3. The node gets `P` held between 50 and 500, shifted right by 1. Done. In the phrase's last syllable with lowered set, the node gets the same without the 3. Done.
13. **The final fall**, in the phrase's last syllable without lowered. By the phrase's kind, its low byte:

    | Kind | `P` |
    |---|---|
    | 1 | loses `S >> 4`, and 22 more at the nucleus for a phoneme in class vowel whose next symbol is in class break |
    | 3 | gains `(S >> 2) >> 3` |
    | 4, 5 | becomes `2B − (B >> 3)` |
    | others | as it is |

    `F` becomes `P` held between 50 and 500, and the node gets `F >> 1`.

### 4.4 Resetting the pitch

When a sentence starts (section 3), the pitch's state is reset: the following consonant becomes 4, `p` and `F` 0, there is no nucleus, and rise, word end, phrase end, at nucleus, after stress and raised are cleared. Lowered stays.

## 5. What the recorded runs don't reach

- A division by zero in step 5 of section 4.3: the original faults; the port gives 0.
- A reset of the pitch (section 4.4) with the fall, the nucleus or the flags it clears still set: a sentence starts after a break, whose pitch context already cleared them (section 4.2, step 2). Only a reset of the stages within a phrase would leave them set.
- A whispered phoneme that already has a pitch: the stages before leave a phoneme's pitch 0.
- Rise set by a mark of value 17 at exactly `p = m17` (section 4.3, step 7): the phoneme before the marked word was never in the phrase's last syllable.
- Contexts where the steps' details make no difference: at `p = m7` with contour 19 the declination cancels the excursion, so 4 or 6 times it gives the same pitch (section 4.3, steps 4 and 5); the contours' quotients agree where they meet at a mark; and the phoneme before a nucleus of stress 3 (step 8) was always at the 500 Hz limit.
- The nucleus search running past the window's end before the phrase's end (section 4.2, step 4).
- A phrase of 0 phonemes at a phoneme's pitch, and contour 6 with `m6` 0 (the scan doesn't choose contour 6 then); a contour other than those of section 4.3.
- A symbol of 128 or more: the port gives it no classes.
