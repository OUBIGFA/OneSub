package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/jobs"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/media"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/models"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
)

type diarizeRequest struct {
	Root, WAV, Output string
	Count             int
}

const diarizationRevision = "sherpa-2.0.3/pyannote3-campplus-zh-en/1"

func (t toolset) speakerModels(ctx context.Context, prefs settings.Settings) error {
	for _, id := range []string{"speaker-segmentation", "speaker-embedding"} {
		model, err := models.ByID(id)
		if err != nil {
			return err
		}
		if err = models.Verify(ctx, model, t.modelPath(prefs, id)); err != nil {
			return fmt.Errorf("%s: %w", model.Label, err)
		}
	}
	return nil
}

func (t toolset) analyzeSpeakers(ctx context.Context, prefs settings.Settings, source, wav string, step func(recognitionStep)) (*subtitle.SpeakerState, error) {
	if prefs.SpeakerCount < 0 || prefs.SpeakerCount > 100 || prefs.SpeakerCount != float64(int(prefs.SpeakerCount)) {
		return nil, errors.New("speaker count must be an integer from 0 to 100")
	}
	if err := t.speakerModels(ctx, prefs); err != nil {
		return nil, err
	}
	probe := os.Getenv("ONESUB_FFPROBE")
	if probe == "" {
		probe = filepath.Join(filepath.Dir(t.ffmpeg), "ffprobe.exe")
	}
	info, err := media.Probe(ctx, probe, source)
	if err != nil {
		return nil, err
	}
	offset, err := speakerAudioOffset(info)
	if err != nil {
		return nil, err
	}
	scratch, err := os.MkdirTemp(t.worker.Scratch, "speakers-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(scratch)
	if wav == "" {
		wav = filepath.Join(scratch, "input.wav")
		step(recognitionStep{Stage: "extract"})
		if err = jobs.Run(ctx, jobs.CommandContext(ctx, t.ffmpeg, "-v", "error", "-i", source, "-map", "0:a:0", "-vn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", wav), nil); err != nil {
			return nil, err
		}
	}
	file, err := os.Open(wav)
	if err != nil {
		return nil, err
	}
	hash := sha256.New()
	buffer := make([]byte, 256*1024)
	for {
		if err = ctx.Err(); err != nil {
			break
		}
		n, readErr := file.Read(buffer)
		if n > 0 {
			hash.Write(buffer[:n])
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			err = readErr
			break
		}
	}
	file.Close()
	if err != nil {
		return nil, err
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	mediaFile, err := os.Stat(source)
	if err != nil {
		return nil, err
	}
	key := sha256.Sum256([]byte(fmt.Sprintf("%s/%s/%d/%d", diarizationRevision, digest, int(prefs.SpeakerCount), offset)))
	cache := filepath.Join(t.dataRoot, "cache", "speakers")
	if err = os.MkdirAll(cache, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(cache, fmt.Sprintf("%x.json", key))
	var state subtitle.SpeakerState
	if raw, e := os.ReadFile(path); e == nil {
		if json.Unmarshal(raw, &state) == nil && state.AnalysisID == fmt.Sprintf("%x", key) && state.MediaSHA256 == digest && subtitle.ValidateSpeakerState(&state) == nil {
			state.MediaPath = source
			state.MediaBytes, state.MediaModifiedNs = mediaFile.Size(), mediaFile.ModTime().UnixNano()
			return &state, nil
		}
	}
	root, cleanup, err := models.RuntimeRoot(ctx, scratch, map[string]string{"speaker-segmentation": t.modelPath(prefs, "speaker-segmentation"), "speaker-embedding": t.modelPath(prefs, "speaker-embedding")})
	if err != nil {
		return nil, err
	}
	defer cleanup()
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if override := os.Getenv("ONESUB_DIARIZE_WORKER"); override != "" {
		self = override
	}
	output := filepath.Join(scratch, "result.json")
	input, _ := json.Marshal(diarizeRequest{Root: root, WAV: wav, Output: output, Count: int(prefs.SpeakerCount)})
	cmd := jobs.CommandContext(ctx, self, "diarize", t.sherpaRuntime())
	cmd.Stdin = strings.NewReader(string(input))
	step(recognitionStep{Stage: "diarize"})
	if err = jobs.Run(ctx, cmd, func(line string) {
		var p struct{ Done, Total int }
		if json.Unmarshal([]byte(line), &p) == nil && p.Total > 0 {
			step(recognitionStep{Stage: "diarize", Done: p.Done, Total: p.Total})
		}
	}); err != nil {
		return nil, err
	}
	f, err := os.Open(output)
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(f, 64*1024*1024+1))
	f.Close()
	if err != nil {
		return nil, err
	}
	if len(raw) > 64*1024*1024 {
		return nil, errors.New("speaker result too large")
	}
	if err = json.Unmarshal(raw, &state); err != nil {
		return nil, err
	}
	if err = subtitle.ValidateSpeakerState(&state); err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	state.AnalysisID = fmt.Sprintf("%x", key)
	state.MediaSHA256 = digest
	state.OffsetMs = offset
	for i := range state.Turns {
		state.Turns[i].StartMs += offset
		state.Turns[i].EndMs += offset
	}
	state.MediaPath = source
	state.MediaBytes, state.MediaModifiedNs = mediaFile.Size(), mediaFile.ModTime().UnixNano()
	// IDs are stable within this analysis, never borrowed from another file or
	// re-clustering run whose numeric labels happen to be the same.
	names := map[string]string{}
	for id, name := range state.Names {
		names[state.AnalysisID[:16]+"/"+id] = name
	}
	for i := range state.Turns {
		state.Turns[i].ID = state.AnalysisID[:16] + "/" + state.Turns[i].ID
	}
	state.Names = names
	if err = subtitle.ValidateSpeakerState(&state); err != nil {
		return nil, err
	}
	raw, err = json.Marshal(state)
	if err != nil {
		return nil, err
	}
	if err = subtitle.Replace(path, raw); err != nil {
		return nil, err
	}
	return &state, nil
}
