package main

import (
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
)

// The pinned WinOne base palette and OneSub's original blue emphasis are
// explicit contracts; appearance changes must not silently drift either.
func TestWinOneLightColorContract(t *testing.T) {
	for _, scale := range []float32{1, 1.5, 2} {
		base := *ui.LightTheme()
		base.FontSize, base.Font = 14*scale, "system-test-font"
		got, _ := studioTheme(&base, false)
		for _, pair := range []struct {
			got ui.Color
			hex string
		}{
			{got.Background, "#fcfcfc"}, {got.Surface, "#f3f3f3"}, {got.SurfaceHover, "#ebebeb"}, {got.SurfacePressed, "#e2e2e2"},
			{got.Border, "#d6d6d6"}, {got.Text, "#262626"}, {got.TextMuted, "#5e5e5e"},
			{got.Accent, "#2563eb"}, {got.AccentHover, "#1d4ed8"}, {got.AccentPressed, "#1e40af"}, {got.AccentText, "#ffffff"},
			{got.Danger, "#c42b1c"}, {got.Warning, "#9d5d00"}, {got.Success, "#0f7b0f"}, {got.Inverse, "#2b2b2b"}, {got.InverseText, "#fafafa"},
		} {
			if pair.got != ui.Hex(pair.hex) {
				t.Fatalf("light palette drift: %v != %s", pair.got, pair.hex)
			}
		}
		if got.Selection != ui.RGBA(37, 99, 235, 0.25) || got.Focus != ui.RGBA(37, 99, 235, 0.55) || got.Scrollbar != ui.RGBA(0, 0, 0, 0.3) {
			t.Fatal("selection, focus or scrollbar drift")
		}
		if got.FontSize != bodySize*scale || got.Font != base.Font || got.Radius != base.Radius || got.Spacing != base.Spacing {
			t.Fatal("color reuse changed native metrics", got)
		}
	}
}

func TestWinOneLightSurfaceRolesAndContrast(t *testing.T) {
	var look studio
	settled(func(c *ui.Context) {
		theme, _ := studioTheme(c.Theme(), false)
		c.SetTheme(&theme)
		look = studioColors(c)
		ui.Column(c).Fill().Background(look.panel)
	}, 300, 200)
	if look.panel != ui.Hex("#ffffff") || look.chrome != ui.Hex("#f3f3f3") || look.hairline != ui.RGBA(0, 0, 0, 0.08) || look.selected != ui.Hex("#2563eb").Alpha(0.12) || look.warningSoft != ui.RGBA(157, 93, 0, 0.1) {
		t.Fatal("surface roles differ from WinOne", look)
	}
	high, _ := studioTheme(ui.LightTheme(), true)
	if high.Border != ui.Hex("#d6d6d6").Mix(ui.Hex("#262626"), 0.45) || high.TextMuted != ui.Hex("#5e5e5e").Mix(ui.Hex("#262626"), 0.4) || high.Focus != high.Accent {
		t.Fatal("WinOne high-contrast adaptation was lost")
	}
}

func TestWinOneLightReuseLeavesDarkThemeUnchanged(t *testing.T) {
	base := ui.DarkTheme()
	want := *base
	want.FontSize = bodySize
	primary := ui.LightTheme()
	want.Accent, want.AccentHover, want.AccentPressed, want.AccentText = primary.Accent, primary.AccentHover, primary.AccentPressed, primary.AccentText
	got, _ := studioTheme(base, false)
	if !reflect.DeepEqual(got, want) {
		t.Fatal("light palette changed dark appearance")
	}
}
