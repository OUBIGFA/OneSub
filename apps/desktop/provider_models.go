package main

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/security"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/translator"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
	"github.com/google/uuid"
)

// fetchProviderModels asks the configured endpoint what models it offers.
// The answer is the provider's catalog: the list the user keeps models from.
// The models kept for the provider are not disturbed by it, except that a
// kept model now knows the limits the endpoint advertises for it.
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
			p := a.settings.Provider()
			if p == nil {
				return
			}
			// The catalog is what the endpoint offers now. The models kept
			// stay kept; a kept model learns the limits the endpoint
			// advertises, unless the user entered its own.
			p.Catalog = append([]translator.ModelInfo{}, items...)
			for _, item := range items {
				if p.KeepsModel(item.ID) {
					p.KeepModel(item)
				}
			}
			a.saveSettings()
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
		for _, model := range p.Catalog {
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

// providerModelSelect is the model in use for this provider. It takes any
// model the endpoint offers and any name the user types, so a service that
// publishes no catalog stays usable.
func (a *NativeApp) providerModelSelect(c *ui.Context) ui.Element {
	names := a.providerModelNames()
	// Autocomplete binds free text as well as catalog selections. Combobox
	// accepts catalog items only and discards manually entered model names.
	// Choosing a name from the catalog keeps it for this provider, so it
	// stays in the list to switch back to.
	return ui.Autocomplete(c, &a.settings.ProviderModel, names).Placeholder(locales.TranslationModelPlaceholder()).Label(locales.TranslationModel()).Tooltip(a.settings.ProviderModel).OnChange(func() {
		a.settings.SyncProvider()
		// A model the endpoint offers is kept for this provider, so it
		// stays in the list and the user can switch back to it.
		if p := a.settings.Provider(); p != nil {
			for _, m := range p.Catalog {
				if m.ID == a.settings.ProviderModel {
					p.KeepModel(m)
					break
				}
			}
		}
		a.saveSettings()
	})
}

// providerModelNames are the models the user may choose: those the endpoint
// offers, those kept for this provider, and the one in use.
func (a *NativeApp) providerModelNames() []string {
	names := append([]string{}, a.providerModels...)
	if p := a.settings.Provider(); p != nil {
		for _, m := range p.Catalog {
			names = append(names, m.ID)
		}
		for _, m := range p.Models {
			names = append(names, m.ID)
		}
	}
	if a.settings.ProviderModel != "" {
		names = append(names, a.settings.ProviderModel)
	}
	slices.Sort(names)
	return slices.Compact(names)
}

// providerKeptModels are the models kept for this provider, in the order the
// user kept them.
func (a *NativeApp) providerKeptModels() []translator.ModelInfo {
	p := a.settings.Provider()
	if p == nil {
		return nil
	}
	return append([]translator.ModelInfo{}, p.Models...)
}

// keepProviderModel keeps a model for this provider, and makes it the one in
// use when it is not kept yet. A model the user entered by hand is kept with
// the limits it has.
func (a *NativeApp) keepProviderModel(id string) {
	p := a.settings.Provider()
	if p == nil || id == "" {
		return
	}
	info := translator.ModelInfo{ID: id}
	for _, m := range p.Catalog {
		if m.ID == id {
			info = m
			break
		}
	}
	for _, m := range p.Models {
		if m.ID == id {
			info = m
			break
		}
	}
	p.KeepModel(info)
	a.settings.ProviderModel = id
	a.settings.SyncProvider()
	a.saveSettings()
}

// dropProviderModel takes a model out of the kept ones. The model in use
// cannot be taken out: a provider without a model cannot translate.
func (a *NativeApp) dropProviderModel(id string) {
	p := a.settings.Provider()
	if p == nil || id == "" || p.Model == id {
		return
	}
	p.DropModel(id)
	a.saveSettings()
}

// providerModelLimits says the limits of the model in use, or of the model
// the user is looking at.
func providerModelLimits(info *translator.ModelInfo) (int, int, int) {
	if info == nil {
		return 0, 0, 0
	}
	return info.MaxInputTokens, info.MaxOutputTokens, info.ContextWindow
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

// providerModelsView is the list of the models the endpoint offers, with the
// ones this provider keeps marked. It is what the user asked the service for
// and never saw: the models were fetched into the settings, where only the
// names in use could be seen.
func (a *NativeApp) providerModelsView(c *ui.Context) {
	p := a.settings.Provider()
	if p == nil {
		return
	}
	models := append([]translator.ModelInfo{}, p.Catalog...)
	for _, m := range p.Models {
		if !slices.ContainsFunc(models, func(c translator.ModelInfo) bool { return c.ID == m.ID }) {
			models = append(models, m)
		}
	}
	if len(models) == 0 {
		return
	}
	slices.SortFunc(models, func(x, y translator.ModelInfo) int { return strings.Compare(x.ID, y.ID) })
	ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Children(func() {
		muted(c, locales.TranslationModelsHint()).FontSize(em(c, captionSize))
		ui.Spacer(c)
		ui.Button(c, locales.TranslationFetchModels()).Disabled(a.modelsLoading || a.settings.ProviderURL == "").OnClick(a.fetchProviderModels)
	})
	// The list has a fixed height of its own: the settings dialog has a
	// fixed size, and a longer list of models scrolls inside it.
	a.modelList.Key = func(i int) any { return models[i].ID }
	ui.List(c, &a.modelList, len(models), func(i int) {
		info := models[i]
		kept := p.KeepsModel(info.ID)
		ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Padding(em(c, 2), em(c, 4)).Children(func() {
			ui.Checkbox(c, &kept, info.ID).Tooltip(info.ID).OnChange(func() {
				if kept {
					a.keepProviderModel(info.ID)
				} else {
					a.dropProviderModel(info.ID)
				}
			})
			if p.Model == info.ID {
				ui.Badge(c, locales.TranslationModelInUse()).Background(c.Theme().Accent.Alpha(0.16)).TextColor(c.Theme().Accent)
			}
			if input, output, window := providerModelLimits(&info); input > 0 || output > 0 || window > 0 {
				muted(c, locales.TranslationModelLimits(strconv.Itoa(input), strconv.Itoa(output), strconv.Itoa(window))).FontSize(em(c, captionSize)).FontFeatures("tnum")
			}
		})
	}).Height(em(c, 120)).Label(locales.TranslationModels())
}
