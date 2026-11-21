package media

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/styles"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
)

func TestComposeMappingAndGuards(t *testing.T) {
	var info Info
	info.Format.StartTime = "0"
	info.Streams = []Stream{{Index: 0, CodecType: "video", CodecName: "h264"}, {Index: 1, CodecType: "audio", CodecName: "aac"}, {Index: 2, CodecType: "audio", CodecName: "aac"}}
	opts := ComposeOptions{Source: "source.mp4", Destination: "out.mkv", Mode: "soft", Language: "zh-CN"}
	args, err := ComposeArgs(opts, info, "render.ass", "unique.partial.mkv")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, part := range []string{"-map 0:1", "-map 0:2", "-map 1:s:0", "-c copy", "-c:s:0 ass", "language=zho"} {
		if !strings.Contains(joined, part) {
			t.Fatal(joined)
		}
	}
	opts.Mode = "hard"
	opts.Quality = CompressQualityNearLossless
	info.Streams[0].ColorTransfer = "smpte2084"
	if _, err = ComposeArgs(opts, info, "render.ass", "partial.mkv"); err == nil {
		t.Fatal("HDR burn accepted")
	}
	info.Streams[0].ColorTransfer = ""
	info.Streams = append(info.Streams, Stream{Index: 3, CodecType: "attachment"})
	opts.Destination = "out.mp4"
	if _, err = ComposeArgs(opts, info, "render.ass", "partial.mp4"); err == nil {
		t.Fatal("MP4 silently lost attachment")
	}
}

func TestRealSmartSubComposition(t *testing.T) {
	worker := os.Getenv("ONESUB_STYLE_WORKER")
	if worker == "" {
		t.Skip("build/test.ps1 enables real SmartSub/FFmpeg integration")
	}
	ffmpeg, probe := os.Getenv("ONESUB_FFMPEG"), os.Getenv("ONESUB_FFPROBE")
	if ffmpeg == "" || probe == "" {
		t.Fatal("missing media tools")
	}
	root := t.TempDir()
	source := filepath.Join(root, "source.mp4")
	args := []string{"-v", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=10:duration=2", "-f", "lavfi", "-i", "sine=frequency=440:duration=2", "-map", "0:v", "-map", "1:a", "-map", "1:a", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", source}
	if out, err := exec.Command(ffmpeg, args...).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, out)
	}
	client := styles.Client{Executable: worker, Scratch: root}
	catalog, err := client.Catalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	document, err := client.Build(context.Background(), []subtitle.Cue{{ID: "c1", Text: "测试字幕", StartMs: 100, EndMs: 1500}}, catalog.DefaultStyle, 16.0/9)
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Fonts) == 0 || !strings.Contains(document.ASS, "测试字幕") {
		t.Fatal("style worker did not produce fonts/ASS")
	}
	// Container handling is covered by the chapter and timing matrices; here
	// each mode runs once with SmartSub fonts and a CRF level.
	for _, c := range [][2]string{{"hard", ".mp4"}, {"soft", ".mkv"}} {
		mode, extension := c[0], c[1]
		output := filepath.Join(root, mode+extension)
		options := ComposeOptions{Source: source, Destination: output, Mode: mode, Quality: CompressQualityHigh, Language: "zh-CN", Revision: 1}
		report, err := Compose(context.Background(), ffmpeg, probe, root, options, document, func(p ComposeProgress) {
			if p.Stage == "" || p.Fraction < -1 || p.Fraction > 1 {
				t.Errorf("invalid progress: %+v", p)
			}
		})
		if err != nil {
			t.Fatal(mode, extension, err)
		}
		if report.Streams["audio"] != 2 || report.OutputSHA256 == "" {
			t.Fatal(report)
		}
		if mode == "soft" && report.Streams["subtitle"] != 1 {
			t.Fatal("missing soft subtitle")
		}
		if _, err = Compose(context.Background(), ffmpeg, probe, root, options, document, func(ComposeProgress) {}); err == nil {
			t.Fatal("overwrote output")
		}
	}
}

// Hard burn-in uses the shared 10-bit SVT-AV1 perceptual presets and rejects
// retired H.264/lossless choices.
func TestComposeQualityLevels(t *testing.T) {
	var info Info
	info.Format.StartTime = "0"
	info.Streams = []Stream{{Index: 0, CodecType: "video", CodecName: "h264", PixFmt: "yuv444p10le"}}
	for quality, wantPreset := range map[string]string{
		CompressQualityNearLossless: "8",
		CompressQualityHigh:         "8",
		CompressQualityCompact:      "8",
	} {
		opts := ComposeOptions{Source: "source.mkv", Destination: "out.mkv", Mode: "hard", Quality: quality, ResolvedCRF: 26.25}
		args, err := ComposeArgs(opts, info, "render.ass", "partial.mkv")
		if err != nil {
			t.Fatal(quality, err)
		}
		joined := strings.Join(args, " ")
		for _, required := range []string{
			"ass=render.ass:fontsdir=fonts,crop=trunc(iw/2)*2:trunc(ih/2)*2:0:0",
			"-c:v:0 libsvtav1",
			"-preset " + wantPreset,
			"-pix_fmt:v:0 yuv420p10le",
			"-svtav1-params crf=26.25:tune=0:enable-variance-boost=1:enable-overlays=1",
		} {
			if !strings.Contains(joined, required) {
				t.Fatalf("%s missing %q: %s", quality, required, joined)
			}
		}
	}
	for _, retired := range []string{"lossless", "crf18", "crf20", "crf23", "original"} {
		opts := ComposeOptions{Source: "source.mkv", Destination: "out.mkv", Mode: "hard", Quality: retired}
		if _, err := ComposeArgs(opts, info, "render.ass", "partial.mkv"); err == nil {
			t.Fatalf("retired or unknown quality %q accepted", retired)
		}
	}
}
