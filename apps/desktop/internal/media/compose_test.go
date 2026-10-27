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
	opts.Quality = "high"
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
	document, err := client.Build(context.Background(), []subtitle.Cue{{ID: "c1", Text: "测试字幕", StartMs: 100, EndMs: 1500}}, catalog.DefaultStyle)
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Fonts) == 0 || !strings.Contains(document.ASS, "测试字幕") {
		t.Fatal("style worker did not produce fonts/ASS")
	}
	for _, mode := range []string{"hard", "soft"} {
		for _, extension := range []string{".mp4", ".mkv"} {
			output := filepath.Join(root, mode+extension)
			options := ComposeOptions{Source: source, Destination: output, Mode: mode, Quality: "standard", Language: "zh-CN", Revision: 1}
			report, err := Compose(context.Background(), ffmpeg, probe, root, options, document, func(p ComposeProgress) {
				if p.Stage == "" || p.Fraction < -1 || p.Fraction >= 1 {
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
}
