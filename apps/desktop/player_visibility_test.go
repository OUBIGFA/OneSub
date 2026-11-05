package main

import (
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo"
)

// A placed WebView alone is not sufficient: the native scene must actually
// leave its pixels transparent before and after task overlays are removed.
func TestPlayerKeepsTransparentViewport(t *testing.T) {
	a := nativeForTest(t)
	a.player = &mygo.WebView{}
	a.mediaURL, a.frameAspect = "test-media", 16.0/9
	tt := settled(a.playerView, 900, 600)
	for _, task := range []string{"initial", "recognize", "done", "compose", "done"} {
		a.recognitionRunning, a.composeRunning = task == "recognize", task == "compose"
		tt.Frame()
		r, ok := tt.Find(locales.A11yPlayerViewport())
		if !ok {
			t.Fatal("missing viewport")
		}
		p := tt.Image().RGBAAt(int(r.X+r.W/2), int(r.Y+r.H/2))
		if task == "initial" || task == "done" {
			if p.A != 0 {
				t.Fatalf("%s: native scene covers video: %v", task, p)
			}
		} else if p.A != 255 {
			t.Fatalf("%s: missing task overlay: %v", task, p)
		}
	}
}
