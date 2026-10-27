package main

import (
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/egoist/mygo"
)

// MyGo's work area is already in DIP and excludes the taskbar. Do not divide by
// the display scale again. StateKey still restores an existing user's bounds.
func initialWindowBounds(work mygo.Rectangle) mygo.Rectangle {
	if work.Width <= 0 || work.Height <= 0 {
		return mygo.Rectangle{Width: 1440, Height: 900}
	}
	height := min(work.Height-24, max(640, work.Height*90/100))
	// An ultrawide monitor should not turn the video into a narrow strip between
	// empty gutters. Keep a generous working size with a bounded aspect ratio.
	width := min(work.Width-24, max(900, min(work.Width*88/100, height*2)))
	return mygo.Rectangle{X: work.X + (work.Width-width)/2, Y: work.Y + (work.Height-height)/2, Width: width, Height: height}
}

func cueDisplayText(cue subtitle.Cue, tab int) string {
	if tab == 1 {
		if cue.Translated {
			return cue.Text
		}
		return ""
	}
	if cue.Translated {
		if cue.TranslationSource != "" {
			return cue.TranslationSource
		}
		return cue.OriginalText
	}
	return cue.Text
}
