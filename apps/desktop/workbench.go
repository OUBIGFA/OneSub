package main

import (
	"reflect"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/egoist/mygo"
)

// Keep the three columns and the nested style split usable at the minimum.
const minimumWindowWidth = 1360

// MyGo's work area is already in DIP and excludes the taskbar. Do not divide by
// the display scale again. StateKey still restores an existing user's bounds.
func initialWindowBounds(work mygo.Rectangle) mygo.Rectangle {
	if work.Width <= 0 || work.Height <= 0 {
		return mygo.Rectangle{Width: 1440, Height: 900}
	}
	height := min(work.Height-24, max(640, work.Height*90/100))
	// An ultrawide monitor should not turn the video into a narrow strip between
	// empty gutters. Keep a generous working size with a bounded aspect ratio.
	width := min(work.Width-24, max(minimumWindowWidth, min(work.Width*88/100, height*2)))
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

func (a *NativeApp) activeDocument() subtitle.Document {
	if a.snapshot == nil {
		return subtitle.Document{}
	}
	document := a.snapshot.TrackDocument(a.subtitleTab)
	if a.subtitleTab == 1 && !a.hasRebuiltSubtitles() {
		// The store mirrors original cues until a working track exists. That
		// implementation detail is not an optimized result to show or edit.
		document.Cues = nil
	}
	return document
}

func (a *NativeApp) hasRebuiltSubtitles() bool {
	if a.snapshot == nil {
		return false
	}
	for _, cue := range a.snapshot.Document.Cues {
		if cue.Translated || cue.SourceRange != nil || len(cue.SourceCueIDs) > 0 {
			return true
		}
	}
	return a.snapshot.SourceDocument.Cues != nil && !reflect.DeepEqual(a.snapshot.Document.Cues, a.snapshot.SourceDocument.Cues)
}

// subtitleCounts describes the actual complete tracks, independently of the
// selected tab/search and transient task/error text. A store mirror or pending
// translation preview is not an optimized subtitle result.
func (a *NativeApp) subtitleCounts() (original, optimized int) {
	if a.snapshot == nil {
		return 0, 0
	}
	if a.countSnapshot != a.snapshot {
		a.countSnapshot = a.snapshot
		a.originalCount, a.optimizedCount = len(a.snapshot.TrackDocument(0).Cues), 0
		if a.hasRebuiltSubtitles() {
			a.optimizedCount = len(a.snapshot.Document.Cues)
		}
	}
	return a.originalCount, a.optimizedCount
}

// The rebuilt track has two editable texts on its own shared timeline.
func (a *NativeApp) editorTextTrack() int {
	if a.subtitleTab == 1 && !a.editRebuiltSource {
		return 1
	}
	return 0
}
