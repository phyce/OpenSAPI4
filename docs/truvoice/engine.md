# TruVoice: the engine

How L&H TruVoice (`tv_enua.dll`) behaves at its interface, around the speaking pipeline the other specs describe. This spec covers what TextData makes of a text before it queues it for the speaking thread (section 1), and how the engine speaks the texts and reports what it speaks: the speaking threads, the audio's formats, the queue to the destination, the events and where they land, and pitch, speed and volume (section 2). The modes, the other attributes, `Inject`, pausing, resets and the user lexicon come with the next parts.

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

**Exactness.** Ported from the original's code, and checked against it on every text it queued for the speaking thread in 1,367 recorded runs: the 24 runs of TextData's own set, which test every tag with values in and out of its range, word marks, tags in words, emphasis before words and at the end, spelling, voices, contexts, the characters 128 and up, best fit, IPA with every rule, engine phonemes, the ANSI interface, the character sets it refuses and sizes that leave out the NUL; and the texts of the input, normalizer and stage sets. Of their 2,376 texts, 2,361 were identical to the original's. Three were refused, as the original refused them. The other 12 have characters in the address, e-mail or spreadsheet context, whose pre-formatter isn't ported. Four runs that call `Inject` were left out (`Inject` comes with the next part).

Section 2 was checked against the original's speaking in 1,760 recorded runs, every run of the sets but those that need the next parts (`Inject`, the user lexicon, pausing and resets), the 8 runs with the pre-formatter's contexts and 5 runs whose calls land wherever the speaking thread happens to be. In all of them the audio was the original's, 659,067,040 samples, in the format the destination took; the texts started and ended in the original's order, 5,104 times; and the 75,234 words and bookmarks and 573,209 visual events came at the original's samples, with its IPA characters, engine phonemes and mouth shapes. Where a text's start and end and an attribute change land varies from run to run in the original (section 2.4): in 34 runs that it spoke ten times each, the port put each of the 101 at the place most of its runs had it.

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

**A pronunciation**: `\Prn=word=pronunciation\`, or `\Prn=word=//phonemes\` for engine phonemes up to a `/`. Each part is at most 19 characters, and a tag without `=pronunciation` removes the word. The tag writes `ESC[b0;b1;b2;b3n` and a space with the record of the word and its pronunciation, which the normalizer gives the user lexicon (part T18). The original writes the record's address. The port numbers the records of an engine from 1 and writes the number.

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

### 2.2 The audio's formats

The engine offers its destination four formats, mono PCM, in this order, and takes the first the destination accepts: 11025 Hz with 16 bits, 11025 Hz with 8 bits, 8000 Hz with 16 bits, 8000 Hz with 8 bits. If the destination accepts none, selecting the mode fails with `TTSERR_WAVEFORMATNOTSUPPORTED`. The pipeline renders at the format's sample rate ([pipeline.md](pipeline.md) §8.1), always in 16 bits; in 8 bits, each sample is the high byte of the 16-bit one plus 128.

### 2.3 The queue and the window

The thread doesn't write to the destination. It queues **items** for the engine's window: the audio buffers it hands over ([pipeline.md](pipeline.md) §8.4), the visual events ([parameters.md](parameters.md) §6), the word and bookmark events ([pipeline.md](pipeline.md) §4.1), and an end mark at its end ([pipeline.md](pipeline.md) §8.5). To have them written, it posts the window a message, which **flushes** the queue: the window takes the items in order, writes each buffer to the destination, and hands each event to the destination as a bookmark, which comes back when the destination plays to it. The destination of the original's recorded runs plays at once, so each event comes back before the next item; the port's sink gets the audio and the events in that order.

The thread flushes the queue:
1. after queuing a visual, word or bookmark event, at once;
2. after a hand-over, as follows. A thread starts **prebuffering**: a hand-over then flushes only when more than six items were waiting in the queue before its buffer, and stops prebuffering. After that, every hand-over flushes;
3. at its end, after the end mark.

The thread's other messages, a text started or done (section 2.1) and an attribute changed (section 2.5), go to the window in the same order as the flushes, which it handles in the order they came.

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

- **The queue's limit.** In the original, a hand-over that finds ten items or more waiting posts a flush and waits until the window has flushed (at most 5 seconds, or until the engine is reset), then queues its buffer and flushes again. With a window that keeps up, at most seven items wait (section 2.3), so the port has no limit. Nor does it have the original's handling of a destination that has no room: the window then stops at the first buffer that doesn't fit, with 50 bytes to spare, and goes on at the next flush.
- **Hand-overs of 0 or 2 samples.** The original's queue refuses an empty item, and the thread then waits for the window for ever; it takes an item of 4 bytes for an event. None of the 539,095 hand-overs of the recorded runs had fewer than 3 samples.
- **An error from the destination.** The original's window resets the audio when the destination refuses a buffer (part T18). The port's `Run` stops at an error from the sink, drops the texts still queued and returns the error.
- **ANSI notification sinks** get 0 for every visual event's IPA character. The port has one kind of sink.
- **Symbols outside the table.** No phoneme symbol is 0 or 128 and up. For 0, the original would read the mouth shape past its table's end; the port gives no IPA character and no mouth shape for such symbols.
