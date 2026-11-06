package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
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

func TestSubtitleSaveAsIconAndFileSeparator(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := draftApp(t)
			var border ui.Color
			tt := settled(func(c *ui.Context) {
				border = c.Theme().Border
				a.subtitlesView(c)
			}, 1600, 600)
			group, found := tt.Find(locales.A11ySubtitleFileActions())
			save, _ := tt.Find(locales.ListSave())
			saveAs, _ := tt.Find(locales.ListSaveAs())
			next, _ := tt.Find(locales.ReadingNext())
			fileIcon, hasIcon := tt.Find(locales.A11yIconOf(locales.ListSaveAs()))
			if !found || group.X <= next.X+next.W || save.X <= group.X || !hasIcon || fileIcon.W <= 0 || fileIcon.X < saveAs.X || fileIcon.X+fileIcon.W > saveAs.X+saveAs.W {
				t.Fatal("save group separator or Save As icon missing", group, saveAs, fileIcon)
			}
			// A native Divider is a real vertical line, not blank padding.
			longest := 0
			frame := tt.Image()
			for x := int(group.X); x < int(save.X); x++ {
				run := 0
				for y := int(group.Y); y < int(group.Y+group.H); y++ {
					p := frame.RGBAAt(x, y)
					if p.R == border.R && p.G == border.G && p.B == border.B {
						run++
						longest = max(longest, run)
					} else {
						run = 0
					}
				}
			}
			if longest < 10 {
				t.Fatal("save group has no visible native separator", longest)
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
