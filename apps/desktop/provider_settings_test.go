package main

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
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
