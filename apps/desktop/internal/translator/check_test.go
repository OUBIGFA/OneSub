package translator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
)

func TestPackagedSkillCheckerAndLibse(t *testing.T) {
	worker, skill, root := os.Getenv("ONESUB_SUBTITLE_WORKER"), os.Getenv("ONESUB_SKILL_WORKER"), os.Getenv("ONESUB_SKILL_ROOT")
	if worker == "" || skill == "" || root == "" {
		t.Fatal("Run build/test.ps1 with packaged workers")
	}
	client := subtitle.Client{Executable: worker, Scratch: t.TempDir()}
	checker := Checker{Executable: skill, SkillRoot: root, Scratch: t.TempDir(), Subtitle: client}
	path := filepath.Join(t.TempDir(), "source.srt")
	os.WriteFile(path, []byte("1\n00:00:00,000 --> 00:00:03,000\n<i>Hello</i>\n\n2\n00:00:04,000 --> 00:00:07,000\nGoodbye\n"), 0600)
	source, err := client.Read(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	candidate := source
	candidate.Cues = append([]subtitle.Cue{}, source.Cues...)
	candidate.Cues[0].Text = "<i>你好</i>"
	candidate.Cues[1].Text = "再见"
	findings, err := checker.Check(context.Background(), source, candidate, "zh-CN")
	if err != nil || len(findings.Errors) != 0 || findings.Blocks != 2 {
		t.Fatal(err, findings)
	}
	candidate.Cues[1].StartMs = 2000
	findings, err = checker.Check(context.Background(), source, candidate, "zh-CN")
	if err == nil && len(findings.Errors) == 0 {
		t.Fatal("overlap and changed timeline accepted")
	}
	candidate.Cues[1].StartMs = source.Cues[1].StartMs
	candidate.Cues[0].Text = "你好"
	findings, err = checker.Check(context.Background(), source, candidate, "zh-CN")
	if err == nil && len(findings.Errors) == 0 {
		t.Fatal("protected markup removal accepted")
	}
}
