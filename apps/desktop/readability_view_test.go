package main

import (
	"context"
	"fmt"
	"math"
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
					if !tt.HasText(locales.ReadingLongCues()) {
						t.Fatal("default issue type is not shown on the picker")
					}
					nextLong := func() {
						if err := tt.Click(locales.ReadingNext()); err != nil {
							t.Fatal(err)
						}
					}
					nextLong()
					if a.selected != 1 || a.query != "" || a.position != 1.5 || !a.followHeld {
						t.Fatal("locate changed playback or ignored search", a.selected, a.query)
					}
					if slices.Contains(tt.Texts(), locales.ReadingLong()) {
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
				a := storeApp(t, subtitle.Cue{ID: "short", Text: "Please check every single file. Then run the whole program.", StartMs: 0, EndMs: 4000}, subtitle.Cue{ID: "long", Text: strings.Repeat("word ", 30), StartMs: 5000, EndMs: 25000})
				if _, err := a.store.ApplyTranslation("translate", 0, []project.Edit{{ID: "short", Text: "早上好大家好。晚上好各位好。", StartMs: 0, EndMs: 4000}}); err != nil {
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
				apply, err := a.smartSplitWork("short", false)(context.Background())
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

func TestSingleReadingSplitKeepsCompactCueWithoutAnEditBothTracks(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			for _, tab := range []int{0, 1} {
				a := storeApp(t, subtitle.Cue{ID: "short", Text: "为什么公司几乎立即就道歉？", StartMs: 0, EndMs: 4000}, subtitle.Cue{ID: "long", Text: strings.Repeat("word ", 30), StartMs: 5000, EndMs: 25000})
				if _, err := a.store.ApplyTranslation("translate", 0, []project.Edit{{ID: "short", Text: "你能不能具体讲讲发生了什么？", StartMs: 0, EndMs: 4000}}); err != nil {
					t.Fatal(err)
				}
				before, _ := a.store.Snapshot()
				a.setSnapshot(before)
				a.changeSubtitleTab(tab)
				a.markSaved(0, "original.srt")
				a.markSaved(1, "translated.srt")
				apply, err := a.smartSplitWork("short", false)(context.Background())
				if err != nil || apply == nil {
					t.Fatal("a no-op split was reported as an error", err)
				}
				apply()
				after, _ := a.store.Snapshot()
				if !reflect.DeepEqual(before, after) || a.trackUnsaved(tab) || a.status != locales.ReadingReadable() {
					t.Fatal("a compact cue created an edit or lost its no-split explanation", tab, a.status)
				}
				footer := settled(func(c *ui.Context) { a.statusBar(c, studioColors(c)) }, 640, 48)
				if !footer.HasText(locales.StatusOwnerStatus() + locales.ReadingReadable()) {
					t.Fatal("no-split explanation did not reach the native status bar", footer.Texts())
				}
			}
		})
	}
}

// A forced split cuts a cue that already reads well, on a word boundary and
// inside the original time, as one undo step. The normal command leaves it.
func TestForcedSplitCutsReadableCueOnce(t *testing.T) {
	a := storeApp(t, subtitle.Cue{ID: "short", Text: "The quick brown fox jumps over the lazy dog", StartMs: 0, EndMs: 6000})
	a.changeSubtitleTab(0)
	plain, err := a.smartSplitWork("short", false)(context.Background())
	if err != nil || plain == nil {
		t.Fatal(err)
	}
	plain()
	if len(a.activeDocument().Cues) != 1 {
		t.Fatal("the normal split changed a readable cue")
	}
	apply, err := a.smartSplitWork("short", true)(context.Background())
	if err != nil || apply == nil {
		t.Fatal(err)
	}
	apply()
	cues := a.activeDocument().Cues
	if len(cues) != 2 || cues[0].StartMs != 0 || cues[1].EndMs != 6000 || cues[0].EndMs != cues[1].StartMs {
		t.Fatal("forced split did not produce two joined cues", cues)
	}
	if strings.Join(strings.Fields(cues[0].Text+" "+cues[1].Text), " ") != "The quick brown fox jumps over the lazy dog" {
		t.Fatal("forced split changed the words", cues[0].Text, cues[1].Text)
	}
}

// Explicit splitting always cleans the resulting source/translation pieces,
// independently of automatic translation preferences, in the same undo step.
func TestSmartSplitAutomaticallyCleansBothTextLayersWithOneUndo(t *testing.T) {
	for _, tab := range []int{0, 1} {
		t.Run(fmt.Sprint(tab), func(t *testing.T) {
			a := storeApp(t, subtitle.Cue{ID: "target", Text: "Version 3.14 is ready today. Are we all done here?", StartMs: 0, EndMs: 8000}, subtitle.Cue{ID: "neighbor", Text: "Keep this. Neighbor.", StartMs: 9000, EndMs: 15000})
			if _, err := a.store.ApplyTranslation("translate", 0, []project.Edit{{ID: "target", Text: "版本3.14已经准备完成。还有问题？", StartMs: 0, EndMs: 8000}}); err != nil {
				t.Fatal(err)
			}
			state, _ := a.store.Snapshot()
			a.setSnapshot(state)
			a.changeSubtitleTab(tab)
			a.settings.StripPunctuation, a.settings.ReplacePeriods = false, false
			a.settings.KeepPunctuation = "?"
			before := *a.snapshot
			work := a.smartSplitWork("target", false)
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
			if source != "Version 3.14 is ready today Are we all done here? " {
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

// An unselected list starts at the relevant end. A single match wraps to
// itself; unrelated categories leave selection, search and playback alone.
func TestReadingNavigationUnselectedSingleAndNoMatches(t *testing.T) {
	a := storeApp(t,
		subtitle.Cue{ID: "first", Text: strings.Repeat("长", 30), EndMs: 10000},
		subtitle.Cue{ID: "normal", Text: "Short", StartMs: 11000, EndMs: 15000},
		subtitle.Cue{ID: "last", Text: strings.Repeat("长", 30), StartMs: 16000, EndMs: 26000})
	for _, target := range []struct{ direction, index int }{{1, 0}, {-1, 2}} {
		a.selected, a.query, a.position = -1, "Short", 7.5
		a.locateReadingIssue("long", target.direction)
		if a.selected != target.index || a.query != "" || a.position != 7.5 {
			t.Fatal("unselected navigation did not start at the expected end", a.selected)
		}
	}
	for _, direction := range []int{-1, 1} {
		a.query = "Short"
		a.locateReadingIssue("brief", direction)
		if a.selected != 2 || a.query != "Short" || a.position != 7.5 {
			t.Fatal("missing category changed list state")
		}
	}
	single := storeApp(t, subtitle.Cue{ID: "only", Text: strings.Repeat("长", 30), EndMs: 10000})
	for _, direction := range []int{-1, 1} {
		single.locateReadingIssue("long", direction)
		if single.selected != 0 || single.locateCue != 0 {
			t.Fatal("single issue did not wrap to itself")
		}
	}
}

func TestReadingNavigationRetainsCategoryAcrossTracksAndDisablesLiveProgress(t *testing.T) {
	a := storeApp(t, subtitle.Cue{ID: "brief", Text: "字", EndMs: 500})
	if _, err := a.store.ApplyTranslation("translate", a.snapshot.Revision, []project.Edit{{ID: "brief", Text: "词", StartMs: 0, EndMs: 500}}); err != nil {
		t.Fatal(err)
	}
	state, err := a.store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	a.setSnapshot(state)
	tt := settled(a.readingTools, 500, 200)
	if err := tt.Click(locales.ReadingIssueType()); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(locales.ReadingViewBrief(1)); err != nil {
		t.Fatal(err)
	}
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			for _, tab := range []int{1, 0, 1} {
				a.changeSubtitleTab(tab)
				tt.Frame()
				if !tt.HasText(locales.ReadingBriefCues()) {
					t.Fatal("track or language change reset the category", language, tab)
				}
			}
		})
	}
	a.translationLive, a.translationRunning = &translationLive{}, true
	a.query = "hidden"
	tt.Frame()
	for _, label := range []string{locales.ReadingIssueType(), locales.ReadingPrevious(), locales.ReadingNext()} {
		if err := tt.Click(label); err != nil || tt.Menu() != nil || tt.Focused(label) || a.query != "hidden" {
			t.Fatal("reading navigation activated during live translation", label, err)
		}
	}
}

// The picker reserves its longest label. All three controls stay on one row
// at the minimum pane width, in both themes and all supported text scales.
func TestReadingCategoryControlsKeepLayout(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			for _, dark := range []bool{false, true} {
				for _, scale := range []float32{1, 1.5, 2} {
					a := storeApp(t, subtitle.Cue{ID: "all", Text: strings.Repeat("长", 30), EndMs: 500})
					tt := settled(a.subtitlesView, int(800*scale), int(800*scale))
					tt.SetDark(dark)
					tt.SetPreferences(ui.Preferences{TextScale: scale, ReduceMotion: true})
					picker, _ := tt.Find(locales.ReadingIssueType())
					previous, _ := tt.Find(locales.ReadingPrevious())
					next, _ := tt.Find(locales.ReadingNext())
					if picker.X < 0 || previous.X < picker.X+picker.W || next.X < previous.X+previous.W || next.X+next.W > 800*scale || math.Abs(float64(picker.Y+picker.H/2-previous.Y-previous.H/2)) > 0.5 || math.Abs(float64(previous.Y+previous.H/2-next.Y-next.H/2)) > 0.5 {
						t.Fatal("issue picker and navigation clipped or separated", language, scale, picker, previous, next)
					}
					for _, label := range []string{locales.ReadingPrevious(), locales.ReadingNext()} {
						button, ok := tt.Find(label)
						if !ok || math.Abs(float64(button.W-button.H)) > 0.5 {
							t.Fatal("navigation button is not icon-sized", label, button)
						}
						if bounds, ok := tt.Find(locales.A11yIconOf(label)); !ok || bounds.W <= 0 || bounds.H <= 0 {
							t.Fatal("navigation button has no Lucide icon", label)
						}
					}
					for _, category := range []string{locales.ReadingViewFast(1), locales.ReadingViewBrief(1), locales.ReadingViewLong(1)} {
						if err := tt.Click(locales.ReadingIssueType()); err != nil {
							t.Fatal(err)
						}
						if err := tt.ChooseMenuItem(category); err != nil {
							t.Fatal(err)
						}
						p, _ := tt.Find(locales.ReadingIssueType())
						prev, _ := tt.Find(locales.ReadingPrevious())
						n, _ := tt.Find(locales.ReadingNext())
						if p != picker || prev != previous || n != next {
							t.Fatal("category change moved the controls", picker, p, previous, prev, next, n)
						}
					}
					captureRedesign(t, tt, fmt.Sprintf("reading-picker-%s-dark%t-%g", language, dark, scale))
				}
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

// The split action counts only long cues. The picker exposes the current
// category; Previous and Next navigate that category without seeking media.
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
					if err := tt.Click(locales.ReadingIssueType()); err != nil {
						t.Fatal(err)
					}
					if want := []string{locales.ReadingViewLong(1), locales.ReadingViewFast(2), locales.ReadingViewBrief(2)}; !slices.Equal(tt.Menu(), want) {
						t.Fatal("categories do not count paired cues independently", tt.Menu())
					}
					if err := tt.ChooseMenuItem(locales.ReadingViewFast(2)); err != nil {
						t.Fatal(err)
					}
					if !tt.HasText(locales.ReadingFastCues()) || a.selected != 0 || a.query != "Short" || a.position != 9.4 {
						t.Fatal("category choice did not update the picker or unexpectedly navigated")
					}
					for _, target := range []struct {
						button string
						index  int
					}{{locales.ReadingNext(), 1}, {locales.ReadingNext(), 2}, {locales.ReadingPrevious(), 1}, {locales.ReadingPrevious(), 2}} {
						if err := tt.Click(target.button); err != nil {
							t.Fatal(err)
						}
						if a.selected != target.index || a.query != "" || a.position != 9.4 || !a.followHeld {
							t.Fatal("category navigation ignored direction, search or video position", a.selected)
						}
					}
					if err := tt.Click(locales.ReadingIssueType()); err != nil {
						t.Fatal(err)
					}
					if err := tt.ChooseMenuItem(locales.ReadingViewBrief(2)); err != nil {
						t.Fatal(err)
					}
					if !tt.HasText(locales.ReadingBriefCues()) {
						t.Fatal("picker did not retain the brief category")
					}
					for _, target := range []struct {
						button string
						index  int
					}{{locales.ReadingNext(), 3}, {locales.ReadingNext(), 1}, {locales.ReadingPrevious(), 3}} {
						if err := tt.Click(target.button); err != nil {
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
			for _, label := range []string{locales.ReadingPrevious(), locales.ReadingNext()} {
				if err := tt.Click(label); err != nil || a.selected != 0 || tt.Focused(label) {
					t.Fatal("zero-count category still navigates", label, err)
				}
			}
			if err := tt.Click(locales.ReadingIssueType()); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem(locales.ReadingViewLong(0)); err == nil {
				t.Fatal("zero-count category is still enabled")
			}
			if err := tt.ChooseMenuItem(locales.ReadingViewFast(1)); err != nil {
				t.Fatal("speed-only issue cannot be reviewed", err)
			}
			if err := tt.Click(locales.ReadingNext()); err != nil || !a.followHeld || a.editPending {
				t.Fatal("navigation did not commit the draft and locate its issue", err)
			}
			a.draft, a.end = "Short", 4
			a.changed(tt.Now())
			tt.Frame()
			tt.Advance(time.Second)
			if !tt.HasText(locales.ReadingSplitCount(0)) || tt.HasText(locales.A11yWarningIcon()) {
				t.Fatal("cleared draft left a misleading warning")
			}
			if err := tt.Click(locales.ReadingIssueType()); err != nil || len(tt.Menu()) != 0 {
				t.Fatal("empty reading categories still open a menu", err)
			}
			if !tt.HasText(locales.ReadingFastCues()) {
				t.Fatal("clearing the last issue reset the chosen category")
			}
			for _, label := range []string{locales.ReadingPrevious(), locales.ReadingNext()} {
				if err := tt.Click(label); err != nil || tt.Focused(label) {
					t.Fatal("cleared category navigation stayed active", label, err)
				}
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
				cues[i] = subtitle.Cue{ID: fmt.Sprint(i), Text: strings.Repeat("word ", 20), StartMs: int64(i) * 16000, EndMs: int64(i)*16000 + 15000}
			}
			a := storeApp(t, cues...)
			tt := settled(a.subtitlesView, 320, 600)
			button, found := tt.Find(locales.ReadingInspect())
			if !found || button.X < 0 || button.X+button.W > 320 {
				t.Fatal("review menu clipped in the minimum pane", language, button)
			}
			if err := tt.Click(locales.ReadingInspect()); err != nil {
				t.Fatal(err)
			}
			for _, label := range []string{locales.ReadingSplitCount(len(cues)), locales.ReadingViewLong(len(cues)), locales.ReadingPrevious(), locales.ReadingNext()} {
				if !slices.Contains(tt.Menu(), label) {
					t.Fatal("large reading count missing from compact menu", label, tt.Menu())
				}
			}
		})
	}
}

func TestReadingToolbarNarrowPane(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := storeApp(t, subtitle.Cue{ID: "a", Text: strings.Repeat("word ", 20), StartMs: 0, EndMs: 15000})
			for _, width := range []int{360, 520, 900} {
				tt := settled(a.subtitlesView, width, 600)
				if width < 680 {
					for _, label := range []string{locales.ReadingInspect(), locales.SubtitleActionsMenu(), locales.ListSaveAs(), locales.ListSearch()} {
						bounds, ok := tt.Find(label)
						if !ok || bounds.X < 0 || bounds.X+bounds.W > float32(width)+1 {
							t.Fatalf("clipped %q at width %v: %+v", label, width, bounds)
						}
					}
					if err := tt.Click(locales.ReadingInspect()); err != nil {
						t.Fatal(err)
					}
					for _, label := range []string{locales.ReadingSplitCount(1), locales.ReadingPrevious(), locales.ReadingNext()} {
						if !slices.Contains(tt.Menu(), label) {
							t.Fatal("compact reading command missing", label)
						}
					}
					tt.CloseMenu()
					if err := tt.Click(locales.SubtitleActionsMenu()); err != nil {
						t.Fatal(err)
					}
					for _, label := range []string{locales.FindReplace(), locales.TextReplacePeriods(), locales.TextStripTrailing()} {
						if !slices.Contains(tt.Menu(), label) {
							t.Fatal("compact edit command missing", label)
						}
					}
					continue
				}
				for _, label := range []string{locales.ReadingSplitCount(1), locales.ReadingIssueType(), locales.ReadingPrevious(), locales.ReadingNext(), locales.FindReplace(), locales.TextReplacePeriods(), locales.TextStripTrailing(), locales.ListSaveAs()} {
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
				if width == 900 && replace.Y < search.Y+search.H {
					t.Fatal("editing tools overlap the file/search row", replace, search)
				}
			}
		})
	}
}
