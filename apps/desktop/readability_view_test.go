package main

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

func TestReadingToolsBothTracksLocateOutsideSearchAndUpdateDraft(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			for _, track := range []int{0, 1} {
				t.Run(fmt.Sprintf("%s/%d", language, track), func(t *testing.T) {
					a := storeApp(t, subtitle.Cue{ID: "a", Text: "Short", StartMs: 0, EndMs: 2000}, subtitle.Cue{ID: "b", Text: strings.Repeat("内容", 20), StartMs: 3000, EndMs: 15000}, subtitle.Cue{ID: "c", Text: strings.Repeat("说明", 20), StartMs: 16000, EndMs: 28000})
					if _, err := a.store.ApplyTranslation("translate", a.snapshot.Revision, []project.Edit{{ID: "b", Text: strings.Repeat("翻译", 20), StartMs: 3000, EndMs: 15000}}); err != nil {
						t.Fatal(err)
					}
					state, _ := a.store.Snapshot()
					a.setSnapshot(state)
					a.changeSubtitleTab(track)
					a.query = "Short"
					a.position = 1.5
					a.paused = false
					tt := settled(a.subtitlesView, 800, 560)
					if !tt.HasText(locales.ReadingSplitCount(2)) {
						t.Fatal("split target count missing")
					}
					nextLong := func() {
						if err := tt.Click(locales.ReadingInspect()); err != nil {
							t.Fatal(err)
						}
						if err := tt.ChooseMenuItem(locales.ReadingViewLong(2)); err != nil {
							t.Fatal(err)
						}
					}
					nextLong()
					if a.selected != 1 || a.query != "" || a.position != 1.5 || !a.followHeld {
						t.Fatal("locate changed playback or ignored search", a.selected, a.query)
					}
					if tt.HasText(locales.ReadingLong()) {
						t.Fatal("reading hint is still inside the subtitle list")
					}
					a.status = "Previous operation"
					footer := settled(func(c *ui.Context) { a.statusBar(c, studioColors(c)) }, 800, 32)
					// Ownership is part of the displayed text, not a separate label.
					hintLabel := locales.StatusOwnerSubtitle() + locales.ReadingLong()
					statusLabel := locales.StatusOwnerStatus() + a.status
					hint, shown := footer.Find(hintLabel)
					result, resultShown := footer.Find(statusLabel)
					if !shown || !resultShown || hint.X >= result.X {
						t.Fatal("selected reading hint is not first in the status bar", hint, result)
					}
					nextLong()
					if a.selected != 2 {
						t.Fatal("not cycling")
					}
					nextLong()
					if a.selected != 1 {
						t.Fatal("not wrapping")
					}
					captureRedesign(t, tt, fmt.Sprintf("reading-%s-%d", language, track))
					a.selected = 0
					a.loadDraft()
					footer.Frame()
					if footer.HasText(hintLabel) || !footer.HasText(statusLabel) {
						t.Fatal("reading hint stayed after selecting a readable cue")
					}
				})
			}
		})
	}
}

func TestReadingCacheAndOneBatchUndo(t *testing.T) {
	a := storeApp(t, subtitle.Cue{ID: "a", Text: strings.Repeat("内容", 20), StartMs: 0, EndMs: 16000})
	if !a.cueReading(0).Long() {
		t.Fatal("no long issue")
	}
	a.draft = "短句"
	a.start = 0
	a.end = 16
	a.editPending = true
	if a.cueReading(0).Long() {
		t.Fatal("draft did not refresh reading load")
	}
	a.editPending = false
	if _, err := a.store.SplitCues(context.Background(), "split", a.snapshot.Revision, true, subtitle.PunctuationOptions{}); err != nil {
		t.Fatal(err)
	}
	a.refreshAfterStructure(0)
	if len(a.readingLoads()) < 2 || !a.trackUnsaved(0) {
		t.Fatal("split did not update cache/dirty state")
	}
	a.undo(false)
	if len(a.readingLoads()) != 1 || !a.cueReading(0).Long() {
		t.Fatal("undo did not restore reading load")
	}
}

func TestSingleReadingSplitMenuAndCommandBothTracks(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			for _, tab := range []int{0, 1} {
				a := storeApp(t, subtitle.Cue{ID: "short", Text: "Hello wonderful world today.", StartMs: 0, EndMs: 4000}, subtitle.Cue{ID: "long", Text: strings.Repeat("word ", 30), StartMs: 5000, EndMs: 25000})
				if _, err := a.store.ApplyTranslation("translate", 0, []project.Edit{{ID: "short", Text: "早上好大家好晚上好各位好", StartMs: 0, EndMs: 4000}}); err != nil {
					t.Fatal(err)
				}
				state, _ := a.store.Snapshot()
				a.setSnapshot(state)
				a.changeSubtitleTab(tab)
				tt := settled(a.subtitlesView, 900, 500)
				if err := tt.RightClick(cueDisplayText(a.activeDocument().Cues[0], tab)); err != nil {
					t.Fatal(err)
				}
				found := false
				for _, label := range tt.Menu() {
					if label == locales.ReadingSplit() {
						found = true
					}
				}
				if !found {
					t.Fatal("single-cue menu missing")
				}
				tt.CloseMenu()
				apply, err := a.smartSplitWork("short")(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				apply()
				if len(a.activeDocument().Cues) != 3 || a.activeDocument().Cues[2].ID != "long" || !a.trackUnsaved(tab) {
					t.Fatal("single split modified other problems or lost dirty state")
				}
				a.undo(false)
				if len(a.activeDocument().Cues) != 2 {
					t.Fatal("single split was not one undo step")
				}
			}
		})
	}
}

// Explicit splitting always cleans the resulting source/translation pieces,
// independently of automatic translation preferences, in the same undo step.
func TestSmartSplitAutomaticallyCleansBothTextLayersWithOneUndo(t *testing.T) {
	for _, tab := range []int{0, 1} {
		t.Run(fmt.Sprint(tab), func(t *testing.T) {
			a := storeApp(t, subtitle.Cue{ID: "target", Text: "Version 3.14 is ready. Are we done?", StartMs: 0, EndMs: 8000}, subtitle.Cue{ID: "neighbor", Text: "Keep this. Neighbor.", StartMs: 9000, EndMs: 15000})
			if _, err := a.store.ApplyTranslation("translate", 0, []project.Edit{{ID: "target", Text: "版本3.14已经准备完成。还有问题？", StartMs: 0, EndMs: 8000}}); err != nil {
				t.Fatal(err)
			}
			state, _ := a.store.Snapshot()
			a.setSnapshot(state)
			a.changeSubtitleTab(tab)
			a.settings.StripPunctuation, a.settings.ReplacePeriods = false, false
			a.settings.KeepPunctuation = "?"
			before := *a.snapshot
			work := a.smartSplitWork("target")
			a.settings.KeepPunctuation = "!" // The task keeps its startup options.
			apply, err := work(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			apply()
			parts := a.activeDocument().Cues
			if len(parts) != 3 || parts[2].Text != "Keep this. Neighbor." {
				t.Fatal("single split changed a neighboring cue", parts)
			}
			source, translated := "", ""
			for _, cue := range parts[:2] {
				source += subtitle.SourceText(cue) + " "
				translated += cue.Text
				for _, text := range []string{subtitle.SourceText(cue), cue.Text} {
					if strings.ContainsAny(strings.ReplaceAll(text, "3.14", ""), ".。．") || strings.HasSuffix(text, ",") || strings.HasSuffix(text, "，") {
						t.Fatal("split punctuation was not cleaned", text)
					}
				}
			}
			if source != "Version 3.14 is ready Are we done? " {
				t.Fatal("cleanup order or kept question mark changed", source)
			}
			if tab == 1 && translated != "版本3.14已经准备完成还有问题？" {
				t.Fatal("paired translation was not cleaned", translated)
			}
			if !reflect.DeepEqual(before.TrackDocument(1-tab), a.snapshot.TrackDocument(1-tab)) || a.settings.StripPunctuation || a.settings.ReplacePeriods {
				t.Fatal("split changed the other track or translation preferences")
			}
			a.undo(false)
			if !reflect.DeepEqual(before.Document, a.snapshot.Document) || !reflect.DeepEqual(before.SourceDocument, a.snapshot.SourceDocument) {
				t.Fatal("one undo did not restore splitting and punctuation")
			}
		})
	}
}

func BenchmarkReadingToolbar20000(b *testing.B) {
	cues := make([]subtitle.Cue, 20000)
	for i := range cues {
		cues[i] = subtitle.Cue{ID: fmt.Sprint(i), Position: i, Text: "A readable subtitle", TranslationSource: "A readable source", Translated: true, StartMs: int64(i) * 5000, EndMs: int64(i)*5000 + 4000}
	}
	a := NativeApp{snapshot: &project.Snapshot{Document: subtitle.Document{Cues: cues}, SourceDocument: subtitle.Document{Cues: cues}}, subtitleTab: 1, selected: -1}
	tt := ui.NewTester(a.readingTools, 700, 100)
	tt.Frame()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tt.Frame()
	}
}

// The split action counts only long cues. Its neutral face and the separate
// category menu keep automatic splitting distinct from timing review.
func TestReadingToolbarCountMatchesSplitAndCategoryNavigation(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			for _, tab := range []int{0, 1} {
				t.Run(fmt.Sprintf("%s/%d", language, tab), func(t *testing.T) {
					a := storeApp(t,
						subtitle.Cue{ID: "normal", Text: "Short", StartMs: 0, EndMs: 2000},
						subtitle.Cue{ID: "all", Text: strings.Repeat("长", 30), StartMs: 3000, EndMs: 3500},
						subtitle.Cue{ID: "fast", Text: strings.Repeat("字", 10), StartMs: 4000, EndMs: 5000},
						subtitle.Cue{ID: "brief", Text: "字", StartMs: 6000, EndMs: 6500})
					if _, err := a.store.ApplyTranslation("translate", a.snapshot.Revision, []project.Edit{{ID: "all", Text: strings.Repeat("译", 35), StartMs: 3000, EndMs: 3500}}); err != nil {
						t.Fatal(err)
					}
					state, err := a.store.Snapshot()
					if err != nil {
						t.Fatal(err)
					}
					a.setSnapshot(state)
					a.changeSubtitleTab(tab)
					a.query, a.position = "Short", 9.4
					var danger ui.Color
					tt := settled(func(c *ui.Context) {
						danger = c.Theme().Danger
						a.readingTools(c)
					}, 800, 200)
					split, found := tt.Find(locales.ReadingSplitCount(1))
					if !found || tt.HasText(locales.A11yWarningIcon()) {
						t.Fatal("split count is not part of a neutral action", tt.Texts())
					}
					frame := tt.Image()
					for y := int(split.Y); y < int(split.Y+split.H); y++ {
						for x := int(split.X); x < int(split.X+split.W); x++ {
							p := frame.RGBAAt(x, y)
							if p.R == danger.R && p.G == danger.G && p.B == danger.B {
								t.Fatal("split target count is still red")
							}
						}
					}
					if err := tt.Click(locales.ReadingInspect()); err != nil {
						t.Fatal(err)
					}
					if want := []string{locales.ReadingViewLong(1), locales.ReadingViewFast(2), locales.ReadingViewBrief(2)}; !slices.Equal(tt.Menu(), want) {
						t.Fatal("categories do not count paired cues independently", tt.Menu())
					}
					if err := tt.ChooseMenuItem(locales.ReadingViewFast(2)); err != nil {
						t.Fatal(err)
					}
					if a.selected != 1 || a.query != "" || a.position != 9.4 || !a.followHeld {
						t.Fatal("category navigation ignored search or changed the video")
					}
					for _, target := range []struct {
						label string
						index int
					}{{locales.ReadingViewFast(2), 2}, {locales.ReadingViewBrief(2), 3}, {locales.ReadingViewBrief(2), 1}} {
						if err := tt.Click(locales.ReadingInspect()); err != nil {
							t.Fatal(err)
						}
						if err := tt.ChooseMenuItem(target.label); err != nil {
							t.Fatal(err)
						}
						if a.selected != target.index || a.position != 9.4 {
							t.Fatal("category navigation selected another issue type", a.selected)
						}
					}
					captureRedesign(t, tt, fmt.Sprintf("reading-action-%s-%d", language, tab))
				})
			}
		})
	}
}

func TestReadingToolbarDraftCountAndTimingOnlyIssues(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := storeApp(t, subtitle.Cue{ID: "normal", Text: "Short", StartMs: 0, EndMs: 4000})
			tt := settled(a.readingTools, 800, 200)
			if !tt.HasText(locales.ReadingSplitCount(0)) || tt.HasText(locales.A11yWarningIcon()) {
				t.Fatal("zero split targets are not shown without a warning", tt.Texts())
			}
			a.draft = strings.Repeat("长", 30)
			a.changed(tt.Now())
			tt.Frame()
			if !tt.HasText(locales.ReadingSplitCount(1)) || !a.editPending {
				t.Fatal("draft did not immediately update the split target count")
			}
			a.draft, a.end = strings.Repeat("字", 10), 1
			a.changed(tt.Now())
			tt.Frame()
			if !tt.HasText(locales.ReadingSplitCount(0)) {
				t.Fatal("speed-only issue inflated the split count")
			}
			if err := tt.Click(locales.ReadingSplitCount(0)); err != nil || a.busy {
				t.Fatal("zero long cues still started a split task", err)
			}
			if err := tt.Click(locales.ReadingInspect()); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem(locales.ReadingViewLong(0)); err == nil {
				t.Fatal("zero-count category is still enabled")
			}
			if err := tt.ChooseMenuItem(locales.ReadingViewFast(1)); err != nil {
				t.Fatal("speed-only issue cannot be reviewed", err)
			}
			a.draft, a.end = "Short", 4
			a.changed(tt.Now())
			tt.Frame()
			tt.Advance(time.Second)
			if !tt.HasText(locales.ReadingSplitCount(0)) || tt.HasText(locales.A11yWarningIcon()) {
				t.Fatal("cleared draft left a misleading warning")
			}
			if err := tt.Click(locales.ReadingInspect()); err != nil || len(tt.Menu()) != 0 {
				t.Fatal("empty reading categories still open a menu", err)
			}
		})
	}
}

// Large counts are user-facing subtitle counts too; they must not force the
// whole reading group beyond the minimum panel width.
func TestReadingToolbarLargeCountsFitMinimumPane(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			cues := make([]subtitle.Cue, 20000)
			for i := range cues {
				cues[i] = subtitle.Cue{ID: fmt.Sprint(i), Text: strings.Repeat("word ", 15), StartMs: int64(i) * 16000, EndMs: int64(i)*16000 + 15000}
			}
			a := storeApp(t, cues...)
			tt := settled(a.subtitlesView, 320, 600)
			for _, label := range []string{locales.ReadingSplitCount(len(cues)), locales.ReadingInspect()} {
				bounds, found := tt.Find(label)
				if !found || bounds.X < 0 || bounds.X+bounds.W > 320 {
					t.Fatal("large reading count clipped in the minimum pane", language, label, bounds)
				}
			}
		})
	}
}

func TestReadingToolbarNarrowPane(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := storeApp(t, subtitle.Cue{ID: "a", Text: strings.Repeat("word ", 15), StartMs: 0, EndMs: 15000})
			for _, width := range []int{360, 520, 900} {
				tt := settled(a.subtitlesView, width, 600)
				for _, label := range []string{locales.ReadingSplitCount(1), locales.ReadingInspect(), locales.FindReplace(), locales.TextReplacePeriods(), locales.TextStripTrailing(), locales.ListSaveAs()} {
					bounds, ok := tt.Find(label)
					if !ok || bounds.X < 0 || bounds.X+bounds.W > float32(width)+1 {
						t.Fatalf("clipped %q at width %v: %+v", label, width, bounds)
					}
				}
				search, _ := tt.Find(locales.ListSearch())
				replace, _ := tt.Find(locales.FindReplace())
				locate, _ := tt.Find(locales.ListLocate())
				periods, _ := tt.Find(locales.TextReplacePeriods())
				trailing, _ := tt.Find(locales.TextStripTrailing())
				if periods.X <= locate.X+locate.W || trailing.X < periods.X+periods.W || periods.Y != locate.Y || trailing.Y != locate.Y {
					t.Fatal("punctuation is not grouped after navigation", locate, periods, trailing)
				}
				if replace.X < trailing.X+trailing.W || replace.X-trailing.X-trailing.W > 8 || replace.Y != trailing.Y {
					t.Fatal("find/replace is not immediately right of trailing punctuation", trailing, replace)
				}
				if width == 900 && (replace.X+replace.W > search.X || replace.Y > search.Y+search.H) {
					t.Fatal("editing tools are not left of search", replace, search)
				}
			}
		})
	}
}
