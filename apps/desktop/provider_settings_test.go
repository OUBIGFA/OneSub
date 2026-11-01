package main

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/translator"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

func TestProviderSwitchKeepsEachModelAndItsLimits(t *testing.T) {
	a := nativeForTest(t)
	a.addProvider()
	first := a.settings.ActiveProvider
	a.settings.Provider().Name = "First"
	a.settings.ProviderURL, a.settings.ProviderModel = "https://example.com/v1", "first-model"
	a.settings.SyncProvider()
	a.settings.ModelInfo().MaxInputTokens = 96000
	a.settings.ModelInfo().MaxOutputTokens = 12000
	a.saveSettings()
	firstScope := a.settings.CredentialScope()
	a.addProvider()
	second := a.settings.ActiveProvider
	a.settings.Provider().Name = "Second"
	a.settings.ProviderURL, a.settings.ProviderModel, a.settings.ProviderAPI = "https://example.com/v1", "second-model", "responses"
	a.settings.SyncProvider()
	a.settings.ModelInfo().ContextWindow = 32000
	a.saveSettings()
	if a.settings.CredentialScope() == firstScope {
		t.Fatal("two provider accounts share a key")
	}
	a.providerKey = "unsaved-test-value"
	a.providerSelectionChanged(first)
	if a.settings.ProviderModel != "first-model" || a.settings.ModelInfo().MaxInputTokens != 96000 || a.settings.ModelInfo().MaxOutputTokens != 12000 || a.providerKey != "" {
		t.Fatal("provider switch mixed settings")
	}
	a.providerSelectionChanged(second)
	loaded, err := settings.Load(filepath.Join(a.dataRoot, "settings.json"))
	if err != nil || loaded.ProviderModel != "second-model" || loaded.ProviderAPI != "responses" || loaded.ModelInfo().ContextWindow != 32000 {
		t.Fatal("provider not restored", err, loaded)
	}
}

// The models a provider offers are shown as a list, and several of them can
// be kept at once, so the user switches between them without asking again.
func TestProviderModelListKeepsSeveralModelsAndSwitches(t *testing.T) {
	a := nativeForTest(t)
	a.addProvider()
	a.settings.ProviderURL = "https://example.com/v1"
	a.settings.SyncProvider()
	p := a.settings.Provider()
	p.Catalog = []translator.ModelInfo{
		{ID: "model-a", MaxInputTokens: 96000, MaxOutputTokens: 12000, ContextWindow: 128000},
		{ID: "model-b", MaxInputTokens: 32000, LimitsSource: "manual"},
	}
	a.keepProviderModel("model-a")
	a.keepProviderModel("model-b")
	if len(a.settings.Provider().Models) != 2 {
		t.Fatal("models were not kept", a.settings.Provider().Models)
	}
	if a.settings.ProviderModel != "model-b" {
		t.Fatal("the model in use did not follow the last kept", a.settings.ProviderModel)
	}
	// The limits a user entered by hand survive a new catalog.
	a.keepProviderModel("model-b")
	if a.settings.Provider().Models[1].MaxInputTokens != 32000 {
		t.Fatal("manual limits were overwritten", a.settings.Provider().Models[1])
	}
	a.settings.ProviderModel = "model-a"
	a.settings.SyncProvider()
	a.saveSettings()
	loaded, err := settings.Load(filepath.Join(a.dataRoot, "settings.json"))
	if err != nil || loaded.ProviderModel != "model-a" || len(loaded.Provider().Models) != 2 {
		t.Fatal("the kept models were not restored", err, loaded.ProviderModel)
	}
	// The model in use cannot be taken out; another one can.
	a.dropProviderModel("model-a")
	if len(a.settings.Provider().Models) != 2 {
		t.Fatal("the model in use was taken out", a.settings.Provider().Models)
	}
	a.dropProviderModel("model-b")
	if len(a.settings.Provider().Models) != 1 {
		t.Fatal("the model was not taken out", a.settings.Provider().Models)
	}
}

// The models the endpoint offers are visible in the service settings, not
// only as suggestions in a field.
func TestProviderSettingsShowTheModelList(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := nativeForTest(t)
			a.addProvider()
			a.settings.ProviderURL = "https://example.com/v1"
			a.settings.SyncProvider()
			a.settings.Provider().Catalog = []translator.ModelInfo{{ID: "model-visible"}, {ID: "model-other"}}
			a.keepProviderModel("model-visible")
			a.settingsOpen, a.settingsTab = true, 2
			tt := settled(a.view, 1100, 700)
			for _, label := range []string{locales.TranslationModels(), "model-visible", "model-other", locales.TranslationModelInUse()} {
				if !tt.HasText(label) {
					t.Fatal("the model list is not shown", language, label)
				}
			}
		})
	}
}

func TestEditorAndStyleTitlesAreCentered(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := withStyleCatalog(t)
			for _, width := range []int{900, 1500} {
				tt := settled(a.view, width, 900)
				for _, pair := range [][2]string{{locales.EditorTitle(), locales.A11yEditorPanel()}, {locales.StyleTitle(), locales.A11yStylePanel()}} {
					title, ok := tt.Find(pair[0])
					panel, found := tt.Find(pair[1])
					if !ok || !found || math.Abs(float64(title.X+title.W/2-panel.X-panel.W/2)) > 2 {
						t.Fatal("title is not centered", language, width, title, panel)
					}
				}
			}
		})
	}
}

func TestProviderSettingsKeepSizeAndVisibleControls(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := nativeForTest(t)
			a.settingsOpen, a.settingsTab = true, 2
			tt := settled(a.view, 900, 640)
			before, _ := tt.Find(locales.A11ySettingsContent())
			a.addProvider()
			a.settings.ProviderModel = "model"
			a.settings.SyncProvider()
			a.settings.ModelInfo().ContextWindow = 128000
			a.modelsError = "service unavailable"
			tt.Frame()
			after, _ := tt.Find(locales.A11ySettingsContent())
			if before != after {
				t.Fatal("provider state resized settings", before, after)
			}
			if tt.HasText(locales.TranslationConcurrency()) || tt.HasText(locales.TranslationBatchSize()) {
				t.Fatal("task scheduling still appears in service settings")
			}
			for _, label := range []string{locales.TranslationProviderName(), locales.TranslationMaxInput(), locales.TranslationMaxOutput(), locales.TranslationContextWindow(), locales.ActionClose()} {
				box, ok := tt.Find(label)
				if !ok || box.W == 0 || box.H == 0 || box.Y < 0 || box.Y+box.H > 640 {
					t.Fatal("setting clipped", language, label, box)
				}
			}
		})
	}
}
