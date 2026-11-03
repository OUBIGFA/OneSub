package models

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// One model folder is enough: every link goes through the same PowerShell
// junction call, which costs seconds per launch. The path keeps Chinese text,
// a space and shell metacharacters because quoting is what can break.
func TestIndependentRuntimeDirectoriesDoNotCopyOrDeleteWeights(t *testing.T) {
	scratch := t.TempDir()
	dir := filepath.Join(t.TempDir(), "识别 &' 模型")
	const id = "Qwen3-ASR-0.6B-hf"
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "marker"), []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	root, cleanup, err := RuntimeRoot(context.Background(), scratch, map[string]string{id: dir})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := os.Stat(filepath.Join(dir, "marker"))
	b, err := os.Stat(filepath.Join(root, "models", id, "marker"))
	if err != nil || !os.SameFile(a, b) {
		t.Fatal("not same physical file", err)
	}
	if err = cleanup(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "marker"))
	if err != nil || string(data) != "untouched" {
		t.Fatal("weights removed", err)
	}
	if _, err = os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("runtime root remains", err)
	}
}
