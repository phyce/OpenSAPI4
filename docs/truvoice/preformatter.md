# TruVoice: the pre-formatter

How L&H TruVoice (`tv_enua.dll`) pre-formats the text it reads in its address, e-mail and spreadsheet contexts. TextData sends a text's segments in these contexts through the pre-formatter before it queues them ([engine.md](engine.md) §1.4), with their characters 128 and up made spaces but `’ £ ¥ © ®`. This spec covers the spreadsheet context: how the pre-formatter reads a text's lines and writes them (section 1) and what it makes of a line (section 2); how the address and e-mail contexts read a text and its e-mail header lines, join its lines and space its words (section 3); and where the port differs (section 4). The rest of the address and e-mail contexts' pre-formatter isn't ported yet, and the engine doesn't use section 3 yet.

**In short.**
- In the spreadsheet context, the text is read a line at a time, up to each line feed. A line ends in CR LF, and the text's last one, if no line feed ends it, in CR CR LF.
- A line of more than 720 characters is cut after a dot and a space, else after a space, else at 720 characters, and each part ends in CR CR LF. Lines are wrapped at 77 columns without line breaks: a word too long for a row gets a character made a space.
- A line with two slashes is a date: `12/25/1999` is `December 25, 1999`.
- A line with one slash has a fraction: `3 1/2` is `3 and 1 half`, `2/3` `2 thirds`, `1/16` `1 sixteenth`, `5/24` `5 20 fourths`, `1/100` `1 /100`.
- A dash at the start, or between characters below A, makes every dash of the line `minus`: `-5` is `minus 5`, `7 - 3` and `7-3` `7 minus 3`. In a line with both brackets, `(` is `minus` and `)` is dropped: `(12)` is `minus 12`.
- `E+` and `E-` after a number are exponents: `1.5E+10` is `1.5 times 10 to the 10`, `2E-3` `2 times 10 to the minus 3`.
- Escape sequences at a line's start are kept as they are.
- In the address and e-mail contexts, lines are joined into paragraphs, each followed by CR CR LF, and the text's end writes ` ` CR CR LF. A line under 39 characters gets `.  ` after it: `Hello` LF `World` is `Hello.  World`. A blank line writes `.` CR CR LF. A paragraph ends before its pieces measure more than 5,001 characters (section 3.3). A colon early in a line, or followed by more words, gets a space after it: `a:b` is `a: b`.
- Spaces become one, but two after `!`, `?` and a dot that ends a sentence, which depends on the word before the dot (an abbreviation, a number, a letter, a function word) and on whether the next word is capitalized: `Wow! nice. Yes` is `Wow!  nice.  Yes`.
- E-mail header lines are read too (sections 3.6 and 3.7). A `From:` line names its sender: `From: Jane Doe <jane@site.com>` is `From: Jane Doe.`, `From: jane@site.com (jane doe)` `From: Jane Doe.`. An `Author:` line names its authors: `Author: a@site.com (Jane Doe), bob@site.com` is `Author: Jane Doe, Bob.`. `To:`, `Cc:` and the lines of a header block that aren't headers are dropped, `Received:` starts a message (`Start of Message.`), `Subject:` and, in a header block, `Date:` are kept, and a `>` that starts a line goes.

**Exactness.** Checked against the original on 667,533 texts: each of the characters 1 to 255 but the line feed and the carriage return next to every rule's characters, dates and fractions with numbers of every length, signs, leading zeros and marks after them, words before and after the slash, dashes and brackets in every place, exponents, escape sequences, line ends, carriage returns, long lines with breaks at every place of their windows and runs of them, windows that end at the text's end, and 60,000 random texts of up to 4,577 characters. 659,470 gave the original's output. The other 8,063 are those section 4 lists. 28 recorded runs of the whole engine with texts in the spreadsheet context, tagged and untagged, at every mode, gave the original's audio and events ([engine.md](engine.md) §2).

Section 3 was checked against the original on 55,834 texts of up to 24,516 characters: lines of every length around the short line's, cut and whole, joined in every order; blank lines, carriage returns, tabs and runs of spaces and dots; paragraphs up to and past the limit, and last lines that do and don't fit; sentence ends after every kind of word, with every mark and quote after the dot, before capitalized and other words; paragraphs that start after sentence ends, blank lines and brackets; 10,000 random texts; each header's lines outside, in and after header blocks, quoted once and twice, in every order and with every message count; senders and authors with names, addresses, brackets and quotes; values and lines that end in commas, and held lines up to and past their limit; colons at every place of a line, with and without spaces and tabs around them; and 5,933 random e-mail texts. 51,847 gave the original's output. 445 are those section 4 lists, and the other 3,542 have words this section doesn't cover.

## 1. Lines

### 1.1 Windows

The pre-formatter reads its text, code page 1252 without the NUL, in **windows**. A window runs from where the reading stands to the next line feed, to the text's end, or for 720 characters, whichever comes first; it is **full** when it holds 720 characters and the text doesn't end there. The reading also knows whether it **continues** a line it cut, which it doesn't at the text's start.

1. A window that continues no line and isn't full is a line: its characters without their carriage returns. If the line is empty, it writes nothing, or CR LF if a line feed ends the window. Otherwise it writes the line (section 1.3), then CR LF if a line feed ends the window, else CR CR LF. The reading goes on after the line feed.
2. A full window that continues no line is cut at its **first cut** (section 1.2). The characters before it, without their carriage returns, are written as a line, then CR CR LF, and the reading goes on at the cut, continuing a line.
3. A window that continues a line is cut at its **cut**. With one, the characters before it, carriage returns kept, are written as a line, then CR CR LF, and the reading goes on at the cut, still continuing. Without one, the window is a line as in step 1, carriage returns kept, and the reading goes on continuing no line.

A carriage return is dropped from a window that continues no line even before a line feed: `a\r\nb` is `a` CR LF `b` CR CR LF.

### 1.2 Cuts

A **sentence break** is a dot followed by a space, even a space past the window's end.

A full window's **first cut** comes from a scan of its characters from the first. Each space is a space break, which it takes unless it takes the space right before it; each sentence break is one, which it takes unless it takes the sentence break two characters before it. In a run of spaces, it takes the first, the third and so on. The first cut is after the last sentence break it takes (after its space), else after the last space it takes, else at the window's end.

The **cut** of a window that continues a line is after its last sentence break, with the space after that break's space if there is one; else after its last space; else at its end if it is full. A window that isn't full and has neither has no cut.

### 1.3 Writing a line

A line is converted (section 2), then wrapped. The wrapping counts each character as a column, escape characters and tabs too, from the start of a **row**, the first at the line's start. At each character 77 columns or more past its row's start:
1. A space starts a new row after it.
2. A character that is the line's last, or before a space, is left alone.
3. Otherwise a new row starts after the row's last space but its first character. Without one, the character becomes a space, and a new row starts after it.

The rows aren't written apart: what shows of the wrapping is the characters it makes spaces.

## 2. A line

A line starts with its **escape sequences**: from its start, spaces and escape sequences, each `ESC [` up to the first letter after it (A to Z and a to z), the letter included. They are written as they are, to the last escape sequence's letter; the spaces after it go with the rest. An escape sequence without a letter takes the rest of the line. The rest of the line is converted in turn:
1. If it has two slashes or more, its date (section 2.1), and nothing else.
2. If it has one slash, its fraction (section 2.2).
3. If it then has a dash: if its first dash is a minus sign, its minus signs (section 2.3); else if its first dash is an exponent's, its exponent's dashes (section 2.5); else nothing more is done.
4. Its brackets (section 2.4), then its exponents (section 2.5).

A **number** is read in 16 bits as C's `atoi` reads one: spaces, tabs, line feeds, vertical tabs, form feeds and carriage returns, a `+` or a `-`, and the digits up to the first other character, modulo 65536. `65537` is 1, `-1` 65535, `x1` 0.

The words are those of the data pack's `preformatter.json` ([datapack.md](datapack.md) §15): `minus` is its `minus` word followed by a space.

### 2.1 Dates

The **month** is the characters from the last space before the first slash, or the line's start, up to the slash. If its number is 1 to 12 and it has three characters at most, the month and the slash become the month's name and a space, and every later slash a comma and a space: `12/25/1999` is `December 25, 1999`, `x 3/1/2` `x March 1, 2`, `03/1//` `March 1, , `. Otherwise the line is left as it is.

### 2.2 Fractions

The **numerator** is the characters before the slash back to the last space before them, the character right before the slash included even when it is a space; or nothing when the slash is the line's first character. If it has a dash, only what follows the first dash counts. The **token** is the characters after the slash up to the next space or the line's end. If the numerator's number or the token's is 0, the line is left as it is.

The token's **marks** are the `! , . : ; ?` at its end; the rest is the **denominator**. The fraction is written:
- for a denominator of three characters or more: `/`, the denominator and the marks in reverse order: `1/100.!` is `1 /100!.`;
- for one of one or two characters: its word, `s` unless the numerator's number is 1, and the marks in reverse order. The word of a denominator of one character, a digit 1 to 9, is its `denominators` word: 1 `one`, 2 `half`, 3 `third` and so on. One of two characters is read as tens and units:

  | Tens | Units | Word |
  |---|---|---|
  | `1` | a digit | its `teens` word: 10 `tenth` to 19 `nineteenth` |
  | `1` | another character | the first `otherUnits` word, `one` |
  | `2` to `9` | `0` | its `tens` word: 20 `twentieth` to 90 `ninetieth` (40 is `fourtieth`) |
  | another character | `1` to `9` | the tens, `0`, a space and its `ordinals` word: 23 `20 third`, 05 `00 fifth` |
  | another character | another character | the tens, `0`, a space and the second `otherUnits` word, `inetieth` |

The line becomes its characters up to the slash, a space, the fraction and, if characters follow the space after the token, a space and them. A space right after the token at the line's end is dropped: `1/2 ` is `1 half`.

If the line's first space after its leading spaces comes before the slash, and neither character next to it is a dash, `and` and a space are written after it: `3 1/2` is `3 and 1 half`, `1 /2` `1 and  half`. Then, for as long as the line holds ` and -` or `- and `, each ` and -` becomes ` -`, then each `- and ` `- `, those the text had included.

### 2.3 Minus signs

The line's first dash is a **minus sign** when only spaces come before it, or when it isn't the line's last character and the characters before and after it are both below `A` as signed bytes: below 65, or 128 and up. A line whose first dash is a minus sign is written again: its leading spaces as they are, then each dash as `minus`, after a space unless what is written ends in one, each space unless what is written ends in a space past the leading ones, and each other character as it is. `5 - 3` and `5-3` are `5 minus 3`, `x - y` is `x minus y`, and `x -y` and `a-b` stay as they are.

### 2.4 Brackets

In a line with a `(` and a `)`, the line is written again: its leading spaces as they are, then each `(` as `minus`, no `)`, each space unless what is written ends in a space past the leading ones, and each other character as it is. `(12)` is `minus 12`, `( 3 )` `minus 3 `.

### 2.5 Exponents

The line's first dash is an **exponent's** when it isn't a minus sign, follows an `E` after a digit or a dot, and isn't the line's last character. In such a line, each `E-` becomes ` times 10 to the minus `, then each `E+` ` times 10 to the `, then each `E minus ` ` times 10 to the minus `, before the brackets and the exponents: `2E-3` is `2 times 10 to the minus 3`.

The **exponents**: if the line's first `E` starts `E+` or `E minus ` (with the space), each `E+` becomes ` times 10 to the `, then each `E minus ` ` times 10 to the minus `. `1.5E+10` is `1.5 times 10 to the 10`, and `4E -2`, whose dash is a minus sign, `4 times 10 to the minus 2`. The words `times 10 to the` are the data pack's `exponent`.

## 3. The address and e-mail contexts

In the address and e-mail contexts, the pre-formatter reads the text in the windows of sections 1.1 and 1.2, but converts and wraps nothing: it reads e-mail messages' header lines, joins the text's lines into **paragraphs**, spaces each paragraph's words and writes it, followed by CR CR LF. This section covers how it reads, joins and spaces a text, and the header lines. What it makes of some words isn't ported yet: it reads out some abbreviations (`Mr.` is `Mister`, `etc.` `et cetera`, `Re` `Regarding`) and smileys (`;)` is `wink`), spells some words (`gre` is `g-r-e`, `BCC` `B-C-C`), reads some marks and letters as words (`;` alone is `semicolon`, `A` `aye`), splits some marks from words (`hello#` is `hello #`), makes a dash between letters a space (`Return-Path` is `Return Path`), drops a `>` followed by a space past a line's start, reads runs of some marks (`--`, `==`, `~`), numbers, runs of single letters and the values of `Date:` lines its own way. This section doesn't cover texts with such words.

### 3.1 Pieces

Each window gives a **piece**, its characters without the line feed, carriage returns dropped as section 1.1 has it: a **cut piece** when the window is cut, a **line** when a line feed ends it, and the **last line** when it runs to the text's end. A piece is **cleaned**: its tabs become spaces, each run of two spaces or more becomes one space unless it follows a dot, and its leading spaces are dropped. Then its colons are spaced when the line it was read as gets them, but where section 3.6 says otherwise. Its **length** is the cleaned piece's, the colons' spaces counted.

A line's colons get spaces when, without its leading spaces and one `>` at its start, its first colon has no space or tab before it or right after it, and either a space or a tab comes after the colon or the colon is the line's 2nd to 12th character. Then each colon of the piece that no space or tab follows gets a space after it: `To:` is `To: `, `a:b:c` `a: b: c` and `see:the list` `see: the list`; `Dear Sir:`, TAB `To:`, `:ab`, `> a:b` and `abcdefghijkl:m` stay as they are.

### 3.2 Paragraphs

The pre-formatter starts with an empty paragraph and takes each piece but the last line in turn, those the e-mail reading leaves (section 3.6) and as it leaves them:

1. A cut piece of one character at most, leading spaces and tabs and trailing spaces aside, ends the paragraph. If the paragraph has anything, it is written (section 3.4), then the character, or a space if there is none, and CR CR LF; otherwise the character and CR CR LF.
2. An empty line, or a line of spaces, ends the paragraph too. If the paragraph has anything, it is written, then `.` and CR CR LF, and the next paragraph starts a sentence (section 3.4); otherwise CR LF is written.
3. A piece that ends in a dash loses the dash, unless it is the piece's only character or a space comes before it, and gets a space: `well-` is `well `, `a -` `a - `.
4. If the paragraph has anything and the piece doesn't fit in it (section 3.3), the paragraph is written.
5. The piece is added to the paragraph:
   - a piece that ends in a colon, or a short line, with `.  ` after it, or `  ` if it ends in a dot. A line is **short** when its length before step 3, with the characters the paragraph holds from the pieces cut from the same line before it, and 1 more if the paragraph holds a line before those, is under 39;
   - a line that ends in a dot and spaces, as it is;
   - any other piece without its trailing spaces, followed by a space.
6. If the piece didn't fit, the paragraph is written, the piece alone in it.

`Hello` LF `World` is `Hello.  World` CR CR LF, then ` ` CR CR LF for the text's end (section 3.5). `Dear Sir:` LF `ok` is `Dear Sir:.  ok`, and `Note:` LF `ok` is `Note: . ok`. `one` LF LF LF `two` is `one.` CR CR LF `.` CR CR LF CR LF `two` CR CR LF ` ` CR CR LF.

### 3.3 The paragraph limit

The paragraph's **size** is the sum of the lengths of the pieces added to it, before step 3, with 2 more for each line. A piece **doesn't fit** when the size and twice its length after step 3, with 2 more for a line, come to more than 5,001. The last line doesn't fit when the size and twice its length without its trailing spaces, plus 1, come to more than 5,001.

### 3.4 Writing a paragraph

A paragraph is written spaced, without its trailing spaces, followed by CR CR LF. One that starts with `(` is spaced with a space before it, which is written: `(see below)` is ` (see below)`.

The spacing first changes the paragraph:
1. Each run of two dots or more becomes `.  `: `end...  More` is `end.  More`.
2. Each run of two spaces or more after a dot becomes two spaces, followed by `. ` once for each six spaces past the first: `end.` and seven spaces before `More` are `end.  .  More`.
3. Each `,.` after a character other than a digit, a comma or a space becomes `, .`: `Hi,. there` is `Hi, . there`.
4. Each dash after a letter or a digit, before a space or at the end, becomes a space.

Then it writes the spaces before the first word as they are, and each word after one space, or two where the rules below say. Letters are `A` to `Z` and `a` to `z`, capitals `A` to `Z`, digits `0` to `9`. The spacing keeps track of whether a sentence **starts**, and counts the words it **skips** while one does.

A paragraph starts a sentence when the paragraph written before it ended where one starts, or ended at a blank line (section 3.2, step 2), or when one of its words **restarts** it: a word that starts with two digits or more, not all zeros, a dot and `)`, `]` or a `"` not followed by `)`, such as `12.)`. A paragraph that starts with `(` doesn't. The text's first paragraph starts one only when a word restarts it.

A word **ends in a dot** when its last character is a dot, or its last but one is a dot and its last one of `) " ; , :`. Its **dotted part** runs to that dot, and a `)` or `"` after the dot **closes** it. Its **core** is the dotted part without its dot, then without the `) ! ? : ; , .` at its end and the `(` at its start. Its **kind** comes from its dotted part:
1. A dotted part ending in `".` is an **initial** if what comes before the quote, without the `. ! ?` at its end, is one capital, a **letter** if it is one other letter, and a **word** otherwise.
2. Otherwise, by its core: a core ending in `"` is a word; one without letters and digits is **empty**; a digit followed by digits and dots is a **number**, or a word when a sentence starts and the dot is closed; one capital is an initial, one other letter a letter; a function word is a **start** when a sentence starts and a **function word** otherwise; anything else is a word.

The **function words** are the data pack's `address` `functionWords` ([datapack.md](datapack.md) §15), `about`, `after`, `and` and so on, case aside, and the **abbreviations** its `abbreviations`, `adj`, `asst` and so on. A word ending in a dot is an abbreviation when its core, without a quote before the dot, is one, case aside. A word is **capitalized** when, without the `(` and `"` at its start, it starts with a capital that is alone or followed by a character other than a capital. A word is **skippable** when, without the `(` at its start, the `, ) ; : "` at its end and then a `~` at its start, it is a function word, or, without a `"` at its start, a digit followed by digits, dots and dashes.

Each word in turn:
1. A word that ends in a dot:
   - The spaces after it are two if its dotted part, without its dot and then the `, ) : ; "` at its end, ends in `!` or `?`. Otherwise, with two spaces or more after it in the paragraph, two after a word, one after a number or a function word, and after the other kinds two before a capitalized word, else one. With one space after it, one after an abbreviation that is neither a start nor one of the data pack's `spacedAbbreviations`, `lat` and `long`; otherwise two after a word, a letter, a start or an empty one before a capitalized word, else one.
   - A word of kind word that ends in `".` and has two spaces or more after it loses its quote, and a `.`, `!` or `?` before the quote: `"Stop!".  Go` is `"Stop.  Go`.
   - If a sentence starts and the word is a number, it is skipped. Otherwise a sentence starts after it when it is a start, or neither a function word, a number nor an abbreviation, and the count of skipped words starts again.
2. A word that ends in `!` or `?`, the `, ) : ; "` at its end aside, has two spaces after it. Without those and then the `!` and `?` at its end, if it ends in a dot, a sentence starts after it as step 1 has it; if not, it is skipped when a sentence starts and it is skippable; otherwise no sentence starts after it, and the count starts again.
3. Any other word is skipped when a sentence starts and it is skippable; otherwise no sentence starts after it, and the count starts again.

Before each word but the first come two spaces if a sentence starts, words were skipped and the word is capitalized; otherwise the spaces the word before it gave, or one. The paragraph **ends where a sentence starts** when one starts after its last word. `Wow! nice. Yes` is `Wow!  nice.  Yes`, `See p. Two` is `See p.  Two`, `Item 3. Next` stays as it is, and `Done.` LF `by the Way` is `Done.  by the  Way`.

### 3.5 The text's end

After the last piece:
- Without a last line, when the text is empty or ends in a line feed, if the paragraph has anything, it is written, then a space and CR CR LF; otherwise nothing is: `hi` LF is `hi.` CR CR LF ` ` CR CR LF, and `hi` LF LF `hi.` CR CR LF `.` CR CR LF.
- With a last line of one character, leading spaces and tabs and trailing spaces aside, the paragraph, if it has anything, is written, then the character and CR CR LF: `x` is `x` CR CR LF.
- With a last line that has other characters and doesn't fit (section 3.3), while the paragraph has anything, the paragraph is written, then the last line as a paragraph of its own, and nothing more.
- Otherwise the last line, without its trailing spaces, is added to the paragraph. If the paragraph has anything, it is written, then a space and CR CR LF; otherwise CR CR LF is written.

### 3.6 E-mail messages

Before section 3.2 takes a piece, a cut piece too, the pre-formatter reads it as a line of an e-mail message. The piece's **line** is the piece without its leading spaces. A line is a **header line** when, after a `>` at its start if it has one, it starts with a header's name, as the data pack's `address` `headers` ([datapack.md](datapack.md) §15) write it, case and all, and a colon: `From: Jane` and `>From: Jane` are header lines, `from: Jane`, `From : Jane` and `> From: Jane` aren't. In and after a header block, a header marked `quotedTwice` may follow two `>`: `>>From: Jane` is a header line there. Its **value** is what follows the colon. A line is **blank** when it is empty or is `>`. A line **unquoted** is the line without its `>`, if it starts with one, leading spaces aside, and a space after that `>`: `> Hi` and `>Hi` are `Hi`.

Without its leading spaces and a `>` at its start, a piece is **colon-led** when its first colon is its 3rd character or later, and **tightly** colon-led when no space or tab comes before that colon either. A line **looks like a header** when, unquoted, cleaned (section 3.1), as the last line without its trailing spaces too, and its colons spaced if the line's get spaces, it has a colon followed by a space: `ab:cd` and `:ab c` look like headers, `:ab`, `a :b` and TAB `a:b` don't.

The reading is **outside** a header block at first. Header lines take it **in** a block, and a blank line in a block takes it **after** the block. It counts the **messages** that have started, and keeps whether the block has an Author line, a **held** line, and a value that **goes on**: an author's, a names value or a dropped one.

Each header has an action outside a block, one in a block and one after a block, or none there (the header's `outside`, `block` and `after`). A header with message counts (`outsideNames`, `insideNames`) is read as **names** instead once that many messages have started: outside a block the first count, in and after a block the second. The data pack's headers:

| Header | Outside a block | In a block | After a block | Names: outside, in and after | Marked |
|---|---|---|---|---|---|
| `From` | sender | sender | sender | | `quotedTwice` |
| `To`, `TO` | block | | block | after 1, after 2 | `continues` |
| `Cc`, `CC`, `cc`, `BCC`, `Priority` | dropped | | block | after 2, after 2 | |
| `Subject` | subject | subject | subject | | |
| `Received`, `Return-Path` | message | | | | `clearsAuthor` |
| `Path`, `Xref` | message | | block | | |
| `Author` | author | author | author | | |
| `Date` | | date | date | | |
| `Organization` | | kept | kept | | |

Each piece in turn:
1. If a line is held, it is let go. If the piece isn't colon-led, the held line is **released**, and nothing more is done with the piece. The reading is outside a block, and an empty line is added to the paragraph as section 3.2 adds a line, without step 2: it gets `.  `. Then, if the held line's and the piece's **measures** come to under 743, a cut piece's without its trailing spaces and a line's with 1 more, the held line, unquoted and cleaned, and `.  ` are added to the paragraph, and the next line's length counts from there (section 3.2, step 5). Then the piece, unquoted, goes to section 3.2 without step 2, its colons spaced if the piece's get spaces; as the last line, it doesn't count as one character (section 3.5). A line's measure is its length without its leading spaces and a `>` at its start, its tabs made spaces, each run of two spaces or more one unless it follows a dot, and its colons' spaces counted.
2. If a value goes on, it stops. Unless the piece is tightly colon-led, or the value is a dropped one and the line is blank, the piece goes on with it, and nothing more is done with the piece. If the piece, its trailing spaces and tabs aside, ends in a comma, the value goes on again. A dropped value's piece writes CR CR LF. An author's or a names value's piece is written as **continued names** (below), followed by `.` unless they end in one: an author's after CR CR LF and followed by CR LF if the piece is a line and the last name wasn't taken out of an address, else by CR CR LF, and the reading is in a block; a names value's as a paragraph (section 3.4).
3. Outside a block, a header line is written (section 3.7). Any other line goes to section 3.2 unquoted.
4. In a block:
   1. A line right after a Subject line **goes on** after it. If it is blank, CR LF is written for a line and CR CR LF otherwise, and the reading is after the block. If it doesn't look like a header, it goes on unquoted (section 3.7), it is held if its last character is a comma and it isn't colon-led, and nothing more is done. Otherwise it is read as the next steps have it.
   2. A blank line writes CR CR LF, and the reading is after the block.
   3. A header line with an action in a block is written.
   4. Any other line is dropped: CR CR LF is written. If it is a line of a header marked `continues`, and its value, its trailing spaces and tabs aside, ends in a comma, the value goes on as a dropped one. The line is held if its last character is a comma and it isn't colon-led.
5. After a block, a header line is written. Any other line, a blank one too, goes to section 3.2 unquoted, and the reading is outside a block.

`From: Jane Doe <jane@site.com>` LF `To: Bob` LF `Subject: Lunch` LF `at noon` LF LF `See you` LF is `From: Jane Doe.` CR CR LF CR CR LF `Subject: Lunch.` CR CR LF `at noon.` CR CR LF CR CR LF `See you.` CR CR LF ` ` CR CR LF. `From: jo` LF `hi there,` LF `more` LF is `From: Jo.` CR CR LF CR CR LF `. hi there, . more.` CR CR LF ` ` CR CR LF.

The **continued names** in a piece: if the piece, its spaces and tabs aside, is empty or is `>`, there are none. Otherwise its first word, unquoted and without its leading spaces and tabs, runs to the first space, tab, colon, comma or semicolon after its first character. At a space, tab or colon, they are the word, the colon if it is one, then a space and the names (section 3.7) in the rest of the piece if those have anything. At a comma or semicolon, they are the word and the names in the rest of the piece from that character on. With none of these characters, they are the piece as it is. `Author: jane doe,` LF `bob smith` LF is CR CR LF `Author: Jane Doe,.` CR LF CR CR LF `bob Smith.` CR LF.

### 3.7 Header lines

A header line's value first has its colons spaced if the header's name, a colon and the value, read as a line, get spaces (section 3.1). The value is **spaced** when it starts with a space or a tab, or its colons got spaces. If the value, its trailing spaces and tabs aside, ends in a comma, an author's or names value goes on into the next line (section 3.6), and so does a value of a header marked `continues`, as a dropped one.

Then the header line ends the paragraph: if it has anything, it is written (section 3.4). Then, by the action:
- **Dropped** and **block**: CR CR LF is written.
- **Message**: CR CR LF, the data pack's `messageStart`, `. ` and CR CR LF are written: `Received: x` LF is CR CR LF `Start of Message. ` CR CR LF.
- **Author**: the value's tabs become spaces, and each dot followed by two spaces or more becomes as many dots as there are spaces, then a space: `J.  Doe` is `J.. Doe`. The header's name and `:` are written, then a space and the value's names (below) if it has any, and `.` unless that ends in a dot. CR CR LF comes before them unless the line before was a Subject line, and after them CR LF if the piece is a line and the last name wasn't taken out of an address, else CR CR LF. `Author: jane doe` LF is CR CR LF `Author: Jane Doe.` CR LF.
- **Sender**, **names**, **kept**, **date** and **subject**: what the value gives is, for sender, the value's person, for names its names, and for the others the value cleaned (section 3.1) without its trailing spaces. The paragraph is the header's name and `:`, then a space if what the value gives has anything and the value is spaced, then what it gives. The header line is **short** when the piece is a line, the paragraph is under 38 characters, and it is no date and no person or names taken out of an address. If the paragraph ends in a dash, the dash goes, unless a space comes before it, and a space follows. A short header line gets `.  `, or `  ` if it ends in a dot; any other gets `.` unless it ends in one. The paragraph is written. `From: jane@site.com` LF is `From: Jane.` CR CR LF, and in a block `Organization:a b` LF is `Organization: a b.` CR CR LF and `Organization:ab` LF `Organization:ab.` CR CR LF.
- A subject line in or after a block that has an Author line always gets its space. As a line, its paragraph is instead the header's name, `: ` and the value cleaned, trailing spaces and all, ended as section 3.2 ends a line it adds: a dash at its end goes as step 3 has it; then, under 39 characters before that or ending in a colon, it gets `.  `, or `  ` if it ends in a dot; otherwise one that ends in a dot and spaces stays as it is, and any other loses its trailing spaces and gets a space; then it gets `.`. The paragraph is written.

Then the reading moves on:
- An author line, unless the line before was a Subject line, starts a message: the count goes up by one, and the block has an Author line. Outside a block, a block, message or sender line starts a message too, and if its header is marked `clearsAuthor`, the block has no Author line.
- After a subject line in or after a block, the reading is in the block and the next line goes on after it (section 3.6), unless the block has an Author line: then the reading is outside a block.
- After any other header line, the reading is in a block if it was in or after one, or if the line was a block, message or sender line, or an author line but right after a Subject line. Otherwise it stays outside.

A line that **goes on** after a Subject line, unquoted: if it is the text's last line, its leading spaces go, then its tabs become spaces and each run of two spaces or more one, unless it follows a dot. If that leaves only spaces, they are written, then CR CR LF. Otherwise the paragraph is what is left without leading and trailing spaces, and is written. A piece that isn't the last line is added as section 3.2 has it, but without step 2 and with its colons as they are, and the paragraph, if it has anything, is written.

The **person** in a value: the value's tabs become spaces, and its leading and trailing spaces go. A `(` or `<` is **free** when it comes at the value's start or after a space, comma or semicolon. The **delimiters** are `@ ! % / :` and an `at` after a space, comma or semicolon and before one or the value's end.
1. If the first free `(` follows a space and the word before it has an `@`, the person is what lies between it and the first `)` at the value's end or before a space, comma or semicolon, without leading and trailing spaces, if that has anything: `jane@site.com (Jane Doe)` is `Jane Doe`.
2. Otherwise, with a free `(`, the person is the part before it, up to its first delimiter. With no free `(` but a free `<`, it is the part before the `<` without leading and trailing spaces, and without a `"` at its start and one at its end: `"Jane Doe" <jane@site.com>` is `Jane Doe`. With neither, it is the value up to its first delimiter: `Jane at Site` is `Jane`.
3. The person is **taken out of an address** but in the last case, where it is only when the delimiter cut something off.
4. The person loses its leading and trailing spaces, each run of two spaces or more becomes one unless it follows a dot, and each word's first letter becomes a capital. A word starts at the person's start, after a space, comma or semicolon, and after a dash that follows another character. Each other capital becomes small unless the character before it is a small letter, a word's first letter counting as made a capital: `JANE mcDONALD` is `Jane McDonald`, and `jOHN dE` `John De`.

The **names** in a value: the value's tabs become spaces, and it is split at its commas and semicolons into **items**. Each item gives a name, with the item's leading spaces before it; the names, with the commas and semicolons between them, are made as a person is in step 4. They are taken out of an address when the last name is. An item's name:
1. If the item, without its leading and trailing spaces, has a `(` after a space, and the word before the first such `(` has an `@`, the name is what lies between that `(` and the first `)` at the item's end or before a space, without leading and trailing spaces, if that has anything.
2. Otherwise, with such a `(`, the name is the item before it, up to its first delimiter.
3. With no such `(` but a free `<`, the name is the item's part before the `<`, its leading and trailing spaces kept and what lies between them without a `"` at its start and one at its end.
4. With neither, the name is the item up to its first delimiter, followed by a space if one comes after the delimiter: `ab:cd ef, gh` is `Ab , Gh`, and `ab:cd, ef` `Ab, Ef`.
5. The name is taken out of an address but in the last case, where it is only when the delimiter cut something off. `Author: a@site.com (Jane Doe), bob@site.com` LF is CR CR LF `Author: Jane Doe, Bob.` CR CR LF.

## 4. Where the port differs

- **A month of more than three characters.** A date whose month has a number of 1 to 12 but more than three characters, such as `0001/2/3` or `65537/1/1`, doesn't give the original a usable result: it can crash. The port leaves the line as it is.
- **Very long words and lines.** Where a fraction's token runs to about 60 characters, or the conversion makes a line grow to about 720 characters, the original's output doesn't follow section 2's rules. The port applies them all the same. Of the texts checked, 2,517 were such texts.
- **Paragraphs near the limit.** In a paragraph of about 4,500 characters or more, the original keeps some of the spaces and tabs at a line's end where lines join, and writes different dots after a dot's run of spaces (section 3.4). The port applies section 3 all the same. Of the texts checked, 445 had such a paragraph.
- **Output size.** TextData gives the pre-formatter an output buffer of the text's length and 4,001 bytes, at least 13,500. The port has no such limit.
