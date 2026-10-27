package main

import (
	"context"
	"fmt"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/jobs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/models"
)

// recognize invokes the existing OneAsr CLI. The CLI remains the ASR/aligner
// authority; there is no replacement recognition implementation in the UI.
func (a *NativeApp) recognize() {
	if a.snapshot == nil || a.guardChanges(a.recognize) {
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
			a.settingsTab = 2
			a.errorText = "请下载或选择识别与对齐模型"
			return
		}
	}
	prefs := a.settings
	a.run("识别字幕", func(ctx context.Context) (func(), error) {
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
		// The unpatched upstream CLI resolves both catalog IDs under data-root/models.
		// A mismatched custom layout must fail visibly, never silently load another model.
		root := filepath.Dir(filepath.Dir(asrDir))
		if filepath.Base(filepath.Dir(asrDir)) != "models" || filepath.Dir(alignDir) != filepath.Dir(asrDir) {
			return nil, fmt.Errorf("模型目录需要共享 models 根目录；独立目录适配尚未通过验收")
		}
		scratch, err := os.MkdirTemp(filepath.Join(a.dataRoot, "cache", "worker"), "asr-")
		if err != nil {
			return nil, err
		}
		wav := filepath.Join(scratch, "input.wav")
		args := []string{"-v", "error", "-i", source, "-map", "0:a:0", "-vn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", wav}
		if output, err := jobs.CommandContext(ctx, a.ffmpeg, args...).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("音轨提取失败：%w；%s", err, boundedText(output))
		}
		output := filepath.Join(scratch, "result.srt")
		args = []string{"transcribe", "--input", wav, "--app-root", install, "--data-root", root, "--asr", asr.ID, "--aligner", aligner.ID, "--language", prefs.Language, "--backend", prefs.Backend, "--chunk-seconds", strconv.Itoa(int(prefs.ChunkSeconds)), "--max-new-tokens", strconv.Itoa(int(prefs.MaxTokens)), "--output", output}
		if prefs.VocalSeparation {
			args = append(args, "--vocal-separation", "--demucs-model-dir", a.modelPath("htdemucs_ft"))
		}
		if strings.TrimSpace(prefs.Hotwords) != "" {
			args = append(args, "--hotwords", prefs.Hotwords, "--sensevoice-model-dir", a.modelPath("SenseVoiceSmall"))
		}
		if logs, err := jobs.CommandContext(ctx, filepath.Join(install, "oneasr-cli.exe"), args...).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("OneAsr 识别失败：%w；%s", err, boundedText(logs))
		}
		return a.importFiles(ctx, []string{source, output})
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
	if a.snapshot == nil || a.guardChanges(a.translate) {
		return
	}
	if a.settings.ProviderURL == "" || a.settings.ProviderModel == "" {
		a.settingsOpen = true
		a.settingsTab = 3
		return
	}
	a.translationConfirm = true
}
func (a *NativeApp) compose() {
	if a.snapshot == nil || a.guardChanges(a.compose) {
		return
	}
	if a.composeMode == "" {
		a.composeMode = "硬字幕（烧录） · MP4"
	}
	if a.composeQuality == "" {
		a.composeQuality = "CRF 20"
	}
	a.composeOpen = true
}
