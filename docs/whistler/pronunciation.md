# SAPI 4 Microsoft engine: pronunciation

How the Microsoft Speech Synthesis Engine (`msttssyn.dll`) decides the phones of each word. It builds on [text.md](text.md) (how text reaches the analyser) and [rules.md](rules.md) (the rule interpreter). What happens to the phones afterwards (stress realisation, durations, intonation, units) is in `prosody.md` and `units.md`.

A word's pronunciation comes from the first of these that knows it:
1. the user lexicon, then 160 built-in exceptions (section 6);
2. ten dictionaries, which the rules look up (section 3). The main one has 31,617 words;
3. morphology: split off suffixes and look up the stem (section 4);
4. letter-to-sound rules (section 5);
5. spelling out letter by letter.

Steps 2–5 are driven by rule module C (rules.md). The dictionaries and the rules are data: the converter copies them into the data pack, and the Go engine looks them up and runs them. What the Go engine must get exactly right is the behaviour around them: key validation, which tiers a lookup writes, padding and the span's new end.

## 1. Order of decisions

**In the sentence reader,** for each whitespace-delimited token (text.md §8.4):
1. The user lexicon, then the built-in exceptions.
2. A hit becomes a phonetic span bracketed by 0x10. Every rule module leaves such a span alone, so a lexicon word is never normalised, looked up or run through letter-to-sound.

**In module C, group 50.** One decision per token. The first that applies wins:

| # | Token | Result |
|---|---|---|
| 1 | punctuation already marked in T3 | kept |
| 2 | a phonetic span (lexicon, `ESC /` input, normaliser strings) | kept |
| 3 | part-of-speech and pause markers | kept |
| 4 | normaliser pronunciations (bracketed by 0x92) | kept |
| 5 | the token as typed | main dictionary |
| 6 | Roman numerals in set phrases ("World War II") | rewritten |
| 7 | capitalised or upper-case words | acronyms and names (group 49: upper-case abbreviations, capitalised names) |
| 8 | phone numbers | group 39 |
| 9 | numbers after a month | dates (group 37) |
| 10 | any token | case folding (group 44): marks runs of capitals in T3, lowercases T1 and T2, splits punctuation. Then the next members try the lowercased token |
| 11 | the lowercased token | main dictionary |
| 12 | letters with `& ' - . / \ _`, or consonant strings | abbreviations |
| 13 | one character other than `a` or `i` | abbreviations (character names) |
| 14 | dotted initials `a.b.` | spelling (group 29) |
| 15 | sequences of 2–3 words | phrases |
| 16 | any token | function words |
| 17 | consonant-only letters | spelling |
| 18 | digits | numbers, money, units (groups 38/39) |
| 19 | letters (including accented letters and `& ' - . / \ _`) | group 33 (below) |
| 20 | anything else | spelling |

This is a summary. The exact order, with the punctuation members, is rule data.

**Group 33,** for one word:
1. Split at `-` and `&` into `#…#` parts.
2. For each part: the main dictionary, then function words.
3. A single letter or symbol: abbreviations. A consonant-only part: spelling.
4. Plural retry (group 24, section 4.3).
5. Morphology (section 4).
6. Letter-to-sound (section 5).
7. Possessive `'s`.

## 2. Phone alphabet

Tier 1 holds one byte per phone, except `dZ` and `tS`, which take two cells. Units are named from this table (`units.md`). Vowels get stress `1` or `0` there.

| T1 | Unit name | Engine phoneme | IPA | Example |
|---|---|---|---|---|
| `a` | AA | aa | ɑ | car, hot |
| 0x91 | AE | ae | æ | cat |
| `^` | AH | ah | ʌ | cut |
| `$` | AH (unstressed) | ax | ə | reduced vowels |
| `A` | AO | ao | ɔ | talk |
| `o` | AO | ao | ɔ | before r: for, war |
| `@` | AW | aw | aʊ | out |
| `Y` | AY | ay | aɪ | kind |
| `E` | EH | eh | ɛ | bed |
| `e` | ER | er | ɚ | her; always followed by `r`, which the unit drops |
| `J` | EY | ey | eɪ | make |
| `I` | IH | ih | ɪ | sit |
| `i` | IY | iy | i | see |
| `O` | OW | ow | oʊ | phone |
| `y` | OY | oy | ɔɪ | boy |
| `U` | UH | uh | ʊ | book |
| `u` | UW | uw | u | food |
| `p b t d k g` | P B T D K G | same | | |
| `f v s z h m n l r w` | F V S Z HH M N L R W | same | | |
| `T` `D` | TH DH | th dh | θ ð | thin, the |
| `S` `Z` | SH ZH | sh zh | ʃ ʒ | ship, measure |
| `N` | NG | nx | ŋ | sing |
| `j` | Y | y | j | yes |
| `dZ` `tS` | JH CH | jh ch | dʒ tʃ | judge, church |
| `R` | DX | dx | ɾ | flap |
| `?` | none (glottal stop) | – | ʔ | |

Other T1 symbols:
- **Stress:** `'` primary, `` ` `` secondary, `"` emphatic primary.
- **Boundaries:** `-` syllable, space word, `#` pause.
- **Annotations:** `[+30]` and `[-50]` change a phone's duration (`prosody.md`).

These symbols are used during transcription and removed before units (`prosody.md`):
- `F B C P Q K`: vowel placeholders inside letter-to-sound (→ `a e i o u y`);
- `X`: prefix and plural marker;
- `P`, `F`: plural and irregular-plural endings;
- `G`: glottal-stop candidate;
- `B`: preposition mark;
- `M`: month;
- 0x9d: state abbreviation;
- 0x9e: ordinal;
- 0x10, 0x92: phonetic spans;
- 0x11: escapes;
- 0x16, 0x19: short boundary, phrase break;
- `%`;
- `~`: empty cell.

## 3. Dictionaries

### 3.1 The ten dictionaries

| Procedure | Dictionary | Entries | Key bytes | Length | Fields → tiers | Flag |
|---|---|---|---|---|---|---|
| 22 | main | 31,617 | lowercase letters, digits, `' - . / &`, a few accented letters | 1–31 | T1 `!` T3 | – |
| 21 | function words | 542 | `' -` a–z à | 1–15 | T1 `!` T3 `!` T4 | function word found |
| 25 | prefixes | 206 | letters | 1–7 | T1 (the whole entry) | – |
| 26 | suffixes | 576 | letters | 1–10 | T1 `"` T3 `"` T4 | – |
| 27 | abbreviations, units, character names, lowercase Roman numerals | 549 | printable | 1–8 | T1 `!` T3 `!` T4 | – |
| 34 | uppercase abbreviations, state codes, uppercase Roman numerals | 321 | `A–Z - . a` 0x9d | 1–7 | T1 `#` T3 | uppercase abbreviation found |
| 35 | capitalised names | 380 | `& '` letters | 1–12 | T1 `!` T3 `!` T4 | – |
| 36 | phrases | 55 | lowercase letters, space, a few capitals | 1–14 | T1 `!` T3 | – |
| 53 | homographs | 492 | letters | 1–15 | **T3** `#` **T1** `#` T4 | homograph found |
| 54 | verbs | 5,768 | `' -` a–z é | 1–18 | T1 `!` T3 `!` T4 | verb found |

The field separators are the bytes shown (`!`, `"`, `#`). "Length" is the focus length in cells that the lookup accepts.

### 3.2 Lookup

A lookup gets the focus span `[s, e]` from the rule's CALL (rules.md §7). All positions are cells.

1. **Fail** (return false, change nothing):
   - if the length is outside the dictionary's range;
   - if any T1 byte in the span is below 0x21 (0x20 for phrases) or is 0xff;
   - if the bytes are not exactly one of the dictionary's words.

   Matching is exact and case-sensitive, so `The` never matches `the`.
2. **On a match,** the entry's fields are written from cell `s` into their tiers (3.1), with no length limit.
3. **Let** `end(k)` be the last cell written in tier k. A tier with an empty field gets `s − 1`. A tier with no field at all keeps `e`.
4. **The new end** is `m = max(s, e, end(T1), end(T3), end(T4))`. The span never shrinks.
5. **Padding:** in T1–T4, the cells from `end(k) + 1` to `m` become `~`. So:
   - T2 keeps the spelling over `[s, e]` and gets `~` beyond it;
   - a tier with no field keeps its contents over `[s, e]` and gets `~` beyond;
   - a tier with an empty field becomes `~` over the whole span.

   T5 and the anchor counts are not touched. Cells added beyond `e` keep whatever the parked tail left in T5, and their anchor counts are 0.
6. **Return** true; the span's end is now `m`.

The four flags are set to the result on every call, including early failures. Morphology reads the homograph and verb flags (section 4). The other two are never read, and the Go engine doesn't keep them.

A span of length 0 (a CALL whose focus matched nothing) is never found: the main dictionary's search walks past it without reaching a word end.

**In the original,** the main dictionary is a Huffman-coded word graph and the others are binary searches over bit-packed keys. Matching depends only on the focus bytes, so the Go engine uses a plain map from word to entry, built from the data pack. The converter checks that this map gives the same result as the original search for every word, and that no key can be matched by a word that isn't in the list (section 8).

### 3.3 Field contents

- **T1:** the pronunciation. Stress marks come before the syllable, and `-` separates syllables.
- **T3:** a part-of-speech code (one per word part):
  - `N` noun, `J` adjective, `A` adverb;
  - `V` verb, `v` base verb, `R` past or past participle, `r` `-ing` form;
  - `P` pronoun, `p` preposition, `d` determiner (`d1` singular, `d2` plural);
  - `C`/`c` conjunctions, `Q` wh-word, `M` modal;
  - `X`/`x` be/have forms, `S<n>` subject pronoun of person n;
  - `u` number word, `*` proper name, `I` interjection, `.` abbreviation;
  - `\` separates alternatives (`N\V`); `=` joins the codes of compound parts;
  - `H<n>` homograph class;
  - `z`: no tag (6,219 main-dictionary words).

  Module B resolves alternatives (rules.md, `prosody.md`).
- **T4:** codes for module B: `Z`, three-digit word classes (`001` forms of *be*, `400` negated auxiliaries, …), alternative tags, or the second pronunciation of a homograph (53, and "uses" in 21).
- **Odd values** in the data are kept as they are (`66`, `3NJ`, an empty T3 field).

The main dictionary and the function words share 22 words (`august`, `january`, `february`, `often`, `policy`, …). The main dictionary is always tried first, so its entry wins, and its months have no month marker.

## 4. Morphology

Procedure 30, called by group 33 on a `#word#` span that is still letters:

```
homograph found = verb found = false
run group 57
while neither flag is set:
    if group 56 doesn't fire: stop
    run group 57
if a flag is set: run group 58; return true
else:             run group 59; return false
```

- **Group 57:**
  1. `un-` (except `under`, `undulat`) becomes its own part, pronounced `` `^n ``.
  2. Each part is looked up as a homograph.
  3. If no homograph matched, each part is looked up as a verb. The first hit stops the group.
- **Group 56:** makes **one** suffix split per run, the first rule that matches anywhere, at its rightmost place. Splits are marked with `&`:
  - `cities` → `city&es`;
  - `stopped` → `stop&&ed` (doubled consonant);
  - `hoping` → `hope&ing`;
  - `-ness`, `-less`, `-ful`, `-ly`, `-ment`, `-ation`, `-able`, `-ible`, `-ance`, `-or`, `-ive`, `-al`, `-ant`.
- **Group 58** (found):
  - derives the part of speech from the suffix (past, `-ing`, plural, nouns in `-ness`, …);
  - picks the homograph reading;
  - pronounces the suffixes (`-ed` → `t`/`d`/`Id`, `-s` → `s`/`z`/`Iz`, `-ing` → `IN`, `-ation` → `'J-S$n`, …);
  - syllabifies the joins, fixes the stress and removes the markers.
- **Group 59** (not found): restores the spelling and the cells exactly as they were.
- **Plural retry** (group 24, which runs before the morphology):
  - looks up the stem without `-s`, `-es` or `-ies` (→ `y`) in the main dictionary and the function words;
  - on a hit, the ending becomes a plural placeholder that module B realises as `s`, `z` or `Iz`;
  - on a miss, the spelling is restored.

Examples:
- `rewriting` → `ri-'rY-tIN` (homograph `rewrite` + `ing`);
- `stopped` → `'stapt`;
- `unhappiness` → not found, so letter-to-sound.

## 5. Letter-to-sound

Group 32, on a word no dictionary knew. In order:
1. **Prefixes:** `over-`, `out-`, `under-` and `un-` are split off with their pronunciation.
2. **Known beginnings:** the longest leading 7…2 letters found in the prefix dictionary, keeping at least two letters after them.
3. **Suffixes:**
   - `-ies`, `-s`, `-es`, `-ly`, `-ness`, `-less`, `-man`, `-hood`, `-ing` and `-ed` are split off with their pronunciation;
   - then the longest ending found in the suffix dictionary;
   - the ending also sets the part of speech.
4. **Weight marks** for the stress rules: final checked syllables, doubled consonants.
5. **The stem, in order:**
   1. palatalisation contexts (group 12);
   2. affixes with stress classes (group 16);
   3. consonants (group 19);
   4. vowels (group 28);
   5. syllables, stress and vowel reduction (group 11).
6. **Endings:** `-ed`, `-s` and `-es` are realised, double consonants are reduced, and prefix stress is adjusted.

**Stress classes.** Affix rules put a digit before the suffix phones:
- `0`: the suffix is stress-neutral;
- `1`–`5`: stress that syllable from the end;
- `6`: the Latin rule relative to the suffix (stress the syllable before it if it is heavy, else the one before that);
- `7`: like 6, one syllable further left.

Without a class:
- the penult is stressed if it is heavy and the final syllable is checked;
- else the antepenult, in words of four or more syllables;
- else the first syllable after a prefix, or the first syllable.

Secondary stress goes two syllables before the primary when that syllable is heavy or word-initial. Unstressed short vowels in open or final syllables reduce to `$`.

**Syllables** use onset maximisation: V-CV, legal two- and three-consonant onsets (`pl`, `tr`, `str`, `spl`, …), and VC-CV otherwise. `s` clusters move to a following stressed syllable.

All of this is rule data. Examples: `zindle` → `'zYn-d$l`, `blorptastic` → `blorp-'t\x91s-tIk`, `glimphation` → `glIm-'fJ-S$n`.

## 6. Lexicons

### 6.1 Built-in exceptions

160 words with ARPAbet pronunciations, looked up after the user lexicon (text.md §8.4). Examples: `ALLAH`, `APRIL`, `EDT` (Eastern Daylight Time), `GMT`, `CPU`, `SQL`, `HMM`, `E-MAIL`, `VOLKSWAGEN`.

Two entries have a pronunciation token with no phone (`RAILROAD`, `STRENGTHEN`), so they never apply. The data pack keeps them as they are.

### 6.2 ARPAbet → phones

Used for both lexicons:
- Split at single spaces; empty names are skipped, and any other byte, a tab included, is part of a name.
- A final digit is removed. `1` writes `'` and `2` writes `` ` `` **immediately before that vowel**; `0` and 3–9 write nothing.
- `AH` with digit 0 or no digit → `$`. Every other name maps to the first T1 code with that unit name (AO → `A`, AH → `^`, JH → `dZ`, CH → `tS`, ER → `er`).
- An unknown name means no replacement, and the word goes to the rules. A name that is only a digit is unknown once the digit is removed.
- The names are matched case-sensitively against the phone table, in its order, as `prosody.json` stores it (datapack.md §8). Besides the phones, it names `#` `SIL`, `?` `#` and `-` `-`.
- There are no syllable breaks: `ALLAH` (`AA1 L AH0`) → `'al$`.

The result replaces the word as `prefix + " \x10 " + phones + " \x10 " + suffix`, where prefix and suffix are the non-letter bytes around the core (text.md §8.4).

### 6.3 User lexicon

The user lexicon holds words with a pronunciation in ARPAbet. During speech the reader looks every word up in it before the built-in exceptions (text.md §8.4). A word found there is pronounced only from it: if the pronunciation has a name the phone table lacks (6.2), the word goes to the rules, and the built-in exceptions aren't tried. An empty pronunciation is a word with no phones.

**Entries.**
- A word holds only ASCII letters and `- ' .`, or nothing. No byte from 0x80 up counts as a letter (MSVCRT `isalpha` in the C locale, checked for every byte).
- Words are kept in ascending order, compared as `_stricmp` does in the C locale (A–Z as a–z, then by byte), and each is there once, in any case.
- The pronunciation is ARPAbet as 6.4 writes it: every symbol followed by a space.

**Operations** (section 6.6 lists the methods):
- **Add** converts the pronunciation to ARPAbet (6.4) and inserts the word and the pronunciation, both upper-cased (a–z only). If the lexicon has the word already, in any case, Add changes nothing and still succeeds, with `LEXERR_ALREADYINLEX`. To change a pronunciation, remove the word first. An empty word can be added.
- **Remove** finds the word in any case.
- **Get from the user lexicon** returns the stored ARPAbet as engine phoneme names (6.4).
- **Get from the engine** runs the front end on the word (6.5).
- Every engine sees a change from the next word its reader ends.

**File.** The original keeps it at `Lex\<Windows user name>.lex` in the DLL's folder:
- Bytes in code page 1252: the header `[MSTTSUSERLEXVER100]` CR LF, then one line per entry: the word, a tab, the pronunciation, CR LF. Nothing follows the last line.
- It is written in the lexicon's order. A lexicon with no words writes only the header.

The file is read up to its first NUL byte and accepted only if:
1. it starts with the header;
2. there are as many entries as CR bytes after the header's;
3. each entry is a word up to the next tab, valid and greater than the word before (the first must be greater than the empty word, so an empty word fails), then the pronunciation up to the next CR, which must be followed by LF;
4. each pronunciation, converted to engine phoneme names (6.4), parses (text.md §2.3). What follows its last space isn't converted, so it isn't checked;
5. nothing is left after the last entry.

Anything else gives an empty lexicon: one bad line hides the whole file. Entries are kept as the file has them, in any case, even when the lookup can't use them (lower-case ARPAbet, which the phone table doesn't match; a tab between names; a name after the last space).

**When the original reads and writes it.** It reads the file when its first engine starts, and writes it back when its last engine is released, only if Add or Remove changed something since. All its engines share one lexicon. Two processes overwrite each other's file: the last one released wins.

**In Go,** `Data.Lexicon` is the lexicon every engine of the data shares (package `lexicon`). The application keeps the file where it wants:
- `Read` replaces the entries with a file's. A file the original rejects empties the lexicon and gives `ErrInvalidFile`.
- `Write` writes the entries in the original's format.
- `Modified` says whether Add or Remove changed anything since the lexicon was made or read.

Reading at the start, and writing at the end when `Modified`, does what the original does. A file the original wrote reads as it is.

### 6.4 Conversions

The lexicon meets four alphabets: IPA (the wide interfaces), engine phoneme names (the ANSI interfaces, `hhax1low`), ARPAbet (what is stored) and the units' names (6.5). Engine phoneme names are parsed as text.md §2.3 says. The parse also reports an error, keeping what it read before it:
- a name it doesn't know;
- a stress mark followed by a boundary symbol (`-`, `#`, space, `_`, `.`, `?`, `!`, `,`) or another stress mark before any vowel;
- a stress mark left at the end;
- an escape that runs to the end.

**IPA → engine phoneme names** (Add on a wide interface): each character's engine name (the first symbol with that IPA code), with no separators, up to the first U+0000. A character the table doesn't have, or a backslash, is an error.

**Engine phoneme names → ARPAbet** (Add): parse, then for each symbol except stress marks:
- its ARPAbet name, from the data pack. Most are the engine name; `ax` is `ah`, `dx` is `t`, `nx` is `ng`, and the boundary symbols have none;
- after a vowel, the digit of the last stress mark since the previous vowel (`1` or `2`), or `0`;
- a space.

A parse error makes Add fail with `LEXERR_INVALIDPRONCHAR`. Examples:
- `hhax1low` → `hh ah0 l ow1 `;
- `z ay n d ax l` → `z  ay0  n  d  ah0  l ` (a space is a symbol, with no ARPAbet name);
- `hh ax 1l ow` fails: the stress mark meets a space before its vowel.

**ARPAbet → engine phoneme names** (Get, and reading the file). For each name followed by a space:
- `AH0` is `ax` and `NG` is `nx`, in any case;
- otherwise a final `1` is written as `1` before the name, a final `0` or `1` is dropped, and the rest is written in lower case.

There are no separators, and what follows the last space is left out: `HH AH0 L OW1 ` → `hhaxl1ow`. A final `2` stays with its name (`AY2` → `ay2`), so the parse reads it as stress on the next vowel, and as an error after the last one.

**Engine phoneme names → IPA** (Get on a wide interface): what the parse reads, error or not. `aw`, `ay` and `oy` are written `ɑʊ`, `ɑɪ` and `ɔɪ`, and every other symbol is its IPA code: `hhaxl1ow` → `həlˈo`.

During speech, the stored ARPAbet is converted straight to phones (6.2).

### 6.5 Get from the engine

`ILexPronounce::Get`, and `ILexPronounce2::GetFrom` with `ILP2_LTS`, give the engine's own pronunciation of a word. The word goes through the front end on its own, the lexicons included:
1. The reader reads the word as a segment (text.md §8), without preprocessing. If it finds nothing, the result is empty.
2. Otherwise modules C, B and A and the unit builder run on that first sentence (prosody.md §10), without the one-character rule. If the analysis fails, the reader reads on from where it stopped, and the result is empty.
3. The units' names, without `#`, tags and `SIL`, each followed by a space, are ARPAbet, which 6.4 converts to engine phoneme names. `zindle` → `z1ayndaxl`.

The result has `1` before each stressed vowel and no secondary stress. A long word gives only what its first sentence holds.

The original runs this on the reader and rule modules that speech uses, since their state is process-wide. So the result depends on earlier texts, and the next text sees what the word left behind. Before the first text of the process, the duration style is 0 (prosody.md §11), so connected speech runs group 14, which flaps: `butter` is `b1ahdxer` before the first text and `b1ahter` after it.

In Go, `Engine.Pronounce` (and `PronounceIPA`) runs on the engine's own front end, as `Get` does in a process with one voice. Like a text, it must not run while the engine speaks.

### 6.6 The interfaces

| Method | Original | Go |
|---|---|---|
| `Add`, `AddTo` | ANSI: engine phoneme names. Wide: IPA, converted to names, then the ANSI method with the word in code page 1252 (best fit) | `Lexicon.Add` (names), `Lexicon.AddIPA`. `LEXERR_ALREADYINLEX` gives false, with no error |
| `Remove`, `RemoveFrom` | | `Lexicon.Remove` |
| `GetFrom` with `ILP2_USER` | the stored pronunciation, as engine phoneme names (ANSI) or IPA (wide) | `Lexicon.Pronunciation`, `PronunciationIPA` |
| `Get`, `GetFrom` with `ILP2_LTS` | 6.5 | `Engine.Pronounce`, `PronounceIPA` |
| `QueryLexicons` | `ILP2_USER` (2) for writing, `ILP2_USER` + `ILP2_LTS` (10) for reading | none |
| `ChangeSpelling` | `E_NOTIMPL` | none |

The original checks in this order:
1. `AddTo` and `RemoveFrom` with any lexicon but `ILP2_USER` (2): `LEXERR_INVALIDLEX`.
2. Wide `Add`: IPA that doesn't convert: `LEXERR_INVALIDPRONCHAR`.
3. `Add`, `Get`, `GetFrom`: a character set other than `CHARSET_IPAPHONETIC` (1): `E_INVALIDARG`. With it, the wide interfaces take and give IPA, the ANSI ones engine phoneme names.
4. `Remove`, `Get`, `GetFrom`: a sense other than 0: `LEXERR_INVALIDSENSE`.
5. A word with a byte other than a letter or `- ' .`: `LEXERR_INVALIDTEXTCHAR`. A wide word is first converted to code page 1252 with best fit (`ā` → `a`, `Ａ` → `A`).
6. `GetFrom` with neither `ILP2_USER` nor `ILP2_LTS` in the low byte of the lexicon: `LEXERR_INVALIDLEX`. `ILP2_LTS` gives the engine's pronunciation, `ILP2_USER` alone the stored one. `Get` is `GetFrom` with every bit set.
7. `Add`: a pronunciation that doesn't parse: `LEXERR_INVALIDPRONCHAR`. `Remove`, and `Get` from the user lexicon, of a word it doesn't have: `LEXERR_NOTINLEX`.

Part of speech and engine information are ignored, and `Get` reports `VPS_UNKNOWN`. A buffer that is too small gives `LEXERR_PRNBUFTOOSMALL` once the work is done; the size needed is the engine names' length plus one, twice that in bytes for a wide interface. The Go methods take and return Go strings, so the character set, the sense, the buffers and the lexicon flags have no Go form; every call those checks stop changes nothing in the original.

## 7. Quirks we don't reproduce

- **An escape in engine phoneme names.** When the parse accepts an escape (0x1B followed by a space), `Add` converts each of its bytes as a phonetic symbol code and stores what it reads outside its ARPAbet table. The Go lexicon refuses a pronunciation with an escape (`ErrInvalidPronunciation`). One the parse rejects fails in both.
- **A stored pronunciation that starts with a space** (from names that start with a boundary symbol, `-z1ay` → ` Z AY1 `). Converting it to engine phoneme names, the original looks at the byte before its buffer, part of the heap, to see whether it is `0` or `1`. We take it as neither.
- **The IPA of an escape** in a stored pronunciation (only a hand-written file has one): the original reads outside its IPA table. We leave the escape out.

## 8. For the data pack

The converter writes `dictionaries.json` (datapack.md §10): for each dictionary its fields and its words with their entries as stored. Before writing it checks, per dictionary:
- that its decoding finds every word with the original algorithm;
- that every byte of the dictionary area is accounted for;
- that each word appears once and each key belongs to one spelling;
- that no key could be matched by a word outside the list. The original's key packing adds the code of a byte that has none (0xff) unmasked into the neighbouring bits, which leaves the all-ones code at that letter's place. So the converter checks that no stored key holds the all-ones code, or a code that several bytes share.

It also writes the 160 built-in exceptions, as stored.

## 9. Open questions

1. **The `z` tag** (no part of speech, 6,219 main-dictionary words): confirm against the original engine that such words get no content-word accent from module A.
2. **Months:** the main dictionary's `january` and `february` have no month marker. Do dates like "January 5, 1998" still get the date rules? Settled for the port: "January 5, 1998.", "He was born on February 14, 2001." and "The meeting is on Monday, January 5th." were checked against the original engine on all five voices: identical bit for bit, so the rules and dictionary in the data pack give the original's reading.
3. **Stale T5 cells** after a lookup that grows the span. Module A overwrites T5 before reading it, so they should never matter.
