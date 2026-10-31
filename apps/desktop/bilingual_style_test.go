package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/styles"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

func TestBilingualIndependentControlsAndFontPayload(t *testing.T) {
	for _, primary := range []string{"source", "translation"} {
		t.Run(primary, func(t *testing.T) {
			a := withStyleCatalog(t)
			a.settings.SubtitleOutput = subtitle.OutputOptions{Mode: "bilingual", Primary: primary}
			a.currentStyle.FontName = "Arial"
			a.currentStyle.FontSize = 30
			a.secondaryStyle = a.currentStyle
			a.secondaryStyle.FontName = "Courier New"
			a.secondaryStyle.FontSize = 16
			a.snapshot.Document.Cues[0].Text = "Translated line"
			a.snapshot.Document.Cues[0].Translated = true
			a.snapshot.Document.Cues[0].TranslationSource = "Source line"
			tt := settled(a.stylePanel, 1200, 700)
			if err := tt.Click(locales.StyleSecondaryLayer()); err != nil {
				t.Fatal(err)
			}
			if err := tt.Click(locales.StyleFontSize()); err != nil {
				t.Fatal(err)
			}
			tt.Key(ui.Cmd, ui.KeyA)
			tt.Type("14")
			tt.Key(0, ui.KeyEnter)
			if a.currentStyle.FontSize != 30 || a.secondaryStyle.FontSize != 14 || a.editedLayer() != 1 {
				t.Fatal("secondary controls changed the wrong layer", a.currentStyle, a.secondaryStyle)
			}
			a.observeStyle(time.Now())
			a.saveSettings()
			prefs, err := settings.Load(filepath.Join(a.dataRoot, "settings.json"))
			if err != nil || prefs.SecondaryStyle == nil || prefs.SecondaryStyle.FontName != "Courier New" || prefs.SecondaryStyle.FontSize != 14 {
				t.Fatal("secondary style not saved", err)
			}
			client := styles.Client{Executable: os.Getenv("ONESUB_STYLE_WORKER"), Scratch: t.TempDir()}
			built, err := client.BuildOutput(context.Background(), a.snapshot.Document, a.currentStyle, a.secondaryStyle, a.settings.SubtitleOutput, 16.0/9)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(built.ASS, "Style: Default,Arial,30,") || !strings.Contains(built.ASS, "Style: Secondary,Courier New,14,") {
				t.Fatal("bilingual styles lost", built.ASS)
			}
			fonts := map[string]bool{}
			for _, font := range built.Fonts {
				fonts[font.Name] = true
			}
			if !fonts["Arial"] || !fonts["Courier New"] {
				t.Fatal("secondary font omitted from preview/burn payload", fonts)
			}
		})
	}
}
