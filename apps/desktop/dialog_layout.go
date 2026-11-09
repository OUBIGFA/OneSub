package main

import (
	"slices"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// All variants share a grid cell in the current layout pass. Invisible pages
// contribute their full intrinsic size but cannot draw, focus or take input.
// This keeps a dialog stable even when its shorter mode is opened first.
func stableVariants(c *ui.Context, selected int, pages ...func()) ui.Element {
	return ui.Grid(c).Columns(1).Children(func() {
		for i, build := range pages {
			page := ui.Column(c.Key(i)).ColumnStart(1).RowStart(1).MinWidth(0)
			if i != selected {
				page.Invisible()
			}
			page.Children(build)
		}
	})
}

// Notes are laid out with every message their options can select. An empty
// note retains the same room; errors remain real text, never clipped to a box.
func stableDialogNote(c *ui.Context, current string, warning bool, variants ...string) ui.Element {
	if !slices.Contains(variants, current) {
		variants = append(variants, current)
	}
	pages := make([]func(), len(variants))
	for i, text := range variants {
		pages[i] = func() {
			ui.Column(c).Gap(em(c, 4)).Children(func() {
				for _, line := range strings.Split(text, "\n") {
					note := muted(c, line).FontSize(em(c, captionSize)).LineHeight(1.45)
					if text == current && warning {
						note.TextColor(c.Theme().Warning)
					}
				}
			})
		}
	}
	return stableVariants(c, slices.Index(variants, current), pages...)
}

// These are the validation messages SelectOutput can show when the user
// switches tracks or file formats. Reserve the complete messages up front;
// formatting the maximum missing-cue count also covers its smaller counts.
func (a *NativeApp) outputNoteVariants(notes ...string) []string {
	count := 0
	if a.snapshot != nil {
		count = len(a.snapshot.Document.Cues)
	}
	return append(notes, locales.ErrOutputMode(), locales.ErrOutputPrimary(), locales.ErrOutputAssBilingual(), locales.ErrOutputMissing(count))
}
