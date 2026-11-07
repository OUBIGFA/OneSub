package main

import (
	"fmt"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// Empty tracks keep import and track navigation, without presenting disabled
// editing commands as useful next actions. Existing source data stays intact.
func TestSubtitleEmptyTrackShowsImportAndTrackNavigation(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		for _, state := range []string{"workbench", "empty-project", "optimized-mirror"} {
			for _, width := range []int{320, 640} {
				t.Run(fmt.Sprintf("%s/%s/%d", language, state, width), func(t *testing.T) {
					inLocale(t, language, func() {
						a := nativeForTest(t)
						switch state {
						case "empty-project":
							a = storeApp(t)
						case "optimized-mirror":
							a = storeApp(t, subtitle.Cue{ID: "a", Text: "Original subtitle", StartMs: 0, EndMs: 3000})
							a.changeSubtitleTab(1)
						}
						tt := settled(a.subtitlesView, width, 800)
						tools := []string{locales.SubtitleActionsMenu(), locales.ListLocate(), locales.TextReplacePeriods(), locales.TextStripTrailing(), locales.FindReplace(), locales.ListSearch(), locales.ReadingSplitCount(0), locales.ReadingIssueType(), locales.ReadingPrevious(), locales.ReadingNext(), locales.TranslationResults(), locales.ListSave(), locales.ListSaveAs()}
						for _, label := range []string{locales.ListImport(), locales.A11ySubtitleTabs()} {
							r, ok := tt.Find(label)
							if !ok || r.X < 0 || r.X+r.W > float32(width) || r.H <= 0 {
								t.Fatal("empty-track navigation clipped", label, r)
							}
						}
						for _, label := range tools {
							if _, ok := tt.Find(label); ok {
								t.Fatal("empty track presents an unavailable edit", label)
							}
						}
						captureRedesign(t, tt, "empty-toolbar-"+language+"-"+state)
					})
				})
			}
		}
	}
}

// Once subtitles exist, all edits are available through the compact menu.
func TestSubtitleToolbarFindReplaceBecomesAvailable(t *testing.T) {
	a := storeApp(t, subtitle.Cue{ID: "a", Text: "First subtitle", StartMs: 0, EndMs: 3000})
	tt := settled(a.subtitlesView, 640, 800)
	if err := tt.Click(locales.SubtitleActionsMenu()); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(locales.FindReplace()); err != nil {
		t.Fatal(err)
	}
	if !a.findReplace.open {
		t.Fatal("Find and replace stayed disabled after loading subtitles")
	}
	// Translation checks stay visible and activate only when checks exist.
	a.translationCandidate = &translationCandidate{}
	tt.Frame()
	if err := tt.Click(locales.TranslationResults()); err != nil || !a.translationReview {
		t.Fatal("translation checks stayed disabled after results arrived", err)
	}
}
