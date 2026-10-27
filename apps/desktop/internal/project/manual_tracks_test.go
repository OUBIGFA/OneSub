package project

import "testing"

func TestManualEditsKeepSourceAndTranslationIndependent(t *testing.T) {
	s := newStore(t)
	if _, err := s.ApplyTranslation("t", 0, edit("译文")); err != nil {
		t.Fatal(err)
	}
	e := edit("校正原文")
	e[0].Track = "source"
	if _, err := s.Apply("source", 1, e); err != nil {
		t.Fatal(err)
	}
	cue := snapshot(t, s).Document.Cues[0]
	if cue.Text != "译文" || cue.TranslationSource != "校正原文" || cue.OriginalText != "原句" {
		t.Fatal(cue)
	}
	e = edit("校正译文")
	e[0].Track = "translation"
	if _, err := s.Apply("translation", 2, e); err != nil {
		t.Fatal(err)
	}
	cue = snapshot(t, s).Document.Cues[0]
	if cue.Text != "校正译文" || cue.TranslationSource != "校正原文" {
		t.Fatal(cue)
	}
	if _, err := s.Undo("u", 3, false); err != nil {
		t.Fatal(err)
	}
	if snapshot(t, s).Document.Cues[0].Text != "译文" {
		t.Fatal("translation undo failed")
	}
	if _, err := s.Undo("u2", 4, false); err != nil {
		t.Fatal(err)
	}
	cue = snapshot(t, s).Document.Cues[0]
	if cue.TranslationSource != "原句" || cue.Text != "译文" {
		t.Fatal(cue)
	}
}

func TestFirstManualTranslationRetainsSource(t *testing.T) {
	s := newStore(t)
	e := edit("手写译文")
	e[0].Track = "translation"
	if _, err := s.Apply("manual", 0, e); err != nil {
		t.Fatal(err)
	}
	cue := snapshot(t, s).Document.Cues[0]
	if !cue.Translated || cue.TranslationSource != "原句" || !cue.TextLocked {
		t.Fatal(cue)
	}
}
