package lexicon

import "bytes"

// shortRun is the most entries sortEntries sorts by selection.
const shortRun = 8

// sortEntries sorts the entries by their words, byte for byte, as the C runtime's qsort the original calls does
// (engine.md §7.1): a quicksort on the median of three, by selection for 8 entries or fewer. Words a file gives
// twice end up in the order it leaves them.
func sortEntries(entries []entry) {
	if len(entries) < 2 {
		return
	}
	compare := func(first, second int) int { return bytes.Compare(entries[first].word, entries[second].word) }
	swap := func(first, second int) { entries[first], entries[second] = entries[second], entries[first] }
	type span struct{ low, high int }
	var pending []span
	low, high := 0, len(entries)-1
	for {
		if size := high - low + 1; size <= shortRun {
			for ; high > low; high-- {
				largest := low
				for index := low + 1; index <= high; index++ {
					if compare(index, largest) > 0 {
						largest = index
					}
				}
				swap(largest, high)
			}
		} else {
			middle := low + size/2
			if compare(low, middle) > 0 {
				swap(low, middle)
			}
			if compare(low, high) > 0 {
				swap(low, high)
			}
			if compare(middle, high) > 0 {
				swap(middle, high)
			}
			lower, upper := low, high
			for {
				if middle > lower {
					for lower++; lower < middle && compare(lower, middle) <= 0; lower++ {
					}
				}
				if middle <= lower {
					for lower++; lower <= high && compare(lower, middle) <= 0; lower++ {
					}
				}
				for upper--; upper > middle && compare(upper, middle) > 0; upper-- {
				}
				if upper < lower {
					break
				}
				swap(lower, upper)
				if middle == upper {
					middle = lower
				}
			}
			upper++
			if middle < upper {
				for upper--; upper > middle && compare(upper, middle) == 0; upper-- {
				}
			}
			if middle >= upper {
				for upper--; upper > low && compare(upper, middle) == 0; upper-- {
				}
			}
			if upper-low >= high-lower {
				if low < upper {
					pending = append(pending, span{low, upper})
				}
				if lower < high {
					low = lower
					continue
				}
			} else {
				if lower < high {
					pending = append(pending, span{lower, high})
				}
				if low < upper {
					high = upper
					continue
				}
			}
		}
		if len(pending) == 0 {
			return
		}
		low, high = pending[len(pending)-1].low, pending[len(pending)-1].high
		pending = pending[:len(pending)-1]
	}
}

// search finds a word, byte for byte, as the C runtime's bsearch the original calls does, and returns its index, or
// -1. Of a word the lexicon has twice, it finds the one its halving reaches first.
func (lexicon *Lexicon) search(word []byte) int {
	low, count := 0, len(lexicon.entries)
	for count > 0 {
		half := count / 2
		if half == 0 {
			if bytes.Equal(word, lexicon.entries[low].word) {
				return low
			}
			return -1
		}
		before := half - 1
		if count&1 != 0 {
			before = half
		}
		middle := low + before
		order := bytes.Compare(word, lexicon.entries[middle].word)
		switch {
		case order == 0:
			return middle
		case order < 0:
			count = before
		default:
			low, count = middle+1, half
		}
	}
	return -1
}
