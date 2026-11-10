package main

import (
	"fmt"
	"image"
	"math"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// Empty tracks keep import and track navigation, without presenting disabled
// editing commands as useful next actions. Existing source data stays intact.
func TestSubtitleEmptyTrackShowsImportAndTrackNavigation(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		for _, state := range []string{"workbench", "empty-project", "optimized-mirror"} {
			for _, width := range []int{320, 640} {
				t.Run(fmt.Sprintf("%s/%s/%d", language, state, width), func(t *testing.T) {
					inLocale(t, language, func() {
						a := nativeForTest(t)
						switch state {
						case "empty-project":
							a = storeApp(t)
						case "optimized-mirror":
							a = storeApp(t, subtitle.Cue{ID: "a", Text: "Original subtitle", StartMs: 0, EndMs: 3000})
							a.changeSubtitleTab(1)
						}
						tt := settled(a.subtitlesView, width, 800)
						tools := []string{locales.SubtitleActionsMenu(), locales.ListLocate(), locales.TextReplacePeriods(), locales.TextStripTrailing(), locales.FindReplace(), locales.ListSearch(), locales.ReadingSplitCount(0), locales.ReadingIssueType(), locales.ReadingPrevious(), locales.ReadingNext(), locales.TranslationResults(), locales.ListSave(), locales.ListSaveAs()}
						for _, label := range []string{locales.ListImport(), locales.A11ySubtitleTabs()} {
							r, ok := tt.Find(label)
							if !ok || r.X < 0 || r.X+r.W > float32(width) || r.H <= 0 {
								t.Fatal("empty-track navigation clipped", label, r)
							}
						}
						for _, label := range tools {
							if _, ok := tt.Find(label); ok {
								t.Fatal("empty track presents an unavailable edit", label)
							}
						}
						captureRedesign(t, tt, "empty-toolbar-"+language+"-"+state)
					})
				})
			}
		}
	}
}

// Center the visible ink, not just the line box: the system font reserves
// unequal space above and below the glyphs even in a centered native row.
func TestLabeledToolbarButtonCentersVisibleIconAndText(t *testing.T) {
	nativeForTest(t) // load the production icon resources
	for _, label := range []string{"字幕列表操作", "Subtitle actions"} {
		for _, scale := range []float32{1, 1.5, 2} {
			t.Run(fmt.Sprintf("%s/%g", label, scale), func(t *testing.T) {
				tt := ui.NewTester(func(c *ui.Context) {
					ui.Toolbar(c, func() { iconButton(c, label, "ellipsis") }).Padding(0)
				}, 600, 100)
				tt.SetPreferences(ui.Preferences{TextScale: scale})
				tt.Frame()
				button, _ := tt.Find(label)
				glyph, _ := tt.Find(locales.A11yIconOf(label))
				frame := tt.Image()
				inkCenter := func(r image.Rectangle) float64 {
					low, high := r.Max.Y, r.Min.Y
					for y := r.Min.Y; y < r.Max.Y; y++ {
						for x := r.Min.X; x < r.Max.X; x++ {
							pixel := frame.RGBAAt(x, y)
							if pixel.R < 128 && pixel.G < 128 && pixel.B < 128 {
								low, high = min(low, y), max(high, y)
							}
						}
					}
					if low > high {
						t.Fatal("button content was not rendered", r)
					}
					return float64(low+high) / 2
				}
				iconCenter := inkCenter(image.Rect(int(glyph.X), int(button.Y), int(math.Ceil(float64(glyph.X+glyph.W))), int(button.Y+button.H)))
				textCenter := inkCenter(image.Rect(int(math.Ceil(float64(glyph.X+glyph.W)))+1, int(button.Y), int(button.X+button.W), int(button.Y+button.H)))
				t.Logf("icon/text ink centers: %.1f / %.1f", iconCenter, textCenter)
				if math.Abs(iconCenter-textCenter) > 1 {
					t.Fatal("icon and label ink are not vertically centered", iconCenter, textCenter)
				}
			})
		}
	}
}

// Once subtitles exist, all edits are available through the compact menu.
func TestSubtitleToolbarFindReplaceBecomesAvailable(t *testing.T) {
	a := storeApp(t, subtitle.Cue{ID: "a", Text: "First subtitle", StartMs: 0, EndMs: 3000})
	tt := settled(a.subtitlesView, 640, 800)
	if err := tt.Click(locales.SubtitleActionsMenu()); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(locales.FindReplace()); err != nil {
		t.Fatal(err)
	}
	if !a.findReplace.open {
		t.Fatal("Find and replace stayed disabled after loading subtitles")
	}
	// Translation checks stay visible and activate only when checks exist.
	a.translationCandidate = &translationCandidate{}
	tt.Frame()
	if err := tt.Click(locales.TranslationResults()); err != nil || !a.translationReview {
		t.Fatal("translation checks stayed disabled after results arrived", err)
	}
}

func TestSubtitleToolbarShowsLabelsWithIconsExceptNavigation(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := storeApp(t, subtitle.Cue{ID: "a", Text: "First subtitle", StartMs: 0, EndMs: 3000})
			tt := settled(a.subtitlesView, 1800, 800)
			for _, label := range []string{locales.SubtitleActionsMenu(), locales.ListLocate(), locales.TextReplacePeriods(), locales.TextStripTrailing(), locales.FindReplace(), locales.RetranslateSelect(), locales.RetranslateCheckAll(), locales.ReadingSplitCount(0), locales.ReadingLongCues(), locales.TranslationResults()} {
				if box, ok := tt.Find(label); !ok || box.W < box.H*1.5 {
					t.Fatal("wide toolbar does not allocate space for the label", label, box)
				}
				if _, ok := tt.Find(locales.A11yIconOf(label)); !ok {
					t.Fatal("wide toolbar has no icon", label)
				}
			}
			for _, label := range []string{locales.ReadingPrevious(), locales.ReadingNext()} {
				if box, ok := tt.Find(label); !ok || box.W > box.H+2 {
					t.Fatal("navigation must remain icon-only", label)
				}
			}
			captureRedesign(t, tt, "subtitle-tools-wide-"+language)
			tt.SetSize(640, 800)
			tt.Frame()
			for _, label := range []string{locales.SubtitleActionsMenu(), locales.ListLocate(), locales.FindReplace(), locales.RetranslateSelect(), locales.RetranslateCheckAll(), locales.TranslationResults()} {
				if box, ok := tt.Find(label); !ok || box.W > box.H+2 {
					t.Fatal("narrow toolbar must retain icon-only actions", label)
				}
			}
			captureRedesign(t, tt, "subtitle-tools-narrow-"+language)
		})
	}
}

// The same production toolbar must spend its spare padding before hiding
// labels, including after selection adds controls and at larger text scales.
func TestSubtitleToolbarKeepsLabelsWhileTightening(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			for _, scale := range []float32{1, 1.5, 2} {
				for _, checked := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%g/checked=%t", language, scale, checked), func(t *testing.T) {
						a := draftApp(t)
						if checked {
							a.checkAllSubtitles()
							a.retranslation = &retranslationState{}
						}
						labels := []string{locales.ListLocate(), locales.FindReplace(), locales.TranslationResults(), locales.ReadingSplitCount(0), locales.ReadingIssueType()}
						width := 1320
						if language == "en" {
							width = 1720
						}
						if checked {
							width += 300
						}
						var previousWidth float32
						for _, w := range []int{width + 350, width} {
							tt := ui.NewTester(func(c *ui.Context) {
								theme, _ := studioTheme(c.Theme(), false)
								c.SetTheme(&theme)
								cw, _ := c.Size()
								a.subtitleToolbar(c, cw, true)
							}, int(float32(w)*scale), int(180*scale))
							tt.SetPreferences(ui.Preferences{TextScale: scale, ReduceMotion: true})
							firstFrameSettled(t, tt, "toolbar", func() {})
							edit, _ := tt.Find(locales.SubtitleActionsMenu())
							for _, label := range labels {
								r, ok := tt.Find(label)
								if !ok || r.W <= 32*scale {
									t.Fatalf("hid %q before using compact spacing at width %d", label, w)
								}
								if !ok || r.X+r.W > float32(w)*scale || r.Y != edit.Y {
									t.Fatal("second row wrapped or clipped", label, r, edit)
								}
							}
							locate, _ := tt.Find(locales.ListLocate())
							if previousWidth != 0 && locate.W >= previousWidth {
								t.Fatal("buttons did not tighten before hiding labels", previousWidth, locate.W)
							}
							previousWidth = locate.W
							captureRedesign(t, tt, fmt.Sprintf("toolbar-%s-%d-%g-%t", language, w, scale, checked))
						}
					})
				}
			}
		})
	}
}
