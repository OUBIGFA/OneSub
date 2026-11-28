package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo"
)

func stamp(seconds float64) string {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 {
		seconds = 0
	}
	ms := int64(math.Round(seconds * 1000))
	return fmt.Sprintf("%02d:%02d:%02d.%03d", ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
}
func (a *NativeApp) evalPlayer(script string) {
	if a.player == nil {
		return
	}
	generation := a.mediaGeneration
	go func() {
		_, err := a.player.Page().Eval(script)
		a.win.Update(func() {
			if generation != a.mediaGeneration {
				return
			}
			if err != nil {
				a.showError("player-command", locales.ErrPlayer(err.Error()))
			} else {
				a.clearError("player-command")
			}
		})
	}()
}
func (a *NativeApp) loadVideo() {
	if !a.playerReady {
		return
	}
	a.previewPending = true
	a.mediaGeneration++
	value, _ := json.Marshal(a.mediaURL)
	// Load commands run asynchronously; a delayed older command cannot
	// restore the previous video after clear or a second import.
	a.evalPlayer(fmt.Sprintf("loadMedia(%s,%d);", value, a.mediaGeneration) + a.volumeScript() + a.rateScript())
}

// syncPlayer mirrors the page's playback state into the native controls.
func (a *NativeApp) syncPlayer() {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		if a.player == nil {
			continue
		}
		result, err := mygo.EvalAs[struct {
			Position float64 `json:"position"`
			Duration float64 `json:"duration"`
			Paused   bool    `json:"paused"`
			Error    string  `json:"error"`
			Source   string  `json:"source"`
			Aspect   float64 `json:"aspect"`
		}](a.player.Page(), "({position:video.currentTime,duration:Number.isFinite(video.duration)?video.duration:0,paused:video.paused,error:window.previewError||'',source:video.getAttribute('src')||'',aspect:video.videoHeight?video.videoWidth/video.videoHeight:0})")
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			continue
		}
		a.win.Update(func() {
			if result.Source != a.mediaURL {
				return // an older media poll cannot overwrite a newly loaded source
			}
			if result.Aspect > 0 {
				a.frameAspect = result.Aspect
			}
			// Playing again follows the playhead again, from its cue.
			if a.paused && !result.Paused && !a.followHeld && a.pendingClick == nil {
				a.list.StopScroll()
				a.activeCue = -1
				a.resumeFollow()
			}
			a.position = result.Position
			a.duration = result.Duration
			a.paused = result.Paused
			// A page error shows once, as it happens, not again every poll.
			if result.Error != a.playerError {
				a.playerError = result.Error
				a.showError("player", playerErrorText(result.Error))
			} else if result.Error == "" {
				a.clearError("player")
			}
		})
	}
}
