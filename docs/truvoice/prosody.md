# TruVoice: prosody

How L&H TruVoice (`tv_enua.dll`) gives each phoneme its duration and its pitch. The prosody stage works on the lexical stage's phonemes, the word marks before the words and the punctuation ([pipeline.md](pipeline.md) §3). It finds where its phrases end, puts in the pauses between them, and turns each phoneme into a segment for the parameter generator ([parameters.md](parameters.md)): a node of type 4 whose value is its duration in frames and whose second value is its pitch, in units of 2 Hz.

This spec covers the arithmetic (section 1), the phonemes the stage sees and what it notes about them (section 2), how it finds a phrase, breaks it and chooses its contour (section 3), the pitch (section 4), how it walks the phrase's nodes, with their presets and pauses (section 5), the durations (section 6), with the vowels' (sections 6.7 and 6.8), the sonorants' (section 6.9), the fricatives' (section 6.10) and the stops' (section 6.11), and its reset (section 7).

**In short.**
- The stage scans ahead for a sentence's end, a `C` command, or a limit: 180 phonemes (70 when text comes into an empty pipeline, 15 with `\Pro=0\`) or 240 nodes. Each comma on the way ends a phrase of its own, and the stage hands on the nodes before it as soon as it has timed them.
- Within a phrase it puts in breaks between words where the words' dictionary classes say, and a `P` command at each phrase's start that tells the pitch the phrase's kind: falling, rising, a comma's, or without an end of its own.
- At each break comes a pause: 39 frames at a sentence's end, 10 at a comma; at a rate of 25 or more, or with `\Pro=0\`, a short one. In slow speech, some function words after a content word get a pause before them, by a 16-step pattern whose position the whole process shares.
- Phonetic input can preset a phoneme's duration and pitch; the stage puts them back after it times the phoneme.
- A consonant's duration is its shortest plus a share of the rest, by where it is in its phrase and word and the sounds around it. At rate 7 and up a pair of the same consonant, or of two close ones, becomes one, longer. Stress 3 and emphasis lengthen a word, by 25 to 100 % and 50 % more for a function word, and the rate scales every duration. A duration is 2 to 55 frames.
- A vowel's duration comes from a grid, by its stress, whether its word is the phrase's last, whether it ends its word, and the sound after it. A glide before it with a nasal after it, approximants around it with a lateral after it, or a break after it shorten it, and so do its function word's rules.
- A sonorant's duration comes from a grid too, by its stress, whether its word is the phrase's last, where it is in its word and the sounds around it. Consonants on both sides take a frame off; a merged nasal before a vowel gets 3 to 8 more.
- A fricative's duration comes from a grid as well, but `H`, `V`, `d`, `x` and `z` have fixed durations by the same columns and rows, and a few clusters give a fricative a duration of their own.
- A stop's duration is fixed by the sounds around it, whether its word is the phrase's last and whether it starts its word, less a frame or two in a cluster of consonants; between vowels a `T` usually gets 8 frames and a `D` 6. `P`, `T` and `K` also get release frames, which the next phoneme's duration gains.
- A phrase's pitch starts a third of the voice's pitch above it, and comes down over the phrase's phonemes. Phonetic input can place word marks that make it come down to a mark and rise from it instead: the phrase's **contour**.
- The phrase's **nucleus** is its last stressed vowel before a break. The syllables before it and the stressed ones go up; the nucleus carries the phrase's accent.
- In the phrase's last syllable the pitch moves as the phrase's kind says: down at a sentence's end, up a little at a comma, up at a question. From there every phoneme is 8 Hz lower than the one before, down to 46 Hz.
- Small changes for the sounds: a voiceless stop starting a word before a vowel is 20 Hz higher, an unstressed voiced sound before a consonant 3 Hz lower (8 if it isn't a sonorant), a high vowel or glide 3 Hz higher, and an `N` before a `T` or a glottal stop at a word's end 55 Hz lower.
- A whispered phoneme gets pitch 0, a monotone one the voice's pitch. Otherwise the pitch stays between 50 and 500 Hz.

**Exactness.** Ported from the original's code, and checked against it by replaying every call of its functions recorded in 346 runs: the parameter generator's and the pipeline's runs ([parameters.md](parameters.md), [pipeline.md](pipeline.md)), and 118 more for the phrases and the pitch: questions, exclamations, lists, long sentences without punctuation, emphasis, `\Pro=0\`, pitch, speed, character and voice changes within a text, slow and fast speech, 30 with the word marks phonetic input places (section 3.5), 4 with long runs of consonants in phonetic input, and 47 for the phrase's breaks and pauses (speed, `v`, `r` and `P` commands within a sentence, speed at its end, `\Pro=0\` within a phrase, exclamations without a stressed vowel, the word classes that break a phrase, question words, the Address context, phrases of more nodes and word marks than the scan looks at, a held phrase broken off, word marks of values 6, 7 and 12 at more places, pauses as long as exactly 1 and 50 phonemes, and a pause before any text), and 3 with an engine reset during a text, one while a phrase is held. Each call left the speaking thread's state and the stage's shared state (section 3.1) byte-identical to the original's: the 40,436 runs of the stage (section 5.1) with their 40,436 scans (section 3.2), 2,290 placings of the breaks (section 3.3) with their 186 breaks, 6 of them after a stop, and 81 chances of a slow pause (section 3.4), 51 of which put one in, 64,530 walk steps, 34,349 nodes processed (section 5.2), 58,966 notings and restorings of presets (section 5.3), 2,444 pauses (section 5.4) and 349 resets (section 7); each of the 37,384 contexts (section 2.3), the 34,349 pitch contexts (section 4.2), the 34,349 pitches (section 4.3) and the 1,239 resets of the pitch (section 4.4). The durations in these replays were the Go stage's own (section 6), which are also checked on their own (below).

The durations were replayed the same way in 732 runs: those 346 and 386 more for the durations (pairs of the same consonant across words, after a function word, after a word of one consonant, within a word and at rates 6 and 7, words of stress 3, emphasized words of every kind, phonetic input's pairs and pauses, among them a pause of 100 frames, phonemes timed by their shortest durations alone, every vowel after a glide before a nasal and between approximants before a lateral, stressed and not, in the phrase's last word or not, inside its word or at its end, vowels before two pauses, function words within written words the stages split, function words before and after words of the classes their rules test, and every sonorant after and before each consonant its column tests, between vowels and before approximants, at its word's start, inside it and at its end, stressed and not, in the phrase's last word and not, in clusters, after a pause, and before `D` or `Z` and a full stop at five speeds, and every fricative and the frications of `C` and `J` after and before each sound their column tests, inside a word, at its start and at its end, alone, after a pause, stressed and not, in the phrase's last word and not, after a nasal before a stop, with a break 15 to 17 phonemes after them, and in the exceptions' contexts, and every stop after and before each sound its column tests, inside a word, at its start and at its end, after a pause, stressed and not before every vowel, in the phrase's last word and not, in clusters, after a nasal before a stop or a fricative, 16 phonemes before a break, and in the release frames' contexts). Each call left the speaking thread's state byte-identical to the original's: the 64,827 durations (section 6.1), with 17,768 nodes that aren't phonemes, 195 emphases and 82 phonemes timed by their shortest durations; the 44,960 base durations (section 6.2) and phonemes' durations (section 6.3), with as many tests for a pair, 650 of which merged one, 595 of them geminates (section 6.4), and as many emphasis steps (section 6.5); 17,719 scalings by the rate; 1,318 pauses (section 6.6), 64 of them over 50 frames; the 18,176 vowels' durations (section 6.7), 217 of them before a nasal after a glide and 147 before a lateral between approximants, and the 4,175 function words' vowels (section 6.8), 630 of which lost frames by 62 of the 101 rules; the 8,865 sonorants' durations (section 6.9), in all 20 columns, 174 of them after `S` or `Z` and a pause, 1,579 losing a frame to a cluster, 62 merged nasals before a vowel, 66 before `D` or `Z` and a full stop, and 47 unstressed `~`s whose grid holds 0; the 7,656 fricatives' durations (section 6.10), in every column a fricative takes, 614 of them the frications of `C` and `J`, 620 given by an exception, of every kind, 205 seeing past a pause, 99 `V`s and `F`s changed by step 7, 48 taking column 3 of their grid for a 0 and 204 their own duration, and 394 of the fixed durations' places; and the 8,188 stops' durations (section 6.11), in 532 of the table's places and all 156 of the release frames', 41 shortened after a nasal before a stop or a fricative, 1,600 between vowels, 692 of them flaps, 1,745 after a consonant, `l` or `n`, 925 of which lost frames, by every case of step 9, 27 in column 8 after `| N`, 43 `D`s before `p` after `N`, 16 `D`s and `T`s after `S` before a glide, 246 `T`s before `p`, and 14 held at 3 frames; of their release frames, 602 at a word's end, 382 after `S`, 60 after a space before a glide, 13 before `N` and 1 of a `P` before `l`.

The stage was also checked as a whole, in the pipeline, in 729 recorded runs: these 732 but the 3 with an engine reset during a text, which starts a new speaking thread ([pipeline.md](pipeline.md) §8.2; the calls above check those runs). The Go speaking thread started each run as the original's, and the Go scheduler, prosody stage, parameter generator ([parameters.md](parameters.md)), output stage and synthesizer ran all 339,250 of the original's scheduler steps, with what the stages before prosody wrote taken from the recordings. Each of the stage's 80,958 runs started from the original's state, reported the same and left the speaking thread's whole state and the shared state the same. The audio, 61,026,100 samples in 47,574 hand-overs, was the original's PCM in every run, and each of the 12,473 words and 31 bookmarks came at the sample where the original reported it.

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
| (1, 0x01) | boundary | `! , . ? \ ]` |
| (1, 0x08) | affricate | `C J` |
| (1, 0x20) | consonant | every phoneme but the vowels, `p`, `Y`, `l`, `m`, `n` and the pause |
| (1, 0x40) | break | `! ) , . ? \ ]` |
| (2, 0x01) | closure | `B P D T t J C G K Q q` and the nasals |
| (2, 0x02) | vowel | the vowels and `p` |
| (2, 0x04) | alveolar | `D N S T Z n q t` |
| (2, 0x40) | lateral | `L j l` |
| (3, 0x01) | approximant | `L R W Y h j l` |
| (3, 0x02) | laryngeal | the pause, `H Q d` |
| (3, 0x40) | front-ending | `5 A E I y` |
| (3, 0x80) | lax | `@ a e i p u w \|` |
| (4, 0x01) | high | `4 E U W Y b c h i u \|` |
| (5, 0x40) | fortis | `C F H K P S T Y s` |

A class can also be **negated**: the symbols not in it. A missing node is in no class, negated or not.

### 2.2 Walking the window

The stage walks the nodes of its window ([pipeline.md](pipeline.md) §2.3: there is no node after the window's end or before its start). From a node:

- its **next phoneme** is the first node of type 3 after it;
- its **previous phoneme** is the last node of type 3 or 4 before it;
- its **next sound** is the first node of type 3 after it that isn't a word mark, `%` or `&`;
- its **previous sound** is the last node of type 3 or 4 before it that is neither a word mark, `%` or `&`, nor in class break.

There is none when the window has none.

**Scanning** looks at up to `n` phonemes from the cursor's scan node, the **current** node: its next phonemes going forward, its previous phonemes going back. With a class to find and a class to stop at, it reports a find at the first phoneme in the class to find, and stops without one at the first phoneme not in the class to stop at; the scan stops without a find after `n` phonemes or at the window's end.

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
   - a vowel (class vowel): its duration (section 6.7);
   - a stop but `q`: its duration (section 6.11), plus a geminate's frames (below);
   - a fricative, `H` or `d`: its duration (section 6.10), plus a geminate's frames;
   - a sonorant but `h`: its duration (section 6.9);
   - the pause: section 6.6;
   - `q`: 7 before a nasal, and `d` otherwise; the flap `t`: 3; any other: `d`.
3. After a `P`, `T` or `K` (the preceding sound), the duration gains the preceding node's byte 0x10, its release frames, which the stop's duration sets (section 6.11).
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

### 6.7 A vowel's duration

A vowel's duration starts from its **grid** in the data pack: 8 rows of 20 columns, row `r` and column `c` at position `20 · r + c`. Grid `g`'s position `k` is byte `gridOffsets[g] + k` of `durationGrids`, and a byte before the first is 0. The vowels `E i A e a o w O u b v I f U y g k c r 3 4 5 @ |` have grids 0 to 23, in that order.

The steps use the symbols the context noted (section 2.3): the next, the following, the after following, the preceding and the before preceding; and the **one after next**, the symbol of the next node's next phoneme, 0 when there is none.
1. **Flaps.** A following flap `t` becomes `T` when the list's node after the vowel's has byte 0x18 `T`, and an after following `t` when the node two after the vowel's has.
2. **`p`.** The vowel `p` gets 3 frames after `B`, 4 after `D`, `M`, `N` and `P`, 5 after `G`, 8 after `K`, after `T` 4 when the before preceding is a nasal and 6 otherwise, 6 after `~`, and 7 after anything else. The other steps don't apply to it.
3. **Inside its word.** The vowel is inside its word unless the next is a word mark or in class boundary, or the following is `p`, a space or a comma.
4. **The last word.** The vowel's word is the phrase's last if a scan forward of 16 phonemes finds a break before a word mark.
5. **No following sound.** If the following is a space, it becomes the after following, and the after following becomes the symbol of the after following node's next sound, 0 if there is none.
6. In the last word, a vowel inside its word whose next is neither a vowel nor `l`, `m` or `n` stays inside it only if, from the next node's next phoneme on, the list's nodes of any type before a `&`, `%`, `.`, `?` or `!` include a vowel other than `p`, or an `l`, `m` or `n`.
7. A vowel before a flap `t` (the following) is inside its word, and `y` before `l` (the next) with a word mark or a break one after next isn't.
8. **The row** is 0 for a stressed vowel and 4 for an unstressed one, plus 2 in the last word and 1 inside the word.
9. **The column**, by the sounds around the vowel:
   - after `R`, `L`, `W`, `Y` or `j` (the preceding), before a fricative or a closure (the following): 16, or 18 when the following is voiced, plus 1 when it isn't a closure;
   - otherwise: 1 when the preceding and the before preceding are spaces and the following is in class boundary; else 0 when the following isn't a phoneme; else 2 when the preceding and the following are both approximants; else 13 before a vowel;
   - otherwise by the following: 3 for `C K P T q`, 4 for `F H S X s`, 5 for `B D G J t`, 6 for `V Z d x z`, 14 for `R` and 15 for `W Y`. For `L j l`: 11 when the after following isn't a stop or the one after next is a word mark, else 7 when the after following isn't voiced or is `q`, else 8. For the nasals `M N m n ~` the same with 12, 9 and 10;
   - −1 for any other following: the pause, `Q` or `h`. It reads the byte before the row's first: the row before's last, and in row 0 the byte before the grid in `durationGrids`.
10. The duration `d` is the grid's byte at the row and column.
11. **A nasal after a glide.** In column 18, when the next is a nasal and the one after next a closure in class fortis or `q`, `d` loses the frames of the table below for a nasal.
12. **A lateral between approximants.** In column 2, when the next is a lateral and the one after next a closure in class fortis or `q`, `d` loses the frames of the table below for a lateral.
13. A front-ending vowel gains 3 before a vowel (the next).
14. A vowel other than `i`, `v`, `|` and `@` after `R L W Y j`, before a vowel (the following), loses 2 when it is stressed and in the last word, and 1 when it is only one of the two.
15. **No duration.** If `d` is now 0, it becomes the grid's byte in column 15 of row 1 for a stressed vowel and of row 5 for an unstressed one; if that is 0 too, the vowel's inherent duration when stressed and its shortest otherwise. At stress 1 it loses 2. The vowel's duration is `d`, at least 3, and the steps below don't apply.
16. At stress 1, `d` loses 2.
17. A stressed `i`, `v`, `|` or `@` after `R L W Y j`, before a vowel or `R L W Y j` (the following), loses 1.
18. In its word's first syllable (its node's byte 0x17 is 1), a vowel other than `i`, `v`, `|` and `@` whose previous symbol is `R L W Y j` loses 1, or 2 in the last word, when the list's node two before it is a closure in class consonant and the node three before it `S`, as in a word starting with "spr".
19. **Before a break.** After `R L W Y j`, before a break (the next), it loses byte `g` of `breakReductions` when unstressed and byte `g + 23` when stressed, `g` its grid. The two rows overlap: grid 23's unstressed byte is grid 0's stressed one.
20. **A function word's vowel.** A vowel whose node's byte 0x19, its word's class (section 2.1), isn't 0 loses the frames of its class's rules (section 6.8), and then:
    - of class 15, `k` after `x` ("though"), when the list's node after it is a break, in the last word and stressed: 10 when the first word mark before the list's node three before it, up to 16 phonemes back, has class 2, 8, 9, 11 or 12;
    - of class 1, `v` before `S`: 3 when the first word mark before the node two before it, up to 16 phonemes back, has class 3, 4, 10 or 12;
    - of class 5, `a` after `x` ("that"), not in the last word nor at its phrase's start (section 6.8), when there is a word mark up to 16 phonemes before the node two before it: 4 when that mark has class 7, unless the two nodes after it are `w j` ("all") or the nodes five, four and three before the vowel are `E`, `x` and `3` ("either"), or when the node two after the vowel has class 1 or 5, class 2 unless the three nodes after it are `K a N` ("can"), or class 13 unless the four nodes after it are `W i C s` ("which"). The text has a word command before each written word, so that node is a word mark only in a written word the stages before split, as `know-that-he`;
    - of class 1, `E`: 3 after anything but `W` before `j`, `D` or `t`, and 3 after `W` before a consonant other than `R L W Y j`.
21. A `u` of class 10 gets at most 6 frames, and 3 before `P`, `T` or `K`, or before `S` with the after following one of `P T K B D G`.
22. A `v` with a `&` before it and an `N` after it in the list gets at least 8.
23. The vowel's duration is `d`, at least 3.

The frames a vowel loses before a nasal (step 11) and before a lateral (step 12), stressed or not: not in the last word, in the last word inside the word, and in the last word at the word's end. Not in the last word, a vowel before a nasal or a lateral is always inside its word.

| Vowel | Nasal: stressed, not in the last word | stressed, inside | stressed, at the end | unstressed, not in the last word | unstressed, inside | unstressed, at the end | Lateral: stressed, not in the last word | stressed, inside | stressed, at the end | unstressed, not in the last word | unstressed, inside | unstressed, at the end |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `E` | 3 | 3 | 10 | 2 | 3 | 6 | 2 | 3 | 3 | 1 | 1 | 2 |
| `i` | 3 | 3 | 7 | 1 | 2 | 4 | 1 | 1 | 2 | 1 | 1 | 2 |
| `A` | 3 | 3 | 10 | 2 | 3 | 6 | 2 | 2 | 3 | 1 | 1 | 2 |
| `e` | 3 | 4 | 10 | 1 | 2 | 4 | 0 | 0 | 2 | 1 | 1 | 2 |
| `a` | 3 | 4 | 10 | 1 | 2 | 4 | 2 | 3 | 3 | 1 | 1 | 2 |
| `o` | 4 | 4 | 10 | 2 | 3 | 6 | 2 | 3 | 3 | 1 | 1 | 2 |
| `w` | 4 | 4 | 10 | 2 | 3 | 6 | 2 | 3 | 2 | 1 | 1 | 2 |
| `O` | 3 | 4 | 10 | 2 | 3 | 6 | 2 | 2 | 3 | 1 | 1 | 2 |
| `u` | 3 | 3 | 7 | 1 | 2 | 4 | 1 | 1 | 2 | 0 | 0 | 2 |
| `b` | 3 | 4 | 10 | 2 | 3 | 6 | 1 | 1 | 2 | 1 | 1 | 2 |
| `v` | 1 | 4 | 7 | 1 | 2 | 4 | 1 | 1 | 2 | 1 | 1 | 2 |
| `I` | 3 | 2 | 12 | 3 | 4 | 7 | 3 | 4 | 4 | 3 | 3 | 4 |
| `f` | 3 | 2 | 10 | 3 | 4 | 7 | 3 | 4 | 4 | 3 | 3 | 4 |
| `U` | 2 | 2 | 12 | 3 | 4 | 7 | 3 | 4 | 4 | 3 | 3 | 4 |
| `y` | 2 | 2 | 12 | 3 | 4 | 7 | 3 | 4 | 4 | 3 | 3 | 4 |
| `g` | 2 | 2 | 12 | 3 | 4 | 7 | 3 | 4 | 4 | 3 | 3 | 4 |
| `k` | 2 | 2 | 12 | 3 | 4 | 7 | 3 | 4 | 4 | 3 | 3 | 4 |
| `c` | 2 | 2 | 12 | 3 | 4 | 7 | 3 | 4 | 4 | 3 | 3 | 4 |
| `r` | 2 | 2 | 12 | 3 | 4 | 7 | 3 | 4 | 4 | 3 | 3 | 4 |
| `3` | 1 | 1 | 10 | 1 | 2 | 4 | 2 | 2 | 3 | 3 | 3 | 4 |
| `4` | 2 | 2 | 12 | 3 | 4 | 7 | 3 | 4 | 4 | 3 | 3 | 4 |
| `5` | 2 | 2 | 12 | 3 | 4 | 7 | 3 | 4 | 4 | 3 | 3 | 4 |
| `@` | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 1 |
| `\|` | 1 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 1 |

### 6.8 A function word's vowel

The data pack's `functionWordRules` has a list of rules for each word class from 0 to 15. With the following symbol as section 6.7 left it after its step 5, the vowel loses no frames when the following is an approximant and the vowel is `k` of class 13, `3` of class 12 or `b` of class 7, or, when the list's node before the vowel is a word mark and the node before that an approximant, `r` of class 12, `g` of class 14, or a vowel of class 7 with a `j` after it in the list. A class of 16 or more has no rules.

What the rules test about the vowel:
- it is at its phrase's **start** when the first word mark before it, up to 16 phonemes back, comes right after a `P` command or a comma in the list;
- its word is the phrase's last, as in section 6.7;
- its phrase ends in a question when the first break after it, up to 16 phonemes forward, is `?`;
- it is stressed.

A rule has `back`, `symbols`, `conditions`, `tests` and `frames`. The vowel loses the frames of the first rule of its class that it passes, and none if it passes none. It passes a rule when:
1. the list's nodes from the one `back` nodes before the vowel's have the rule's `symbols`, and another node comes after them;
2. it meets each condition whose bit is set: 1 at its phrase's start, 2 in the last word, 4 not at its phrase's start, 8 not in the last word, 16 the phrase doesn't end in a question, 32 unstressed and 64 stressed;
3. it passes the tests. Each test looks at a node: with `direction` 1, the vowel's `steps`-th next phoneme (section 2.2), and the rule fails when there is none; with `direction` 0, the node the test before looked at, or after the first the node after the symbols. By its `kind`, the test passes when:
   - 0: the node is in class `(value >> 8, value & 0xFF)`;
   - 1: the node's second value, a word mark's class, is `value`. A rule's tests of kind 1 make one test, which passes when one of them does;
   - 2: the node's second value isn't `value`;
   - 3: the following is in class `(value >> 8, value & 0xFF)`; 4: it isn't; 5: the following isn't the symbol `value`.

### 6.9 A sonorant's duration

The sonorants `M N ~ R Y L W l j` have grids 29 to 37 (section 6.7), in that order; `n` has `N`'s grid and `m` `M`'s. The grids of `M` and `N` have 12 rows. The steps use the symbols the context noted (section 2.3).
1. **A pause before.** When the preceding is a space and the before preceding `S` or `Z`, the preceding becomes that `S` or `Z`, but not for `~`. Within a word the lexical stage puts such a pause between `S` or `Z` and `L`, `M`, `N`, `R`, `W` or `j`.
2. **The last word**, as in section 6.7, step 4.
3. **Its place in its word.** The sonorant is at its word's **start** when the preceding and the before preceding are spaces or the previous symbol is a word mark; otherwise at its **end** when the next is a word mark or the sonorant is **before silence**: the following and the after following are spaces, or the following is `.`, `!`, `,`, `?` or `\`; otherwise **inside** it.
4. **The column**, by the sounds around it:
   - after a vowel, before an approximant: 2; before a vowel in class lax: 1; before another vowel: 0;
   - otherwise: 3 when the preceding and the before preceding are spaces; else 4 before silence; else 5 when the preceding is a consonant but not a nasal and the before preceding a nasal;
   - otherwise by the preceding, an `s` after `C` counting as `C` and a `z` after `J` as `J`: 6 for `j`, 7 for `l`, 8 for `C K P T`, 9 for `F H S X s`, 10 for `B D G J Q q t`, 11 for `V Z d x z` and 12 for the nasals;
   - otherwise by the following: 13 for `R`, 14 for `d`, 15 for `C K P T`, 16 for `F H S X s`, 17 for `B D G J Q q t`, 18 for `V Z x z`, 19 for the nasals, and 1 for any other.
5. **The row.** For `M m N n`: 0 at its word's end, 1 at its start and 2 inside it, plus 3 in the last word and 6 when it is unstressed. For `R L W Y`: 0 at its word's start and 1 otherwise; for `~ l j`: 0 at its word's end and 1 otherwise; plus 2 in the last word and 4 when unstressed.
6. The duration `d` is the grid's byte at the row and column.
7. **`n`.** Not at its word's end: after `q` (the preceding), 12 in the last word and 10 otherwise; after anything else `d` gains 5. At its end: 23 in column 4, else 14 after `q` outside the last word.
8. **No duration.** If `d` is 0, it becomes the grid's byte in column 1 of row 0 for a stressed sonorant and of row 4 for an unstressed one; if that is 0 too, the sonorant's inherent duration when stressed and its shortest otherwise.
9. **Clusters.** A sonorant other than `l`, `m` and `n`, in a column other than 5, 7 and 13, loses 1 when the preceding, as the context noted it, and the before preceding are consonants, or the following and the after following are, or the preceding and the following are.
10. `N` before `L`, `l` or `j` (the following) loses 1.
11. **A merged nasal.** A geminate `M` (section 6.4) before a vowel gains 8, or 5 when the following's node is stressed; in the last word 6, or 4. A geminate `N` before a vowel gains 3, or 5 when the following's node is stressed, and 5 in the last word.
12. An `n` outside the last word whose following isn't a space gets 9.
13. **Before `D` or `Z` and a full stop** (the following and the after following): before `D`, `l` and `j` gain 5, `M` 4, and `N` and `~` 3; before `Z`, `l`, `j` and `~` gain 4, and `M` and `N` 2.
14. `R` after `4` or `5` (the previous symbol) before a vowel gets 3.
15. The duration is `d`, at least 3.

### 6.10 A fricative's duration

The fricatives `F X S s Z` have grids 24 to 28 (section 6.7), in that order. `H`, `V`, `d`, `x` and `z` have none: their durations are fixed, in the tables below. An `s` after `C` (the preceding) is the **frication** of the affricate `C`, and a `z` after `J` that of `J`. The steps use the symbols the context noted (section 2.3).
1. **Past a pause.** When the following is a space, `S` and `Z` before `M`, `N` or an approximant (the after following), and `S` before `Z`, take the after following as the following; so do an `s` whose previous symbol is `C` and a `z` whose previous symbol is `J`, before a nasal or `L`, when the list's node three after their own is a vowel. A `Z` after a space (the preceding) with `S` before it (the before preceding) takes `S` as the preceding.
2. **The last word**, as in section 6.7, step 4. An `S` is in the last word too when the list's node two after its own is a space whose value is above 20.
3. **Inside its word**, as in section 6.7, step 3, with step 1's following.
4. **The column**, by the sounds around it:
   - after a space: 0;
   - after a vowel, before a vowel: 19 when the previous symbol is in class word mark, 1 otherwise;
   - in the last word and not inside its word: 2;
   - otherwise by the preceding: for `B D G J Q q t`, 6 when the previous symbol is in class word mark and 5 otherwise; for `C K P T`, 4 when it is and 3 otherwise, **column 3 after a stop**; 7 for `F H S X s`; 10 for `L R W Y j l`; for `M N ~`, 13 before a closure that is a consonant (the following) and 9 otherwise; 8 for `V Z d x z`; 9 for `m n`;
   - otherwise by the following: 12 for `B D G J Q q t`, 11 for `C K P T`, 14 for `F H S X s`, 17 for `L R W Y j l`, 16 for `M N m n ~`, 15 for `V Z x z`, and 3 for any other.

   No fricative takes column 18.
5. **The row**, as a vowel's: 1 inside its word, plus 2 in the last word and 4 when unstressed.
6. **The duration** `d` is an exception's when the sounds around the fricative give one (below); otherwise, for `H V d x z`, their table's value at the row and column, and for the others the grid's byte at the row and column, in column 3 after a stop.
7. **`V` and `F`.** `V` before `X` gains 4, and before a voiced fricative (the following in classes voiced and fricative) loses 2. `F` in column 0 in the last word, after `&` or `%` (the previous symbol), gains 2.
8. **No duration.** If `d` is 0, a fricative whose `d` came from its grid takes the grid's byte in column 3 of row 0 when stressed and of row 4 otherwise. If that is 0 too, or for any other fricative, `d` becomes its inherent duration when stressed and its shortest otherwise.
9. The duration is `d`, at least 3.

**The exceptions**, by column (step 4):
- 2: the frication of `C`: 16.
- 3 after a stop: `S` after a closure that isn't voiced, before a consonant that isn't voiced, unstressed, outside the last word and not inside its word: 6. The frication of `C` inside its word: when the following is a consonant and the before preceding or the after following is one too, 5, plus 1 when stressed and 1 in the last word; otherwise 9 when stressed, and when unstressed 8 in the last word and 7 outside it.
- 4: `V` inside its word, after a closure that is a consonant, with a consonant before that (the before preceding): 5 when unstressed, 6 in the last word; 7 when stressed, 9 in the last word.
- 5: the frication of `J` with a consonant before it or after it (the before preceding or the after following): 6 when unstressed, 7 in the last word; when stressed, 7 inside its word and 0 otherwise.
- 7: `F` after a fricative that is a consonant, with a closure that is a consonant before that, unstressed, inside its word, in the last word: 7.
- 9: `F` after `N` and not after a word mark (the previous symbol), before a consonant, stressed, inside its word, outside the last word: 9.
- 11 and 12: `S` before a closure in class alveolar with a consonant after it (the after following), outside the last word: 6 when unstressed; when stressed, 9 inside its word and 0 otherwise. `V` before a closure that is a consonant, with a consonant after it, unstressed, outside the last word: 5.
- 13: `S` after `N` and not after a word mark, with a consonant after its following, stressed, inside its word, outside the last word: 5. `z` with a consonant after its following, unstressed, not inside its word: 7.
- 14: `F` before a fricative with a fricative after it, unstressed, inside its word, outside the last word: 5.

**The fixed durations**, by row and column; 0 is the fricative's own duration (step 8), and `–` marks the places no fricative comes to: column 2 takes every fricative in the last word that isn't inside its word, and no fricative takes column 18.

| `H` row | 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 10 | 11 | 12 | 13 | 14 | 15 | 16 | 17 | 18 | 19 | 3 after a stop |
| ---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|--- |
| 0 | 8 | 8 | – | 6 | 7 | 6 | 0 | 7 | 6 | 0 | 7 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | – | 9 | 6 |
| 1 | 8 | 8 | – | 6 | 5 | 6 | 0 | 7 | 6 | 0 | 7 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | – | 9 | 6 |
| 2 | 7 | 8 | 0 | – | – | – | – | – | – | – | – | – | – | – | – | – | – | – | – | 9 | – |
| 3 | 7 | 8 | – | 6 | 7 | 6 | 0 | 6 | 6 | 0 | 7 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | – | 9 | 6 |
| 4 | 6 | 8 | – | 5 | 7 | 6 | 0 | 5 | 6 | 0 | 7 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | – | 9 | 5 |
| 5 | 6 | 8 | – | 5 | 5 | 6 | 0 | 5 | 6 | 0 | 7 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | – | 9 | 5 |
| 6 | 7 | 8 | 0 | – | – | – | – | – | – | – | – | – | – | – | – | – | – | – | – | 9 | – |
| 7 | 7 | 8 | – | 5 | 6 | 6 | 0 | 5 | 6 | 0 | 7 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | – | 9 | 5 |

| `V` row | 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 10 | 11 | 12 | 13 | 14 | 15 | 16 | 17 | 18 | 19 | 3 after a stop |
| ---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|--- |
| 0 | 9 | 8 | – | 9 | 10 | 6 | 8 | 5 | 7 | 8 | 6 | 5 | 6 | 6 | 6 | 8 | 5 | 6 | – | 5 | 6 |
| 1 | 9 | 8 | – | 9 | 10 | 8 | 8 | 7 | 7 | 8 | 6 | 5 | 6 | 6 | 6 | 18 | 5 | 6 | – | 9 | 6 |
| 2 | 9 | 9 | 19 | – | – | – | – | – | – | – | – | – | – | – | – | – | – | – | – | 9 | – |
| 3 | 9 | 9 | – | 10 | 11 | 8 | 9 | 7 | 6 | 7 | 6 | 7 | 9 | 6 | 5 | 7 | 8 | 8 | – | 9 | 6 |
| 4 | 8 | 6 | – | 8 | 10 | 6 | 7 | 5 | 5 | 7 | 6 | 6 | 6 | 6 | 6 | 6 | 7 | 7 | – | 5 | 6 |
| 5 | 8 | 5 | – | 8 | 10 | 6 | 7 | 5 | 5 | 7 | 6 | 6 | 7 | 6 | 6 | 6 | 6 | 5 | – | 7 | 6 |
| 6 | 8 | 8 | 19 | – | – | – | – | – | – | – | – | – | – | – | – | – | – | – | – | 7 | – |
| 7 | 8 | 4 | – | 11 | 8 | 7 | 8 | 5 | 5 | 7 | 6 | 6 | 8 | 6 | 6 | 6 | 7 | 7 | – | 7 | 6 |

| `d` row | 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 10 | 11 | 12 | 13 | 14 | 15 | 16 | 17 | 18 | 19 | 3 after a stop |
| ---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|--- |
| 0 | 6 | 8 | – | 0 | 0 | 7 | 6 | 0 | 10 | 5 | 7 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | – | 9 | 6 |
| 1 | 6 | 8 | – | 0 | 0 | 7 | 6 | 0 | 6 | 7 | 7 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | – | 9 | 6 |
| 2 | 6 | 9 | 0 | – | – | – | – | – | – | – | – | – | – | – | – | – | – | – | – | 9 | – |
| 3 | 6 | 9 | – | 0 | 0 | 8 | 7 | 0 | 6 | 7 | 8 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | – | 9 | 6 |
| 4 | 6 | 7 | – | 0 | 0 | 6 | 6 | 0 | 10 | 6 | 5 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | – | 6 | 6 |
| 5 | 6 | 7 | – | 0 | 0 | 6 | 6 | 0 | 5 | 6 | 5 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | – | 6 | 6 |
| 6 | 6 | 7 | 0 | – | – | – | – | – | – | – | – | – | – | – | – | – | – | – | – | 7 | – |
| 7 | 6 | 7 | – | 0 | 0 | 7 | 6 | 0 | 5 | 5 | 5 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | – | 7 | 6 |

| `x` row | 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 10 | 11 | 12 | 13 | 14 | 15 | 16 | 17 | 18 | 19 | 3 after a stop |
| ---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|--- |
| 0 | 6 | 7 | – | 6 | 6 | 6 | 5 | 7 | 7 | 5 | 6 | 7 | 7 | 0 | 6 | 8 | 8 | 8 | – | 5 | 6 |
| 1 | 6 | 5 | – | 6 | 6 | 7 | 7 | 7 | 7 | 5 | 6 | 7 | 7 | 0 | 6 | 8 | 8 | 8 | – | 5 | 6 |
| 2 | 8 | 7 | 13 | – | – | – | – | – | – | – | – | – | – | – | – | – | – | – | – | 5 | – |
| 3 | 8 | 7 | – | 6 | 9 | 7 | 7 | 8 | 8 | 6 | 7 | 8 | 8 | 0 | 7 | 8 | 8 | 8 | – | 5 | 6 |
| 4 | 5 | 7 | – | 5 | 5 | 5 | 5 | 6 | 6 | 5 | 5 | 6 | 6 | 0 | 5 | 8 | 7 | 7 | – | 5 | 6 |
| 5 | 5 | 5 | – | 5 | 5 | 5 | 5 | 6 | 6 | 5 | 5 | 6 | 6 | 0 | 5 | 7 | 7 | 7 | – | 5 | 6 |
| 6 | 5 | 6 | 15 | – | – | – | – | – | – | – | – | – | – | – | – | – | – | – | – | 5 | – |
| 7 | 5 | 6 | – | 5 | 6 | 5 | 5 | 6 | 6 | 5 | 5 | 7 | 7 | 0 | 6 | 7 | 7 | 7 | – | 5 | 6 |

| `z` row | 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 10 | 11 | 12 | 13 | 14 | 15 | 16 | 17 | 18 | 19 | 3 after a stop |
| ---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|--- |
| 0 | 6 | 9 | – | 0 | 0 | 8 | 0 | 0 | 7 | 0 | 0 | 8 | 10 | 0 | 9 | 9 | 9 | 7 | – | 9 | 6 |
| 1 | 6 | 9 | – | 0 | 0 | 8 | 0 | 0 | 7 | 0 | 0 | 8 | 10 | 0 | 9 | 9 | 9 | 7 | – | 9 | 6 |
| 2 | 6 | 11 | 18 | – | – | – | – | – | – | – | – | – | – | – | – | – | – | – | – | 11 | – |
| 3 | 6 | 11 | – | 0 | 0 | 8 | 0 | 0 | 7 | 0 | 0 | 9 | 10 | 0 | 10 | 10 | 10 | 7 | – | 11 | 6 |
| 4 | 6 | 11 | – | 0 | 0 | 8 | 0 | 0 | 7 | 0 | 0 | 7 | 9 | 0 | 8 | 8 | 8 | 8 | – | 8 | 6 |
| 5 | 6 | 11 | – | 0 | 0 | 7 | 0 | 0 | 7 | 0 | 0 | 7 | 9 | 0 | 8 | 8 | 8 | 6 | – | 8 | 6 |
| 6 | 6 | 10 | 18 | – | – | – | – | – | – | – | – | – | – | – | – | – | – | – | – | 10 | – |
| 7 | 6 | 10 | – | 0 | 0 | 7 | 0 | 0 | 7 | 0 | 0 | 8 | 9 | 0 | 9 | 9 | 8 | 7 | – | 10 | 6 |

### 6.11 A stop's duration

The stops `B C D G J K P T` (class stop but `q`) have fixed durations, in the tables below, by row and column. `P`, `T` and `K` also get **release frames**, their node's byte 0x10, which the duration of the phoneme after them gains (section 6.3, step 3). The steps use the symbols the context noted (section 2.3); a **glide** is `L R W Y j`.
1. **The last word**, as in section 6.7, step 4.
2. **At its word's start.** The stop is at its word's start when the preceding is a space or the previous symbol is `&` or `%`.
3. **The column**, by the sounds around it:
   - after a space: 11;
   - after a vowel or a glide: 0 before `E b i u`, 2 before a glide, 1 before another vowel but `p`;
   - after anything else, before a glide: 3 after a nasal and 4 otherwise;
   - otherwise by the following: 5 for `p`, 6 for `d`, 7 for `l`, 8 for `n`;
   - otherwise by the preceding: 9 for `C K P T q`, 10 for `F H S X s`, 12 for `B D G J t`, 13 for `V Z x z`; for `M N ~` 14, but 8 for an `N` after `|` (the before preceding) when the stop is at its word's start;
   - otherwise by the following: 15 for `C K P T q`, 16 for `F H S X s`, 17 for `B D G J t`, 18 for `V Z x z`, 19 for `M N ~`, and 20 for any other.
4. **The row** is 1 when the stop isn't at its word's start, plus 2 in the last word.
5. **The duration** `d` is the table's value at the row and column. A `D` after `N` (the preceding) before `p`, in the last word, gets 5.
6. **After a nasal.** When the previous symbol is a nasal and the next a stop or a fricative, `P` loses 2, and `T` 1 outside the last word.
7. **Release frames** for `P`, `T` and `K` (below).
8. **Between vowels.** After a vowel or a glide, before a vowel, when the phrase's last node (section 5.1) is unstressed, `D` gets 6 and `T` 8.
9. **Clusters.** Otherwise a stop other than `C` and `J` after a consonant that isn't a nasal, or after `l` or `n`, loses the frames of the first of these that applies, a **cluster consonant** being a consonant that isn't a glide, or `l` or `n`:
   - 2 before a cluster consonant (the following), when the before preceding is a consonant that isn't a nasal or the after following is a consonant;
   - 1 for `P`, `T` and `K` after `n` before a cluster consonant;
   - 1 after a sound that isn't a glide, when the before preceding is a nasal;
   - 2 for `B`, `D` and `G` after `n` before a cluster consonant;
   - 2 when the after following is `l` or `n`, or the following is a consonant and the after following a cluster consonant;
   - none before a sound that isn't a cluster consonant, when the before preceding is neither a consonant nor `l` or `n`;
   - 1 otherwise.
10. `D` and `T` at their word's start after `S`, before a glide, gain 1. Before `p`, `T` gets 9, and `C` and `J` gain 1.
11. The duration is `d`, at least 3.

**The release frames** of `P`, `T` and `K` are none after a vowel, before a vowel, when the phrase's first and last nodes (section 5.1) are stressed. Otherwise they are the table's value by row, 1 when the stop isn't at its word's start plus 2 in the last word and 4 when it is unstressed, and by **group**: after a space 6, else by the following: 0 for `4 E i |`, 1 for `O U b c g u w y`, 2 for `A a e k`, 3 for `3 @ I f o r v`, 4 for `L R W Y`, and 5 for any other. Then the first of these that applies:
- `T` before `l`: 2;
- before `N`: 1 fewer;
- at its word's end (the next is `&` or `%`): `T` after `S` before a vowel whose node is stressed (the following's) gets none; `P` and `T` before a vowel 1; any of them before a glide 2;
- after a space, before a glide: 2 more;
- after `S` not at its word's start: none for `T` and 1 for `P` and `K`.

`P` before `l` then has 1 fewer, unless it has none.

**The durations**, by row and column; `–` marks the places no stop comes to, after a space and not at its word's start.

| `B` row | 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 10 | 11 | 12 | 13 | 14 | 15 | 16 | 17 | 18 | 19 | 20 |
| ---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|--- |
| 0 | 7 | 10 | 10 | 5 | 8 | 7 | 7 | 6 | 7 | 8 | 9 | 10 | 7 | 7 | 5 | 7 | 6 | 6 | 6 | 6 | 7 |
| 1 | 7 | 7 | 7 | 5 | 9 | 7 | 7 | 6 | 7 | 9 | 8 | – | 7 | 7 | 4 | 7 | 6 | 6 | 6 | 6 | 7 |
| 2 | 9 | 8 | 9 | 6 | 10 | 11 | 7 | 7 | 7 | 8 | 9 | 10 | 8 | 8 | 4 | 7 | 8 | 7 | 8 | 8 | 11 |
| 3 | 7 | 7 | 8 | 5 | 8 | 11 | 7 | 7 | 7 | 8 | 8 | – | 8 | 9 | 4 | 7 | 8 | 7 | 8 | 8 | 11 |

| `C` row | 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 10 | 11 | 12 | 13 | 14 | 15 | 16 | 17 | 18 | 19 | 20 |
| ---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|--- |
| 0 | 8 | 7 | 8 | 5 | 6 | 11 | 5 | 6 | 6 | 4 | 6 | 10 | 3 | 5 | 3 | 7 | 5 | 7 | 8 | 6 | 11 |
| 1 | 8 | 7 | 8 | 5 | 6 | 11 | 5 | 6 | 6 | 3 | 5 | – | 3 | 5 | 3 | 7 | 5 | 7 | 8 | 6 | 11 |
| 2 | 8 | 7 | 8 | 5 | 6 | 11 | 5 | 6 | 6 | 3 | 6 | 10 | 3 | 5 | 3 | 7 | 6 | 7 | 8 | 6 | 11 |
| 3 | 8 | 7 | 8 | 5 | 6 | 11 | 5 | 6 | 6 | 4 | 5 | – | 3 | 5 | 3 | 7 | 6 | 7 | 8 | 6 | 11 |

| `D` row | 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 10 | 11 | 12 | 13 | 14 | 15 | 16 | 17 | 18 | 19 | 20 |
| ---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|--- |
| 0 | 8 | 8 | 8 | 5 | 6 | 3 | 5 | 3 | 7 | 7 | 7 | 11 | 7 | 6 | 4 | 7 | 6 | 6 | 6 | 6 | 3 |
| 1 | 7 | 8 | 5 | 4 | 5 | 3 | 5 | 3 | 7 | 5 | 6 | – | 7 | 6 | 3 | 7 | 6 | 6 | 6 | 6 | 3 |
| 2 | 8 | 9 | 8 | 5 | 6 | 8 | 5 | 4 | 7 | 8 | 8 | 11 | 8 | 7 | 5 | 8 | 6 | 7 | 6 | 7 | 8 |
| 3 | 7 | 9 | 8 | 5 | 6 | 8 | 5 | 4 | 7 | 5 | 7 | – | 7 | 7 | 3 | 8 | 6 | 7 | 6 | 7 | 8 |

| `G` row | 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 10 | 11 | 12 | 13 | 14 | 15 | 16 | 17 | 18 | 19 | 20 |
| ---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|--- |
| 0 | 9 | 9 | 8 | 5 | 7 | 6 | 6 | 6 | 6 | 7 | 7 | 9 | 5 | 7 | 4 | 7 | 7 | 6 | 7 | 6 | 6 |
| 1 | 7 | 7 | 7 | 5 | 6 | 6 | 6 | 6 | 6 | 6 | 6 | – | 4 | 7 | 4 | 7 | 7 | 6 | 7 | 6 | 6 |
| 2 | 10 | 9 | 7 | 5 | 7 | 11 | 6 | 6 | 6 | 8 | 7 | 9 | 6 | 7 | 5 | 8 | 8 | 7 | 9 | 8 | 11 |
| 3 | 7 | 6 | 6 | 5 | 6 | 11 | 6 | 6 | 6 | 7 | 6 | – | 7 | 8 | 3 | 8 | 8 | 7 | 9 | 8 | 11 |

| `J` row | 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 10 | 11 | 12 | 13 | 14 | 15 | 16 | 17 | 18 | 19 | 20 |
| ---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|--- |
| 0 | 8 | 7 | 8 | 5 | 6 | 11 | 5 | 6 | 6 | 6 | 5 | 11 | 2 | 5 | 3 | 7 | 6 | 7 | 6 | 6 | 11 |
| 1 | 8 | 7 | 8 | 5 | 6 | 11 | 5 | 6 | 6 | 4 | 5 | – | 2 | 4 | 3 | 7 | 6 | 7 | 4 | 6 | 11 |
| 2 | 8 | 7 | 8 | 5 | 6 | 11 | 5 | 6 | 6 | 6 | 8 | 11 | 2 | 6 | 3 | 7 | 6 | 7 | 6 | 6 | 11 |
| 3 | 8 | 7 | 8 | 5 | 6 | 11 | 5 | 6 | 6 | 6 | 6 | – | 2 | 5 | 2 | 7 | 6 | 7 | 5 | 6 | 11 |

| `K` row | 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 10 | 11 | 12 | 13 | 14 | 15 | 16 | 17 | 18 | 19 | 20 |
| ---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|--- |
| 0 | 8 | 7 | 8 | 5 | 7 | 7 | 4 | 8 | 8 | 7 | 7 | 11 | 6 | 7 | 6 | 7 | 7 | 7 | 8 | 10 | 7 |
| 1 | 7 | 7 | 6 | 5 | 6 | 7 | 4 | 8 | 8 | 6 | 6 | – | 5 | 8 | 4 | 7 | 7 | 7 | 8 | 10 | 7 |
| 2 | 9 | 8 | 7 | 7 | 8 | 11 | 5 | 9 | 9 | 6 | 8 | 11 | 6 | 8 | 5 | 7 | 8 | 8 | 8 | 11 | 11 |
| 3 | 8 | 7 | 8 | 5 | 8 | 11 | 5 | 9 | 9 | 6 | 7 | – | 6 | 8 | 5 | 7 | 8 | 8 | 8 | 11 | 11 |

| `P` row | 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 10 | 11 | 12 | 13 | 14 | 15 | 16 | 17 | 18 | 19 | 20 |
| ---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|--- |
| 0 | 10 | 9 | 8 | 5 | 8 | 8 | 5 | 9 | 9 | 8 | 8 | 10 | 6 | 10 | 6 | 8 | 6 | 9 | 7 | 10 | 8 |
| 1 | 8 | 7 | 8 | 5 | 7 | 8 | 5 | 9 | 9 | 8 | 9 | – | 7 | 10 | 6 | 8 | 6 | 9 | 7 | 10 | 8 |
| 2 | 10 | 9 | 9 | 6 | 8 | 12 | 6 | 9 | 10 | 9 | 8 | 10 | 7 | 9 | 7 | 10 | 7 | 10 | 7 | 10 | 12 |
| 3 | 9 | 7 | 9 | 6 | 7 | 12 | 6 | 9 | 10 | 7 | 8 | – | 8 | 10 | 7 | 10 | 7 | 10 | 7 | 10 | 12 |

| `T` row | 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 10 | 11 | 12 | 13 | 14 | 15 | 16 | 17 | 18 | 19 | 20 |
| ---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|--- |
| 0 | 8 | 7 | 8 | 6 | 6 | 6 | 4 | 4 | 6 | 7 | 6 | 10 | 6 | 6 | 5 | 5 | 6 | 6 | 8 | 5 | 6 |
| 1 | 6 | 7 | 7 | 5 | 5 | 6 | 4 | 4 | 6 | 5 | 6 | – | 6 | 7 | 5 | 5 | 6 | 6 | 8 | 5 | 6 |
| 2 | 7 | 9 | 8 | 6 | 6 | 10 | 5 | 4 | 6 | 8 | 6 | 10 | 6 | 7 | 5 | 5 | 6 | 5 | 11 | 6 | 10 |
| 3 | 8 | 7 | 8 | 5 | 6 | 10 | 5 | 4 | 6 | 7 | 6 | – | 6 | 8 | 5 | 5 | 6 | 5 | 11 | 6 | 10 |

**The release frames**, by row and group; `–` marks the places no stop comes to, after a space and not at its word's start.

| `P` row | 0 | 1 | 2 | 3 | 4 | 5 | 6 |
| ---|---|---|---|---|---|---|--- |
| 0 | 5 | 3 | 2 | 3 | 3 | 1 | 4 |
| 1 | 5 | 3 | 3 | 3 | 3 | 1 | – |
| 2 | 5 | 3 | 2 | 2 | 3 | 2 | 4 |
| 3 | 3 | 3 | 2 | 2 | 3 | 2 | – |
| 4 | 1 | 2 | 1 | 2 | 3 | 1 | 3 |
| 5 | 2 | 2 | 2 | 2 | 3 | 1 | – |
| 6 | 5 | 2 | 2 | 2 | 2 | 2 | 3 |
| 7 | 2 | 2 | 2 | 2 | 3 | 2 | – |

| `T` row | 0 | 1 | 2 | 3 | 4 | 5 | 6 |
| ---|---|---|---|---|---|---|--- |
| 0 | 5 | 4 | 4 | 4 | 7 | 1 | 4 |
| 1 | 5 | 5 | 4 | 4 | 3 | 1 | – |
| 2 | 6 | 5 | 4 | 3 | 7 | 1 | 4 |
| 3 | 5 | 5 | 4 | 3 | 9 | 1 | – |
| 4 | 4 | 2 | 2 | 2 | 3 | 1 | 2 |
| 5 | 3 | 4 | 2 | 2 | 3 | 1 | – |
| 6 | 5 | 3 | 3 | 2 | 3 | 1 | 2 |
| 7 | 4 | 2 | 2 | 2 | 3 | 1 | – |

| `K` row | 0 | 1 | 2 | 3 | 4 | 5 | 6 |
| ---|---|---|---|---|---|---|--- |
| 0 | 4 | 4 | 4 | 3 | 5 | 1 | 5 |
| 1 | 5 | 5 | 4 | 3 | 3 | 1 | – |
| 2 | 5 | 5 | 4 | 3 | 3 | 1 | 5 |
| 3 | 5 | 4 | 5 | 3 | 3 | 1 | – |
| 4 | 3 | 3 | 4 | 3 | 3 | 1 | 3 |
| 5 | 3 | 4 | 3 | 2 | 3 | 1 | – |
| 6 | 3 | 3 | 4 | 2 | 3 | 1 | 3 |
| 7 | 3 | 3 | 3 | 2 | 3 | 1 | – |

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
- Dead code left out of the port (section 2.2): the original's scan can also find a phoneme, or stop at one, by its stress (3, or 2 and 3) instead of its class; every scan of the stage is by class.
- A phoneme whose inherent duration is below its shortest (section 6.2, step 3): no symbol's is.
- For `P`, `T` and `K` geminates after a vowel before a sound that isn't one (section 6.3), the original also tests whether the vowel is a nasal (8 frames for `P`, 5 for `T`, 9 for `K`) or a consonant (9). No symbol is both, so the port leaves the test out.
- The walk back for a function word's mark (section 6.3) reaching the list's start: every word has a mark.
- Dead code left out of the port (section 6.4): the original also takes `D` then `T` for a geminate, a pair that never merges, and makes a merged phoneme stressed when its context says it is, which it already is.
- A front-ending vowel before a vowel (section 6.7, step 13): the lexical stage puts a `Y` between them.
- An unstressed `i` before a nasal or a lateral (section 6.7, steps 11 and 12): the lexical stage writes phonetic input's unstressed `i` as `|`, and no word gave one.
- A `u` of class 10 before `S` and `P`, `T` or `K` (section 6.7, step 21): the lexical stage writes `S P`, `S T` and `S K` as `S B`, `S D` and `S G`. Nor did a class 1 `E` come before a flap (step 20): no word starts with one.
- A word mark right after a comma (section 6.8): the stage puts the `P` command of the phrase the break starts between them (section 3.3), and a word command comes between a comma of the text and the word after it.
- A grid's byte of 0 where section 6.7, step 15 looks, so a vowel's inherent or shortest duration: the vowels' grids have no 0. Their step 15 is reached only by a 0 from step 10, in column −1, or from steps 11 to 14.
- A vowel with no grid (section 6.7): every symbol in class vowel has one; the port takes grid 0.
- The search for another syllable (section 6.7, step 6) with no next phoneme after the next node, and rules whose nodes run out at the list's start or the window's end (section 6.8): the port takes them as no syllable and a failed rule.
- A vowel of a word class of 16 or more (section 6.8): only a function word after phonetic input's word mark `#` has one, class 59 ([lexical.md](lexical.md) §13). The original reads the class's rules past the end of their table, and faults; the port gives it no rules.
- A phrase without a break up to 16 phonemes after a function word's vowel (section 6.8): the original then tests a byte it never set for `?`; in the 142 tests of a recorded rule's condition 16 it wasn't, and the port takes such a phrase as not a question.
- Dead code left out of the port (sections 6.7 and 6.8): the original also takes a flap whose byte 0x18 is `E` as `D`, but nothing writes `E` there (the lexical stage writes `T` and `D`); and it steps back for a test of direction −1, but tests the direction as a byte against −1 as a 32-bit number, which never matches, so such a test would look at the vowel's node. No rule has one; the data pack's tests have directions 0 and 1.
- An `n` inside its word after `q`, outside the last word (section 6.9, step 7): its 10 frames last only when its following is a space, as step 12 gives it 9 otherwise, and the `n`s inside their words had a phoneme after them.
- A grid's byte of 0 for a stressed sonorant (section 6.9, step 8), and so a sonorant's inherent or shortest duration: the stressed rows hold 0 only at a word's end, where a stressed sonorant came only as an `l`, `m` or `n` before a break, in column 4, which holds none there; and no sonorant's grid has 0 in column 1 of row 4. The 46 sonorants that reached step 8 were all unstressed `~`s.
- `H`, `Q` or `m` before a sonorant, `Q` after one, and `l` before `D` and a full stop (section 6.9, steps 4 and 13): the lexical stage leaves out an `H` before a consonant or at a word's end, writes phonetic input's `Q` as `q` inside a word and as a pause at its start or end, keeps `m` only before a break, where a pause follows it, and writes an `l` before `D` as `@ j`.
- Dead code left out of the port (section 6.9): the original also gives an `n` at its word's end after `q`, outside the last word, 9 frames before a vowel, which step 12 gives it anyway.
- An `S` in the last word by a space of value above 20 two nodes after it (section 6.10, step 2): the pauses after a recorded fricative had value 0 when it was timed.
- Contexts where step 1 of section 6.10 makes no difference: an `S` that sees a `Z` past a pause is unstressed at its word's end outside the last word, where its grid holds 9 in both column 3 and column 15; the frication of `J` takes column 5's exception either way, which gives the same inside its word and at its end unless the frication is stressed, and no frication before such a pause was. No plain `s` or `z` came before a pause and a nasal or `L`: within a word the lexical stage puts such a pause only after `S`, `Z` and the frications of `C` and `J`.
- `Q`, `H` or `m` before a fricative, and `Q` or `m` after one (section 6.10, step 4): the lexical stage writes phonetic input's `Q` as `q` or as a pause, leaves an `H` before a consonant out, and keeps `m` only before a break. Nor did a fricative come before `p` (step 3, and row 6 of columns 1 and 19): the lexical stage puts `p` only after a stop; before a comma the next is the comma, in class boundary.
- A fricative stressed at its word's end: every recorded stressed fricative began a stressed syllable. So rows 0 and 2 of the fixed durations (section 6.10), the grids' bytes of 0 for a stressed fricative (step 8: inside a word the stressed rows hold 0 only in columns 2 and 18, which a fricative inside its word never takes), and the 0 the exceptions of columns 5, 11 and 12 give a stressed fricative at its word's end.
- The fixed durations' other places no recorded fricative reached: of their 610, the recorded runs reached 394; besides the 115 of rows 0 and 2 and the 8 before `p`, 77 of `H`, which came only before a vowel and never between vowels, after a glide, or after `B D G J Q q t` inside its word, and 16 of a stressed `d` before a consonant.
- Dead code left out of the port (section 6.10): the original also gives the frication of `C` durations in columns 1, 5, 7, 9, 11, 12, 14 to 16, 19 and column 3 by its following, and the frication of `J` in columns 7 to 9, 11, 12, 14, 15, column 3 by its following, and in column 2 after `z`'s, where they never come: the frication of `C` reaches only columns 2, 3 and 4, that of `J` only columns 2, 5 and 6. It changes the frication of `C`'s value in column 4 too, where its grid's byte replaces it; and it gives a fricative in the last word at its word's end durations in columns 3 to 17, where column 2 takes it first.
- A stop after a space whose previous symbol isn't `&` or `%` (section 6.11, step 2): a stop after a space always began its word, after the word's mark.
- `C` and `J` before anything but their frications (section 6.11): the lexical stage writes them as `C s` and `J z`, so of their places in the table they reach only columns 8 (at a word's start), 9 to 14, and 16 for `C` and 18 for `J`; neither comes before `p` (step 10) or before another stop (columns 9 and 12 by the preceding). Nor does `H` come before a stop (column 10): the lexical stage leaves out an `H` before a consonant.
- `B`, `G`, `K`, `P` and `T` before `l` in their own word (column 7 in rows 0, 2 and 3): the lexical stage writes such an `l` as `@ j` or `| j`. A `T` at its word's end before a word starting with `l` becomes the flap `t`, so no `T` came before `l` at all (column 7 in row 1, and the release frames' 2). Of the table's 656 places, the recorded runs reached the other 532.
- A stop's release frames of none between vowels (section 6.11): the phrase's first node was stressed only at the mark of an emphasized word (phonetic input's `[:]`, section 3.5) that started a phrase without a `P` command, at a rate of 25 or more or before a `!` (section 3.2), and its last node only in a phrase the limit broke off; no phrase was both.
- A stop after a glide with `n` before it (section 6.11, step 9), where the before preceding being `n` decides between no frame and 1: no recorded stop came after one.
- Dead code left out of the port (section 6.11): a stop's duration of 0 after step 10, for which the original takes column 20's value when the stop isn't at its word's start and its inherent duration when it is: the table's smallest values less the most that steps 6 and 9 take leave at least 1 frame. Before step 6 the original also tests whether the stop is in class fortis, which `P` and `T` are, and before step 9 whether it is a closure and a consonant, which every stop is; in step 9 it gives 2 frames after a sound that isn't a glide with a nasal before it, which the third case has already given 1, and before giving none it tests again whether the preceding is a consonant, `l` or `n`, which step 9 already requires.
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
