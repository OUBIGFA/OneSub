package main

import (
	"fmt"
	"math"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/egoist/mygo/ui"
)

// Actions follow Subtitle Edit's separate single/double-click preferences.
// Avalonia controls stay upstream; the adapter drives MyGo's existing player.
var subtitleActions = []struct{ ID, Label string }{
	{"none", "仅选择字幕"},
	{"seek", "定位，保持播放状态"},
	{"seek-pause", "定位并暂停"},
	{"seek-play", "定位并播放"},
}

func (a *NativeApp) playbackActionField(c *ui.Context, label string, value *string) {
	labels, chosen := []string{}, ""
	for _, action := range subtitleActions {
		labels = append(labels, action.Label)
		if action.ID == *value {
			chosen = action.Label
		}
	}
	ui.Field(c, label, func() {
		ui.Select(c, &chosen, labels).OnChange(func() {
			for _, action := range subtitleActions {
				if chosen == action.Label {
					*value = action.ID
					break
				}
			}
			a.saveSettings()
		})
	})
}

func (a *NativeApp) seekSubtitle(action string) {
	if a.mediaURL == "" || a.selected < 0 || a.snapshot == nil {
		return
	}
	cue := a.snapshot.Document.Cues[a.selected]
	if script := subtitleSeekScript(action, cue.StartMs); script != "" {
		a.activeCue = a.selected
		a.evalPlayer(script)
	}
}

func subtitleSeekScript(action string, startMs int64) string {
	if action == "none" || action == "" {
		return ""
	}
	script := fmt.Sprintf("video.currentTime=%f", float64(startMs)/1000)
	switch action {
	case "seek-pause":
		return script + ";video.pause()"
	case "seek-play":
		return script + ";video.play()"
	case "seek":
		return script
	default:
		return ""
	}
}

// Imported ASS can contain overlapping/out-of-order cues: keep document order
// when several events are active instead of assuming an exclusive timeline.
func cueAt(cues []subtitle.Cue, seconds float64) int {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return -1
	}
	ms := int64(math.Round(seconds * 1000))
	for i, cue := range cues {
		if cue.StartMs <= ms && ms < cue.EndMs && !cue.ReadOnly {
			return i
		}
	}
	return -1
}
