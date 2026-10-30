package main

import (
	"path/filepath"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

func TestContinueAfterRecognitionSkipsRecognition(t *testing.T) {
	a := draftApp(t)
	a.settings.Processing = &settings.PendingProcessing{ProjectPath: a.snapshot.Path, Stage: "recognize", Translate: true}
	a.recognitionFinished()
	if a.settings.Processing == nil || a.settings.Processing.Stage != "translate" || !a.settingsOpen || a.settingsTab != 2 {
		t.Fatal("did not continue to translation setup")
	}
	loaded, err := settings.Load(filepath.Join(a.dataRoot, "settings.json"))
	if err != nil || loaded.Processing == nil || loaded.Processing.Stage != "translate" {
		t.Fatal("completed recognition not remembered", err)
	}
	a.settingsOpen = false
	a.processMedia()
	if !a.settingsOpen || a.settingsTab != 2 || a.recognitionRunning {
		t.Fatal("continue repeated recognition")
	}
}

func TestNativeSubtitleMenuInsertsAndDeletes(t *testing.T) {
	a := draftApp(t)
	tt := settled(a.editorView, 640, 450)
	if err := tt.Click(locales.SubtitleActionsMenu()); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(locales.SubtitleInsertAfter()); err != nil {
		t.Fatal(err)
	}
	if !a.insertOpen || a.insertPosition != 1 {
		t.Fatal("insert action chose wrong position")
	}
	a.insertSource = "New subtitle"
	tt = settled(a.insertSubtitleView, 800, 600)
	if err := tt.Click(locales.SubtitleInsertConfirm()); err != nil {
		t.Fatal(err)
	}
	if len(a.activeDocument().Cues) != 3 || a.activeDocument().Cues[1].Text != "New subtitle" {
		t.Fatal("insert dialog did not save the subtitle")
	}
	tt = settled(a.editorView, 640, 450)
	if err := tt.Click(locales.SubtitleActionsMenu()); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(locales.SubtitleDelete()); err != nil {
		t.Fatal(err)
	}
	if len(a.activeDocument().Cues) != 2 || a.activeDocument().Cues[1].ID != "b" {
		t.Fatal("delete menu removed the wrong subtitle")
	}
}

func TestFrequentProcessingActionFitsSmallWindow(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := draftApp(t)
			tt := settled(a.view, 900, 640)
			for _, label := range []string{locales.ActionAddFiles(), locales.ActionRecognizeTranslate(), locales.ActionClearWorkbench(), locales.MenuExportSubtitles(), locales.ActionCompose()} {
				r, ok := tt.Find(label)
				if !ok || r.W <= 0 || r.X < 0 || r.X+r.W > 900 {
					t.Fatal("toolbar control clipped", language, label, r)
				}
			}
		})
	}
}

func TestStandaloneRecognitionDoesNotStartTranslation(t *testing.T) {
	a := draftApp(t)
	a.settings.Processing = &settings.PendingProcessing{Stage: "recognize", Translate: false}
	a.recognitionFinished()
	if a.settings.Processing != nil || a.settingsOpen || a.translationRunning {
		t.Fatal("standalone recognition started translation")
	}
}

func TestOneClickChecksTranslationSetupBeforeStartingRecognition(t *testing.T) {
	a := draftApp(t)
	a.snapshot.Manifest.MediaPath = "selected-video.mp4"
	a.processMedia()
	if !a.settingsOpen || a.settingsTab != 2 || a.busy {
		t.Fatal("one-click recognition began without translation setup")
	}
}

func TestSubtitleInsertDeleteUsesSelectedSetAndKeepsUndo(t *testing.T) {
	a := draftApp(t)
	a.deleteSubtitle("a")
	if len(a.activeDocument().Cues) != 1 || a.activeDocument().Cues[0].ID != "b" || a.activeDocument().Cues[0].Position != 0 || !a.trackUnsaved(0) {
		t.Fatal("delete did not update list and unsaved state")
	}
	a.openSubtitleInsert("b", true)
	a.insertSource = "补充字幕"
	a.insertStart, a.insertEnd = 0, 1
	a.insertSubtitle()
	if len(a.activeDocument().Cues) != 2 || a.activeDocument().Cues[0].Text != "补充字幕" || a.selected != 0 || a.insertOpen {
		t.Fatal("insertion not selected")
	}
	a.undo(false)
	a.undo(false)
	if len(a.activeDocument().Cues) != 2 || a.activeDocument().Cues[0].ID != "a" {
		t.Fatal("undo did not restore deleted subtitle")
	}
}
