package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// Task flags are the shared playback boundary; keep selection/editing working
// and discard blocked clicks rather than replaying them after a short task.
func TestTaskProgressBlocksSubtitlePlayback(t *testing.T) {
	for _, task := range []string{"compose", "translate", "recognize"} {
		for _, double := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/double=%t", task, double), func(t *testing.T) {
				a := storeApp(t, navigationCues(3)...)
				a.mediaURL, a.position, a.paused = "local-test-video", 9.4, true
				a.settings.SingleClickAction, a.settings.DoubleClickAction = "seek-play", "seek-play"
				a.composeRunning = task == "compose"
				a.translationRunning = task == "translate"
				a.recognitionRunning = task == "recognize"
				tt := settled(a.subtitlesView, 900, 500)
				label := a.activeDocument().Cues[1].Text
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
				if double {
					tt.Advance(100 * time.Millisecond)
					if err := tt.Click(label); err != nil {
						t.Fatal(err)
					}
				}
				if a.selected != 1 || a.draft != label || a.position != 9.4 || !a.paused || a.locateCue != -1 || a.pendingClick != nil {
					t.Fatal("task click did not stay a selection-only action", a.selected, a.position, a.paused, a.pendingClick)
				}
				// The direct action is also used by Enter/keyboard activation.
				a.selectSubtitle(2, "seek-pause")
				a.seekSubtitle("seek-play")
				if a.selected != 2 || a.position != 9.4 || !a.paused {
					t.Fatal("subtitle activation bypassed the task playback boundary")
				}
				a.composeRunning, a.translationRunning, a.recognitionRunning = false, false, false
				tt.Advance(subtitleDoubleClickInterval() + time.Second)
				if a.position != 9.4 || !a.paused {
					t.Fatal("blocked click replayed after task completion")
				}
				// A new click works immediately after the task, without changing
				// the saved single/double-click preferences.
				if err := tt.Click(a.activeDocument().Cues[2].Text); err != nil {
					t.Fatal(err)
				}
				tt.Advance(subtitleDoubleClickInterval() + time.Second)
				if a.position != 2 || a.paused || a.settings.SingleClickAction != "seek-play" {
					t.Fatal("task completion did not restore ordinary playback", a.position, a.paused)
				}
			})
		}
	}
}

// Translation may begin while media is already playing. Block both seek/play
// and seek/pause without changing that existing playback state.
func TestTaskProgressPreservesPlayingVideo(t *testing.T) {
	for _, task := range []string{"compose", "translate"} {
		for _, action := range []string{"seek-play", "seek-pause"} {
			t.Run(task+"/"+action, func(t *testing.T) {
				a := storeApp(t, navigationCues(3)...)
				a.mediaURL, a.position, a.paused = "local-test-video", 9.4, false
				a.composeRunning, a.translationRunning = task == "compose", task == "translate"
				a.settings.DoubleClickAction = action
				a.clickSubtitle(1, true, time.Now())
				a.seekSubtitle(action)
				if a.selected != 1 || a.position != 9.4 || a.paused || a.locateCue != -1 || a.pendingClick != nil {
					t.Fatal("task click changed an already playing video", a.position, a.paused)
				}
			})
		}
	}
}

func TestTaskDiscardsPendingSubtitlePlayback(t *testing.T) {
	for _, task := range []string{"compose", "translate"} {
		t.Run(task, func(t *testing.T) {
			a := storeApp(t, navigationCues(3)...)
			a.mediaURL, a.position = "local-test-video", 9.4
			a.settings.SingleClickAction = "seek-play"
			now := time.Now()
			a.clickSubtitle(1, false, now)
			if a.pendingClick == nil {
				t.Fatal("idle click did not await double-click detection")
			}
			a.composeRunning, a.translationRunning = task == "compose", task == "translate"
			a.finishSubtitleClick(now.Add(subtitleDoubleClickInterval()))
			if a.pendingClick != nil || a.position != 9.4 || !a.paused {
				t.Fatal("a delayed click bypassed task progress", a.position, a.paused)
			}
		})
	}
}

func TestTaskDisablesPlayerContextActionsAndRestoresThem(t *testing.T) {
	for _, task := range []string{"compose", "translate", "recognize"} {
		t.Run(task, func(t *testing.T) {
			a := nativeForTest(t)
			a.mediaURL = "local-test-video"
			a.composeRunning = task == "compose"
			a.translationRunning = task == "translate"
			a.recognitionRunning = task == "recognize"
			if a.playbackAvailable() {
				t.Fatal("task progress still allows player clicks", task)
			}
			tt := settled(a.playerOverlay, 600, 400)
			for _, label := range []string{locales.PlayerPlay(), locales.PlayerStart(), locales.PlayerBack(), locales.PlayerForward()} {
				if err := tt.RightClick(locales.A11yVideo()); err != nil {
					t.Fatal(err)
				}
				if err := tt.ChooseMenuItem(label); err == nil || !strings.Contains(err.Error(), "disabled") {
					t.Fatal("native player menu bypassed task progress", label, err)
				}
				tt.CloseMenu()
			}
			a.composeRunning, a.translationRunning, a.recognitionRunning = false, false, false
			tt.Frame()
			if !a.playbackAvailable() {
				t.Fatal("finished task left playback disabled")
			}
			if err := tt.RightClick(locales.A11yVideo()); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem(locales.PlayerPlay()); err != nil {
				t.Fatal("finished task left the native play action disabled", err)
			}
		})
	}
}
