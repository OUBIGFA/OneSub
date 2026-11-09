package main

import (
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// Media tools share the native destination controls and verified export job.
// Each operation retains its own encoding choices when the user switches modes.
func (a *NativeApp) compressView(c *ui.Context) {
	fixedDialog(c, &a.compressOpen, locales.ActionCompress(), 620, 0, func() {
		ui.Form(c, func() {
			ui.Fieldset(c, locales.MediaOutputGroup(), func() {
				ui.Field(c, locales.MediaProcessMode(), func() {
					idSelect(c, &a.settings.MediaProcessMode, []option{{"compress", locales.MediaCompressVideo()}, {"audio", locales.MediaExtractAudio()}}).Label(locales.MediaProcessMode()).OnChange(a.saveSettings)
				})
				ui.Field(c, locales.CompressFormat(), func() {
					if a.settings.MediaProcessMode == "audio" {
						idSelect(c, &a.settings.AudioFormat, []option{{"mp3", "MP3"}, {"m4a", "M4A (AAC)"}, {"aac", "AAC"}, {"wav", "WAV"}, {"flac", "FLAC"}, {"ogg", "OGG (Vorbis)"}, {"opus", "Opus"}}).Label(locales.CompressFormat()).OnChange(a.saveSettings)
					} else {
						idSelect(c, &a.settings.CompressionFormat, []option{{"mp4", "MP4"}, {"mkv", "MKV"}}).Label(locales.CompressFormat()).OnChange(a.saveSettings)
					}
				})
				if a.settings.MediaProcessMode != "audio" {
					ui.Field(c, locales.ComposeQuality(), func() {
						idSelect(c, &a.settings.CompressionQuality, composeQualities()).Label(locales.ComposeQuality()).OnChange(a.saveSettings)
					})
				}
			})
		})
		note := locales.ComposeCrfNote()
		if a.settings.MediaProcessMode == "audio" {
			note = locales.AudioExtractNote()
		}
		muted(c, note).FontSize(em(c, captionSize))
		separator(c)
		folderField(c, a.settings.OutputDir, locales.RecognitionBesideMedia(), a.busy, a.pickComposeDir, func() { a.settings.OutputDir = ""; a.saveSettings() })
		ui.Checkbox(c, &a.settings.ComposeAskPath, locales.ComposeAskPath()).OnChange(a.saveSettings)
	}, func() {
		ui.Button(c, locales.ActionCancel()).OnClick(func() { a.compressOpen = false })
		action := locales.CompressStart()
		if a.settings.MediaProcessMode == "audio" {
			action = locales.AudioExtractStart()
		} else {
			ui.Button(c, locales.ComposeSample()).OnClick(func() { a.compressOpen = false; a.startVideoExport(true, true) })
		}
		ui.PrimaryButton(c, action).OnClick(func() { a.compressOpen = false; a.startVideoExport(false, true) })
	})
}
