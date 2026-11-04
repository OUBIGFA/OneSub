package settings

import (
	"os"
	"path/filepath"
	"testing"
)

// Adding an explicit route/manual choice must preserve old fixed-language
// workflows. Only the old "auto" source language becomes smart routing.
func TestRecognitionModeMigratesAndPersistsChoices(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	fresh, err := Load(path)
	if err != nil || fresh.RecognitionMode != "route" {
		t.Fatal("fresh settings must use smart routing", fresh.RecognitionMode, err)
	}
	for _, c := range []struct{ raw, mode, language string }{
		{`{"version":5,"language":"en","asrModel":"sherpa-parakeet-tdt-0.6b-v3-fp32"}`, "manual", "en"},
		{`{"version":5,"language":"zh"}`, "manual", "zh"},
		{`{"version":5,"language":"auto"}`, "route", "auto"},
		{`{"version":5,"recognitionMode":"route","language":"en"}`, "route", "en"},
		{`{"version":5,"recognitionMode":"manual","language":"auto","asrModel":"sherpa-sense-voice-small-int8"}`, "manual", "auto"},
	} {
		if err := os.WriteFile(path, []byte(c.raw), 0600); err != nil {
			t.Fatal(err)
		}
		prefs, err := Load(path)
		if err != nil || prefs.RecognitionMode != c.mode || prefs.Language != c.language {
			t.Fatal("recognition migration changed a saved workflow", c.raw, prefs.RecognitionMode, prefs.Language, err)
		}
		prefs.LanguageModels["en"] = "sherpa-parakeet-tdt-0.6b-v3-fp32"
		if err := Save(path, prefs); err != nil {
			t.Fatal(err)
		}
		restored, err := Load(path)
		if err != nil || restored.RecognitionMode != c.mode || restored.Language != c.language || restored.AsrModel != prefs.AsrModel || restored.LanguageModels["en"] != prefs.LanguageModels["en"] {
			t.Fatal("recognition choices did not survive saving and restarting", c.raw, restored.RecognitionMode, err)
		}
		if c.mode == "manual" && restored.ManualLanguages[restored.AsrModel] != c.language {
			t.Fatal("legacy manual source language was not retained per model", c.raw, restored.ManualLanguages)
		}
	}
}
