package main

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"time"

	opensapi4 "github.com/phyce/OpenSAPI4"
	"github.com/phyce/OpenSAPI4/internal/wave"
)

//go:embed page.html
var page []byte

const maximumRequestSize = 65536

// serve speaks over HTTP: GET /voices lists the voices, GET or POST /speak speaks, and / is a page to try them.
func serve(arguments []string) int {
	flags, folder := newFlags("serve", "[-address host:port] [-origin origin]")
	address := flags.String("address", "127.0.0.1:5040", "the address to listen on; :5040 listens on every interface")
	origin := flags.String("origin", "", `the origin web pages may call from, such as https://example.com, or * for any (default: none)`)
	others, code, proceed := parseFlags(flags, arguments)
	if !proceed {
		return code
	}
	if len(others) != 0 {
		flags.Usage()
		return 2
	}
	voices, err := openVoices(*folder)
	if err != nil {
		return fail(err)
	}
	if _, err := voices.List(); err != nil {
		return fail(err)
	}
	listener, err := net.Listen("tcp", *address)
	if err != nil {
		return fail(err)
	}
	server := &http.Server{Handler: newHandler(voices, *origin), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute}
	interrupted, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		<-interrupted.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
	}()
	fmt.Fprintf(os.Stderr, "opensapi4 %s: serving on http://%s\n", programVersion(), listener.Addr())
	if err := server.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		return fail(err)
	}
	return 0
}

type handler struct {
	voices  *opensapi4.Voices
	origin  string
	workers chan struct{}
	routes  *http.ServeMux
}

func newHandler(voices *opensapi4.Voices, origin string) http.Handler {
	served := &handler{voices: voices, origin: origin, workers: make(chan struct{}, runtime.GOMAXPROCS(0)), routes: http.NewServeMux()}
	served.routes.HandleFunc("GET /{$}", served.page)
	served.routes.HandleFunc("GET /voices", served.listVoices)
	served.routes.HandleFunc("GET /speak", served.speakQuery)
	served.routes.HandleFunc("POST /speak", served.speakBody)
	return served
}

func (served *handler) ServeHTTP(response http.ResponseWriter, incoming *http.Request) {
	if served.origin != "" {
		response.Header().Set("Access-Control-Allow-Origin", served.origin)
		if incoming.Method == http.MethodOptions {
			response.Header().Set("Access-Control-Allow-Methods", "GET, POST")
			response.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			response.WriteHeader(http.StatusNoContent)
			return
		}
	}
	served.routes.ServeHTTP(response, incoming)
}

func (served *handler) page(response http.ResponseWriter, _ *http.Request) {
	response.Header().Set("Content-Type", "text/html; charset=utf-8")
	response.Write(page)
}

func (served *handler) listVoices(response http.ResponseWriter, _ *http.Request) {
	list, err := served.voices.List()
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	response.Header().Set("Content-Type", "application/json")
	json.NewEncoder(response).Encode(list)
}

func (served *handler) speakQuery(response http.ResponseWriter, incoming *http.Request) {
	parsed, err := queryRequest(incoming.URL.Query())
	if err != nil {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	served.speak(response, incoming, parsed)
}

func (served *handler) speakBody(response http.ResponseWriter, incoming *http.Request) {
	content, err := io.ReadAll(http.MaxBytesReader(response, incoming.Body, maximumRequestSize))
	if err != nil {
		writeError(response, http.StatusRequestEntityTooLarge, fmt.Errorf("a request may have up to %d bytes", maximumRequestSize))
		return
	}
	parsed, err := decodeRequest(content)
	if err != nil {
		writeError(response, http.StatusBadRequest, fmt.Errorf("read the request: %w", err))
		return
	}
	if format := incoming.URL.Query().Get("format"); format != "" {
		parsed.Format = format
	}
	served.speak(response, incoming, parsed)
}

// queryRequest reads a request from a URL's query: its fields as parameters, text given once or more.
func queryRequest(query url.Values) (request, error) {
	var parsed request
	texts := query["text"]
	if len(texts) > 0 {
		parsed.Text, parsed.Texts = texts[0], texts[1:]
	}
	if size := len(strings.Join(texts, "")); size > maximumRequestSize {
		return request{}, fmt.Errorf("a request may have up to %d bytes", maximumRequestSize)
	}
	parsed.Voice, parsed.Inject, parsed.Format = query.Get("voice"), query.Get("inject"), query.Get("format")
	numbers := map[string]**int{"pitch": &parsed.Pitch, "speed": &parsed.Speed, "volume": &parsed.Volume}
	for name, field := range numbers {
		if value := query.Get(name); value != "" {
			number, err := strconv.Atoi(value)
			if err != nil {
				return request{}, fmt.Errorf("%s must be a number, not %q", name, value)
			}
			*field = &number
		}
	}
	for name, field := range map[string]*int{"rate": &parsed.Rate, "bits": &parsed.Bits} {
		if value := query.Get(name); value != "" {
			number, err := strconv.Atoi(value)
			if err != nil {
				return request{}, fmt.Errorf("%s must be a number, not %q", name, value)
			}
			*field = number
		}
	}
	tagged := true
	switches := map[string]*bool{"tagged": &tagged, "ipa": &parsed.IPA, "events": &parsed.Events, "visual": &parsed.Visual}
	for name, field := range switches {
		if value := query.Get(name); value != "" {
			on, err := strconv.ParseBool(value)
			if err != nil {
				return request{}, fmt.Errorf("%s must be true or false, not %q", name, value)
			}
			*field = on
		}
	}
	if query.Get("tagged") != "" {
		parsed.Tagged = &tagged
	}
	return parsed, nil
}

// speak speaks a request as WAV (the default), raw PCM or the stream's JSON messages ("format": "wav", "pcm" or
// "json").
func (served *handler) speak(response http.ResponseWriter, incoming *http.Request, parsed request) {
	if parsed.Output != "" {
		writeError(response, http.StatusBadRequest, errors.New(`the server takes no "output"`))
		return
	}
	format := parsed.Format
	if format == "" {
		format = "wav"
	}
	if format != "wav" && format != "pcm" && format != "json" {
		writeError(response, http.StatusBadRequest, fmt.Errorf(`format must be "wav", "pcm" or "json", not %q`, format))
		return
	}
	if (parsed.Events || parsed.Visual) && format != "json" {
		writeError(response, http.StatusBadRequest, errors.New(`events come only with "format": "json"`))
		return
	}
	speech, err := served.voices.NewSpeech(parsed.Request)
	var requestError *opensapi4.RequestError
	if errors.As(err, &requestError) {
		writeError(response, http.StatusBadRequest, err)
		return
	}
	if err != nil {
		writeError(response, http.StatusInternalServerError, err)
		return
	}
	requestContext := incoming.Context()
	select {
	case served.workers <- struct{}{}:
		defer func() { <-served.workers }()
	case <-requestContext.Done():
		return
	}
	speechFormat := speech.Format()
	switch format {
	case "wav":
		buffer := &bufferSink{}
		if err := speech.Speak(contextSink{sink: buffer, context: requestContext}); err != nil {
			writeError(response, http.StatusInternalServerError, err)
			return
		}
		response.Header().Set("Content-Type", "audio/wav")
		response.Header().Set("Content-Length", strconv.Itoa(wave.HeaderSize+len(buffer.data)))
		response.Write(wave.Header(wave.Format(speechFormat), uint32(len(buffer.data))))
		response.Write(buffer.data)
	case "pcm":
		response.Header().Set("Content-Type", "application/octet-stream")
		response.Header().Set("X-Sample-Rate", strconv.Itoa(speechFormat.SampleRate))
		response.Header().Set("X-Bits", strconv.Itoa(speechFormat.Bits))
		response.Header().Set("X-Channels", "1")
		output := bufio.NewWriterSize(response, audioMessageSize)
		if err := speech.Speak(contextSink{sink: pcmSink{output: output}, context: requestContext}); err != nil {
			panic(http.ErrAbortHandler)
		}
		output.Flush()
	case "json":
		response.Header().Set("Content-Type", "application/x-ndjson")
		controller := http.NewResponseController(response)
		writer := &messages{output: bufio.NewWriterSize(response, audioMessageSize), flush: func() { controller.Flush() }, id: parsed.ID,
			events: parsed.Events, visual: parsed.Visual, stop: requestContext.Err}
		if err := writer.sendFormat(speechFormat); err != nil {
			return
		}
		writer.finish(speech.Speak(writer), "")
	}
}

// contextSink stops a speech when its request's context ends: the client went away.
type contextSink struct {
	sink interface {
		Audio(data []byte) (int, error)
	}
	context context.Context
}

func (sink contextSink) Audio(data []byte) (int, error) {
	if err := sink.context.Err(); err != nil {
		return 0, err
	}
	return sink.sink.Audio(data)
}

func (sink contextSink) Event(opensapi4.Event) error {
	return nil
}

func writeError(response http.ResponseWriter, status int, err error) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	json.NewEncoder(response).Encode(map[string]string{"error": err.Error()})
}
