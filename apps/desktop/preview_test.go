package main

import (
	"testing"
	"time"
)

// Repeated input must keep the preview runnable, including before focus leaves
// the field; only preference writes wait for a pause in editing.
func TestStyleChangesRefreshWithoutWaitingForInputToStop(t *testing.T) {
	a := withStyleCatalog(t)
	now := time.Now()
	for range 5 {
		a.currentStyle.FontSize++
		a.observeStyle(now)
		if !a.previewPending || a.previewDue.After(now) {
			t.Fatal("style preview waits for input to stop", a.previewDue.Sub(now))
		}
		if !a.prefsSaveDue.After(now) {
			t.Fatal("preference writes should remain debounced")
		}
		now = now.Add(50 * time.Millisecond)
	}
}

// An empty selected output is a synchronous refresh, so this exercises the
// real view scheduler without a window or asynchronous renderer substitute.
func TestBackgroundTaskDoesNotBlockPendingPreview(t *testing.T) {
	a := withStyleCatalog(t)
	a.snapshot.Document.Cues = nil
	a.playerReady, a.mediaURL, a.busy = true, "test-media", true
	a.observeStyle(time.Now().Add(-time.Second))
	settled(a.view, 1700, 1100)
	if a.previewPending {
		t.Fatal("background task blocked subtitle preview refresh")
	}
}
