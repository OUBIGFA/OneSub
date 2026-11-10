package project

import (
	"database/sql"
	"errors"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/google/uuid"
)

// ImportProofreading replaces only the chosen language, pairing by row order.
// The source owns shared timing; a translation-only row uses its own timing.
// One history entry makes the whole import reversible, including unmatched rows.
func (s *Store) ImportProofreading(commandID string, expected int64, doc subtitle.Document, source bool) (int64, error) {
	return s.mutate(commandID, expected, func(tx *sql.Tx, cursor int64) (int64, error) {
		if len(doc.Cues) == 0 {
			return cursor, errors.New(locales.ErrSubtitleEmpty())
		}
		before, err := orderedCues(tx, "cues")
		if err != nil {
			return cursor, err
		}
		after := make([]subtitle.Cue, 0, max(len(before), len(doc.Cues)))
		for i := 0; i < max(len(before), len(doc.Cues)); i++ {
			var cue subtitle.Cue
			if i < len(before) {
				cue = before[i]
			} else {
				cue.ID = "proof-" + uuid.NewString()
				if len(before) == 0 && i < len(doc.Cues) && doc.Cues[i].ID != "" {
					cue.ID = doc.Cues[i].ID
				}
			}
			if cue.ReadOnly {
				return cursor, errors.New(locales.ErrEditProtected())
			}
			original, translation := subtitle.SourceText(cue), ""
			if cue.Translated {
				translation = cue.Text
			}
			incoming := ""
			if i < len(doc.Cues) {
				item := doc.Cues[i]
				if item.ReadOnly {
					return cursor, errors.New(locales.ErrEditProtected())
				}
				if strings.TrimSpace(item.Text) == "" || len(item.Text) > 1024*1024 {
					return cursor, errors.New(locales.ErrEditText())
				}
				if item.StartMs < 0 || item.EndMs <= item.StartMs || item.EndMs > maxSafeTime {
					return cursor, errors.New(locales.ErrEditTiming())
				}
				incoming = item.Text
				if source || original == "" {
					cue.StartMs, cue.EndMs = item.StartMs, item.EndMs
				}
			}
			if source {
				original = incoming
			} else {
				translation = incoming
			}
			if original == "" && translation == "" {
				continue
			}
			cue.Position = len(after)
			cue.Text, cue.Translated, cue.TranslationSource = original, translation != "", ""
			if cue.Translated {
				cue.Text, cue.TranslationSource = translation, original
			}
			// An absent source must stay absent, including legacy display fallbacks.
			cue.OriginalText = original
			cue.Words, cue.SourceRange = nil, nil
			cue.TextLocked, cue.TimeLocked = cue.Translated, true
			after = append(after, cue)
		}
		return recordTrackHistory(tx, cursor, before, after, "working")
	})
}
