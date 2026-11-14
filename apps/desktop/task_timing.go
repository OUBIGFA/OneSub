package main

import (
	"context"
	"sync"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

type taskModule int

const (
	timingNone taskModule = iota
	timingRecognition
	timingTranslation
	timingMedia
	timingCompose
	timingSubtitle
	timingCount
)

// A clock records execution intervals, not the time since a progress view was
// opened. Workers stop it before queueing their UI result; dialogs pause it.
type taskTiming struct {
	mu       sync.Mutex
	started  time.Time
	elapsed  time.Duration
	finished bool
}

func (clock *taskTiming) duration(now time.Time) time.Duration {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	elapsed := clock.elapsed
	if !clock.started.IsZero() {
		elapsed += max(0, now.Sub(clock.started))
	}
	return elapsed
}

func (clock *taskTiming) pause(now time.Time) {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	if !clock.started.IsZero() {
		clock.elapsed += max(0, now.Sub(clock.started))
		clock.started = time.Time{}
	}
}

func (clock *taskTiming) resume(now time.Time) {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	if !clock.finished && clock.started.IsZero() {
		clock.started = now
	}
}

func (clock *taskTiming) stop(now time.Time) {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	if !clock.started.IsZero() {
		clock.elapsed += max(0, now.Sub(clock.started))
		clock.started = time.Time{}
	}
	clock.finished = true
}

type taskTimingKey struct{}

// File selection is user wait time. The context identifies this operation's
// clock even when an earlier completed module is still shown in the footer.
func pauseTaskTiming(ctx context.Context) func() {
	clock, _ := ctx.Value(taskTimingKey{}).(*taskTiming)
	if clock == nil {
		return func() {}
	}
	clock.pause(time.Now())
	return func() { clock.resume(time.Now()) }
}

func moduleLabel(module taskModule) string {
	switch module {
	case timingRecognition:
		return locales.TimingRecognition()
	case timingTranslation:
		return locales.TimingTranslation()
	case timingMedia:
		return locales.TimingMedia()
	case timingCompose:
		return locales.TimingCompose()
	case timingSubtitle:
		return locales.TimingSubtitle()
	}
	return ""
}

func elapsedLabel(elapsed time.Duration) string {
	elapsed = max(0, elapsed)
	seconds := int(elapsed / time.Second)
	if seconds >= 3600 {
		return locales.TimingHours(seconds/3600, seconds/60%60, seconds%60)
	}
	if seconds >= 60 {
		return locales.TimingMinutes(seconds/60, seconds%60)
	}
	if elapsed < time.Second && elapsed > 0 {
		return locales.TimingSeconds(float64(elapsed/(10*time.Millisecond)) / 100)
	}
	return locales.TimingSeconds(float64(seconds))
}

// One native row keeps module receipts beside the status, counts and output.
// Each complete label retains its natural width; only transient status flexes.
func (a *NativeApp) moduleTimesView(c *ui.Context) {
	ui.Row(c).Gap(em(c, 12)).AlignItems(ui.Center).Shrink(0).Children(func() {
		for module := timingRecognition; module < timingCount; module++ {
			clock := a.taskTimes[module]
			// Subtitle edits retain their task clock without a persistent receipt.
			if clock == nil || module == timingSubtitle {
				continue
			}
			muted(c, moduleLabel(module)+" "+elapsedLabel(clock.duration(c.Now()))).FontFeatures("tnum").FontSize(em(c, captionSize)).SingleLine().Shrink(0)
		}
	})
	if a.activeTaskTiming != nil {
		c.After(100 * time.Millisecond)
	}
}
