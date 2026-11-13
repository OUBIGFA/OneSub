package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/jobs"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/models"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// Sherpa inputs stay under the engines' utterance length: joined speech
// stops at the limit, long speech is cut into equal parts, and context
// never overlaps a neighbor.
func TestSpeechRegionsLimitLength(t *testing.T) {
	raw := []byte(`{"dur":60,"event2timestamps":{"speech":[[0.5,8],[8.4,16],[16.3,19],[20,50]],"singing":[],"music":[]}}`)
	got, err := speechRegions(matchingVAD(t, raw), raw, 20000)
	want := []speechRegion{{250, 19250}, {19750, 35000}, {35000, 50250}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("regions = %+v, %v; want %+v", got, err, want)
	}
	for i := 1; i < len(got); i++ {
		if got[i].StartMs < got[i-1].EndMs {
			t.Fatal("overlap", got)
		}
	}
}

func TestPCMWritesRegionWAV(t *testing.T) {
	dir := t.TempDir()
	samples := make([]byte, 32000*2) // two seconds
	for i := range samples {
		samples[i] = byte(i)
	}
	source := filepath.Join(dir, "in.wav")
	// FFmpeg writes a LIST chunk before the data.
	header := []byte("RIFF\x00\x00\x00\x00WAVEfmt \x10\x00\x00\x00\x01\x00\x01\x00\x80\x3e\x00\x00\x00\x7d\x00\x00\x02\x00\x10\x00LIST\x04\x00\x00\x00INFOdata")
	header = binary.LittleEndian.AppendUint32(header, uint32(len(samples)))
	os.WriteFile(source, append(header, samples...), 0600)
	pcm, err := openPCM(source)
	if err != nil {
		t.Fatal(err)
	}
	defer pcm.Close()
	out := filepath.Join(dir, "part.wav")
	if err = pcm.write(out, speechRegion{StartMs: 500, EndMs: 2500}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(out)
	if len(data) != 44+48000 || data[44] != samples[16000] || binary.LittleEndian.Uint32(data[40:]) != 48000 {
		t.Fatal(len(data))
	}
	os.WriteFile(source, []byte("RIFF\x00\x00\x00\x00WAVEdata\x00\x00\x00\x00"), 0600)
	if _, err = openPCM(source); err == nil {
		t.Fatal("WAV without format accepted")
	}
}

func TestSherpaArgsUseRelativeModelFiles(t *testing.T) {
	for _, m := range models.Catalog() {
		if !m.Sherpa() || m.Kind != "asr" {
			continue
		}
		args := strings.Join(sherpaArgs(m, `oneasr-models-1\models\`+m.ID, "zh"), " ")
		for _, f := range m.Files {
			if strings.HasSuffix(f.Name, ".onnx") && f.Name != "punct.onnx" && !strings.Contains(args, "oneasr-models-1/models/"+m.ID+"/"+f.Name) {
				t.Fatal(m.ID, args)
			}
		}
	}
}

func TestSherpaModelChecksLanguageAndSeparation(t *testing.T) {
	prefs := settings.Default()
	prefs.AsrModel, prefs.Language = "sherpa-parakeet-tdt-0.6b-v3-int8", "zh"
	tools := toolset{dataRoot: t.TempDir()}
	if _, _, err := tools.recognitionModels(prefs); err == nil || !strings.Contains(err.Error(), "Parakeet") {
		t.Fatal(err)
	}
	prefs.Language, prefs.VocalSeparation = "en", true
	if _, _, err := tools.recognitionModels(prefs); err == nil {
		t.Fatal("separation accepted")
	}
	prefs.VocalSeparation = false
	if _, _, err := tools.recognitionModels(prefs); err == nil {
		t.Fatal("missing model accepted")
	}
	os.MkdirAll(tools.modelPath(prefs, prefs.AsrModel), 0700)
	if asr, _, err := tools.recognitionModels(prefs); err != nil || !asr.Sherpa() {
		t.Fatal(asr, err)
	}
}

// With ONESUB_SHERPA_TEST_MODELS set to a folder of the catalog's sherpa
// models (one folder per model ID, and test_wavs with 16 kHz recordings)
// and ONESUB_SHERPA_ROOT to the runtime, run the real engines on their own
// test recordings. Nothing is downloaded.
func TestSherpaEnginesRecognizeRealSpeech(t *testing.T) {
	root, cli := os.Getenv("ONESUB_SHERPA_TEST_MODELS"), os.Getenv("ONESUB_SHERPA_ROOT")
	if root == "" || cli == "" {
		t.Skip("set ONESUB_SHERPA_TEST_MODELS and ONESUB_SHERPA_ROOT")
	}
	cases := []struct{ model, language, wav, want string }{
		{"sherpa-sense-voice-small-int8", "zh", "zh.wav", "开饭时间早上9点至下午5点。"},
		{"sherpa-parakeet-tdt-0.6b-v3-int8", "en", "en.wav", "Ask not what your country can do for you"},
		{"sherpa-parakeet-tdt-0.6b-v3-fp32", "en", "en.wav", "Ask not what your country can do for you"},
		{"sherpa-parakeet-tdt-0.6b-v3-fp32", "de", "de.wav", "Alles hat ein Ende"},
		{"sherpa-paraformer-zh-int8", "zh", "paraformer.wav", "我想说的是"},
	}
	for _, c := range cases {
		t.Run(c.model+"/"+c.language, func(t *testing.T) {
			model, err := models.ByID(c.model)
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(root, c.model)
			if _, err := os.Stat(dir); err != nil {
				t.Skip(err)
			}
			scratch := t.TempDir()
			wav := filepath.Join(root, "test_wavs", c.wav)
			pcm, err := openPCM(wav)
			if err != nil {
				t.Fatal(err)
			}
			duration := pcm.length / pcmBytesPerMs
			pcm.Close()
			cues, err := toolset{}.recognizeSherpa(context.Background(), model, dir, c.language, wav, scratch, []speechRegion{{0, duration}}, func(recognitionStep) {})
			if err != nil {
				t.Fatal(err)
			}
			if len(cues) != 1 {
				t.Fatalf("one engine result became %d cues", len(cues))
			}
			text := ""
			for _, cue := range cues {
				text += cue.Text
			}
			if !strings.Contains(text, c.want) {
				t.Fatal(text)
			}
			t.Log(text)
		})
	}
}

// The whole sherpa pipeline, with the real FireRedVAD, FFmpeg and subtitle
// worker: speech, 40 s of silence, the same speech. No cue or word reaches
// the silence, the words attach to their cues, and a silent recording
// stops before the model runs.
func TestSherpaRecognitionSpeechGapIntegration(t *testing.T) {
	modelsRoot := os.Getenv("ONESUB_SHERPA_TEST_MODELS")
	if modelsRoot == "" || os.Getenv("ONESUB_ONEASR_ROOT") == "" || os.Getenv("ONESUB_SUBTITLE_WORKER") == "" {
		t.Skip("set ONESUB_SHERPA_TEST_MODELS and run build/test.ps1")
	}
	for _, c := range []struct{ model, language, wav string }{
		{"sherpa-sense-voice-small-int8", "zh", "zh.wav"},
		{"sherpa-sense-voice-small-int8", "auto", "zh.wav"},
		{"sherpa-parakeet-tdt-0.6b-v3-int8", "en", "en.wav"},
		{"sherpa-parakeet-tdt-0.6b-v3-fp32", "en", "en.wav"},
	} {
		t.Run(c.model+"/"+c.language, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			tools := toolset{dataRoot: root, ffmpeg: os.Getenv("ONESUB_FFMPEG"), worker: subtitle.Client{Executable: os.Getenv("ONESUB_SUBTITLE_WORKER"), Scratch: root}}
			prefs := settings.Default()
			prefs.RecognitionMode, prefs.AsrModel, prefs.Language = recognitionManual, c.model, c.language
			prefs.ModelPaths = map[string]string{c.model: filepath.Join(modelsRoot, c.model)}
			speech, wav := filepath.Join(modelsRoot, "test_wavs", c.wav), filepath.Join(root, "speech-gap.wav")
			if err := jobs.Run(ctx, jobs.CommandContext(ctx, tools.ffmpeg, "-v", "error", "-i", speech, "-f", "lavfi", "-t", "40", "-i", "anullsrc=r=16000:cl=mono", "-filter_complex", "[0:a]asplit=2[first][last];[first][1:a][last]concat=n=3:v=0:a=1[out]", "-map", "[out]", "-c:a", "pcm_s16le", wav), nil); err != nil {
				t.Fatal(err)
			}
			pcm, err := openPCM(speech)
			if err != nil {
				t.Fatal(err)
			}
			length := pcm.length / pcmBytesPerMs
			pcm.Close()
			srt, evidence, cleanup, err := tools.recognize(ctx, prefs, wav, func(recognitionStep) {})
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
				if cue.StartMs < length+40000-300 && cue.EndMs > length+300 || cue.EndMs > 2*length+40000+300 || cue.EndMs <= cue.StartMs {
					t.Fatalf("cue reached the silence: %+v", cue)
				}
				first = first || cue.StartMs < length
				last = last || cue.StartMs >= length+40000-300
			}
			if !first || !last {
				t.Fatalf("speech missing: %+v", doc.Cues)
			}
			attached, warnings, err := subtitle.AttachOneAsrWords(doc, evidence)
			if err != nil || len(warnings) > 0 {
				t.Fatal(warnings, err)
			}
			for _, cue := range attached.Cues {
				if len(cue.Words) == 0 {
					t.Fatalf("cue without words: %+v", cue)
				}
			}
			silent := filepath.Join(root, "silence.wav")
			if err := jobs.Run(ctx, jobs.CommandContext(ctx, tools.ffmpeg, "-v", "error", "-f", "lavfi", "-i", "anullsrc=r=16000:cl=mono", "-t", "45", "-c:a", "pcm_s16le", silent), nil); err != nil {
				t.Fatal(err)
			}
			inferred := false
			_, _, remove, err := tools.recognize(ctx, prefs, silent, func(s recognitionStep) { inferred = inferred || s.Stage == "load-asr" })
			if err == nil || !strings.Contains(err.Error(), locales.RecognitionNoSpeech()) || inferred || remove != nil {
				t.Fatalf("silent pipeline: err=%v inferred=%v", err, inferred)
			}
		})
	}
}

// Choosing a sherpa-onnx model lists only its languages, moves an
// unsupported language to its first one, and turns off OneAsr's options;
// the model's name stays whole beside the language in narrow panels.
func TestWorkflowSherpaModelLanguagesAndOptions(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		for _, width := range []int{320, 450} {
			t.Run(fmt.Sprintf("%s/%d", language, width), func(t *testing.T) {
				inLocale(t, language, func() {
					a := nativeForTest(t)
					a.settings.RecognitionMode, a.settings.AsrModel, a.settings.Language = recognitionManual, "sherpa-parakeet-tdt-0.6b-v3-int8", "zh"
					a.asrModelChanged()
					if a.settings.Language != "en" {
						t.Fatal("unsupported language kept", a.settings.Language)
					}
					// Only a model that detects the language lists "auto".
					if got := modelLanguages(a.settings.AsrModel); len(got) != 7 || got[0].ID != "en" {
						t.Fatal(got)
					}
					tt := settled(a.workflowPanel, width, 900)
					for range 4 {
						tt.Frame()
					}
					captureRedesign(t, tt, fmt.Sprintf("sherpa-workflow-%s-%d", language, width))
					if !tt.HasText("Parakeet-TDT 0.6B v3 INT8") {
						t.Fatal("model name missing")
					}
					box, ok := tt.Find(locales.WorkflowRecognitionModel())
					if !ok || box.X < 0 || box.X+box.W > float32(width) {
						t.Fatal("model select clipped", box)
					}
					a.settings.AsrModel = "Qwen3-ASR-0.6B-hf"
					a.asrModelChanged()
					if a.settings.Language != "en" || len(modelLanguages(a.settings.AsrModel)) != len(sourceLanguages())-1 {
						t.Fatal("OneAsr languages changed")
					}
				})
			})
		}
	}
}

// The models page holds the language detector and a model per language
// group inside the fixed settings content, in both languages and widths.
func TestSettingsLanguageRoutesFitFixedDialog(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		for _, width := range []int{900, 1400} {
			t.Run(fmt.Sprintf("%s/%d", language, width), func(t *testing.T) {
				inLocale(t, language, func() {
					a := nativeForTest(t)
					a.settingsOpen, a.settingsTab = true, 5
					tt := settled(a.view, width, 640)
					for range 6 {
						tt.Frame()
					}
					captureRedesign(t, tt, fmt.Sprintf("settings-routes-%s-%d", language, width))
					insideRedesign(t, tt, locales.A11ySettingsContent(), locales.ModelKindLid(), locales.RouteGroupEu())
					if !tt.HasText("Parakeet-TDT 0.6B v3 INT8") || !tt.HasText("SenseVoiceSmall INT8") {
						t.Fatal("default routes missing", tt.Texts())
					}
				})
			})
		}
	}
}

func TestMajorityAndProbeWindows(t *testing.T) {
	if v, ok := majority([]string{"zh", "ja", "zh"}); !ok || v != "zh" {
		t.Fatal(v, ok)
	}
	if _, ok := majority([]string{"zh", "ja", "ko"}); ok {
		t.Fatal("no majority accepted")
	}
	if _, ok := majority([]string{"zh", "ja"}); ok {
		t.Fatal("split vote accepted")
	}
	if v, ok := majority([]string{"en"}); !ok || v != "en" {
		t.Fatal(v, ok)
	}
	regions := []speechRegion{{0, 1}, {2, 3}, {4, 5}, {6, 7}, {8, 9}}
	if got := pickWindows(regions); !reflect.DeepEqual(got, []speechRegion{{0, 1}, {4, 5}, {8, 9}}) {
		t.Fatal(got)
	}
	if got := pickWindows(regions[:1]); len(got) != 1 {
		t.Fatal(got)
	}
}

func TestRouteModelKeepsValidChoices(t *testing.T) {
	prefs := settings.Default()
	if routeModel(prefs, "en") != "sherpa-parakeet-tdt-0.6b-v3-int8" || routeModel(prefs, "zh") != models.FallbackModel {
		t.Fatal("defaults")
	}
	prefs.LanguageModels = map[string]string{"zh": "sherpa-paraformer-zh-int8", "ja": "sherpa-paraformer-zh-int8", "eu": "Qwen3-ASR-0.6B-hf"}
	if routeModel(prefs, "zh") != "sherpa-paraformer-zh-int8" || routeModel(prefs, "ja") != models.FallbackModel || routeModel(prefs, "eu") != "Qwen3-ASR-0.6B-hf" {
		t.Fatal("choices")
	}
}

// Real detection and routing on the models' own recordings in six
// languages: Cantonese, which Whisper reads as Chinese, comes out as
// Cantonese through SenseVoiceSmall's confirmation.
func TestRouteRecognitionDetectsRealSpeech(t *testing.T) {
	modelsRoot := os.Getenv("ONESUB_SHERPA_TEST_MODELS")
	if modelsRoot == "" || os.Getenv("ONESUB_ONEASR_ROOT") == "" || os.Getenv("ONESUB_FFMPEG") == "" {
		t.Skip("set ONESUB_SHERPA_TEST_MODELS and run build/test.ps1")
	}
	root := t.TempDir()
	tools := toolset{dataRoot: root, ffmpeg: os.Getenv("ONESUB_FFMPEG"), worker: subtitle.Client{Scratch: root}}
	prefs := settings.Default()
	for _, id := range []string{models.LanguageDetector, models.FallbackModel, "sherpa-parakeet-tdt-0.6b-v3-int8"} {
		prefs.ModelPaths[id] = filepath.Join(modelsRoot, id)
	}
	for _, c := range []struct{ wav, language, model string }{
		{"zh.wav", "zh", models.FallbackModel}, {"yue.wav", "yue", models.FallbackModel},
		{"ja.wav", "ja", models.FallbackModel}, {"ko.wav", "ko", models.FallbackModel},
		{"en.wav", "en", "sherpa-parakeet-tdt-0.6b-v3-int8"}, {"de.wav", "de", "sherpa-parakeet-tdt-0.6b-v3-int8"},
	} {
		t.Run(c.wav, func(t *testing.T) {
			steps := []string{}
			routed, route, err := tools.routeRecognition(context.Background(), prefs, filepath.Join(modelsRoot, "test_wavs", c.wav), func(s recognitionStep) { steps = append(steps, s.Stage) })
			if err != nil {
				t.Fatal(err)
			}
			if routed.Language != c.language || routed.AsrModel != c.model || route.Fallback || len(steps) == 0 || steps[0] != "lid" {
				t.Fatalf("%+v %+v %v", route, routed.AsrModel, steps)
			}
		})
	}
	// A missing Parakeet model: English falls back to SenseVoiceSmall,
	// German has no fallback; Chinese uses SenseVoiceSmall when its chosen
	// Paraformer is missing.
	prefs.ModelPaths["sherpa-parakeet-tdt-0.6b-v3-int8"] = filepath.Join(root, "absent")
	if routed, route, err := tools.routeRecognition(context.Background(), prefs, filepath.Join(modelsRoot, "test_wavs", "en.wav"), func(recognitionStep) {}); err != nil || !route.Fallback || routed.AsrModel != models.FallbackModel || routed.Language != "en" {
		t.Fatal(route, err)
	}
	if _, _, err := tools.routeRecognition(context.Background(), prefs, filepath.Join(modelsRoot, "test_wavs", "de.wav"), func(recognitionStep) {}); err == nil || !strings.Contains(err.Error(), "Parakeet") {
		t.Fatal(err)
	}
	prefs.LanguageModels = map[string]string{"zh": "sherpa-paraformer-zh-int8"}
	prefs.ModelPaths["sherpa-paraformer-zh-int8"] = filepath.Join(root, "absent")
	routed, route, err := tools.routeRecognition(context.Background(), prefs, filepath.Join(modelsRoot, "test_wavs", "zh.wav"), func(recognitionStep) {})
	if err != nil || !route.Fallback || routed.AsrModel != models.FallbackModel {
		t.Fatal(route, err)
	}
	// Without the detector, automatic recognition says what to download.
	prefs.ModelPaths[models.LanguageDetector] = filepath.Join(root, "absent")
	if _, _, err := tools.routeRecognition(context.Background(), prefs, filepath.Join(modelsRoot, "test_wavs", "zh.wav"), func(recognitionStep) {}); err == nil || err.Error() != locales.ErrLanguageDetectorNeeded() {
		t.Fatal(err)
	}
}

// Smart routing asks for no model or language; choosing a model shows both,
// and a detecting model alone offers its own "auto".
func TestWorkflowRecognitionModes(t *testing.T) {
	a := nativeForTest(t)
	if a.settings.RecognitionMode != recognitionRouted {
		t.Fatal("smart routing is not the default", a.settings.RecognitionMode)
	}
	tt := settled(a.workflowPanel, 450, 900)
	if !tt.HasText(locales.RecognitionModeRouted()) || tt.HasText(locales.SettingsSourceLanguage()) || tt.HasText("Qwen3-ASR 0.6B") {
		t.Fatal(tt.Texts())
	}
	a.settings.RecognitionMode, a.settings.AsrModel, a.settings.Language = recognitionManual, models.FallbackModel, languageAuto
	a.asrModelChanged()
	if a.settings.Language != languageAuto {
		t.Fatal("SenseVoiceSmall lost its own detection")
	}
	tt = settled(a.workflowPanel, 450, 900)
	if !tt.HasText(locales.SettingsSourceLanguage()) || !tt.HasText("SenseVoiceSmall INT8") {
		t.Fatal(tt.Texts())
	}
	a.settings.AsrModel = "Qwen3-ASR-0.6B-hf"
	a.asrModelChanged()
	if a.settings.Language != "zh" {
		t.Fatal("OneAsr kept auto", a.settings.Language)
	}
}
