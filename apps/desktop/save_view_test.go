package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
)

func draftApp(t *testing.T) *NativeApp {
	t.Helper()
	return storeApp(t, subtitle.Cue{ID: "a", Text: "原文", OriginalText: "原文", StartMs: 100, EndMs: 2000}, subtitle.Cue{ID: "b", Text: "第二句", StartMs: 2000, EndMs: 3000})
}

// Saving is about the subtitle file: actions that close the workbench ask,
// while switching cues or tracks never does.
func TestUnsavedFileGuardsClosingActionsOnly(t *testing.T) {
	for _, choice := range []string{"discard", "cancel"} {
		t.Run(choice, func(t *testing.T) {
			a := draftApp(t)
			a.draft = "修改"
			a.changed(time.Now())
			a.changeSubtitleTab(1)
			a.changeSubtitleTab(0)
			if a.savePromptOpen || !a.unsaved[0] {
				t.Fatal("tab switch prompted or lost the edit")
			}
			ran := 0
			var action func()
			action = func() {
				if a.guardUnsaved(action) {
					return
				}
				ran++
			}
			action()
			if !a.savePromptOpen || ran != 0 {
				t.Fatal("closing action did not ask")
			}
			tt := settled(a.view, 1400, 900)
			if !tt.HasText("是否保存字幕修改？") || !tt.HasText("原文：覆盖 original.srt") {
				t.Fatal(tt.Texts())
			}
			a.finishSavePrompt(choice)
			if choice == "discard" && (ran != 1 || a.savePromptOpen) {
				t.Fatal("discard did not continue")
			}
			if choice == "cancel" && (ran != 0 || a.savePromptOpen || !a.unsaved[0]) {
				t.Fatal("cancel continued or lost the change")
			}
		})
	}
}

// Both tracks are written through libse; a translation with missing cues is
// refused, and the original file is replaced in its own format.
func TestTracksAreWrittenInTheirFormats(t *testing.T) {
	executable := os.Getenv("ONESUB_SUBTITLE_WORKER")
	if executable == "" {
		t.Skip("build/test.ps1 runs the real subtitle worker")
	}
	a := draftApp(t)
	a.worker = subtitle.Client{Executable: executable, Scratch: t.TempDir()}
	if _, err := a.encodeTrack(context.Background(), a.snapshot.Document, 1, "srt"); err == nil {
		t.Fatal("untranslated track written")
	}
	a.draft = "校正原文"
	a.changed(time.Now())
	a.commitEdit(true)
	data, err := a.encodeTrack(context.Background(), a.snapshot.Document, 0, "srt")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "lesson.srt")
	if err = os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = subtitle.Replace(path, data); err != nil {
		t.Fatal(err)
	}
	read, err := a.worker.Read(context.Background(), path)
	if err != nil || read.Cues[0].Text != "校正原文" || len(read.Cues) != 2 {
		t.Fatal(read, err)
	}
}

func TestDefaultSaveNamesFollowTheSubtitleOrVideo(t *testing.T) {
	a := draftApp(t)
	a.snapshot.Manifest.SubtitlePath = filepath.Join("videos", "lesson.srt")
	a.savedPath = [2]string{}
	a.settings.TargetLanguage = "zh-CN"
	if got := a.defaultTrackPath(1); got != filepath.Join("videos", "lesson.zh-CN.srt") {
		t.Fatal(got)
	}
	a.snapshot.Manifest.SubtitlePath = filepath.Join(a.dataRoot, "cache", "worker", "asr-1", "result.srt")
	a.snapshot.Manifest.MediaPath = filepath.Join("videos", "talk.mp4")
	if a.originalSubtitlePath() != "" || a.defaultTrackPath(0) != filepath.Join("videos", "talk.srt") {
		t.Fatal("recognition output treated as the original file", a.defaultTrackPath(0))
	}
}

// Clearing asks once, offers saving when needed, and leaves no project open.
func TestClearWorkbenchConfirmsAndCloses(t *testing.T) {
	a := draftApp(t)
	a.requestClear()
	tt := settled(a.view, 1400, 900)
	if !a.clearOpen || !tt.HasText("清空工作台？") || !tt.HasText("清空") || tt.HasText("保存并清空") {
		t.Fatal(tt.Texts())
	}
	a.clearOpen = false
	a.draft = "修改"
	a.changed(time.Now())
	a.requestClear()
	tt.Frame()
	if !tt.HasText("保存并清空") || !tt.HasText("不保存并清空") {
		t.Fatal(tt.Texts())
	}
	if err := tt.Click("不保存并清空"); err != nil {
		t.Fatal(err)
	}
	if a.store != nil || a.snapshot != nil || a.mediaURL != "" || a.clearOpen {
		t.Fatal("workbench not cleared")
	}
	tt.Frame()
	if !tt.HasText("未加载字幕") {
		t.Fatal(tt.Texts())
	}
}
