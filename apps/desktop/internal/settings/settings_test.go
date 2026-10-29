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

// Settings of the first test builds keep the user's choices; only the
// defaults they never changed move to the current ones.
func TestVersionOneDefaultsMoveToCurrentOnes(t *testing.T) {
	dir := t.TempDir()
	load := func(raw string) Settings {
		path := filepath.Join(dir, "settings.json")
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		s, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	old := `{"version":1,"volume":100,"stylePreset":"default","lastStyle":{"borderStyle":1,"shadow":1,"backOpacity":50,"outline":2},` +
		`"secondaryPreset":"classic","secondaryStyle":{"borderStyle":1,"shadow":1,"backOpacity":50}}`
	s := load(old)
	if s.Version != 3 || s.Volume != 50 || s.LastStyle.Shadow != 0 || s.LastStyle.BackOpacity != 0 || s.LastStyle.Outline != 2 {
		t.Fatalf("untouched defaults not moved: %+v %+v", s, *s.LastStyle)
	}
	if s.SecondaryStyle.Shadow != 1 || s.SecondaryStyle.BackOpacity != 50 {
		t.Fatal("a chosen preset's shadow was changed")
	}
	s = load(`{"version":1,"volume":80,"stylePreset":"default","lastStyle":{"borderStyle":1,"shadow":2,"backOpacity":50}}`)
	if s.Volume != 80 || s.LastStyle.Shadow != 2 {
		t.Fatal("a chosen value was changed")
	}
	if s = load(`{"version":2,"volume":100,"editorWidth":25,"styleWidth":66.66666666666667}`); s.Volume != 100 || s.EditorWidth != 20 || s.StyleWidth != 50 {
		t.Fatal("version 2 panes not moved to 1:2:2, or its volume changed", s.Volume, s.EditorWidth, s.StyleWidth)
	}
	if s = load(`{"version":3,"editorWidth":25,"styleWidth":66.66666666666667}`); s.EditorWidth != 25 {
		t.Fatal("current settings were upgraded again")
	}
}

// The pane sizes the user drags are kept; ones that would hide a pane are
// not.
func TestPaneSizesSurviveRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := Default()
	// The editor, the style and recognition with translation: 1:2:2.
	if style := (100 - s.EditorWidth) * s.StyleWidth / 100; style < 2*s.EditorWidth-0.1 || style > 2*s.EditorWidth+0.1 {
		t.Fatalf("editor and style are not 1:2 by default: %v %v", s.EditorWidth, s.StyleWidth)
	}
	s.PlayerWidth, s.TopHeight, s.EditorWidth = 52.5, 70, 95
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil || loaded.PlayerWidth != 52.5 || loaded.TopHeight != 70 || loaded.EditorWidth != Default().EditorWidth {
		t.Fatal(loaded.PlayerWidth, loaded.TopHeight, loaded.EditorWidth, err)
	}
}
