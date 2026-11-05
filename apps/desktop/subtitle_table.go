package main

import (
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// A quick departure and a longer, non-bouncing tail confirm edge navigation
// without changing the shorter cadence used for ordinary playback following.
var subtitleNavigationAnimation = ui.Animation{
	Duration: 420 * time.Millisecond,
	Ease:     ui.CubicBezier(0.22, 1, 0.36, 1),
}

// subtitleTable keeps native navigation beside the scrolling content, not
// in a padded-off area of the table. The text column fills the actual table;
// the narrow rail never covers text or its scrollbar and keeps editor focus.
func subtitleTable(c *ui.Context, state *ui.ListState, columns []ui.TableColumn, count int, cell func(int, int), onNavigate func(toEnd bool)) ui.Element {
	// Checking rows adds width to the number column. Keep every header whole
	// even when that leaves little room for text: MyGo scrolls the table
	// horizontally instead of ellipsizing action-independent column labels.
	for i := range columns {
		width, _ := c.MeasureText(0, ui.Span{Text: columns[i].Title, Weight: 600})
		columns[i].MinWidth = max(columns[i].MinWidth, width+c.Theme().Space(5))
	}
	var table ui.Element
	ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
		table = ui.Table(c, state, columns, count, cell).Grow(1).MinWidth(0)
		ui.Column(c).Width(em(c, 40)).Padding(0, em(c, 4)).Shrink(0).Justify(ui.Center).Gap(em(c, 8)).KeepFocus().Children(func() {
			toolIcon(c, locales.ListTop(), "arrow-up-to-line").Size(em(c, 32), em(c, 32)).Disabled(count == 0).OnClick(func() {
				onNavigate(false)
				state.ScrollToEdgeAnimated(false, subtitleNavigationAnimation)
			})
			toolIcon(c, locales.ListBottom(), "arrow-down-to-line").Size(em(c, 32), em(c, 32)).Disabled(count == 0).OnClick(func() {
				onNavigate(true)
				// Include the bottom of an unusually tall last row in the landing.
				state.ScrollToEdgeAnimated(true, subtitleNavigationAnimation)
			})
		})
	})
	return table
}
