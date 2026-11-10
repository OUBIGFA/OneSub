package project

import (
	"reflect"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
)

// A second file fills one side by row order; missing rows remain explicit.
func TestProofreadingImportPreservesOtherSideAndUndo(t *testing.T) {
	s := newStore(t)
	original := snapshot(t, s)
	doc := subtitle.Document{Cues: []subtitle.Cue{{Text: "Translation", StartMs: 300, EndMs: 1800}, {Text: "Extra translation", StartMs: 2200, EndMs: 3000}}}
	if _, err := s.ImportProofreading("translation", 0, doc, false); err != nil {
		t.Fatal(err)
	}
	state := snapshot(t, s)
	if len(state.Document.Cues) != 2 || subtitle.SourceText(state.Document.Cues[0]) != "原句" || state.Document.Cues[0].Text != "Translation" || state.Document.Cues[0].StartMs != 100 || subtitle.SourceText(state.Document.Cues[1]) != "" {
		t.Fatal(state.Document)
	}
	if !reflect.DeepEqual(state.SourceDocument, original.SourceDocument) {
		t.Fatal("import changed original track")
	}
	doc.Cues = []subtitle.Cue{{Text: "Corrected source", StartMs: 400, EndMs: 1700}}
	if _, err := s.ImportProofreading("source", state.Revision, doc, true); err != nil {
		t.Fatal(err)
	}
	after := snapshot(t, s)
	if subtitle.SourceText(after.Document.Cues[0]) != "Corrected source" || after.Document.Cues[0].Text != "Translation" || after.Document.Cues[0].StartMs != 400 || after.Document.Cues[1].Text != "Extra translation" {
		t.Fatal(after.Document)
	}
	if _, err := s.Undo("undo", after.Revision, false); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snapshot(t, s).Document, state.Document) {
		t.Fatal("undo did not restore both sides and timing")
	}
}
