package main

import (
	"bytes"
	"fmt"
	"image/png"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/media"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/egoist/mygo/ui"
)

func TestDialogBusyStateDoesNotReflowWorkbench(t *testing.T) {
	a := nativeForTest(t)
	tt := settled(a.view, 1400, 900)
	before, _ := tt.Find("视频播放区")
	footer, _ := tt.Find("状态栏")
	a.busy = true
	a.cancel = func() {}
	a.status = "打开文件"
	tt = settled(a.view, 1400, 900)
	during, _ := tt.Find("视频播放区")
	busyFooter, _ := tt.Find("状态栏")
	if before != during || footer != busyFooter {
		t.Fatal("busy controls resized workspace", before, during, footer, busyFooter)
	}
	a.choosingFile = true
	tt = settled(a.view, 1400, 900)
	choosing, _ := tt.Find("视频播放区")
	if choosing != before || tt.HasText("取消任务") {
		t.Fatal("file chooser exposed task controls or resized panes")
	}
}

func TestNumberColumnFitsEveryDigitAtUIScales(t *testing.T) {
	for _, size := range []float32{15, 22.5, 30} {
		last := float32(0)
		for _, maxNumber := range []int{9, 99, 999, 9999, 10000, 100000} {
			var col ui.TableColumn
			tt := settled(func(c *ui.Context) {
				theme := *c.Theme()
				theme.FontSize = size
				c.SetTheme(&theme)
				col = numberColumn(c, maxNumber)
				for digit := '0'; digit <= '9'; digit++ {
					width, _ := c.MeasureText(0, ui.Span{Text: strings.Repeat(string(digit), len(fmt.Sprint(maxNumber)))})
					if col.Width < width+2*theme.Space(2.5) {
						t.Errorf("clipped %d at %g", maxNumber, size)
					}
				}
				ui.Table(c, nil, []ui.TableColumn{col, {Title: "字幕"}}, 1, func(_, column int) {
					if column == 0 {
						ui.Text(c, fmt.Sprint(maxNumber)).SingleLine()
					}
				}).Height(100)
			}, 600, 200)
			if !tt.HasText(fmt.Sprint(maxNumber)) || col.Width < last || !col.Fixed {
				t.Fatal(maxNumber, col)
			}
			last = col.Width
		}
	}
}

func TestSettingsHeightIsStableAcrossTabs(t *testing.T) {
	a := nativeForTest(t)
	a.settingsOpen = true
	tt := settled(a.view, 1400, 900)
	first, ok := tt.Find("设置内容")
	if !ok {
		t.Fatal(tt.Texts())
	}
	for _, tab := range []string{"识别与模型", "翻译服务", "常规"} {
		if err := tt.Click(tab); err != nil {
			t.Fatal(err)
		}
		got, ok := tt.Find("设置内容")
		if !ok || got.H != first.H || got.Y != first.Y {
			t.Fatal(tab, first, got)
		}
	}
}

func TestPlayerIconsFitAndStayCentered(t *testing.T) {
	for _, size := range []float32{15, 22.5, 30} {
		tt := settled(func(c *ui.Context) {
			theme := *c.Theme()
			theme.FontSize = size
			c.SetTheme(&theme)
			toolIcon(c, "播放", "play")
		}, 300, 120)
		b, _ := tt.Find("播放")
		i, _ := tt.Find("播放图标")
		if math.Abs(float64(b.X+b.W/2-i.X-i.W/2)) > 1 || math.Abs(float64(b.Y+b.H/2-i.Y-i.H/2)) > 1 || i.X < b.X || i.X+i.W > b.X+b.W {
			t.Fatal("icon not centered inside native button", size, b, i)
		}
	}
}

func TestAllStyleFieldsSurviveSavingAndReload(t *testing.T) {
	a := withStyleCatalog(t)
	a.currentStyle.Bold = true
	a.currentStyle.Italic = true
	a.currentStyle.MarginL = 35
	a.currentStyle.MarginR = 42
	a.observeStyle(time.Now())
	a.saveSettings()
	prefs, err := settings.Load(filepath.Join(a.dataRoot, "settings.json"))
	if err != nil || prefs.LastStyle == nil || !reflect.DeepEqual(*prefs.LastStyle, a.currentStyle) {
		t.Fatal(prefs, err)
	}
}

func TestTrayLogoAtPhysicalSizes(t *testing.T) {
	for _, scale := range []float64{1, 1.25, 1.5, 2, 3} {
		data, err := trayLogo(scale)
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		want := int(math.Round(16 * scale))
		if img.Bounds().Dx() != want || img.Bounds().Dy() != want {
			t.Fatal(img.Bounds())
		}
		softEdge := false
		for y := 0; y < want; y++ {
			for x := 0; x < want; x++ {
				_, _, _, a := img.At(x, y).RGBA()
				if a > 0 && a < 65535 {
					softEdge = true
				}
			}
		}
		if !softEdge {
			t.Fatal("alpha antialiasing lost", scale)
		}
	}
}

func TestSubtitlePlaybackActionsAndBoundaries(t *testing.T) {
	for action, tail := range map[string]string{"none": "", "seek": "video.currentTime=1.250000", "seek-pause": "video.currentTime=1.250000;video.pause()", "seek-play": "video.currentTime=1.250000;video.play()"} {
		if got := subtitleSeekScript(action, 1250); got != tail {
			t.Fatal(got, tail)
		}
	}
	cues := []subtitle.Cue{{StartMs: 100, EndMs: 1000}, {StartMs: 1200, EndMs: 2000}}
	for seconds, want := range map[float64]int{0: -1, .1: 0, 1: -1, 1.2: 1, 2: -1} {
		if got := cueAt(cues, seconds); got != want {
			t.Fatal(seconds, got, want)
		}
	}
}

func TestComposeNamesTermsAndPlayerProgress(t *testing.T) {
	if got := composeFilename(filepath.Join("source", "课程.v2.mp4"), "mkv", false); got != "课程.v2-OneSub.mkv" {
		t.Fatal(got)
	}
	if got := composeFilename("lesson.mp4", "mp4", true); got != "lesson-OneSub-preview.mp4" {
		t.Fatal(got)
	}
	a := nativeForTest(t)
	a.composeOpen = true
	tt := settled(a.view, 1400, 900)
	if !tt.HasText("硬字幕（烧录） · MP4") || !tt.HasText("CRF 20") || a.settings.ComposeMode != "hard-mp4" {
		t.Fatal(tt.Texts())
	}
	// The lossless level is offered, explained, and remembered by ID.
	a.settings.ComposeQuality = "lossless"
	a.settings.ComposeMode = "hard-mkv"
	a.saveSettings()
	prefs, err := settings.Load(filepath.Join(a.dataRoot, "settings.json"))
	if err != nil || prefs.ComposeQuality != "lossless" || prefs.ComposeMode != "hard-mkv" {
		t.Fatal(prefs, err)
	}
	tt = settled(a.view, 1400, 900)
	if !tt.HasText("原画质（无损）") || !tt.HasText("硬字幕（烧录） · MKV") {
		t.Fatal(tt.Texts())
	}
	a.composeOpen = false
	a.composeRunning = true
	a.composeProgress = media.ComposeProgress{Stage: "视频编码", Fraction: .42}
	tt = settled(a.view, 1400, 900)
	if !tt.HasText("正在合成视频") || !tt.HasText("42.0%") || !tt.HasText("取消合成") {
		t.Fatal(tt.Texts())
	}
	progress, _ := tt.Find("合成进度")
	editor, _ := tt.Find("字幕编辑")
	if progress.Y >= editor.Y {
		t.Fatal("progress outside player", progress, editor)
	}
}

// Volume and mute apply to the page and are remembered like other
// preferences; the volume starts at half, and a silent slider unmutes to it.
func TestVolumeAndMuteAreRemembered(t *testing.T) {
	a := nativeForTest(t)
	a.setVolume(35, false, time.Now())
	a.toggleMute(time.Now())
	if !a.settings.Muted || a.volumeScript() != "video.volume=0.000;video.muted=true;" {
		t.Fatal(a.volumeScript())
	}
	a.saveSettings()
	prefs, err := settings.Load(filepath.Join(a.dataRoot, "settings.json"))
	if err != nil || prefs.Volume != 35 || !prefs.Muted {
		t.Fatal(prefs, err)
	}
	if settings.Default().Volume != 50 {
		t.Fatal("the volume does not start at half")
	}
	a.setVolume(0, false, time.Now())
	a.toggleMute(time.Now())
	if a.settings.Volume != 50 || a.settings.Muted {
		t.Fatal("silent slider did not restore sound", a.settings)
	}
	tt := settled(a.view, 1400, 900)
	if _, ok := tt.Find("静音"); !ok {
		t.Fatal(tt.Texts())
	}
	if _, ok := tt.Find("音量"); !ok {
		t.Fatal("no volume slider")
	}
}

// Labels are said again in the chosen language, while the stored IDs keep
// the user's choices.
func TestChoicesSurviveALanguageChange(t *testing.T) {
	a := nativeForTest(t)
	a.composeOpen = true
	inLocale(t, "en", func() {
		tt := settled(a.view, 1400, 900)
		if !tt.HasText("Hard subtitles (burn-in) · MP4") || !tt.HasText("Encoding quality") {
			t.Fatal(tt.Texts())
		}
	})
	tt := settled(a.view, 1400, 900)
	if !tt.HasText("硬字幕（烧录） · MP4") || a.settings.ComposeMode != "hard-mp4" {
		t.Fatal(tt.Texts())
	}
}
