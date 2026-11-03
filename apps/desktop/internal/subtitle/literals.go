package subtitle

import (
	"net/netip"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// textSpan uses UTF-8 byte offsets, like Go slicing and ASR word coverage.
// Literal recognition is shared by cleanup and splitting: neither operation
// may interpret an address, number or identifier's punctuation as prose.
type textSpan struct{ start, end int }

var splitEntity = regexp.MustCompile(`&(?:#[0-9]+|#x[0-9A-Fa-f]+|[A-Za-z][A-Za-z0-9]+);`)

type literalPattern struct {
	expression *regexp.Regexp
	trimEnding bool
	leadingDot bool
	tokenStart bool
}

var literalPatterns = []literalPattern{
	// Unicode/quoted local parts, IDN labels and address literals are shapes,
	// not deliverability checks. No DNS or network lookup is performed.
	{expression: regexp.MustCompile(`(?:[\pL\pN.!#$%&'*+/=?^_` + "`" + `{|}~-]+|"(?:[^"\\\r\n]|\\.)+")[＠@](?:(?:[\pL\pN_-]+[.．])+[\pL\pN_-]+|\[[A-Za-z0-9:._%+-]+\])`)},
	{expression: regexp.MustCompile("(?i)(?:[a-z][a-z0-9+.-]*://|mailto:|www[.．])[^\\s<>\"'`，。！？；：、（）【】《》“”‘’]+"), trimEnding: true},
	{expression: regexp.MustCompile("`[^`\\r\\n]+`")},
	// Keep decimals (also .5), thousands separators, clocks and ratios atomic.
	{expression: regexp.MustCompile(`[+\-]?(?:\pN+(?:[.．,:：/]\pN+)+|[.．]\pN+)(?:[eE][+\-]?\pN+)?[%％°]?`)},
	// Dotted names include domains, filenames and qualified code identifiers.
	// An unspaced dotted name is ambiguous; preserve its literal spelling.
	{expression: regexp.MustCompile(`[\pL\pN_$#]+(?:\.[\pL\pN_$#-]+)+`), leadingDot: true},
	{expression: regexp.MustCompile(`\.[\pL_][\pL\pN_.-]*`), trimEnding: true, tokenStart: true},
	{expression: regexp.MustCompile(`[\p{Latin}\p{Greek}\p{Cyrillic}\pN_$#]+(?:．[\p{Latin}\p{Greek}\p{Cyrillic}\pN_$#-]+)+`)},
	{expression: regexp.MustCompile(`[\pL\pN_#]+(?:[-+][\pL\pN_#+]+)+|[A-Za-z][A-Za-z0-9_]*[+#]{1,2}|[\pL\pN_]+(?:::[\pL\pN_]+)+`)},
	{expression: regexp.MustCompile(`[+\-]?\pN+(?:[.．,:：/]\pN+)*(?:[ \x{00A0}]?)(?:GHz|MHz|kHz|Hz|fps|kg|km|cm|mm|ms|mg|KB|MB|GB|TB|°C|°F|毫米|厘米|千米|公里|小时|小時|分钟|分鐘|公斤|秒|米|年|元|吨|噸|克|个|個|%|％)`)},
	{expression: regexp.MustCompile(`(?:[\pL\pN_~.][\pL\pN_~.#-]*[\\/])+(?:[\pL\pN_.~#-]+)?[\\/]?`), trimEnding: true},
	{expression: regexp.MustCompile(`"[A-Za-z]:[\\/][^"\r\n]+"|'[A-Za-z]:[\\/][^'\r\n]+'`)},
	{expression: regexp.MustCompile(`(?:[A-Za-z]:[\\/]|\\\\|\.{1,2}[\\/]|/)[^\s<>"` + "`" + `，。！？；：、（）【】《》“”‘’]+`), trimEnding: true},
}

var ipv6Tokens = regexp.MustCompile(`[0-9A-Fa-f]*(?::[0-9A-Fa-f]*){2,}(?:%[A-Za-z0-9_.-]+)?`)

// Single-dot titles and conventional abbreviations cannot be inferred from
// dot shape alone ("Dr." and "Go." have the same shape). This semantic list
// supplements, rather than replaces, general multi-dot/initial recognition.
var stopAbbreviations = strings.Fields("mr mrs ms dr prof rev hon sr jr st mt gen col lt capt sgt gov pres sen rep supt etc vs cf ed eds vol vols no nos fig figs eq eqs approx dept inc ltd co corp assoc est misc min max sec jan feb mar apr jun jul aug sep sept oct nov dec")

func literalSpans(text string) []textSpan {
	var spans []textSpan
	for _, pattern := range literalPatterns {
		for _, match := range pattern.expression.FindAllStringIndex(text, -1) {
			start, end := match[0], match[1]
			if pattern.tokenStart && start > 0 {
				previous, _ := utf8.DecodeLastRuneInString(text[:start])
				if previous == '.' || previous == '．' || previous == '_' || (unicode.IsLetter(previous) || unicode.IsDigit(previous)) && !unicode.Is(unicode.Han, previous) {
					continue // an extension or ellipsis is not a hidden filename prefix
				}
			}
			if pattern.trimEnding {
				// A sentence stop after a URL/path is not part of its payload.
				// Balanced URL parentheses are retained; an enclosing ')' is not.
				for end > start {
					r, size := utf8.DecodeLastRuneInString(text[start:end])
					if strings.ContainsRune(".．,!?;:", r) || r == ')' && strings.Count(text[start:end], ")") > strings.Count(text[start:end], "(") || r == ']' && strings.Count(text[start:end], "]") > strings.Count(text[start:end], "[") {
						end -= size
					} else {
						break
					}
				}
			}
			if pattern.leadingDot && start > 0 && text[start-1] == '.' {
				start-- // .env.local, without treating a prose ellipsis as a path.
			}
			if end > start {
				spans = append(spans, textSpan{start, end})
			}
		}
	}

	for _, match := range ipv6Tokens.FindAllStringIndex(text, -1) {
		if _, err := netip.ParseAddr(text[match[0]:match[1]]); err == nil {
			spans = append(spans, textSpan{match[0], match[1]})
		}
	}

	// Examine whole alphabetic dotted tokens. In particular Ph.D., e.g.,
	// U.S.A. and J. K. have a meaningful final dot unlike a filename's stop.
	start := -1
	for i, r := range text {
		if r == '.' || unicode.IsLetter(r) && !unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul, unicode.Thai) {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			if abbreviationToken(text[start:i], text[i:]) {
				spans = append(spans, textSpan{start, i})
			}
			start = -1
		}
	}
	if start >= 0 && abbreviationToken(text[start:], "") {
		spans = append(spans, textSpan{start, len(text)})
	}
	slices.SortFunc(spans, func(a, b textSpan) int {
		if a.start != b.start {
			return a.start - b.start
		}
		return b.end - a.end
	})
	merged := spans[:0]
	for _, span := range spans {
		if len(merged) > 0 && span.start <= merged[len(merged)-1].end {
			merged[len(merged)-1].end = max(merged[len(merged)-1].end, span.end)
		} else {
			merged = append(merged, span)
		}
	}
	return merged
}

func abbreviationToken(token, following string) bool {
	if !strings.HasSuffix(token, ".") || strings.Contains(token, "..") {
		return false
	}
	stem := strings.TrimSuffix(token, ".")
	if slices.Contains(stopAbbreviations, strings.ToLower(stem)) {
		return true
	}
	groups := strings.Split(stem, ".")
	if len(groups) > 1 {
		first := []rune(groups[0])
		if len(first) > 3 || len(first) == 0 || len(first) > 1 && !slices.ContainsFunc(first, unicode.IsUpper) {
			return false
		}
		for i, group := range groups {
			letters := []rune(group)
			if len(letters) == 0 || len(letters) > 3 || !slices.ContainsFunc(letters, unicode.IsLetter) || i > 0 && len(letters) > 1 && !strings.EqualFold(group, "sc") {
				return false // BBC.CO.UK is a domain, unlike U.S.A. or B.Sc.
			}
		}
		return true
	}
	letters := []rune(stem)
	if len(letters) == 1 && unicode.IsUpper(letters[0]) {
		next, _ := utf8.DecodeRuneInString(strings.TrimLeftFunc(following, unicode.IsSpace))
		return unicode.IsUpper(next)
	}
	return false
}

// literalRunes maps byte spans back to visible characters. Formatting tags
// are handled by the caller and do not interrupt a literal's recognition.
func literalRunes(text string) []bool {
	protected := make([]bool, utf8.RuneCountInString(text))
	spans := literalSpans(text)
	span, index := 0, 0
	for offset := range text {
		for span < len(spans) && offset >= spans[span].end {
			span++
		}
		if span < len(spans) && offset >= spans[span].start {
			protected[index] = true
		}
		index++
	}
	return protected
}

func sentencePeriod(chars []rune, index int, literals []bool) bool {
	if literals[index] {
		return false
	}
	r := chars[index]
	if r != '.' && r != '．' && r != '。' {
		return false
	}
	// ASCII/fullwidth dot runs are an ellipsis, not repeated full stops.
	if r != '。' && (index > 0 && (chars[index-1] == '.' || chars[index-1] == '．') || index+1 < len(chars) && (chars[index+1] == '.' || chars[index+1] == '．')) {
		return false
	}
	if r != '。' && index+1 < len(chars) && unicode.IsDigit(chars[index+1]) && (index == 0 || !unicode.IsLetter(chars[index-1])) {
		return false // .5 and .5e-3 remain numeric even without a leading zero.
	}
	return true
}
