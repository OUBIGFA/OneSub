package main

import (
	"fmt"
	"math"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
	lobeField "github.com/lobehub/lobe-mygo/pkg/field"
)

// option is a stable value with the label said in the current language.
type option struct{ ID, Label string }

// idSelect is a native Select over labels that stores the option's ID, so
// saved choices survive a change of the interface language.
// An optional placeholder is shown only when the stored choice is absent;
// it is not added to the selectable items.
func idSelect(c *ui.Context, value *string, options []option, placeholder ...string) ui.Element {
	hint := ""
	if len(placeholder) > 0 {
		hint = placeholder[0]
	}
	return idSelectWithProps(c, value, options, lobeField.SelectProps{Placeholder: hint})
}

// Keep ID-to-label conversion shared when a caller configures popup sizing.
func idSelectWithProps(c *ui.Context, value *string, options []option, props lobeField.SelectProps) ui.Element {
	items := make([]lobeField.SelectOption, len(options))
	for i, option := range options {
		items[i] = lobeField.SelectOption{Value: option.ID, Label: option.Label}
	}
	props.Options = items
	return lobeField.Select(c, value, props)
}

// Actions follow Subtitle Edit's separate single/double-click preferences.
// Avalonia controls stay upstream; the adapter drives MyGo's existing player.
func subtitleActions() []option {
	return []option{
		{"none", locales.ClickSelectOnly()},
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
	if !a.playbackAvailable() || a.selected < 0 || a.snapshot == nil {
		return
	}
	cue := a.activeDocument().Cues[a.selected]
	if script := subtitleSeekScript(action, cue.StartMs); script != "" {
		a.activeCue = a.selected
		a.position = float64(cue.StartMs) / 1000
		a.paused = action == "seek-pause"
		if action == "seek-play" {
			// Explicit play seeks always show the target, even if it was
			// already active or ordinary playback-follow is disabled.
			a.locateCue = a.selected
		}
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
