package project

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
)

func TestSmartSplitTransactionTracksHistoryAndReopen(t *testing.T) {
	for _, original := range []bool{true, false} {
		doc := subtitle.Document{Format: "srt", Cues: []subtitle.Cue{
			{ID: "a", Text: "This first sentence explains the complete workflow. This second sentence explains every remaining operation.", StartMs: 1000, EndMs: 24000},
			{ID: "b", Text: "Done", StartMs: 25000, EndMs: 27000},
		}}
		path := filepath.Join(t.TempDir(), "split.onesub")
		s, err := Create(path, "", "", doc)
		if err != nil {
			t.Fatal(err)
		}
		// A distinct working track must never be overwritten by source edits.
		if _, err = s.ApplyTranslation("translate", 0, []Edit{{ID: "a", Text: "首先完整地说明当前工作流程，然后继续说明所有操作方法，最后仔细检查相关字幕。", StartMs: 1000, EndMs: 24000}}); err != nil {
			t.Fatal(err)
		}
		before, _ := s.Snapshot()
		if _, err = s.SplitCues(context.Background(), "split", before.Revision, original, subtitle.PunctuationOptions{}); err != nil {
			t.Fatal(err)
		}
		after, _ := s.Snapshot()
		track := 1
		if original {
			track = 0
		}
		if len(after.TrackDocument(track).Cues) <= 2 || !reflect.DeepEqual(before.TrackDocument(1-track), after.TrackDocument(1-track)) {
			t.Fatal("wrong track mutated")
		}
		for i, cue := range after.TrackDocument(track).Cues {
			if cue.Position != i {
				t.Fatal("numbering")
			}
		}
		if revision, err := s.SplitCues(context.Background(), "split", before.Revision, original, subtitle.PunctuationOptions{}); err != nil || revision != after.Revision {
			t.Fatal("not idempotent", err)
		}
		if _, err = s.SplitCues(context.Background(), "stale", before.Revision, original, subtitle.PunctuationOptions{}); err == nil {
			t.Fatal("accepted stale revision")
		}
		if _, err = s.Undo("undo", after.Revision, false); err != nil {
			t.Fatal(err)
		}
		undone, _ := s.Snapshot()
		if !reflect.DeepEqual(undone.Document, before.Document) || !reflect.DeepEqual(undone.SourceDocument, before.SourceDocument) {
			t.Fatal("undo failed")
		}
		if _, err = s.Undo("redo", undone.Revision, true); err != nil {
			t.Fatal(err)
		}
		s.Close()
		s, err = Open(path)
		if err != nil {
			t.Fatal(err)
		}
		reopened, _ := s.Snapshot()
		s.Close()
		if !reflect.DeepEqual(reopened.Document, after.Document) || !reflect.DeepEqual(reopened.SourceDocument, after.SourceDocument) {
			t.Fatal("reopen lost split metadata")
		}
	}
}

func TestSingleSplitReadableCueDoesNotBatchOtherProblems(t *testing.T) {
	doc := subtitle.Document{Format: "srt", Cues: []subtitle.Cue{
		{ID: "short", Text: "Hello wonderful world today.", StartMs: 0, EndMs: 4000},
		{ID: "long", Text: "This is a much longer neighboring subtitle which must remain unchanged by the single-cue command.", StartMs: 5000, EndMs: 19000},
	}}
	s, err := Create(filepath.Join(t.TempDir(), "single.onesub"), "", "", doc)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	before, _ := s.Snapshot()
	if _, err = s.SplitCue(context.Background(), "single", before.Revision, true, "short", subtitle.PunctuationOptions{}); err != nil {
		t.Fatal(err)
	}
	after, _ := s.Snapshot()
	if len(after.SourceDocument.Cues) != 3 || after.SourceDocument.Cues[2].Text != before.SourceDocument.Cues[1].Text || after.SourceDocument.Cues[2].Position != 2 {
		t.Fatal("wrong single-cue split", after)
	}
	if _, err = s.SplitCue(context.Background(), "single", before.Revision, true, "short", subtitle.PunctuationOptions{}); err != nil {
		t.Fatal("not idempotent", err)
	}
	if _, err = s.Undo("undo", after.Revision, false); err != nil {
		t.Fatal(err)
	}
	undone, _ := s.Snapshot()
	if !reflect.DeepEqual(before.Document, undone.Document) || !reflect.DeepEqual(before.SourceDocument, undone.SourceDocument) {
		t.Fatal("single undo did not restore both mirrors")
	}
}

func TestOriginalSmartSplitMirrorsOnlyUntouchedWorkingTrack(t *testing.T) {
	doc := subtitle.Document{Format: "srt", Cues: []subtitle.Cue{{ID: "a", Text: "这是一段需要完整保留的较长字幕，我们希望仔细说明整个操作流程，同时避免每一屏出现太多文字。", StartMs: 0, EndMs: 20000}}}
	s, err := Create(filepath.Join(t.TempDir(), "source.onesub"), "", "", doc)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.SplitCues(context.Background(), "split", 0, true, subtitle.PunctuationOptions{}); err != nil {
		t.Fatal(err)
	}
	state, _ := s.Snapshot()
	if !reflect.DeepEqual(state.SourceDocument.Cues, state.Document.Cues) {
		t.Fatal("untouched mirror diverged")
	}
}
