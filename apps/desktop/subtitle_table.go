package main

import (
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// subtitleTable keeps native navigation buttons fixed beside both subtitle
// tables. A small gutter keeps them off the text and scrollbar; KeepFocus
// lets a pointer click scroll without committing an editor's unfinished draft.
func subtitleTable(c *ui.Context, state *ui.ListState, columns []ui.TableColumn, count int, cell func(int, int), onNavigate func(toEnd bool)) ui.Element {
	var table ui.Element
	ui.Box(c).Grow(1).Children(func() {
		table = ui.Table(c, state, columns, count, cell).Fill().Padding(0, em(c, 48), 0, 0)
		ui.Column(c).Absolute().Right(em(c, 8)).Bottom(em(c, 12)).Gap(em(c, 8)).KeepFocus().Children(func() {
			toolIcon(c, locales.ListTop(), "arrow-up-to-line").Size(em(c, 32), em(c, 32)).Disabled(count == 0).OnClick(func() {
				onNavigate(false)
				state.ScrollTo(0, ui.Start)
			})
			toolIcon(c, locales.ListBottom(), "arrow-down-to-line").Size(em(c, 32), em(c, 32)).Disabled(count == 0).OnClick(func() {
				onNavigate(true)
				// ScrollToEnd includes the bottom of an unusually tall last row.
				state.ScrollToEnd()
			})
		})
	})
	return table
}
