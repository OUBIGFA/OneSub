package media

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/styles"
)

func TestH264UsesOriginalFixedCRF(t *testing.T) {
	info := Info{Streams: []Stream{{Index: 0, CodecType: "video", CodecName: "h264"}, {Index: 1, CodecType: "audio", CodecName: "aac"}}}
	for _, mode := range []string{"hard", "compress"} {
		for _, quality := range []string{"crf18", "crf20", "crf23"} {
			// A resolved value from another encoder must not override this choice.
			opts := ComposeOptions{Source: "source.mp4", Destination: "out.mp4", Mode: mode, Encoder: "cpu-h264", Quality: quality, ResolvedCRF: 49}
			args, err := ComposeArgs(opts, info, "render.ass", "partial.mp4")
			if err != nil {
				t.Fatal(mode, quality, err)
			}
			joined := strings.Join(args, " ")
			for _, want := range []string{"-c:v:0 libx264", "-preset medium", "-crf " + strings.TrimPrefix(quality, "crf"), "-pix_fmt:v:0 yuv420p", "-map 0:1", "-map_chapters 0", "-fps_mode:v:0 passthrough"} {
				if !strings.Contains(joined, want) {
					t.Fatalf("missing %q: %s", want, joined)
				}
			}
			if strings.Contains(joined, "svtav1") || strings.Contains(joined, "nvenc") || strings.Contains(joined, "yuv420p10le") || strings.Contains(joined, "ass=") != (mode == "hard") {
				t.Fatal("wrong encoder or subtitle filter", joined)
			}
		}
	}
	for _, quality := range []string{"near-lossless", "high", "compact", "lossless", "crf0", "crf99"} {
		if _, err := ComposeArgs(ComposeOptions{Destination: "out.mp4", Mode: "compress", Encoder: "cpu-h264", Quality: quality}, info, "", "partial.mp4"); err == nil {
			t.Fatal("H.264 accepted another encoder's quality", quality)
		}
	}
}

// Exercise the complete export path with generated media, including burn-in,
// audio/chapter preservation and the absence of a quality-search stage.
func TestH264ExportsWithoutQualitySearch(t *testing.T) {
	ffmpeg, probe := os.Getenv("ONESUB_FFMPEG"), os.Getenv("ONESUB_FFPROBE")
	if ffmpeg == "" || probe == "" {
		t.Skip("build/test.ps1 -CoreOnly supplies the media runtime")
	}
	root := t.TempDir()
	source := chapterMediaFixture(t, ffmpeg, root)
	document := styles.Document{ASS: "[Script Info]\nScriptType: v4.00+\nPlayResX: 320\nPlayResY: 180\n[V4+ Styles]\nFormat: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\nStyle: Default,Arial,28,&H00FFFFFF,&H000000FF,&H00000000,&H00000000,0,0,0,0,100,100,0,0,1,2,0,2,10,10,10,1\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:00.00,0:00:06.00,Default,,0,0,0,,H264 TEST\n"}
	var plain, burned []byte
	for _, tc := range []struct {
		mode, ext string
		sample    bool
	}{{"compress", ".mp4", false}, {"hard", ".mp4", false}, {"compress", ".mkv", true}, {"hard", ".mkv", true}} {
		output := filepath.Join(root, tc.mode+tc.ext)
		opts := ComposeOptions{Source: source, Destination: output, Mode: tc.mode, Encoder: "cpu-h264", Quality: "crf20", Sample: tc.sample}
		report, err := Compose(context.Background(), ffmpeg, probe, root, opts, document, func(p ComposeProgress) {
			if p.Stage == StageAnalyze {
				t.Error("fixed CRF export ran quality search")
			}
		})
		if err != nil {
			t.Fatal(tc, err)
		}
		if report.Compression != nil || report.Streams["audio"] != 2 || report.OutputSHA256 == "" || (report.ASSSHA256 != "") != (tc.mode == "hard") {
			t.Fatal("incomplete or misleading export report", report)
		}
		info, err := Probe(context.Background(), probe, output)
		if err != nil {
			t.Fatal(err)
		}
		if info.Streams[0].CodecName != "h264" || info.Streams[0].PixFmt != "yuv420p" {
			t.Fatal("wrong output format", info.Streams[0])
		}
		assertChapterOutput(t, probe, output, tc.sample)
		if tc.ext == ".mp4" {
			frame, err := exec.Command(ffmpeg, "-v", "error", "-ss", "0.5", "-i", output, "-frames:v", "1", "-f", "rawvideo", "-pix_fmt", "rgb24", "-").Output()
			if err != nil || len(frame) != 320*180*3 {
				t.Fatal("cannot decode the subtitle frame", err, len(frame))
			}
			if tc.mode == "hard" {
				burned = frame
			} else {
				plain = frame
			}
		}
		if _, err := Compose(context.Background(), ffmpeg, probe, root, opts, document, func(ComposeProgress) {}); err == nil {
			t.Fatal("overwrote an existing output")
		}
	}
	if bytes.Equal(plain, burned) {
		t.Fatal("burned subtitles did not change the video frame")
	}
}
