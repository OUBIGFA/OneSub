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
	for i, names := range [][2]string{
		{"课程 01.mp4", "课程 01.srt"},
		{"课程 01.mp4", "课程 01-zh.srt"},
		{"Blender Material Editing.mp4", "Blnder Material Editing.srt"},
		{"Introduction to Blender Materials.mp4", "Blender Materials.srt"},
	} {
		nextVideo, nextSub := filepath.Join(root, names[0]), filepath.Join(root, names[1])
		if nextVideo != video {
			if err := os.Rename(video, nextVideo); err != nil {
				t.Fatal(err)
			}
			video = nextVideo
		}
		if nextSub != sub {
			if err := os.Rename(sub, nextSub); err != nil {
				t.Fatal(err)
			}
			sub = nextSub
		}
		// Discovery is pure, so both entry points are checked to pick the same
		// pair; the real worker/ffprobe validation runs once per pair, entering
		// from the video and the subtitle in turn.
		selections := [2]Selection{}
		for k, path := range []string{video, sub} {
			selection, err := Discover([]string{path})
			if err != nil {
				t.Fatal(err)
			}
			selections[k] = selection
		}
		if selections[0].Media != selections[1].Media || selections[0].Subtitle != selections[1].Subtitle {
			t.Fatalf("entry points chose different pairs: %+v %+v", selections[0], selections[1])
		}
		got, err := Load(context.Background(), selections[i%2], worker, probe)
		if err != nil || len(got.Document.Cues) != 1 || !got.Media.HasVideo() || len(got.Warnings) != 0 || got.Selection.Match == nil {
			t.Fatalf("validated weighted pair: %+v %v", got, err)
		}
	}
	if err := os.WriteFile(video, []byte("broken media"), 0600); err != nil {
		t.Fatal(err)
	}
	selection, _ := Discover([]string{sub})
	got, err := Load(context.Background(), selection, worker, probe)
	if err != nil || len(got.Document.Cues) != 1 || got.Selection.Media != "" || got.Selection.Match != nil || len(got.Warnings) != 1 {
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
	if err != nil || !got.Media.HasVideo() || got.Selection.Subtitle != "" || got.Selection.Match != nil || len(got.Warnings) != 1 {
		t.Fatalf("media lost: %+v %v", got, err)
	}
}
