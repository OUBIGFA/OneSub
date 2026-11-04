package main

import (
	"slices"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/models"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// Refresh on startup and model-management changes, not on every UI frame.
// Complete catalog files are sufficient for listing; Verify remains the
// authority for using their contents in an actual recognition.
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

// Settings retain the full ASR catalog, but missing assets are disabled and
// muted. The manager's separate catalog remains available for downloading.
// SelectBase preserves MyGo's native popup, keyboard and accessible selection.
func (a *NativeApp) settingsAsrSelect(c *ui.Context) ui.Element {
	t := c.Theme()
	sel := ui.SelectBase(c, &a.settings.AsrModel)
	trigger := sel.Trigger.Padding(t.Space(1.5), t.Space(3.5)).Gap(t.Space(1.5)).Radius(t.Radius).Border(1, t.Border).AlignItems(ui.Center).Disabled(a.busy)
	trigger.Background(t.Surface)
	if trigger.Pressed() {
		trigger.Background(t.SurfacePressed)
	} else if trigger.Hovered() {
		trigger.Background(t.SurfaceHover)
	}
	name := a.settings.AsrModel
	if model, err := models.ByID(a.settings.AsrModel); err == nil {
		name = model.Label
	}
	if !a.selectedAsrDownloaded() {
		trigger.TextColor(t.TextMuted)
	}
	trigger.Tooltip(name).Children(func() {
		ui.Text(c, name).SingleLine().Grow(1).MinWidth(0)
		ui.Icon(c, icons["chevron-down"]).Size(em(c, 14), em(c, 14)).TextColor(t.TextMuted).Shrink(0)
	})
	sel.Popup(func(panel ui.Element) {
		panel.Margin(t.Space(1), 0, 0, 0).Padding(t.Space(1)).Radius(t.Radius+2).Background(t.Background).Border(1, t.Border)
		ui.Scroll(c).MaxHeight(em(c, 360)).Children(func() {
			for _, model := range models.Catalog() {
				if model.Kind != "asr" {
					continue
				}
				ready := a.asrDownloaded(model.ID)
				item := sel.Item(model.ID).Disabled(!ready).Padding(t.Space(1.5), t.Space(2.5)).Radius(t.Radius).Tooltip(model.Label)
				if !ready {
					item.TextColor(t.TextMuted).Description(locales.ModelNotDownloaded())
				} else if item.Highlighted() {
					item.Background(t.Accent).TextColor(t.AccentText)
				} else if model.ID == a.settings.AsrModel {
					item.Background(t.Surface)
				}
				item.Children(func() {
					ui.Text(c, model.Label).SingleLine().Grow(1).MinWidth(0)
					if !ready {
						muted(c, locales.ModelNotDownloaded()).FontSize(em(c, captionSize)).SingleLine().Shrink(0)
					}
				})
			}
		})
	})
	return trigger.OnChange(a.asrModelChanged)
}
