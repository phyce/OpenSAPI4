# SAPI 4 Microsoft engine: text front end

How the Microsoft Speech Synthesis Engine (`msttssyn.dll`) turns the text of a `TextData` call into sentences in the text analyser, and how the analyser normalises them. The interface around it is in [interface.md](interface.md). The rule engine that does the normalisation, letter-to-sound and phrasing is in [rules.md](rules.md). Dictionary lookups are in `pronunciation.md`. Durations, intonation and how phones become units are in `prosody.md` and `units.md`.

The front end has two parts:
- **Microsoft's preprocessing:** character sets, tags, line breaks, special tokens and a sentence breaker (sections 2–6).
- **A text-analysis library compiled into the engine:** a Lernout & Hauspie front end, "TTS (Eng USA 10) V5.10" from 1997. It reads one sentence at a time (sections 7–8) and runs three compiled rule modules over it (section 9 and rules.md).

The Go engine reproduces all of this byte for byte, quirks included, because the output text decides every pronunciation. The exceptions are memory-safety bugs; section 10 lists them, with the behaviour we use instead.

## 1. Pipeline

For one `TextData` call:

1. **IPA → engine phoneme names** (character set 1 only, section 2.2).
2. **UTF-16 → ANSI**, Windows-1252 (section 2.1).
3. **Tags → escape sequences**, plus a control table (section 3). From here on the text is ANSI with `ESC` (0x1b) sequences.
4. **For text** (character sets 0 and 4):
   1. line breaks (section 4);
   2. special tokens (section 5);
   3. sentence breaking (section 6). Every sentence ends with ` \r\n`.

   **For phonetic input** (character sets 1 and 5): parse engine phoneme names into `ESC /` … `ESC /` (section 2.3).
5. **Segments:** the result is cut at every pitch escape `ESC I<n> `. Each segment goes to the analyser on its own (section 7).
6. **For each sentence of a segment:**
   1. the reader puts it into tier 1 (section 8);
   2. the rule modules transcribe it to phones (section 9);
   3. prosody and units follow (`prosody.md`, `units.md`).

Escape sequences that exist after step 3 (`␠` is a space):

| Sequence | Made by | Meaning |
|---|---|---|
| `ESC A P ␠` | start of every `TextData` text | duration style "P" (the analyser's only style in practice, `prosody.md`) |
| `ESC N<k> ␠` | before every word, and for most tags | reference to control-table entry `k` (decimal) |
| `ESC I<n> ␠` | `\Pit=n\`, `\Rst\` (`I0`) | pitch change; segment boundary |
| `ESC H<n> ␠` | `\Pau=n\` | pause (has no effect in the analyser; the pause comes from the control entry) |
| `ESC "␠` | `\Emp\` | emphasis on what follows |
| `ESC @N␠` `ESC @V␠` `ESC @J␠` `ESC @R␠` | `\Prt=` | part of speech of what follows |
| `ESC [addr]␠` / `ESC [\addr]␠` | `\Ctx=` | address context on / off |
| `ESC /` | phonetic input | raw phonetic text on / off (no space after it) |

## 2. Character sets and code page

`TextData` accepts character sets 0 (`CHARSET_TEXT`) and 1 (`CHARSET_IPAPHONETIC`). The ANSI interface converts its text to UTF-16 with the system code page and passes set + 4, so the front end sees 0, 1, 4 or 5.

| Set | Comes from | Path | Word offsets in control entries |
|---|---|---|---|
| 0 | W, text | text | ANSI byte offset × 2 |
| 1 | W, IPA | IPA → engine names (2.2), then phonetic (2.3) | offset in the *converted* text × 2 |
| 4 | A, text | text | ANSI byte offset |
| 5 | A, IPA | phonetic directly: the "IPA" is read as engine phoneme names (`hh ax l ow`) | ANSI byte offset |

The ×2 makes W offsets count bytes of UTF-16, but they are measured in the ANSI text. A character that becomes two ANSI bytes, or none, shifts every later offset. We reproduce this.

### 2.1 UTF-16 → ANSI

- The text ends at the first U+0000.
- It is converted with Windows-1252 **best fit**: characters with a best-fit mapping become that byte (for example U+0100 Ā → `A`), and everything else becomes `?`. The data pack carries the full mapping (section 11).
- **One-character flag:** set when the converted text is exactly one byte long. For set 1 this is measured after the IPA conversion, so `ə` (→ `ax`) doesn't set it. It changes special tokens (5.4).

### 2.2 IPA → engine phoneme names (set 1)

Each UTF-16 unit is handled as follows:
- **`\`:** in untagged text it ends the conversion. In tagged text it toggles "inside a tag" and is copied.
- **Inside a tag:** copied unchanged.
- **Otherwise:** looked up in the IPA table (53 entries). A match appends that entry's engine phoneme name (1–2 letters) with no separator, so `həˈloʊ` → `hhax1lowuh`.
  - Stress `ˈ` → `1`, `ˌ` → `2`; `ɫ` → `l`.
  - `aʊ`, `aɪ` and `ɔɪ` have no single symbol. They are written `ɑʊ`, `ɑɪ`, `ɔɪ`, and 2.3 merges them.

A symbol not in the table ends the conversion. Either way, the text converted so far is used.

### 2.3 Engine phoneme names → phonetic text (sets 1 and 5)

The input is the tagged text (section 3), so it starts with `ESC A P ␠` and has `ESC N<k> ␠` before each space-separated group.

**Pass 1** reads left to right:
- **`ESC`:** copied up to and including the next space. A NUL before that space ends parsing.
- **TAB, CR and LF** count as spaces.
- **Any other byte:**
  - Look up the 1-byte name and the 2-byte name (this byte and the next; at the last byte both are the same byte). Matching is case-insensitive over the 52 engine names (table `engine phoneme names`). `ao` is listed twice (`A` and `o`); the first match wins, so it is always `A`.
  - **Neither found:** parsing stops. Everything parsed so far is kept.
  - **Only one found:** take it.
  - **Both found** (`dh dx th sh zh nx`): count the `h`/`H` bytes right after the pair. Even → take the pair, odd → take the single letter. So `thhh` = `th` + `hh`, and `thh` = `t` + `hh`.
- **Stress check:**
  - A stress mark (`1` or `2`) makes stress pending.
  - A vowel clears it.
  - While stress is pending, a boundary, space, punctuation or another stress mark stops parsing. Consonants are allowed in between.

**Pass 2** merges adjacent codes, not across escapes or spaces: `aa`+`ih` → `ay`, `aa`+`uh` → `aw`, `ao`+`ih` → `oy`.

**Output:** `ESC /`, then each code's internal symbol, with escapes copied back as they were, then `ESC /`. The internal symbols are the analyser's phone alphabet (`units.md`), for example `aa` → `a`, `ae` → 0x91, `ax` → `$`, `ey` → `J`, `jh` → `dZ`, `1` → `'`, `2` → `` ` ``, `#` → space, `_` → `#`.

Example (set 5): `hh ax l ow` → `ESC/ESC AP ESC N0 h ESC N1 $ ESC N2 l ESC N3 OESC/`.

Phonetic text skips sections 4–6.

## 3. Tags

### 3.1 Before the scan

1. Every `ESC` byte in the caller's text becomes a space, so callers can't inject escapes.
2. Whitespace is found with the delimiters `\`, space, TAB and LF.
   - In tagged text, every `\` toggles "inside a tag".
   - Outside tags, every space, TAB and LF is marked as whitespace.
   - CR is never marked.

### 3.2 The scan

The output starts with `ESC A P ␠`. If the address context is on (3.4), `ESC [addr]␠` follows.

The scan walks the text with an "in word" flag (initially off). At each byte, in order:

1. **Marked whitespace is restored:**
   - as LF if the byte before it in the input is CR;
   - otherwise as a space.

   So outside tags, TAB and lone LF become spaces, CR LF stays CR LF, and a space or TAB right after a CR becomes LF.
2. **Pending `\Btp`:** if one is pending and its end position has been reached, emit its end (3.4).
3. **Tags:** in tagged text, a `\` starts a tag (3.3).
4. **Any other byte** is copied, and the in-word flag is updated:
   - space or LF: in-word off;
   - TAB or CR: no change, so a lone CR doesn't separate words;
   - anything else with in-word off: in-word on, add control entry (type 1 "word", value = offset, section 2), and write `ESC N<k> ␠` before the byte.

At the end, if the address context is still on, `ESC [\addr]␠` is appended. The context stays on, so the next `TextData` text is wrapped too, until a `\Ctx=` or `\Rst\` turns it off.

Every word therefore starts with its own `ESC N<k> ␠`. A word after a lone CR (`a\rb`) is part of the previous word: it gets no control entry and no `WordPosition` event.

Example (W, tagged): `Hello \Pau=500\ world` → `ESC AP ESC N0 Hello ESC H500 ESC N1  ESC N2 world`, with entries 0 = (word, 0), 1 = (Pau, 500), 2 = (word, 32).

### 3.3 Tag parsing

After the `\`, the keyword is read up to `, : ; = \` or NUL. The tokeniser is the same as for `Inject` (interface.md §7). Keywords, and the names of `\Ctx=` and `\Prt=`, are looked up with `lstrcmpiA`, which on the development machine (code page 1252, English locale) ignores ASCII case and soft hyphens (0xAD) and reads ß (0xDF) as `ss`: `\Ctx="Addreß"\` is `"Address"`. Nothing else differs from ASCII case-insensitive equality, checked against every one-byte insertion, substitution and two-letter contraction of every keyword and name. Every tag sets in-word off. A keyword is at most 79 bytes; one of 80 is a token error. "Skip" means: move past the next `\`. It stops early, without consuming it, at whitespace marked in 3.1 (only the swallowing case below can meet any) or at the end of the text.

| Tag | Accepted form | Output | Control entries |
|---|---|---|---|
| `Chr` | `\Chr="a"[,"b"…]\` | `ESC N<k> ␠` per entry | always first `"Normal"`, even if the rest of the tag is then skipped; then each quoted name with its quotes, in order. A missing `=`, an unquoted value, a name of more than 78 characters (more than the text's length − 1 after a longer `\Btp=` text in the same call) or a separator other than `,` → skip (names before it are kept) |
| `Wrd`, `Com` | – | skip | – |
| `Ctx` | `\Ctx="name"\` | `"Address"`: `ESC [addr]␠` (if not already on), context on. Any other known name (`"Spreadsheet"`, `"Numbers"`, `"Document"`, `"Unknown"`, …, 8 in all, case-insensitive): `ESC [\addr]␠` if it was on, context off. Unknown name: nothing | – |
| `Emp` | `\Emp` up to the next `\` | `ESC "␠`, then skip | – |
| `Eng`, `Eow`, `Fs`, `Hs`, `Prn`, `Pro`, `Vce`, `Etp` | – | skip | – |
| `Mrk` | `\Mrk=n\` | `ESC N<k> ␠` | (Mrk, n) |
| `Pau` | `\Pau=n\` | `ESC H<n> ␠ ESC N<k> ␠` | (Pau, n) |
| `Pit` | `\Pit=n\` | `ESC I<n> ␠ ESC N<k> ␠` | (Pit, n) |
| `Prt` | `\Prt="x"\` | `"N"` → `ESC @N␠`, `"V"` → `ESC @V␠`, `"Adj"` → `ESC @J␠`, `"Adv"` → **`ESC @V␠`**, `"PP"` → `ESC @R␠` (case-insensitive); anything else: the last escape written again (below) | – |
| `Rst` | `\Rst` up to the next `\` | `ESC [\addr]␠` if the context was on; context off; then `ESC I0␠ ESC N<k> ␠`, then skip | (Rst, 0) |
| `Spd`, `Vol` | `\Spd=n\`, `\Vol=n\` | `ESC N<k> ␠` | (Spd, n), (Vol, n) |
| `Btp`, `Tp` | `\Btp=n\` | see below | |
| `\\` | | `ESC N<k> ␠` then `\`; in-word on | (word, offset of the byte *after* `\\`) |
| unknown keyword, number or punctuation | | skip | – |

- **Number forms.** `=n` needs exactly `=`, an unsigned decimal number (wrapping modulo 2³²) and `\` as the next three tokens. Anything else → skip. Numbers are written back with `%u`, so `\Pit=0\` gives `ESC I0␠` (the default pitch, interface.md §7).
- **Swallowing.** When a required token is missing and the token read was the closing `\` itself (`\Pit\`, `\Chr\`, `\Mrk\`), the skip starts after that backslash. Whitespace after it stops the skip at once, so `\Pit\ x` loses nothing, but `\Pit\xyz\ abc` loses `xyz\`.
- **Unknown `\Prt=` values** write the last escape the scan formatted in this call again: an `ESC N<k> ␠` reference (the usual case, so the analyser gets that reference twice), `ESC H<n> ␠`, `ESC I<n> ␠`, or an address or part-of-speech escape. `ESC A P`, Emp's `ESC "` and Rst's `ESC I0` don't count. `Hello \Prt="X"\ world` → `ESC AP ESC N0 Hello ESC N0  ESC N1 world`. If nothing was formatted yet, the original appends uninitialised stack bytes (section 10).
- **A lone `\`** in tagged text makes everything up to the next `\` (or the end) a tag, which is then dropped, whitespace included.
- **Untagged text:** backslashes are ordinary characters (special tokens name them, section 5).
- **`\Btp=n\` / `\Tp=n\`:**
  1. If a Btp is pending, emit its end first.
  2. Read `=` and a number in C `%i` form (sign, `0x` hex and leading-zero octal accepted). On failure, skip. With no closing `\` at all, the rest of the text is dropped, whether or not the number was read.
  3. Add (Btp, offset of the byte after the keyword). For sets 0 and 1 the offset is negated and doubled, then written with `%u` (for example `4294967286`). Emit `ESC N<k> ␠`.
  4. Add (Btp text, the text between the keyword and the next `\`, i.e. `=n`). Emit `ESC N<k> ␠`.
  5. Its end is due n bytes after that `\`. At the first byte at or past that point, or at the next Btp, add (Etp, 0) and emit `ESC N<k> ␠`. If the text ends first, it is never emitted.

  The Btp text is a prosody transplant when it holds a `<` (engine.md §13): its phones replace those of the text up to the end entry. Otherwise the entries have no effect.
- **`\Chr=` resets.** Because every `\Chr=` tag starts with a `"Normal"` entry, each tag first goes back to the mode's own effects and then adds its names. Effects of earlier `\Chr=` tags don't carry over. Effects of names in the same tag do: `\Chr="Whisper","Monotone"\`.

### 3.4 Control table

A per-voice list of `{type, value string}` entries, indexed from 0. It is cleared for every `TextData` call.

| Type | From | Value |
|---|---|---|
| 0 | Chr | quoted name |
| 1 | word | offset |
| 9 | Mrk | n |
| 0xa | Pau | n |
| 0xb | Pit | n |
| 0xf | Rst | 0 |
| 0x10 | Spd | n |
| 0x12 | Vol | n |
| 0x13, 0x14, 0x15 | Btp, Etp, Btp text | see 3.3 |

What each entry does when its `ESC N<k>` is reached (WordPosition, bookmarks, speed, volume, pause, character) is in `prosody.md` and interface.md §8. Entry numbers are what the analyser carries through as markers (section 8.2).

## 4. Line breaks

This step works in place on the tagged text.

**Pass 1:**
- LF becomes CR. If a CR follows, it becomes LF and is skipped, so LF CR becomes CR LF. (Section 3.2 has already turned every LF that doesn't follow a CR into a space, so this never happens to the FrontEnd's text.)
- CR LF is skipped as a pair. A lone CR stays.

**Pass 2** keeps "seen a break" and "pending CR":

| Byte | Action |
|---|---|
| TAB, space | nothing |
| LF | becomes a space |
| CR, after a break | becomes a space; pending CR = none |
| CR, otherwise | seen a break; pending CR = this CR |
| `ESC` | skip to the next space, TAB, CR, LF or 0x1a, and continue after it |
| anything else | the pending CR (if any) becomes a space; seen a break = no |

Result:
- A single line break disappears: `a\r\nb` → `a  b`.
- Two or more breaks with only spaces, TABs and escapes between them leave **one CR** (a paragraph end): `a\r\n\r\nb` → `a\r   b`.

## 5. Special tokens

### 5.1 Records

The text is split into records of an **escape group** (`ESC` sequences and the spaces after them, up to 100 bytes) followed by **one word with its trailing whitespace and CRs** (up to 99 bytes). After section 3 every word has its own escape group.

- The records live in one flat buffer: `floor(length / 2)` records of 201 bytes, each 101 bytes of escapes followed by 100 bytes of text.
- Reading stops at the end of the text.
- **Giving up.** If the text doesn't start (after spaces) with `ESC`, or an escape group exceeds 100 bytes, or a word with its whitespace reaches 100 bytes, every special-token rewrite is dropped for the whole call, and the text goes on unchanged.

Each record is then rewritten (5.2). If any record changed, the output is every record's escapes followed by its text, in order. Otherwise the text is unchanged.

Rewrites write into the flat buffer:
- A text longer than 100 bytes runs on into the next record's escape group. This happens with long addresses (5.3) and with exactly-100-byte results (the terminating NUL lands in the next record).
- That record's escapes are overwritten, so it loses its `ESC N` and repeats the overflowing text.

We reproduce this, except past the end of the buffer (section 10).

### 5.2 Order

Records with empty text are skipped.

**One-character flag set:** the text becomes ` <name> ` (space, the character's name, space), using the character-name table (5.4). For example `a` → ` eigh `, `?` → ` question mark `, `5` → ` five `.

**Otherwise** the first recogniser that applies wins:
1. emoticon;
2. character cleaning (which also handles a quoted single character, `~` and `*`);
3. address;
4. thousands;
5. plus signs.

So a record that character cleaning changed is never treated as an address or a number.

"Letter" and "digit" below mean ASCII only.

**Splitting off punctuation.** Several recognisers split the text into:
- `lead`: the bytes before the first letter or digit (no letter or digit at all: the recogniser doesn't apply);
- `core`: from the first to the last letter or digit;
- `trail`: everything after it, including the trailing whitespace.

### 5.3 Recognisers

**Emoticon.** Scan the text with `eyes` and `mouth` unset, collecting `before` and `after`:
- **No eyes yet:** `:` or `;` sets the eyes. Any other byte goes to `before`.
- **Eyes, no mouth:**
  - `(` or `)` sets the mouth;
  - `-` is skipped (any number of them);
  - any other byte goes to `before` and clears the eyes. The `:` itself is lost: `a:b:)` → `ab (smiley face )`.
- **Mouth set:** every byte goes to `after`.

Not an emoticon if there is no mouth, or `before` or `after` is longer than 33 bytes. Otherwise the text becomes `before + " (" + ["winking " if the eyes were ;] + ("smiley face " for ")" | "frowning face " for "(") + ")" + after`.
- Examples: `:-)` → ` (smiley face )`; `Hi;) ` → `Hi (winking smiley face ) `; `Note:(see` → `Note (frowning face )see`.
- Other faces (`:-I`, `8-)`, `:^)`) are left to the sentence breaker, which treats them as one token (6.2).

**Character cleaning.** Let `p` be the first non-space byte (all spaces: doesn't apply) and `end` the last byte that is neither space nor CR. These are tried in order:
1. **Quoted single character:** `p[0] = p[2] = '"'`, `end − p = 2`, and `p[1]` is in 0x21–0x40 or is ≥ 0x7e. The text becomes `<name of p[1]> ` (the trailing whitespace is dropped), provided the name plus 2 is under 100 bytes. For example `"!"` → `exclamation mark `.
2. **`p[0] = '~'`:** tilde (below).
3. **The text contains `*`:**
   - It applies only if the text is under 99 bytes, the split succeeds, `lead` contains only `*` and spaces, and `core` contains only letters and `'`.
   - The text becomes `lead + " " + core + " " + trail`: `*bold*` → `* bold * `, `word*` → ` word * `.
   - No cleaning is done on this path.
4. **Cleaning** of every byte from `p` on:
   - **Copied and fixed:** bytes below 0x80 other than `\` and `_`, and the bytes 0xa0, 0x91–0x94, 0xab and 0xbb.
     - 0xa0 → space;
     - 0x91 and 0x92 → `'`;
     - 0x93 and 0x94 → `"`;
     - any other byte ≥ 0x80 or below 0x20 (except LF, CR and ESC) → space. So 0xab, 0xbb and TAB become spaces, and DEL stays.
   - **`\`, `_` and every other byte ≥ 0x80:**
     - If the accent-folding table has a letter for it (é → `e`, Š → `S`), it becomes that letter.
     - Otherwise it becomes ` <name> `. An empty name gives two spaces.
   - **Length limits:**
     - over 100 bytes copied → doesn't apply;
     - 100 or more after inserting a name → doesn't apply;
     - the final length must be ≤ 100.
   - It applies only if some byte changed.

**Tilde.**
- `~` followed by a digit: the word is `approximately`. The rewrite doesn't apply if 18 + the text length exceeds 99.
- Otherwise the word is the name of `~` (`tilde`), with no length check.
- The text becomes `" " + word + " " + the text after the ~ and any spaces`: `~5 ` → ` approximately 5 `, `~ ` → ` tilde `.

**Address** (URLs, e-mail, file names, dotted numbers). It applies only if the text is under 99 bytes and the split succeeds. Scan `core`:
- count the dots;
- note whether it contains any byte that is not a letter, digit or `.` ("special");
- note whether it contains any letter;
- a space in `core` means it is not an address.

**Deciding:**
- With exactly one dot and (no special bytes or no letters): it is an address only if exactly three letters follow the dot.
- Otherwise it is an address if: at least one dot, and (special or 2+ dots), and (letters or 2+ dots).
- So `www.example.com`, `user@host.org`, `file.txt`, `U.S.A`, `10.12.2001`, `192.168.0.1` and `end.The` are addresses. `e.g`, `3.14` and `abc.de` are not.

**Output** starts with `" "` + `lead`; a `lead` of exactly `<` becomes `left angle bracket `. Then walk `core`:
- `-` `.` `/` `:` `@` end the current part, then append ` dash `, ` dot `, ` slash `, ` colon ` or ` at `;
- `<` and `>` end the current part with nothing appended;
- spaces are dropped;
- other bytes collect into the part.

Ending a part appends it, with two replacements:
- a 1-byte part that is not a digit is replaced by its name (`a` → `eigh`);
- the part `edu` becomes `ee dee yuu`.

If the output then exceeds 100 bytes, the recogniser doesn't apply.

At the end:
1. The last part is appended **without** a length check.
2. Then `trail`, where exactly `>` becomes `right angle bracket`, and `>` followed by more becomes `right angle bracket ` plus the rest. There is no space before it, so `<a@b.com> ` → ` left angle bracket eigh at bee dot comright angle bracket  `.

**Thousands.**
- It applies only if the text is under 99 bytes, `core` is exactly four digits, and its value is not in 1492–2099.
- The text becomes `lead + d₀ "," d₁d₂d₃ " " + trail`: `1234.` → `1,234 . `, `0123` → `0,123 `. Years stay as they are.

**Plus signs.**
- It applies only if the text is under 99 bytes and `core` contains only letters, digits, `+`, LF and CR.
- The text becomes `lead + core with each "+" → " + " + trail`: `1+2` → `1 + 2`.
- It counts as applied even without a `+`. That rebuilds the output but doesn't change the text.

### 5.4 Tables

- **Character names:** 256 entries indexed by the ANSI byte, each up to 29 bytes. The names are spelled for the letter-to-sound rules ("eigh", "aytch", "double yu", "per cent sign", "capital eigh grav"), and the table's typos are kept. Bytes 00–20, 7f, 81, 8d–90, 9d and 9e have empty names.
- **Accent folding:** for bytes 0x80–0xff, an ASCII letter or none.

## 6. Sentence breaker

The breaker runs over the text from section 5. It finds one sentence at a time and starts afresh at the next byte, so nothing (open quotes, brackets) carries over between sentences. Each sentence is copied with its CR and LF bytes turned into spaces and ` \r\n` appended. The text is never reordered.

### 6.1 Character classes

Every byte has a class (table `sentence character classes`):

| Class | Bytes |
|---|---|
| 1 consonant | consonant letters, including Ç Ð Ñ ß ç ñ |
| 2 vowel | A E I O U Y in either case, accented vowels |
| 3 digit | 0–9, ¼ ½ ¾ |
| 4 / 5 / 6 | `!` / `?` / `.` |
| 7 space | TAB, space, 0xa0 |
| 8 / 9 | `'` / `"` |
| 10 / 11 | `( [ {` / `) ] }` |
| 12 / 13 | « / » |
| 14 | `, : ;` |
| 15 | … (0x85) |
| 17 paragraph end | CR, NUL |
| 21 other | everything else: LF, ESC, `-`, `#`, `$`, `/`, `@`, `\`, `_`, curly quotes, … |

### 6.2 Tokens

A token is `{type, start, length}`. The scanner hands out one token at a time and gives the same token again until the state machine accepts it. It can also mark a position and later rewind to it.

| Type | Name | Produced by |
|---|---|---|
| 1 | gap | a run of class 7; see below |
| 3 | word | word machine |
| 4 | abbreviation | a word ending in `.` that passes the abbreviation test |
| 5 | title | a word ending in `.` that passes the title test |
| 6 | ellipsis | `...` or … |
| 7 | spaced ellipsis | `. . .`, `. . . . .` or `. . . . . . .` |
| 8 / 9 | group begin / end | `( [ {` / `) ] }` |
| 10 | end punctuation | `!`, `?`, a single `.` |
| 11 | quote | `'`, `"`, «, » (one byte) |
| 12 | other punctuation | `, : ;` |
| 13 | other | one class-21 byte (ESC, `-`, `/`, …) |
| 14 | paragraph end | CR or NUL |
| 15 | end of text | at the end, length 0 |
| 16 | smiley | optional `>` or `}`, one of `: ; 8 %`, `-` or `^`, one of `) ( > I O o 0`. Tried first at every byte except class 17 |

**Gap.**
- A gap starts at a class-7 byte and continues over class-7 bytes.
- An `ESC` inside the run is skipped up to and including the next class-7 byte. So `␠ESC N5␠` and `␠␠ESC I120␠ESC N6␠` are each one gap.
- The gap's length is its span minus (the length of the **last** absorbed escape + 1). Earlier absorbed escapes count as spaces, so a tag between two words makes a gap of several spaces.
- An `ESC` that doesn't follow a class-7 byte is a type-13 token, and the escape's letters form a word.

**Periods** (class 6):
- `..` followed by a non-dot gives `.` (the second dot is the next token).
- `...` gives a length-3 token; a fourth dot starts a new token.
- Single dots separated by single class-7 bytes: 3, 5 or 7 dots are accepted (the longest seen; it stops at 7), and 2, 4 or 6 fall back to the last accepted count. In the FrontEnd's text every word has its own `ESC N`, so such dots arrive only from no-break spaces, which special tokens turn into spaces (`.\xa0.\xa0.`).
- Length 1 → type 10, 3 → type 6, other → type 7.

**Word machine** (starts at class 1, 2 or 3; `c` = class of the next byte):

| State | letter | digit | `.` | `'` | `-` | class 14 | other |
|---|---|---|---|---|---|---|---|
| L letters | L | L | P | A | L | end | end |
| P letters + `.` | Q | Q | – | A | – | – | period decision |
| Q after `x.y` | Q | Q | P | end | Q | end | end |
| A after `'` | A | A | see below | end | A | end | see below |
| N digits | after-number | N | S | end | S | S for `,` `:` | after-number |
| S digit + separator | – | N | give the separator back, after-number |||||
| after-number | L | – | end | A | end | end | end |

- A word starts in L (letter) or N (digit).
- In A, a `.` ends the word. If the word so far ends in `.'`, the `'` is given back and the period decision runs.
- **Period decision** (P meets something other than a letter, digit or `'`; the word includes its final period):
  1. title test without capitalising → type 5;
  2. abbreviation test → type 4;
  3. title test with the first letter capitalised → type 5;
  4. otherwise the period is given back: type 3 `word`, then a type-10 `.`.
- **`(s)`:** a type-3 word that ended normally and is followed by `(s)` takes it in (`word(s)`).
- Single words: `3.14`, `1,000`, `10:30`, `1-2`, `555-1234`, `10th`, `rock'n'roll`, `e.g`, `U.S`, and `end.Next` (no space after the period).

**Title test:** copy the word; optionally uppercase its first byte. Search the 87-entry title list (`Mr.`, `Dr.`, `St.`, `Prof.`, `e.g.`, `i.e.`, `vs.`, `cf.`, …), case-sensitive.

**Abbreviation test,** on the part after the word's last `-`:
1. Search the 1415-entry abbreviation list, case-sensitive.
2. If the part starts uppercase, search its lowercase form; this needs every byte to lowercase.
3. If the part is longer than 2 bytes, has no vowel (A E I O U Y, either case) and has a lowercase byte: abbreviation (`Ltd.`, `Blvd.`).
4. If the part is longer than 3 bytes, of even length, with letters at even positions and `.` at odd ones: abbreviation (`U.S.`, `U.S.A.`).

Steps 2–4 apply only to parts that start uppercase.

**List search:** binary search with **signed** byte comparison. A key matches only an entry of exactly its length. All the lists are sorted that way.

Case tests use Windows' character classification for code page 1252 (section 12).

### 6.3 Helpers

**Bracket stack:**
- 50 entries of `{kind: () {} [], start}`.
- A group-begin token pushes its start. A group-end pops if the stack isn't empty; the kinds aren't compared.
- The 51st push stores its kind in the first entry's start. Further pushes set "overflowed", after which the stack does nothing and `top` fails.

**Quote counter** keeps counts of double quotes, single quotes, possessives, digit quotes and guillemets:
- « adds 1 and » subtracts 1, whether opening or closing.
- Otherwise, the byte counted is the quote token's byte, or the last byte of any other token:
  - `"` counts ±1 (opening/closing);
  - `'` counts as a possessive if flagged as an apostrophe, else single quotes ±1.
- **Normalise:** clamps negative counts to 0.
- **Open:** double quotes > 0 or guillemets > 0. **Single quotes never matter.**
- **Trailing apostrophe:** for a word ending in `'`, count a possessive if the byte before is `s`/`S` and the word is longer than 1, else close a single quote.

**Predicates:**
- **Ordinary word:** type 3, at least 2 bytes, not starting with a digit, no `.` after its first byte, and not made only of `IVXivx`. A word that isn't ordinary can be a list label.
- **Initials:** type 3, odd length ≥ 3, uppercase `.` uppercase … (`U.S`, `A.B.C`).
- **Starts with a digit:** type 3.
- **Number word:** lowercase form in `zero`…`nine`.
- **Sentence starter:** uppercase first byte and in the 96-entry starter list (`A`, `About`, …, `However`, `I`, `If`, …, `The`, `Then`, …, `You`, `Your`), case-sensitive.
- **Equals ignoring case:** same length and equal under case-insensitive comparison.

### 6.4 State machine

**Per-sentence variables:**
- `cur`: the token being looked at;
- `prev`, `prev2`: the last two accepted tokens, initially type 0;
- `prevType`: `prev.type`, sampled at the top of each step;
- `words`: +1 whenever a new token of type 3–5 is fetched;
- `labelEnd` (where a list label ended), `quoteEnd` (where an opening quote ended), `bracketSentence`, `atBreak`, `final`: all 0;
- a bracket stack and a quote counter.

**Each step:** fetch the next token (it becomes `cur` if it differs), then act on `cur.type` = T in the current state.

**Notation:**
- **accept:** take the token and shift `prev2 = prev; prev = cur`;
- **→ n:** go to state n without accepting, so the same token is seen again;
- **A→ n:** accept, then → n.

The text keeps going only through these rules. T ≤ 0 means "no token".

| State | Rules |
|---|---|
| 0 start | T≤0 → 2. T 1–2: accept. T 8: push, A→ 1. T 11: count quote (open), accept. Else → 2 |
| 1 after `(` | T≤0 → 2. 1–2: accept. 3–7 → 2. 8–9: bracket update, accept. Else → 2 |
| 2 first token | T≤0 → 7. 1–2: accept. T 3 and not an ordinary word: save `words`, `cur`, `prev`, `prev2`, the stack and the counter; mark the scanner; A→ 3. Else → 7 |
| 3 after a label | T≤0 → 5. 1–2: A→ 6. 9: bracket update, A→ 4. 10 that is `.`: A→ 4. Else → 5 |
| 4 after label punctuation | T≤0 → 5. 1–2: `labelEnd = cur.start`, `words = 0`, A→ 7. 9: bracket update, accept. 10 `.`: accept. Else → 5 |
| 5 rewind | rewind the scanner, restore everything saved in state 2, → 7 |
| 6 after label + space | 9: bracket update, A→ 4. Else → 5 |
| 7 in the sentence | T≤0: accept. 1–2: A→ 18. 3–5: `bracketSentence = 0`, `final = 0`, trailing apostrophe + normalise, then: **T 3:** accept; length 1 and not a digit → 11; odd length ≥ 3 → 8; else stay. **T 4:** accept; → 12 unless `words` = 1 and `cur` equals (ignoring case) `PS.` or `P.S.`. **T 5:** if `cur` is `Dr.` or `St.`, `prev2.type` ≠ 0 and `prev2` starts uppercase: `cur.type` = 4, A→ 12; else accept. 6–7: A→ 17. 8: push, accept. **9:** next = 7; if `top` gives P with P ≥ `labelEnd`: next = 22, `bracketSentence = 1`; but if any of the first (P − `labelEnd`) bytes from the sentence start is not class 7: next = 7, `bracketSentence = 0`. Pop; A→ next. 10: `final = 1`, A→ 14. 11: A→ 10. 12–13: accept. 14–15: paragraph end, → 23. 16: A→ 17 |
| 8 after an odd-length word | T 10 `.` and `prev` is initials: A→ 9. Else → 7 |
| 9 after initials + `.` | T 14–15: `final = 1`, → 23. Else → 7 |
| 10 after a quote | `digitStart` = `cur` is a word starting with a digit. If `prev2.type` ∈ {0, 1, 8}: count `prev` (open), `quoteEnd = cur.start`. Else: `open = (prev.start == quoteEnd)`; if open, `quoteEnd = cur.start`; else `apostrophe` = `prev` is `'` and `cur` is non-empty and starts with a letter or digit; count `prev` (open, apostrophe, digitStart). Normalise. → 7 |
| 11 after a single letter | T 10 `.`: A→ 12. Else → 7 |
| 12 after an abbreviation | T≤0 → 7. 1–2: length ≤ 1 → A→ 13, else `final = 1`, → 19. 3–8 → 7. 9: `prev2.type` = 8 → 7, else `final = 1`, → 19. 10: `final = 1`, → 19. 11: `"` or » → `final = 1`, → 19, else → 7. 12–13 → 7. 14–15: paragraph end, → 23 (with `final = 1` unless `prev` is `No.` and `prev2.type` = 0). 16: `final = 1`, → 19 |
| 13 after abbreviation + one space (`prev2` = the abbreviation, or `.` after an initial) | T<3 → 7. **3–5:** if `prev2` is `no.`, `fig.` or `figs.`: `cur` starts with a digit or is a number word → 7, else `final = 1`, → 23. Otherwise: `cur` doesn't start uppercase → 7; `prev2.type` ∉ {4, 10} → 7; `prev2` is `in.` or `etc.` → `final = 1`, → 23; `cur` is not a sentence starter → 7; the byte after `cur` is past the end or is `.` → 7; else `final = 1`, → 23. 6–8 → 7. 9–10: `final = 1`, → 19. Else → 7 |
| 14 after `. ! ?` | `final = 1`. T≤0 → 7. 1–2 → 19. **9:** `bracketSentence = 1`; if `top` gives P and any of the first P bytes of the sentence is a letter or digit: accept, `bracketSentence = 0`; → 19. 10 → 19. 11: if `'`: count (close), normalise, accept; → 19. 16 → 19. Else → 7 |
| 15 after a pause + space | T<3 → 7. 3–5: starts uppercase → `final = 1`, → 23; else → 7. 6–7 → 7. 8–10: `final = 1`, → 19. 11 → 23. Else → 7 |
| 17 after an ellipsis or smiley | `final = 1`. T 1–2: A→ 15. Else → 7 |
| 18 after a space | T 6–7: accept. → 7 |
| 19 check quotes | S = open quotes ? 21 : 20; handle the same token in S |
| 20 closing, 21 closing inside quotes (S) | T≤0 → 23. **T 1:** if `bracketSentence` = 0, length 1 and `prevType` = 11 → A→ 22. Else if S = 21 → accept, stay. Else if `bracketSentence` = 0, length 1 and (`prevType` ∈ {11, 9} or `top` succeeds) → A→ 22. Else if `prev2.type` = 3, `prevType` = 10, `prev` is a lone `.` and `prev2` equals `at` (ignoring case) → `atBreak = 1`, A→ 22. Else A→ 23. **2:** accept. **3–8** → 23. **9:** had = `top` gives P; pop; if not had or P = 0, `bracketSentence = 1`; accept. **10:** accept. **11:** count (close), normalise, accept; if `"`: S 20 → 21, S 21 → 19. **12** → 7. **13** → 23. **14–15:** paragraph end, → 23. **16:** accept. Else → 23 |
| 22 maybe end | T≤0 → 7. 1–2: accept, stay. 3–5: `atBreak` = 0 → (starts uppercase ? 23 : 7); else (`cur` is `no.` ? 7 : 23). 8 → 23. 9 → 19. 11: `prevType` = 1 → 23, else → 7. Else → 7. Then `atBreak = 0` in every case |
| 23 | end of the sentence |

**Ending the sentence:**
1. Fetch the next token. If it is a paragraph end (type 14), accept it.
2. The sentence's length is the next token's start.
3. If the last token was accepted, it is inside the sentence. Otherwise the sentence ends before it.
4. A CR straight after the break is always pulled into the sentence.

Unused: state 16, critique messages ("Sentence is too long", "Space After Punctuation", …), per-sentence statistics, two small word lists. There is no sentence length limit here (the analyser has one, 8.4).

### 6.5 In words

- **`. ! ?` + whitespace** end the sentence, with these exceptions:
  - the word before is `at`;
  - a quote or `)` came just before a single space, or a bracket is still open. Then only a capitalised word, `(`, a quote after whitespace or a paragraph end ends it.
- **Inside open double quotes or « »,** the whitespace is kept and the next word ends the sentence.
- **`. ! ?` without whitespace** don't end it: `end!Next`, and also `Stop.\Pau=500\ Go`, because the escape that follows is not a gap.
- **Abbreviations** end the sentence before 2+ spaces, `)` (unless opened just before), `.`, `!`, `?`, `"`, », a smiley or a paragraph end. After one space, see state 13.
- **Titles** never end it, except `Dr.`/`St.` after a capitalised word (`Main St.`).
- **Ellipses and smileys** followed by whitespace end the sentence before a capitalised word, `(`, `)`, `.`, `!`, `?` or a quote.
- **List labels** at the start (`1.`, `a)`, `IV.`, `(2)`) stay with the sentence.
- **Paragraph ends and the end of the text** always end it.

Examples (`|` marks the breaks):
- `Hello world. | How are you?`
- `Dr. Smith went to Washington. | He arrived at 5 p.m. | Then he left.`
- `I live on Main St. Next to the bank.` (one sentence: `Next` is not a starter)
- `See fig. 3 for details. | It is good.`
- `He said "Stop." | Then he left.` but `He said "Stop." she said.` is one sentence.
- `The U.S. Army is here. | The U.S.A. is big.`

The gap before a sentence's first word absorbs that word's `ESC N<k> ␠`, so the reference ends up at the end of the previous sentence: `ESC AP ESC N0 Hello ESC N1 world. ESC N2 ␠\r\nHow ESC N3 are ESC N4 you?␠\r\n`.

## 7. Segments

The final text is cut at every `ESC I<n> ␠`:
1. The first segment (up to the first `ESC I`) is analysed.
2. Then each `ESC I<n>` sets the pitch (`n` = 0: the mode's default, otherwise `n` Hz, interface.md §5) and the next segment is analysed.

So **`\Pit=` and `\Rst\` always end a sentence**, and the analyser never sees `ESC I`.

**Analysing a segment:**
1. Copy it and append 0x1a (end of text).
2. Read sentences (section 8) until the reader reports "nothing left". Each read starts at the first byte the previous one didn't consume, normally the CR that ended its sentence, which the next read skips as whitespace; the offsets in its event records count from there. A segment of only whitespace and escapes has no sentence. Its `ESC N<k>` entries are still posted, so `\Mrk=` and similar still fire.

The analyser is set up once per voice: one sentence per line, rate 5 (normal), and events for sentences, words, phones and markers. Its other settings (speaker 1, pitch level 5, sentence pause 2) never change.

## 8. Reading a sentence

The reader copies one sentence into **tier 1** of the analyser state.
- The state holds five parallel byte tiers of 0x580 cells each and a per-cell **anchor count**. Cells are numbered from 1. The tiers start as spaces when the voice is created.
- In the original, cells beyond a sentence's end keep whatever earlier sentences left there; they are never cleared. Nothing reads them (rules.md §1), so the Go engine reads every sentence into a fresh state.
- What the reader keeps from one read to the next (the duration style, and the positions the length limits use) lasts as long as the voice, as in the original.

### 8.1 Bytes

| Input byte | Becomes |
|---|---|
| 0x1a | end of text |
| CR, LF, TAB, space | whitespace (stored as a space) |
| < 0x0e (other than the above) or > 0xaf | space |
| 0x0e–0xaf (other than the above) | itself. No case change |

So Latin-1 letters 0xb0–0xff never reach the rules. Section 5 has already folded or named most bytes ≥ 0x80.

### 8.2 Escapes

The front end sends only these. All others are unreachable.

| Escape | Effect |
|---|---|
| `ESC A P` | duration style P. It always comes before any content |
| `ESC N<k>` | the number is read up to whitespace (at most 64 bytes), and the whitespace is consumed; marker `k` is anchored at the next tier-1 cell to be written (its anchor count + 1). The rules keep anchors with their cells (rules.md §7), and `prosody.md` turns them into times |
| `ESC H<n>` | a pause event (type 0x20, value `n`) that nothing downstream reads. It is queued and anchored like a marker, so it takes a place when the records are handed out (prosody.md §7). The whitespace after the number is not consumed: it is read as the next byte, so it is stored as a space once the sentence has content |
| `ESC "` | 0x11 and `"` are written into tier 1 |
| `ESC @x` | 0x11, `@`, `x`, space are written into tier 1 |
| `ESC [addr]` / `ESC [\addr]` | address mode on / off. Space, 0x11 and the bracket text up to and including `]` are written into tier 1 (the copy stops at index 0x432), each byte of the text adding 1 to the weight; then a space. A repeated "on", or an "off" while off, is taken out again with its weight (its bytes stay in tier 1 until later bytes overwrite them). Address mode lasts until the end of the read: a sentence cut by a length limit (8.5) while it is on gets space, 0x11, `[\addr]` and a NUL after its end |
| `ESC /` | space, 0x11, `/` into tier 1; raw (phonetic) mode toggles |

An escape inside a word ends the word first (8.4).

The reader also knows escapes for analyser settings (rate, speaker, sentence mode, …) and for aborting, which the FrontEnd never sends. The Go reader skips each of them with the byte after it.

### 8.3 The sentence loop

The front end always uses line mode. For each sentence:
1. Skip whitespace.
2. Copy characters into tier 1 until a CR or LF that comes after content, or the end of text. That CR/LF is not consumed.
3. A word starts at a non-whitespace byte and ends at whitespace. Ending a word runs 8.4. The CR/LF test comes **before** the word-ending step, so a word directly followed by a line break is never ended. The front end's ` \r\n` ensures every word is followed by a space.

**Weight:** each byte stored adds 16 if it is a digit or one of `()[]><$%~+-/\{}*_`, and 2 otherwise. Marker bytes from escapes add nothing, except the text of an `ESC [` bracket (8.2).

**Events.** Each read queues event records, each with its type, its offset in the text the read started at, a length and a value:
- the sentence (type 1), first in the queue although it is added last, anchored at cell 1;
- markers (type 8, value `k`) and pauses (type 0x20, value `n`), anchored at the next cell to be written;
- every word that isn't a lone `. ? ! : ;` (type 2), anchored at its first cell.

prosody.md §7 hands them out by the anchor counts.

**Raw mode:** CR, LF and TAB are dropped; every byte adds 1; there is no word handling. The sentence is cut when the weight passes 0x428 at a byte that is not a digit or one of `[](),`.

**End of text** with nothing read reports "nothing left".

### 8.4 Ending a word

For the word in tier 1, from `start` to `end`:
1. **Repeated symbols.** Let `b` be the first byte and `j` the end of its run.
   - If `b` is not a letter or digit and the run is at least 3 long (10 for `.`), the word is dropped.
   - If `b` is a letter or digit and the run is at least 10 long, the word is dropped too, but the reader still thinks it is inside a word.

   A dropped word ends a sentence that already has content. Dropped: `---`, `!!!`, `..........`, `aaaaaaaaaa`. Kept: `..`, `...`. A dropped word's bytes stay in tier 1 past the sentence's end. The rules never read past the edges (rules.md §1), so nothing reads them.
2. **Final punctuation.** If the last byte is one of `. ? ! : ; ,`, it is set aside.
3. **Lexicon replacement** on the rest (below). If it fails and the set-aside byte is `.`, the word is tried again with the period (`Dr.`, `St.`). Any set-aside byte is put back after the result.
4. **Word events.** A word that is a single punctuation byte gets no word event. The analyser's word events are not used by the engine.

**Lexicon replacement:**
- **Splitting the word:**
  - `prefix`: bytes that are not letters and not one of `- ' .`;
  - `core`: letters and `- ' .`;
  - `suffix`: the rest, which must contain no letter.
  - No core, or letters after the suffix: no replacement.
- **Lookup:** the core is looked up case-insensitively, first in the user lexicon and then in the built-in exception list (160 words: `AIFF`, `ALLAH`, `APRIL`, `AUTOEXEC`, …, `VOLKSWAGEN`, `WAV`).
- **Result:** the word becomes `prefix + " \x10 " + phones + " \x10 " + suffix`, where 0x10 brackets phonetic text that the rules keep as it is.
- **Pronunciations** are ARPAbet with stress digits, converted to the analyser's phone alphabet (`pronunciation.md`): `ALLAH` (`AA1 L AH0`) → `'al$`.
- Letters here are ASCII. Bytes 0x80–0xaf are an open question (section 12).

### 8.5 Length limits

Both source-byte limits are 0x500 (the defaults; nothing sets them). Checked at each stored byte outside raw mode:

| Condition | Ends the sentence at |
|---|---|
| 0x500 source bytes read, after one of `, ; :` | the next space |
| 0x500 source bytes read | the next whitespace |
| weight > 0x490 (about 584 letters) | the next `, . ! ? ; :`; also arms both source limits |
| weight > 0x4a0 | the start of the current word (the reader backs up to it) |

The last two are what limit long sentences in practice: about 580 ordinary characters, or about 70 digits.

## 9. Transcription by rules

After reading, the analyser state holds the sentence in tier 1, cells `start`..`end`. Transcription runs three rule passes ([rules.md](rules.md)):
1. **Module C, group 50:** copies tier 1 to tier 2 and blanks tiers 3–5 first.
2. **Module B, groups 16, 6 and 3.**
3. **Check:** if either module reports an error, or tier 1 holds only `#`, `~` and spaces, the sentence is dropped.

**What the rules do.** This is data, not code: the Go engine runs the same rules from the data pack.

**Module C, group 50: normalisation, then one decision per word.**
- **Normalisation:**
  - abbreviations with built-in pronunciations (`Dr.` → `` '`dak-t$r ``);
  - `(s)` / `(es)` → "x or xs";
  - punctuation → tier-3 marks;
  - quotes, ordinals, currency.
- **Per space-delimited token,** the first that applies:
  - inline phonetic text (0x10 … 0x10) is kept;
  - dictionary lookups (`pronunciation.md`);
  - Roman numerals;
  - acronyms;
  - phone numbers;
  - number ranges after a month;
  - numbers with units and money;
  - spelling letter by letter;
  - otherwise morphology and letter-to-sound.
- **Numbers** become phonetic words (group 20). Dates, times, ranges, units, money, phone numbers (`+` → "country code") and digit-by-digit reading (`0` → "oh") have their own groups. Spelled letters use group 29.
- **Afterwards:** tier 1 is a phone string with stress marks (`'` primary, `` ` `` secondary), `-` syllable breaks, spaces between words and `#` boundaries. Tier 2 keeps the spelling aligned. Tier 3 holds part of speech and punctuation marks. Tier 4 holds syllable and stress codes.

**Module B, groups 16, 6 and 3:** part-of-speech disambiguation, `\Prt=` overrides (the `0x11 @x` markers), phrase breaks and pauses, and plural and possessive endings.

**Character set mismatch.** The rules were written for a DOS code page (437): their accented letters are 0x81–0x96 and 0xa0–0xa4. The front end sends Windows-1252, where those bytes are curly quotes, dashes and other symbols.
- Normally this doesn't matter: section 5 has turned every byte ≥ 0x80 into ASCII, a name or a space.
- When special tokens give up (a 100-byte word), raw 1252 bytes 0x80–0xaf reach the rules and are read as code page 437 letters. For example 0x91 (‘) is the vowel æ there.

We reproduce this.

**Where each kind of input is handled:**

| Input | Where |
|---|---|
| Emoticons `:-)` `;)` `:(` | special tokens (5.3) |
| Other faces `8-)` `:-I` | sentence breaker keeps them as one token; rules read the characters |
| URLs, e-mail, file names, IP addresses, dotted dates | special tokens: spoken with "dot", "slash", "at" |
| `~5` | special tokens: "approximately 5" |
| Non-ASCII characters | special tokens: folded to ASCII letters or named |
| A single quoted character `"!"`, or a one-character text | special tokens: its name |
| 4-digit numbers outside 1492–2099 | special tokens add a comma; rules read the number |
| Numbers, ordinals, Roman numerals, dates, times, money, units, phone numbers, ranges | rules, module C |
| Abbreviations | sentence breaker (where sentences end); rules and the built-in exceptions (how they are read) |
| Acronyms, spelling | rules, module C |
| Symbols `%`, `$`, `&`, `+`, `#`, `@` | rules (`+` spacing in special tokens) |
| Repeated symbols `---`, `!!!` | dropped by the reader |

## 10. Quirks we don't reproduce

These are memory-safety bugs. The Go engine does the stated thing instead:
- **System code page.** The original uses whatever the system's ANSI code page is; with a double-byte code page its buffer is too small. We always use Windows-1252, the code page of every English Windows.
- **Text starting with whitespace.** Restoring whitespace at the very first byte reads the byte before the text. We treat it as "not CR".
- **Flat special-token buffer.** Writes past the end of the last record corrupt the heap in the original. We drop them.
- **Bracket stack.** The 51st push also writes past the stack. We drop that write and keep the effect inside the stack (6.3).
- **ESC N token.** A marker or pause token of 64 bytes with no whitespace makes the reader recurse until the stack overflows. The front end never produces one; we treat it as ending at 64 bytes.
- **Tier 1 overflow.** Escapes that store markers without weight (`ESC "`, `ESC @x`, `ESC /`), hundreds of them with no ordinary byte between them, make the original write past tier 1 into the other tiers. The Go reader stops writing at the last cell and flags the read, and the engine drops that sentence.
- **Empty sentence.** If the sentence breaker found a sentence of length 0, the original would loop forever. The Go engine takes the rest of the text as one sentence; none of the tested texts reaches it.
- **Tier growth.** Rules that grow a sentence past the tier size are handled as a rule error (rules.md §8).
- **Quoted tag values.** A quoted value of 78 characters or more (a `\Chr=` name) makes the original write a NUL one byte past the tag scanner's 80-byte buffer, which can corrupt its heap. When it survives, it accepts 78 characters and rejects longer values (3.3). We do the same without the overflow.
- **Unknown `\Prt=` first.** With nothing formatted yet in the call, the original appends uninitialised stack bytes (3.3). We append nothing.

## 11. For the data pack

The data pack carries these tables:
- **IPA:** IPA table, engine phoneme names, internal phone symbols.
- **Tags:** context names; character names for `\Chr=` ("Normal", "Monotone", …).
- **Special tokens:** character names (256), accent folding (128).
- **Sentence breaker:** character classes (256); title (87), abbreviation (1415), starter (96) and number-word (10) lists, kept in their original order.
- **Lexicon:** the built-in exception list (160).
- **Rules:** the rule modules (rules.md §10).

They are in `text.json` (datapack.md §11). It also needs a Windows-1252 best-fit table (UTF-16 → byte, and `?` for the rest) and the 1252 character classification (upper, lower, alpha, alphanumeric, and lower/upper case mapping). Both were recorded on the development machine through the Windows API (section 12).

## 12. Open questions

1. **Character classification.** `IsCharUpperA`, `IsCharLowerA`, `IsCharAlphaA`, `IsCharAlphaNumericA`, `CharLowerBuffA`/`CharUpperBuffA` and case-insensitive `CompareStringA` on bytes 0x80–0xff in code page 1252. They decide titles, abbreviations and starters such as `Évian.` (ª, º and µ may count as lowercase). *Answered* (recorded on the development machine): uppercase are A–Z, Š Œ Ž Ÿ (0x8a, 0x8c, 0x8e, 0x9f) and 0xc0–0xde except ×. Lowercase are a–z, ƒ š œ ž (0x83, 0x9a, 0x9c, 0x9e), ª µ º (0xaa, 0xb5, 0xba) and 0xdf–0xff except ÷. Alphabetic is the two together; alphanumeric adds 0–9 and ¹ ² ³ (0xb9, 0xb2, 0xb3). The case mappings pair them, except that ƒ, ª, µ, º and ß have no uppercase and ÿ uppercases to Ÿ. The Go engine treats `CompareStringA` with `NORM_IGNORECASE` as equality of the lowercased bytes; every literal it compares with is ASCII. That is exact here: `EqualsIgnoreCase` compares lengths first, the words it compares never hold a soft hyphen, and no one-byte substitution in a title, abbreviation, starter or number word compares equal other than by case. Tag names are compared differently (§3.3).
2. **`isalpha` on bytes 0x80–0xaf.** The lexicon split calls MSVCRT `isalpha` on signed bytes, which reads outside its table. Find what the shipped `msvcrt.dll` returns. *Answered:* false for every byte from 0x80, as for the control bytes. The lexicon's word check uses the same function and refuses each of them (pronunciation.md §6.3).
3. **Best fit.** The exact `WideCharToMultiByte` output for every BMP character in code page 1252 with no flags. *Answered:* 707 UTF-16 code units map to something other than `?` (recorded on the development machine). A character outside the BMP gives `??`, one per surrogate.
4. **Sentence breaker.** Confirm it against the original engine. *Answered:* sections 2–6 were checked against the original engine on 23,587 texts: identical. That check found the two positions in 6.4 (`labelEnd` and `quoteEnd` had been one) and the tag details in 3.3.
5. **Stale cells.** Does any rule read tier cells beyond the sentence end? If so, the persistent state matters to the output. *Answered:* no (rules.md §1), so the Go engine reads every sentence into a fresh state (section 8).
