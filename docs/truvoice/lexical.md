# TruVoice: the lexical stage

How L&H TruVoice (`tv_enua.dll`) turns words into phonemes. The lexical stage works on the words the normalizer hands it ([pipeline.md](pipeline.md) §3): words of letters, words of phonemes from phonetic input, and the punctuation. It converts each word into phonemes with their stress, marks the word's class, and gives the word before it a second, post-lexical pass. The prosody stage ([prosody.md](prosody.md)) takes the phonemes it hands on.

This spec covers the nodes the stage works on and the classes it tests (section 1), its step and how it finds the next word (section 2), a word's conversion (section 3), the post-lexical pass (section 4), the stage's reset (section 5), the dictionary (section 6) and the user lexicon (section 7). The stage's **rules** are the parts that give the phonemes themselves: the lookup of a word in the user lexicon, the dictionary and by its affixes, letter-to-sound, the post-lexical rules, and the function words' stress and reductions. Sections 6 and 7 say how the dictionary and the user lexicon work, and later sections will say how the others do; sections 3 and 4 say where the stage applies them.

**In short.**
- Each step converts one word, then gives the word before it its post-lexical pass and hands on the nodes up to the word just converted. A word's pass waits for the word after it, as the rules look at the next word.
- A word of letters can start with `~`, which emphasizes it, or `` ` ``, which doesn't. Its letters, between two `#`, are looked up in eleven lists of function words, which give the word its class: articles, pronouns, quantifiers, conjunctions, prepositions, numbers, possessives, the forms of be and have, demonstratives. A word ending in `'ve` gets have's class and one ending in `'s` a class of its own, but `let's`. Then the rules look the word up, or convert it letter by letter.
- A digit after a syllabic phoneme gives it its stress: `1` primary, `2` secondary, `"` emphatic. A stressed syllable's vowel and the consonants of its onset are marked, for the prosody stage and the parameter generator.
- Phonetic input's word marks can make a word a function word, a content word or a juncture, emphasize it, or make it keep the form it is written in.
- In the post-lexical pass a schwa before a front vowel in the next syllable, with at most one consonant between, becomes a barred i, and a barred i before a consonant and `l` a schwa. The vowels are numbered by syllable. The filter command can drop all words but every n-th content word.
- The dictionary has 6,494 entries: a word's phonemes with stress digits, or a stress pattern for letter-to-sound, and its class: verbs, adverbs and kinds of function word. A function word found whole becomes `%`. A homograph's verb's entry is taken or passed over by the word before it: `to record`, `the record`. Phonetic input's word mark `*` takes a word's second entry.
- The user lexicon, which an application fills, is looked up before the dictionary, for the word and for the stem a suffix leaves. Its pronunciations have a stress digit after the phoneme it stresses; without a `1`, the stress falls as the dictionary's default does. It gives the word no class.

**Exactness.** Ported from the original's code, and checked against it by replaying every call of its functions recorded in 1,357 runs: the 729 runs of the stages in the pipeline ([prosody.md](prosody.md), [parameters.md](parameters.md)), 70 more for the stage, 35 texts at two speeds: words with `~` and `` ` ``, stress digits in phonetic input, `"` among them, every word of the function words' lists and the words they treat apart, words ending in `'ve` and `'s`, numbers, single letters after a word ending in `S`, in text, spell mode and phonetic input, spelled letters, schwas before front vowels within and across words, barred i before a consonant and `l` or `j`, the onsets of stressed syllables, 13 commands between two words, runs of punctuation, every word mark of phonetic input before function and content words and before a phonetic word, the filter command at three values with words of every class it counts, the post-lexical rules switched off in phonetic input, words with affixes after a schwa, and bytes of 128 and up in phonetic input, 534 for the dictionary and 24 for the user lexicon (below). Each call left the speaking thread's state byte-identical to the original's: the 233,233 steps (section 2.1), the 119,170 conversions (section 3), the 50,209 post-lexical passes (section 4), the 1,361 resets (section 5), the 54,259 lookups in the dictionary (section 6) and the 51,226 in the user lexicon (section 7). The rules ran as the original's recorded calls: each started from the state the Go stage had reached, checked against the original's, and left what the original's left.

In the first 799 runs, the conversions took 8,572 words of letters, 92 with `~` and 8 with `` ` ``, 13,722 words of phonemes and 832 nodes of type 0; 2,983 of the words of letters went to letter-to-sound, 133 of them for their stress pattern. The lists classified 3,742 words, in every class, 292 of them by an exception, and the endings 54: 14 `'ve`, 36 `'s` and 4 `let's`. 4,648 stress digits gave 4,636 syllabic phonemes their stress, 4 of them `"`, and 1,319 were their word's last node; the onsets of stressed syllables marked 7,780 consonants, 1,349 second consonants and 76 `S`s, and 1,236 onsets in function words. The mark byte became 1 at 582 words and 2 at 608, 592 of them number words and 16 single letters after `S`. Codes 2, 3, 5 and 11 came six times each, code 1 four times and code 9 eight times. The passes skipped 979 nodes; the filter counted 65 words and dropped 60; 3,837 function words got their stress and reductions, 4,085 words gave their vowels their class, and the post-lexical rules ran on 49,894 phonemes. 1,461 schwas started the alternation, and 56 became barred i in their word; 450 times a front vowel walked back to an earlier word's schwas, 141 times joined, and 6 schwas became barred i; 18 barred i became schwas. 19,756 vowels got their syllable's number and 387 nodes `[` were freed.

The 534 runs for the dictionary speak every word in it; the homographs and `read` alone, after a word of every class the lists give, and with endings, each plural after every one of these; an article up to 10 commands before the previous word, or emphasized; `subject` before a colon; the word mark `*` before the words with two entries; the words of every class but 0, and every tenth other word, with `~` and `` ` ``; long words; and every sixth word with prefixes and suffixes. Of the 54,139 lookups in the first 1,333 runs, 41,543 were of a whole word, 9,417 of a stem before a suffix, 1,195 after a prefix and 1,984 between both; 8 stems had more than 20 letters and 83 more than their letter's lists. 35,135 stems were found, in every class: 27,285 entries with phonemes and 7,850 with a stress pattern. 14,053 headers became `%`, 18,368 words got the entry's class, and 780 emphasized words with a class got neither. 3,838 digits `1` gave the primary stress, 283 times 3 and 279 times 1, and 462 digits `2` stress 1; 23,507 entries got the default stress, 755 of them on a reduced vowel. The word mark `*` passed 646 entries over and started the search again 20 times. The homographs' test met 1,440 verbs' entries that aren't homographs and 852 stems before `ING`, `ED` or `D`; the previous word decided 6,293 times, after every class of the lists, with a plural and without, and 124 homographs had none; the word before it was tested 1,749 times, 251 of them an article; `subject` before a colon was passed over 3 times, and of 40 tests of `read`, 11 passed its first entry over.

The 24 runs for the user lexicon give it 38 words and speak 10 texts with them, at two modes and speeds: stress digits after full and reduced vowels and after consonants, first, doubled and last; pronunciations without a `1`, with only reduced vowels and without vowels; the phonemes `3`, `4`, `5`, `@`, `|`, `~` and `&`; words given in lower case and in capitals, words the dictionary has, a letter, which spelled words look up, a word of 29 letters and words of 40 and 41, which reach the stage in pieces; the words with suffixes, `~`, `` ` `` and `\Emp\`, in spell mode, between commands, in the filter command's words and after phonetic input's word marks. 4 more runs have lexicons of their own: pronunciations of 39 to 87 symbols, empty ones, ones of only digits and ones with `&`. Of their 366 lookups in the user lexicon, 250 found the stem: 212 a whole word and 38 a stem before a suffix, 20 of an emphasized word and 22 of a de-emphasized one. 162 digits `1` gave the primary stress, 15 of them to a consonant, 30 digits `2` stress 1, and 11 digits were skipped; the default stress fell 50 times on a full vowel and 26 times on a reduced one, and 6 pronunciations found had no vowel and 8 no phoneme. The other runs have no user lexicon: their 50,860 lookups found nothing.

## 1. Nodes and classes

### 1.1 The words

The stage's window ([pipeline.md](pipeline.md) §3) holds nodes of types 2 and 3, and control commands. The normalizer hands it:

- a **word of letters**: a header of type 2, the word mark `&`, and the letters, nodes of type 2 whose symbols are `A` to `Z`, `@` for an apostrophe, and `~` or `` ` `` before the first;
- a **word of phonemes**, from phonetic input: a header of type 3, the word mark `&`, and the phonemes, nodes of type 3 with digits `1`, `2` or `"` after the stressed vowels;
- **punctuation** and other nodes of type 3, each a word of one node.

A header's **code** is its value when its flag bit 0x20 is set; the normalizer sets it from phonetic input's word marks (section 3.6). A header's **mark byte**, byte 0x14, is 1 at a phrase's start and between spelled letters and 2 in numbers ([parameters.md](parameters.md) §8); the stage sets it too (section 3.1). The pipeline's **number word** flag is set by the normalizer when it writes a digit's, a teen's or a ten's word.

The stage makes every word's nodes type 3. A header becomes the word's mark: `&` before a content word, `%` before a function word (the rules decide), or `$`, or a space, by its code. Its flag bits 8 to 15 are the word's **class** (section 3.4), and its second value the word's class in the dictionary. A phoneme's flag bits 3 and 4 are its **stress**, 0 to 3, and flag bit 0x20 marks a stressed syllable's nucleus and onset; its byte 0x17 is its syllable's number in its word, and byte 0x19 a function word's class in the dictionary.

### 1.2 Classes

The stage tests the symbols' classes ([parameters.md](parameters.md) §3.2). Symbols of 128 and up never reach the stage (section 8); they are in no class.

| Class | Name here | Symbols |
|---|---|---|
| (0, 0x01) | syllabic | the vowels but `p`, and `l m n` |
| (0, 0x02) | sonorant | the vowels, `p`, `j l L W Y R`, `d h H`, the nasals `M m N n ~` |
| (0, 0x08) | word mark | `% & [` |
| (0, 0x80) | phoneme | every phoneme and the pause, a space |
| (1, 0x01) | boundary | `! , . ? \ ]` |
| (1, 0x02) | stress mark | `" 1 2` |
| (1, 0x20) | consonant | `B C D F G H J K L M N P Q R S T V W X Z d h j q s t x z ~` |
| (1, 0x40) | break | `! ) , . ? \ ]` |
| (2, 0x01) | closure | `B C D G J K M N P Q T m n q t ~` |
| (2, 0x02) | vowel | the vowels and `p` |
| (2, 0x20) | front | `4 A E U a e i k \|` |
| (4, 0x40) | reduced | `3 @ l m n \|` |

The vowels are `3 4 5 @ A E I O U a b c e f g i k o r u v w y |`. A word mark is `&` or `%` where the spec says so, and the class word mark where it names the class.

## 2. The step

### 2.1 The stage's step

The stage keeps a **current word** (its header, its first letter and its last node) and a **previous word** (its header and its last node), the word converted before.

1. Begin the stage's cursor ([pipeline.md](pipeline.md) §3.2).
2. If the cursor's first node is its done node, no word is converted ahead: convert the word at the first node (section 3). If there is none, go to step 5. Otherwise move the words on (section 2.3).
3. Find the next word (section 2.2). If there is none: while the done node is in class break, the **previous symbol** becomes 0 and the done node moves to its next node; if that leaves the window, go to step 5. Then go to step 5.
4. Convert the next word (section 3) and give the previous word its post-lexical pass (section 4). The node the pass returns becomes the cursor's done and scan nodes, the node after the current word's last node its first node. Move the words on.
5. Finish the cursor ([pipeline.md](pipeline.md) §3.3), and report whether it handed nodes on.

**Moving the words on**: the current word becomes the previous word, its last node the previous word's; the search starts at that node, with no nodes passed; the current word becomes none.

### 2.2 The next word

The search moves from the node it has reached. If that is the window's last node, the cursor's first node becomes it, and there is no next word. Otherwise, repeatedly: the search moves to the next node. It has found a word when the node is of type 2 or 3; otherwise it counts the node, and has found a word when it has counted more than 11 or the node is the control command `C`. Otherwise, when the node is the window's last, the cursor's first node becomes it and there is no next word. When it has found a word, the cursor's first node becomes the node.

### 2.3 The state

| Value | Meaning |
|---|---|
| previous word, previous end | The previous word's header and last node |
| word, first letter, last node | The current word's header, first letter and last node |
| search, searched | The node the search for the next word has reached, and the nodes it has counted |
| emphasized, de-emphasized | Whether the current word is emphasized, and whether it isn't (section 3.3) |
| stress pattern, word class, affix stress | What the rules found for the current word: its stress pattern and class in the dictionary, and the stress its affix gives |
| filtered | The words the filter has counted (section 4.1) |
| alternating | Whether a schwa may still become a barred i (section 4.3) |
| previous symbol | The last symbol of the word before, which the post-lexical rules read |

## 3. A word's conversion

There is no word when the cursor's first node is none. Otherwise its first node is the word's header, and the word's last node too, for now.

### 3.1 The mark byte and number words

1. If the node after the header in the window has mark byte 1, clear it. Then skip the nodes after it that are in class stress mark or have mark byte 1; if that runs past the window's last node, the header's mark byte becomes 1.
2. The header's mark byte becomes 2, and the number word flag is cleared, when the number word flag is set, or when the header is in class word mark, the node before it in the list is `S`, and the word is a single letter `A` to `Z`, or the letters `A Y`, followed in the list by a space or a node in class boundary.

### 3.2 By the header's type

- A word of phonemes, of type 3: the word's last node becomes the last of the nodes after the header, in the window, of type 3 and in class phoneme or stress mark. Then stress (section 3.5).
- A word of letters, of type 2: section 3.3, then stress (section 3.5).
- A word of any other type is converted as it is.

The stage reports that a word was converted.

### 3.3 A word of letters

1. If the node after the header is `~`, free it and the word is emphasized; otherwise it isn't. Then if the next node is `` ` ``, free it and the word is de-emphasized; otherwise it isn't. A header of value 1, 2 or 5 makes the word de-emphasized, one of value 9 emphasized, whether or not the value is a code.
2. The header becomes type 3: its flag bit 2 is cleared and bits 0 and 1 are set.
3. The word's first letter is the next node. Its last node is the last of the nodes from there, in the window, of type 2, up to a `&`.
4. The stress pattern, the word class and the affix stress become 0.
5. If the word has at most 96 letters, classify it (section 3.4).
6. The rules look the word up. If they don't find it, or the stress pattern isn't 0, they convert it by letter-to-sound.
7. The header's second value becomes the word class.

### 3.4 The word's class

The word's **spelling** is `#`, its letters' symbols from the first letter to the last node, and `#`, read as a C string: it ends at a symbol 0. The header's flag bits 8 to 15 are cleared. Then the lists of function words are searched in order, and the first that has the spelling anywhere in it gives the header its class, unless the spelling is one of the list's exceptions, which has a class of its own:

| Class | List |
|---|---|
| 0x01 | articles: `a`, `an`, `the` |
| 0x12 | `he`, `she`, `it` |
| 0x03 | quantifiers: `such`, `some`, `each`, `every`… |
| 0x13 | quantifiers: `few`, `many`, `all`, `several`, `both` |
| 0x04 | conjunctions and question words: `and`, `or`, `if`, `when`, `who`…; `that` 0x14, `which` 0x24 |
| 0x05 | prepositions: `in`, `from`, `with`, `of`…; `to` 0x15 |
| 0x06 | numbers: `zero` to `twelve`, `hundred`… `first`, `last`, `next`; `one` 0x16 |
| 0x08 | possessives: `our`, `his`, `my`…, and `new`, `old` |
| 0x0c | the forms of be |
| 0x0d | the forms of have |
| 0x1f | demonstratives: `these`, `those`; `this` 0x0f |

The lists are words between two `#`, so the search finds a whole word. The data pack holds them ([datapack.md](datapack.md)).

When no list has the spelling, each of the endings is tested in turn: a spelling with at least one letter before the ending, and the ending right before its last `#`, gets the ending's class, unless it is one of its exceptions. The endings are `@VE` (`'ve`), class 0x0d, and `@S` (`'s`), class 0x0e, but `#LET@S#`, which keeps class 0.

### 3.5 Stress

From the header to the last node, each node in turn:

1. If the node isn't the word's last node and the node after it in the window is `1`, `2` or `"`: if the node is syllabic, its stress becomes 2, 1 or 3. The digit is freed, and if it was the word's last node, the node becomes it.
2. Unless the header is `%`, a node with a stress other than 0 marks its onset (below).

Then the word's code (section 3.6).

**Marking an onset.** The node gets flag bit 0x20. If the phoneme before it in the window, the nearest node of type 3 or 4, is a consonant or `Y`, but not `~`, it gets flag bit 0x20 too. Then if the phoneme before that is a consonant, not a sonorant, and not a closure before a closure, it gets the flag, and so does an `S` right before it, as the phoneme before.

### 3.6 The word's code

The normalizer gives a header the code of the word mark phonetic input puts before it, in its second mode ([prosody.md](prosody.md) §3.5): the symbol right before the `]` gives the code, `*` 4, `\` 6, `/` 7, `/\` 8, `:` 10, `{` 12 and `_` 17 among them, which are for the stages after this one. The stage acts on these:

| Code | Symbol | What the stage does |
|---|---|---|
| 1 | `=` | The header becomes `$`, unless the cursor's first node is its done node |
| 2 | `` ` `` | The header becomes `%`, its second value 1 |
| 3 | `'` | The header becomes `&`, its second value 0 |
| 5 | `-` | The header becomes a space, value 4 and second value 59 |
| 9 | `!` | The word is emphasized |
| 11 | `#` | A space of type 3 and value 8 comes after the header; the header's second value becomes 59 |
| 13 | `$` | The post-lexical pass leaves the word as converted: no post-lexical rules, and for a function word no stress or reductions (section 4.2) |

A function word after `#` makes the original fault (section 8).

## 4. The post-lexical pass

The pass works on the previous word, from its header to the node after its last node, the pass's **end**, which it returns.

### 4.1 Words the pass skips or drops

1. If the previous word's header isn't of type 3, or is in class break, the pass returns the node after it.
2. If the cursor's filter setting ([pipeline.md](pipeline.md) §3.1) isn't 0: a word whose second value is 0, 6, 8 or 15 counts, and filtered becomes filtered plus 1, modulo the filter. The word is kept when it counts and filtered is then 0, and dropped otherwise. A dropped word's nodes are freed, from its header to its last node, and the pass returns the node after them in the window, or none at the window's end. The previous word stays the freed last node.

### 4.2 The phonemes

1. A function word, with the header `%`, that doesn't have code 13 gets the function words' stress from the rules.
2. A function word, or a word of second value 15, gives each of its vowels the word's second value, as byte 0x19.
3. Each node from the header to the end, in the window, in turn, stopping at the window's end:
   1. A node of type 3 in class phoneme gets the post-lexical rules, which return the node to go on with, when the cursor's flag 0x100 is set or its phonetic input setting is 0, and the word's header doesn't have code 13.
   2. Schwas and barred i, section 4.3.
   3. A barred i `|` before a consonant followed by `l` or `j`, in the list, becomes a schwa `@`.
   4. In a function word, a node with a stress other than 0 marks its onset (section 3.5).
4. Each node from the header to the end in the window: a vowel gets the next syllable number, from 1, as byte 0x17. A node `[` is freed.
5. A function word without code 13 gets the function words' reductions from the rules.
6. The previous symbol becomes the symbol of the node before the end, sign-extended.

### 4.3 Schwas and barred i

The pass keeps the **schwa** that started the alternation, none at the pass's start. While **alternating** is set, a node in class vowel:

1. if it isn't front, or is `a`: unless it is `@`, alternating is cleared;
2. otherwise, if there is no schwa, the schwa that started the alternation was in an earlier word: if the current word's header is in class break, the word is **joined**. Walk back from the node before the vowel, along the list, until a node in class break or the list's start:
   - a word mark `&` or `%` counts a word; at more than one the word isn't joined;
   - another node that isn't a vowel counts a consonant if it is in class consonant;
   - a `@`: if the word is joined and one word has been counted, it isn't joined any more; otherwise the `@` becomes `|` unless the node after the next is a space followed by `|`, more than one consonant was counted, the node after it or before it is a word mark, or the node after the next isn't a vowel. The consonants' count becomes 0 either way;
   - any other vowel clears alternating and the consonants' count, and ends the walk.

   Then alternating is cleared. The counts of words and consonants and joined carry over from one walk to the next within a pass.
3. otherwise, if the schwa is `@` and the node before it isn't a word mark: the `@` becomes `|` unless the two nodes after it, or after a `[` right after it, are both consonants. Alternating is cleared.
4. otherwise alternating is cleared.

When alternating is clear, a node `@` starts the alternation, as the schwa, unless the node before it, or the one before that, is in class break, or the one before that has no node before it in the list. Alternating carries over to the next word's pass.

## 5. The reset

At the pipeline's reset ([pipeline.md](pipeline.md) §7), the current and previous words become none, filtered and the previous symbol 0, and alternating is cleared. The search, the emphasis and what the rules found keep their values. The cursor's types are 2 and 3 (the pipeline sets them).

## 6. The dictionary

The rules' lookup (section 3.3, step 6) looks the word up in the user lexicon and the dictionary, then takes off its affixes and looks up what they leave. The lookup in the dictionary works on a **stem**: the nodes from the cursor's first working node to its second ([pipeline.md](pipeline.md) §3), the word's letters from its first letter to its last node, or what its affixes leave of them. The data pack holds the dictionary ([datapack.md](datapack.md) §9).

### 6.1 The entries

The dictionary has 6,494 entries for 6,334 words. An entry has a **word**, a letter `A` to `Z` and up to 20 more of `A` to `Z` and `@`; a **class**, 0 to 15; and either **phonemes**, up to 15 symbols with their stress digits, or a **stress pattern**, 0 to 255, for letter-to-sound. 4,747 entries have phonemes and 1,747 a stress pattern.

| Class | Words |
|---|---|
| 0 | the rest |
| 1 | pronouns and their forms: `he`, `it's`, `myself`, `anyone`…, and a few others: `much`, `per`, `so` |
| 2 | `be`, `can`, `have`, `the` |
| 3 | `a`, `an`, `am`, `for`, `from`, `of`, `shall` |
| 4 | prepositions: `about`, `after`, `before`, `without`… |
| 5 | conjunctions and linking adverbs: `because`, `if`, `that`, `therefore`… |
| 6 | verbs, 447 |
| 7 | quantifiers: `all`, `any`, `both`, `many`… |
| 8 | negations: `not`, `never`, `can't`… |
| 9 | `do`, `did`, `does` |
| 10 | prepositions: `as`, `at`, `by`, `in`, `to`… |
| 11 | `may`, `must`, `might` |
| 12 | auxiliaries: `are`, `been`, `could`, `has`, `will`… |
| 13 | question words: `how`, `who`, `what`… |
| 14 | `and`, `but`, `nor`, `or` |
| 15 | adverbs: `early`, `only`, `perhaps`… |

The entries are in lists by their first letter and their number of letters, each list in the order the lookup searches it. 160 words have two entries: 116 homographs, whose verb's entry, of class 6, comes first but for `frequent` (section 6.3), the past tense word `read`, whose second entry is its past tense, and 43 words with a second way to say them, `data`, `minute`, `Paris`…, which only the word mark `*` reaches (section 6.2).

### 6.2 The search

1. The stem's **letters** are its nodes after the first. With more than 20, the stem isn't found. Its first node must be a letter `A` to `Z`; otherwise the stem isn't found.
2. The list of the stem's first letter and number of letters is searched in order. An entry **fits** when its letters are the stem's, compared by their five low bits: `@` is 0 and the letters 1 to 26.
3. At an entry that fits: if the word's header has code 4, from phonetic input's word mark `*` (section 3.6), the code is cleared (flag bit 0x20), the entry is remembered and the search goes on. Otherwise, a verb's entry goes through the homographs' test (section 6.3), which can pass over it: the search goes on. Otherwise the entry is taken (section 6.4), and the stem is found.
4. If the search ends with an entry remembered, it starts again at that entry, which the code no longer passes over. If it ends again, the stem isn't found.

The word mark `*` so gives a word its second entry, or its only one.

### 6.3 Homographs

A verb's entry is taken unless the words around the stem make it a noun, or a past tense. The stem's **spelling** is `#`, its symbols, `#`.

1. If the stem ends at the word's last node: when its spelling is the past tense word, `#READ#`, there is a previous word, and the past tense word wasn't tested before in this lookup, the entry is passed over after a form of be or have (class 0x0c or 0x0d in the low four bits, section 3.4), or after `he`, `she` or `it` (class 0x12).
   If the stem ends before the word's last node, its **ending** is the symbols after it up to the word's last node, but `[`, at most 30. After `ING`, `ED` or `D`, the entry is taken. After `S` or `ES`, the stem is **plural**.
2. Unless the list of homographs has the spelling, the entry is taken. The list has 116 words, from `ally`, `abuse` and `affix` to `wind` and `wound`.
3. `#SUBJECT#` before a colon, a node `:` of type 1 right after the word's last node in the list, is passed over.
4. With no previous word, the entry is taken. Otherwise the previous word's class (section 3.4) decides, its low four bits the list and its high four the subclass:

| Previous word | Class, low bits | The verb's entry is taken |
|---|---|---|
| articles, possessives, forms of be, words ending in `'s` | 0x01, 0x08, 0x0c, 0x0e | never |
| quantifiers, prepositions, demonstratives | 0x03, 0x05, 0x0f | with subclass 1, `few`, `many`…, `to`, `these` and `those`, when the stem isn't plural |
| numbers | 0x06 | with subclass 1, `one`, when the stem is plural |
| conjunctions | 0x04 | with subclass 0; with subclass 1, `that`, when the stem is plural; never with subclass 2, `which` |
| other words: content words, `he`, `she`, `it`, forms of have | | unless the word before the previous word is an article, class 0x01 in the low bits: the nearest header `&` or `%` among the 8 nodes before the previous word's header, before a node in class break |

So `to record` takes the verb's entry and `the record` the noun's, `one records` the verb's and `all records` the noun's.

### 6.4 Taking an entry

1. If the entry's class isn't 0 and the word isn't emphasized: if the stem is the whole word, from its first letter to its last node, and the class is neither 6 nor 15, the header becomes `%`, a function word; and if the word class is 0, it becomes the entry's class.
2. An entry without phonemes gives the word its stress pattern, and the lookup ends; letter-to-sound converts the word (section 3.3, step 6).
3. Otherwise the stem's letters are freed, and the entry's phonemes, nodes of type 3, take their place. The working nodes become the first and the last phoneme, and so do the word's first letter and last node when the stem starts or ends there.
4. A `1` gives the phoneme after it the word's **primary stress**: 3 when the word is emphasized, 1 when it is de-emphasized, else 2. A `2` gives the phoneme after it stress 1. A `0` keeps the phoneme after it from the default stress; no entry has one.
5. If the entry has no `1`, the **default stress**: the first syllabic phoneme with no digit before it gets the primary stress, the first that isn't reduced, or else the first reduced one, `3 @ l m n |`.

## 7. The user lexicon

The **user lexicon** holds words with their pronunciations, which an application gives the engine; the interface's spec will say how it adds, removes, saves and reads them. Its words are in capitals, of letters only, and its pronunciations are of the symbols `A` to `Z`, `a` to `z`, `1` to `5`, `@`, `|`, `~` and `&`. The rules' lookup (section 3.3, step 6) looks up the whole word in the user lexicon before the dictionary, and so the stem a suffix leaves; the stem a prefix leaves only in the dictionary. The lexicon can change while the engine speaks: the search and the copy of the pronunciation it finds are made under the lexicon's lock.

### 7.1 The lookup

1. The stem is the nodes from the cursor's first working node to its second, as for the dictionary (section 6), and its **word** their symbols, the first included. With more than 40, the stem isn't found.
2. The lexicon is searched for the word, symbol by symbol. If it hasn't the word, the stem isn't found. The stage's letters are capitals, so a word is found whatever the case it was given in.
3. The stem's letters are freed, and the pronunciation's phonemes, nodes of type 3, take their place: every symbol but the digits `0`, `1` and `2`, so `3`, `4` and `5` are phonemes, and `&` becomes a phoneme too. The working nodes become the first and the last phoneme, and so do the word's first letter and last node when the stem starts or ends there. The word keeps its header and its class: the user lexicon gives none.
4. A digit after a phoneme gives it its stress: a `1` the word's **primary stress**, 3 when the word is emphasized, 1 when it is de-emphasized, else 2, whether the phoneme is syllabic or not; a `2` stress 1; a `0` none, and it keeps the phoneme from the default stress. A digit after another digit, or first, is skipped. Unlike the dictionary's, these digits follow the phoneme they stress.
5. If no phoneme has a `1` after it, the **default stress**: the first syllabic phoneme with no digit after it gets the primary stress, the first that isn't reduced, or else the first reduced one. A pronunciation without a syllabic phoneme gets no stress.

A pronunciation without phonemes, empty or only digits, still frees the stem: nothing takes its place, the first working node becomes the node after the stem and the second the node before it, and the word's first letter or last node with them.

## 8. What the recorded runs don't reach

- A word of more than 96 letters, which section 3.4 doesn't classify: the input stage splits a word into pieces of 29 letters.
- A symbol 0 in a word's spelling (section 3.4), which would end it early.
- Symbols of 128 and up: the input stage maps or drops them, in text and in phonetic input.
- A header with a value but no code (section 3.6): a header without a code has value 0.
- Letters followed by a `&` of type 2 (section 3.3, step 3): a word command `z`, or another node, comes between two words of letters.
- A stress digit right after a word's last node (section 3.5, step 1): the word's last node takes in the digits after its phonemes.
- A node that isn't a phoneme between a word's phonemes, which the phoneme before a node skips (section 3.5): a command within phonetic input splits the word in two. Nor does the window hold segments, which it would take as phonemes.
- The first node of a stressed syllable, or the consonant before it, being the first phoneme in the window (section 3.5): every word's header is a phoneme node in the window before it.
- A single letter `A` after `S`, a single symbol below `A`, or a single letter followed by a space (section 3.1, step 2): a word follows a node `S` only among spelled letters, where each letter's word is followed by a full stop and `A` is spelled `A Y`, and in phonetic input, whose words of one consonant are followed by punctuation; a phonetic word has a space before it.
- A schwa that starts the alternation right after a break, or with a break or the list's start before the node before it (section 4.3): a word command `z` comes before every word's header, and a space before a phonetic word's.
- Tests of the walk back (section 4.3, case 2) that never change a schwa. A pass walks back at most once, as only a pass that starts alternating has no schwa, so the counts and joined never carry over from a walk. A schwa that becomes `|` has a vowel two nodes after it, and the walk has passed that vowel: it is the front vowel, or a schwa, where the count of consonants started again; so the count is at most 1 there, and a space two nodes after the schwa is no vowel. The first schwa the walk meets after one word mark, which joined keeps as it is, has no vowel two nodes after it: only consonants, the mark and the front vowel's word come after it.
- The pass running out of the window before its end (section 4.2, step 3), and the post-lexical rules returning no node.
- A reset of a stage that has run (section 5): the recorded resets are of a new speaking thread's state, whose values are all 0. A reset within a session drops the rest of the text, so a recorded run never ends ([pipeline.md](pipeline.md) §9).
- A function word after phonetic input's word mark `#`, code 11, which puts a space after the word's header (section 3.6): the original faults. The spec of the function words' rules will say where, and what the port does.
- A stem whose first node isn't a letter (section 6.2, step 1). The input stage gives words only letters and `@`, and an affix leaves a letter first. For a symbol after `[` the original reads past the dictionary's index; the port doesn't find the stem.
- A search that ends twice without an entry (section 6.2, step 4): the original searches again, without end. A word with the code that has only homographs' verb entries would do it, and no word has: every homograph has an entry of another class.
- An ending of more than 30 symbols (section 6.3, step 1): a word has at most 29 letters.
- An entry with a `0`, or with only digits (section 6.4): the dictionary has none, and the data pack refuses one with only digits.
- A stem of more than 40 symbols in the user lexicon's lookup (section 7.1, step 1): a word has at most 29 letters.
- A `0` in a pronunciation of the user lexicon (section 7.1, step 4): the engine takes none, from an application or a file.
- A pronunciation of 88 symbols or more in the user lexicon (section 7.1): the original copies the pronunciation it finds into a buffer of 44 bytes on its stack. Up to 87 symbols the copy overwrites only the stem's copy, which the lookup no longer needs; from 88 it overwrites the return address, and the engine crashes. An application can add one; the port takes it whole.
- A pronunciation without phonemes for a stem before a suffix (section 7.1): the original crashes after the lookup. The recorded runs have such pronunciations only for whole words, which don't crash it. A later part of this spec will say where it crashes, and what the port does.
- Dead code left out of the port: code 5 also makes the header type 3 (section 3.6), which it already is; case 1 of section 4.3 also clears the schwa, which nothing reads before the next schwa that starts the alternation sets it; and the original copies at most 97 symbols of a stem's spelling (section 6.3), when the stem has at most 21.
