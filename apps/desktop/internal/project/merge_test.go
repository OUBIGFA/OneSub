package project

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
)

func TestMergeCuesRenumbersBothTracksUndoRedoAndReopen(t *testing.T) {
	doc := subtitle.Document{Format: "srt", Cues: []subtitle.Cue{
		{ID: "a", Text: "First", OriginalText: "First", StartMs: 1000, EndMs: 3000},
		{ID: "b", Text: "Second\nline", OriginalText: "Second\nline", StartMs: 2500, EndMs: 5000},
		{ID: "c", Text: "Last", OriginalText: "Last", StartMs: 6000, EndMs: 7000},
	}}
	path := filepath.Join(t.TempDir(), "merge.onesub")
	s, err := Create(path, "", "", doc)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := s.Snapshot()
	if _, err = s.MergeCues("merge", 0, true, "a", "b"); err != nil {
		t.Fatal(err)
	}
	state, _ := s.Snapshot()
	cue := state.SourceDocument.Cues[0]
	if len(state.SourceDocument.Cues) != 2 || len(state.Document.Cues) != 2 || cue.Text != "First, Second\nline" || cue.StartMs != 1000 || cue.EndMs != 5000 || !cue.TimeLocked || state.SourceDocument.Cues[1].Position != 1 || state.SourceDocument.Cues[1].ID != "c" {
		t.Fatal("merge lost text/time or did not renumber", state.SourceDocument.Cues)
	}
	if receipt, err := s.MergeCues("merge", 0, true, "a", "b"); err != nil || receipt != 1 {
		t.Fatal("merge was not idempotent", receipt, err)
	}
	if _, err = s.Undo("undo", state.Revision, false); err != nil {
		t.Fatal(err)
	}
	undone, _ := s.Snapshot()
	if !reflect.DeepEqual(undone.SourceDocument.Cues, before.SourceDocument.Cues) || !reflect.DeepEqual(undone.Document.Cues, before.Document.Cues) {
		t.Fatal("one undo did not restore both tracks")
	}
	if _, err = s.Undo("redo", undone.Revision, true); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	reopened, _ := s.Snapshot()
	if !reflect.DeepEqual(reopened.SourceDocument.Cues, state.SourceDocument.Cues) {
		t.Fatal("merged IDs/numbering did not persist")
	}
}

func TestWorkingMergePreservesBilingualTextAndOtherTrack(t *testing.T) {
	s := newStore(t)
	// newStore has one original cue: insert a neighbor before translating.
	if _, err := s.InsertCue("insert", 0, true, Insertion{Position: 1, Source: "Second", StartMs: 3000, EndMs: 4000}); err != nil {
		t.Fatal(err)
	}
	state, _ := s.Snapshot()
	edits := []Edit{}
	for _, cue := range state.Document.Cues {
		edits = append(edits, Edit{ID: cue.ID, Text: "Translation " + cue.Text, StartMs: cue.StartMs, EndMs: cue.EndMs})
	}
	if _, err := s.ApplyTranslation("translate", state.Revision, edits); err != nil {
		t.Fatal(err)
	}
	before, _ := s.Snapshot()
	if _, err := s.MergeCues("merge", before.Revision, false, before.Document.Cues[0].ID, before.Document.Cues[1].ID); err != nil {
		t.Fatal(err)
	}
	after, _ := s.Snapshot()
	cue := after.Document.Cues[0]
	if len(after.Document.Cues) != 1 || cue.Text != "Translation 原句，Translation Second" || cue.TranslationSource != "原句，Second" || !cue.Translated || !cue.TextLocked || !reflect.DeepEqual(after.SourceDocument, before.SourceDocument) {
		t.Fatal("bilingual merge lost a track or its locks", cue)
	}
}

func TestMergeRejectsStaleAndNonadjacentWithoutMutation(t *testing.T) {
	s := newStore(t)
	if _, err := s.InsertCue("insert", 0, true, Insertion{Position: 1, Source: "Second", StartMs: 3000, EndMs: 4000}); err != nil {
		t.Fatal(err)
	}
	before, _ := s.Snapshot()
	for _, ids := range [][2]string{{"missing", before.Document.Cues[1].ID}, {before.Document.Cues[1].ID, before.Document.Cues[0].ID}} {
		if _, err := s.MergeCues("invalid-"+ids[0], before.Revision, true, ids[0], ids[1]); err == nil {
			t.Fatal("invalid adjacency accepted")
		}
	}
	if _, err := s.MergeCues("stale", 0, true, before.Document.Cues[0].ID, before.Document.Cues[1].ID); err == nil {
		t.Fatal("stale merge accepted")
	}
	after, _ := s.Snapshot()
	if !reflect.DeepEqual(before, after) {
		t.Fatal("failed merge mutated history/project")
	}
}
