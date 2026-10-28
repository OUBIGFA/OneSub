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

func MissingTranslations(doc Document) int {
	missing := 0
	for _, c := range doc.Cues {
		if !c.ReadOnly && (!c.Translated || strings.TrimSpace(c.Text) == "") {
			missing++
		}
	}
	return missing
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
	if o.Mode == "bilingual" && (doc.Format == "ass" || doc.Format == "ssa") {
		return Document{}, errors.New(locales.ErrOutputAssBilingual())
	}
	if o.Mode != "source" {
		if missing := MissingTranslations(doc); missing > 0 {
			return Document{}, errors.New(locales.ErrOutputMissing(missing))
		}
	}
	result := doc
	result.Cues = append([]Cue{}, doc.Cues...)
	for i, cue := range result.Cues {
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
