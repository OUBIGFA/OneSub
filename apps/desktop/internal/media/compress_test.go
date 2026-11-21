package media

import (
	"context"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/styles"
)

func TestCompressionHasNoSubtitleInputOrBurnFilter(t *testing.T) {
	info := Info{Streams: []Stream{{Index: 0, CodecType: "video", CodecName: "h264"}, {Index: 1, CodecType: "audio", CodecName: "aac"}}}
	args, err := ComposeArgs(ComposeOptions{Source: "source.mp4", Destination: "compressed.mp4", Mode: "compress", Quality: CompressQualityNearLossless, ResolvedCRF: 27.5}, info, "must-not-use.ass", "output.partial.mp4")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, forbidden := range []string{"must-not-use", "ass=", "fontsdir", "1:s:0", "libx264"} {
		if strings.Contains(joined, forbidden) {
			t.Fatal("compression used a forbidden pipeline", forbidden, joined)
		}
	}
	for _, required := range []string{
		"-c:v:0 libsvtav1",
		"-preset 8",
		"-pix_fmt:v:0 yuv420p10le",
		"-svtav1-params crf=27.5:tune=0:enable-variance-boost=1:enable-overlays=1",
		"-map 0:1",
	} {
		if !strings.Contains(joined, required) {
			t.Fatalf("compression args missing %q: %s", required, joined)
		}
	}
	if _, err := ComposeArgs(ComposeOptions{Source: "source.mp4", Destination: "compressed.mp4", Mode: "compress", Quality: "crf23"}, info, "", "output.partial.mp4"); err == nil {
		t.Fatal("retired H.264 CRF quality must be rejected for AV1 compression")
	}
}

func TestSampleWindows(t *testing.T) {
	sample := sampleWindows(120_000, true)
	if len(sample) != 1 || sample[0].StartSec != 0 || sample[0].DurationSec != 5 {
		t.Fatal("unexpected 5s sample window", sample)
	}
	short := sampleWindows(8_000, false)
	if len(short) != 1 || short[0].StartSec != 0 || short[0].DurationSec != 8 {
		t.Fatal("unexpected short window", short)
	}
	medium := sampleWindows(24_000, false)
	if len(medium) != 1 || math.Abs(medium[0].StartSec-7) > 0.01 || medium[0].DurationSec != 10 {
		t.Fatal("unexpected medium window", medium)
	}
	long := sampleWindows(600_000, false)
	if len(long) != 3 {
		t.Fatal("expected 3 long windows", long)
	}
	for i := 1; i < len(long); i++ {
		if long[i].StartSec <= long[i-1].StartSec+long[i-1].DurationSec {
			t.Fatal("long windows overlapped", long)
		}
	}
}

func TestParseXPSNR(t *testing.T) {
	score, ok := parseXPSNR("[Parsed_xpsnr_0 @ 0000013c68735b80] XPSNR  y: 44.0442  u: 39.5583  v: 40.0255  (minimum: 39.5583)")
	want := (4*44.0442 + 39.5583 + 40.0255) / 6.0
	if !ok || math.Abs(score-want) > 0.0001 {
		t.Fatalf("got %v (%v), want %v", score, ok, want)
	}
	infScore, ok := parseXPSNR("[Parsed_xpsnr_0 @ 0x1] XPSNR  y: inf  u: inf  v: inf  (minimum: inf)")
	if !ok || infScore != xpsnrInfScore {
		t.Fatalf("got %v (%v), want %v", infScore, ok, xpsnrInfScore)
	}
	if _, ok := parseXPSNR("frame=   60 fps=0.0"); ok {
		t.Fatal("non-xpsnr line should not parse")
	}
}

func TestSearchCRFConverges(t *testing.T) {
	preset, ok := CompressPresetFor(CompressQualityNearLossless)
	if !ok {
		t.Fatal("missing near-lossless preset")
	}
	// Monotonic synthetic curve: score(crf) = 56 - 0.5*crf => target 42.0 at crf = 28.0.
	outcome, err := searchCRF(preset, func(crf float64) (float64, int64, error) {
		return 56.0 - 0.5*crf, int64(1000000 / crf), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Best.Score < preset.TargetXPSNR || math.Abs(outcome.Best.CRF-28.0) > 0.5 {
		t.Fatalf("search did not converge near 28.0: %+v", outcome)
	}
	if outcome.Iterations > crfSearchMaxIterations {
		t.Fatalf("exceeded iteration budget: %d", outcome.Iterations)
	}
	if math.Mod(outcome.Best.CRF*4, 1) != 0 {
		t.Fatalf("crf %v is not quantized to 0.25 steps", outcome.Best.CRF)
	}
}

// A smooth curve should not spend the six-attempt budget stepping toward the
// target after two measurements already establish its slope.
func TestSearchCRFUsesMeasuredSlopeBeforeBracketing(t *testing.T) {
	preset, _ := CompressPresetFor(CompressQualityNearLossless)
	for _, targetCRF := range []float64{18, 46} {
		outcome, err := searchCRF(preset, func(crf float64) (float64, int64, error) {
			return 42 + (targetCRF-crf)*0.4, 1000, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if outcome.Best.Score < 42 || math.Abs(outcome.Best.CRF-targetCRF) > 0.5 {
			t.Fatalf("target CRF %v: missed quality target: %+v", targetCRF, outcome)
		}
		if outcome.Iterations > 3 {
			t.Fatalf("target CRF %v: repeated avoidable trial encodes: %+v", targetCRF, outcome)
		}
	}
}

func TestSearchCRFRejectsUnmetQualityTarget(t *testing.T) {
	preset, _ := CompressPresetFor(CompressQualityHigh)
	_, err := searchCRF(preset, func(crf float64) (float64, int64, error) {
		return 30 - crf*0.1, 1000, nil
	})
	if err == nil {
		t.Fatal("search accepted an encoding below the requested quality target")
	}
}

func TestEstimateOutputPercent(t *testing.T) {
	if got := estimateOutputPercent(1000, 100, 100, 20); got != 50 {
		t.Fatalf("got %d, want 50", got)
	}
	if got := estimateOutputPercent(0, 100, 100, 20); got != 0 {
		t.Fatalf("got %d, want 0", got)
	}
}

// Opt in with a local fixture to measure the actual search, encode and full
// validation stages together. Normal unit checks do not run this benchmark.
func TestCompressionPerformance(t *testing.T) {
	source := os.Getenv("ONESUB_COMPRESSION_BENCHMARK")
	if source == "" {
		t.Skip("set ONESUB_COMPRESSION_BENCHMARK to a local video")
	}
	ffmpeg, probe := os.Getenv("ONESUB_FFMPEG"), os.Getenv("ONESUB_FFPROBE")
	if ffmpeg == "" || probe == "" {
		t.Fatal("build/test.ps1 -CoreOnly supplies the media runtime")
	}
	root := t.TempDir()
	stage := "prepare"
	started := time.Now()
	totalStart := started
	timings := map[string]time.Duration{}
	report, err := Compose(context.Background(), ffmpeg, probe, root,
		ComposeOptions{Source: source, Destination: filepath.Join(root, "compressed.mkv"), Mode: "compress", Quality: CompressQualityNearLossless},
		styles.Document{}, func(p ComposeProgress) {
			if p.Stage != stage {
				timings[stage] += time.Since(started)
				stage, started = p.Stage, time.Now()
			}
		})
	if err != nil {
		t.Fatal(err)
	}
	timings[stage] += time.Since(started)
	stat, err := os.Stat(filepath.Join(root, "compressed.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Compression benchmark: total=%s stages=%v bytes=%d report=%+v", time.Since(totalStart), timings, stat.Size(), report.Compression)
}

// Real FFmpeg proves that a subtitle-free source needs no style worker,
// runs XPSNR-guided SVT-AV1 compression, and produces one validated video
// with audio and existing sources intact.
func TestRealCompressionWithoutSubtitles(t *testing.T) {
	ffmpeg, probe := os.Getenv("ONESUB_FFMPEG"), os.Getenv("ONESUB_FFPROBE")
	if ffmpeg == "" || probe == "" {
		t.Skip("build/test.ps1 enables real media integration")
	}
	root := t.TempDir()
	source := filepath.Join(root, "source.mp4")
	// A width not divisible by the decoder's row alignment exposes XPSNR's
	// high-bit-depth stride bug; 320-wide fixtures alone can miss it.
	args := []string{"-v", "error", "-f", "lavfi", "-i", "testsrc2=size=318x178:rate=30:duration=2", "-f", "lavfi", "-i", "sine=frequency=440:duration=2", "-c:v", "libx264", "-crf", "0", "-pix_fmt", "yuv420p", "-c:a", "aac", source}
	if out, err := exec.Command(ffmpeg, args...).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, out)
	}
	before, err := hashFile(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	for _, extension := range []string{"mp4", "mkv"} {
		dir := filepath.Join(root, extension)
		if err = os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		output := filepath.Join(dir, "compressed."+extension)
		options := ComposeOptions{Source: source, Destination: output, Mode: "compress", Quality: CompressQualityHigh}
		sawAnalyze := false
		analyzeFraction := 0.0
		report, err := Compose(context.Background(), ffmpeg, probe, root, options, styles.Document{}, func(p ComposeProgress) {
			if p.Stage == StageAnalyze {
				sawAnalyze = true
				if p.Fraction < analyzeFraction {
					t.Errorf("analysis progress went backward: %v -> %v", analyzeFraction, p.Fraction)
				}
				analyzeFraction = p.Fraction
			}
		})
		if err != nil {
			t.Fatal(extension, err)
		}
		if !sawAnalyze {
			t.Fatal("compression skipped XPSNR analysis stage")
		}
		if report.Compression == nil || report.Compression.Encoder != "libsvtav1" || report.Compression.CRF <= 0 || report.Compression.MeasuredXPSNR <= 0 || report.Compression.Iterations <= 0 {
			t.Fatal("missing compression report details", report.Compression)
		}
		// This fixture is shorter than one sample, so an independent full-file
		// measurement must agree with the report. Reference first is required
		// by FFmpeg; reversing it changes XPSNR's perceptual weights.
		metricArgs := []string{"-hide_banner", "-nostdin", "-v", "info", "-i", source, "-i", output,
			"-filter_complex", "[0:v]setpts=PTS-STARTPTS,format=yuv420p[r];[1:v]setpts=PTS-STARTPTS,format=yuv420p[d];[r][d]xpsnr=ts_sync_mode=nearest",
			"-an", "-f", "null", "-"}
		metric, err := exec.Command(ffmpeg, metricArgs...).CombinedOutput()
		if err != nil {
			t.Fatalf("independent XPSNR: %v %s", err, metric)
		}
		score, ok := parseXPSNR(string(metric))
		if !ok || math.Abs(score-report.Compression.MeasuredXPSNR) > 0.02 {
			t.Fatalf("reference-first XPSNR %v differs from reported %v", score, report.Compression.MeasuredXPSNR)
		}
		if analyzeFraction != 1 {
			t.Fatalf("analysis never completed its progress: %v", analyzeFraction)
		}
		if report.ASSSHA256 != "" || report.Streams["subtitle"] != 0 || report.Streams["audio"] != 1 || report.Streams["video"] != 1 || report.OutputSHA256 == "" {
			t.Fatal("unexpected compressed streams or missing validation", report)
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(output) {
			t.Fatal("output folder contains intermediates", entries, err)
		}
		cancelled := options
		cancelled.Destination = filepath.Join(dir, "cancelled."+extension)
		ctx, cancel := context.WithCancel(context.Background())
		_, err = Compose(ctx, ffmpeg, probe, root, cancelled, styles.Document{}, func(p ComposeProgress) {
			if p.Stage == StageAnalyze || p.Stage == StageEncode {
				cancel()
			}
		})
		cancel()
		if !errors.Is(err, context.Canceled) {
			t.Fatal("compression cancellation was not reported", err)
		}
		if remaining, err := os.ReadDir(dir); err != nil || len(remaining) != 1 {
			t.Fatal("cancelled compression left partial output", remaining, err)
		}
		inputStat, _ := os.Stat(source)
		outputStat, _ := os.Stat(output)
		if outputStat.Size() >= inputStat.Size() {
			t.Fatal("high-bitrate fixture did not compress", inputStat.Size(), outputStat.Size())
		}
		if _, err = Compose(context.Background(), ffmpeg, probe, root, options, styles.Document{}, func(ComposeProgress) {}); err == nil {
			t.Fatal("compression overwrote an existing file")
		}
	}
	if after, err := hashFile(context.Background(), source); err != nil || after != before {
		t.Fatal("source video was changed", err)
	}
}
