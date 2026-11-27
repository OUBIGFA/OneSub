package main

import (
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/media"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// Media tools share the native destination controls and verified export job.
// Each operation retains its own encoding choices when the user switches modes.
func (a *NativeApp) compressView(c *ui.Context) {
	fixedDialog(c, &a.compressOpen, locales.ActionCompress(), 620, 0, func() {
		audio := a.settings.MediaProcessMode == "audio"
		mode := 0
		if audio {
			mode = 1
		}
		encoders := composeEncoders()
		var encoderWidth float32
		for _, enc := range encoders {
			width, _ := c.MeasureText(0, ui.Span{Text: enc.Label})
			encoderWidth = max(encoderWidth, width)
		}
		qualities := composeQualities(a.settings.CompressionEncoder)
		var qualityWidth float32
		for _, q := range append(composeQualities(media.EncoderCPU), composeQualities(media.EncoderCPUH264)...) {
			width, _ := c.MeasureText(0, ui.Span{Text: q.Label})
			qualityWidth = max(qualityWidth, width)
		}
		ui.Form(c, func() {
			ui.Fieldset(c, locales.MediaOutputGroup(), func() {
				ui.Field(c, locales.MediaProcessMode(), func() {
					idSelect(c, &a.settings.MediaProcessMode, []option{{"compress", locales.MediaCompressVideo()}, {"audio", locales.MediaExtractAudio()}}).Label(locales.MediaProcessMode()).OnChange(a.saveSettings)
				})
				ui.Field(c, locales.CompressFormat(), func() {
					stableVariants(c, mode, func() {
						idSelect(c, &a.settings.CompressionFormat, []option{{"mp4", "MP4"}, {"mkv", "MKV"}}).Label(locales.CompressFormat()).OnChange(a.saveSettings)
					}, func() {
						idSelect(c, &a.settings.AudioFormat, []option{{"mp3", "MP3"}, {"m4a", "M4A (AAC)"}, {"aac", "AAC"}, {"wav", "WAV"}, {"flac", "FLAC"}, {"ogg", "OGG (Vorbis)"}, {"opus", "Opus"}}).Label(locales.CompressFormat()).OnChange(a.saveSettings)
					})
				})
				encoder := ui.Field(c, locales.ComposeEncoder(), func() {
					idSelect(c, &a.settings.CompressionEncoder, encoders).MinWidth(encoderWidth + 48).Label(locales.ComposeEncoder()).OnChange(a.saveSettings)
				})
				quality := ui.Field(c, locales.ComposeQuality(), func() {
					idSelect(c, a.settings.VideoQuality(true), qualities).MinWidth(qualityWidth + 48).Label(locales.ComposeQuality()).OnChange(a.saveSettings)
				})
				if audio {
					encoder.Invisible()
					quality.Invisible()
				}
			})
		})
		note := locales.CompressAv1Note()
		if a.settings.CompressionEncoder == media.EncoderGPU {
			note = locales.CompressGpuNote()
		} else if a.settings.CompressionEncoder == media.EncoderCPUH264 {
			note = locales.CompressH264Note()
		}
		if a.settings.MediaProcessMode == "audio" {
			note = locales.AudioExtractNote()
		}
		stableDialogNote(c, note, false, locales.CompressAv1Note(), locales.CompressGpuNote(), locales.CompressH264Note(), locales.AudioExtractNote())
		separator(c)
		folderField(c, a.settings.OutputDir, locales.RecognitionBesideMedia(), a.busy, a.pickComposeDir, func() { a.settings.OutputDir = ""; a.saveSettings() })
		lobeCheckbox(c, &a.settings.ComposeAskPath, locales.ComposeAskPath()).OnChange(a.saveSettings)
	}, func() {
		lobeButton(c, locales.ActionCancel()).OnClick(func() { a.compressOpen = false })
		audio := a.settings.MediaProcessMode == "audio"
		mode := 0
		if audio {
			mode = 1
		}
		if !audio {
			lobeButton(c, locales.ComposeSample()).OnClick(func() { a.compressOpen = false; a.startVideoExport(true, true) })
		}
		// Keep the primary slot aligned, but only lay out visible actions:
		// audio extraction must not leave a blank preview-button slot.
		stableVariants(c, mode, func() {
			lobePrimaryButton(c, locales.CompressStart()).OnClick(func() { a.compressOpen = false; a.startVideoExport(false, true) })
		}, func() {
			lobePrimaryButton(c, locales.AudioExtractStart()).OnClick(func() { a.compressOpen = false; a.startVideoExport(false, true) })
		})
	})
}
