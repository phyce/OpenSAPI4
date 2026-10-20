# TruVoice: the pre-formatter

How L&H TruVoice (`tv_enua.dll`) pre-formats the text it reads in its address, e-mail and spreadsheet contexts. TextData sends a text's segments in these contexts through the pre-formatter before it queues them ([engine.md](engine.md) §1.4), with their characters 128 and up made spaces but `’ £ ¥ © ®`. This spec covers the spreadsheet context: how the pre-formatter reads a text's lines and writes them (section 1), what it makes of a line (section 2), and where the port differs (section 3). The address and e-mail contexts' pre-formatter isn't ported yet.

**In short.**
- The text is read a line at a time, up to each line feed. A line ends in CR LF, and the text's last one, if no line feed ends it, in CR CR LF.
- A line of more than 720 characters is cut after a dot and a space, else after a space, else at 720 characters, and each part ends in CR CR LF. Lines are wrapped at 77 columns without line breaks: a word too long for a row gets a character made a space.
- A line with two slashes is a date: `12/25/1999` is `December 25, 1999`.
- A line with one slash has a fraction: `3 1/2` is `3 and 1 half`, `2/3` `2 thirds`, `1/16` `1 sixteenth`, `5/24` `5 20 fourths`, `1/100` `1 /100`.
- A dash at the start, or between characters below A, makes every dash of the line `minus`: `-5` is `minus 5`, `7 - 3` and `7-3` `7 minus 3`. In a line with both brackets, `(` is `minus` and `)` is dropped: `(12)` is `minus 12`.
- `E+` and `E-` after a number are exponents: `1.5E+10` is `1.5 times 10 to the 10`, `2E-3` `2 times 10 to the minus 3`.
- Escape sequences at a line's start are kept as they are.

**Exactness.** Checked against the original on 666,948 texts: each of the characters 1 to 255 but the line feed and the carriage return next to every rule's characters, dates and fractions with numbers of every length, signs, leading zeros and marks after them, words before and after the slash, dashes and brackets in every place, exponents, escape sequences, line ends, carriage returns, long lines with breaks at every place of their windows and runs of them, and 60,000 random texts of up to 4,577 characters. 658,881 gave the original's output. The other 8,067 are those section 3 lists. 28 recorded runs of the whole engine with texts in the spreadsheet context, tagged and untagged, at every mode, gave the original's audio and events ([engine.md](engine.md) §2).

## 1. Lines

### 1.1 Windows

The pre-formatter reads its text, code page 1252 without the NUL, in **windows**. A window runs from where the reading stands to the next line feed, to the text's end, or for 720 characters, whichever comes first; it is **full** when it holds 720 characters. The reading also knows whether it **continues** a line it cut, which it doesn't at the text's start.

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

## 3. Where the port differs

- **A month of more than three characters.** A date whose month has a number of 1 to 12 but more than three characters, such as `0001/2/3` or `65537/1/1`, doesn't give the original a usable result: it can crash. The port leaves the line as it is.
- **Very long words and lines.** Where a fraction's token runs to about 60 characters, or the conversion makes a line grow to about 720 characters, the original's output doesn't follow section 2's rules. The port applies them all the same. Of the texts checked, 2,521 were such texts.
- **Output size.** TextData gives the pre-formatter an output buffer of the text's length and 4,001 bytes, at least 13,500. The port has no such limit.
