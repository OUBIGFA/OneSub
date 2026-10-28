package project

import "testing"

func TestTranslationCommitUndoLocksAndStaleResults(t *testing.T) {
	s := newStore(t)
	if _, err := s.ApplyTranslation("translate", 0, edit("译文")); err != nil {
		t.Fatal(err)
	}
	state := snapshot(t, s)
	if state.Document.Cues[0].Text != "译文" || state.Document.Cues[0].TextLocked {
		t.Fatal(state)
	}
	if _, err := s.Undo("undo", 1, false); err != nil {
		t.Fatal(err)
	}
	if snapshot(t, s).Document.Cues[0].Text != "原句" {
		t.Fatal("translation not undoable")
	}
	// A hand-written translation is protected from the next run.
	manual := edit("手改")
	manual[0].Track = "translation"
	if _, err := s.Apply("manual", 2, manual); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyTranslation("stale", 1, edit("过期")); err == nil {
		t.Fatal("stale result overwrote edit")
	}
	if _, err := s.ApplyTranslation("locked", 3, edit("覆盖")); err == nil {
		t.Fatal("lock bypassed")
	}
	if _, err := s.SetTextLock("unlock", 3, "c1", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyTranslation("unlocked", 4, edit("重译")); err != nil {
		t.Fatal(err)
	}
	if snapshot(t, s).Document.Cues[0].OriginalText != "原句" {
		t.Fatal("source lost")
	}
	if _, err := s.Undo("undo-translation", 5, false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Undo("undo-unlock", 6, false); err != nil {
		t.Fatal(err)
	}
	if !snapshot(t, s).Document.Cues[0].TextLocked {
		t.Fatal("undo failed to restore lock")
	}
}
func TestTranslationCannotRetime(t *testing.T) {
	s := newStore(t)
	edits := edit("译文")
	edits[0].StartMs = 200
	if _, err := s.ApplyTranslation("retime", 0, edits); err == nil {
		t.Fatal("candidate changed timestamps")
	}
	if snapshot(t, s).Revision != 0 {
		t.Fatal("failed result committed")
	}
}
