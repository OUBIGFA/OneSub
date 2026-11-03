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
	raw := []byte(`{"dur":60,"timestamps":[[0.5,8],[8.4,16],[16.3,19],[20,50]],"sample_rate_in":16000,"sample_rate_model":16000}`)
	got, err := speechRegions(raw, 20000)
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
		if !m.Sherpa() {
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
		{"sherpa-paraformer-zh-int8", "zh", "paraformer.wav", "我想说的是"},
	}
	for _, c := range cases {
		t.Run(c.model, func(t *testing.T) {
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
			words, err := toolset{}.recognizeSherpa(context.Background(), model, dir, c.language, wav, scratch, []speechRegion{{0, duration}}, func(recognitionStep) {})
			if err != nil {
				t.Fatal(err)
			}
			cues := subtitle.WordCues(words)
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
		{"sherpa-parakeet-tdt-0.6b-v3-int8", "en", "en.wav"},
	} {
		t.Run(c.model, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			tools := toolset{dataRoot: root, ffmpeg: os.Getenv("ONESUB_FFMPEG"), worker: subtitle.Client{Executable: os.Getenv("ONESUB_SUBTITLE_WORKER"), Scratch: root}}
			prefs := settings.Default()
			prefs.AsrModel, prefs.Language = c.model, c.language
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
					a.settings.AsrModel, a.settings.Language = "sherpa-parakeet-tdt-0.6b-v3-int8", "zh"
					a.asrModelChanged()
					if a.settings.Language != "en" {
						t.Fatal("unsupported language kept", a.settings.Language)
					}
					if got := modelLanguages(a.settings.AsrModel); len(got) != 7 || got[0].ID != "en" {
						t.Fatal(got)
					}
					a.recognitionAdvanced = true
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
					if a.settings.Language != "en" || len(modelLanguages(a.settings.AsrModel)) != len(sourceLanguages()) {
						t.Fatal("OneAsr languages changed")
					}
				})
			})
		}
	}
}
