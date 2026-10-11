package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	opensapi4 "github.com/phyce/OpenSAPI4"
	"github.com/phyce/OpenSAPI4/internal/wave"
	"github.com/phyce/OpenSAPI4/whistler"
)

func TestParseFlags(t *testing.T) {
	for _, test := range []struct {
		arguments []string
		voice     string
		others    []string
	}{
		{[]string{"-voice", "Sam", "Hello", "there"}, "Sam", []string{"Hello", "there"}},
		{[]string{"Hello", "-voice", "Sam", "there"}, "Sam", []string{"Hello", "there"}},
		{[]string{"Hello", "there", "-voice=Sam"}, "Sam", []string{"Hello", "there"}},
		{[]string{"-voice", "Sam", "--", "-5", "-voice"}, "Sam", []string{"-5", "-voice"}},
		{[]string{"Hello", "--", "-voice", "Mike"}, "", []string{"Hello", "-voice", "Mike"}},
		{nil, "", nil},
	} {
		flags := flag.NewFlagSet("test", flag.ContinueOnError)
		voice := flags.String("voice", "", "")
		others, _, proceed := parseFlags(flags, test.arguments)
		if !proceed || *voice != test.voice || !slices.Equal(others, test.others) {
			t.Errorf("%q: voice %q, others %q, expected %q, %q", test.arguments, *voice, others, test.voice, test.others)
		}
	}
}

func TestQueryRequest(t *testing.T) {
	query, _ := url.ParseQuery(`voice=Sam&text=One.&text=Two.&pitch=120&speed=0&volume=100&rate=8000&bits=8&tagged=false&ipa=1&inject=%5CSpd%3D200%5C&format=json&events=true`)
	parsed, err := queryRequest(query)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(parsed)
	expected := `{"voice":"Sam","text":"One.","texts":["Two."],"pitch":120,"speed":0,"volume":100,"rate":8000,"bits":8,"tagged":false,"ipa":true,` +
		`"inject":"\\Spd=200\\","events":true,"format":"json"}`
	if string(got) != expected {
		t.Errorf("got %s\nexpected %s", got, expected)
	}
	for _, bad := range []string{"pitch=high", "tagged=maybe", "rate=x"} {
		query, _ := url.ParseQuery(bad)
		if _, err := queryRequest(query); err == nil {
			t.Errorf("%s: no error", bad)
		}
	}
}

func TestDecodeRequest(t *testing.T) {
	decoded, err := decodeRequest([]byte("\xEF\xBB\xBF{\"id\": 7, \"voice\": \"Sam\", \"text\": \"Hi\", \"visual\": true}\n"))
	if err != nil || string(decoded.ID) != "7" || decoded.Voice != "Sam" || decoded.Text != "Hi" || !decoded.Visual {
		t.Errorf("%+v, %v", decoded, err)
	}
	for _, bad := range []string{`{"voice": "Sam", "txt": "Hi"}`, `{"text": "Hi"} {"text": "Again"}`, `{"pitch": "high"}`, `[]`} {
		if _, err := decodeRequest([]byte(bad)); err == nil {
			t.Errorf("%s: no error", bad)
		}
	}
}

// fakeSpeech feeds a messages writer as an engine would.
func fakeSpeech(writer *messages) error {
	if err := writer.sendFormat(whistler.Format{SampleRate: 8000, Bits: 16}); err != nil {
		return err
	}
	writer.Event(whistler.Event{Kind: whistler.TextStarted})
	writer.Audio([]byte{1, 2, 3, 4})
	writer.Event(whistler.Event{Kind: whistler.WordPosition, Sample: 2, Value: 12})
	writer.Event(whistler.Event{Kind: whistler.Visual, Sample: 2})
	writer.Audio([]byte{5, 6})
	writer.Event(whistler.Event{Kind: whistler.TextDone, Sample: 3})
	return writer.finish(nil, "")
}

func TestMessages(t *testing.T) {
	var output bytes.Buffer
	writer := &messages{output: bufio.NewWriter(&output), id: json.RawMessage(`"a"`), events: true}
	if err := fakeSpeech(writer); err != nil {
		t.Fatal(err)
	}
	expected := `{"id":"a","type":"format","sampleRate":8000,"bits":16,"channels":1}
{"id":"a","type":"event","event":"textStarted","sample":0,"text":0}
{"id":"a","type":"audio","data":"AQIDBA=="}
{"id":"a","type":"event","event":"word","sample":2,"text":0,"offset":6}
{"id":"a","type":"audio","data":"BQY="}
{"id":"a","type":"event","event":"textDone","sample":3,"text":0}
{"id":"a","type":"done","samples":3}
`
	if output.String() != expected {
		t.Errorf("got\n%s\nexpected\n%s", output.String(), expected)
	}

	output.Reset()
	writer = &messages{output: bufio.NewWriter(&output), raw: true}
	if err := fakeSpeech(writer); err != nil {
		t.Fatal(err)
	}
	expected = "{\"type\":\"format\",\"sampleRate\":8000,\"bits\":16,\"channels\":1}\n{\"type\":\"audio\",\"bytes\":6}\n\x01\x02\x03\x04\x05\x06" +
		"{\"type\":\"done\",\"samples\":3}\n"
	if output.String() != expected {
		t.Errorf("got %q\nexpected %q", output.String(), expected)
	}
}

func openTestVoices(t *testing.T) *opensapi4.Voices {
	t.Helper()
	root := os.Getenv("OPENSAPI4_CONFORMANCE")
	if root == "" {
		t.Skip("OPENSAPI4_CONFORMANCE is not set")
	}
	voices, err := opensapi4.Open(filepath.Join(root, "datapack"))
	if err != nil {
		t.Fatal(err)
	}
	return voices
}

// expectedAudio is what the voices give for a request, as PCM.
func expectedAudio(t *testing.T, voices *opensapi4.Voices, request opensapi4.Request) ([]byte, whistler.Format) {
	t.Helper()
	speech, err := voices.NewSpeech(request)
	if err != nil {
		t.Fatal(err)
	}
	buffer := &bufferSink{}
	if err := speech.Speak(buffer); err != nil {
		t.Fatal(err)
	}
	return buffer.data, speech.Format()
}

// readMessages reads a stream's messages, with the audio of the raw ones.
func readMessages(t *testing.T, reader *bufio.Reader) ([]map[string]any, []byte) {
	t.Helper()
	var read []map[string]any
	var audio []byte
	for {
		line, err := reader.ReadBytes('\n')
		if err == io.EOF && len(line) == 0 {
			return read, audio
		}
		if err != nil {
			t.Fatal(err)
		}
		var message map[string]any
		if err := json.Unmarshal(line, &message); err != nil {
			t.Fatalf("%q: %v", line, err)
		}
		read = append(read, message)
		if message["type"] != "audio" {
			continue
		}
		if data, found := message["data"]; found {
			decoded, err := base64.StdEncoding.DecodeString(data.(string))
			if err != nil {
				t.Fatal(err)
			}
			audio = append(audio, decoded...)
			continue
		}
		raw := make([]byte, int(message["bytes"].(float64)))
		if _, err := io.ReadFull(reader, raw); err != nil {
			t.Fatal(err)
		}
		audio = append(audio, raw...)
	}
}

func TestStream(t *testing.T) {
	voices := openTestVoices(t)
	request := opensapi4.Request{Voice: "Sam", Text: `Hello \Mrk=5\ there, this is a longer sentence so that the audio comes in several messages.`}
	want, _ := expectedAudio(t, voices, request)
	path := filepath.Join(t.TempDir(), "out.wav")
	text, _ := json.Marshal(request.Text)
	quotedPath, _ := json.Marshal(path)
	lines := fmt.Sprintf(`{"id": 1, "voice": "Sam", "text": %s, "events": true}
{"id": 2, "voice": "Nobody", "text": "Hi"}
not json

{"id": "file", "voice": "Sam", "text": %s, "output": %s}
`, text, text, quotedPath)
	for _, raw := range []bool{false, true} {
		var output bytes.Buffer
		buffered := bufio.NewWriter(&output)
		for _, line := range strings.SplitAfter(lines, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			if err := streamRequest(voices, []byte(line), buffered, raw); err != nil {
				t.Fatal(err)
			}
		}
		read, audio := readMessages(t, bufio.NewReader(&output))
		if !bytes.Equal(audio, want) {
			t.Errorf("raw %v: %d bytes of audio, expected %d", raw, len(audio), len(want))
		}
		var types []string
		audioMessages := 0
		for _, message := range read {
			if message["type"] == "audio" {
				audioMessages++
				continue
			}
			kind := message["type"].(string)
			if event, found := message["event"]; found {
				kind += ":" + event.(string)
			}
			types = append(types, kind)
		}
		expected := []string{"format", "event:textStarted", "event:word", "event:bookmark", "event:word"}
		for range 13 {
			expected = append(expected, "event:word")
		}
		expected = append(expected, "event:textDone", "done", "error", "error", "format", "done")
		if !slices.Equal(types, expected) || audioMessages < 2 {
			t.Errorf("raw %v: %d audio messages and %v", raw, audioMessages, types)
		}
		if read[len(read)-1]["output"] != path || read[len(read)-3]["id"] != nil || read[len(read)-4]["id"] != float64(2) {
			t.Errorf("raw %v: the last messages %v", raw, read[len(read)-4:])
		}
		file, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(file, append(wave.Header(whistler.Format{SampleRate: 22050, Bits: 16}, uint32(len(want))), want...)) {
			t.Errorf("raw %v: the WAV file has %d bytes", raw, len(file))
		}
	}
}

func TestServer(t *testing.T) {
	voices := openTestVoices(t)
	server := httptest.NewServer(newHandler(voices, "*"))
	defer server.Close()
	request := opensapi4.Request{Voice: "Mike", Text: "Testing, one, two, three."}
	want, format := expectedAudio(t, voices, request)
	wavFile := append(wave.Header(format, uint32(len(want))), want...)

	check := func(name string, response *http.Response, err error, status int, contentType string) []byte {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if response.StatusCode != status || response.Header.Get("Content-Type") != contentType || response.Header.Get("Access-Control-Allow-Origin") != "*" {
			t.Errorf("%s: %d %s %q", name, response.StatusCode, response.Header.Get("Content-Type"), body)
		}
		return body
	}

	response, err := http.Get(server.URL + "/speak?voice=Mike&text=" + url.QueryEscape("Testing, one, two, three."))
	if body := check("GET wav", response, err, 200, "audio/wav"); !bytes.Equal(body, wavFile) {
		t.Errorf("GET wav: %d bytes, expected %d", len(body), len(wavFile))
	}
	response, err = http.Post(server.URL+"/speak", "application/json", strings.NewReader(`{"voice": "Mike", "text": "Testing, one, two, three."}`))
	if body := check("POST wav", response, err, 200, "audio/wav"); !bytes.Equal(body, wavFile) {
		t.Errorf("POST wav: %d bytes, expected %d", len(body), len(wavFile))
	}
	response, err = http.Post(server.URL+"/speak?format=pcm", "application/json", strings.NewReader(`{"voice": "Mike", "text": "Testing, one, two, three."}`))
	if body := check("POST pcm", response, err, 200, "application/octet-stream"); !bytes.Equal(body, want) || response.Header.Get("X-Sample-Rate") != "22050" {
		t.Errorf("POST pcm: %d bytes, expected %d", len(body), len(want))
	}
	response, err = http.Post(server.URL+"/speak", "application/json",
		strings.NewReader(`{"id": "x", "voice": "Mike", "text": "Testing, one, two, three.", "format": "json", "visual": true}`))
	body := check("POST json", response, err, 200, "application/x-ndjson")
	read, audio := readMessages(t, bufio.NewReader(bytes.NewReader(body)))
	visual := 0
	for _, message := range read {
		if message["event"] == "visual" {
			visual++
		}
	}
	if !bytes.Equal(audio, want) || visual < 10 || read[len(read)-1]["type"] != "done" || read[0]["id"] != "x" {
		t.Errorf("POST json: %d bytes of audio, %d visual events, last %v", len(audio), visual, read[len(read)-1])
	}

	for _, bad := range []string{`{"voice": "Nobody", "text": "Hi"}`, `{"voice": "Sam", "text": "Hi", "output": "x.wav"}`, `{"text": "Hi", "format": "mp3"}`,
		`{"text": "Hi", "events": true}`, `{"text": 5}`} {
		response, err := http.Post(server.URL+"/speak", "application/json", strings.NewReader(bad))
		body := check(bad, response, err, 400, "application/json")
		var message map[string]string
		if json.Unmarshal(body, &message) != nil || message["error"] == "" {
			t.Errorf("%s: %s", bad, body)
		}
	}
	response, err = http.Post(server.URL+"/speak", "application/json", strings.NewReader(`{"text": "`+strings.Repeat("a", maximumRequestSize)+`"}`))
	check("too large", response, err, 413, "application/json")

	response, err = http.Get(server.URL + "/voices")
	var list []opensapi4.Voice
	if json.Unmarshal(check("voices", response, err, 200, "application/json"), &list) != nil || len(list) != 19 {
		t.Errorf("voices: %d", len(list))
	}
	response, err = http.Get(server.URL + "/")
	check("page", response, err, 200, "text/html; charset=utf-8")
	options, _ := http.NewRequest(http.MethodOptions, server.URL+"/speak", nil)
	response, err = http.DefaultClient.Do(options)
	check("options", response, err, 204, "")
	if methods := response.Header.Get("Access-Control-Allow-Methods"); methods != "GET, POST" {
		t.Errorf("options: %q", methods)
	}
	response, err = http.Get(server.URL + "/nothing")
	check("not found", response, err, 404, "text/plain; charset=utf-8")
}
