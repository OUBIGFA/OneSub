package main

import (
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// Compression has its own entry and encoding choices, but uses the same
// native dialog, destination policy and verified media job as video export.
func (a *NativeApp) compressView(c *ui.Context) {
	fixedDialog(c, &a.compressOpen, locales.ActionCompress(), 760, 440, func() {
		settingsGroup(c, locales.ComposeEncodingGroup(), func() {
			ui.Grid(c).Columns(2).Gap(em(c, 24)).Children(func() {
				ui.Field(c, locales.CompressFormat(), func() {
					idSelect(c, &a.settings.CompressionFormat, []option{{"mp4", "MP4"}, {"mkv", "MKV"}}).Label(locales.CompressFormat()).OnChange(a.saveSettings)
				})
				ui.Field(c, locales.ComposeQuality(), func() {
					idSelect(c, &a.settings.CompressionQuality, composeQualities()).Label(locales.ComposeQuality()).OnChange(a.saveSettings)
				})
			})
			muted(c, locales.ComposeCrfNote()).FontSize(em(c, captionSize))
		})
		folderField(c, a.settings.OutputDir, locales.RecognitionBesideMedia(), a.busy, a.pickComposeDir, func() { a.settings.OutputDir = ""; a.saveSettings() })
		ui.Checkbox(c, &a.settings.ComposeAskPath, locales.ComposeAskPath()).OnChange(a.saveSettings)
	}, func() {
		ui.Button(c, locales.ActionCancel()).OnClick(func() { a.compressOpen = false })
		ui.Button(c, locales.ComposeSample()).OnClick(func() { a.compressOpen = false; a.startVideoExport(true, true) })
		ui.PrimaryButton(c, locales.CompressStart()).OnClick(func() { a.compressOpen = false; a.startVideoExport(false, true) })
	})
}
