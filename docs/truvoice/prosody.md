# TruVoice: prosody

How L&H TruVoice (`tv_enua.dll`) gives each phoneme its duration and its pitch. The prosody stage works on the lexical stage's phonemes, the word marks before the words and the punctuation ([pipeline.md](pipeline.md) §3). It finds where its phrases end, puts in the pauses between them, and turns each phoneme into a segment for the parameter generator ([parameters.md](parameters.md)): a node of type 4 whose value is its duration in frames and whose second value is its pitch, in units of 2 Hz.

This spec covers the arithmetic (section 1), the phonemes the stage sees and what it notes about them (section 2), how it finds a phrase, breaks it and chooses its contour (section 3), the pitch (section 4), how it walks the phrase's nodes, with their presets and pauses (section 5), the durations (section 6) and its reset (section 7). The durations of the vowels, fricatives, stops and sonorants will follow.

**In short.**
- The stage scans ahead for a sentence's end, a `C` command, or a limit: 180 phonemes (70 when text comes into an empty pipeline, 15 with `\Pro=0\`) or 240 nodes. Each comma on the way ends a phrase of its own, and the stage hands on the nodes before it as soon as it has timed them.
- Within a phrase it puts in breaks between words where the words' dictionary classes say, and a `P` command at each phrase's start that tells the pitch the phrase's kind: falling, rising, a comma's, or without an end of its own.
- At each break comes a pause: 39 frames at a sentence's end, 10 at a comma; at a rate of 25 or more, or with `\Pro=0\`, a short one. In slow speech, some function words after a content word get a pause before them, by a 16-step pattern whose position the whole process shares.
- Phonetic input can preset a phoneme's duration and pitch; the stage puts them back after it times the phoneme.
- A consonant's duration is its shortest plus a share of the rest, by where it is in its phrase and word and the sounds around it. At rate 7 and up a pair of the same consonant, or of two close ones, becomes one, longer. Stress 3 and emphasis lengthen a word, by 25 to 100 % and 50 % more for a function word, and the rate scales every duration. A duration is 2 to 55 frames.
- A phrase's pitch starts a third of the voice's pitch above it, and comes down over the phrase's phonemes. Phonetic input can place word marks that make it come down to a mark and rise from it instead: the phrase's **contour**.
- The phrase's **nucleus** is its last stressed vowel before a break. The syllables before it and the stressed ones go up; the nucleus carries the phrase's accent.
- In the phrase's last syllable the pitch moves as the phrase's kind says: down at a sentence's end, up a little at a comma, up at a question. From there every phoneme is 8 Hz lower than the one before, down to 46 Hz.
- Small changes for the sounds: a voiceless stop starting a word before a vowel is 20 Hz higher, an unstressed voiced sound before a consonant 3 Hz lower (8 if it isn't a sonorant), a high vowel or glide 3 Hz higher, and an `N` before a `T` or a glottal stop at a word's end 55 Hz lower.
- A whispered phoneme gets pitch 0, a monotone one the voice's pitch. Otherwise the pitch stays between 50 and 500 Hz.

**Exactness.** Ported from the original's code, and checked against it by replaying every call of its functions recorded in 346 runs: the parameter generator's and the pipeline's runs ([parameters.md](parameters.md), [pipeline.md](pipeline.md)), and 118 more for the phrases and the pitch: questions, exclamations, lists, long sentences without punctuation, emphasis, `\Pro=0\`, pitch, speed, character and voice changes within a text, slow and fast speech, 30 with the word marks phonetic input places (section 3.5), 4 with long runs of consonants in phonetic input, and 47 for the phrase's breaks and pauses (speed, `v`, `r` and `P` commands within a sentence, speed at its end, `\Pro=0\` within a phrase, exclamations without a stressed vowel, the word classes that break a phrase, question words, the Address context, phrases of more nodes and word marks than the scan looks at, a held phrase broken off, word marks of values 6, 7 and 12 at more places, pauses as long as exactly 1 and 50 phonemes, and a pause before any text), and 3 with an engine reset during a text, one while a phrase is held. Each call left the speaking thread's state and the stage's shared state (section 3.1) byte-identical to the original's: the 40,436 runs of the stage (section 5.1) with their 40,436 scans (section 3.2), 2,290 placings of the breaks (section 3.3) with their 186 breaks, 6 of them after a stop, and 81 chances of a slow pause (section 3.4), 51 of which put one in, 64,530 walk steps, 34,349 nodes processed (section 5.2), 58,966 notings and restorings of presets (section 5.3), 2,444 pauses (section 5.4) and 349 resets (section 7); each of the 37,384 contexts (section 2.3), the 34,349 pitch contexts (section 4.2), the 34,349 pitches (section 4.3) and the 1,239 resets of the pitch (section 4.4). In these replays each duration's call started from the state the Go stage had reached, checked against the original's, and its result was the original's; the durations are checked on their own (below).

The durations were replayed the same way in 386 runs: those 346 and 40 more for the durations (pairs of the same consonant across words, after a function word, after a word of one consonant, within a word and at rates 6 and 7, words of stress 3, emphasized words of every kind, phonetic input's pairs and pauses, among them a pause of 100 frames, and phonemes timed by their shortest durations alone). Each call left the speaking thread's state byte-identical to the original's: the 40,089 durations (section 6.1), with 11,429 nodes that aren't phonemes, 195 emphases and 82 phonemes timed by their shortest durations; the 27,840 base durations (section 6.2) and phonemes' durations (section 6.3), with as many tests for a pair, 448 of which merged one, 396 of them geminates (section 6.4), and as many emphasis steps (section 6.5); 16,286 scalings by the rate, and 750 pauses (section 6.6), 64 of them over 50 frames. The durations of the vowels, fricatives, stops and sonorants aren't ported yet: in these replays each of their calls started from the state the Go stage had reached, checked against the original's, and its result was the original's.

## 1. Arithmetic

All arithmetic is in 32-bit signed integers. Divisions truncate toward zero; shifts of negative numbers are arithmetic. The **Q15 product** of `a` and `b` is `a · b / 32767`, as in [parameters.md](parameters.md) §1. In a quotient such as `E · (p − m) / (N − m)` the product comes first.

## 2. Phonemes

### 2.1 Nodes and classes

The stage's window ([pipeline.md](pipeline.md) §3) holds control commands (type 0), the lexical stage's nodes (type 3) and the segments the stage has made (type 4). The nodes of type 3 are the phonemes, a word mark before each word, and the punctuation. A phoneme node's flag bits 3 and 4 are its **stress**, 0 to 3; 3 is an emphasized word's. Flag bit 0x20 the lexical stage sets on a stressed syllable's consonants and vowel: such a node is **stressed**, as in [parameters.md](parameters.md) §8. Flag bit 0x80 marks a phrase's nucleus (section 4.3). Byte 0x18 of a flap `t` is the phoneme it stands for, `T` or `D`.

A word mark is `&` before a content word and `%` before a function word. Its second value is the word's **class** in the dictionary, and the lexical stage's rules set flag bit 0x80 on some marks. Classes seen in the recorded runs are, for instance: 0 most content words, 6 verbs, 12 `is`, `was`, `would`, 13 question words (`what`, `who`, `when`), 14 `and`, `or`, `but`.

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
| (3, 0x02) | laryngeal | the pause, `H Q d` |
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

### 3.1 The state

The stage's cursor ([pipeline.md](pipeline.md) §3) gives it three places in its window: its done node is the phrase's **first** node, its scan node the **current** node, and its first node where the scan has got to, the **scan's** node, which is the phrase's **last** once the scan has found its end.

For the phrase the stage keeps:
- the scan's **result** (section 3.2);
- the phrase's **word marks**, up to 50, and the sentence's word marks before the phrase, a count;
- `N`, the phonemes of the sentence counted so far; the **sentence's phonemes** at its end; the **padding** a phrase without an end of its own adds to `N` (section 5.1);
- the **phrase limit**, in phonemes;
- the flags **slow speech**, **no phrasing** and **prosody off**, which the scan sets from the commands, and **first phrase**, set until the sentence's first `P` command is put in;
- the contour and the marks `m6`, `m7`, `m8` and `m17` (section 3.5);
- for the walk (section 5): the **countdown** of nodes before the stage lets the synthesizer catch up, the **phonemes since the last mark** of value 12, and the presets (section 5.3).

The original keeps the rest in global variables, which every speaking thread of the process shares:
- the scan's counts: the nodes **scanned**, the phonemes **pending** since the last node that isn't one, and the phonemes **counted** toward the limit;
- whether a phoneme has come since the last break, the **last node** scanned that isn't a phoneme, and **held**, set by a `P` command that comes before any phoneme;
- the walk's **state** and the scan's result it puts back at the phrase's end (section 5.1);
- the current node's preset pitch, its symbol and the node (section 5.3);
- the **slow-pause counter** (section 3.4), which nothing resets.

The port keeps these in one state for the whole process too, and a stage holds it while it runs (section 5.1). Two engines speaking at once in the original could each change it anywhere in their runs; the port lets them take turns by runs. An engine reset gives the speaking thread a new state ([pipeline.md](pipeline.md) §7) and leaves the shared state as it is: its last node and its noted node then point into the state the reset replaced, and the stage writes both before it reads them.

### 3.2 Scanning for a phrase

The scan's result is 0 while the window runs out before the phrase's end. Otherwise it says what ended the phrase: 1 a `C` command, 3 a sentence's end, 4 a limit (the phrase is **broken off**), 5 a comma while no phrasing is set. It is 2 while the stage walks the phrase (section 5.1).

1. While the stage walks a phrase, the scan does nothing. Result 2.
2. If the last scan found a phrase's end, a new phrase starts: `N` loses the padding, which becomes 0; the scanned, pending and counted counts, the phrase's word marks and the marks `m6`, `m7`, `m8` and `m17` become 0, 0, 0, none, −1, −1, −1 and 0; no phoneme has come. After a sentence's end or a comma while no phrasing is set, a sentence starts too: the pitch's state is reset (section 4.4), `N` and the sentence's word marks become 0, held is cleared and first phrase set.
3. While the phrase's first node isn't of type 3 and the scan's node is still the first node: the node is run as a control command at the stage ([pipeline.md](pipeline.md) §4), prosody off is cleared, the phrase limit becomes 180, and the next node becomes the first, the current and the scan's node.
4. The last node becomes the scan's node. Then for each node from there, while the result is 0 and there is a node:
   1. The node counts as scanned.
   2. A node of type 3 in class phoneme: a phoneme has come, and one more is pending.
   3. Any other node: the pending phonemes are added to the counted ones and to `N`, and become 0. The node becomes the last node. Then:
      - **A word mark** (type 3, `%` or `&`), while the phrase has fewer than 50: by its value, 6, 7, 8 or 17, `m6`, `m7`, `m8` or `m17` becomes `N`. It becomes the phrase's next word mark.
      - **A break** (type 3, class break). At `.`, `?` or `!` the result is 3: the sentence's phonemes become `N`, and `N` loses one for each node of type 3 in class phoneme among the nodes before the break that aren't in class syllabic, going back to the first one that is. At `,` while no phrasing is set the result is 5. Unless held or no phrasing is set, the rate is 25 or more, or the break is `!`, the breaks are placed with the break's symbol (section 3.3). No phoneme has come, held is cleared, and the sentence's word marks gain the phrase's, which become none.
      - **A control command** (type 0): at `C`, unless held or no phrasing is set, the breaks are placed with the symbol 0; the result is 1. At `N`, prosody off is bit 0x20 of the second value, flag 0x2000 ([pipeline.md](pipeline.md) §4), and the phrase limit becomes 15 with it, 180 without. At `P`, no phrasing is the value being 0; if no phoneme has come, held is set, and otherwise, with more than 3 nodes free, a node `\` of type 3 is put before the command, counts as scanned, and becomes the next node scanned. At `r` and `v`, slow speech is the value being below 9.
   4. While the result is 0: with fewer than 240 nodes scanned and fewer phonemes counted than the phrase limit, the scan goes on to the next node. Otherwise the phrase is broken off: the scan's node becomes the node before the last node, which leaves the phrase if it is a word mark; unless held or no phrasing is set, the breaks are placed with the symbol 0; the result is 4.
5. The contour is chosen (section 3.5).

The rate is the prosody cursor's ([pipeline.md](pipeline.md) §3.1). The phrase limit becomes 70 at the start of a stage's run while the **short phrase** flag is set ([pipeline.md](pipeline.md) §1): the speaking thread sets it when text comes in while more than 608 nodes are free, unless prosody off is set; prosody clears it as it walks a phrase (section 5.1), and the parameter generator once it is more than 64 frames ahead of the synthesizer ([parameters.md](parameters.md) §17.2).

### 3.3 Breaks

Placing the breaks, with an end symbol:

1. A phrase without word marks gets nothing.
2. The rules look at the phrase's word marks `w0, w1, …`, four at a time: `wk−2`, `wk−1`, `wk` and `wk+1` for `k` from 2 while `k` is below the count of word marks less 2. `c`, `p`, `b` and `n` are the classes of `wk`, `wk−1`, `wk−2` and `wk+1`; `wk` is **flagged** if it has flag bit 0x80; `s` is the symbol of the node after `wk`, the word's first. A **function word after a content word** is a `wk` of `%`, not flagged, after a `wk−1` of `&`. The first rule that applies:
   - **Before `wk`**, if `p` is 0 and either `c` is 8, 11 or 12, or `wk` isn't flagged and `c` is 9, or is 6 with a node other than `~` before `wk+1`.
   - **Before `wk`**, for a function word after a content word with `p` other than 6, and `c` 5, or `c` 14 with `n` 5.
   - For a function word after a content word, with `c` 13, or with `b` other than 10, `p` other than 6 and 15, and: `c` 10, byte 0x18 of `wk` other than `-` and `s` other than `T`, `t` and `a`; or `c` 2 and `s` other than `x` and `B`; or `c` 3 and `s` `F` or `s`. **Before `wk+1`** if `c` is 10 or 3 and `n` is 14, or `c` and `n` are both 10; otherwise **before `wk`**.
   - **Before `wk`**, if `p` is 15, `wk` isn't flagged and `c` is 10, 5 or 14.

   A break before `wk` puts one in (below) and `k` goes up by 3; before `wk+1`, by 4; without a break, by 1.
3. With 3 or more nodes free, a `P` command is put before the word mark the last break went before, or `w0`; if the node after it is the phrase's first node, the command becomes the first node. Its value is the phrase's kind by the end symbol: 1 at `.` and `!`; 3 at `,`; at `?`, 1 if `w0`'s class is 13, a question word, or else the class of the word mark the last break went before is, and 4 otherwise; 2 at any other symbol. With the 1 of a question word, the node after the word's first node, if it is of type 3 and in class phoneme, becomes stressed with stress 1. The command's second value is 0 while first phrase is set, which it then clears, and 1 after.
4. In slow speech, the slow pauses come (section 3.4).

**Putting in a break** before a word mark `w`, with more than 2 nodes free:
1. A node `,` of type 3 is put before `w`. A flap `t` on either side of it becomes the phoneme its byte 0x18 keeps.
2. The node before the break: a stop `P`, `T`, `K`, `B`, `D` or `G` (not in class affricate, which none of them is) at a rate below 25 gets a node `p` of type 3 after it, its release, and the sentence's phonemes gain 1.
3. The node after `w`: a vowel (class vowel) other than `U`, while the cursor's pitch is 0, gets flag bit 0x40; otherwise a `d` becomes `H`.
4. A `P` command of value 3, a comma's phrase, is put before the word mark where the phrase the break ends started (`w0`, or the one the break before went before), with its second value as in step 3 above; if the node after it is the phrase's first node, the command becomes the first node.

The node before a word mark is the word's command (`z`, [pipeline.md](pipeline.md) §4.1), except inside a written word the stages before split into several, as `bad-man` ([parameters.md](parameters.md) §10), and in the Address context (`\Ctx="Address"\`), which has no word commands. Elsewhere step 2 changes nothing.

### 3.4 Slow speech's pauses

In slow speech, after a phrase's breaks are placed, the stage goes over the phrase's nodes from its first word mark up to its last one, which it doesn't look at, stopping when fewer than 4 nodes are free. The **rate** starts as the cursor's; an `r` command makes it `(v − 46) >> 3`, with `v` the low byte of its value, and a `v` command its value's low byte. At each `%` after a word mark `&` (the previous word mark seen), while the rate is below 9 and the node before isn't `,`, the stage takes the next **slot**: the slow-pause counter's low 4 bits, and the counter goes up by 1. If bit `slot` of the rate's **pattern** is set, a node `)` of type 3 is put before the `%`: a pause (section 5.4).

The patterns are the data pack's slow-pause patterns, by rate ([datapack.md](datapack.md) §7). An `r` command's value is a rate (0 to 25), which the formula takes for words a minute, so after one the rate is −6 to −3; the original then reads the table before the patterns, the pause percents by rate, and the port does too: the pattern of rate `−k` is the pause percent of rate `26 − k`, 30, slots 1 to 4.

The counter is the process's, and nothing resets it: which words get a pause depends on what the process has spoken before.

### 3.5 What the pitch takes from the phrase

The pitch takes from the scan:

- the phrase's **phonemes** `N` (section 3.2): the sentence's nodes in class phoneme up to the phrase's end, without the consonants after a sentence's last syllabic phoneme, and with the padding of a phrase without an end of its own;
- the **marks** `m6`, `m7`, `m8` and `m17`: for the last word marks of the phrase whose value is 6, 7, 8 and 17, `N` when the scan came to them; −1 without one, but 0 for `m17`;
- the phrase's **contour**: 0, 6, 7, 8, 15, 16 or 19;
- the phrase's **kind**, the prosody cursor's phrase setting ([pipeline.md](pipeline.md) §3.1), which the `P` commands the breaks put in set (section 3.3). Its low byte is 1 for a falling phrase (at `.`, or a question that starts with a question word), 2 for a phrase without an end of its own, 3 for a comma's or one a break ends, 4 for any other question; a phrase that ends at `!` gets no `P` command and keeps the kind of the one before; the text's own `P` commands can set other values, and the pitch sets 3 or 6 (section 4.3). Its high byte is 1 for a phrase that goes on from the one before in the same sentence.

A word mark's value is 0 but in phonetic input. When input flag 0x20 is set (`ESC[6A`), `[` starts phonetic input, in its second mode while input flag 0x40 is set, as it is by default, and `]` ends it. In that mode a symbol right before the `]` gives the next word mark its value: `\` 6, `/` 7, `/\` 8, `{` 12, `_` 17.

**The contour**, after the scan:
- with neither `m6` nor `m7`: 8 with `m8`, 0 without;
- with `m7` only: 0 if `m7` is `N`, 7 otherwise;
- with both and `m7` at most `m6`: 15 if `m7` isn't 0, otherwise 6 if `m6` isn't 0, otherwise 0;
- with both and `m7` above `m6`: 16 if `m6` isn't 0, otherwise 0 if `m7` is `N`, otherwise 7;
- with `m6` only: 6 if `m6` isn't 0, 0 otherwise.

A mark is there when it isn't −1. If the scan's **last symbol** is `!`, the contour is 19 instead: `m6` becomes the sentence's phonemes, and loses 1 for each node in class phoneme from the scan's node going back toward the phrase's first node, which it doesn't look at, until a stressed node in class vowel, until `m6` is 0, or until the window's start. Then `m6` loses 1 more, and `m7` becomes 0 if `m6` is then 1, and otherwise `(m6 + 1) / 2`. The last symbol is that of the last node scanned, or of the last node when the phrase is broken off; 0 when the scan looked at no node.

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
4. **The nucleus.** Without one, the stage looks for it, from the current node to the phrase's last node (section 3.1), or the window's end. Each phoneme of stress 2 or 3 (a node of type 3 in class phoneme) becomes the nucleus. At the first node of type 3 in class break the search ends. If it reaches the phrase's end or the window's end before a break, there is no nucleus, whatever it found.
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

When a sentence starts (section 3.2), the pitch's state is reset: the following consonant becomes 4, `p` and `F` 0, there is no nucleus, and rise, word end, phrase end, at nucleus, after stress and raised are cleared. Lowered stays.

## 5. The walk

### 5.1 The stage's run

The scheduler runs the stage ([pipeline.md](pipeline.md) §6). A run, holding the shared state (section 3.1):
1. Begins on the prosody cursor ([pipeline.md](pipeline.md) §3.2). The countdown becomes 2. Unless prosody off is set, while the short phrase flag is set and the rate is below 25, the phrase limit becomes 70.
2. Scans (section 3.2). By the result: at 1 or 4, a phrase without an end of its own, the padding becomes `(N >> 2) + 4` and `N` gains it; at 1, 3, 4 or 5, the current node becomes the phrase's first node; at 1 to 5, unless prosody off is set, the short phrase flag is cleared.
3. Unless the result is 0, walks: it takes the next node (below) and runs it as a control command at the stage ([pipeline.md](pipeline.md) §4), or processes it (section 5.2), until the next node says the phrase is done or the stage stops.
4. Finishes ([pipeline.md](pipeline.md) §3.3), and reports whether it handed nodes on or stopped.

**The next node.** The walk's state is 1 at a phrase's start, 2 on a node, and 3 on the phrase's last node.
1. If the scan's result isn't 2, the walk starts: its state becomes 1, and the result is put aside. The result becomes 2.
2. In state 3 the phrase is **done**: the pauses (segments `' '`) from the phrase's first node up to its last lose their stress bits; the node after the last becomes the first, the current and the scan's node; the result put aside comes back; state 1.
3. At a `,` of type 3 in state 2, unless prosody off or no phrasing is set, the stage **stops** at the comma: the pauses from the phrase's first node up to the comma lose their stress bits; the comma becomes the phrase's first node, which the run's finish doesn't hand on yet, and the node after it the current node; state 1.
4. The countdown loses 1. Once it is below 0, the stage **stops** while the synthesizer is fewer than 33 frames ahead (its `written − lookahead − position`, [synthesizer.md](synthesizer.md) §5.1).
5. In state 2 the next node becomes the current node; otherwise the state becomes 2, on the current node.
6. On the phrase's last node the state becomes 3. Past the window's end the phrase is **done**: the first, the current and the scan's node become none, the result put aside comes back, and the state is 1.
7. A current node of type 3 is processed; any other is run as a control command.

### 5.2 A node

The stage processes a node of type 3:
1. It notes the node's context (section 2.3) and its presets (section 5.3), gives it its pitch (sections 4.2 and 4.3) and its duration (section 6).
2. At a word mark (class word mark) of value 12, it puts in a pause as long as the phonemes since the last mark (section 5.4), and they become 0.
3. At a break (class break), it puts in a pause of the break (section 5.4), unless the break is a `]` whose nodes before are, back to the list's start, a bookmark (`i`) and a generated pause (`s`, type 5).
4. A node in class phoneme becomes a segment (type 4), counts as a phoneme since the last mark, and gets its presets back (section 5.3).

### 5.3 Presets

Phonetic input can give a phoneme a duration, its value in frames, and a pitch, its second value in units of 2 Hz. With flag bit 0x80 they are relative to the stage's own. Before the stage times the node:
1. The preset pitch is the node's second value and the preset duration its value. If bit 7 of either is set, so are bits 8 to 15. The node and its symbol are noted.
2. The presets' bits become 4 with flag bit 0x80, 0 without; they gain 1 with a preset duration other than 0, 2 with a preset pitch other than 0. The previous bits are kept.
3. A pause's (symbol space) preset duration is held at 50 at most.

After, if the current node is still the noted node with the noted symbol:
1. With a duration: when relative, the preset becomes itself plus the node's duration, held between 2 and 60. The node gets the preset.
2. With a pitch: when relative, the preset gains the node's pitch and is held between 25 and 100; otherwise a preset of 1 becomes 0. The node gets the preset.

A negative relative preset written as a byte thus becomes a large one, and holds at 60 or 100.

### 5.4 Pauses

The stage puts in pauses before the current node, for a break or as long as `L` phonemes when `L` is above 0:
1. At a `)` with a rate of 6 or more, none.
2. At a rate of 25 or more, or with prosody off set: at a `]` with `L` 0, one pause of 12 frames; otherwise two of 3 frames, or of 2 with prosody off.
3. Otherwise, the length in frames: `L` held between 30 and 80 when `L` is above 0, and **split**; or by the symbol: 39 at `.`, `?` and `!`, split; 10 at `,`, 4 at `\`, 12 at `]`, 8 at any other.
4. While the length is above 3: a split length below 50 makes two pauses, a third of it and two thirds, and is no longer split; any other length makes one pause, of 50 frames at most. The length loses 50, and becomes 4 if that leaves it between 1 and 3. With fewer than 3 nodes free the pauses stop.
5. The context is noted again (section 2.3).

A pause is a segment `' '` (type 4) whose value is its frames and whose second value is 50, a pitch of 100 Hz.

## 6. Durations

A phoneme's duration is in frames. The data pack gives each symbol an **inherent** and a **shortest** duration (`durations`), and each rate a **duration percent** (`durationPercents`, 344 at rate 0 to 65 at rate 25, 117 at rate 13). The stage keeps for the current node its inherent and shortest durations times 10, four **percents**, the **emphasis** of its word, and whether it is a **geminate** (section 6.4).

The durations of the vowels, fricatives, stops and sonorants (section 6.3, step 2) will follow.

### 6.1 A node's duration

The stage times the current node:
1. Its inherent and shortest durations become the symbol's, times 10, and the first percent 100.
2. A node not in class phoneme: a word mark (class word mark) sets the emphasis. A mark of value 10 (phonetic input's `[:]`, section 3.5) gives a **content word's** emphasis when its word's class (its second value) is 0 or 6, and a **function word's** otherwise; any other value gives none. Any other node's value becomes 0.
3. A phoneme with a preset duration (section 5.3) gets the preset.
4. With input flag 0x10 (`ESC[5A`) a phoneme is timed by its shortest duration `S` alone: the pause gets `S / 4`, at least 4 frames; another phoneme `S · 17 / 20`, halved unless it is stressed, at least 2. With the data pack's durations the pause always gets 4 frames and an unstressed phoneme 2.
5. Otherwise its value becomes its base duration (section 6.2), and then its duration (section 6.3).

### 6.2 The base duration

A syllabic phoneme's base duration is 0, and a phoneme whose inherent and shortest durations are the same gets its inherent one. For any other, with `I` and `S` its inherent and shortest durations times 10:
1. Unless prosody off is set, if a scan forward of 6 phonemes (section 2.2) finds a break before a syllabic phoneme, the phoneme is in its phrase's last syllable: the next percent is 160, 150 for `q`.
2. If a scan back of 7 finds a syllabic phoneme before a word mark, the phoneme isn't in its word's first syllable: the next percent is 85.
3. If `I` is at most `S`, the base duration is `I / 10`.
4. The context's percent `c` is 100. If the phoneme isn't laryngeal and the preceding sound is neither syllabic, nor an affricate, nor laryngeal: `c` is 90, 60 for an approximant. Unless the phoneme is stressed, `c` then loses 20, and becomes 40 for a closure after a closure of the same articulator ([parameters.md](parameters.md) §9.2), and the next step is skipped.
5. Unless the phoneme is a sonorant or an affricate, or has no following sound: if the following sound is neither syllabic nor laryngeal and the preceding sound isn't `S`, `c` becomes 70, and 30 for a fricative before a closure, or before a fricative unless after an affricate.
6. `c` is the next percent. Their product `P` starts at 100 and becomes `P · p / 100` for each percent `p`, from the last to the first. At rate 13 it becomes `(P / 2) · R / 100 · 2`, `R` the rate's duration percent. It becomes the first percent.
7. The base duration is `(I − S) · P / 100 + S`, halved for a stressed `q` after `S` before `L` or `R`, and divided by 10.

### 6.3 A phoneme's duration

From the node's value, its base duration `d`:
1. The phoneme may merge with the consonant before it (section 6.4). If it does, the context is noted again (section 2.3) and `d` becomes the merged node's value.
2. By the phoneme's class:
   - a vowel (class vowel): the vowel's duration;
   - a stop but `q`: the stop's duration, plus a geminate's frames (below);
   - a fricative, `H` or `d`: the fricative's duration, plus a geminate's frames;
   - a sonorant but `h`: the sonorant's duration;
   - the pause: section 6.6;
   - `q`: 7 before a nasal, and `d` otherwise; the flap `t`: 3; any other: `d`.
3. After a `P`, `T` or `K` (the preceding sound), the duration gains the preceding node's byte 0x10, its release frames, which the stop's duration sets.
4. Emphasis (section 6.5).
5. With contour 19 (section 3.5), when the sentence's word marks before the phrase (section 3.1) are 1 the duration becomes `d · 144 / 100`, and when they are 2 `d · 123 / 100`.
6. At any rate but 13: `R · (91 · d / 100) / 100`, `R` the rate's duration percent, but at least the shortest duration (`S / 10`) and at most 55.
7. It is held between 2 and 55 frames.

**A geminate's frames.** A geminate gets frames for the pair, unless the node before it in the list is a function word's mark (`%`). A fricative gets 8 for `F`, 6 for `x`, 6 for `S` between vowels (its preceding and following sounds), 5 for any other `S`, and 4 for the rest. A stop gets them only outside a function word. With `B` and `A` the sounds before and after the pair as the context noted them before the merge:
- `P`, `T` or `K`: 6 unless `B` is a vowel; else, before a glide (`R L W Y`), 9 for `K` and 6 for `P` and `T`; 6 before a sound that isn't a vowel, and for `T`; 10 for `P` and 7 for `K` before a vowel that isn't high; 8 for `P` and 6 for `K` before a high vowel.
- `D` between vowels: 7; any other `D` or `G`: 6.
- `B`: 8 when `B` is a vowel or a glide and `A` a high vowel; else 6 unless `B` is a vowel; else 8 before a vowel, 5 before a glide, and 6 otherwise.
- `C` and `J` are never geminates.

The phoneme is **in a function word** when, from the phoneme before the previous one (section 2.2), or with none from the symbol before the preceding one noted before the merge, a `%` comes before any `&` going back through the list's nodes.

### 6.4 Merging a pair

At rate 7 and up, a consonant (class consonant) merges with its previous sound (section 2.2) when the two are:
- the same symbol, unless the phoneme has flag bit 0x40 (section 3.3);
- `S` or `Z` then `s`, `Z` then `S`, `V` then `F`, `L` then `j`, or `B` then `P`;

but not `S` then `S` when the phoneme's previous phoneme is a word mark and its next one a consonant. The phoneme is a **geminate** when the two are the same symbol, `B` then `P` or `Z` then `S`, and it is a sonorant, a fricative or a stop but `C`, `J` and `q`.

The phoneme's node takes the pair: its value becomes the two values' sum, less 4 for `V` then `F`, and 55 at the most; its byte 0x18 becomes 1, a merged phoneme ([parameters.md](parameters.md) §10.1); and its second value, its pitch, becomes the sound before's. The sound before's node is freed, going back ([pipeline.md](pipeline.md) §2.3).

### 6.5 Emphasis

A phoneme of stress 3 gets `d · 130 / 100`. In a word with an emphasis, a syllabic phoneme then gets `d · 200 / 100` when stressed and `d · 125 / 100` otherwise; another sonorant `d · 150 / 100` when stressed and `d · 125 / 100` otherwise; a fricative `d · 125 / 100`. A function word's emphasis then gives `d · 150 / 100`.

### 6.6 A pause's duration

Phonetic input gives pauses of type 3. A pause's duration `D` is `(p / 4) · d / 100 · 4`, `p` the rate's pause percent (data pack `pausePercents`), and at least 4. When `D` is above 50, a pause node of 50 frames (section 5.4) goes before it and `D` loses 50, until `D` is 50 or less or fewer than 4 nodes are free. The pause's value becomes `D`, at most 50, and its second value 50, unless the node after it in the list is a `&`. The context is noted again (section 2.3), and the duration is `D`.

## 7. The reset

At the pipeline's reset ([pipeline.md](pipeline.md) §7), the marks become −1, −1, −1 and 0 and the scan's result 3, so that the next scan starts a sentence; no phrasing, slow speech, prosody off, the short phrase flag and lowered are cleared; the phrase limit becomes 180; the phonemes since the last mark, the padding, the presets' bits and the previous bits become 0, and the emphasis none (section 6.1). The cursor's types are 0, 3 and 4 (the pipeline sets them). The shared state (section 3.1) isn't reset.

## 8. What the recorded runs don't reach

- A division by zero in step 5 of section 4.3: the original faults; the port gives 0.
- A reset of the pitch (section 4.4) with the fall, the nucleus or the flags it clears still set: a sentence starts after a break, whose pitch context already cleared them (section 4.2, step 2). Only a reset of the stages within a phrase would leave them set.
- A whispered phoneme that already has a pitch: the stages before leave a phoneme's pitch 0.
- Rise set by a mark of value 17 at exactly `p = m17` (section 4.3, step 7): the phoneme before the marked word was never in the phrase's last syllable.
- Contexts where the steps' details make no difference: at `p = m7` with contour 19 the declination cancels the excursion, so 4 or 6 times it gives the same pitch (section 4.3, steps 4 and 5); the contours' quotients agree where they meet at a mark; and the phoneme before a nucleus of stress 3 (step 8) was always at the 500 Hz limit.
- The nucleus search running past the window's end before the phrase's end (section 4.2, step 4).
- A phrase of 0 phonemes at a phoneme's pitch, and contour 6 with `m6` 0 (the scan doesn't choose contour 6 then); a contour other than those of section 4.3.
- A symbol of 128 or more: the port gives it no classes and no durations.
- A phoneme whose inherent duration is below its shortest (section 6.2, step 3): no symbol's is.
- For `P`, `T` and `K` geminates after a vowel before a sound that isn't one (section 6.3), the original also tests whether the vowel is a nasal (8 frames for `P`, 5 for `T`, 9 for `K`) or a consonant (9). No symbol is both, so the port leaves the test out.
- The walk back for a function word's mark (section 6.3) reaching the list's start: every word has a mark.
- Dead code left out of the port (section 6.4): the original also takes `D` then `T` for a geminate, a pair that never merges, and makes a merged phoneme stressed when its context says it is, which it already is.
- Pairs of `C`, `J` or `q` (section 6.4): the lexical stage writes `C` and `J` as `C s` and `J z`, and no recorded `q` pair merged. Nor did a pair of `h` or `Q`: with `q`, the only consonants whose duration is the merged node's value (section 6.3, step 2).
- Durations at which a percent one higher would give another result: emphasis's 125 % from 27 frames, 150 % from 51 and 200 % from 100 (section 6.5), and the stressed `q`'s halving at 59 tenths of a frame (section 6.2, step 7). The recorded durations were all below.
- Running short of free nodes: the scheduler runs the stage with 106 free ([pipeline.md](pipeline.md) §6), and no phrase's `\` commands, breaks, slow pauses or pauses used them up (sections 3.2 to 3.4, 5.4 and 6.6).
- A vowel after a break while the cursor's pitch is 0 (section 3.3): the pitch is 50 Hz at least.
- Contour 0 with both `m6` and `m7` at 0: the marks of a sentence's first word have one value.
- Contour 0 for `m6` at 0 without `m7` replacing another contour (section 3.5): the contour is chosen at every scan, and a scan before a sentence's first word had always chosen 0 already.
- The walk running past the window's end (section 5.1, step 6): only a phrase broken off by a scan that started at the window's start and found no node but phonemes; a word that long leaves too few nodes free for the stage to run.
- Breaks placed at `!`, where the scan places none, and at a `C` command (section 3.2): at each of the 372 recorded ones the phrase had no words yet, the text's last phrase having ended before it.
- Relative, negative and pause presets, and a preset pitch of 1 (section 5.3), so a preset duration other than the node's value (section 6.1, step 3): the recorded presets are the lexical stage's durations of a few phonemes and pauses, and one pitch. Phonetic input writes presets in a syntax of the normalizer's rules, which will follow.
- Pauses with stress bits when the walk clears them (section 5.1, steps 2 and 3): no recorded pause has any.
- The synthesizer exactly 33 frames ahead (section 5.1, step 4): once the countdown had run out it was 0 frames ahead at 19,892 of 19,984 steps, and never between 24 and 38.
- A reset of a stage that has run (section 7): the recorded resets are of a new speaking thread's state, whose values are all 0. A reset within a session drops the rest of the text, so a recorded run never ends ([pipeline.md](pipeline.md) §9).
- The last symbol of a scan that looked at no node (section 3.5): in the original it is the low byte of the stage's state's address, which is never `!`.
