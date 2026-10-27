package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/egoist/mygo/ui"
)

func nativeForTest(t *testing.T) *NativeApp {
	t.Helper()
	entries, err := iconFiles.ReadDir("icons")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := iconFiles.ReadFile("icons/" + e.Name())
		if err != nil {
			t.Fatal(err)
		}
		icons[strings.TrimSuffix(e.Name(), ".svg")] = ui.MustParseSVG(data)
	}
	return &NativeApp{dataRoot: t.TempDir(), settings: settings.Default(), splitX: 60, splitY: 64, bottomSplit: 42, selected: -1, paused: true, modelStates: map[string]string{}, exportFormat: "srt"}
}
func TestNativeWorkbenchUsesDirectActionsAndSettings(t *testing.T) {
	a := nativeForTest(t)
	tt := ui.NewTester(a.view, 1280, 800)
	for _, label := range []string{"添加文件", "识别字幕", "翻译字幕", "合成视频", "设置"} {
		if !tt.HasText(label) {
			t.Fatal("missing action", label)
		}
	}
	for _, label := range []string{"准备", "从视频或字幕开始", "创建工程并开始"} {
		if tt.HasText(label) {
			t.Fatal("unexpected preparation flow", label)
		}
	}
	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	if !a.settingsOpen || !tt.HasText("关闭窗口") || !tt.HasText("直接退出") {
		t.Fatal(tt.Texts())
	}
	if err := tt.Click("模型"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Qwen3-ASR 0.6B · 1.58 GB") {
		t.Fatal(tt.Texts())
	}
}
func TestNativeChineseEditWaitsForExplicitSave(t *testing.T) {
	a := nativeForTest(t)
	store, err := project.Create(filepath.Join(t.TempDir(), "edit.onesub"), "", "", subtitle.Document{Format: "srt", Cues: []subtitle.Cue{{ID: "c1", Text: "原文", OriginalText: "原文", StartMs: 0, EndMs: 1000}}})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	a.store = store
	a.snapshot = &state
	a.selected = 0
	a.loadDraft()
	tt := ui.NewTester(a.view, 1280, 800)
	if err = tt.Click("字幕文字"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("中文修改")
	tt.Advance(time.Second)
	before, err := store.Snapshot()
	if err != nil || before.Document.Cues[0].Text != "原文" || !a.dirty {
		t.Fatal("draft saved without confirmation", before, err)
	}
	if err = tt.Click("保存"); err != nil {
		t.Fatal(err)
	}
	saved, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if saved.Document.Cues[0].Text != "中文修改" || !saved.Document.Cues[0].TextLocked {
		t.Fatal(saved.Document.Cues[0], tt.Texts())
	}
}
