package main

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"path/filepath"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/apperr"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/jobs"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

type speechRegion struct {
	StartMs, EndMs int64
}

// speechRegions consumes the pinned FireRedVAD CLI's seconds-based output.
// Keep sentence pauses up to one second together and 250 ms of context on
// either side, but never join across a long silence. This is input selection,
// not another VAD implementation or a rewrite of OneAsr's chunk planner.
func speechRegions(data []byte) ([]speechRegion, error) {
	var wire struct {
		Duration        float64     `json:"dur"`
		Timestamps      [][]float64 `json:"timestamps"`
		SampleRateIn    int         `json:"sample_rate_in"`
		SampleRateModel int         `json:"sample_rate_model"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, err
	}
	if wire.Duration <= 0 || wire.Duration*1000 > 9007199254740991 || wire.Timestamps == nil || wire.SampleRateIn != 16000 || wire.SampleRateModel != 16000 {
		return nil, errors.New(locales.ErrSpeechRegions())
	}
	regions := []speechRegion{}
	previousEnd := float64(0)
	for _, pair := range wire.Timestamps {
		if len(pair) != 2 || pair[0] < previousEnd || pair[1] <= pair[0] || pair[1] > wire.Duration {
			return nil, errors.New(locales.ErrSpeechRegions())
		}
		previousEnd = pair[1]
		region := speechRegion{StartMs: int64(math.Round(pair[0] * 1000)), EndMs: int64(math.Round(pair[1] * 1000))}
		if region.EndMs <= region.StartMs {
			return nil, errors.New(locales.ErrSpeechRegions())
		}
		if len(regions) > 0 && region.StartMs-regions[len(regions)-1].EndMs <= 1000 {
			regions[len(regions)-1].EndMs = region.EndMs
		} else {
			regions = append(regions, region)
		}
	}
	duration := int64(math.Round(wire.Duration * 1000))
	for i := range regions {
		regions[i].StartMs = max(0, regions[i].StartMs-250)
		regions[i].EndMs = min(duration, regions[i].EndMs+250)
	}
	return regions, nil
}

// detectSpeech uses the same FireRedVAD v1.0.0 model and VadConfig defaults
// as OneAsr, including for short recordings. A missing/broken helper is an
// error; it must not silently fall back to feeding silence to the recognizer.
func detectSpeech(ctx context.Context, install, wav string) ([]speechRegion, error) {
	var output strings.Builder
	const limit = 4 * 1024 * 1024
	err := jobs.Run(ctx, jobs.CommandContext(ctx, filepath.Join(install, "fireredvad.exe"), "vad", wav), func(line string) {
		if output.Len() <= limit {
			output.WriteString(line)
			output.WriteByte('\n')
		}
	})
	if err != nil {
		return nil, apperr.Wrap(locales.ErrSpeechDetection(err.Error()), err)
	}
	if output.Len() > limit {
		return nil, errors.New(locales.ErrSpeechRegions())
	}
	return speechRegions([]byte(output.String()))
}

// recognitionRegion bounds normalization to actual input audio. In particular,
// a collapsed aligner tail cannot be extended into the excluded silence. Apply
// the region's source offset once, to both subtitles and word evidence.
func recognitionRegion(doc subtitle.Document, region speechRegion) subtitle.Document {
	duration := region.EndMs - region.StartMs
	local := doc
	local.Cues = nil
	for _, cue := range doc.Cues {
		if cue.StartMs < duration && cue.EndMs >= 0 {
			local.Cues = append(local.Cues, cue)
		}
	}
	local, _ = subtitle.NormalizeRecognition(local)
	out := local
	out.Cues = nil
	for _, cue := range local.Cues {
		cue.EndMs = min(cue.EndMs, duration)
		if cue.EndMs <= cue.StartMs {
			continue
		}
		cue.StartMs += region.StartMs
		cue.EndMs += region.StartMs
		cue.Position = len(out.Cues)
		out.Cues = append(out.Cues, cue)
	}
	return out
}

func recognitionWordRegion(words []subtitle.Word, region speechRegion) []subtitle.Word {
	out := []subtitle.Word{}
	duration := region.EndMs - region.StartMs
	for _, word := range words {
		if word.StartMs >= duration {
			continue
		}
		// Zero-duration tokens remain evidence, never invented split anchors.
		word.EndMs = min(word.EndMs, duration) + region.StartMs
		word.StartMs += region.StartMs
		out = append(out, word)
	}
	return out
}
