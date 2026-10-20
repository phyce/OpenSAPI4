# TruVoice: the pre-formatter

How L&H TruVoice (`tv_enua.dll`) pre-formats the text it reads in its address, e-mail and spreadsheet contexts. TextData sends a text's segments in these contexts through the pre-formatter before it queues them ([engine.md](engine.md) §1.4), with their characters 128 and up made spaces but `’ £ ¥ © ®`. This spec covers the spreadsheet context: how the pre-formatter reads a text's lines and writes them (section 1) and what it makes of a line (section 2); how the address and e-mail contexts read a text, join its lines and space its words (section 3); and where the port differs (section 4). The rest of the address and e-mail contexts' pre-formatter isn't ported yet, and the engine doesn't use section 3 yet.

**In short.**
- In the spreadsheet context, the text is read a line at a time, up to each line feed. A line ends in CR LF, and the text's last one, if no line feed ends it, in CR CR LF.
- A line of more than 720 characters is cut after a dot and a space, else after a space, else at 720 characters, and each part ends in CR CR LF. Lines are wrapped at 77 columns without line breaks: a word too long for a row gets a character made a space.
- A line with two slashes is a date: `12/25/1999` is `December 25, 1999`.
- A line with one slash has a fraction: `3 1/2` is `3 and 1 half`, `2/3` `2 thirds`, `1/16` `1 sixteenth`, `5/24` `5 20 fourths`, `1/100` `1 /100`.
- A dash at the start, or between characters below A, makes every dash of the line `minus`: `-5` is `minus 5`, `7 - 3` and `7-3` `7 minus 3`. In a line with both brackets, `(` is `minus` and `)` is dropped: `(12)` is `minus 12`.
- `E+` and `E-` after a number are exponents: `1.5E+10` is `1.5 times 10 to the 10`, `2E-3` `2 times 10 to the minus 3`.
- Escape sequences at a line's start are kept as they are.
- In the address and e-mail contexts, lines are joined into paragraphs, each followed by CR CR LF, and the text's end writes ` ` CR CR LF. A line under 39 characters gets `.  ` after it: `Hello` LF `World` is `Hello.  World`. A blank line writes `.` CR CR LF. A paragraph ends before its pieces measure more than 5,001 characters (section 3.3).
- Spaces become one, but two after `!`, `?` and a dot that ends a sentence, which depends on the word before the dot (an abbreviation, a number, a letter, a function word) and on whether the next word is capitalized: `Wow! nice. Yes` is `Wow!  nice.  Yes`.

**Exactness.** Checked against the original on 667,533 texts: each of the characters 1 to 255 but the line feed and the carriage return next to every rule's characters, dates and fractions with numbers of every length, signs, leading zeros and marks after them, words before and after the slash, dashes and brackets in every place, exponents, escape sequences, line ends, carriage returns, long lines with breaks at every place of their windows and runs of them, windows that end at the text's end, and 60,000 random texts of up to 4,577 characters. 659,470 gave the original's output. The other 8,063 are those section 4 lists. 28 recorded runs of the whole engine with texts in the spreadsheet context, tagged and untagged, at every mode, gave the original's audio and events ([engine.md](engine.md) §2).

Section 3 was checked against the original on 42,290 texts of up to 24,516 characters: lines of every length around the short line's, cut and whole, joined in every order; blank lines, carriage returns, tabs and runs of spaces and dots; paragraphs up to and past the limit, and last lines that do and don't fit; sentence ends after every kind of word, with every mark and quote after the dot, before capitalized and other words; paragraphs that start after sentence ends, blank lines and brackets; and 10,000 random texts. 40,529 gave the original's output. 449 are those section 4 lists, and the other 1,312 have words this section doesn't cover.

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

In the address and e-mail contexts, the pre-formatter reads the text in the windows of sections 1.1 and 1.2, but converts and wraps nothing: it joins the text's lines into **paragraphs**, spaces each paragraph's words and writes it, followed by CR CR LF. This section covers how it reads, joins and spaces a text. What it makes of some words isn't ported yet: it reads out some abbreviations (`Mr.` is `Mister`, `etc.` `et cetera`) and smileys (`;)` is `wink`), spells some words (`gre` is `g-r-e`), reads some marks and letters as words (`;` alone is `semicolon`, `A` `aye`), splits some marks from words (`hello#` is `hello #`), and reads numbers and runs of single letters its own way. This section doesn't cover texts with such words.

### 3.1 Pieces

Each window gives a **piece**, its characters without the line feed, carriage returns dropped as section 1.1 has it: a **cut piece** when the window is cut, a **line** when a line feed ends it, and the **last line** when it runs to the text's end. A piece is **cleaned**: its tabs become spaces, each run of two spaces or more becomes one space unless it follows a dot, and its leading spaces are dropped. Its **length** is the cleaned piece's.

### 3.2 Paragraphs

The pre-formatter starts with an empty paragraph and takes each piece but the last line in turn:

1. A cut piece of one character at most, leading spaces and tabs and trailing spaces aside, ends the paragraph. If the paragraph has anything, it is written (section 3.4), then the character, or a space if there is none, and CR CR LF; otherwise the character and CR CR LF.
2. An empty line, or a line of spaces, ends the paragraph too. If the paragraph has anything, it is written, then `.` and CR CR LF, and the next paragraph starts a sentence (section 3.4); otherwise CR LF is written.
3. A piece that ends in a dash loses the dash, unless it is the piece's only character or a space comes before it, and gets a space: `well-` is `well `, `a -` `a - `. A piece that ends in a colon, has no space and had no tab before it gets a space: `To:` is `To: `, and `Dear Sir:` and TAB `To:` stay as they are.
4. If the paragraph has anything and the piece doesn't fit in it (section 3.3), the paragraph is written.
5. The piece is added to the paragraph:
   - a piece that ends in a colon, or a short line, with `.  ` after it, or `  ` if it ends in a dot. A line is **short** when its length before step 3, with the characters the paragraph holds from the pieces cut from the same line before it, and 1 more if the paragraph holds a line before those, is under 39;
   - a line that ends in a dot and spaces, as it is;
   - any other piece without its trailing spaces, followed by a space.
6. If the piece didn't fit, the paragraph is written, the piece alone in it.

`Hello` LF `World` is `Hello.  World` CR CR LF, then ` ` CR CR LF for the text's end (section 3.5). `Dear Sir:` LF `ok` is `Dear Sir:.  ok`, and `To:` LF `ok` is `To: .  ok`. `one` LF LF LF `two` is `one.` CR CR LF `.` CR CR LF CR LF `two` CR CR LF ` ` CR CR LF.

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
1. A word that ends in a dot, with a word or spaces after it:
   - The spaces after it are two if its dotted part, without its dot and then the `, ) : ; "` at its end, ends in `!` or `?`. Otherwise, with two spaces or more after it in the paragraph, two after a word, one after a number or a function word, and after the other kinds two before a capitalized word, else one. With one space after it, one after an abbreviation that is neither a start nor one of the data pack's `spacedAbbreviations`, `lat` and `long`; otherwise two after a word, a letter, a start or an empty one before a capitalized word, else one.
   - A word of kind word that ends in `".` and has two spaces or more after it loses its quote, and a `.`, `!` or `?` before the quote: `"Stop!".  Go` is `"Stop.  Go`.
   - If a sentence starts and the word is a number, it is skipped. Otherwise a sentence starts after it when it is a start, or neither a function word, a number nor an abbreviation, and the count of skipped words starts again.
2. A word that ends in `!` or `?`, the `, ) : ; "` at its end aside, has two spaces after it. Without those and then the `!` and `?` at its end, if it ends in a dot, a sentence starts after it as step 1 has it; if not, it is skipped when a sentence starts and it is skippable; otherwise no sentence starts after it, and the count starts again.
3. Any other word, a word ending in a dot with nothing after it too, is skipped when a sentence starts and it is skippable; otherwise no sentence starts after it, and the count starts again.

Before each word but the first come two spaces if a sentence starts, words were skipped and the word is capitalized; otherwise the spaces the word before it gave, or one. The paragraph **ends where a sentence starts** when one starts after its last word. `Wow! nice. Yes` is `Wow!  nice.  Yes`, `See p. Two` is `See p.  Two`, `Item 3. Next` stays as it is, and `Done.` LF `by the Way` is `Done.  by the  Way`.

### 3.5 The text's end

After the last piece:
- Without a last line, when the text is empty or ends in a line feed, the paragraph, if it has anything, is written, then a space and CR CR LF.
- With a last line of one character, leading spaces and tabs and trailing spaces aside, the paragraph, if it has anything, is written, then the character and CR CR LF: `x` is `x` CR CR LF.
- With a last line that has other characters and doesn't fit (section 3.3), while the paragraph has anything, the paragraph is written, then the last line as a paragraph of its own, and nothing more.
- Otherwise the last line, without its trailing spaces, is added to the paragraph. If the paragraph has anything, it is written, then a space and CR CR LF; otherwise CR CR LF is written.

## 4. Where the port differs

- **A month of more than three characters.** A date whose month has a number of 1 to 12 but more than three characters, such as `0001/2/3` or `65537/1/1`, doesn't give the original a usable result: it can crash. The port leaves the line as it is.
- **Very long words and lines.** Where a fraction's token runs to about 60 characters, or the conversion makes a line grow to about 720 characters, the original's output doesn't follow section 2's rules. The port applies them all the same. Of the texts checked, 2,517 were such texts.
- **Paragraphs near the limit.** In a paragraph of about 4,500 characters or more, the original keeps some of the spaces and tabs at a line's end where lines join, and writes different dots after a dot's run of spaces (section 3.4). The port applies section 3 all the same. Of the texts checked, 449 had such a paragraph.
- **Output size.** TextData gives the pre-formatter an output buffer of the text's length and 4,001 bytes, at least 13,500. The port has no such limit.
