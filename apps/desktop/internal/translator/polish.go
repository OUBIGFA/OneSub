package translator

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// polish applies the target profile's mechanical rules, the ones the
// skill's checker tests: where final_punctuation is "none", no line ends
// in ordinary stopping punctuation (a question mark stays); where spacing
// is on, a half-width space separates Chinese from Latin letters and
// digits. Markup is left as it is, and the wording stays the model's.
func polish(text string, l language) string {
	lines := []string{}
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		if l.FinalPunctuation == "none" {
			line = trimFinalPunctuation(line)
		}
		if l.Spacing {
			line = spaceLatin(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// disallowedFinal is the checker's DISALLOWED_TRAILING_PUNCT.
const disallowedFinal = "。．.;；:：、,，！!"

// trailingTags are the markup tags that close a line, which punctuation
// before them still ends.
var trailingTags = regexp.MustCompile(`(?:<[^>\n]+>|\{[^}\n]*\})+$`)

func trimFinalPunctuation(line string) string {
	closing := trailingTags.FindString(line)
	body := strings.TrimSuffix(line, closing)
	trimmed := strings.TrimRightFunc(body, func(r rune) bool {
		return strings.ContainsRune(disallowedFinal, r) || r == ' '
	})
	if trimmed == "" {
		return line // punctuation alone is left for the checker to name
	}
	return trimmed + closing
}

// spaceLatin puts one space between a CJK character and a Latin letter or
// digit next to it, outside markup tags: the checker's missing-space rule.
func spaceLatin(line string) string {
	var b strings.Builder
	var previous rune
	inTag := byte(0)
	for i, r := range line {
		switch {
		case inTag != 0:
			if byte(r) == inTag {
				inTag = 0
			}
			b.WriteRune(r)
			previous = 0
			continue
		case r == '<' || (r == '{' && strings.HasPrefix(line[i:], "{\\")):
			inTag = map[rune]byte{'<': '>', '{': '}'}[r]
			b.WriteRune(r)
			previous = 0
			continue
		}
		if previous != 0 && ((isCJK(previous) && isLatin(r)) || (isLatin(previous) && isCJK(r))) {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
		previous = r
	}
	return b.String()
}

func isLatin(r rune) bool {
	return r < utf8.RuneSelf && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9')
}
