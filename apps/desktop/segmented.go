package main

import (
	"github.com/egoist/mygo/ui"
	lobeAction "github.com/lobehub/lobe-mygo/pkg/action"
)

// Lobe's block variant shares the width while MyGo retains radio selection,
// keyboard navigation and accessibility. Peer options use Lobe's neutral
// selected surface and border over an unoutlined track; blue is reserved for
// navigation, lists and primary actions.
func segmentedFill(c *ui.Context, selected *int, labels ...string) ui.Element {
	return lobeAction.Segmented(c, selected, labels, lobeAction.SegmentedProps{Block: true}).
		Background(c.Theme().Surface).Border(0, ui.Transparent)
}
