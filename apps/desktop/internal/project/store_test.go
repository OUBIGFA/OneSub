package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
)

func sample() subtitle.Document {
	return subtitle.Document{Format: "srt", Original: "source", Cues: []subtitle.Cue{{ID: "c1", Text: "原句", OriginalText: "原句", StartMs: 100, EndMs: 2000}}}
}
func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Create(filepath.Join(t.TempDir(), "课程.onesub"), "", "", sample())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func snapshot(t *testing.T, s *Store) Snapshot {
	t.Helper()
	state, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	return state
}
func edit(text string) []Edit { return []Edit{{ID: "c1", Text: text, StartMs: 100, EndMs: 2000}} }

func TestCommitReopenAndOriginalBytes(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "原文.srt")
	raw := []byte("\xef\xbb\xbf1\r\n00:00:00,100 --> 00:00:02,000\r\n原句\r\n")
	if err := os.WriteFile(source, raw, 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "课程.onesub")
	s, err := Create(path, "", source, sample())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Apply("first", 0, edit("手改")); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got := snapshot(t, s)
	// A source correction stays translatable: it is not locked.
	if got.Revision != 1 || got.Document.Cues[0].Text != "手改" || got.Document.Cues[0].TextLocked || got.Document.Cues[0].OriginalText != "原句" {
		t.Fatalf("bad reopen: %+v", got)
	}
	saved, err := os.ReadFile(filepath.Join(path, "originals/source.srt"))
	if err != nil || string(saved) != string(raw) {
		t.Fatalf("original changed: %v", err)
	}
}

func TestConflictIdempotencyAndAtomicFailure(t *testing.T) {
	s := newStore(t)
	if rev, err := s.Apply("one", 0, edit("一次")); err != nil || rev != 1 {
		t.Fatal(rev, err)
	}
	if rev, err := s.Apply("one", 0, edit("重复不得改")); err != nil || rev != 1 {
		t.Fatal(rev, err)
	}
	if _, err := s.Apply("late", 0, edit("旧结果")); err == nil || !strings.Contains(err.Error(), "REVISION_CONFLICT") {
		t.Fatal(err)
	}
	invalid := append(edit("部分提交不得发生"), Edit{ID: "missing", Text: "x", StartMs: 0, EndMs: 1})
	if _, err := s.Apply("invalid", 1, invalid); err == nil {
		t.Fatal("accepted missing cue")
	}
	state := snapshot(t, s)
	if state.Revision != 1 || state.Document.Cues[0].Text != "一次" {
		t.Fatal(state)
	}
}

func TestTwoHundredUndoRedoRestoreLocksAndDiscardRedoBranch(t *testing.T) {
	s := newStore(t)
	var revision int64
	for i := 0; i < 200; i++ {
		var err error
		revision, err = s.Apply(fmt.Sprintf("edit-%d", i), revision, edit(fmt.Sprint(i)))
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 200; i++ {
		var err error
		revision, err = s.Undo(fmt.Sprintf("undo-%d", i), revision, false)
		if err != nil {
			t.Fatal(err)
		}
	}
	state := snapshot(t, s)
	if state.Document.Cues[0].Text != "原句" || state.Document.Cues[0].TextLocked || state.CanUndo || !state.CanRedo {
		t.Fatal(state)
	}
	var err error
	revision, err = s.Undo("redo", revision, true)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot(t, s).Document.Cues[0].Text != "0" {
		t.Fatal("redo did not restore")
	}
	if _, err = s.Apply("branch", revision, edit("新分支")); err != nil {
		t.Fatal(err)
	}
	if snapshot(t, s).CanRedo {
		t.Fatal("stale redo branch survived")
	}
}

func TestActiveWriterAndFutureSchemaDoNotModifyProject(t *testing.T) {
	s := newStore(t)
	if other, err := Open(s.path); err == nil {
		other.Close()
		t.Fatal("second writer acquired active project")
	}
	path := filepath.Join(t.TempDir(), "future.onesub")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"schema":999,"id":"future"}`)
	file := filepath.Join(path, "project.json")
	if err := os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("future schema accepted")
	}
	after, err := os.ReadFile(file)
	if err != nil || string(after) != string(raw) {
		t.Fatal("future manifest changed")
	}
	if _, err := os.Stat(filepath.Join(path, "project.sqlite")); !os.IsNotExist(err) {
		t.Fatal("future database created")
	}
}
