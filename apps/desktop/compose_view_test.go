package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/media"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
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
