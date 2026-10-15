# TruVoice: the input side

How L&H TruVoice (`tv_enua.dll`) reads the text it speaks into the pipeline's nodes. The speaking thread's input side takes each text the engine queued: the **feeder** writes it into the **input ring** a few lines at a time, and the thread pumps the ring through the **TextIn tokenizer**, which splits it into tokens and expands some of them, such as `£` into ` pound `. The **escape parser** reads what the tokenizer gives it a character at a time: it keeps the text's characters and turns its commands, escape sequences such as `ESC[150p`, into a stream of characters and commands in the **command ring**. In the scheduler's steps ([pipeline.md](pipeline.md) §6), the **input stage** makes the nodes of a word at a time from the command ring and hands them to the normalizer.

This spec covers the escape parser (section 1), the input stage (section 2), their reset (section 3), the TextIn tokenizer (section 4), the feeder (section 5) and the speaking thread's input side (section 6).

**In short.**
- The escape parser makes line ends and tabs spaces and drops the other control characters. In text it keeps the characters below 128, drops 128 to 191, and writes 192 to 255 as one or two letters: `é` as `ay`, `ß` as `ss`, `Ñ` as `Nj`.
- A command is `ESC[`, up to 16 decimal values separated by `;`, and a letter. The parser keeps its values within their ranges, updates its settings, and writes the command with the values it carries. A value over 255 or more than 16 values make it a command of letter 0. Some commands act at once and aren't written: hold, a reset, the defaults.
- The input stage reads a word: its characters up to the spaces after them, at most 10 nodes. A character becomes a node of type 1, a command a control node; `[` and `]` become phonetic input commands when input flag 6 is on. The nodes of bookmarks and words carry the text being read.
- The tokenizer reads up to 210 tokens: words, numbers, punctuation, symbols, line ends and escape sequences. It splits words at letters next to digits and at dots: `abc123` is `abc` and `123`. It says the symbols `¢ £ ¥ § © ® ± ¶ ¼ ½ ¾ × ÷` as words, `-5` as `minus 5`, `aaa` as `AAA`, `unknown` and `unsent` as `un known` and `un sent`, `am` after a number as `AM`, and `jan` and `mar` next to a number as `January` and `March`, and puts `ESC[C` before a quoted exclamation. It keeps the hundred tokens before the one it expands, and gives the parser what the tokens before them say, or all of them at the text's end. In a mode only escape sequences turn on, it announces quoted text in German.
- The feeder writes a text into the input ring ten lines at a time, and breaks a line of more than 200 characters at a space. It lowercases lines in capitals in a text of 41 characters or more, ends a short line after a line end with a dot, makes a line of punctuation a dot in a text of 131 characters or more, and ends the text with `ESC[0i ESC[C`.
- The speaking thread takes the next text when the last is all fed, the tokenizer has found the input ring empty and the escape parser has run out. Until a text is all fed, the thread feeds more of it whenever the ring has room for more than 2,048 characters, and pumps the ring when the escape parser runs out.

**Exactness.** Ported from the original's code, and checked against it by replaying every call of its escape parser and input stage recorded in 170 runs: the 88 runs of the check set (6 sentences at every mode, tagged texts, other formats and gains, attributes and an untagged tag), the tokenizer's 38 runs and the feeder's 20 (sections 4 and 5), 20 texts with every command letter, without values, with one value and with more, at the ends of their ranges and past them, the flags' commands with `[` and `]` and without, hold and continue, the defaults, letters no command has, values over 255, more than 16 values, an empty first or last value, a digit after the 16th semicolon, leading zeros, characters next to the digits among a command's values, escape characters inside a command and not followed by `[`, control characters, tabs and line ends, commands inside words, long words and runs of spaces, the characters 128 to 255 in text and in phonetic input, a long text, and two texts in one speaking thread. Each call left the speaking thread's state byte-identical to the original's: the 186,902 characters given to the parser and the 26,055 steps of the input stage. Of the parser's characters, 125,753 went into the command ring as they were and 93 of 192 and up as their letters, 33 of them as two; 64 of 128 to 191 and 7 control characters were dropped, and 383 line ends and tabs became spaces. 5,892 escape characters started 5,888 commands, 4 of them among another's values: 5,483 of 22 command letters, all but `S`, `x` and `n`, with their values in and out of their ranges, 5 of letter 0, from three values over 255 and two of 17 values, and 400 of 13 letters that are no command; the one digit for the 17th value came. In the runs of long words the command ring ran out of room: 41 commands and 6,328 characters weren't written. The input stage made 125,396 nodes, 119,545 of them characters and 5,851 control nodes, 4,811 of them words and 226 bookmarks; 6 of the 17 `[` and `]` it read became phonetic input commands; it stopped 6,005 words at ten nodes and found the ring empty 3,284 times.

The input stage was also checked as a whole, in the pipeline, from the texts, in the 1,005 recorded runs of section 6: each of its 58,015 steps started from the original's state, reported the same and left the speaking thread's whole state the same, and the audio was the original's PCM in every run. 678 of the 696 `[` and `]` it read became phonetic input commands.

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
| `g`, `s` | `n` < 2 | `v` below 0 becomes 100, 0 becomes 1, and above 255 255 (section 7) | `v` |
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

The command ring holds 4,096 bytes: the escape parser writes at its write position and the input stage reads at its read position, each moving on by one and back to 0 after 4,095. Its **room** is `read − write − 1`, plus 4,096 when that is negative: 4,095 bytes less those it holds. A byte goes in only if there is room for it. Taking a byte from the ring gives it, or −1 when the ring is empty.

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

At the pipeline's reset ([pipeline.md](pipeline.md) §7) the input ring is emptied (both positions 0) and marked drained, the escape parser reads text, the command ring is emptied, no character is pending, and the input stage's flags become 0x17c0 and its input flags 0x41. The parser's values, the value its digits go to and the overflow flag keep theirs, and the tokenizer keeps its tokens and modes: a new text empties its list (section 4.5). The feeder's position, and whether the text is all fed, keep theirs too.

## 4. The TextIn tokenizer

Between the input ring and the escape parser, the text passes through the TextIn tokenizer. Each time the speaking thread pumps the input ring ([pipeline.md](pipeline.md) §8.2), the tokenizer reads tokens into a list, expands some of them and gives the escape parser what the tokens before the last hundred say (section 4.5).

The original reads a registry setting that can turn the tokenizer off: the escape parser then gets the input ring's characters as they are, up to 400 at a time. Without the setting, as in the port, the tokenizer is on.

**Exactness.** Ported from the original's code, and checked against it by replaying, in each of the 170 recorded runs, every pump of the input ring in order through one Go tokenizer, made new at each start of the speaking thread as the original makes its own: each of the 3,500 pumps gave the escape parser the original's characters and left the speaking thread's state the original's. The tokenizer's 38 runs are 16 texts for its rules (the symbols, negative numbers, repeated vowels, the split words, `am`, the months, exclamations in quotes, splits, punctuation, spaces and control characters, the characters 128 to 255, the modes, long words and escape sequences, escape sequences right after punctuation, and 700 and 300 tokens), a tagged text, 9 runs with texts given without a notification sink, which have no word marks (quote mode with each kind of prefix, a text's first tokens, two texts in quote mode), and 12 runs of long lines of long words, which the feeder cuts so that the input ring runs dry inside words and escape sequences; the feeder's runs (section 5) add texts with NULs, which it reads as spaces. The tokenizer read 35,506 tokens, 24,476 of them without characters, 375 line ends and 54 symbols, and 5,695 escape sequences; it dropped 362 escape characters not followed by `[`, cut 10 tokens at 100 characters, took 685 trailing characters off and gave 931 characters back; the ring ran dry inside a token twice, inside an escape sequence three times and after an escape character 16 times. It split 2,510 tokens into 2,535 more, kept marks that split nothing in 634, and made 364 numbers as it read them and 2,500 as it split. It expanded 53 symbols, 20 minus signs, 23 repeated vowels, 15 split words, 13 `am` and 17 months (5 after a number and 12 before one, 3 of those with a dot or hyphen made a space), and put 11 exclamation commands and 11 word starts; 8 tokens `-$` became `-` with `$`; escape sequences set quote mode 10 times, phonetic mode 19 times and word start 14 times; it kept 27 tokens from expansion in phonetic mode and 37 held ones. It checked 207 lines for quotes, announced 38 quoted texts and 37 ends (one text ended in quoted text), and made 7 quotes spaces. 3,275 of its pumps expanded ten tokens and 225 ended a text.

### 4.1 The input ring and the token classes

The input ring holds 4,096 characters, as the command ring does (section 2.1): the feeder writes at its write position and the tokenizer reads at its read position. Taking a character gives it, or −1 when the ring is empty, which marks the ring **drained**. Giving a character back moves the read position back by one: whatever character was taken last is taken again.

Each character has **token classes**, the data pack's `tokenClasses` ([datapack.md](datapack.md) §10):

| Bit | Class | Characters |
|---|---|---|
| 0x01 | separator | tab, line feed, carriage return, space |
| 0x02 | trailing | tab, space, the escape character, `! " ' , . : ; ?` and `` ` `` |
| 0x04 | letter | `A` to `Z`, `a` to `z`, and 0xC0 to 0xFF but `×` and `÷` |
| 0x08 | digit | `0` to `9` |
| 0x10 | (not read) | `* + - / < = >` |
| 0x20 | punctuation | `! " % & ' ( ) * + , - . / : ; < = > ? @ [ \ ] ^ _` `` ` `` `{ \| }` and the symbols |
| 0x40 | read again | line feed, carriage return, the escape character, `@`, `\` and the symbols |

The **symbols** are the characters the data pack's `symbols` give a word: `¢ £ ¥ § © ® ± ¶ ¼ ½ ¾ × ÷`. The other characters, `#`, `$` and `~` among them, have no class.

### 4.2 Reading a token

A **token** has characters, up to 100 but in an escape sequence (a NUL among them ends what it says of them), its **length**, a **replacement** it says instead of its characters, its **delimiter**, the character it says after them, or none, the **marks** of its characters, which say where it splits, and whether it is a **number**, an **escape sequence**, or **held** from expansion. The tokenizer keeps a list of tokens, from the first to the last, the **current** one, which it expanded last, and how many there are.

To read a token, the tokenizer takes characters from the input ring into a new one:
1. With 100 characters, the token ends.
2. If the ring is empty, the token ends, and with no characters there is no token.
3. The escape character ends a token that has characters. At a token's start, the tokenizer reads escape sequences (below) and goes on with the character after them; with none, there is no token.
4. A symbol at a token's start is its only character and ends it. Otherwise the symbol is given back, and ends the token.
5. A line feed at a token's start is its only character and ends it. Otherwise it ends the token.
6. A NUL is read as a space. A separator ends the token: at its start, it is a token without characters.
7. A dot in a **word**, a token that began with a letter, is marked as a dot. Otherwise punctuation ends a word or a number, a token that began with a digit; in other tokens it is **leading** punctuation.
8. A digit at a token's start makes it a number. Elsewhere, it ends the token after leading punctuation or right after punctuation; right after a letter, it is marked as a split. A digit makes the token no longer a word.
9. A letter at a token's start makes it a word. Elsewhere, it ends the token after leading punctuation; right after a digit, it is marked as a split. A letter makes the token no longer a number.
10. Any other character, or one that didn't end the token, goes into it.

The character that ended the token is its **ending**, but at steps 1, 2 and 4, and at a line feed at the token's start, which leave none. When the token has ended, unless all its characters are leading punctuation, each trailing character at its end is taken off and given back: the read position moves back by one for each, and the token's delimiter becomes the last one taken off. Then, if the class of the ending (or of the last character taken off) has bit 0x04, 0x08 or 0x40, the read position moves back by one more and the token has no delimiter; otherwise that character is its delimiter. The token goes at the end of the list. A token `-$` without delimiter becomes `-` with `$` as its delimiter. A token that began with a digit and met no letter is a number; any other token keeps its marks, if a dot or a split was marked.

So `ab.` before a space is the token `ab`, said with `.`, and `ab..` the token `ab` with `.`, then the token `.`, as the second dot is taken again. `ab.£` is `ab` with `.`, then `.` and `£`: the dot is said twice.

**Escape sequences.** After an escape character, the tokenizer:
1. takes the next character; another escape character is skipped, and the one after it taken;
2. if the character isn't `[`, it is the character after the escape sequences;
3. takes the characters up to one with class 0x04: `ESC[`, they and that letter make a token at the end of the list, an escape sequence, whose length is its number of characters;
4. takes the next character: if it is the escape character, goes back to step 1; otherwise it is the character after the escape sequences.

If the ring runs out on the way, there is no character after them. So an escape character followed by anything but `[` is dropped, and the escape parser only ever gets escape sequences that begin with `ESC[`. Escape sequences of more than 20 characters crash the original (section 7).

### 4.3 Splits

A token with marks is split: going through its characters, at a character marked as a split, the characters since the last split become a new token before it, without delimiter; at a dot, they become a new token before it with the dot as its delimiter, and the dot is dropped. The token keeps the characters after the last split, and its marks go. If it split, it and the new tokens whose characters (up to a NUL) are all digits `0` to `9` become numbers.

So `abc123def` is `abc`, the number `123` and `def`, `U.S.A` is `U` and `S`, each said with `.`, and `A`, and `7am` is the number `7` and `am`.

The original also checks every token it reads for being all digits, and clears the mark of the character after a dot. A token of digits began with a digit and met no letter, so it is a number already (section 4.2), and the character after a dot is never marked: a digit there ends the token. The port does neither. The original also classifies each token with flags no rule reads: its capitals, vowels, punctuation and operators, a number's place in a date or a time, and the lines of quoted text whose quote is the whole token (section 4.6).

### 4.4 Expanding a token

To **expand**, the tokenizer takes the token after the current one, or the first token when there is no current one; with none, there is nothing to expand. The token becomes the current one. In **phonetic mode** only escape sequences are expanded, and a held token isn't. When **word start** is on and the token's first character is an ASCII letter, a token of the data pack's `wordStart`, `~`, of length 1, goes before it, and word start goes off. Then the token is expanded by these rules, in order (a token's characters are those up to a NUL):

1. A token `-` without delimiter or with `$`, followed by a number, says the data pack's `minus`, `minus `, when there is no token before it, or the one before it has a space as its delimiter, or is a line end without delimiter.
2. A token of three or more of the same lowercase vowel `a`, `e`, `i`, `o`, `u` or `y` says them in capitals: `aaa` says `AAA`.
3. An escape sequence whose fourth character is `X`, `I` or `e`: with `X` the **quote mode** becomes its third character's value, as a signed byte, minus 48 (`0`), with `I` the phonetic mode, and with `e` word start goes on. So `ESC[4X` sets quote mode 4, and `ESC[1I` phonetic mode 1, which `ESC[0I` ends: phonetic mode is on while it is 1.
4. A token of one symbol says its word: `£` says ` pound `.
5. A token `unknown` or `unsent` (the data pack's `splitWords`) says `un known` or `un sent`.
6. A token of length 2 that is `am` (`meridiem`) says `AM` when the token before it is a number with a space or no delimiter, skipping escape sequences and tokens of length 0 with a space or no delimiter.
7. A token of length 3 that is `jan` or `mar` (`months`) says `January` or `March` next to a number: when the token before it, skipping escape sequences and tokens of length 0 with a space as delimiter, is a number with a space, a hyphen or no delimiter; or else when the token after it, skipped the same way, is a number and the month has a space, hyphen, dot or no delimiter, a hyphen or dot then becoming a space. So `12 mar` says `12 March` and `jan-5` `January 5`.
8. A token with `!` as its delimiter: going back from the token before it, until a token with `.`, `!` or `?` as its delimiter, the first token of length 1 whose character is `"`, `\` or `(` gets the data pack's `exclamation`, `ESC[C`, after it as a token of length 0. So `"Hello!"` gives the parser `"ESC[CHello!"`, which clears the synthesizer's hold (section 1.4).

The words are compared as Windows' `lstrcmpiA` compares them on a system with code page 1252 ([whistler/text.md](../whistler/text.md) §3.3): ASCII case doesn't count, and nor do soft hyphens (0xAD), so `UNKNOWN`, and `unknown` with a soft hyphen in it, say `un known`.

### 4.5 A pump

The speaking thread pumps the input ring (section 6), with a new text or not:
1. With a new text, the list is emptied (there is no current token), the stored quote prefix forgotten and quoted text ended (section 4.6). The modes stay.
2. The tokenizer reads tokens while the list holds fewer than 210, or until there is none. It splits each (section 4.3), and in quote mode 4 checks for quoted text at a token read right after a line end (section 4.6).
3. Unless the input ring is drained and the text is all fed, it expands ten tokens, and says the tokens more than a hundred before the current one.
4. Otherwise it expands all the tokens left and says them all.

A token says its replacement, or else its characters up to a NUL, then its delimiter, if any. The tokenizer drops the tokens it says. Each character it says goes to the escape parser (section 1); when there were any, the scheduler is no longer out of input ([pipeline.md](pipeline.md) §6). With no current token, it says nothing.

### 4.6 Quote mode

In quote mode 4 the tokenizer announces quoted text: lines that begin with the same **quote marks**, the data pack's `quoteMarks`, `! @ # % * + | > < :`, as in quoted e-mail. Since the engine puts a word's command, an escape sequence, before each word of a text given with a notification sink (the engine's spec will say how), only a text given without one has a token right after a line end that isn't an escape sequence.

The tokenizer stores a **quote prefix** of up to 7 characters, ending at a NUL, how many of them a line's quote takes, and how many lines have **repeated** it, and whether it is in **quoted** text. It looks at the first five characters of a token read right after a line end, or at all of them and at its delimiter, in the place after them, when it has fewer than five:
- **matched** counts the places whose character is the stored prefix's at the same place, and the **last quoted** place is the last place with a quote mark (or a NUL among the characters) at which every place so far matched;
- a token without characters forgets the prefix (its first character becomes NUL);
- without a quote mark (or a NUL among the characters), the prefix is forgotten;
- with a stored prefix and a last quoted place, the line's quote takes the places up to it;
- otherwise the token's characters and delimiter become the prefix, and the repeats become 0.

Outside quoted text, a last quoted place adds 1 to the repeats, and when that makes them more than 1 the quoted text starts: from the token, three times, the tokenizer goes back to the first token of its line, makes the line's quote spaces in it if the token is longer than the quote, and goes back to the token before. Before the token it is at, it puts a token saying the data pack's `quoteStart`, `. Achtung: Anfang des eingesetzten textes.  `, with a space as its delimiter.

In quoted text, a line without a last quoted place ends it: a held token saying `quoteEnd`, `. Achtung: Ende des eingesetzten textes.  `, with a space as its delimiter, goes before the token, and the repeats become 0. A line with one has its quote made spaces in the token, if the token is longer than the quote. The original also makes the repeats 0 at a token without characters, and with a new prefix stores how many places the quote takes, up to the last quote mark: neither is read before it is set again.

So when the third line in a row beginning with `> ` is read, the quoted text is announced before the line end before the first, and the next line without the quote is announced as its end.

## 5. The feeder

The **feeder** writes the text the speaking thread is reading into the input ring (section 4.1). The text is as the engine queued it: given with a notification sink, it has a word's command before each word and ends at its first NUL; given without one, it keeps its NULs (the engine's spec will say how). Each time the thread feeds it (section 6), the feeder reads up to ten **lines** of the text from its **position** (section 5.1) and writes them (section 5.2). It keeps whether the last line it wrote **ended with a line end**.

**Exactness.** Ported from the original's code, and checked against it by replaying, in the 170 recorded runs of section 4, each of the 291 feedings of a text into the input ring, 225 of them from a text's start, with the text the speaking thread was given: each left the speaking thread's state the original's. The feeder's 20 runs are texts given with a notification sink and without: lines in capitals, with `ESC[1I`, a lowercase `a` or `z`, and the characters 128 and 255, short lines after a line end ending in punctuation and not, lines of 39, 40 and 41 characters, lines of punctuation with and without a digit, a symbol and characters from 192, line ends of each kind and in pairs, trailing spaces and tabs, empty lines, lines of escape sequences, a run of 201 characters without a space, lines of more than 200 characters and of more than 1,000, ten lines and more, texts of 40 and 41 characters and of 130 to 132, texts longer than the input ring, NULs and right quotes, an empty text, and texts that leave the input ring room for 10 to 17 characters before their end of text, or room for a line of 80 characters and not of 81, or for 10 characters before an empty line. The feeder read 1,064 lines: 382 ended at a line feed and 4 at a carriage return, 3 of them with a line end of the other kind after it, 374 broke at a space for their characters and 71 for their length, and 233 were a text's last; 54 feedings stopped at ten lines and 16 at a line the ring had no room for. It lowercased 20 lines (and left one in capitals, in a text under 41 characters), and ended 291 short lines with a dot; 112 short lines after a line end ended in punctuation and 8 didn't follow one. It wrote 10 lines of punctuation as dots (3 more were in texts under 131 characters), 375 line feeds, 7 right quotes as apostrophes and 7 NULs; of 225 ends of text, 4 lost their last characters, 20 in all, to a ring that had no room for them.

### 5.1 Reading lines

A line starts at the position, and the feeder reads the text's characters into it:
1. A line feed or a carriage return ends the line. It is skipped, with a line end of the other kind right after it. The line's trailing spaces and tabs are taken off, and it ended with a line end.
2. Otherwise, a line that has more than 200 characters other than spaces, tabs and its escape sequences', or more than 1,000 in all, ends before its last space: the position goes back to the space, with which the next line starts.
3. Otherwise, `ESC[` and the characters after it up to a letter (`A` to `Z` or `a` to `z`) go into the line as an escape sequence.
4. Any other character goes into the line.

After a line has ended, a new line starts, unless that was the tenth. When the text ends first, the line being read, without its trailing spaces, tabs and NULs, is the last line, if anything is left of it. The position is then after the lines read.

A line is in **capitals** unless it has a lowercase letter `a` to `z`, a character from 128 to 255, or the escape sequence `ESC[1I` (exactly: it starts phonetic input). It is **punctuation** unless it has a letter `A` to `Z` or `a` to `z`, a digit, a symbol (section 4.1), a character from 192 to 255 or an escape sequence.

When the line has no space after its first character at step 2, the original looks for one before the line, and loses its way (section 7). The port ends the line where it is.

### 5.2 Writing lines

The feeder writes the lines into the input ring, in order. A character goes in only while the ring has room for 10 (the ring's room is 4,095 characters less those it holds, as the command ring's, section 2.1). A right single quote, 0x92, goes in as an apostrophe.
1. A punctuation line with characters, in a text of 131 characters or more, goes in as a dot `.`.
2. Otherwise, a line with characters goes in, unless the ring's room less 10 is no more than its length: the feeder then stops, and its position goes back to the line's start. The line's characters go in lowercased (`A` to `Z`) when it is in capitals and the text has 41 characters or more, but for its escape sequences, which go in as they are. A dot follows when the line is **short** after a line end: the line before it ended with a line end (for the first line, the last line the feeder wrote before did, or the line is the text's first); it has fewer than 40 characters, but at least one, outside its escape sequences and the spaces right after them; and the last of those that isn't a space or a tab isn't `.`, `!`, `?`, `,` or `;`.
3. A line that ended with a line end is followed by a line feed.

The feeder keeps whether the last line it wrote ended with a line end. If its position has reached the text's end, it then writes the **end of text**, `ESC[0i ESC[C` and two spaces: a bookmark of value 0, which marks the text's end ([pipeline.md](pipeline.md) §4.1), and a continue (section 1.4). The text is then **all fed**. With room for fewer than 18 characters, only the first `room − 9` of them go in, and with room for fewer than 10, none. If the position moved on, the input ring is no longer drained.

So, in a text of 41 characters or more, a line `THE CAT.` goes in as `the cat.`, a line `Hello` after a line end as `Hello.`, and in a text of 131 characters or more a line `----` as `.`.

The original reads a registry setting that can turn the feeder's lines off: the text's characters then go in as they are, while the ring has room for 10. Without the setting, as in the port, the lines are on.

## 6. The speaking thread's input side

Each time round its loop ([pipeline.md](pipeline.md) §8.2), before its step, the speaking thread:
1. if the text it was feeding is all fed, the scheduler is out of input ([pipeline.md](pipeline.md) §6) and the input ring is drained, takes the first text the engine queued. The feeder's position is the text's start, as after a line end; the text is the one the nodes of the input stage's bookmarks and words carry (section 2.3); the engine's window is told it has started; the text is fed (section 5), and the input ring pumped as a new text (section 4.5). The thread is then **busy**. Without a queued text, a thread that isn't busy ends ([pipeline.md](pipeline.md) §8.5);
2. otherwise, if the text isn't all fed and the input ring has room for more than 2,048 characters, feeds it more;
3. otherwise, if the scheduler is out of input, pumps the input ring; the thread is busy.

A text stays first in the engine's queue until it is all fed. A step that reports idle makes the thread no longer busy. A new thread starts as if its last text were all fed, so that it takes a text first.

**Exactness.** Ported from the original's code, and checked against it in 1,005 recorded runs: the 923 runs of the stages after the input side ([lexical.md](lexical.md), [prosody.md](prosody.md), [parameters.md](parameters.md)), and the 82 runs of sections 1 to 5 but the check set's. Go speaking threads ran each from the texts the original's took, with the Go normalizer ([normalizer.md](normalizer.md)) and every stage after it, and nothing taken from the recordings. Before each of the 894,238 steps, after the thread's input side had fed and pumped, the speaking thread's whole state was the original's. The 1,009 threads took the 1,080 texts where the original's did, 40 of them given without a notification sink, and ended where they did; they fed more of a text 68 times and pumped the input ring 3,660 times, 945 of them before the text was all fed, and found no text 1,009 times while busy. The texts started and ended in the original's order, and the audio was the original's PCM in every run, with every word and bookmark at the original's sample.

## 7. What the recorded runs don't reach

- An escape character followed by anything but `[` at the escape parser: the tokenizer drops them, and passes only escape sequences that begin with `ESC[` (section 4.2). An escape character among a command's values starts a new command: the runs have four. One followed by anything but `[` among a command's values would be dropped with that character.
- Escape sequences of more than 20 characters, counting the escape character: the tokenizer copies a sequence onto the stack, into 20 bytes, with a NUL after it, and with 21 or more the original crashes. The port reads them whole. So a value's digits never go past the 17th value, and the 8-bit count of semicolons never wraps.
- A digit for the 17th value (after 16 semicolons): the original tests whether the 32-bit word after the 16 values is −1 and writes 0 there if it is. In the speaking thread's state that word is the normalizer's position in its program, which is never −1, so the digit does nothing; the port does nothing. The runs have one.
- A command written without values with room for exactly 2 bytes in the command ring: the original checks for room for 2 bytes and writes 3, so that only the escape character and the letter would go in.
- The commands that make the original drop the rest of the text without reporting it done: `S` and `x` ([pipeline.md](pipeline.md) §9), and bookmarks and words whose node has flag bit 5, from a first value with bit 2 among three; and `ESC[0i`, a bookmark of value 0, which marks a text's end ([pipeline.md](pipeline.md) §4.1).
- The command `n` from the text: its value is the address of a pronunciation the engine's `\Prn=\` tag makes, which the normalizer reads. The user lexicon's part will say how.
- The limits of 255 of `g`, `s` and the second value of `t`: a value over 255 has overflowed, which makes the command one of letter 0.
- Hold set while the scheduler steps: the runs set it only with `ESC[H` and clear it with `ESC[C` before the next step.
- The tokenizer's own cases the speaking thread doesn't give it: a pump with no tokens, and a pump without a new text after the end of a text (the original's list then still has its last token, which it has freed; the thread waits for a new text, [pipeline.md](pipeline.md) §8.2). The feeder never puts a carriage return into the input ring: the original's tokenizer would read it as a space, as it reads a NUL. An escape sequence with a NUL right after its `[`: the original keeps its characters up to the NUL and reads its fourth character from memory it hasn't set (section 4.4, rule 3); the port reads none. Giving a character back when the input ring has no room: the original then leaves the read position, but it has always just taken a character.
- A line that grows too long without a space after its first character (section 5.1 step 2): a run of more than 201 characters without a space, not counting tabs and escape sequences, or of more than 1,000 characters. The original looks for the space back from the character it was about to read, and on past the line's start. With a notification sink, each word follows its command and a space, so it finds that space at the line's start, and makes the same empty line over and over: the thread feeds nothing more and never ends. Without one, it finds a space in a line before, and feeds the text from there over and over; or it reads the memory before the text, where finding a space ends the text there and finding none would crash it. The port ends the line where it is, so that the run goes in whole.
- An end of text with room for fewer than 10 characters, which doesn't go in at all (section 5.2): the runs left room for 10 to 17. A line of punctuation, the dot after a short line and a line feed go in without the room check of a line, so that a text can end with less room.
