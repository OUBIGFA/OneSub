package main

import (
	"image"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	lobeTheme "github.com/lobehub/lobe-mygo/pkg/theme"
)

// Exercise the shared native tooltip, including its rendered edge. Changing a
// toolbar wrapper alone would leave input and settings hints with the old look.
func TestLightTooltipsUseDarkFillWhiteTextWithoutShadow(t *testing.T) {
	nativeForTest(t)
	for _, variant := range []string{"native", "lobe"} {
		t.Run(variant, func(t *testing.T) {
			tt := settled(func(c *ui.Context) {
				applyStudioTheme(c)
				ui.Column(c).Fill().Background(c.Theme().Background).Padding(90, 100).Children(func() {
					if variant == "native" {
						toolIcon(c, "悬停提示 Tooltip", "file-warning")
					} else {
						lobeMenuButton(c, "悬停提示 Tooltip", "file-warning", true, func() []menuCommand { return nil })
					}
				})
			}, 420, 250)
			tt.SetDark(false)
			tt.Frame()
			before := tt.Image()
			button, _ := tt.Find("悬停提示 Tooltip")
			tt.Move(button.X+button.W/2, button.Y+button.H/2)
			tt.Advance(2 * time.Second)
			var tip ui.Rect
			for _, node := range tt.AccessAll() {
				if node.Role == ui.RoleTooltip {
					tip = node.Bounds
				}
			}
			if tip.W == 0 {
				t.Fatal("native hover tooltip did not appear")
			}
			after := tt.Image()
			bounds := image.Rect(int(tip.X), int(tip.Y), int(tip.X+tip.W), int(tip.Y+tip.H))
			dark, white := 0, 0
			for y := bounds.Min.Y + 4; y < bounds.Max.Y-4; y++ {
				for x := bounds.Min.X + 4; x < bounds.Max.X-4; x++ {
					p := after.RGBAAt(x, y)
					if p.R <= 60 && p.G <= 60 && p.B <= 60 {
						dark++
					}
					if p.R >= 245 && p.G >= 245 && p.B >= 245 {
						white++
					}
				}
			}
			if dark < (bounds.Dx()-8)*(bounds.Dy()-8)/2 || white < 10 {
				t.Errorf("tooltip is not dark with white text: dark=%d white=%d bounds=%v", dark, white, bounds)
			}
			// The top edge is away from the button's hover feedback and rounded corners.
			for y := bounds.Min.Y - 8; y < bounds.Min.Y-1; y++ {
				for x := bounds.Min.X + 8; x < bounds.Max.X-8; x++ {
					if after.RGBAAt(x, y) != before.RGBAAt(x, y) {
						t.Fatal("tooltip still casts a shadow outside its bounds", x, y)
					}
				}
			}
			captureRedesign(t, tt, "light-tooltip-"+variant)
		})
	}
}

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
