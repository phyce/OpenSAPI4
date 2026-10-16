package lexicon

import (
	"bytes"
	"errors"
	"math/rand"
	"slices"
	"strings"
	"testing"
)

func TestReadWrite(t *testing.T) {
	file := "zebra\x00zE1br@\x00BAD1\x00x\x00APPLE\x00@1pL\x00" + strings.Repeat("W", 40) + "\x00x\x00MANGO\x00m@1NgO\x00" +
		"\x1aPEAR\x00pe1R\x00"
	lexicon := New(nil, nil, nil)
	if err := lexicon.Read(strings.NewReader(file)); !errors.Is(err, ErrInvalidEntries) {
		t.Errorf("Read gave %v, expected ErrInvalidEntries", err)
	}
	if lexicon.Changed() {
		t.Error("Read changed the lexicon")
	}
	if pronunciation, found := lexicon.Pronunciation("ZEBRA"); !found || pronunciation != "zE1br@" {
		t.Errorf("ZEBRA gave %q, %v", pronunciation, found)
	}
	if _, found := lexicon.Pronunciation("PEAR"); found {
		t.Error("PEAR, after Ctrl+Z, was read")
	}
	var written bytes.Buffer
	if err := lexicon.Write(&written); err != nil {
		t.Fatal(err)
	}
	if expected := "APPLE\x00@1pL\x00MANGO\x00m@1NgO\x00ZEBRA\x00zE1br@\x00"; written.String() != expected {
		t.Errorf("Write gave %q, expected %q", written.String(), expected)
	}
	if err := lexicon.Read(strings.NewReader("APPLE\x00@1pL\x00ZEBRA\x00x")); err != nil {
		t.Errorf("a file without its last NUL gave %v", err)
	}
	if _, found := lexicon.Pronunciation("ZEBRA"); found {
		t.Error("the entry without its NUL was read")
	}
}

func TestClone(t *testing.T) {
	lexicon := New(nil, nil, nil)
	if err := lexicon.Read(strings.NewReader("APPLE\x00@1pL\x00ZEBRA\x00zE1br@\x00")); err != nil {
		t.Fatal(err)
	}
	lexicon.store([]byte("MANGO"), []byte("m@1NgO"))
	clone := lexicon.Clone(nil)
	if clone.Changed() {
		t.Error("the clone is changed")
	}
	clone.store([]byte("APPLE"), []byte("A1pL"))
	clone.store([]byte("PEAR"), []byte("pe1R"))
	for _, test := range []struct {
		lexicon       *Lexicon
		word          string
		pronunciation string
	}{
		{lexicon, "APPLE", "@1pL"}, {lexicon, "MANGO", "m@1NgO"}, {lexicon, "PEAR", ""},
		{clone, "APPLE", "A1pL"}, {clone, "MANGO", "m@1NgO"}, {clone, "ZEBRA", "zE1br@"}, {clone, "PEAR", "pe1R"},
	} {
		if pronunciation, _ := test.lexicon.Pronunciation(test.word); pronunciation != test.pronunciation {
			t.Errorf("%s gave %q, expected %q", test.word, pronunciation, test.pronunciation)
		}
	}
}

func TestOrder(t *testing.T) {
	random := rand.New(rand.NewSource(18))
	for _, size := range []int{2, 5, 8, 9, 10, 33, 100, 1000} {
		var entries []entry
		for range size {
			word := []byte{byte('A' + random.Intn(4)), byte('A' + random.Intn(3))}
			entries = append(entries, entry{word: word[:1+random.Intn(2)]})
		}
		sortEntries(entries)
		if !slices.IsSortedFunc(entries, func(first, second entry) int { return bytes.Compare(first.word, second.word) }) {
			t.Errorf("%d entries not sorted", size)
		}
		lexicon := &Lexicon{entries: entries}
		for _, current := range entries {
			if index := lexicon.search(current.word); index < 0 || !bytes.Equal(entries[index].word, current.word) {
				t.Errorf("%d entries: %s not found", size, current.word)
			}
		}
		if lexicon.search([]byte("Z")) >= 0 {
			t.Errorf("%d entries: Z found", size)
		}
	}
}
