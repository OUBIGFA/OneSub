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

type providerModelDraft struct {
	provider, selected, text string
}

// fetchProviderModels asks the configured endpoint what models it offers.
// The answer is the provider's catalog: the list the user keeps models from.
// The models kept for the provider are not disturbed by it, except that a
// kept model now knows the limits the endpoint advertises for it.
func (a *NativeApp) fetchProviderModels() {
	if a.modelsLoading || a.win == nil || a.settings.ProviderURL == "" {
		return
	}
	// Snapshot the selected account only on this explicit command. Rendering
	// settings must never perform discovery or use a half-edited connection.
	a.settings.SyncProvider()
	endpoint := a.settings.ProviderURL
	providerID := a.settings.ActiveProvider
	a.modelsGeneration++
	generation := a.modelsGeneration
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
	a.commitProviderModelInput()
	if !a.settings.SelectProvider(id) {
		return
	}
	a.modelsGeneration++
	a.providerKey, a.keyStatus, a.modelsError = "", "", ""
	a.modelsLoading = false
	a.modelQuery = ""
	a.modelList.ScrollTo(0, ui.Start)
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

// providerModelSelect pairs MyGo's editable input with an explicit native
// drop-down. Autocomplete alone only offers suggestions after typing; it is
// not a discoverable way to switch an already selected model.
func (a *NativeApp) providerModelSelect(c *ui.Context) ui.Element {
	row := ui.Row(c.Key(a.settings.ActiveProvider)).Gap(0).MinWidth(0).AlignItems(ui.Stretch)
	if a.modelEdit.provider != a.settings.ActiveProvider || a.modelEdit.selected != a.settings.ProviderModel {
		a.modelEdit = providerModelDraft{a.settings.ActiveProvider, a.settings.ProviderModel, a.settings.ProviderModel}
	}
	wasFocused := ui.Local(row, "model-was-focused", func() bool { return false })
	names := a.providerModelNames()
	row.Children(func() {
		input := ui.Autocomplete(c, &a.modelEdit.text, names).Grow(1).MinWidth(0).
			Radius(c.Theme().Radius, 0, 0, c.Theme().Radius).
			Placeholder(locales.TranslationModelPlaceholder()).Label(locales.TranslationModel()).
			Tooltip(a.modelEdit.text).Disabled(a.settings.Provider() == nil)
		// Only this input's blur commits it. The same model selector can
		// also be mounted behind the settings modal without saving partial IDs.
		if input.Submitted() || (*wasFocused && !input.Focused()) {
			a.commitProviderModelInput()
		}
		*wasFocused = input.Focused()
		ui.MenuButton(c, "", func(m *ui.Menu) {
			for _, id := range a.providerModelNames() {
				if m.Item(id).Checked(id == a.settings.ProviderModel).Chosen() {
					a.useProviderModel(id)
				}
			}
		}).Padding(em(c, 8)).Radius(0, c.Theme().Radius, c.Theme().Radius, 0).
			Label(locales.TranslationSwitchModel()).Tooltip(locales.TranslationSwitchModel()).
			Disabled(len(names) == 0).Shrink(0)
	})
	return row
}

// Commit before switching accounts or starting a task too: those callbacks
// may run before the input observes its blur. A draft never crosses providers.
func (a *NativeApp) commitProviderModelInput() {
	edit := a.modelEdit
	if edit.provider == a.settings.ActiveProvider && edit.selected == a.settings.ProviderModel && edit.text != a.settings.ProviderModel {
		a.useProviderModel(edit.text)
	}
}

// The catalog and saved choices have separate lifetimes. Manual IDs and
// models removed from a later catalog remain available until the user removes
// them; manually edited limits take precedence over advertised metadata.
func (a *NativeApp) providerModelCatalog() []translator.ModelInfo {
	p := a.settings.Provider()
	if p == nil {
		return nil
	}
	byID := map[string]translator.ModelInfo{}
	for _, info := range p.Catalog {
		byID[info.ID] = info
	}
	for _, info := range p.Models {
		byID[info.ID] = info
	}
	if id := a.settings.ProviderModel; id != "" {
		if _, exists := byID[id]; !exists {
			byID[id] = translator.ModelInfo{ID: id}
		}
	}
	items := make([]translator.ModelInfo, 0, len(byID))
	for _, info := range byID {
		items = append(items, info)
	}
	slices.SortFunc(items, func(x, y translator.ModelInfo) int { return strings.Compare(x.ID, y.ID) })
	return items
}

func (a *NativeApp) providerModelNames() []string {
	items := a.providerModelCatalog()
	names := make([]string, 0, len(items))
	// Only enabled models belong in the switcher. Discovery does not opt a
	// model into use; manual IDs remain enabled through the same saved list.
	for _, info := range items {
		if a.settings.Provider().KeepsModel(info.ID) {
			names = append(names, info.ID)
		}
	}
	return names
}

// Keeping a model and choosing it are distinct actions. Multi-select must
// never change the model a translation will use behind the user's back.
func (a *NativeApp) keepProviderModel(id string) {
	p := a.settings.Provider()
	if p == nil || id == "" {
		return
	}
	info := translator.ModelInfo{ID: id}
	for _, m := range a.providerModelCatalog() {
		if m.ID == id {
			info = m
			break
		}
	}
	p.KeepModel(info)
	a.saveSettings()
}

func (a *NativeApp) useProviderModel(id string) {
	if a.settings.Provider() == nil {
		return
	}
	id = strings.TrimSpace(id)
	a.settings.ProviderModel = id
	a.modelEdit = providerModelDraft{a.settings.ActiveProvider, id, id}
	if id == "" {
		a.saveSettings()
	} else {
		a.keepProviderModel(id)
	}
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
	if confirmationDialog(c, &a.providerDelete, locales.TranslationDeleteProviderTitle(), locales.TranslationDeleteProviderNote(), locales.ActionCancel(), locales.TranslationDeleteProvider()) != 1 {
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
	a.providerKey, a.keyStatus, a.modelsError = "", "", ""
	a.modelQuery, a.modelsLoading = "", false
	if len(a.settings.Providers) > 0 {
		a.providerSelectionChanged(a.settings.Providers[0].ID)
	} else {
		a.saveSettings()
	}
}

// The model directory is a native table: retention, model ID and use action
// have stable columns. Long identifiers elide in their own column, never push
// controls away, and keep their complete name and limits in a tooltip.
func (a *NativeApp) providerModelsView(c *ui.Context) {
	p := a.settings.Provider()
	items := a.providerModelCatalog()
	models := slices.DeleteFunc(items, func(info translator.ModelInfo) bool {
		return !strings.Contains(strings.ToLower(info.ID), strings.ToLower(strings.TrimSpace(a.modelQuery)))
	})
	// The shared settings layout gives the catalog its remaining height.
	// A minimum reading area keeps the form usable at the smallest window;
	// fetching or filtering models cannot change the size of any settings page.
	ui.Column(c).Height(em(c, 160)).Grow(1).Shrink(0).Background(studioColors(c).panel).Border(1, studioColors(c).hairline).Radius(c.Theme().Radius).Clip().Label(locales.A11yModelCatalog()).Children(func() {
		if len(models) == 0 {
			label := locales.TranslationModelsEmpty()
			if a.modelQuery != "" {
				label = locales.TranslationModelsNoMatch()
			}
			ui.Column(c).Fill().Center().Padding(em(c, 16)).Children(func() { muted(c, label).TextAlign(ui.Center) })
			return
		}
		measure := func(labels ...string) float32 {
			width := float32(0)
			for _, label := range labels {
				w, _ := c.MeasureText(0, ui.Span{Text: label})
				width = max(width, w)
			}
			return width + em(c, 24)
		}
		statusWidth := measure(locales.TranslationModelInUse(), locales.TranslationUseModel(), locales.TranslationModelUnused())
		a.modelList.Key = func(i int) any { return models[i].ID }
		a.modelList.Label = func(i int) string { return models[i].ID }
		ui.Table(c, &a.modelList, []ui.TableColumn{
			{Title: locales.TranslationKeep(), Width: measure(locales.TranslationKeep()), Fixed: true, Align: ui.Center},
			{Title: locales.TranslationModel(), MinWidth: em(c, 80), Fixed: true},
			{Title: locales.TranslationModelState(), Width: max(statusWidth+em(c, 24), measure(locales.TranslationModelInUse(), locales.TranslationModelState())), Fixed: true, Align: ui.Center},
			// MyGo scrollbars overlay their content. A native empty column
			// reserves a stable gutter in both the header and every row.
			{ID: "scrollbar-gutter", Width: em(c, 20), Fixed: true},
		}, len(models), func(i, col int) {
			info := models[i]
			switch col {
			case 0:
				kept := p.KeepsModel(info.ID)
				ui.Checkbox(c, &kept, "").Label(locales.TranslationKeepModel(info.ID)).Tooltip(locales.TranslationKeepModel(info.ID)).
					Disabled(a.settings.ProviderModel == info.ID).OnChange(func() {
					if kept {
						a.keepProviderModel(info.ID)
					} else {
						a.dropProviderModel(info.ID)
					}
				})
			case 1:
				tip := info.ID
				if input, output, window := providerModelLimits(&info); input > 0 || output > 0 || window > 0 {
					tip += "\n" + locales.TranslationModelLimits(strconv.Itoa(input), strconv.Itoa(output), strconv.Itoa(window))
				}
				label := ui.Text(c, info.ID).SingleLine().Tooltip(tip)
				if !p.KeepsModel(info.ID) {
					label.TextColor(c.Theme().TextMuted)
				}
			case 2:
				label := locales.TranslationUseModel()
				if a.settings.ProviderModel == info.ID {
					label = locales.TranslationModelInUse()
				} else if !p.KeepsModel(info.ID) {
					label = locales.TranslationModelUnused()
				}
				// Every state uses the same native button geometry, including
				// the current model; only unused models are disabled/muted.
				ui.Button(c, label).Width(statusWidth).Shrink(0).Padding(em(c, 2), em(c, 8)).Disabled(!p.KeepsModel(info.ID)).
					Label(locales.TranslationUseModelNamed(info.ID)).OnClick(func() { a.useProviderModel(info.ID) })
			}
		}).Fill().Label(locales.TranslationModels())
	})
}
