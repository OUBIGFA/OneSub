package main

import (
	"fmt"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// A narrow monitor pane must not push time or audio controls outside its
// bounds. Native rows wrap complete command groups; the split stays user-owned.
func TestPlayerTransportGroupsFitNarrowPanes(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		for _, width := range []int{320, 500, 900} {
			for _, scale := range []float32{1, 1.5, 2} {
				t.Run(fmt.Sprintf("%s/%d/%g", language, width, scale), func(t *testing.T) {
					inLocale(t, language, func() {
						a := nativeForTest(t)
						a.position, a.duration = 123, 456
						a.mediaURL = "http://127.0.0.1/video"
						tt := settled(func(c *ui.Context) {
							theme, _ := studioTheme(c.Theme(), false)
							c.SetTheme(&theme)
							a.playerView(c)
						}, int(float32(width)*scale), int(500*scale))
						tt.SetPreferences(ui.Preferences{TextScale: scale})
						labels := []string{locales.PlayerBack(), locales.PlayerPlay(), locales.PlayerForward(), locales.PlayerSpeed(), locales.PlayerLoop(), stamp(123), stamp(456), locales.PlayerMute(), locales.PlayerVolume()}
						insideRedesign(t, tt, locales.A11yPlayer(), labels...)
						for i, label := range labels {
							r, _ := tt.Find(label)
							for _, other := range labels[i+1:] {
								s, _ := tt.Find(other)
								if r.X < s.X+s.W-1 && r.X+r.W > s.X+1 && r.Y < s.Y+s.H-1 && r.Y+r.H > s.Y+1 {
									t.Fatal("transport controls overlap", label, other, r, s)
								}
							}
						}
					})
				})
			}
		}
	}
}
