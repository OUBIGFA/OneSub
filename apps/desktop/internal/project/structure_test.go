package project

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
)

func TestInsertDeleteRenumberAndUndo(t *testing.T) {
	doc := subtitle.Document{Format: "srt"}
	for i := 0; i < 205; i++ {
		doc.Cues = append(doc.Cues, subtitle.Cue{ID: fmt.Sprint(i), Position: i, Text: fmt.Sprintf("Line %d", i+1), OriginalText: fmt.Sprintf("Line %d", i+1), StartMs: int64(i * 2000), EndMs: int64(i*2000 + 1500)})
	}
	s, err := Create(filepath.Join(t.TempDir(), "edit.onesub"), "", "", doc)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.DeleteCue("delete-200", 0, true, "199"); err != nil {
		t.Fatal(err)
	}
	state, _ := s.Snapshot()
	if len(state.SourceDocument.Cues) != 204 || state.SourceDocument.Cues[199].ID != "200" || state.SourceDocument.Cues[199].Position != 199 || state.SourceDocument.Cues[199].StartMs != 400000 {
		t.Fatal("delete did not renumber without changing times")
	}
	if len(state.Document.Cues) != 204 {
		t.Fatal("unrebuilt working copy was not updated")
	}
	if _, err = s.InsertCue("insert-200", state.Revision, true, Insertion{Position: 199, Source: "Inserted", StartMs: 398000, EndMs: 399500}); err != nil {
		t.Fatal(err)
	}
	state, _ = s.Snapshot()
	if state.SourceDocument.Cues[199].Text != "Inserted" || state.SourceDocument.Cues[200].ID != "200" || state.SourceDocument.Cues[200].Position != 200 {
		t.Fatal("insert did not renumber")
	}
	if _, err = s.Undo("undo-insert", state.Revision, false); err != nil {
		t.Fatal(err)
	}
	state, _ = s.Snapshot()
	if len(state.SourceDocument.Cues) != 204 || state.SourceDocument.Cues[199].ID != "200" {
		t.Fatal("undo did not restore deletion")
	}
	if _, err = s.Undo("undo-delete", state.Revision, false); err != nil {
		t.Fatal(err)
	}
	state, _ = s.Snapshot()
	if len(state.SourceDocument.Cues) != 205 || state.SourceDocument.Cues[199].ID != "199" {
		t.Fatal("undo did not restore original IDs")
	}
}
