package main

import (
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/plugins/loading"
	"github.com/egoist/mygo/ui"
)

const (
	blocksStop = 520 * time.Millisecond
	blocksFade = 480 * time.Millisecond
)

// Waiting is presentation state only. Task completion, result publishing and
// controls never wait for the outro; another task can interrupt it immediately.
type waitingState struct {
	shown bool
	ended time.Time
}

func (w *waitingState) visible(c *ui.Context, running bool) bool {
	if running {
		w.shown, w.ended = true, time.Time{}
	} else if w.shown {
		if w.ended.IsZero() {
			w.ended = c.Now()
		}
		total := blocksStop + blocksFade
		if c.Preferences().ReduceMotion {
			total = blocksFade
		}
		if left := total - c.Now().Sub(w.ended); left > 0 {
			c.After(left)
		} else {
			w.shown = false
		}
	}
	return w.shown
}

// blocksBackdrop uses one enlarged native Blocks indicator. The library owns
// all nine blocks, their shrink/grow timing and reduced-motion behaviour. It
// takes no focus or pointer events, and is not a second progress announcement.
func blocksBackdrop(c *ui.Context, finished bool) {
	layer := ui.Column(c.Key("blocks-background")).Absolute().Top(0).Left(0).Right(0).Bottom(0).Padding(em(c, 24)).PassThrough().Clip()
	fade, delay := float32(0), blocksStop
	if finished {
		fade = 1
	}
	if !finished || c.Preferences().ReduceMotion {
		delay = 0
	}
	// First decelerate the plugin clock to zero. Only then fade the blocks;
	// one native pose drives the entire background without rebuilding text.
	layer.Opacity(.025).AnimatePose(ui.Pose{Fade: fade}, ui.Animation{Duration: blocksFade, Delay: delay, Ease: ui.EaseInOut})
	layer.Children(func() {
		loading.Blocks(c, loading.Options{Color: c.Theme().Text, Paused: finished, StopDuration: blocksStop}).Fill().Role(ui.RoleNone).PassThrough()
	})
}

// All background operations use the same waiting scene, including model
// downloads/verification and subtitle workers. File pickers keep their native
// UI; streaming translations retain their live list alongside this backdrop.
func (a *NativeApp) waitingView(c *ui.Context) {
	finished := !a.waiting.ended.IsZero()
	scene := ui.Column(c).Grow(1).Clip()
	scene.Children(func() {
		blocksBackdrop(c, finished)
		switch {
		case finished:
			ui.Column(c).Grow(1).Center().Padding(em(c, 24)).Children(func() {
				text := a.status
				if a.errorText != "" {
					text = a.errorText
				}
				ui.Text(c, text).MaxWidth(em(c, 640))
			})
		case a.recognitionRunning:
			a.recognitionProgressView(c)
		case a.composeRunning:
			a.composeProgressView(c)
		default:
			ui.Column(c).Grow(1).Center().Gap(em(c, 16)).Padding(em(c, 24)).Children(func() {
				label, fraction := a.status, float64(-1)
				if a.translationRunning && a.translationLive != nil {
					l := a.translationLive
					label = l.label()
					if l.total > 0 && (l.stage == "segment" || l.stage == "translate") {
						fraction = float64(l.done) / float64(l.total)
					}
				}
				dialogTitle(c, label)
				ui.Progress(c, fraction).Width(em(c, 400)).Label(locales.A11yTaskProgress())
				ui.Button(c, locales.ActionCancelTask()).Disabled(a.cancel == nil).OnClick(func() {
					if a.cancel != nil {
						a.cancel()
					}
				})
			})
		}
	})
}
