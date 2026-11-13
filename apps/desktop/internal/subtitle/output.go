package subtitle

import (
	"errors"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// OutputOptions selects language content independently from ASS visual priority.
// SourceFirst controls reading order, not which language receives the main style.
type OutputOptions struct {
	Mode        string `json:"mode"`
	Primary     string `json:"primary"`
	SourceFirst bool   `json:"sourceFirst"`
	SourceTrack string `json:"sourceTrack,omitempty"` // "original" selects the independent original timeline
	SkipMissing bool   `json:"-"`                     // explicit choice for this output only, never a saved default
}

func (o OutputOptions) Normalized() OutputOptions {
	if o.Mode == "" {
		o.Mode = "source"
	}
	if o.Primary == "" {
		o.Primary = "translation"
	}
	return o
}

// SourceText is the current, editable source track; OriginalText is immutable
// import evidence and must not replace a user's corrected source on export.
func SourceText(c Cue) string {
	if c.Translated {
		return c.TranslationSource
	}
	return c.Text
}

func (c Cue) MissingTranslation() bool {
	return !c.ReadOnly && (!c.Translated || strings.TrimSpace(c.Text) == "")
}

// Positions come from the cue state, never from a localized error message.
func MissingTranslationIndices(doc Document) []int {
	var missing []int
	for i, cue := range doc.Cues {
		if cue.MissingTranslation() {
			missing = append(missing, i)
		}
	}
	return missing
}

func MissingTranslations(doc Document) int {
	return len(MissingTranslationIndices(doc))
}

// SelectOutput copies the frozen document and refuses incomplete translation
// tracks. It never silently fills untranslated cues with source text.
func SelectOutput(doc Document, options OutputOptions) (Document, error) {
	o := options.Normalized()
	if o.Mode != "source" && o.Mode != "translation" && o.Mode != "bilingual" {
		return Document{}, errors.New(locales.ErrOutputMode())
	}
	if o.Primary != "source" && o.Primary != "translation" {
		return Document{}, errors.New(locales.ErrOutputPrimary())
	}
	// Explicit TXT output has no ASS events to preserve. Other outputs keep
	// the strict ASS/SSA rule against creating new bilingual event layouts.
	if o.Mode == "bilingual" && (doc.Format == "ass" || doc.Format == "ssa") && doc.TargetFormat != "txt" {
		return Document{}, errors.New(locales.ErrOutputAssBilingual())
	}
	if o.Mode != "source" {
		if missing := MissingTranslations(doc); missing > 0 && !o.SkipMissing {
			return Document{}, errors.New(locales.ErrOutputMissing(missing))
		}
	}
	result := doc
	result.Cues = make([]Cue, 0, len(doc.Cues))
	for _, cue := range doc.Cues {
		if o.Mode != "source" && o.SkipMissing && cue.MissingTranslation() {
			continue
		}
		if o.Mode != "source" && o.SkipMissing {
			cue.Position = len(result.Cues)
		}
		result.Cues = append(result.Cues, cue)
		i := len(result.Cues) - 1
		if cue.ReadOnly {
			continue
		}
		source := SourceText(cue)
		switch o.Mode {
		case "source":
			result.Cues[i].Text = source
		case "bilingual":
			if o.SourceFirst {
				result.Cues[i].Text = source + "\n" + cue.Text
			} else {
				result.Cues[i].Text = cue.Text + "\n" + source
			}
		}
	}
	return result, nil
}
