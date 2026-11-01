package media

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/styles"
)

func TestCompressionHasNoSubtitleInputOrBurnFilter(t *testing.T) {
	info := Info{Streams: []Stream{{Index: 0, CodecType: "video", CodecName: "h264"}, {Index: 1, CodecType: "audio", CodecName: "aac"}}}
	args, err := ComposeArgs(ComposeOptions{Source: "source.mp4", Destination: "compressed.mp4", Mode: "compress", Quality: "crf23"}, info, "must-not-use.ass", "output.partial.mp4")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, forbidden := range []string{"must-not-use", "ass=", "fontsdir", "1:s:0"} {
		if strings.Contains(joined, forbidden) {
			t.Fatal("compression used a subtitle pipeline", joined)
		}
	}
	if !strings.Contains(joined, "-c:v:0 libx264") || !strings.Contains(joined, "-crf 23") || !strings.Contains(joined, "-map 0:1") {
		t.Fatal("compression bypassed the shared encoder or audio mapping", joined)
	}
}

// Real FFmpeg proves that a subtitle-free source needs no style worker and
// produces only one validated video, with audio and existing sources intact.
func TestRealCompressionWithoutSubtitles(t *testing.T) {
	ffmpeg, probe := os.Getenv("ONESUB_FFMPEG"), os.Getenv("ONESUB_FFPROBE")
	if ffmpeg == "" || probe == "" {
		t.Skip("build/test.ps1 enables real media integration")
	}
	root := t.TempDir()
	source := filepath.Join(root, "source.mp4")
	args := []string{"-v", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=30:duration=2", "-f", "lavfi", "-i", "sine=frequency=440:duration=2", "-c:v", "libx264", "-crf", "0", "-pix_fmt", "yuv420p", "-c:a", "aac", source}
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
		options := ComposeOptions{Source: source, Destination: output, Mode: "compress", Quality: "crf23"}
		report, err := Compose(context.Background(), ffmpeg, probe, root, options, styles.Document{}, func(ComposeProgress) {})
		if err != nil {
			t.Fatal(extension, err)
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
			if p.Stage == StageEncode {
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
