package contracts

import (
	"slices"
	"strings"
	"unicode"
)

// SearchRanges returns merged UTF-8 byte ranges in the original text. Case
// folding may change encoded lengths, so offsets are mapped through runes.
func SearchRanges(text, query string) []MatchRange {
	var lowered []rune
	var offsets []int
	for offset, r := range text {
		offsets = append(offsets, offset)
		lowered = append(lowered, unicode.ToLower(r))
	}
	offsets = append(offsets, len(text))
	var matches []MatchRange
	for _, word := range strings.Fields(query) {
		term := []rune(strings.Map(unicode.ToLower, word))
		if len(term) == 0 {
			continue
		}
		for i := 0; i+len(term) <= len(lowered); i++ {
			if slices.Equal(lowered[i:i+len(term)], term) {
				matches = append(matches, MatchRange{Start: offsets[i], End: offsets[i+len(term)]})
				i += len(term) - 1
			}
		}
	}
	slices.SortFunc(matches, func(a, b MatchRange) int {
		if a.Start != b.Start {
			return a.Start - b.Start
		}
		return a.End - b.End
	})
	var merged []MatchRange
	for _, match := range matches {
		if len(merged) > 0 && merged[len(merged)-1].End >= match.Start {
			merged[len(merged)-1].End = max(merged[len(merged)-1].End, match.End)
		} else {
			merged = append(merged, match)
		}
	}
	return merged
}
