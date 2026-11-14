package main

import (
	"bytes"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
	lobeNav "github.com/lobehub/lobe-mygo/pkg/nav"
)

// Motion stays enabled: reduced-motion layout tests conceal the delayed
// selected surface. Resizing must already paint the final surface in one frame.
func TestSelectedTagsResizeInTheCurrentFrame(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, scale := range []float32{1, 1.5, 2} {
			for _, kind := range []string{"properties", "editor", "export", "settings"} {
				t.Run(fmt.Sprintf("%s/dark%t/%g", kind, dark, scale), func(t *testing.T) {
					labels := []string{locales.StyleText(), locales.StyleAppearance(), locales.StyleLayout()}
					switch kind {
					case "editor":
						labels = []string{locales.TabSource(), locales.TabTranslation()}
					case "export":
						labels = []string{"SRT", "VTT", "ASS", "SSA", "TXT"}
					case "settings":
						labels = []string{locales.SettingsGeneral(), locales.SettingsRecognition(), locales.SettingsTranslation()}
					}
					selected, width := 0, float32(330)*scale
					tt := settled(func(c *ui.Context) {
						applyStudioTheme(c)
						ui.Column(c).Fill().AlignItems(ui.Center).Children(func() {
							ui.Column(c).Width(width).Children(func() {
								if kind == "settings" {
									items := make([]lobeNav.TabItem, len(labels))
									for i, label := range labels {
										items[i] = lobeNav.TabItem{Label: label}
									}
									lobeNav.Tabs(c, &selected, lobeNav.TabsProps{Items: items, Vertical: true, Variant: lobeNav.TabsRounded}, nil).Width(width)
								} else {
									segmentedFill(c, &selected, labels...)
								}
							})
						})
					}, int(640*scale), int(200*scale))
					tt.SetDark(dark)
					tt.SetPreferences(ui.Preferences{TextScale: scale})
					bounds := func(label string) (ui.Rect, bool) {
						for _, node := range tt.AccessAll() {
							if node.Name == label && (node.Role == ui.RoleRadio || node.Role == ui.RoleTab) {
								return node.Bounds, true
							}
						}
						return ui.Rect{}, false
					}
					for index := range labels {
						selected = index
						tt.Frame()
						tt.Advance(time.Second)
						for _, target := range []float32{510, 390, 330} {
							before, _ := bounds(labels[selected])
							width = target * scale
							tt.FrameOnce()
							first, ok := bounds(labels[selected])
							if !ok || first.W == before.W {
								t.Fatal("selected tag did not resize", before, first)
							}
							pixels := append([]byte(nil), tt.Image().Pix...)
							tt.Advance(time.Second)
							if !bytes.Equal(pixels, tt.Image().Pix) {
								t.Fatalf("selected %q surface changed after resize to %g", labels[selected], target)
							}
						}
					}
				})
			}
		}
	}
}

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
						// Lobe reserves its native outer inset and inter-item gaps.
						if !ok || r.W < track.W/3-6 || i > 0 && math.Abs(float64(r.W-previous.W)) > 1 {
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
