package importing

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
)

func TestRealImportPairAndBrokenCompanions(t *testing.T) {
	workerPath := os.Getenv("ONESUB_SUBTITLE_WORKER")
	if workerPath == "" {
		t.Skip("统一 build/test.ps1 设置真实字幕 worker；此处不以 mock 代替")
	}
	worker := subtitle.Client{Executable: workerPath, Scratch: t.TempDir()}
	ffmpeg := os.Getenv("ONESUB_FFMPEG")
	probe := os.Getenv("ONESUB_FFPROBE")
	if ffmpeg == "" || probe == "" {
		t.Fatal("integration test requires FFmpeg/ffprobe")
	}
	root := t.TempDir()
	video := filepath.Join(root, "课程 01.mp4")
	sub := filepath.Join(root, "课程 01.srt")
	run := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "color=size=160x90:rate=10:duration=1", "-an", "-c:v", "mpeg4", video)
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, out)
	}
	if err := os.WriteFile(sub, []byte("1\n00:00:00,100 --> 00:00:00,900\n自制测试字幕\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{video, sub} {
		selection, err := Discover([]string{path})
		if err != nil {
			t.Fatal(err)
		}
		got, err := Load(context.Background(), selection, worker, probe)
		if err != nil || len(got.Document.Cues) != 1 || !got.Media.HasVideo() || len(got.Warnings) != 0 {
			t.Fatalf("pair: %+v %v", got, err)
		}
	}
	if err := os.WriteFile(video, []byte("broken media"), 0600); err != nil {
		t.Fatal(err)
	}
	selection, _ := Discover([]string{sub})
	got, err := Load(context.Background(), selection, worker, probe)
	if err != nil || len(got.Document.Cues) != 1 || got.Selection.Media != "" || len(got.Warnings) != 1 {
		t.Fatalf("primary lost: %+v %v", got, err)
	}
	if err := os.WriteFile(sub, []byte("broken subtitle"), 0600); err != nil {
		t.Fatal(err)
	}
	// Make the primary video valid again, then exercise the opposite failure.
	if out, err := exec.Command(ffmpeg, "-y", "-v", "error", "-f", "lavfi", "-i", "color=size=160x90:rate=10:duration=1", "-an", "-c:v", "mpeg4", video).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %s %v", out, err)
	}
	selection, _ = Discover([]string{video})
	got, err = Load(context.Background(), selection, worker, probe)
	if err != nil || !got.Media.HasVideo() || got.Selection.Subtitle != "" || len(got.Warnings) != 1 {
		t.Fatalf("media lost: %+v %v", got, err)
	}
}
