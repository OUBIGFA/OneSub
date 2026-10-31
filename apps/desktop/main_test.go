package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

// testLocale is the language the package's messages are said in. Tests pin
// it, as the app's own choice follows the system they run on.
var testLocale = "zh-Hans"

func TestMain(m *testing.M) {
	i18n.SetLocaleFunc(func() string { return testLocale })
	os.Exit(m.Run())
}

// inLocale runs fn with the messages in another language.
func inLocale(t *testing.T, tag string, fn func()) {
	t.Helper()
	previous := testLocale
	testLocale = tag
	defer func() { testLocale = previous }()
	fn()
}

// settled is a tester whose view ran the frames that apply what one frame
// measures, as the style sections' widths, as the app's next frames do.
func settled(view func(*ui.Context), width, height int) *ui.Tester {
	tt := ui.NewTester(view, width, height)
	tt.Frame()
	tt.Frame()
	return tt
}

func nativeForTest(t *testing.T) *NativeApp {
	t.Helper()
	entries, err := iconFiles.ReadDir("icons")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := iconFiles.ReadFile("icons/" + e.Name())
		if err != nil {
			t.Fatal(err)
		}
		icons[strings.TrimSuffix(e.Name(), ".svg")] = ui.MustParseSVG(data)
	}
	prefs := settings.Default()
	return &NativeApp{dataRoot: t.TempDir(), settings: prefs, splitX: float32(prefs.PlayerWidth), splitY: float32(prefs.TopHeight), editorSplit: float32(prefs.EditorWidth), styleSplit: float32(prefs.StyleWidth), selected: -1, paused: true, activeCue: -1, followRow: -1, locateCue: -1, modelStates: map[string]string{}, exportFormat: "srt"}
}

// storeApp opens a real project store with the given cues as the workbench.
func storeApp(t *testing.T, cues ...subtitle.Cue) *NativeApp {
	t.Helper()
	a := nativeForTest(t)
	s, err := project.Create(filepath.Join(t.TempDir(), "edit.onesub"), "", "", subtitle.Document{Format: "srt", Cues: cues})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	state, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	a.store = s
	a.selected = 0
	a.setSnapshot(state)
	a.markSaved(0, "original.srt")
	a.markSaved(1, "")
	a.loadDraft()
	return a
}

func TestNativeWorkbenchUsesDirectActionsAndSettings(t *testing.T) {
	a := nativeForTest(t)
	tt := settled(a.view, 1280, 800)
	for _, label := range []string{"添加文件", "清空工作台", "识别与翻译", "开始识别字幕", "合成视频", "设置"} {
		if !tt.HasText(label) {
			t.Fatal("missing action", label)
		}
	}
	for _, label := range []string{"准备", "从视频或字幕开始", "创建工程并开始", "打开工程"} {
		if tt.HasText(label) {
			t.Fatal("unexpected preparation flow", label)
		}
	}
	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	if !a.settingsOpen || !tt.HasText("关闭窗口") || !tt.HasText("最小化到托盘") || !tt.HasText("语言") || !tt.HasText("跟随系统") {
		t.Fatal(tt.Texts())
	}
	if err := tt.Click("识别与模型"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("语音识别") || !tt.HasText("时间轴对齐") || !tt.HasText("热词召回") || !tt.HasText("人声分离") {
		t.Fatal(tt.Texts())
	}
}

// Recognition and translation are a panel beside the style, each a tab that
// ends in the button starting it; the list's tabs stay centred.
func TestActionsMovedBesideTheirContent(t *testing.T) {
	a := storeApp(t, subtitle.Cue{ID: "c1", Text: "原句", OriginalText: "原句", StartMs: 0, EndMs: 1000})
	tt := settled(a.view, 1600, 1000)
	style, _ := tt.Find("字幕样式")
	panel, ok := tt.Find("识别与翻译")
	if !ok || panel.X <= style.X || panel.Y > style.Y+20 {
		t.Fatal("recognition and translation not right of the style", style, panel)
	}
	if !tt.HasText("开始识别字幕") || !tt.HasText("中国（普通话）-ZH") || !tt.HasText("移除句末标点") {
		t.Fatal(tt.Texts())
	}
	if err := tt.Click("AI 翻译"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("开始翻译字幕") || !tt.HasText("目标语言") {
		t.Fatal(tt.Texts())
	}
	if err := tt.Click("开始翻译字幕"); err != nil {
		t.Fatal(err)
	}
	// No service yet: the button opens its settings.
	if !a.settingsOpen || a.settingsTab != 2 {
		t.Fatal("translation without a service did not open its settings")
	}
	a.settingsOpen = false
	list, _ := tt.Find("字幕语言")
	source, _ := tt.Find("原始字幕")
	translation, _ := tt.Find("重建字幕")
	panelCentre := list.X + list.W/2
	tabsCentre := (source.X + translation.X + translation.W) / 2
	if d := tabsCentre - panelCentre; d > 30 || d < -30 {
		t.Fatal("tabs not centred", tabsCentre, panelCentre)
	}
}

func TestEditingIsLiveAndCommitsAfterAPause(t *testing.T) {
	a := storeApp(t, subtitle.Cue{ID: "c1", Text: "原文", OriginalText: "原文", StartMs: 0, EndMs: 1000}, subtitle.Cue{ID: "c2", Text: "第二句", OriginalText: "第二句", StartMs: 1000, EndMs: 2000})
	tt := settled(a.view, 1400, 900)
	for _, label := range []string{"字幕文字", "锁定文字", "播放当前句", "第 1 条"} {
		if label != "字幕文字" && tt.HasText(label) {
			t.Fatal("removed control still shown", label)
		}
	}
	if !tt.HasText("序号 1") || !tt.HasText("#") || tt.HasText("序号\n") {
		t.Fatal(tt.Texts())
	}
	if err := tt.Click("字幕文字"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("中文修改")
	if !tt.HasText("中文修改") || !a.editPending || !tt.HasText("未保存") {
		t.Fatal("draft not shown live", tt.Texts())
	}
	before, _ := a.store.Snapshot()
	if before.Revision != 0 {
		t.Fatal("committed before the pause")
	}
	tt.Advance(time.Second)
	tt.Frame()
	saved, err := a.store.Snapshot()
	if err != nil || saved.Document.Cues[0].Text != "中文修改" || saved.Document.Cues[0].TextLocked || saved.Revision != 1 {
		t.Fatal(saved.Document.Cues[0], err)
	}
	a.undo(false)
	if a.snapshot.Document.Cues[0].Text != "原文" || a.trackUnsaved(0) {
		t.Fatal("undo did not return to the saved file state", a.snapshot.Document.Cues[0])
	}
	a.undo(true)
	if a.snapshot.Document.Cues[0].Text != "中文修改" || !a.trackUnsaved(0) {
		t.Fatal("redo lost the edit")
	}
}

// Leaving a cue commits the draft; an invalid draft is restored instead.
func TestLeavingCueCommitsOrRestores(t *testing.T) {
	a := storeApp(t, subtitle.Cue{ID: "a", Text: "原文", OriginalText: "原文", StartMs: 100, EndMs: 2000}, subtitle.Cue{ID: "b", Text: "第二句", StartMs: 2000, EndMs: 3000})
	a.draft = "修改"
	a.changed(time.Now())
	a.selectSubtitle(1, "none")
	if a.selected != 1 || a.snapshot.Document.Cues[0].Text != "修改" || a.savePromptOpen {
		t.Fatal("switching cues did not commit", a.snapshot.Document.Cues[0])
	}
	a.draft = ""
	a.changed(time.Now())
	a.selectSubtitle(0, "none")
	if a.selected != 0 || a.snapshot.Document.Cues[1].Text != "第二句" || a.errorText == "" {
		t.Fatal("empty draft was not restored", a.snapshot.Document.Cues[1], a.errorText)
	}
}

// While playing, the list keeps the cue at the playhead in its middle. A
// hand scroll stops it moving the list, though the cue is still chosen;
// the locate button brings the cue back to the middle and follows again.
func TestPlaybackFollowsUntilTheUserScrolls(t *testing.T) {
	cues := []subtitle.Cue{}
	for i := 0; i < 80; i++ {
		cues = append(cues, subtitle.Cue{ID: "c" + strings.Repeat("x", i), Text: "line", StartMs: int64(i * 1000), EndMs: int64(i*1000 + 900)})
	}
	a := storeApp(t, cues...)
	tt := settled(a.view, 1400, 900)
	centered := func(row int) bool {
		first, last := a.list.Visible()
		return first < row && row < last && row-first >= (last-first)/2-2 && last-row >= (last-first)/2-2
	}
	a.paused = false
	a.position = 70.2
	tt.Frame()
	tt.Frame()
	if a.selected != 70 || !centered(70) {
		first, last := a.list.Visible()
		t.Fatal("did not follow to the middle", a.selected, first, last)
	}
	list, _ := tt.Find("字幕列表")
	tt.Scroll(list.X+list.W/2, list.Y+list.H/2, 0, -600)
	tt.Frame()
	if !a.followPaused {
		t.Fatal("a hand scroll did not stop following")
	}
	before, _ := a.list.Visible()
	a.position = 75.5
	tt.Frame()
	tt.Frame()
	if after, _ := a.list.Visible(); a.selected != 75 || after != before {
		t.Fatal("the list moved back while the user was reading", a.selected, before, after)
	}
	if err := tt.Click("回到当前字幕"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.followPaused || !centered(75) {
		t.Fatal("locate did not bring the cue back", a.followPaused)
	}
	a.position = 79.5
	tt.Frame()
	tt.Frame()
	if first, last := a.list.Visible(); a.selected != 79 || first > 79 || last < 79 {
		t.Fatal("did not follow again after locating", a.selected, first, last)
	}
}

// The locate button finds the cue at the playhead while paused too, and the
// cue before the playhead between cues.
func TestLocateFindsTheCueAtThePlayhead(t *testing.T) {
	a := storeApp(t, subtitle.Cue{ID: "a", Text: "一", StartMs: 0, EndMs: 1000}, subtitle.Cue{ID: "b", Text: "二", StartMs: 2000, EndMs: 3000}, subtitle.Cue{ID: "c", Text: "三", StartMs: 4000, EndMs: 5000})
	tt := settled(a.view, 1400, 900)
	a.position = 3.5
	tt.Frame()
	if err := tt.Click("回到当前字幕"); err != nil {
		t.Fatal(err)
	}
	if a.selected != 1 {
		t.Fatal("chose", a.selected)
	}
}

// Multi-line subtitles are shown whole: their rows grow to the text.
func TestListRowsGrowForEveryLine(t *testing.T) {
	a := storeApp(t, subtitle.Cue{ID: "one", Text: "单行", StartMs: 0, EndMs: 1000}, subtitle.Cue{ID: "two", Text: "第一行\n第二行也完整显示", StartMs: 1000, EndMs: 2000})
	tt := settled(a.view, 1400, 900)
	first, ok1 := tt.Find("单行")
	second, ok2 := tt.Find("第一行\n第二行也完整显示")
	if !ok1 || !ok2 || second.H < first.H*1.6 {
		t.Fatal("second line clipped", first, second, tt.Texts())
	}
}

func TestEnglishInterface(t *testing.T) {
	inLocale(t, "en", func() {
		a := storeApp(t, subtitle.Cue{ID: "c1", Text: "Hello", OriginalText: "Hello", StartMs: 0, EndMs: 1000})
		tt := settled(a.view, 1400, 900)
		for _, label := range []string{"Add files", "Clear workbench", "Export subtitles", "Transcribe & translate", "Transcribe subtitles", "Export video", "Original subtitles", "Rebuilt subtitles", "Subtitle editor", "No. 1", "Save as", "Subtitle style"} {
			if !tt.HasText(label) {
				t.Fatal("missing English text", label, tt.Texts())
			}
		}
		for _, text := range tt.Texts() {
			for _, r := range text {
				if r >= 0x4e00 && r <= 0x9fff && text != "Hello" {
					t.Fatal("Chinese text in the English interface", text)
				}
			}
		}
	})
}
