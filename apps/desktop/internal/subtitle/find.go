package subtitle

import (
	"regexp"
	"strings"
)

// LiteralFinder searches visible text runs, not ASS/VTT tags or attributes.
// Quoting the input makes punctuation literal; Unicode case folding preserves
// offsets into the original UTF-8 string (lowercasing would not always do so).
type LiteralFinder struct{ expression *regexp.Regexp }

type TextMatch struct{ Start, End int }

func NewLiteralFinder(query string, matchCase bool) LiteralFinder {
	if query == "" {
		return LiteralFinder{}
	}
	pattern := regexp.QuoteMeta(query)
	if !matchCase {
		pattern = "(?i)" + pattern
	}
	return LiteralFinder{regexp.MustCompile(pattern)}
}

func (f LiteralFinder) Matches(text string) []TextMatch {
	if f.expression == nil {
		return nil
	}
	matches := []TextMatch{}
	at := 0
	collect := func(start, end int) {
		for _, span := range f.expression.FindAllStringIndex(text[start:end], -1) {
			matches = append(matches, TextMatch{start + span[0], start + span[1]})
		}
	}
	for _, tag := range punctuationMarkup.FindAllStringIndex(text, -1) {
		collect(at, tag[0])
		at = tag[1]
	}
	collect(at, len(text))
	return matches
}

func (f LiteralFinder) ReplaceAll(text, replacement string) (string, int) {
	matches := f.Matches(text)
	if len(matches) == 0 {
		return text, 0
	}
	var out strings.Builder
	at := 0
	for _, m := range matches {
		out.WriteString(text[at:m.Start])
		out.WriteString(replacement)
		at = m.End
	}
	out.WriteString(text[at:])
	return out.String(), len(matches)
}
