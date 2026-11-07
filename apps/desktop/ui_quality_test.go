package main

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// Validate the actual shared foreground/background pairs used by status text,
// unsaved actions and secondary labels. The dark status bar exposed a warning
// text pair below WCAG AA even though text on the main canvas passed.
func TestInterfaceTextContrast(t *testing.T) {
	luminance := func(color ui.Color) float64 {
		linear := func(v uint8) float64 {
			s := float64(v) / 255
			if s <= 0.04045 {
				return s / 12.92
			}
			return math.Pow((s+0.055)/1.055, 2.4)
		}
		return 0.2126*linear(color.R) + 0.7152*linear(color.G) + 0.0722*linear(color.B)
	}
	for _, base := range []*ui.Theme{ui.LightTheme(), ui.DarkTheme()} {
		theme, _ := studioTheme(base, false)
		for _, pair := range []struct {
			name                   string
			foreground, background ui.Color
		}{
			{"body", theme.Text, theme.Background},
			{"control", theme.Text, theme.Surface},
			{"secondary", theme.TextMuted, theme.Background},
			{"secondary on chrome", theme.TextMuted, theme.Surface},
			{"error and unsaved", theme.Danger, theme.Surface},
			{"warning", theme.Warning, theme.Surface},
			{"primary action", theme.AccentText, theme.Accent},
		} {
			fg, bg := luminance(pair.foreground), luminance(pair.background)
			ratio := (max(fg, bg) + 0.05) / (min(fg, bg) + 0.05)
			if ratio < 4.5 {
				t.Errorf("dark=%t %s contrast %.2f:1, want at least 4.5:1", theme.Dark, pair.name, ratio)
			}
		}
	}
}

// A technically un-clipped toolbar is not sufficient: the actual work must
// remain visible. Exercise the real workbench at its minimum size, including
// the scaled equivalents, rather than measuring a standalone wide list.
func TestCompactWorkbenchKeepsSubtitlesReadable(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		for _, scale := range []float32{1, 1.5, 2} {
			t.Run(fmt.Sprintf("%s/%g", language, scale), func(t *testing.T) {
				inLocale(t, language, func() {
					a := storeApp(t,
						subtitle.Cue{ID: "first", Position: 0, Text: "Read the subtitle", EndMs: 4000},
						subtitle.Cue{ID: "second", Position: 1, Text: "Check the timing", StartMs: 5000, EndMs: 9000},
					)
					a.splitX, a.splitY = 60, 64
					tt := ui.NewTester(a.view, int(900*scale), int(640*scale))
					tt.SetPreferences(ui.Preferences{TextScale: scale, ReduceMotion: true})
					tt.Frame()
					list, ok := tt.Find(locales.A11ySubtitleList())
					if !ok || list.H < 140*scale {
						t.Fatalf("toolbar displaced subtitle content: %v", list)
					}
					text, ok := tt.Find("Read the subtitle")
					if !ok || text.W < 110*scale || text.Y+text.H > list.Y+list.H {
						t.Fatalf("subtitle requires horizontal scrolling to read: %v, list %v", text, list)
					}
					for _, label := range []string{locales.ListColumnStart() + " / " + locales.ListColumnEnd(), locales.ListColumnText()} {
						if _, ok := tt.Find(label); !ok {
							t.Fatalf("timing/text column not visible: %s", label)
						}
					}
				})
			})
		}
	}
}

func TestCompactSubtitleMenusKeepEditingAndReviewReachable(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := storeApp(t,
				subtitle.Cue{ID: "first", Position: 0, Text: strings.Repeat("Read this sentence. ", 12), EndMs: 4000},
				subtitle.Cue{ID: "second", Position: 1, Text: "Check the timing.", StartMs: 5000, EndMs: 9000},
			)
			a.splitX, a.splitY = 60, 64
			a.selected = 1
			a.loadDraft()
			tt := settled(a.view, 900, 640)
			if err := tt.Click(locales.SubtitleActionsMenu()); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem(locales.FindReplace()); err != nil || !a.findReplace.open {
				t.Fatal("overflow cannot open find and replace", err)
			}
			a.findReplace.open = false
			tt.Frame()
			if err := tt.Click(locales.ReadingInspect()); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem(locales.ReadingNext()); err != nil || a.selected != 0 {
				t.Fatal("compact review cannot locate a long cue", err, a.selected)
			}
			// Once a task owns the document, the menu must not expose an
			// editing bypass that the normal toolbar would have disabled.
			a.busy = true
			tt.Frame()
			if err := tt.Click(locales.SubtitleActionsMenu()); err != nil || len(tt.Menu()) != 0 {
				t.Fatal("busy workbench opened editing actions", err)
			}
		})
	}
}

func TestProviderProtocolNameRemainsCompleteAtMinimumWindow(t *testing.T) {
	for _, lang := range []string{"zh-Hans", "en"} {
		inLocale(t, lang, func() {
			a := nativeForTest(t)
			a.addProvider()
			a.settingsOpen, a.settingsTab, a.providerSettingsTab = true, 2, 1
			var needed float32
			tt := settled(func(c *ui.Context) {
				a.view(c)
				needed, _ = c.MeasureText(0, ui.Span{Text: "OpenAI Chat Completions"})
			}, 900, 640)
			field, ok := tt.Find(locales.TranslationApi())
			if !ok || field.W < needed+44 {
				t.Fatal("fixed API label would be truncated", field, needed)
			}
			insideRedesign(t, tt, locales.A11ySettingsContent(), locales.TranslationEndpoint(), "API Key", locales.TranslationSaveKey())
		})
	}
}
