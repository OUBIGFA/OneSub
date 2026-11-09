package main

import (
	"strings"
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

func TestToggleSelectionAndButtonHoverFeedback(t *testing.T) {
	for _, dark := range []bool{false, true} {
		selected := false
		tt := ui.NewTester(func(c *ui.Context) {
			applyStudioTheme(c)
			ui.Column(c).Padding(20).Gap(20).Children(func() {
				lobeToggle(c, &selected, "Toggle").Width(160)
				iconButton(c, "Compose", "clapperboard")
				toolIcon(c, "Clear workspace", "eraser")
			})
		}, 300, 240)
		tt.SetDark(dark)
		tt.SetPreferences(ui.Preferences{ReduceMotion: true})
		tt.Frame()
		r, _ := tt.Find("Toggle")
		sample := func() uint8 { return tt.Image().RGBAAt(int(r.X+8), int(r.Y+r.H/2)).R }
		before := sample()
		if err := tt.Click("Toggle"); err != nil {
			t.Fatal(err)
		}
		tt.Move(290, 230)
		tt.Advance(time.Second)
		if !selected || sample() <= before {
			t.Fatalf("selected surface is not elevated: dark=%t before=%d after=%d", dark, before, sample())
		}
		for _, name := range []string{"Compose", "Clear workspace"} {
			r, _ = tt.Find(name)
			tt.Move(r.X+r.W/2, r.Y+r.H/2)
			tt.Advance(2 * time.Second)
			shown := false
			for _, node := range tt.AccessAll() {
				if node.Role == ui.RoleTooltip {
					shown = true
				}
			}
			if shown != (name == "Clear workspace") {
				t.Fatalf("incorrect tooltip for %s: shown=%t", name, shown)
			}
		}
	}
}

func TestElidedTextKeepsNaturalHeaderWidth(t *testing.T) {
	var required float32
	tt := settled(func(c *ui.Context) {
		applyStudioTheme(c)
		required, _ = c.MeasureText(0, ui.Span{Text: "test.mp4"})
		panelHeader(c, nil, func() { elidedText(c, "test.mp4").Shrink(1) }, nil)
	}, 700, 80)
	r, ok := tt.Find("test.mp4")
	if !ok || r.W < required-1 {
		t.Fatalf("short file name was collapsed: %v, required=%g", r, required)
	}
}

// A full-value hint is useful only while the native input actually elides it.
// Extra instructions must still appear, even for a short editable value.
func TestInputValueTooltipOnlyWhenElided(t *testing.T) {
	for _, combo := range []bool{false, true} {
		for _, mode := range []string{"short", "long", "instruction"} {
			value := "model-a"
			if mode == "long" {
				value = strings.Repeat("long-model-name-", 8)
			}
			hint := value
			if mode == "instruction" {
				hint = "Choose a model or enter its ID"
			}
			tt := settled(func(c *ui.Context) {
				applyStudioTheme(c)
				ui.Column(c).Padding(20).Children(func() {
					var input ui.Element
					if combo {
						input = lobeCombobox(c, &value, []string{value})
					} else {
						input = lobeTextInput(c, &value)
					}
					input.Label("field").Tooltip(hint)
				})
			}, 240, 200)
			r, _ := tt.Find("field")
			tt.Move(r.X+r.W/2, r.Y+r.H/2)
			tt.Advance(2 * time.Second)
			shown := false
			for _, node := range tt.AccessAll() {
				shown = shown || node.Role == ui.RoleTooltip
			}
			if shown != (mode != "short") {
				t.Fatalf("combo=%t mode=%s: incorrect value tooltip, shown=%t", combo, mode, shown)
			}
		}
	}
}
