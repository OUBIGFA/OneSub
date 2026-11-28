package media

import (
	"context"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/styles"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/videoencoding"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

func TestPickHardwareEncoderPrefersAV1ThenFallsBack(t *testing.T) {
	rejecting := func(names ...string) func(context.Context, hardwareEncoder) error {
		return func(_ context.Context, e hardwareEncoder) error {
			for _, name := range names {
				if e.Name == name {
					return errors.New("unsupported")
				}
			}
			return nil
		}
	}
	cases := []struct {
		reject []string
		want   string
	}{
		{nil, "av1_nvenc"},
		{[]string{"av1_nvenc"}, "av1_qsv"},
		// An Ampere NVIDIA GPU with no Intel/AMD encoder: HEVC on NVENC.
		{[]string{"av1_nvenc", "av1_qsv", "av1_amf"}, "hevc_nvenc"},
		{[]string{"av1_nvenc", "av1_qsv", "av1_amf", "hevc_nvenc", "hevc_qsv"}, "hevc_amf"},
	}
	for _, tc := range cases {
		got, err := pickHardwareEncoder(context.Background(), rejecting(tc.reject...))
		if err != nil || got.Name != tc.want {
			t.Fatalf("reject %v: got %q, %v; want %q", tc.reject, got.Name, err, tc.want)
		}
	}
	all := []string{}
	for _, e := range hardwareEncoders {
		all = append(all, e.Name)
	}
	if _, err := pickHardwareEncoder(context.Background(), rejecting(all...)); err == nil || err.Error() != locales.ErrGpuEncoderUnavailable() {
		t.Fatalf("no encoder must fail explicitly, got %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := pickHardwareEncoder(ctx, rejecting()); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled detection must return the context error, got %v", err)
	}
}

func TestHardwareVideoArgs(t *testing.T) {
	for _, e := range hardwareEncoders {
		joined := strings.Join(e.videoArgs(29.6, 1920*1080), " ")
		if !strings.HasPrefix(joined, "-c:v:0 "+e.Name+" ") || !strings.HasSuffix(joined, "-pix_fmt:v:0 p010le") {
			t.Fatalf("%s: encoder or 10-bit format missing: %s", e.Name, joined)
		}
		switch e.Vendor {
		case "nvenc":
			// An explicit peak avoids NVENC's implicit level-derived cap.
			for _, want := range []string{"-preset p5", "-tune hq", "-rc vbr", "-cq 30", "-b:v:0 0", "-maxrate:v:0 124416000", "-rc-lookahead 32"} {
				if !strings.Contains(joined, want) {
					t.Fatalf("%s missing %q: %s", e.Name, want, joined)
				}
			}
			if strings.Contains(joined, "uhq") || strings.Contains(joined, "multipass") {
				t.Fatalf("%s must not use the slow UHQ/2-pass setup: %s", e.Name, joined)
			}
		case "qsv":
			if !strings.Contains(joined, "-global_quality 30") || strings.Contains(joined, "maxrate") {
				t.Fatalf("%s must use ICQ without a peak limit: %s", e.Name, joined)
			}
		case "amf":
			if !strings.Contains(joined, "-rc qvbr -qvbr_quality_level 30") || !strings.Contains(joined, "-maxrate:v:0 124416000") {
				t.Fatalf("%s must use QVBR with an explicit peak: %s", e.Name, joined)
			}
		}
	}
}

func TestGPUPresetKeepsCPUTarget(t *testing.T) {
	for _, quality := range []string{videoencoding.QualityNearLossless, videoencoding.QualityHigh, videoencoding.QualityCompact} {
		cpu, _ := CompressPresetFor(quality)
		for _, e := range hardwareEncoders {
			preset, ok := gpuPresetFor(e, quality)
			if !ok || preset.TargetXPSNR != cpu.TargetXPSNR || preset.MaxCRF != e.MaxQ || preset.MinCRF < 1 || preset.InitialCRF <= preset.MinCRF || preset.InitialCRF >= preset.MaxCRF {
				t.Fatalf("%s %s: unexpected preset %+v", e.Name, quality, preset)
			}
		}
	}
	if _, ok := gpuPresetFor(hardwareEncoders[0], "crf23"); ok {
		t.Fatal("unknown quality must be rejected")
	}
}

func TestComposeArgsUseHardwareEncoderInTheSharedPipeline(t *testing.T) {
	info := Info{Streams: []Stream{{Index: 0, CodecType: "video", CodecName: "h264", Width: 1921, Height: 1080}, {Index: 1, CodecType: "audio", CodecName: "aac"}}}
	for _, tc := range []struct {
		encoder hardwareEncoder
		ext     string
		hvc1    bool
	}{
		{hardwareEncoders[3], ".mp4", true}, // hevc_nvenc
		{hardwareEncoders[3], ".mkv", false},
		{hardwareEncoders[0], ".mp4", false}, // av1_nvenc
	} {
		encoder := tc.encoder
		args, err := ComposeArgs(ComposeOptions{Source: "source.mp4", Destination: "out" + tc.ext, Mode: "hard", Quality: videoencoding.QualityHigh, Encoder: videoencoding.EncoderGPU, ResolvedCRF: 33, hardware: &encoder}, info, "render.ass", "partial"+tc.ext)
		if err != nil {
			t.Fatal(err)
		}
		joined := strings.Join(args, " ")
		for _, want := range []string{"-filter:v:0 ass=render.ass:fontsdir=fonts,crop=trunc(iw/2)*2:trunc(ih/2)*2:0:0", "-c:v:0 " + encoder.Name, "-cq 33", "-maxrate:v:0 124416000", "-map 0:1", "-fps_mode:v:0 passthrough"} {
			if !strings.Contains(joined, want) {
				t.Fatalf("%s %s missing %q: %s", encoder.Name, tc.ext, want, joined)
			}
		}
		if strings.Contains(joined, "libsvtav1") || strings.Contains(joined, "-tag:v:0 hvc1") != tc.hvc1 {
			t.Fatalf("%s %s: wrong encoder or MP4 tag: %s", encoder.Name, tc.ext, joined)
		}
	}
}

func TestRealGPUCompressionAndHardComposition(t *testing.T) {
	ffmpeg, probe := os.Getenv("ONESUB_FFMPEG"), os.Getenv("ONESUB_FFPROBE")
	if ffmpeg == "" || probe == "" {
		t.Skip("build/test.ps1 enables real GPU encoder integration")
	}
	encoder, err := DetectGPUEncoder(context.Background(), ffmpeg)
	if err != nil {
		t.Skip("no supported GPU hardware on this host:", err)
	}
	t.Logf("Detected GPU encoder: %+v", encoder)

	root := t.TempDir()
	// Create a 3s 640x360 source with nonzero start time (0.5s), chapter metadata, and dual audio.
	metadata := ";FFMETADATA1\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=0\nEND=1500\ntitle=Part1\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=1500\nEND=3000\ntitle=Part2\n"
	metaPath := filepath.Join(root, "chapters.ffmetadata")
	if err := os.WriteFile(metaPath, []byte(metadata), 0600); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source.mp4")
	srcArgs := []string{
		"-v", "error",
		"-f", "lavfi", "-i", "testsrc2=size=640x360:rate=30:duration=3,setparams=color_primaries=bt709:color_trc=bt709:colorspace=bt709:range=tv",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=3",
		"-itsoffset", "0.4", "-f", "lavfi", "-i", "sine=frequency=880:duration=3",
		"-f", "ffmetadata", "-i", metaPath,
		"-map", "0:v", "-map", "1:a", "-map", "2:a", "-map_chapters", "3",
		"-c:v", "libx264", "-crf", "0", "-pix_fmt", "yuv420p", "-c:a", "aac",
		"-output_ts_offset", "0.5", "-avoid_negative_ts", "disabled", source,
	}
	if out, err := exec.Command(ffmpeg, srcArgs...).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, out)
	}
	beforeHash, err := hashFile(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	inputInfo, err := Probe(context.Background(), probe, source)
	if err != nil {
		t.Fatal(err)
	}
	origin, _ := strconv.ParseFloat(inputInfo.Format.StartTime, 64)

	var compressedPath string
	for _, ext := range []string{"mp4", "mkv"} {
		outDir := filepath.Join(root, "compress-"+ext)
		scratch := filepath.Join(root, "scratch-"+ext)
		if err := os.MkdirAll(outDir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(scratch, 0700); err != nil {
			t.Fatal(err)
		}
		output := filepath.Join(outDir, "gpu."+ext)
		opts := ComposeOptions{
			Source:      source,
			Destination: output,
			Mode:        "compress",
			Quality:     videoencoding.QualityHigh,
			Encoder:     videoencoding.EncoderGPU,
		}
		start := time.Now()
		report, err := Compose(context.Background(), ffmpeg, probe, scratch, opts, styles.Document{}, func(ComposeProgress) {})
		if err != nil {
			t.Fatalf("GPU compress %s: %v", ext, err)
		}
		t.Logf("GPU compress %s took %s: %+v", ext, time.Since(start), report.Compression)
		if ext == "mkv" {
			compressedPath = output
		}
		// The GPU target is the preset target or the CPU floor, whichever is
		// higher, and the measured result must reach it.
		if c := report.Compression; c == nil || c.Encoder != encoder.Name || c.FloorXPSNR <= 0 || c.TargetXPSNR < max(39.0, c.FloorXPSNR) || c.MeasuredXPSNR < c.TargetXPSNR {
			t.Fatalf("unexpected GPU compression report: %+v", report.Compression)
		}
		if len(report.Checks) == 0 || report.Checks[0] != "xpsnr gpu quality search" {
			t.Fatalf("missing GPU check label: %v", report.Checks)
		}
		if report.Streams["audio"] != 2 || report.Streams["video"] != 1 || report.Chapters != 2 || report.OutputSHA256 == "" {
			t.Fatalf("unexpected GPU output report: %+v", report)
		}

		// Independent reference-first XPSNR check.
		metricArgs := []string{"-hide_banner", "-nostdin", "-v", "info", "-i", source, "-i", output,
			"-filter_complex", "[0:v]setpts=PTS-STARTPTS,format=yuv420p[r];[1:v]setpts=PTS-STARTPTS,format=yuv420p[d];[r][d]xpsnr=ts_sync_mode=nearest",
			"-an", "-f", "null", "-"}
		metric, err := exec.Command(ffmpeg, metricArgs...).CombinedOutput()
		if err != nil {
			t.Fatalf("independent XPSNR: %v %s", err, metric)
		}
		score, ok := parseXPSNR(string(metric))
		if !ok || math.Abs(score-report.Compression.MeasuredXPSNR) > 0.05 {
			t.Fatalf("independent XPSNR %v differs from reported %v", score, report.Compression.MeasuredXPSNR)
		}

		// Verify A/V start_time alignment and BT.709 color tags preserved.
		outInfo, err := Probe(context.Background(), probe, output)
		if err != nil {
			t.Fatal(err)
		}
		if outInfo.Streams[0].ColorSpace != "bt709" || outInfo.Streams[0].ColorTransfer != "bt709" {
			t.Fatalf("lost BT.709 color metadata in %s: %+v", ext, outInfo.Streams[0])
		}
		if ext == "mp4" && encoder.Codec == "hevc" && outInfo.Streams[0].CodecTag != "hvc1" {
			t.Fatalf("expected hvc1 tag in MP4 HEVC output, got %q", outInfo.Streams[0].CodecTag)
		}
		for idx := 0; idx < 3; idx++ {
			b, _ := strconv.ParseFloat(inputInfo.Streams[idx].StartTime, 64)
			a, _ := strconv.ParseFloat(outInfo.Streams[idx].StartTime, 64)
			if math.Abs(a-(b-origin)) > 0.035 {
				t.Fatalf("stream %d timing shifted: before=%g origin=%g after=%g", idx, b, origin, a)
			}
		}

		// No sidecars in output or scratch.
		entries, err := os.ReadDir(outDir)
		if err != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(output) {
			t.Fatalf("output sidecars left: %v %v", entries, err)
		}
		if rem, err := os.ReadDir(scratch); err != nil || len(rem) != 0 {
			t.Fatalf("scratch sidecars left: %v %v", rem, err)
		}
	}

	// Hard subtitle burn-in with real SmartSub fonts on nonzero start source.
	if worker := os.Getenv("ONESUB_STYLE_WORKER"); worker != "" {
		client := styles.Client{Executable: worker, Scratch: root}
		catalog, err := client.Catalog(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		doc, err := client.Build(context.Background(), []subtitle.Cue{{ID: "c1", Text: "GPU 硬字幕测试", StartMs: 200, EndMs: 1800}}, catalog.DefaultStyle, 16.0/9)
		if err != nil {
			t.Fatal(err)
		}
		hardOut := filepath.Join(root, "hard-gpu.mp4")
		hardReport, err := Compose(context.Background(), ffmpeg, probe, root, ComposeOptions{
			Source:      source,
			Destination: hardOut,
			Mode:        "hard",
			Quality:     videoencoding.QualityNearLossless,
			Encoder:     videoencoding.EncoderGPU,
			Language:    "zh-CN",
			Revision:    1,
		}, doc, func(ComposeProgress) {})
		if err != nil {
			t.Fatalf("GPU hard compose: %v", err)
		}
		if hardReport.ASSSHA256 == "" || hardReport.Streams["audio"] != 2 || hardReport.Chapters != 2 {
			t.Fatalf("unexpected hard report: %+v", hardReport)
		}
		if hardReport.Compression == nil || hardReport.Compression.MeasuredXPSNR < 42.0 {
			t.Fatalf("unexpected near-lossless hard GPU report: %+v", hardReport.Compression)
		}
		// Prove subtitles actually burned into the video at 0.5..1.5s (vs subtitle-free GPU compress).
		diffArgs := []string{"-hide_banner", "-nostdin", "-v", "info", "-ss", "0.5", "-t", "1.0", "-i", compressedPath, "-ss", "0.5", "-t", "1.0", "-i", hardOut,
			"-filter_complex", "[0:v]format=yuv420p[a];[1:v]format=yuv420p[b];[a][b]xpsnr", "-an", "-f", "null", "-"}
		diffOut, err := exec.Command(ffmpeg, diffArgs...).CombinedOutput()
		if err != nil {
			t.Fatalf("hard sub diff: %v %s", err, diffOut)
		}
		diffScore, ok := parseXPSNR(string(diffOut))
		if !ok || diffScore >= 45.0 {
			t.Fatalf("subtitles did not visibly burn onto GPU hard output (XPSNR=%v): %s", diffScore, diffOut)
		}
	}

	if afterHash, err := hashFile(context.Background(), source); err != nil || afterHash != beforeHash {
		t.Fatal("source video was modified", err)
	}
}
