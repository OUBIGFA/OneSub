package main

import (
	"fmt"
	"math"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/translator"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

func TestProviderStatusWidthsAndScrollbarGutter(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := nativeForTest(t)
			a.addProvider()
			a.useProviderModel("a-current")
			a.keepProviderModel("b-kept")
			p := a.settings.Provider()
			p.Catalog = []translator.ModelInfo{{ID: "c-unused"}}
			for i := 0; i < 80; i++ {
				p.Catalog = append(p.Catalog, translator.ModelInfo{ID: fmt.Sprintf("z-model-%03d", i)})
			}
			tt := settled(func(c *ui.Context) { a.providerModelsView(c) }, 650, 300)
			used, ok := tt.Find(locales.TranslationUseModelNamed("b-kept"))
			unused, found := tt.Find(locales.TranslationUseModelNamed("c-unused"))
			current, currentFound := tt.Find(locales.TranslationUseModelNamed("a-current"))
			if !ok || !found || !currentFound || math.Abs(float64(current.W-used.W)) > 1 || math.Abs(float64(used.W-unused.W)) > 1 || used.X+used.W > 630 || unused.X+unused.W > 630 {
				t.Fatal("status sizes differ or overlap the scrollbar gutter", language, used, unused)
			}
		})
	}
}

func TestPunctuationControlsBelongToTranslationOnly(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := nativeForTest(t)
			tt := settled(a.workflowPanel, 600, 1400)
			if tt.HasText(locales.WorkflowStripPunctuation()) || tt.HasText(locales.WorkflowReplacePeriods()) {
				t.Fatal("recognition still contains local translation cleanup")
			}
			a.workflowTab = 1
			tt.Frame()
			for _, label := range []string{locales.WorkflowTranslationAdvanced(), locales.TranslationBatchSize(), locales.WorkflowStripPunctuation(), locales.WorkflowKeepPunctuation(), locales.WorkflowReplacePeriods()} {
				if _, ok := tt.Find(label); !ok {
					t.Fatal("translation cleanup control missing", label)
				}
			}
			for _, width := range []int{320, 500, 900} {
				for _, scale := range []float32{1, 1.5, 2} {
					tt := settled(func(c *ui.Context) {
						theme, _ := studioTheme(c.Theme(), false)
						c.SetTheme(&theme)
						a.workflowPanel(c)
					}, int(float32(width)*scale), int(850*scale))
					tt.SetPreferences(ui.Preferences{TextScale: scale, ReduceMotion: true})
					tt.Scroll(float32(width)*scale/2, 500*scale, 0, 3000)
					visibleRedesign(t, tt, locales.TranslationConcurrency(), locales.TranslationBatchSize(), locales.WorkflowStripPunctuation(), locales.WorkflowKeepPunctuation(), locales.WorkflowReplacePeriods())
					if scale == 1 {
						captureRedesign(t, tt, fmt.Sprintf("translation-advanced-%s-%d", language, width))
						tt.SetDark(true)
						captureRedesign(t, tt, fmt.Sprintf("translation-advanced-%s-%d-dark", language, width))
					}
				}
			}
		})
	}
}
