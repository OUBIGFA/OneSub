package main

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

func TestStatusBarCountsActualTracksAndSurvivesTasks(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := storeApp(t,
				subtitle.Cue{ID: "a", Text: "First complete subtitle", StartMs: 0, EndMs: 2000},
				subtitle.Cue{ID: "b", Text: "Second complete subtitle", StartMs: 2000, EndMs: 4000},
				subtitle.Cue{ID: "c", Text: "Third complete subtitle", StartMs: 4000, EndMs: 6000})
			footer := settled(func(c *ui.Context) { a.statusBar(c, studioColors(c)) }, 900, 32)
			if !footer.HasText(locales.StatusOriginalCount(3)) || footer.HasText(locales.StatusOptimizedCount(3)) {
				t.Fatal("counted the store mirror as an optimized result", footer.Texts())
			}
			candidate, _, err := subtitle.Resegment(a.snapshot.Document, []subtitle.SourceRange{{FirstAtom: "a", LastAtom: "b", Reason: "complete phrase"}, {FirstAtom: "c", LastAtom: "c"}})
			if err != nil {
				t.Fatal(err)
			}
			for i := range candidate.Cues {
				cue := &candidate.Cues[i]
				cue.TranslationSource, cue.Translated = cue.Text, true
				cue.Text = fmt.Sprintf("完整译文 %d", i+1)
			}
			if _, err = a.store.ApplyTranslationDocument("optimized", a.snapshot.Revision, candidate); err != nil {
				t.Fatal(err)
			}
			a.refreshAfterStructure(0)
			for _, tab := range []int{0, 1} {
				a.changeSubtitleTab(tab)
				a.query = "No matching subtitles"
				a.busy, a.translationRunning, a.errorText, a.status = true, true, "Test error", "Previous result"
				a.translationLive = newTranslationLive()
				a.translationLive.units = []subtitle.Cue{{Text: "Pending preview"}}
				footer.Frame()
				if !footer.HasText(locales.StatusOriginalCount(3)) || !footer.HasText(locales.StatusOptimizedCount(2)) {
					t.Fatal("task, error, tab or search hid the counts", footer.Texts())
				}
				footer.Advance(errorDuration + time.Second)
				if !footer.HasText(locales.StatusOriginalCount(3)) || !footer.HasText(locales.StatusOptimizedCount(2)) {
					t.Fatal("error timeout cleared persistent counts")
				}
				a.busy, a.translationRunning, a.translationLive = false, false, nil
			}
			a.query, a.status = "", ""
			a.changeSubtitleTab(1)
			a.deleteSubtitle(a.activeDocument().Cues[0].ID)
			footer.Frame()
			if !footer.HasText(locales.StatusOptimizedCount(1)) || !footer.HasText(locales.StatusOriginalCount(3)) {
				t.Fatal("deletion did not refresh independent counts")
			}
			a.undo(false)
			footer.Frame()
			if !footer.HasText(locales.StatusOptimizedCount(2)) {
				t.Fatal("undo did not refresh counts")
			}
			if _, err = a.store.InsertCue("insert", a.snapshot.Revision, false, project.Insertion{Position: 0, Source: "New source", Translation: "新译文", StartMs: 0, EndMs: 1000}); err != nil {
				t.Fatal(err)
			}
			a.refreshAfterStructure(0)
			footer.Frame()
			if !footer.HasText(locales.StatusOptimizedCount(3)) || !footer.HasText(locales.StatusOriginalCount(3)) {
				t.Fatal("insertion did not refresh counts")
			}
			path := a.snapshot.Path
			a.store.Close()
			a.store, err = project.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer a.store.Close()
			state, err := a.store.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			a.setSnapshot(state)
			footer.Frame()
			if !footer.HasText(locales.StatusOriginalCount(3)) || !footer.HasText(locales.StatusOptimizedCount(3)) {
				t.Fatal("reopen lost independent counts")
			}
			a.snapshot = nil
			footer.Frame()
			if footer.HasText(locales.StatusOriginalCount(3)) || footer.HasText(locales.StatusOptimizedCount(3)) || footer.HasText(locales.StatusOriginalCount(0)) {
				t.Fatal("empty workbench kept stale counts")
			}
		})
	}
}

func TestStatusBarCountsFitBeforeOutputInBothThemes(t *testing.T) {
	source, optimized := navigationCues(20000), navigationCues(12345)
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			for _, dark := range []bool{false, true} {
				// Timings, both counts and output share one row at the supported
				// workbench minimum; the retired stacked 320-DIP footer is not used.
				for _, width := range []int{1360, 1600, 2200} {
					for _, scale := range []float32{1, 1.5, 2} {
						t.Run(fmt.Sprintf("%s/dark%t/%d/%g", language, dark, width, scale), func(t *testing.T) {
							a := nativeForTest(t)
							a.snapshot = &project.Snapshot{SourceDocument: subtitle.Document{Cues: source}, Document: subtitle.Document{Cues: optimized}}
							a.query, a.errorText = "No matches", "A long transient error is separate from the permanent subtitle counts"
							a.lastOutput = filepath.Join(a.dataRoot, "result.srt")
							w, h := int(float32(width)*scale), int(32*scale)
							var danger ui.Color
							tt := settled(func(c *ui.Context) {
								base := ui.LightTheme()
								if dark {
									base = ui.DarkTheme()
								}
								theme := *base
								theme.FontSize = 14 * c.Preferences().TextScale
								styled, look := studioTheme(&theme, false)
								c.SetTheme(&styled)
								danger = styled.Danger
								a.statusBar(c, look)
							}, w, h)
							tt.SetPreferences(ui.Preferences{TextScale: scale})
							for range 3 {
								tt.Frame()
							}
							bar, _ := tt.Find(locales.A11yStatusBar())
							folder, ok := tt.Find(locales.StatusOwnerOutput() + locales.ResultOpenFolder())
							if !ok || folder.X+folder.W > bar.X+bar.W+1 {
								t.Fatal("output action does not fit", folder, bar)
							}
							for _, label := range []string{locales.StatusOriginalCount(20000), locales.StatusOptimizedCount(12345)} {
								count, exists := tt.Find(label)
								if !exists || count.X < bar.X || count.X+count.W > folder.X || count.Y < bar.Y-1 || count.Y+count.H > bar.Y+bar.H+1 {
									t.Fatal("count is missing, clipped or not left of output", label, count, folder, bar)
								}
								measured, _ := uiTextMeasureForCount(t, label, scale)
								if count.W+1 < measured {
									t.Fatal("count label was shrunk", count, measured)
								}
								image := tt.Image()
								for y := int(count.Y); y < int(count.Y+count.H); y++ {
									for x := int(count.X); x < int(count.X+count.W); x++ {
										p := image.RGBAAt(x, y)
										if p.R == danger.R && p.G == danger.G && p.B == danger.B {
											t.Fatal("persistent counts use warning color")
										}
									}
								}
							}
							captureRedesign(t, tt, fmt.Sprintf("status-counts-%s-%t-%d-%g", language, dark, width, scale))
						})
					}
				}
			}
		})
	}
}

// Measure the same native text, not an estimated glyph width. The production
// footer and this reference use the same theme/font/features and caption size.
func uiTextMeasureForCount(t *testing.T, label string, scale float32) (float32, float32) {
	t.Helper()
	var w, h float32
	tt := ui.NewTester(func(c *ui.Context) {
		theme := *c.Theme()
		theme.FontSize = 15 * scale
		c.SetTheme(&theme)
		w, h = c.MeasureText(0, ui.Span{Text: label, Size: em(c, captionSize)})
	}, 1000, 100)
	tt.Frame()
	return w, h
}
