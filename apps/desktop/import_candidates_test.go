package main

import (
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/importing"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// Candidate filenames may be long user content, but score labels are UI
// text and must remain whole on both localized native dialogs.
func TestImportCandidateScoresRemainVisible(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := nativeForTest(t)
			a.candidatesOpen = true
			a.pending = &importing.Loaded{Selection: importing.Selection{Candidates: []importing.Candidate{
				{Path: "Introduction to Blender Materials.srt", Score: 91, Reason: "similar"},
				{Path: "Introduction for Blender Materials.vtt", Score: 88, Reason: "similar"},
			}}}
			tt := settled(a.candidatesView, 900, 640)
			for _, score := range []int{91, 88} {
				label := locales.CandidatesScore(score)
				rect, found := tt.Find(label)
				if !found || rect.W <= 0 || rect.H <= 0 || rect.X < 0 || rect.X+rect.W > 900 || rect.Y < 0 || rect.Y+rect.H > 640 {
					t.Fatal("candidate score is hidden or clipped", language, label, rect)
				}
			}
			visibleRedesign(t, tt, locales.CandidatesTitle(), locales.CandidatesLater())
			captureRedesign(t, tt, "candidates-"+language)
		})
	}
}
