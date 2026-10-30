package main

import (
	"strings"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/translator"
)

// The pinned OneAsr CLI's stage lines, in either system language, become
// the app's stages with their chunks; nothing else is read as one.
func TestOneAsrStageLines(t *testing.T) {
	for line, want := range map[string]recognitionStep{
		"[阶段] 转码音频":                     {Stage: "convert"},
		"[stage] Converting audio":      {Stage: "convert"},
		"[阶段] 转写 3/12":                  {Stage: "transcribe", Done: 3, Total: 12},
		"[stage] Transcribing 12/12":    {Stage: "transcribe", Done: 12, Total: 12},
		"[阶段] 打轴 1/40":                  {Stage: "align", Done: 1, Total: 40},
		"[stage] Separating vocals 2/9": {Stage: "separate", Done: 2, Total: 9},
		"[stage] Loading aligner model": {Stage: "load-aligner"},
		"[阶段] 导出字幕":                     {Stage: "export"},
	} {
		if got, ok := parseOneAsrStage(line); !ok || got != want {
			t.Errorf("%q = %+v, %v; want %+v", line, got, ok, want)
		}
	}
	for _, line := range []string{"", "[警告] 人声分离已改用 CPU", "[stage] Dancing", "转写 3/12", "[stage] Transcribing 13/12", "[stage] Transcribing 0/12"} {
		if step, ok := parseOneAsrStage(line); ok {
			t.Errorf("%q read as %+v", line, step)
		}
	}
	if got := recognitionStepLabel(recognitionStep{Stage: "transcribe", Done: 3, Total: 12}); got != "转写 3/12" {
		t.Fatal(got)
	}
	inLocale(t, "en", func() {
		if got := recognitionStepLabel(recognitionStep{Stage: "align", Done: 2, Total: 5}); got != "Aligning timings 2/5" {
			t.Fatal(got)
		}
	})
}

// Recognition shows its stage and, for chunked stages, a determinate
// bar where the player was.
func TestRecognitionShowsItsStage(t *testing.T) {
	a := withStyleCatalog(t)
	a.busy, a.recognitionRunning, a.recognitionStarted = true, true, time.Now()
	a.recognitionStep = recognitionStep{Stage: "transcribe", Done: 3, Total: 12}
	tt := settled(a.view, 1600, 1000)
	if !tt.HasText("转写 3/12") {
		t.Fatal(tt.Texts())
	}
	area, _ := tt.Find("字幕识别进度")
	editor, _ := tt.Find("字幕编辑")
	if area.Y >= editor.Y {
		t.Fatal("progress outside the player", area, editor)
	}
}

// An error shows in the status bar for a while, then the status returns.
func TestErrorsLeaveTheStatusBar(t *testing.T) {
	a := nativeForTest(t)
	a.status = "已完成"
	a.errorText = "出错了"
	tt := settled(a.view, 1400, 900)
	if !tt.HasText("出错了") {
		t.Fatal(tt.Texts())
	}
	tt.Advance(errorDuration - time.Second)
	if !tt.HasText("出错了") {
		t.Fatal("the error left too early")
	}
	tt.Advance(2 * time.Second)
	if tt.HasText("出错了") || !tt.HasText("已完成") || a.errorText != "" {
		t.Fatal("the error stayed", tt.Texts())
	}
	a.errorText = "出错了"
	tt.Frame()
	if !tt.HasText("出错了") {
		t.Fatal("the same error, again, did not show")
	}
}

// The speed menu offers slow to fast speeds beside the transport, and the
// chosen one applies to the page and to the next videos.
func TestPlaybackSpeed(t *testing.T) {
	a := nativeForTest(t)
	a.mediaURL = "http://127.0.0.1/video"
	tt := settled(a.view, 1400, 900)
	if err := tt.Click("播放速度"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(tt.Menu(), " "); got != "0.5× 0.75× 1× 1.25× 1.5× 2×" {
		t.Fatal(got)
	}
	if err := tt.ChooseMenuItem("1.5×"); err != nil {
		t.Fatal(err)
	}
	if a.playbackRate() != 1.5 || a.rateScript() != "video.defaultPlaybackRate=1.5;video.playbackRate=1.5;video.loop=false;" || !tt.HasText("1.5×") {
		t.Fatal(a.rateScript(), tt.Texts())
	}
	speed, _ := tt.Find("播放速度")
	forward, _ := tt.Find("前进 15 秒")
	if speed.X <= forward.X || speed.Y+speed.H/2-(forward.Y+forward.H/2) > 1 || forward.Y+forward.H/2-(speed.Y+speed.H/2) > 1 {
		t.Fatal("speed not beside the transport", speed, forward)
	}
}

// After the layout update, both lower dividers and the player divider keep
// the sizes the user chooses, saved once the drag stops.
func TestPaneSizesAreRemembered(t *testing.T) {
	a := withStyleCatalog(t)
	tt := settled(a.view, 1800, 1000)
	a.styleSplit, a.editorSplit, a.splitX = 25, 60, 55
	tt.Frame()
	tt.Advance(time.Second)
	prefs, err := settings.Load(a.dataRoot + "/settings.json")
	if err != nil || prefs.StyleWidth != 25 || prefs.EditorWidth != 60 || prefs.PlayerWidth != 55 {
		t.Fatal(prefs.StyleWidth, prefs.EditorWidth, prefs.PlayerWidth, err)
	}
}

// The cue being edited is named on the editor's first line, in its middle,
// not in the header.
func TestEditorNamesTheCueOnItsFirstLine(t *testing.T) {
	a := withStyleCatalog(t)
	tt := settled(a.view, 1600, 1000)
	title, _ := tt.Find("字幕编辑")
	number, ok := tt.Find("序号 1")
	duration, _ := tt.Find("时长")
	editor, _ := tt.Find("字幕编辑面板")
	if !ok || number.Y <= title.Y || number.Y >= duration.Y {
		t.Fatal("cue number not between header and timing", title, number, duration)
	}
	if delta := number.X + number.W/2 - editor.X - editor.W/2; delta < -1 || delta > 1 {
		t.Fatal("cue number not centered", number, editor)
	}
}

// While a translation runs, the list shows every subtitle with its
// translation as the model writes it, and changes no subtitle.
func TestTranslationShowsEachSubtitleAsItIsWritten(t *testing.T) {
	a := withStyleCatalog(t)
	a.busy, a.translationRunning = true, true
	a.subtitleTab = 1
	a.translationLive = newTranslationLive()
	units := []subtitle.Cue{{ID: "u1", Text: "First line", StartMs: 0, EndMs: 2000}, {ID: "u2", Text: "Second line", StartMs: 2000, EndMs: 4000}}
	a.translationLive.apply(translator.Event{Stage: "translate", Total: 2, Units: units})
	a.translationLive.apply(translator.Event{Stage: "translate", Done: 1, Total: 2, Requests: 1, Input: 1200, Output: 40, Updates: []translator.UnitText{{Index: 0, Text: "第一行", Final: true}, {Index: 1, Text: "第二"}}})
	tt := settled(a.view, 1600, 1000)
	for _, text := range []string{"翻译 1 / 2", "第一行", "第二", "First line", "1 次请求 · 输入 1,200 · 输出 40 tokens"} {
		if !tt.HasText(text) {
			t.Fatal(text, tt.Texts())
		}
	}
	if a.snapshot.Document.Cues[0].Text != "source" {
		t.Fatal("a subtitle changed before the translation was checked")
	}
}

// A language is its flag, name and code, in the choice and in each option.
func TestLanguagesShowFlagNameAndCode(t *testing.T) {
	a := withStyleCatalog(t)
	tt := settled(a.workflowPanel, 800, 800)
	if !tt.HasText("中国（普通话）-ZH") {
		t.Fatal(tt.Texts())
	}
	if err := tt.Click("源语言"); err != nil {
		t.Fatal(err)
	}
	for _, o := range sourceLanguages() {
		if !tt.HasText(o.Label) || languageFlags()[o.Country] == nil {
			t.Fatal("option without its flag or label", o)
		}
	}
	if err := tt.Click("日本-JA"); err != nil {
		t.Fatal(err)
	}
	if a.settings.Language != "ja" {
		t.Fatal(a.settings.Language)
	}
}

// OneSub's default style draws the outline only; choosing the box then
// shows a box.
func TestDefaultStyleHasNoShadow(t *testing.T) {
	a := withStyleCatalog(t)
	if a.styleCatalog.DefaultStyle.Shadow != 0 || a.styleCatalog.DefaultStyle.BackOpacity != 0 {
		t.Fatal(a.styleCatalog.DefaultStyle)
	}
	tt := settled(a.view, 1600, 1000)
	panel, _ := tt.Find("字幕样式面板")
	box, _ := tt.Find("背景框")
	// The narrower style pane stacks sections; reach the border controls
	// through its native scroll area instead of clicking off-screen text.
	tt.Scroll(panel.X+panel.W/2, panel.Y+panel.H/2, 0, max(0, box.Y+box.H-panel.Y-panel.H+16))
	if err := tt.Click("背景框"); err != nil {
		t.Fatal(err)
	}
	if a.currentStyle.BorderStyle != 3 || a.currentStyle.BackOpacity != boxOpacity {
		t.Fatal(a.currentStyle)
	}
}

// Line-end punctuation goes, in both widths, except the marks kept; closing
// quotes and brackets stay, and so does a line of punctuation only.
func TestStripTrailingPunctuation(t *testing.T) {
	for in, want := range map[string]string{
		"大家好，欢迎使用。":   "大家好，欢迎使用",
		"真的吗？":        "真的吗？",
		"Really?":     "Really?",
		"Done.\n第二行，": "Done\n第二行",
		"他说“好的。”":     "他说“好的。”",
		"（注释）":        "（注释）",
		"Wait...":     "Wait",
		"……":          "……",
		"不要！ ":        "不要",
	} {
		if got := stripTrailingPunctuation(in, "?"); got != want {
			t.Errorf("%q = %q, want %q", in, got, want)
		}
	}
	if got := stripTrailingPunctuation("太好了！", "!"); got != "太好了！" {
		t.Error("full-width mark of a kept ASCII mark removed", got)
	}
}

// Loop playback is off until chosen, beside the transport, and applies to
// the page.
func TestLoopPlayback(t *testing.T) {
	a := nativeForTest(t)
	a.mediaURL = "http://127.0.0.1/video"
	tt := settled(a.view, 1400, 900)
	if a.loop {
		t.Fatal("loop on by default")
	}
	if err := tt.Click("循环播放"); err != nil {
		t.Fatal(err)
	}
	if !a.loop || !strings.HasSuffix(a.rateScript(), "video.loop=true;") {
		t.Fatal(a.rateScript())
	}
}
