package settings

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestAutomaticUpdatesDefaultAndExplicitOptOutSurviveRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	for _, raw := range []string{`{"version":1}`, `{"version":5}`, `{"version":5,"autoUpdate":true}`, `{"version":5,"autoUpdate":false}`} {
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		s, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		want := raw != `{"version":5,"autoUpdate":false}`
		if s.AutoUpdate != want {
			t.Fatal("incorrect default or ignored opt-out", raw, s.AutoUpdate)
		}
		if err := Save(path, s); err != nil {
			t.Fatal(err)
		}
		loaded, err := Load(path)
		if err != nil || loaded.AutoUpdate != want {
			t.Fatal("automatic update choice lost on restart", raw, err)
		}
	}
}

func TestTranslationSchedulingDefaultsAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	for _, raw := range []string{`{"version":3}`, `{"version":3,"translationConcurrency":5,"translationBatchSize":7}`} {
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		s, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if err = Save(path, s); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var values map[string]any
		if err = json.Unmarshal(data, &values); err != nil {
			t.Fatal(err)
		}
		concurrency, batch := float64(2), float64(20)
		if raw != `{"version":3}` {
			concurrency, batch = 5, 7
		}
		if values["translationConcurrency"] != concurrency || values["translationBatchSize"] != batch {
			t.Fatal("translation scheduling lost", values)
		}
	}
}

func TestTranslationSchedulingRejectsInvalidValues(t *testing.T) {
	for _, raw := range []string{`{"version":3,"translationConcurrency":0}`, `{"version":3,"translationConcurrency":10001}`, `{"version":3,"translationBatchSize":0}`, `{"version":3,"translationBatchSize":10001}`, `{"version":3,"translationBatchSize":2.5}`} {
		path := filepath.Join(t.TempDir(), "settings.json")
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatal("invalid scheduling accepted", raw)
		}
		data, _ := os.ReadFile(path)
		if string(data) != raw {
			t.Fatal("invalid settings overwritten")
		}
	}
}

// Saved providers keep their own model and advertised limits after restart.
func TestSavedProvidersSurviveRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	raw := `{"version":3,"activeProvider":"b","providers":[{"id":"a","name":"Studio","url":"https://a.example/v1","api":"chat-completions","model":"a1"},{"id":"b","name":"Backup","url":"https://b.example/v1","api":"responses","model":"b1","models":[{"id":"b1","maxInputTokens":120000,"maxOutputTokens":8192,"contextWindow":128000}]}]}`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.ProviderURL != "https://b.example/v1" || s.ProviderModel != "b1" || s.ProviderAPI != "responses" {
		t.Fatalf("selected provider not restored: %s %s %s", s.ProviderURL, s.ProviderModel, s.ProviderAPI)
	}
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Providers []struct {
			Name   string
			Models []struct{ MaxInputTokens, MaxOutputTokens, ContextWindow int }
		}
	}
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.Providers) != 2 || saved.Providers[1].Name != "Backup" || len(saved.Providers[1].Models) != 1 || saved.Providers[1].Models[0].MaxInputTokens != 120000 || saved.Providers[1].Models[0].MaxOutputTokens != 8192 || saved.Providers[1].Models[0].ContextWindow != 128000 {
		t.Fatalf("provider data lost: %s", data)
	}
}

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
	if s.Version != version || s.Volume != 50 || s.LastStyle.Shadow != 0 || s.LastStyle.BackOpacity != 0 || s.LastStyle.Outline != 2 {
		t.Fatalf("untouched defaults not moved: %+v %+v", s, *s.LastStyle)
	}
	if s.SecondaryStyle.Shadow != 1 || s.SecondaryStyle.BackOpacity != 50 {
		t.Fatal("a chosen preset's shadow was changed")
	}
	s = load(`{"version":1,"volume":80,"stylePreset":"default","lastStyle":{"borderStyle":1,"shadow":2,"backOpacity":50}}`)
	if s.Volume != 80 || s.LastStyle.Shadow != 2 {
		t.Fatal("a chosen value was changed")
	}
	if s = load(`{"version":2,"volume":100,"editorWidth":25,"styleWidth":66.66666666666667}`); s.Volume != 100 || s.EditorWidth != Default().EditorWidth || s.SubtitleHeight != Default().SubtitleHeight {
		t.Fatal("version 2 panes not moved to 3:3:1, or its volume changed", s.Volume, s.EditorWidth, s.SubtitleHeight)
	}
	if s = load(`{"version":3,"editorWidth":20,"styleWidth":50}`); s.EditorWidth != Default().EditorWidth || s.SubtitleHeight != Default().SubtitleHeight {
		t.Fatal("older default was not upgraded", s.EditorWidth, s.SubtitleHeight)
	}
}

// The three-column layout takes effect once on an existing install.
// Other settings stay intact; later divider drags
// survive subsequent loads rather than resetting on every startup.
func TestReorderedPaneUpgradeAppliesOnlyOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	raw := `{"version":5,"editorWidth":45,"styleWidth":60,"playerWidth":52.5,"topHeight":70,"volume":80,"uiLanguage":"en","closeToTray":true,"modelPaths":{"local":"model-folder"}}`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil || s.Version != version || s.EditorWidth != Default().EditorWidth || s.SubtitleHeight != Default().SubtitleHeight {
		t.Fatal("new column layout was not applied", s.EditorWidth, s.SubtitleHeight, err)
	}
	if s.PlayerWidth != Default().PlayerWidth || s.TopHeight != Default().TopHeight || s.Volume != 80 || s.UILanguage != "en" || !s.CloseToTray || s.ModelPaths["local"] != "model-folder" {
		t.Fatal("layout upgrade changed unrelated preferences")
	}
	s.EditorWidth, s.SubtitleHeight = 60, 25
	if err = Save(path, s); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil || loaded.EditorWidth != 60 || loaded.SubtitleHeight != 25 {
		t.Fatal("new layout was reset on restart", loaded.EditorWidth, loaded.SubtitleHeight, err)
	}
}

// The pane sizes the user drags are kept; ones that would hide a pane are
// not.
func TestPaneSizesSurviveRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := Default()
	// The inner horizontal split divides the room right of the player.
	middle := (100 - s.PlayerWidth) * s.EditorWidth / 100
	workflow := 100 - s.PlayerWidth - middle
	if math.Abs(middle-s.PlayerWidth) > .01 || math.Abs(workflow*3-s.PlayerWidth) > .01 || s.TopHeight != 75 || s.SubtitleHeight != 75 {
		t.Fatal("three-column defaults differ from 3:3:1 and 3:1", s)
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
