package main

import (
	"slices"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/models"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
	lobeField "github.com/lobehub/lobe-mygo/pkg/field"
)

// Refresh on startup and model-management changes, not on every UI frame.
// File presence lists a model; Verify still authorizes its use for inference.
func (a *NativeApp) refreshDownloadedAsrModels() {
	a.downloadedAsr = make([]option, 0)
	for _, model := range models.Catalog() {
		if model.Kind == "asr" && model.FilesPresent(a.modelPath(model.ID)) {
			a.downloadedAsr = append(a.downloadedAsr, option{model.ID, model.Label})
		}
	}
}

func (a *NativeApp) downloadedAsrModels() []option {
	if a.downloadedAsr == nil {
		a.refreshDownloadedAsrModels()
	}
	return a.downloadedAsr
}

func (a *NativeApp) asrDownloaded(id string) bool {
	return slices.ContainsFunc(a.downloadedAsrModels(), func(model option) bool { return model.ID == id })
}

func (a *NativeApp) selectedAsrDownloaded() bool { return a.asrDownloaded(a.settings.AsrModel) }

// Keep the complete catalog in the Lobe selector, with missing models disabled.
// Their separate download manager remains available without changing selection.
func (a *NativeApp) settingsAsrSelect(c *ui.Context) ui.Element {
	items := []lobeField.SelectOption{}
	for _, model := range models.Catalog() {
		if model.Kind == "asr" {
			items = append(items, lobeField.SelectOption{Value: model.ID, Label: model.Label, Disabled: !a.asrDownloaded(model.ID)})
		}
	}
	return lobeField.Select(c, &a.settings.AsrModel, lobeField.SelectProps{
		Options: items, Disabled: a.busy, Placeholder: a.settings.AsrModel,
		RenderLabel: func(option lobeField.SelectOption) {
			ui.Row(c).Grow(1).MinWidth(0).Gap(em(c, 8)).AlignItems(ui.Center).Children(func() {
				ui.Text(c, option.Label).SingleLine().Grow(1).MinWidth(0)
				if option.Disabled {
					muted(c, locales.ModelNotDownloaded()).FontSize(em(c, captionSize)).NoWrap().Shrink(0)
				}
			})
		},
	}).OnChange(a.asrModelChanged)
}
