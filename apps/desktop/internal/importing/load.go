package importing

import (
	"context"
	"errors"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/media"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

type Loaded struct {
	Selection Selection         `json:"selection"`
	Document  subtitle.Document `json:"document"`
	Media     media.Info        `json:"media"`
	Warnings  []string          `json:"warnings"`
}

// Load is the common parser path for drops and native file dialogs. The explicit
// input must be valid; failure of only an automatic companion is recoverable.
func Load(ctx context.Context, selection Selection, worker subtitle.Client, ffprobe string) (Loaded, error) {
	result := Loaded{Selection: selection, Warnings: []string{}, Document: subtitle.Document{Cues: []subtitle.Cue{}}}
	if selection.Subtitle != "" {
		doc, err := worker.Read(ctx, selection.Subtitle)
		if err != nil {
			if selection.Automatic != "subtitle" {
				return result, err
			}
			result.Warnings = append(result.Warnings, locales.WarnSiblingSubtitle(err.Error()))
			result.Selection.Subtitle, result.Selection.Automatic = "", ""
		} else {
			result.Document = doc
		}
	}
	if selection.Media != "" {
		info, err := media.Probe(ctx, ffprobe, selection.Media)
		if err == nil && selection.Automatic == "video" && !info.HasVideo() {
			err = errors.New(locales.ErrSiblingNoVideo())
		}
		if err != nil {
			if selection.Automatic != "video" {
				return result, err
			}
			result.Warnings = append(result.Warnings, locales.WarnSiblingVideo(err.Error()))
			result.Selection.Media, result.Selection.Automatic = "", ""
		} else {
			result.Media = info
		}
	}
	return result, nil
}
