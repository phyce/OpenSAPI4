package lexicon

import (
	"bytes"
	"errors"
	"io"
)

// ErrInvalidEntries is a file with entries the original skips: it then says once, in a message box, that the file may
// be corrupted, and reads the rest.
var ErrInvalidEntries = errors.New("lexicon: the file has invalid entries, which were skipped")

const (
	// longestEntry is the most characters of a word or a pronunciation a valid entry has.
	longestEntry = 39
	endOfFile    = 0x1a
)

// Read replaces the lexicon's words with a file's, as the original reads its file when its first engine starts
// (engine.md §7.4). The file is the original's: each word and its pronunciation, each followed by a NUL, and Ctrl+Z
// its end. An entry of 40 characters or more, or with characters a word or a pronunciation can't have, is skipped,
// and Read returns ErrInvalidEntries; the lexicon keeps the rest, up to Capacity entries. Words the file gives twice
// stay twice. Read doesn't change what Changed reports. A file that can't be read leaves the lexicon empty, and Read
// returns the error.
func (lexicon *Lexicon) Read(file io.Reader) error {
	data, err := io.ReadAll(file)
	lexicon.lock.Lock()
	defer lexicon.lock.Unlock()
	lexicon.entries = nil
	if err != nil {
		return err
	}
	if end := bytes.IndexByte(data, endOfFile); end >= 0 {
		data = data[:end]
	}
	reader := &fieldReader{data: data}
	var result error
	for len(lexicon.entries) < Capacity {
		word, pronunciation, read := reader.entry()
		if !read {
			break
		}
		if !validEntry(word, pronunciation) {
			result = ErrInvalidEntries
			continue
		}
		lexicon.entries = append(lexicon.entries, entry{word: upper(word), pronunciation: pronunciation})
	}
	sortEntries(lexicon.entries)
	return result
}

func validEntry(word, pronunciation []byte) bool {
	if len(word) > longestEntry || len(pronunciation) > longestEntry {
		return false
	}
	for _, character := range word {
		if !isLetter(character) {
			return false
		}
	}
	for _, symbol := range pronunciation {
		if bytes.IndexByte([]byte(pronunciationSymbols), symbol) < 0 {
			return false
		}
	}
	return true
}

// fieldReader reads a file's entries.
type fieldReader struct {
	data     []byte
	position int
}

// entry reads a word and its pronunciation, each up to its NUL, which it skips. The original keeps at most 100
// characters of each, which changes nothing: an entry of 40 or more is invalid. It reports false at the end of the
// file, and drops an entry the file ends in. An empty field gives what the original's buffer held before, which the
// port makes empty.
func (reader *fieldReader) entry() ([]byte, []byte, bool) {
	word, read := reader.field()
	if !read {
		return nil, nil, false
	}
	pronunciation, read := reader.field()
	return word, pronunciation, read
}

func (reader *fieldReader) field() ([]byte, bool) {
	end := bytes.IndexByte(reader.data[reader.position:], 0)
	if end < 0 {
		return nil, false
	}
	value := bytes.Clone(reader.data[reader.position : reader.position+end])
	reader.position += end + 1
	return value, true
}

// Write writes the lexicon in the original's file format: each word, in capitals, and its pronunciation, each
// followed by a NUL, in the lexicon's order. Changed reports false after it.
func (lexicon *Lexicon) Write(file io.Writer) error {
	lexicon.lock.Lock()
	output := []byte{}
	for _, current := range lexicon.entries {
		output = append(append(append(append(output, current.word...), 0), current.pronunciation...), 0)
	}
	lexicon.changed = false
	lexicon.lock.Unlock()
	_, err := file.Write(output)
	return err
}
