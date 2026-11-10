package main

import (
	"github.com/egoist/mygo/ui"
	lobeAction "github.com/lobehub/lobe-mygo/pkg/action"
)

// Lobe's block variant shares the width while MyGo retains radio selection,
// keyboard navigation and accessibility. Peer options use Lobe's neutral
// selected surface; blue is reserved for navigation, lists and primary actions.
func segmentedFill(c *ui.Context, selected *int, labels ...string) ui.Element {
	track := lobeAction.Segmented(c, selected, labels, lobeAction.SegmentedProps{Block: true})
	track.Border(1, tabBorder(c))
	return track
}
