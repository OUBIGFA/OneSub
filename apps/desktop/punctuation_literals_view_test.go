package main

import (
	"context"
	"reflect"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
)

// Original text and both optimized layers use the same literal-aware cleanup.
// A hidden row is still processed; only the chosen text layer changes, and one
// undo restores it. No subtitle file or preference is written by this command.
func TestListPunctuationPreservesLiteralsInEachTextLayer(t *testing.T) {
	const raw = "Email AsiaPacific@bbc.co.uk. Use 3.14. Done."
	for _, layer := range []string{"original", "source", "translation"} {
		t.Run(layer, func(t *testing.T) {
			a := storeApp(t, subtitle.Cue{ID: "literal", Text: raw, OriginalText: raw, StartMs: 1000, EndMs: 8000})
			if _, err := a.store.ApplyTranslation("translation", 0, []project.Edit{{ID: "literal", Text: raw, StartMs: 1000, EndMs: 8000}}); err != nil {
				t.Fatal(err)
			}
			state, err := a.store.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			a.setSnapshot(state)
			tab := 1
			if layer == "original" {
				tab = 0
			}
			a.changeSubtitleTab(tab)
			a.editRebuiltSource = layer == "source"
			a.loadDraft()
			a.query = "Hidden by search"
			a.settings.ReplacePeriods, a.settings.StripPunctuation = false, false
			before := *a.snapshot
			apply, err := a.listTextEditWork(a.punctuationEdits(false))(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			apply()
			cue := a.activeDocument().Cues[0]
			text := cue.Text
			if layer == "source" {
				text = subtitle.SourceText(cue)
			}
			if text != "Email AsiaPacific@bbc.co.uk, Use 3.14, Done," || !reflect.DeepEqual(before.TrackDocument(1-tab), a.snapshot.TrackDocument(1-tab)) || a.settings.ReplacePeriods || a.settings.StripPunctuation {
				t.Fatal("cleanup changed literals, another track, or preferences", text)
			}
			if tab == 1 && layer == "source" && cue.Text != raw || tab == 1 && layer == "translation" && subtitle.SourceText(cue) != raw {
				t.Fatal("changed the other optimized text layer")
			}
			a.undo(false)
			if !reflect.DeepEqual(before.Document, a.snapshot.Document) || !reflect.DeepEqual(before.SourceDocument, a.snapshot.SourceDocument) {
				t.Fatal("one undo did not restore the literal-aware edit")
			}
		})
	}
}
