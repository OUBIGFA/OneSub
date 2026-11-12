package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/media"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

func TestComposeDestinationAndDialogPreferences(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.mp4")
	path, err := composeOutputPath(source, "", "mp4", false)
	if err != nil || path != filepath.Join(root, "source-OneSub.mp4") {
		t.Fatal(path, err)
	}
	if err = os.WriteFile(path, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	next, err := composeOutputPath(source, "", "mp4", false)
	if err != nil || next != filepath.Join(root, "source-OneSub (2).mp4") {
		t.Fatal(next, err)
	}
	other := t.TempDir()
	sample, err := composeOutputPath(source, other, "mkv", true)
	if err != nil || sample != filepath.Join(other, "source-OneSub-preview.mkv") {
		t.Fatal(sample, err)
	}
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := nativeForTest(t)
			a.composeOpen = true
			tt := settled(a.composeView, 900, 640)
			before, _ := tt.Find(locales.ComposeStart())
			if err := tt.Click(locales.ComposeAskPath()); err != nil {
				t.Fatal(err)
			}
			after, _ := tt.Find(locales.ComposeStart())
			if before != after || after.Y+after.H > 640 {
				t.Fatal("output option resized dialog", before, after)
			}
			prefs, err := settings.Load(filepath.Join(a.dataRoot, "settings.json"))
			if err != nil || !prefs.ComposeAskPath {
				t.Fatal("output prompt not saved", err)
			}
		})
	}
}

func TestBackgroundComposeReturnsPlaybackToPlayer(t *testing.T) {
	a := nativeForTest(t)
	a.mediaURL = "local-test-video"
	a.busy, a.composeRunning = true, true
	a.cancel = func() {}
	a.composeProgress = media.ComposeProgress{Stage: media.StageEncode, Fraction: .42}
	tt := settled(a.playerView, 900, 600)
	if !tt.HasText(locales.ComposeBackground()) || !tt.HasText(locales.ComposeCancel()) {
		t.Fatal(tt.Texts())
	}
	if a.playbackAvailable() {
		t.Fatal("foreground playback was available before moving composition to the background")
	}
	if err := tt.Click(locales.ComposeBackground()); err != nil {
		t.Fatal(err)
	}
	if !a.composeInBackground || !a.playbackAvailable() {
		t.Fatal("background composition did not release the player", a.composeInBackground, a.playbackAvailable())
	}
	tt.Frame()
	if _, ok := tt.Find(locales.A11yComposeProgressArea()); ok {
		t.Fatal("the composition overlay still owns the player after moving to the background")
	}
	if _, ok := tt.Find(locales.A11yPlayerControls()); !ok {
		t.Fatal("native player controls did not return")
	}
}

func TestBackgroundComposeCanReturnFromTopBar(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := draftApp(t)
			a.snapshot.Manifest.MediaPath = "source.mp4"
			a.mediaURL = "local-test-video"
			a.busy, a.composeRunning, a.composeInBackground = true, true, true
			a.composeProgress = media.ComposeProgress{Stage: media.StageEncode, Fraction: .42}
			tt := settled(func(c *ui.Context) { a.topBar(c, applyStudioTheme(c)) }, 1800, 100)
			if !tt.HasText(locales.ComposeShowProgress()) || tt.HasText(locales.ActionCompose()) {
				t.Fatal(tt.Texts())
			}
			if err := tt.Click(locales.ComposeShowProgress()); err != nil {
				t.Fatal(err)
			}
			if a.composeInBackground {
				t.Fatal("top bar did not return to composition progress")
			}
			progress := settled(a.playerView, 900, 600)
			if !progress.HasText(locales.A11yComposeProgressArea()) {
				t.Fatal("composition progress overlay did not return", progress.Texts())
			}
		})
	}
}

func TestComposeReportStaysInRecoveryProject(t *testing.T) {
	project := t.TempDir()
	report := media.ComposeReport{Version: 1, Source: "source.mp4", Output: "result.mp4", SourceStart: .032989, OutputSHA256: "test-hash", Checks: []string{"full audiovisual decode"}}
	if err := saveComposeReport(project, report); err != nil {
		t.Fatal(err)
	}
	if err := saveComposeReport(project, report); err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(filepath.Join(project, "artifacts"))
	if err != nil || len(files) != 2 {
		t.Fatal(files, err)
	}
	for _, file := range files {
		raw, err := os.ReadFile(filepath.Join(project, "artifacts", file.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var got media.ComposeReport
		if err := json.Unmarshal(raw, &got); err != nil || got.SourceStart != report.SourceStart || got.OutputSHA256 != report.OutputSHA256 {
			t.Fatal(got, err)
		}
	}
}
