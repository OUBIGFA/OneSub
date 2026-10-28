package models

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestIndependentRuntimeDirectoriesDoNotCopyOrDeleteWeights(t *testing.T) {
	scratch := t.TempDir()
	first := filepath.Join(t.TempDir(), "识别 &' 模型")
	second := filepath.Join(t.TempDir(), "对齐模型")
	for _, dir := range []string{first, second} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "marker"), []byte("untouched"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	root, cleanup, err := RuntimeRoot(context.Background(), scratch, map[string]string{"Qwen3-ASR-0.6B-hf": first, "omniASR-CTC-300M-v2-hf": second})
	if err != nil {
		t.Fatal(err)
	}
	for id, dir := range map[string]string{"Qwen3-ASR-0.6B-hf": first, "omniASR-CTC-300M-v2-hf": second} {
		a, _ := os.Stat(filepath.Join(dir, "marker"))
		b, err := os.Stat(filepath.Join(root, "models", id, "marker"))
		if err != nil || !os.SameFile(a, b) {
			t.Fatal("not same physical file", err)
		}
	}
	if err = cleanup(); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{first, second} {
		data, err := os.ReadFile(filepath.Join(dir, "marker"))
		if err != nil || string(data) != "untouched" {
			t.Fatal("weights removed", err)
		}
	}
	if _, err = os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("runtime root remains", err)
	}
}
