package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
	lobeTheme "github.com/lobehub/lobe-mygo/pkg/theme"
)

func TestLobePaletteKeepsBlueAndTextScaling(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, scale := range []float32{1, 1.5, 2} {
			base, tokens := *ui.LightTheme(), lobeTheme.Light()
			if dark {
				base, tokens = *ui.DarkTheme(), lobeTheme.Dark()
			}
			base.Font, base.FontSize = "system-test-font", 14*scale
			got, _ := studioTheme(&base, false)
			if got.Background != tokens.BgLayout || got.Surface != tokens.FillTertiary || got.Border != tokens.FillBorder() || got.Radius != tokens.BorderRadius || got.Text != tokens.Text {
				t.Fatalf("not the Lobe palette: dark=%v %+v", dark, got)
			}
			if got.Accent != ui.Hex("#2563eb") || got.AccentHover != ui.Hex("#1d4ed8") || got.AccentPressed != ui.Hex("#1e40af") || got.AccentText != ui.Hex("#ffffff") {
				t.Fatalf("blue selection was lost: dark=%v", dark)
			}
			if got.Font != base.Font || got.FontSize != bodySize*scale || got.Dark != dark {
				t.Fatal("theme lost native font/scale/appearance", got)
			}
			high, _ := studioTheme(&base, true)
			if high.Border != high.Text || high.TextMuted != high.Text || high.Focus != high.Accent {
				t.Fatal("high contrast adaptation was lost")
			}
		}
	}
}

func TestLobeSurfaceRoles(t *testing.T) {
	var look studio
	tt := settled(func(c *ui.Context) {
		applyStudioTheme(c)
		look = studioColors(c)
		ui.Column(c).Fill().Background(look.panel)
	}, 300, 200)
	for _, dark := range []bool{false, true} {
		tt.SetDark(dark)
		tt.Frame()
		tokens := lobeTheme.Light()
		hairline := tokens.BorderSecondary
		if dark {
			tokens = lobeTheme.Dark()
			hairline = tokens.Border
		}
		if look.chrome != tokens.BgLayout || look.panel != tokens.BgContainer || look.hairline != hairline || look.selected != ui.Hex("#2563eb").Alpha(0.12) {
			t.Fatal("surfaces do not use Lobe roles", look)
		}
		if look.hairline == tokens.BgElevated || look.hairline == tokens.BgContainer {
			t.Fatal("separator disappears on dialog or content surface")
		}
	}
}
