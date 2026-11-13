package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/translator"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// A search or playback follow must not hide the subtitle the missing-translation
// action selects. Repeated clicks visit the remaining cues in document order.
func TestInterruptedTranslationLocatesMissingCues(t *testing.T) {
	a := nativeForTest(t)
	a.subtitleTab = 1
	a.translationLive = newTranslationLive()
	live := a.translationLive
	live.stage, live.stopped, live.done, live.total = "translate", "Stopped", 58, 60
	for i := range 60 {
		live.units = append(live.units, subtitle.Cue{ID: fmt.Sprint(i), Text: fmt.Sprintf("Source subtitle %d", i+1), StartMs: int64(i * 2000), EndMs: int64((i + 1) * 2000)})
		live.texts[i], live.final[i] = "已完成译文", true
	}
	live.texts[20], live.final[20] = "未完成片段", false
	live.texts[59], live.final[59] = "", false
	a.query = "No matching subtitles"
	tt := settled(a.subtitlesView, 740, 600)
	for _, want := range []int{20, 59, 20} {
		if err := tt.Click("定位未翻译字幕"); err != nil {
			t.Fatal("missing-translation prompt has no locate action", err)
		}
		tt.Frame()
		if a.query != "" || live.list.Selected == nil || *live.list.Selected != want {
			t.Fatal("locate did not clear the filter and select the missing cue", a.query, live.list.Selected, want)
		}
		if !tt.HasText(fmt.Sprintf("Source subtitle %d", want+1)) {
			t.Fatal("selected missing cue is outside the visible list", want, tt.Texts())
		}
	}
}

func TestMissingLocatorUsesWorkingTrackAndRetainsTheSelection(t *testing.T) {
	a := storeApp(t,
		subtitle.Cue{ID: "a", Text: "已完成", Translated: true, TranslationSource: "First", StartMs: 0, EndMs: 2000},
		subtitle.Cue{ID: "b", Text: "Missing source", TextLocked: true, StartMs: 2100, EndMs: 4000})
	a.subtitleTab, a.position, a.paused, a.query = 0, 0, false, "No matches"
	a.locateMissingTranslation()
	if a.subtitleTab != 1 || a.selected != 1 || a.locateCue != 1 || !a.followHeld || a.query != "" || a.position != 0 || a.pendingClick != nil {
		t.Fatal("locate used the original track, playback follow, or queued a seek", a.subtitleTab, a.selected, a.followHeld)
	}
	live := newTranslationLive()
	live.units = []subtitle.Cue{{ID: "done", Text: "Source"}, {ID: "locked", Text: "Locked source", TextLocked: true}, {ID: "protected", Text: "Drawing", ReadOnly: true}}
	live.final[0], live.texts[0] = true, "已完成"
	if got := live.missingIndices(); !reflect.DeepEqual(got, []int{1}) {
		t.Fatal("missing count excluded a locked source or counted protected content", got)
	}
}

func TestSkippingTranslationPublishesCompletedCuesAndKeepsMissingSource(t *testing.T) {
	a := storeApp(t,
		subtitle.Cue{ID: "a", Text: "Hello world", StartMs: 0, EndMs: 2500},
		subtitle.Cue{ID: "b", Text: "Good evening", StartMs: 2600, EndMs: 5000},
		subtitle.Cue{ID: "c", Text: "Thank you", StartMs: 5100, EndMs: 7500})
	a.worker = subtitle.Client{Executable: os.Getenv("ONESUB_SUBTITLE_WORKER"), Scratch: t.TempDir()}
	original := a.snapshot.TrackDocument(0)
	source, _, err := subtitle.Resegment(a.snapshot.Document, []subtitle.SourceRange{{FirstAtom: "a", LastAtom: "a"}, {FirstAtom: "b", LastAtom: "b"}, {FirstAtom: "c", LastAtom: "c"}})
	if err != nil {
		t.Fatal(err)
	}
	result := translator.Result{Source: source, Document: source, Report: translator.Report{Translated: 2}}
	result.Document.Cues = append([]subtitle.Cue{}, source.Cues...)
	for index, text := range map[int]string{0: "你好，世界", 2: "谢谢"} {
		cue := &result.Document.Cues[index]
		cue.TranslationSource, cue.Text, cue.Translated = cue.Text, text, true
	}
	task := &translationTask{ProjectID: a.snapshot.Manifest.ID, ProjectPath: a.snapshot.Path, Revision: a.snapshot.Revision, Source: a.snapshot.Document, Config: translator.Config{TargetLanguage: "zh-CN"}, Target: filepath.Join(t.TempDir(), "lesson-zh.srt"), Format: "srt"}
	if err = writeTranslationRecord(task.ProjectPath, translationRecord{Task: task}, true); err != nil {
		t.Fatal(err)
	}
	if err = writeTranslationRecord(task.ProjectPath, translationRecord{Result: &result, Error: "One subtitle is unfinished"}, false); err != nil {
		t.Fatal(err)
	}
	a.translationTask, a.translationLive = task, newTranslationLive()
	apply, err := a.skipTranslationWork(task)(context.Background())
	if err != nil || apply == nil {
		t.Fatal("skip failed to validate completed translations", err)
	}
	a.finishTask(locales.ActionTranslate(), apply, err)
	if !reflect.DeepEqual(a.snapshot.TrackDocument(0), original) || a.snapshot.Revision != task.Revision+1 {
		t.Fatal("skip changed original subtitles or did not use one transaction")
	}
	missing := a.snapshot.Document.Cues[1]
	if len(a.snapshot.Document.Cues) != 3 || missing.Translated || missing.Text != "Good evening" {
		t.Fatal("skip deleted the missing source or marked it translated", a.snapshot.Document.Cues)
	}
	if a.lastOutput == "" || a.translationLive != nil || a.settings.TranslationRecovery != "" {
		t.Fatal("skip did not complete its output/recovery state", a.status, a.lastOutput)
	}
	back, err := a.worker.Read(context.Background(), a.lastOutput)
	if err != nil || len(back.Cues) != 2 || back.Cues[0].Text != "你好，世界" || back.Cues[1].Text != "谢谢" || back.Cues[1].StartMs != 5100 {
		t.Fatal("skip output contains unfinished source or changed retained timing", err, back.Cues)
	}
	_, _, _, finished, err := readTranslationProgress(task.ProjectPath)
	if err != nil || !finished {
		t.Fatal("skip did not close the translation journal", err)
	}
}

func TestTranslationJournalRetainsElapsedExecutionWithoutOfflineWait(t *testing.T) {
	path := t.TempDir()
	elapsed := 12345 * time.Millisecond
	if err := writeTranslationRecord(path, translationRecord{Task: &translationTask{ProjectPath: path}, Elapsed: &elapsed}, true); err != nil {
		t.Fatal(err)
	}
	_, live, _, _, err := readTranslationProgress(path)
	if err != nil || live.elapsed != elapsed {
		t.Fatal("translation elapsed time was not restored", err, live)
	}
	a := nativeForTest(t)
	a.taskTimes[timingTranslation] = &taskTiming{elapsed: live.elapsed, finished: true}
	tt := settled(a.view, 1280, 800)
	tt.Advance(24 * time.Hour)
	if !tt.HasText("字幕翻译 12 秒") {
		t.Fatal("offline wait extended the restored elapsed time", tt.Texts())
	}
}

// Export and composition must let users act on a missing-translation warning,
// while retaining the same dialog geometry when the skip choice is toggled.
func TestOutputMissingTranslationsHaveActions(t *testing.T) {
	for _, compose := range []bool{false, true} {
		t.Run(fmt.Sprint(compose), func(t *testing.T) {
			a := storeApp(t,
				subtitle.Cue{ID: "a", Text: "完整译文", Translated: true, TranslationSource: "Complete source", StartMs: 0, EndMs: 2000},
				subtitle.Cue{ID: "b", Text: "Missing source", StartMs: 2000, EndMs: 4000})
			a.settings.SubtitleOutput.Mode = "translation"
			a.exportOpen, a.composeOpen = !compose, compose
			build, action := a.exportView, locales.ExportConfirm()
			if compose {
				build, action = a.composeView, locales.ComposeStart()
			}
			tt := settled(func(c *ui.Context) { applyStudioTheme(c); build(c) }, 1000, 800)
			before, _ := tt.Find(action)
			for _, label := range []string{"定位未翻译字幕", locales.TranslationResume(), "跳过未翻译字幕"} {
				if !tt.HasText(label) {
					t.Fatal("missing-translation warning has no action", label, tt.Texts())
				}
			}
			if err := tt.Click("跳过未翻译字幕"); err != nil {
				t.Fatal(err)
			}
			if after, _ := tt.Find(action); before != after {
				t.Fatal("skip choice moved the dialog action", before, after)
			}
			if a.snapshot.Document.Cues[1].Translated || a.snapshot.Document.Cues[1].Text != "Missing source" {
				t.Fatal("skip changed or falsely completed the missing subtitle")
			}
		})
	}
}
