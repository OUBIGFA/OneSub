package main

import (
	"fmt"
	"math"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// option is a stable value with the label said in the current language.
type option struct{ ID, Label string }

// idSelect is a native Select over labels that stores the option's ID, so
// saved choices survive a change of the interface language.
func idSelect(c *ui.Context, value *string, options []option) ui.Element {
	labels, chosen := make([]string, len(options)), ""
	for i, o := range options {
		labels[i] = o.Label
		if o.ID == *value {
			chosen = o.Label
		}
	}
	return ui.Select(c, &chosen, labels).OnChange(func() {
		for _, o := range options {
			if o.Label == chosen {
				*value = o.ID
				return
			}
		}
	})
}

// Actions follow Subtitle Edit's separate single/double-click preferences.
// Avalonia controls stay upstream; the adapter drives MyGo's existing player.
func subtitleActions() []option {
	return []option{
		{"none", locales.ClickSelectOnly()},
		{"seek", locales.ClickSeek()},
		{"seek-pause", locales.ClickSeekPause()},
		{"seek-play", locales.ClickSeekPlay()},
	}
}

func (a *NativeApp) playbackActionField(c *ui.Context, label string, value *string) {
	ui.Field(c, label, func() {
		idSelect(c, value, subtitleActions()).Label(label).OnChange(a.saveSettings)
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

// cueNear is the cue at seconds or, between cues, the last one that started
// before it; the first cue when none has.
func cueNear(cues []subtitle.Cue, seconds float64) int {
	if i := cueAt(cues, seconds); i >= 0 {
		return i
	}
	ms := int64(math.Round(seconds * 1000))
	before, first := -1, -1
	for i, cue := range cues {
		if cue.ReadOnly {
			continue
		}
		if first < 0 {
			first = i
		}
		if cue.StartMs <= ms && (before < 0 || cue.StartMs >= cues[before].StartMs) {
			before = i
		}
	}
	if before >= 0 {
		return before
	}
	return first
}
