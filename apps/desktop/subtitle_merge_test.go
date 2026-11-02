package main

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

func TestListMergeActionsUseDocumentNeighborsAndRenumber(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			for _, original := range []bool{true, false} {
				for _, previous := range []bool{true, false} {
					t.Run(fmt.Sprintf("%s/original%t/previous%t", language, original, previous), func(t *testing.T) {
						a := storeApp(t, navigationCues(3)...)
						edits := []project.Edit{}
						for _, cue := range a.snapshot.Document.Cues {
							edits = append(edits, project.Edit{ID: cue.ID, Text: "Translation " + cue.Text, StartMs: cue.StartMs, EndMs: cue.EndMs})
						}
						if _, err := a.store.ApplyTranslation("translate", a.snapshot.Revision, edits); err != nil {
							t.Fatal(err)
						}
						state, _ := a.store.Snapshot()
						a.setSnapshot(state)
						if !original {
							a.changeSubtitleTab(1)
						}
						a.query = "Subtitle 2"
						other := a.snapshot.TrackDocument(1 - a.subtitleTab)
						tt := settled(a.subtitlesView, 800, 550)
						// Right-click acts on the visible second cue; its real neighbor
						// remains eligible even though search hides the other rows.
						if err := tt.RightClick(cueDisplayText(a.activeDocument().Cues[1], a.subtitleTab)); err != nil {
							t.Fatal(err)
						}
						label, at := locales.SubtitleMergeNext(), 1
						if previous {
							label, at = locales.SubtitleMergePrevious(), 0
						}
						if err := tt.ChooseMenuItem(label); err != nil {
							t.Fatal(err)
						}
						cues := a.activeDocument().Cues
						if len(cues) != 2 || a.selected != at || cues[0].Position != 0 || cues[1].Position != 1 || !a.trackUnsaved(a.subtitleTab) || !a.followHeld {
							t.Fatal("merge did not select/renumber/mark unsaved", cues, a.selected)
						}
						if !reflect.DeepEqual(a.snapshot.TrackDocument(1-a.subtitleTab), other) {
							t.Fatal("merge changed the other track")
						}
						a.undo(false)
						if len(a.activeDocument().Cues) != 3 {
							t.Fatal("merge was not a single undo step")
						}
					})
				}
			}
		})
	}
}

func TestMergeMenuAvailabilityAtEdgesAndDuringTask(t *testing.T) {
	a := storeApp(t, subtitle.Cue{ID: "a", Text: "First", StartMs: 0, EndMs: 1000}, subtitle.Cue{ID: "b", Text: "Second", StartMs: 1000, EndMs: 2000})
	tt := settled(a.subtitlesView, 700, 450)
	if err := tt.RightClick("First"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(locales.SubtitleMergePrevious()); err == nil {
		t.Fatal("first cue has a previous merge")
	}
	tt.CloseMenu()
	a.busy = true
	tt.Frame()
	if err := tt.RightClick("Second"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(locales.SubtitleMergePrevious()); err == nil {
		t.Fatal("task allowed structure mutation")
	}
}
