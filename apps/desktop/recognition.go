package main

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/styles"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *NativeApp) pickTranslationDir() {
	a.run(locales.RecognitionChooseFolder(), func(context.Context) (func(), error) {
		paths, err := a.openDialog(mygo.OpenDialogOptions{Parent: a.win, Title: locales.RecognitionChooseFolder(), Directory: true, DefaultPath: a.settings.TranslationDir})
		if err != nil || len(paths) == 0 {
			return nil, err
		}
		return func() { a.settings.TranslationDir = paths[0]; a.saveSettings() }, nil
	})
}

// translationSuffix names a translation's file after its language, as
// lesson-zh.srt: the language's code, with the region only where the
// language has scripts of its own (zh-TW).
func translationSuffix(language string) string {
	switch language {
	case "zh-CN", "zh":
		return "zh"
	}
	return strings.ToLower(language)
}

func (a *NativeApp) pickRecognitionDir() {
	a.run(locales.RecognitionChooseFolder(), func(context.Context) (func(), error) {
		paths, err := a.openDialog(mygo.OpenDialogOptions{Parent: a.win, Title: locales.RecognitionChooseFolder(), Directory: true, DefaultPath: a.settings.RecognitionDir})
		if err != nil || len(paths) == 0 {
			return nil, err
		}
		return func() { a.settings.RecognitionDir = paths[0]; a.saveSettings() }, nil
	})
}

// recognitionStep is where a recognition is: a stage, and in a stage done
// in chunks, the chunk it is at of Total.
type recognitionStep struct {
	Stage       string
	Done, Total int
}

// oneAsrStages are the stage names the pinned OneAsr v1.3.0 CLI writes to
// stderr, as "[阶段] 转写 3/12" or "[stage] Transcribing 3/12" in the
// language of the system; the chunk count only when a stage has more than
// one. The names are its AsrStage labels, by stage.
var oneAsrStages = map[string]string{
	"转码音频": "convert", "Converting audio": "convert",
	"人声分离": "separate", "Separating vocals": "separate",
	"分段规划": "plan", "Planning segments": "plan",
	"热词提取": "hotwords", "Extracting hotwords": "hotwords",
	"加载识别模型": "load-asr", "Loading ASR model": "load-asr",
	"转写": "transcribe", "Transcribing": "transcribe",
	"加载对齐模型": "load-aligner", "Loading aligner model": "load-aligner",
	"打轴": "align", "Aligning": "align",
	"导出字幕": "export", "Exporting subtitles": "export",
}

// parseOneAsrStage reads a stage line of the CLI; other lines are not.
func parseOneAsrStage(line string) (recognitionStep, bool) {
	rest, ok := strings.CutPrefix(line, "[阶段] ")
	if !ok {
		if rest, ok = strings.CutPrefix(line, "[stage] "); !ok {
			return recognitionStep{}, false
		}
	}
	step := recognitionStep{}
	if i := strings.LastIndexByte(rest, ' '); i > 0 {
		if done, total, found := strings.Cut(rest[i+1:], "/"); found {
			d, err1 := strconv.Atoi(done)
			t, err2 := strconv.Atoi(total)
			if err1 == nil && err2 == nil && d >= 1 && d <= t {
				step.Done, step.Total, rest = d, t, rest[:i]
			}
		}
	}
	if step.Stage, ok = oneAsrStages[rest]; !ok {
		return recognitionStep{}, false
	}
	return step, true
}

// recognitionStepLabel says a step in the app's language: OneSub's own
// stages around the CLI, and the CLI's, with the chunk.
func recognitionStepLabel(step recognitionStep) string {
	label := map[string]func() string{
		"verify": locales.RecognitionVerify, "extract": locales.RecognitionExtract,
		"infer": locales.RecognitionInfer, "save": locales.RecognitionSave,
		"convert": locales.RecognitionStageConvert, "separate": locales.RecognitionStageSeparate,
		"plan": locales.RecognitionStagePlan, "hotwords": locales.RecognitionStageHotwords,
		"load-asr": locales.RecognitionStageLoadAsr, "transcribe": locales.RecognitionStageTranscribe,
		"load-aligner": locales.RecognitionStageLoadAligner, "align": locales.RecognitionStageAlign,
		"export": locales.RecognitionStageExport,
	}[step.Stage]
	if label == nil {
		return ""
	}
	if step.Total > 1 {
		return locales.RecognitionStageChunk(label(), step.Done, step.Total)
	}
	return label()
}

// recognitionProgressView takes the player's place while OneSub
// recognizes: the stage the CLI reports, its chunks done in a determinate
// bar, and the time it has taken. A stage without chunks has no
// percentage; none is estimated.
func (a *NativeApp) recognitionProgressView(c *ui.Context) {
	ui.Column(c).Grow(1).Center().Gap(em(c, 16)).Label(locales.RecognitionProgressArea()).Children(func() {
		label := recognitionStepLabel(a.recognitionStep)
		dialogTitle(c, locales.ActionRecognize())
		muted(c, label).FontFeatures("tnum")
		fraction := float64(-1)
		if step := a.recognitionStep; step.Total > 1 {
			fraction = float64(step.Done) / float64(step.Total)
		}
		ui.Progress(c, fraction).Width(em(c, 400)).Label(label)
		seconds := max(0, int(c.Now().Sub(a.recognitionStarted).Seconds()))
		muted(c, locales.RecognitionElapsed(seconds)).FontFeatures("tnum")
		c.After(time.Second)
		ui.Button(c, locales.ActionCancelTask()).Disabled(a.cancel == nil).OnClick(func() {
			if a.cancel != nil {
				a.cancel()
			}
		})
	})
}

// recognitionStepChanged takes a step from the recognition's goroutine.
func (a *NativeApp) recognitionStepChanged(step recognitionStep) {
	a.win.Update(func() {
		a.recognitionStep = step
		a.status = recognitionStepLabel(step)
	})
}

// Recognition never overwrites an existing subtitle, including symlink/hardlink
// aliases. The saved filename is also the UI receipt.
func publishRecognition(source, directory, format string, data []byte) (string, error) {
	if directory == "" {
		directory = filepath.Dir(source)
	}
	return publishBeside(directory, strings.TrimSuffix(filepath.Base(source), filepath.Ext(source)), format, data)
}

// finishRecognition validates the CLI artifacts, saves the selected format,
// then loads that exact file. A save failure still leaves the recognized text
// editable and explicitly unsaved; it never masquerades as a successful export.
func (a *NativeApp) finishRecognition(ctx context.Context, source, srt string, words []byte, prefs settings.Settings, style styles.Style, aspect float64) (func(), error) {
	data, repaired, err := a.toolset().recognitionFile(ctx, srt, prefs, style, aspect)
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	path, saveErr := publishRecognition(source, prefs.RecognitionDir, prefs.RecognitionFormat, data)
	input := path
	if saveErr != nil {
		input = srt
	}
	apply, err := a.importFiles(ctx, []string{source, input}, words)
	if err != nil {
		return nil, err
	}
	return func() {
		apply()
		if saveErr != nil {
			a.markSaved(0, "")
			a.errorText = locales.RecognitionSaveFailed(saveErr.Error())
			a.status = locales.StatusFailed()
			return
		}
		a.markSaved(0, path)
		a.lastOutput = path
		a.status = locales.RecognitionSaved(len(a.snapshot.Document.Cues), path)
		if repaired > 0 {
			a.status += locales.ListSeparator() + locales.RecognitionRepaired(repaired)
		}
	}, nil
}
