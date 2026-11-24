package main

import (
	"image"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
	lobeTheme "github.com/lobehub/lobe-mygo/pkg/theme"
)

func TestWorkbenchHidesOnlyDividersBetweenPanels(t *testing.T) {
	a := nativeForTest(t)
	tt := settled(a.view, 1440, 1000)
	for _, dark := range []bool{false, true} {
		tt.SetDark(dark)
		tt.Frame()
		player, _ := tt.Find(locales.A11yPlayer())
		editor, _ := tt.Find(locales.A11yEditorPanel())
		style, _ := tt.Find(locales.A11yStylePanel())
		workflow, _ := tt.Find(locales.A11yWorkflowColumn())
		controls, _ := tt.Find(locales.A11yPlayerControls())
		canvas, panel := ui.Hex("#F1F1F3"), ui.Hex("#FFFFFF")
		if dark {
			canvas, panel = ui.Hex("#141519"), ui.Hex("#25262B")
		}
		pixel := func(x, y float32, want ui.Color) {
			t.Helper()
			got := tt.Image().RGBAAt(int(x), int(y))
			if got.R != want.R || got.G != want.G || got.B != want.B || got.A != 255 {
				t.Fatalf("dark=%t at (%.1f, %.1f): got %v; want %v", dark, x, y, got, want)
			}
		}
		pixel(player.X+18, player.Y+18, ui.Hex("#000000"))
		for _, r := range []ui.Rect{style, editor, workflow, controls} {
			pixel(r.X+16, r.Y+2, panel)
		}
		pixel(editor.X+16, player.Y+2, panel)
		// In contrast, the editor's title rule remains painted inside its
		// own panel. A fractional divider may cover either neighboring pixel.
		pixels := tt.Image()
		xRule, yRule := int(editor.X+18), int(editor.Y+headerSize)
		rule, next := pixels.RGBAAt(xRule, yRule), pixels.RGBAAt(xRule, yRule+1)
		if rule == next && rule.R == panel.R && rule.G == panel.G && rule.B == panel.B {
			t.Fatal("the internal editor divider disappeared", dark, rule)
		}
		// At rest, all four gaps between the five main panels show pure canvas.
		// Hovering any gap reveals only a centered grip icon without painting a line.
		paneCenterY := (player.Y + style.Y + style.H) / 2
		gaps := []struct {
			name             string
			vertical         bool
			hoverX, hoverY   float32
			centerX, centerY float32
			offCenterX, offY float32
		}{
			{"player-middle", false, player.X + player.W + workbenchInset + .5, player.Y + player.H/2, player.X + player.W + workbenchInset + .5, paneCenterY, player.X + player.W + workbenchInset + .5, player.Y + 24},
			{"player-style", true, player.X + 24, style.Y - workbenchInset - .5, player.X + player.W/2, style.Y - workbenchInset - .5, player.X + 24, style.Y - workbenchInset - .5},
			{"middle-workflow", false, workflow.X - workbenchInset - .5, workflow.Y + 24, workflow.X - workbenchInset - .5, paneCenterY, workflow.X - workbenchInset - .5, workflow.Y + 24},
			{"subtitles-editor", true, editor.X + 24, editor.Y - workbenchInset - .5, editor.X + editor.W/2, editor.Y - workbenchInset - .5, editor.X + 24, editor.Y - workbenchInset - .5},
		}
		hasGrip := func(cx, cy float32, vertical bool) bool {
			img := tt.Image()
			rx, ry := 3, 7
			if vertical {
				rx, ry = 7, 3
			}
			for py := int(cy) - ry; py <= int(cy)+ry; py++ {
				for px := int(cx) - rx; px <= int(cx)+rx; px++ {
					got := img.RGBAAt(px, py)
					if got.R != canvas.R || got.G != canvas.G || got.B != canvas.B {
						return true
					}
				}
			}
			return false
		}
		tt.Move(player.X+18, player.Y+18)
		tt.Frame()
		for _, g := range gaps {
			pixel(g.centerX, g.centerY, canvas)
			pixel(g.offCenterX, g.offY, canvas)
			if hasGrip(g.centerX, g.centerY, g.vertical) {
				t.Fatalf("dark=%t %s showed grip icon at rest", dark, g.name)
			}
		}
		for i, g := range gaps {
			tt.Move(g.hoverX, g.hoverY)
			tt.Frame()
			if !hasGrip(g.centerX, g.centerY, g.vertical) {
				t.Fatalf("dark=%t %s did not show grip icon on hover at (%.1f, %.1f) center (%.1f, %.1f)", dark, g.name, g.hoverX, g.hoverY, g.centerX, g.centerY)
			}
			for j, other := range gaps {
				if j != i && hasGrip(other.centerX, other.centerY, other.vertical) {
					t.Fatalf("dark=%t hovering %s also showed grip icon on %s", dark, g.name, other.name)
				}
			}
			pixel(g.offCenterX, g.offY, canvas)
		}
		tt.Move(player.X+18, player.Y+18)
		tt.Frame()
		for _, g := range gaps {
			if hasGrip(g.centerX, g.centerY, g.vertical) {
				t.Fatalf("dark=%t %s grip icon remained after pointer left", dark, g.name)
			}
		}
	}
}

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
		frame, panel := ui.Hex("#F1F1F3"), ui.Hex("#FFFFFF")
		if dark {
			frame, panel = ui.Hex("#141519"), ui.Hex("#25262B")
		}
		if look.chrome != frame || look.panel != panel || look.hairline != hairline || look.selected != ui.Hex("#2563eb").Alpha(0.12) {
			t.Fatal("the neutral canvas, shared panel surface or selection changed", look)
		}
		if look.hairline == tokens.BgElevated || look.hairline == tokens.BgContainer {
			t.Fatal("separator disappears on dialog or content surface")
		}
	}
}
