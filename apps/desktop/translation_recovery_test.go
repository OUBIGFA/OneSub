package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/translator"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

func TestRecoveryDoesNotOverwriteUnreadableSettings(t *testing.T) {
	a := nativeForTest(t)
	a.settingsError = true
	path := filepath.Join(a.dataRoot, "settings.json")
	if err := os.WriteFile(path, []byte("unreadable settings"), 0600); err != nil {
		t.Fatal(err)
	}
	err := a.rememberTranslationTask(&translationTask{ProjectPath: t.TempDir()}, newTranslationLive())
	data, readErr := os.ReadFile(path)
	if err == nil || readErr != nil || string(data) != "unreadable settings" {
		t.Fatal("recovery overwrote unreadable settings", err, readErr)
	}
}

func TestTranslationJournalRestoresCompletedAndDraftTextWithoutKeys(t *testing.T) {
	path := t.TempDir()
	task := &translationTask{ProjectPath: path, ProjectID: "p", Revision: 3, Config: translator.Config{Endpoint: "https://example.com/v1", Model: "saved-model", Key: "never-persist-test-key"}}
	if err := writeTranslationRecord(path, translationRecord{Task: task}, true); err != nil {
		t.Fatal(err)
	}
	units := []subtitle.Cue{{ID: "c1", Text: "First"}, {ID: "c2", Text: "Second"}}
	event := translator.Event{Stage: "translate", Done: 1, Total: 2, Units: units, Resume: &translator.ResumeState{InputHash: "input", Units: subtitle.Document{Format: "srt", Cues: units}}, Updates: []translator.UnitText{{Index: 0, Text: "已完成", Final: true}, {Index: 1, Text: "尚未完成"}}}
	if err := writeTranslationRecord(path, translationRecord{Event: &event}, false); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(translationProgressPath(path), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.WriteString(`{"event":`)
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	restored, live, _, finished, err := readTranslationProgress(path)
	if err != nil || finished || restored.Config.Model != "saved-model" || restored.Config.Key != "" || len(live.units) != 2 || live.texts[1] != "尚未完成" || !live.final[0] || live.final[1] || live.resume.Completed[0] != "已完成" || len(live.resume.Completed) != 1 || live.stopped == "" {
		t.Fatal("progress did not survive restart", err, restored, live)
	}
	data, err := os.ReadFile(translationProgressPath(path))
	if err != nil || strings.Contains(string(data), "never-persist-test-key") {
		t.Fatal("key persisted", err)
	}
	a := nativeForTest(t)
	a.translationTask, a.translationLive, a.subtitleTab = restored, live, 1
	tt := settled(a.subtitlesView, 700, 600)
	for _, label := range []string{locales.TranslationInterrupted(), locales.TranslationResume(), "已完成", "尚未完成"} {
		if !tt.HasText(label) {
			t.Fatal("interrupted view hid progress", label, tt.Texts())
		}
	}
}
