package lexical

import "github.com/phyce/OpenSAPI4/truvoice/pipeline"

// The dictionary's classes of function words the rules name (lexical.md §6.1).
const (
	pronounClass          = 1
	theClass              = 2
	ofClass               = 3
	prepositionClass      = 4
	shortPrepositionClass = 10
	auxiliaryClass        = 12
	andClass              = 14
)

// The rates above which a function word loses all its stress, `the` and the words of class 3 reduce their vowels,
// and `to`, `into` and `and` reduce theirs (lexical.md §12).
const (
	unstressedRate = 8
	theRate        = 6
	reductionRate  = 5
)

// weakClass reports whether a function word of the class loses its stress: every class of function words but the
// quantifiers, the negations, and `may`, `must` and `might`.
func weakClass(class byte) bool {
	switch class {
	case 1, 2, 3, 4, 5, 9, 10, 12, 13, 14:
		return true
	}
	return false
}

// FunctionWordStress lowers the previous word's stress, a function word's, and changes its sounds by the word
// after it (lexical.md §12.1).
func (stage *Stage) FunctionWordStress() {
	list := stage.Pipeline
	rate := stage.cursor().Rate
	word, next := stage.PreviousWord, stage.Word
	end := list.Next(stage.PreviousEnd)
	var nextSymbol byte
	var nextStress uint32
	var nextClass, lastVowel byte
	beforeBreak, keepsStress, beforeTo := false, false, false
	vowels, nodes := 0, 0
	if next != nil {
		if !stage.hasClass(next.Symbol(), phraseBreak) {
			last, count := next, 0
			for node := next; node != list.Next(stage.LastNode); node = list.Next(node) {
				last = node
				count++
				if stress := stressOf(node); stress != 0 {
					nextStress = stress
				}
			}
			beforeTo = count == 3 && last.Symbol() == 'b' && last.Previous.Symbol() == 'T'
			nextClass = next.Second()
			if nextClass == pronounClass || nextClass == shortPrepositionClass || nextClass == ofClass {
				nextStress = 0
			}
			if weakClass(nextClass) && nextClass != prepositionClass && nextStress == 0 && stage.hasClass(last.Next.Symbol(), phraseBreak) {
				keepsStress = true
				for node := word; node != end; node = list.Next(node) {
					if node.Symbol() == '@' || node.Symbol() == '|' {
						keepsStress = false
					}
				}
			}
		} else {
			beforeBreak = true
			for node := word; node != end; node = list.Next(node) {
				if stage.hasClass(node.Symbol(), vowel) {
					vowels++
					lastVowel = node.Symbol()
				}
			}
		}
		nextSymbol, nextStress = next.Symbol(), stressOf(next)
		if !stage.hasClass(nextSymbol, phoneme) {
			if after := list.Next(next); after != nil {
				nextSymbol, nextStress = after.Symbol(), stressOf(after)
			}
		}
	}
	class := word.Second()
	if beforeBreak {
		if vowels < 2 && (class == shortPrepositionClass && lastVowel != 'v' && lastVowel != 'f' && lastVowel != 'w' || class == ofClass || class == pronounClass) {
			for node := word; node != end; node = list.Next(node) {
				if stress := stressOf(node); stress == 1 || stress == 2 {
					setStress(node, 0)
				}
			}
		}
		return
	}
	if weakClass(class) && !keepsStress {
		for node := word; node != end; node = list.Next(node) {
			nodes++
			if class == prepositionClass {
				continue
			}
			stress := stressOf(node)
			if stress == 1 || stress == 2 {
				stress--
			}
			if rate > unstressedRate {
				stress = 0
			}
			setStress(node, stress)
		}
	}
	first := list.Next(word)
	if class == auxiliaryClass && nextClass == shortPrepositionClass {
		last := next.Previous
		if last.Symbol() == 'Z' && last.Previous.Symbol() == 'a' && next.Next.Symbol() == 'T' && next.Next.Next.Symbol() == 'b' {
			last.SetSymbol('S')
		}
	}
	beforeWord := first.Previous.Previous.Symbol()
	reduces := rate > reductionRate && (!stage.hasClass(nextSymbol, vowel) || nextStress > 0 && stage.hasClass(nextSymbol, rounded)) &&
		beforeWord != '.' && beforeWord != ',' && beforeWord != 0
	if class == theClass || class == shortPrepositionClass {
		second := list.Next(first)
		switch {
		case first.Symbol() == 'x':
			if !stage.hasClass(nextSymbol, vowel) && rate > theRate {
				second.SetSymbol('@')
				setStress(second, 0)
			}
		case first.Symbol() == 'T' && second.Symbol() == 'b' && reduces:
			second.SetSymbol('u')
			setStress(second, 0)
		}
		return
	}
	first = word.Next
	if first.Symbol() == 'Y' && nodes == 5 && first.Next.Symbol() == 'b' && first.Next.Next.Symbol() == 'Z' &&
		first.Next.Next.Next.Symbol() == 'D' && beforeTo {
		first.Next.Next.SetSymbol('S')
		first.Next.Next.Next.SetSymbol('T')
		return
	}
	if first.Symbol() == 'i' && nodes == 5 && first.Next.Symbol() == 'N' && first.Next.Next.Symbol() == 'T' &&
		first.Next.Next.Next.Symbol() == 'b' && reduces {
		first.Next.Next.Next.SetSymbol('@')
	}
}

// FunctionWordReductions reduces the previous word's vowels, a function word's, and an unstressed `v` and the
// `e` of `them` after it (lexical.md §12.2).
func (stage *Stage) FunctionWordReductions() {
	list := stage.Pipeline
	rate := stage.cursor().Rate
	class := stage.PreviousWord.Second()
	if stage.Word == nil || !stage.hasClass(stage.Word.Symbol(), phraseBreak) {
		first := list.Next(stage.PreviousWord)
		switch {
		case class == ofClass && rate > theRate:
			for node := first; node != nil && node.Type() == pipeline.Phoneme && !isWordMark(node.Symbol()); node = list.Next(node) {
				if stage.hasClass(node.Symbol(), vowel) && node.Symbol() != 'g' && stressOf(node) == 0 {
					node.SetSymbol('@')
				}
			}
		case class == andClass:
			beforeWord := first.Previous.Previous.Symbol()
			ends := beforeWord == '.' || beforeWord == ',' || beforeWord == 0
			if first.Symbol() == 'a' && stage.PreviousEnd.Symbol() == 'D' && rate > reductionRate {
				if !ends && beforeWord != '@' {
					first.SetSymbol('@')
					setStress(first, 0)
					stage.PreviousEnd = list.FreeNode(stage.PreviousEnd, false)
				}
			} else if first.Symbol() == 'F' && rate > reductionRate && !ends {
				second := list.Next(first)
				second.SetSymbol('@')
				setStress(second, 0)
			}
		}
	}
	for node := stage.PreviousWord; node != nil; node = list.Next(node) {
		symbol := node.Symbol()
		unstressed := stressOf(node) == 0
		if !weakClass(class) && symbol == 'v' && unstressed && !stage.hasClass(node.Previous.Previous.Symbol(), phraseBreak) &&
			node.Previous.Symbol() != ' ' && !isWordMark(node.Previous.Symbol()) {
			node.SetSymbol('@')
			continue
		}
		if symbol == 'e' && unstressed && node.Previous.Symbol() == 'x' && node.Next.Symbol() == 'M' {
			mark := node.Previous.Previous
			if isWordMark(mark.Symbol()) && mark.Previous != nil && mark.Previous.Symbol() != ' ' && mark.Previous.Symbol() != '.' {
				node.SetSymbol('@')
			}
		}
	}
}
