package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/apperr"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/jobs"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/models"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

const (
	languageAuto = "auto"
	// probeSeconds of the recording's start are searched for speech to
	// detect its language; a recording without speech there is searched
	// whole.
	probeSeconds = 300
	// probeWindows of at most probeWindowMs speech each are identified.
	// Whisper reads 30 s at once; a few seconds of speech make it reliable,
	// 1.5 s did not (Cantonese was read as Japanese).
	probeWindows  = 3
	probeWindowMs = 10000
)

// recognitionRoute is the outcome of automatic recognition: the language
// detected, the model chosen for it, and whether the fallback model took
// over because the language stayed unclear or its model is missing.
type recognitionRoute struct {
	Language, Model string
	Fallback        bool
}

// routeModel is the model the user chose for a language group, or the
// group's default.
func routeModel(prefs settings.Settings, group string) string {
	if id := prefs.LanguageModels[group]; id != "" {
		if model, err := models.ByID(id); err == nil && model.GroupSupported(group) {
			return id
		}
	}
	return models.DefaultRoute(group)
}

// routeRecognition detects the spoken language of source and returns the
// preferences that recognize it: that language and its group's model.
// Detection votes over windows of speech: the language most windows agree
// on wins. Whisper does not tell Cantonese from Chinese and confuses short
// East Asian speech, so SenseVoiceSmall, which names its five languages,
// confirms a Chinese, Japanese or Korean vote when it is downloaded.
// Speech no window majority agrees on goes to SenseVoiceSmall with its own
// language detection, as does a language whose chosen model is missing
// when SenseVoiceSmall recognizes it.
func (t toolset) routeRecognition(ctx context.Context, prefs settings.Settings, source string, step func(recognitionStep)) (settings.Settings, recognitionRoute, error) {
	detector, err := models.ByID(models.LanguageDetector)
	if err != nil {
		return prefs, recognitionRoute{}, err
	}
	detectorDir := t.modelPath(prefs, detector.ID)
	if _, err = os.Stat(detectorDir); err != nil {
		return prefs, recognitionRoute{}, errors.New(locales.ErrLanguageDetectorNeeded())
	}
	step(recognitionStep{Stage: "lid"})
	if err = models.Verify(ctx, detector, detectorDir); err != nil {
		return prefs, recognitionRoute{}, err
	}
	scratch, err := os.MkdirTemp(t.worker.Scratch, "lid-")
	if err != nil {
		return prefs, recognitionRoute{}, err
	}
	defer os.RemoveAll(scratch)
	windows, wav, err := t.probeWindows(ctx, source, scratch)
	if err != nil {
		return prefs, recognitionRoute{}, err
	}
	if len(windows) == 0 {
		return prefs, recognitionRoute{}, errors.New(locales.RecognitionNoSpeech())
	}
	votes, err := t.identifyLanguages(ctx, detector, detectorDir, wav, scratch, windows)
	if err != nil {
		return prefs, recognitionRoute{}, err
	}
	language, decided := majority(votes)
	fallback, fallbackErr := models.ByID(models.FallbackModel)
	if fallbackErr != nil {
		return prefs, recognitionRoute{}, fallbackErr
	}
	fallbackDir := t.modelPath(prefs, fallback.ID)
	_, statErr := os.Stat(fallbackDir)
	senseVoice := statErr == nil
	if decided && senseVoice && (language == "zh" || language == "ja" || language == "ko") {
		tags, err := t.senseVoiceLanguages(ctx, fallback, fallbackDir, wav, scratch, windows)
		if err != nil {
			return prefs, recognitionRoute{}, err
		}
		if confirmed, ok := majority(tags); ok {
			language = confirmed
		}
	}
	out := prefs
	if !decided {
		if !senseVoice {
			return prefs, recognitionRoute{}, errors.New(locales.ErrRouteModelMissing(fallback.Label))
		}
		out.AsrModel, out.Language = fallback.ID, languageAuto
		return out, recognitionRoute{Language: languageAuto, Model: fallback.Label, Fallback: true}, nil
	}
	group := models.RouteGroup(language)
	if group == "" {
		return prefs, recognitionRoute{}, errors.New(locales.ErrLanguageNoModel(language))
	}
	model, err := models.ByID(routeModel(prefs, group))
	if err != nil {
		return prefs, recognitionRoute{}, err
	}
	route := recognitionRoute{Language: language, Model: model.Label}
	if !model.Supports(language) {
		return prefs, recognitionRoute{}, errors.New(locales.ErrLanguageUnsupported(model.Label))
	}
	if _, err = os.Stat(t.modelPath(prefs, model.ID)); err != nil {
		if !senseVoice || !fallback.Supports(language) {
			return prefs, recognitionRoute{}, errors.New(locales.ErrRouteModelMissing(model.Label))
		}
		model, route.Model, route.Fallback = fallback, fallback.Label, true
	}
	out.AsrModel, out.Language = model.ID, language
	return out, route, nil
}

// probeWindows extracts the first probeSeconds of source, or all of it
// when they hold no speech, and returns up to probeWindows speech windows
// of it: the first, middle and last regions, each at most probeWindowMs,
// on the clock of the returned WAV.
func (t toolset) probeWindows(ctx context.Context, source, scratch string) ([]speechRegion, string, error) {
	install := filepath.Join(t.resources, "oneasr")
	if value := os.Getenv("ONESUB_ONEASR_ROOT"); value != "" {
		install = value
	}
	wav := filepath.Join(scratch, "probe.wav")
	for _, whole := range []bool{false, true} {
		args := []string{"-v", "error", "-y", "-i", source, "-map", "0:a:0", "-vn"}
		if !whole {
			args = append(args, "-t", fmt.Sprint(probeSeconds))
		}
		args = append(args, "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", wav)
		if err := jobs.Run(ctx, jobs.CommandContext(ctx, t.ffmpeg, args...), nil); err != nil {
			return nil, "", apperr.Wrap(locales.ErrAudioExtract(err.Error(), ""), err)
		}
		regions, err := detectSpeech(ctx, install, wav, probeWindowMs)
		if err != nil {
			return nil, "", err
		}
		pcm, err := openPCM(wav)
		if err != nil {
			return nil, "", err
		}
		short := pcm.length/pcmBytesPerMs < probeSeconds*1000
		pcm.Close()
		// The start was the whole recording, or it has speech.
		if len(regions) > 0 || short || whole {
			return pickWindows(regions), wav, nil
		}
	}
	return nil, wav, nil
}

// pickWindows takes the first, middle and last of regions, distinct.
func pickWindows(regions []speechRegion) []speechRegion {
	picked := []speechRegion{}
	seen := map[int]bool{}
	for _, i := range []int{0, len(regions) / 2, len(regions) - 1} {
		if i >= 0 && i < len(regions) && !seen[i] && len(picked) < probeWindows {
			seen[i] = true
			picked = append(picked, regions[i])
		}
	}
	return picked
}

// identifyLanguages runs the language detector on each window. Its CLI
// takes one file a run and prints the language code on stdout.
func (t toolset) identifyLanguages(ctx context.Context, detector models.Model, dir, wav, scratch string, windows []speechRegion) ([]string, error) {
	rel, names, cleanup, err := t.prepareWindows(ctx, detector, dir, wav, scratch, windows)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	cli := filepath.Join(t.sherpaRuntime(), "sherpa-onnx-offline-language-identification.exe")
	votes := []string{}
	for _, name := range names {
		var output strings.Builder
		command := jobs.CommandContext(ctx, cli, "--whisper-encoder="+filepath.ToSlash(filepath.Join(rel, "tiny-encoder.int8.onnx")), "--whisper-decoder="+filepath.ToSlash(filepath.Join(rel, "tiny-decoder.int8.onnx")), "--num-threads=2", name)
		command.Dir = scratch
		if err = jobs.Run(ctx, command, func(line string) { output.WriteString(strings.TrimSpace(line)) }); err != nil {
			return nil, apperr.Wrap(locales.ErrSherpaRecognition(err.Error(), ""), err)
		}
		language := output.String()
		if language == "" || len(language) > 8 || strings.ContainsAny(language, " {") {
			return nil, errors.New(locales.ErrSherpaOutput())
		}
		votes = append(votes, language)
	}
	return votes, nil
}

// senseVoiceLanguages are the language tags SenseVoiceSmall gives the
// windows when it detects the language itself.
func (t toolset) senseVoiceLanguages(ctx context.Context, model models.Model, dir, wav, scratch string, windows []speechRegion) ([]string, error) {
	rel, names, cleanup, err := t.prepareWindows(ctx, model, dir, wav, scratch, windows)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	command := jobs.CommandContext(ctx, filepath.Join(t.sherpaRuntime(), "sherpa-onnx-offline.exe"), append(sherpaArgs(model, rel, languageAuto), names...)...)
	command.Dir = scratch
	var output strings.Builder
	if err = jobs.Run(ctx, command, func(line string) {
		if strings.HasPrefix(line, "{") {
			output.WriteString(line + "\n")
		}
	}); err != nil {
		return nil, apperr.Wrap(locales.ErrSherpaRecognition(err.Error(), ""), err)
	}
	results, err := subtitle.ParseSherpaResults(output.String())
	if err != nil {
		return nil, err
	}
	tags := []string{}
	for _, r := range results {
		if tag := strings.Trim(r.Lang, "<|>"); tag != "" && r.Text != "" {
			tags = append(tags, tag)
		}
	}
	return tags, nil
}

// prepareWindows writes the windows as WAV files in scratch and mounts the
// model folder there, so the CLI opens only ASCII relative paths.
func (t toolset) prepareWindows(ctx context.Context, model models.Model, dir, wav, scratch string, windows []speechRegion) (string, []string, func(), error) {
	root, remove, err := models.RuntimeRoot(ctx, scratch, map[string]string{model.ID: dir})
	if err != nil {
		return "", nil, nil, err
	}
	cleanup := func() { remove() }
	rel, err := filepath.Rel(scratch, filepath.Join(root, "models", model.ID))
	if err != nil {
		cleanup()
		return "", nil, nil, err
	}
	pcm, err := openPCM(wav)
	if err != nil {
		cleanup()
		return "", nil, nil, err
	}
	defer pcm.Close()
	names := []string{}
	for i, window := range windows {
		name := fmt.Sprintf("window-%d.wav", i)
		if err = pcm.write(filepath.Join(scratch, name), window); err != nil {
			cleanup()
			return "", nil, nil, err
		}
		names = append(names, name)
	}
	return rel, names, cleanup, nil
}

// majority is the value more than half of votes agree on.
func majority(votes []string) (string, bool) {
	counts := map[string]int{}
	for _, v := range votes {
		counts[v]++
		if counts[v]*2 > len(votes) {
			return v, true
		}
	}
	return "", false
}
