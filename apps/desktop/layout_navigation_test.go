package main

import (
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// The workbench has two independently stacked work columns and one
// full-height workflow column. Ratios apply when minimum controls fit.
func TestUpdatedPaneOrderAndRatio(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := nativeForTest(t)
			tt := settled(a.view, 2520, 1100)
			player, _ := tt.Find(locales.A11yPlayer())
			style, _ := tt.Find(locales.A11yStylePanel())
			editor, _ := tt.Find(locales.A11yEditorPanel())
			workflow, _ := tt.Find(locales.A11yWorkflowColumn())
			tabs, _ := tt.Find(locales.A11ySubtitleTabs())
			if !(player.X < editor.X && editor.X < workflow.X) || player.X != style.X || player.W != style.W || tabs.Y < player.Y || tabs.Y > player.Y+headerSize || workflow.Y != player.Y || style.Y <= player.Y+player.H || editor.Y <= tabs.Y {
				t.Fatal("three-column workbench order is wrong", player, style, editor, workflow, tabs)
			}
			if math.Abs(float64(editor.W/player.W)-1) > .015 || math.Abs(float64(workflow.W/player.W)-1.0/3) > .015 {
				t.Fatal("column widths are not 3:3:1", player, editor, workflow)
			}
			listHeight := editor.Y - player.Y - 1
			if math.Abs(float64(player.H/style.H)-3) > .015 || math.Abs(float64(listHeight/editor.H)-3) > .015 || math.Abs(float64(workflow.H-player.H-style.H-1)) > 1 {
				t.Fatal("independent upper/lower shares are not 3:1", player, style, editor, workflow)
			}
			captureRedesign(t, tt, "three-columns-"+language)
		})
	}
}

// Nested splits must still reserve the actual labels at the minimum window.
// Text measurement can produce fractional widths at the clamp boundary.
func TestCompactWorkflowLabelsRemainWhole(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		for _, scale := range []float32{1, 1.5, 2} {
			t.Run(fmt.Sprintf("%s/%g", language, scale), func(t *testing.T) {
				inLocale(t, language, func() {
					a := nativeForTest(t)
					var titleWidth, commandWidth float32
					label := locales.WorkflowStartRecognize()
					tt := ui.NewTester(func(c *ui.Context) {
						a.view(c)
						titleWidth, _ = c.MeasureText(0, ui.Span{Text: locales.TranslationOptimizeTimeline(), Weight: 500})
						commandWidth, _ = c.MeasureText(0, ui.Span{Text: label})
					}, int(900*scale), int(720*scale))
					tt.SetPreferences(ui.Preferences{TextScale: scale, ReduceMotion: true})
					for tab := 0; tab < 2; tab++ {
						a.workflowTab = tab
						if tab == 1 {
							label = locales.WorkflowStartTranslate()
						}
						tt.Frame()
						command, ok := tt.Find(label)
						if !ok || command.W+1 < commandWidth+56*scale {
							t.Fatal("workflow command text is truncated", label, command, commandWidth)
						}
						insideRedesign(t, tt, locales.A11yWorkflowColumn(), locales.WorkflowRecognize(), locales.WorkflowTranslate())
						if tab == 1 {
							choice, _ := tt.Find(locales.TranslationOptimizeTimeline())
							if choice.W+.01 < titleWidth+24*scale+2 {
								t.Fatal("timeline title cannot fit on one line", choice, titleWidth)
							}
						}
					}
				})
			})
		}
	}
}

// Temporary viewport clamps never overwrite preferences. Each of the four
// native dividers is independently draggable and persists its own share.
func TestPaneRatiosSurviveWindowResize(t *testing.T) {
	a := nativeForTest(t)
	tt := settled(a.view, 2520, 1100)
	before := [4]float32{a.splitX, a.editorSplit, a.splitY, a.subtitleSplit}
	tt.SetSize(900, 640)
	tt.SetSize(2520, 1100)
	tt.Advance(time.Second)
	if got := [4]float32{a.splitX, a.editorSplit, a.splitY, a.subtitleSplit}; got != before {
		t.Fatal("temporary viewport overwrote pane ratios", before, got)
	}
	for divider := 0; divider < 4; divider++ {
		player, _ := tt.Find(locales.A11yPlayer())
		style, _ := tt.Find(locales.A11yStylePanel())
		editor, _ := tt.Find(locales.A11yEditorPanel())
		workflow, _ := tt.Find(locales.A11yWorkflowColumn())
		x, y, dx, dy := player.X+player.W+.5, player.Y+player.H/2, float32(45), float32(0)
		switch divider {
		case 1:
			x = workflow.X - .5
		case 2:
			x, y, dx, dy = player.X+player.W/2, style.Y-.5, 0, -45
		case 3:
			x, y, dx, dy = editor.X+editor.W/2, editor.Y-.5, 0, -45
		}
		old := [4]float32{a.splitX, a.editorSplit, a.splitY, a.subtitleSplit}
		tt.Press(x, y)
		tt.Move(x+dx, y+dy)
		tt.Release(x+dx, y+dy)
		tt.Advance(time.Second)
		got := [4]float32{a.splitX, a.editorSplit, a.splitY, a.subtitleSplit}
		for i := range got {
			if (i == divider && got[i] == old[i]) || (i != divider && got[i] != old[i]) {
				t.Fatal("divider did not resize independently", divider, old, got)
			}
		}
		prefs, err := settings.Load(a.dataRoot + "/settings.json")
		if err != nil || prefs.PlayerWidth != float64(got[0]) || prefs.EditorWidth != float64(got[1]) || prefs.TopHeight != float64(got[2]) || prefs.SubtitleHeight != float64(got[3]) {
			t.Fatal("native divider drag was not persisted", got, prefs, err)
		}
	}
}

// A downward divider drag can fold either inspector to its header. Growing
// it back must restore the same draft, style and saved divider preference.
func TestInspectorsFoldToTitleAndRestore(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			for _, scale := range []float32{1, 1.5, 2} {
				for _, panel := range []string{locales.A11yStylePanel(), locales.A11yEditorPanel()} {
					t.Run(fmt.Sprintf("%s/%g/%s", language, scale, panel), func(t *testing.T) {
						a := withStyleCatalog(t)
						draft, style := a.draft, a.currentStyle
						tt := settled(a.view, int(1800*scale), int(1000*scale))
						tt.SetPreferences(ui.Preferences{TextScale: scale, ReduceMotion: true})
						tt.Frame()
						before, _ := tt.Find(panel)
						x, y := before.X+before.W/2, before.Y-.5
						tt.Press(x, y)
						tt.Move(x, y+2000*scale)
						tt.Release(x, y+2000*scale)
						folded, _ := tt.Find(panel)
						if math.Abs(float64(folded.H-(headerSize*scale+1))) > 1 {
							t.Fatal("inspector cannot fold to its title", folded)
						}
						content := locales.OutputContent()
						if panel == locales.A11yEditorPanel() {
							content = locales.A11yCueText()
						}
						if _, shown := tt.Find(content); shown {
							t.Fatal("folded inspector still exposes hidden controls", content)
						}
						tt.Advance(time.Second)
						prefs, err := settings.Load(a.dataRoot + "/settings.json")
						if err != nil || float32(prefs.TopHeight) != a.splitY || float32(prefs.SubtitleHeight) != a.subtitleSplit {
							t.Fatal("folded height was lost on reload", prefs.TopHeight, prefs.SubtitleHeight, err)
						}
						firstFrameSettled(t, tt, "folded-inspector", func() {})
						captureRedesign(t, tt, fmt.Sprintf("folded-%s-%s-%g", panel, language, scale))
						tt.Press(x, folded.Y-.5)
						tt.Move(x, before.Y-.5)
						tt.Release(x, before.Y-.5)
						if restored, _ := tt.Find(panel); math.Abs(float64(restored.H-before.H)) > 1 || a.draft != draft || !reflect.DeepEqual(a.currentStyle, style) {
							t.Fatal("expanding changed inspector content or height", before, restored)
						}
						if _, shown := tt.Find(content); !shown {
							t.Fatal("expanding did not restore inspector controls", content)
						}
					})
				}
			}
		})
	}
}

// Check the rendered button, not just its helper name: the original blue
// accent and white content stay in the fixed workflow band in both appearances.
func TestOneClickActionStaysAboveWorkflowTabs(t *testing.T) {
	for language, label := range map[string]string{"zh-Hans": "一键识别并翻译", "en": "One-click transcribe & translate"} {
		for _, width := range []int{900, 1800} {
			for _, dark := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%d/dark%t", language, width, dark), func(t *testing.T) {
					inLocale(t, language, func() {
						a := draftApp(t)
						a.snapshot.Manifest.MediaPath = "test.mp4"
						tt := settled(a.view, width, 1000)
						tt.SetDark(dark)
						button, ok := tt.Find(label)
						bar, _ := tt.Find(locales.A11yToolbar())
						column, _ := tt.Find(locales.A11yWorkflowColumn())
						tabs, _ := tt.Find(locales.WorkflowTitle())
						if !ok || math.Abs(float64(button.X+button.W/2-column.X-column.W/2)) > 1 || button.Y < bar.Y+bar.H || button.Y+button.H >= tabs.Y {
							t.Fatal("one-click action is not above the workflow tabs", label, button, column, tabs)
						}
						tt.Scroll(column.X+column.W/2, column.Y+column.H-50, 0, 2000)
						if after, _ := tt.Find(label); after != button {
							t.Fatal("one-click action scrolled with workflow fields", button, after)
						}
						for _, name := range []string{locales.ActionAddFiles(), locales.ActionClearWorkbench(), locales.ActionSettings(), locales.MenuExportSubtitles(), locales.ActionCompose()} {
							r, found := tt.Find(name)
							if !found || r.W <= 0 || r.X < bar.X || r.X+r.W > bar.X+bar.W || r.Y < bar.Y || r.Y+r.H > bar.Y+bar.H {
								t.Fatal("toolbar action clipped", name, r, bar)
							}
							if r.X < button.X+button.W && r.X+r.W > button.X && r.Y < button.Y+button.H && r.Y+r.H > button.Y {
								t.Fatal("toolbar action overlaps the primary action", name, r, button)
							}
						}
						primaryPixels, white := 0, 0
						accent := ui.Hex("#2563eb")
						image := tt.Image()
						for y := int(button.Y); y < int(button.Y+button.H); y++ {
							for x := int(button.X); x < int(button.X+button.W); x++ {
								p := image.RGBAAt(x, y)
								if p.R == accent.R && p.G == accent.G && p.B == accent.B {
									primaryPixels++
								}
								if p.R == 255 && p.G == 255 && p.B == 255 {
									white++
								}
							}
						}
						if primaryPixels < int(button.W*button.H)/4 || white < 20 {
							t.Fatal("primary button does not use the selected theme's accent and white text", primaryPixels, white, button)
						}
						if err := tt.Click(label); err != nil {
							t.Fatal(err)
						}
						if !a.settingsOpen || a.settingsTab != 2 || a.busy {
							t.Fatal("workflow action did not keep the one-click setup check")
						}
					})
				})
			}
		}
	}
}

func navigationCues(count int) []subtitle.Cue {
	cues := make([]subtitle.Cue, count)
	for i := range cues {
		text := fmt.Sprintf("Subtitle %d", i+1)
		if i%3 == 0 {
			text += " match"
		}
		if i%5 == 0 {
			text += "\nSecond line\nThird line"
		}
		cues[i] = subtitle.Cue{ID: fmt.Sprint("cue-", i), Position: i, Text: text, StartMs: int64(i * 1000), EndMs: int64(i*1000 + 900)}
	}
	return cues
}

// Navigation is a viewport command on either track, not a selection,
// playback or edit command. A paused video leaves the viewport where requested.
func TestSubtitleScrollButtonsPreserveState(t *testing.T) {
	for _, track := range []int{0, 1} {
		t.Run(fmt.Sprint(track), func(t *testing.T) {
			cues := navigationCues(150)
			if track == 1 {
				for i := range cues {
					cues[i].Translated, cues[i].TranslationSource = true, cues[i].Text
				}
			}
			a := storeApp(t, cues...)
			a.subtitleTab, a.selected, a.activeCue = track, 30, 30
			a.position, a.paused = 30.2, true
			a.loadDraft()
			a.draft, a.editPending, a.editing = "Uncommitted draft", true, true
			tt := settled(a.subtitlesView, 700, 500)
			for _, label := range []string{"返回底部", "返回顶部"} {
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
				tt.Advance(subtitleNavigationAnimation.Duration + 20*time.Millisecond)
				first, last := a.list.Visible()
				if label == "返回顶部" && first != 0 || label == "返回底部" && (last != 149 || !a.list.AtEnd()) {
					t.Fatal("navigation did not reach the list edge", label, first, last)
				}
				if a.selected != 30 || a.position != 30.2 || !a.paused || a.draft != "Uncommitted draft" || !a.editPending {
					t.Fatal("navigation changed selection, playback or the draft")
				}
			}
			state, err := a.store.Snapshot()
			if err != nil || state.Revision != 0 {
				t.Fatal("scrolling wrote the project", err)
			}
			a.position = 31.2
			tt.Frame()
			if first, _ := a.list.Visible(); first != 0 {
				t.Fatal("paused playback undid manual navigation", first)
			}
		})
	}
}

func TestSubtitleNavigationUsesFilteredLongList(t *testing.T) {
	a := nativeForTest(t)
	a.snapshot = &project.Snapshot{Document: subtitle.Document{Format: "srt", Cues: navigationCues(20000)}}
	a.query = "match"
	tt := settled(a.subtitlesView, 640, 450)
	if err := tt.Click("返回底部"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(subtitleNavigationAnimation.Duration + 20*time.Millisecond)
	if _, last := a.list.Visible(); last != 6666 || !a.list.AtEnd() || !tt.HasText("Subtitle 19999 match") {
		t.Fatal("bottom did not use filtered rows", last, tt.Texts())
	}
	if err := tt.Click("返回顶部"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(subtitleNavigationAnimation.Duration + 20*time.Millisecond)
	if first, _ := a.list.Visible(); first != 0 || !tt.HasText("Subtitle 1 match") {
		t.Fatal("top did not use filtered rows", first)
	}
	a.query = "no matches"
	tt.Frame()
	for _, label := range []string{"返回顶部", "返回底部"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if first, last := a.list.Visible(); last >= first || a.selected != -1 {
		t.Fatal("empty search still navigated", first, last)
	}
}

func TestSubtitleNavigationButtonsCenteredAndStayFixed(t *testing.T) {
	for language, labels := range map[string][]string{"zh-Hans": {"返回顶部", "返回底部"}, "en": {"Back to top", "Go to bottom"}} {
		for _, width := range []int{320, 640} {
			for _, scale := range []float32{1, 1.5, 2} {
				t.Run(fmt.Sprintf("%s/%d/%g", language, width, scale), func(t *testing.T) {
					inLocale(t, language, func() {
						a := nativeForTest(t)
						a.snapshot = &project.Snapshot{Document: subtitle.Document{Format: "srt", Cues: navigationCues(120)}}
						tt := settled(func(c *ui.Context) {
							theme, _ := studioTheme(c.Theme(), false)
							c.SetTheme(&theme)
							a.subtitlesView(c)
						}, width, 800)
						tt.SetPreferences(ui.Preferences{TextScale: scale})
						list, _ := tt.Find(locales.A11ySubtitleList())
						before := make([]ui.Rect, len(labels))
						for i, label := range labels {
							r, ok := tt.Find(label)
							if !ok || r.W < 32*scale-1 || r.H < 32*scale-1 || r.X < list.X+list.W-subtitleNavigationRailSize*scale || r.X+r.W > float32(width)-6*scale || r.Y < list.Y || r.Y+r.H > list.Y+list.H {
								t.Fatal("floating button outside list or too small", label, r, list)
							}
							before[i] = r
						}
						if before[0].Y+before[0].H > before[1].Y || before[0].X != before[1].X {
							t.Fatal("floating buttons are not a vertical group", before)
						}
						if math.Abs(float64((before[0].Y+before[1].Y+before[1].H)/2-list.Y-list.H/2)) > 1 {
							t.Fatal("floating buttons are not vertically centered in the list", before, list)
						}
						if err := tt.Click(labels[1]); err != nil {
							t.Fatal(err)
						}
						tt.Frame()
						for i, label := range labels {
							if after, _ := tt.Find(label); after != before[i] {
								t.Fatal("floating button moved with list rows", label, before[i], after)
							}
						}
						// Resizing the viewport must recenter the group, not retain
						// a fixed vertical offset from the previous layout.
						tt.SetSize(width, 1000)
						list, _ = tt.Find(locales.A11ySubtitleList())
						top, _ := tt.Find(labels[0])
						bottom, _ := tt.Find(labels[1])
						if math.Abs(float64((top.Y+bottom.Y+bottom.H)/2-list.Y-list.H/2)) > 1 {
							t.Fatal("floating buttons did not recenter after resizing", top, bottom, list)
						}
					})
				})
			}
		}
	}
}

// The same controls remain available while translations stream. Navigation
// is temporary; later completed rows still follow without a resume command.
func TestTranslationProgressNavigation(t *testing.T) {
	a := nativeForTest(t)
	a.subtitleTab, a.translationRunning = 1, true
	a.translationLive = newTranslationLive()
	l := a.translationLive
	l.units, l.latest, l.stage = navigationCues(120), 75, "translate"
	tt := settled(a.subtitlesView, 700, 650)
	list, _ := tt.Find(locales.TranslationLive())
	top, _ := tt.Find(locales.ListTop())
	bottom, _ := tt.Find(locales.ListBottom())
	if math.Abs(float64((top.Y+bottom.Y+bottom.H)/2-list.Y-list.H/2)) > 1 {
		t.Fatal("live translation navigation is not vertically centered", top, bottom, list)
	}
	if err := tt.Click("返回顶部"); err != nil {
		t.Fatal(err)
	}
	l.latest = 76
	tt.Advance(time.Second)
	if first, _ := l.list.Visible(); first != 0 {
		t.Fatal("translation progress stole the browsing viewport", first)
	}
	tt.Advance(browseGrace + time.Second)
	if first, last := l.list.Visible(); first == 0 || first > 76 || last < 76 {
		t.Fatal("translation updates stopped following after navigation", first, last)
	}
	if err := tt.Click("返回底部"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(subtitleNavigationAnimation.Duration + 20*time.Millisecond)
	if _, last := l.list.Visible(); last != 119 || !l.list.AtEnd() {
		t.Fatal("live translation did not reach bottom", last)
	}
}
