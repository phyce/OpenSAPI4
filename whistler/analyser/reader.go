package analyser

import (
	"bytes"
	"strings"

	"github.com/phyce/OpenSAPI4/whistler/datapack"
	"github.com/phyce/OpenSAPI4/whistler/pronunciation"
	"github.com/phyce/OpenSAPI4/whistler/prosody"
	"github.com/phyce/OpenSAPI4/whistler/rules"
)

type ReadResult int

const (
	ReadError            ReadResult = 0x6E
	ReadNothingLeft      ReadResult = 0x6F
	ReadNormal           ReadResult = 0x70
	ReadClauseLimit      ReadResult = 0x71
	ReadSourceLimit      ReadResult = 0x72
	ReadPunctuationLimit ReadResult = 0x73
	ReadWordCut          ReadResult = 0x74
	ReadPhoneticCut      ReadResult = 0x75
	ReadOther            ReadResult = 0x76
)

const (
	escape         = 0x1B
	endOfText      = 0x1A
	marker         = 0x11
	carriageReturn = '\r'
	lineFeed       = '\n'
	tab            = '\t'
	space          = ' '
	sourceLimit    = 0x500
	bracketLimit   = 0x432
	tokenLimit     = 0x40
	lastTierCell   = rules.Cells - 1
)

// Event is a record the reader queues: the sentence first, then markers, pauses and words in reading order.
// Offsets count bytes from the start of the read.
type Event struct {
	Kind   prosody.EventKind
	Offset int32
	Length int32
	Value  uint32
}

// Read is one call of the reader on the unread rest of a segment. State holds what it read: tier 1, the
// anchors and the events. Overflowed reports a sentence whose markers would have filled tier 1; the
// engine drops it (text.md §10).
type Read struct {
	Result     ReadResult
	Consumed   int
	NaturalEnd bool
	Overflowed bool
	State      *State
}

// Sentence reports whether the read holds a sentence to transcribe.
func (read *Read) Sentence() bool {
	return read.Result >= ReadNormal && read.Result <= ReadPhoneticCut && !read.Overflowed
}

type readerJump int

// Reader reads the FrontEnd's segments sentence by sentence into tier 1, in line mode (text.md §8). Some of
// its values last from one read to the next, as the original's globals do, so one reader serves one voice.
type Reader struct {
	// UserLexicon, when set, is looked up before the built-in exceptions: it gives a word's stored ARPAbet.
	UserLexicon interface {
		Find(word []byte) ([]byte, bool)
	}

	tables               *datapack.Prosody
	lexicon              map[string][]byte
	durationStyle        int
	wordsSinceCandidate  int
	wordStart            int
	wordStartIndex       int
	suppressCapitalBreak bool

	state                 *State
	data                  []byte
	cursor                int
	outIndex              int
	contentCount          int
	wordLength            int
	textStart             int
	offsetOrigin          int
	sentenceStart         int
	lastByte              int
	rawMode               bool
	addressMode           bool
	rawAddressMode        bool
	weight                int16
	sentenceEndCandidate  int
	lastWordEnd           int
	breakReason           int
	limit1Armed           bool
	limit2Armed           bool
	weightBreakArmed      bool
	weightCutArmed        bool
	clausePunctuationSeen bool
	overflowed            bool
}

// NewReader makes a reader with the built-in exceptions as its lexicon. Entries whose pronunciation has
// a name the phone table lacks are left out, as the original's conversion fails on them.
func NewReader(dictionaries *datapack.Dictionaries, tables *datapack.Prosody) *Reader {
	reader := &Reader{tables: tables, lexicon: map[string][]byte{}, wordStart: -1}
	for _, exception := range dictionaries.BuiltInExceptions {
		phones, converted := pronunciation.FromARPAbet(tables, exception.Pronunciation)
		if _, found := reader.lexicon[strings.ToUpper(exception.Word)]; converted && !found {
			reader.lexicon[strings.ToUpper(exception.Word)] = phones
		}
	}
	return reader
}

// Segment is a segment as the reader reads it: its text followed by the end-of-text byte.
type Segment []byte

func NewSegment(text []byte) Segment {
	return append(append(Segment(nil), text...), endOfText)
}

// Length is the length of the segment's text.
func (segment Segment) Length() int {
	return len(segment) - 1
}

// ReadSegment reads every sentence of a segment, each into a fresh state. The last read is the one that
// found nothing left (or failed).
func (reader *Reader) ReadSegment(text []byte) []Read {
	segment := NewSegment(text)
	var reads []Read
	for position := 0; ; {
		read := reader.ReadSentence(segment, position)
		reads = append(reads, read)
		if read.Result < ReadNormal || read.Result > ReadPhoneticCut {
			return reads
		}
		position += read.Consumed
	}
}

// ReadSentence reads the sentence at position into a fresh state. A read that stops without finding the
// end of a sentence consumes the whole rest, the end-of-text byte included.
func (reader *Reader) ReadSentence(segment Segment, position int) Read {
	return reader.readSentence(segment, position)
}

func isAlphabetic(character byte) bool {
	return character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z'
}

func isWhitespace(character byte) bool {
	return character == carriageReturn || character == lineFeed || character == space || character == tab
}

func weightOf(character byte) int16 {
	if isDigit(character) || bytes.IndexByte([]byte("()[]><$%~+-/\\{}*_"), character) >= 0 {
		return 16
	}
	return 2
}

// scanUnsigned reads up to count numbers as sscanf("%lu %lu ...") does; missing ones stay 0.
func scanUnsigned(text []byte, count int) []uint32 {
	values := make([]uint32, count)
	position := 0
	for index := range count {
		for position < len(text) && bytes.IndexByte([]byte(" \t\r\n"), text[position]) >= 0 {
			position++
		}
		negative := false
		if position < len(text) && (text[position] == '+' || text[position] == '-') {
			negative = text[position] == '-'
			position++
		}
		digitsStart := position
		var value uint32
		for position < len(text) && isDigit(text[position]) {
			value = value*10 + uint32(text[position]-'0')
			position++
		}
		if position == digitsStart {
			break
		}
		if negative {
			value = -value
		}
		values[index] = value
	}
	return values
}

func (reader *Reader) tierCell(index int) *byte {
	if index+1 > lastTierCell {
		reader.overflowed = true
		var discarded byte
		return &discarded
	}
	return &reader.state.Tier(1)[index+1]
}

func (reader *Reader) get(index int) byte {
	return *reader.tierCell(index)
}

func (reader *Reader) put(index int, character byte) {
	*reader.tierCell(index) = character
}

func (reader *Reader) emit(characters ...byte) {
	for _, character := range characters {
		reader.put(reader.outIndex, character)
		reader.outIndex++
	}
}

// addEvent queues an event record and anchors it at tier-1 index anchor. The FrontEnd's event mask
// records every kind the reader makes.
func (reader *Reader) addEvent(kind prosody.EventKind, offset int, length int, value uint32, anchor int) {
	event := Event{Kind: kind, Offset: int32(offset), Length: int32(length), Value: value}
	if kind == prosody.SentenceEvent {
		reader.state.Events = append([]Event{event}, reader.state.Events...)
	} else {
		reader.state.Events = append(reader.state.Events, event)
	}
	if anchor+1 <= lastTierCell {
		reader.state.Anchors[anchor+1]++
	} else {
		reader.overflowed = true
	}
}

func (reader *Reader) advance() byte {
	if reader.cursor == reader.lastByte {
		panic(readerJump(-1))
	}
	reader.cursor++
	return reader.data[reader.cursor]
}

func (reader *Reader) back() byte {
	if reader.textStart < reader.cursor {
		reader.cursor--
	}
	return reader.data[reader.cursor]
}

func (reader *Reader) readSentence(data []byte, start int) (read Read) {
	state := &State{State: *rules.NewState(), Start: 0, End: 0}
	reader.state = state
	reader.overflowed = false
	read.State = state
	defer func() {
		state.DurationStyle = reader.durationStyle
		read.Overflowed = reader.overflowed
	}()
	if len(data) == start {
		read.Result = ReadError
		return read
	}
	reader.addressMode, reader.rawAddressMode = false, false
	defer func() {
		if recovered := recover(); recovered != nil {
			jump, isJump := recovered.(readerJump)
			if !isJump {
				panic(recovered)
			}
			read.Result = ReadError
			if jump == -2 {
				read.Result = ReadNothingLeft
			}
			read.Consumed = len(data) - start
		}
	}()
	reader.data, reader.cursor, reader.textStart, reader.offsetOrigin, reader.sentenceStart = data, start, start, start, start
	reader.outIndex, reader.contentCount, reader.wordLength = 0, 0, 0
	reader.lastByte = len(data) - 1
	reader.rawMode = false
	for {
		result := reader.scanSentence()
		read.Consumed = reader.cursor - reader.textStart
		if result < 0 {
			read.Result = map[int]ReadResult{-1: ReadError, -3: ReadNothingLeft}[result]
			if read.Result == 0 {
				read.Result = ReadOther
			}
			return read
		}
		if reader.contentCount == 0 {
			continue
		}
		state.Start = 1
		reader.addEvent(prosody.SentenceEvent, reader.sentenceStart-reader.textStart, reader.cursor-reader.sentenceStart, 0, 0)
		state.End = min(reader.outIndex, lastTierCell)
		if result > 0xA00 {
			reader.closeAddressMode()
			read.Result = ReadPhoneticCut
			return read
		}
		if result < 0x501 {
			read.NaturalEnd = true
		} else {
			reader.closeAddressMode()
		}
		read.Result = ReadNormal + ReadResult(reader.breakReason)
		return read
	}
}

// closeAddressMode appends " \x11[\addr]" and a NUL after the end of a sentence cut by a length limit in
// address mode.
func (reader *Reader) closeAddressMode() {
	if reader.addressMode {
		reader.emit([]byte(" \x11[\\addr]")...)
		reader.put(reader.outIndex, 0)
	}
}

func (reader *Reader) readToken() []byte {
	var token []byte
	for index := 0; ; {
		character := reader.advance()
		if isWhitespace(character) {
			break
		}
		token = append(token, character)
		index++
		if index >= tokenLimit {
			break
		}
	}
	return token
}

// readBracket stores ESC [ outside raw mode: ' ', 0x11 and the bracket text up to ']'. "[addr]" turns
// address mode on and "[\addr]" off; a repeated one, or an "off" while off, is taken out again.
func (reader *Reader) readBracket() {
	reader.emit(space)
	markerIndex := reader.outIndex
	reader.emit(marker)
	savedWeight := reader.weight
	bracket := reader.outIndex
	reader.emit('[')
	for reader.outIndex < bracketLimit {
		character := reader.advance()
		reader.emit(character)
		if character == ']' {
			break
		}
		reader.weight++
	}
	reader.put(reader.outIndex, 0)
	equals := func(literal string) bool {
		for offset, character := range []byte(literal + "\x00") {
			if reader.get(bracket+offset) != character {
				return false
			}
		}
		return true
	}
	if equals("[addr]") {
		if !reader.addressMode {
			reader.addressMode = true
			reader.emit(space)
		} else {
			reader.outIndex = markerIndex - 1
			reader.weight = savedWeight
		}
	}
	if equals("[\\addr]") {
		if !reader.addressMode {
			reader.outIndex = markerIndex - 1
			reader.weight = savedWeight
		} else {
			if reader.sentenceEndCandidate >= 0 {
				reader.lastWordEnd = reader.outIndex
				reader.sentenceEndCandidate = reader.cursor
			}
			reader.addressMode, reader.rawAddressMode = false, false
			reader.emit(space)
		}
	}
	reader.put(reader.outIndex, space)
}

// readCharacter returns the current byte without consuming it, and runs and consumes escape sequences
// (text.md §8.1–8.2). Escape commands the FrontEnd never sends are skipped with the byte after them.
func (reader *Reader) readCharacter() byte {
	character := reader.data[reader.cursor]
	if character == endOfText {
		reader.rawMode, reader.addressMode = false, false
		if reader.weight != 0 {
			return endOfText
		}
		panic(readerJump(-2))
	}
	if character != escape {
		if isWhitespace(character) {
			return character
		}
		if character < 0x0E || character > 0xAF {
			return space
		}
		return character
	}
	escapeAt := reader.cursor
	if reader.outIndex == 0 {
		reader.sentenceStart = reader.cursor
	}
	if reader.wordLength != 0 {
		reader.endWord(reader.wordStartIndex)
	}
	command := reader.advance()
	switch {
	case command == '"' || command == '#' || command == '\'' || command == '+' || command == '@':
		reader.emit(marker, command)
		if reader.contentCount == 0 {
			reader.contentCount = 2
		}
		if command == '@' {
			reader.contentCount++
			reader.emit(reader.advance(), space)
		}
	case command == '/' || command == '[' && reader.rawMode:
		reader.emit(space, marker, '/')
		if reader.contentCount == 0 {
			reader.contentCount = 2
		}
		reader.rawMode = !reader.rawMode
		reader.rawAddressMode = reader.addressMode && reader.rawMode
	case command == '[':
		reader.readBracket()
	case command == 'H' || command == 'h':
		value := scanUnsigned(reader.readToken(), 1)[0]
		reader.addEvent(prosody.PauseEvent, escapeAt-reader.textStart, 0, value, reader.outIndex)
		return reader.readCharacter()
	case command == 'P' || command == 'p':
		reader.emit(space, marker, 'P', reader.advance())
		if reader.contentCount == 0 {
			reader.contentCount = 4
		}
	case command == 'A' || command == 'a':
		if reader.weight != 0 {
			reader.back()
			return endOfText
		}
		switch reader.advance() {
		case 'F', 'f':
			reader.durationStyle = 0
		case 'P', 'p':
			reader.durationStyle = 1
		}
	case command == 'N' || command == 'n':
		token := reader.readToken()
		for _, separator := range []byte("[,]") {
			if index := bytes.IndexByte(token, separator); index >= 0 {
				token[index] = space
			}
		}
		values := scanUnsigned(token, 3)
		offset := int(int32(values[1]))
		if values[1] == 0 {
			offset = escapeAt - reader.offsetOrigin
		}
		reader.addEvent(prosody.MarkerEvent, offset, int(int32(values[2])), values[0], reader.outIndex)
	default:
		reader.advance()
		return space
	}
	reader.advance()
	return reader.readCharacter()
}

// scanSentence reads one sentence in line mode and returns its weight, plus 0x500 for a length break and
// 0xa00 for a phonetic cut (text.md §8.3–8.5).
func (reader *Reader) scanSentence() int {
	pendingNewline := false
	reader.weight = 0
	reader.suppressCapitalBreak = false
	reader.sentenceEndCandidate = -1
	reader.lastWordEnd = 0
	reader.checkLengthBreak(0)
	for character := reader.readCharacter(); character != endOfText; character = reader.readCharacter() {
		newline := character == carriageReturn || character == lineFeed
		if pendingNewline {
			if newline {
				return int(reader.weight)
			}
			if character != space && character != tab {
				pendingNewline = false
			}
		}
		if newline && reader.weight != 0 {
			pendingNewline = true
			return int(reader.weight)
		}
		skip := false
		if !reader.rawMode {
			if reader.contentCount == 0 {
				if isWhitespace(character) {
					skip = true
				} else {
					reader.sentenceStart = reader.cursor
					reader.contentCount = 1
				}
			} else {
				reader.contentCount++
			}
			if !skip {
				switch {
				case reader.wordLength == 0:
					if !isWhitespace(character) {
						reader.wordStart = reader.cursor
						reader.wordStartIndex = reader.outIndex
						reader.wordLength = 1
					}
				case isWhitespace(character):
					reader.wordsSinceCandidate++
					if !reader.endWord(reader.wordStartIndex) {
						if reader.weight != 0 {
							return int(reader.weight)
						}
						reader.checkLengthBreak(0)
					} else if newline && reader.sentenceEndCandidate >= 0 && reader.wordsSinceCandidate == 0 {
						return int(reader.weight)
					}
				default:
					reader.wordLength++
				}
				if isWhitespace(character) {
					character = space
				}
				reader.weight += weightOf(character)
			}
		} else if character == carriageReturn || character == lineFeed || character == tab {
			skip = true
		} else {
			reader.contentCount++
			reader.weight++
			if reader.weight > 0x428 && !isDigit(character) && bytes.IndexByte([]byte("[](),"), character) < 0 {
				reader.breakReason = 5
				return int(reader.weight) + 0xA00
			}
		}
		if !skip {
			reader.emit(character)
			if !reader.rawMode {
				switch reason := reader.checkLengthBreak(character); {
				case reason == 1:
					if reader.weight != 0 {
						return int(reader.weight)
					}
					return int(reader.weight) + 0x500
				case reason >= 2 && reason <= 4:
					return int(reader.weight) + 0x500
				}
			}
		}
		reader.advance()
	}
	if reader.wordLength != 0 {
		reader.endWord(reader.wordStartIndex)
	}
	return int(reader.weight)
}

// endWord ends the word from tier-1 index start: repeated symbols drop it, and the lexicon may replace it
// (text.md §8.4). It reports false for a dropped word.
func (reader *Reader) endWord(start int) bool {
	end := reader.outIndex
	first := reader.get(start)
	for index := start; index < end; index++ {
		reader.weight -= weightOf(reader.get(index))
	}
	runEnd := start + 1
	for runEnd < end && reader.get(runEnd) == first {
		runEnd++
	}
	if !isAlphabetic(first) && !isDigit(first) {
		length := 3
		if first == '.' {
			length = 10
		}
		if start+length <= runEnd {
			reader.outIndex = start
			reader.wordLength = 0
			return false
		}
	} else if start+10 <= runEnd {
		reader.outIndex = start
		return false
	}
	reader.put(end, 0)
	var stripped byte
	if last := reader.get(end - 1); bytes.IndexByte([]byte(".?!:;,"), last) >= 0 {
		stripped = last
		reader.put(end-1, 0)
	}
	currentWord := func() []byte {
		var word []byte
		for index := start; reader.get(index) != 0 && !reader.overflowed; index++ {
			word = append(word, reader.get(index))
		}
		return word
	}
	copyResult := func(result []byte) int {
		for offset, character := range result {
			reader.put(start+offset, character)
		}
		reader.put(start+len(result), 0)
		return start + len(result)
	}
	if result, found := reader.replaceWord(currentWord()); found {
		end = copyResult(result)
		if stripped != 0 {
			end++
			reader.put(end-1, stripped)
		}
	} else {
		if stripped == '.' {
			reader.put(end-1, '.')
			stripped = 0
			if result, found := reader.replaceWord(currentWord()); found {
				end = copyResult(result)
			}
		}
		if stripped != 0 {
			reader.put(end-1, stripped)
		}
	}
	sentencePunctuation := []byte(".?!:;")
	last := reader.get(end - 1)
	if bytes.IndexByte(sentencePunctuation, last) >= 0 && !(end-start > 1 && reader.get(end-2) >= 'A' && reader.get(end-2) <= 'Z') {
		reader.wordsSinceCandidate = 0
		reader.sentenceEndCandidate = reader.cursor - 1
	}
	if end-start > 1 && last == '"' && bytes.IndexByte(sentencePunctuation, reader.get(end-2)) >= 0 {
		reader.wordsSinceCandidate = 0
		reader.sentenceEndCandidate = reader.cursor - 1
	}
	if !(bytes.IndexByte(sentencePunctuation, last) >= 0 && end-start == 1) {
		reader.addEvent(prosody.WordEvent, reader.wordStart-reader.offsetOrigin, reader.wordLength, 0, start)
	}
	reader.lastWordEnd = end
	for index := start; index < end; index++ {
		reader.weight += weightOf(reader.get(index))
	}
	reader.outIndex = end
	reader.wordLength = 0
	return true
}

// replaceWord looks the word's letters (with - ' .) up in the user lexicon, then in the built-in exceptions,
// ignoring case, and brackets the pronunciation in 0x10 (text.md §8.4). A user entry whose pronunciation
// has a name the phone table lacks leaves the word to the rules.
func (reader *Reader) replaceWord(word []byte) ([]byte, bool) {
	inCore := func(character byte) bool {
		return isAlphabetic(character) || character == '-' || character == '\'' || character == '.'
	}
	prefixEnd := 0
	for prefixEnd < len(word) && !inCore(word[prefixEnd]) {
		prefixEnd++
	}
	if prefixEnd == len(word) {
		return nil, false
	}
	coreEnd := prefixEnd
	for coreEnd < len(word) && inCore(word[coreEnd]) {
		coreEnd++
	}
	for rest := coreEnd; rest < len(word); rest++ {
		if inCore(word[rest]) {
			return nil, false
		}
	}
	core := word[prefixEnd:coreEnd]
	phones, found := reader.lexicon[string(bytes.ToUpper(core))]
	if reader.UserLexicon != nil {
		if arpabet, inUserLexicon := reader.UserLexicon.Find(core); inUserLexicon {
			phones, found = pronunciation.FromARPAbet(reader.tables, string(arpabet))
		}
	}
	if !found {
		return nil, false
	}
	result := append([]byte(nil), word[:prefixEnd]...)
	result = append(result, " \x10 "...)
	result = append(result, phones...)
	result = append(result, " \x10 "...)
	return append(result, word[coreEnd:]...), true
}

// checkLengthBreak arms and applies the length limits after each stored byte; 0 resets them (text.md §8.5).
func (reader *Reader) checkLengthBreak(character byte) int {
	if character == 0 {
		reader.weightBreakArmed, reader.weightCutArmed, reader.limit2Armed, reader.limit1Armed = false, false, false, false
		reader.breakReason = 0
		reader.clausePunctuationSeen = false
		return 0
	}
	if reader.rawMode {
		return 0
	}
	consumed := reader.cursor - reader.textStart
	switch {
	case !reader.limit1Armed:
		reader.limit1Armed = sourceLimit <= consumed
	case character == ',' || character == ';' || character == ':':
		reader.clausePunctuationSeen = true
	default:
		if (character == ' ' || character == tab || character == lineFeed) && reader.clausePunctuationSeen {
			reader.breakReason = 1
			return 1
		}
		reader.clausePunctuationSeen = false
	}
	if !reader.limit2Armed {
		reader.limit2Armed = sourceLimit <= consumed
	} else if isWhitespace(character) {
		reader.breakReason = 2
		return 2
	}
	if !reader.weightBreakArmed {
		reader.weightBreakArmed = reader.weight > 0x490
		if reader.weightBreakArmed {
			reader.limit1Armed, reader.limit2Armed = true, true
		}
	} else if bytes.IndexByte([]byte(",.!?;:"), character) >= 0 {
		reader.breakReason = 3
		return 3
	}
	if !reader.weightCutArmed {
		reader.weightCutArmed = reader.weight > 0x4A0
	} else {
		reader.breakReason = 4
		if reader.wordStart >= 0 && reader.wordStartIndex != 0 {
			for reader.wordStart < reader.cursor {
				reader.back()
			}
			reader.outIndex = reader.wordStartIndex
			return 4
		}
		reader.weight = 0
		reader.outIndex = 0
		reader.checkLengthBreak(0)
	}
	return 0
}
