package main

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

func TestFindReplaceSingleAndAllUseSharedAtomicEdit(t *testing.T) {
	for _, track := range []int{0, 1} {
		for _, all := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d/%t", track, all), func(t *testing.T) {
				a := storeApp(t, subtitle.Cue{ID: "a", Text: "word word.", StartMs: 0, EndMs: 4000}, subtitle.Cue{ID: "b", Text: "word.", StartMs: 5000, EndMs: 8000})
				if _, err := a.store.ApplyTranslation("translate", a.snapshot.Revision, []project.Edit{{ID: "a", Text: "word word.", StartMs: 0, EndMs: 4000}, {ID: "b", Text: "word.", StartMs: 5000, EndMs: 8000}}); err != nil {
					t.Fatal(err)
				}
				state, _ := a.store.Snapshot()
				a.setSnapshot(state)
				a.changeSubtitleTab(track)
				before := *a.snapshot
				a.findReplace = findReplaceState{find: "word", replacement: "new"}
				a.query = "not visible"
				edits, count, _ := a.replacementEdits(all)
				want := 1
				if all {
					want = 3
				}
				if count != want {
					t.Fatal("occurrence count", count)
				}
				apply, err := a.listTextEditWork(edits)(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				apply()
				text := a.activeDocument().Cues[0].Text
				if !all && text != "new word." || all && text != "new new." {
					t.Fatal(text)
				}
				if !reflect.DeepEqual(before.TrackDocument(1-track), a.snapshot.TrackDocument(1-track)) {
					t.Fatal("changed other track")
				}
				a.undo(false)
				if !reflect.DeepEqual(before.Document, a.snapshot.Document) || !reflect.DeepEqual(before.SourceDocument, a.snapshot.SourceDocument) {
					t.Fatal("one undo did not restore batch")
				}
			})
		}
	}
}

func TestListPunctuationWorksWithoutTranslationOptions(t *testing.T) {
	for _, track := range []int{0, 1} {
		for _, strip := range []bool{false, true} {
			a := storeApp(t, subtitle.Cue{ID: "a", Text: "First. Next。", StartMs: 0, EndMs: 5000})
			if _, err := a.store.ApplyTranslation("translate", 0, []project.Edit{{ID: "a", Text: "你好。世界。", StartMs: 0, EndMs: 5000}}); err != nil {
				t.Fatal(err)
			}
			state, _ := a.store.Snapshot()
			a.setSnapshot(state)
			a.changeSubtitleTab(track)
			a.settings.StripPunctuation, a.settings.ReplacePeriods = false, false
			before := *a.snapshot
			edits := a.punctuationEdits(strip)
			apply, err := a.listTextEditWork(edits)(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			apply()
			want := "First, Next，"
			if strip {
				want = "First. Next"
			}
			if track == 1 {
				want = "你好，世界，"
				if strip {
					want = "你好。世界"
				}
			}
			if a.activeDocument().Cues[0].Text != want {
				t.Fatal(a.activeDocument().Cues[0].Text, want)
			}
			if a.settings.StripPunctuation || a.settings.ReplacePeriods {
				t.Fatal("manual action changed translation defaults")
			}
			if !reflect.DeepEqual(before.TrackDocument(1-track), a.snapshot.TrackDocument(1-track)) {
				t.Fatal("manual cleanup changed other track")
			}
			a.undo(false)
			if !reflect.DeepEqual(before.Document, a.snapshot.Document) || !reflect.DeepEqual(before.SourceDocument, a.snapshot.SourceDocument) {
				t.Fatal("cleanup undo failed")
			}
		}
	}
}

func TestListTextRejectsEmptyCueAtomicallyAndFreezesTrack(t *testing.T) {
	a := storeApp(t, subtitle.Cue{ID: "a", Text: "word", StartMs: 0, EndMs: 3000}, subtitle.Cue{ID: "b", Text: "word rest", StartMs: 4000, EndMs: 7000})
	before := *a.snapshot
	a.findReplace = findReplaceState{find: "word", replacement: ""}
	edits, _, _ := a.replacementEdits(true)
	if _, err := a.listTextEditWork(edits)(context.Background()); err == nil {
		t.Fatal("empty subtitle accepted")
	}
	after, _ := a.store.Snapshot()
	if !reflect.DeepEqual(before, after) {
		t.Fatal("partial replacement committed")
	}
	a.findReplace.replacement = "new"
	edits, _, _ = a.replacementEdits(true)
	work := a.listTextEditWork(edits)
	a.subtitleTab = 1
	apply, err := work(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	apply()
	if a.snapshot.SourceDocument.Cues[0].Text != "new" {
		t.Fatal("tab switching redirected frozen batch")
	}
}

func TestFindReplaceDialogLayoutAtScales(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			for _, scale := range []float32{1, 1.5, 2} {
				a := storeApp(t, subtitle.Cue{ID: "a", Text: "word word", StartMs: 0, EndMs: 3000})
				a.openFindReplace()
				tt := settled(a.findReplaceView, int(900*scale), int(640*scale))
				tt.SetPreferences(ui.Preferences{TextScale: scale, ReduceMotion: true})
				insideRedesign(t, tt, locales.FindReplace(), locales.FindText(), locales.ReplaceWith(), locales.FindMatchCase(), locales.FindNext(), locales.ReplaceOne(), locales.ReplaceAll())
				before, _ := tt.Find(locales.FindReplace())
				a.findReplace.find = "word"
				a.findNextText()
				tt.Frame()
				after, _ := tt.Find(locales.FindReplace())
				if before != after {
					t.Fatal("dialog resized for a match", before, after)
				}
			}
		})
	}
}

func TestFindReplaceDialogAndButtonsInBothLanguages(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := storeApp(t, subtitle.Cue{ID: "a", Text: "word word", StartMs: 0, EndMs: 3000})
			a.openFindReplace()
			a.findReplace.find = "word"
			tt := settled(a.findReplaceView, 900, 650)
			for _, label := range []string{locales.FindText(), locales.ReplaceWith(), locales.ReplaceOne(), locales.ReplaceAll(), locales.FindNext()} {
				if _, ok := tt.Find(label); !ok {
					t.Fatal(label)
				}
			}
			if err := tt.Click(locales.FindNext()); err != nil {
				t.Fatal(err)
			}
			if a.findReplace.current == nil || a.findReplace.current.Start != 0 {
				t.Fatal("first occurrence missing")
			}
			if err := tt.Click(locales.FindNext()); err != nil {
				t.Fatal(err)
			}
			if a.findReplace.current.Start != 5 {
				t.Fatal("next occurrence did not advance inside cue")
			}
			captureRedesign(t, tt, "find-replace-"+language)
		})
	}
}
