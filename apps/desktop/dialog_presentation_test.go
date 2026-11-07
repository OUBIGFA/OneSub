package main

import (
	"fmt"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// These review captures use the production workbench theme and real dialog
// functions with a finite subtitle sample, not an approximation in HTML.
func TestDialogPresentationReview(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			for _, size := range [][2]int{{900, 640}, {1440, 1000}} {
				for _, dark := range []bool{false, true} {
					a := checkedReviewApp(t)
					r := a.retranslation
					r.rows[0].cue.OriginalText = "Check the timing before exporting the subtitles."
					r.rows[0].cue.TranslationSource = "Check the timing before exporting the subtitles."
					r.rows[0].cue.Text = "在导出之前检查时间。"
					r.rows[0].result = &retranslationResult{text: "导出字幕前，请检查时间轴。", valid: true}
					tt := settled(a.view, size[0], size[1])
					tt.SetPreferences(ui.Preferences{ReduceMotion: true})
					tt.SetDark(dark)
					captureRedesign(t, tt, fmt.Sprintf("review-reader-%s-%d-dark%t", language, size[0], dark))
					insideRedesign(t, tt, locales.RetranslateTitle(), locales.RetranslateCompare(), locales.RetranslateList(), locales.RetranslateApplyOne(), locales.RetranslateApply())
				}
			}
		})
	}
}
