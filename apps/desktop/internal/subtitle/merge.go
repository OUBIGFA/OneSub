package subtitle

import (
	"errors"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// MergeAdjacent joins two document neighbors, not two filtered table rows.
// Text lines stay in order; a comma joins the two boundaries, and the
// envelope of both cues is the new time span.
// New identity lets strict ASS/VTT serialization replace the original blocks
// without treating a deliberate merge as an illegal edit to one old event.
func MergeAdjacent(doc Document, at int, id string) (Document, error) {
	if at < 0 || at+1 >= len(doc.Cues) {
		return Document{}, errors.New(locales.ErrCuePosition())
	}
	first, second := doc.Cues[at], doc.Cues[at+1]
	if first.ReadOnly || second.ReadOnly {
		return Document{}, errors.New(locales.ErrEditProtected())
	}
	if first.Translated != second.Translated {
		return Document{}, errors.New(locales.ErrMergeTranslation())
	}
	join := func(a, b string) string { return joinMergeText(a, b, doc.Format == "ass" || doc.Format == "ssa") }
	merged := Cue{
		ID: id, Position: at, StartMs: min(first.StartMs, second.StartMs), EndMs: max(first.EndMs, second.EndMs),
		Text: join(first.Text, second.Text), OriginalText: join(first.OriginalText, second.OriginalText),
		Translated: first.Translated, TextLocked: first.TextLocked || second.TextLocked || first.Translated,
		TimeLocked: true, TimingQuality: "manual",
	}
	if first.Translated {
		merged.TranslationSource = join(SourceText(first), SourceText(second))
	}
	if merged.StartMs < 0 || merged.EndMs <= merged.StartMs || merged.EndMs > 9007199254740991 {
		return Document{}, errors.New(locales.ErrEditTiming())
	}
	if len(merged.Text) > 1024*1024 || len(merged.TranslationSource) > 1024*1024 {
		return Document{}, errors.New(locales.ErrEditText())
	}
	// Preserve source provenance and flatten repeated manual merges. Word
	// anchors and AI segmentation ranges describe the old revision, not this
	// hand-edited span, and are deliberately not carried into the new cue.
	for _, cue := range []Cue{first, second} {
		for _, pair := range []struct {
			from []string
			to   *[]string
		}{
			{cue.SourceCueIDs, &merged.SourceCueIDs}, {cue.MergedCueIDs, &merged.MergedCueIDs},
		} {
			ids := pair.from
			if len(ids) == 0 && pair.to == &merged.MergedCueIDs {
				ids = cue.SplitCueIDs
			}
			if len(ids) == 0 {
				ids = []string{cue.ID}
			}
			for _, sourceID := range ids {
				if !slices.Contains(*pair.to, sourceID) {
					*pair.to = append(*pair.to, sourceID)
				}
			}
		}
	}
	out := doc
	out.Cues = append([]Cue{}, doc.Cues[:at]...)
	out.Cues = append(out.Cues, merged)
	out.Cues = append(out.Cues, doc.Cues[at+2:]...)
	for i := range out.Cues {
		out.Cues[i].Position = i
	}
	return out, nil
}

// Boundary marks may sit inside closing/opening tags or quotes. Normalize
// only periods and commas on either side; question/exclamation meaning stays.
var mergeClosingSuffix = regexp.MustCompile(`(?:<[^>\n]+>|\{\\[^}\n]*\}|[\p{Pe}\p{Pf}"'\s])*$`)
var mergeOpeningPrefix = regexp.MustCompile(`^(?:<[^>\n]+>|\{\\[^}\n]*\}|[\p{Ps}\p{Pi}"'\s])*`)

func joinMergeText(first, second string, ass bool) string {
	first, second = strings.TrimSpace(first), strings.TrimSpace(second)
	suffix := mergeClosingSuffix.FindString(first)
	body := strings.TrimRight(strings.TrimSuffix(first, suffix), ".。．,，")
	first = body + suffix
	prefix := mergeOpeningPrefix.FindString(second)
	second = prefix + strings.TrimLeft(strings.TrimPrefix(second, prefix), ".。．,， \t\r\n")
	// Choose typography from the visible join boundary, independently for
	// source and translation, not from the app's interface language.
	comma := ", "
	left := []rune(punctuationMarkup.ReplaceAllString(first, ""))
	right := []rune(punctuationMarkup.ReplaceAllString(second, ""))
	boundaryCJK := func(chars []rune, backward bool) bool {
		for i := range chars {
			if backward {
				i = len(chars) - 1 - i
			}
			r := chars[i]
			if unicode.IsSpace(r) || unicode.IsPunct(r) {
				continue
			}
			return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
		}
		return false
	}
	if boundaryCJK(left, true) || boundaryCJK(right, false) {
		comma = "，"
	}
	reset := ""
	if ass && strings.Contains(first, `{\`) {
		// Static overrides on the first event must not bleed into the
		// second. The writer rejects incompatible/timed ASS effects.
		reset = `{\r}`
	}
	return first + comma + reset + second
}
