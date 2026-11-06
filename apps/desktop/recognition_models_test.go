package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/models"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// An empty or partial model directory must not expose an unusable model name
// in recognition. Management retains all catalog choices so it can be fetched.
func TestManualRecognitionHidesUndownloadedModels(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := nativeForTest(t)
			a.downloadedAsr = nil
			a.settings.RecognitionMode = recognitionManual
			dir := filepath.Join(a.dataRoot, "models", models.FallbackModel)
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			os.WriteFile(filepath.Join(dir, "model.int8.onnx.partial"), []byte("unfinished"), 0600)
			tt := settled(a.workflowPanel, 450, 900)
			if !tt.HasText(locales.RecognitionNoDownloadedModel()) || tt.HasText("Qwen3-ASR") || tt.HasText("SenseVoiceSmall INT8") {
				t.Fatal("missing/partial models leaked into recognition", tt.Texts())
			}
			if err := tt.Click(locales.WorkflowRecognitionModel()); err != nil || tt.Focused(locales.WorkflowRecognitionModel()) {
				t.Fatal("empty recognition model list was active", err)
			}
			a.settingsOpen, a.settingsTab = true, 1
			tt = settled(a.view, 1400, 800)
			if !tt.HasText("Qwen3-ASR 0.6B") || !tt.HasText(locales.ModelManage()) {
				t.Fatal("model management lost the download catalog", tt.Texts())
			}
		})
	}
}

// The native selector consumes a filtered availability snapshot. First use of
// SenseVoice defaults to auto, an explicit language persists through returns,
// and a non-detecting model never inherits auto.
func TestDownloadedModelSelectionDefaultsAutoAndKeepsOverride(t *testing.T) {
	a := nativeForTest(t)
	const fp32 = "sherpa-parakeet-tdt-0.6b-v3-fp32"
	a.downloadedAsr = []option{{models.FallbackModel, "SenseVoiceSmall INT8"}, {fp32, "Parakeet-TDT 0.6B v3 FP32"}}
	a.settings.RecognitionMode = recognitionManual
	tt := settled(a.workflowPanel, 450, 1000)
	chooseModel := func(label string) {
		t.Helper()
		if err := tt.Click(locales.WorkflowRecognitionModel()); err != nil {
			t.Fatal(err)
		}
		if tt.HasText("Qwen3-ASR") || tt.HasText("Paraformer-zh") || tt.HasText("Parakeet-TDT 0.6B v3 INT8") {
			t.Fatal("selector contains an undownloaded model", tt.Texts())
		}
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	chooseModel("SenseVoiceSmall INT8")
	if a.settings.AsrModel != models.FallbackModel || a.settings.Language != languageAuto {
		t.Fatal("first detecting model choice did not default to auto", a.settings.AsrModel, a.settings.Language)
	}
	if err := tt.Click(locales.SettingsSourceLanguage()); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(locales.LanguageJapan() + "-JA"); err != nil {
		t.Fatal(err)
	}
	if a.settings.Language != "ja" {
		t.Fatal("explicit source-language choice was ignored", a.settings.Language)
	}
	chooseModel("Parakeet-TDT 0.6B v3 FP32")
	if a.settings.Language == languageAuto || a.settings.Language != "en" {
		t.Fatal("non-detecting model retained auto or an unsupported language", a.settings.Language)
	}
	chooseModel("SenseVoiceSmall INT8")
	if a.settings.Language != "ja" {
		t.Fatal("returning to the model reset the user's language", a.settings.Language)
	}
	// Mode changes use the same handler, but never erase a remembered override.
	a.settings.RecognitionMode = recognitionRouted
	a.asrModelChanged()
	a.settings.RecognitionMode = recognitionManual
	a.asrModelChanged()
	prefs, err := settings.Load(filepath.Join(a.dataRoot, "settings.json"))
	if err != nil || prefs.Language != "ja" || prefs.ManualLanguages[models.FallbackModel] != "ja" {
		t.Fatal("source-language override was lost on save/restart", prefs.Language, prefs.ManualLanguages, err)
	}
}

// Each route filters the same availability snapshot used by the workbench,
// then keeps only compatible languages. A stale saved ID must not be exposed.
func TestLanguageRouteMenusOnlyOfferDownloadedCompatibleModels(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			for _, route := range []struct{ group, title, id string }{
				{"zh", locales.LanguageChina() + "-ZH", "sherpa-paraformer-zh-int8"},
				{"en", locales.LanguageBritain() + "-EN", "sherpa-parakeet-tdt-0.6b-v3-fp32"},
				{"eu", locales.RouteGroupEu(), "sherpa-parakeet-tdt-0.6b-v3-fp32"},
			} {
				a := nativeForTest(t)
				a.downloadedAsr = []option{}
				for _, id := range []string{"sherpa-paraformer-zh-int8", "sherpa-parakeet-tdt-0.6b-v3-fp32"} {
					model, err := models.ByID(id)
					if err != nil {
						t.Fatal(err)
					}
					a.downloadedAsr = append(a.downloadedAsr, option{id, model.Label})
				}
				tt := settled(func(c *ui.Context) { a.routeField(c, route.group) }, 480, 200)
				if !tt.HasText(locales.RecognitionChooseDownloadedModel()) {
					t.Fatal("missing default route model is shown as usable", tt.Texts())
				}
				if err := tt.Click(route.title); err != nil {
					t.Fatal(err)
				}
				for _, model := range models.Catalog() {
					if model.Kind == "asr" && tt.HasText(model.Label) != (model.ID == route.id) {
						t.Fatal("route menu includes a missing/incompatible model or omits the usable one", route.group, model.ID)
					}
				}
				model, _ := models.ByID(route.id)
				if err := tt.Click(model.Label); err != nil {
					t.Fatal(err)
				}
				prefs, err := settings.Load(filepath.Join(a.dataRoot, "settings.json"))
				if err != nil || prefs.LanguageModels[route.group] != route.id {
					t.Fatal("downloaded route choice did not persist", err, prefs.LanguageModels)
				}
				// Refresh after a removal must hide the old choice without
				// discarding the preference if the user restores its files.
				a.refreshDownloadedAsrModels()
				tt.Frame()
				if !tt.HasText(locales.RecognitionNoDownloadedModel()) || tt.HasText(model.Label) || a.settings.LanguageModels[route.group] != route.id {
					t.Fatal("removal left a missing model visible or discarded the route choice")
				}
				if err := tt.Click(route.title); err != nil || tt.Focused(route.title) {
					t.Fatal("empty route model menu remained active", err)
				}
			}
		})
	}
}

func TestLanguageRoutesWithoutDownloadsKeepManagementAvailable(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := nativeForTest(t)
			a.refreshDownloadedAsrModels()
			a.settingsOpen, a.settingsTab = true, 5
			tt := settled(a.view, 1400, 800)
			if !tt.HasText(locales.RecognitionNoDownloadedModel()) {
				t.Fatal("missing language models have no download hint")
			}
			for _, group := range []string{locales.LanguageChina() + "-ZH", locales.LanguageCantonese() + "-YUE", locales.LanguageJapan() + "-JA", locales.LanguageKorea() + "-KO", locales.LanguageBritain() + "-EN", locales.RouteGroupEu(), locales.A11yModelOf(locales.ModelKindLid())} {
				if err := tt.Click(group); err != nil || tt.Focused(group) {
					t.Fatal("undownloaded language model selector is active", group, err)
				}
			}
			if err := tt.Click(locales.A11yManageModel(locales.ModelKindLid())); err != nil || !a.modelOpen {
				t.Fatal("download management is unavailable", err)
			}
			if err := tt.Click(locales.ModelCatalog()); err != nil {
				t.Fatal(err)
			}
			if len(tt.Menu()) != len(models.Catalog()) {
				t.Fatal("download manager lost the complete model catalog", tt.Menu())
			}
		})
	}
}

// When local inference assets are provided, discovery itself reads their real
// catalog filenames and lengths. No OneAsr, aligner or LID asset leaks into the
// ASR menu, and an external file change is reflected on the next refresh.
func TestDownloadedAsrDiscoveryUsesRealAssets(t *testing.T) {
	root := os.Getenv("ONESUB_SHERPA_TEST_MODELS")
	if root == "" {
		t.Skip("set ONESUB_SHERPA_TEST_MODELS to complete model folders")
	}
	a := nativeForTest(t)
	a.downloadedAsr = nil
	for _, model := range models.Catalog() {
		if model.Sherpa() {
			a.settings.ModelPaths[model.ID] = filepath.Join(root, model.ID)
		}
	}
	a.refreshDownloadedAsrModels()
	ids := []string{}
	for _, model := range a.downloadedAsrModels() {
		ids = append(ids, model.ID)
	}
	want := []string{models.FallbackModel, "sherpa-paraformer-zh-int8", "sherpa-parakeet-tdt-0.6b-v3-int8", "sherpa-parakeet-tdt-0.6b-v3-fp32"}
	if !slices.Equal(ids, want) {
		t.Fatal("real downloaded-model discovery differs from complete assets", ids)
	}
	// Changing a configured folder invalidates that choice on refresh without
	// touching or deleting any real model files.
	a.settings.ModelPaths[models.FallbackModel] = t.TempDir()
	a.refreshDownloadedAsrModels()
	if slices.ContainsFunc(a.downloadedAsrModels(), func(model option) bool { return model.ID == models.FallbackModel }) {
		t.Fatal("changed empty model folder stayed in the selector")
	}
}
