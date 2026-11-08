package subtitle

import (
	"context"
	"slices"
)

// SegmentRecognition runs the local linguistic splitter before publishing ASR
// output. Unlike a manual SmartSplit, it never estimates a missing word clock.
// Short utterances and cues without exact coverage keep their engine boundary.
// Punctuation and text are preserved; no translation or network service runs.
func SegmentRecognition(ctx context.Context, doc Document) (Document, error) {
	out := doc
	out.Cues = make([]Cue, 0, len(doc.Cues))
	for _, cue := range doc.Cues {
		if err := ctx.Err(); err != nil {
			return Document{}, err
		}
		parts := []Cue{cue}
		if InspectCueReading(cue).Long() {
			if times, _ := wordBoundaries(cue, SourceText(cue)); times != nil {
				split, _, err := splitCue(ctx, cue, doc.Format, "recognition", true, false)
				if err != nil {
					return Document{}, err
				}
				// Multi-line lanes can require estimated projection in the manual
				// editor even with a source clock. Automatic output must not use it.
				if len(split) > 1 && !slices.ContainsFunc(split, func(c Cue) bool { return c.TimingQuality != "aligned" }) {
					parts = split
				}
			}
		}
		for _, part := range parts {
			part.Position = len(out.Cues)
			out.Cues = append(out.Cues, part)
		}
	}
	return out, nil
}
