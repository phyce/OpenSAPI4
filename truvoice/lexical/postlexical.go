package lexical

import "github.com/phyce/OpenSAPI4/truvoice/pipeline"

// The dictionary's classes the word filter counts besides content words (lexical.md §4.1): verbs, negations and
// adverbs; the code of a word that keeps its form, and the command flag that applies the post-lexical rules in
// phonetic input too.
const (
	verbClass     = 6
	negationClass = 8
	adverbClass   = 0xf
	codeKept      = 13
	rulesFlag     = 0x100
)

// PostLexicalWord gives the previous word its post-lexical pass, once the word after it is converted, and
// returns the node after it (lexical.md §4).
func (stage *Stage) PostLexicalWord() *pipeline.Node {
	list := stage.Pipeline
	cursor := stage.cursor()
	word := stage.PreviousWord
	if word.Type() != pipeline.Phoneme || stage.hasClass(word.Symbol(), phraseBreak) {
		return list.Next(word)
	}
	if filter := cursor.Filter; filter != 0 {
		dropped := true
		if class := word.Second(); class == 0 || class == verbClass || class == adverbClass || class == negationClass {
			stage.Filtered = (stage.Filtered + 1) % filter
			dropped = stage.Filtered != 0
		}
		if dropped {
			if stage.PreviousWord != stage.PreviousEnd {
				for {
					stage.PreviousWord = list.FreeNode(stage.PreviousWord, true)
					if stage.PreviousWord == stage.PreviousEnd {
						break
					}
				}
			}
			return list.FreeNode(stage.PreviousWord, true)
		}
	}
	if word.Symbol() == '%' && word.Value() != codeKept {
		stage.FunctionWordStress()
	}
	symbol := stage.PreviousWord.Symbol()
	if symbol == '%' || stage.PreviousWord.Second() == adverbClass {
		stage.copyWordClass()
	}
	end := list.Next(stage.PreviousEnd)
	stage.passNodes(symbol, end)
	syllable := byte(0)
	for node := stage.PreviousWord; node != end && node != nil; {
		if stage.hasClass(node.Symbol(), vowel) {
			syllable++
			node.Data[syllableOffset] = syllable
		}
		next := list.Next(node)
		if node.Symbol() == '[' {
			list.FreeNode(node, false)
		}
		node = next
	}
	if stage.PreviousWord.Symbol() == '%' && stage.PreviousWord.Value() != codeKept {
		stage.FunctionWordReductions()
	}
	stage.PreviousSymbol = int32(int8(end.Previous.Symbol()))
	return end
}

// copyWordClass gives the previous word's vowels its word class.
func (stage *Stage) copyWordClass() {
	list := stage.Pipeline
	for node := stage.PreviousWord; node != list.Next(stage.PreviousEnd); node = list.Next(node) {
		if stage.hasClass(node.Symbol(), vowel) {
			node.Data[wordClassOffset] = stage.PreviousWord.Second()
		}
	}
}

func isWordMark(symbol byte) bool {
	return symbol == '&' || symbol == '%'
}

// passNodes applies the post-lexical rules to the previous word's phonemes, makes a schwa before a front vowel a
// barred i, a barred i before a consonant and l or j a schwa, and marks a function word's stressed onsets
// (lexical.md §4.2 and §4.3).
func (stage *Stage) passNodes(wordSymbol byte, end *pipeline.Node) {
	list := stage.Pipeline
	cursor := stage.cursor()
	var schwa *pipeline.Node
	consonants, words, joined := 0, 0, false
	for node := stage.PreviousWord; node != end; node = list.Next(node) {
		if node == nil {
			break
		}
		if node.Type() == pipeline.Phoneme && stage.hasClass(node.Symbol(), phoneme) &&
			(cursor.Flags&rulesFlag != 0 || cursor.PhoneticInput == 0) && stage.PreviousWord.Value() != codeKept {
			node = stage.PostLexicalRules(node)
		}
		if stage.Alternating {
			if symbol := node.Symbol(); stage.hasClass(symbol, vowel) {
				switch {
				case !stage.hasClass(symbol, front) || symbol == 'a':
					if symbol != '@' {
						stage.Alternating = false
					}
				case schwa == nil:
					if stage.Word != nil && stage.hasClass(stage.Word.Symbol(), phraseBreak) {
						joined = true
					}
					stage.alternateBack(node, &consonants, &words, &joined)
					stage.Alternating = false
				case schwa.Symbol() == '@' && !isWordMark(schwa.Previous.Symbol()):
					after := schwa.Next
					if after.Symbol() == '[' {
						after = after.Next
					}
					if !stage.hasClass(after.Symbol(), consonant) || !stage.hasClass(after.Next.Symbol(), consonant) {
						schwa.SetSymbol('|')
					}
					stage.Alternating = false
				default:
					stage.Alternating = false
				}
			}
		}
		if !stage.Alternating && node.Symbol() == '@' {
			before := node.Previous.Previous
			if before.Previous != nil && !stage.hasClass(before.Symbol(), phraseBreak) && !stage.hasClass(node.Previous.Symbol(), phraseBreak) {
				stage.Alternating = true
				schwa = node
			}
		}
		if node.Symbol() == '|' && stage.hasClass(node.Next.Symbol(), consonant) {
			if after := node.Next.Next.Symbol(); after == 'l' || after == 'j' {
				node.SetSymbol('@')
			}
		}
		if wordSymbol == '%' && node.Flags()&stressBits != 0 {
			stage.markStressedOnset(node)
		}
	}
}

// alternateBack walks back from a front vowel when the schwa that started the alternation is in an earlier
// word: each schwa before a vowel, with at most one consonant between, becomes a barred i, until a vowel that
// isn't a schwa or a phrase break (lexical.md §4.3).
func (stage *Stage) alternateBack(node *pipeline.Node, consonants, words *int, joined *bool) {
	for walker := node.Previous; walker != nil; walker = walker.Previous {
		symbol := walker.Symbol()
		if stage.hasClass(symbol, phraseBreak) {
			return
		}
		switch {
		case isWordMark(symbol):
			*words++
			if *words > 1 {
				*joined = false
			}
		case !stage.hasClass(symbol, vowel):
			if stage.hasClass(symbol, consonant) {
				*consonants++
			}
		case symbol == '@':
			if *joined && *words == 1 {
				*joined = false
				*consonants = 0
				break
			}
			after := walker.Next.Next
			if !(after.Symbol() == ' ' && after.Next.Symbol() == '|') && *consonants <= 1 && !isWordMark(walker.Next.Symbol()) &&
				stage.hasClass(after.Symbol(), vowel) && !isWordMark(walker.Previous.Symbol()) {
				walker.SetSymbol('|')
			}
			*consonants = 0
		default:
			stage.Alternating = false
			*consonants = 0
		}
		if !stage.Alternating {
			return
		}
	}
}
