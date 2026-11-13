package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/jobs"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/models"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// Matching detector fixtures keep the existing padding/clock cases concise.
// Disagreement and non-speech tests below supply independent VAD/AED payloads.
func matchingVAD(t *testing.T, events []byte) []byte {
	t.Helper()
	var aed struct {
		Duration float64 `json:"dur"`
		Events   struct {
			Speech json.RawMessage `json:"speech"`
		} `json:"event2timestamps"`
	}
	if err := json.Unmarshal(events, &aed); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]any{"dur": aed.Duration, "timestamps": aed.Events.Speech, "sample_rate_in": 16000, "sample_rate_model": 16000})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// Long gaps must never be sent to the recognizer. Short pauses stay with
// their sentence, and region offsets remain on the original media clock.
func TestSpeechRegionsExcludeLongSilence(t *testing.T) {
	raw := []byte(`{"dur":52,"event2timestamps":{"speech":[[0.07,1.12],[1.72,3.42],[4.02,5.47],[46.04,47.1],[47.7,49.41],[50.01,51.45]],"singing":[],"music":[]}}`)
	got, err := speechRegions(matchingVAD(t, raw), raw, 0)
	want := []speechRegion{{StartMs: 0, EndMs: 5720}, {StartMs: 45790, EndMs: 51700}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("regions = %+v, %v; want %+v", got, err, want)
	}
}

func TestSpeechRegionsSilenceAndInvalidOutput(t *testing.T) {
	silent := []byte(`{"dur":45,"event2timestamps":{"speech":[],"singing":[],"music":[]}}`)
	got, err := speechRegions(matchingVAD(t, silent), silent, 0)
	if err != nil || len(got) != 0 {
		t.Fatalf("silence reached recognition: %+v, %v", got, err)
	}
	for _, timestamps := range []string{
		`null`, `[[5,4]]`, `[[-1,4]]`, `[[1,46]]`, `[[5,6],[1,2]]`, `[[1,3],[2,4]]`, `[[5]]`, `[[1,2,3]]`, `[[1,1.00001]]`,
	} {
		raw := fmt.Sprintf(`{"dur":45,"event2timestamps":{"speech":%s,"singing":[],"music":[]}}`, timestamps)
		if _, err := speechRegions(matchingVAD(t, []byte(raw)), []byte(raw), 0); err == nil {
			t.Fatalf("invalid speech timestamps accepted: %s", raw)
		}
	}
	for _, raw := range []string{
		`{}`, `{"dur":45,"event2timestamps":{}}`, `{"dur":45,"event2timestamps":{"music":[[0,45]],"singing":[]}}`,
		`{"dur":0,"event2timestamps":{"speech":[]}}`, `{"dur":1e20,"event2timestamps":{"speech":[]}}`,
		`{"dur":45,"timestamps":[[1,2]],"sample_rate_in":16000,"sample_rate_model":16000}`,
	} {
		if _, err := speechRegions(matchingVAD(t, silent), []byte(raw), 0); err == nil {
			t.Fatalf("invalid AED metadata accepted: %s", raw)
		}
	}
}

func TestSpeechRegionPaddingKeepsFileBoundsAndShortPauses(t *testing.T) {
	raw := []byte(`{"dur":6,"event2timestamps":{"speech":[[0,1],[2,3],[4.01,6]],"singing":[],"music":[]}}`)
	got, err := speechRegions(matchingVAD(t, raw), raw, 0)
	want := []speechRegion{{StartMs: 0, EndMs: 3250}, {StartMs: 3760, EndMs: 6000}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("regions = %+v, %v; want %+v", got, err, want)
	}
}

func TestSpeechRegionsKeepDialogueOverMusicAndExcludeOtherEvents(t *testing.T) {
	// Speech and music labels overlap by design. Comparing the dominant
	// label, or excluding every music interval, would erase the dialogue.
	raw := []byte(`{"dur":60,"event2timestamps":{"speech":[[1,4],[4.5,8]],"singing":[[20,30]],"music":[[0,60]]},"event2ratio":{"speech":0.12,"singing":0.17,"music":1}}`)
	vad := []byte(`{"dur":60,"timestamps":[[1,4],[4.5,8],[20,30],[45,47]],"sample_rate_in":16000,"sample_rate_model":16000}`)
	got, err := speechRegions(vad, raw, 0)
	want := []speechRegion{{StartMs: 750, EndMs: 8250}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("spoken dialogue changed or music entered ASR: %+v, %v", got, err)
	}
	for _, events := range []string{
		`{"speech":[],"singing":[],"music":[[0,60]]}`,
		`{"speech":[],"singing":[[0,60]],"music":[[0,60]]}`,
	} {
		got, err = speechRegions(vad, []byte(`{"dur":60,"event2timestamps":`+events+`}`), 0)
		if err != nil || len(got) != 0 {
			t.Fatalf("non-spoken events entered ASR: %+v, %v", got, err)
		}
	}
}

func TestSpeechRegionsConfirmUnpaddedCandidatesBeforeJoining(t *testing.T) {
	vad := []byte(`{"dur":10,"timestamps":[[0,4.5],[5.1,5.3],[5.5,5.8],[6,8]],"sample_rate_in":16000,"sample_rate_model":16000}`)
	aed := []byte(`{"dur":10,"event2timestamps":{"speech":[[0.9,4.5],[5.1,5.3],[6,7.8]],"singing":[[0,1.3]],"music":[[0,10]]}}`)
	got, err := speechRegions(vad, aed, 0)
	// Keep the weak onset and the short confirmed word. The rejected candidate
	// at 5.5 must prevent the accepted neighbours from joining across it.
	want := []speechRegion{{StartMs: 0, EndMs: 5550}, {StartMs: 5750, EndMs: 8250}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("confirmation lost speech or rejoined a rejected candidate: %+v, %v", got, err)
	}
}

func TestSpeechRegionsRequireBothValidDetectors(t *testing.T) {
	validVAD := `{"dur":2,"timestamps":[[0,1]],"sample_rate_in":16000,"sample_rate_model":16000}`
	validAED := `{"dur":2,"event2timestamps":{"speech":[[0,1]]}}`
	for _, c := range []struct{ vad, aed string }{
		{`not-json`, validAED}, {validVAD, `not-json`},
		{`{"dur":2,"timestamps":[[0,1]],"sample_rate_in":48000,"sample_rate_model":16000}`, validAED},
		{`{"dur":2,"timestamps":null,"sample_rate_in":16000,"sample_rate_model":16000}`, validAED},
		{validVAD, `{"dur":3,"event2timestamps":{"speech":[[0,1]]}}`},
		{validVAD, `{"dur":2,"event2timestamps":{"speech":[[0,1],[0.5,1.5]]}}`},
		{validVAD, validVAD},
	} {
		if _, err := speechRegions([]byte(c.vad), []byte(c.aed), 0); err == nil {
			t.Fatalf("invalid/mismatched detector output accepted: %+v", c)
		}
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
	if _, err := detectSpeech(context.Background(), t.TempDir(), "absent.wav", 0); err == nil {
		t.Fatal("missing VAD silently bypassed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := detectSpeech(ctx, t.TempDir(), "absent.wav", 0); !errors.Is(err, context.Canceled) {
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
			regions, err := detectSpeech(ctx, install, wav, 0)
			if err != nil || len(regions) != 0 {
				t.Fatalf("silence became speech: %+v, %v", regions, err)
			}
		})
	}
}

// Local regression recordings stay outside the repository. Removing the
// speech/music distinction must fail here before any recognizer is loaded.
func TestDetectSpeechRejectsNonSpeechFixtureIntegration(t *testing.T) {
	source, install := os.Getenv("ONESUB_NONSPEECH_TEST_AUDIO"), os.Getenv("ONESUB_ONEASR_ROOT")
	if source == "" || install == "" {
		t.Skip("Set ONESUB_NONSPEECH_TEST_AUDIO to a 16 kHz mono PCM non-speech recording")
	}
	for _, limit := range []int64{0, sherpaChunkMs, probeWindowMs} {
		regions, err := detectSpeech(context.Background(), install, source, limit)
		if err != nil {
			t.Fatal(err)
		}
		if len(regions) != 0 {
			t.Fatalf("non-speech reached an ASR/language detection input (limit=%d): %+v", limit, regions)
		}
	}
}

// A music exclusion based on the dominant event would drop speech over a
// soundtrack. The detector must also keep genuinely spoken repeated words.
func TestDetectSpeechKeepsSpokenRepetitionWithMusicIntegration(t *testing.T) {
	install, ffmpeg := os.Getenv("ONESUB_ONEASR_ROOT"), os.Getenv("ONESUB_FFMPEG")
	if install == "" || ffmpeg == "" {
		t.Skip("Run build/test.ps1 to test the bundled AED and FFmpeg")
	}
	wav := filepath.Join(t.TempDir(), "spoken-repetition.wav")
	args := []string{"-v", "error", "-f", "lavfi", "-i", "flite=text='Oh, oh, oh, oh. This is spoken dialogue with background music.':voice=slt",
		"-f", "lavfi", "-i", "aevalsrc=0.04*sin(2*PI*220*t)+0.03*sin(2*PI*330*t)+0.02*sin(2*PI*440*t):s=16000",
		"-filter_complex", "[0:a][1:a]amix=inputs=2:duration=first:normalize=0[out]", "-map", "[out]", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", wav}
	if err := jobs.Run(context.Background(), jobs.CommandContext(context.Background(), ffmpeg, args...), nil); err != nil {
		t.Fatal(err)
	}
	regions, err := detectSpeech(context.Background(), install, wav, 0)
	if err != nil || len(regions) == 0 || regions[0].StartMs > 500 || regions[len(regions)-1].EndMs < 3000 {
		t.Fatalf("spoken repetition or dialogue was excluded: %+v, %v", regions, err)
	}
}

// Exercise the common entry used by desktop, batch and MCP with each installed
// engine. No download is allowed, and non-speech must stop before inference or
// publishing even when the selected model and aligner are valid.
func TestRecognitionRejectsNonSpeechForInstalledModelsIntegration(t *testing.T) {
	source, modelsRoot := os.Getenv("ONESUB_NONSPEECH_TEST_AUDIO"), os.Getenv("ONESUB_ASR_TEST_MODELS")
	if source == "" || modelsRoot == "" || os.Getenv("ONESUB_SUBTITLE_WORKER") == "" {
		t.Skip("Set ONESUB_NONSPEECH_TEST_AUDIO and ONESUB_ASR_TEST_MODELS and run build/test.ps1")
	}
	for _, model := range models.Catalog() {
		if model.Kind != "asr" {
			continue
		}
		t.Run(model.ID, func(t *testing.T) {
			if _, err := os.Stat(filepath.Join(modelsRoot, model.ID)); errors.Is(err, os.ErrNotExist) {
				t.Skip("Model is not installed")
			} else if err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			tools := toolset{dataRoot: modelsRoot, ffmpeg: os.Getenv("ONESUB_FFMPEG"), worker: subtitle.Client{Executable: os.Getenv("ONESUB_SUBTITLE_WORKER"), Scratch: root}}
			prefs := settings.Default()
			prefs.RecognitionMode, prefs.AsrModel, prefs.Aligner, prefs.Language = recognitionManual, model.ID, "Qwen3-ForcedAligner-0.6B-hf", "en"
			prefs.ModelPaths = map[string]string{model.ID: filepath.Join(modelsRoot, model.ID), prefs.Aligner: filepath.Join(modelsRoot, prefs.Aligner)}
			if model.Sherpa() {
				prefs.Language = model.Languages[0]
			}
			inferred := false
			srt, words, cleanup, err := tools.recognize(context.Background(), prefs, source, func(s recognitionStep) {
				inferred = inferred || s.Stage == "infer" || s.Stage == "load-asr" || s.Stage == "transcribe" || s.Stage == "save"
			})
			if cleanup != nil {
				defer cleanup()
			}
			if err == nil || !strings.Contains(err.Error(), locales.RecognitionNoSpeech()) || inferred || srt != "" || len(words) != 0 || cleanup != nil {
				t.Fatalf("non-speech pipeline: err=%v inferred/published=%v output=%q", err, inferred, srt)
			}
		})
	}
}

// Accept a local speech-then-music fixture and its independently checked speech
// end. Only the fixture's location is configurable; no private media or text
// is checked in. This tests actual recognition, word mapping and segmentation.
func TestRecognitionSpeechThenMusicFixtureIntegration(t *testing.T) {
	source, modelsRoot := os.Getenv("ONESUB_SPEECH_MUSIC_TEST_AUDIO"), os.Getenv("ONESUB_ASR_TEST_MODELS")
	if source == "" || modelsRoot == "" || os.Getenv("ONESUB_SUBTITLE_WORKER") == "" {
		t.Skip("Set ONESUB_SPEECH_MUSIC_TEST_AUDIO, ONESUB_SPEECH_END_MS and ONESUB_ASR_TEST_MODELS")
	}
	endMs, err := strconv.ParseInt(os.Getenv("ONESUB_SPEECH_END_MS"), 10, 64)
	if err != nil || endMs <= 0 {
		t.Fatal("ONESUB_SPEECH_END_MS must identify the fixture's last spoken audio")
	}
	for _, id := range []string{"Qwen3-ASR-1.7B-hf", "sherpa-sense-voice-small-int8", "sherpa-parakeet-tdt-0.6b-v3-int8"} {
		t.Run(id, func(t *testing.T) {
			if _, err := os.Stat(filepath.Join(modelsRoot, id)); errors.Is(err, os.ErrNotExist) {
				t.Skip("Model is not installed")
			} else if err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			tools := toolset{ffmpeg: os.Getenv("ONESUB_FFMPEG"), worker: subtitle.Client{Executable: os.Getenv("ONESUB_SUBTITLE_WORKER"), Scratch: root}}
			prefs := settings.Default()
			prefs.RecognitionMode, prefs.AsrModel, prefs.Aligner, prefs.Language = recognitionManual, id, "Qwen3-ForcedAligner-0.6B-hf", "en"
			prefs.ModelPaths = map[string]string{id: filepath.Join(modelsRoot, id), prefs.Aligner: filepath.Join(modelsRoot, prefs.Aligner)}
			srt, evidence, cleanup, err := tools.recognize(context.Background(), prefs, source, func(recognitionStep) {})
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			doc, err := tools.worker.Read(context.Background(), srt)
			if err != nil || len(doc.Cues) == 0 {
				t.Fatal("spoken dialogue missing", err)
			}
			for _, cue := range doc.Cues {
				if cue.EndMs > endMs+250 || cue.EndMs <= cue.StartMs {
					t.Fatalf("subtitle reached the music-only tail: %+v", cue)
				}
			}
			words, err := subtitle.ParseOneAsrWords(evidence)
			if err != nil || len(words) == 0 {
				t.Fatal("spoken word evidence missing", err)
			}
			for _, word := range words {
				if word.EndMs > endMs+250 {
					t.Fatalf("word reached the music-only tail: %+v", word)
				}
			}
			if _, warnings, err := subtitle.AttachOneAsrWords(doc, evidence); err != nil || len(warnings) != 0 {
				t.Fatal("word evidence no longer matches subtitles", warnings, err)
			}
			t.Logf("kept %d spoken cues and %d words; no output after the speech boundary", len(doc.Cues), len(words))
		})
	}
}

func TestParseOneAsrRegion(t *testing.T) {
	for _, sample := range []struct {
		line            string
		region, regions int
		valid           bool
	}{
		{"[region] 0/2", 0, 2, true},
		{"[region] 2/2", 2, 2, true},
		{"[region] 1/1", 1, 1, true},
		{"[region] 3/2", 0, 0, false},
		{"[region] -1/2", 0, 0, false},
		{"[region] 0/0", 0, 0, false},
		{"[region] 1/a", 0, 0, false},
		{"[stage] Loading ASR model", 0, 0, false},
	} {
		progress, valid := parseOneAsrRegion(sample.line)
		if valid != sample.valid || progress.Region != sample.region || progress.Regions != sample.regions {
			t.Fatalf("%q: progress=%+v valid=%v", sample.line, progress, valid)
		}
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
	if backend := os.Getenv("ONESUB_ASR_TEST_BACKEND"); backend != "" {
		prefs.Backend = backend
	}
	prefs.RecognitionMode, prefs.AsrModel, prefs.Aligner, prefs.Language = recognitionManual, "Qwen3-ASR-1.7B-hf", "Qwen3-ForcedAligner-0.6B-hf", "en"
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
	loads := map[string]int{}
	srt, evidence, cleanup, err := tools.recognize(ctx, prefs, wav, func(s recognitionStep) {
		regionCount = max(regionCount, s.Regions)
		if s.Stage == "load-asr" || s.Stage == "load-aligner" {
			loads[s.Stage]++
		}
	})
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
	if loads["load-asr"] != 1 || loads["load-aligner"] != 1 {
		t.Fatalf("each model must load once per task, got %v", loads)
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
