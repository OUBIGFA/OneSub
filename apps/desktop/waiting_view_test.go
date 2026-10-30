package main

import (
	"bytes"
	"image"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

func capturePixels(tt *ui.Tester) []byte { return append([]byte(nil), tt.Image().Pix...) }

func TestBlocksBackdropAnimatesThenStopsBeforeFading(t *testing.T) {
	finished := false
	tt := settled(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { blocksBackdrop(c, finished) })
	}, 800, 480)
	first := capturePixels(tt)
	tt.Advance(200 * time.Millisecond)
	if bytes.Equal(first, capturePixels(tt)) {
		t.Fatal("Blocks is a static background")
	}
	finished = true
	tt.Frame()
	first = capturePixels(tt)
	tt.Advance(blocksStop / 2)
	if bytes.Equal(first, capturePixels(tt)) {
		t.Fatal("blocks stopped abruptly rather than decelerating")
	}
	tt.Advance(blocksStop / 2)
	stopped := capturePixels(tt)
	tt.Advance(blocksFade / 2)
	fading := capturePixels(tt)
	if bytes.Equal(stopped, fading) {
		t.Fatal("blocks did not fade after stopping")
	}
	tt.Advance(blocksFade/2 + 10*time.Millisecond)
	gone := capturePixels(tt)
	if bytes.Equal(gone, fading) {
		t.Fatal("fade did not finish")
	}
	tt.Advance(time.Second)
	if !bytes.Equal(gone, capturePixels(tt)) {
		t.Fatal("invisible background still changes")
	}
}

// Pixel intensity distinguishes stopping motion from prematurely fading it:
// the shape's maximum opacity stays fixed through deceleration, then drops.
func TestBlocksMotionEndsBeforeOpacityFalls(t *testing.T) {
	finished := false
	tt := settled(func(c *ui.Context) {
		ui.Column(c).Fill().Background(ui.RGB(0, 0, 0)).Children(func() {
			theme := *c.Theme()
			theme.Text = ui.RGB(255, 255, 255)
			c.SetTheme(&theme)
			blocksBackdrop(c, finished)
		})
	}, 640, 400)
	maxValue := func(img *image.RGBA) byte {
		var value byte
		for i := 0; i < len(img.Pix); i += 4 {
			value = max(value, img.Pix[i])
		}
		return value
	}
	tt.Advance(200 * time.Millisecond)
	value := maxValue(tt.Image())
	if value == 0 || value > 7 {
		t.Fatal("background is missing or too strong", value)
	}
	finished = true
	tt.Frame()
	tt.Advance(blocksStop - 20*time.Millisecond)
	if maxValue(tt.Image()) != value {
		t.Fatal("opacity faded before movement settled")
	}
	tt.Advance(20*time.Millisecond + blocksFade/2)
	if got := maxValue(tt.Image()); got == 0 || got >= value {
		t.Fatal("blocks are not fading after the motion stopped", got, value)
	}
}

func TestWaitingBackdropDoesNotInterceptCancelAndClearsAfterOutro(t *testing.T) {
	a := nativeForTest(t)
	a.busy = true
	a.status = "Working"
	cancelled := false
	a.cancel = func() { cancelled = true }
	tt := settled(a.playerView, 900, 600)
	if err := tt.Click(locales.ActionCancelTask()); err != nil || !cancelled {
		t.Fatal("backdrop intercepted cancellation", err)
	}
	a.busy = false
	a.status = locales.StatusDone()
	tt.Frame()
	if !a.waiting.shown || !tt.HasText(locales.StatusDone()) {
		t.Fatal("task result/outro missing")
	}
	tt.Advance(blocksStop / 2)
	if !a.waiting.shown {
		t.Fatal("outro disappeared before deceleration")
	}
	tt.Advance(blocksStop/2 + blocksFade + time.Millisecond)
	if a.waiting.shown {
		t.Fatal("waiting scene remained after fade")
	}
	// A follow-up task during the outro must not disappear on the old deadline.
	a.busy = true
	tt.Frame()
	a.busy = false
	tt.Frame()
	tt.Advance(200 * time.Millisecond)
	a.busy = true
	tt.Frame()
	tt.Advance(2 * time.Second)
	if !a.waiting.shown || !a.waiting.ended.IsZero() {
		t.Fatal("new task lost its waiting scene")
	}
}

func TestReducedMotionWaitingStillFinishes(t *testing.T) {
	a := nativeForTest(t)
	a.busy = true
	tt := settled(a.playerView, 900, 600)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
	a.busy = false
	tt.Frame()
	tt.Advance(blocksFade + time.Millisecond)
	if a.waiting.shown {
		t.Fatal("reduced-motion outro remained visible")
	}
}
