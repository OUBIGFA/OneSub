package main

import (
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

// Lobe and the native table/split substrate must share one palette. The kit
// migration must not replace OneSub's blue emphasis with black/white selection.
func TestLobeThemeOwnsWorkbench(t *testing.T) {
	var native ui.Theme
	var tokens *theme.Tokens
	tt := ui.NewTester(func(c *ui.Context) {
		applyStudioTheme(c)
		native, tokens = *c.Theme(), theme.TokensOf(c)
	}, 200, 100)
	for _, dark := range []bool{false, true} {
		tt.SetDark(dark)
		tt.Frame()
		if tokens.Dark != dark || tokens.BgLayout != native.Background || tokens.Primary != native.Accent || tokens.Text != native.Text || tokens.FontSize != native.FontSize {
			t.Fatalf("Lobe and native themes disagree: dark=%v", dark)
		}
		if native.Accent != ui.Hex("#2563eb") || native.AccentText != ui.Hex("#ffffff") {
			t.Fatalf("blue emphasis lost: dark=%v", dark)
		}
	}
}

// A theme switch must not leave the old light surfaces under the new dark
// text. Color transitions also need to settle on the very first dark frame.
func TestLobeDarkControlsSwitchTogether(t *testing.T) {
	inLocale(t, "zh-Hans", func() {
		a := nativeForTest(t)
		a.settingsOpen, a.settingsTab = true, 2
		tt := settled(a.view, 1400, 800)
		tt.SetDark(true)
		tt.FrameOnce()
		check := func(phase string) {
			t.Helper()
			for _, label := range []string{locales.ActionClose(), locales.TranslationSearchModels(), locales.TranslationMaxInput(), locales.TranslationMaxOutput()} {
				r, ok := tt.Find(label)
				if !ok {
					t.Fatalf("%s: missing %s", phase, label)
				}
				p := tt.Image().RGBAAt(int(r.X+r.W-8), int(r.Y+r.H/2))
				if int(p.R)+int(p.G)+int(p.B) > 3*150 {
					t.Errorf("%s: %s retains a light surface: %v", phase, label, p)
				}
			}
			captureRedesign(t, tt, "lobe-dark-"+phase)
		}
		check("first")
		tt.Advance(time.Second)
		check("settled")
	})
}
