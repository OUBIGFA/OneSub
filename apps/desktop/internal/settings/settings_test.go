package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTrayAndPerModelPathsSurviveRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := Default()
	s.CloseToTray = true
	s.ModelPaths["Qwen3-ASR-0.6B-hf"] = "model-a"
	s.ModelPaths["Qwen3-ASR-1.7B-hf"] = "model-b"
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil || !loaded.CloseToTray || loaded.ModelPaths["Qwen3-ASR-1.7B-hf"] != "model-b" {
		t.Fatal(loaded, err)
	}
	loaded.CloseToTray = false
	if err = Save(path, loaded); err != nil {
		t.Fatal(err)
	}
	loaded, err = Load(path)
	if err != nil || loaded.CloseToTray {
		t.Fatal(loaded, err)
	}
}
func TestInvalidSettingsAreNotOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	raw := []byte("broken settings")
	os.WriteFile(path, raw, 0600)
	if _, err := Load(path); err == nil {
		t.Fatal("invalid settings accepted")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(raw) {
		t.Fatal("file overwritten")
	}
}
