package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/translator"
)

func TestNativeTranslationSettings(t *testing.T) {
	a := nativeForTest(t)
	a.settingsOpen = true
	a.settingsTab = 2
	tt := settled(a.view, 1280, 840)
	for _, label := range []string{"API Key", "保存 Key", "API 请求上限", "翻译风格"} {
		if !tt.HasText(label) {
			t.Fatal("missing setting", label)
		}
	}
}

// translationApp is a workbench whose subtitles come from lesson.srt in a
// folder of their own, and a translation of them ready to apply.
func translationApp(t *testing.T) (*NativeApp, string, *translationCandidate) {
	t.Helper()
	a := nativeForTest(t)
	dir := t.TempDir()
	source := filepath.Join(dir, "lesson.srt")
	if err := os.WriteFile(source, []byte("1\n00:00:00,000 --> 00:00:02,000\nHello\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := project.Create(filepath.Join(t.TempDir(), "lesson.onesub"), "", source, subtitle.Document{Format: "srt", Cues: []subtitle.Cue{{ID: "c1", Text: "Hello", OriginalText: "Hello", StartMs: 0, EndMs: 2000}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	state, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	a.store = s
	a.selected = 0
	a.setSnapshot(state)
	a.markSaved(0, source)
	a.markSaved(1, "")
	a.loadDraft()
	return a, dir, candidateFor(t, state, "你好")
}

func candidateFor(t *testing.T, state project.Snapshot, text string) *translationCandidate {
	t.Helper()
	doc, _, err := subtitle.Resegment(state.Document, []subtitle.SourceRange{{FirstAtom: "c1", LastAtom: "c1"}})
	if err != nil {
		t.Fatal(err)
	}
	doc.Cues[0].Text, doc.Cues[0].Translated, doc.Cues[0].TranslationSource = text, true, "Hello"
	return &translationCandidate{ProjectID: state.Manifest.ID, Revision: state.Revision, Result: translator.Result{Document: doc, Report: translator.Report{Translated: 1}}, Checks: translator.Findings{Blocks: 1}}
}

// A finished translation becomes the translation track in one undoable
// step and is saved beside its source as lesson-zh.srt; translating again
// saves a new file, never over the first.
func TestFinishedTranslationIsAppliedAndSavedBesideItsSource(t *testing.T) {
	a, dir, candidate := translationApp(t)
	target := a.translationFile()
	if target != filepath.Join(dir, "lesson-zh.srt") {
		t.Fatal(target)
	}
	a.applyTranslation(candidate, target, []byte("first"))
	if a.snapshot.Document.Cues[0].Text != "你好" || a.subtitleTab != 1 || a.unsaved[1] || a.savedPath[1] != target || !strings.Contains(a.status, "lesson-zh.srt") {
		t.Fatal(a.snapshot.Document.Cues, a.status, a.errorText)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "first" {
		t.Fatal(err, string(data))
	}
	a.applyTranslation(candidateFor(t, *a.snapshot, "您好"), target, []byte("second"))
	if data, err := os.ReadFile(target); err != nil || string(data) != "first" {
		t.Fatal("the first translation was overwritten", err)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "lesson-zh-OneSub.srt")); err != nil || string(data) != "second" {
		t.Fatal(err)
	}
	a.undo(false)
	a.undo(false)
	if a.snapshot.Document.Cues[0].Translated {
		t.Fatal("undo did not restore the subtitles")
	}
	// The folder chosen for translations takes the file instead.
	a.settings.TranslationDir = t.TempDir()
	if got := a.translationFile(); got != filepath.Join(a.settings.TranslationDir, "lesson-zh.srt") {
		t.Fatal(got)
	}
}

// A translation the checker finds errors in is kept in the project, but
// not saved, and its check results are shown.
func TestTranslationWithCheckErrorsStaysUnsaved(t *testing.T) {
	a, dir, candidate := translationApp(t)
	candidate.Checks.Errors = []string{"#1: protected subtitle markup changed"}
	a.applyTranslation(candidate, a.translationFile(), nil)
	if a.snapshot.Document.Cues[0].Text != "你好" || !a.unsaved[1] || !a.translationReview || !strings.Contains(a.status, "1 项错误") {
		t.Fatal(a.status, a.unsaved)
	}
	if _, err := os.Stat(filepath.Join(dir, "lesson-zh.srt")); err == nil {
		t.Fatal("a translation with errors was saved")
	}
	tt := settled(a.view, 1400, 900)
	if !tt.HasText("检查结果") || !tt.HasText("#1: protected subtitle markup changed") {
		t.Fatal(tt.Texts())
	}
}

func TestTranslationRefusesChangedRevision(t *testing.T) {
	a := nativeForTest(t)
	a.snapshot = &project.Snapshot{Revision: 2, Manifest: project.Manifest{ID: "p"}}
	a.applyTranslation(&translationCandidate{ProjectID: "p", Revision: 1}, "", nil)
	if !strings.Contains(a.errorText, "REVISION_CONFLICT") {
		t.Fatal(a.errorText)
	}
}

func TestApplyFailureKeepsTranslationResultsVisible(t *testing.T) {
	a, _, candidate := translationApp(t)
	a.subtitleTab = 1
	a.translationLive = newTranslationLive()
	a.translationLive.apply(translator.Event{Stage: "translate", Done: 1, Total: 1, Units: candidate.Result.Document.Cues, Updates: []translator.UnitText{{Index: 0, Text: "你好", Final: true}}})
	a.store.Close()
	a.applyTranslation(candidate, "", nil)
	if a.errorText == "" || !a.showsTranslationProgress() || a.translationLive.texts[0] != "你好" {
		t.Fatal("failed project write hid the translation", a.errorText)
	}
}

// The translate tab chooses the language and the style on one line, then
// the model, the folder and the sense-group timing.
func TestTranslateTabHoldsItsOptions(t *testing.T) {
	a := withStyleCatalog(t)
	a.workflowTab = 1
	tt := settled(a.workflowPanel, 800, 800)
	for _, text := range []string{"开始翻译字幕", "目标语言", "翻译风格", "按意群重建时间轴", "保存目录"} {
		if !tt.HasText(text) {
			t.Fatal(text, tt.Texts())
		}
	}
	language, _ := tt.Find("目标语言")
	style, _ := tt.Find("翻译风格")
	if language.Y != style.Y {
		t.Fatal("language and style are not on one line", language, style)
	}
}
