package main

import (
	"fmt"
	"math"
	"strings"

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

// Step in document order, including for overlapping ASS events. A filtered
// list keeps its search and its issue filter; never land on an invisible row.
func (a *NativeApp) adjacentPlayerCue(direction int) int {
	if a.snapshot == nil || a.busy || a.composing || a.subtitleSelectionMode() || a.showsTranslationProgress() || a.mediaURL != "" && !a.playbackAvailable() {
		return -1
	}
	cues := a.activeDocument().Cues
	query := strings.ToLower(a.query)
	var problems [][]subtitle.CueProblem
	if a.problems.filter {
		problems = a.subtitleProblems()
	}
	first := a.selected + direction
	if a.playerFullscreen && !a.paused {
		// The list is unmounted in fullscreen. Use the playing cue instead
		// of the row selected before entering the preview.
		if near := cueNear(cues, a.position); near >= 0 {
			ms := int64(math.Round(a.position * 1000))
			first = near + direction
			if direction > 0 && ms < cues[near].StartMs || direction < 0 && ms >= cues[near].EndMs {
				first = near
			}
		}
	}
	for i := first; i >= 0 && i < len(cues); i += direction {
		if !cues[i].ReadOnly && (!a.problems.filter || len(problems[i]) > 0) && matchesSubtitleQuery(cues[i], a.subtitleTab, query) {
			return i
		}
	}
	return -1
}

func (a *NativeApp) stepPlayerCue(direction int) {
	index := a.adjacentPlayerCue(direction)
	if index < 0 {
		return
	}
	action := "seek-pause"
	if !a.paused {
		action = "seek-play"
	}
	a.pendingClick, a.rowClick = nil, nil
	a.selectSubtitle(index, action)
	if a.selected == index {
		a.list.StopScroll()
		a.browseHeld, a.locateCue = false, index
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
