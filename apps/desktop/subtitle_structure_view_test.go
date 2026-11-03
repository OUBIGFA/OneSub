package main

import (
	"fmt"
	"slices"
	"strings"
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
				want := []string{locales.SubtitleLocateVideo(), "-", locales.SubtitleInsertBefore(), locales.SubtitleInsertAfter(), locales.ReadingSplit(), "-", locales.SubtitleMergePrevious(), locales.SubtitleMergeNext(), "-", locales.SubtitleDelete()}
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

// The explicit menu seek uses the clicked cue's own track time, always pauses,
// and never inherits the configurable single/double-click playback action.
func TestSubtitleContextLocateVideoBothTracks(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			for _, tab := range []int{0, 1} {
				for _, paused := range []bool{false, true} {
					for _, action := range []string{"none", "seek-pause", "seek-play"} {
						t.Run(fmt.Sprintf("%s/tab%d/paused%t/%s", language, tab, paused, action), func(t *testing.T) {
							a := draftApp(t)
							if _, err := a.store.Apply("manual-translation", a.snapshot.Revision, []project.Edit{{ID: "a", Track: "translation", Text: "First translation", StartMs: 100, EndMs: 2000}, {ID: "b", Track: "translation", Text: "Target translation", StartMs: 5000, EndMs: 7000}}); err != nil {
								t.Fatal(err)
							}
							state, err := a.store.Snapshot()
							if err != nil {
								t.Fatal(err)
							}
							a.setSnapshot(state)
							a.changeSubtitleTab(tab)
							a.settings.SingleClickAction, a.settings.DoubleClickAction = action, action
							a.mediaURL, a.position, a.paused = "local-test-video", 9.4, paused
							cue := a.activeDocument().Cues[1]
							a.query = cueDisplayText(cue, tab)
							tt := settled(a.subtitlesView, 900, 500)
							if err := tt.RightClick(a.query); err != nil {
								t.Fatal(err)
							}
							if a.selected != 1 || a.position != 9.4 || a.paused != paused {
								t.Fatal("opening the menu changed playback or selected a filtered row number")
							}
							if err := tt.ChooseMenuItem(locales.SubtitleLocateVideo()); err != nil {
								t.Fatal(err)
							}
							tt.Advance(subtitleDoubleClickInterval() + time.Second)
							if a.selected != 1 || a.position != float64(cue.StartMs)/1000 || !a.paused || a.followHeld || a.pendingClick != nil || a.rowClick != nil {
								t.Fatal("menu seek did not locate and pause the clicked cue", a.selected, a.position, a.paused)
							}
							if a.settings.SingleClickAction != action || a.settings.DoubleClickAction != action {
								t.Fatal("explicit menu action changed playback preferences")
							}
						})
					}
				}
			}
		})
	}
}

// Locating does not edit ASS content, but missing media and active tasks must
// disable the command immediately rather than replaying it after completion.
func TestSubtitleContextLocateVideoAvailability(t *testing.T) {
	for _, state := range []string{"protected", "no-media", "busy", "compose", "recognize", "translate"} {
		t.Run(state, func(t *testing.T) {
			a := storeApp(t, subtitle.Cue{ID: "cue", Text: "Target subtitle", StartMs: 2000, EndMs: 4000, ReadOnly: state == "protected"})
			a.mediaURL, a.position, a.paused = "local-test-video", 9.4, true
			if state == "no-media" {
				a.mediaURL = ""
			}
			a.busy = state == "busy"
			a.composeRunning, a.recognitionRunning, a.translationRunning = state == "compose", state == "recognize", state == "translate"
			tt := settled(a.subtitlesView, 900, 500)
			if err := tt.RightClick("Target subtitle"); err != nil {
				t.Fatal(err)
			}
			err := tt.ChooseMenuItem(locales.SubtitleLocateVideo())
			if state == "protected" {
				if err != nil || a.position != 2 || !a.paused {
					t.Fatal("format protection incorrectly blocked a non-editing seek", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "disabled") || a.position != 9.4 || !a.paused {
				t.Fatal("unavailable menu command changed playback", err, a.position)
			}
			tt.CloseMenu()
			a.busy, a.composeRunning, a.recognitionRunning, a.translationRunning = false, false, false, false
			tt.Advance(subtitleDoubleClickInterval() + time.Second)
			if a.position != 9.4 || a.pendingClick != nil {
				t.Fatal("blocked menu command replayed after the task")
			}
		})
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
	if err := tt.ChooseMenuItem(locales.SubtitleLocateVideo()); err == nil {
		t.Fatal("empty list enabled video positioning")
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
					var danger ui.Color
					tt := settled(func(c *ui.Context) {
						danger = c.Theme().Danger
						a.subtitlesView(c)
					}, width, 450)
					labels := []string{locales.A11ySubtitleTabs(), locales.SubtitleActionsMenu(), locales.ListLocate(), locales.ListSearch(), locales.TranslationResults(), locales.ListSave(), locales.ListSaveAs()}
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
					// Unsaved is the visible save label, not another toolbar item.
					save, _ := tt.Find(locales.ListSave())
					status, found := tt.Find(locales.ListUnsaved())
					if !found || status.X < save.X || status.Y < save.Y || status.X+status.W > save.X+save.W || status.Y+status.H > save.Y+save.H {
						t.Fatal("unsaved status is outside the save button", save, status)
					}
					red := 0
					image := tt.Image()
					for y := int(status.Y); y < int(status.Y+status.H); y++ {
						for x := int(status.X); x < int(status.X+status.W); x++ {
							pixel := image.RGBAAt(x, y)
							if pixel.R == danger.R && pixel.G == danger.G && pixel.B == danger.B {
								red++
							}
						}
					}
					if red < 5 {
						t.Fatal("unsaved label is not red", red)
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
			toolbar, _ := tt.Find(locales.A11ySubtitleActions())
			for i, r := range header {
				if r.X < toolbar.X || r.X+r.W > toolbar.X+toolbar.W {
					t.Fatal("list header control clipped", language, r)
				}
				for _, other := range header[i+1:] {
					if r.X < other.X+other.W && r.X+r.W > other.X && r.Y < other.Y+other.H && r.Y+r.H > other.Y {
						t.Fatal("list header controls overlap", language, r, other)
					}
				}
			}
			// A wrapped English toolbar can leave the second cue below the
			// viewport. Use the real list navigation before testing its menu;
			// the headless finder also knows overscanned, non-visible cells.
			if err := tt.Click(locales.ListBottom()); err != nil {
				t.Fatal(err)
			}
			tt.Advance(subtitleNavigationAnimation.Duration)
			target, visible := tt.Find("第二句")
			if !visible || target.W <= 0 || target.H <= 0 || target.Y+target.H/2 < list.Y || target.Y+target.H/2 >= list.Y+list.H {
				t.Fatal("menu target is outside the visible list", language, list, target)
			}
			if err := tt.RightClick("第二句"); err != nil {
				t.Fatal(err)
			}
			if len(tt.Menu()) == 0 || !tt.Focused(locales.A11ySubtitleList()) {
				t.Fatal("visible row menu did not retain list focus", language, tt.Menu())
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
