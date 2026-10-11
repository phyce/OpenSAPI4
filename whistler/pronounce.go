package whistler

import (
	"github.com/phyce/OpenSAPI4/whistler/analyser"
	"github.com/phyce/OpenSAPI4/whistler/lexicon"
)

// Pronounce is how the engine pronounces a word, in engine phoneme names (z1ayndaxl), as ILexPronounce::Get
// and ILexPronounce2::GetFrom with ILP2_LTS give it (pronunciation.md §6.5): the word goes through the front
// end on its own, the lexicons included, and its units are named. A word may hold only letters and - ' .
// The front end keeps what a word changes, as it does after a text, so Pronounce must not run while the
// engine speaks.
func (engine *Engine) Pronounce(word string) (string, error) {
	names, err := engine.pronounce(word)
	return string(names), err
}

// PronounceIPA is Pronounce in IPA, as the wide interfaces give it.
func (engine *Engine) PronounceIPA(word string) (string, error) {
	names, err := engine.pronounce(word)
	if err != nil {
		return "", err
	}
	return engine.userLexicon.IPA(names), nil
}

func (engine *Engine) pronounce(word string) ([]byte, error) {
	ansi := engine.userLexicon.ANSI(word)
	if !lexicon.ValidWord(ansi) {
		return nil, lexicon.ErrInvalidWord
	}
	segment := analyser.NewSegment(ansi)
	read := engine.reader.ReadSentence(segment, 0)
	var names []string
	if read.Result != analyser.ReadNothingLeft {
		built, analysed := engine.buildUnits(read, false)
		if !analysed {
			engine.reader.ReadSentence(segment, read.Consumed)
		}
		for _, unit := range built {
			names = append(names, unit.Name)
		}
	}
	return lexicon.ARPAbetToEngineNames(lexicon.UnitsToARPAbet(names)), nil
}
