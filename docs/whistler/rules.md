# SAPI 4 Microsoft engine: rewrite rules

The text analyser inside `msttssyn.dll` (text.md) does most of its work with compiled rewrite rules: normalisation, letter-to-sound, syllables, stress, part of speech, phrasing and connected speech. There are three rule modules:

| Module | Groups | Rules | Run by | Does |
|---|---|---|---|---|
| C | 64 | 1467 | transcription, group 50 | normalisation, dictionary, morphology, letter-to-sound, syllables, stress (text.md §9, `pronunciation.md`) |
| B | 16 | 351 | transcription, groups 16, 6, 3 | part of speech, phrase breaks, plural and possessive endings |
| A | 16 | 176 | analysis, groups 5, 10, 16 | sentence type, accents and tones, connected speech, cleanup (`prosody.md`) |

All three run on the same interpreter. This spec describes the interpreter and the rule format. The rules themselves are data: the converter copies them into the data pack, and the Go engine runs them. Behaviour must match byte for byte, including the quirks in section 11.

## 1. Cells and tiers

The analyser state has:
- `start` and `end`: the sentence's first and last cell, numbered from 1, inclusive;
- five byte **tiers**, T1–T5, of 0x580 cells each;
- an **anchor count** per cell (event markers anchored there, text.md §8.2).

Cell i of every tier and of the anchors belong together. Every insert and delete applies to all six arrays at once, so they stay aligned.

Rules see only symbols 0x0e–0xaf. Two symbols are special:
- **0x0e: edge.** It marks both ends of the text while a group runs. Only the interpreter writes it, and no rule uses it as a literal.
- **0x7e `~`: empty cell.** Inserted cells start as `~`, padding writes `~`, and matching can skip `~` cells.

Between the edges the tiers hold only symbols 0x0e–0xaf: the reader stores no others (text.md §8.1), and the rules, the series maps, the dictionaries and the natives write no others (the data pack checks the first three, datapack.md §9 and §10). So a class or series test never sees another byte. The Go engine treats a byte outside the range as a member of nothing. The tables in the original are laid out so that such a byte would read a neighbouring class's row.

## 2. Module data

Each module has the tables below. All are 1-based: entry 1 is the first. The converter checks that every byte of the code area belongs to exactly one rule. The data pack stores them per module, with each rule's bytecode on its own (datapack.md §9).

| Table | Indexed by | Entry |
|---|---|---|
| groupFirst, groupLast | group | first and last rule of the group |
| classes | class, symbol 0x0e–0xaf | member or not |
| series | series, symbol | member or not |
| seriesIndex | series, symbol | the member's index 1–26 |
| seriesMaps | map, index | output symbol |
| variableTypes | variable | {class or series, which one}: the first binding must be a member |
| ruleOffset | rule | where the rule's bytecode starts |
| minimumLeft (L), minimumRight (R) | rule | cells needed before the focus and from the focus start to the end |
| outputLength | rule | symbols the action writes |
| skipOnSuccess | rule | rules skipped after a success (modes 3, 4, 7, 8, 11, 12, 15, 16) |
| blockCount | rule | members of the block that starts here (modes 9–16) |
| required1, required2 | rule | symbols that must occur in T1 for the rule to be tried; 0x0e means none |
| code | | the bytecode, 1-based |
| procedures | 3…n | what `CALL n` runs: rule group n (returning its "fired" result, always true, or always false) or a native routine (section 9) |

Module constants:

| | A | B | C |
|---|---|---|---|
| groups | 16 | 16 | 64 |
| rules | 176 | 351 | 1467 |
| bytecode | 3439 | 7591 | 29409 |
| classes / series / maps / variables | 34 / 1 / 0 / 7 | 45 / 2 / 2 / 6 | 86 / 8 / 8 / 29 |
| variable store cleared per group run | 8 bytes | 7 bytes | 30 bytes |
| error checking | before every rule attempt | after CALL | after CALL |

## 3. Bytecode

A rule is:

```
mode  focus… ff  left… ff  right… ff  action… ff
```

**Mode:**
- 1–16: a scan mode (section 5).
- 99: a directive, `63 in out ff ff ff ff`. It sets the default input and output tiers for the rest of this run of the group.
- 100: a no-op, `64 ff ff ff ff`. Every group ends with one.
- Values above 100 have 100 subtracted (no rule has one). 0 and 17–98 do nothing.

**Left context** is stored nearest-first, the reverse of reading order. Its tier switch and restore opcodes are exchanged, so reading nearest-first the switch still comes first.

**Prefilter modes 1–4** need a literal as the first focus byte (the scan compares it before trying the rule).

**Opcodes.** X is an operand byte, C a class, S a series, M a map, V a variable, T a tier number 1–5.

| Byte | Focus | Context | Test list | Action |
|---|---|---|---|---|
| any other byte | that literal | literal | (not used) | write the byte |
| `e9` | literal | literal | – | first action byte only: pad mode |
| `ea V` | any symbol except bound V | same | same | literal |
| `eb` | the rest of the text | literal | – | literal |
| `ec` | skip `~` cells | skip `~` cells | – | write the padding `~`s |
| `ed m n op X ed` | `op X` repeated m..n times, greedy | same | – | literal |
| `ee X`, `f0 X` | back to the saved tier | same | same | output tier = default |
| `ef T`, `f1 T` | switch to tier T | same | same | output tier = T |
| `f2 X` | any symbol except X and the edge | same | ≠ X | literal |
| `f3 X` | exactly X, not the edge | same | = X | literal |
| `f4 m n tests f4` | a test list repeated m..n times, greedy | same | – | literal |
| `f5 k … f5 00` | literal | optional group (k ≠ 0 opens) | – | literal |
| `f6 01 … f6 k … f6 00` | literal | choice: `01` opens, `k` starts alternative k, `00` closes | – | literal |
| `f7 tests f7` | one symbol passing every test | `f7 m n tests f7`: lazy repeat m..n | – | literal |
| `f8 S` / `f8 M` | a member of series S; remember its index | literal | – | write map M at the remembered index |
| `f9 S` / `fa S` | not in / in series S, not the edge | same | same, no edge check | literal |
| `fb C` / `fc C` | not in / in class C, not the edge | same | same, no edge check | literal |
| `fd V` | bind V or compare with it | same | same | write V |
| `fe n` | literal | literal | – | first action byte only: `01` ACCEPT, `02` STOP, else CALL procedure n |
| `ff` | end of section | end | – | end |

Notes:
- In `ed` and `f4` in the focus, a maximum of 0xdc means 2200 (unbounded). `f7` in a context doesn't convert it.
- An `f4` with a maximum of 0 would leave the interpreter inside its test list, so it would read the minimum as the next element. No rule has one, and the data pack rejects it.
- `op` in `ed` is one of `f2 f3 f9 fa fb fc`. Any other `op` matches zero symbols (one rule in B does this).
- `ee`/`ef` behave exactly like `f0`/`f1`.
- **Variables:** the first `fd V` in a rule attempt binds V to the current symbol, after checking its type. Later `fd V` compare. `ea V` fails only if V is bound and equal. Bindings last until the end of the attempt and are **not** undone by backtracking.

## 4. Running a group

`ApplyGroup(g, start, &end)`:

1. **Open.**
   - Move cells start..end up by one in all six arrays.
   - Write the edge 0x0e into T1–T5 at cells `start` and `end + 2`, with anchor counts 0.
   - `end += 1`.
   - The text is now cells `first = start + 1` to `end`.
2. **Initialise.**
   - Default and current input and output tiers = T1.
   - Clear the variables, the series index, "fired" and "keep largest".
   - Build the histogram: for each symbol 0x0e–0xaf, the number of T1 cells `first..end` holding it, then set the 0x0e count to 2.
3. **Rule loop,** for r = groupFirst[g] … groupLast[g]: run rule r by its mode (section 5), which may move r forward. B and C first check their error flag; if it is set, close and return false.
4. **Close.**
   - `end −= 1`.
   - Move cells start+1..end+1 down by one.
   - Return "fired".

`start` and `end` are the caller's. A CALL passes the focus span (section 7), and module entry points pass the sentence. Groups change `end`.

## 5. Scan modes

Definitions:
- **F:** the rule's first focus byte.
- **try(m, p, q):** try rule m with the focus starting at cell p and matching from cell q (section 6).
- **adv:** the advance set by the action (section 7).
- **Forward scans** re-read `end` at every step. **Backward scans** compute their starting cell once.
- **Requirements:** before modes 1–8, skip the rule unless the histogram counts for required1 and required2 are both non-zero. In block modes each member m is tried only if required1[m] is present.
- **Prefilter:** modes 1–4 start matching after the first focus byte, with `~` skipping on. All other modes start matching at the focus start with `~` skipping off.
- **Blocks:** modes 9–16 treat rules r … r + blockCount[r] as one block. Only the members' bytecode, required1 and outputLength are used.

| Mode | Cells | At each cell p | After |
|---|---|---|---|
| 1 | p = first+L … end−R, forward | if T[p] = F: adv = 0, try(r, p, p+1), p += adv+1; else p += 1 | – |
| 2 | p = end−R+1 … first+L, backward | if T[p] = F: try(r, p, p+1). p −= 1 | – |
| 3 | as 1 | if T[p] = F and the try succeeds: r += skipOnSuccess[r], stop | – |
| 4 | as 2 | the same, backward | – |
| 5 | as 1 | adv = 0, try(r, p, p), p += adv+1 | – |
| 6 | as 2 | try(r, p, p), p −= 1 | – |
| 7 | as 5 | first success: r += skipOnSuccess[r], stop | – |
| 8 | as 6 | the same, backward | – |
| 13 | forward, keep largest on | adv = −2; try members in order until one fires; p += (a member fired ? adv+1 : 1) | r += blockCount[r]; keep largest off |
| 14 | backward | try members in order until one fires; p −= 1 | r += blockCount[r] |
| 16 | backward; stops at the first cell where a member fires | try members in order until one fires | fired ? r += skipOnSuccess[r] : r += blockCount[r] |

T is the current input tier.

The rules use only modes 1–8, 13, 14 and 16:
- **11, 12, 15** are 16's siblings (11 forward, 12 backward: try every member at each cell; 15 forward version of 16).
- **9 and 10** are broken in the original (the block's rule counter is not reset between cells).

The data pack rejects modes 9–12 and 15, so the engine doesn't implement them.

## 6. Trying a rule

```
try(m, p, q):
    A only: if tailTop − end < 2: error = 1; return false
    B, C:   if error: return false
    matchEnd = MatchFocus(q)                          // 0: no match
    if matchEnd ≠ 0:
        focusLength = matchEnd − p
        left context:  direction −1, from cell p − 1
        right context: direction +1, from cell matchEnd
        if both match and the action succeeds: fired = 1, result = true
    if any variable was bound: clear the variables
    input tier = the default input tier
    return result
```

The input tier carries over from the focus into the left context and from there into the right context. Only the next rule starts again from the default.

**Skipping `~`:** "skip" means: if skipping is on, move past `~` cells in the current direction. Every consuming element turns skipping on (to the current direction).

### 6.1 Focus

Matched left to right, with no backtracking:

| Element | Match |
|---|---|
| literal | skip; the cell must equal it; next cell |
| `ea V` | skip; fail if V is bound and the cell equals it; next cell (no edge check) |
| `eb` | jump to end + 1 |
| `ec` | move past `~` cells (consumes nothing else) |
| `ed m n op X ed` | skip; then while fewer than n: stop at the edge or a cell failing `op X`; count it, next cell, skip `~`. Fail if fewer than m. Matching continues after the last counted cell |
| `f4 m n tests f4` | the same, with the test list as the test |
| `f7 tests f7` | skip; fail at the edge or if a test fails; next cell |
| `f2`, `f3`, `f9`–`fc` | skip; fail at the edge or on the test; next cell |
| `f8 S` | skip; must be in series S; remember its index; next cell |
| `fd V` | skip; bind (checking the type) or compare; next cell (no edge check) |
| `ee`/`f0` | back to the saved focus tier; skipping off |
| `ef`/`f1 T` | save the current tier as the focus tier; switch to T; skipping off |
| `ff` | done: the match ends here (an empty focus is allowed) |

### 6.2 Test lists

A test list applies every test to one symbol, ending at its `f4` or `f7`:
- `f2 f3 f9 fa fb fc ea fd` test as in the focus, without edge checks.
- `ee`/`f0` go back to the saved test tier, and `ef`/`f1 T` save the current tier and switch. Both re-read the symbol at the same cell.
- A switch changes the current input tier for the rest of the rule.

### 6.3 Contexts

Contexts are matched element by element in their direction, with a backtracking stack (up to 20 entries; the rules use at most 4).

```
skip on (direction), depth = 0, fresh = 1
RUN:  for each element until ff: if it fails goto BACK; cell += direction
      return true
BACK: if depth = 0: return false
      pop the top entry's saved state (cell, program position, tier, skipping); its count += 1
      f5: depth −= 1; continue RUN with the group's content
      f6: if alternative (count) exists: continue RUN at it; else depth −= 1, goto BACK
      f7: if count > maximum: depth −= 1, goto BACK; else fresh = 0, continue RUN (one more repeat)
```

| Element | Effect |
|---|---|
| literal (including `eb e9 f8 fe`) | skip; the cell must equal it |
| `ea fd f2 f3 f9`–`fc` | as in the focus, one cell |
| `ec` | move over `~` cells; end on the last one |
| `ed`, `f4` | greedy, as in the focus |
| `ee`/`f0`, `ef`/`f1` | as in the focus, using the saved context tier; consumes nothing |
| `f5 k` | push {f5, state}; skip to the matching `f5 00`. The group is first tried **without** its content |
| `f5 00` | nothing |
| `f6 01` | push {f6, count 1, state}; continue into alternative 1 |
| `f6 k` (k > 1) | end of an alternative: jump past the closing `f6 00` |
| `f7 m n tests f7` | push {f7, count 0}; match m cells; the repeat is **lazy**: it grows by one each time something later fails |

**Finding markers.** Jumps to `f5 00`, `f6 k` and `f6 00` use a byte-wise search from the current position:
- A nested group (`f6 01`, or `f5 k` with k ≠ 0) is skipped by a recursive search for its closing pair. The search resumes **three bytes** after that closing pair, so the byte right after a nested group is never examined.
- The engine must search the same way, not use a parsed tree.

**Test lists in left contexts** were stored with switch and restore exchanged but not reversed. So the restore runs before the switch, and the rule continues on the switched tier. Two rules do this (one in A, one in C). The saved test tier they restore to has never been set. Section 11 covers that case.

## 7. Actions

```
out = default output tier
first action byte:
  fe 01  ACCEPT: adv = focusLength − 1; true. Nothing changes
  fe 02  STOP: the scan ends at once and the group's rule loop ends (no more rules of this group run); true.
         Only forward scans have STOP rules; the data pack rejects one in a backward scan (quirk 7)
  fe n   CALL procedure n on the focus (below)
  else   rewrite (below)
keep largest ? adv = max(adv, previous) : adv
```

**Rewrite:**
```
newLength = outputLength[m]; pad = 0
if the first byte is e9: next byte; if focusLength > newLength: pad = focusLength − newLength, newLength = focusLength
adv = newLength − 1; delta = newLength − focusLength
histogram −= T1 cells p … p + focusLength − 1
if delta ≠ 0: shift the cells from p + focusLength to end + 1 by delta (all six arrays); end += delta
if delta > 0: the new cells read `~` in T1–T5
w = p; for each action byte until ff:
    ec: write `~` pad times     ee/f0: out = default     ef/f1 T: out = T
    f8 M: write map M at the remembered index     fd V: write V     other: write the byte
histogram += T1 cells p … w − 1
true
```

**Shifting:**
- **Deleting (delta < 0):** cells move down in ascending order. Each moved anchor count is **added** to the count already at its destination, and the source is set to 0. So the anchors of deleted cells merge into the cells that slide into their place.
- **Inserting (delta > 0):** cells move up in descending order. Anchors move with them, and new cells get 0.

**CALL n:**
```
histogram −= T1 cells of the focus
park the tail: cells matchEnd … end + 1 (with the edge) are copied, last first, to the top of the
               arrays (cells tailTop − 1 downward); tailTop starts at 0x501
callEnd = matchEnd − 1
result = procedure n (start = p, end = callEnd)            // may change callEnd
B, C: if callEnd ≥ tailTop: error = 1; fired = 0; return false (the tail is not restored)
add the anchors of cells callEnd + 1 … matchEnd − 1 into cell callEnd
histogram += T1 cells p … callEnd
adv = result ? callEnd − p : 0
restore the tail after callEnd, until a copied T1 cell is the edge; end = its new position
```

A group procedure runs `ApplyGroup(n, p, &callEnd)`, so the called group opens its own edges around the focus. Procedures that always return false run a group and still let the next block member try the same cell.

**Continuing a scan.** Forward scans continue at p + newLength, right after the output:
- a deletion looks at the same cell again;
- ACCEPT skips the whole focus (rules use this to protect `\x10…\x10` phonetic text).

Backward scans always step by one.

## 8. Errors and limits

- **Size.** The tiers hold 0x580 cells. The tail is parked from cell 0x500 downward, and the text plus its edge must stay below the tail.
- **Module A** checks before every rule attempt: if `tailTop − end < 2`, it sets its error flag and the attempt fails. The check runs again at every attempt, so a smaller group called from a larger one can fail while its caller goes on. The analysis then abandons the sentence (`prosody.md`).
- **Module A after a CALL** has no check: when a procedure leaves the text at or past the tail, the original goes on and writes past its tiers. The Go engine stops the analysis at once instead: no more rules run, the open groups stay open and the tail stays parked, and the sentence is dropped.
- **Modules B and C** check only after a CALL. On error, every running group aborts, and transcription drops the sentence (text.md §9).
- **Growth.** Nothing in the original checks growth from plain rewrites beyond these. A rewrite that would pass 0x582 cells overwrites the next tier in the original. The Go engine treats it as the module's error instead (module A: its overflow flag), and so it treats a group opened past the tiers, a tail parked below cell 0 and a tail restored without finding its edge.

## 9. Procedures

Procedures 1 and 2 don't exist (`fe 01` and `fe 02` are ACCEPT and STOP). Every other entry is a group or a native routine. Natives get the focus span and return true or false. The analyser checks at load that it has a native for every native entry of each module and that the groups it runs itself exist.

| Module | n | Native | Returns |
|---|---|---|---|
| A | 3 | T3: every cell of the span gets the span's last tag that is not `~` (`prosody.md` §3.6) | true |
| A | 4 | T5 = `~` over the span | true |
| A | 11 | applies `[…]` duration and pitch annotations, then deletes the span (`prosody.md`) | true |
| A | 12 | copies a T4 chunk code from a `"`, `'`, `` ` `` or `G` cell to the next non-`~` cell (`prosody.md` §3.6) | true |
| A | 13 | runs group 15 when the duration style is P (from the first `TextData` of the process on, text.md §1), else group 14 | true |
| B | 4 | inserts a phrase break (0x19) between a noun-phrase subject of three or more words and its verb (`prosody.md` §2.3) | true |
| B | 5 | inserts phrase breaks (0x19) before long prepositional phrases; joins a predicate noun phrase to a preceding "be" (`prosody.md` §2.3) | true |
| B | 7 | T1 = T4 over the span | true |
| B | 8 | applies the `\x11@x` part-of-speech markers (text.md §8.2) to T3 | true |
| C | 3 | the k-th `-` from the end of T1 gets the digit k in T4 (k ≤ 9) | true |
| C | 4 | T4 = space over the span | true |
| C | 23 | T1 = T2 over the span (restore the spelling) | true |
| C | 31 | T2 = T3 = T4 = T1 over the span | true |
| C | 41 | nothing | false |
| C | 42 | `stem(suffix)rest` → `stemrest or stemsuffixrest`, using T2 | true |
| C | 52 | T1 = T4 over the span | true |
| C | 30 | morphology: groups 57 and 56 until a dictionary hit, then group 58, or 59 when nothing was found (`pronunciation.md`) | found |
| C | 21, 22, 25, 26, 27, 34, 35, 36, 53, 54 | dictionary lookups: function words, main dictionary, prefixes, suffixes, abbreviations, uppercase abbreviations, capitalised names, phrases, homographs, verbs. They write the entry's fields into T1, T3 and T4 and pad with `~` (`pronunciation.md` §3) | found |

The rows marked with another spec are specified there, where their effects can be checked.

## 10. Module entry points

- **C (transcription):** clear the error flag; tail top 0x501. Over cells start..end: T2 = T1, and T3, T4, T5 = spaces. Then `ApplyGroup(50, start, &end)`.
- **B (transcription):** clear the error flag; tail top 0x501. Then groups 16, 6 and 3 in turn, each with `(start, &end)`.
- **A (analysis):** clear the error flag. Group 5's result is the "more than one word" flag. Then groups 10 and 16, with a non-rule step between them (`prosody.md`).

The error flags, saved tiers and backtracking stack belong to the module, not to one group run. Groups called through CALL share them.

## 11. Quirks

Reproduce these:
1. The input tier carries over between a rule's focus and contexts (section 6).
2. Variables are not unbound on backtracking.
3. The byte-wise marker search (6.3). One rule in C has an alternative that can never be tried because of it.
4. Anchor counts merge on deletion and after CALL (section 7). After CALL, the merge can also pick up a count left in the cell beyond the old end.
5. The histogram counts T1 only, whichever tier a rule reads. The compiler gave requirements only to rules that read T1.
6. `ed` with an unknown test matches zero symbols.
7. STOP in a backward scan would restart the scan at cell 0x500 and run the same rule down through the cells past the text. No rule does it, and the data pack rejects it.

Don't reproduce:
- **Saved tiers start as null** in the original, and the two left-context test lists of section 6.3 would read through a null pointer if they ran before any switch in their module. The Go engine starts every saved tier at T1, their usual value. Whether the original ever reaches that read is open question 1.
- **Tier overflow** and module A after a CALL (section 8).

**Requirements.** Every `required1` and `required2` of the shipped rules is a symbol the rule's focus or context reads on T1 outside any group, except module C rule 13's `#`, which only its dead alternative has. Skipping a rule whose required symbol is missing therefore never changes the result, except that rule 13 runs only when the text has a `#`.

## 12. For the data pack

For each module the converter writes the tables of section 2, unchanged, into `rules.json` (datapack.md §9). Procedure entries become a group with its result kind, or a native. The engine never parses the DLL.

## 13. Open questions

1. Does the original ever reach the null saved-tier read? The case is a first sentence that ends in "… the one?", spoken right after the engine loads. "Is this the one?", spoken first by a fresh engine on all five voices, doesn't crash, and its audio matches this spec, which starts the saved tiers at T1. The audio alone doesn't show whether the read happens.
