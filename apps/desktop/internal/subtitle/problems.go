package subtitle

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// CueProblem describes an observable text/timing problem without rewriting the
// cue. Track is source or translation; Other is the position of an overlapping cue.
type CueProblem struct {
	Kind, Track, Characters string
	Other                   int
}

// BlocksCompose separates corrupt, absent or unrenderable content from reading
// advice. Overlap may be intentional; it remains available for manual review.
func (p CueProblem) BlocksCompose() bool {
	switch p.Kind {
	case "characters", "glyphs", "empty", "missing", "time":
		return true
	}
	return false
}

// InspectProblems shares the existing reading thresholds and the style
// worker's missing-glyph report. Valid combining marks, emoji joiners, bidi
// controls and private-use font characters are not guessed to be corruption.
func InspectProblems(doc Document, expectTranslation bool, missingGlyphs []string) [][]CueProblem {
	missing := make(map[rune]bool, len(missingGlyphs))
	for _, text := range missingGlyphs {
		for _, r := range text {
			missing[r] = true
		}
	}
	rows := make([][]CueProblem, len(doc.Cues))
	latest := -1
	for i, cue := range doc.Cues {
		if cue.ReadOnly {
			continue // Protected ASS drawings/comments are not editable dialogue.
		}
		validTime := cue.StartMs >= 0 && cue.EndMs > cue.StartMs
		if !validTime {
			rows[i] = append(rows[i], CueProblem{Kind: "time"})
		} else if doc.Format != "ass" && doc.Format != "ssa" {
			// Compare with the furthest previous end, so a long overlapping
			// subtitle is not hidden by a short intervening one.
			if latest >= 0 && cue.StartMs < doc.Cues[latest].EndMs {
				rows[i] = append(rows[i], CueProblem{Kind: "overlap", Other: doc.Cues[latest].Position + 1})
				rows[latest] = append(rows[latest], CueProblem{Kind: "overlap", Other: cue.Position + 1})
			}
			if latest < 0 || cue.EndMs > doc.Cues[latest].EndMs {
				latest = i
			}
		}
		texts := []struct{ track, text string }{{"source", SourceText(cue)}}
		if cue.Translated {
			texts = append(texts, struct{ track, text string }{"translation", cue.Text})
		} else if expectTranslation {
			rows[i] = append(rows[i], CueProblem{Kind: "missing", Track: "translation"})
		}
		for _, value := range texts {
			text := visibleReadingText(value.text)
			if strings.TrimSpace(text) == "" {
				rows[i] = append(rows[i], CueProblem{Kind: "empty", Track: value.track})
				continue
			}
			var invalid, unavailable []rune
			for _, r := range text {
				bad := r == utf8.RuneError || (!unicode.IsGraphic(r) && !unicode.IsSpace(r) && !unicode.In(r, unicode.Cf, unicode.Co))
				if bad && !slices.Contains(invalid, r) {
					invalid = append(invalid, r)
				} else if !bad && missing[r] && !slices.Contains(unavailable, r) {
					unavailable = append(unavailable, r)
				}
			}
			if len(invalid) > 0 {
				rows[i] = append(rows[i], CueProblem{Kind: "characters", Track: value.track, Characters: string(invalid)})
			}
			if len(unavailable) > 0 {
				rows[i] = append(rows[i], CueProblem{Kind: "glyphs", Track: value.track, Characters: string(unavailable)})
			}
			if !validTime {
				continue // Speed/duration warnings would repeat the invalid timing.
			}
			load := InspectReading(value.text, cue.EndMs-cue.StartMs)
			for _, check := range []struct {
				kind string
				on   bool
			}{{"long", load.TooLong}, {"fast", load.TooFast}, {"brief", load.TooBrief}} {
				if check.on {
					rows[i] = append(rows[i], CueProblem{Kind: check.kind, Track: value.track})
				}
			}
		}
	}
	return rows
}
