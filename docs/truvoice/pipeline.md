# TruVoice: pipeline

How L&H TruVoice (`tv_enua.dll`) passes a text through its stages. The speaking thread keeps everything in one state: a pool of nodes, linked into one list that every stage works on, and a cursor for each stage, its window on the list. Each stage rewrites the nodes in its window and hands the finished ones to the next stage's cursor. The text's commands (pitch, speed, voice, bookmarks, words…) travel as nodes too, and each stage runs them when it reaches them.

This spec covers the nodes (section 2), the cursors (section 3) and the control commands (section 4). The output stage, the scheduler, the resets and the attributes are still to come.

**In short.**
- There are 618 nodes. A free node is taken from the front of the free list and goes back to its end, so nodes are reused in a fixed order. Taking a node clears only a few of its bytes: the rest is what its last use left.
- The stages with cursors are, in order: the normalizer, the lexical stage, prosody, the parameter generator and the output stage. The input stage before them appends its nodes at the end of the list.
- Each cursor has its own copy of the settings the commands change (rate, pitch, attenuation, voice…). A command therefore takes effect at each stage when that stage reaches it.
- Most commands only change the settings. A few act at one stage only: the parameter generator queues the bookmark and word events and changes the volume and the voice, prosody changes the pitch, the speed and the character, and the normalizer adds nodes.

**Exactness.** Ported from the original's code, and checked against it by replaying every call of its node, cursor and control functions in 100 recorded runs on the Go pipeline (the ten voices, both rates, tags for every attribute, voice changes, bookmarks, tones, pauses, raw commands, texts spoken one after another in one session). Each call returned and left the same as the original's: 17,468 nodes taken and 17,369 freed, 56,976 beginnings (section 3.2) and 55,659 finishes (section 3.3), 10,050 commands of 20 letters with their 1,277 events, and the free nodes after each of 125,346 stage steps.

## 1. The state

What the stages share:

- the nodes: a pool, the list of nodes in use and the free list (section 2);
- five cursors (section 3);
- the **settings**: the escape parser's current settings, which a reset gives to every cursor (section 3.1);
- the **attributes** the speaking thread last applied: pitch in Hz, speed in words a minute, volume, character, and the voice, which is also the synthesizer's output-filter voice. Some commands change them too (section 4);
- the **saved rate**, which the filter command keeps, and the **track levels**, 22 bytes;
- a **reset request** and its value;
- the **engine**: its attributes (pitch, speed, volume, character, voice), which its interface sets and reads, a **queue** that carries the audio and the events to the destination, in order, and the **messages** the pipeline posts to the engine's window. The engine is absent when it only converts text to phonemes.

## 2. Nodes

### 2.1 A node

A node is 32 bytes: a pointer to the next node, one to the previous node, and then:

| Offset | Size | Content |
|---|---|---|
| 0x08 | 4 | Flags. Bits 0–2 are the type; the other bits belong to the stages |
| 0x0c | 4 | Value |
| 0x10 | 4 | For the stages |
| 0x14 | 1 | For the stages |
| 0x15 | 1 | Second value |
| 0x16 | 1 | Symbol: a command's letter, a character, a phoneme |
| 0x17–0x1b | 5 | For the stages |
| 0x1c | 4 | The text, on bookmark and word commands |

The types:

| Type | Node |
|---|---|
| 0 | A control command (section 4) |
| 1 | A character |
| 2 | A letter |
| 3 | A phoneme |
| 4 | A segment: a phoneme or a pause with its duration |
| 5 | A generated segment: a tone or a fixed sound |
| 6 | Free |
| 7 | A sentinel |

### 2.2 The pool and the lists

The pool holds 618 nodes. Two lists run through it: the list in use and the free list, each with a sentinel node at both ends. The free list is a queue: nodes are taken from its front and freed to its back.

**Reset.**
1. Link every node into the free list, in pool order, and make it free: clear flag bit 0 and set bits 1 and 2 (type 6). The node's other bytes keep what they had.
2. Set the type bits of the four sentinels (type 7). Empty the list in use.
3. The free count is 618. No cursor is current.
4. Empty every cursor: its start, done, scan, first and end, and its two working pointers. Give it the settings.

A new speaking thread's nodes are all zero bytes. Within a session, a reset leaves the nodes' bytes as they were.

### 2.3 Taking and freeing nodes

**Taking a node** next to a node `at`, before it or after it, with a type and a symbol:
1. Take the free list's first node. Subtract 1 from the free count.
2. Link it before `at`, or before `at`'s next node when after.
3. Set its symbol. Clear its value, its second value and its bytes 0x14 and 0x19. Set flag bits 0–7 to the type: bits 3–7 are cleared, bits 8–31 keep what they had. Every other byte keeps what the node's last use left, and some stages read them.
4. If a cursor is current: when `at` is its end and the node goes after it, the node becomes its end. Otherwise, when `at` is its start and the node goes before it, the node becomes its start.

Nothing checks that a node is free: the scheduler only runs a stage when enough are.

**Freeing a node**, going forward or back, gives a neighbour: the node's next node going forward, its previous node going back.
1. If a cursor is current:
   - when the node is its end: if the end is also its start, the cursor's start and end become none; otherwise its end becomes the end's previous node. Going forward, the neighbour is none;
   - then, when the node is its start: its start becomes the node's next node. Going back, the neighbour is none.
2. Unlink the node and link it at the end of the free list. Add 1 to the free count.
3. Clear flag bit 0 and set bits 1 and 2 (type 6). The other bits and bytes keep what they had.

**Within a window.** The next node of a node is none when the node is the current cursor's end. The previous node is none when it is the current cursor's start.

## 3. Cursors

Each stage with a cursor works on a window of the list: the nodes the previous stage handed over.

| Field | Meaning |
|---|---|
| start | The window's first node |
| done | The first node the stage hasn't finished. None when the stage has finished the whole window |
| scan | The next node whose command the stage hasn't run |
| first | The first node of the stage's types |
| end | The window's last node |
| working | Two node pointers that belong to the stage: the normalizer and the lexical stage use them |
| settings | Section 3.1 |
| types | The node types the stage works on |

| Stage | Types |
|---|---|
| Normalizer | 0, 1, 2, 3 |
| Lexical | 2, 3 |
| Prosody | 0, 3, 4 |
| Parameter generator | 4, 5 |
| Output | 0–5 |

A stage makes its cursor **current** while it works. The input stage appends its nodes at the end of the list and adds them to the normalizer's window; its spec will say how.

### 3.1 Settings

| Setting | Command | Default |
|---|---|---|
| Phonetic input | `I` | 0 |
| Flush | `P` | 1 |
| Rate index | `r`, `v` | 13 |
| Pitch, Hz | `p` | 85 |
| Attenuation, dB | `a` | 0 |
| Filter | `f` | 0 |
| Flags | `N`, `F` | 0x17c0 |
| Input flags | `A`, `D` | 0x41 |
| Voice | `V` | 0 |

The escape parser keeps the current settings as it reads the text. Every cursor has its own copy, which the commands change as the stage reaches them (section 4). While flush is 0, the parameter generator leaves the end of speech unmarked.

### 3.2 Beginning

A stage begins work on its cursor:
1. Make the cursor current.
2. Move `first` on to the first node, from `first` to the end of the window, whose type is one of the stage's types. None if there is none.
3. While `scan` is a node and its type isn't one of the stage's types, run its command (section 4). If the command says to stop, stop. Otherwise, if `done` is `scan`, move both on to the next node; else move only `scan` on.
4. Report whether `first` is a node.

So a stage runs the commands up to its first node, and the commands between its nodes as it finishes them and begins again.

### 3.3 Finishing

A stage finishes with the current cursor and hands its finished nodes, from `start` up to `done`, to the next stage:
1. If `start` is none or `start` is `done`, there is nothing to hand over. Go to 5.
2. At the output stage, free the nodes from `start` until `start` is `done`, going forward. Section 2.3 moves `start` on each time.
3. At the other stages, give the nodes to the next stage's cursor:
   - its end becomes the node before `done`, or this cursor's end when `done` is none;
   - each of its `first`, `scan`, `done` and `start` that is none becomes this cursor's `start`.
4. If `done` is none, empty this cursor (its start, scan, first and end become none). Otherwise its `start` becomes `done`.
5. No cursor is current. Report whether anything was handed over.

## 4. Control commands

A control node (type 0) holds a command from the text: its letter is the symbol, its value is the value's low byte (all four bytes for `i`, `n` and `z`), and its second value is byte 0x15. The escape parser makes them (its spec will say how).

Running the current cursor's `scan` node:
1. At the prosody stage, a generated segment says to stop.
2. A node that isn't a control node does nothing.
3. A control node does what the table says, and says to go on.

| Letter | Every stage | Only at the stage named |
|---|---|---|
| `A` | input flags = second · 256 + value | |
| `C` | | Normalizer: take a node of type 3, symbol `]`, before it |
| `I` | phonetic input = value | |
| `N` | flags = second · 256 + value | |
| `P` | flush = second · 256 + value | |
| `V` | voice = value; pitch and rate index = the voice's own | Parameter generator: the attribute voice (and so the output-filter voice) = value; the attribute speed = the voice's own; the attribute pitch = the cursor's pitch. With an engine, the engine's voice (and so its mode information), pitch and speed become the same |
| `a` | attenuation = value | Parameter generator: value 16 makes the attribute volume 0 and the cursor's attenuation 100; any other value makes the attribute volume `volumes[value]` (section 4.2) and clears the mute flag. The engine's volume becomes the same, and a volume message is posted |
| `c` | | Prosody, with an engine: the attribute character and the engine's = value |
| `f` | if the filter was 0, the saved rate = the rate index; filter = value; rate index = second; if the filter is now 0, rate index = the saved rate | |
| `g`, `s`, `t` | | Prosody: the node becomes a generated segment (flag bit 1 cleared, bits 0 and 2 set) |
| `i` | | Parameter generator: a bookmark (section 4.1) |
| `l` | track level `value` = second | |
| `p` | pitch = value · 2 | Prosody: the attribute pitch = the cursor's pitch. With an engine, the engine's pitch becomes the same and a pitch message is posted |
| `r`, `v` | rate index = value | Prosody: the attribute speed = `trunc((value · 8 + 50) / 10) · 10`. With an engine, the engine's speed becomes the same and a speed message is posted |
| `x` | | Output: request a reset with value 0. Normalizer: take a node of type 0, symbol `C`, after it |
| `z` | | Parameter generator: a word (section 4.1) |

Other letters do nothing. The command `n`, a pronunciation the text adds to the user lexicon, is specified with the user lexicon.

The voices' own pitch, rate index and speed:

| Voice | 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 |
|---|---|---|---|---|---|---|---|---|---|---|
| Pitch (Hz) | 85 | 50 | 125 | 73 | 129 | 89 | 117 | 203 | 208 | 152 |
| Rate index | 13 | 13 | 13 | 13 | 13 | 9 | 13 | 13 | 13 | 13 |
| Speed (words a minute) | 150 | 150 | 150 | 150 | 150 | 120 | 150 | 150 | 150 | 150 |

### 4.1 Bookmarks and words

With an engine, a **bookmark** command whose node holds a text:
1. If its value is 0, the text's end: post a message that the text is done.
2. Queue a bookmark event with the text and the value.
3. If its value is 0, the node no longer holds the text.

With an engine, a **word** command whose node holds a text queues a word event with the text and the value: the word's offset in the text.

Then, with an engine or without, if the node's flag bit 5 is set, request a reset with the command's value.

Events go into the engine's queue, behind the audio queued so far: the samples the synthesizer has rendered and not yet handed over come after them. Messages go to the engine's window, which handles them in its own time.

### 4.2 Volumes

After an attenuation command of `a` dB, the engine reports the volume `trunc(65535 · 10^(−a / 10))`:

| a | 0 | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 10 | 11 | 12 | 13 | 14 | 15 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| Volume | 65535 | 52056 | 41349 | 32845 | 26089 | 20723 | 16461 | 13075 | 10386 | 8250 | 6553 | 5205 | 4134 | 3284 | 2608 | 2072 |

The original computes it in x87 floating point. The value closest to an integer is 0.0047 away from it, so a 64-bit floating-point computation gives the same table.

## 5. What the recorded runs don't reach

- Freeing a cursor's start going back (section 2.3 step 1).
- The reset requests: `x` at the output stage, bookmarks and words with flag bit 5. The escape parser's own reset command, and `x`, make the original drop the rest of the text without reporting it done, so a recorded run never ends.
- The command `n` (section 4).
