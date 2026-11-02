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
			a.recognitionAdvanced, a.translationAdvanced = true, true
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
		})
	}
}
