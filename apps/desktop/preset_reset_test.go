package main

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/styles"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// Reset applies the selected built-in's original values only to the edited
// layer, then follows the normal live-preview and preference-saving path.
func TestBuiltInPresetResetMenu(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := withStyleCatalog(t)
			a.splitY = 40
			a.settings.SubtitleOutput = subtitle.OutputOptions{Mode: "bilingual"}
			label := "恢复默认"
			if language == "en" {
				label = "Restore defaults"
			}
			presets := append([]styles.Preset{{ID: "default", Style: a.styleCatalog.DefaultStyle}}, a.styleCatalog.Presets...)
			for _, preset := range presets {
				for layer := 0; layer < 2; layer++ {
					t.Run(fmt.Sprintf("%s/%s/%d", language, preset.ID, layer), func(t *testing.T) {
						a.styleLayer = layer
						style, id := a.layerStyle(layer)
						*id, *style = preset.ID, preset.Style
						style.FontSize += 3
						style.Shadow += 2
						other, _ := a.layerStyle(1 - layer)
						before := *other
						tt := settled(a.view, 1800, 1100)
						revision := a.previewRevision
						if err := tt.Click(locales.PresetActions()); err != nil {
							t.Fatal(err)
						}
						if err := tt.ChooseMenuItem(label); err != nil {
							t.Fatal(err, tt.Menu())
						}
						tt.Frame()
						if !reflect.DeepEqual(*style, preset.Style) || !reflect.DeepEqual(*other, before) || *id != preset.ID || a.previewRevision <= revision {
							t.Fatal("reset changed the wrong style or did not refresh", *id, *style)
						}
						tt.Advance(time.Second)
						prefs, err := settings.Load(filepath.Join(a.dataRoot, "settings.json"))
						if err != nil {
							t.Fatal(err)
						}
						saved, savedID := prefs.LastStyle, prefs.StylePreset
						if layer == 1 {
							saved, savedID = prefs.SecondaryStyle, prefs.SecondaryPreset
						}
						if saved == nil || !reflect.DeepEqual(*saved, preset.Style) || savedID != preset.ID {
							t.Fatal("reset did not persist", saved, savedID)
						}
					})
				}
			}
			// User presets and a detached custom style have no factory default.
			for _, id := range []string{"user-example", "custom"} {
				a.styleLayer, a.stylePreset = 0, id
				a.settings.UserPresets = []settings.UserPreset{{ID: "user-example", Name: "My style", Style: a.currentStyle}}
				before := a.currentStyle
				tt := settled(a.view, 1800, 1100)
				if err := tt.Click(locales.PresetActions()); err != nil {
					t.Fatal(err)
				}
				if err := tt.ChooseMenuItem(label); err == nil || !reflect.DeepEqual(a.currentStyle, before) {
					t.Fatal("custom preset was reset", id)
				}
			}
		})
	}
}
