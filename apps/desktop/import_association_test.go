package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/media"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
)

func associationFixture(t *testing.T) (*NativeApp, string, string) {
	t.Helper()
	ffmpeg, probe, worker := os.Getenv("ONESUB_FFMPEG"), os.Getenv("ONESUB_FFPROBE"), os.Getenv("ONESUB_SUBTITLE_WORKER")
	if ffmpeg == "" || probe == "" || worker == "" {
		t.Skip("build/test.ps1 enables real import workers")
	}
	a := nativeForTest(t)
	a.probe = probe
	a.worker = subtitle.Client{Executable: worker, Scratch: t.TempDir()}
	server, err := media.NewServer()
	if err != nil {
		t.Fatal(err)
	}
	a.preview = server
	t.Cleanup(func() {
		server.Close()
		if a.store != nil {
			a.store.Close()
		}
	})
	folder := t.TempDir()
	video, file := filepath.Join(folder, "picture.mp4"), filepath.Join(folder, "different.srt")
	if out, err := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "color=size=320x180:rate=24:duration=2", "-c:v", "libx264", video).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	if err := os.WriteFile(file, []byte("1\n00:00:00,100 --> 00:00:01,900\nSource subtitle\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return a, video, file
}

func TestDroppedDifferentSubtitleKeepsCurrentVideo(t *testing.T) {
	a, video, file := associationFixture(t)
	for _, path := range []string{video, file} {
		apply, err := a.importFiles(context.Background(), []string{path})
		if err != nil {
			t.Fatal(err)
		}
		apply()
	}
	if a.snapshot.Manifest.MediaPath != video || a.snapshot.Manifest.SubtitlePath != file || len(a.snapshot.SourceDocument.Cues) != 1 || a.candidatesOpen {
		t.Fatal("separately dropped subtitle did not use the current video", a.snapshot.Manifest)
	}
}

// Adding media must retain both working tracks, undo history and saved-file
// baselines. An external subtitle changed since import must not be reread.
func TestDroppedDifferentVideoKeepsEditedSubtitleProject(t *testing.T) {
	verifyMediaAssociationPreservesProject(t, "different.srt", "")
}

func TestMatchedCurrentSubtitleKeepsEditedProject(t *testing.T) {
	for _, name := range []string{"picture.srt", "picture-zh.srt", "picturre.srt"} {
		t.Run(name, func(t *testing.T) { verifyMediaAssociationPreservesProject(t, name, "") })
	}
}

func TestMatchedSubtitleFileAliasKeepsEditedProject(t *testing.T) {
	verifyMediaAssociationPreservesProject(t, "different.srt", "picture.srt")
}

func verifyMediaAssociationPreservesProject(t *testing.T, name, alias string) {
	t.Helper()
	a, video, file := associationFixture(t)
	// Open a genuinely subtitle-only project first. Later name discovery
	// must recognize its already loaded source without reading it again.
	held := filepath.Join(t.TempDir(), "held.mp4")
	if err := os.Rename(video, held); err != nil {
		t.Fatal(err)
	}
	if renamed := filepath.Join(filepath.Dir(file), name); renamed != file {
		if err := os.Rename(file, renamed); err != nil {
			t.Fatal(err)
		}
		file = renamed
	}
	apply, err := a.importFiles(context.Background(), []string{file})
	if err != nil {
		t.Fatal(err)
	}
	apply()
	cue := a.snapshot.Document.Cues[0]
	if _, err = a.store.ApplyTranslation("translation", a.snapshot.Revision, []project.Edit{{ID: cue.ID, Text: "Translation retained", StartMs: 100, EndMs: 1900}}); err != nil {
		t.Fatal(err)
	}
	state, err := a.store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	a.setSnapshot(state)
	a.markSaved(1, "translation.srt")
	a.loadDraft()
	a.draft, a.editPending = "Edited source retained", true
	if !a.commitEdit(true) {
		t.Fatal("source edit failed")
	}
	a.subtitleTab = 1
	a.settings.SubtitleOutput.Mode = "translation"
	before, savedHash, savedPath := *a.snapshot, a.savedHash, a.savedPath
	if err := os.WriteFile(file, []byte("external file is now invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if alias != "" {
		if err := os.Link(file, filepath.Join(filepath.Dir(file), alias)); err != nil {
			t.Skipf("file identity fixture unavailable: %v", err)
		}
	}
	if err := os.Rename(held, video); err != nil {
		t.Fatal(err)
	}
	apply, err = a.importFiles(context.Background(), []string{video})
	if err != nil {
		t.Fatal(err)
	}
	apply()
	if a.snapshot.Path != before.Path || a.snapshot.Manifest.ID != before.Manifest.ID || a.snapshot.Manifest.MediaPath != video || a.snapshot.Manifest.SubtitlePath != file {
		t.Fatal("media replaced the subtitle project", before.Manifest, a.snapshot.Manifest)
	}
	if !reflect.DeepEqual(a.snapshot.SourceDocument, before.SourceDocument) || !reflect.DeepEqual(a.snapshot.Document, before.Document) || a.snapshot.Revision != before.Revision || !a.snapshot.CanUndo || a.savedHash != savedHash || a.savedPath != savedPath || a.subtitleTab != 1 || a.settings.SubtitleOutput.Mode != "translation" {
		t.Fatal("media association discarded working edits, history or saved baselines")
	}
	if err := a.store.Close(); err != nil {
		t.Fatal(err)
	}
	a.store = nil
	reopened, err := project.Open(before.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restored, err := reopened.Snapshot()
	if err != nil || restored.Manifest.MediaPath != video || !reflect.DeepEqual(restored.Document, before.Document) || !reflect.DeepEqual(restored.SourceDocument, before.SourceDocument) {
		t.Fatal("media association was not durable", err)
	}
}

func TestSameNameDiscoveryPrecedesCurrentVideo(t *testing.T) {
	a, video, file := associationFixture(t)
	apply, err := a.importFiles(context.Background(), []string{video})
	if err != nil {
		t.Fatal(err)
	}
	apply()
	raw, err := os.ReadFile(video)
	if err != nil {
		t.Fatal(err)
	}
	matched := filepath.Join(filepath.Dir(file), "different.mp4")
	if err = os.WriteFile(matched, raw, 0600); err != nil {
		t.Fatal(err)
	}
	apply, err = a.importFiles(context.Background(), []string{file})
	if err != nil {
		t.Fatal(err)
	}
	apply()
	if a.snapshot.Manifest.MediaPath != matched || a.pending.Selection.Automatic != "video" {
		t.Fatal("current video overrode the same-name match", a.snapshot.Manifest)
	}
	if err = os.WriteFile(filepath.Join(filepath.Dir(file), "different.mkv"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	apply, err = a.importFiles(context.Background(), []string{file})
	if err != nil {
		t.Fatal(err)
	}
	apply()
	if !a.candidatesOpen || len(a.pending.Selection.Candidates) != 2 || a.snapshot.Manifest.MediaPath != "" {
		t.Fatal("ambiguous siblings silently used the current video")
	}
}

func TestSingleFileDropRetainsUnsavedCancel(t *testing.T) {
	for _, path := range []string{"different.srt", "different.mp4"} {
		a := draftApp(t)
		a.snapshot.Manifest.MediaPath = "current.mp4"
		a.unsaved[0] = true
		before := a.snapshot
		a.acceptFiles([]string{path})
		if !a.savePromptOpen || a.snapshot != before {
			t.Fatal("single-file drop bypassed unsaved protection", path)
		}
		a.finishSavePrompt("cancel")
		if a.snapshot != before || a.snapshot.Manifest.MediaPath != "current.mp4" {
			t.Fatal("cancel changed the workbench", path)
		}
	}
}
