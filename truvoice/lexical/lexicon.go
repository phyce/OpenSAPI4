package lexical

import "github.com/phyce/OpenSAPI4/truvoice/pipeline"

// A Lexicon is the user lexicon: the words an application gave pronunciations, in capitals.
type Lexicon interface {
	// Pronunciation returns the word's pronunciation, its phonemes each followed by its stress digit when it has
	// one, and whether the lexicon has the word.
	Pronunciation(word string) (string, bool)
}

// lexiconLetters is the most symbols of a stem the user lexicon looks up.
const lexiconLetters = 40

// UserLexiconLookup looks up the stem between the cursor's working nodes in the user lexicon, and replaces its
// letters with the pronunciation it finds. It reports whether it found the stem (lexical.md §7).
func (stage *Stage) UserLexiconLookup() bool {
	if stage.Lexicon == nil {
		return false
	}
	cursor := stage.cursor()
	first, last := cursor.Working[0], cursor.Working[1]
	toWordEnd := stage.LastNode == last
	fromWordStart := stage.FirstLetter == first
	var letters []byte
	for node := first; node != last.Next; node = node.Next {
		if len(letters) == lexiconLetters {
			return false
		}
		letters = append(letters, node.Symbol())
	}
	pronunciation, found := stage.Lexicon.Pronunciation(string(letters))
	if !found {
		return false
	}
	node := stage.freeStem()
	primary := false
	var full, weak *pipeline.Node
	for index := range len(pronunciation) {
		symbol := pronunciation[index]
		if symbol == '0' || symbol == '1' || symbol == '2' {
			continue
		}
		node = stage.Pipeline.NewNode(node, true, pipeline.Phoneme, symbol)
		var digit byte
		if index+1 < len(pronunciation) {
			digit = pronunciation[index+1]
		}
		switch digit {
		case '1':
			primary = true
			node.SetFlags(node.Flags()&^stressBits | stage.primaryStress()<<stressShift)
		case '2':
			node.SetFlags(node.Flags()&^stressBits | 1<<stressShift)
		case '0':
		default:
			if !primary && full == nil && stage.hasClass(symbol, syllabic) {
				if !stage.hasClass(symbol, reduced) {
					full = node
				} else if weak == nil {
					weak = node
				}
			}
		}
	}
	stage.finishStem(node, fromWordStart, toWordEnd)
	if !primary {
		stage.stressVowel(full, weak)
	}
	return true
}
