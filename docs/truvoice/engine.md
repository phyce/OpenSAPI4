# TruVoice: the engine

How L&H TruVoice (`tv_enua.dll`) behaves at its interface, around the speaking pipeline the other specs describe. This spec covers what TextData makes of a text before it queues it for the speaking thread (section 1), and how the engine speaks the texts and reports what it speaks: the speaking threads, the audio's formats, the queue to the destination, the events and where they land, and pitch, speed and volume (section 2); the modes, the enumerator, Select, ModeGet and Find (section 3); `Inject` and the other attributes (section 4); pausing, resuming and resets (section 5); `Phoneme` (section 6); and the user lexicon (section 7).

**In short.**
- TextData takes text, IPA and engine phonemes, from the wide interface and the ANSI one (which takes no IPA). Text is converted to code page 1252 with Windows' best fit: `ā` becomes `a`, `€` the byte 0x80, a character the code page lacks `?`. Engine phonemes go between `ESC[1I` and `ESC[0I`, and IPA is converted to engine phonemes between them.
- A text with a notification sink gets a word mark, `ESC[b0;b1;b2;b3z`, before each word: the bytes of the word's offset, low first, doubled for the wide interface.
- With the tagged flags, the tags become escape sequences: `\Pit=200\` is `ESC[100p`, `\Mrk=5\` `ESC[5;0;0;0i`, `\Pau=500\` `ESC[50s`. A tag is known by the three letters after its backslash, so `\Emphasis\` is `\Emp\`. `\Vce=Speaker=Wanda\` finds the best voice among the ten, `\RmS=1\` spells the rest of the text, and unknown tags are dropped.
- `\Ctx="..."\` sets the engine's context, which lasts from text to text. In a tagged text, the characters 128 and up become spaces, but `’ £ ¥ © ®`.
- In the address, e-mail and spreadsheet contexts, the original pre-formats the text. The port doesn't, so it differs from the original there.
- Some tags make the original read past the text's end or loop for ever. The port does something harmless there and section 1.6 says what.
- A speaking thread speaks the queued texts one after another and ends when none is left; the next text starts a new one. The audio is 11025 or 8000 Hz, 16 or 8 bits, whichever the destination takes first.
- The thread queues the audio and the events for the engine's window, which gives them to the destination when the thread **flushes** the queue: after each visual, word and bookmark event, and after a hand-over of audio once seven buffers have gathered at the thread's start, then after every one. A text's start and end, and a change of attribute that a tag makes, are reported where the window had flushed when they were posted: usually before the last audio queued.
- Visual events report each phoneme's IPA character and mouth shape. Pitch is 50 to 400 Hz and speed 50 to 250 words a minute.
- The engine has ten modes, one for each voice: eight men, Peter, Sidney, Eddie, Douglas, Biff, Amos, Melvin and Alex, and two women, Wanda and Julia. Select takes a mode's GUID. Find scores each mode on the names, the speaker, the gender and the age wanted, by the caller's ranks, and gives the first with the highest score.
- `Inject` changes the engine at once: `\Pit=\`, `\Spd=\`, `\Vol=\`, `\Chr=\`, `\Vce=Speaker=\` and `\Rst\` set its attributes, character and voice, and a string that is exactly `\gain=1\`, `\gain=2\` or `\gain=3\` the gain of the next speaking thread. The real-time attribute isn't implemented.
- `AudioPause` holds the audio, and the speaking thread runs on until ten items wait in its queue. `AudioResume` goes on. `AudioReset` ends the thread and drops the texts queued: each text not done gets a `TextDataDone` with `TTSBNS_ABORTED`.
- `Phoneme` converts a text to engine phonemes or IPA without speaking it, on a pipeline of its own at voice 0 and the default attributes: each word's mark, its phonemes with each stressed vowel's stress after it, and the punctuation that ends a sentence. `Hello world.` is `&HeLO1&W31jDp.`, or `&hɛloʊ1&wɚ1ɫd.` in IPA. With flags 1, the tags go over as they are and do nothing.
- The user lexicon holds up to 5,000 words of letters, with their pronunciations in engine phonemes, and every engine looks words up in it before its dictionary. `Add` takes a pronunciation in text, which `Phoneme` converts, in IPA or in engine phonemes, `Get` gives it in engine phonemes or IPA, and `Remove` removes a word. Spaces and quotation marks go from the words, but every second one of a run. `\Prn=` adds or removes a word when the normalizer reaches it. The original keeps the lexicon in a file, which the port reads and writes in the same format.

**Exactness.** Ported from the original's code, and checked against it on every text it queued for the speaking thread in 1,367 recorded runs: the 24 runs of TextData's own set, which test every tag with values in and out of its range, word marks, tags in words, emphasis before words and at the end, spelling, voices, contexts, the characters 128 and up, best fit, IPA with every rule, engine phonemes, the ANSI interface, the character sets it refuses and sizes that leave out the NUL; and the texts of the input, normalizer and stage sets. Of their 2,376 texts, 2,361 were identical to the original's. Three were refused, as the original refused them. The other 12 have characters in the address, e-mail or spreadsheet context, whose pre-formatter isn't ported. Four runs that call `Inject` were left out here, as `Inject` changes what the conversion reads; section 4's check speaks them.

Section 2 was checked against the original's speaking in 1,760 recorded runs, every run of the sets but those that need `Inject` (section 4), pauses and resets (section 5) or the user lexicon (section 7), the 8 runs with the pre-formatter's contexts and 5 runs whose calls land wherever the speaking thread happens to be. In all of them the audio was the original's, 659,067,040 samples, in the format the destination took; the texts started and ended in the original's order, 5,104 times; and the 75,234 words and bookmarks and 573,209 visual events came at the original's samples, with its IPA characters, engine phonemes and mouth shapes. Where a text's start and end and an attribute change land varies from run to run in the original (section 2.4): in 34 runs that it spoke ten times each, the port put each of the 101 at the place most of its runs had it.

Sections 3 and 4 were checked against the original's interface. The ten modes' information was the original's, every field. 4,180 recorded runs of calls on new enumerators gave the original's results, modes and positions in all 14,748 calls: 5,054 Next, 4,390 Skip, 1,008 Clone, 891 Reset, 501 Select with what ModeGet then gave, and 2,904 Find, of which 2,249 found a mode, with names in every case, NULs, the default ranks, ranks that count for nothing, and ranks that make the scores wrap, which pin each gender's and age's score. `Inject` was checked in 107 runs: the 8 runs of the other sets that call it before their texts, and 99 runs of its own set, which test the gain, every tag with values in and out of its range, empty values, `\Rst\` after changes made by `Inject`, by the attribute calls and by texts, the context it resets, and voices. In the 99 runs, the 583 calls (150 `Inject`, 107 Speak, 92 PitchGet, 85 SpeedGet, 82 VolumeGet, 22 ModeGet, 21 PitchSet, 14 SpeedSet, 10 VolumeSet) gave the original's results and values, between the original's 188 attribute changes and texts' starts and ends, in its order. In all 107, the audio was the original's. Five runs of the set were left out: the two with an empty `\Vol=\` (section 4.4), the two that call RealTime and QueryInterface, which the port doesn't have (section 4.3 gives what they gave), and one with the pre-formatter's contexts.

Section 5 was checked against the original in the 146 runs of the pause set, which it spoke three times each. Their destination takes only so many bytes, from none to 310,000, so that the audio stops, and the runs pause there, make calls and resume or reset; in the four formats, with calls of every kind during the pause, texts queued during it, a second pause, the thread ending during it, resets without a pause, a pause before a text and before none, the errors, and resets in a text too long for the tokenizer to read at once and in one too long to feed at once. In all of them the audio was the original's, 9,442,640 samples; the texts started, ended and were aborted in the original's order, 289, 202 and 194 times; the 2,640 words and bookmarks and 9,271 visual events came at the original's samples; and the 416 calls besides TextData (157 `AudioPause`, 96 `AudioReset`, 66 `AudioResume`, 41 PitchSet, 17 SpeedSet, 11 VolumeSet, 8 `Inject`, 18 attribute gets and 2 ModeGet) gave the original's results, with the destination's position at each pause, resume and reset, between the original's 158 attribute changes and texts' starts and ends, in its order. Where the starts, ends and attribute changes land varies from run to run, as in section 2.4: of the 2,529 in the three runs, 2,212 landed at the port's place and 317 later, none earlier.

Section 6 was checked against the original in the 135 runs of the phoneme set, which call `Phoneme` 792 times: on texts with and without tags, tags closed and unclosed, escaped backslashes, spaces after tags, characters 128 and up in tags and out of them, the IPA conversion's tags past the text's length, long texts (around the 400 characters a pump takes, the input ring's 4,096 bytes and the command ring's), characters the escape parser drops, escape sequences, phrases broken off at the scan's limits, NULs in the counted text, texts without their NUL, stress digits after consonants in phonetic input, and `Phoneme` before, between and after speaking. The 28 calls with character sets the interfaces refuse gave `E_INVALIDARG` and left the size as it was. The other 764, 634 wide (361 to engine phonemes, 273 to IPA) and 130 ANSI, 394 with flags 1 and 370 without, gave the original's phonemes and sizes, NULs included, but in one run, where the original reads past its listing (section 6.5): there the sizes were the original's. The 7 texts spoken among them gave the original's audio.

Section 7 was checked against the original in the 81 runs of the pronounce set. 56 of them start from a file: with invalid entries, entries of 39, 40 and more than 100 characters, CR and LF, a Ctrl+Z between entries and in a field, entries without their last NULs, no entries, more than 5,000, and 2 to 3,000 words each given twice, which test where the sort leaves them and which one the search finds. The runs make 14,916 calls, 10,319 Add (10,221 wide and 98 ANSI), 4,497 Get (4,419 and 78) and 100 Remove (75 and 25): words with spaces and quotation marks, characters 128 and up and characters code page 1252 lacks, empty words, pronunciations in text, IPA and engine phonemes with every symbol and invalid ones, character sets the interfaces refuse, sizes around the size needed, and the lexicon filled to its capacity and past it. They speak 22 texts with 336 `\Prn=` tags, which add, remove and re-add words in text and engine phonemes, with parts of 18 to 25 characters, before and after the words they pronounce. All the calls gave the original's results: 10,239 `S_OK`, 33 `LEXERR_INVALIDTEXTCHAR`, 27 `LEXERR_INVALIDPRONCHAR`, 9 `LEXERR_ALREADYINLEX` and 11 `E_INVALIDARG` for Add, 4,228 `S_OK`, 159 `LEXERR_NOTINLEX`, 100 `LEXERR_PRNBUFTOOSMALL` and 10 `E_INVALIDARG` for Get, and 66 `S_OK` and 34 `LEXERR_NOTINLEX` for Remove. The Gets gave the original's sizes needed, parts of speech and buffers, every byte up to 16 past the size, but for the failing ANSI Gets (section 7.5). The 6 files with invalid entries gave the message box once each, and every run's file, the one the original saved or else the one it read, was the port's, byte for byte. The texts' audio was the original's, 685,080 samples, with the 118 words and 610 visual events at the original's samples. The lexical stage's 25 runs with user lexicons, whose words a wide Add gives the engine before their texts ([lexical.md](lexical.md) §7), gave the original's audio too, 2,412,410 samples, with the 333 words and 2,065 visual events at the original's samples.

## 1. TextData's text

TextData converts a text in up to four steps, then queues the result with its notification sink, if it has one:
1. the character set (section 1.1);
2. for text with a notification sink, the word marks (section 1.2);
3. with the tagged flags (exactly 1), the tags (section 1.3), and then the contexts and the characters 128 and up (section 1.4). With any other flags, it pre-formats text in the address, e-mail and spreadsheet contexts instead (section 1.4).

Each step works on the text up to its first NUL and gives the result with one NUL. A text that no step touches keeps all its bytes, NULs and what follows them included.

### 1.1 The character sets

The wide interface takes three character sets. Any other gives `TTSERR_INVALIDCHAR` (0x80040208), and so does a text without data. A notification sink with an interface other than `ITTSBufNotifySink` gives `E_NOINTERFACE` (0x80004002). The text is the whole `SDATA`: a NUL the caller counts is one of its characters, and what follows the bytes it counts isn't read. A text of no bytes is taken, and queued as a NUL.
- **Text** (`CHARSET_TEXT`): each UTF-16 code unit becomes a byte, as Windows' `WideCharToMultiByte` does for code page 1252 with best fit. A code unit the data pack's `bestFit` table lists becomes its byte, NUL stays NUL, and every other code unit becomes `?`, each half of a surrogate pair too. One NUL follows.
- **Engine phonemes** (`CHARSET_ENGINEPHONETIC`): converted in the same way, into a buffer 9 bytes longer than the text: `ESC[1I`, then the bytes, then `ESC[0I` and a NUL written at the first NUL. So `ab NUL cd` is `ESC[1I ab ESC[0I` and 5 NULs, `cd` overwritten.
- **IPA** (`CHARSET_IPAPHONETIC`): section 1.5.

The ANSI interface takes text and engine phonemes; IPA gives `TTSERR_INVALIDCHAR`. Windows converts the text to UTF-16 and the wide interface's conversion converts it back, which gives each byte of code page 1252 back as it was. The ANSI interface also turns off the doubling of the word marks' offsets (section 1.2).

### 1.2 Word marks

Only text with a notification sink gets word marks. Each is `ESC[b0;b1;b2;b3z` and a space: the four bytes of the word's offset in the converted text, low first, in decimal. The offset is the position of the word's first byte, doubled while the engine **doubles offsets**. A new engine doubles them. The ANSI interface's TextData turns doubling off, and a text with word marks turns it on again after its marks. So the wide interface's marks are the offsets of UTF-16 code units in bytes, as long as each character is one code unit, and the ANSI interface's are byte offsets. After an ANSI text without a notification sink, the next wide text with one isn't doubled either.

A **space** is a byte from 9 to 13 or 32: the C library's classes, in which no byte of 128 and up is a space, letter or digit.

**Without the tagged flags**, each run of bytes that aren't spaces is a word. The text is read as: its spaces, copied; a mark; the word, copied. This repeats to the end, so trailing spaces get a mark after them too: `a ` is `ESC[0;0;0;0z a ESC[4;0;0;0z `.

**With the tagged flags**, the text is read as follows, again to the end:
1. Spaces are copied.
2. A **tag**, a backslash not followed by another, is copied up to its closing backslash, or to the text's end, and gets no mark. If it starts `\Ctx=` (case aside) and the next nine characters are `"address"`, the text is in the address context from there; any other `\Ctx=` ends it. The engine's own address context starts it. A tag whose name starts with `emp` gets its word's mark in front (below). So does a `\Ctx=` tag whose value starts with `emp`.
3. Anything else starts a word. Its mark is written unless the text is in the address context, and the word is copied up to the next space. A word can hold tags and `\\`, which isn't a tag: `a\Mrk=5\b` is one word.

An **emphasis tag** puts the mark of the word after it in front of itself. Spaces after the tag are passed over, and so are tags other than `\\`. At the first other character, the word's mark is written, then the emphasis tag and the tags passed over, without their spaces, then the word up to its space. `x \Emp\\Pit=100\ b` becomes `ESC[0;0;0;0z x ESC[34;0;0;0z \Emp\\Pit=100\b`: the second mark is `b`'s, at offset 17. An emphasis tag with no word after it is dropped with everything after it.

### 1.3 Tags

With the tagged flags, the text is copied with each tag converted:
- `\\` is kept as `\\`. Section 1.4 makes it one backslash.
- A backslash among the last two characters of the text ends the text before it. So does a tag without a closing backslash.
- A tag of fewer than three characters between its backslashes is dropped.
- A tag is known by its **name**: the three characters after the backslash, case aside, so `\Emphasis\` is `\Emp\` and `\rst=1\` is `\Rst\`, but `\Reset\` is unknown (its name is `res`). Some tags also need `=` as the fourth character, so `\spd=150\` is speed and `\Speed=150\` an unknown tag. Their **value** runs from the fifth character to the closing backslash.
- Before what a tag writes, a **space** is written when the tag starts at the third character or later and the character before it isn't a space: `a\Mrk=5\b` is `a ESC[5;0;0;0i b`. The tags marked "space" below write it, even when they then write nothing else.
- Unknown tags are dropped without a space: `\Eng\`, `\Com=\`, `\RPit=\`, `\gain=2\` and every tag whose name isn't one below.

| Name | `=` | What it writes |
|---|---|---|
| `ctx` | No | Space, then the tag as it is, for section 1.4 |
| `emp` | No | Space, `ESC[0e` |
| `rst` | No | Space, then the reset: the saved voice, if a `\Vce\` saved one (below), as `ESC[vV`, which unsaves it; `ESC[14F`; the tag `\Ctx="Unknown"\`; `ESC[cc` with the mode's character (0); `ESC[sr` with the mode's speed; `ESC[pp` with half the mode's pitch; `ESC[aa` with the attenuation of full volume (0). It turns spelling off |
| `mrk` | Yes | When the value is all digits (or empty): space, the number (below), and if it isn't 0, `ESC[b0;b1;b2;b3i` and a space, the number's four bytes, low first |
| `chr` | Yes | Space; then the value up to a comma or a backslash, at most 19 characters, case aside: `"normal"` writes `ESC[0c`, `"whisper"` `ESC[1c` and `"monotone"` `ESC[2c`, quotes included. Any other value writes nothing more |
| `pau` | Yes | When the value is all digits: space; with two digits or more, the pause (below) |
| `pit` | Yes | When the value is all digits: space; the number, 0 counting as 50 and 65535 as 400; from 50 to 400, `ESC[pp` with half of it |
| `spd` | Yes | When the value is all digits: space; the number, 0 counting as 50 and 4294967295 as 250; from 50 to 250, `ESC[sr` with it |
| `vol` | Yes | When the value is all digits: space; the volume, the value's number (empty: the last volume, 65535 at first) cut to 16 bits, as an attenuation ([synthesizer.md](synthesizer.md) §5.4), 16 for a volume below 80: `ESC[aa` |
| `vce` | Yes | Space, then the voice (below) |
| `rms` | Yes | With a value of one character: `1` turns spelling on, `0` off. No space |
| `pro` | Yes | Space; with a value of one character, `1` writes `ESC[14F` and `0` `ESC[14N` |
| `prn` | Yes | Space, then the pronunciation (below) |

**The number** of `mrk`, `pau`, `pit` and `spd` is read as the C library's `sscanf` reads `%u`: 32 bits that wrap, so `\Mrk=4294967301\` is mark 5. An empty value leaves the last number, which each of them changes: `mrk` leaves 0, `pau` its rest (below), and `pit` and `spd` the number after their corrections, so `\Pit=0\\Spd=\` gives speed 50. At a text's start the original's number is whatever its stack holds; the port's is 0. `prn` also leaves 0.

**A pause** is the number divided by 10, in hundredths of a second, written in chunks. While it is over 255, and for at most nine chunks, a chunk of 255 is taken off; what's left is the **rest**. The rest's command is written first, then the chunks': `\Pau=5000\` is `ESC[245s ESC[255s`, `\Pau=25600\` `ESC[265s` and nine `ESC[255s`. The escape parser keeps each value to 255 ([input.md](input.md) §1.4). The original prints each chunk into a buffer of 4 bytes and writes at most its first 4 characters. A rest of 4 digits or more overflows the buffer into the text's length: its fifth and later digits, and a NUL, replace the length's low bytes, one byte each. The text then ends where the new length says. `\Pau=32950\` (rest 1000) clears the low byte, so a text of 300 bytes ends at byte 256 and a shorter one right after the tag. `\Pau=1000000\` (rest 97705) makes the length 53, the code of its fifth digit, and `\Pau=1023000\` (rest 100005) 13,616 (0x3530, its fifth and sixth digits), which cuts a text of 20,000 bytes there.

**Spelling**, which `\RmS=1\` turns on, changes how the text between tags is copied:
- An escape character is copied with what follows, up to and including the next letter. A backslash among what it copies isn't a tag here, but section 1.4 reads it as one.
- A run of letters and digits is spelled: each letter followed by `.` and a space, each digit by a space, and a comma after the run. `ab1` is `a. b. 1 ,`.
- Anything else is copied as it is.

**A voice**: `\Vce=` reads keys and values separated by commas, `key=value`, until the closing backslash:
- a key is at most 18 characters before its `=`, case aside;
- a value, after an opening quote if there is one, is at most 19 characters up to a comma or a backslash; a closing quote is dropped, case doesn't count. A value still of 19 characters then passes over the rest up to a comma or the backslash. Otherwise the next key starts where the value stopped, so `Speaker=abcdefghijklmnopqr"tu,Gender=Female` reads the key `tu,Gender`, which counts for nothing.

Only `speaker` and `gender` count: `male`, `female` or `neutral`; another value leaves the gender the last one, neutral at first. The other keys the tag knows, `language`, `accent`, `dialect`, `age` and `style`, and unknown keys, are read and change nothing. The first of the keys that count ranks 10, and each later one 4 less; a key given twice keeps its last value and rank. Each of the ten modes scores the ranks of the keys it matches: the speaker's name, case aside, exactly, so `Wan` and `Wan`-soft-hyphen-`da` match nobody; and the gender. The first mode with the highest score is the voice, written `ESC[vV`. So `\Vce=Speaker=Nobody\` and `\Vce=\` choose Peter, `Gender=Female` chooses Wanda, `Gender=Female,Speaker=Peter` Wanda, `Speaker=Peter,Gender=Female` Peter, `Gender=Male,Speaker=Wanda` Peter, and `Gender=Female,Gender=Neutral` Peter, as no mode is neutral. The first `\Vce\` in the process saves the engine's voice for `\Rst\`. The original asks SAPI's enumerator for the best mode of every engine on the system. TruVoice's modes always win, because the tag also asks for TruVoice's engine and product, so the port looks only among them.

**A pronunciation**: `\Prn=word=pronunciation\`, or `\Prn=word=//phonemes\` for engine phonemes up to a `/`. Each part is at most 19 characters, and a tag without `=pronunciation` removes the word. The tag writes `ESC[b0;b1;b2;b3n` and a space with the record of the word and its pronunciation, which the normalizer gives the user lexicon (section 7.3). The original writes the record's address. The port numbers the records of an engine from 1 and writes the number.

### 1.4 Contexts and the characters 128 and up

After the tags, the text is read again, in **segments** that the `\Ctx=` tags end:
- A `\Ctx=` tag (`=` as fourth character; the tag `\Ctx\` alone is dropped) writes the segment before it, then reads its value up to a comma or a backslash, at most 19 characters, case aside. `"email"` and `"e-mail"` make the engine's **context** the e-mail one (4), `"spreadsheet"` the spreadsheet one (6), `"address"` the address one (1), and any other value none (0). The quotes are part of the value. The tag writes `ESC[nX`, whose letter the escape parser knows as no command.
- `\\` becomes `\`. Any other tag is dropped. A tag without a closing backslash ends the text.

The tags left are `\Ctx` tags, unless spelling copied a backslash after an escape character (section 1.3). Such a backslash starts a tag here too: `\RmS=1\a` ESC `[1\Bcd` is `a. ,` ESC `[1`, as its tag isn't closed, and ESC `[1\C\RmS=0\tx="Address"\\` switches to the address context.
- At the text's end, the last segment is written.

The context lasts from text to text, and a new engine has none. A segment of text, but not of IPA or engine phonemes, is written as follows:
1. Its characters 128 and up become spaces, but for `’ £ ¥ © ®` (0x92, 0xA3, 0xA5, 0xA9, 0xAE).
2. In the address and e-mail contexts, it is written pre-formatted between `ESC[7F ESC[8F` and `ESC[7N ESC[8N`, even when it is empty: spelling words of capitals and reading abbreviations are off while it is spoken ([normalizer.md](normalizer.md) §5.1). In the spreadsheet context it is written pre-formatted. Otherwise it is written as it is.

Without the tagged flags, text in the address, e-mail or spreadsheet context goes through the same two steps as one segment.

The **pre-formatter** reads addresses, e-mail and spreadsheets: about 300 KB of the original's code, reached only in these contexts. It isn't ported, so the port writes a segment as it is in these contexts too, which is right for an empty one. Even a segment of one space shows the difference: the original writes it as CR CR LF in the address and e-mail contexts, and as a space and CR CR LF in the spreadsheet one.

### 1.5 IPA

IPA text is converted to engine phonemes between `ESC[1I` and `ESC[0I`, a character at a time. At each character, the data pack's IPA rules (`ipaRules`) give the longest sequence they know that starts there, and its engine phonemes. A character that starts none gives nothing: `aɪ` is `I` and `aɪɹ` `5`, but `a`, `e` and `o` alone, `x` and NUL are nothing.

With the tagged flags, a tag is copied from its backslash to its closing backslash, or to the end. Its characters above 255 are dropped. `\\` is dropped altogether. The result is cut at its first NUL. A NUL inside a tag, or a backslash as the text's last character, which copies the NUL TextData puts after its copy of the text, therefore drops the rest, `ESC[0I` included. The tags and contexts are then converted as in sections 1.3 and 1.4, but the characters 128 and up stay as they are.

### 1.6 What the recorded runs don't reach

- **The number at a text's start.** An empty `\Mrk=`, `\Pit=`, `\Spd=` or `\Pau=` before any other number in the text reads what the original's stack holds; one recorded run read 1. The port reads 0.
- **Reading past the end.** The original reads past the text's end, into whatever memory follows, in four places. The port ends the text there instead:
  - after a pause's rest that makes the length longer than the text: one of 6 digits or more, or of 5 digits in a text shorter than the code of its fifth digit (48 to 57). A rest of 8 digits or more also overwrites the pointer to the buffer of the converted text, which a later `\Vce=` in the same text writes through;
  - after an emphasis tag without a closing backslash;
  - in spelling, after an escape character with no letter after it;
  - in a `\Prn=` tag whose word of 19 characters is followed by neither `=` nor `\`, where the original's pronunciation is whatever memory held (the port's is empty).
- **`\Vce=` keys without `=`.** A key that reaches the closing backslash, or 19 characters, without an `=` is read again for ever, and TextData never returns (checked: `\Vce=Speaker\` never returns). There are two exceptions: a key starting with a comma, and a key that stops at the text's 20th character. The port ends the tag there and chooses the voice as for the keys before.
- **The pre-formatter** (section 1.4).

## 2. Speaking

### 2.1 Speaking threads and sessions

TextData queues the converted text with its notification sink, if it has one, and starts a **speaking thread** if none runs ([pipeline.md](pipeline.md) §8). The thread speaks the queued texts one after another, each when the last has gone through, a text queued while it runs included, and ends when none is left. The next text starts a new thread, with a new pipeline. A **session** is what one thread speaks.

When the thread takes a text, it posts a message to the engine's window that the text started ([input.md](input.md) §6); at the text's end mark, a message that it is done ([pipeline.md](pipeline.md) §4.1). The window reports both to the text's notification sink, with where the audio stands then (section 2.4); a text without a sink reports nothing, but its audio and its visual events are spoken as any other's.

What a text changes lasts into the next one, in the same session or the next: the engine's attributes, which the commands also set ([pipeline.md](pipeline.md) §4), the voice, the context (section 1.4) and the doubling of the word marks' offsets (section 1.2).

The port's engine has no thread of its own. `Queue` converts and queues a text, `Run` runs a speaking thread over the texts queued, and `Speak` is both. Texts the original's application queues while a thread speaks, the port's caller queues before `Run`. Whether such a text joins the session in the original depends on whether the thread has ended by then, and a new session starts with new noise and filters in the synthesizer ([pipeline.md](pipeline.md) §8.5).

Some of what lasts the original keeps for the whole process, so that every engine's texts change it: the prosody stage's shared state ([prosody.md](prosody.md) §3.1), the voice `\Rst\` goes back to (section 1.3) and the user lexicon (section 7). The port keeps it on `Data`, which every engine made from it shares. `Data.NewProcess` gives a `Data` with the same tables and the rest as a new process starts with, its user lexicon a copy of the words the old one has, as a new process reads the file the old one saved.

### 2.2 The audio's formats

The engine offers its destination four formats, mono PCM, in this order, and takes the first the destination accepts: 11025 Hz with 16 bits, 11025 Hz with 8 bits, 8000 Hz with 16 bits, 8000 Hz with 8 bits. If the destination accepts none, selecting the mode fails with `TTSERR_WAVEFORMATNOTSUPPORTED`. The pipeline renders at the format's sample rate ([pipeline.md](pipeline.md) §8.1), always in 16 bits; in 8 bits, each sample is the high byte of the 16-bit one plus 128.

### 2.3 The queue and the window

The thread doesn't write to the destination. It queues **items** for the engine's window: the audio buffers it hands over ([pipeline.md](pipeline.md) §8.4), the visual events ([parameters.md](parameters.md) §6), the word and bookmark events ([pipeline.md](pipeline.md) §4.1), and an end mark at its end ([pipeline.md](pipeline.md) §8.5). To have them written, it posts the window a message, which **flushes** the queue: the window takes the items in order, writes each buffer to the destination, and hands each event to the destination as a bookmark, which comes back when the destination plays to it. The destination of the original's recorded runs plays at once, so each event comes back before the next item; the port's sink gets the audio and the events in that order.

The thread flushes the queue:
1. after queuing a visual, word or bookmark event, at once;
2. after a hand-over, as follows. A thread starts **prebuffering**: a hand-over then flushes only when more than six items were waiting in the queue before its buffer, and stops prebuffering. After that, every hand-over flushes;
3. at its end, after the end mark.

The thread's other messages, a text started or done (section 2.1) and an attribute changed (section 2.5), go to the window in the same order as the flushes, which it handles in the order they came. Behind a destination without room, and behind a pause, the window holds the audio and the queue fills (section 5.2).

### 2.4 Where the events land

An event's **position** is how much audio the destination had been given when the window reported it, in samples of the engine's format, counted from the engine's first text.
- A visual, word or bookmark event comes back from the destination after the audio queued before it: its position is where the last hand-over before it ended ([synthesizer.md](synthesizer.md) §5.3). It is the same in every run.
- A text's start and end, and an attribute change a tag makes, stand where the window had flushed the queue when the thread posted them. So a text is often reported done before the audio of its last word: in Peter's `Hello there, this is a test.`, at sample 20,020 of 26,070, at the last visual event, as the buffers after it waited in the queue.

The port's window handles each message as soon as it is posted. The original's is the application's window: it handles a message when the application's message loop gets to it. It also flushes the queue when the destination reports free space, which the recorded runs' destination does after 10 ms without messages. So where the original reports these events varies from run to run, always at or after the port's place:
- In the first session of an application whose loop waits for messages, the window mostly keeps up: in each of the 34 runs that the original spoke ten times, the port's place for each event was the one most of its runs had (from 5 of 10 to 10 of 10).
- In later sessions it lags by varying amounts: in three sessions of one text, the second's end landed at the port's sample in 2 of 10 runs, and at four later ones in the others.
- While the application makes other calls, nothing is flushed. Ten texts queued at once let the thread run ahead, so that more than six buffers gathered and prebuffering stopped at the start: in all ten runs, the texts' starts and ends landed later than the port's, mostly where they land when every hand-over flushes.

### 2.5 Pitch, speed and volume

A new engine has its voice's pitch and speed ([pipeline.md](pipeline.md) §4, Peter 85 Hz and 150 words a minute) and volume 65535.
- **Pitch** is in Hz. Setting it to 0 sets the lowest, 50, and to 65535 the highest, 400. Any other value outside 50 to 400 gives `E_UNEXPECTED` (0x8000FFFF) and changes nothing.
- **Speed** is in words a minute. 0 sets the lowest, 50, and 4294967295 the highest, 250. Any other value outside 50 to 250 gives `E_UNEXPECTED`.
- **Volume** takes the value's low word, the left channel's, without a check. Reading it gives that word in both halves.

Each change posts the window a message that the attribute changed, which it reports to the engine's notification sinks. The speaking thread takes the new value at its next step ([pipeline.md](pipeline.md) §8.3). The commands that change the pitch, speed or volume also set the engine's attribute and post the message ([pipeline.md](pipeline.md) §4), the pitch and the speed from the prosody stage and the volume from the parameter generator's, so in stream order: its place is that of section 2.4. The port tells its `AttributeChanged` function of a call's change at once, and of a command's at that place.

### 2.6 Visual events

Each visual event the parameter generator queues reports a phoneme symbol to the engine's notification sinks, from the data pack's `engine.json`:
- the IPA character: `U` gives `u`, the space and the symbols without one give 0;
- the engine phoneme, the symbol, sign-extended;
- the hints, always 2 (`TTSNSHINT_STATEMENT`);
- the mouth shape (`TTSMOUTH`) of the 53 symbols that have one, the space's included, or none.

Visual events belong to no text: they go to the engine's notification sinks, not to a text's, and texts without a sink have them too. The port gives them to the sink of `Run`, with text −1.

### 2.7 What the recorded runs don't reach

- **Hand-overs of 0 or 2 samples.** The original's queue refuses an empty item, and the thread then waits for the window for ever; it takes an item of 4 bytes for an event. None of the 539,095 hand-overs of the recorded runs had fewer than 3 samples.
- **ANSI notification sinks** get 0 for every visual event's IPA character. The port has one kind of sink.
- **Symbols outside the table.** No phoneme symbol is 0 or 128 and up. For 0, the original would read the mouth shape past its table's end; the port gives no IPA character and no mouth shape for such symbols.

## 3. Modes

### 3.1 The ten modes

The engine has ten modes, one for each of its voices, in the order of the voices. Their information (`TTSMODEINFO`) is in the data pack's `modes.json`:

| Mode | Speaker | Gender | Name |
|---|---|---|---|
| 0 | Peter | male (2) | `Adult Male #1, American English (TruVoice)` |
| 1 | Sidney | male | `Adult Male #2, …` |
| 2 | Eddie | male | `Adult Male #3, …` |
| 3 | Douglas | male | `Adult Male #4, …` |
| 4 | Biff | male | `Adult Male #5, …` |
| 5 | Amos | male | `Adult Male #6, …` |
| 6 | Melvin | male | `Adult Male #7, …` |
| 7 | Alex | male | `Adult Male #8, …` |
| 8 | Wanda | female (1) | `Adult Female #1, American English (TruVoice)` |
| 9 | Julia | female | `Adult Female #2, …` |

Mode *n*'s ID is `{CA141FD0-AC7F-11D1-97A3-00600827300n}`. Every mode has the engine ID `{CA141FD0-AC7F-11D1-97A3-0060082730FF}`, the manufacturer `Lernout && Hauspie Speech Products` (the ampersand doubled), the product `TruVoice`, the language 1033 (US English) without a dialect, the style `Business`, the age 30 (adult), the features 1023 (any word, volume, speed, pitch, tagged text, IPA, visual events, word positions, optimized for the PC and for the phone), the interfaces 15 (`ILexPronounce`, `ITTSCentral`, `ITTSDialogs`, `ITTSAttributes`) and no engine features.

### 3.2 The enumerator

`ITTSEnum` goes through the ten modes in order from its **position**, 0 for a new enumerator:
- **Next** writes up to the count asked for of the modes from the position, moves past them, and gives how many it wrote. It gives `S_OK` when it wrote the count, and `S_FALSE` otherwise. At the end it writes none and gives `S_FALSE`, even for a count of 0; before the end, a count of 0 gives `S_OK`.
- **Skip** moves past the count of modes. When the position plus the count, in 32 bits that wrap, is past the end, it moves to the end and gives `S_FALSE`. So at position 1, skipping 0xFFFFFFFF wraps to position 0 and gives `S_OK`.
- **Reset** goes back to position 0, and **Clone** gives a new enumerator at the same position.
- **Select** makes an engine with the mode whose ID it is given, whatever the position. A mode ID that isn't one of the ten gives `TTSERR_INVALIDMODE` (0x80040206). The engine speaks with the mode's voice, its pitch and its speed (section 2.5), in the first of its formats the destination takes (section 2.2).

The port's `Data.Enumerate` gives an `Enumerator` with `Next`, `Skip`, `Reset`, `Clone` and `Find`. `Data.Select` takes the mode ID as a string, `{…}` with the digits in either case, and `Data.NewEngine` the voice's number.

### 3.3 ModeGet

ModeGet gives the information of the engine's voice: the selected mode's, until a `\Vce\` tag in a text (section 1.3) or given to `Inject` (section 4.2) changes the voice. From then on it gives the new voice's mode, all of it, ID included. A text's `\Vce\` changes the voice when the speaking thread reaches it. The port's `Engine.ModeInfo` is ModeGet.

### 3.4 Find

`ITTSFind`'s Find gives the mode that best matches the mode information it is given, by **ranks** (`TTSMODEINFORANK`) that say how much each field counts. Without ranks it takes the mode name's 4, the speaker's 3, the gender's 2, the age's 1 and 0 for the rest. Each mode scores:
- for each of the engine ID, the manufacturer, the product name, the mode name and the speaker that equals the one wanted, 10 times its rank. The engine IDs are compared as GUIDs. The names are compared up to a NUL, with ASCII letters' case aside and no other: `sIDNEY` is Sidney, but `Sİdney` and `ſidney` match nobody, and `Wanda` NUL `Julia` is Wanda;
- when the gender wanted is neutral (0), female (1) or male (2), the gender's rank times how close the mode's gender is: 10 for the same, 6 for a mode's female or male when neutral is wanted, 3 for female and male. Any other gender wanted counts for nothing;
- when the age wanted is one of SAPI's ages, 1, 3, 6, 14, 30 or 70 (baby, toddler, child, adolescent, adult, elderly), the age's rank times how close the mode's age is: (6 − *d*) × 10 / 6, rounded down, where *d* is how many places apart the two ages are in that list. Every mode is 30, so wanting 1 gives 3, 3 gives 5, 6 gives 6, 14 and 70 give 8 and 30 gives 10. Any other age wanted counts for nothing.

The ranks of the mode ID, the language, the dialect, the style, the features, the interfaces and the engine features count for nothing. The scores are 32 bits and wrap. Find gives the first mode with the highest score, or `E_FAIL` (0x80004005) when every mode scores 0, and leaves the enumerator at the end either way. Its Select is the enumerator's.

The port's `Enumerator.Find` takes the ranks as a `*Rank`, nil for none, and gives `ErrNoModeFound` for `E_FAIL`.

### 3.5 What the recorded runs don't reach

- **Select at 8000 Hz first.** Select takes the first format it offers from the mode's number: 8000 Hz for a mode numbered 10 to 19, 11025 Hz otherwise. With ten modes it is always 11025 Hz (section 2.2), and the port has only that order.
- **Select's failures.** A destination without SAPI's audio interfaces gives `E_INVALIDARG`, and an engine that can't be set up `E_OUTOFMEMORY`. The port's engine has no destination object and needs nothing set up.
- **Null pointers** give `E_INVALIDARG`, and so does Next with a count other than 1 and no place for how many it wrote. The port has no pointers.
- **The ANSI interfaces** `ITTSEnumA` and `ITTSFindA` give the same modes with ANSI strings. The port has one kind of string.
- **Aggregation.** Asking the class factory for the enumerator inside another object gives `CLASS_E_NOAGGREGATION`.
- **Ages and genders the modes don't have.** Find would count a mode's age that isn't one of SAPI's as 70, and a neutral mode's gender as 6 from female or male. Every mode is a man or a woman of 30, so the port leaves both out.

## 4. Inject and the other attributes

### 4.1 Inject

`Inject` changes the engine with a string of tags, at once rather than at a place in the text. It converts the string as TextData converts text (section 1.1), up to its first NUL, then:
- When the string is exactly `\gain=1\`, `\gain=2\` or `\gain=3\`, in that case and with nothing before or after it, it sets the engine's output gain to that number. Each speaking thread takes the gain when it starts ([pipeline.md](pipeline.md) §8.1) as the synthesizer's ([synthesizer.md](synthesizer.md) §5.4), so it counts from the next thread on. A new engine has gain 1. Any other string, `\gain=4\` and `\Gain=2\` too, is read for tags.
- Otherwise the string is read for **tags**, and what is between them is dropped:
  - `\\` is passed over;
  - a tag runs from a backslash to the next one. A backslash among the string's last two characters ends the string, and so does a tag without its closing backslash;
  - a tag with fewer than three characters between its backslashes is dropped;
  - reading goes on after the closing backslash, which isn't shared with the next tag: `\Pit=230\Spd=120\` sets the pitch only, and `\Pit=240\\Spd=130\` both.
- A tag is known by its name, the three characters after its backslash, case aside, as in TextData (section 1.3). `rst` resets whatever follows it (`\Rst=5\`, `\Rstxyz\`); `pit`, `spd`, `vol`, `chr` and `vce` need `=` as the fourth character, and their value runs from the fifth to the closing backslash. Every other tag, `\Pau=\`, `\Mrk=\`, `\Emp\` and `\Ctx=\` included, does nothing.

### 4.2 The tags

| Tag | What it does |
|---|---|
| `\Pit=` | When the value is all ASCII digits: the number, 0 counting as 50 and 65535 as 400. From 50 to 400 it becomes the pitch, in Hz, and the window is told the pitch changed |
| `\Spd=` | When the value is all ASCII digits: the number, 0 counting as 50 and 4294967295 as 250. From 50 to 250 it becomes the speed, and the window is told |
| `\Vol=` | When the value is all ASCII digits: the number's low word becomes the volume, without a check, and the window is told, even when the volume stays the same |
| `\Chr=` | The value up to a comma or a backslash, at most 19 characters, case aside: `"normal"`, `"whisper"` and `"monotone"`, quotes included, set the character 0, 1 or 2. The window isn't told |
| `\Vce=` | The first key, up to its `=`, must be `speaker`, case aside. Its value, after an opening quote if there is one, runs up to a comma or a backslash, at most 19 characters, and a closing quote is dropped. The first mode whose speaker it is, case aside, gives the engine its voice, with the voice's pitch and speed (section 2.5). The window isn't told |
| `\Rst\` | The pitch and the speed of the mode Select chose, volume 65535, the normal character and no context (section 1.4). The window is told the pitch, the speed and the volume changed, in that order. The voice stays |

Numbers are read as the C library's `sscanf` reads `%u`: 32 bits that wrap, so `\Pit=4294967346\` is 50. Signs, spaces and other digits make the value not all digits: `\Pit=+100\`, `\Pit= 200\`, `\Pit=200 \`, `\Pit=2²³\` and `\Pit=1é00\` do nothing, while `\Pit=0100\` is 100 and the fullwidth `\Pit=２００\` 200, as best fit makes it ASCII first. `\Vol=65536\` is volume 0 and `\Vol=70000\` 4464.

Each of `\Pit=`, `\Spd=` and `\Vol=` keeps the last number it read in the string, after its corrections, and an empty value takes it again: `\Pit=200\ \Pit=\` sets 200 twice and tells the window twice, `\Pit=0\ \Pit=\` sets 50 twice, `\Pit=500\ \Pit=\` does nothing, and `\Vol=900\ \Pit=210\ \Vol=\ \Pit=\` sets the volume, the pitch, the volume and the pitch. An empty value before its tag has read a number is section 4.4's.

`\Chr=whisper\` without quotes, `\Chr="whisper" \` and a value longer than 19 characters do nothing; `\Chr="whisper",x\` is whisper. `\Vce=Speaker="julia"\`, `\Vce=Speaker=Julia,Gender=Male\`, `\Vce=Speaker="Wanda\`, `\Vce=Speaker=Wanda"\` and `\Vce=Speaker=Melvin,\` find their speakers. `\Vce=Gender=Male,Speaker=Julia\`, `\Vce=Speaker = Wanda\`, `\Vce=Speaker=\` and an unknown speaker do nothing: unlike TextData's `\Vce\`, `Inject`'s reads only the speaker and keeps the voice when it finds nobody.

`\Rst\` goes back to the mode Select chose, not to the voice: after `\Vce=Speaker=Wanda\`, it gives Wanda's voice Peter's pitch, 85 Hz, and ModeGet still gives Wanda. A text's `\Rst\` instead writes the commands of section 1.3.

The pitch, speed, volume, character and voice reach the texts already queued at the speaking thread's next step ([pipeline.md](pipeline.md) §8.3). The context is read by the next TextData, and the gain by the next speaking thread.

### 4.3 The other attributes

- **RealTimeGet** and **RealTimeSet** give `E_NOTIMPL` (0x80004001), and RealTimeGet writes nothing. The port has neither.
- **Interfaces.** The engine has `ITTSCentral`, `ITTSAttributes`, `ILexPronounce` and `ITTSDialogs`, wide and ANSI. It has neither `ILexPronounce2` nor `IAttributes`: asking for them gives `E_NOINTERFACE`, as asking the engine for `ITTSEnum` does. The dialogs show the engine's About, General, Lexicon and Translate windows; the port has none.

### 4.4 What the recorded runs don't reach, and where the port differs

- **An empty value before a number.** An empty `\Pit=`, `\Spd=` or `\Vol=` before that tag has read a number in the string reads a variable the original never set. In the recorded runs, an empty `\Vol=\` set volume 7, every time, and told the window; an empty `\Pit=\` or `\Spd=\` did nothing, as if theirs were out of range. The port leaves the attribute as it is for all three.
- **A null string** gives `TTSERR_INVALIDCHAR` (0x80040208), and running out of memory `E_OUTOFMEMORY`. The port has no null strings.
- **The ANSI interface's `Inject`** converts the string to UTF-16 and calls the wide one.

## 5. Pause, resume and reset

### 5.1 The calls

The engine **claims** its destination when a speaking thread starts, unless it is paused, and releases it when the window takes the thread's end mark (section 2.3). It is **speaking** from that start until that end mark.

| Call | What it does |
|---|---|
| `AudioPause` | Fails with `TTSERR_ALREADYPAUSED` (0x80040502) when the engine is paused. Otherwise the engine is paused: the speaking thread waits the next time round its loop ([pipeline.md](pipeline.md) §8.2), and the destination is stopped and, if the engine had claimed it, released. It returns what stopping the destination returned, and the engine is paused even when that is an error |
| `AudioResume` | Fails with `TTSERR_NOTPAUSED` (0x80040501) when the engine isn't paused. Otherwise it claims the destination and starts it; if either fails, it returns that error and the engine stays paused. Then the engine is no longer paused, the speaking thread goes on, and the window flushes the queue. When the engine isn't speaking, the destination is released again |
| `AudioReset` | When the engine isn't paused and hasn't claimed the destination, it stops and flushes the destination and returns what stopping it returned. Otherwise it **resets** the engine (section 5.3), and returns `S_OK`, or what stopping the destination returned when it stopped it |

The port has no destination: a sink takes the audio and the events (section 2.3). `Pause`, `Resume` and `Reset` are `AudioPause`, `AudioResume` and `AudioReset`, and `Pause` and `Resume` fail with `ErrAlreadyPaused` and `ErrNotPaused`. A sink pauses the engine by returning `ErrPaused` (section 5.2) and resets it by returning any other error (section 5.4). The port's engine has no thread of its own (section 2.1), so the calls come while `Run` doesn't run, or from the sink's methods while it runs:
- `Pause` before `Run` makes the next thread wait before its first step: `Run`, and `Speak`, return `ErrPaused`. From the sink's methods, it pauses the engine as the sink's `ErrPaused` does.
- `Resume` gives the sink the queue and runs the thread on from its wait, and returns as `Run` does. From the sink's methods, it only ends the pause.
- `Reset` resets the engine when it is paused or has texts to speak. It does nothing while `Run` runs, and nothing to an engine that has nothing to speak and isn't paused.

### 5.2 Pausing

The window gives the destination a buffer only when the destination's free space is more than the buffer's size, in bytes of the engine's format, plus 50. Otherwise it stops at that buffer, and the events after it wait too; it tries again at the next flush, or when the destination reports free space. While the engine is paused, the window gives the destination nothing. So behind a destination without room and behind a pause, the queue fills.

The speaking thread runs on behind the window. A hand-over that finds ten items or more waiting in the queue flushes it, waits until the window has flushed the whole queue, at most 5 seconds, and then queues its buffer and flushes again; it doesn't end the prebuffering ([pipeline.md](pipeline.md) §8.4). The events are queued whatever the queue holds. The thread's messages, the texts started and done and the attribute changes, are reported while the window holds the audio, at the position where it stopped.

In the recorded runs, the destination takes only so many bytes. The window stops at the first buffer that doesn't fit, and the thread runs on until a hand-over finds the queue full and waits. `AudioPause` comes during that wait, then the calls of the run, then `AudioResume`, which flushes the queue and so ends the wait. While the engine is paused:
- `PitchSet`, `SpeedSet`, `VolumeSet` and `Inject` change the engine's attributes, which the thread applies at its next step (section 2.5, [pipeline.md](pipeline.md) §8.3), after `AudioResume`. The attribute calls report their changes at once.
- `TextData` queues its text, without claiming the destination. If no thread runs, it starts one, which waits before its first step: the text's start is reported after `AudioResume`.

`AudioResume` made after more than 5 seconds finds the thread past its wait: it has queued its buffer and waits the next time round its loop. The audio and the events are the same.

The port's sink returns `ErrPaused` where the original's window would stop: from `Audio`, after taking none of the buffer or some of it, or from `Event`. The engine is then paused: it holds the audio from the bytes the sink took, or from the next buffer after an event, and gives the sink the events before that buffer and the thread's messages. The thread runs on until a hand-over finds ten items waiting, and `Run` returns `ErrPaused`. `Resume` gives the sink the rest of the queue and runs the thread on.

### 5.3 Reset

A reset stops the destination (unless the engine is paused), flushes it and releases it. The engine is no longer paused, and the speaking thread is told to end and woken from its wait, whichever it was in: the pause or the window. The reset waits for the thread to end, at most 5 seconds.

The thread ends the next time round its loop, without an end mark, and posts the window a `TextDataDone` with flags 1 (`TTSBNS_ABORTED`) for each text it hasn't reported done:
1. each text whose end is in the pipeline: each control node `i` with value 0 that carries a text ([pipeline.md](pipeline.md) §4.1), in the nodes' order;
2. then the text it was reading, if that is all fed and its end isn't a node yet: the input ring isn't drained ([input.md](input.md) §4.1), or the command ring holds more than six bytes ([input.md](input.md) §2.1). The end of a text, `ESC[0i ESC[C` ([input.md](input.md) §5.2), takes seven.

The reset itself then gives each text still queued, the one the thread was feeding and those after it, a `TextDataDone` with `TTSBNS_ABORTED`, in order, at the destination's position. It drops the window's queue: the audio and the events in it are never given. When the application's loop gets to them, after `AudioReset` has returned, the window handles the messages still posted, in order: those the thread posted before the reset, then the thread's aborted texts. So the notification sinks get, all at the destination's position: the texts the thread hadn't all fed, those whose end was in the pipeline, and the text it was reading. A text that hadn't started gets its `TextDataDone` without a `TextDataStarted`.

What the texts changed stays (section 2.1). The next `TextData` starts a new thread.

### 5.4 An error from the destination

When the destination refuses a buffer, the window resets the engine (section 5.3). The port's sink is the destination and the notification sinks in one: an error from its `Audio` or `Event`, other than `ErrPaused`, resets the engine the same way, and `Run` returns the error. The sink gets the `TextDone` events of the reset, and its errors there are ignored.

### 5.5 What the recorded runs don't reach, and where the port differs

- **Destination errors.** The recorded runs' destination never refuses a buffer (section 5.4), and its `Claim`, `Start` and `Stop` never fail.
- **A pause while the thread runs freely.** `AudioPause` made while the thread isn't waiting for the window makes it wait the next time round its loop, wherever it is then. The port's `Pause`, called from the sink's methods, lets the thread run on until its queue is full, as behind a destination without room, so where it waits doesn't depend on timing.
- **A reset while the engine speaks without a pause.** The thread ends the next time round its loop, wherever it is then. The three recorded runs that do this are among the five that section 2's check leaves out. In the port, `Reset` does nothing while `Run` runs, and a sink resets the engine by returning an error. Between `Queue` and `Run`, `Reset` aborts each text queued, as if the thread had taken none.
- **The end of a text alone in the command ring.** A reset finds the text the thread was reading unfinished when the command ring holds more than six bytes, so when it holds the seven of the text's end and nothing else. The recorded runs' resets find the ring empty or holding more, or holding seven with the input ring not drained, so they don't tell six from seven.
- **The waits' 5 seconds.** Behind a destination without room and without a pause, the original's thread queues a buffer and goes on every 5 seconds. A reset that waits 5 seconds for the thread goes on without it. The port's thread waits until `Resume` or `Reset`.
- **Part of a buffer.** The original's window gives a buffer whole. A port's sink can take part of one and return `ErrPaused`: it gets the rest after `Resume`. A sink that takes part of a buffer without an error resets the engine with `io.ErrShortWrite`.

## 6. Phoneme

### 6.1 The calls

`Phoneme` converts a text to phonemes without speaking it. It runs a pipeline of its own (section 6.2), and the engine's mode, attributes and texts play no part.

| Interface | What it does |
|---|---|
| Wide | Takes IPA (1) and engine phonemes (2); any other character set gives `E_INVALIDARG` (0x80070057), and the size it gives back stays as the caller left it. The text is `dwSize / 2` code units of the `SDATA` (rounded down), converted to code page 1252 as TextData converts text (section 1.1), with a NUL after them. With flags 1, its tags go over as they are (section 6.3); with any other flags, they are read as text. The phonemes come back as UTF-16 code units: engine phonemes are the listing (section 6.2), each byte converted as Windows' `MultiByteToWideChar` converts code page 1252 (the data pack's `wide` table); IPA is section 6.4. A NUL follows them, which the size, in bytes, counts |
| ANSI | Takes engine phonemes only; any other character set gives `E_INVALIDARG`. Windows converts the `dwSize` bytes to UTF-16, the wide interface's `Phoneme` converts them, and Windows converts the phonemes back to code page 1252, which gives each byte back as it was. The size counts the NUL |

The port's `Phoneme` is the wide interface's: it takes the code units and whether the tags go over, and returns the phonemes with their NUL, or `ErrInvalidArgument`. `PhonemeANSI` is the ANSI interface's, and `PhonemeString` converts a Go string, with its tags, to engine phonemes.

### 6.2 The listing

Phoneme's pipeline is constructed as a speaking thread's ([pipeline.md](pipeline.md) §8.1), at voice 0, 85 Hz, 150 words a minute and 8000 Hz, and initialized ([pipeline.md](pipeline.md) §7), but not started: it renders nothing and queues no events. It has no engine, so its control commands change no attributes but its own and report nothing. Instead, it **lists** the phonemes its prosody stage scans ([prosody.md](prosody.md) §3.2): each word's mark, `%` or `&`, then the word's phonemes but pauses, a vowel of stress 1 followed by `2` and one of stress 2 or 3 by `1`, and the `.`, `?` or `!` that ends a sentence. Where a long phrase is broken off, its last word mark is listed twice. So `Hello world.` is `&HeLO1&W31jDp.`. A consonant's stress isn't listed: phonetic input `ESC[1I%h1Ed2l1ESC[0I` is `%hEdl`.

Without flags 1, the text up to its first NUL is read as it is ([input.md](input.md) §7): fed into the input ring and pumped into the escape parser without the feeder's lines and without the tokenizer's expansions ([input.md](input.md) §4.4), and the pipeline steps until a step reports idle. The listing, with a NUL after it, is what Phoneme converts.

### 6.3 Tags

With flags 1, the tags aren't converted as TextData converts them (section 1.3), and the characters 128 and up stay as they are. Phoneme walks the text up to its first NUL a character at a time, gathering a **piece** of text.
1. `\\` adds one backslash to the piece.
2. Another backslash starts a tag. Its end is the next backslash from the tag's third character on, or else the text's NUL. If the piece has characters, they go to the escape parser as a string ([input.md](input.md) §7), with an end of text after them, the pipeline steps until a step reports idle, and the piece is emptied. Then the tag, from its backslash to its end, goes into the listing as it is. A space right after the tag goes in too, and the walk passes it.
3. Any other character is added to the piece.

At the text's end, a piece with characters is read as it is (section 6.2). So a tag does nothing but appear in the listing, and a phrase never runs across it: `Hello \Pit=200\ world.` is `&HeLO1\Pit=200\ &W31jDp.`, at 85 Hz. A tag without a closing backslash takes the text's NUL into the listing, and the listing's NUL follows it.

### 6.4 IPA

The listing is converted a byte at a time. Each byte from space to `~` becomes its string in the data pack's `ipa` table (`H` is `h`, `U` `ju`, `5` `aɪɹ`, `1` `1`), or nothing if the table has none; any other byte becomes nothing. So `Hello world.` is `&hɛloʊ1&wɚ1ɫd.`.

With flags 1, a backslash in the listing starts a tag. Its end is the next backslash from its third byte on, looked for only while the position is below the text's length: the length of the converted text up to its first NUL, not the listing's. Where the search stops first, the tag ends. The bytes from the backslash to the end, both included, go over as code units of their own, and the conversion goes on after them. When the listing is longer than the text, a tag past the text's length is cut short: in `1000000 \Pit=200\`, the tag goes over as `\Pi`, and the rest is converted as phonemes: `t` becomes `ɾ`, `=` and `0` nothing, `2` stays `2`, and the closing backslash starts a tag that goes on past the listing's NUL (section 6.5).

### 6.5 What the recorded runs don't reach, and where the port differs

- **The pre-formatter.** With a registry setting of the original's on (`CertificationTags`), a tagged text that starts with `\Ctx="Address"\` or `\Ctx="E-Mail"\`, case aside, goes through the pre-formatter (section 1.4) first. The setting is off unless someone sets it, as in the recorded runs. The port has no settings and doesn't pre-format.
- **Past the listing.** In IPA, a tag that ends past the listing's NUL (section 6.4) copies whatever memory follows the listing, which varies from run to run. The port copies NULs. The one recorded run with such tags is compared by the sizes alone.
- **Past the text.** A text whose `SDATA` doesn't count its NUL, with a last tag without a closing backslash: the walk (section 6.3) looks for a space in the byte after the NUL, past the end of its copy of the text. In the recorded runs that byte was the one a previous text had left in the memory the copy took over, and a space there added a space to the listing. The port reads a NUL. Such runs aren't recorded.
- **The `n` command** in the text: the original runs it through the engine, which Phoneme's pipeline doesn't have (section 7.3).
- **Running out of memory** gives `E_OUTOFMEMORY`.

## 7. The user lexicon

### 7.1 The store

The **user lexicon** holds words with their pronunciations, which the lexical stage looks up before its dictionary ([lexical.md](lexical.md) §7). The original keeps one for the whole process: every engine looks words up in it, and the `ILexPronounce` of any engine changes it. The port keeps one for each `Data`, which every engine made from it shares, and `Data.Lexicon` gives it.

An entry is a word, in capitals, of the letters `A` to `Z`, and its pronunciation in engine phonemes, of the symbols `A` to `Z`, `a` to `z`, `1` to `5`, `@`, `|`, `~` and `&`. The lexicon holds at most 5,000 entries. They are kept sorted by their words, compared a byte at a time, a word before a longer one it starts: after a word is added, and after a file is read (section 7.4). The original sorts them with the C runtime's `qsort` and looks words up with its `bsearch`, and the port sorts and looks up as they do, since a file can give a word twice: which of the two a lookup finds depends on where the sort leaves them and where the search looks first.

**The sort** works on a span of the entries, from `low` to `high`, at first all of them, and keeps a stack of spans to sort later. Each time it is done with a span, it takes the last one off the stack, and it ends when the stack is empty.
1. A span of 8 entries or fewer is sorted by selection: for each place from the span's last down to its second, the greatest entry from the span's start to that place, the first of equal ones, is swapped into it.
2. Otherwise, `middle` is `low` and half the span's size, rounded down. The entries at `low` and `middle` are swapped if the first is greater, then those at `low` and `high`, then those at `middle` and `high`.
3. `lower` and `upper` start at `low` and `high`. Then, over and over:
   1. if `lower` is below `middle`, it goes up by one, and on while it is below `middle` and its entry isn't greater than `middle`'s;
   2. if `lower` is then at `middle` or above, it goes up by one, and on while it is at most `high` and its entry isn't greater than `middle`'s;
   3. `upper` goes down by one, and on while it is above `middle` and its entry is greater than `middle`'s;
   4. if `upper` is below `lower`, the loop ends. Otherwise, their entries are swapped, and if `middle` was `upper`, it becomes `lower`.
4. `upper` goes up by one. If `middle` is below `upper`, `upper` goes down by one, and on while it is above `middle` and its entry equals `middle`'s; if `middle` is then at `upper` or above, `upper` goes down by one, and on while it is above `low` and its entry equals `middle`'s.
5. That leaves two spans, from `low` to `upper` and from `lower` to `high`. If the first is at least as long (`upper − low ≥ high − lower`), it goes on the stack, and the second is sorted next; otherwise the second goes on the stack, and the first is sorted next. A span of fewer than two entries doesn't go on the stack, and isn't sorted next: the sort takes the next one off the stack.

**The search** looks among `count` entries from `low`, at first all of them. While `count` isn't 0: if it is 1, the word is the entry at `low`, or not in the lexicon. Otherwise, the entry it compares the word with is the `count / 2`th after `low` when `count` is odd, and the one before it when `count` is even (`count / 2` rounded down). If the word is that entry's, it is found; if it is less, the search goes on among the entries before that one, and if it is greater, among those after it.

Adding or removing a word sets the lexicon's **changed** flag, and saving it clears it (section 7.4). `Changed` reports it.

### 7.2 Add, Get and Remove

TruVoice has `ILexPronounce`, wide and ANSI, but not `ILexPronounce2`. The wide interface's methods convert the word, and a pronunciation in text or engine phonemes, to code page 1252 as TextData converts text (section 1.1), up to its NUL. In a word, and in a pronunciation, spaces and quotation marks (`"`) are **stripped**: each is removed, but the character after each one removed isn't looked at, so of a run of them, every second one stays: `a  b` becomes `a b`.

**Add** takes text (0), IPA (1) and engine phonemes (2); any other character set gives `E_INVALIDARG` (0x80070057).
1. The word is converted and stripped. Each of its characters must be a letter, `A` to `Z` or `a` to `z`, or Add gives `LEXERR_INVALIDTEXTCHAR` (0x80040801): `café` and `x-ray` give it, and `Ābc`, which best fit makes `Abc`, doesn't. An empty word is one too.
2. If the lexicon has the word, in any case, Add gives `LEXERR_ALREADYINLEX` (0x80040806) and changes nothing.
3. The pronunciation is converted to engine phonemes. Engine phonemes are converted to code page 1252. IPA is converted as TextData converts it (section 1.5), without `ESC[1I` and `ESC[0I`. Text is converted to code page 1252, stripped, and given to the ANSI interface's `Phoneme` without flags and without its NUL, so its tags are read as text (section 6). Of the listing, a final `.` is dropped, then a final `p`, then a first `&` or `%`: `stop` gives `SDo1P`, `hello world` (`helloworld`) `He1jO3jD`, and `a, b` `A1&BE1`. A text with nothing to say gives an empty pronunciation.
4. The pronunciation is stripped. Each of its symbols must be one of the lexicon's (section 7.1), or Add gives `LEXERR_INVALIDPRONCHAR` (0x80040805). An empty pronunciation is one.
5. When the lexicon has 5,000 entries, Add adds nothing and gives `S_OK`, and the changed flag stays as it was. Otherwise the word goes in, in capitals, with its pronunciation, and the lexicon is sorted. A word the lexicon has by then, as when two Adds of one word run at once, gets the new pronunciation instead.

The part of speech and the engine's information are ignored.

**Get** takes the same character sets; any other gives `E_INVALIDARG`. The word is converted but not stripped, and looked up in any case; a word the lexicon hasn't gives `LEXERR_NOTINLEX` (0x80040803). For text and engine phonemes, the pronunciation comes back as engine phonemes, each byte converted as Windows' `MultiByteToWideChar` converts code page 1252 (the data pack's `wide` table). For IPA, each symbol from space to `~` becomes its string in the data pack's `ipa` table, as Phoneme converts its listing (section 6.4). The size Get needs is the code units with the NUL for text and engine phonemes, and the bytes with the NUL for IPA. If `dwPronounceSize` is less, Get gives `LEXERR_PRNBUFTOOSMALL` (0x80040807) and writes the size it needs into `pdwPronounceNeeded`. Otherwise it writes the pronunciation and its NUL, 2 bytes a code unit, writes 0 into the part of speech and into the engine information's size, and leaves `pdwPronounceNeeded` as it was. So for text and engine phonemes, a buffer as many bytes long as the pronunciation has code units passes, and gets twice as many bytes. The sense is ignored.

**Remove** converts and strips the word, as Add does, and looks it up in any case; a word the lexicon hasn't gives `LEXERR_NOTINLEX`. Otherwise its entry goes, and the changed flag is set. The sense is ignored.

**The ANSI interface** converts the word and the pronunciation from code page 1252 to UTF-16, and calls the wide interface's method, which converts them back as they were. Add and Get take text and engine phonemes; IPA gives `E_INVALIDARG`. Get gives the wide Get a buffer of `dwPronounceSize` code units and one more, and the same size, so it needs the pronunciation's bytes with the NUL; then it converts what is in that buffer, up to its first NUL, into the caller's, with a NUL after it.

The port's `Add`, `Get` and `Remove` are the wide interface's, on code units, and `AddANSI`, `GetANSI` and `RemoveANSI` the ANSI interface's, on bytes. Get returns the pronunciation without its NUL, and leaves the size to the caller.

### 7.3 `\Prn=`

A `\Prn=` tag in a text (section 1.3) writes an `n` command with its record of a word and a pronunciation. When the normalizer runs the command ([pipeline.md](pipeline.md) §4), the engine gives the record to its own ANSI interface: a record without a pronunciation removes its word, and any other adds it, as text, or as engine phonemes after `//`. What they give is ignored, so a word the lexicon has keeps its pronunciation, and a word it hasn't stays out. Then the record is freed. Words the lexical stage converts afterwards find the change, as they would find any other.

The original writes the record's address in the command, and the port the record's number (section 1.3). An `n` command that a text writes itself, as an escape sequence, has a value that is no record's: the original reads whatever is at that address, and the port does nothing. A value that is the number of a record the engine's tags made, which the normalizer hasn't run, runs that record. Phoneme's pipeline (section 6.2) has no engine, and only an escape sequence can give it an `n`: the original reads the engine's interface from an engine that isn't there and faults, and the port does nothing.

### 7.4 The file

The original keeps the lexicon in a file, `english.dic`. When the first engine of the process starts, it reads the file the registry names, or else the one in its folder, and when the last engine is released, it empties the lexicon. When an application releases `ILexPronounce` with the changed flag set, the original saves the lexicon. If its path didn't come from the registry, it first asks in a message box whether to save it there, and on No, asks for another with a Save As dialog; then it writes the file and records its path in the registry.

The port's lexicon lives as long as its `Data`, and the application decides when to read and write it: `Read` replaces the lexicon with a file's words, and `Write` writes the file and clears the changed flag, which `Changed` reports.

The file is each word and its pronunciation, each followed by a NUL. The original writes them in the lexicon's order, the words in capitals, and reads a file as follows.
- It opens the file as text, so a Ctrl+Z (0x1A) ends it.
- It reads a word, then its pronunciation, each up to its NUL. It keeps at most 100 characters of each, which changes nothing, as an entry of 40 or more is invalid. An entry the file ends in before its pronunciation's NUL isn't read.
- An entry is **invalid** when its word or its pronunciation has 40 characters or more, its word a character that isn't a letter, `A` to `Z` or `a` to `z`, or its pronunciation a symbol that isn't the lexicon's. The original skips it, and at the first one, says in a message box (`MB_ICONEXCLAMATION | MB_TASKMODAL`, 0x2030) that the file may be corrupted. The port's `Read` returns `ErrInvalidEntries`.
- It keeps the valid entries, the words in capitals, and stops at 5,000. Then it sorts them (section 7.1). Words the file gives twice stay twice: Add refuses the word, a lookup finds one of them, and Remove removes that one.
- Reading doesn't change the changed flag.

### 7.5 What the recorded runs don't reach, and where the port differs

- **The dialogs.** The original's `LexiconDlg` edits the lexicon in a dialog of its own, and saving asks with a message box and a Save As dialog. The port has neither: the application reads and writes the file.
- **Empty fields in the file.** A word or a pronunciation that is only its NUL leaves the original's field as it was: what the stack held. The port reads it as empty: an empty pronunciation is valid, and so is an empty word.
- **An ANSI Get that fails.** When the wide Get gives `LEXERR_NOTINLEX` or `LEXERR_PRNBUFTOOSMALL`, the original's ANSI Get still converts its buffer, which the wide Get hasn't written, into the caller's: whatever the memory held, up to its first NUL, with a NUL after it, past the caller's buffer if it is long. The port writes nothing. The recorded runs' failing ANSI Gets are compared by their results and sizes.
- **An empty listing.** When the text of a pronunciation gives an empty listing, or a `.` alone, the original tests the byte before the listing for `.` and `p`, which is the memory allocator's, and would write a NUL there. The port tests nothing before the listing.
- **A word of 19 characters** in `\Prn=` followed by neither `=` nor `\` (section 1.6).
- **NULL pointers.** A NULL word gives `LEXERR_INVALIDTEXTCHAR`, a NULL pronunciation or buffer `LEXERR_INVALIDPRONCHAR`, and a NULL `pdwPronounceNeeded` `E_INVALIDARG`. The port takes slices.
- **Running out of memory** gives `E_OUTOFMEMORY`.
