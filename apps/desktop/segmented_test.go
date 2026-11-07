package main

import (
	"fmt"
	"math"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// A full-width navigation track must spend all its width on equally sized,
// operable choices, including long English labels and increased text scale.
func TestPropertySegmentsFillTrackAndKeepNativeNavigation(t *testing.T) {
	for _, lang := range []string{"zh-Hans", "en"} {
		for _, scale := range []float32{1, 1.5, 2} {
			t.Run(fmt.Sprintf("%s/%g", lang, scale), func(t *testing.T) {
				inLocale(t, lang, func() {
					selected := 0
					labels := []string{locales.StyleText(), locales.StyleAppearance(), locales.StyleLayout()}
					tt := settled(func(c *ui.Context) {
						theme, _ := studioTheme(c.Theme(), false)
						c.SetTheme(&theme)
						segmentedFill(c, &selected, labels...).Label("property choices")
					}, int(300*scale), int(80*scale))
					tt.SetPreferences(ui.Preferences{TextScale: scale, ReduceMotion: true})
					track, _ := tt.Find("property choices")
					previous := ui.Rect{}
					for i, label := range labels {
						r, ok := tt.Find(label)
						if !ok || r.W < track.W/3-5 || i > 0 && math.Abs(float64(r.W-previous.W)) > 1 {
							t.Fatal("choices do not equally fill the track", label, r, previous, track)
						}
						if i == 2 && track.X+track.W-r.X-r.W > 4*scale {
							t.Fatal("track has unused space after the final choice", r, track)
						}
						if err := tt.Click(label); err != nil || selected != i {
							t.Fatal("native choice is not operable", label, selected, err)
						}
						previous = r
					}
					tt.Key(0, ui.KeyHome)
					if selected != 0 {
						t.Fatal("native Home navigation was lost")
					}
					tt.Key(0, ui.KeyRight)
					if selected != 1 {
						t.Fatal("native arrow navigation was lost")
					}
				})
			})
		}
	}
}
