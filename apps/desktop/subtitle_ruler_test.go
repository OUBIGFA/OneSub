package main

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// The reference is a viewport overlay: dragging it must not move table columns,
// select a subtitle or disappear when virtual rows scroll underneath it.
func TestSubtitleLengthGuideMovesWithoutChangingRows(t *testing.T) {
	a := storeApp(t, navigationCues(160)...)
	tt := settled(a.subtitlesView, 900, 680)
	if _, exists := tt.Find("字幕长度参考线"); exists {
		t.Fatal("length guide is visible before it is enabled")
	}
	if err := tt.Click("长度参考线"); err != nil {
		t.Fatal("second toolbar row has no length-guide action", err)
	}
	guide, exists := tt.Find("字幕长度参考线")
	table, _ := tt.Find(locales.A11ySubtitleList())
	if !exists || guide.X < table.X || guide.X+guide.W > table.X+table.W || guide.Y < table.Y || guide.Y+guide.H > table.Y+table.H+1 {
		t.Fatal("length guide is outside the subtitle list", guide, table)
	}
	selected, revision := a.selected, a.snapshot.Revision
	x, y := guide.X+guide.W/2, guide.Y+guide.H/2
	tt.Move(x, y)
	tt.Press(x, y)
	tt.Advance(20 * time.Millisecond)
	tt.Move(x+90, y)
	tt.Advance(20 * time.Millisecond)
	tt.Release(x+90, y)
	tt.Frame()
	moved, _ := tt.Find("字幕长度参考线")
	if delta := moved.X - guide.X; delta < 89 || delta > 91 || moved.Y != guide.Y || moved.H != guide.H {
		t.Fatal("length guide did not follow the horizontal drag", guide, moved)
	}
	if a.selected != selected || a.snapshot.Revision != revision {
		t.Fatal("dragging the reference selected or edited a subtitle")
	}
	if after, _ := tt.Find(locales.A11ySubtitleList()); after != table {
		t.Fatal("dragging the reference resized the subtitle table", table, after)
	}
	tt.Scroll(table.X+table.W/2, table.Y+table.H/2, 0, 700)
	if after, _ := tt.Find("字幕长度参考线"); after != moved {
		t.Fatal("length guide moved with the subtitle rows", moved, after)
	}
	if err := tt.Click("长度参考线"); err != nil {
		t.Fatal(err)
	}
	if _, exists := tt.Find("字幕长度参考线"); exists {
		t.Fatal("length guide did not hide when switched off")
	}
	if err := tt.Click("长度参考线"); err != nil {
		t.Fatal(err)
	}
	if after, _ := tt.Find("字幕长度参考线"); after != moved {
		t.Fatal("toggling the reference lost its chosen position", moved, after)
	}
}

func TestSubtitleLengthGuideStaysAtToolbarRightAndUsesPinkNativeLine(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			for _, dark := range []bool{false, true} {
				for _, width := range []int{356, 400, 900} {
					t.Run(fmt.Sprintf("%s/%t/%d", language, dark, width), func(t *testing.T) {
						a := storeApp(t, navigationCues(60)...)
						tt := settled(func(c *ui.Context) { applyStudioTheme(c); a.subtitlesView(c) }, width, 680)
						tt.SetDark(dark)
						tt.Frame()
						label, lineLabel := "长度参考线", "字幕长度参考线"
						if language == "en" {
							label, lineLabel = "Length guide", "Subtitle length guide"
						}
						button, found := tt.Find(label)
						if !found || button.W > 34 || button.X+button.W > float32(width) || button.X+button.W < float32(width)-14 {
							t.Fatal("length-guide action is not an icon at the toolbar's right edge", button)
						}
						if err := tt.Click(label); err != nil {
							t.Fatal(err)
						}
						line, _ := tt.Find(lineLabel)
						pink := 0
						image := tt.Image()
						for y := int(line.Y) + 6; y < int(line.Y+line.H)-6; y++ {
							for x := int(line.X); x < int(line.X+line.W); x++ {
								p := image.RGBAAt(x, y)
								if p.R > 180 && p.G < 140 && p.B > 100 {
									pink++
									break
								}
							}
						}
						if pink < int(line.H)-30 {
							t.Fatal("reference is not a continuous pink line", line, pink)
						}
						before := append([]byte(nil), image.Pix...)
						tt.Frame()
						if !bytes.Equal(before, tt.Image().Pix) {
							t.Fatal("length reference required a second layout frame")
						}
						captureRedesign(t, tt, fmt.Sprintf("subtitle-ruler-%s-%t-%d", language, dark, width))
					})
				}
			}
		})
	}
}
