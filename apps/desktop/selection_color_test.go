package main

import (
	"fmt"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// Look at the rendered glyphs, not just the row's inherited style: an explicit
// muted color on any descendant can override the selected row's foreground.
func assertSelectedText(t *testing.T, tt *ui.Tester, labels ...string) {
	t.Helper()
	for _, label := range labels {
		r, ok := tt.Find(label)
		if !ok {
			t.Errorf("selected text is missing: %q", label)
			continue
		}
		white := 0
		pixels := tt.Image()
		for y := int(r.Y); y < int(r.Y+r.H); y++ {
			for x := int(r.X); x < int(r.X+r.W); x++ {
				p := pixels.RGBAAt(x, y)
				if p.R > 220 && p.G > 220 && p.B > 220 {
					white++
				}
			}
		}
		if white == 0 {
			t.Errorf("selected text does not use the white row foreground: %q", label)
		}
	}
}

func TestTranslationSelectionKeepsEveryColumnReadable(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, scale := range []float32{1, 1.5, 2} {
			t.Run(fmt.Sprintf("dark%t/%g", dark, scale), func(t *testing.T) {
				a := nativeForTest(t)
				a.translationLive = newTranslationLive()
				live := a.translationLive
				live.stage, live.total, live.thinking = "translate", 2, "Working"
				live.units = []subtitle.Cue{
					{ID: "first", Text: "First source", StartMs: 1250, EndMs: 3000},
					{ID: "second", Text: "Second source", StartMs: 4500, EndMs: 7000},
				}
				live.texts[0], live.final[0] = "已完成译文", true
				live.texts[1] = "正在生成的译文"
				tt := settled(func(c *ui.Context) {
					applyStudioTheme(c)
					ui.Column(c).Fill().Children(func() { a.translationProgressView(c) })
				}, int(900*scale), int(400*scale))
				tt.SetDark(dark)
				tt.SetPreferences(ui.Preferences{TextScale: scale})
				for _, stopped := range []string{"", "Interrupted"} {
					live.stopped = stopped
					for _, index := range []int{0, 1} {
						a.query = ""
						tt.Frame()
						if err := tt.Click(live.units[index].Text); err != nil {
							t.Fatal(err)
						}
						if live.selected != index {
							t.Fatal("row selection did not reach the live translation", live.selected)
						}
						assertSelectedText(t, tt, fmt.Sprint(index+1), stamp(float64(live.units[index].StartMs)/1000), live.units[index].Text, live.texts[index])
					}
					// The selected canonical cue is now visible row zero. Selection
					// styling must follow the filtered native row, not its old index.
					a.query = "Second source"
					tt.Frame()
					assertSelectedText(t, tt, "2", "00:00:04.500", "Second source", live.texts[1])
				}
			})
		}
	}
}

func TestRetranslationSelectionKeepsSecondaryTextReadable(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(fmt.Sprint(dark), func(t *testing.T) {
			a := checkedReviewApp(t)
			r := a.retranslation
			r.rows[0].result = &retranslationResult{text: "A translated candidate", valid: true}
			tt := settled(func(c *ui.Context) {
				applyStudioTheme(c)
				a.retranslationView(c)
			}, 1600, 1000)
			tt.SetDark(dark)
			if err := tt.Click("First source"); err != nil {
				t.Fatal(err)
			}
			assertSelectedText(t, tt, "# 1", "00:00:00.000 – 00:00:04.000", r.rows[0].status(), locales.TabSource(), "First source", locales.RetranslateCandidate(), "A translated candidate", locales.RetranslateExisting(), "原有人工译文")
		})
	}
}
