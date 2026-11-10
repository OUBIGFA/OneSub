package main

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// Real MyGo pointer events, all 3×3 action pairs, on both stopped and playing
// media. A native double click must execute only the double-click action.
func TestSubtitleClickActionMatrix(t *testing.T) {
	for _, paused := range []bool{true, false} {
		for _, single := range []string{"none", "seek-pause", "seek-play"} {
			for _, double := range []string{"none", "seek-pause", "seek-play"} {
				t.Run(fmt.Sprintf("paused=%t/%s/%s", paused, single, double), func(t *testing.T) {
					a := storeApp(t, navigationCues(160)...)
					a.settings.SingleClickAction, a.settings.DoubleClickAction = single, double
					a.mediaURL, a.position, a.paused = "http://127.0.0.1/fixture", 20.2, paused
					tt := settled(a.subtitlesView, 900, 500)
					tt.Advance(time.Second)
					_ = tt.Click(locales.ListBottom())
					tt.Advance(time.Second)
					row, ok := tt.Find(a.activeDocument().Cues[155].Text)
					if !ok {
						first, last := a.list.Visible()
						t.Fatal("target is not visible", a.activeDocument().Cues[155].ID, first, last, tt.Texts())
					}
					tt.ClickAt(row.X+row.W/2, row.Y+row.H/2)
					if a.selected != 155 || a.position != 20.2 || a.paused != paused {
						t.Fatal("first click executed playback instead of selecting", a.selected, a.position, a.paused)
					}
					tt.Advance(100 * time.Millisecond)
					again, _ := tt.Find(a.activeDocument().Cues[155].Text)
					if math.Abs(float64(row.Y-again.Y)) > 1 {
						t.Fatal("row moved before second click", row, again)
					}
					tt.ClickAt(row.X+row.W/2, row.Y+row.H/2)
					tt.Advance(subtitleDoubleClickInterval() + time.Second)
					if a.pendingClick != nil || a.selected != 155 {
						t.Fatal("double click lost target", a.selected)
					}
					if double == "none" {
						if a.position != 20.2 || a.paused != paused || !a.followHeld {
							t.Fatal("select-only moved playback")
						}
					} else if a.position != 155 || a.paused != (double == "seek-pause") || a.followHeld {
						t.Fatal("wrong double-click action", double, a.position, a.paused, a.followHeld)
					}
				})
			}
		}
	}
}

// A lone click has to work in both playback states too, even when automatic
// follow is disabled. Explicit seek/play centers; pause/select leave the row.
func TestSubtitleSingleClickActionsInBothPlaybackStates(t *testing.T) {
	for _, paused := range []bool{true, false} {
		for _, follow := range []bool{true, false} {
			for _, action := range []string{"none", "seek-pause", "seek-play"} {
				t.Run(fmt.Sprintf("paused=%t/follow=%t/%s", paused, follow, action), func(t *testing.T) {
					a := storeApp(t, navigationCues(160)...)
					a.settings.SingleClickAction, a.settings.FollowPlayback = action, follow
					a.mediaURL, a.position, a.paused = "http://127.0.0.1/fixture", 20.2, paused
					tt := settled(a.subtitlesView, 900, 500)
					if err := tt.Click(locales.ListBottom()); err != nil {
						t.Fatal(err)
					}
					// Browse to a middle cue at the top of the viewport so a
					// forced center cannot hide behind the list's end clamp.
					a.list.ScrollTo(80, ui.Start)
					tt.Frame()
					tt.Advance(time.Second)
					row, ok := tt.Find(a.activeDocument().Cues[80].Text)
					if !ok {
						t.Fatal("middle target is not visible")
					}
					tt.ClickAt(row.X+row.W/2, row.Y+row.H/2)
					if a.selected != 80 || a.position != 20.2 || a.paused != paused {
						t.Fatal("lone click acted before double-click detection", a.selected, a.position, a.paused)
					}
					tt.Advance(subtitleDoubleClickInterval() + time.Second)
					if action == "none" {
						if a.position != 20.2 || a.paused != paused || !a.followHeld {
							t.Fatal("select-only changed playback or released follow hold")
						}
					} else if a.position != 80 || a.paused != (action == "seek-pause") || a.followHeld {
						t.Fatal("wrong lone-click action", a.position, a.paused, a.followHeld)
					}
					if action == "seek-play" {
						first, last := a.list.Visible()
						if math.Abs(float64(first+last)/2-80) > 1 {
							t.Fatal("explicit seek/play did not center its target", first, last)
						}
					} else if after, _ := tt.Find(a.activeDocument().Cues[80].Text); math.Abs(float64(after.Y-row.Y)) > 1 {
						t.Fatal("select/pause unexpectedly centered the row", row, after)
					}
				})
			}
		}
	}
}

func TestSubtitleSingleClickHoldRepeatAndPlayFollow(t *testing.T) {
	a := storeApp(t, navigationCues(160)...)
	a.mediaURL, a.position, a.paused = "http://127.0.0.1/fixture", 20.2, false
	a.settings.SingleClickAction = "none"
	tt := settled(a.subtitlesView, 900, 500)
	tt.Advance(time.Second)
	_ = tt.Click(locales.ListBottom())
	tt.Advance(time.Second)
	row, _ := tt.Find(a.activeDocument().Cues[155].Text)
	tt.ClickAt(row.X+row.W/2, row.Y+row.H/2)
	tt.Advance(subtitleDoubleClickInterval())
	a.position = 24.2
	tt.Advance(2 * browseGrace)
	if a.selected != 155 || !a.followHeld {
		t.Fatal("select-only did not hold selection", a.selected)
	}
	first, _ := a.list.Visible()
	if first < 100 {
		t.Fatal("select-only did not hold viewport", first)
	}
	// Clicking an already selected row still acts. It cannot rely on Changed.
	a.settings.SingleClickAction = "seek-play"
	row, _ = tt.Find(a.activeDocument().Cues[155].Text)
	tt.ClickAt(row.X+row.W/2, row.Y+row.H/2)
	tt.Advance(subtitleDoubleClickInterval() + time.Second)
	if a.position != 155 || a.paused || a.followHeld {
		t.Fatal("repeat click did not play", a.position, a.paused)
	}
	a.position = 60.2
	tt.Frame()
	tt.Advance(time.Second)
	first, last := a.list.Visible()
	if a.selected != 60 || first < 100 {
		t.Fatal("play seek released the manual navigation hold", a.selected, first, last)
	}
	_ = tt.Click(locales.ListLocate())
	tt.Advance(time.Second)
	if first, last = a.list.Visible(); first >= 60 || last <= 60 {
		t.Fatal("locate did not restore viewport following", first, last)
	}
	a.selectSubtitle(60, "none")
	a.position = 80.2
	tt.Advance(2 * browseGrace)
	_ = tt.Click(locales.ListLocate())
	tt.Advance(time.Second)
	if a.followHeld || a.selected != 80 {
		t.Fatal("locate did not release hold")
	}
}

func TestSinglePauseDoesNotCenterAndNavigationCancelsPending(t *testing.T) {
	a := storeApp(t, navigationCues(160)...)
	a.mediaURL, a.position, a.paused = "http://127.0.0.1/fixture", 20.2, false
	tt := settled(a.subtitlesView, 900, 500)
	tt.Advance(time.Second)
	_ = tt.Click(locales.ListBottom())
	tt.Advance(time.Second)
	row, _ := tt.Find(a.activeDocument().Cues[155].Text)
	tt.ClickAt(row.X+row.W/2, row.Y+row.H/2)
	tt.Advance(subtitleDoubleClickInterval() + time.Second)
	after, _ := tt.Find(a.activeDocument().Cues[155].Text)
	if a.position != 155 || !a.paused || math.Abs(float64(row.Y-after.Y)) > 1 {
		t.Fatal("single pause changed the viewport", row, after, a.position, a.paused)
	}
	tt.ClickAt(after.X+after.W/2, after.Y+after.H/2)
	_ = tt.Click(locales.ListTop())
	tt.Advance(subtitleDoubleClickInterval() + time.Second)
	if a.pendingClick != nil {
		t.Fatal("navigation retained delayed action")
	}
}

func TestRemovedSeekSettingMigratesWithoutChangingOtherPreferences(t *testing.T) {
	a := nativeForTest(t)
	a.settings.SingleClickAction, a.settings.DoubleClickAction = "seek", "seek"
	a.settings.PlayerWidth = 55
	a.saveSettings()
	loaded, err := settings.Load(a.dataRoot + "/settings.json")
	if err != nil || loaded.SingleClickAction != "seek-play" || loaded.DoubleClickAction != "seek-play" || loaded.PlayerWidth != 55 {
		t.Fatal("legacy action was not migrated independently", err, loaded.SingleClickAction)
	}
}
