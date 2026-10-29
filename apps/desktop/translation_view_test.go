package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/translator"
)

func TestNativeTranslationSettingsAndReviewCommit(t *testing.T) {
	a := nativeForTest(t)
	a.settingsOpen = true
	a.settingsTab = 2
	tt := settled(a.view, 1280, 840)
	for _, label := range []string{"API Key", "保存 Key", "API 请求上限"} {
		if !tt.HasText(label) {
			t.Fatal("missing setting", label)
		}
	}
	a.settingsOpen = false
	s, err := project.Create(filepath.Join(t.TempDir(), "review.onesub"), "", "", subtitle.Document{Format: "srt", Cues: []subtitle.Cue{{ID: "c1", Text: "Hello", OriginalText: "Hello", StartMs: 0, EndMs: 2000}}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	state, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	a.store = s
	a.snapshot = &state
	a.selected = 0
	a.loadDraft()
	candidate, _, err := subtitle.Resegment(state.Document, []subtitle.SourceRange{{FirstAtom: "c1", LastAtom: "c1"}})
	if err != nil {
		t.Fatal(err)
	}
	candidate.Cues[0].Text = "你好"
	candidate.Cues[0].Translated = true
	candidate.Cues[0].TranslationSource = "Hello"
	a.translationCandidate = &translationCandidate{ProjectID: state.Manifest.ID, Revision: 0, Result: translator.Result{Document: candidate, Report: translator.Report{Translated: 1}}, Checks: translator.Findings{Blocks: 1}}
	a.translationReview = true
	tt = settled(a.view, 1280, 840)
	if err = tt.Click("接受译文"); err != nil {
		t.Fatal(err)
	}
	if a.snapshot.Document.Cues[0].Text != "你好" || a.translationReview || a.snapshot.Document.Cues[0].TextLocked {
		t.Fatal(a.snapshot, a.errorText)
	}
	a.undo(false)
	if a.snapshot.Document.Cues[0].Text != "Hello" {
		t.Fatal("native undo lost original")
	}
}
func TestNativeReviewRefusesChangedRevision(t *testing.T) {
	a := nativeForTest(t)
	a.snapshot = &project.Snapshot{Revision: 2, Manifest: project.Manifest{ID: "p"}}
	a.translationCandidate = &translationCandidate{ProjectID: "p", Revision: 1}
	a.acceptTranslation()
	if !strings.Contains(a.errorText, "REVISION_CONFLICT") {
		t.Fatal(a.errorText)
	}
}
