package media

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/styles"
)

func TestAudioExtractionMapsOnlyFirstAudio(t *testing.T) {
	info := Info{Streams: []Stream{{Index: 0, CodecType: "video", ColorTransfer: "smpte2084"}, {Index: 2, CodecType: "audio", Channels: 6}, {Index: 3, CodecType: "audio"}, {Index: 4, CodecType: "subtitle"}}}
	args, err := ComposeArgs(ComposeOptions{Source: "source.mkv", Destination: "podcast.mp3", Mode: "audio"}, info, "unused.ass", "partial.mp3")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"-map 0:2", "-c:a libmp3lame", "-vn", "-sn", "-dn", "-ac 2"} {
		if !strings.Contains(joined, want) {
			t.Fatal("missing audio option", want, joined)
		}
	}
	for _, forbidden := range []string{"unused.ass", "-map 0:0", "-map 0:3", "-map 0:4"} {
		if strings.Contains(joined, forbidden) {
			t.Fatal("included unwanted content", joined)
		}
	}
	for _, invalid := range []struct {
		info        Info
		destination string
	}{{Info{Streams: info.Streams[:1]}, "out.mp3"}, {info, "out.mp4"}} {
		if _, err := ComposeArgs(ComposeOptions{Mode: "audio", Destination: invalid.destination}, invalid.info, "", "partial"); err == nil {
			t.Fatal("invalid extraction accepted", invalid)
		}
	}
}

// Real encoding and full decoding exercise the shipped encoders and containers,
// including a video longer than its first audio track and a second audio track.
func TestRealAudioExtraction(t *testing.T) {
	ffmpeg, probe := os.Getenv("ONESUB_FFMPEG"), os.Getenv("ONESUB_FFPROBE")
	if ffmpeg == "" || probe == "" {
		t.Skip("build/test.ps1 enables real media integration")
	}
	root := t.TempDir()
	source := filepath.Join(root, "source.mkv")
	args := []string{"-v", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=10:duration=4", "-f", "lavfi", "-i", "sine=frequency=440:duration=2", "-f", "lavfi", "-i", "sine=frequency=880:duration=4", "-map", "0:v", "-map", "1:a", "-map", "2:a", "-c:v", "libx264", "-c:a", "pcm_s16le", source}
	if out, err := exec.Command(ffmpeg, args...).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, out)
	}
	before, err := hashFile(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	for _, format := range []struct{ ext, codec string }{{"mp3", "mp3"}, {"m4a", "aac"}, {"aac", "aac"}, {"wav", "pcm_s16le"}, {"flac", "flac"}, {"ogg", "vorbis"}, {"opus", "opus"}} {
		t.Run(format.ext, func(t *testing.T) {
			dir := filepath.Join(root, format.ext)
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(dir, "podcast."+format.ext)
			options := ComposeOptions{Source: source, Destination: output, Mode: "audio"}
			report, err := Compose(context.Background(), ffmpeg, probe, root, options, styles.Document{}, func(ComposeProgress) {})
			if err != nil {
				t.Fatal(err)
			}
			info, err := Probe(context.Background(), probe, output)
			if err != nil || len(info.Streams) != 1 || info.Streams[0].CodecName != format.codec || report.Streams["audio"] != 1 || report.OutputSHA256 == "" || report.ASSSHA256 != "" || report.DurationMs < 1900 || report.DurationMs > 2250 {
				t.Fatal("incorrect audio export", info, report, err)
			}
			if _, err := Compose(context.Background(), ffmpeg, probe, root, options, styles.Document{}, func(ComposeProgress) {}); err == nil {
				t.Fatal("overwrote existing audio")
			}
			options.Destination = filepath.Join(dir, "cancelled."+format.ext)
			ctx, cancel := context.WithCancel(context.Background())
			_, err = Compose(ctx, ffmpeg, probe, root, options, styles.Document{}, func(p ComposeProgress) {
				if p.Stage == StageEncode {
					cancel()
				}
			})
			cancel()
			if !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation lost", err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(output) {
				t.Fatal("left partial files", entries, err)
			}
		})
	}
	if after, err := hashFile(context.Background(), source); err != nil || after != before {
		t.Fatal("source changed", err)
	}
}

func TestRealAudioExtractionRejectsSilentVideoAndChecksLongAAC(t *testing.T) {
	ffmpeg, probe := os.Getenv("ONESUB_FFMPEG"), os.Getenv("ONESUB_FFPROBE")
	if ffmpeg == "" || probe == "" {
		t.Skip("build/test.ps1 enables real media integration")
	}
	root := t.TempDir()
	for _, audio := range []bool{false, true} {
		dir := filepath.Join(root, fmt.Sprint(audio))
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		source := filepath.Join(dir, "source.mkv")
		args := []string{"-v", "error", "-f", "lavfi", "-i", "color=size=160x90:rate=1:duration=30"}
		if audio {
			args = append(args, "-f", "lavfi", "-i", "aevalsrc=if(lt(t\\,15)\\,0\\,sin(440*2*PI*t)):s=48000:d=30", "-c:a", "pcm_s16le")
		}
		args = append(args, "-c:v", "libx264", source)
		if out, err := exec.Command(ffmpeg, args...).CombinedOutput(); err != nil {
			t.Fatalf("fixture: %v %s", err, out)
		}
		options := ComposeOptions{Source: source, Destination: filepath.Join(dir, "out.aac"), Mode: "audio"}
		report, err := Compose(context.Background(), ffmpeg, probe, root, options, styles.Document{}, func(ComposeProgress) {})
		if audio {
			if err != nil || report.DurationMs < 29900 || report.DurationMs > 30250 {
				t.Fatal("valid variable-rate AAC rejected", report, err)
			}
		} else {
			if err == nil {
				t.Fatal("silent video accepted")
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 {
				t.Fatal("failed extraction left files", entries, err)
			}
		}
	}
}
