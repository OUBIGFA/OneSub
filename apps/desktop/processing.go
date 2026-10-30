package main

import (
	"context"
	"path/filepath"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

func (a *NativeApp) processMedia() {
	if a.busy || a.snapshot == nil {
		return
	}
	if a.translationLive != nil && a.translationLive.stopped != "" {
		a.retryTranslation()
		return
	}
	if pending := a.settings.Processing; pending != nil {
		switch pending.Stage {
		case "translate":
			a.translate()
		case "save-recognition":
			a.saveTracks([]int{0}, false, a.recognitionFinished)
		default:
			a.recognizeWithTranslation(pending.Translate)
		}
		return
	}
	a.recognizeWithTranslation(true)
}

func (a *NativeApp) processingLabel() string {
	if !a.busy && (a.settings.Processing != nil || a.translationLive != nil && a.translationLive.stopped != "") {
		return locales.TranslationResume()
	}
	return locales.ActionRecognizeTranslate()
}

func (a *NativeApp) rememberProcessing(stage string, translate bool) bool {
	if a.snapshot == nil {
		return false
	}
	if a.settingsError {
		a.showError("settings", locales.ErrSettingsNeedRepair())
		return false
	}
	a.settings.Processing = &settings.PendingProcessing{ProjectPath: a.snapshot.Path, Stage: stage, Translate: translate, SavedHash: a.savedHash, SavedPath: a.savedPath}
	if err := settings.Save(filepath.Join(a.dataRoot, "settings.json"), a.settings); err != nil {
		a.showError("settings", err.Error())
		return false
	}
	a.clearError("settings")
	return true
}

func (a *NativeApp) recognitionFinished() {
	pending := a.settings.Processing
	if pending == nil {
		return
	}
	if !pending.Translate {
		a.settings.Processing = nil
		a.saveSettings()
		return
	}
	if !a.rememberProcessing("translate", true) || a.quitAfterTask {
		return
	}
	a.translate()
}

func (a *NativeApp) restoreProcessing() {
	pending := a.settings.Processing
	if pending == nil {
		return
	}
	a.run(locales.TranslationResume(), func(context.Context) (func(), error) {
		store, err := project.Open(pending.ProjectPath)
		if err != nil {
			return nil, err
		}
		state, err := store.Snapshot()
		if err != nil {
			store.Close()
			return nil, err
		}
		url, previewErr := a.preview.Register(state.Manifest.MediaPath)
		return func() {
			a.replaceProject(store, state, url)
			a.settings.Processing = pending
			a.savedHash, a.savedPath = pending.SavedHash, pending.SavedPath
			a.refreshUnsaved()
			a.saveSettings()
			a.status = locales.ProcessingInterrupted()
			if previewErr != nil {
				a.showError("player", previewErr.Error())
			}
		}, nil
	})
}
