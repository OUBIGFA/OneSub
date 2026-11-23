package main

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

func TestPlayerCueButtonsFollowVisibleTrackAndKeepPlaybackState(t *testing.T) {
	a := storeApp(t,
		subtitle.Cue{ID: "a", Text: "First target", EndMs: 1000},
		subtitle.Cue{ID: "b", Text: "Hidden row", StartMs: 1100, EndMs: 2100},
		subtitle.Cue{ID: "c", Text: "Last target", StartMs: 2200, EndMs: 3200})
	a.query, a.selected = "TARGET", 0
	a.loadDraft()
	a.draft = "First target edited"
	a.changed(time.Now())
	tt := settled(a.playerView, 700, 500)
	if err := tt.Click(locales.PlayerNextCue()); err != nil || a.selected != 2 || a.locateCue != 2 || a.position != 0 || a.activeDocument().Cues[0].Text != "First target edited" {
		t.Fatal("subtitle-only navigation lost the filter, draft, or target", err, a.selected, a.position)
	}
	if a.adjacentPlayerCue(1) != -1 {
		t.Fatal("next should stop at the last matching cue")
	}
	a.mediaURL, a.duration, a.paused, a.loop = "test-media", 4, false, true
	tt.Frame()
	if err := tt.Click(locales.PlayerPreviousCue()); err != nil || a.selected != 0 || a.paused || a.position != 0 || a.query != "TARGET" {
		t.Fatal("previous should keep playing the selected filtered cue", err, a.position, a.selected)
	}
	if err := tt.Click(locales.PlayerEnd()); err != nil || !a.paused || a.position != 4 || !a.loop {
		t.Fatal("go to end should pause without clearing loop", err, a.position, a.paused)
	}
	if err := tt.Click(locales.PlayerNextCue()); err != nil || a.selected != 2 || a.position != 2.2 || !a.paused {
		t.Fatal("next should seek to the cue and remain paused", err, a.position, a.paused)
	}
	a.playerFullscreen, a.paused, a.selected, a.position, a.query = true, false, 0, 2.6, ""
	if a.adjacentPlayerCue(-1) != 1 || a.adjacentPlayerCue(1) != -1 {
		t.Fatal("fullscreen navigation used a stale list selection")
	}
	a.position = 2.15
	if a.adjacentPlayerCue(-1) != 1 || a.adjacentPlayerCue(1) != 2 {
		t.Fatal("fullscreen navigation skipped the nearest cue across a gap")
	}
}

func TestFullscreenPreviewRestoresWorkbench(t *testing.T) {
	a := nativeForTest(t)
	a.mediaURL, a.position, a.duration, a.rate = "test-media", 12, 30, 1.25
	tt := settled(a.view, 1360, 800)
	before, _ := tt.Find(locales.A11yPlayer())
	if err := tt.Click(locales.PlayerFullscreen()); err != nil {
		t.Fatal(err)
	}
	full, ok := tt.Find(locales.A11yPlayer())
	if !ok || !a.playerFullscreen || full.W != 1360 || full.H != 800 || tt.HasText(locales.A11yToolbar()) || !tt.HasText(locales.PlayerExitFullscreen()) {
		t.Fatal("fullscreen did not give the viewport to the player", full, a.playerFullscreen)
	}
	tt.Key(0, ui.KeyEscape)
	after, _ := tt.Find(locales.A11yPlayer())
	if a.playerFullscreen || before != after || a.position != 12 || a.rate != 1.25 || !tt.HasText(locales.A11yToolbar()) {
		t.Fatal("Escape lost the saved layout or playback state", before, after)
	}
	tt.Key(0, ui.KeyF11)
	if err := tt.Click(locales.PlayerExitFullscreen()); err != nil || a.playerFullscreen {
		t.Fatal("F11 and the exit button did not round trip", err)
	}
	a.mediaURL = ""
	tt.Frame()
	tt.Key(0, ui.KeyF11)
	if a.playerFullscreen {
		t.Fatal("empty player entered fullscreen")
	}
}

// A narrow monitor pane must not push time or audio controls outside its
// bounds. Native rows wrap complete command groups; the split stays user-owned.
func TestPlayerTransportGroupsFitNarrowPanes(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		for _, width := range []int{320, 500, 639, 640, 900} {
			for _, scale := range []float32{1, 1.5, 2} {
				t.Run(fmt.Sprintf("%s/%d/%g", language, width, scale), func(t *testing.T) {
					inLocale(t, language, func() {
						a := nativeForTest(t)
						a.position, a.duration = 123, 456
						a.rate = 1.25
						a.mediaURL = "http://127.0.0.1/video"
						tt := settled(func(c *ui.Context) {
							theme, _ := studioTheme(c.Theme(), false)
							c.SetTheme(&theme)
							a.playerView(c)
						}, int(float32(width)*scale), int(500*scale))
						tt.SetPreferences(ui.Preferences{TextScale: scale})
						labels := []string{locales.PlayerStart(), locales.PlayerBack(), locales.PlayerPreviousCue(), locales.PlayerPlay(), locales.PlayerNextCue(), locales.PlayerForward(), locales.PlayerEnd(), locales.PlayerSpeed(), locales.PlayerLoop(), stamp(123), stamp(456), locales.PlayerPosition(), locales.PlayerMute(), locales.PlayerVolume(), locales.PlayerFullscreen()}
						insideRedesign(t, tt, locales.A11yPlayer(), labels...)
						player, _ := tt.Find(locales.A11yPlayer())
						play, _ := tt.Find(locales.PlayerPlay())
						if math.Abs(float64(play.X+play.W/2-player.X-player.W/2)) > 1 {
							t.Fatal("playback controls are not centered in the video pane", player, play)
						}
						for i, label := range labels {
							r, _ := tt.Find(label)
							for _, other := range labels[i+1:] {
								s, _ := tt.Find(other)
								if r.X < s.X+s.W-1 && r.X+r.W > s.X+1 && r.Y < s.Y+s.H-1 && r.Y+r.H > s.Y+1 {
									t.Fatal("transport controls overlap", label, other, r, s)
								}
							}
						}
					})
				})
			}
		}
	}
}
