package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
)

func originalTrack(t *testing.T, state Snapshot) subtitle.Document {
	t.Helper()
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var tracks struct {
		SourceDocument subtitle.Document `json:"sourceDocument"`
	}
	if err = json.Unmarshal(raw, &tracks); err != nil {
		t.Fatal(err)
	}
	return tracks.SourceDocument
}

func TestVersionOneProjectRecoversOriginalTrackBeforeRebuild(t *testing.T) {
	doc := subtitle.Document{Format: "srt", Cues: []subtitle.Cue{{ID: "a", Position: 0, Text: "Hello", OriginalText: "Hello", StartMs: 0, EndMs: 1000}, {ID: "b", Position: 1, Text: "world", OriginalText: "world", StartMs: 1000, EndMs: 2000}}}
	path := filepath.Join(t.TempDir(), "legacy.onesub")
	s, err := Create(path, "", "", doc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	candidate, _, err := subtitle.Resegment(doc, []subtitle.SourceRange{{FirstAtom: "a", LastAtom: "b", Reason: "sense group"}})
	if err != nil {
		t.Fatal(err)
	}
	candidate.Cues[0].TranslationSource = candidate.Cues[0].Text
	candidate.Cues[0].Text, candidate.Cues[0].Translated = "你好世界", true
	if _, err = s.ApplyTranslationDocument("translate", 0, candidate); err != nil {
		t.Fatal(err)
	}
	// Produce the exact v1 layout using the real database, not a mock store.
	if _, err = s.db.Exec("DROP TABLE source_cues; ALTER TABLE history DROP COLUMN track; PRAGMA user_version=1;"); err != nil {
		t.Fatal(err)
	}
	manifest := s.manifest
	manifest.Schema = 1
	raw, _ := json.Marshal(manifest)
	if err = os.WriteFile(filepath.Join(path, "project.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	state, err := s.Snapshot()
	if err != nil || !reflect.DeepEqual(originalTrack(t, state).Cues, doc.Cues) || !reflect.DeepEqual(state.Document.Cues, candidate.Cues) {
		t.Fatal("migration lost a track", err, state)
	}
	backups, err := filepath.Glob(filepath.Join(path, "originals", "project-v1-*.sqlite"))
	if err != nil || len(backups) != 1 {
		t.Fatal("migration has no backup", err, backups)
	}
	if _, err = s.Undo("undo", state.Revision, false); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	backups, _ = filepath.Glob(filepath.Join(path, "originals", "project-v1-*.sqlite"))
	if len(backups) != 1 {
		t.Fatal("migration repeated")
	}
}

func TestRebuiltTimelineLeavesOriginalTrackIndependent(t *testing.T) {
	doc := subtitle.Document{Format: "srt", Cues: []subtitle.Cue{{ID: "c1", Position: 0, Text: "Hello", OriginalText: "Hello", StartMs: 0, EndMs: 1000}, {ID: "c2", Position: 1, Text: "world", OriginalText: "world", StartMs: 1000, EndMs: 2000}}}
	path := filepath.Join(t.TempDir(), "tracks.onesub")
	s, err := Create(path, "", "", doc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	candidate, _, err := subtitle.Resegment(doc, []subtitle.SourceRange{{FirstAtom: "c1", LastAtom: "c2", Reason: "complete thought"}})
	if err != nil {
		t.Fatal(err)
	}
	candidate.Cues[0].TranslationSource = candidate.Cues[0].Text
	candidate.Cues[0].Text, candidate.Cues[0].Translated = "你好世界", true
	if _, err = s.ApplyTranslationDocument("rebuild", 0, candidate); err != nil {
		t.Fatal(err)
	}
	state, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(originalTrack(t, state).Cues, doc.Cues) {
		t.Fatal("rebuild lost the original two cues", originalTrack(t, state))
	}
	if _, err = s.Apply("edit-original", state.Revision, []Edit{{ID: "c1", Track: "original", Text: "Hello there", StartMs: 0, EndMs: 900}}); err != nil {
		t.Fatal(err)
	}
	state, _ = s.Snapshot()
	if !reflect.DeepEqual(state.Document.Cues, candidate.Cues) {
		t.Fatal("editing original changed rebuilt subtitles")
	}
	if originalTrack(t, state).Cues[0].Text != "Hello there" {
		t.Fatal("original edit was not saved")
	}
	if _, err = s.Undo("undo-original", state.Revision, false); err != nil {
		t.Fatal(err)
	}
	state, _ = s.Snapshot()
	if !reflect.DeepEqual(originalTrack(t, state).Cues, doc.Cues) || !reflect.DeepEqual(state.Document.Cues, candidate.Cues) {
		t.Fatal("undo touched the wrong track")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	state, _ = s.Snapshot()
	if !reflect.DeepEqual(originalTrack(t, state).Cues, doc.Cues) {
		t.Fatal("original track missing after reopen")
	}
}
