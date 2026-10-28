package project

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
)

func TestTimelineCandidateAtomicUndoRedoAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "timeline.onesub")
	doc := subtitle.Document{Format: "srt", Cues: []subtitle.Cue{{ID: "c1", Position: 0, Text: "Hello", OriginalText: "Hello", StartMs: 0, EndMs: 1000}, {ID: "c2", Position: 1, Text: "world", OriginalText: "world", StartMs: 1000, EndMs: 2000}}}
	s, err := Create(path, "", "", doc)
	if err != nil {
		t.Fatal(err)
	}
	candidate, _, err := subtitle.Resegment(doc, []subtitle.SourceRange{{FirstAtom: "c1", LastAtom: "c2", Reason: "完整语义"}})
	if err != nil {
		t.Fatal(err)
	}
	candidate.Cues[0].Translated = true
	candidate.Cues[0].TranslationSource = candidate.Cues[0].Text
	candidate.Cues[0].Text = "你好"
	if _, err = s.ApplyTranslationDocument("accept", 0, candidate); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ApplyTranslationDocument("stale", 0, candidate); err == nil {
		t.Fatal("stale candidate committed")
	}
	state, _ := s.Snapshot()
	if state.Revision != 1 || len(state.Document.Cues) != 1 {
		t.Fatal(state)
	}
	if _, err = s.Undo("undo", 1, false); err != nil {
		t.Fatal(err)
	}
	state, _ = s.Snapshot()
	if !reflect.DeepEqual(state.Document.Cues, doc.Cues) {
		t.Fatal(state.Document.Cues)
	}
	if _, err = s.Undo("redo", 2, true); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	state, _ = s.Snapshot()
	if !reflect.DeepEqual(state.Document.Cues, candidate.Cues) {
		t.Fatal(state)
	}
	bad := candidate
	bad.Cues = append([]subtitle.Cue{}, candidate.Cues...)
	bad.Cues[0].StartMs = 99
	if _, err = s.ApplyTranslationDocument("bad", state.Revision, bad); err == nil {
		t.Fatal("tampered candidate accepted")
	}
	after, _ := s.Snapshot()
	if after.Revision != state.Revision || !reflect.DeepEqual(state.Document, after.Document) {
		t.Fatal("failed transaction wrote data")
	}
}
