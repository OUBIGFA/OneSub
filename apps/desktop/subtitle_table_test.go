package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// Exercise the actual navigation rail and virtual table. A distant jump must
// still have a visible landing, including the bottom of an oversized last row.
func TestSubtitleNavigationEasesToRest(t *testing.T) {
	duration := subtitleNavigationAnimation.Duration
	for _, toEnd := range []bool{false, true} {
		for _, tallLast := range []bool{false, true} {
			t.Run(fmt.Sprintf("end=%t/tall=%t", toEnd, tallLast), func(t *testing.T) {
				nativeForTest(t) // Load the same licensed icons as the workbench.
				var state ui.ListState
				const count = 20000
				jump := count / 2
				tt := settled(func(c *ui.Context) {
					ui.Column(c).Fill().Children(func() {
						subtitleTable(c, &state, []ui.TableColumn{{Title: "Subtitle"}}, count, func(row, col int) {
							height := float32(32 + row%3*12)
							if tallLast && row == count-1 {
								height = 720
							}
							// A small label at the cell's bottom stays measurable even
							// when the tall cell itself covers the whole viewport.
							ui.Column(c).Height(height).Justify(ui.End).Children(func() {
								ui.Text(c, fmt.Sprint("Row ", row))
							})
						}, func(bool) {}).Label("Navigation table")
						if jump >= 0 {
							state.ScrollTo(jump, ui.Center)
							jump = -1
						}
					})
				}, 600, 400)
				label, target, sign := locales.ListTop(), "Row 1", float32(1)
				if toEnd {
					label, target, sign = locales.ListBottom(), fmt.Sprint("Row ", count-1), -1
				}
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
				first, _ := state.Visible()
				if !state.Scrolling() || toEnd && state.AtEnd() || !toEnd && first == 0 {
					t.Fatal("navigation snapped to its destination without feedback", first, state.AtEnd())
				}
				tt.Advance(duration / 2)
				early, ok := tt.Find(target)
				if !ok || !state.Scrolling() {
					t.Fatal("landing ended before its slow tail", target)
				}
				tt.Advance(duration / 4)
				middle, ok := tt.Find(target)
				if !ok || !state.Scrolling() {
					t.Fatal("landing lost its destination", target)
				}
				tt.Advance(duration/4 + 20*time.Millisecond)
				after, ok := tt.Find(target)
				first, last := state.Visible()
				if !ok || state.Scrolling() || toEnd && (!state.AtEnd() || last != count-1) || !toEnd && first != 0 {
					t.Fatal("navigation did not settle at the exact edge", first, last)
				}
				fast, slow := sign*(middle.Y-early.Y), sign*(after.Y-middle.Y)
				if !(fast > slow && slow > 0) {
					t.Fatal("landing did not decelerate toward rest", fast, slow)
				}
			})
		}
	}
}
