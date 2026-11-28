package main

import (
	"context"
	"errors"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

func (a *NativeApp) run(label string, work func(context.Context) (func(), error)) {
	a.runTimed(timingNone, false, label, work)
}

// A new operation replaces its module's previous result. Explicit continuation
// adds only a new execution interval, leaving every other module unchanged.
func (a *NativeApp) runTimed(module taskModule, continuing bool, label string, work func(context.Context) (func(), error)) {
	if a.busy {
		return
	}
	if !a.commitEdit(true) {
		return
	}
	a.busy = true
	a.clearError(label)
	a.status = label
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	var clock *taskTiming
	if module != timingNone {
		var previous time.Duration
		if continuing && a.taskTimes[module] != nil {
			previous = a.taskTimes[module].duration(time.Now())
		}
		clock = &taskTiming{started: time.Now(), elapsed: previous}
		a.taskTimes[module], a.activeTaskTiming = clock, clock
		ctx = context.WithValue(ctx, taskTimingKey{}, clock)
	}
	go func() {
		apply, err := work(ctx)
		if clock != nil {
			clock.stop(time.Now())
		}
		cancel()
		a.win.Update(func() { a.finishTask(label, apply, err) })
	}()
}

// finishTask reconciles worker results on the UI thread before exit or update.
func (a *NativeApp) finishTask(label string, apply func(), err error) {
	// Finalize before apply: recognition may start translation from this
	// callback, and the next module must not extend the completed ASR clock.
	if a.activeTaskTiming != nil {
		a.activeTaskTiming.stop(time.Now())
		a.activeTaskTiming = nil
	}
	a.busy = false
	a.composeRunning, a.composeInBackground, a.compressRunning, a.audioExtractRunning = false, false, false, false
	a.recognitionRunning = false
	if a.translationRunning && err != nil && a.translationLive != nil {
		a.translationLive.stopped = err.Error()
		if errors.Is(err, context.Canceled) {
			a.translationLive.stopped = locales.StatusCancelled()
		}
	}
	a.translationRunning = false
	a.cancel = nil
	if err == nil {
		a.clearError(label)
		a.status = ""
		if apply != nil {
			a.status = locales.StatusDone()
		}
	}
	// A worker may have committed part of its result before a later error or
	// cancellation. Reconcile it, then keep the failure as the final status.
	if apply != nil {
		apply()
	}
	if errors.Is(err, context.Canceled) {
		a.status = locales.StatusCancelled()
	} else if err != nil {
		a.showError(label, err.Error())
		a.status = locales.StatusFailed()
	}
	if a.quitAfterTask {
		a.quitAfterTask = false
		a.requestExit()
	} else {
		a.installAutomaticUpdate()
	}
}
