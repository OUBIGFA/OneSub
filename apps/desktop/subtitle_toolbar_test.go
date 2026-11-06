package main

import (
	"fmt"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// Empty workbenches, empty projects and an optimized track that is only a
// storage mirror all keep the full toolbar. No unavailable action takes focus
// or opens a menu/dialog; importing remains available to populate the list.
func TestSubtitleToolbarKeepsEmptyTrackToolsVisibleAndDisabled(t *testing.T) {
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
						insideRedesign(t, tt, locales.A11ySubtitleActions(), append([]string{locales.ListImport()}, tools...)...)
						followPending := a.followPending
						for _, label := range tools {
							if err := tt.Click(label); err != nil {
								t.Fatal(err)
							}
							if tt.Focused(label) || tt.Menu() != nil || a.findReplace.open || a.translationReview || a.busy || a.insertOpen || a.followPending != followPending {
								t.Fatal("empty-track tool was active", label, tt.Menu())
							}
						}
						captureRedesign(t, tt, "empty-toolbar-"+language+"-"+state)
					})
				})
			}
		}
	}
}

// Once subtitles exist, the same visible Find and replace action operates
// normally. Hiding the commands is not an acceptable disabled state.
func TestSubtitleToolbarFindReplaceBecomesAvailable(t *testing.T) {
	a := storeApp(t, subtitle.Cue{ID: "a", Text: "First subtitle", StartMs: 0, EndMs: 3000})
	tt := settled(a.subtitlesView, 640, 800)
	if err := tt.Click(locales.FindReplace()); err != nil {
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
