package subtitle

import (
	"slices"
	"strings"
	"unicode"
)

// Remove presentation indentation only after all split offsets are resolved.
// The immutable OriginalText retains the exact source slices. Word evidence
// follows the displayed source text, with its original clock left untouched.
func trimSplitPadding(cue Cue, format string) Cue {
	source := SourceText(cue)
	trimmed := trimSplitLines(source, format)
	if trimmed != source && len(cue.Words) > 0 {
		var evidence strings.Builder
		for _, word := range cue.Words {
			evidence.WriteString(word.Text)
		}
		if evidence.String() == source {
			cue.Words = slices.Clone(cue.Words)
			// Aligned splitting only has one plain source lane; its leading
			// padding belongs to the first nonempty word, never its timestamp.
			cue.Words[0].Text = strings.TrimLeftFunc(cue.Words[0].Text, unicode.IsSpace)
		}
	}
	if cue.Translated {
		cue.TranslationSource = trimmed
	}
	cue.Text = trimSplitLines(cue.Text, format)
	return cue
}

func trimSplitLines(text, format string) string {
	separator := "\n"
	if format == "ass" || format == "ssa" {
		separator = `\N`
	}
	lines := strings.Split(text, separator)
	for i := range lines {
		lines[i] = strings.TrimLeftFunc(lines[i], unicode.IsSpace)
	}
	return strings.Join(lines, separator)
}
