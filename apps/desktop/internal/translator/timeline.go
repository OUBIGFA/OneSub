package translator

import (
	"context"
	"errors"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// Optimize only within bounded contiguous responsibility regions. The model
// chooses atom ranges and reasons; it never authors timestamps or source text.
func (e *engine) resegment(ctx context.Context, doc subtitle.Document, contextData any) (subtitle.Document, error) {
	atoms, err := subtitle.TimelineAtoms(doc)
	if err != nil {
		return subtitle.Document{}, err
	}
	ranges := []subtitle.SourceRange{}
	for start := 0; start < len(atoms); {
		if atoms[start].Protected {
			ranges = append(ranges, subtitle.SourceRange{FirstAtom: atoms[start].ID, LastAtom: atoms[start].ID})
			start++
			continue
		}
		end, size := start, 0
		for end < len(atoms) && !atoms[end].Protected {
			if end > start && atoms[end].CueID != atoms[end-1].CueID && (end-start >= 160 || size >= 12000) {
				break
			}
			size += len(atoms[end].Text)
			end++
		}
		if size > 24000 {
			return subtitle.Document{}, errors.New(locales.ErrSegmentRegion())
		}
		var proposal struct {
			Segments      []subtitle.SourceRange `json:"segments"`
			Uncertainties []string               `json:"uncertainties"`
		}
		err = e.request(ctx, `First audit seams using the whole context. Do not translate yet. Return {"segments":[{"firstAtom":"exact ID","lastAtom":"exact ID","reason":"specific seam defect or empty for unchanged"}],"uncertainties":[]}. Cover every supplied atom once, in order, in contiguous closed ranges. Preserve useful original cue boundaries; merge only a named seam defect, not topic continuity. Never split an atom, never cross intentional overlap. Word atoms permit interior cuts; cue atoms do not. Include a specific reason for every change or crossed gap. No new source text, drops, timestamps or other keys.`, map[string]any{"operation": "segment", "context": contextData, "atoms": atoms[start:end]}, &proposal)
		if err != nil {
			return subtitle.Document{}, err
		}
		// Each response must stay within its own region before concatenation.
		if len(proposal.Segments) == 0 || proposal.Segments[0].FirstAtom != atoms[start].ID || proposal.Segments[len(proposal.Segments)-1].LastAtom != atoms[end-1].ID {
			return subtitle.Document{}, errors.New(locales.ErrSegmentCoverage())
		}
		ranges = append(ranges, proposal.Segments...)
		e.report.Warnings = append(e.report.Warnings, proposal.Uncertainties...)
		start = end
	}
	result, warnings, err := subtitle.Resegment(doc, ranges)
	e.report.Warnings = append(e.report.Warnings, warnings...)
	if err == nil {
		for _, cue := range result.Cues {
			if cue.SourceRange != nil && cue.SourceRange.Reason != "" {
				e.report.TimelineChanges++
			}
		}
	}
	return result, err
}
