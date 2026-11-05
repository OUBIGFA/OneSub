package translator

import (
	"context"
	"errors"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// TranslateSelected produces review candidates, never a project mutation. Locks
// are relaxed only in this isolated input; accepting a candidate is a separate
// explicit user edit. Every unselected cue in the returned document is intact.
func TranslateSelected(ctx context.Context, config Config, document subtitle.Document, ids []string) (Result, error) {
	selected := make(map[string]bool, len(ids))
	for _, id := range ids {
		selected[id] = true
	}
	if len(selected) == 0 {
		return Result{}, errors.New(locales.ErrNothingToTranslate())
	}
	include := make([]bool, len(document.Cues))
	found := 0
	for i, cue := range document.Cues {
		if !selected[cue.ID] {
			continue
		}
		if cue.ReadOnly {
			return Result{}, errors.New(locales.ErrEditProtected())
		}
		found++
		for j := max(0, i-neighbors); j <= min(len(include)-1, i+neighbors); j++ {
			include[j] = true
		}
	}
	if found != len(selected) {
		return Result{}, errors.New(locales.ErrCuePosition())
	}
	input := document
	input.Cues = nil
	for i, cue := range document.Cues {
		if !include[i] || cue.ReadOnly {
			continue
		}
		cue.Text = subtitle.SourceText(cue)
		cue.TextLocked = !selected[cue.ID]
		input.Cues = append(input.Cues, cue)
	}
	// An explicit new attempt must not silently replay an old paid answer or
	// rebuild the user's timing. Context is bounded before entering the engine.
	config.OptimizeTimeline, config.CacheDir, config.Resume = false, "", nil
	result, err := Translate(ctx, config, input)
	if err != nil {
		return result, err
	}
	answers := map[string]string{}
	for _, cue := range result.Document.Cues {
		if selected[cue.ID] && cue.Translated {
			answers[cue.ID] = cue.Text
		}
	}
	if len(answers) != len(selected) {
		return result, errors.New(locales.ErrTranslateIncomplete(len(selected) - len(answers)))
	}
	result.Source, result.Document = document, document
	result.Document.Cues = append([]subtitle.Cue{}, document.Cues...)
	for i, cue := range document.Cues {
		if text, ok := answers[cue.ID]; ok {
			// Keep the real IDs, word evidence, locks and original provenance,
			// not the temporary scope mask or Resegment's identity ranges.
			result.Document.Cues[i].TranslationSource = subtitle.SourceText(cue)
			result.Document.Cues[i].Translated = true
			result.Document.Cues[i].Text = text
		}
	}
	result.Resume = nil
	return result, nil
}

// CheckSelected validates the full native document before checking only the
// selected dialogue. Protected drawings remain in strict ASS projections: the
// worker must not interpret their omission as permission to delete them.
func (c Checker) CheckSelected(ctx context.Context, source, candidate subtitle.Document, ids []string, language string) (Findings, error) {
	if _, err := c.Subtitle.Write(ctx, candidate); err != nil {
		return Findings{}, err
	}
	selected := map[string]bool{}
	for _, id := range ids {
		selected[id] = true
	}
	project := func(doc subtitle.Document, original bool) subtitle.Document {
		out := doc
		out.Cues = nil
		for _, cue := range doc.Cues {
			if !selected[cue.ID] && !cue.ReadOnly {
				continue
			}
			if original {
				cue.Text = subtitle.SourceText(cue)
			}
			cue.Position = len(out.Cues)
			out.Cues = append(out.Cues, cue)
		}
		return out
	}
	return c.Check(ctx, project(source, true), project(candidate, false), language, true)
}
