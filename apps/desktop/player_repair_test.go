package main

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo"
)

func TestPlayerViewportSurvivesTasksAndResizing(t *testing.T) {
	a := nativeForTest(t)
	a.player = &mygo.WebView{}
	a.frameAspect = 16.0 / 9
	tt := settled(a.playerView, 900, 600)
	for _, state := range []string{"empty", "import", "video", "translate", "compose", "recognize", "done"} {
		a.busy = state == "import" || state == "translate" || state == "compose" || state == "recognize"
		a.composeRunning = state == "compose"
		a.recognitionRunning = state == "recognize"
		if state != "empty" {
			a.mediaURL = "test-media"
		}
		for _, size := range [][2]int{{900, 600}, {550, 900}, {1200, 500}, {700, 650}} {
			tt.SetSize(size[0], size[1])
			tt.Frame()
			tt.Frame()
			viewport, ok := tt.Find(locales.A11yPlayerViewport())
			area, _ := tt.Find(locales.A11yPlayerSurface())
			if !ok || viewport.W <= 0 || viewport.H <= 0 || math.Abs(float64(viewport.W/viewport.H)-16.0/9) > .001 {
				t.Fatal("viewport missing or stretched", state, viewport)
			}
			if viewport.X < area.X-1 || viewport.Y < area.Y-1 || viewport.X+viewport.W > area.X+area.W+1 || viewport.Y+viewport.H > area.Y+area.H+1 {
				t.Fatal("viewport out of area", state, viewport, area)
			}
			if tt.WebViewAt(area.X+area.W*.2, area.Y+area.H*.2) != nil {
				t.Fatal("browser, not native import, receives the first drop", state)
			}
		}
	}
}

func TestParameterContentCannotBlockDividerDrag(t *testing.T) {
	a := withStyleCatalog(t)
	tt := settled(a.view, 1800, 1000)
	before, _ := tt.Find(locales.A11yStylePanel())
	x, y := float32(600), before.Y-.5
	tt.Press(x, y)
	tt.Move(x, y+180)
	tt.Release(x, y+180)
	tt.Frame()
	tt.Advance(time.Second)
	after, _ := tt.Find(locales.A11yStylePanel())
	if after.H >= before.H-150 {
		t.Fatal("natural content height blocked drag", before, after)
	}
	prefs, err := settings.Load(filepath.Join(a.dataRoot, "settings.json"))
	if err != nil || prefs.TopHeight != float64(a.splitY) {
		t.Fatal("manual size not saved", prefs.TopHeight, a.splitY, err)
	}
}

func TestMuteShowsZeroAndRestoresVolume(t *testing.T) {
	a := nativeForTest(t)
	a.settings.Volume = 37
	tt := settled(a.playerView, 900, 600)
	if err := tt.Click(locales.PlayerMute()); err != nil {
		t.Fatal(err)
	}
	if a.audibleVolume() != 0 || a.volumeScript() != "video.volume=0.000;video.muted=true;" {
		t.Fatal("mute did not zero actual volume", a.volumeScript())
	}
	a.saveSettings()
	prefs, err := settings.Load(filepath.Join(a.dataRoot, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	a.settings = prefs
	if err := tt.Click(locales.PlayerUnmute()); err != nil {
		t.Fatal(err)
	}
	if a.audibleVolume() != 37 || a.settings.Muted {
		t.Fatal("unmute lost previous volume")
	}
}

func TestSuccessfulRetryClearsOnlyItsError(t *testing.T) {
	a := nativeForTest(t)
	root := a.dataRoot
	a.dataRoot = filepath.Join(root, "blocked")
	if err := os.WriteFile(a.dataRoot, []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	a.saveSettings()
	if a.errorText == "" {
		t.Fatal("failure not reported")
	}
	if err := os.Remove(a.dataRoot); err != nil {
		t.Fatal(err)
	}
	a.saveSettings()
	if a.errorText != "" {
		t.Fatal("successful save left failure", a.errorText)
	}
	a.showError("preview", "preview failed")
	a.saveSettings()
	if a.errorText == "" {
		t.Fatal("unrelated autosave hid preview failure")
	}
	a.clearError("preview")
	if a.errorText != "" {
		t.Fatal("successful retry not cleared")
	}
}

func TestTranslationProgressShowsRealFractionAndStopsOnFailure(t *testing.T) {
	a := nativeForTest(t)
	a.translationLive = newTranslationLive()
	l := a.translationLive
	l.stage = "translate"
	l.done = 3
	l.total = 10
	if l.progress() != .3 {
		t.Fatal(l.progress())
	}
	tt := settled(a.translationProgressView, 700, 500)
	if !tt.HasText(locales.Percent(30)) {
		t.Fatal("real percentage missing", tt.Texts())
	}
	l.stage = "analyze"
	l.stopped = "network error"
	tt.Frame()
	if l.progress() != 0 {
		t.Fatal("interrupted task still animates indefinite progress")
	}
	l.stopped = ""
	if l.progress() != -1 {
		t.Fatal("unknown duration reported a made-up percentage")
	}
}
