package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/apperr"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/jobs"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/models"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// recognize invokes the existing OneAsr CLI. The CLI remains the ASR/aligner
// authority; there is no replacement recognition implementation in the UI.
// Its result replaces the subtitles, so unsaved files are offered first.
func (a *NativeApp) recognize() {
	if a.snapshot == nil {
		return
	}
	source := a.snapshot.Manifest.MediaPath
	asr, err := models.ByID(a.settings.AsrModel)
	if err != nil {
		a.errorText = err.Error()
		return
	}
	aligner, err := models.ByID(a.settings.Aligner)
	if err != nil {
		a.errorText = err.Error()
		return
	}
	asrDir, alignDir := a.modelPath(asr.ID), a.modelPath(aligner.ID)
	for _, path := range []string{asrDir, alignDir} {
		if _, err := os.Stat(path); err != nil {
			a.settingsOpen = true
			a.settingsTab = 1
			a.errorText = locales.ErrModelsNeeded()
			return
		}
	}
	if strings.TrimSpace(a.settings.Hotwords) != "" {
		// v1.3.0 does not consume --hotwords; never pretend a silently ignored
		// flag configured the core. Keep the requested value for future support.
		a.errorText = locales.ErrHotwordsUnsupported()
		return
	}
	if a.guardUnsaved(a.recognize) {
		return
	}
	prefs := a.settings
	a.run(locales.ActionRecognize(), func(ctx context.Context) (func(), error) {
		if err := models.Verify(ctx, asr, asrDir); err != nil {
			return nil, err
		}
		if err := models.Verify(ctx, aligner, alignDir); err != nil {
			return nil, err
		}
		install := filepath.Join(a.resources, "oneasr")
		if value := os.Getenv("ONESUB_ONEASR_ROOT"); value != "" {
			install = value
		}
		root, cleanup, err := models.RuntimeRoot(ctx, a.worker.Scratch, map[string]string{asr.ID: asrDir, aligner.ID: alignDir})
		if err != nil {
			return nil, err
		}
		defer cleanup()
		if prefs.VocalSeparation {
			model, err := models.ByID("htdemucs_ft")
			if err != nil {
				return nil, err
			}
			if err = models.Verify(ctx, model, a.modelPath(model.ID)); err != nil {
				return nil, err
			}
		}
		scratch, err := os.MkdirTemp(filepath.Join(a.dataRoot, "cache", "worker"), "asr-")
		if err != nil {
			return nil, err
		}
		wav := filepath.Join(scratch, "input.wav")
		defer os.Remove(wav)
		args := []string{"-v", "error", "-i", source, "-map", "0:a:0", "-vn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", wav}
		if output, err := jobs.CommandContext(ctx, a.ffmpeg, args...).CombinedOutput(); err != nil {
			return nil, apperr.Wrap(locales.ErrAudioExtract(err.Error(), boundedText(output)), err)
		}
		output := filepath.Join(scratch, "result.srt")
		wordsPath := filepath.Join(scratch, "words.json")
		args = []string{"transcribe", "--input", wav, "--app-root", install, "--data-root", root, "--asr", asr.ID, "--aligner", aligner.ID, "--language", prefs.Language, "--backend", prefs.Backend, "--chunk-seconds", strconv.Itoa(int(prefs.ChunkSeconds)), "--max-new-tokens", strconv.Itoa(int(prefs.MaxTokens)), "--output", output, "--words-json", wordsPath}
		if prefs.VocalSeparation {
			args = append(args, "--vocal-separation", "--demucs-model-dir", a.modelPath("htdemucs_ft"))
		}
		if logs, err := jobs.CommandContext(ctx, filepath.Join(install, "oneasr-cli.exe"), args...).CombinedOutput(); err != nil {
			return nil, apperr.Wrap(locales.ErrRecognition(err.Error(), boundedText(logs)), err)
		}
		words, err := os.ReadFile(wordsPath)
		if err != nil {
			return nil, apperr.Wrap(locales.ErrWordTimingMissing(err.Error()), err)
		}
		return a.importFiles(ctx, []string{source, output}, words)
	})
}
func boundedText(data []byte) string {
	if len(data) > 2000 {
		data = data[len(data)-2000:]
	}
	return strings.TrimSpace(string(data))
}

// Actions open the native workflows; only validated candidates can be applied.
func (a *NativeApp) translate() {
	if a.snapshot == nil {
		return
	}
	a.commitEdit(true)
	a.reloadTranslationProfiles()
	if a.settings.ProviderURL == "" || a.settings.ProviderModel == "" {
		a.settingsOpen = true
		a.settingsTab = 2
		return
	}
	a.translationConfirm = true
}
func (a *NativeApp) compose() {
	if a.snapshot == nil {
		return
	}
	a.commitEdit(true)
	a.composeOpen = true
}
