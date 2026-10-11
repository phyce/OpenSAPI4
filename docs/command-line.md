# The command-line tools

A release has three programs and the voice data:

| | |
|---|---|
| `opensapi4` | Speaks into WAV files, as a stream over standard input and output, or as an HTTP server |
| `sapi4out`, `sapi4limits` | Drop-in replacements for the tools of [TETYYS/SAPI4](https://github.com/TETYYS/SAPI4) |
| `voices/` | The voice data. Keep it next to the programs |

The programs look for the voice data in the folder named by the `OPENSAPI4_VOICES` environment variable, or else in the `voices` folder next to the program. `opensapi4` also takes `-voices <folder>`.

## opensapi4 speak

```sh
opensapi4 speak -voice Sam -output hello.wav "Hello, I am Microsoft Sam."
opensapi4 speak -voice Mary -pitch 200 -speed 120 -output slow.wav < story.txt
opensapi4 speak -json '{"voice": "Mike", "text": "Hello.", "speed": 250}' > hello.wav
```

The text comes from the arguments, or from standard input when there are none. The audio goes to the `-output` file, or to standard output when it isn't a terminal.

| Flag | |
|---|---|
| `-voice` | The voice, `Mary` by default. `opensapi4 voices` lists them. The name may be in any case |
| `-pitch` | The pitch in Hz, in the voice's range. 0 is the lowest |
| `-speed` | The speed in words per minute, in the voice's range. 0 is the slowest |
| `-volume` | 0 to 65535, the default |
| `-rate`, `-bits` | Another format: 22050 Hz voices also give 8000 and 16000 Hz, 8000 Hz voices 11025 Hz, each in 16 or 8 bits |
| `-untagged` | Read `\` as text, not as the start of a tag |
| `-ipa` | The text is IPA, such as `həˈloʊ` |
| `-inject` | Tags applied before the text: `\Pit=n\`, `\Spd=n\`, `\Vol=n\`, `\Rst\` |
| `-output` | The WAV file to write; `-` for standard output |
| `-pcm` | Write raw PCM, without the WAV header |
| `-json` | The request as JSON (see [Requests](#requests)): inline, a file, or `-` for standard input. Flags given with it override its fields |

Flags may come before or after the text. After `--`, everything is text.

The texts may have tags, as in SAPI 4: `\Pau=500\` pauses for half a second, `\Pit=150\` and `\Spd=200\` change the pitch and speed, `\Emp\` stresses the next word, `\Chr="Whisper"\` whispers, `\Mrk=n\` sets a bookmark, `\Rst\` resets everything.

## opensapi4 voices

```sh
opensapi4 voices
opensapi4 voices -json
```

Lists the voices with their sample rate, pitch and speed (default, minimum and maximum). With `-json`, every detail, as `GET /voices` gives it.

## Requests

A request is a JSON object. Only the text is needed.

```json
{
  "voice": "Sam",
  "text": "Hello, \\Pau=300\\ world.",
  "pitch": 120,
  "speed": 180,
  "volume": 65535,
  "rate": 22050,
  "bits": 16,
  "tagged": true,
  "ipa": false,
  "inject": "\\Spd=200\\"
}
```

| Field | |
|---|---|
| `voice` | The voice, `Mary` by default |
| `text` | The text |
| `texts` | More texts, spoken after `text` by the same voice into the same audio, so a tag's change lasts into the next one |
| `pitch`, `speed`, `volume`, `rate`, `bits`, `ipa`, `inject` | As the flags of `opensapi4 speak` |
| `tagged` | Read tags, `true` by default |

In a stream and over HTTP, a request may also have:

| Field | |
|---|---|
| `id` | Any JSON value, sent back in every message of the request |
| `events` | Send the events: text start and end, words and bookmarks |
| `visual` | Send the mouth shapes, for lip sync |
| `output` | Stream only: write the audio to this WAV file instead of sending it |
| `format` | HTTP only: `wav`, `pcm` or `json` |

## opensapi4 stream

```sh
opensapi4 stream [-raw]
```

Reads requests from standard input, one JSON object a line, and speaks them in order. Each request gives messages on standard output, one JSON object a line. For example, `{"id": 1, "voice": "Mary", "text": "Hello \\Mrk=3\\ world", "events": true, "visual": true}` gives (shortened):

```json
{"id":1,"type":"format","sampleRate":22050,"bits":16,"channels":1}
{"id":1,"type":"event","event":"textStarted","sample":0,"text":0}
{"id":1,"type":"audio","data":"AAAAAAAAAAAA..."}
{"id":1,"type":"event","event":"word","sample":330,"text":0,"offset":0}
{"id":1,"type":"event","event":"visual","sample":330,"text":0,"ipa":"ɦ","phoneme":72,"mouth":{"height":48,"width":208,"upturn":160,"jawOpen":128,"upperTeethVisible":48,"lowerTeethVisible":112,"tonguePosition":0,"lipTension":32}}
{"id":1,"type":"audio","data":"AAAAAAEA/v/4//j/..."}
...
{"id":1,"type":"event","event":"bookmark","sample":7979,"text":0,"bookmark":3}
{"id":1,"type":"event","event":"word","sample":7979,"text":0,"offset":14}
...
{"id":1,"type":"event","event":"textDone","sample":26513,"text":0}
{"id":1,"type":"done","samples":26513}
```

- **`format`** comes first: the audio is mono PCM, 16-bit signed little-endian or 8-bit unsigned.
- **`audio`** has the next piece of the audio, base64. With `-raw`, it has `"bytes": n` instead, and the n bytes follow right after its line.
- **`event`** is where the event stands: `sample` counts the samples before it, and `text` is the text's number in the request (0 for `text`, then 1, 2… for `texts`). Events come only when the request asks for them:
  - `textStarted`, `textDone`;
  - `word`: `offset` is where the word starts in the text, in UTF-16 code units (JavaScript string indexes);
  - `bookmark`: `bookmark` is the `\Mrk=n\` number;
  - `visual`: the phone starting there, as `ipa` and the engine's `phoneme`, and the `mouth` shape, each value 0 to 255.
- **`done`** ends the request, with the number of samples. With `output`, it also has the file's name.
- **`error`** ends a request that can't be spoken, with `error`, the reason. The stream goes on with the next request.

## opensapi4 serve

```sh
opensapi4 serve [-address 127.0.0.1:5040] [-origin <origin>]
```

Serves on `http://127.0.0.1:5040` by default; `-address :5040` serves on every network interface. `-origin` lets web pages from another origin call the server (`*` for any).

| Request | |
|---|---|
| `GET /` | A page to try the voices |
| `GET /voices` | The voices, as JSON |
| `POST /speak` | Speaks a request, given as JSON in the body |
| `GET /speak?voice=Sam&text=Hello` | The same, with the request's fields as parameters. `text` may be given more than once |

`format` (a field, or a parameter) chooses the answer:
- `wav` (the default): a WAV file;
- `pcm`: the raw PCM, with its format in the `X-Sample-Rate`, `X-Bits` and `X-Channels` headers;
- `json`: the stream's messages, one a line (`application/x-ndjson`), sent as they are made. Only this format has events.

A request that can't be spoken gets status 400 and `{"error": "..."}`. Requests may have up to 64 KiB of text.

```sh
curl -o hello.wav "http://127.0.0.1:5040/speak?voice=Sam&text=Hello%20world"
curl -o hello.wav -H "Content-Type: application/json" -d '{"voice": "Sam", "text": "Hello world"}' http://127.0.0.1:5040/speak
```

## sapi4out and sapi4limits

```sh
sapi4out <voice> <pitch> <speed> <text>
sapi4limits [<voice>]
```

The same arguments, output and WAV files as the tools of [TETYYS/SAPI4](https://github.com/TETYYS/SAPI4), so programs that call those can call these instead:

- `sapi4out` writes a WAV file in the current folder, named with 16 random letters and no extension, and prints its name. A voice name that isn't exactly one of the voices gives Mary. Pitch and speed outside the voice's range give its maximum.
- `sapi4limits` lists the voices, or with a voice, prints its default, minimum and maximum pitch and speed.

Where the old tools failed silently (no voice data) or crashed (missing arguments), these print the reason on standard error and exit with code 1 or 2.
