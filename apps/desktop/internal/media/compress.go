package media

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/jobs"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

const (
	CompressQualityNearLossless = "near-lossless"
	CompressQualityHigh         = "high"
	CompressQualityCompact      = "compact"

	crfStep                = 0.25
	crfSearchMaxIterations = 6
	crfScoreToleranceDB    = 0.25
	crfBoundsTolerance     = 0.5
	maxEncodedSizePercent  = 95
	xpsnrInfScore          = 100.0
)

// CompressPreset defines a perceptual SVT-AV1 compression target guided by
// FFmpeg's built-in Fraunhofer XPSNR filter. The sampling and interpolation
// search follows the open methodology of ab-av1 (MIT) without copying code or
// adding an external binary.
type CompressPreset struct {
	ID          string
	Preset      int
	TargetXPSNR float64
	MinCRF      float64
	MaxCRF      float64
	InitialCRF  float64
}

// CompressionReport records the perceptual search outcome inside ComposeReport.
type CompressionReport struct {
	Encoder          string  `json:"encoder"`
	Preset           int     `json:"preset"`
	CRF              float64 `json:"crf"`
	TargetXPSNR      float64 `json:"targetXpsnr"`
	MeasuredXPSNR    float64 `json:"measuredXpsnr"`
	EstimatedPercent int     `json:"estimatedPercent,omitempty"`
	Iterations       int     `json:"iterations"`
}

func CompressPresetFor(quality string) (CompressPreset, bool) {
	// SVT recommends presets 7-11 for fast encoding. Quality is still selected
	// by the measured target below, using the SAME preset for trials and output.
	// Presets 5/6 spent substantially more CPU on every trial and on the final
	// encode; preset 8 trades some compression efficiency for turnaround time.
	switch quality {
	case CompressQualityNearLossless:
		return CompressPreset{
			ID:          CompressQualityNearLossless,
			Preset:      8,
			TargetXPSNR: 42.0,
			MinCRF:      10.0,
			MaxCRF:      55.0,
			InitialCRF:  26.0,
		}, true
	case CompressQualityHigh:
		return CompressPreset{
			ID:          CompressQualityHigh,
			Preset:      8,
			TargetXPSNR: 39.0,
			MinCRF:      12.0,
			MaxCRF:      60.0,
			InitialCRF:  32.0,
		}, true
	case CompressQualityCompact:
		return CompressPreset{
			ID:          CompressQualityCompact,
			Preset:      8,
			TargetXPSNR: 36.0,
			MinCRF:      16.0,
			MaxCRF:      65.0,
			InitialCRF:  38.0,
		}, true
	default:
		return CompressPreset{}, false
	}
}

func formatCRF(crf float64) string {
	return strconv.FormatFloat(quantizeCRF(crf), 'f', -1, 64)
}

func quantizeCRF(crf float64) float64 {
	return math.Round(crf/crfStep) * crfStep
}

func svtAV1Params(crf float64) string {
	return fmt.Sprintf("crf=%s:tune=0:enable-variance-boost=1:enable-overlays=1", formatCRF(crf))
}

type sampleWindow struct {
	StartSec    float64
	DurationSec float64
}

func sampleWindows(durationMs int64, sample bool) []sampleWindow {
	total := float64(durationMs) / 1000.0
	if total <= 0 {
		return []sampleWindow{{StartSec: 0, DurationSec: 5}}
	}
	if sample {
		return []sampleWindow{{StartSec: 0, DurationSec: min(5.0, total)}}
	}
	if total <= 10.0 {
		return []sampleWindow{{StartSec: 0, DurationSec: total}}
	}
	if total <= 30.0 {
		length := 10.0
		start := max(0, (total-length)/2)
		return []sampleWindow{{StartSec: start, DurationSec: length}}
	}
	length := min(10.0, total*0.2)
	fractions := []float64{0.2, 0.5, 0.8}
	windows := make([]sampleWindow, 0, len(fractions))
	for _, f := range fractions {
		start := f*total - length/2
		if start < 0 {
			start = 0
		}
		if start+length > total {
			start = max(0, total-length)
		}
		windows = append(windows, sampleWindow{StartSec: start, DurationSec: length})
	}
	return windows
}

var xpsnrLinePattern = regexp.MustCompile(`XPSNR[ \t]+y:[ \t]*([0-9]+(?:\.[0-9]+)?|inf)[ \t]+u:[ \t]*([0-9]+(?:\.[0-9]+)?|inf)[ \t]+v:[ \t]*([0-9]+(?:\.[0-9]+)?|inf)`)

func parseXPSNRValue(raw string) (float64, bool) {
	if strings.EqualFold(raw, "inf") {
		return xpsnrInfScore, true
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v, true
}

// parseXPSNR extracts FFmpeg's summary XPSNR line and computes the 4:2:0
// weighted average (4*Y + U + V) / 6.
func parseXPSNR(line string) (float64, bool) {
	match := xpsnrLinePattern.FindStringSubmatch(line)
	if len(match) != 4 {
		return 0, false
	}
	y, okY := parseXPSNRValue(match[1])
	u, okU := parseXPSNRValue(match[2])
	v, okV := parseXPSNRValue(match[3])
	if !okY || !okU || !okV {
		return 0, false
	}
	return (4*y + u + v) / 6.0, true
}

type crfEvaluation struct {
	CRF          float64
	Score        float64
	EncodedBytes int64
}

type crfSearchOutcome struct {
	Best       crfEvaluation
	Iterations int
}

// searchCRF finds the highest CRF (smallest output) in [preset.MinCRF, preset.MaxCRF]
// whose perceptual XPSNR meets preset.TargetXPSNR, using bounded secant
// interpolation with binary-search fallback.
func searchCRF(preset CompressPreset, evaluate func(crf float64) (float64, int64, error)) (crfSearchOutcome, error) {
	var bestPass *crfEvaluation
	var bestFail *crfEvaluation
	tried := map[float64]bool{}

	current := quantizeCRF(min(preset.MaxCRF, max(preset.MinCRF, preset.InitialCRF)))
	var previous *crfEvaluation
	iterations := 0

	for iterations < crfSearchMaxIterations {
		// Reserve the last trial for the highest quality boundary if no pass
		// exists yet. This decides feasibility within the same encoding budget.
		if iterations == crfSearchMaxIterations-1 && bestPass == nil {
			current = preset.MinCRF
		}
		if tried[current] {
			break
		}
		tried[current] = true
		iterations++

		score, encodedBytes, err := evaluate(current)
		if err != nil {
			return crfSearchOutcome{}, err
		}
		eval := crfEvaluation{CRF: current, Score: score, EncodedBytes: encodedBytes}

		if score >= preset.TargetXPSNR {
			if bestPass == nil || eval.CRF > bestPass.CRF {
				copyEval := eval
				bestPass = &copyEval
			}
			if score-preset.TargetXPSNR <= crfScoreToleranceDB {
				break
			}
		} else {
			if bestFail == nil || eval.CRF < bestFail.CRF {
				copyEval := eval
				bestFail = &copyEval
			}
		}

		next, ok := nextSearchCRF(preset, bestPass, bestFail, eval, tried)
		// Before a pass/fail bracket exists, use the measured slope instead of
		// repeatedly guessing a fixed dB-to-CRF step. Bound extrapolation and
		// keep the original search when a flat/noisy curve predicts no progress.
		if previous != nil && (bestPass == nil || bestFail == nil) {
			slope := (eval.Score - previous.Score) / (eval.CRF - previous.CRF)
			if slope < -0.01 {
				candidate := eval.CRF + (preset.TargetXPSNR-eval.Score)/slope
				candidate = quantizeCRF(min(preset.MaxCRF, max(preset.MinCRF, candidate)))
				forward := eval.Score >= preset.TargetXPSNR && candidate > eval.CRF || eval.Score < preset.TargetXPSNR && candidate < eval.CRF
				if forward && !tried[candidate] {
					next, ok = candidate, true
				}
			}
		}
		if !ok {
			break
		}
		previous = &eval
		current = next
	}

	// A failed search must not trigger a full encode below the chosen quality.
	if bestPass == nil {
		return crfSearchOutcome{}, errors.New(locales.ErrCompressQualityTarget())
	}
	return crfSearchOutcome{Best: *bestPass, Iterations: iterations}, nil
}

func nextSearchCRF(preset CompressPreset, bestPass, bestFail *crfEvaluation, last crfEvaluation, tried map[float64]bool) (float64, bool) {
	if bestPass != nil && bestFail != nil {
		span := bestFail.CRF - bestPass.CRF
		if span <= crfBoundsTolerance {
			return 0, false
		}
		var candidate float64
		if bestPass.Score > bestFail.Score {
			ratio := (bestPass.Score - preset.TargetXPSNR) / (bestPass.Score - bestFail.Score)
			ratio = min(0.85, max(0.15, ratio))
			candidate = bestPass.CRF + ratio*span
		} else {
			candidate = (bestPass.CRF + bestFail.CRF) / 2.0
		}
		candidate = quantizeCRF(candidate)
		if candidate <= bestPass.CRF {
			candidate = bestPass.CRF + crfStep
		}
		if candidate >= bestFail.CRF {
			candidate = bestFail.CRF - crfStep
		}
		if candidate > bestPass.CRF && candidate < bestFail.CRF && !tried[candidate] {
			return candidate, true
		}
		mid := quantizeCRF((bestPass.CRF + bestFail.CRF) / 2.0)
		if mid > bestPass.CRF && mid < bestFail.CRF && !tried[mid] {
			return mid, true
		}
		return 0, false
	}

	if last.Score >= preset.TargetXPSNR {
		if last.CRF >= preset.MaxCRF {
			return 0, false
		}
		delta := max(4.0, (last.Score-preset.TargetXPSNR)*1.5)
		candidate := quantizeCRF(min(preset.MaxCRF, last.CRF+delta))
		if candidate == last.CRF {
			candidate = quantizeCRF(min(preset.MaxCRF, last.CRF+crfStep))
		}
		if tried[candidate] {
			return 0, false
		}
		return candidate, true
	}

	if last.CRF <= preset.MinCRF {
		return 0, false
	}
	delta := max(4.0, (preset.TargetXPSNR-last.Score)*1.5)
	candidate := quantizeCRF(max(preset.MinCRF, last.CRF-delta))
	if candidate == last.CRF {
		candidate = quantizeCRF(max(preset.MinCRF, last.CRF-crfStep))
	}
	if tried[candidate] {
		return 0, false
	}
	return candidate, true
}

func estimateOutputPercent(sourceBytes int64, totalDurationSec float64, sampleEncodedBytes int64, sampleDurationSec float64) int {
	if sourceBytes <= 0 || totalDurationSec <= 0 || sampleEncodedBytes <= 0 || sampleDurationSec <= 0 {
		return 0
	}
	estimatedVideoBytes := (float64(sampleEncodedBytes) / sampleDurationSec) * totalDurationSec
	return int(math.Round((estimatedVideoBytes / float64(sourceBytes)) * 100.0))
}

func searchCompressionCRF(
	ctx context.Context,
	ffmpeg string,
	scratch string,
	options ComposeOptions,
	info Info,
	durationMs int64,
	progress func(ComposeProgress),
) (float64, *CompressionReport, error) {
	preset, ok := CompressPresetFor(options.Quality)
	if !ok {
		return 0, nil, errors.New(locales.ErrComposeQuality())
	}
	var mainVideo Stream
	foundVideo := false
	for _, s := range compositionStreams(info) {
		if s.CodecType == "video" && s.Disposition.AttachedPic == 0 {
			mainVideo = s
			foundVideo = true
			break
		}
	}
	if !foundVideo {
		return 0, nil, errors.New(locales.ErrComposeMainVideo())
	}

	sampleDir := filepath.Join(scratch, "samples")
	if err := os.MkdirAll(sampleDir, 0700); err != nil {
		return 0, nil, err
	}

	windows := sampleWindows(durationMs, options.Sample)
	progress(ComposeProgress{Stage: StageAnalyze, Fraction: 0})

	refs := make([]string, 0, len(windows))
	totalSampleSec := 0.0
	for i, w := range windows {
		if err := ctx.Err(); err != nil {
			return 0, nil, err
		}
		refPath := filepath.Join(sampleDir, fmt.Sprintf("ref-%d.mkv", i))
		extractArgs := []string{"-hide_banner", "-nostdin", "-v", "error"}
		if w.StartSec > 0 {
			extractArgs = append(extractArgs, "-ss", strconv.FormatFloat(w.StartSec, 'f', 3, 64))
		}
		extractArgs = append(extractArgs,
			"-i", options.Source,
			"-t", strconv.FormatFloat(w.DurationSec, 'f', 3, 64),
			"-map", fmt.Sprintf("0:%d", mainVideo.Index),
			"-filter:v:0", "crop=trunc(iw/2)*2:trunc(ih/2)*2:0:0",
			"-c:v", "libx264",
			"-qp", "0",
			"-preset", "ultrafast",
			"-fps_mode:v:0", "passthrough",
			"-an", "-sn", "-dn",
			"-progress", "pipe:1", "-nostats",
			"-y", refPath,
		)
		if err := jobs.Run(ctx, exec.Command(ffmpeg, extractArgs...), func(line string) {
			if fraction, ok := composeFraction(line, int64(w.DurationSec*1000)); ok {
				progress(ComposeProgress{Stage: StageAnalyze, Fraction: 0.15 * (float64(i) + fraction) / float64(len(windows))})
			}
		}); err != nil {
			return 0, nil, err
		}
		refs = append(refs, refPath)
		totalSampleSec += w.DurationSec
		progress(ComposeProgress{Stage: StageAnalyze, Fraction: 0.15 * float64(i+1) / float64(len(windows))})
	}

	evalCount := 0
	outcome, err := searchCRF(preset, func(crf float64) (float64, int64, error) {
		evalCount++
		scoreSum := 0.0
		var bytesSum int64
		for i, refPath := range refs {
			if err := ctx.Err(); err != nil {
				return 0, 0, err
			}
			encPath := filepath.Join(sampleDir, fmt.Sprintf("enc-%d-%d.mkv", evalCount, i))
			// FFmpeg's progress stream keeps long trials visibly active. Reserve
			// the last fifth of each trial for its full-frame XPSNR measurement.
			trialProgress := func(offset, weight float64) func(string) {
				return func(line string) {
					if fraction, ok := composeFraction(line, int64(windows[i].DurationSec*1000)); ok {
						done := float64((evalCount-1)*len(refs)+i) + offset + weight*fraction
						progress(ComposeProgress{Stage: StageAnalyze, Fraction: min(0.99, 0.15+0.84*done/float64(crfSearchMaxIterations*len(refs)))})
					}
				}
			}
			encArgs := []string{
				"-hide_banner", "-nostdin", "-v", "error",
				"-i", refPath,
				"-c:v", "libsvtav1",
				"-preset", strconv.Itoa(preset.Preset),
				"-pix_fmt", "yuv420p10le",
				"-svtav1-params", svtAV1Params(crf),
				"-fps_mode:v:0", "passthrough",
				"-an", "-sn", "-dn",
				"-progress", "pipe:1", "-nostats",
				"-y", encPath,
			}
			if err := jobs.Run(ctx, exec.Command(ffmpeg, encArgs...), trialProgress(0, 0.8)); err != nil {
				return 0, 0, err
			}
			stat, err := os.Stat(encPath)
			if err != nil {
				return 0, 0, err
			}
			bytesSum += stat.Size()

			xpsnrArgs := []string{
				"-hide_banner", "-nostdin", "-v", "info",
				"-i", encPath,
				"-i", refPath,
				// XPSNR weights its FIRST (reference) input. FFmpeg 9.1's
				// high-bit-depth path assumes a tightly packed second input,
				// which AV1 decoders do not guarantee (vf_xpsnr.c: s_rec).
				// Its 8-bit path explicitly copies both strides correctly.
				// Normalize only the measurement; trial/final encodes stay 10-bit.
				"-filter_complex", "[0:v]setpts=PTS-STARTPTS,format=yuv420p[d];[1:v]setpts=PTS-STARTPTS,format=yuv420p[r];[r][d]xpsnr=ts_sync_mode=nearest",
				"-an", "-sn", "-dn", "-progress", "pipe:1", "-nostats",
				"-f", "null", "-",
			}
			score := 0.0
			gotScore := false
			err = jobs.RunLines(ctx, exec.Command(ffmpeg, xpsnrArgs...), trialProgress(0.8, 0.2), func(line string) {
				if parsed, ok := parseXPSNR(line); ok {
					score = parsed
					gotScore = true
				}
			})
			_ = os.Remove(encPath)
			if err != nil {
				return 0, 0, err
			}
			if !gotScore {
				return 0, 0, errors.New("xpsnr filter produced no score")
			}
			scoreSum += score

			doneSteps := float64((evalCount-1)*len(refs) + (i + 1))
			totalSteps := float64(crfSearchMaxIterations * len(refs))
			progress(ComposeProgress{Stage: StageAnalyze, Fraction: min(0.99, 0.15+0.84*(doneSteps/totalSteps))})
		}
		return scoreSum / float64(len(refs)), bytesSum, nil
	})
	if err != nil {
		return 0, nil, err
	}
	progress(ComposeProgress{Stage: StageAnalyze, Fraction: 1})

	var estimatedPercent int
	if sourceStat, statErr := os.Stat(options.Source); statErr == nil && durationMs > 0 {
		estimatedPercent = estimateOutputPercent(sourceStat.Size(), float64(durationMs)/1000.0, outcome.Best.EncodedBytes, totalSampleSec)
	}
	report := &CompressionReport{
		Encoder:          "libsvtav1",
		Preset:           preset.Preset,
		CRF:              outcome.Best.CRF,
		TargetXPSNR:      preset.TargetXPSNR,
		MeasuredXPSNR:    math.Round(outcome.Best.Score*100) / 100,
		EstimatedPercent: estimatedPercent,
		Iterations:       outcome.Iterations,
	}
	if options.Mode == "compress" && !options.Sample && estimatedPercent >= maxEncodedSizePercent {
		return outcome.Best.CRF, report, errors.New(locales.ErrCompressNoSavings(strconv.Itoa(estimatedPercent)))
	}
	return outcome.Best.CRF, report, nil
}
