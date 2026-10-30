package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/media"
)

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
