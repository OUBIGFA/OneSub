package main

import (
	"context"
	"strings"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// recognize invokes the chosen model's CLI: OneAsr, or sherpa-onnx. The CLI
// remains the ASR/aligner authority; there is no replacement recognition
// implementation in the UI.
// Its result replaces the subtitles, so unsaved files are offered first.
func (a *NativeApp) recognize() {
	a.recognizeWithTranslation(false)
}

func (a *NativeApp) recognizeWithTranslation(translate bool, continuing ...bool) {
	if a.busy || a.snapshot == nil || a.snapshot.Manifest.MediaPath == "" {
		return
	}
	if translate {
		a.commitProviderModelInput()
	}
	if translate && (a.settings.ProviderURL == "" || a.settings.ProviderModel == "") {
		a.settingsOpen, a.settingsTab = true, 2
		return
	}
	source := a.snapshot.Manifest.MediaPath
	t := a.toolset()
	if _, _, err := t.recognitionModels(a.settings); err != nil {
		// Missing models open the settings that get them.
		if err.Error() == locales.ErrModelsNeeded() {
			a.settingsOpen = true
			a.settingsTab = 1
		}
		a.showError(locales.ActionRecognize(), err.Error())
		return
	}
	if a.guardUnsaved(func() { a.recognizeWithTranslation(translate) }) {
		return
	}
	if !a.rememberProcessing("recognize", translate) {
		return
	}
	prefs, style, aspect := a.recognitionPreferences(), a.currentStyle, a.frameAspect
	a.recognitionRunning = true
	a.recognitionStarted = time.Now()
	a.recognitionStep, a.recognitionRoute = recognitionStep{Stage: "verify"}, recognitionRoute{}
	a.evalPlayer("video.pause()")
	a.runTimed(timingRecognition, len(continuing) > 0 && continuing[0], locales.ActionRecognize(), func(ctx context.Context) (func(), error) {
		srt, words, cleanup, err := t.recognize(ctx, prefs, source, a.recognitionStepChanged)
		if err != nil {
			return nil, err
		}
		defer cleanup()
		return a.finishRecognition(ctx, source, srt, words, prefs, style, aspect)
	})
}
func boundedText(data []byte) string {
	if len(data) > 2000 {
		data = data[len(data)-2000:]
	}
	return strings.TrimSpace(string(data))
}

// translate starts a translation at once. Without a service or model it
// opens the settings that set them instead.
func (a *NativeApp) translate() {
	if a.busy || a.snapshot == nil {
		return
	}
	if !a.commitEdit(true) {
		return
	}
	a.commitProviderModelInput()
	a.reloadTranslationProfiles()
	if a.settings.ProviderURL == "" || a.settings.ProviderModel == "" {
		a.settingsOpen = true
		a.settingsTab = 2
		return
	}
	a.startTranslation()
}
func (a *NativeApp) compose() {
	if a.snapshot == nil {
		return
	}
	if !a.commitEdit(true) {
		return
	}
	a.composeOpen = true
	a.composeSkipMissing = false
}
