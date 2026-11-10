package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/models"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// A failed second track must not leave the first successful save dirty, or
// continue the closing action. Exercise the real serializer and file write.
func TestSaveTracksKeepsCompletedSaveWhenLaterTrackFails(t *testing.T) {
	a := draftApp(t)
	a.worker = subtitle.Client{Executable: os.Getenv("ONESUB_SUBTITLE_WORKER"), Scratch: t.TempDir()}
	a.savedPath = [2]string{filepath.Join(t.TempDir(), "original.srt"), filepath.Join(t.TempDir(), "translation.srt")}
	a.draft = "已修改原文"
	a.changed(time.Now())
	if !a.commitEdit(true) || !a.unsaved[0] {
		t.Fatal("source edit was not pending save")
	}
	continued := false
	a.busy = true
	apply, err := a.saveTracksWork([]int{0, 1}, false, func() { continued = true })(context.Background())
	if err == nil {
		t.Fatal("incomplete translation was saved")
	}
	a.finishTask(locales.TaskSaveSubtitles(), apply, err)
	doc, readErr := a.worker.Read(context.Background(), a.savedPath[0])
	if readErr != nil || doc.Cues[0].Text != "已修改原文" {
		t.Fatal("first track was not written", readErr)
	}
	if a.unsaved[0] || a.busy || continued || a.errorText == "" || a.status != locales.StatusFailed() {
		t.Fatalf("partial save state lost: dirty=%v busy=%v continued=%v status=%q error=%q", a.unsaved, a.busy, continued, a.status, a.errorText)
	}
	if _, err := os.Stat(a.savedPath[1]); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed track created a file", err)
	}
}

func TestCancelledTaskKeepsCompletedState(t *testing.T) {
	a := nativeForTest(t)
	a.busy, a.translationRunning = true, true
	a.cancel = func() {}
	a.finishTask("save", func() { a.lastOutput = "completed.srt"; a.status = "saved" }, context.Canceled)
	if a.lastOutput != "completed.srt" || a.busy || a.translationRunning || a.cancel != nil || a.status != locales.StatusCancelled() || a.errorText != "" {
		t.Fatal("cancellation discarded completed state or reported success", a.lastOutput, a.status, a.errorText)
	}
}

// Task reconciliation also runs after failure; a model verification failure
// must never supply the success callback used by verification and download.
func TestFailedModelTaskDoesNotMarkVerified(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		a := nativeForTest(t)
		m := models.Model{ID: "missing", Files: []models.File{{Name: "model.bin", Size: 10}}}
		ctx, stop := context.WithCancel(context.Background())
		if cancel {
			stop()
		}
		a.modelStates[m.ID] = locales.ModelStateVerified()
		err := models.Verify(ctx, m, t.TempDir())
		stop()
		if err == nil {
			t.Fatal("missing model verified")
		}
		apply, err := a.modelVerificationResult(m, err)
		a.finishTask("verify", apply, err)
		if a.modelStates[m.ID] == locales.ModelStateVerified() {
			t.Fatal("failed verification marked model verified")
		}
	}
}

// A storage failure is not an invalid draft. Every action that can replace
// the editor must keep its text and stop until the draft can be committed.
func TestStorageFailureKeepsDraftAndBlocksLeaving(t *testing.T) {
	for _, action := range []string{"commit", "select", "tab", "clear", "export", "guard", "follow", "playback"} {
		t.Run(action, func(t *testing.T) {
			a := draftApp(t)
			a.draft = "仍未写入的修改"
			a.changed(time.Now())
			if err := a.store.Close(); err != nil {
				t.Fatal(err)
			}
			switch action {
			case "commit":
				if a.commitEdit(true) {
					t.Error("failed database write reported success")
				}
			case "select":
				a.selectSubtitle(1, "none")
			case "tab":
				a.changeSubtitleTab(1)
			case "clear":
				a.requestClear()
			case "export":
				a.requestExport()
			case "guard":
				if !a.guardUnsaved(func() { t.Error("continued after a failed edit") }) {
					t.Error("closing action was allowed")
				}
			case "follow":
				a.position = 2.5
				a.locateCurrent()
			case "playback":
				a.position, a.paused = 2.5, false
				settled(a.view, 1400, 900)
			}
			if a.draft != "仍未写入的修改" || !a.editPending || a.selected != 0 || a.subtitleTab != 0 || a.clearOpen || a.exportOpen || a.errorText == "" {
				t.Fatalf("failed write lost the draft: text=%q pending=%v selected=%d tab=%d clear=%v export=%v error=%q", a.draft, a.editPending, a.selected, a.subtitleTab, a.clearOpen, a.exportOpen, a.errorText)
			}
		})
	}
}

func TestDraftCanBeRetriedAfterStorageRecovers(t *testing.T) {
	a := draftApp(t)
	a.draft = "恢复后写入的修改"
	a.changed(time.Now())
	if err := a.store.Close(); err != nil {
		t.Fatal(err)
	}
	if a.commitEdit(false) || a.errorText == "" || !a.editDue.IsZero() {
		t.Fatal("background save hid the failure or kept retrying every frame")
	}
	s, err := project.Open(a.snapshot.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a.store = s
	if !a.commitEdit(true) || a.editPending || a.errorText != "" {
		t.Fatal("draft could not be retried after recovery", a.errorText)
	}
	state, err := s.Snapshot()
	if err != nil || state.TrackDocument(0).Cues[0].Text != "恢复后写入的修改" || !a.unsaved[0] {
		t.Fatal("recovered edit was not persisted", err)
	}
}

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
			if !tt.HasText("是否保存字幕修改？") || !tt.HasText("原始字幕：覆盖 original.srt") {
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

// Automatic saves, Save and Save As share encodeTrack: translations must
// contain only translated text, regardless of preview/export preferences.
func TestTranslationSavesOnlyTranslatedText(t *testing.T) {
	a := draftApp(t)
	a.worker = subtitle.Client{Executable: os.Getenv("ONESUB_SUBTITLE_WORKER"), Scratch: t.TempDir()}
	doc := a.snapshot.Document
	doc.Cues = append([]subtitle.Cue{}, doc.Cues...)
	for i := range doc.Cues {
		doc.Cues[i].TranslationSource = doc.Cues[i].Text
		doc.Cues[i].Text, doc.Cues[i].Translated = "Translation only", true
	}
	a.settings.SubtitleOutput.Mode = "bilingual"
	for _, format := range []string{"srt", "vtt"} {
		data, err := a.encodeTrack(context.Background(), doc, 1, format)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "translated."+format)
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		read, err := a.worker.Read(context.Background(), path)
		if err != nil || len(read.Cues) != 2 {
			t.Fatal("saved translation could not be read", read, err)
		}
		for _, cue := range read.Cues {
			if cue.Text != "Translation only" || strings.ContainsAny(cue.Text, "\r\n") {
				t.Fatal("translation save included source or a second line", cue)
			}
		}
	}
	before := trackHash(doc, 1)
	doc.Cues[0].TranslationSource = "Corrected rebuilt source"
	if trackHash(doc, 1) != before {
		t.Fatal("source-only edit changed the translation file digest")
	}
	doc.Cues[0].Text = "Edited translation"
	if trackHash(doc, 1) == before {
		t.Fatal("translated text edit did not change the file digest")
	}
}

func TestDefaultSaveNamesFollowTheSubtitleOrVideo(t *testing.T) {
	a := draftApp(t)
	a.snapshot.Manifest.SubtitlePath = filepath.Join("videos", "lesson.srt")
	a.savedPath = [2]string{}
	a.settings.TargetLanguage = "zh-CN"
	if got := a.defaultTrackPath(1); got != filepath.Join("videos", "lesson-zh.srt") {
		t.Fatal(got)
	}
	a.snapshot.Manifest.SubtitlePath = filepath.Join(a.dataRoot, "cache", "worker", "asr-1", "result.srt")
	a.snapshot.Manifest.MediaPath = filepath.Join("videos", "talk.mp4")
	if a.originalSubtitlePath() != "" || a.defaultTrackPath(0) != filepath.Join("videos", "talk.srt") {
		t.Fatal("recognition output treated as the original file", a.defaultTrackPath(0))
	}
}

// Save and Unsaved are two labels of one native button. Neither an edit nor
// undo may resize its face or move the neighbouring Save As action.
func TestSubtitleSaveStatusKeepsOneButtonWidth(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			for _, dark := range []bool{false, true} {
				for _, scale := range []float32{1, 1.5, 2} {
					for _, width := range []int{320, 640} {
						t.Run(fmt.Sprintf("%s/dark%t/%g/%d", language, dark, scale, width), func(t *testing.T) {
							a := draftApp(t)
							tt := settled(a.subtitlesView, int(float32(width)*scale), int(600*scale))
							tt.SetDark(dark)
							tt.SetPreferences(ui.Preferences{TextScale: scale, ReduceMotion: true})
							before, found := tt.Find(locales.ListSave())
							beforeAs, foundAs := tt.Find(locales.ListSaveAs())
							if !found || !foundAs {
								t.Fatal("save actions missing")
							}
							a.draft = "手工校正"
							a.changed(tt.Now())
							tt.Frame()
							after, _ := tt.Find(locales.ListSave())
							afterAs, _ := tt.Find(locales.ListSaveAs())
							if before != after || beforeAs != afterAs || !tt.HasText(locales.ListUnsaved()) {
								t.Fatal("dirty state resized the save face or moved Save As", before, after, beforeAs, afterAs)
							}
							a.undo(false)
							tt.Frame()
							restored, _ := tt.Find(locales.ListSave())
							if restored != before || a.trackUnsaved(0) || tt.HasText(locales.ListUnsaved()) {
								t.Fatal("undo did not restore the same saved button")
							}
							captureRedesign(t, tt, fmt.Sprintf("save-width-%s-dark%t-%g-%d", language, dark, scale, width))
						})
					}
				}
			}
		})
	}
}

func TestSubtitleSaveAsIconWithoutFileSeparator(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := draftApp(t)
			tt := settled(a.subtitlesView, 1600, 600)
			group, found := tt.Find(locales.A11ySubtitleFileActions())
			save, _ := tt.Find(locales.ListSave())
			saveAs, _ := tt.Find(locales.ListSaveAs())
			fileIcon, hasIcon := tt.Find(locales.A11yIconOf(locales.ListSaveAs()))
			// Editing is on a separate row; its adaptive width must not
			// constrain the horizontal position of these file actions.
			if !found || save.X != group.X || saveAs.X < save.X+save.W || !hasIcon || fileIcon.W <= 0 || fileIcon.X < saveAs.X || fileIcon.X+fileIcon.W > saveAs.X+saveAs.W {
				t.Fatal("save group retains a leading separator or lost Save As icon", group, save, saveAs, fileIcon)
			}
			captureRedesign(t, tt, "subtitle-toolbar-"+language)
		})
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

// The save choice grows to hold its three buttons: each name stays on one line
// and all three share a row, in both languages and a narrow window.
func TestSaveChoiceButtonsStayOnOneRow(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			for _, width := range []int{1400, 760} {
				a := draftApp(t)
				a.draft = "修改"
				a.changed(time.Now())
				a.requestClear()
				tt := settled(a.view, width, 600)
				labels := []string{locales.ActionCancel(), locales.ClearDiscard(), locales.ClearSave()}
				first, ok := tt.Find(labels[0])
				if !ok {
					t.Fatal(tt.Texts())
				}
				for _, label := range labels {
					box, found := tt.Find(label)
					if !found || box.Y != first.Y || box.H != first.H || box.X+box.W > float32(width) {
						t.Fatal("button wrapped or overflowed", language, width, label, box, first)
					}
				}
			}
		})
	}
}
