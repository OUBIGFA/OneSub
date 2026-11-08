package subtitle

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// SegmentRecognition runs the local linguistic splitter before publishing ASR
// output. Unlike a manual SmartSplit, it never estimates a missing word clock.
// Short utterances and cues without exact coverage keep their engine boundary.
// Punctuation and text are preserved; no translation or network service runs.
func SegmentRecognition(ctx context.Context, doc Document) (Document, error) {
	out := doc
	out.Cues = make([]Cue, 0, len(doc.Cues))
	for index := 0; index < len(doc.Cues); index++ {
		cue := doc.Cues[index]
		if err := ctx.Err(); err != nil {
			return Document{}, err
		}
		parts := []Cue{cue}
		if times, _ := wordBoundaries(cue, SourceText(cue)); times != nil && !cue.ReadOnly && !cue.Translated {
			lanes, _ := splitLanes(cue.Text, doc.Format)
			if len(lanes) == 1 {
				// Audio chunk edges are not sentence boundaries. Rejoin only
				// contiguous, fully timed prose before choosing punctuation cuts.
				for index+1 < len(doc.Cues) {
					joined, ok := joinRecognitionContinuation(cue, doc.Cues[index+1], doc.Format)
					if !ok {
						break
					}
					cue = joined
					index++
				}
				// Sentence boundaries are structural, not a reward that balancing
				// the entire ASR chunk can outweigh. Only long sentences continue
				// through clause punctuation and finally the lexical splitter.
				parts = recognitionPunctuationParts(cue, recognitionSplitText(cue.Text), true)
				var formatted []Cue
				for _, sentence := range parts {
					clauses := []Cue{sentence}
					lane := recognitionSplitText(sentence.Text)
					if recognitionLong(lane) {
						clauses = recognitionPunctuationParts(sentence, lane, false)
					}
					for _, clause := range clauses {
						pieces := []Cue{clause}
						lane := recognitionSplitText(clause.Text)
						if recognitionLong(lane) {
							split, _, err := splitCueLanes(ctx, clause, doc.Format, "recognition", true, false, []splitText{lane})
							if err != nil {
								return Document{}, err
							}
							if len(split) > 1 && !slices.ContainsFunc(split, func(c Cue) bool { return c.TimingQuality != "aligned" }) {
								pieces = split
							}
						}
						formatted = append(formatted, pieces...)
					}
				}
				parts = formatted
			}
		}
		for _, part := range parts {
			part.Position = len(out.Cues)
			out.Cues = append(out.Cues, part)
		}
	}
	return out, nil
}

// Automatic captions prefer a complete sentence within two display lines.
// These are layout units (wide CJK glyphs count as one), not timing estimates.
func recognitionSplitText(text string) splitText {
	lane := newSplitText(text)
	lane.limit, lane.ceiling = 72, 84
	if lane.language == splitChinese || lane.language == splitJapanese || lane.language == splitKorean {
		lane.limit, lane.ceiling = 26, 32
	}
	return lane
}

func recognitionLong(lane splitText) bool {
	// Source slices retain padding to preserve the transcript and word clock.
	// That invisible edge padding must not push a full sentence over the limit.
	if trimmed := strings.TrimSpace(lane.text); trimmed != lane.text {
		return newSplitText(trimmed).units > lane.ceiling
	}
	return lane.units > lane.ceiling
}

func joinRecognitionContinuation(before, next Cue, format string) (Cue, bool) {
	end := strings.TrimRightFunc(before.Text, func(r rune) bool { return unicode.IsSpace(r) || closingPunctuation(r) })
	last, _ := utf8.DecodeLastRuneInString(end)
	if end == "" || strings.ContainsRune(".!?。！？…؟", last) {
		return before, false
	}
	gap := next.StartMs - before.EndMs
	if gap < 0 || gap > 750 || next.ReadOnly || next.Translated {
		return before, false
	}
	if times, _ := wordBoundaries(next, next.Text); times == nil {
		return before, false
	}
	lanes, _ := splitLanes(next.Text, format)
	if len(lanes) != 1 || next.Words[0].StartMs-before.Words[len(before.Words)-1].EndMs > 750 {
		return before, false
	}
	separator := ""
	if spaced(before.Text, next.Text) {
		separator = " "
	}
	joined := before
	joined.Text += separator + next.Text
	// Keep the existing solver's finite capacity for unpunctuated speech.
	if len(newSplitText(joined.Text).boundaries) > maximumSplitBounds {
		return before, false
	}
	joined.OriginalText, joined.EndMs = joined.Text, next.EndMs
	joined.Words = append(slices.Clone(before.Words), next.Words...)
	joined.Words[len(before.Words)].Text = separator + joined.Words[len(before.Words)].Text
	joined.TimingQuality = "aligned"
	return joined, true
}

// Keep short introductions with their clause and never strand an untimed or
// tiny tail. All boundaries already obey the shared word/literal/quote masks.
// A missing punctuation clock is not replaced by a character-ratio estimate.
func recognitionPunctuationParts(cue Cue, text splitText, sentences bool) []Cue {
	times, wordAt := wordBoundaries(cue, cue.Text)
	previous := text.boundaries[0]
	last := text.boundaries[len(text.boundaries)-1]
	var parts []Cue
	appendPart := func(next textBoundary) {
		part := cue
		part.ID = fmt.Sprintf("punct-%s-%d", cue.ID, len(parts))
		part.Text = cue.Text[previous.offset:next.offset]
		part.OriginalText = part.Text
		part.StartMs, part.EndMs = times[previous.offset], times[next.offset]
		part.Words = slices.Clone(cue.Words[wordAt[previous.offset]:wordAt[next.offset]])
		part.TimingQuality = "aligned"
		parts = append(parts, part)
		previous = next
	}
	for _, boundary := range text.boundaries[1 : len(text.boundaries)-1] {
		punctuated := boundary.penalty <= -15
		if sentences {
			punctuated = boundary.penalty <= -60 || recognitionInitialSentenceEnd(cue.Text, boundary.offset)
		}
		if !punctuated {
			continue
		}
		at, measured := times[boundary.offset]
		if !measured || at-times[previous.offset] < minimumSplitDuration || times[last.offset]-at < minimumSplitDuration {
			continue
		}
		// A full stop already supplies sentence evidence: "So this is it."
		// is complete despite failing the lexical fragment-length heuristic.
		if !sentences && (!text.enoughContent(previous, boundary) || !text.enoughContent(boundary, last)) {
			continue
		}
		appendPart(boundary)
	}
	if len(parts) == 0 {
		return []Cue{cue}
	}
	appendPart(last)
	return parts
}

// The generic literal masker conservatively treats "H. Today" like a name
// initial. In ASR prose, an explicit sentence introducer is stronger evidence
// for the full stop. Ordinary initials such as "J. Smith" remain protected.
func recognitionInitialSentenceEnd(text string, offset int) bool {
	left, right := strings.Fields(text[:offset]), strings.Fields(text[offset:])
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	token := []rune(left[len(left)-1])
	if len(token) != 2 || !unicode.IsUpper(token[0]) || token[1] != '.' {
		return false
	}
	return slices.Contains([]string{"Today", "Tomorrow", "Yesterday", "Now", "Next", "First", "Finally", "However", "Meanwhile", "Therefore", "We", "They", "You", "These", "Those"}, right[0])
}
