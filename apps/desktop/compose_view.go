package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/apperr"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/media"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/google/uuid"
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

// CRF choices are labelled by their actual value, not as original quality.
func composeQualities() []option {
	return []option{
		{"crf18", "CRF 18"},
		{"crf20", "CRF 20"},
		{"crf23", "CRF 23"},
	}
}

// Content, encoding and destination are separate decisions. Fixed geometry
// and an anchored action row keep sample/export actions in the same place.
func (a *NativeApp) composeView(c *ui.Context) {
	var outputErr error
	if a.composeOpen && a.snapshot != nil {
		_, outputErr = subtitle.SelectOutput(a.snapshot.OutputDocument(a.settings.SubtitleOutput), a.settings.SubtitleOutput)
	}
	fixedDialog(c, &a.composeOpen, locales.ActionCompose(), 760, 552, func() {
		hard := strings.HasPrefix(a.settings.ComposeMode, "hard")
		ui.Grid(c).Columns(2).Gap(em(c, 12)).Children(func() { a.outputFields(c, true) })
		ui.Grid(c).ColumnTracks(ui.Fr(3), ui.Fr(2)).Gap(em(c, 16)).Children(func() {
			ui.Field(c, locales.ComposeMode(), func() {
				idSelect(c, &a.settings.ComposeMode, composeModes()).Label(locales.ComposeMode()).OnChange(a.saveSettings)
			})
			ui.Field(c, locales.ComposeQuality(), func() {
				idSelect(c, &a.settings.ComposeQuality, composeQualities()).Label(locales.ComposeQuality()).Disabled(!hard).OnChange(a.saveSettings)
			})
		})
		line(c, func() {
			folderField(c, a.settings.OutputDir, locales.RecognitionBesideMedia(), a.busy, a.pickComposeDir, func() { a.settings.OutputDir = ""; a.saveSettings() })
			ui.Checkbox(c, &a.settings.ComposeAskPath, locales.ComposeAskPath()).OnChange(a.saveSettings)
		})
		ui.Column(c).MinHeight(em(c, 72)).Gap(em(c, 8)).Children(func() {
			switch {
			case outputErr != nil:
				ui.Text(c, outputErr.Error()).TextColor(c.Theme().Warning)
			case a.settings.ComposeMode == "soft-mp4":
				ui.Text(c, locales.ComposeMovTextNote()).TextColor(c.Theme().Warning)
			case !hard:
				muted(c, locales.ComposeSoftNote())
			default:
				muted(c, locales.ComposeCrfNote())
				muted(c, locales.ComposeCrfRecommendation())
			}
			if a.currentStyle.FontName != "" {
				muted(c, a.presetLabel(a.stylePreset)+" · "+a.currentStyle.FontName).SingleLine()
			}
		})
	}, func() {
		ui.Button(c, locales.ActionCancel()).OnClick(func() { a.composeOpen = false })
		ui.Button(c, locales.ComposeSample()).Disabled(outputErr != nil).OnClick(func() { a.composeOpen = false; a.startCompose(true) })
		ui.PrimaryButton(c, locales.ComposeStart()).Disabled(outputErr != nil).OnClick(func() { a.composeOpen = false; a.startCompose(false) })
	})
}

// Progress occupies the player's media area, not a status-bar-only indicator.
// Preparation and full decode verification have unknown duration, never a fake %.
func (a *NativeApp) composeProgressView(c *ui.Context) {
	ui.Column(c).Grow(1).Center().Gap(em(c, 16)).Label(locales.A11yComposeProgressArea()).Children(func() {
		dialogTitle(c, locales.ComposeRunning())
		muted(c, composeStageLabel(a.composeProgress.Stage)).Label(locales.A11yComposeStage())
		ui.Progress(c, a.composeProgress.Fraction).Width(em(c, 400)).Label(locales.A11yComposeProgress())
		if a.composeProgress.Fraction >= 0 {
			ui.Text(c, locales.Percent(100*a.composeProgress.Fraction)).FontSize(em(c, 28)).FontWeight(600).FontFeatures("tnum")
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
	outputDir, askPath := a.settings.OutputDir, a.settings.ComposeAskPath
	a.composeRunning = true
	a.composeProgress = media.ComposeProgress{Stage: media.StageChooseOutput, Fraction: -1}
	a.evalPlayer("video.pause()")
	a.run(locales.ActionCompose(), func(ctx context.Context) (func(), error) {
		extension := "mp4"
		if strings.HasSuffix(mode, "mkv") {
			extension = "mkv"
		}
		output, err := composeOutputPath(state.Manifest.MediaPath, outputDir, extension, sample)
		if err != nil {
			return nil, err
		}
		if askPath {
			output, err = a.saveDialog(mygo.SaveDialogOptions{Parent: a.win, Title: locales.ActionCompose(), DefaultPath: output, Filters: []mygo.FileFilter{{Name: strings.ToUpper(extension), Extensions: []string{extension}}}})
			if err != nil || output == "" {
				return nil, err
			}
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
		report, err := media.Compose(ctx, a.ffmpeg, a.probe, filepath.Join(a.dataRoot, "cache", "worker"), options, document, progress)
		if err != nil {
			return nil, apperr.Wrap(locales.ErrCompose(err.Error()), err)
		}
		reportErr := saveComposeReport(state.Path, report)
		return func() {
			a.lastOutput, a.status = output, locales.StatusExported(filepath.Base(output))
			if reportErr != nil {
				// The validated video is already published. Keep it reachable
				// instead of reporting a failed encode and inviting a rerun.
				a.showError("compose-report", locales.ErrComposeReport(reportErr.Error()))
			} else {
				a.clearError("compose-report")
			}
		}, nil
	})
}

// Automatic exports use the source folder unless explicitly configured. A
// collision chooses a new name; Compose's atomic publish still refuses a race.
func composeOutputPath(source, directory, extension string, sample bool) (string, error) {
	if directory == "" {
		directory = filepath.Dir(source)
	}
	name := composeFilename(source, extension, sample)
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	for suffix := 1; ; suffix++ {
		candidate := name
		if suffix > 1 {
			candidate = fmt.Sprintf("%s (%d).%s", stem, suffix, extension)
		}
		path := filepath.Join(directory, candidate)
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			return path, nil
		} else if err != nil {
			return "", err
		}
	}
}

func (a *NativeApp) pickComposeDir() {
	a.run(locales.RecognitionChooseFolder(), func(context.Context) (func(), error) {
		paths, err := a.openDialog(mygo.OpenDialogOptions{Parent: a.win, Title: locales.RecognitionChooseFolder(), Directory: true, DefaultPath: a.settings.OutputDir})
		if err != nil || len(paths) == 0 {
			return nil, err
		}
		return func() { a.settings.OutputDir = paths[0]; a.saveSettings() }, nil
	})
}

// saveComposeReport retains validation evidence inside the recovery project;
// media/output directories contain only the user's requested deliverables.
func saveComposeReport(projectPath string, report media.ComposeReport) error {
	artifacts := filepath.Join(projectPath, "artifacts")
	if err := os.MkdirAll(artifacts, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return subtitle.Publish(filepath.Join(artifacts, "compose-"+uuid.NewString()+".json"), data)
}
