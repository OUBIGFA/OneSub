package main

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/apperr"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/media"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Compose choices are stored by ID in settings, so the dialog opens with the
// user's last mode and quality whatever language the labels are in.
func composeModes() []option {
	return []option{
		{"hard-mp4", locales.ComposeHardMp4()},
		{"hard-mkv", locales.ComposeHardMkv()},
		{"soft-mkv", locales.ComposeSoftMkv()},
		{"soft-mp4", locales.ComposeSoftMp4()},
	}
}

// Lossless H.264 keeps every source pixel outside the subtitles; the CRF
// levels are lossy and are labelled by their actual value.
func composeQualities() []option {
	return []option{
		{"lossless", locales.ComposeQualityLossless()},
		{"crf18", "CRF 18"},
		{"crf20", "CRF 20"},
		{"crf23", "CRF 23"},
	}
}

func (a *NativeApp) composeView(c *ui.Context) {
	ui.Modal(c, &a.composeOpen, func() {
		ui.Text(c, locales.ActionCompose()).FontSize(18).FontWeight(600)
		mode := a.settings.ComposeMode
		ui.Form(c, func() {
			a.outputFields(c)
			ui.Field(c, locales.ComposeMode(), func() {
				idSelect(c, &a.settings.ComposeMode, composeModes()).Label(locales.ComposeMode()).OnChange(a.saveSettings)
			})
			if strings.HasPrefix(mode, "hard") {
				ui.Field(c, locales.ComposeQuality(), func() {
					idSelect(c, &a.settings.ComposeQuality, composeQualities()).Label(locales.ComposeQuality()).OnChange(a.saveSettings)
				})
				if a.settings.ComposeQuality == "lossless" {
					ui.Text(c, locales.ComposeLosslessNote()).TextColor(c.Theme().TextMuted)
				} else {
					ui.Text(c, locales.ComposeCrfNote()).TextColor(c.Theme().TextMuted)
				}
			}
		})
		if mode == "soft-mp4" {
			ui.Text(c, locales.ComposeMovTextNote()).TextColor(c.Theme().Warning)
		}
		if a.currentStyle.FontName != "" {
			ui.Text(c, a.presetLabel(a.stylePreset)+" · "+a.currentStyle.FontName)
		}
		var outputErr error
		if a.snapshot != nil {
			_, outputErr = subtitle.SelectOutput(a.snapshot.Document, a.settings.SubtitleOutput)
		}
		if outputErr != nil {
			ui.Text(c, outputErr.Error()).TextColor(c.Theme().Warning)
		}
		ui.Row(c).Gap(8).Children(func() {
			ui.Button(c, locales.ActionCancel()).OnClick(func() { a.composeOpen = false })
			ui.Button(c, locales.ComposeSample()).Disabled(outputErr != nil).OnClick(func() { a.composeOpen = false; a.startCompose(true) })
			ui.PrimaryButton(c, locales.ComposeStart()).Disabled(outputErr != nil).OnClick(func() { a.composeOpen = false; a.startCompose(false) })
		})
	})
}

// Progress occupies the player's media area, not a status-bar-only indicator.
// Preparation and full decode verification have unknown duration, never a fake %.
func (a *NativeApp) composeProgressView(c *ui.Context) {
	ui.Column(c).Grow(1).Center().Gap(18).Label(locales.A11yComposeProgressArea()).Children(func() {
		ui.Text(c, locales.ComposeRunning()).FontSize(20).FontWeight(600)
		ui.Text(c, composeStageLabel(a.composeProgress.Stage)).Label(locales.A11yComposeStage())
		ui.Progress(c, a.composeProgress.Fraction).Width(400).Label(locales.A11yComposeProgress())
		if a.composeProgress.Fraction >= 0 {
			ui.Text(c, locales.Percent(100*a.composeProgress.Fraction)).FontSize(24)
		}
		ui.Button(c, locales.ComposeCancel()).Disabled(a.cancel == nil).OnClick(func() {
			if a.cancel != nil {
				a.cancel()
			}
		})
	})
}

// composeStageLabel says media.Compose's stage IDs in the app's language.
func composeStageLabel(stage string) string {
	switch stage {
	case media.StageChooseOutput:
		return locales.ComposeStageChooseOutput()
	case media.StagePrepare:
		return locales.ComposeStagePrepare()
	case media.StageVerifyInput:
		return locales.ComposeStageVerifyInput()
	case media.StageEncode:
		return locales.ComposeStageEncode()
	case media.StageMux:
		return locales.ComposeStageMux()
	case media.StageVerifyOutput:
		return locales.ComposeStageVerifyOutput()
	}
	return stage
}

func composeFilename(source, extension string, sample bool) string {
	name := strings.TrimSuffix(filepath.Base(source), filepath.Ext(source)) + "-OneSub"
	if sample {
		name += "-preview"
	}
	return name + "." + extension
}

func (a *NativeApp) startCompose(sample bool) {
	if a.busy || a.snapshot == nil {
		return
	}
	a.commitEdit(true)
	state, style, mode, aspect := *a.snapshot, a.currentStyle, a.settings.ComposeMode, a.frameAspect
	secondary, outputOptions := a.secondaryStyle, a.settings.SubtitleOutput
	language := a.settings.TargetLanguage
	if outputOptions.Normalized().Mode == "source" {
		language = a.settings.Language
	}
	if outputOptions.Normalized().Mode == "bilingual" {
		language = "mul"
	}
	quality := a.settings.ComposeQuality
	a.composeRunning = true
	a.composeProgress = media.ComposeProgress{Stage: media.StageChooseOutput, Fraction: -1}
	a.evalPlayer("video.pause()")
	a.run(locales.ActionCompose(), func(ctx context.Context) (func(), error) {
		extension := "mp4"
		if strings.HasSuffix(mode, "mkv") {
			extension = "mkv"
		}
		output, err := a.saveDialog(mygo.SaveDialogOptions{Parent: a.win, Title: locales.ActionCompose(), DefaultPath: composeFilename(state.Manifest.MediaPath, extension, sample), Filters: []mygo.FileFilter{{Name: strings.ToUpper(extension), Extensions: []string{extension}}}})
		if err != nil || output == "" {
			return nil, err
		}
		if !strings.EqualFold(filepath.Ext(output), "."+extension) {
			output += "." + extension
		}
		progress := func(p media.ComposeProgress) {
			a.win.Update(func() { a.composeProgress = p; a.status = composeStageLabel(p.Stage) })
		}
		progress(media.ComposeProgress{Stage: media.StagePrepare, Fraction: -1})
		document, err := a.styledDocument(ctx, state, style, secondary, outputOptions, aspect)
		if err != nil {
			return nil, err
		}
		subtitleMode := "hard"
		if strings.HasPrefix(mode, "soft") {
			subtitleMode = "soft"
		}
		options := media.ComposeOptions{Source: state.Manifest.MediaPath, Destination: output, Mode: subtitleMode, Quality: quality, Sample: sample, Language: language, Revision: state.Revision}
		_, err = media.Compose(ctx, a.ffmpeg, a.probe, filepath.Join(a.dataRoot, "cache", "worker"), options, document, progress)
		if err != nil {
			return nil, apperr.Wrap(locales.ErrCompose(err.Error()), err)
		}
		return func() { a.lastOutput = output; a.status = locales.StatusExported(filepath.Base(output)) }, nil
	})
}
