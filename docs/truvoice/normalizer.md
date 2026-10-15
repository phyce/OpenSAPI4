# TruVoice: the normalizer

How L&H TruVoice (`tv_enua.dll`) normalizes the words the input stage made. The normalizer runs a **program**, from the data pack, on the nodes in its window ([pipeline.md](pipeline.md) §3): a small machine of conditions and actions, with calls, registers and lists of words. The program spells words as letters for the lexical stage, says numbers, abbreviations, states, directions and symbols as phonemes, runs the control commands, and reads phonetic input of two kinds: the phonemes the lexical stage reads, and, between `[` and `]`, a phonetic alphabet the normalizer converts itself.

This spec covers the nodes the normalizer works on (section 1), the program's machine (section 2), its reset (section 3), the phonetic alphabet (section 4) and what the program does (section 5).

**In short.**
- Each step goes on from where the last stopped: the machine keeps its place in the program, its calls and its registers between steps. A step stops when a condition needs a node the input stage hasn't made yet, or when the program yields, and hands on the nodes before the one it is reading.
- The program is a list of blocks. A block is a condition, a byte that gives the length of the actions to run when it passes and when it fails, and those actions; the next block follows, unless an action goes elsewhere. 19 conditions test the node being read, its class, the nodes ahead, a list of words, a flag or a register; 22 actions insert letters and phonemes, spell a word as letters, make nodes phonemes, delete nodes, set registers, run a control command, go to a block, call one and return, or yield.
- 22 lists of words give the phonemes of numbers, abbreviations, states, directions and symbols: `million` is `&Mi1jY@N`.
- A word becomes letters in capitals, with `@` for an apostrophe and `&` for a hyphen.
- Between `[` and `]`, each letter, alone or with the letter after it, is a phoneme: `AA` and `A` are `o`, `SH` is `s`, `NG` is `~`. A mark right before the `]` is kept for the next word's start: `*`, `\`, `/`, `=` and others. A digit before it gives the pending mark a second value, and two digits give it their first digit's character code times ten, at most 63: a slip of the original's.
- The marks `!p;d!` and `/p;d/` after a phoneme of phonetic input give it a pitch and a duration.
- The program reads numbers with commas whole, up to trillions, and numbers of three or four digits in pairs, as years: `1997` is "nineteen ninety-seven" and `555-1234` "five fifty-five, twelve thirty-four". It reads money, times, decimals, ordinals and plurals of numbers, expands abbreviations, titles and units, spells words of capitals and words without vowels, and makes the sentences' ends phonemes. Twelve flags switch spell mode, punctuation read out, numbers digit by digit, states, directions and other readings.

**Exactness.** Ported from the original's code, and checked against it by replaying every step of the normalizer recorded in 1,173 runs: the 1,005 runs of the stages in the pipeline ([input.md](input.md) §6, [lexical.md](lexical.md), [prosody.md](prosody.md), [parameters.md](parameters.md)) and 168 more for the normalizer. These speak numbers of every size, with commas, decimals and signs, ordinals, money, times, years, percentages and phone numbers; every word of the 22 lists, alone, capitalized, after and before a number, in capitals, with dots and inside other words; words with apostrophes and hyphens, in capitals, single letters, words without vowels, and the symbols alone, doubled and between letters; tags and commands between each two characters of numbers and abbreviations, and 15 to 25 of them before the next word; texts with each of the flags the program tests set or cleared and with the phrase setting 0, given with a notification sink and without, which leaves no word marks after the spaces; and phonetic input of both kinds: every letter alone and with each letter that pairs with it, in both cases, each mark before the end and elsewhere, after a phoneme and not, digits, pending values taken by the next word, by a phoneme that isn't a word mark or by `%` and `&`, the marks `!` and `/` with values at the ends of their ranges and past them, after phonemes and after other symbols, `<` and `>`. Each of the 250,277 steps started from the original's state, reported the same and left the speaking thread's state byte-identical to the original's.

The steps ran all 450 of the program's 454 blocks that can run (section 6); of the 367 conditions in them that can both pass and fail, 331 did both. They matched every one of the 373 words of the lists, inserted 46,635 letters and 273,577 phonemes, 33,432 replacements among them, spelled 46,332 words and made 24,994 words phonemes. 45,317 steps found no node of theirs; the others waited for a node 97,856 times, 82,686 of them at a control condition, 11,496 at the next control condition, 2,877 looking ahead and 797 at a phonetic symbol, and yielded 107,104 times. They returned from 444,922 calls, nested at most 7 deep. In phonetic input they read all 42 letter pairs and the 26 letters alone, 32 other symbols in 82 of their four cases (before the end or elsewhere, after a phoneme or not), two digits 17 times and one 13 times; the next word took the pending values 392 times; the marks `!` and `/` came 82 times and gave their values 39 times.

The normalizer was also checked as a whole, in the pipeline, in the 1,005 recorded runs of the stages. Go speaking threads ran each run from the texts the original's took, and ended where the original's did, a new one starting for a later text in 4 runs ([pipeline.md](pipeline.md) §8.5; [input.md](input.md) §6). The Go input side ([input.md](input.md)), scheduler, normalizer, lexical stage ([lexical.md](lexical.md)), prosody stage ([prosody.md](prosody.md)), parameter generator ([parameters.md](parameters.md)), output stage and synthesizer ran all 894,238 of the original's scheduler steps, with nothing taken from the recordings. The normalizer was reset at each of the 1,009 threads' starts, and each of its 129,599 steps started from the original's state, reported the same and left the speaking thread's whole state the same. The audio, 200,579,270 samples in 170,378 hand-overs, was the original's PCM in every run, and each of the 19,527 words and 60 bookmarks came at the sample where the original reported it.

## 1. The nodes

The normalizer's window holds the nodes the input stage made ([input.md](input.md) §2): control nodes (type 0) and characters (type 1). The program reads it with four of the cursor's pointers ([pipeline.md](pipeline.md) §3):
- **first**: the node the conditions test;
- **scan**: where the word being read starts; the program inserts its nodes before it;
- the **mark**, the cursor's first working node: where a word the lists match starts;
- the **search** node, its second working node, which matching and looking ahead move.

The program changes:
- a node's type: characters become letters (type 2) or phonemes (type 3);
- its symbol;
- its flag bit 0x20, value and second value: the pending values of phonetic input's marks (section 4.2), which the lexical stage reads as the word's code ([lexical.md](lexical.md) §3.6);
- flag bit 0x40 of the window's start: a word spelled while register 2 is set (section 2.3);
- flag bit 0x80, value and second value of a phoneme the marks `!` and `/` give values (section 2.5);
- byte 0x14, which becomes 1 on a capital it inserts and on a capital it spells, and byte 0x18, which becomes `-` on a letter that stands for a hyphen.

## 2. The program

The program is the data pack's ([datapack.md](datapack.md) §11): a list of **blocks**. A block is a **condition**, a byte from 0 to 18 and its operands, a **branch** byte, and two runs of **actions**: the branch byte's high four bits are the length in bytes of the run for when the condition passes, its low four bits that of the run for when it fails, which comes after. An action is a byte from 0 to 22 but 15, and its operands, or a **goto** or a **call**: two bytes, the first with bit 0x80 set, and bit 0x40 too for a call. The 14 low bits of the two bytes, the first's high, are the **place** they go to: an offset in the program, which starts at 0. Operands are bytes: a string is two, the high first, an offset into the data pack's string table, and the string ends at the first NUL from there.

The machine's state:
- its **place** in the program, and whether a condition comes next;
- the bytes **left** in the run of actions, and the bytes to **skip** after it;
- whether the program has **yielded**;
- 20 **calls**, each a place, the bytes left and the bytes to skip, and how many are open;
- 12 **registers** of a byte: the program uses 0 to 9;
- the list **entry** a match found last, and whether its **replacement** has been inserted;
- the **pending** values of phonetic input: a value, a second value and a flag (section 4.2).

### 2.1 The step

1. Begin with the cursor ([pipeline.md](pipeline.md) §3.2). If the window has no node of the cursor's types, report that nothing was handed on. If the mark is none, it becomes the scan node.
2. If a condition comes next, run it (section 2.2). If it waits, finish waiting (step 5). Otherwise no condition comes next, and the place moves on to the branch byte: if the condition passed, the bytes left become its high four bits and the bytes to skip its low four, and the place moves on to the run after it; if it failed, the bytes left become its low four bits, none are skipped, and the place moves on past the run its high four bits measure.
3. While bytes are left, the action at the place:
   - a goto, or a call, which first keeps the place, the bytes left and the bytes to skip in the next call: the place becomes the one it goes to, a condition comes next with one byte left and none to skip, and the step goes on at 2;
   - a yield, if the program hasn't yielded: it has, and the step finishes yielding (step 5). The place stays at the yield, which the next step reaches again, and passes;
   - any other: it runs (section 2.3), and the place moves on past it. Its bytes, with its operands, are no longer left.
4. A condition comes next, after the bytes to skip. Go on at 2.
5. When it waits, `done` becomes `start`, then moves on over control nodes and phonemes `]` and space, but not past the scan node. When it yields, `done` becomes the scan node. The cursor finishes ([pipeline.md](pipeline.md) §3.3): report whether it handed nodes on, and, after a yield, that it did.

So the scheduler counts a yield as work done, and runs the normalizer again ([pipeline.md](pipeline.md) §6).

### 2.2 Conditions

| Byte | Operands | Passes when |
|---|---|---|
| 0 | | The cursor's phonetic input setting isn't 0 |
| 1 | | Its phrase setting is 0 |
| 2 | `f` | Flag `f` is on: bit `f` of the cursor's flags, or bit `f − 16` of its input flags when `f` is 16 or more. `f` is below 32 |
| 3 | | In phonetic input of the second kind (the setting is 2), the step waits if first is the window's end, and otherwise reads first through the phonetic alphabet (section 4). Then first's symbol is a letter `A` to `Z` or `a` to `z`, a digit `1` to `5`, or one of the data pack's punctuation, `` "$%&),.?@[]\|~\`` and the space |
| 4 to 7 | | First's symbol is in class 4 to 7 |
| 8 | `c` | Looking ahead: the search node moves from first over the nodes after it. At the window's end the step waits. A control node of `C`, `F`, `I`, `N` or `x` stops the look: the condition passes if `a` is in class `c`. Any other control node is passed over. A node in class `c` stops it, and the condition passes; a space, `~` or `` ` `` is passed over, and any other node stops it. After 20 nodes passed over, the condition fails. `c` is one of the classes |
| 9 | `r v` | Register `r` is `v` |
| 10 | `r v` | Register `r` is above `v` |
| 11 | `c` | First's symbol is `c`, below 128 |
| 12 | `l` | List `l` matches, lowercased (section 2.4) |
| 13 | | The step waits if first is the window's end. Otherwise first moves on to the node after it, and the condition passes if it is a control node |
| 14 | | The step waits if there is no first. Otherwise first is a control node |
| 15 | | Always |
| 16 | | First's symbol isn't `C`, `F`, `I`, `N` or `x` |
| 17 | `s` | First's symbol is one of the string's characters |
| 18 | `l` | List `l` matches as it is |

The classes: 4, a letter `A` to `Z` or `a` to `z`, or `'`; 5, a digit; 6, a letter `a` to `z`; 7, a letter `A` to `Z`; 14, a symbol below 32 or from 128; 19, `.`.

### 2.3 Actions

| Byte | Operands | Does |
|---|---|---|
| 0 | `s` | Inserts the string's characters, in order, as letters (type 2; section 2.4) |
| 1 | `s` | Inserts them as phonemes (type 3) |
| 2 | | Skips: first, the scan node and the mark move on to the node after first, or none at the window's end |
| 3 | | Spells the word from the scan node to first (below), then skips |
| 4 | | Makes the nodes from the scan node to first, but control nodes, phonemes: their flag bit 2 is cleared and bits 0 and 1 set. Then skips |
| 5 | | Inserts the replacement of the entry a match found last as phonemes, unless it has been inserted |
| 6 | `r v` | Register `r` becomes `v` |
| 7, 8 | `r` | Register `r` goes up, or down, by 1, in 8 bits |
| 9 | | The mark and first become the scan node |
| 10 | | First moves on to the node after it, or none at the window's end |
| 11 | | First moves back to the node before it, or to the window's end when first is none |
| 12 | | Deletes first: if it is the scan node, the scan node and the mark first move on to the node after it. First becomes the node after it, or none at the window's end ([pipeline.md](pipeline.md) §2.3) |
| 13 | | Deletes the nodes from the scan node to first; the scan node, the mark and first become the node after them |
| 14 | `c` | Takes a letter (type 2) of symbol `c` after first, which becomes first ([pipeline.md](pipeline.md) §2.3) |
| 16 | `l` | Matches list `l`, lowercased, for its replacement |
| 17 | | Yields (section 2.1). The second time, it passes, and the program hasn't yielded |
| 18 | | Runs the scan node's control command ([pipeline.md](pipeline.md) §4) |
| 19 | | Returns: takes back the last call's place, bytes left and bytes to skip, and moves on past the call |
| 20, 21 | | The marks `!` and `/` (section 2.5) |
| 22 | | The mark becomes first |

Spelling the word from the scan node to first:
1. If register 2 isn't 0, the window's start gets flag bit 0x40.
2. If `done` is a node whose symbol is `-` and the node before the scan node is a `&`, that node's byte 0x18 becomes `-`.
3. Each node from the scan node to first: a `'` becomes `@`; a `-` becomes `&`, with byte 0x18 `-`; a letter `a` to `z` becomes its capital; a letter `A` to `Z` gets byte 0x14 1. The node becomes a letter: its flag bits 0 and 2 are cleared and bit 1 set.

### 2.4 Inserting and matching

A node the program **inserts** is taken before the scan node, or, when there is none, at the end of the list, which then ends the window, and starts it if it is empty ([pipeline.md](pipeline.md) §2.3). If its symbol is of the class word mark ([lexical.md](lexical.md) §1.2: `%`, `&` and `[`) and the pending flag is set, the node takes the pending values: its flag bit 0x20 is set, and its second value and value become the pending ones, which become 0, and the pending flag is cleared. A symbol `A` to `Z` gets byte 0x14 1.

A list **matches** when one of its words is spelled by the nodes from the mark to first, all characters (type 1), with their symbols `A` to `Z` lowercased, when the match is lowercased. The words are tried in their order; the first that matches is the entry, with its replacement not inserted. A match of list 1, 13 or 14, the digits, the teens and the tens, sets the number word flag ([pipeline.md](pipeline.md) §1).

### 2.5 The marks after a phoneme

The program reads a phoneme of phonetic input followed by `!` and values up to another `!`, or `/` and values up to another `/`, with the scan node at the phoneme. At the closing mark, action 20 sets the scan node's flag bit 0x80, for `!`, and action 21 clears it, for `/`. Then the values are read from the node after the opening mark: a value is a run of digits, read while it is 255 or less, and `;` goes on to the second value; reading stops at any other character, at a digit that takes a value past 255, or at a third value.

The first value is the **pitch** `p`: 0 when there is none, and 2 when it is 0; the second the **duration** `d`, 0 when there is none. If the scan node's symbol is of the class phoneme ([lexical.md](lexical.md) §1.2) and reading stopped:
- at a `!`, with `p` at most 300, which a value read to a mark always is, and `d` from 40 to 160, or 0: the node's second value becomes `(p − 100) >> 1` unless `p` is 0, and its value `d − 100` unless `d` is 0;
- at a `/`, with `p` from 50 to 200, or 2 or less, and `d` at most 60: the node's second value becomes `p >> 1` and its value `d`.

So `a!150;80!` gives `a` a second value of 25 and a value of −20, and `a/60;40/` 30 and 40.

## 3. The reset

The machine starts at place 0, with a condition next, no call open and not yielded, and the number word flag is cleared. The registers, the calls kept, the entry and the pending values stay.

## 4. The phonetic alphabet

In phonetic input of the second kind, which `[` starts and `]` ends when input flags 6 and 7 are on and flags 1 and 2 off ([input.md](input.md) §2.2), condition 3 reads first through the alphabet before it tests it. The node after first **ends** the input when it is a control node `I` of value 0, a `]`. First is **after a phoneme** when the node before it is a phoneme (type 3).

### 4.1 Letters

A letter, in either case, becomes the phoneme in the table, or, when the node after it has one of the letters that pair with it as its symbol, in either case, the pair's phoneme, and that node is freed ([pipeline.md](pipeline.md) §2.3).

| Letter | Alone | Pairs |
|---|---|---|
| A | `o` | AA `o`, AE `a`, AH `o`, AO `w`, AR `r`, AW `f`, AX `@`, AY `I` |
| B | `B` | |
| C | `C` | CH `C` |
| D | `D` | DH `x`, DT `t` |
| E | `e` | EH `e`, ER `k`, EY `A` |
| F, G | `F`, `G` | |
| H | `H` | HH `d`, HW `h`, HX `H` |
| I | `i` | IH `i`, IR `4`, IX `\|`, IY `E` |
| J | `J` | JH `J` |
| K | `K` | |
| L | `L` | LX `j` |
| M | `M` | |
| N | `N` | NG `~`, NX `~` |
| O | `o` | OR `g`, OW `O`, OY `y` |
| P | `P` | |
| Q | `Q` | QQ `q` |
| R | `R` | RR `3` |
| S | `S` | SH `s` |
| T | `T` | TH `X` |
| U | `u` | UH `u`, UL `l`, UM `m`, UN `n`, UR `c`, UW `b`, UX `v` |
| V, W | `V`, `W` | |
| X | removed | XX `p` |
| Y | `Y` | YR `5` |
| Z | `Z` | ZH `z` |

A **removed** symbol becomes 0: condition 3 fails, and the program deletes it.

### 4.2 Marks and digits

A mark that is **pending** `n` sets the pending value to `n` and the pending flag, and is removed. The next node inserted whose symbol is a word mark takes the pending values (section 2.4).

| Symbol | Before the end | Elsewhere |
|---|---|---|
| space `( ) , . ; ?` | kept | kept |
| `+`, `\|`, `}` | `[`, `\`, `]` | `[`, `\`, `]` |
| `!`, `"`, `~` | pending 9, or `"` after a phoneme | `"` |
| `#` | pending 11, or a space of value 8 after a phoneme | a space of value 8 |
| `$` | pending 13 | kept |
| `'` | `1` after a phoneme; otherwise kept, with the pending value 3 and the pending flag set | kept |
| `` ` `` | pending 2, or `2` after a phoneme | `2` |
| `-` | pending 5 | `1` |
| `=`, `*`, `\`, `:`, `{`, `_` | pending 1, 4, 6, 10, 12 and 17 | removed |
| `/` | pending 7 | before `\` and the end, pending 8, and the `\` is freed; otherwise removed |
| `%`, `&` | kept, taking the pending values if the pending flag is set: the node's value and second value become the pending ones, and the pending flag is cleared | the same |
| `<` | removed, and the synthesizer's hold is set ([synthesizer.md](synthesizer.md) §5.2) | removed |
| `>` | ends phonetic input: the node before it and it become control nodes of symbol 0, the end's `I` becomes `C`, the cursor's phonetic input setting 0, and hold is cleared | removed |
| a digit | removed, the pending second value becoming the digit, with the pending flag set | before another digit: removed, and the other digit is freed; if the end follows them, the pending second value becomes the first digit's character code times ten, in 8 bits, at most 63, and the pending flag is set. Otherwise removed |
| any other | removed | removed |

## 5. What the program does

The program is the original's, and the port runs it as it is. This section says what it does, routine by routine, with the place each starts at. The examples are what the input side and the normalizer make of a text, with the default settings unless a flag is given: a word in capitals, like `SMITH`, is a word of letters, which the lexical stage looks up ([lexical.md](lexical.md) §1.1 and §3.3), and words in quotes, like "twenty-five", stand for the phonemes the lists give them.

### 5.1 Flags and registers

The program tests these flags (section 2.2). `N` and `A` commands set them, and `F` and `D` commands clear them ([input.md](input.md) §1.4):

| Flag | Set by | At first | When set |
|---|---|---|---|
| 0 | `ESC[1N` | off | Spell mode: every word is spelled (section 5.4) |
| 1 | `ESC[2N` | off | Punctuation is read out between pauses: "comma", "dash", "open paren", "period"; `&` is "ampersand", a decimal point "period", and `$` "dollar" before the amount |
| 2 | `ESC[3N` | off | Numbers, times and money are read digit by digit |
| 3 | `ESC[4N` | off | Each space is read out: "space" and a pause |
| 4 | `ESC[5N` | off | A hyphen before a digit is "minus" |
| 5 | `ESC[6N` | off | Numbers of three and four digits are read whole: `999` is "nine hundred ninety-nine" |
| 6 | `ESC[7N` | on | Words of capitals are spelled |
| 7 | `ESC[8N` | on | Abbreviations are expanded |
| 12 | `ESC[13N` | on | Spelling says "cap" before each capital |
| 17 | `ESC[2A` | off | A state's two-letter abbreviation is the state's name, and numbers are read digit by digit |
| 18 | `ESC[3A` | off | Directions are expanded: `NW` is "northwest" |
| 19 | `ESC[4A` | off | Numbers are ordinals: `5` is "fifth", `$5` "fifth dollar" |

Register 6 holds the kind of the last thing the main loop read: 0 a control node, 1 phonetic input, 2 a word, 3 a space, 4 a number, 5 a sentence's end, 6 punctuation of list 9, 7 a symbol of list 6. Register 7 is set to the same kinds but for control nodes and spaces, and nothing reads it. Register 3 is 1 for a word after a `~` and 2 after a `` ` `` (section 5.7); register 9 is 1 after a number or a number word, which makes money's "dollars" "dollar" (section 5.8). The others hold the counts of the routine that runs: a word's letters, its lower-case letters and whether it has a vowel, a number's digits and its groups, and the like.

### 5.2 The main loop (place 0)

The program sets registers 6 and 9 to 0 and register 7 to 5, then loops at place 0x0D, on the node at first:
- a control node: its command runs (action 18), and it is passed over;
- in phonetic input, when the setting isn't 0: phonetic input (section 5.10), whatever the node;
- a letter or `'`: a word (section 5.3);
- a space: it is deleted; with flag 3, "space" and a pause go in instead;
- a digit: a number (section 5.5);
- `.`, `?` or `!` (list 15): a sentence's end (section 5.9);
- `` "$(),-:;[]{}`~ `` (list 9): punctuation (section 5.7);
- `#%&*+<=>@/\^_|` (list 6): its name; `&` is "and", or "ampersand" with flag 1. `5 + 3 = 8` is "five plus three equals eight";
- any other node is deleted.

Each routine yields after what it made (section 2.1), so that the lexical stage gets each word as soon as it is made. With the phrase setting 0 (`ESC[0P`), a pause `,` goes in before the yield, unless the node at first is `.`, `?` or `!`: every word is a phrase of its own.

### 5.3 Words (place 0x1FD)

A word is the run of letters and apostrophes at first, up to 30 of them. One that starts with `'` loses it, and one that ends with `'` too: `'tis` is `TIS`, `students'` `STUDENTS`. Then, in order:
1. With flag 0, the word is spelled (section 5.4).
2. With flags 7 and 17, a word of two letters that is a state's abbreviation in either case (list 3) is the state's name: `TX` and `tx` are "Texas". A `.` after it stays, and ends the sentence.
3. With flag 6, a word with no lower-case letter is spelled letter by letter (section 5.4): `FBI` is `F`, `B` and "I", and `US` `U` and `S`. So is a word whose only lower-case letter is the `s` of a final `'s`: `NASA's` is `N`, `AY`, `S` and `AY@S`. A word of one letter isn't, nor one of the acronyms read as words (list 21, in capitals: `NASA`, `UNIX`, `SCSI` and others), nor a direction with flag 18. `FBIs` stays a word.
4. With flag 7, an abbreviation is expanded (section 5.6).
5. A word of two letters or more without a vowel (`aeiouy`) is spelled: `Hmm` is `H`, `M`, `M`; `TVs` is `T`, `V`, `S`.
6. A word of one letter: `a` or `A` is the article, `A`, when a space or a control node follows it and the last thing read was a space, a control node or punctuation, or nothing; otherwise it is the letter, `AY`. So `a b` has the article, and `a.`, `a,`, `(a)` and `5a` have the letter. Another letter is a word of that letter. A `.` right after a word of one letter is deleted, as an initial's: `J. Smith` is `J`, `SMITH`, and `J.R.R.` `J`, `R`, `R`. Two spaces after it keep it, and it ends the sentence: `Plan B.  Then`. With flag 1 it is read out.
7. Otherwise it is a word of letters: `&`, then `~` or `` ` `` after one of those (section 5.7), and the word's letters in capitals, `'` as `@` (section 2.3). `don't` is `DON@T`.

### 5.4 Spelling (places 0x865 and 0x8BA)

A word spelled letter by letter is a word of letters for each letter: `A` is `AY`, so that the lexical stage says the letter's name, and `I` the phonemes of "I". An apostrophe is "apostrophe", but before a final `s` it stays with the letter before it, as in `AY@S`; after an `I` it doesn't. An emphasized word (`~FBI`) gets a space phoneme after each letter.

In spell mode (flag 0), every word is spelled that way, `a` and `i` as `A` and `I` are, with "cap" before each capital while flag 12 is on, and a pause after the word unless `.`, `?` or `!` follows it. `Hi 42.` is "cap", `H`, "I", "forty-two": numbers are still numbers.

### 5.5 Numbers (places 0x3BD and 0x5AE)

A number is a run of digits, with commas and decimal points:
- With flag 2 or 17, it is read digit by digit (place 0x8FD), with a pause at each comma: `1,234` is "one", a pause, "two three four".
- With commas between groups of three digits, it is read whole, with "thousand", "million", "billion" and "trillion": `1,234,567` is "one million", a pause, "two hundred thirty-four thousand five hundred sixty-seven". Groups of other lengths, or more than five of them, are read one by one as numbers without commas, with pauses: `12,34` is "twelve", a pause, "thirty-four".
- Without commas, by its count of digits (place 0x957). Each leading `0` is "zero": `007` is "zero zero seven". One digit is its name, and two are a teen or a ten and a digit (lists 1, 13 and 14). Three and four are read in pairs, as years are: `1997` is "nineteen ninety-seven", `999` "nine ninety-nine", `101` "one oh one", `2005` "twenty oh five". `1900` is "nineteen hundred", `100` "one hundred" and `2000` "two thousand". With flag 5 or 19, in money, and before an ordinal's ending, three and four digits are read whole: `1997` is then "one thousand nine hundred ninety-seven". Five digits or more are read digit by digit.
- A decimal point and digits after the number, or alone, are "point", or "period" with flag 1, and the digits one by one: `3.14` is "three point one four", `.5` "point five" and `10.25.3` "ten point two five point three".
- An ending `st`, `nd`, `rd` or `th` (list 7) makes it an ordinal (list 8, and the tens and teens with "th"): `21st` is "twenty-first", `101st` "one hundred first". An `s` or `'s` makes it a plural: `1990s` is "nineteen nineties". With flag 19 every number is an ordinal; with flag 0 or 2 an ending is read as a word of its own.

A hyphen between numbers is a pause (section 5.7), so `555-1234` is "five fifty-five", a pause, "twelve thirty-four".

### 5.6 Abbreviations (place 0x91)

The routine takes words of up to five letters, matched in lower case:
- With flag 18, the directions of list 4 (`ne`, `nw`, `ene` and the others), and `n`, `s`, `e` and `w` (list 5) before a `.`: `NW` is "northwest", `N.` "north".
- The abbreviations of list 2, with a `.` or without: units (`lbs`, `km`, `mhz`), addresses (`ave`, `blvd`, `rd`), months (`feb`, `apr`), `etc`, `ie`, `eg`, `jr`, `inc` and others, 101 in all: `5 lbs.` is "five pounds".
- The titles of list 16: `mr`, `mrs`, `ms`, `capt`, `gen`, `prof`, `vs` and others. `Mrs. Smith` is "missus", `SMITH`.
- `fig`, `in`, `gal`, `co` and `prop` (list 17), before a `.` and when no capital follows: `5 in.` is "five inches", but `in. Then` stays `IN`. `Mar`, `Wed`, `Sat` and `Sun`, written so (list 19), are expanded before any `.`, and in lower case (list 20) never: `Wed.` is "Wednesday", `wed.` stays `WED`.
- `dr`, `st` and `ft` (list 10), with a `.` or without: before a capital they are "doctor", "saint" and "fort", and otherwise "drive", "street" and "feet" (list 11). `Dr. Smith` is "doctor", `SMITH`, and `Elm St. now` `ELM`, "street", `NOW`.
- `i.e.` and `e.g.` (list 12): "that is" and "for example", each with a pause.

The `.` after an expanded abbreviation is deleted, or read out with flag 1. After those of lists 2 and 4, and `Mar.`, `Wed.`, `Sat.` and `Sun.`, it stays when a capital follows, and ends the sentence: `etc. Then` keeps it.

### 5.7 Punctuation (place 0xA90)

- `-` with a digit after it, past spaces, is a pause, or "minus" with flag 4. Any other `-` is deleted: `a-b` is `AY`, `B`.
- `:` is a time, and `$` money (section 5.8).
- `~` before a word emphasizes it, and `` ` `` de-emphasizes it ([lexical.md](lexical.md) §3.3): the word's letters start with `~` or `` ` ``. Before anything else, or with flag 0 or 1, they are read out: "tilde" and "grave", between pauses.
- A closing `"`, `)`, `]` or `}` is a pause when a letter or a digit follows it, past spaces, and is deleted otherwise. Every other mark is a pause. With flag 1, each is read out between pauses: "comma", "open paren", "quote".

### 5.8 Money and times (places 0x3D8 and 0x30C)

`$` and a number are the number, read whole, and "dollars", and then the cents after a point, "and" and "cents". `$5.25` is "five dollars and twenty-five cents", `$1.01` "one dollar and one cent" and `$1.00` "one dollar". "Dollars" is "dollar" for `1`, or when the word before it was a number or a number word (list 18: `a`, `an`, `one` to `nineteen`, the tens, `hundred`, `thousand`, `million`, `billion`): `a $5 bill` is "a five dollar bill". A point followed by one digit, or by three or more, is a decimal point: `$1.5` is "one point five dollars". `thousand`, `million`, `billion` and `trillion` after the number come before "dollars": `$5 million` is "five million dollars". `$` alone, or with flag 1 or 2, is "dollar", and a number after it is read as numbers are.

`:` between numbers is a time: `10:30` is "ten thirty", `10:05` "ten oh five" and `10:00` "ten o'clock". A `:` with no two digits after it is a pause: `1:5` is "one", a pause, "five". With flag 1 it is "colon" between pauses; with flag 2 the digits are read one by one.

### 5.9 Sentences' ends (place 0xAD4)

A `.` becomes the phoneme `.`, a `?` the phonemes `?` and `.`, and a `!` the phoneme `!`: they end the sentence for prosody. With flag 1, the mark's name follows, "period", "question" or "exclamation", and another `.`. A `.` before a digit is a decimal point, and one right before a letter is deleted: `a.b` is `AY`, `B`.

### 5.10 Phonetic input (place 0x7AD)

With the phonetic input setting 1 (`ESC[1I`) or 2 (between `[` and `]`, section 4), every node but control nodes goes to this routine. A symbol that isn't a phonetic symbol (condition 3) is deleted. A word of phonetic symbols becomes phonemes, with `&` before it unless it starts with the word mark `&` or `%`. The word ends at a control node, at a `&` or `%`, which starts the next, or at one of `.?,]) \`, which becomes a phoneme too, and a word of more than 31 symbols is split. The marks `!` and `/` after a phoneme give it values (section 2.5). With `ESC[1I`, `hElO1 wP1Rld.` is the phonemes `&hElO1 &wP1Rld.`

## 6. What the recorded runs don't reach

- The program's four blocks at places 0x2F3 to 0x30B, which nothing goes to: the block before them ends in a return. The original's program also ends with four zero bytes, which the data pack leaves out.
- 36 conditions passed and never failed, or failed and never passed. Most test for a control node right after a character of a word, where the runs' commands, put between each two characters of their words, didn't reach; others test for a symbol that the lists, the TextIn tokenizer ([input.md](input.md) §4) or the conditions before them always give or always rule out. Condition 8 with class 14 never passes: no node the normalizer reads has a symbol below 32 or from 128.
- Inserting with no scan node (section 2.4): the program inserts only while it reads a word.
- A hyphen in a word being spelled (section 2.3, step 3): the program spells words of letters and apostrophes.
- Action 5 before any match: the original would read its replacement from address 0. The port inserts nothing. The program matches before each replacement it inserts.
- The replacements of lists 18 to 21, whose words the program only tests for: they are never inserted. In the data pack each is the string that follows its word in the original, which is what it would insert.
- More than 20 open calls: the original would keep the 21st call over the count of calls. The steps nest at most 7 deep.
- A reset during a text ([pipeline.md](pipeline.md) §7): the original drops the rest of the text, so the recorded runs check the reset only at a thread's start, where the values it gives are a new normalizer's anyway.
- A control node `F` or `x` where the program looks for a command it doesn't pass over (conditions 8 and 16): the escape parser writes `F` as `N` ([input.md](input.md) §1.4), and the original didn't finish a text with `x`, which resets the stages, when it was recorded.
- The original's machine has an action 15, which sets first's symbol to its operand. The program has none, and the port doesn't have it: the data pack can't hold it.
