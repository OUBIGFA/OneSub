package main

import (
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

type subtitleLengthGuide struct {
	visible     bool
	initialized bool
	position    ui.Point
}

// The guide uses MyGo's own Divider and bounded horizontal Draggable. It lives
// above the table, outside the virtual rows, so scrolling and text layout stay
// unchanged. A wider transparent hit area makes the thin line easy to grab.
func (guide *subtitleLengthGuide) view(c *ui.Context, width float32) {
	if !guide.visible {
		return
	}
	hitWidth := em(c, 12)
	maximum := max(0, width-em(c, subtitleNavigationRailSize)-hitWidth)
	if !guide.initialized {
		guide.position.X, guide.initialized = maximum*.75, true
	}
	guide.position.X, guide.position.Y = min(max(0, guide.position.X), maximum), 0
	ui.Row(c.Key("subtitle-length-guide")).Absolute().Top(0).Bottom(0).Left(0).Width(hitWidth).
		Justify(ui.Center).KeepFocus().Cursor(ui.CursorResizeEW).Label(locales.SubtitleLengthGuide()).
		Draggable(&guide.position, ui.DraggableOptions{Axis: ui.Horizontal, Bounds: &ui.Rect{W: maximum}, Rigid: true}).
		Children(func() {
			ui.Divider(c).Width(em(c, 2)).FillHeight().Background(ui.Hex("#ec4899"))
		})
}
