package main

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

func TestUpdatedPaneOrderAndRatio(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := nativeForTest(t)
			tt := settled(a.view, 1800, 1000)
			style, _ := tt.Find(locales.A11yStylePanel())
			editor, _ := tt.Find(locales.A11yEditorPanel())
			workflow, _ := tt.Find(locales.WorkflowTitle())
			if !(style.X < editor.X && editor.X < workflow.X) || style.Y != editor.Y || editor.Y != workflow.Y {
				t.Fatal("panes are not style, editor, workflow", style, editor, workflow)
			}
			if math.Abs(float64(editor.W/style.W)-4.0/3) > 0.015 || math.Abs(float64(workflow.W/style.W)-1) > 0.015 {
				t.Fatal("pane widths are not 1.5:2:1.5", style, editor, workflow)
			}
		})
	}
}

// A temporary narrow viewport may clamp the displayed panes, but must not
// overwrite the chosen ratios. This also covers intermediate startup sizes.
func TestPaneRatiosSurviveWindowResize(t *testing.T) {
	a := nativeForTest(t)
	tt := settled(a.view, 1800, 1000)
	styleShare, editorShare := a.styleSplit, a.editorSplit
	tt.SetSize(900, 640)
	tt.SetSize(1800, 1000)
	tt.Advance(time.Second)
	if a.styleSplit != styleShare || a.editorSplit != editorShare {
		t.Fatal("temporary minimum width overwrote pane ratios", styleShare, editorShare, a.styleSplit, a.editorSplit)
	}
	style, _ := tt.Find(locales.A11yStylePanel())
	editor, _ := tt.Find(locales.A11yEditorPanel())
	workflow, _ := tt.Find(locales.WorkflowTitle())
	if math.Abs(float64(editor.W/style.W)-4.0/3) > .015 || math.Abs(float64(workflow.W/style.W)-1) > .015 {
		t.Fatal("expanded window did not restore 1.5:2:1.5", style, editor, workflow)
	}
	// A real pointer drag, unlike resizing, changes and saves the proportion.
	x, y := style.X+style.W+0.5, style.Y+style.H/2
	tt.Press(x, y)
	tt.Move(x+60, y)
	tt.Release(x+60, y)
	tt.Advance(time.Second)
	prefs, err := settings.Load(a.dataRoot + "/settings.json")
	if err != nil || a.styleSplit <= styleShare || prefs.StyleWidth != float64(a.styleSplit) {
		t.Fatal("native divider drag was not persisted", a.styleSplit, prefs.StyleWidth, err)
	}
}

// Check the rendered button, not just its helper name: both appearances
// must show the blue face and white label, centered between unequal groups.
func TestOneClickToolbarCenteredAndPrimary(t *testing.T) {
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
						if !ok || math.Abs(float64(button.X+button.W/2-bar.X-bar.W/2)) > 1 {
							t.Fatal("one-click action is not centered in the toolbar", label, button, bar)
						}
						for _, name := range []string{locales.ActionAddFiles(), locales.ActionClearWorkbench(), locales.ActionSettings(), locales.MenuExportSubtitles(), locales.ActionCompose()} {
							r, found := tt.Find(name)
							if !found || r.W <= 0 || r.X < bar.X || r.X+r.W > bar.X+bar.W || r.Y < bar.Y || r.Y+r.H > bar.Y+bar.H {
								t.Fatal("toolbar action clipped", name, r, bar)
							}
							if r.X < button.X+button.W && r.X+r.W > button.X {
								t.Fatal("toolbar action overlaps the primary action", name, r, button)
							}
						}
						blue, white := 0, 0
						image := tt.Image()
						for y := int(button.Y); y < int(button.Y+button.H); y++ {
							for x := int(button.X); x < int(button.X+button.W); x++ {
								p := image.RGBAAt(x, y)
								if p.R == 0x25 && p.G == 0x63 && p.B == 0xeb {
									blue++
								}
								if p.R == 255 && p.G == 255 && p.B == 255 {
									white++
								}
							}
						}
						if blue < int(button.W*button.H)/4 || white < 20 {
							t.Fatal("primary button is not blue with white content", blue, white, button)
						}
						if err := tt.Click(label); err != nil {
							t.Fatal(err)
						}
						if !a.settingsOpen || a.settingsTab != 2 || a.busy {
							t.Fatal("centered action did not keep the one-click setup check")
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
			a := storeApp(t, navigationCues(150)...)
			a.subtitleTab, a.selected, a.activeCue = track, 30, 30
			a.position, a.paused = 30.2, true
			a.loadDraft()
			a.draft, a.editPending, a.editing = "Uncommitted draft", true, true
			tt := settled(a.subtitlesView, 700, 500)
			for _, label := range []string{"返回底部", "返回顶部"} {
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
				tt.Frame()
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
	tt.Frame()
	if _, last := a.list.Visible(); last != 6666 || !a.list.AtEnd() || !tt.HasText("Subtitle 19999 match") {
		t.Fatal("bottom did not use filtered rows", last, tt.Texts())
	}
	if err := tt.Click("返回顶部"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
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

func TestSubtitleNavigationButtonsFitAndStayFixed(t *testing.T) {
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
							if !ok || r.W < 32*scale-1 || r.H < 32*scale-1 || r.X < list.X+list.W/2 || r.X+r.W > list.X+list.W || r.Y < list.Y || r.Y+r.H > list.Y+list.H {
								t.Fatal("floating button outside list or too small", label, r, list)
							}
							before[i] = r
						}
						if before[0].Y+before[0].H > before[1].Y || before[0].X != before[1].X {
							t.Fatal("floating buttons are not a vertical group", before)
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
	if err := tt.Click("返回顶部"); err != nil {
		t.Fatal(err)
	}
	l.latest = 76
	tt.Advance(time.Second)
	if first, last := l.list.Visible(); first == 0 || first > 76 || last < 76 {
		t.Fatal("translation updates stopped following after navigation", first, last)
	}
	if err := tt.Click("返回底部"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, last := l.list.Visible(); last != 119 || !l.list.AtEnd() {
		t.Fatal("live translation did not reach bottom", last)
	}
}
