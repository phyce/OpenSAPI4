package truvoice

import "github.com/phyce/OpenSAPI4/truvoice/text"

// pronounce runs a text's \Prn=\ command when the normalizer reaches it (engine.md §7.3): through the ANSI
// interface, a record without a pronunciation removes its word, and any other adds it, in text or, after //, in
// engine phonemes. What they return is ignored, so a word the lexicon has keeps its pronunciation. A number no
// record has does nothing.
func (engine *Engine) pronounce(number uint32) {
	record, found := engine.converter.TakePronunciation(number)
	if !found {
		return
	}
	if record.Pronunciation == "" {
		engine.data.userLexicon.RemoveANSI([]byte(record.Word))
		return
	}
	characterSet := text.Text
	if record.Phonetic {
		characterSet = text.EnginePhonetic
	}
	engine.data.userLexicon.AddANSI(characterSet, []byte(record.Word), []byte(record.Pronunciation))
}
