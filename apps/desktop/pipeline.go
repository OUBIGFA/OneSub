package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/apperr"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/jobs"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/models"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/styles"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// toolset is what recognition and translation run with: the bundled
// programs and the user's data folder. The window and the MCP server share
// it, so both make the same files the same way.
type toolset struct {
	resources, dataRoot, ffmpeg string
	worker                      subtitle.Client
	styles                      styles.Client
}

func (a *NativeApp) toolset() toolset {
	return toolset{resources: a.resources, dataRoot: a.dataRoot, ffmpeg: a.ffmpeg, worker: a.worker, styles: a.styleClient}
}

// modelPath is where a model's files are: the folder the user chose, or
// the default one in the data folder.
func (t toolset) modelPath(prefs settings.Settings, id string) string {
	if path := prefs.ModelPaths[id]; path != "" {
		return path
	}
	return filepath.Join(t.dataRoot, "models", id)
}

// recognitionModels checks that the chosen models' folders exist.
func (t toolset) recognitionModels(prefs settings.Settings) (asr, aligner models.Model, err error) {
	if asr, err = models.ByID(prefs.AsrModel); err != nil {
		return
	}
	if aligner, err = models.ByID(prefs.Aligner); err != nil {
		return
	}
	for _, m := range []models.Model{asr, aligner} {
		if _, statErr := os.Stat(t.modelPath(prefs, m.ID)); statErr != nil {
			return asr, aligner, errors.New(locales.ErrModelsNeeded())
		}
	}
	if strings.TrimSpace(prefs.Hotwords) != "" {
		// v1.3.0 does not consume --hotwords; never pretend a silently ignored
		// flag configured the core.
		return asr, aligner, errors.New(locales.ErrHotwordsUnsupported())
	}
	return asr, aligner, nil
}

// recognize runs the pinned OneAsr CLI on source and returns its SRT and
// word timings, in a scratch folder that cleanup removes. step hears the
// stages as they happen.
func (t toolset) recognize(ctx context.Context, prefs settings.Settings, source string, step func(recognitionStep)) (srt string, words []byte, cleanup func(), err error) {
	asr, aligner, err := t.recognitionModels(prefs)
	if err != nil {
		return "", nil, nil, err
	}
	asrDir, alignDir := t.modelPath(prefs, asr.ID), t.modelPath(prefs, aligner.ID)
	step(recognitionStep{Stage: "verify"})
	for _, m := range []struct {
		model models.Model
		dir   string
	}{{asr, asrDir}, {aligner, alignDir}} {
		if err = models.Verify(ctx, m.model, m.dir); err != nil {
			return "", nil, nil, err
		}
	}
	install := filepath.Join(t.resources, "oneasr")
	if value := os.Getenv("ONESUB_ONEASR_ROOT"); value != "" {
		install = value
	}
	root, removeRoot, err := models.RuntimeRoot(ctx, t.worker.Scratch, map[string]string{asr.ID: asrDir, aligner.ID: alignDir})
	if err != nil {
		return "", nil, nil, err
	}
	defer removeRoot()
	if prefs.VocalSeparation {
		model, err := models.ByID("htdemucs_ft")
		if err != nil {
			return "", nil, nil, err
		}
		if err = models.Verify(ctx, model, t.modelPath(prefs, model.ID)); err != nil {
			return "", nil, nil, err
		}
	}
	scratch, err := os.MkdirTemp(t.worker.Scratch, "asr-")
	if err != nil {
		return "", nil, nil, err
	}
	removeScratch := func() { os.RemoveAll(scratch) }
	cleanup = removeScratch
	defer func() {
		if err != nil {
			// Error returns clear the cleanup result; retain the owned closure
			// instead of calling that now-nil named return value.
			removeScratch()
		}
	}()
	wav := filepath.Join(scratch, "input.wav")
	step(recognitionStep{Stage: "extract"})
	args := []string{"-v", "error", "-i", source, "-map", "0:a:0", "-vn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", wav}
	if err = jobs.Run(ctx, jobs.CommandContext(ctx, t.ffmpeg, args...), nil); err != nil {
		return "", nil, nil, apperr.Wrap(locales.ErrAudioExtract(err.Error(), ""), err)
	}
	step(recognitionStep{Stage: "vad"})
	regions, err := detectSpeech(ctx, install, wav)
	if err != nil {
		return "", nil, nil, err
	}
	if len(regions) == 0 {
		return "", nil, nil, errors.New(locales.RecognitionNoSpeech())
	}
	combined := subtitle.Document{Format: "srt", TargetFormat: "srt"}
	allWords := []subtitle.Word{}
	for i, region := range regions {
		// OneAsr's own VAD plans continuous chunks; it does not remove silence.
		// Give it only this speech region, then map both artifacts back to the
		// original recording. Its models, decoder and chunk settings stay intact.
		regionStep := func(s recognitionStep) {
			s.Region, s.Regions = i+1, len(regions)
			step(s)
		}
		regionStep(recognitionStep{Stage: "extract"})
		part := filepath.Join(scratch, fmt.Sprintf("region-%06d", i))
		args = []string{"-v", "error", "-ss", strconv.FormatFloat(float64(region.StartMs)/1000, 'f', 3, 64), "-i", wav, "-t", strconv.FormatFloat(float64(region.EndMs-region.StartMs)/1000, 'f', 3, 64), "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", part + ".wav"}
		if err = jobs.Run(ctx, jobs.CommandContext(ctx, t.ffmpeg, args...), nil); err != nil {
			return "", nil, nil, apperr.Wrap(locales.ErrAudioExtract(err.Error(), ""), err)
		}
		args = []string{"transcribe", "--input", part + ".wav", "--app-root", install, "--data-root", root, "--asr", asr.ID, "--aligner", aligner.ID, "--language", prefs.Language, "--backend", prefs.Backend, "--chunk-seconds", strconv.Itoa(int(prefs.ChunkSeconds)), "--max-new-tokens", strconv.Itoa(int(prefs.MaxTokens)), "--output", part + ".srt", "--words-json", part + ".json"}
		if prefs.VocalSeparation {
			args = append(args, "--vocal-separation", "--demucs-model-dir", t.modelPath(prefs, "htdemucs_ft"))
		}
		regionStep(recognitionStep{Stage: "infer"})
		command := jobs.CommandContext(ctx, filepath.Join(install, "oneasr-cli.exe"), args...)
		// OneAsr falls back to PATH when its own bin folder has no FFmpeg. Give
		// this child the verified bundled tools without changing the user's PATH.
		command.Env = append(command.Environ(), "PATH="+filepath.Dir(t.ffmpeg)+string(os.PathListSeparator)+os.Getenv("PATH"))
		if err = jobs.RunLines(ctx, command, nil, func(line string) {
			if s, ok := parseOneAsrStage(line); ok {
				regionStep(s)
			}
		}); err != nil {
			return "", nil, nil, apperr.Wrap(locales.ErrRecognition(err.Error(), ""), err)
		}
		doc, readErr := t.worker.Read(ctx, part+".srt")
		if readErr != nil {
			return "", nil, nil, readErr
		}
		for _, cue := range recognitionRegion(doc, region).Cues {
			cue.Position = len(combined.Cues)
			cue.ID = fmt.Sprintf("asr-%d", cue.Position)
			combined.Cues = append(combined.Cues, cue)
		}
		raw, readErr := os.ReadFile(part + ".json")
		if readErr != nil {
			return "", nil, nil, apperr.Wrap(locales.ErrWordTimingMissing(readErr.Error()), readErr)
		}
		partWords, readErr := subtitle.ParseOneAsrWords(raw)
		if readErr != nil {
			return "", nil, nil, readErr
		}
		allWords = append(allWords, recognitionWordRegion(partWords, region)...)
	}
	step(recognitionStep{Stage: "save"})
	data, err := t.worker.Write(ctx, combined)
	if err != nil {
		return "", nil, nil, err
	}
	srt = filepath.Join(scratch, "result.srt")
	if err = os.WriteFile(srt, data, 0600); err != nil {
		return "", nil, nil, err
	}
	if words, err = subtitle.MarshalOneAsrWords(allWords); err != nil {
		return "", nil, nil, err
	}
	return srt, words, cleanup, nil
}

// recognitionFile is the recognized subtitle file in the chosen format:
// zero-length cues given a time, line-end punctuation removed as chosen,
// word timings checked, an ASS file styled with style. It also reports how
// many cues had no length.
func (t toolset) recognitionFile(ctx context.Context, srt string, prefs settings.Settings, style styles.Style, aspect float64) ([]byte, int, error) {
	doc, err := t.worker.Read(ctx, srt)
	if err != nil {
		return nil, 0, err
	}
	doc, repaired := subtitle.NormalizeRecognition(doc)
	data, err := t.renderRecognition(ctx, doc, prefs, style, aspect)
	return data, repaired, err
}

// renderRecognition writes the cues in the chosen format. Word timings
// only refine later splits and are checked as the window loads the file,
// where a fault is a notice, not a failed recognition.
func (t toolset) renderRecognition(ctx context.Context, doc subtitle.Document, prefs settings.Settings, style styles.Style, aspect float64) ([]byte, error) {
	if prefs.StripPunctuation {
		for i := range doc.Cues {
			doc.Cues[i].Text = stripTrailingPunctuation(doc.Cues[i].Text, prefs.KeepPunctuation)
		}
	}
	switch prefs.RecognitionFormat {
	case "ass":
		result, err := t.styles.Build(ctx, doc.Cues, style, aspect)
		if err != nil {
			return nil, err
		}
		return []byte(result.ASS), nil
	case "srt", "vtt":
		doc.TargetFormat = prefs.RecognitionFormat
		return t.worker.Write(ctx, doc)
	}
	return nil, errors.New(locales.RecognitionFormatInvalid())
}

// publishBeside writes data as stem.ext in directory, never over a file:
// a taken name gets -OneSub, then -OneSub-2 and on. Publish reserves the
// name atomically, so a file that appears meanwhile is not overwritten.
func publishBeside(directory, stem, ext string, data []byte) (string, error) {
	for n := 0; ; n++ {
		name := stem
		if n == 1 {
			name += "-OneSub"
		} else if n > 1 {
			name += fmt.Sprintf("-OneSub-%d", n)
		}
		path := filepath.Join(directory, name+"."+ext)
		err := subtitle.Publish(path, data)
		if err == nil {
			return path, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", err
		}
	}
}
