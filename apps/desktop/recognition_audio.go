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

// speechRegions confirms the pinned FireRedVAD's candidate speech with AED.
// VAD also accepts singing and speech-like music. Require AED speech in at
// least half of each unpadded VAD candidate before joining or adding context.
// Keeping confirmed VAD boundaries preserves weak word onsets which AED may
// label as singing. AED labels are independent, so background music cannot
// overrule speech. See the upstream pipeline_vad_aed example and port notes.
// Keep sentence pauses up to one second together and 250 ms of context on
// either side, but never join across a long silence. This is input selection,
// not another VAD implementation or a rewrite of OneAsr's chunk planner.
// With maxMs above 0, no region lasts longer: speech is joined only while
// it fits, and a longer stretch of speech is cut into equal parts. Context
// never reaches past the middle of the pause to the next region, so no
// audio is recognized twice.
func speechRegions(vadData, aedData []byte, maxMs int64) ([]speechRegion, error) {
	var vad struct {
		Duration        float64     `json:"dur"`
		Timestamps      [][]float64 `json:"timestamps"`
		SampleRateIn    int         `json:"sample_rate_in"`
		SampleRateModel int         `json:"sample_rate_model"`
	}
	var aed struct {
		Duration float64 `json:"dur"`
		Events   struct {
			Speech [][]float64 `json:"speech"`
		} `json:"event2timestamps"`
	}
	if err := json.Unmarshal(vadData, &vad); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(aedData, &aed); err != nil {
		return nil, err
	}
	// A missing/null speech field is a broken detector response, not silence.
	// In particular, an old binary-VAD response must not bypass the AED gate.
	if vad.SampleRateIn != 16000 || vad.SampleRateModel != 16000 || math.Abs(vad.Duration-aed.Duration) > 0.001 {
		return nil, errors.New(locales.ErrSpeechRegions())
	}
	for _, timeline := range []struct {
		duration float64
		spans    [][]float64
	}{{vad.Duration, vad.Timestamps}, {aed.Duration, aed.Events.Speech}} {
		if timeline.duration <= 0 || timeline.duration*1000 > 9007199254740991 || timeline.spans == nil {
			return nil, errors.New(locales.ErrSpeechRegions())
		}
		previousEnd := float64(0)
		for _, pair := range timeline.spans {
			if len(pair) != 2 || pair[0] < previousEnd || pair[1] <= pair[0] || pair[1] > timeline.duration || math.Round(pair[1]*1000) <= math.Round(pair[0]*1000) {
				return nil, errors.New(locales.ErrSpeechRegions())
			}
			previousEnd = pair[1]
		}
	}
	regions := []speechRegion{}
	confirmed, firstEvent, mayJoin := aed.Events.Speech, 0, true
	for _, pair := range vad.Timestamps {
		for firstEvent < len(confirmed) && confirmed[firstEvent][1] <= pair[0] {
			firstEvent++
		}
		overlap := float64(0)
		for j := firstEvent; j < len(confirmed) && confirmed[j][0] < pair[1]; j++ {
			overlap += max(0, min(pair[1], confirmed[j][1])-max(pair[0], confirmed[j][0]))
		}
		if overlap*2 < pair[1]-pair[0] {
			// Do not reintroduce a rejected event by joining its neighbours.
			mayJoin = false
			continue
		}
		region := speechRegion{StartMs: int64(math.Round(pair[0] * 1000)), EndMs: int64(math.Round(pair[1] * 1000))}
		join := mayJoin
		mayJoin = true
		if n := len(regions); join && n > 0 && region.StartMs-regions[n-1].EndMs <= 1000 && (maxMs <= 0 || region.EndMs-regions[n-1].StartMs <= maxMs) {
			regions[n-1].EndMs = region.EndMs
			continue
		}
		if maxMs > 0 && region.EndMs-region.StartMs > maxMs {
			parts := (region.EndMs - region.StartMs + maxMs - 1) / maxMs
			length := (region.EndMs - region.StartMs + parts - 1) / parts
			for start := region.StartMs; start < region.EndMs; start += length {
				regions = append(regions, speechRegion{StartMs: start, EndMs: min(start+length, region.EndMs)})
			}
			continue
		}
		regions = append(regions, region)
	}
	duration := int64(math.Round(vad.Duration * 1000))
	padded := append([]speechRegion{}, regions...)
	for i := range padded {
		before, after := int64(250), int64(250)
		if i > 0 {
			before = min(before, (regions[i].StartMs-regions[i-1].EndMs)/2)
		}
		if i+1 < len(regions) {
			after = min(after, (regions[i+1].StartMs-regions[i].EndMs)/2)
		}
		padded[i].StartMs = max(0, regions[i].StartMs-before)
		padded[i].EndMs = min(duration, regions[i].EndMs+after)
	}
	return padded, nil
}

// detectSpeech applies the bundled FireRedVAD v1.0.0 VAD and AED, with their
// defaults, before every recognizer and language probe. Each runs once over
// the whole extracted WAV, including short recordings. A missing/broken
// detector is an error; neither detector may silently replace the other.
func detectSpeech(ctx context.Context, install, wav string, maxMs int64) ([]speechRegion, error) {
	const limit = 4 * 1024 * 1024
	outputs := [2][]byte{}
	for i, mode := range []string{"vad", "aed"} {
		var output strings.Builder
		err := jobs.Run(ctx, jobs.CommandContext(ctx, filepath.Join(install, "fireredvad.exe"), mode, wav), func(line string) {
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
		outputs[i] = []byte(output.String())
	}
	return speechRegions(outputs[0], outputs[1], maxMs)
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
