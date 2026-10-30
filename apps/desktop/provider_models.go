package main

import (
	"context"
	"fmt"
	"slices"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/security"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/translator"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
	"github.com/google/uuid"
)

func (a *NativeApp) fetchProviderModels() {
	if a.modelsLoading || a.win == nil || a.settings.ProviderURL == "" {
		return
	}
	endpoint := a.settings.ProviderURL
	providerID := a.settings.ActiveProvider
	a.modelsGeneration++
	generation := a.modelsGeneration
	a.modelsEndpoint = endpoint
	key, err := security.ReadKey(a.settings.CredentialScope())
	if err != nil {
		a.modelsError = err.Error()
		return
	}
	a.modelsLoading = true
	a.modelsError = ""
	go func() {
		items, err := translator.ModelCatalog(context.Background(), endpoint, key, nil)
		a.win.Update(func() {
			if a.settings.ProviderURL != endpoint || a.settings.ActiveProvider != providerID || a.modelsGeneration != generation {
				return
			}
			a.modelsLoading = false
			if err != nil {
				a.modelsError = err.Error()
				return
			}
			a.modelsError = ""
			a.providerModels = nil
			for _, item := range items {
				a.providerModels = append(a.providerModels, item.ID)
			}
			if p := a.settings.Provider(); p != nil {
				for _, item := range items {
					i := slices.IndexFunc(p.Models, func(old translator.ModelInfo) bool { return old.ID == item.ID })
					if i < 0 {
						p.Models = append(p.Models, item)
					} else if p.Models[i].LimitsSource != "manual" {
						p.Models[i] = item
					}
				}
				a.saveSettings()
			}
		})
	}()
}

func (a *NativeApp) providerSelectionChanged(id string) {
	if !a.settings.SelectProvider(id) {
		return
	}
	a.modelsGeneration++
	a.providerKey, a.keyStatus, a.modelsError, a.modelsEndpoint = "", "", "", ""
	a.modelsLoading = false
	a.providerModels = nil
	if p := a.settings.Provider(); p != nil {
		for _, model := range p.Models {
			a.providerModels = append(a.providerModels, model.ID)
		}
	}
	a.saveSettings()
}

func (a *NativeApp) addProvider() {
	a.settings.SyncProvider()
	id := uuid.NewString()
	a.settings.Providers = append(a.settings.Providers, settings.Provider{ID: id, Name: fmt.Sprintf("%s %d", locales.TranslationProvider(), len(a.settings.Providers)+1), API: translator.APIChatCompletions})
	a.providerSelectionChanged(id)
}

func (a *NativeApp) providerSelect(c *ui.Context) ui.Element {
	options := []option{}
	counts := map[string]int{}
	for _, p := range a.settings.Providers {
		counts[p.Name]++
	}
	for i, p := range a.settings.Providers {
		label := p.Name
		if label == "" {
			label = locales.TranslationProvider()
		}
		if counts[p.Name] > 1 {
			label += fmt.Sprintf(" (%d)", i+1)
		}
		options = append(options, option{p.ID, label})
	}
	if len(options) == 0 {
		options = append(options, option{"", locales.WorkflowNoService()})
	}
	id := a.settings.ActiveProvider
	return idSelect(c, &id, options).Label(locales.TranslationProvider()).OnChange(func() { a.providerSelectionChanged(id) })
}

func (a *NativeApp) providerModelSelect(c *ui.Context) ui.Element {
	names := append([]string{}, a.providerModels...)
	if p := a.settings.Provider(); p != nil {
		for _, m := range p.Models {
			names = append(names, m.ID)
		}
	}
	slices.Sort(names)
	names = slices.Compact(names)
	return ui.Combobox(c, &a.settings.ProviderModel, names).Placeholder(locales.TranslationModelPlaceholder()).Label(locales.TranslationModel()).OnChange(func() { a.settings.SyncProvider(); a.saveSettings() })
}

func (a *NativeApp) providerDeleteView(c *ui.Context) {
	if !a.providerDelete {
		return
	}
	if ui.AlertDialog(c, &a.providerDelete, locales.TranslationDeleteProviderTitle(), locales.TranslationDeleteProviderNote(), locales.ActionCancel(), locales.TranslationDeleteProvider()) != 1 {
		return
	}
	i := slices.IndexFunc(a.settings.Providers, func(p settings.Provider) bool { return p.ID == a.settings.ActiveProvider })
	if i < 0 {
		return
	}
	if err := security.DeleteKey(a.settings.CredentialScope()); err != nil {
		a.showError("provider-key", err.Error())
		return
	}
	a.clearError("provider-key")
	a.settings.Providers = slices.Delete(a.settings.Providers, i, i+1)
	a.settings.ActiveProvider, a.settings.ProviderURL, a.settings.ProviderModel = "", "", ""
	a.providerKey, a.keyStatus, a.modelsEndpoint, a.modelsError = "", "", "", ""
	a.providerModels, a.modelsLoading = nil, false
	if len(a.settings.Providers) > 0 {
		a.providerSelectionChanged(a.settings.Providers[0].ID)
	} else {
		a.saveSettings()
	}
}
