package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/translator"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

func (live *translationLive) missingIndices() []int {
	var indices []int
	for i, cue := range live.units {
		if cue.MissingTranslation() && (!live.final[i] || strings.TrimSpace(live.texts[i]) == "") {
			indices = append(indices, i)
		}
	}
	return indices
}

func nextMissing(indices []int, current int) int {
	for _, index := range indices {
		if index > current {
			return index
		}
	}
	if len(indices) > 0 {
		return indices[0]
	}
	return -1
}

// Locate is independent of search and playback follow, and never seeks or
// plays the video. Live units may use a rebuilt timeline, so select their list.
func (a *NativeApp) locateMissingTranslation() {
	if !a.commitEdit(true) {
		return
	}
	a.pendingClick, a.rowClick, a.query = nil, nil, ""
	if live := a.translationLive; live != nil && live.stopped != "" {
		a.changeSubtitleTab(1)
		a.followHeld = true
		index := nextMissing(live.missingIndices(), live.selected)
		if index >= 0 {
			live.list.StopScroll()
			live.selected, live.locate = index, index
		}
		return
	}
	if a.snapshot == nil {
		return
	}
	a.changeSubtitleTab(1)
	a.followHeld = true
	index := nextMissing(subtitle.MissingTranslationIndices(a.snapshot.Document), a.selected)
	if index >= 0 {
		a.list.StopScroll()
		a.selected, a.locateCue = index, index
		a.loadDraft()
	}
}

// An interrupted run reuses its exact progress. An accepted partial track
// starts a scoped continuation without rebuilding or replacing completed cues.
func (a *NativeApp) continueMissingTranslation() {
	if a.busy || a.snapshot == nil || !a.commitEdit(true) {
		return
	}
	if a.translationLive != nil && a.translationLive.stopped != "" {
		a.retryTranslation()
		return
	}
	a.commitProviderModelInput()
	if a.settings.ProviderURL == "" || a.settings.ProviderModel == "" {
		a.settingsOpen, a.settingsTab = true, 2
		return
	}
	config := a.translationConfig()
	config.OnlyMissing, config.OptimizeTimeline = true, false
	a.startTranslationConfigured(config)
}

// Hidden and visible variants both participate in the dialog's native sizing.
// The output-only checkbox cannot turn skipping into a persistent preference.
func (a *NativeApp) outputMissingActions(c *ui.Context, skip *bool, close func()) {
	missing := 0
	if a.snapshot != nil && a.settings.SubtitleOutput.Normalized().Mode != "source" {
		missing = subtitle.MissingTranslations(a.snapshot.Document)
	}
	visible := 1
	if missing > 0 {
		visible = 0
	}
	stableVariants(c, visible, func() {
		ui.Row(c).Wrap().Gap(em(c, 8)).AlignItems(ui.Center).Children(func() {
			lobeButton(c, locales.TranslationLocateMissing()).OnClick(func() { close(); a.locateMissingTranslation() })
			lobeButton(c, locales.TranslationResume()).Disabled(a.busy).OnClick(func() { close(); a.continueMissingTranslation() })
			lobeCheckbox(c, skip, locales.TranslationSkipMissing()).Tooltip(locales.TranslationSkipNote())
		})
	}, func() {})
}

func (a *NativeApp) skipTranslation() {
	if a.busy || a.translationLive == nil || a.translationLive.stopped == "" || a.translationTask == nil || !a.commitEdit(true) {
		return
	}
	task := *a.translationTask
	if a.snapshot == nil || task.ProjectID != a.snapshot.Manifest.ID || task.Revision != a.snapshot.Revision {
		a.showError(locales.ActionTranslate(), locales.TranslationResumeStale())
		return
	}
	a.translationRunning = true
	a.runTimed(timingTranslation, true, locales.ActionTranslate(), a.skipTranslationWork(&task))
}

// The journal contains the engine's assembled result, not merely streamed
// fragments. Check completed cues before accepting them; unfinished source
// remains in the project and is omitted only from the explicitly chosen file.
func (a *NativeApp) skipTranslationWork(task *translationTask) func(context.Context) (func(), error) {
	return func(ctx context.Context) (apply func(), runErr error) {
		defer func() {
			if clock, _ := ctx.Value(taskTimingKey{}).(*taskTiming); clock != nil {
				elapsed := clock.duration(time.Now())
				if err := writeTranslationRecord(task.ProjectPath, translationRecord{Elapsed: &elapsed}, false); err != nil {
					runErr = errors.Join(runErr, errors.New(locales.TranslationProgressWriteError(err.Error())))
				}
			}
		}()
		stored, _, result, finished, err := readTranslationProgress(task.ProjectPath)
		if err != nil {
			return nil, err
		}
		if finished || stored.ProjectID != task.ProjectID || stored.Revision != task.Revision || result == nil {
			return nil, errors.New(locales.TranslationResumeStale())
		}
		ids := []string{}
		for _, cue := range result.Document.Cues {
			if !cue.ReadOnly && !cue.MissingTranslation() {
				ids = append(ids, cue.ID)
			}
		}
		skipped := subtitle.MissingTranslations(result.Document)
		if len(ids) == 0 {
			return func() {
				a.completeTranslationRecovery()
				a.status = locales.TranslationSkipped(skipped)
			}, nil
		}
		root, executable := a.skillTools()
		checker := translator.Checker{Executable: executable, SkillRoot: root, Scratch: a.worker.Scratch, Subtitle: a.worker}
		checks, err := checker.CheckSelected(ctx, result.Source, result.Document, ids, task.Config.TargetLanguage)
		if err != nil {
			return nil, err
		}
		candidate := &translationCandidate{ProjectID: task.ProjectID, Revision: task.Revision, Result: *result, Checks: checks, Skipped: skipped}
		candidate.Result.Report.Warnings = append(candidate.Result.Report.Warnings, locales.TranslationSkipped(skipped))
		var file []byte
		if len(checks.Errors) == 0 && task.Target != "" {
			file, err = a.encodeTrackWithSkipping(ctx, result.Document, 1, task.Format, true)
			if err != nil {
				return nil, err
			}
		}
		return func() { a.applyTranslation(candidate, task.Target, file) }, nil
	}
}

func (a *NativeApp) missingSaveView(c *ui.Context) {
	fixedDialog(c, &a.missingSaveOpen, locales.TranslationSkipMissing(), 680, 0, func() {
		missing := 0
		if a.snapshot != nil {
			missing = subtitle.MissingTranslations(a.snapshot.Document)
		}
		ui.Column(c).Gap(em(c, 12)).Children(func() {
			ui.Text(c, locales.TranslationMissingCount(missing))
			muted(c, locales.TranslationSkipNote())
		})
	}, func() {
		lobeButton(c, locales.TranslationLocateMissing()).OnClick(func() { a.missingSaveOpen = false; a.locateMissingTranslation() })
		lobeButton(c, locales.ActionCancel()).OnClick(func() { a.missingSaveOpen = false; a.missingSaveNext = nil })
		lobeButton(c, locales.TranslationSkipMissing()).OnClick(func() {
			next := a.missingSaveNext
			a.missingSaveOpen, a.missingSaveNext = false, nil
			if next != nil {
				next()
			}
		})
		lobePrimaryButton(c, locales.TranslationResume()).OnClick(func() { a.missingSaveOpen = false; a.missingSaveNext = nil; a.continueMissingTranslation() })
	})
}
