package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/jobs"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// Long gaps must never be sent to the recognizer. Short pauses stay with
// their sentence, and region offsets remain on the original media clock.
func TestSpeechRegionsExcludeLongSilence(t *testing.T) {
	raw := []byte(`{"dur":52,"timestamps":[[0.07,1.12],[1.72,3.42],[4.02,5.47],[46.04,47.1],[47.7,49.41],[50.01,51.45]],"sample_rate_in":16000,"sample_rate_model":16000}`)
	got, err := speechRegions(raw)
	want := []speechRegion{{StartMs: 0, EndMs: 5720}, {StartMs: 45790, EndMs: 51700}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("regions = %+v, %v; want %+v", got, err, want)
	}
}

func TestSpeechRegionsSilenceAndInvalidOutput(t *testing.T) {
	got, err := speechRegions([]byte(`{"dur":45,"timestamps":[],"sample_rate_in":16000,"sample_rate_model":16000}`))
	if err != nil || len(got) != 0 {
		t.Fatalf("silence reached recognition: %+v, %v", got, err)
	}
	for _, timestamps := range []string{
		`null`, `[[5,4]]`, `[[-1,4]]`, `[[1,46]]`, `[[5,6],[1,2]]`, `[[1,3],[2,4]]`, `[[5]]`, `[[1,2,3]]`, `[[1,1.00001]]`,
	} {
		raw := fmt.Sprintf(`{"dur":45,"timestamps":%s,"sample_rate_in":16000,"sample_rate_model":16000}`, timestamps)
		if _, err := speechRegions([]byte(raw)); err == nil {
			t.Fatalf("invalid VAD timestamps accepted: %s", raw)
		}
	}
	for _, raw := range []string{
		`{}`, `{"dur":45,"timestamps":[]}`, `{"dur":0,"timestamps":[],"sample_rate_in":16000,"sample_rate_model":16000}`,
		`{"dur":2,"timestamps":[[0,1]],"sample_rate_in":48000,"sample_rate_model":16000}`,
		`{"dur":1e20,"timestamps":[],"sample_rate_in":16000,"sample_rate_model":16000}`,
	} {
		if _, err := speechRegions([]byte(raw)); err == nil {
			t.Fatalf("invalid VAD metadata accepted: %s", raw)
		}
	}
}

func TestSpeechRegionPaddingKeepsFileBoundsAndShortPauses(t *testing.T) {
	raw := []byte(`{"dur":6,"timestamps":[[0,1],[2,3],[4.01,6]],"sample_rate_in":16000,"sample_rate_model":16000}`)
	got, err := speechRegions(raw)
	want := []speechRegion{{StartMs: 0, EndMs: 3250}, {StartMs: 3760, EndMs: 6000}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("regions = %+v, %v; want %+v", got, err, want)
	}
}

// An aligner's collapsed tail must not become a new one-second subtitle
// in the silence after a speech region. Normal text keeps the real offset.
func TestRecognitionRegionKeepsTimingsInsideSpeech(t *testing.T) {
	doc := subtitle.Document{Format: "srt", Cues: []subtitle.Cue{
		{ID: "a", Text: "Hello", StartMs: 300, EndMs: 900},
		{ID: "b", Text: "End", StartMs: 1800, EndMs: 1800},
		{ID: "c", Text: "Outside", StartMs: 2300, EndMs: 2300},
	}}
	out := recognitionRegion(doc, speechRegion{StartMs: 45000, EndMs: 47000})
	if len(out.Cues) != 2 || out.Cues[0].StartMs != 45300 || out.Cues[0].EndMs != 45900 || out.Cues[1].StartMs != 46800 || out.Cues[1].EndMs != 47000 {
		t.Fatalf("region escaped its source clock: %+v", out.Cues)
	}
	if doc.Cues[0].StartMs != 300 || doc.Cues[1].EndMs != 1800 {
		t.Fatal("changed the input artifact")
	}
}

func TestRecognitionWordRegionsRoundTripOnSourceClock(t *testing.T) {
	raw := []byte(`{"unit":"aligner_token","word_count":4,"words":[{"text":"Hello","start":0.3,"end":0.9},{"text":"!","start":1.8,"end":1.8},{"text":"End","start":1.9,"end":2.1},{"text":"Outside","start":2.3,"end":2.3}]}`)
	words, err := subtitle.ParseOneAsrWords(raw)
	if err != nil {
		t.Fatal(err)
	}
	mapped := recognitionWordRegion(words, speechRegion{StartMs: 45000, EndMs: 47000})
	data, err := subtitle.MarshalOneAsrWords(mapped)
	if err != nil {
		t.Fatal(err)
	}
	got, err := subtitle.ParseOneAsrWords(data)
	want := []subtitle.Word{{Text: "Hello", StartMs: 45300, EndMs: 45900}, {Text: "!", StartMs: 46800, EndMs: 46800}, {Text: "End", StartMs: 46900, EndMs: 47000}}
	if err != nil || !reflect.DeepEqual(got, want) || words[0].StartMs != 300 || words[2].EndMs != 2100 {
		t.Fatalf("word evidence = %+v, %v; source = %+v", got, err, words)
	}
}

func TestDetectSpeechDoesNotBypassFailureOrCancellation(t *testing.T) {
	if _, err := detectSpeech(context.Background(), t.TempDir(), "absent.wav"); err == nil {
		t.Fatal("missing VAD silently bypassed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := detectSpeech(ctx, t.TempDir(), "absent.wav"); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
}

// The normal build tests exercise the real bundled helper, not a mock VAD.
func TestDetectSpeechRejectsSilentAudioIntegration(t *testing.T) {
	install, ffmpeg := os.Getenv("ONESUB_ONEASR_ROOT"), os.Getenv("ONESUB_FFMPEG")
	if install == "" || ffmpeg == "" {
		t.Skip("Run build/test.ps1 to test the bundled VAD and FFmpeg")
	}
	for _, duration := range []string{"0.1", "2", "45"} {
		t.Run(duration, func(t *testing.T) {
			wav := filepath.Join(t.TempDir(), "silence.wav")
			ctx := context.Background()
			if err := jobs.Run(ctx, jobs.CommandContext(ctx, ffmpeg, "-v", "error", "-f", "lavfi", "-i", "anullsrc=r=16000:cl=mono", "-t", duration, "-c:a", "pcm_s16le", wav), nil); err != nil {
				t.Fatal(err)
			}
			regions, err := detectSpeech(ctx, install, wav)
			if err != nil || len(regions) != 0 {
				t.Fatalf("silence became speech: %+v, %v", regions, err)
			}
		})
	}
}

// Opt-in real Qwen inference: use already downloaded user-selected model
// directories. No network service or automatic model download is involved.
func TestRecognitionSpeechGapIntegration(t *testing.T) {
	modelsRoot := os.Getenv("ONESUB_ASR_TEST_MODELS")
	if modelsRoot == "" {
		t.Skip("Set ONESUB_ASR_TEST_MODELS to test real recognition")
	}
	ctx := context.Background()
	root := t.TempDir()
	tools := toolset{dataRoot: root, ffmpeg: os.Getenv("ONESUB_FFMPEG"), worker: subtitle.Client{Executable: os.Getenv("ONESUB_SUBTITLE_WORKER"), Scratch: root}}
	prefs := settings.Default()
	prefs.AsrModel, prefs.Aligner, prefs.Language = "Qwen3-ASR-1.7B-hf", "Qwen3-ForcedAligner-0.6B-hf", "en"
	prefs.ModelPaths = map[string]string{prefs.AsrModel: filepath.Join(modelsRoot, prefs.AsrModel), prefs.Aligner: filepath.Join(modelsRoot, prefs.Aligner)}
	speech, wav := filepath.Join(root, "speech.wav"), filepath.Join(root, "speech-gap.wav")
	for _, args := range [][]string{
		{"-v", "error", "-f", "lavfi", "-i", "flite=text='Hello world. This is a subtitle test. Thank you for watching.':voice=slt", "-af", "apad=whole_dur=6", "-t", "6", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", speech},
		{"-v", "error", "-i", speech, "-f", "lavfi", "-t", "40", "-i", "anullsrc=r=16000:cl=mono", "-filter_complex", "[0:a]asplit=2[first][last];[first][1:a][last]concat=n=3:v=0:a=1[out]", "-map", "[out]", "-c:a", "pcm_s16le", wav},
	} {
		if err := jobs.Run(ctx, jobs.CommandContext(ctx, tools.ffmpeg, args...), nil); err != nil {
			t.Fatal(err)
		}
	}
	regionCount := 0
	srt, evidence, cleanup, err := tools.recognize(ctx, prefs, wav, func(s recognitionStep) { regionCount = max(regionCount, s.Regions) })
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	doc, err := tools.worker.Read(ctx, srt)
	if err != nil {
		t.Fatal(err)
	}
	first, last := false, false
	for _, cue := range doc.Cues {
		if cue.StartMs < 45500 && cue.EndMs > 6500 || cue.EndMs > 52000 || cue.EndMs <= cue.StartMs {
			t.Fatalf("cue reached excluded silence or escaped the source: %+v", cue)
		}
		first = first || cue.StartMs < 6000
		last = last || cue.StartMs >= 45500
	}
	words, err := subtitle.ParseOneAsrWords(evidence)
	if err != nil || len(words) == 0 {
		t.Fatal("missing word evidence", err)
	}
	for _, word := range words {
		if word.StartMs < 45500 && word.EndMs > 6500 || word.EndMs > 52000 {
			t.Fatalf("word reached excluded silence: %+v", word)
		}
	}
	if !first || !last || regionCount != 2 {
		t.Fatalf("speech missing: first=%v last=%v regions=%d cues=%+v", first, last, regionCount, doc.Cues)
	}

	// A silent recording must stop before inference and must not publish an
	// empty/fictitious subtitle or advance the one-click translation stage.
	silent := filepath.Join(root, "silence.wav")
	if err := jobs.Run(ctx, jobs.CommandContext(ctx, tools.ffmpeg, "-v", "error", "-f", "lavfi", "-i", "anullsrc=r=16000:cl=mono", "-t", "45", "-c:a", "pcm_s16le", silent), nil); err != nil {
		t.Fatal(err)
	}
	inferred := false
	output, raw, remove, err := tools.recognize(ctx, prefs, silent, func(s recognitionStep) { inferred = inferred || s.Stage == "infer" || s.Stage == "load-asr" })
	if err == nil || !strings.Contains(err.Error(), locales.RecognitionNoSpeech()) || inferred || output != "" || len(raw) != 0 || remove != nil {
		t.Fatalf("silent pipeline: err=%v inferred=%v output=%q", err, inferred, output)
	}
}
