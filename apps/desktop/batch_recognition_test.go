package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/jobs"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// The recognition queue takes media files, keeps one copy of each, and
// leaves what it was given when the app restarts.
func TestBatchQueueKeepsFilesOnceAndSurvivesRestart(t *testing.T) {
	a := nativeForTest(t)
	folder := t.TempDir()
	first := filepath.Join(folder, "a.mp4")
	second := filepath.Join(folder, "b.mkv")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	a.queueBatchFiles([]string{first, second, first})
	if len(a.batchItems) != 2 {
		t.Fatal("the queue took the same file twice", a.batchItems)
	}
	if a.batchItems[0].Status != batchWaiting {
		t.Fatal("a queued file is not waiting", a.batchItems[0])
	}
	// The queue is kept in the settings, so it is there after a restart.
	if len(a.settings.BatchSources) != 2 {
		t.Fatal("the queue was not kept", a.settings.BatchSources)
	}
	loaded, err := settings.Load(filepath.Join(a.dataRoot, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.BatchSources) != 2 {
		t.Fatal("the queue was not saved", loaded.BatchSources)
	}
	// A file that is gone is not offered again.
	if err = os.Remove(second); err != nil {
		t.Fatal(err)
	}
	restored := nativeForTest(t)
	restored.settings = loaded
	restored.restoreBatchSources()
	if len(restored.batchItems) != 1 || restored.batchItems[0].Source != first {
		t.Fatal("the queue came back wrong", restored.batchItems)
	}
}

// The batch transcribes a file and publishes only its subtitle beside it:
// the media folder keeps nothing but the requested subtitle file.
func TestBatchPublishesOnlyTheSubtitle(t *testing.T) {
	folder := t.TempDir()
	source := filepath.Join(folder, "clip.mp4")
	if err := os.WriteFile(source, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	path, err := publishRecognition(source, "", "srt", []byte("1\n00:00:00,000 --> 00:00:01,000\nhi\n"))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != folder {
		t.Fatal("the subtitle was not published beside the media", path)
	}
	entries, err := os.ReadDir(folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatal("the media folder holds more than the subtitle", entries)
	}
	for _, entry := range entries {
		if entry.Name() != "clip.mp4" && entry.Name() != "clip.srt" {
			t.Fatal("the media folder holds an unrelated file", entry.Name())
		}
	}
}

// Opt-in real recognition: the batch transcribes several files one after
// another and writes each one's subtitle beside it. It uses the user's own
// downloaded models; no network service is involved.
func TestBatchRecognitionPublishesEachFileIntegration(t *testing.T) {
	modelsRoot := os.Getenv("ONESUB_ASR_TEST_MODELS")
	if modelsRoot == "" {
		t.Skip("Set ONESUB_ASR_TEST_MODELS to test real batch recognition")
	}
	ffmpeg := os.Getenv("ONESUB_FFMPEG")
	if ffmpeg == "" {
		t.Skip("Run build/test.ps1 for FFmpeg")
	}
	ctx := context.Background()
	root := t.TempDir()
	tools := toolset{dataRoot: root, ffmpeg: ffmpeg, worker: subtitle.Client{Executable: os.Getenv("ONESUB_SUBTITLE_WORKER"), Scratch: root}}
	prefs := settings.Default()
	// Pin manual mode as well as the models: routing's current default would
	// require a language detector and would not exercise this Qwen fixture.
	prefs.RecognitionMode, prefs.AsrModel, prefs.Aligner, prefs.Language = recognitionManual, "Qwen3-ASR-1.7B-hf", "Qwen3-ForcedAligner-0.6B-hf", "en"
	prefs.ModelPaths = map[string]string{prefs.AsrModel: filepath.Join(modelsRoot, prefs.AsrModel), prefs.Aligner: filepath.Join(modelsRoot, prefs.Aligner)}
	media := filepath.Join(root, "media")
	if err := os.MkdirAll(media, 0700); err != nil {
		t.Fatal(err)
	}
	sources := []string{filepath.Join(media, "one.wav"), filepath.Join(media, "two.wav")}
	for _, source := range sources {
		args := []string{"-v", "error", "-f", "lavfi", "-i", "flite=text='Hello world. This is a subtitle test.':voice=slt", "-t", "5", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", source}
		if err := jobs.Run(ctx, jobs.CommandContext(ctx, ffmpeg, args...), nil); err != nil {
			t.Fatal(err)
		}
	}
	a := nativeForTest(t)
	a.worker, a.ffmpeg = tools.worker, tools.ffmpeg
	a.settings = prefs
	a.queueBatchFiles(sources)
	if len(a.batchItems) != 2 {
		t.Fatal("the queue did not take both files", a.batchItems)
	}
	for _, item := range a.batchItems {
		path, err := a.transcribeBatchFile(ctx, tools, item.Source, prefs, a.currentStyle, 16.0/9)
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Dir(path) != media {
			t.Fatal("the subtitle was not written beside the media", path)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(media)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 {
		t.Fatal("the media folder holds more than the subtitles", entries)
	}
}

func TestBatchQueueCountsFailures(t *testing.T) {
	a := nativeForTest(t)
	a.batchItems = []batchItem{
		{Source: "a.mp4", Status: batchDone},
		{Source: "b.mp4", Status: batchFailed, Error: "no model"},
		{Source: "c.mp4", Status: batchWaiting},
	}
	if _, failed := a.batchCounts(); failed != 1 {
		t.Fatal("the failures were not counted", failed)
	}
}

// The queue inherits current-file settings and gives its list the remaining
// viewport. Switching targets must not copy, reset or duplicate the settings.
func TestBatchQueueSharesTheRecognitionPage(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := nativeForTest(t)
			a.recognitionMode = 1
			a.settings.Language, a.settings.Backend, a.settings.RecognitionFormat = "ja", "cpu", "vtt"
			tt := settled(a.workflowPanel, 900, 900)
			for _, label := range []string{locales.RecognitionFileQueue(), locales.BatchAddFiles(), locales.BatchStart(), locales.BatchRemove()} {
				if !tt.HasText(label) {
					t.Fatal("queue control is missing", language, label)
				}
			}
			settingsLabels := []string{locales.WorkflowRecognitionSettings(), locales.RecognitionMode(), locales.ExportFormat(), locales.WorkflowRecognitionAdvanced()}
			for _, label := range settingsLabels {
				if tt.HasText(label) {
					t.Fatal("queue duplicates current-file settings", language, label)
				}
			}
			if !tt.HasText(locales.BatchEmpty()) || tt.HasText(locales.BatchRecognition()) {
				t.Fatal("queue is empty without a drop hint, or is still a third tab", language)
			}
			before, _ := tt.Find(locales.BatchDropArea())
			tt.SetSize(900, 1100)
			tt.Frame()
			after, _ := tt.Find(locales.BatchDropArea())
			if delta := after.H - before.H; delta < 199 || delta > 201 || after.Y+after.H > 1100 {
				t.Fatal("queue does not fill the resized viewport", before, after)
			}
			if err := tt.Click(locales.RecognitionCurrentFile()); err != nil {
				t.Fatal(err)
			}
			for _, label := range settingsLabels {
				if !tt.HasText(label) {
					t.Fatal("current-file settings are not reachable", language, label)
				}
			}
			if !tt.HasText(locales.WorkflowStartRecognize()) || tt.HasText(locales.BatchAddFiles()) || a.settings.Language != "ja" || a.settings.Backend != "cpu" || a.settings.RecognitionFormat != "vtt" {
				t.Fatal("switching recognition target changed settings", language)
			}
		})
	}
}
