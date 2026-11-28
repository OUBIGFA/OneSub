package project

import (
	"encoding/json"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"os"
	"path/filepath"
	"testing"
)

func TestSpeakerVersionTwoMigrationAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.onesub")
	s, err := Create(path, "", "", sample())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("DROP TABLE speaker_state; PRAGMA user_version=2"); err != nil {
		t.Fatal(err)
	}
	manifest := s.manifest
	manifest.Schema = 2
	raw, _ := json.Marshal(manifest)
	if err = os.WriteFile(filepath.Join(path, "project.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot(t, s).Manifest.Schema != 3 {
		t.Fatal("not migrated")
	}
	if _, err = s.AddSpeaker("add", 0, "Guest"); err != nil {
		t.Fatal(err)
	}
	got := snapshot(t, s)
	var speaker string
	for id := range got.Document.Speakers.Names {
		speaker = id
	}
	if _, err = s.SetSpeakers("assign", got.Revision, []string{"c1"}, speaker); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got = snapshot(t, s)
	if !got.Document.Cues[0].Speaker.Manual || got.Document.Speakers.Names[speaker] != "Guest" {
		t.Fatal(got)
	}
	backup, _ := filepath.Glob(filepath.Join(path, "originals", "project-v2-*.sqlite"))
	if len(backup) != 1 {
		t.Fatal("backup missing", backup)
	}
}

func TestSpeakerTranslationKeepsMetadata(t *testing.T) {
	s := newStore(t)
	state := &subtitle.SpeakerState{AnalysisID: "run", Names: map[string]string{"a": "A"}, Turns: []subtitle.SpeakerTurn{{StartMs: 0, EndMs: 2000, ID: "a"}}}
	if _, err := s.ApplySpeakers("annotate", 0, state); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, s)
	candidate, _, err := subtitle.Resegment(before.Document, []subtitle.SourceRange{{FirstAtom: "c1", LastAtom: "c1"}})
	if err != nil {
		t.Fatal(err)
	}
	candidate.Cues[0].TranslationSource = candidate.Cues[0].Text
	candidate.Cues[0].Text = "Translation"
	candidate.Cues[0].Translated = true
	if _, err := s.ApplyTranslationDocument("translate", before.Revision, candidate); err != nil {
		t.Fatal(err)
	}
	after := snapshot(t, s)
	if after.Document.Cues[0].Speaker.IDs[0] != "a" {
		t.Fatal("lost speaker")
	}
}

func TestSpeakerTransactionsUndoAndProtectManual(t *testing.T) {
	s := newStore(t)
	state := &subtitle.SpeakerState{AnalysisID: "run", Names: map[string]string{"a": "A", "b": "B"}, Turns: []subtitle.SpeakerTurn{{StartMs: 0, EndMs: 2100, ID: "a"}}}
	if _, err := s.ApplySpeakers("analysis", 0, state); err != nil {
		t.Fatal(err)
	}
	got := snapshot(t, s)
	if got.Document.Cues[0].Speaker.IDs[0] != "a" || got.SourceDocument.Cues[0].Speaker == nil {
		t.Fatal(got)
	}
	if _, err := s.SetSpeakers("manual", got.Revision, []string{"c1"}, "b"); err != nil {
		t.Fatal(err)
	}
	got = snapshot(t, s)
	if _, err := s.ApplySpeakers("again", got.Revision, state); err != nil {
		t.Fatal(err)
	}
	got = snapshot(t, s)
	if got.Document.Cues[0].Speaker.IDs[0] != "b" {
		t.Fatal("manual label replaced")
	}
	if _, err := s.RenameSpeaker("name", got.Revision, "b", "Guest"); err != nil {
		t.Fatal(err)
	}
	got = snapshot(t, s)
	if got.Document.Speakers.Names["b"] != "Guest" {
		t.Fatal(got)
	}
	if _, err := s.Undo("undo", got.Revision, false); err != nil {
		t.Fatal(err)
	}
	got = snapshot(t, s)
	if got.Document.Speakers.Names["b"] != "B" {
		t.Fatal("name undo failed")
	}
	if _, err := s.ApplySpeakers("stale", 0, state); err == nil {
		t.Fatal("accepted stale analysis")
	}
}

func TestSpeakerNameSurvivesSameAnalysis(t *testing.T) {
	s := newStore(t)
	state := &subtitle.SpeakerState{AnalysisID: "run", Names: map[string]string{"a": "A"}, Turns: []subtitle.SpeakerTurn{{StartMs: 0, EndMs: 2100, ID: "a"}}}
	if _, err := s.ApplySpeakers("initial", 0, state); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RenameSpeaker("rename", snapshot(t, s).Revision, "a", "Host"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplySpeakers("again", snapshot(t, s).Revision, state); err != nil {
		t.Fatal(err)
	}
	if snapshot(t, s).Document.Speakers.Names["a"] != "Host" {
		t.Fatal("same analysis reset a user name")
	}
}

func TestSpeakerMediaChangeDoesNotBlockTranslation(t *testing.T) {
	s := newStore(t)
	if err := s.SetMedia("old.mp4"); err != nil {
		t.Fatal(err)
	}
	state := &subtitle.SpeakerState{AnalysisID: "run", MediaPath: "old.mp4", Names: map[string]string{"a": "A"}, Turns: []subtitle.SpeakerTurn{{StartMs: 0, EndMs: 2100, ID: "a"}}}
	if _, err := s.ApplySpeakers("initial", 0, state); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMedia("new.mp4"); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, s)
	candidate, _, err := subtitle.Resegment(before.Document, []subtitle.SourceRange{{FirstAtom: "c1", LastAtom: "c1"}})
	if err != nil {
		t.Fatal(err)
	}
	candidate.Cues[0].TranslationSource = candidate.Cues[0].Text
	candidate.Cues[0].Text = "Translation"
	candidate.Cues[0].Translated = true
	if _, err = s.ApplyTranslationDocument("translate", before.Revision, candidate); err != nil {
		t.Fatal(err)
	}
	if snapshot(t, s).Document.Cues[0].Speaker.Status != "unknown" {
		t.Fatal("stale identity restored")
	}
}
