package main

import (
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// subtitleNavigationRailSize is shared by the fixed right navigation rail and
// the always-present left selection gutter, so the list keeps a mirrored frame.
const subtitleNavigationRailSize = 40

// A quick departure and a longer, non-bouncing tail confirm edge navigation
// without changing the shorter cadence used for ordinary playback following.
var subtitleNavigationAnimation = ui.Animation{
	Duration: 420 * time.Millisecond,
	Ease:     ui.CubicBezier(0.22, 1, 0.36, 1),
}

// subtitleSelectionColumn reserves the mirrored left gutter for row checks.
// The column remains present when selection is off; its cells are empty then.
func subtitleSelectionColumn(c *ui.Context) ui.TableColumn {
	width := em(c, subtitleNavigationRailSize)
	return ui.TableColumn{ID: "selection", Width: width, MinWidth: width, Fixed: true}
}

// The navigation gutter belongs to the native table, so its header divider
// and selected-row background reach the pane edge. Fixed buttons occupy only
// that empty column and leave the native scrollbar accessible at the edge.
func subtitleTable(c *ui.Context, state *ui.ListState, columns []ui.TableColumn, count int, cell func(int, int), onNavigate func(toEnd bool), cellMenu ...func(row int, target ui.Element)) ui.Element {
	// Keep every header whole even when that leaves little room for text:
	// MyGo scrolls the table horizontally instead of ellipsizing labels.
	for i := range columns {
		width, _ := c.MeasureText(0, ui.Span{Text: columns[i].Title, Weight: 600})
		columns[i].MinWidth = max(columns[i].MinWidth, width+c.Theme().Space(5))
	}
	contentColumns := len(columns)
	railWidth := em(c, subtitleNavigationRailSize)
	columns = append(columns, ui.TableColumn{ID: "navigation-gutter", Width: railWidth, MinWidth: railWidth, Fixed: true})
	state.CellElement = nil
	if len(cellMenu) > 0 {
		state.CellElement = func(target ui.Element, row, col int) { cellMenu[0](row, target) }
	}
	var table ui.Element
	ui.Column(c).Grow(1).MinHeight(0).Children(func() {
		table = ui.Table(c, state, columns, count, func(row, col int) {
			if col < contentColumns {
				cell(row, col)
			}
		}).Grow(1).MinWidth(0)
		ui.Column(c).Absolute().Top(0).Bottom(0).Right(0).Width(railWidth).Padding(0, em(c, 8), 0, 0).
			Justify(ui.Center).Gap(em(c, 8)).KeepFocus().PassThrough().Children(func() {
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
