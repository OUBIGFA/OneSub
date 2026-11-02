package subtitle

import (
	"regexp"
	"strings"
	"unicode"
)

// PunctuationOptions is deterministic translation-output cleanup. It never
// changes source tracks, timing, markup, locked cues, or a provider request.
type PunctuationOptions struct {
	StripTrailing  bool   `json:"stripTrailing"`
	Keep           string `json:"keep"`
	ReplacePeriods bool   `json:"replacePeriods"`
}

var punctuationMarkup = regexp.MustCompile(`<[^>\n]+>|\{\\[^}\n]*\}|\\[Nnh]`)
var pairedMarks = map[rune]rune{'?': '？', '!': '！', ',': '，', '.': '。', ';': '；', ':': '：', '？': '?', '！': '!', '，': ',', '。': '.', '；': ';', '：': ':'}

// Apply treats both real and ASS line breaks as line endings. Tags (including
// decimal parameters and URLs) remain byte-for-byte intact. "All periods"
// intentionally includes decimal/abbreviation dots in visible text.
func (p PunctuationOptions) Apply(text string) string {
	if !p.StripTrailing && !p.ReplacePeriods {
		return text
	}
	kept := map[rune]bool{}
	for _, r := range p.Keep {
		kept[r] = true
		if pair, ok := pairedMarks[r]; ok {
			kept[pair] = true
		}
	}
	var out strings.Builder
	var chars []rune
	var protected []bool
	flush := func() {
		// Match the previous trailing-cleanup behavior without trimming
		// protected markup or the line breaks themselves.
		if p.StripTrailing {
			for len(chars) > 0 && !protected[len(chars)-1] && unicode.IsSpace(chars[len(chars)-1]) {
				chars, protected = chars[:len(chars)-1], protected[:len(protected)-1]
			}
		}
		visible := []int{}
		for i, r := range chars {
			if !protected[i] && !unicode.IsSpace(r) {
				visible = append(visible, i)
			}
		}
		// Closing quotes/brackets do not make a preceding stop internal.
		end := len(visible)
		for end > 0 && closingPunctuation(chars[visible[end-1]]) {
			end--
		}
		start := end
		for start > 0 && unicode.IsPunct(chars[visible[start-1]]) && !closingPunctuation(chars[visible[start-1]]) {
			start--
		}
		trailing := len(chars)
		if start < end {
			trailing = visible[start]
		}
		remove := map[int]bool{}
		if p.StripTrailing && start > 0 {
			for j := end - 1; j >= start; j-- {
				i := visible[j]
				if kept[chars[i]] {
					break
				}
				remove[i] = true
			}
		}
		for i, r := range chars {
			if remove[i] {
				continue
			}
			if p.ReplacePeriods && !protected[i] && i < trailing {
				switch r {
				case '.', '．':
					r = ','
				case '。':
					r = '，'
				}
			}
			out.WriteRune(r)
		}
		chars, protected = nil, nil
	}
	appendText := func(s string, markup bool) {
		for _, r := range s {
			if r == '\n' {
				flush()
				out.WriteRune(r)
				continue
			}
			chars = append(chars, r)
			protected = append(protected, markup)
		}
	}
	at := 0
	for _, span := range punctuationMarkup.FindAllStringIndex(text, -1) {
		appendText(text[at:span[0]], false)
		tag := text[span[0]:span[1]]
		if tag == `\N` || tag == `\n` {
			flush()
			out.WriteString(tag)
		} else {
			appendText(tag, true)
		}
		at = span[1]
	}
	appendText(text[at:], false)
	flush()
	return out.String()
}

func closingPunctuation(r rune) bool {
	return unicode.In(r, unicode.Pe, unicode.Pf) || r == '"' || r == '\''
}
