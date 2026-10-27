package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/egoist/mygo/ui"
)

func draftApp(t *testing.T) *NativeApp {
	t.Helper()
	a := nativeForTest(t)
	s, err := project.Create(filepath.Join(t.TempDir(), "edit.onesub"), "", "", subtitle.Document{Format: "srt", Cues: []subtitle.Cue{{ID: "a", Text: "原文", OriginalText: "原文", StartMs: 100, EndMs: 2000}, {ID: "b", Text: "第二句", StartMs: 2000, EndMs: 3000}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	state, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	a.store = s
	a.snapshot = &state
	a.selected = 0
	a.loadDraft()
	return a
}
func TestSavePromptSaveDiscardAndCancel(t *testing.T) {
	for _, choice := range []string{"save", "discard", "cancel"} {
		t.Run(choice, func(t *testing.T) {
			a := draftApp(t)
			a.draft = "修改"
			a.changed(time.Now())
			a.selectSubtitle(1, "none")
			if !a.savePromptOpen || a.selected != 0 {
				t.Fatal("unsaved transition was not guarded")
			}
			tt := ui.NewTester(a.view, 1400, 900)
			if !tt.HasText("是否保存字幕修改？") {
				t.Fatal(tt.Texts())
			}
			a.finishSavePrompt(choice)
			state, err := a.store.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			if choice == "save" && (a.selected != 1 || state.Document.Cues[0].Text != "修改") {
				t.Fatal(a.selected, state)
			}
			if choice == "discard" && (a.selected != 1 || state.Document.Cues[0].Text != "原文" || a.dirty) {
				t.Fatal(a.selected, state)
			}
			if choice == "cancel" && (a.selected != 0 || a.draft != "修改" || !a.dirty || state.Revision != 0) {
				t.Fatal(a.selected, state)
			}
		})
	}
}
func TestInvalidSaveKeepsPromptDraftAndAction(t *testing.T) {
	a := draftApp(t)
	a.draft = "修改"
	a.end = 0
	a.changed(time.Now())
	a.changeSubtitleTab(1)
	a.finishSavePrompt("save")
	if !a.savePromptOpen || a.subtitleTab != 0 || a.draft != "修改" || !a.dirty {
		t.Fatal("failed write discarded draft")
	}
}
func TestBothTextTracksCanBeEditedAndSaved(t *testing.T) {
	a := draftApp(t)
	for _, step := range []struct {
		tab  int
		text string
	}{{1, "手写译文"}, {0, "校正原文"}, {1, "校正译文"}} {
		a.changeSubtitleTab(step.tab)
		tt := ui.NewTester(a.view, 1400, 900)
		if err := tt.Click("字幕文字"); err != nil {
			t.Fatal(err)
		}
		tt.Key(ui.Cmd, ui.KeyA)
		tt.Type(step.text)
		if !a.dirty {
			t.Fatal("track read-only", step)
		}
		if err := tt.Click("保存"); err != nil {
			t.Fatal(err)
		}
		if cueDisplayText(a.snapshot.Document.Cues[0], step.tab) != step.text {
			t.Fatal(a.snapshot.Document.Cues[0])
		}
	}
	cue := a.snapshot.Document.Cues[0]
	if cue.TranslationSource != "校正原文" || cue.Text != "校正译文" || cue.OriginalText != "原文" {
		t.Fatal(cue)
	}
}
