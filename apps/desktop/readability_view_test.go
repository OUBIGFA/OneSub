package main

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

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
					if !tt.HasText(locales.ReadingSplit()) {
						t.Fatal("split tool missing")
					}
					if err := tt.Click(locales.ReadingIssues(2)); err != nil {
						t.Fatal(err)
					}
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
					if err := tt.Click(locales.ReadingIssues(2)); err != nil {
						t.Fatal(err)
					}
					if a.selected != 2 {
						t.Fatal("not cycling")
					}
					if err := tt.Click(locales.ReadingIssues(2)); err != nil {
						t.Fatal(err)
					}
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
				a := storeApp(t, subtitle.Cue{ID: "short", Text: "Hello world.", StartMs: 0, EndMs: 4000}, subtitle.Cue{ID: "long", Text: strings.Repeat("word ", 30), StartMs: 5000, EndMs: 25000})
				if _, err := a.store.ApplyTranslation("translate", 0, []project.Edit{{ID: "short", Text: "早上好晚上好", StartMs: 0, EndMs: 4000}}); err != nil {
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
					if strings.ContainsAny(text, ".。．") || strings.HasSuffix(text, ",") || strings.HasSuffix(text, "，") {
						t.Fatal("split punctuation was not cleaned", text)
					}
				}
			}
			if source != "Version 3,14 is ready Are we done? " {
				t.Fatal("cleanup order or kept question mark changed", source)
			}
			if tab == 1 && translated != "版本3,14已经准备完成还有问题？" {
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

func TestReadingToolbarNarrowPane(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := storeApp(t, subtitle.Cue{ID: "a", Text: strings.Repeat("word ", 15), StartMs: 0, EndMs: 15000})
			for _, width := range []int{360, 520, 900} {
				tt := settled(a.subtitlesView, width, 600)
				for _, label := range []string{locales.ReadingSplit(), locales.ReadingIssues(1), locales.FindReplace(), locales.TextReplacePeriods(), locales.TextStripTrailing(), locales.ListSaveAs()} {
					bounds, ok := tt.Find(label)
					if !ok || bounds.X < 0 || bounds.X+bounds.W > float32(width)+1 {
						t.Fatalf("clipped %q at width %v: %+v", label, width, bounds)
					}
				}
				search, _ := tt.Find(locales.ListSearch())
				replace, _ := tt.Find(locales.FindReplace())
				if replace.X < search.X+search.W || replace.X-search.X-search.W > 8 || replace.Y > search.Y+search.H || replace.Y+replace.H < search.Y {
					t.Fatal("find/replace is not immediately right of search", search, replace)
				}
				locate, _ := tt.Find(locales.ListLocate())
				periods, _ := tt.Find(locales.TextReplacePeriods())
				trailing, _ := tt.Find(locales.TextStripTrailing())
				if periods.X <= locate.X+locate.W || trailing.X < periods.X+periods.W || periods.Y != locate.Y || trailing.Y != locate.Y {
					t.Fatal("punctuation is not grouped after navigation", locate, periods, trailing)
				}
				if width == 900 && (trailing.X+trailing.W > search.X || trailing.Y > search.Y+search.H) {
					t.Fatal("punctuation is not left of search", trailing, search)
				}
			}
		})
	}
}
