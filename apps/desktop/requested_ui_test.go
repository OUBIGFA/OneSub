package main

import (
	"context"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/media"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

func TestSubtitleCommandsShareSearchRow(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := draftApp(t)
			a.unsaved[0] = true
			a.translationCandidate = &translationCandidate{}
			tt := settled(a.subtitlesView, 1100, 600)
			search, _ := tt.Find(locales.ListSearch())
			for _, label := range []string{locales.ListImport(), locales.SubtitleActionsMenu(), locales.ListLocate(), locales.TranslationResults(), locales.ListUnsaved(), locales.ListSave(), locales.ListSaveAs()} {
				r, ok := tt.Find(label)
				if !ok || math.Abs(float64(r.Y+r.H/2-search.Y-search.H/2)) > 1 {
					t.Fatal("subtitle command is not on the search row", label, r, search)
				}
			}
		})
	}
}

func TestRawSubtitlesDoNotPopulateOptimizedView(t *testing.T) {
	a := storeApp(t, subtitle.Cue{ID: "raw", Text: "Only the imported source", EndMs: 2000})
	a.changeSubtitleTab(1)
	tt := settled(a.subtitlesView, 800, 600)
	if len(a.activeDocument().Cues) != 0 || !tt.HasText(locales.ListRebuiltEmpty()) || tt.HasText("Only the imported source") {
		t.Fatal("raw cues were presented as an optimized result", tt.Texts())
	}
	label, _ := tt.Find(locales.ListRebuiltEmpty())
	commands, _ := tt.Find(locales.A11ySubtitleActions())
	if math.Abs(float64(label.X+label.W/2-400)) > 1 || math.Abs(float64(label.Y+label.H/2-(commands.Y+commands.H+600)/2)) > 1 {
		t.Fatal("empty optimized message is not centered", label, commands)
	}
	a.changeSubtitleTab(0)
	tt.Frame()
	if !tt.HasText("Only the imported source") || len(a.snapshot.Document.Cues) != 1 {
		t.Fatal("hiding the working view removed original subtitles")
	}
}

func TestSubtitleTextFillsTableAfterColumnResize(t *testing.T) {
	text := strings.Repeat("This subtitle uses the remaining table width. ", 15)
	a := storeApp(t, subtitle.Cue{ID: "wide", Text: text, EndMs: 2000})
	// A stale manual width must not leave the last column and its text narrow.
	a.list.Columns.Widths = map[string]float32{locales.ListColumnText(): 80}
	tt := settled(a.subtitlesView, 900, 650)
	list, _ := tt.Find(locales.A11ySubtitleList())
	body, ok := tt.Find(text)
	if !ok || list.X+list.W-body.X-body.W > 16 || body.X+body.W > list.X+list.W {
		t.Fatal("subtitle body left unused table width", body, list)
	}
	nav, _ := tt.Find(locales.ListTop())
	if nav.X < list.X+list.W {
		t.Fatal("navigation covers the subtitle table", nav, list)
	}
}

func TestRecognitionTargetAndTransportOrder(t *testing.T) {
	for _, width := range []int{360, 900} {
		a := nativeForTest(t)
		tt := settled(a.workflowPanel, width, 800)
		group, _ := tt.Find(locales.RecognitionTarget())
		if math.Abs(float64(group.X+group.W/2-float32(width)/2)) > 1 {
			t.Fatal("recognition target is not centered", group)
		}
	}
	a := nativeForTest(t)
	a.position, a.duration = 123, 456
	tt := settled(a.playerView, 1000, 600)
	stamp, _ := tt.Find(stamp(123))
	play, _ := tt.Find(locales.PlayerPlay())
	if stamp.X+stamp.W >= play.X || math.Abs(float64(stamp.Y+stamp.H/2-play.Y-play.H/2)) > 1 {
		t.Fatal("time and transport groups have not swapped", stamp, play)
	}
}

func TestSettingsUseOneLargerFixedSidebar(t *testing.T) {
	a := nativeForTest(t)
	a.settingsOpen = true
	tt := settled(a.settingsView, 1800, 1100)
	shell, _ := tt.Find(locales.SettingsTitle())
	content, _ := tt.Find(locales.A11ySettingsContent())
	navigation, _ := tt.Find(locales.A11ySettingsTabs())
	if shell.W <= 1120 || shell.H <= 640 || navigation.X+navigation.W >= content.X || navigation.H < content.H-1 {
		t.Fatal("settings did not use the enlarged sidebar layout", shell, navigation, content)
	}
	for tab := 0; tab < 5; tab++ {
		a.settingsTab = tab
		tt.Frame()
		before, _ := tt.Find(locales.A11ySettingsContent())
		for range 4 {
			tt.Frame()
			after, _ := tt.Find(locales.A11ySettingsContent())
			if after != before || after != content {
				t.Fatal("settings page changed its fixed content region", tab, before, after)
			}
		}
	}
}

func TestUncheckedModelCannotBeUsedUntilEnabled(t *testing.T) {
	a := modelApp(t)
	tt := settled(a.providerModelsView, 700, 500)
	if !tt.HasText(locales.TranslationModelUnused()) {
		t.Fatal("unchecked model has no unused status")
	}
	if err := tt.Click(locales.TranslationUseModelNamed("model-b")); err != nil {
		t.Fatal(err)
	}
	if a.settings.ProviderModel != "model-a" {
		t.Fatal("unchecked model was selectable")
	}
	if err := tt.Click(locales.TranslationKeepModel("model-b")); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(locales.TranslationUseModelNamed("model-b")); err != nil || a.settings.ProviderModel != "model-b" {
		t.Fatal("enabled model could not be selected", err)
	}
}

func TestTimeFieldsFitLongValuesAndRememberEdits(t *testing.T) {
	a := storeApp(t, subtitle.Cue{ID: "long", Text: "Late subtitle", StartMs: 123456789, EndMs: 123467891})
	tt := settled(a.editorView, 500, 650)
	for _, label := range []string{locales.EditorStart(), locales.EditorEnd()} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Key(ui.Cmd, ui.KeyA)
		value := "34:17:36.789"
		if label == locales.EditorEnd() {
			value = "34:17:47.891"
		}
		tt.Type(value)
	}
	if !a.commitEdit(true) || a.snapshot.SourceDocument.Cues[0].StartMs != 123456789 || a.snapshot.SourceDocument.Cues[0].EndMs != 123467891 {
		t.Fatal("complete times lost their precision")
	}
}

func TestCompressionEntryDoesNotRequireSubtitles(t *testing.T) {
	a := nativeForTest(t)
	a.snapshot = &project.Snapshot{Manifest: project.Manifest{MediaPath: "source.mp4"}}
	tt := settled(a.view, 1800, 1000)
	compress, _ := tt.Find(locales.ActionCompress())
	compose, _ := tt.Find(locales.ActionCompose())
	if compress.X+compress.W > compose.X {
		t.Fatal("compression is not immediately before video export", compress, compose)
	}
	if err := tt.Click(locales.ActionCompress()); err != nil || !a.compressOpen {
		t.Fatal("subtitle-free video could not open compression", err)
	}
	if tt.HasText(locales.OutputContent()) || !tt.HasText(locales.CompressStart()) {
		t.Fatal("compression still requires subtitle output options", tt.Texts())
	}
}

func TestIndependentSubtitleImportKeepsUnsavedGuard(t *testing.T) {
	a := draftApp(t)
	a.snapshot.Manifest.MediaPath = "current.mp4"
	a.unsaved[0] = true
	before := a.snapshot
	a.importSubtitle("different-zh.srt")
	if !a.savePromptOpen || a.snapshot != before || a.snapshot.Manifest.MediaPath != "current.mp4" {
		t.Fatal("subtitle replacement bypassed unsaved protection")
	}
}

func TestExplicitNonMatchingSubtitleImportIntegration(t *testing.T) {
	ffmpeg, probe, worker := os.Getenv("ONESUB_FFMPEG"), os.Getenv("ONESUB_FFPROBE"), os.Getenv("ONESUB_SUBTITLE_WORKER")
	if worker == "" || ffmpeg == "" || probe == "" {
		t.Skip("build/test.ps1 enables the real import workers")
	}
	a := nativeForTest(t)
	a.probe = probe
	a.worker = subtitle.Client{Executable: worker, Scratch: t.TempDir()}
	server, err := media.NewServer()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	a.preview = server
	folder := t.TempDir()
	video := filepath.Join(folder, "lesson.mp4")
	if out, err := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "color=size=320x180:rate=30:duration=2", "-c:v", "libx264", video).CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	file := filepath.Join(folder, "different-zh.srt")
	original := "1\n00:00:00,100 --> 00:00:01,900\nTranslated subtitle for editing\n"
	if err := os.WriteFile(file, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	a.settings.SubtitleOutput = subtitle.OutputOptions{Mode: "bilingual"}
	apply, err := a.importFiles(context.Background(), []string{file, video})
	if err != nil {
		t.Fatal(err)
	}
	apply()
	defer a.store.Close()
	if a.snapshot.Manifest.MediaPath != video || a.snapshot.Manifest.SubtitlePath != file || a.activeDocument().Cues[0].Text != "Translated subtitle for editing" || a.candidatesOpen {
		t.Fatal("explicit subtitle was not paired with the selected video", a.snapshot.Manifest)
	}
	if _, err := subtitle.SelectOutput(a.snapshot.OutputDocument(a.settings.SubtitleOutput), a.settings.SubtitleOutput); err != nil {
		t.Fatal("imported subtitle cannot be composed without another translation", err)
	}
	a.draft = "Edited translated subtitle"
	a.editPending = true
	if !a.commitEdit(true) || a.activeDocument().Cues[0].Text != a.draft || !a.trackUnsaved(0) {
		t.Fatal("imported subtitle could not be edited")
	}
	if raw, err := os.ReadFile(file); err != nil || string(raw) != original {
		t.Fatal("editing silently overwrote the imported file", err)
	}
}
