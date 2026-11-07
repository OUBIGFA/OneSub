package main

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// The title spans both panes; moving their native divider must not move the
// outer workbench dividers or lose the chosen share when the window resizes.
func TestStyleContentAndPropertiesSplit(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := withStyleCatalog(t)
			tt := settled(a.stylePanel, 901, 600)
			title, _ := tt.Find(locales.StyleTitle())
			content, _ := tt.Find(locales.OutputContent())
			preset, _ := tt.Find(locales.StylePreset())
			properties, _ := tt.Find(locales.StyleProperties())
			if content.X+content.W >= properties.X || preset.X+preset.W >= properties.X || title.Y+title.H > content.Y || title.Y+title.H > properties.Y {
				t.Fatal("style content and presets must sit left of properties below the shared title", title, content, preset, properties)
			}
			captureRedesign(t, tt, "style-split-default-"+language)
			before := properties
			tt.Press(300.5, properties.Y+10)
			tt.Move(390.5, properties.Y+10)
			tt.Release(390.5, properties.Y+10)
			properties, _ = tt.Find(locales.StyleProperties())
			if math.Abs(float64(properties.X-before.X-90)) > 1 {
				t.Fatal("style divider did not move from its initial 1:2 position", before, properties)
			}
			tt.SetSize(1201, 600)
			tt.SetSize(901, 600)
			if after, _ := tt.Find(locales.StyleProperties()); after != properties {
				t.Fatal("resizing changed the chosen style split", properties, after)
			}
			for tab := 0; tab < 3; tab++ {
				firstFrameSettled(t, tt, "style", func() { a.styleTab = tab })
			}
			tt.SetSize(901, 180)
			content, _ = tt.Find(locales.OutputContent())
			tt.Scroll(properties.X+100, properties.Y+60, 0, 2000)
			if after, _ := tt.Find(locales.OutputContent()); after != content || a.styleScroll.Y <= 0 {
				t.Fatal("scrolling properties moved the content pane or did not scroll", content, after, a.styleScroll.Y)
			}
			tt.SetSize(901, 600)
			// Flush the same preference debounce used by the full workbench,
			// then recreate the panel from the stored settings.
			workbench := settled(a.view, 1800, 1000)
			workbench.Advance(time.Second)
			prefs, err := settings.Load(a.dataRoot + "/settings.json")
			if err != nil {
				t.Fatal(err)
			}
			a.settings = prefs
			reopened := ui.NewTester(a.stylePanel, 901, 600)
			if after, _ := reopened.Find(locales.StyleProperties()); after.X != properties.X {
				t.Fatal("saved style split was not restored", properties, after)
			}
			captureRedesign(t, reopened, "style-split-"+language)
		})
	}
}

// The narrowest supported workbench must leave complete labels in both style
// panes, including bilingual layer choices and the longest English tab.
func TestStyleSplitLabelsFitMinimumWorkbench(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		for _, scale := range []float32{1, 1.5, 2} {
			t.Run(fmt.Sprintf("%s/%g", language, scale), func(t *testing.T) {
				inLocale(t, language, func() {
					a := withStyleCatalog(t)
					a.settings.SubtitleOutput = subtitle.OutputOptions{Mode: "bilingual"}
					a.splitY = 40
					labels := []string{locales.StylePrimaryLayer(), locales.StyleSecondaryLayer(), locales.StyleText(), locales.StyleAppearance(), locales.StyleLayout()}
					widths := make(map[string]float32)
					tt := ui.NewTester(func(c *ui.Context) {
						a.view(c)
						for _, label := range labels {
							widths[label], _ = c.MeasureText(0, ui.Span{Text: label})
						}
					}, int(minimumWindowWidth*scale), int(1000*scale))
					tt.SetPreferences(ui.Preferences{TextScale: scale, ReduceMotion: true})
					checkLabels := func() {
						for _, label := range labels {
							box, ok := tt.Find(label)
							if !ok || box.W < widths[label]+16*scale {
								t.Fatalf("style label is truncated: %s (%v, text width %g)", label, box, widths[label])
							}
						}
					}
					checkLabels()
					captureRedesign(t, tt, fmt.Sprintf("style-minimum-%s-%g", language, scale))
					for _, distance := range []float32{-2000, 2000} {
						properties, _ := tt.Find(locales.StyleProperties())
						x, y := properties.X-panelPad*scale-.5, properties.Y+10*scale
						tt.Press(x, y)
						tt.Move(x+distance, y)
						tt.Release(x+distance, y)
						checkLabels()
					}
				})
			})
		}
	}
}
