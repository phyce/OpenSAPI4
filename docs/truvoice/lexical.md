# TruVoice: the lexical stage

How L&H TruVoice (`tv_enua.dll`) turns words into phonemes. The lexical stage works on the words the normalizer hands it ([pipeline.md](pipeline.md) §3): words of letters, words of phonemes from phonetic input, and the punctuation. It converts each word into phonemes with their stress, marks the word's class, and gives the word before it a second, post-lexical pass. The prosody stage ([prosody.md](prosody.md)) takes the phonemes it hands on.

This spec covers the nodes the stage works on and the classes it tests (section 1), its step and how it finds the next word (section 2), a word's conversion (section 3), the post-lexical pass (section 4), the stage's reset (section 5), the dictionary (section 6), the user lexicon (section 7), the contexts the rules test (section 8), the lookup by affixes (section 9), letter-to-sound (section 10), how it reduces its vowels (section 11), the function words' stress and reductions (section 12) and the post-lexical rules (section 13). The stage's **rules** are the parts that give the phonemes themselves: the lookup of a word in the user lexicon, the dictionary and by its affixes, letter-to-sound, the post-lexical rules, and the function words' stress and reductions. Sections 6 to 13 say how they work; sections 3 and 4 say where the stage applies them.

**In short.**
- Each step converts one word, then gives the word before it its post-lexical pass and hands on the nodes up to the word just converted. A word's pass waits for the word after it, as the rules look at the next word.
- A word of letters can start with `~`, which emphasizes it, or `` ` ``, which doesn't. Its letters, between two `#`, are looked up in eleven lists of function words, which give the word its class: articles, pronouns, quantifiers, conjunctions, prepositions, numbers, possessives, the forms of be and have, demonstratives. A word ending in `'ve` gets have's class and one ending in `'s` a class of its own, but `let's`. Then the rules look the word up, or convert it letter by letter.
- A digit after a syllabic phoneme gives it its stress: `1` primary, `2` secondary, `"` emphatic. A stressed syllable's vowel and the consonants of its onset are marked, for the prosody stage and the parameter generator.
- Phonetic input's word marks can make a word a function word, a content word or a juncture, emphasize it, or make it keep the form it is written in.
- In the post-lexical pass a schwa before a front vowel in the next syllable, with at most one consonant between, becomes a barred i, and a barred i before a consonant and `l` a schwa. The vowels are numbered by syllable. The filter command can drop all words but every n-th content word.
- The dictionary has 6,494 entries: a word's phonemes with stress digits, or a stress pattern for letter-to-sound, and its class: verbs, adverbs and kinds of function word. A function word found whole becomes `%`. A homograph's verb's entry is taken or passed over by the word before it: `to record`, `the record`. Phonetic input's word mark `*` takes a word's second entry.
- The user lexicon, which an application fills, is looked up before the dictionary, for the word and for the stem a suffix leaves. Its pronunciations have a stress digit after the phoneme it stresses; without a `1`, the stress falls as the dictionary's default does. It gives the word no class.
- A word not found whole loses its suffixes, from its end, then its prefixes, each found by its letters and a context of the letters next to it, and the stems they leave are looked up: a stem may have lost a silent E or a T, doubled its last consonant or made a Y an I, and is looked up as it was. A stem found leaves the affixes' letters to letter-to-sound. A word's first suffix can make it a verb, an adverb or a negation, and prefixes stress it.
- Letter-to-sound converts a word's letters from its last to its first by 753 rules: the first rule of a letter's list whose letters before it, context and test of the phonemes written after it hold converts the letter, with those letters, into its phonemes. A dictionary's stress pattern stresses the vowels, or the primary stress falls on the third vowel from the end, or the second when two consonants follow it, as the rules' weights move it. Each vowel is settled once the vowel before it is written: a `U` becomes `b` after a `Y`, or after `D`, `T`, `Z` or `S` made palatal, and a vowel that may be reduced becomes a schwa, a barred i or `3`, unless it is the first after a word mark and two consonants, or the like, follow it.
- A function word loses a step of its stress, or all of it above rate 8, but keeps it before a function word that ends its phrase, and before a break only `a`, `of` and the like, one-syllable pronouns and short prepositions lose it. Above rate 5, `to` before a consonant becomes `T u`, `into` `i N T @` and `and` `@ N`; above rate 6, `the` before a consonant becomes `x @`, and `a`, `an`, `of`, `for` and `from` reduce their unstressed vowels. `Used to` becomes `Y b S T`, and `has-to` `H a S`. After a quantifier, a negation and `may`, `must` and `might`, an unstressed `v` becomes a schwa, in the words after it too, and so does `them`'s `e` after any function word.
- The post-lexical rules: a word's first vowel gets a glottal stop by the sound before it and the rate, or a `Y` after the same gliding vowel, and a pause after a stressed vowel; `ex-` and `be-` before `h` change their vowel, and an unstressed `i` in a content word becomes a barred i; a schwa before `L`, and before `N` after a stop like `q`, becomes a syllabic `l` or `n`, and syllabic `l`, `m` and `n` become a vowel and a consonant before some sounds; a vowel before `R` becomes r-coloured and the `R` goes, or links it to a vowel after; a key's name said after `press` at a phrase's end gets a pause; `T` and `D` become flaps between vowels, and a `T` a glottal `q` before a consonant or `@ N`, and both go between some consonants at the faster rates; a `D` or `T` before `R` or `you` becomes `J` or `C`, which get their fricative; `P`, `T` and `K` after `S` become `B`, `D` and `G`; a word's last `L` becomes the dark `j`; `H` goes, or becomes voiced; a stop before a boundary gets a release; and some of these flag the word's mark, for the prosody stage's breaks.

**Exactness.** Ported from the original's code, and checked against it by replaying every call of its functions recorded in 1,457 runs: the 729 runs of the stages in the pipeline ([prosody.md](prosody.md), [parameters.md](parameters.md)), 70 more for the stage, 35 texts at two speeds: words with `~` and `` ` ``, stress digits in phonetic input, `"` among them, every word of the function words' lists and the words they treat apart, words ending in `'ve` and `'s`, numbers, single letters after a word ending in `S`, in text, spell mode and phonetic input, spelled letters, schwas before front vowels within and across words, barred i before a consonant and `l` or `j`, the onsets of stressed syllables, 13 commands between two words, runs of punctuation, every word mark of phonetic input before function and content words and before a phonetic word, the filter command at three values with words of every class it counts, the post-lexical rules switched off in phonetic input, words with affixes after a schwa, and bytes of 128 and up in phonetic input, 534 for the dictionary, 24 for the user lexicon, 28 for the affixes, 20 for letter-to-sound and 52 for the post-lexical pass (below). Each call left the speaking thread's state byte-identical to the original's: the 243,324 steps (section 2.1), the 126,510 conversions (section 3), the 52,232 post-lexical passes (section 4), the 1,461 resets (section 5), the 57,259 lookups in the dictionary (section 6), the 53,688 in the user lexicon (section 7), the 43,647 lookups of a word (section 9), the 18,822 conversions by letter-to-sound (section 10), the 38,508 settlings of a vowel (section 11), the stress and reductions of the 14,450 function words (section 12), and the 238,970 runs of the post-lexical rules (section 13). In the other functions' replays the post-lexical rules ran as the original's recorded calls: each started from the state the Go stage had reached, checked against the original's, and left what the original's left.

In the first 799 runs, the conversions took 8,572 words of letters, 92 with `~` and 8 with `` ` ``, 13,722 words of phonemes and 832 nodes of type 0; 2,983 of the words of letters went to letter-to-sound, 133 of them for their stress pattern. The lists classified 3,742 words, in every class, 292 of them by an exception, and the endings 54: 14 `'ve`, 36 `'s` and 4 `let's`. 4,648 stress digits gave 4,636 syllabic phonemes their stress, 4 of them `"`, and 1,319 were their word's last node; the onsets of stressed syllables marked 7,780 consonants, 1,349 second consonants and 76 `S`s, and 1,236 onsets in function words. The mark byte became 1 at 582 words and 2 at 608, 592 of them number words and 16 single letters after `S`. Codes 2, 3, 5 and 11 came six times each, code 1 four times and code 9 eight times. The passes skipped 979 nodes; the filter counted 65 words and dropped 60; 3,837 function words got their stress and reductions, 4,085 words gave their vowels their class, and the post-lexical rules ran on 49,894 phonemes. 1,461 schwas started the alternation, and 56 became barred i in their word; 450 times a front vowel walked back to an earlier word's schwas, 141 times joined, and 6 schwas became barred i; 18 barred i became schwas. 19,756 vowels got their syllable's number and 387 nodes `[` were freed.

The 534 runs for the dictionary speak every word in it; the homographs and `read` alone, after a word of every class the lists give, and with endings, each plural after every one of these; an article up to 10 commands before the previous word, or emphasized; `subject` before a colon; the word mark `*` before the words with two entries; the words of every class but 0, and every tenth other word, with `~` and `` ` ``; long words; and every sixth word with prefixes and suffixes. Of the 54,139 lookups in the first 1,333 runs, 41,543 were of a whole word, 9,417 of a stem before a suffix, 1,195 after a prefix and 1,984 between both; 8 stems had more than 20 letters and 83 more than their letter's lists. 35,135 stems were found, in every class: 27,285 entries with phonemes and 7,850 with a stress pattern. 14,053 headers became `%`, 18,368 words got the entry's class, and 780 emphasized words with a class got neither. 3,838 digits `1` gave the primary stress, 283 times 3 and 279 times 1, and 462 digits `2` stress 1; 23,507 entries got the default stress, 755 of them on a reduced vowel. The word mark `*` passed 646 entries over and started the search again 20 times. The homographs' test met 1,440 verbs' entries that aren't homographs and 852 stems before `ING`, `ED` or `D`; the previous word decided 6,293 times, after every class of the lists, with a plural and without, and 124 homographs had none; the word before it was tested 1,749 times, 251 of them an article; `subject` before a colon was passed over 3 times, and of 40 tests of `read`, 11 passed its first entry over.

The 24 runs for the user lexicon give it 38 words and speak 10 texts with them, at two modes and speeds: stress digits after full and reduced vowels and after consonants, first, doubled and last; pronunciations without a `1`, with only reduced vowels and without vowels; the phonemes `3`, `4`, `5`, `@`, `|`, `~` and `&`; words given in lower case and in capitals, words the dictionary has, a letter, which spelled words look up, a word of 29 letters and words of 40 and 41, which reach the stage in pieces; the words with suffixes, `~`, `` ` `` and `\Emp\`, in spell mode, between commands, in the filter command's words and after phonetic input's word marks. 4 more runs have lexicons of their own: pronunciations of 39 to 87 symbols, empty ones, ones of only digits and ones with `&`. Of their 366 lookups in the user lexicon, 250 found the stem: 212 a whole word and 38 a stem before a suffix, 20 of an emphasized word and 22 of a de-emphasized one. 162 digits `1` gave the primary stress, 15 of them to a consonant, 30 digits `2` stress 1, and 11 digits were skipped; the default stress fell 50 times on a full vowel and 26 times on a reduced one, and 6 pronunciations found had no vowel and 8 no phoneme. The other runs have no user lexicon: their 52,196 lookups in the first 1,405 runs found nothing.

The 28 runs for the affixes speak 14 texts at two modes, of words picked by running the lookup on words made of the dictionary's words and the affixes: a first suffix `n't`; stems that end in `IE` or `I` before a suffix, found with a `Y`; stems that lost a `T` before `-CE` or `-CY`; stems that lost a silent `E`, found or not, and later stems tried with an `E` after one wasn't found; doubled consonants; `'s` after an `I`; `-ABLE`, `-ABLY` and `-OR` after `IT`, `C` or `G`; prefixes that aren't looked up and that stress the word; foreign endings and an `I` before them; plurals whose `S` reads past its context's end; words of three consonants; stems ending in `AN` or `EN` before `-CRAFT`, `-CAKE` and the like, which get no `T`; and stems ending in `U` that lost an `E`. Of the 42,675 lookups of a word in the first 1,405 runs, 28,757 found it whole and 860 had too few letters for affixes. 9,897 suffixes and 3,530 prefixes were found: 1,515 first suffixes gave the word their class, 18 of them made it a negation, 99 prefixes stressed the header and 56 affixes set the affix stress, 37 of them foreign endings; 10 suffixes and 324 prefixes weren't looked up. 6,311 stems before a suffix were found and 615 after a prefix. Restoring tried 243 doubled consonants, 19 found; 122 stems that lost a silent `E` and 35 tried with one after another wasn't found, 44 found, and 26 kept from it by `IT`, `C` or `G`; 66 stems in `IE`, 26 found; 18 in `I`, 9 found, while 8 before a foreign ending and 6 before `'s` kept the `I`; and 65 `T`s, 18 found. 78 words kept an `E` for letter-to-sound. The contexts passed over an item 2,235 times, 554 of them reading past the context's end.

The 20 runs for letter-to-sound speak 10 texts at two modes, of words picked by running letter-to-sound and its vowel reduction on the original's calls for 32,892 English words: `U` after a vowel or `TH`; `U` after `D`, `T` and `Z` before a break and before a space; `E` before `e` after `L`; words that start with `CON` and the like before two consonants; `O` kept full as a word's first vowel; `S` after `T` under an ending's stress; the prefixes `MC` and `H`, which rules join to the stem or don't, and `H`'s stress; de-emphasized stems with a secondary stress; endings after a stem; prefixes joined after `M` or before `C`; weights after two consonants; `a` before a vowel; and `E` made `Y`. Of the 37,969 settlings of a vowel in the first 1,405 runs, 22,010 were final. 1,023 `U`s became `b`: 461 got a `Y` before them, and 42 after a palatal, a vowel or `X` didn't; 7 of 92 after `S` made it `s`; after the other alveolars and approximants, 70 made `D` `J`, 17 `T` `C` and 8 `Z` `z`, and 21 after `N` got a `Y`, while 297 were stressed, final, or before a word mark, a break or a space; and 22 after `H` weren't reducible. Of the 13,397 reducible vowels, 15 made the `E` before them a `Y`; 14 `O`s without a word mark before them stayed before a consonant at the word's end, and 586 vowels after one stayed full, 518 of them before two consonants, 54 `b`s and 14 `O`s; 51 were reduced already, and 5,964 became `|`, 6,776 `@` and 6 `3`.

Of the 18,456 conversions by letter-to-sound in the first 1,405 runs, 9,844 went on after the stem and 7,872 took a stress pattern. Rules converted 99,875 letters: 1,394 by the first rule of the letter's list, and 60,757 by the 21st or a later one. 2,024 rules that join a prefix to the stem came before the stem's start: 307 met a prefix's mark and 293 joined it, as 6 after `MC` and 8 after `H` didn't; 3,992 rules had a weight; 3,801 consonants weren't written twice, and 2,786 started a syllable with the one after them. 6,296 marks `[` were written, 92 taking the header's stress. 2,282 vowels got the primary stress by their weight, 339 of them closed by two consonants, and 4,725 at the end of the letters; 1,691 got stress 1. The pattern gave its codes 0 to 3 6,806, 4,122, 157 and 7,857 times, ran out 131 times and was passed over in a prefix 1,729 times. 18,541 nodes of a found stem were passed, 3,715 of them stressed; 64 vowels and 416 consonants came after a prefix's mark. 38 conversions had the affix stress 2 and 18 the affix stress 4.

The 52 runs for the post-lexical pass speak 38 texts. For the function words, 14 texts at 90, 98 and 106 words a minute, rates 5 to 7, and at the voices' own speeds: `has-to`, `as-to` and other split words; `the` and `to` before consonants and vowels, rounded and stressed or not; `into`; `used` before `to` and other words; function words with a reduced vowel, `da` and `la` among them, before a weak word and a break; `up` before a break; `to`, `the` and `and` after a comma; `and` after a schwa; `of`, `a` and `for` before a juncture; and `them`, `x e M` after a phoneme and `v` after a juncture, after a quantifier, a negation and `must`. Of the 14,279 function words in the first 1,426 runs, 387 came before a break, and 189 of them lost stress 1 or 2; 69 of the 80 before a weak word that ends its phrase kept their stress; 11,740 went through step 5 of section 12.1, 197 of them in class 4, which only counts its nodes, and 10,604 stresses 2 and 181 stresses 1 became 0, and 764 stresses 2 became 1 and 3 stresses 1 became 0. `Has-to` became `H a S` 3 times; 1,621 `the`s before a consonant became `x @`, and 942 of 1,339 `to`s became `T u`; `into` became `i N T @` twice and `used to` `Y b S T` 8 times. The reductions made 610 vowels of class 3 `@`, 513 of them `@` already, and 536 `and`s `@ N`, 48 of them after the post-lexical rules had freed the `D`, at rates 19 to 25; 55 `v`s after a quantifier or a negation became `@`, and 26 `them`s `x @ M`.

The other 24 texts are for the post-lexical rules: `press` before every key's name, a number and other words, in a split word after a comma, at a phrase's end and not, after a phoneme, and at 90 words a minute; r-coloured vowels across split words, `her-earth` and `bother-Earle`, and at a text's start, `or-earth`; `3 3`, `F @ R @`, `U`, `e G`, `B | d`, and vowels before `R` and `i`, `|`, `e`, `p`, `f` or `y`, in phonetic input with the rules switched on (`ESC[256N`); a syllabic `l` after `3` before a space and before a split word's mark; the dictionary's words for `e K`, `B | H`, `Y` after `s` and `z`, syllabic `l`, `i L`, `I R` and `o R`, and an `i` two phonemes before `?`; junctures before `e` and `i`; a pitch command at its lowest; and, for the consonants, `y L` before a break, `oil`; `to` after a vowel in split words, `go-to-bed`; `L` before `Z` after a nasal's schwa, `tunnels`, and after `@` before `D`, `Ronald`; an `L` after an `L` that the word command `[$]` keeps from the rules, across a space, a split word and `[=]`; one text at 94, 102, 118, 126, 150, 158, 238 and 246 words a minute, rates 6 and 7, 9 and 10, 13 and 14, and 24 and 25, for `N [ K`, `H`, the dropped `T` and `D` and the release; in phonetic input with the rules on, `S M` before a full stop, `~ T L`, `~` before a space, `j` after `@ t`, `| L Z`, an unstressed `D A T` and `L T [`; and `T` before `you` after `s`, `T` before `[` after `L`, `@ j` after `t` before every symbol the closing rules take, and, from the user lexicon, a stressed `R` after `S T` and a stressed `D` or `Z` after `@ L` and `| L`. In the rules' 98,737 runs on a phoneme that isn't a consonant, the keypad pause came 5 times; a word's first vowel got a glottal stop 2,249 times and a `Y` before it 54 times; pauses went in 48 times after a stressed vowel, 119 times after `@`, `o` or `v` and 2,065 times after another vowel; 161 `e`s became `|` and 17 `|`s `E`; 20 `Y`s were freed after `z` or `s`, one of them with `C` the third node before the scan node; 26 `l`s became `@ j`, 178 `m`s `@ M` and 86 `n`s `@ N`; 1,227 `i`s became `|`; 290 schwas before `L` and 53 before `N` became `l` and `n`; and 63 `E`s became `Y`. R-colouring changed 4,354 vowels and 12 `R`s, freed 845 `R`s before a vowel and 3,517 others, stopped at `[` 16 times, and freed 3 vowels after `F`, after which the pass went on from the freed node; 35 `I 3`s became `5`; 414 `R`s after an r-coloured vowel were freed, 2 went in between `3`s, and 6 pauses between r-coloured vowels, as one walk back reached the window's start; 312 `Y b`s became `U`, and 1,287 `Y`s went in before a vowel after a gliding one. In the 140,233 runs on a consonant, the keypad pause came 13 times; 1,386 consonants after `q` got a glottal stop; 90 `Q`s became a space and 77 were freed, 25 of them before a vowel that got a glottal stop; 214 `Z`s became `S`; 108 consonants before `[` and the same consonant were freed going forward, and 2 `x`s in `and the`; 10 `L`s after `[` were freed; 174 `L`s became `j` by rule 7, and by rule 8 2,051 became `j` and 7 `l`, and one stayed; 309 approximants got a glottal stop; 109 `N`s became `~`; 48 `H`s were freed, and 1,047 `H`s and `h`s became `d` and `W`; and pauses went in 54 times after an affricate and 1,347 times after `S` or `Z`. Of the stops, 706 after `S` became `B`, `G` or `D`; 1,050 `D`s and `T`s before `R` became `J` and `C`, and 19 `D`s and 4 `T`s before `you`, and 8 `Y`s after a `T` were freed; 3,631 affricates got their fricative; and 4,558 stops got a release. The alveolar rules made 48 `T`s `D`; dropped 230 `D`s and `T`s, 17 before their affricate, 129 before a consonant and 84 before `[ S`; made 1,408 `T`s `q`, 39 before a neutral phoneme and `N`, 1,312 after a sonorant and 57 after a vowel before `x`; made 2,614 flaps, 194 before a neutral phoneme and `N`, 1,585 between vowels, 544 `T`s before a word mark or `[`, 288 `D`s and 3 `to`s; and set 208 `T`s' byte 0x18 without a flap. 1,455 word marks after a consonant and 1,530 before one were flagged. The closing rules put in 284 pauses after a schwa, made 24 `l`s before `@` `j`, split 380 syllabic `l`s, made 13 `@ j`s after `t` `l`, put 697 `p`s after `~`, 6 `R`s before `l` after `3` and 325 before a vowel, and freed 26 `Y`s before `l`.

## 1. Nodes and classes

### 1.1 The words

The stage's window ([pipeline.md](pipeline.md) §3) holds nodes of types 2 and 3, and control commands. The normalizer hands it:

- a **word of letters**: a header of type 2, the word mark `&`, and the letters, nodes of type 2 whose symbols are `A` to `Z`, `@` for an apostrophe, and `~` or `` ` `` before the first;
- a **word of phonemes**, from phonetic input: a header of type 3, the word mark `&`, and the phonemes, nodes of type 3 with digits `1`, `2` or `"` after the stressed vowels;
- **punctuation** and other nodes of type 3, each a word of one node.

A header's **code** is its value when its flag bit 0x20 is set; the normalizer sets it from phonetic input's word marks (section 3.6). A header's **mark byte**, byte 0x14, is 1 at a phrase's start and between spelled letters and 2 in numbers ([parameters.md](parameters.md) §8); the stage sets it too (section 3.1). The pipeline's **number word** flag is set by the normalizer when it writes a digit's, a teen's or a ten's word.

The stage makes every word's nodes type 3. A header becomes the word's mark: `&` before a content word, `%` before a function word (the rules decide), or `$`, or a space, by its code. Its flag bits 8 to 15 are the word's **class** (section 3.4), and its second value the word's class in the dictionary. A phoneme's flag bits 3 and 4 are its **stress**, 0 to 3, and flag bit 0x20 marks a stressed syllable's nucleus and onset; its byte 0x17 is its syllable's number in its word, and byte 0x19 a function word's class in the dictionary.

### 1.2 Classes

The stage tests the symbols' classes ([parameters.md](parameters.md) §3.2). Symbols of 128 and up never reach the stage (section 14); they are in no class.

| Class | Name here | Symbols |
|---|---|---|
| (0, 0x01) | syllabic | the vowels but `p`, and `l m n` |
| (0, 0x02) | sonorant | the vowels, `p`, `j l L W Y R`, `d h H`, the nasals `M m N n ~` |
| (0, 0x04) | voiced | the vowels, `p`, `j l L W Y R d h`, `B D t J G Q q V x Z z`, the nasals |
| (0, 0x08) | word mark | `% & [` |
| (0, 0x10) | nasal | `M N m n ~` |
| (0, 0x20) | stop | `B C D G J K P T q` |
| (0, 0x40) | fricative | `F S V X Z s x z` |
| (0, 0x80) | phoneme | every phoneme and the pause, a space |
| (1, 0x01) | boundary | `! , . ? \ ]` |
| (1, 0x02) | stress mark | `" 1 2` |
| (1, 0x10) | neutral | `@ p \|` |
| (1, 0x20) | consonant | `B C D F G H J K L M N P Q R S T V W X Z d h j q s t x z ~` |
| (1, 0x40) | break | `! ) , . ? \ ]` |
| (2, 0x01) | closure | `B C D G J K M N P Q T m n q t ~` |
| (2, 0x02) | vowel | the vowels and `p` |
| (2, 0x04) | alveolar | `D N S T Z n q t` |
| (2, 0x20) | front | `4 A E U a e i k \|` |
| (2, 0x80) | velar | `G K ~` |
| (3, 0x01) | approximant | `L R W Y h j l` |
| (3, 0x04) | palatal | `C J Y s z` |
| (3, 0x20) | rounded | `O U W b c g h u w y` |
| (3, 0x40) | y-glide | `5 A E I y` |
| (4, 0x01) | high | `4 E U W Y b c h i u \|` |
| (4, 0x02) | r-coloured | `4 5 c g k r` |
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
| previous symbol | The last symbol of the word before, which the post-lexical rules read when they find no sound before a phoneme (section 13.1) |
| affix, tried E | The suffix the lookup found last, and whether it looked up a stem with a silent E and didn't find it (section 9) |
| consonants | Letter-to-sound's count of consonants, which a context can test (section 8.1) |
| letter, reached, vowel, reducible, after stem, following, ending | Letter-to-sound's: the letter it converts and the node a rule reached, the vowel it wrote last and whether it may be reduced, whether it converts the letters after the stem, and what follows the letter and what the letters after it end in (sections 10 and 11) |

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
6. The word is looked up (section 9). If it isn't found, or the stress pattern isn't 0, the rules convert it by letter-to-sound.
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

A function word after `#` makes the original fault, in the prosody stage (section 14).

## 4. The post-lexical pass

The pass works on the previous word, from its header to the node after its last node, the pass's **end**, which it returns.

### 4.1 Words the pass skips or drops

1. If the previous word's header isn't of type 3, or is in class break, the pass returns the node after it.
2. If the cursor's filter setting ([pipeline.md](pipeline.md) §3.1) isn't 0: a word whose second value is 0, 6, 8 or 15 counts, and filtered becomes filtered plus 1, modulo the filter. The word is kept when it counts and filtered is then 0, and dropped otherwise. A dropped word's nodes are freed, from its header to its last node, and the pass returns the node after them in the window, or none at the window's end. The previous word stays the freed last node.

### 4.2 The phonemes

1. A function word, with the header `%`, that doesn't have code 13 gets the function words' stress (section 12.1).
2. A function word, or a word of second value 15, gives each of its vowels the word's second value, as byte 0x19.
3. Each node from the header to the end, in the window, in turn, stopping at the window's end:
   1. A node of type 3 in class phoneme gets the post-lexical rules (section 13), which return the node to go on with, when the cursor's flag 0x100 is set or its phonetic input setting is 0, and the word's header doesn't have code 13.
   2. Schwas and barred i, section 4.3.
   3. A barred i `|` before a consonant followed by `l` or `j`, in the list, becomes a schwa `@`.
   4. In a function word, a node with a stress other than 0 marks its onset (section 3.5).
4. Each node from the header to the end in the window: a vowel gets the next syllable number, from 1, as byte 0x17. A node `[` is freed.
5. A function word without code 13 gets the function words' reductions (section 12.2).
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

The lookup (section 9) looks the word up in the user lexicon and the dictionary, then takes off its affixes and looks up what they leave. The lookup in the dictionary works on a **stem**: the nodes from the cursor's first working node to its second ([pipeline.md](pipeline.md) §3), the word's letters from its first letter to its last node, or what its affixes leave of them. The data pack holds the dictionary ([datapack.md](datapack.md) §9).

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

The **user lexicon** holds words with their pronunciations, which an application gives the engine; the interface's spec will say how it adds, removes, saves and reads them. Its words are in capitals, of letters only, and its pronunciations are of the symbols `A` to `Z`, `a` to `z`, `1` to `5`, `@`, `|`, `~` and `&`. The lookup (section 9) looks up the whole word in the user lexicon before the dictionary, and so the stem a suffix leaves; the stem a prefix leaves only in the dictionary. The lexicon can change while the engine speaks: the search and the copy of the pronunciation it finds are made under the lexicon's lock.

### 7.1 The lookup

1. The stem is the nodes from the cursor's first working node to its second, as for the dictionary (section 6), and its **word** their symbols, the first included. With more than 40, the stem isn't found.
2. The lexicon is searched for the word, symbol by symbol. If it hasn't the word, the stem isn't found. The stage's letters are capitals, so a word is found whatever the case it was given in.
3. The stem's letters are freed, and the pronunciation's phonemes, nodes of type 3, take their place: every symbol but the digits `0`, `1` and `2`, so `3`, `4` and `5` are phonemes, and `&` becomes a phoneme too. The working nodes become the first and the last phoneme, and so do the word's first letter and last node when the stem starts or ends there. The word keeps its header and its class: the user lexicon gives none.
4. A digit after a phoneme gives it its stress: a `1` the word's **primary stress**, 3 when the word is emphasized, 1 when it is de-emphasized, else 2, whether the phoneme is syllabic or not; a `2` stress 1; a `0` none, and it keeps the phoneme from the default stress. A digit after another digit, or first, is skipped. Unlike the dictionary's, these digits follow the phoneme they stress.
5. If no phoneme has a `1` after it, the **default stress**: the first syllabic phoneme with no digit after it gets the primary stress, the first that isn't reduced, or else the first reduced one. A pronunciation without a syllabic phoneme gets no stress.

A pronunciation without phonemes, empty or only digits, still frees the stem: nothing takes its place, the first working node becomes the node after the stem and the second the node before it, and the word's first letter or last node with them.

## 8. Contexts

The affixes and letter-to-sound test the nodes next to a place with **contexts**: conditions on the symbols before the place, or after it. A context is a list of **items**: **clauses**, and **ends**. A clause has a **test**, a byte, and its **elements**, symbols and classes; a symbol is in a class when its class byte of the class's plane has one of the class's bits. The data pack holds the contexts ([datapack.md](datapack.md) §8).

### 8.1 Testing a context

A context is tested at a node, looking back or forward. The nodes before and after a node are its previous and next nodes in the window ([pipeline.md](pipeline.md) §3), none past the window's ends.

1. A context without items holds.
2. Its clauses are tested in turn, from the first. Each has a **result**, and is either **sufficient**, and the context holds when it holds, or **necessary**, and the context fails when it fails. Otherwise the test goes on to the next item; a clause that **read past its end** (below) makes it pass over that item and go on to the one after.
3. At an end, or after the last item, the context's result is the last clause's.

The test's high three bits say what the clause tests:

- `0x20`, **consonants**: letter-to-sound's count of consonants. The context holds when the count is more than 1, and fails otherwise, whatever comes after the clause.
- `0x40`, **a mark before the vowel**: whether the nodes before the node, back to the nearest vowel, include one in class word mark. The walk stops at a vowel, before testing it, and at a `Y` whose node before it in the list is a consonant, after testing it. Sufficient. `0x60` is the same negated, and necessary.
- `0xa0`, **one vowel**: whether the nodes before the node, back to the nearest one in class word mark, include exactly one vowel, a `Y` whose node before it in the list is a consonant counting as one. Sufficient. `0xc0` is the same negated, and necessary.
- `0x80`, **a doubled consonant**: whether the node before the node, or after it when the context looks forward, is a consonant, and the node beyond it in the list has the same symbol. Sufficient.
- `0x00` and `0xe0`, **elements**: the nodes' symbols compared with the elements.

These look back whichever way the context looks, but the elements and the doubled consonant.

An element clause is a **sequence** when the test has bit 0x10: it compares its first element with the first node, the next element with the next node, and so on for (test & 3) + 2 nodes, from the node before the node, or after it, outwards. Otherwise it compares the first node with each of its elements in turn. A symbol element matches a node with that symbol, a class element one whose symbol is in the class. The test's bits 0x04 and 0x08 decide what the clause holds for:

| Bits 0x08, 0x04 | The clause holds when | Kind |
|---|---|---|
| 0, 0 | every comparison matches | sufficient |
| 0, 1 | a comparison matches | sufficient |
| 1, 0 | no comparison matches | necessary |
| 1, 1 | a comparison doesn't match | necessary |

The clause's result starts as true when bit 0x04 is clear, false when it is set. A comparison that doesn't match, when the two bits are alike, or one that matches, when they differ, **decides**: the result becomes the other value, and stays it.

- A clause that isn't a sequence stops at the first comparison that decides. If none does, it has read past its end.
- A sequence goes on to its last node, but stops when the result is true and the test has bits 0x1c. At a node with no element left, the elements being one fewer than the nodes, it compares nothing and has read past its end.
- When the nodes run out before the clause is done, its result is false.

A clause that reads past its end and doesn't decide the context makes the test pass over the next item. When that item is the context's end, the test goes on into the items the original stores after it, which the data pack lists after the end. The context of the suffix `S` ends with whether the node before the `S` is a closure, `E`, `A` or `@`; when it isn't, the test goes on to the next context's first clause, a necessary one: whether that node is neither a vowel nor `D`, `N`, `S`, `T` or `Z`. So most plurals lose their `S`.

## 9. The lookup by affixes

The rules' lookup (section 3.3, step 6) looks the word up in the user lexicon and the dictionary, and reports whether it found it. Otherwise it takes off the word's affixes, suffixes from its end and then prefixes from its start, and looks up the stems they leave; a stem found replaces its letters with its phonemes and leaves the affixes' letters to letter-to-sound, and the lookup reports the word not found.

An **affix** has its letters, a context, whether the stem it leaves is looked up, a word class, a stress, and the list of affixes tried after it. There are 158 suffixes, in 36 lists, and 74 prefixes, in 35 lists; the data pack lists them, and the list each letter `@` to `Z` starts: words ending in `D E F G H K L M N O P R S T U W X Y` have suffixes, words starting with `A B C D E F G H I M N O P R S T U V` prefixes. A suffix's class is 0, 6 (verbs: `-ED`, `-IZE`, `'VE`, `'RE`, `'LL`), 15 (adverbs: `-LY`, `-ILY`, `-ABLY`, `-IBLY`, `-ALLY`) or 8 (negations: `N'T`). Its stress is 0 or 2, a prefix's 0, 1 or 4; stress 2 marks the endings of foreign names (`-MANN`, `-SEN`, `-MEYER`, `-BAUER`, `-BURG`, `-LING`, `-LER`, `-NER`, and `-T`, `-L`, `-AU`). 14 affixes don't look the stem up, and 140 have no context.

### 9.1 The lookup

1. The cursor's working nodes become the word's first letter and its last node. If the user lexicon (section 7) has that stem, or else the dictionary (section 6), the word is found.
2. The **affix**, the suffix found last, becomes none, and **tried E** false.
3. The stem's **shortest end**: walking from the first letter towards the last node, the third node, or the first after it that isn't a consonant when the first three are consonants. If the walk reaches the last node first, go to step 7.
4. **Suffixes.** The list is the suffixes of the second working node's symbol. While there is a list: find an affix in it (section 9.2), back from the second working node, short of the shortest end; it becomes the affix, and if there is none, stop. If it is the first suffix found and its class is 6 or 15, the word class becomes it; if its class is 8, the word class becomes 8 and the header `%`. If the suffix is looked up, look its stem up in the user lexicon, then the dictionary, then restore it (section 9.3); if one finds it, the lookup ends. The list becomes the suffix's next list.
5. The stem's **shortest start**: walking back from the second working node towards the first letter, the third node, or the first after it that isn't a consonant when the first three are consonants. If the walk reaches the first letter, or the affix stress is 2, go to step 7.
6. **Prefixes.** The list is the prefixes of the first working node's symbol. While there is a list: find an affix in it, forward from the first working node, short of the shortest start; if there is none, stop. If the prefix is looked up, look its stem up in the dictionary, then restore it; if one finds it, the lookup ends. The list becomes the prefix's next list.
7. If tried E, a letter `E` goes after the second working node and becomes it, for letter-to-sound.

The first suffix found sets the word's class even when no stem is found, and each affix found its stress and its `[`. A stem found ends the lookup before step 7.

### 9.2 Finding an affix

The affixes of the list are tried in their order; the first whose letters and context fit is found.

1. A suffix's last letter must be the starting node's symbol, and its letters before it the symbols of the nodes before it in the list; a prefix's first letter the starting node's, and its next letters those of the nodes after it. None of them may be on the shortest end, or start.
2. Its context must hold at the node of its letter farthest from the start, looking back for a suffix and forward for a prefix.
3. The stem now ends at the node before the suffix, which becomes the second working node, or starts at the node after the prefix, the first working node. A node `[` of type 2 goes between the affix and the stem.
4. An affix's stress below 2 becomes the header's stress, flag bits 3 and 4; otherwise the affix stress becomes it.

### 9.3 Restoring a stem

A stem not found may be one the affix changed: a doubled consonant, a silent E, a `Y` made `I`, a `T` dropped before `-CE` or `-CY`. Restoring looks it up in the dictionary as it was. Its **end** is the second working node; the contexts are tested at the node after it, looking back.

1. If the end is a consonant and the node before it has the same symbol, the stem without the end is looked up; if found, the end is freed and the stem found. Otherwise go to step 4.
2. If tried E, go to step 3. Otherwise the stem may have lost a silent E only when the affix, the suffix found last (none for a prefix's stem), may drop one: `ING`, `EST` and `ILY` always, `ABLE`, `ABLY` and `OR` unless the end is in context **withoutE** (`IT`, `C`, `G`); and only when the end is a consonant or `U`, and in context **silentE** or not in context **noSilentE**. Otherwise go to step 4.
3. A letter `E` goes after the end and is looked up as the stem's end. If found, the stem is found; otherwise the `E` is freed and tried E becomes true. So once a stem has been tried with an `E` and not found, every later stem of the word but one ending in a doubled consonant is tried with one too.
4. If the end is `E` after an `I`, the end is freed and the `I` becomes `Y`, the stem's end, which is looked up.
5. If the end is `I`, the stem was left by a suffix other than `'s`, and the affix stress isn't 2, the end becomes `Y` and is looked up.
6. If the end isn't the word's last node, it is in context **lostT** (`AN`, `EN`), and the next nodes in the window are `[`, `C`, and a node that is neither `R` nor `A`, a letter `T` goes after the end, becomes the stem's end, and is looked up.
7. The stem isn't found.

A letter `Y` or `T` a step made stays when the stem isn't found, and so letter-to-sound sees it. After step 4 frees the end, steps 5 and 6 test the freed node: it is still `E`, so step 5 doesn't apply, and step 6's context, which needs an `N` there, doesn't hold.

## 10. Letter-to-sound

Letter-to-sound converts a word's letters, nodes of type 2, into phonemes by **rules**, from the last letter to the first: first the letters from the stem's end (section 9.1) back to the word's start, then, when the stem's end isn't the word's last node, the letters from the word's end back to the stem. Each letter `@` to `[` has a list of rules in the data pack ([datapack.md](datapack.md) §8), 753 in all. A rule names the **letters before** the letter it converts, if any, and **phonemes**, which take the letters' place; a context the nodes before those letters must be in (section 8.1); **masks**, which test what follows the letter; **flags**, which tell the rules of the letters before what follows them; a **weight** towards the stress; and whether it **joins the prefix** to the stem. The last rule of each list holds whatever the letters. As the rules write the phonemes from the last, each vowel is stressed as it is written, and the vowel after it is settled, and maybe reduced (section 11).

### 10.1 The conversion

Letter-to-sound keeps, in the speaking thread's state, the **letter** it converts, the node a rule has **reached**, the count of **consonants** a context can test (section 8.1), the **vowel** it wrote last and whether that vowel is **reducible**, whether it converts **after the stem** (section 11), and two sets of bits the rules' masks test: **following**, what follows the letter, and **ending**, what the letters after it end in. Of following's bits, 1 is the word's end, 2 a stem's vowel and 0x4000 a full vowel; of ending's, 4 is an affix's mark `[` and 0x80 to 0x1000 the ending's **stress**. For one word it also keeps the **primary** stress, the vowels' **weight** towards the stress and the rule's, the following bits a rule **carries** to the next, the ending's **mark** bit, the **last symbol** it wrote, the stress **pattern**'s codes left and how many vowels may still take one, and whether it is **at the end** (no vowel or weight written yet), whether the word is **stressed** (it has its primary stress), whether the vowel written last is **marked** (stressed), and whether it has **passed the stem's start**, now and before the current rule.

1. The vowel and reached become none, and after the stem false. The primary stress is 3 when the word is emphasized, 1 when it is de-emphasized, else 2. Consonants become 1, following 1, ending 0, and the vowel isn't reducible. The letter is the cursor's second working node, the stem's end. The pattern is the stress pattern, and 4 vowels may take it. The ending's stress is the affix stress's bit, 0x80 for 2, 0x100 for 3, 0x200 for 4, 0x400 for 5, 0x1000 for 6 and 0x800 for 7, or else 0, and ending becomes it. The weights, carried bits, mark and last symbol are 0; letter-to-sound is at the end, the word isn't stressed, nothing is marked and the stem's start isn't passed.
2. Repeatedly:
   1. If the letter isn't of type 2, pass it (section 10.4); after the stem, that ends the conversion.
   2. Otherwise, if its symbol isn't `@` to `[`, free it; the letter becomes the node before it.
   3. Otherwise convert it (section 10.2).
   4. If the letter is none, the window has no node before the last one passed: the original faults, and the port takes the letter as the word's header (section 14).
   5. If the letter is the word's header and the stem's end wasn't the word's last node, convert the letters after the stem: the letter becomes the word's last node, following 1, ending 0, or the ending's stress when it has one, the mark 0, after the stem true, the carried bits and the weight 0, consonants 1, the last symbol 0, and nothing is marked.
   6. If the letter is the word's header, the conversion ends.

### 10.2 Converting a letter

1. The rules of the letter's symbol are tried in their order; the first that holds converts it. For each, reached becomes the letter, and the rule holds when:
   - its masks match: with `f` and `e` its two masks, when `f` has bit 0x8000, following has none of `f`'s bits and ending none of `e`'s; otherwise, when `e` has bit 0x8000, following has one of `f`'s bits or ending one of `e`'s; otherwise following has all of `f`'s bits and ending all of `e`'s;
   - the letters before it are its letters before: for each, from the nearest, reached moves to the node before it in the window, which must be of type 2 with that symbol;
   - its context holds at reached, looking back (section 8.1).
2. The nodes from reached up to the letter are freed, the letter last, and the letter becomes the node before them. If the cursor's first working node, the stem's start, is free, the stem's start is passed. Reached becomes none.
3. If the rule joins the prefix, the stem's start isn't passed and the node before the first working node in the list is `[`, the prefix's mark: unless the word's first letter is `M` before `C`, or `H` before `[`, the mark is freed and the first working node becomes the word's first letter.
4. If the rule has a weight, the rule's weight becomes it, consonants 0, and letter-to-sound isn't at the end.
5. The rule's phonemes are written from the last; for each symbol `s`, with `t` the last symbol written:
   1. Unless `s` is `t` and a consonant, a node `s` of type 3 goes after the letter, and reached becomes it; if the word's last node is free, it becomes the word's last node.
   2. A syllabic `s` is a vowel (section 10.3).
   3. For `[`: if `t` is a phoneme, `s` becomes `t`. If the header's stress is 1, the vowel written last is marked and the header's stress becomes 0.
   4. Another `s` is a consonant. If consonants is above 0 and the weight isn't 0, or consonants is above 1, the carried bits become 0. Consonants grow by 1, unless the next symbol to write, the one before `s` in the word, is `[`; or `s` is `S` after a `T` and the ending's stress has bit 0x400; or consonants is 1 and `s` starts a syllable with `t`: a closure before `R`, or `K` or `G` before `W`.
   5. The last symbol becomes `s`.
6. Finish the rule (section 10.5).

### 10.3 A vowel

With `V` the vowel the rule wrote:

1. If letter-to-sound is at the end and consonants is above 0, consonants fall by 1.
2. If the rule's second flags have bit 8, the vowel written before isn't reducible.
3. If there is a vowel, settle it (section 11.1). If `V` is no longer syllabic, an `E` the settling made `Y`: `V` isn't reducible, the weight grows by 1, and go to step 7.
4. If the stress pattern isn't 0 and the stem's start wasn't passed before the rule, the count of vowels that may still take the pattern falls by 1, as a signed byte; if it was above 0, `V` takes the pattern's code, its low two bits, and the pattern shifts right by two as a signed byte, so that a pattern with its top bit set gives code 3 when its codes run out. Otherwise go to step 5. Code 0: `V` isn't marked, and is reducible. Code 1: `V` isn't marked nor reducible. Code 2: `V` gets stress 1 and is marked, not reducible. Code 3: `V` gets the primary stress, the word is stressed, and `V` is marked, not reducible. Go to step 7.
5. If the word isn't stressed: if the weight is 1 and consonants above 1, the weight becomes 2. It grows by the rule's weight and 1, and the rule's weight becomes 0. If it is above 2, `V` gets the primary stress, the word is stressed and `V` marked. If the word is stressed and letter-to-sound isn't after the stem: `V` gets stress 1 and is marked when the vowel written before isn't, and isn't marked when it is; so the vowels before the primary stress get stress 1 in turn.
6. `V` is reducible unless letter-to-sound is at the end and the node after `V` in the list is velar; or `V` is `U` after a word mark; or `V` is marked, or consonants is 0 and `V` isn't `o` or `a`, or `V` is reduced, or the rule's first flags have bit 0x4000.
7. Consonants and the carried bits become 0, the vowel becomes `V`, and letter-to-sound isn't at the end.

### 10.4 Passing a phoneme

A node of another type than 2 is a stem's phoneme, or the word's header.

1. The stem's start is passed. After the stem, the vowel, if any, is settled, not final (section 11.1), and the conversion ends.
2. If the node's stress is 0 and it is a vowel, the vowel written last isn't marked. If its stress isn't 0, it is marked, and the word is stressed unless the stress is 1 and the word isn't de-emphasized.
3. Following becomes 0. If the node before it in the window is `[`: if the node isn't a consonant, following becomes 2, with bit 0x4000 when the node isn't reduced; if it is, consonants grow by 1. Ending gets bit 4.
4. The letter becomes the node before it in the window.

### 10.5 Finishing a rule

1. If the stem's start is passed, or letter-to-sound is after the stem, and there is a vowel: if the word isn't stressed, the vowel gets the primary stress, the word is stressed, the vowel is marked and not reducible. Settle the vowel, final (section 11.1); if following then has bit 0x4000, it isn't reducible.
2. With `f` the rule's first flags without bit 0x8000: if the last symbol is marked and a vowel, `f` gets bit 0x4000.
3. Ending becomes the rule's second flags without bit 0x8000. If the mark is 0, it becomes those flags' bit 4; otherwise ending gets it. If the ending's stress is 0, it becomes ending's bits 0x1f80; otherwise ending gets it.
4. If `f` isn't 0: if it is 1, the word's end, the carried bits become following. Following becomes the carried bits and `f`, and the carried bits become `f`'s bits 0x2048. If those aren't 0 and the weight isn't 0, consonants become 0.
5. Whether the stem's start was passed before the rule becomes whether it is now.
6. If the letter is `[`: when the stress pattern is 0, following gets bit 0x4000 if the vowel written last is marked or not reducible; otherwise ending gets bit 4.

## 11. Reducing a vowel

Letter-to-sound (section 10) converts a word's letters from its last to its first, and so writes the phonemes after a letter before the letter's own. It keeps the **vowel** it wrote last, and whether that vowel is **reducible**; when it writes a vowel before it (section 10.3), and at the end of a rule once it has passed the stem's start (section 10.5), it settles that vowel, which can then be reduced. The settling at the end of a rule is **final**. Settling reads two more of letter-to-sound's values: **after stem**, whether it is converting the letters after the stem (section 10.1), and **following**, what follows the letter it converts; settling can set its bit 0x4000.

### 11.1 Settling the vowel

With `V` the vowel, `B` the node before it in the list and `b` the symbol `B` has when the settling starts:

1. If `V` is `U`, it becomes `b`, and:
   - When `b` is neither alveolar nor approximant: unless `b` is palatal, a vowel or `X`, a node `Y` of type 3 goes before `V`.
   - When `b` is `S`: `B` becomes `s` if the node after `V` in the list is `R`, or, unless the settling is final, isn't `L`.
   - Otherwise, when `V` is unstressed, the settling isn't final, and the node after `V` in the list is neither a word mark, a break nor a space, or `b` is `N`: `B` becomes `J` for `D`, `C` for `T` and `z` for `Z`; for `N`, a node `Y` of type 3 goes before `V`.
   - When `b` is `H`, `V` isn't reducible.
2. If `V` is reducible, it is reduced (section 11.2).
3. The vowel becomes none.

### 11.2 Reducing the vowel

The **reduction context**, in the data pack ([datapack.md](datapack.md) §8), is the clause `0x40`: it holds at a node when a word mark is among the nodes before it, back to the nearest vowel (section 8.1). With `v` the symbol of `V` now, and `n` and `m` the symbols of the node after `V` in the window and of the one after that, 0 for none:

1. If `b` is `E`, the node before `B` in the list is `L`, `v` is none of `v`, `a` and `e`, and the context doesn't hold at `B`: `B` becomes `Y`.
2. If the context doesn't hold at `V`, `v` is `O`, `n` is a consonant, `m` is a word mark or 0, and the stress pattern is 0, `V` stays.
3. If the context holds at `V`, letter-to-sound isn't converting after a stem, the stress pattern is 0 and `n` is a consonant, `V` stays, and following gets bit 0x4000, when:
   - `v` is `b`; or
   - `v` is `O` and `m` isn't `[`; or
   - `m` is a consonant, other than `n`, and neither `R` after a stop nor `W` after `K`; and not with `v` `o`, `b` `K` and `n` `N` when the node before `B` is a word mark.
4. If `v` is reduced, `V` stays.
5. Otherwise `V` becomes:
   - `|` for `E`, `I` and `i`;
   - for `e`, `@` before `L` or `R`, and `|` before the others;
   - `3` for `g`, `k` and `r`;
   - for the others, `|` when `n` is palatal or velar and `b` isn't a word mark, or when `b` is palatal and `n` is `S`; `@` otherwise.

## 12. The function words

A function word, a word whose header is `%` without code 13, gets the function words' **stress** before its phonemes get the post-lexical rules (section 4.2, step 1), and their **reductions** once its pass is done (section 4.2, step 5). With `w` the function word, the previous word, and `n` the **next word**, the current word, which the step has just converted (section 2.1), both test `w`'s **class**, its header's second value, which is the dictionary's class (section 6.1), and the **rate**, the cursor's rate index ([pipeline.md](pipeline.md) §3.1). A class is **weak** when it is 1 to 5, 9, 10, 12, 13 or 14: every class of function words but the quantifiers, 7, the negations, 8, and `may`, `must` and `might`, 11.

### 12.1 The stress

1. If `n` isn't in class break: its **stress** is the last stress other than 0 among its nodes, from its header to its last node, or 0 when its class is 1, 3 or 10. `n` is **`to`** when it has three nodes and its last two are `T b`. If `n`'s class is weak but not 4, its stress is 0 and the node after its last node in the list is in class break, `w` **keeps its stress**, unless one of its nodes is `@` or `|`.
2. If `n` is in class break, `w`'s vowels are counted, and the last is noted.
3. The **next symbol** and its stress are those of `n`'s header, or, when the header isn't in class phoneme, those of the node after it in the window, if there is one.
4. If `n` is in class break: when `w` has fewer than 2 vowels and its class is 1, 3, or 10 with a last vowel other than `v`, `f` and `w`, each of its nodes with stress 1 or 2 gets stress 0. That ends it.
5. If `w`'s class is weak and `w` doesn't keep its stress, its nodes are counted, from its header, and unless its class is 4 each node's stress 2 becomes 1 and 1 becomes 0, and above rate 8 any stress becomes 0.
6. If `w`'s class is 12 and `n`'s is 10, the node before `n`'s header in the list is `Z` after `a` and the two nodes after the header are `T b`, the `Z` becomes `S`: `has to`. A word command `z` comes before every written word's header, so only a split word, `has-to`, has it.
7. `w` **reduces** when the rate is above 5, the next symbol isn't a vowel, or is rounded and stressed, and the node before `w`'s header in the list isn't `.`, `,` or 0.
8. If `w`'s class is 2 or 10, with `f` its first node after the header and `s` the node after `f` in the window: if `f` is `x`, `s` becomes an unstressed `@` when the next symbol isn't a vowel and the rate is above 6 (`the`). Otherwise, if `f` and `s` are `T b` and `w` reduces, `s` becomes an unstressed `u` (`to`). That ends it.
9. Otherwise, when step 5 counted 5 nodes: if the nodes after the header in the list are `Y b Z D` and `n` is `to`, the `Z D` become `S T` (`used to`); if they are `i N T b` and `w` reduces, the `b` becomes `@` (`into`).

### 12.2 The reductions

1. If `n` isn't in class break, with `f` `w`'s first node after its header in the window and `b` the symbol of the node before its header in the list:
   - in class 3 above rate 6, each node from `f` on, while it is of type 3 and not a word mark `&` or `%`, to the window's end: an unstressed vowel but `g` becomes `@`;
   - in class 14, if `f` is `a`, `w`'s last node is `D` and the rate is above 5: unless `b` is `.`, `,`, 0 or `@`, `f` becomes an unstressed `@` and the `D` is freed, and the node before it becomes `w`'s last node (`and`). Otherwise, if `f` is `F`, the rate is above 5 and `b` isn't `.`, `,` or 0, the node after `f` in the window becomes an unstressed `@`.
2. Each node from `w`'s header to the window's end, past `w` and into the words after it:
   - when `w`'s class isn't weak, an unstressed `v` becomes `@`, unless the node two before it in the list is in class break, or the node before it is a space or a word mark `&` or `%`;
   - an unstressed `e` with `x` before it and `M` after it in the list becomes `@` when the node before the `x` is a word mark `&` or `%` and has a node before it that isn't a space or `.` (`them`).

**A freed `D`.** The post-lexical rules can free `and`'s `D` before the reductions run; in the recorded runs they did at rates 19 to 25. The word's last node is then the freed node, which is still `D`, and step 1 frees it again: the node moves to the free list's end and the free count grows once more ([pipeline.md](pipeline.md) §2.3), so that the count is one more than the free list's nodes until the pipeline is initialized again. The port does the same. The count only decides the stages' turns near the free-node limits, which a session never comes near ([pipeline.md](pipeline.md) §9).

## 13. The post-lexical rules

The pass gives each phoneme of the previous word the post-lexical rules (section 4.2, step 3). They test the phonemes around it and the words' symbols, and change its symbol and its neighbours', put pauses and phonemes in, free nodes, and mark a vowel's **glottal stop**, flag bit 0x40 ([parameters.md](parameters.md) §15.9). They return the node the pass goes on with. Below, `p` is the phoneme, a node is **stressed** when it has flag bit 0x20 (section 1.1), and **r-coloured** means `3` or in class r-coloured. A new node is a phoneme, of type 3, and its **frames** are its value, the duration the prosody stage gives it ([prosody.md](prosody.md) §6), 0 unless a rule says otherwise. Freeing a node goes back ([pipeline.md](pipeline.md) §2.3), unless a rule says it goes forward: it returns the node before it in the window, or none at the window's start. The rate and the pitch are the cursor's ([pipeline.md](pipeline.md) §3.1).

### 13.1 What the rules see

- The **next** phoneme is the first node of type 3 after `p` in the window, the **previous** the last node of type 3 or 4 before it, and the **one after next** the first node of type 3 after the next. Their symbols are 0 when there is none.
- The **sound before**: from the previous phoneme back, along the nodes of type 3 or 4 in the window, the symbol of the first in class phoneme, until one in class break. If there is none, it is the stage's previous symbol (section 4.2, step 6); otherwise the previous symbol becomes 0.
- The **sound after**: from the next phoneme on, along the nodes of type 3 in the window, the symbol of the first in class phoneme, until one in class break; or 0. That node is the **sound after's node**.
- The previous phoneme is **at the list's start** when there is none, or when it is a word mark `&` or `%` and the node before it in the list has symbol 0: the list's first sentinel.
- `s`, `p`'s **symbol** as the rules start; some rules change it. `p` is a **vowel** when `s` is one as the rules start.
- Both words are **marked** when the previous word or the current one has mark byte 1 (section 1.1).
- The rules' **result**, none at first, and the **closing node**, `p` at first.

Many rules read the nodes next to `p` in the list itself, whatever their type: a word command `z` comes between two written words, so a rule that reads the node before a word mark reads it, and its flag bit 0x20 is the command's reset flag ([pipeline.md](pipeline.md) §4.1). Only split words, `her-earth`, and phonetic input have no command between their words.

The rules run in three groups: the keypad pause (section 13.2), and if it doesn't apply, the rules for a phoneme that isn't a consonant (section 13.3) or for a consonant (section 13.4); then the closing rules (section 13.5). The rules return the result, or the closing node when there is none.

### 13.2 The keypad pause

When the current word is a break, and `k`, the node before `p` in the list, follows `& P R e S` (`press`), whose `&` has a node before it in the list not in class phoneme: if `k` has mark byte 2, a number's word, or the previous word **starts with a key's name**, a pause `' '` of 27 frames goes before `p`, and is the result. The names are, from the node after the word's header in the list: `A T`, `F w R`, `F I V`, `N I N`, `P f N D`, `S i K S`, `S e V | N`, `S T o R`, `T b`, `W v N`, `X R E` and `Z 4 R O`: eight, four, five, nine, pound, six, seven, star, two, one, three and zero, and the words that start like them, `eighty`. As a word command `z` comes before every written word, only a split word after a comma written without a space has it: `go,press-star.`, `go,press12.`

### 13.3 Vowels and the others

For a `p` not in class consonant, these in turn, until one ends the rules:

1. **A word's first vowel.** If `p` is a vowel other than `U`, the previous phoneme is a word mark `&` or `%` and the pitch isn't 0:
   - when `s` is the sound before and in class y-glide, and the words aren't marked, a `Y` of 4 frames goes before `p`, the result. That ends the rules.
   - otherwise `p` gets a glottal stop, if the sound before is 0 or `s` and the words aren't marked: when the sound before is `s`, or `p` has a stress and `s` is front but not `a`. If the sound before is another, or the words are marked: when `p` has a stress, and the sound before is `@`; or it is a vowel and the rate is below 13; or `p`'s stress is 2, the rate is below 13 and the sound before is syllabic; or the rate is below 7 and the sound before is a fricative or a sonorant.
2. An `e`, not stressed, before `K S` or `G Z` (the next and the one after next), after a word mark or a space, becomes `|`: `explain`. That ends the rules.
3. A `|`, not stressed, after `B`, with the sound after `H` or `d` and the next not a word mark, becomes `E`: `behind`. That ends the rules.
4. A `Y` after a word mark, in a word of second value 1 (a pronoun), with the sound before `z`, or `s` while the third node before the cursor's scan node in the list is `C`, is freed, and what freeing returns is the result: `as you`, `which you`. That ends the rules.
5. An `l` before `[` or `D` becomes `j`, and an `@` of 4 frames goes before it, the result. That ends the rules.
6. An `m` not before `.`, `?` or `,` becomes `M`, and an `n` not after `q` with a vowel the sound after becomes `N`; an `@` goes before it, the result. That ends the rules.
7. A stressed vowel `p` after a word mark, when the sound before is a vowel, the node before the previous phoneme in the list is stressed and the pitch isn't 0: a pause of 5 frames goes before `p`, the result. That ends the rules.
8. If the sound before is `@`, `o` or `v`, `p` is stressed and the pitch isn't 0: when `p` is a vowel after a word mark, a pause of 5 frames goes before it, the result, and `p` gets a glottal stop. Otherwise, if `p` is a stressed vowel after a word mark, the sound before a vowel that isn't r-coloured, and the words aren't marked, a pause of 3 frames goes before it, the result, and `p` gets a glottal stop. Either ends the rules.
9. An `i` with no stress and not stressed, in a word whose second value isn't weak (section 12), becomes `|`, and `s` with it, when the previous phoneme isn't at the list's start, the node before `p` in the list isn't a space and has a node before it not in class break, and the three nodes after `p` in the list are there and none is `.` or `?`.
10. A `@` or `|` before `L`, not after a nasal: when there is no one after next, or the node after it in the list isn't `L` and it is reduced, a word mark or a boundary, or it is `Z`, `p` becomes `l` and the `L` is freed; what freeing returns is the result: `bottle`. That ends the rules.
11. An `@`, `p` or `|` before `N`, after a consonant that isn't a sonorant or after a boundary, with the sound before `q`, becomes `n`, and the `N` is freed, the result: `button`. That ends the rules.
12. An `E` of stress 0 before `@`, after `j`, becomes `Y`, and `s` with it.
13. **R-colouring.** A syllabic `s` before `R`, when there is no one after next or it has stress 0:
    1. If `s` is `@` or `|`, the node before `p` in the list is `F` and the node two after `p` is `s` too, `p` is freed. The result is `p`: the pass goes on from the freed node, whose next node is the free list's end, and stops there, before the word's other phonemes (section 4.2, step 3). That ends the rules.
    2. `v`, `|`, `@` and `i` become `3`; `E` becomes `4`; `I` becomes `5`, unless the one after next, or the node after it in the list when it is `[`, is `|`, `@` or `O`; `a`, `e` and `A` become `k`; `o` becomes `r`, unless the one after next is `E`, `i`, `|` or `e`; `O` and `w` become `g`; `b` and `u` become `c`. Otherwise an `I` whose one after next isn't `@`, `O` or `|`, an `f` and a `y` keep their symbol, and the `R` becomes `3`.
    3. If `p`'s symbol changed, and the one after next is a vowel other than `p`, or the node two after `p` in the list is `[` and the node after that a vowel other than `p`, the `R` is freed, the result.
    4. If the one after next isn't a vowel: when it is `[` and the node after it in the list is a vowel, that ends the rules; otherwise, unless the `R` is `3`, the `R` is freed, the result.
14. A `3` after a stressed `I`, the node before it in the list: unless the node after `p` in the list, or the node after that when it is `[`, is `|`, `@` or `O`, the `I` becomes `5` and `p` is freed. What freeing returns is the result and the closing node: `fire`. That ends the rules.
15. An r-coloured `s` before `R`, when the one after next is `E`, `A`, `e`, `a`, `I`, `i`, `f`, `v`, `O`, `y`, `@`, `b` or `|`: the `R` is freed, the result.
16. If `s` is `3`, the sound before is `3` and the previous phoneme isn't `&`, an `R` goes before `p`. That ends the rules.
17. An r-coloured `p` after a word mark whose node before it in the list is r-coloured: walk back from the node before that, along the nodes of type 3 or 4 in the window, to the first in class phoneme that isn't a space. At none that ends the rules; when it isn't r-coloured, a pause of 6 frames goes before `p`, the result, and that ends the rules too: `her-earth`.
18. A `b` with a stress after `Y`, with a consonant before the `Y` in the list, becomes `U`, and the `Y` is freed, the result: `cute`. That ends the rules.
19. A vowel `s` after a phoneme in class y-glide, or after `[` with one before it in the list: a `Y` of 3 frames goes before `p`, the result.

### 13.4 Consonants

For a `p` in class consonant, these in turn, until one ends the rules. Some rules **flag** the word mark after `p` or the one before it: when the rules end, after the closing rules, the next phoneme gets flag bit 0x80 if the rules flagged it and it is a word mark `&` or `%`; otherwise the previous phoneme does, if the rules flagged it and it is one. The prosody stage puts fewer breaks before a flagged mark ([prosody.md](prosody.md) §3.3). A **flap** is a `t` whose byte 0x18 is the stop it stands for, `T` or `D` ([prosody.md](prosody.md) §2.1).

1. **After `q`.** If the sound before is `q`, the pitch isn't 0, `s` is voiced, and the node two before the previous phoneme in the list is there, and isn't a vowel when `s` is `x`: `p` gets a glottal stop, and the mark before it is flagged.
2. A `Q` before a phoneme not in class phoneme becomes a space. Before one in class phoneme, the next phoneme gets a glottal stop if it is a vowel and the pitch isn't 0, and `p` is freed, the result. Either ends the rules.
3. A `Z` with a sound after that isn't voiced, before a phoneme that is neither a word mark `&` or `%` nor in class break, becomes `S`. That ends the rules.
4. If the next phoneme is `[` and the node two after `p` in the list has the symbol `s`, `p` is freed going forward: freeing returns the node after it in the window, or none at the window's end ([pipeline.md](pipeline.md) §2.3), and that is the result: `really`, `lessen`. That ends the rules.
5. An `x` after a word mark `&` or `%` after `D` after `N`, in the list, is freed, the result, and the mark before it is flagged: `and the`. That ends the rules.
6. An `L` or `j` before `[`, with the sound after `L` and the sound before neither `@` nor `|`: the sound after's node is freed.
7. An `L` becomes `j`, and that ends the rules, when the sound before is `L` and the previous phoneme is a word mark `&` or `%`, or `[`; when the sound before is `@`, the sound after `D` or `E`, and neither the previous phoneme nor the next is a word mark `&` or `%`; or when the previous phoneme is `@` or `|`, the next `Z` and the one after next in class word mark or boundary: `tunnels`.
8. An `L` after a syllabic phoneme, before a next phoneme of stress 0: after `y`, before a word mark `&` or `%` or a phoneme in class break, it becomes `l`: `oil`. After `b` with `L` the node before the `b` in the list, before a vowel, it stays. Otherwise it becomes `j`: `feel`. Each ends the rules.
9. An approximant `p` after a word mark `&` or `%`, stressed, gets a glottal stop when the pitch isn't 0, the sound before is 0 and the rate is below 25.
10. An `N` before `[`, with a velar sound after, above rate 9, becomes `~`: `unconventional`. That ends the rules.
11. An `H` with a previous phoneme is freed, the result, when the rate is above 13, the previous phoneme is in class word mark, `p` isn't stressed and the sound before is a consonant; or when the next phoneme isn't a vowel, unless it is `Y` and the window's node after it is `b`: `told him`. That ends the rules.
12. Above rate 6, with a voiced sound before, an `H` becomes `d` and an `h` `W`, and the mark before it is flagged. That ends the rules.
13. An `L` or a nasal with a vowel the sound after: with `b` the previous phoneme, or the node before it in the list when the previous phoneme is in class word mark, if `b` is `z` after `J`, or `s` after `C`, in the list, a pause of 3 frames goes before `p`, or before the previous phoneme when that is in class word mark. The pause is the result, and that ends the rules.
14. With the sound before `S`, an `M`, an `N` or an approximant gets a pause of 3 frames before it, or of 4 frames for an `M` before `.`; with the sound before `Z`, of 2 frames. The pause is the result, and that ends the rules: `small`, `slow`.
15. **Stops.** For a stop `s`, these in turn:
    1. A `P`, a `K`, or a `T` not before `R`, after `S`, or after `[` with the sound before `S`, before a next phoneme with a stress, becomes `B`, `G` or `D`: `speech`. That ends the rules.
    2. A `D` or `T` before `R`, not after `S`, becomes `J` or `C`, and `s` with it: `drive`, `tree`.
    3. When the next phoneme is a word mark `&` or `%`, the sound after is `Y` and the current word's second value is 1, a pronoun: a `D` becomes `J`, and `s` with it: `did you`. A `T` becomes `C`, and `s` with it, when there is a previous phoneme, not `s`, and the node before it in the list isn't `C`; either way the sound after's node is freed going forward, the result: `what you`.
    4. A `C` not before `s`, or a `J` not before `z`, gets that fricative after it, stressed when `p` is, the result: `church`. That ends the rules.
    5. A `D` or `T` with a next phoneme goes through the alveolar rules (below); if one applies, that ends the rules.
    6. Before a phoneme in class boundary, below rate 25, a `p` goes after it, the result: `test.`

The **alveolar rules**, for a `D` or `T` `s`. If the previous phoneme is `~`, only the last applies. Otherwise, the first that applies:

1. A `T` with a stop the sound before, before a word mark `&` or `%` with a vowel the sound after, becomes `D`: `concept and`.
2. **Dropped:** `p` is freed, the result, and the mark after it is flagged, if it is a `T` before `C` or a `D` before `J`: `pitch`; from rate 14, with a consonant the sound after other than `R` and `W` and the previous phoneme not in class word mark, if it is a `T` with the sound before `F`, `S`, `P` or `K`, a `D` with the sound before `N`, or, from rate 25, either with the sound before `s`, `z` or `V`: `thousand things`; or from rate 10, if it is a `T` before `[` with the sound before `N` or `n` and the sound after `S`.
3. After a sonorant previous phoneme that is stressed, before a neutral phoneme with `N` the one after next, a `T` becomes `q` and a `D` a flap: `button`, `student`.
4. A `T` after a sonorant previous phoneme becomes `q`, and the mark after it is flagged, before a phoneme in class word mark with a sonorant the sound after that is neither syllabic nor `H`, or with the sound after `D` or `L`: `it really`, `not one`. A `T` after a vowel becomes `q` with the sound after `x`: `at the`.
5. Above rate 6, when the cursor's phrase isn't 0: after a vowel whose node before it in the list isn't `t`, before a next phoneme that isn't stressed, with the sound after `3`, `@`, `|`, `l`, `E` or `O`, and, when the one after next is `N`, the node after that in the list in class phoneme, `p` becomes a flap: `middle`, `monotone`. Otherwise a `T` after a sonorant previous phoneme, before a word mark `&` or `%`, or before `[` when the previous phoneme isn't `L`, with a vowel or `H` the sound after: the mark after it is flagged, its byte 0x18 becomes `T`, and it becomes `t` unless the previous word's second value is 0 or 6, the sound before is a nasal or `j`, and, if the sound after is `H`, the node before the previous phoneme in the list is stressed: `get it`, but `want it`.
6. A `D` after a previous phoneme not in class word mark, between a vowel the sound before and a vowel the sound after, before a next phoneme that isn't stressed, becomes a flap, unless the next is `A` with `T` the node after it in the list: `ready`.
7. A `T` before `u` after a word mark `&` or `%`, in a word of second value 10, `to`, with a vowel the sound before, when the node before the previous phoneme in the list is stressed or the sound before is high, and the node two after the next phoneme in the list is there and in neither class break nor class word mark, becomes a flap: `go-to-bed`, a split word.

### 13.5 The closing rules

With `c` the closing node, each in turn:

1. If the sound before is `@` or `|`, `c` is `@` or `|` and the previous phoneme is a word mark: a pause of 4 frames goes before `c`, the result, and `c` gets a glottal stop.
2. An `l` before `@` in the list becomes `j`, and an `@` goes before it, the result.
3. An `l` after `T`, `F`, `K`, `R`, `S`, `s` or `X`, or after `W` or `[` after `K`, becomes `@` of 4 frames, and a `j` goes after it, the result. Otherwise an `l` after `G`, `P`, `Z` or `B`, or after `D` after `N`, becomes `|` of 4 frames, and a `j` goes after it, the result: `little`, `table`.
4. A `j` after `@` after `t`, in the list, when the next phoneme is `.`, `&`, `%`, a space, `,` or `?`, becomes `l`, and the `@` is freed, the result.
5. A `~` before a node in class break or a space: a `p` goes after it, the result.
6. An `l` after `3` after `t`: an `R` goes before it, the result. Then an `l` after `3`, before a space, a node in class word mark or one in class break: an `R` of 5 frames goes before it, the result: `squirrel`.
7. If `c` is a vowel and there is a previous phoneme: when the sound before is `4` or `5`, an `R` goes after the previous phoneme, or before it when it is a word mark `&` or `%`, the result; when the previous phoneme is `g`, an `R` of 3 frames goes after it, the result.
8. An `l` after `Y`: the `Y` is freed, the result.

## 14. What the recorded runs don't reach

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
- The post-lexical rules returning no node (section 4.2, step 3): only rule 14 of section 13.3 could, freeing the window's first node, and a word's header comes before its phonemes in the window. The original would then read its closing node through a null pointer, and fault; the port skips the closing rules.
- The cursor's pitch at 0 (section 13.3, rules 1, 7 and 8, and section 13.4, rules 1, 2 and 9): the voices' pitches are 50 to 208 Hz, and a pitch command below 25 gives 50 Hz.
- No one after next where the post-lexical rules read it (section 13.3, rules 2, 13 and 15): an `e` before `K` or `G`, or a syllabic phoneme or an r-coloured one before `R`, with no phoneme after them in the window. The original reads a node through a null pointer, and would fault; the port takes its symbol as 0.
- The third node before the cursor's scan node missing (section 13.3, rule 4): the original would read it through a null pointer; the port takes its symbol as 0. The recorded runs read it 4 times.
- An `i` whose previous phoneme is at the list's start (section 13.3, rule 9): a word's header comes before its phonemes, and a word command or phonetic input's command before the header.
- A `Q` with no next phoneme (section 13.4, rule 2), and an `L` after `@` or `|` before `Z` with no one after next (rule 7): the original reads a node through a null pointer, and would fault; the port takes the symbol as 0, and makes the `Q` a space and leaves the `L`.
- Four decisions of section 13.4 the recorded runs don't tell apart: in rule 6, an `L` rather than a `j`, as rule 4 frees an `L` right before `[` and `L` first; in rule 7, a previous phoneme `[` with the sound before `L`, as rules 4, 6 and 8 free or change an `L` before `[` and `L`; in rule 15.3, a node `C` before the previous phoneme, as a `C` has its `s` after it by then; and in alveolar rule 5, a previous phoneme `L` before `[`, as rule 8 has made an `L` before a `T` `j` by then, the dictionary writes `j` there, and phonetic input's `[` starts a word.
- Two decisions the recorded runs don't tell apart: in rule 11 of section 13.3, a previous phoneme that isn't a phoneme, with `q` the sound before, as `q` comes only right before the schwa the rule tests; and in rule 17, a space the walk passes over before an r-coloured sound, as none of the texts tried puts one there.
- A reset of a stage that has run (section 5): the recorded resets are of a new speaking thread's state, whose values are all 0. A reset within a session drops the rest of the text, so a recorded run never ends ([pipeline.md](pipeline.md) §9).
- A function word after phonetic input's word mark `#`, code 11 (section 3.6): its header's second value, 59, becomes its vowels' class (section 4.2, step 2), and the prosody stage's rules for a function word's vowels read their list for class 59 past the end of their table ([prosody.md](prosody.md) §6.8), and the original faults. The port's prosody stage gives a class of 16 or more no rules.
- A stem whose first node isn't a letter (section 6.2, step 1). The input stage gives words only letters and `@`, and an affix leaves a letter first. For a symbol after `[` the original reads past the dictionary's index; the port doesn't find the stem.
- A search that ends twice without an entry (section 6.2, step 4): the original searches again, without end. A word with the code that has only homographs' verb entries would do it, and no word has: every homograph has an entry of another class.
- An ending of more than 30 symbols (section 6.3, step 1): a word has at most 29 letters.
- An entry with a `0`, or with only digits (section 6.4): the dictionary has none, and the data pack refuses one with only digits.
- A stem of more than 40 symbols in the user lexicon's lookup (section 7.1, step 1): a word has at most 29 letters.
- A `0` in a pronunciation of the user lexicon (section 7.1, step 4): the engine takes none, from an application or a file.
- A pronunciation of 88 symbols or more in the user lexicon (section 7.1): the original copies the pronunciation it finds into a buffer of 44 bytes on its stack. Up to 87 symbols the copy overwrites only the stem's copy, which the lookup no longer needs; from 88 it overwrites the return address, and the engine crashes. An application can add one; the port takes it whole.
- A pronunciation without phonemes for a stem before a suffix (section 7.1): letter-to-sound starts from the stem's end, now the node before the stem, the word's header (section 9.1, step 4). It passes the header and the nodes before it as a stem's phonemes (section 10.4), and once it has passed the window's first node the original reads a node through a null pointer, and faults. The port goes on as though it had reached the header, and converts the suffix's letters (section 10.1, step 2). The recorded runs have such pronunciations only for whole words, which don't crash it.
- A word whose first or last letter isn't `@` to `Z` (section 9.1): the original takes the lists of affixes of any symbol, reading past its tables for the others. The input stage gives words only letters and `@`, and the port gives the others none, as `@` and `Z` have.
- A stem's end not followed by `[` (section 9.3, step 6): an affix's `[` always follows it. Nor is a node after the `[` or the `C` missing, which the original would read through a null pointer; the port adds no `T` then.
- A sequence of more nodes than one more than its elements (section 8.1): the original would compare the items after the clause as elements. The data pack refuses one.
- A vowel `O` that may be reduced, after a word mark, before a consonant and `[` (section 11.2, step 3): neither the recorded runs nor 32,892 other English words have one.
- A vowel `k` or `r` that may be reduced (section 11.2, step 5): the rules of letter-to-sound that write them keep them full, and the stress patterns of the dictionary's words that letter-to-sound writes one in stress it.
- A letter of a word that isn't `@` to `[` (section 10.1, step 2): the input stage gives words only letters and `@`, and the lookup adds `[`. The original frees it without a rule; as the word's first letter, it could end the first conversion before the stem's start is passed, and leave a vowel for the letters after the stem to settle (section 10.4, step 1).
- An affix stress other than 0, 2 and 4 (section 10.1, step 1): no affix has one.
- The letters before a letter running out of the window (section 10.2, step 1): a word's header comes before its letters.
- Some decisions that neither the recorded runs nor the 32,892 other English words tell apart: ending's bit 1 at the word's last letter, which only the rule for an `H` after a word mark tests; the weight the letters after the stem start with (section 10.1, step 2); following after a reduced stem vowel that comes after a prefix's mark (section 10.4, step 3); the counts after an `E` made `Y` (section 10.3, step 3); and a `U` after a word mark from a rule that doesn't keep it full.
- A function word right after `.` or at the list's start (section 12.1, step 7, and section 12.2, step 1): a word command `z` comes before every written word's header and an `I` command after phonetic input, and a full stop written right before a word doesn't stay before it. Only a comma written without a space after it does (`go,to`).
- A word of class 14 whose first phoneme is `F` (section 12.2, step 1): `and`, `but`, `nor` and `or` start with `a`, `B`, `N` and `w`.
- A word of class 10 without a vowel before a break (section 12.1, step 4): every word of class 10 has one. The original would test a last vowel it never set; the port takes none.
- No next word at a function word's pass (section 12): the step converts a word before every pass. The original tests for one, then reads a node after none, and would fault; the port takes the next symbol and its stress as 0.
- `them`'s word mark after a space or a full stop (section 12.2, step 2): neither the recorded runs nor the texts tried put one there in a window the reductions reach.
- The rate class 3's reductions start above (section 12.2, step 1): up to rate 8 the vowels of its words are stressed still, or `@`, and the nodes after a word stop at a word command `z` or a word mark, so a start above any rate from 5 to 8 gives the same in the recorded runs.
- Dead code left out of the port: code 5 also makes the header type 3 (section 3.6), which it already is; case 1 of section 4.3 also clears the schwa, which nothing reads before the next schwa that starts the alternation sets it; the original copies at most 97 symbols of a stem's spelling (section 6.3), when the stem has at most 21; the walks to the shortest stem (section 9.1, steps 3 and 5) also stop after their third node or second node once they have met a vowel, which they have then stopped at already; settling a vowel `U` after `S` (section 11.1, step 1) also tests that there is a node after the vowel in the list, which there always is; the `Y` before a `U` gets the vowel's flag bit 0x20, which neither has: taking a node clears it ([pipeline.md](pipeline.md) §2.3); letter-to-sound tests that the nodes after and before a vowel are there (section 10.3, step 6), which they always are; code 0 of a stress pattern also makes `V` the vowel, which step 7 does; the function words' reductions also find the node after the next word's header when the header isn't in class phoneme, and don't use it; and rule 16 of section 13.3 also puts in its `R` when the sound before is `3` and a byte of the original's is `3`, which it never is: the byte holds the low byte of a pointer to a node, a stress or 0; the rules for a stop (section 13.4, rule 15) also make an `x` after `T` or `D` above rate 25 a `D`, but `s` is a stop there; the release of rule 15.6 also needs `s` not to be an affricate, but a `C` or `J` there comes before its `s` or `z`, not a boundary; and alveolar rule 5's second flap also makes `t` when the previous word's second value is 2, in a branch for 0, 6 and 11.
