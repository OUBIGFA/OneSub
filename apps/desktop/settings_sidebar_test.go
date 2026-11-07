package main

import (
	"fmt"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// A content-area click or a window focus change must not turn the selected
// settings item gray. Test the real dialog and its native sidebar in every
// category; the keyboard focus ring remains independent of the chosen item.
func TestSettingsSidebarKeepsAccentAcrossBlankClicks(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		for _, width := range []int{900, 1400} {
			for _, scale := range []float32{1, 1.5, 2} {
				t.Run(fmt.Sprintf("%s/%d/%g", language, width, scale), func(t *testing.T) {
					inLocale(t, language, func() {
						a := nativeForTest(t)
						a.settingsOpen = true
						tt := settled(a.view, int(float32(width)*scale), int(640*scale))
						tt.SetPreferences(ui.Preferences{TextScale: scale, ReduceMotion: true})
						labels := []string{locales.SettingsGeneral(), locales.SettingsRecognition(), locales.SettingsTranslation(), locales.SettingsTranslationRules(), locales.SettingsMcp(), locales.SettingsLanguageRoutes(), locales.SettingsAbout()}
						for _, dark := range []bool{false, true} {
							tt.SetDark(dark)
							for _, tab := range []int{0, 1, 5, 2, 3, 4, 6} {
								if err := tt.Click(labels[tab]); err != nil {
									t.Fatal(err)
								}
								check := func() {
									t.Helper()
									if a.settingsTab != tab || !a.settingsOpen {
										t.Fatal("a blank click changed the settings category", tab, a.settingsTab)
									}
									nav, _ := tt.Find(locales.A11ySettingsTabs())
									chosen, _ := tt.Find(labels[tab])
									// Sample the selected row away from glyphs and the
									// focus ring, using the sidebar's actual geometry.
									pixel := tt.Image().RGBAAt(int(nav.X+14*scale), int(chosen.Y+chosen.H/2))
									accent := ui.Hex("#2563eb")
									if pixel.R != accent.R || pixel.G != accent.G || pixel.B != accent.B {
										t.Fatal("selected settings item lost its accent", tab, dark, pixel, nav, chosen)
									}
								}
								check()
								area, _ := tt.Find(locales.A11ySettingsContent())
								// The right end of the page heading is native blank
								// content, not another button or a sidebar item.
								tt.ClickAt(area.X+area.W-8*scale, area.Y+8*scale)
								check()
								for range 4 {
									tt.FrameOnce()
									check()
								}
								tt.SetFocused(false)
								check()
								tt.SetFocused(true)
								check()
							}
						}
					})
				})
			}
		}
	}
}
