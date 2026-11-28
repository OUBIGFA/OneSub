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
	"sync"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/jobs"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/videoencoding"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

const (
	crfStep                = 0.25
	crfSearchMaxIterations = 6
	crfScoreToleranceDB    = 0.25
	crfBoundsTolerance     = 0.5
	maxEncodedSizePercent  = 95
	xpsnrInfScore          = 100.0
)

// CompressPreset defines a perceptual compression target guided by FFmpeg's
// built-in Fraunhofer XPSNR filter. The sampling and interpolation search
// follows the open methodology of ab-av1 (MIT) without copying code or adding
// an external binary.
type CompressPreset struct {
	ID          string
	Preset      int
	TargetXPSNR float64
	MinCRF      float64
	MaxCRF      float64
	InitialCRF  float64
	Step        float64
}

// CompressionReport records the perceptual search outcome inside ComposeReport.
type CompressionReport struct {
	Encoder          string  `json:"encoder"`
	Preset           int     `json:"preset"`
	CRF              float64 `json:"crf"`
	TargetXPSNR      float64 `json:"targetXpsnr"`
	FloorXPSNR       float64 `json:"floorXpsnr,omitempty"` // CPU-equivalent floor of a GPU search
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
	case videoencoding.QualityNearLossless:
		return CompressPreset{
			ID:          videoencoding.QualityNearLossless,
			Preset:      8,
			TargetXPSNR: 42.0,
			MinCRF:      10.0,
			MaxCRF:      55.0,
			InitialCRF:  26.0,
		}, true
	case videoencoding.QualityHigh:
		return CompressPreset{
			ID:          videoencoding.QualityHigh,
			Preset:      8,
			TargetXPSNR: 39.0,
			MinCRF:      12.0,
			MaxCRF:      60.0,
			InitialCRF:  32.0,
		}, true
	case videoencoding.QualityCompact:
		return CompressPreset{
			ID:          videoencoding.QualityCompact,
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

func presetStep(preset CompressPreset) float64 {
	if preset.Step > 0 {
		return preset.Step
	}
	return crfStep
}

func formatCRF(crf float64) string {
	return strconv.FormatFloat(quantizeCRF(crf), 'f', -1, 64)
}

func quantizeCRF(crf float64) float64 {
	return quantizeWithStep(crf, crfStep)
}

func quantizeWithStep(value, step float64) float64 {
	if step <= 0 {
		step = crfStep
	}
	return math.Round(value/step) * step
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

// searchCRF finds the highest quality parameter (smallest output) in
// [preset.MinCRF, preset.MaxCRF] whose perceptual XPSNR meets
// preset.TargetXPSNR, using bounded secant interpolation with binary-search
// fallback.
func searchCRF(preset CompressPreset, evaluate func(crf float64) (float64, int64, error)) (crfSearchOutcome, error) {
	var bestPass *crfEvaluation
	var bestFail *crfEvaluation
	tried := map[float64]bool{}
	step := presetStep(preset)

	current := quantizeWithStep(min(preset.MaxCRF, max(preset.MinCRF, preset.InitialCRF)), step)
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
				candidate = quantizeWithStep(min(preset.MaxCRF, max(preset.MinCRF, candidate)), step)
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
	step := presetStep(preset)
	boundsTolerance := max(crfBoundsTolerance, step)
	if bestPass != nil && bestFail != nil {
		span := bestFail.CRF - bestPass.CRF
		if span <= boundsTolerance {
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
		candidate = quantizeWithStep(candidate, step)
		if candidate <= bestPass.CRF {
			candidate = bestPass.CRF + step
		}
		if candidate >= bestFail.CRF {
			candidate = bestFail.CRF - step
		}
		if candidate > bestPass.CRF && candidate < bestFail.CRF && !tried[candidate] {
			return candidate, true
		}
		mid := quantizeWithStep((bestPass.CRF+bestFail.CRF)/2.0, step)
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
		candidate := quantizeWithStep(min(preset.MaxCRF, last.CRF+delta), step)
		if candidate == last.CRF {
			candidate = quantizeWithStep(min(preset.MaxCRF, last.CRF+step), step)
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
	candidate := quantizeWithStep(max(preset.MinCRF, last.CRF-delta), step)
	if candidate == last.CRF {
		candidate = quantizeWithStep(max(preset.MinCRF, last.CRF-step), step)
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

// sampleTrial encodes one lossless reference window at quality q.
type sampleTrial func(ctx context.Context, refPath, encPath string, windowMs int64, q float64, onProgress func(float64)) error

func runSampleEncode(ctx context.Context, ffmpeg, refPath, encPath string, windowMs int64, video []string, onProgress func(float64)) error {
	args := append([]string{"-hide_banner", "-nostdin", "-v", "error", "-i", refPath, "-map", "0:v:0"}, video...)
	args = append(args, "-fps_mode:v:0", "passthrough", "-an", "-sn", "-dn", "-progress", "pipe:1", "-nostats", "-y", encPath)
	return jobs.Run(ctx, exec.Command(ffmpeg, args...), func(line string) {
		if fraction, ok := composeFraction(line, windowMs); ok {
			onProgress(fraction)
		}
	})
}

func svtSampleTrial(ffmpeg string, preset CompressPreset) sampleTrial {
	return func(ctx context.Context, refPath, encPath string, windowMs int64, crf float64, onProgress func(float64)) error {
		return runSampleEncode(ctx, ffmpeg, refPath, encPath, windowMs, []string{
			"-c:v:0", "libsvtav1", "-preset", strconv.Itoa(preset.Preset), "-pix_fmt:v:0", "yuv420p10le", "-svtav1-params", svtAV1Params(crf),
		}, onProgress)
	}
}

// qualitySearch describes one perceptual search. floor, when set, is a
// reference encode whose measured score raises the target before the search,
// so one encoder cannot settle below another one's output for the same preset.
type qualitySearch struct {
	preset   CompressPreset
	encoder  string
	trial    sampleTrial
	parallel bool // run the windows of one trial concurrently
	floor    *qualityFloor
}

type qualityFloor struct {
	trial sampleTrial
	q     float64
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
	return searchQualityTarget(ctx, ffmpeg, scratch, options, info, durationMs, qualitySearch{preset: preset, encoder: "libsvtav1", trial: svtSampleTrial(ffmpeg, preset)}, progress)
}

// searchGPUCompressionQuality keeps GPU output from falling below the CPU
// output of the same preset. The CPU search stops at MaxCRF, so its effective
// quality is the target or the score at MaxCRF, whichever is higher. That
// score is measured on the same samples with SVT-AV1 and used as the floor.
func searchGPUCompressionQuality(
	ctx context.Context,
	ffmpeg string,
	scratch string,
	options ComposeOptions,
	info Info,
	durationMs int64,
	encoder hardwareEncoder,
	pixels int,
	progress func(ComposeProgress),
) (float64, *CompressionReport, error) {
	preset, ok := gpuPresetFor(encoder, options.Quality)
	if !ok {
		return 0, nil, errors.New(locales.ErrComposeQuality())
	}
	cpu, _ := CompressPresetFor(options.Quality)
	trial := func(ctx context.Context, refPath, encPath string, windowMs int64, q float64, onProgress func(float64)) error {
		return runSampleEncode(ctx, ffmpeg, refPath, encPath, windowMs, encoder.videoArgs(q, pixels), onProgress)
	}
	return searchQualityTarget(ctx, ffmpeg, scratch, options, info, durationMs, qualitySearch{
		preset: preset, encoder: encoder.Name, trial: trial, parallel: true,
		floor: &qualityFloor{trial: svtSampleTrial(ffmpeg, cpu), q: cpu.MaxCRF},
	}, progress)
}

// searchProgress merges concurrent window progress into one monotonic value.
type searchProgress struct {
	mu     sync.Mutex
	report func(ComposeProgress)
	total  float64
	done   float64
	active map[string]float64
	last   float64
}

func (p *searchProgress) set(key string, fraction float64, finished bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if finished {
		delete(p.active, key)
		p.done++
	} else {
		p.active[key] = fraction
	}
	sum := p.done
	for _, f := range p.active {
		sum += f
	}
	value := min(0.99, 0.15+0.84*sum/p.total)
	if value > p.last {
		p.last = value
		p.report(ComposeProgress{Stage: StageAnalyze, Fraction: value})
	}
}

func searchQualityTarget(
	ctx context.Context,
	ffmpeg string,
	scratch string,
	options ComposeOptions,
	info Info,
	durationMs int64,
	search qualitySearch,
	progress func(ComposeProgress),
) (float64, *CompressionReport, error) {
	preset := search.preset
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

	units := float64(crfSearchMaxIterations * len(refs))
	if search.floor != nil {
		units += float64(len(refs))
	}
	tracker := &searchProgress{report: progress, total: units, active: map[string]float64{}}

	// measureWindow encodes one window, scores it against its reference and
	// deletes the trial file.
	measureWindow := func(ctx context.Context, trial sampleTrial, tag string, i int, q float64) (float64, int64, error) {
		key := fmt.Sprintf("%s-%d", tag, i)
		windowMs := int64(windows[i].DurationSec * 1000)
		encPath := filepath.Join(sampleDir, "enc-"+key+".mkv")
		defer os.Remove(encPath)
		if err := trial(ctx, refs[i], encPath, windowMs, q, func(f float64) { tracker.set(key, 0.8*f, false) }); err != nil {
			return 0, 0, err
		}
		stat, err := os.Stat(encPath)
		if err != nil {
			return 0, 0, err
		}
		xpsnrArgs := []string{
			"-hide_banner", "-nostdin", "-v", "info",
			"-i", encPath,
			"-i", refs[i],
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
		err = jobs.RunLines(ctx, exec.Command(ffmpeg, xpsnrArgs...), func(line string) {
			if fraction, ok := composeFraction(line, windowMs); ok {
				tracker.set(key, 0.8+0.2*fraction, false)
			}
		}, func(line string) {
			if parsed, ok := parseXPSNR(line); ok {
				score = parsed
				gotScore = true
			}
		})
		if err != nil {
			return 0, 0, err
		}
		if !gotScore {
			return 0, 0, errors.New("xpsnr filter produced no score")
		}
		tracker.set(key, 0, true)
		return score, stat.Size(), nil
	}

	// measure averages all windows. Hardware encoders run the windows in
	// parallel: one window's encode uses the GPU while another's XPSNR uses
	// the CPU. Three sessions stay within consumer NVENC session limits.
	measure := func(trial sampleTrial, tag string, q float64, parallel bool) (float64, int64, error) {
		scores := make([]float64, len(refs))
		sizes := make([]int64, len(refs))
		if parallel {
			runCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			errs := make([]error, len(refs))
			var wg sync.WaitGroup
			for i := range refs {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					if scores[i], sizes[i], errs[i] = measureWindow(runCtx, trial, tag, i, q); errs[i] != nil {
						cancel()
					}
				}(i)
			}
			wg.Wait()
			if err := ctx.Err(); err != nil {
				return 0, 0, err
			}
			if err := errors.Join(errs...); err != nil {
				return 0, 0, err
			}
		} else {
			for i := range refs {
				score, size, err := measureWindow(ctx, trial, tag, i, q)
				if err != nil {
					return 0, 0, err
				}
				scores[i], sizes[i] = score, size
			}
		}
		sum, bytes := 0.0, int64(0)
		for i := range refs {
			sum += scores[i]
			bytes += sizes[i]
		}
		return sum / float64(len(refs)), bytes, nil
	}

	type measured struct {
		score float64
		bytes int64
	}
	memo := map[float64]measured{}
	floorScore := 0.0
	if search.floor != nil {
		// The floor (CPU) and the first trial (GPU) use different hardware,
		// so they run at the same time. The first trial does not depend on
		// the target, and searchCRF reuses its result.
		first := quantizeWithStep(min(preset.MaxCRF, max(preset.MinCRF, preset.InitialCRF)), presetStep(preset))
		var firstResult measured
		var firstErr, floorErr error
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			firstResult.score, firstResult.bytes, firstErr = measure(search.trial, "first", first, search.parallel)
		}()
		floorScore, _, floorErr = measure(search.floor.trial, "floor", search.floor.q, false)
		wg.Wait()
		if err := errors.Join(floorErr, firstErr); err != nil {
			return 0, nil, err
		}
		memo[first] = firstResult
		preset.TargetXPSNR = max(preset.TargetXPSNR, floorScore)
	}

	evalCount := 0
	outcome, err := searchCRF(preset, func(crf float64) (float64, int64, error) {
		if cached, ok := memo[crf]; ok {
			return cached.score, cached.bytes, nil
		}
		evalCount++
		return measure(search.trial, strconv.Itoa(evalCount), crf, search.parallel)
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
		Encoder:          search.encoder,
		Preset:           preset.Preset,
		CRF:              outcome.Best.CRF,
		TargetXPSNR:      math.Round(preset.TargetXPSNR*100) / 100,
		FloorXPSNR:       math.Round(floorScore*100) / 100,
		MeasuredXPSNR:    math.Round(outcome.Best.Score*100) / 100,
		EstimatedPercent: estimatedPercent,
		Iterations:       outcome.Iterations,
	}
	if options.Mode == "compress" && !options.Sample && estimatedPercent >= maxEncodedSizePercent {
		return outcome.Best.CRF, report, errors.New(locales.ErrCompressNoSavings(strconv.Itoa(estimatedPercent)))
	}
	return outcome.Best.CRF, report, nil
}
