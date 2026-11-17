package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/models"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

func TestRecognitionHotwordRequirements(t *testing.T) {
	tools := toolset{dataRoot: t.TempDir()}
	prefs := settings.Default()
	prefs.Hotwords = "OneSub,字幕"
	for _, id := range []string{prefs.AsrModel, prefs.Aligner, models.FallbackModel} {
		if err := os.MkdirAll(tools.modelPath(prefs, id), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := tools.recognitionModels(prefs); err == nil || err.Error() != locales.ErrRouteModelMissing("SenseVoiceSmall") {
		t.Fatalf("missing hotword model: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tools.dataRoot, "models", "SenseVoiceSmall"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, language := range []string{"zh", "en"} {
		prefs.Language = language
		if _, _, err := tools.recognitionModels(prefs); err != nil {
			t.Fatal(language, err)
		}
	}
	for _, language := range []string{"ja", "yue", "auto"} {
		prefs.Language = language
		if _, _, err := tools.recognitionModels(prefs); err == nil {
			t.Fatal("unsupported hotword language accepted", language)
		}
	}
	prefs.Language, prefs.AsrModel = "zh", models.FallbackModel
	if _, _, err := tools.recognitionModels(prefs); err == nil {
		t.Fatal("Sherpa hotwords silently ignored")
	}
	prefs.Hotwords = "  "
	if _, _, err := tools.recognitionModels(prefs); err != nil {
		t.Fatal("blank hotwords changed ordinary recognition", err)
	}
}
