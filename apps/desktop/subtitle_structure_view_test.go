package main

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// A cell menu targets its own cue, not the previous selection or the row
// number in a filtered list. Opening it commits the old draft without seeking.
func TestSubtitleContextMenuTargetsClickedCue(t *testing.T) {
	for _, tab := range []int{0, 1} {
		for _, filtered := range []bool{false, true} {
			t.Run(fmt.Sprintf("tab%d/filtered%t", tab, filtered), func(t *testing.T) {
				a := draftApp(t)
				// Materialize a translated set: before the first translation,
				// the project intentionally mirrors original edits into it.
				edits := []project.Edit{}
				for i, cue := range a.snapshot.Document.Cues {
					edits = append(edits, project.Edit{ID: cue.ID, Text: fmt.Sprintf("Translation %d", i+1), StartMs: cue.StartMs, EndMs: cue.EndMs})
				}
				if _, err := a.store.ApplyTranslation("translate", a.snapshot.Revision, edits); err != nil {
					t.Fatal(err)
				}
				state, err := a.store.Snapshot()
				if err != nil {
					t.Fatal(err)
				}
				a.setSnapshot(state)
				a.markSaved(1, "translated.srt")
				a.changeSubtitleTab(tab)
				a.editRebuiltSource = true
				a.loadDraft()
				a.draft = "Edited first cue"
				a.changed(time.Now())
				a.mediaURL = "test-video"
				a.settings.SingleClickAction = "seek-play"
				a.activeCue = -1
				if filtered {
					a.query = "第二句"
				}
				tt := settled(a.subtitlesView, 640, 450)
				if err := tt.RightClick("第二句"); err != nil {
					t.Fatal(err)
				}
				want := []string{locales.SubtitleInsertBefore(), locales.SubtitleInsertAfter(), "-", locales.SubtitleDelete()}
				if !slices.Equal(tt.Menu(), want) {
					t.Fatal("missing subtitle context actions", tt.Menu())
				}
				if a.selected != 1 || a.editPending || cueDisplayText(a.activeDocument().Cues[0], 0) != "Edited first cue" {
					t.Fatal("right-click did not select its cue and preserve the old draft", a.selected, a.activeDocument().Cues)
				}
				if !a.paused || a.activeCue != -1 {
					t.Fatal("right-click ran the single-click playback action")
				}
				if err := tt.ChooseMenuItem(locales.SubtitleDelete()); err != nil {
					t.Fatal(err)
				}
				if cues := a.activeDocument().Cues; len(cues) != 1 || cues[0].ID != "a" || !a.trackUnsaved(tab) {
					t.Fatal("context menu deleted the wrong cue or missed unsaved state", cues)
				}
				if len(a.snapshot.TrackDocument(1-tab).Cues) != 2 {
					t.Fatal("context action modified the other subtitle set")
				}
			})
		}
	}
}

func TestSubtitleContextMenuInsertsAroundClickedCue(t *testing.T) {
	for _, before := range []bool{true, false} {
		t.Run(fmt.Sprintf("before%t", before), func(t *testing.T) {
			a := draftApp(t)
			a.query = "第二句"
			tt := settled(a.subtitlesView, 640, 450)
			if err := tt.RightClick("第二句"); err != nil {
				t.Fatal(err)
			}
			label, position := locales.SubtitleInsertAfter(), 2
			if before {
				label, position = locales.SubtitleInsertBefore(), 1
			}
			if err := tt.ChooseMenuItem(label); err != nil {
				t.Fatal(err)
			}
			if !a.insertOpen || a.insertPosition != position {
				t.Fatal("insertion used the old selection or filtered row number", a.insertPosition)
			}
		})
	}
}

// Table CellMenu covers the number, timestamps, text and the cell padding,
// not just the glyphs of the last column.
func TestSubtitleContextMenuCoversEveryColumn(t *testing.T) {
	for _, label := range []string{"2", stamp(2), stamp(3), "第二句"} {
		t.Run(label, func(t *testing.T) {
			a := draftApp(t)
			a.query = "第二句" // The first cue's end equals this cue's start.
			tt := settled(a.subtitlesView, 640, 450)
			r, ok := tt.Find(label)
			if !ok {
				t.Fatal("cue cell missing", label)
			}
			tt.RightClickAt(r.X-2, r.Y+r.H/2)
			if err := tt.ChooseMenuItem(locales.SubtitleDelete()); err != nil {
				t.Fatal(err)
			}
			if cues := a.activeDocument().Cues; len(cues) != 1 || cues[0].ID != "a" {
				t.Fatal("cell padding menu did not target the second cue", cues)
			}
		})
	}
}

func TestSubtitleContextMenuRespectsBusyAndProtectedCues(t *testing.T) {
	for _, busy := range []bool{false, true} {
		t.Run(fmt.Sprintf("busy%t", busy), func(t *testing.T) {
			a := storeApp(t, subtitle.Cue{ID: "drawing", Text: "Protected drawing", StartMs: 0, EndMs: 2000, ReadOnly: true})
			a.busy = busy
			tt := settled(a.subtitlesView, 640, 450)
			if err := tt.RightClick("Protected drawing"); err != nil {
				t.Fatal(err)
			}
			if len(tt.Menu()) == 0 {
				t.Fatal("context menu missing")
			}
			if err := tt.ChooseMenuItem(locales.SubtitleDelete()); err == nil {
				t.Fatal("protected cue deletion was enabled")
			}
			err := tt.ChooseMenuItem(locales.SubtitleInsertAfter())
			if busy && err == nil || !busy && err != nil {
				t.Fatal("insert availability does not match task state", err)
			}
			if len(a.activeDocument().Cues) != 1 || a.insertOpen == busy {
				t.Fatal("disabled menu action changed the project")
			}
		})
	}
}

func TestEmptySubtitleListContextMenuCanInsert(t *testing.T) {
	a := storeApp(t)
	tt := settled(a.subtitlesView, 640, 450)
	if err := tt.RightClick(locales.ListEmpty()); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(locales.SubtitleDelete()); err == nil {
		t.Fatal("empty list enabled deletion")
	}
	if err := tt.ChooseMenuItem(locales.SubtitleInsertAfter()); err != nil {
		t.Fatal(err)
	}
	if !a.insertOpen || a.insertPosition != 0 {
		t.Fatal("empty list insertion did not start at the beginning")
	}
}

// Unsaved state and translation results must not push the new list commands
// or save labels out of narrow panels; the native toolbar wraps whole groups.
func TestSubtitleListActionsFitNarrowPanels(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		for _, width := range []int{320, 360, 640} {
			t.Run(fmt.Sprintf("%s/%d", language, width), func(t *testing.T) {
				inLocale(t, language, func() {
					a := draftApp(t)
					a.draft = "Unsaved edit"
					a.changed(time.Now())
					a.translationCandidate = &translationCandidate{}
					tt := settled(a.subtitlesView, width, 450)
					labels := []string{locales.A11ySubtitleTabs(), locales.SubtitleActionsMenu(), locales.ListLocate(), locales.ListSearch(), locales.TranslationResults(), locales.ListUnsaved(), locales.ListSave(), locales.ListSaveAs()}
					boxes := []ui.Rect{}
					for _, label := range labels {
						r, ok := tt.Find(label)
						if !ok || r.W <= 0 || r.X < 0 || r.X+r.W > float32(width) || r.Y < 0 || r.Y+r.H > 450 {
							t.Fatal("list control clipped", label, r)
						}
						for _, other := range boxes {
							if r.X < other.X+other.W && r.X+r.W > other.X && r.Y < other.Y+other.H && r.Y+r.H > other.Y {
								t.Fatal("list controls overlap", label, r, other)
							}
						}
						boxes = append(boxes, r)
					}
				})
			})
		}
	}
}

func TestSubtitleListMenuPlacementAndKeyboard(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := draftApp(t)
			tt := settled(a.view, 900, 640)
			menu, ok := tt.Find(locales.SubtitleActionsMenu())
			list, found := tt.Find(locales.A11ySubtitleList())
			if !ok || !found || menu.X < list.X || menu.X+menu.W > list.X+list.W || menu.Y+menu.H > list.Y {
				t.Fatal("structure menu is not above the subtitle list", language, menu, list)
			}
			tabs, _ := tt.Find(locales.A11ySubtitleTabs())
			locate, _ := tt.Find(locales.ListLocate())
			header := []ui.Rect{menu, tabs, locate}
			for i, r := range header {
				if r.X < list.X || r.X+r.W > list.X+list.W {
					t.Fatal("list header control clipped", language, r)
				}
				for _, other := range header[i+1:] {
					if r.X < other.X+other.W && r.X+r.W > other.X && r.Y < other.Y+other.H && r.Y+r.H > other.Y {
						t.Fatal("list header controls overlap", language, r, other)
					}
				}
			}
			if err := tt.RightClick("第二句"); err != nil {
				t.Fatal(err)
			}
			tt.CloseMenu()
			tt.Key(ui.Shift, ui.KeyF10)
			if !slices.Contains(tt.Menu(), locales.SubtitleDelete()) {
				t.Fatal("keyboard context menu missing", tt.Menu())
			}
			tt.CloseMenu()
			// The list retains keyboard focus after its context menu closes.
			tt.Key(0, ui.KeyDelete)
			if cues := a.activeDocument().Cues; len(cues) != 1 || cues[0].ID != "a" {
				t.Fatal("Delete did not act on the right-clicked subtitle", cues)
			}
		})
	}
}
