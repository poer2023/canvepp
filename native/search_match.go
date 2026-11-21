package main

import (
	"strings"
	"sync"
	"unicode"

	"github.com/mozillazg/go-pinyin"
	"golang.org/x/text/unicode/norm"
)

// Compile the input once per filter pass. Normalization is for comparison only.
type libraryQuery []string

func parseLibraryQuery(raw string) libraryQuery {
	raw = strings.ToLower(norm.NFKC.String(raw))
	var terms libraryQuery
	for _, word := range strings.Fields(raw) {
		start := 0
		var last rune
		for i, r := range word {
			if i > 0 && unicode.IsLetter(last) && unicode.IsLetter(r) && unicode.Is(unicode.Han, last) != unicode.Is(unicode.Han, r) {
				terms = append(terms, word[start:i])
				start = i
			}
			last = r
		}
		terms = append(terms, word[start:])
	}
	return terms
}
func (q libraryQuery) matches(raw string) bool {
	if len(q) == 0 {
		return true
	}
	text := strings.ToLower(norm.NFKC.String(raw))
	for _, term := range q {
		if strings.Contains(text, term) || reversedWords(text, term) {
			continue
		}
		// A single Latin key should only match a literal character.
		if len(term) < 2 || strings.IndexFunc(term, func(r rune) bool { return r < 'a' || r > 'z' }) >= 0 || !matchesReading(text, term) {
			return false
		}
	}
	return true
}

// Adjacent Han words often appear in the opposite order in filenames.
func reversedWords(text, term string) bool {
	if len(term) > 192 || strings.IndexFunc(term, func(r rune) bool { return !unicode.Is(unicode.Han, r) }) >= 0 {
		return false
	}
	chars := []rune(term)
	for i := 2; i <= len(chars)-2; i++ {
		first, second := string(chars[:i]), string(chars[i:])
		at := strings.Index(text, second)
		if at >= 0 && strings.Contains(text[at+len(second):], first) {
			return true
		}
	}
	return false
}

var readings sync.Map // Only Han codepoints from the finite dictionary are cached.
var readingOptions = pinyin.NewArgs()

func reading(r rune) string {
	if v, ok := readings.Load(r); ok {
		return v.(string)
	}
	if syllables := pinyin.SinglePinyin(r, readingOptions); len(syllables) > 0 {
		readings.Store(r, syllables[0])
		return syllables[0]
	}
	return ""
}

// A reachable offset is a prefix of the query ending on a syllable boundary.
// Full syllables and initials can be mixed, without generating all spellings.
func matchesReading(text, term string) bool {
	var active, next []int
	for _, r := range text {
		if !unicode.Is(unicode.Han, r) {
			active = active[:0]
			continue
		}
		syllable := reading(r)
		if syllable == "" {
			active = active[:0]
			continue
		}
		active = append(active, 0)
		next = next[:0]
		for _, at := range active {
			if term[at] != syllable[0] {
				continue
			}
			if at+1 == len(term) {
				return true
			}
			next = appendOffset(next, at+1)
			if len(syllable) > 1 && strings.HasPrefix(term[at:], syllable) {
				if at+len(syllable) == len(term) {
					return true
				}
				next = appendOffset(next, at+len(syllable))
			}
		}
		active, next = next, active[:0]
	}
	return false
}
func appendOffset(offsets []int, n int) []int {
	for _, v := range offsets {
		if v == n {
			return offsets
		}
	}
	return append(offsets, n)
}
