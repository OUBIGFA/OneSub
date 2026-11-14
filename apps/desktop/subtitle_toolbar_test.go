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

func TestSmartSplitToolsStartCollapsedAndExpandInToolbar(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			for _, width := range []int{320, 800, 1800} {
				t.Run(fmt.Sprintf("%s/%d", language, width), func(t *testing.T) {
					a := storeApp(t, subtitle.Cue{ID: "long", Text: "这是一条足够长的字幕，用来核对智能拆分的入口、下拉菜单和前后定位按钮。", EndMs: 15000})
					tt := settled(a.subtitlesView, width, 800)
					button, found := tt.Find(locales.ReadingTools())
					glyph, iconFound := tt.Find(locales.A11yIconOf(locales.ReadingTools()))
					if !found || !iconFound || button.W <= button.H || glyph.W <= 0 || button.X+button.W > float32(width) {
						t.Fatal("collapsed smart split must show its full label and icon", button, glyph)
					}
					for _, label := range []string{locales.ReadingSplitCount(1), locales.ReadingIssueType(), locales.ReadingPrevious(), locales.ReadingNext()} {
						if _, shown := tt.Find(label); shown {
							t.Fatal("reading tool is visible before expansion", label)
						}
					}
					if err := tt.Click(locales.ReadingTools()); err != nil {
						t.Fatal(err)
					}
					for _, label := range []string{locales.ReadingSplitCount(1), locales.ReadingIssueType(), locales.ReadingPrevious(), locales.ReadingNext(), locales.ReadingToolsCollapse()} {
						box, shown := tt.Find(label)
						if !shown || box.X < 0 || box.X+box.W > float32(width)+1 {
							t.Fatal("expanded tool is missing or clipped", label, box)
						}
					}
					// Native accessibility treats the menu button as a leaf. Check
					// its real render children here, including the trailing chevron.
					label, labelFound := tt.Find(locales.ReadingLongCues())
					arrow, arrowFound := tt.Find(locales.A11yIconOf(locales.ReadingLongCues()))
					if width == 1800 && !labelFound || !arrowFound || arrow.W <= 0 || labelFound && arrow.X < label.X+label.W {
						t.Fatal("reading category lost its trailing native chevron", label, arrow)
					}
					if err := tt.Click(locales.ReadingIssueType()); err != nil {
						t.Fatal(err)
					}
					if err := tt.ChooseMenuItem(locales.ReadingViewLong(1)); err != nil {
						t.Fatal("expanded category menu is unavailable", err)
					}
					if err := tt.Click(locales.ReadingToolsCollapse()); err != nil {
						t.Fatal(err)
					}
					if _, shown := tt.Find(locales.ReadingTools()); !shown {
						t.Fatal("cannot collapse the tools again")
					}
					if _, shown := tt.Find(locales.ReadingNext()); shown {
						t.Fatal("navigation remained after collapse")
					}
					tt.SetPreferences(ui.Preferences{TextScale: 2, ReduceMotion: true})
					button, found = tt.Find(locales.ReadingTools())
					if !found || button.W <= button.H || button.X < 0 || button.X+button.W > float32(width)+1 {
						t.Fatal("large text clipped the folded label and icon", button)
					}
				})
			}
		})
	}
}

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

func TestSubtitleToolbarActionsMenuIsIconOnly(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := storeApp(t, subtitle.Cue{ID: "a", Text: "First subtitle", StartMs: 0, EndMs: 3000})
			a.readingExpanded = true
			tt := settled(a.subtitlesView, 1800, 800)
			menu, ok := tt.Find(locales.SubtitleActionsMenu())
			if !ok || menu.W > menu.H+2 {
				t.Fatal("subtitle actions menu still shows its label", menu)
			}
			if _, ok := tt.Find(locales.A11yIconOf(locales.SubtitleActionsMenu())); !ok {
				t.Fatal("subtitle actions menu has no icon")
			}
			for _, label := range []string{locales.ListLocate(), locales.TextReplacePeriods(), locales.TextStripTrailing(), locales.FindReplace(), locales.RetranslateSelect(), locales.RetranslateCheckAll(), locales.ReadingSplitCount(0), locales.ReadingLongCues(), locales.TranslationResults()} {
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
			tt.SetSize(320, 800)
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

// A normal middle pane must retain the most-used labels even when review
// and punctuation actions no longer fit with text. Check rendered geometry,
// because an icon's accessible name alone does not prove its label is visible.
func TestSubtitleToolbarKeepsPrimaryLabelsInNormalPanes(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			for _, scale := range []float32{1, 1.5, 2} {
				for _, checked := range []bool{false, true} {
					for _, width := range []int{640, 900, 1100} {
						t.Run(fmt.Sprintf("%s/%g/%t/%d", language, scale, checked, width), func(t *testing.T) {
							a := draftApp(t)
							if checked {
								a.checkAllSubtitles()
								a.retranslation = &retranslationState{}
							}
							tt := ui.NewTester(func(c *ui.Context) {
								applyStudioTheme(c)
								cw, _ := c.Size()
								a.subtitleToolbar(c, cw, true)
							}, int(float32(width)*scale), int(180*scale))
							tt.SetPreferences(ui.Preferences{TextScale: scale, ReduceMotion: true})
							tt.SetDark(true)
							firstFrameSettled(t, tt, "toolbar-normal", func() {})
							for _, label := range []string{locales.ListLocate(), locales.FindReplace(), locales.ReadingTools()} {
								box, ok := tt.Find(label)
								// Bulk selection adds several controls. The new folded
								// entry keeps its requested full name; Find can use its
								// existing named icon in the smallest normal pane.
								mustShowText := label != locales.FindReplace() || !checked || width >= 900
								if !ok || mustShowText && box.W <= 32*scale {
									t.Fatal("primary action became icon-only too early", label, box)
								}
								if _, ok := tt.Find(locales.A11yIconOf(label)); !ok {
									t.Fatal("primary action lost its icon", label)
								}
							}
							edit, _ := tt.Find(locales.SubtitleActionsMenu())
							for _, control := range auditControls(tt) {
								r := control.Bounds
								if r.X < 0 || r.X+r.W > float32(width)*scale+.5 || r.Y+r.H/2 > edit.Y+edit.H/2+1 {
									t.Fatal("toolbar wrapped or clipped", control.Name, r, edit)
								}
							}
							captureRedesign(t, tt, fmt.Sprintf("toolbar-normal-%s-%g-%t-%d", language, scale, checked, width))
						})
					}
				}
			}
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
						a.readingExpanded = true
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
								applyStudioTheme(c)
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
								if !ok || r.X+r.W > float32(w)*scale || math.Abs(float64(r.Y+r.H/2-edit.Y-edit.H/2)) > 1 {
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
