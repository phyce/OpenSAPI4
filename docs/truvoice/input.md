# TruVoice: the input side

How L&H TruVoice (`tv_enua.dll`) reads the text it speaks into the pipeline's nodes. The speaking thread's input side ([pipeline.md](pipeline.md) §8.2) feeds the text, and the **escape parser** reads it a character at a time: it keeps the text's characters and turns its commands, escape sequences such as `ESC[150p`, into a stream of characters and commands in the **command ring**. In the scheduler's steps ([pipeline.md](pipeline.md) §6), the **input stage** makes the nodes of a word at a time from the command ring and hands them to the normalizer.

This spec covers the escape parser (section 1), the input stage (section 2) and their reset (section 3). The feeder and the TextIn tokenizer, which give the escape parser the text's characters, are added with the parts that port them.

**In short.**
- The escape parser makes line ends and tabs spaces and drops the other control characters. In text it keeps the characters below 128, drops 128 to 191, and writes 192 to 255 as one or two letters: `é` as `ay`, `ß` as `ss`, `Ñ` as `Nj`.
- A command is `ESC[`, up to 16 decimal values separated by `;`, and a letter. The parser keeps its values within their ranges, updates its settings, and writes the command with the values it carries. A value over 255 or more than 16 values make it a command of letter 0. Some commands act at once and aren't written: hold, a reset, the defaults.
- The input stage reads a word: its characters up to the spaces after them, at most 10 nodes. A character becomes a node of type 1, a command a control node; `[` and `]` become phonetic input commands when input flag 6 is on. The nodes of bookmarks and words carry the text being read.

**Exactness.** Ported from the original's code, and checked against it by replaying every call of its escape parser and input stage recorded in 112 runs: the 88 runs of the check set (6 sentences at every mode, tagged texts, other formats and gains, attributes and an untagged tag), 20 texts with every command letter, without values, with one value and with more, at the ends of their ranges and past them, the flags' commands with `[` and `]` and without, hold and continue, the defaults, letters no command has, values over 255, more than 16 values, an empty first or last value, a digit after the 16th semicolon, leading zeros, characters next to the digits among a command's values, escape characters inside a command and not followed by `[`, control characters, tabs and line ends, commands inside words, long words and runs of spaces, the characters 128 to 255 in text and in phonetic input, a long text, and two texts in one speaking thread. Each call left the speaking thread's state byte-identical to the original's: the 31,119 characters given to the parser and the 3,517 steps of the input stage. Of the parser's characters, 10,558 went into the command ring as they were and 79 of 192 and up as their letters, 24 of them as two; 45 of 128 to 191 and 5 control characters were dropped, and 3 line ends and tabs became spaces. 2,076 escape characters started 2,074 commands, 2 of them among another's values: 2,057 of 22 command letters, all but `S`, `x` and `n`, with their values in and out of their ranges, 4 of letter 0, from two values over 255 and two of 17 values, and 13 of 11 letters that are no command; the one digit for the 17th value came. The input stage made 12,733 nodes, 10,655 of them characters and 2,078 control nodes, 1,641 of them words and 130 bookmarks; 6 of the 10 `[` and `]` it read became phonetic input commands; it stopped 113 words at ten nodes and found the ring empty 226 times.

The input stage was also checked as a whole, in the pipeline, in the 923 recorded runs of the stages after it ([lexical.md](lexical.md), [prosody.md](prosody.md), [parameters.md](parameters.md)), with what the escape parser and the normalizer wrote taken from the recordings: each of its 33,795 steps started from the original's state, reported the same and left the speaking thread's whole state the same, and the audio was the original's PCM in every run. 672 of the 679 `[` and `]` it read became phonetic input commands.

## 1. The escape parser

The parser's state:
- what it reads: **text**, an **escape** character, or a command's **values**;
- the 16 **values** of the command it reads, each −1 while empty, the **value** its digits go to, 0 to 255, and whether a value has **overflowed**;
- the **settings** ([pipeline.md](pipeline.md) §3.1): phonetic input, phrase, rate index, pitch, attenuation, filter, flags, input flags and voice, which a reset gives to every cursor, its **speed** in words a minute, and its **track levels**, which a reset gives to the parameter generator ([pipeline.md](pipeline.md) §7).

The original passes each character through a ring of 256 characters and reads the ring to its end before it takes the next one, so that the ring never holds a character between two. The port parses each character as it comes.

### 1.1 A character

1. If the prosody stage's **prosody off** flag ([prosody.md](prosody.md) §3.1) isn't set and more than 608 nodes are free, the **short phrase** flag ([pipeline.md](pipeline.md) §1) is set.
2. A carriage return, line feed or tab becomes a space. The characters 0 and 127, and the others below 32 but the escape character 27, are dropped: the parser does nothing more with them.
3. The escape character makes the parser read an escape, whatever it read.
4. Otherwise, by what it reads:
   - text: the character goes into the command ring as its letters (section 1.2);
   - an escape: `[` makes it read a command's values: every value becomes empty, the value its digits go to 0, and no value has overflowed. Any other character is dropped, with the escape character, and the parser reads text;
   - values: section 1.3.

### 1.2 Text

A character below 128 goes into the command ring as it is. The characters 128 to 191 are dropped. The characters 192 to 255 go in as the data pack's one or two letters for them (`highCharacters`, [datapack.md](datapack.md)):

| | 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | A | B | C | D | E | F |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| C0 | A | A | A | A | A | O | Ae | S | Eh | Ay | Eh | E | I | I | I | I |
| D0 | D | Nj | O | O | O | O | O | space | O | U | U | U | U | Y | th | ss |
| E0 | a | a | a | a | eh | o | ae | ss | eh | ay | eh | e | i | i | i | i |
| F0 | th | ny | o | o | o | o | o | space | o | u | u | u | u | y | th | y |

Each letter goes in only if the ring has room for it (section 2.1).

### 1.3 Values

While the parser reads a command's values:
- A digit `0` to `9`, if the value its digits go to is below 16: the value becomes 0 if it is empty, then `value · 10 + digit`, in 32-bit arithmetic. A value over 255 has overflowed. A digit for a later value does nothing.
- A `;` adds 1 to the value the digits go to, in 8-bit arithmetic; if that is below 16, the value becomes 0.
- Any other character is the command's letter (section 1.4). The parser then reads text.

So `ESC[;5V` has two values, the first empty, and `ESC[5;V` two, the second 0.

### 1.4 Commands

At the letter, the command has `n` values: none when the value the digits go to is 0 and the first value is empty, otherwise that value plus 1. If a value has overflowed or `n` is over 16, the letter becomes 0. `v` is the first value, −1 when empty. The command then does what the table says, and is written into the command ring with the values it carries, or **without values**, or not at all:

| Letter | When | What it does | Written |
|---|---|---|---|
| `A`, `D` | always | The input flags numbered 1 to 7 among the values (flag `k` is bit `k − 1`) are set (`A`) or cleared (`D`) in the input flags setting | As `A`: the input flags' low byte and second byte |
| `N`, `F` | always | The flags numbered 1 to 16 among the values are set (`N`) or cleared (`F`) in the flags setting | As `N`: the flags' low byte and second byte |
| `C` | `n` is 0 | The synthesizer's **hold** is cleared ([synthesizer.md](synthesizer.md) §5.2) | With 0 values |
| `H` | `n` is 0 | Hold is set | Not written |
| `S` | `n` is 0 | A reset of the stages is requested ([pipeline.md](pipeline.md) §6) | Not written |
| `x` | `n` is 0 | The parsed reset flag is set ([parameters.md](parameters.md) §17.3) and hold is cleared | With 0 values |
| `w` | `n` is 0 | The settings get their defaults ([pipeline.md](pipeline.md) §3.1), and the speed 150; every cursor gets the settings | Not written |
| `I` | `n` < 2 and `v` < 2 | `v` below 0 becomes 0. The phonetic input setting becomes `v` | `v` |
| `P` | `n` < 2 and `v` < 2 | `v` below 0 becomes 1. The phrase setting becomes `v` | `v` |
| `V` | `n` < 2 | `v` is kept from 0 to 9. The voice setting becomes `v` | `v` |
| `a` | `n` < 2 | `v` is kept from 0 to 16. The attenuation setting becomes `v` | `v` |
| `c` | `n` < 2 | `v` outside 0 to 2 becomes 0 | `v` |
| `f` | `n` < 2 | `v` is kept from 0 to 9. The filter setting becomes `v`, and the rate index setting at most the data pack's `filterRates[v]` | `v` and the rate index setting |
| `g`, `s` | `n` < 2 | `v` below 0 becomes 100, 0 becomes 1, and above 255 255 (section 4) | `v` |
| `i`, `n`, `z` | `n` < 5 | `v` below 0 becomes 1, and `n` 1 | The `n` values |
| `l` | `v` isn't empty, `n` < 3 and `v` < 22 | The second value, empty, becomes the generated frame's level of track `v` ([datapack.md](datapack.md) `generatedFrame`), and otherwise at most the data pack's `levelLimits[v]`. The track level setting `v` becomes it | `v` and the second value |
| `p` | `n` < 2 | `v` below 0 becomes 42 and the pitch setting 84 Hz; `v` below 25 becomes 25 and the pitch 50 Hz; otherwise `v` is at most 200 and the pitch `2v` Hz | `v` |
| `r` | `n` < 2 | The speed setting becomes `v` when `v` is 50 or more. `v` from 1 to 49 becomes 50, the speed too. Otherwise the speed becomes 150 and `v` stays. Then `v` above 0 becomes `(v − 46) >> 3`, and the command goes on as `v` does, but for the speed | `v` |
| `v` | `n` < 2 | `v` below 0 becomes 13, and above 25 25. With a filter setting other than 0, `v` is at most `filterRates[filter]`. The rate index setting becomes `v`, and, for `v` only, the speed `8v + 50` | `v` |
| `t` | `n` < 4 | `v` below 0 becomes 0. The second value, empty, becomes 100, otherwise it is kept from 1 to 255; the third, empty, becomes 50, otherwise it is kept from 30 to 243 | `v` and the second and third values; without values when `v` is over 9 |
| Any other, or when the condition fails | | Nothing | Without values |

So `ESC[r` makes the speed 150 and the rate index 13, `ESC[0r` the speed 150 and the rate index 0, and `ESC[30r` the speed 50 and the rate index 0. `ESC[2I` writes `I` without values, so that the cursors' phonetic input becomes 0, though the setting stays.

A command is written as the escape character, the letter, the number of values it carries and each value's low byte, the first value as changed above; written without values, the number is 255 and no value follows. It is written only if the command ring has room for 3 bytes more than its values (2 without values).

## 2. The input stage

### 2.1 The command ring

The command ring holds 4,096 bytes: the escape parser writes at its write position and the input stage reads at its read position, each moving on by one and back to 0 after 4,095. Its **room** is `read − write − 1`, plus 4,095 when that is negative: 4,094 bytes less those it holds while the write position is at or after the read position, 4,095 less those while it is before. A byte goes in only if there is room for it. Taking a byte from the ring gives it, or −1 when the ring is empty.

The input stage also keeps the **pending** character, which it took from the ring and hasn't made a node of, or −1, and its own copies of the **flags** and **input flags**, which the nodes of its `N` and `A` commands set.

### 2.2 The step

The input stage's step makes the nodes of the next word:
1. If no character is pending, take one from the ring. If the ring is empty, report that there was no word.
2. Up to ten times, with the pending character:
   1. If it is a space, the word has come to its spaces. Otherwise, if it had, stop.
   2. Make its node at the end of the list:
      - the escape character: a command's node (section 2.3);
      - `[`, when its input flags have bit 0x20 (flag 6) on and its flags have bits 0x3 (flags 1 and 2) off: a control node `I` of value 2 if the input flags have bit 0x40 (flag 7) on, else 1. `]`, in the same case: a control node `I` of value 0;
      - any other character: a node of type 1 with the character as its symbol.
   3. Take the next character from the ring as the pending one. If the ring is empty, stop.
3. Report that there was a word.

A node is made by taking a free node and linking it before the list's last sentinel ([pipeline.md](pipeline.md) §2.3). The normalizer's window then ends at it, and each of the normalizer cursor's `first`, `scan`, `done` and `start` that is none becomes it ([pipeline.md](pipeline.md) §3).

### 2.3 A command's node

A command's node is a control node (type 0) whose symbol is the command's letter, the byte after the escape character. The next byte is the number of its values:
- 1: the value is the next byte, the second value 0;
- 2: the value is the next byte, the second value the one after it. At `A`, the input stage's input flags become `second · 256 + value`; at `N`, its flags;
- 3: the next byte `b` sets the node's flag bits: bits 3 and 4 become `b`'s bits 0 and 1, bit 5 is set if `b` has bit 2 and bit 6 if `b` has bit 3. The value is the next byte and the second value the one after it;
- 4: the value is the next four bytes, the lowest first;
- any other: the value and the second value are 0.

With 1 or 4 values, the node of an `i` (a bookmark) or a `z` (a word) carries the text the speaking thread is reading ([pipeline.md](pipeline.md) §4.1). The original takes the bytes as they come: the escape parser writes a command whole, so they are always there.

## 3. The reset

At the pipeline's reset ([pipeline.md](pipeline.md) §7) the escape parser reads text, the command ring is emptied (both positions 0), no character is pending, and the input stage's flags become 0x17c0 and its input flags 0x41. The parser's values, the value its digits go to and the overflow flag keep theirs.

## 4. What the recorded runs don't reach

- An escape character followed by anything but `[`, and an escape character after an escape character: the input side before the parser drops them, and passes only escape characters followed by `[` (its part will say how). An escape character among a command's values starts a new command: the runs have one.
- Escape sequences of more than 20 characters, counting the escape character: they crash the original before they reach the parser, in its TextIn tokenizer (its part will say where). So a value's digits never go past the 17th value, and the 8-bit count of semicolons never wraps.
- A digit for the 17th value (after 16 semicolons): the original tests whether the 32-bit word after the 16 values is −1 and writes 0 there if it is. In the speaking thread's state that word is the normalizer's position in its program, which is never −1, so the digit does nothing; the port does nothing. The runs have one.
- The command ring without room: it held at most 1,164 bytes, as the input side gives the parser more of the text only after the input stage has found the ring empty ([pipeline.md](pipeline.md) §8.2). Without values, the original checks for room for 2 bytes and writes 3, so that with room for 2 only the escape character and the letter would go in.
- The commands that make the original drop the rest of the text without reporting it done: `S` and `x` ([pipeline.md](pipeline.md) §9), and bookmarks and words whose node has flag bit 5, from a first value with bit 2 among three; and `ESC[0i`, a bookmark of value 0, which marks a text's end ([pipeline.md](pipeline.md) §4.1).
- The command `n` from the text: its value is the address of a pronunciation the engine's `\Prn=\` tag makes, which the normalizer reads. The user lexicon's part will say how.
- The limits of 255 of `g`, `s` and the second value of `t`: a value over 255 has overflowed, which makes the command one of letter 0.
- Hold set while the scheduler steps: the runs set it only with `ESC[H` and clear it with `ESC[C` before the next step.
