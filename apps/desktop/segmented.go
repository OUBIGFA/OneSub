package main

import "github.com/egoist/mygo/ui"

// Equal-width variant of MyGo's native segmented control. SegmentedBase owns
// radio selection, arrow/Home/End navigation, focus and accessibility; native
// flex layout gives every choice its share of the available track in this frame.
func segmentedFill(c *ui.Context, selected *int, labels ...string) ui.Element {
	t := c.Theme()
	parts := ui.SegmentedBase(c, selected, len(labels))
	return parts.Track.AlignItems(ui.Stretch).Padding(t.Space(0.5)).Gap(t.Space(0.5)).
		Radius(t.Radius).Background(t.Surface).Border(1, t.Border).Children(func() {
		for i, label := range labels {
			segment := parts.Segment(i).Grow(1).Basis(0).MinWidth(0).
				Padding(em(c, 6), em(c, 8)).MinHeight(em(c, 32)).Radius(max(0, t.Radius-t.Space(0.5))).Label(label)
			if i == *selected {
				color := t.Background
				if t.Dark {
					color = t.SurfacePressed
				}
				segment.Background(color).TextColor(t.Text)
			} else if segment.Hovered() {
				segment.Background(t.SurfaceHover)
			} else {
				segment.TextColor(t.TextMuted)
			}
			segment.Children(func() { ui.Text(c, label).SingleLine() })
		}
	})
}
