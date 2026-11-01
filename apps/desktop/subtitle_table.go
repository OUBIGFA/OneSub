package main

import (
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// subtitleTable keeps native navigation beside the scrolling content, not
// in a padded-off area of the table. The text column fills the actual table;
// the narrow rail never covers text or its scrollbar and keeps editor focus.
func subtitleTable(c *ui.Context, state *ui.ListState, columns []ui.TableColumn, count int, cell func(int, int), onNavigate func(toEnd bool)) ui.Element {
	var table ui.Element
	ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
		table = ui.Table(c, state, columns, count, cell).Grow(1).MinWidth(0)
		ui.Column(c).Width(em(c, 40)).Padding(0, em(c, 4)).Shrink(0).Justify(ui.Center).Gap(em(c, 8)).KeepFocus().Children(func() {
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
