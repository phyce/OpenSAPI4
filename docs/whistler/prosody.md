# SAPI 4 Microsoft engine: prosody

How the Microsoft Speech Synthesis Engine (`msttssyn.dll`) turns the transcribed phone string of a sentence into the units it hands to the voice: which words are accented, where the phrase breaks and pauses go, how long each phone is, the pitch contour, and the unit records with their pitch and lengthening. It follows [text.md](text.md) §9 (transcription), [rules.md](rules.md) (the rule interpreter) and [pronunciation.md](pronunciation.md) (the phones). What the voice does with the units (senones, real durations, speed) is in `units.md`.

**What the voice receives.** For each sentence, a list of unit records:
- **Phones:** a phone name with a stress digit (`EH1`, `L`), a duration code and one pitch in Hz, taken at the phone's end. The code is 1.0, or 1.2 for the rhyme before `,` `;` `:` `)`, or 1.3 for the last rhyme of the sentence. The voice multiplies the code by its own mean duration for the phone. **The analyser's phone durations never set a phone's length.**
- **Boundaries and pauses:** `#` between words, and `SIL` with a length in seconds for every pause. The sentence ends with `SIL` 0.36 s.
- **Tags** (`\Wrd=`, `\Mrk=`, `\Spd=`, `\Vol=`, `\Pau=`, `\Chr=`, `\Rst\`) at word boundaries.

The analyser's durations therefore matter only for:
- pause lengths;
- whether a glottal stop is dropped or becomes a short pause;
- the times at which the pitch contour is sampled;
- the word boundary each tag lands on.

Most of this step is rule data in modules A and B. The Go engine runs those rules from the data pack. What it must reproduce exactly is the code around them, described here: the natives, the stress, phrase-position, duration, unit and intonation stages, and the unit builder.

## 1. Pipeline

After transcription (text.md §9) the analyser state holds, for cells `start`..`end`:
- **T1:** phones with stress marks (`'` primary, `` ` `` secondary, `"` emphatic), `-` syllable breaks, spaces between words and `#` boundaries.
- **T2:** the spelling, aligned with T1.
- **T3:** part-of-speech tags (section 2.1).
- **T4:** chunk codes (section 2.2).
- **T5:** empty.

Analysis then runs these steps. Steps 1–5 work on local copies of `start`/`end`, which the rules may change; they are written back before step 9.

| # | Step | Reads → writes | Section |
|---|---|---|---|
| 1 | module A group 5 | T1 → the sentence flag | 3.1 |
| 2 | module A group 10 | T1, T3 → T5 labels, T1 | 3.2–3.4 |
| 3 | stress levels | T1, T3 → T3 | 4 |
| 4 | module A group 16 | all tiers → T1, T2, T4, T5; annotations → durations, pitch points | 3.5–3.7 |
| 5 | module A error flag set → drop the sentence | | rules.md §8 |
| 6 | phrase positions | T1, T4 → marks | 5.1 |
| 7 | durations | T1–T5, marks → durations | 5.2 |
| 8 | word breaks become `-` | T1 | 5.4 |
| 9 | analyser units | T1 → frame counts | 6 |
| 10 | duration floors | frame counts → durations | 6.3 |
| 11 | total duration | | 6.4 |
| 12 | event times | events, durations | 7 |
| 13 | intonation | T5, durations, pitch points → pitch targets | 8 |
| 14 | base pitch | pitch targets | 8.6 |
| 15 | analysis string | | 9 |
| 16 | unit builder (the FrontEnd) | analysis string, transcribed T1 → units | 10 |

Module B's phrasing (section 2) runs earlier, during transcription.

**Phone classes and features** used throughout. They come from two 256-entry tables in the data pack:

| Class | Members |
|---|---|
| V (vowel) | the 17 vowel codes of pronunciation.md §2, including `$` |
| C | `D S T Z f h j s v w z` |
| L (sonorant) | `N l m n r` (and code 0xa4, which never survives step 4) |
| P (voiceless stop) | `k p t` |
| Q (voiced stop) | `b d g` |
| a | `"` `'` `` ` `` |
| s | space, `#`, `-` |
| t | `~` |
| U | everything else, including `?` and 0x19 |

A **consonant** is any code whose class byte is at most `Q`: classes C, L, P and Q, and also the codes below 0x0e and from 0xb0, whose table entries are small numbers. Cell 0 holds 0x00, so it counts as a consonant when a walk reaches it.

A **vowel-like** code is class V, or `L` (0x4c) or 0xa4. Neither of the two reaches step 7, but the durations test for them (section 5.2).

| Feature | Members |
|---|---|
| 0x10 fricative | `f v s z T D Z` |
| 0x90 | `S`: fricative plus the function-word bit. Tests for "exactly 0x10" miss it |
| 0x01 approximant | `h j l r w` |
| 0x08 separator | space, `-`, `~` |
| 0x80 function-word tag | the T3 tags `d p P X x M c C +`. `d` and `p` are also the phones D and P |

## 2. Phrasing (module B)

Module B runs during transcription: group 16, then group 6, then group 3 (text.md §9). Most of it is part-of-speech disambiguation, which is rule data. This section covers what prosody depends on.

### 2.1 Part of speech (T3)

Tags come from the dictionaries' T3 fields (pronunciation.md §3), mostly the function-word dictionary. Module B then resolves ambiguities.

| Tag | Words | Kind |
|---|---|---|
| `d` | the, my, its, a, an, every | function |
| `p` | prepositions | function |
| `P` | it, me, him, them, myself, anyone; relative wh-words | function |
| `S` | I, you, he, she, they (and he's) | function |
| `X` | am, is, are, was, been, were | function, auxiliary |
| `x` | has, having | function, auxiliary |
| `M` | can, will, must, might, does, do | function, auxiliary |
| `c` | and, or, nor, than | function |
| `C` | if, unless, because, whether, although | function |
| `+` | infinitive "to" | function |
| `N V v R r J A u I Q *` | noun, finite verb, base verb, -ed form, -ing form, adjective, adverb, number, interjection, wh-word, unknown | content |

- **Unknown words.** A word whose T3 holds no tag gets `*` (group 16). Module A treats `*` as a content word.
- **Function-word tags** are exactly the tags with feature 0x80 (section 1). Step 3 and the durations use that bit.
- **`\Prt=`.** The user part-of-speech marker (`0x11 @x`, text.md §8.2) is applied by native 8 (section 2.4).

### 2.2 Chunks (T4)

Group 15 writes a chunk code on the first cell of each word that starts a chunk:

| Code | Chunk |
|---|---|
| `N` | noun phrase (including "X and Y" and "of" attachments) |
| `V` | verb group (including a subject pronoun or adverb before the verb) |
| `P` | prepositional phrase |
| `R` | relative or wh clause |
| `C` | subordinate clause |
| `p` | preposition without a noun phrase |
| space | word boundary |
| `~` | inside a chunk |

### 2.3 Phrase breaks (group 6)

Group 6 inserts phrase breaks, written as 0x19 followed by a space. Module A later turns each into a 10 ms pause (section 3.2). In order:
1. `\x11P<d>` → `#[d00]`, a fixed pause of d×100 ms. The FrontEnd never sends this marker.
2. A T4 boundary before infinitive "to".
3. A comma before a subordinating conjunction (after an optional coordinating one), except right after "and"/"or" + "if".
4. A break before a relative or wh chunk, unless the previous chunk is a verb group or the clause start (or a comma) is one word away.
5. A break between a relative chunk and a following verb chunk ("The man who came yesterday | left").
6. A sentence-initial subordinate or `-ing` clause is skipped up to its punctuation. Native 4 runs once on the rest.
7. Native 5 runs on the whole sentence.
8. Cleanup: the `B` markers of native 5 are deleted; a break after "as" before a verb is removed; every 0x19 is followed by a space.

**Counting words.** Natives 4 and 5 count words as the number of spaces in a span minus the number of 0x11 bytes in it.

**Punctuation set** for both natives: `, . ; ? ! : #` and 0x19. Native 5 also stops at `(` `)`.

**Native 4: subject breaks.** `words(a, b)` counts cells a..b−1.

```
for i := start; i <= end; i++ {
    if T4[i] != 'V' || T3[i] == 'S' { continue }
    j := i - 1
    for (T4[j] == ' ' || T4[j] == '~') && j > start && T3[j] != 'P' && T3[j] != 'S' && !punctuation(T1[j]) { j-- }
    before := words(j, i)
    k := i + 1
    for k <= end && !punctuation(T1[k]) { k++ }
    after := words(i, k)
    m := i
    if T1[i-1] == '~' { for m > start+1 { m--; if T1[m-1] != '~' { break } } }
    if T1[m-1] == ' ' && T4[j] == 'N' && before >= 3 && after >= 3 { T1[m-1] = 0x19 }
}
```

The subject must be the chunk right before the verb, a noun phrase (not a pronoun) of three or more words. Example: "The little old lady | sold the big house to me."

**Native 5: prepositional phrases.**
1. Every `?` not followed by a space becomes `%`.
2. Walk the cells. At a cell i where T1 is `B` and T4[i − 1] is `P`:
   1. The `B` comes from the T1 field of eight function-word entries: by, in, on, for, from, with, since, through.
   2. Walk left from i − 2 to the previous punctuation or the span start, giving the phrase start (skip one space).
   3. `before` = words from the phrase start to i; `after` = words from i to the next punctuation.
   4. `verb` = any of `V v R r x X M` in T3 among the three words before i.
   5. If T1[i − 2] is a space, there is no verb, `before` ≥ 2 and `after` ≥ 3: T1[i − 2] = 0x19, and continue after the next three spaces.
3. Every `%` becomes `?` again.
4. Walk backward. When a word's T4 is `N`, and the word before it starts with a T3 `X` (a form of "be") whose T4 is `V`, clear the `N` and the cell before it to `~`. "is a man": the predicate joins the verb chunk.

The first and last cells of the span are never examined; they are always `#`.

Example: "The old man | in the brown coat walked slowly …".

### 2.4 Natives 7 and 8

- **Native 7:** T1 = T4 over the span. It installs a homograph's second pronunciation.
- **Native 8:** applies a `\Prt=` marker (`0x11 @x`, its tag already copied to T3).
  1. Find the 0x11, the `@`, and the tag after any `~`.
  2. Let q be the first non-blank T3 cell from the tag + 2.
  3. **Tag `@`:** if T3[q] is `H`, copy T4 into T1 up to the `&` or space that ends the alternative. Stop.
  4. **T3[q] is not `H`:** T3[q] = tag, and T3 = `~` to the span end.
  5. **T3[q] is `H` with digit d** (a homograph):
     - d = 0 and tag `1`: clear both cells.
     - d = 0 otherwise: if the tag (or `v` for `V`) occurs later, set T3[q − 1] and T3[q] to the tag with `~` after. Otherwise run group 9 from q, then set the tag unless it is `2`.
     - d 1/2/3/8 with tag `N`, d 4 with `J` or `N`, d 5/6 with `J`, d 7 with `R`: run group 9 (the second pronunciation).
     - Otherwise set the tag.

   **Quirk:** q is computed 0-based but passed to native 7 and group 9 as a 1-based cell, one cell early. Reproduce it.

### 2.5 Glottal-stop candidates

`G` marks a place where a glottal stop may go. It has three sources:
- the T1 fields of vowel-initial function words (a, an, it, of);
- before a vowel-initial "be" or auxiliary after here, there, this, that, these, those, it or a wh-word;
- before a vowel-initial prepositional phrase after a verb ("looked up").

Module A decides what happens to each `G` (section 3.5).

## 3. Accents and connected speech (module A)

### 3.1 Group 5: the sentence flag

One rule looks for a space whose nearest neighbours on both sides are phones or `?`. The flag is 1 if it fires: "more than one word".
- `?` counts as a phone, so one-word questions ("Who?", "Hello?") get 1.
- "Hello." and "Yes" get 0.

A flag of 0 means citation timing: function words are not destressed (section 4), and the durations use their own branch (section 5.2).

### 3.2 Group 10: the driver

In order:
1. T5 = `~` over the sentence (native 4).
2. Unknown-tag cells of phonetic spans become 0x10. Punctuation tags other than `(` and `)` become `~`.
3. A `?` (glottal stop) not followed by a space is hidden as `%`. Group 16 restores it.
4. The sentence type is set on the first `#` (section 3.3). In a wh-question the last `?` then becomes `.`.
5. Native 3 runs once per word.
6. Group 9 places accent candidates (section 3.4).
7. Leftover `'` marks in T5 are removed.
8. A `.` is added before the last `#` that has no punctuation before it.
9. Punctuation followed by more punctuation is deleted.
10. Punctuation followed by a space, and `(` `)`, is copied to T5.
11. The sentence is split into clauses at that punctuation:
    - a clause ending in `. ! ?` → group 6;
    - a clause ending in `, : ; (` or 0x19 → group 7;
    - a parenthesis → group 8.
12. Cleanup and the extra labels:
    - in a fragment (`S`), the final `j` is removed;
    - remaining punctuation in T5 is removed (`( )` stay);
    - a comma between single-word items that are themselves between commas becomes `B` (lists of four or more);
    - `x` goes on the last phone of the syllable after a `1` accent, unless that syllable is already labelled. Phonetic spans are skipped;
    - every unlabelled vowel between a clause's `1` and its last accent (`l`, `c` or `F`) gets `u`.
13. `0x19 ` → `#[10]`.

### 3.3 Sentence types

Four rules on the first `#`; the first that applies wins:
1. **`\xad` (exclamation):** a `!` follows. The intonation stage never recognises this mark (section 8.2), so exclamations get the default type.
2. **`W` (wh-question):** the first or second tagged word is a wh-word (T3 `Q`) and a `?` follows.
3. **`q` (yes/no question):** a `?` follows.
4. **Guard:** stops here if some boundary has a phone on its left and a phone or `.` on its right. That holds for every sentence of two or more words and every sentence with a final `.`.
5. **`S` (fragment):** reached only when the guard failed, i.e. a single word without `. ! ?` ("Hello", "Yes", headings).

Ordinary statements get no mark, which is type 4 in section 8.

### 3.4 Accent labels (groups 6–9)

**Group 9: candidates.**
1. **Emphasis.**
   - An emphasis marker (`0x11 "`, from `\Emp\`) puts `"` on the stressed vowel of the next word.
   - So does an emphatic stress inside a phonetic span.
   - If any emphasis exists, nothing else is accented.
2. **Stressed vowels.** Every vowel after a `'` gets T5 `'`, and after a `"` gets `"`.
3. **Content words.** A content word's stressed vowel gets its own tag in T5 (`N V v R r J A u I Q *`). A phonetic span's stressed vowel gets `"`.
4. **Particles.** A main verb directly followed by in, out, up, off, back or down loses its accent ("look OUT").
5. **No content word.** If no content word exists, auxiliaries are accented as `"`, or failing that every stressed function word.

**Groups 6, 7 and 8** replace the tags in each clause with labels.

**Group 6: final clause.**
1. **Yes/no questions.** Walking back from the end, the last voiced segment gets `8`, or `7` if it is an accented vowel.
2. **Boundary.** The clause's last punctuation gets `Y` (high) after `7`/`8`, else `j` (low). `!` gets neither.
3. **Fragments.** Accents in a fragment become `h`; the first two become `5` and `a`.
4. **Wh-questions.** The wh-word right after `W` becomes `4`, or `H` when it stands alone before `j`.
5. **Accents.** The first content accent becomes `1`, and the last one before the end becomes `l`. Other non-verb accents become `9`, and main verbs become `T`.
6. **Single accent.** A `1` directly before the boundary or the question rise becomes `H`.
7. **Tails.** `l` becomes `F`, and `H` becomes `f`, when two or more vowels follow and the second is unaccented or the `8`.
8. **Leftover wh-words** lose their accent.
9. **Pronouns.** A pronoun's stressed vowel right before the next `1`, `H` or `f` gets `t`.

**Group 7: non-final clause.**
- The first accent becomes `1`, the last `c`, the others `9` or `T`.
- `, : ;` (not 0x19) becomes `b`.
- `1` or `4` right before `b` becomes `H`.
- Wh-words lose their accent, and pronouns get `t` as in group 6.

**Group 8: parenthesis.** Every accent inside becomes `p`. A `)` that ends the sentence becomes `]`.

**The labels** (section 8.2 gives the tone records each one triggers):

| Label | Placed on | Meaning |
|---|---|---|
| `1` | first content accent of a clause | prenuclear rise |
| `9` | other non-verb content accents | mid accent |
| `T` | main verbs | verb accent |
| `l` | last accent of a final clause | nuclear fall |
| `F` | `l` followed by two or more vowels | fall with a tail |
| `H` | single accent of a clause; lone wh-word | rise-fall |
| `f` | `H` followed by two or more vowels | rise-fall with a tail |
| `c` | last accent of a non-final clause | continuation |
| `b` / `B` | `, : ;` / list comma | continuation / list boundary |
| `j` / `Y` | final punctuation | low / high final boundary |
| `8` / `7` | last voiced segment of a yes/no question / that segment when it is accented | question rise |
| `4` | wh-word of a wh-question | wh accent |
| `t` | pronoun before the next accent | small accent |
| `h`, `5`, `a` | accents of a fragment | |
| `p` | accents inside a parenthesis | compressed accent |
| `(` `)` `]` | parenthesis cells | baseline shift |
| `x` | after a `1` accent | post-accent target |
| `u` | between `1` and the last accent | plateau |
| `S` `q` `W` `\xad` | the first `#` | sentence type |

Every label the rules write is used by the intonation stage, except `\xad`. The intonation stage also knows `2 3 A I L O P U X Z z`, which no rule writes. Only a `[i<mark>]` annotation can put them in T5 (section 3.7).

### 3.5 Group 16: cleanup and connected speech

In order:
1. Native 12 (section 3.6).
2. Delete 0x10, and 0x11 with one following `"`, `'` or `` ` ``.
3. Punctuation not followed by a vowel becomes `#`, so a glottal stop before a vowel stays.
4. `%` → `?`.
5. Spaces next to `#`, 0x16, `"` or other spaces are deleted. Stress marks are deleted.
6. "the" before a vowel becomes "thee" (`D$` → `Di`).
7. Native 13: connected speech (below).
8. **Degemination.** Of two identical consonants across a word or syllable break, the first is deleted and the second gets T2 `+` (lengthened ×1.8 in section 5.2). Also when the first carries an annotation. "than nothing" → `Dæ n^-TIN`.
9. An `x` left on a deleted cell moves to the phone before.
10. 0x16 runs → `#[20]`.
11. `-` next to `#`, `-` or `"` is deleted.
12. Any byte that is not a phone, `-`, a space, `#`, 0x19 or an annotation is deleted (`G`, `~`, `+`, digits, …).
13. Double `#` are merged; the one with a T5 label is kept.
14. `~` is deleted, and runs of spaces and `-` become one space.
15. `R` (flap) → `d[50]`.
16. A duplicated T4 chunk code after a pause-initial `?` is removed.
17. Every `[…]` annotation is parsed by native 11 (section 3.7).

**Connected speech.** Native 13 runs group 15 when the duration style is 1, else group 14. The style is 1 from the first `TextData` of the process on (section 11); only lexicon queries before any speech see group 14.

**Group 15 (normal speech):**
- **`G`:** all deleted.
- **"and":** `æn` before a consonant, kept after `# `.
- **Demonstratives:** "that", "this", "these", "those", when neither they nor the next word start a chunk, get T2 `*` (×1.3).
- **Glottal stops:**
  - `?` after a pause before a vowel;
  - none before "are", "is" or "am" after a vowel-final word, nor after "thee";
  - otherwise a `?` before **every** vowel-initial word. Most of these vanish later (section 10.3).
- **Links.** The space between certain consonant pairs across words becomes 0x19, a word boundary with no pause:
  - `t` + `d m h b D k`;
  - `d` + `? t`;
  - `n` + `s`;
  - `s` + `g k`;
  - `D` + `m`;
  - `g` + `b`;
  - `r` + `p`.
- **`h` after a link:** `h` after `t` 0x19 → `h[+50]`.

**Group 14 (duration style 0):**
- "I am" → `Ym` unless a pause follows.
- **"and":**
  - kept after `# `;
  - `æn` when it starts a chunk or follows `$` + consonants;
  - `?æn` after `$s `;
  - else `$n`.
- **Glottal stops:**
  - `?` after a pause before a vowel, where `G` becomes `?`;
  - none before a T4 `p` word;
  - `?` before a chunk-initial vowel;
  - other `G` deleted.
- **Flapping.** `t` becomes `R` (→ `d[50]`):
  - between a vowel and an unstressed vowel, across `-`;
  - word-finally after a vowel or `n` before a vowel-initial word.

  Example: "water" → `wA-d$r`.

### 3.6 Module A natives

| n | Native |
|---|---|
| 3 | every T3 cell of the span gets the span's **last** T3 byte that is not `~`. An all-`~` span stays `~`. "he's" (`S3X`, digit removed by module B) becomes all `X` |
| 4 | T5 = `~` over the span |
| 11 | annotations (section 3.7) |
| 12 | for each cell before the span end whose T1 is `"`, `'`, `` ` `` or `G` and whose T4 is not `~`: copy T4 to the next cell whose T1 is not `~`. It copies (the source keeps its value), spaces included. This keeps the chunk code on the first phone when the mark is deleted |
| 13 | group 15 if the duration style is not 0, else group 14 |

These replace the descriptions of natives 3 and 12 in rules.md §9.

### 3.7 Annotations

An annotation is `[…]` right after a phone and applies to that phone. Sources:
- **Rules:** `#[10]`, `#[20]`, `#[d00]`, `h[+50]`, `d[50]`.
- **Dictionary entries:** sixteen of them carry `[±n]`.
- **Phonetic input** (`ESC /`, lexicon spans). Inside these spans module C has already rewritten `(` → 0x1e, `)` → 0x1f, and `,` or a space → 0x1d, merging runs.

**Segments.** Native 11 splits the bracket contents into segments at 0x1d outside a 0x1e…0x1f pair, and before every 0x1e. Each segment is one item:

| Item | Meaning | Stored as |
|---|---|---|
| `n` | fixed duration n ms | n + 3000 |
| `+n` (any non-digit first byte other than `-`) | add n ms | n |
| `-n` | subtract n ms | −n |
| `(t,f)` (0x1e t 0x1d f 0x1f), optionally after `i` | pitch point: t ms after the phone's start, f steps (section 8) | a point record |
| `i<mark>` | tone mark | T5 of the phone = mark |

**Durations.**
- n is accumulated as `n×10 + (byte − '0')` over every byte, without checking for digits.
- It is clamped to ±2700 **before** the 3000 is added. A negative value is negated after the 32-bit accumulation, without wrapping again.
- The value is stored in the phone's duration cell. The native indexes the durations (and the point index) one cell below the phone's cell in its group's numbering, and the tone mark at the phone's own cell. Annotations are applied by group 16, which runs on the sentence with one pair of edges around it, so both land on the phone.
- Section 5.2 treats a stored value above 3000 as fixed, and adds anything else. So `[0]` (3000) adds 3 s.

**Pitch points.**
- Each number has an optional sign: a digit means positive, `-` negative, and any other byte is skipped as positive.
- The first number ends at a 0x1d or at the segment end. The second number starts two cells after the first number's last digit and ends at 0x1f or the segment end.
- At most 640 points per sentence; later ones are dropped.
- Each point is `{time t (unclamped, may be negative), value max(f, 1)}`. The phone's pitch-point index is set to the running point count.
- An `i` followed by 0x1e parses the point twice: first cut to one digit by the segment end, then whole. Reproduce it.

The whole `[…]` is deleted afterwards.

## 4. Stress levels

Between groups 10 and 16, T3 is rewritten with stress levels:

```
pending := ' '
for c := start; c <= end; c++ {
    switch T1[c] {
    case ' ', '-': pending = ' '
    case '"':      pending = 'Z'
    case '\'':     pending = 'P'
    case '`':      pending = 'S'
    }
    switch {
    case features[T3[c]] < 0x80 || sentenceFlag == 0:
        T3[c] = pending
    case pending == 'P' && polysyllabic(c):
        T3[c] = 'S'
    case pending == 'Z':
        T3[c] = 'Z'
    default:
        T3[c] = 'F'
    }
    if class(T1[c]) == V { pending = ' ' }
}
```

So `P` (primary), `S` (secondary) or `Z` (emphatic) runs from the stress mark through its vowel, and a space follows. In a function word `P` becomes `F` (function), or `S` in a polysyllabic one ("into", "before"); `Z` stays.

`polysyllabic(c)` is true when a `-` can be reached from c, in either direction, without crossing a space or `#`:

```
if c < end   { k := c; for { if T1[k] == '#' || T1[k] == ' ' { break }; k++; if T1[k] == '-' { return true }; if k >= end { break } } }
if c > start { k := c; for { if T1[k] == '#' || T1[k] == ' ' { break }; k--; if T1[k] == '-' { return true }; if k <= start { break } } }
return false
```

## 5. Durations

Integers throughout. Every `/` truncates toward zero, and every intermediate duration is stored as a signed 16-bit value.

### 5.1 Phrase positions

Each cell gets a mark from its syllable's position. Walk from `end` down to `start` with `current = ' '`:
- **A `-` cell:** current = ' '; remember it as the last `-`. The `-` cell itself keeps ' '.
- **A space cell:** look at T4 of the next cell, or of the cell after that when the next one is `~` or a space. current = `F` if that byte is `~`, else `P`.
  - So `P` = the next word starts a chunk, and `F` = the next word continues the chunk.
- **A `#` cell:** current = `#`.
- **After a space or `#`:** if a `-` was remembered, every cell from here + 1 through that `-` gets `I`. Then forget the `-`.
- **Any other cell** gets `current`.

Resulting marks:

| Mark | Cells |
|---|---|
| `#` | the last syllable of a word before a pause |
| `P` / `F` | the last syllable of a word before a word break |
| `I` | the first syllable, with its `-`, of a word of two or more syllables. The sentence's first word gets it only if cell `start` is `#` |
| space | medial syllables and the separator cells |

The step also sets two scales: `finalScale` = 120 and `durationScale` = 107 when the duration style is 1, else both 100.

### 5.2 Duration of a cell

`dur[p]` holds 0, or an annotation value (section 3.7). For each cell from `start` to `end`:

```
v := dur[p]
if v > 3000 { dur[p] = v - 3000; continue }       // fixed: nothing else applies
pre := v
c := T1[p]; mk := mark[p]
switch {
case c == '#':
    d := 150
    if p == start || p == end { d = 15 } else if T4[p] == '.' { d = 400 } else if T4[p] == ',' { d = 200 }
    dur[p] = d + pre; continue
case c == '?':
    d := 70
    if T1[p-1] == '#' { d = 15 }
    dur[p] = d + pre; continue
case features[c] == 0x08:                         // space, '-', '~'
    dur[p] = pre + pre; continue
}
inh, mn := inherent[c], minimum[c]
if (c == 'i' || c == 'U' || c == 'I') && T1[p+1] == 'r' { inh, mn = 180, 100 }
vowel := class(c) == V || c == 'L' || c == 0xa4    // vowel-like
k := 100
if mk == '#' {
    if vowel {
        k = 180
    } else if p > 2 {
        q := p - 1
        for mark[q] == '#' && consonant(T1[q]) { q-- }
        if mark[q] == '#' && class(T1[q]) == V { k = 180 }   // coda of a pre-pausal syllable
    }
}
if mk == '#' || mk == 'P' {
    if class(c) == L && class(T1[p-1]) == V { k = k * 7 / 5 }
} else if vowel {
    k = k * 3 / 5
}
if vowel && mk != 'F' && mk != '#' && mk != 'P' { k = k * 17 / 20 }
m := k
if vowel && polysyllabic(p) { m = k * 4 / 5 }
if consonant(c) && mk != 'I' { m = m * 17 / 20 }
switch T3[p] {
case ' ', 'F': mn = mn / 2; m = stressFactor(p) * m / 100
case 'S':      m = stressFactor(p) * m / 100
}
if class(c) == V || features[c]&0x01 != 0 {
    if f := followingFactor(p + 1); f != 0 {
        if class(c) == V && mk != '#' && mk != 'P' { f = f*3/10 + 70 }
        m = f * m / 100
    }
}
if f := clusterFactor(p); f != 100 { m = f * m / 100 }
d := (inh-mn)*m/100 + mn
if d < mn { d = mn }
next := T1[p+1]
if consonant(c) && T2[p] == '+' { d = d * 18 / 10 }
if features[c] == 0x10 && (next == '#' || next == 0x19) {
    if c == 'z' { d = d * 16 / 10 } else { d = d * 15 / 10 }
}
if (class(c) == P || class(c) == Q) && (next == '#' || next == 0x19) {
    d = max(d, stopFloor[c])                      // b 155, d 165, g 130, k 180, p 180, t 140
}
if T2[p] == '*' { d = d * 13 / 10 }
if sentenceFlag == 0 {
    d = d * 11 / 10
    if T1[p-1] == '#' {
        if c == 'm' { d = 75 } else if c == 'l' && next != 'A' { d = 130 }
    }
} else {
    switch T3[p] {
    case ' ', 'F':
        if mk == '#' && c == '$' { d = d * 75 / 100 }
    case 'P', 'Z':
        if strings.IndexByte("1FHcfl", T5[p]) >= 0 { d = d * 90 / 100 } else { d = d * 85 / 100 }
    case 'S':
        d = d * 95 / 100
    }
    if mk == 'P' || mk == '#' { d = d * finalScale / 100 }
    d = d * durationScale / 100
    if T1[p] == 0x19 { T1[p] = '#'; d = 0 }
}
d = rateScale * d / 32
dur[p] = d + pre
```

- **Inherent and minimum durations** per phone come from the data pack.
- **`rateScale` is always 32**, so the last scaling is a no-op. Speed is applied by the voice, not here (section 11).
- **0x19 links** become `#` only when the sentence flag is 1. Later cells see the change: the `?` test reads the cell before.

### 5.3 Factors

**Stress factor:**

```
f := 70
if vowelLike(T1[p]) && mark[p] == ' ' { f = 50 }
if features[T1[p]]&0x01 != 0 && T1[p] != 'h' && class(T1[p+1]) == V { f = 40 }
```

**Following-phone factor.**
1. Start at q = p + 1 and skip `-` and `~`.
2. If T1[q] is an approximant (`h j l r w`), step over it, skip `-` and `~` again, and look up the next phone in the **after-approximant** table.
3. Otherwise look up T1[q] in the **direct** table.
4. Both tables (data pack) are 0 except for:

| Next phone | Direct | After an approximant |
|---|---|---|
| space, `#`, `,`, `.` | 120 | 0 |
| `D Z v z` | 160 | 140 |
| `b d g` | 120 | 120 |
| `k p t` | 70 | 70 |
| `N m n` | 85 | 0 |

**Cluster factor:**

```
f := 100
q := p + 1
for T1[q] == '~' || T1[q] == '-' { q++ }          // may read past end
b := (T1[q] == ' ' && (mark[p] == '#' || mark[p] == 'P')) || T1[q] == '#'
for features[T1[q]] == 0x08 { q++ }
if class(T1[p]) == V && class(T1[q]) == V { f = 120 } else if consonant(T1[p]) && consonant(T1[q]) && !b { f = 70 }
q = p - 1
for (T1[q] == '~' || T1[q] == '-') && q > 2 { q-- }
b = ((T1[q] == ' ' || T1[q] == '=') && (mark[q-1] == '#' || mark[q-1] == 'P')) || T1[q] == '#'
for features[T1[q]] == 0x08 { q-- }
if class(T1[q]) == V && class(T1[p]) == V { return 70 }   // overrides the 120
if consonant(T1[q]) && consonant(T1[p]) && !b { if f == 100 { return 70 }; return 50 }
return f
```

The forward skips read past `end`. After group 16, cell `end + 1` repeats cell `end`, and cell `end + 2` is the edge byte 0x0e (rules.md §1). The port must keep those cells. The backward walks can reach cell 0, which holds 0x00, and the marks of cells before `start` read as 0.

### 5.4 Word breaks become `-`

1. The sentence is dropped if it holds only `#`, `~` and spaces:

   ```
   i := start - 1
   for i < end && (T1[i+1] == '#' || T1[i+1] == '~' || T1[i+1] == ' ') { i++ }
   drop := i == end                               // so start > end + 1 counts as not empty
   ```

2. Every space becomes `-`.
3. If cell `end` is `-`, `end` decreases by one, once.

From here on `-` separates words and syllables alike. The analysis string prints it without a duration.

## 6. Analyser units

The analyser splits the phone string into units of its own concatenative inventory. That inventory belongs to the analyser's original product; this engine never plays it. The units matter only for their frame counts, which set a minimum duration for each phone (6.3).

### 6.1 Tables (data pack)

- **Unit classes:** 41 phone codes (`#`, the vowels, the consonants, `?`, `$`, and so on) have a class 1–41; everything else is 0.
- **Pair table:** `[42][42]` 16-bit values indexed by the classes of two phones.
  - The low bits (value & 0x9fff) are a list index, which is also the unit when no longer string matches.
  - Flag 0x2000 and flag 0x4000 change the fallback (6.2).
- **Lists:** list i holds, for each extension length n = 1..3, a set of n-phone strings with their unit ids.
- **Frame counts:** per unit, one byte per phone. Units 1–1543 have 2 phones, 1544–1794 have 3, 1795–1842 have 4 and 1843–1883 have 5.

### 6.2 Finding a unit

`findUnit(pos)` returns a unit and moves `pos` to the unit's last phone. That phone is also the first phone of the next unit.

```
p := pos; mode := 0; var saved uint16; var list int16; var next int
if T1[p+1] != '-' {
    v := pair[unitClass[T1[p]]][unitClass[T1[p+1]]]
    if v&0x2000 != 0 { mode, saved = 1, v }
    list = int16(v & 0x9fff); next = p + 2
} else {
    v := pair[unitClass[T1[p]]][unitClass[T1[p+2]]]
    if v&0x4000 != 0 { mode, saved = 2, v }
    list = int16(v & 0x9fff)
    if list > 0 && list <= 0xa6 { list += 0x561 }  // the same pair across a '-'
    next = p + 3
}
newPos := next - 1
unit := 0
if list > 0 {
    for n := lengths(list); n > 0; n-- {          // longest first
        if end-next+1 >= n {
            if id := lookup(list, n, T1[next:next+n]); id > 0 { unit = id; newPos = next + n - 1; break }
        }
    }
}
if unit == 0 {
    switch mode {
    case 0: unit = list
    case 1:
        u := saved
        if u&0x4000 != 0 { u = 0 }
        unit = int16(u &^ 0x2000)
        if unit > 0 && unit <= 0xa6 { unit += 0x561 }
    case 2:
        if saved&0x2000 != 0 { unit = 0 } else { unit = int16(saved &^ 0x4000) }
    }
}
if unit == 0 { unit = defaultUnit }              // the ('#', 't') pair: 185
pos = newPos
return unit
```

- **Lookups are exact matches** of the n cells after the pair. The original binary-searches sorted ranges; the converter checks that this is the same for the shipped tables.
- **Unmatched string cases.** A unit found through a string ends on the string's last cell. Otherwise the unit ends on the pair's second phone.

### 6.3 Frames and duration floors

**Assigning units.**
1. Clear the unit id, frame count and closure count of every cell from `start` to `end`.
2. pos = start. While pos < end:
   1. u = pos; unit = findUnit(pos).
   2. Record the unit id at cell u.
   3. L = the number of extra phones from the string: pos − u − 1, or pos − u − 2 if cell u + 1 is `-`.
   4. For each non-`-` cell q from u to pos, with k = 1, 2, …:
      - fr = the unit's k-th frame count;
      - frames[q] += fr;
      - closure[q] = min(fr, 255) if T1[q] is one of `? b d g k p t`, else 0.

So a phone shared by two units sums both units' frames and takes its closure from the right-hand unit.

**Raising durations.** R = rateScale × 10 / 8 = 40, in tenths of a millisecond per frame. For each cell with frames ≠ 0:

```
if dur[p] <= 0 { dur[p] = 1 }
var a, b, c int
if T1[p] == '#' {
    a, b, c = 0, 0, frames[p]
} else {
    if closure[p] > frames[p]>>1 { closure[p] = frames[p] >> 1 }  // kept
    a = (frames[p] - closure[p]) >> 2
    if a == 0 { a = 1 }
    b = closure[p]
    if b == 0 { b = a }
    c = frames[p] - a - b
}
if c > 0 {
    stretch := (int(int16(dur[p]*10)) - (a+b)*R) / c   // dur*10 is a 16-bit product
    if int16(stretch) < 0 { dur[p] = ((a+b)*R + 5) / 10 }
}
```

- **What this does.** The onset quarter and the closure (or a second quarter) are fixed frames. A phone too short for them is raised to their length.
- **16-bit wrap.** Durations of 3277 ms or more wrap in `dur*10`. They are then "raised" to the fixed length, which for a `#` is 0.
- **The frame-warp table.** The original also writes a frame-warp table, which nothing reads. Only its size matters. Each cell with frames adds one entry for each of a, c and b that is above 0. Once 3,836 entries are written, every later cell with frames gets `dur[p] = frames[p] × R / 10` after the raise. That takes a word of about 1,280 phones, which only synthetic tests reach.

### 6.4 Total

The total is Σ dur[start..end], plus 200 × the sentence pause (default 2) when the sentence ended naturally. It sets only the sentence event's duration, which nothing downstream reads, so the Go engine leaves it out.

## 7. Event times

The reader queues event records (text.md §8): the sentence record first, then markers (`ESC N`) and words in reading order. It also counts per cell how many records are anchored there.
1. **Sentence anchor.** If the first record is the sentence record and cell `start` has no anchor, take one anchor from the first anchored cell and give it to `start`.
2. **Hand out records.** Walk the cells from `start` to `end` with time = 0:
   - hand out as many queued records as the cell's anchor count, each stamped with `time`;
   - if phone events are on (always) and the cell is not `-`, add a phone record (value = the phone code, duration = dur[p]);
   - then time += dur[p].
3. **Durations.**
   - A word's duration is the time to the next word record. The last word ends at the final time.
   - The sentence record gets time 0 and the total.
4. **Leftovers.** Records never handed out are discarded.

The unit builder uses only two kinds of record:
- the phone records' times (phone start times);
- the marker records' times (where tags go).

Times are passed on ×10 (0.1 ms).

## 8. Intonation

### 8.1 Pitch targets

- **Targets.** A target is `{flag byte, value int16, time uint16}`. Values are pitch steps of 1/48 octave; step 95 is the voice's own pitch (section 10.1).
- **Capacity.** There is room for 640 targets; tone targets stop being added at 620. Fill points, closing targets and explicit points are not limited (section 14).
- **Arithmetic.** Times and values are 16-bit, and wrap.

**Tone records (data pack).** Each tone label in T5 triggers one or two of 33 tone records. A record has:

| Field | Use |
|---|---|
| mark | flag of the first target |
| level | first target value = baseline + level |
| secondOffset | second target value = level + secondOffset + baseline (after the step) |
| duration | second target time = first time + duration; also the largest allowed backward shift |
| fromPhoneEnd | 1: first time measured from the cell's end, 0: from its start |
| timeOffset | added to that time |
| baselineStep | added to the moving baseline after the first target |

### 8.2 Labels → tone records

| Label | Records | Label | Records | Label | Records |
|---|---|---|---|---|---|
| `1` | 0 | `9` | 6 | `p` | 2, 14 |
| `2` | 4 | `A` | 13 | `t` | 23 |
| `3` | 2 | `B` | 17 | `u` | 25 |
| `4` | 3 | `F` | 18 | `x` | 27 |
| `5` | 1 | `H` | 4, 15 | `z` | 28 |
| `7` | 10, 5 | `I` | 3, 15 | `(` | 19 |
| `8` | 5 | `L` | 30 | `)` | 20 |
| `O` | 29 | `P` | 0, 9 | `]` | 21 |
| `T` | 24 | `U` | 26 | `a` | 10 |
| `X` | 31 | `Y` | 22 | `b` | 16 |
| `Z` | 32 | `c` | 8 | `f` | 0, 18 |
| `h` | 1, 9 | `j` | 7 | `l` | 11 |

- **Two records** for one label means two calls with the same cell.
- **Sentence type.** `S` → type 0, `q` → 2, `W` → 3; the last one in the sentence wins. With none the type is 4.
- **Exclamations.** The original compares `\xad` as a sign-extended value against an unsigned byte, so it never matches and exclamations get type 4. Reproduce it.
- **Everything else**, including `~` and spaces, is ignored.
- Record 12 is never used.

### 8.3 The passes

```
baseline := int16(0); stype := 4
// 1. explicit points become absolute
k := 0; t := uint16(0)
for p := start; p <= end; p++ {
    if n := pointIndex[p]; n != 0 {
        for ; k < n; k++ { point[k+1].time += int32(int16(t)) }
        k = n
    }
    t += uint16(dur[p])
}
// 2. tone targets
toneTime := uint16(0)
target[1] = {'0', 0, 0}; count := 1
for p := start; p <= end; p++ {
    switch m := T5[p]; m {
    case 'S': stype = 0
    case 'q': stype = 2
    case 'W': stype = 3
    default:
        for _, r := range records(m) { addTone(r, int(int16(dur[p]))) }
    }
    toneTime += uint16(dur[p])
}
// 3. explicit points
for k := 1; k <= pointCount; k++ {
    i := 1
    for i <= count && !(point[k].time < int32(target[i].time)) { i++ }
    insert {'*', point[k].value, uint16(point[k].time)} before target i; count++
}
// 4. closing targets (flag '*' if the last target's flag is '*', else '0')
for int(toneTime) - int(target[count].time) > 3100 { append {flag, target[count].value, target[count].time + 3000} }
if toneTime > target[count].time { append {flag, target[count].value, toneTime} }
// 5. declination (8.5), then times become deltas
```

**Step 5 deltas.**
- target[count + 1].time = toneTime.
- For k = 1..count:
  - apply the declination to target k;
  - then target[k].time = uint16(target[k+1].time × 10) − uint16(target[k].time × 10). Both products are 16-bit; a delta of 0 becomes 1.
- Finally target[count].time = 0.

### 8.4 Adding a tone

`addTone(r, cellDuration)`, with `last` = target[count].time:

```
if count >= 620 { return }
t := int(toneTime) + r.timeOffset
if r.fromPhoneEnd == 1 { t += cellDuration }
shift := 0
if t < 0 {
    shift = -t; t = 0
} else if int(last) > int(uint16(t)) {
    shift = (int(last) - int(uint16(t))) / 2
    if int(r.duration) < shift { return }                               // tone dropped, baseline unchanged
    t += shift
    if count >= 2 && uint16(t) < target[count-1].time { return }        // dropped
    target[count].time = uint16(t)                                      // the previous target moves to the midpoint
}
if uint16(t) > target[count].time {
    slope := (int(target[count].value) - int(baseline) - r.level) / int(int16(t-int(last)))
    anchor := last
    for int(anchor)+2500 < int(uint16(t)) {                             // a point every 2400 ms
        anchor += 2400
        append {'0', target[count].value - int16(int(anchor-last)*slope), anchor}
    }
    append {r.mark, baseline + r.level, uint16(t)}
} else {
    target[count].flag = r.mark                                         // merged: relabelled, value kept
}
baseline += r.baselineStep
append {' ', r.secondOffset + r.level + baseline, uint16(r.duration - shift + t)}
```

- **The second target** always lands at the original first time + duration.
- **Fill points.** The slope is in steps per millisecond and truncates to 0 for any real contour, so fill points copy the previous value.

### 8.5 Declination

```
partial := uint16(sum of dur[start+1 .. end-1])
L := int16(uint16(dur[start]) + partial)
divisor := 1
for L > 512 { L = L / 2; divisor *= 2 }
Q := uint16(int(partial) / divisor)
perThousand := uint16(1000 / divisor)
floor, slope := floors[stype], slopes[stype]                 // data pack: 76 80 80 80 80 / 30 14 10 8 8
ceiling := int(uint16(100-floor)) * int(perThousand)
rate := slope
if int(Q)*int(slope) > ceiling { rate = ceiling / int(Q) }
if baseline > 0 && int(uint16(baseline)) > int(uint16(rate))*int(Q)/int(perThousand) {
    rate = int(uint16(baseline))*int(perThousand)/int(Q) + 1  // Q == 0: see section 14
}
for k := 1; k <= count; k++ {
    if target[k].flag == '*' { continue }
    q := int(target[k].time) / divisor
    var d int
    if int(L) > int(uint16(q)) { d = int(uint16(int(L)-q))*int(uint16(rate))/int(perThousand) - int(baseline) + floor } else { d = floor - int(baseline) }
    target[k].value += int16(d)
}
```

What this means:
- The contour declines linearly to the type's floor (76 or 80 steps), at most `100 − floor` above it.
- The remaining time is measured to the end of cell `end − 1`.
- Tone values ride on the line, re-referenced to the final baseline.
- Explicit points (`*`) and closing targets copied from them get no declination.
- An `L` of 32768 or more (negative) disables the slope.

### 8.6 Base pitch

Every target except the last gets the analyser's base pitch added. It is always 0 here, so this step does nothing.

## 9. Analysis string

The analyser prints:

```
ESC '/' P0 '[' D0 { '(' T ',' F ')' } ']' { P '[' D ']' | '-' } ESC '/' ESC 'E' ' '
```

- **P:** a tier-1 byte, printed raw.
- **D:** its duration in ms (signed).
- **`-` cells** print without a duration.
- **The targets** are attached to the first cell:
  - F is the value;
  - T is a running sum of `uint16(delta) / 10` over the earlier targets. That is the absolute time while targets are in order and every gap is under 6554 ms.

Example ("Hello world."):

```
ESC/#[15(0,89)(33,88)(373,110)(477,105)(717,75)(1140,72)(1150,80)(1155,80)]h[63]E[68]-l[87]O[231]-w[83]e[180]r[121]l[81]d[211]#[15]ESC/ESCE␠
```

The original sizes its buffer by a budget (7 bytes per cell after the first, 13, and 10 per target), which very long durations can overflow. The Go engine just builds the string.

## 10. Units (the FrontEnd)

The FrontEnd reads the analysis string back and builds the unit records. It also uses a copy of the **transcribed** T1, the tier as it was after module B, with stress marks and punctuation.

### 10.1 Pitch steps

The voice's pitch in Hz (the mode's header pitch, `PitchSet`, `\Pit=`) becomes:

```
pitchSteps = trunc(log(Hz * 0.02) * 48.0 / log(2.0))
```

The computation is in double, but msvcrt's `log` leaves its result in the x87 register at 64-bit precision, and the division uses the second `log(2.0)` unrounded. The result is the exact `trunc(48 × log2(Hz / 50))` for every Hz except some where Hz / 50 is a power of two. There the exact value is a whole number, and the original lands one step lower, or for 25 Hz one higher:

| Hz | 25 | 100 | 200 | 800 | 12800 |
|---|---|---|---|---|---|
| exact | −48 | 48 | 96 | 192 | 384 |
| original | −47 | 47 | 95 | 191 | 383 |

The other powers (50, 400, 1600, 3200, 6400, 25600 and 51200 Hz) give the exact value. This was checked against the original engine for every Hz from 1 to 65535. A pitch must lie between half and twice the mode's default, so only 100 and 200 Hz are reachable: Sam's default pitch, and any voice set to them. A plain double computation is off by one at other powers of two as well (it gives 143 at 400 Hz and 287 at 3200 Hz), so the Go engine finds the exact value with integers, comparing Hz⁴⁸ with 50⁴⁸ · 2ⁿ, and takes the five exceptions from the table.

The Hz of a step value f is:

```
hertz(f) = pow(2.0, (f - float64(95 - pitchSteps)) * 0.020833333333333332) * 50.0
```

So step 95 is 50 × 2^(pitchSteps/48) Hz, just below the voice's pitch (113 Hz → 56 steps → 112.25 Hz). Examples: 169 Hz → 84, 120 → 60, 100 → 47, 75 → 28, 200 → 95.

### 10.2 Pitch at each phone

1. **Points.** Collect every `(T,F)` up to the first `]`, then add a terminator `{0x7fffffff, 0}`.
2. **Phones.** Scan the whole string; stop at the first byte ≤ 0x14 (NUL, or a stray control code). At each `[`, the byte before it is a phone and the number after it its duration. A phone's end time is the running sum of durations. `-` has no `[` and is skipped.
3. **f0 at each phone's end.** For each phone i, move j forward while `points[j+1].time < end[i]`. Then:

   ```
   span := float32(float64(points[j+1].time) - float64(points[j].time))
   frac := (float64(end[i]) - float64(points[j].time)) / float64(span)
   f0[i] = float32((1 - frac)*float64(points[j].value) + float64(points[j+1].value)*frac)
   ```

   After the last target this interpolates toward the terminator. The x87 masks a division by zero, so a span of 0 gives an infinite or NaN pitch instead of a fault, as float64 division does.

### 10.3 Building the units

The builder walks the transcribed T1 (`c`) in step with the final phones (`f = phones[i]`).

**State.**
- `current = following = 110.0`, a raw Hz value used before the first matched phone.
- `inBoundary = false`, `stressed = false`, `afterR = false`.
- `tag = 0`.

**Each step:**
1. **Final `#`, or a `?` whose duration is not exactly 70:**
   - emit `#` unless inBoundary, then `SIL` (dur × 0.001 s) and `#`;
   - set inBoundary and i++;
   - **don't** advance the cursor, and don't place tags.
2. **Any other `?`** (a 70 ms glottal stop): i++ and nothing else. Most glottal stops of section 3.5 vanish here.
3. **Alignment:**

   | Transcribed `c` | Final `f` | Action |
   |---|---|---|
   | same as `f` | | current = hertz(f0[i]); following = hertz(f0[i+1]) if there is a next phone; i++ |
   | `R` | any | current = hertz(f0[i]); i++ |
   | `t` | `d` | c = `R` (also written into the transcribed copy); current = hertz(f0[i]); i++ |
   | 0x91 | `$` | c = `$` (written back); current = hertz(f0[i]); i++ |
   | `$` | `i` | c = `i` (written back); current = hertz(f0[i]); i++ |

   A transcribed phone with no final counterpart keeps the previous pitch.
4. **Then by c:**

   | c | Action |
   |---|---|
   | `# . , ; ( ) : !` | emit `#` unless inBoundary (inBoundary is **not** set). In the write pass, `, ; : )` lengthen the rhyme before (×1.2) |
   | 0x16, space, `?` | emit `#` unless inBoundary; set inBoundary |
   | `~`, `` ` ``, `%`, `-`, `M`, `G`, 0x0e, 0x10, 0x11, 0x19, 0x9d | nothing |
   | `"` `'` | stressed = true |
   | `t` followed by `S` / `d` followed by `Z` | current = following; emit `CH` / `JH`; inBoundary = false; i++ and skip the second letter |
   | `$` followed by `r` | current = following; emit `ER1` if stressed else `ER0`; inBoundary = false; afterR = true; stressed = false |
   | anything else | name = the phone's name (pronunciation.md §2). No name: advance and skip the tag check. A name starting with `R` right after `ER` is dropped. Otherwise emit it with pitch current (vowels get `1` if stressed else `0`, and clear stressed; `$` always gives `AH0`); inBoundary = false. afterR = the name's second letter is `R` |

5. **Tags.** If inBoundary, emit `\<n>` for every pending tag whose time ≤ the next final phone's start time (both in 0.1 ms), in order.
6. Advance the cursor.

**At the end:**
1. emit `#` unless inBoundary;
2. emit `SIL` 0.36 s, unless the text was a single character;
3. emit `#`;
4. in the write pass, lengthen the last rhyme ×1.3.

**Lengthening** walks back from the unit before the last `#`:
- every unit that is not `#`, a tag or `SIL` gets the duration code;
- it stops after the first vowel unit (a name starting with A, E, I, O or U);
- it also stops at a `#` once something was marked.

**Two passes.** The builder runs twice: once to count and once to write. The rewrites of the first pass (`t` → `R` and so on) are already in the copy the second pass reads.

**Quirk.** A final phone that module A inserted, other than `#` and `?`, stalls `i` for the rest of the sentence. Later units then keep a stale pitch, and later pauses produce no `SIL`. Reproduce it.

Example (the same sentence, a 113 Hz voice), as name, code, Hz:

```
# · SIL 0.015 · # · HH 1 105.81 · EH0 1 112.75 · L 1 122.30 · OW1 1 130.86 · # · W 1 114.29 ·
ER1 1.3 84.00 · L 1.3 82.28 · D 1.3 80.52 · # · SIL 0.015 · # · SIL 0.36 · #
```

### 10.4 Unit record

| Field | Phone | `SIL` | `#` | tag |
|---|---|---|---|---|
| name (16 bytes) | name + stress digit | `SIL` | `#` | `\<n>` |
| duration (float32) | code 1.0, 1.2 or 1.3 (float32 constants) | seconds: float32(dur × float32(0.001)); final float32(0.36) | 0 | 0 |
| point count | 1 | 0 | 0 | 0 |
| positions[5] | [0] = 1.0 | 0 | 0 | 0 |
| pitches[5] (Hz) | [0] = float32(current) | 0 | 0 | 0 |
| amplitudes[5] | 0 (default) | 0 | 0 | 0 |

### 10.5 Posting

Before the units go to the voice:
- **Tags.** Each `\<n>` unit becomes its tag from the control table (text.md §3.4): `\Wrd=v\`, `\Mrk=v\`, `\Chr=v\`, `\Spd=v\`, `\Vol=v\`, `\Rst\`, or `#` `\Pau=v\` `#`.
- **Unplaced tags.** Tags the builder did not place are appended after the last unit, in order.
- **Monotone** (`\Chr="Monotone"\` or a monotone variant): every pitch becomes the voice's current pitch in Hz, unquantised. Switching and timing: [effects.md](effects.md) §6.
- **No units.** A sentence that produces no units loses its tags.

## 11. Controls

| Control | Path | Effect |
|---|---|---|
| Voice pitch: mode header, `PitchSet`, `\Pit=` | pitchSteps (10.1). `\Pit=` ends the sentence and applies from the next one; out-of-range values (outside half to twice the default) are ignored; `\Pit=0\` resets | transposes the whole contour |
| Speed: `SpeedSet`, `\Spd=` | not used by the analyser (its rate stays 32). `\Spd=` becomes a tag unit; the voice changes speed from that word on (`units.md`) | phone and pause lengths; pitch targets are untouched |
| `\Pau=n\` | tag unit → `#` `\Pau=n\` `#`; the voice inserts n ms of silence, not speed-scaled | a pause at that word boundary |
| `\Emp\` | `0x11 "` before the next word → module A group 9 (only that word is accented) | the emphatic word's accent; others lose theirs |
| `\Vol=n\` | tag unit; the voice changes gain from that word on | none here |
| `\Rst\` | resets pitch (and ends the sentence), and becomes a `\Rst\` unit: the voice resets speed, volume and effects | |
| `\Prt=` | `0x11 @x` → module B native 8 | part of speech, so accents and homographs |
| `\Chr="Monotone"\` | posting (10.5) | flat pitch |
| Duration style | `ESC A P` at the start of every `TextData` sets 1; nothing sets 0. It is process-wide and starts at 0 | group 15 vs 14, the 107/120 scales |

The analyser's own rate, base pitch and speaker table (`ESC R`, `ESC S`, `ESC I`) are never reached: the FrontEnd turns a user's ESC into a space and handles `\Pit=` itself.

## 12. Events the application sees

- **Analyser events stay inside.** They are used only to place tag units.
- **`WordPosition` and `BookMark`** carry the audio position at which the voice reaches the `\Wrd=`/`\Mrk=` unit. That position is the sum of the **real** synthesised durations before it (`units.md`).
- **`Visual`** comes from the voice, one per synthesised frame (units.md §11). The analyser's phone times play no part.

## 13. Shared state

The original keeps some of this in DLL globals, shared by every voice in the process and by lexicon queries:
- the duration style;
- the two duration scales;
- the rule modules' saved-tier registers.

The registers matter: a rule in group 6 depends on a register set earlier in the same sentence. If no rule set it, it holds whatever an earlier sentence left (rules.md §11). The Go engine keeps these in one process-wide state and never resets them between sentences.

## 14. Quirks

**Reproduce these.** Each one changes the units:
1. A stored duration of 3000 (`[0]`) is added, not fixed. Fixed values skip every other rule, rate scaling included.
2. Separators with an existing duration get twice that duration; `#` and `?` add theirs.
3. Every intermediate duration is 16-bit, and every division truncates. `dur*10` in section 6.3 is a 16-bit product.
4. Fricative lengthening tests feature exactly 0x10, so SH is never lengthened before a pause.
5. Only a glottal stop of exactly 70 ms disappears. One after a pause (15 ms, or 20 after section 6.3) becomes a short `SIL`.
6. Exclamations get the statement type. Wh-questions lose their `?` and decline like statements.
7. One-word questions count as more than one word.
8. Target times and values are 16-bit and wrap.
9. Pitch and duration codes are float32 values computed in double (x87 at 53-bit precision).
10. The transcribed copy is rewritten in place by the count pass.
11. Unit lookups overlap: shared phones sum frames, and the right-hand unit sets the closure.

**Don't reproduce:**
- **Divide by zero.** In section 8.5, Q is 0 with a positive final baseline when a sentence of one or two cells ends on a net upward step (`)`, `]` or `b`). The original faults; we keep `rate` unchanged.
- **Target overflow.** Tone targets stop at 620, but the fill points of the last tone, the closing targets and up to 640 explicit points can go past the original's fixed array. We keep a list without a fixed size.
- **Reads past the builder's arrays.** When tier 1 holds codes the transcribed copy doesn't account for, the builder can read a phone, a pitch or a phone start time past its arrays. We read the end of the phone string (0), the last pitch and the padding time instead. A malformed analysis string, which the analyser never prints, keeps the previous point values, as `sscanf` leaves them. No text reaches either.
- **Analysis string buffer** (section 9): we have no fixed buffer.
- **Dead code:**
  - the frame-warp table;
  - the total duration ×10;
  - the second phrase-position mode;
  - the anchor merge for a non-sentence first record;
  - the duration of a second sentence record;
  - the alternative prosody model the FrontEnd never enables;
  - the analyser base pitch;
  - the speaker table.

## 15. For the data pack

The data pack carries these tables in `prosody.json` ([datapack.md](datapack.md) §8):
- **Phones:** the phone class and feature tables (256 entries each), and the inherent and minimum durations (per phone code).
- **Factors:** the two following-phone factor tables (per phone code).
- **Units:** unit classes, the pair table, the lists with their strings and unit ids, and the frame counts.
- **Intonation:** the 33 tone records, and the declination floors and slopes (5 each).

**Checks:**
- that every list's strings are unique and sorted, so an exact-match lookup equals the original's binary search;
- that every string's unit id lies in the id range of its length;
- that every per-phone table entry the phone alphabet can reach is the one the original reads. Some of these tables are shorter than the codes that index them, so a few codes read the data that follows the table.

## 16. Open questions

The answered ones were settled against the original's audio and `Visual` events, through its COM interface. They matched this spec bit for bit on all five voices.
1. **Internal pauses.** Section 5.2 reads T4 at a `#` cell for 400 or 200 ms. Module A's rules never leave `.` or `,` there, so every internal pause would be 150 ms. *Answered:* "Yes, I know that." pauses 150 ms after the comma.
2. **Cells past the end** after group 16 (`end + 1` = copy of `end`, `end + 2` = 0x0e). The cluster factor reads them.
3. **Links and short pauses.** "Come in seven days." should give a 1 ms `SIL` for the link. "Oh, I see." should give a 20 ms `SIL` for the glottal stop after the comma. *Answered:* both do.
4. **`log` and `pow`.** pitchSteps for 100 Hz and 200 Hz voices depends on `log(2)·48/log(2)` being exactly 48 in the original's maths library; one ulp less gives one step lower. *Answered:* it is one step lower at 100 and 200 Hz (10.1). `pow` matched at every pitch tried: each mode's default, minimum and maximum, and 150 Hz.
5. **x87 precision** on the FrontEnd thread: assumed 53-bit. *Answered:* every case compared is bit-identical with this spec, which assumes 53 bits.
6. **First tag.** Does the transcribed T1 start with `#`? That decides whether the first word's `\Wrd=` lands before or after the first `SIL`. *Answered:* after. The first word's `WordPosition` comes at the end of the first `SIL` (sample 330 for Mary at the default speed), and the audio matches.
7. **Control codes** in the transcribed or final T1 (`\Emp\`, `\Prt=`, inline phonetics): would they cut the analysis string or change a stress digit?
8. **Target order.** Can a tone starting before 0 produce targets out of order?
9. **Exclamations and fragments.** Confirm "Stop!" ≡ "Stop." apart from the final `j`. Confirm the sentence flag for "Hello?" (1), "Hello." (0) and "Hello" (0, type `S`). *Answered:* "Stop!" and "Stop." give the same audio, and all three "Hello"s match.
10. **The saved-tier register** for the question rise: the first question after the engine loads ("Is it raining?"). *Answered:* it matches, spoken as the first text of a fresh engine.
11. **Annotations from users.** Can any input path put `[…]` inside phonetic text? If not, the tone records only annotations reach, and `[0]`, are dead.
12. **Module B breaks.** Confirm "The old man in the brown coat walked slowly along the side of the road." gets a break only before "in". *Answered:* it does.
13. **`\Btp=` text in posting.** Posting may hand a Btp text that contains `<` to Microsoft's phonetic converter (`STPhoneConv`). Find whether text can reach that path and what the converter does. Until then, Btp entries do nothing (text.md §3.3). *Answered:* text reaches it. The converter is part of the SAPI 4 runtime, and only its absence crashes the front end. It is a prosody transplant, now specified and checked against the original ([engine.md](engine.md) §13).
