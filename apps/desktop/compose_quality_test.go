package main

import (
	"path/filepath"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

func TestComposeOffersCRFOnlyAndDailyRecommendation(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := nativeForTest(t)
			a.composeOpen = true
			tt := settled(a.composeView, 900, 640)
			if a.settings.ComposeQuality != "crf20" || !tt.HasText("CRF 20") {
				t.Fatal("incorrect install default")
			}
			note, ok := tt.Find(locales.ComposeCrfNote())
			recommendation, found := tt.Find(locales.ComposeCrfRecommendation())
			if !ok || !found || recommendation.Y <= note.Y {
				t.Fatal("recommendation is not below the CRF explanation", note, recommendation)
			}
			insideRedesign(t, tt, locales.ActionCompose(), locales.ComposeCrfNote(), locales.ComposeCrfRecommendation(), locales.ComposeStart())
			button, _ := tt.Find(locales.ComposeStart())
			if err := tt.Click("CRF 20"); err != nil {
				t.Fatal(err)
			}
			if tt.HasText("原画质") || tt.HasText("lossless") {
				t.Fatal("retired quality is still offered")
			}
			for _, label := range []string{"CRF 18", "CRF 20", "CRF 23"} {
				if !tt.HasText(label) {
					t.Fatal("missing CRF choice", label)
				}
			}
			if err := tt.Click("CRF 23"); err != nil {
				t.Fatal(err)
			}
			prefs, err := settings.Load(filepath.Join(a.dataRoot, "settings.json"))
			if err != nil || prefs.ComposeQuality != "crf23" {
				t.Fatal("quality choice not saved", err)
			}
			if after, _ := tt.Find(locales.ComposeStart()); after != button {
				t.Fatal("quality choice resized the dialog", button, after)
			}
			captureRedesign(t, tt, "compose-crf-"+language)
		})
	}
}
