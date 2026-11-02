package main

import "time"

// The row identity comes from MyGo's native click routing, never pointer
// coordinates or the playback's mutable selected-row index.
type subtitleRowClick struct {
	index  int
	double bool
}

type subtitleClick struct {
	cueID, projectID, action string
	track, index             int
	due                      time.Time
}

// Select immediately, but defer playback until the native double-click window
// has elapsed. The first click must not move the row out from under the second.
func (a *NativeApp) clickSubtitle(index int, double bool, now time.Time) {
	if !a.commitEdit(true) {
		return
	}
	a.selected = index
	a.loadDraft()
	a.list.StopScroll()
	a.locateCue = -1
	a.noteBrowse(now)
	if double {
		a.pendingClick = nil
		a.selectSubtitle(index, a.settings.DoubleClickAction)
		return
	}
	a.pendingClick = &subtitleClick{
		cueID: a.activeDocument().Cues[index].ID, projectID: a.snapshot.Manifest.ID,
		track: a.subtitleTab, index: index, action: a.settings.SingleClickAction,
		due: now.Add(subtitleDoubleClickInterval()),
	}
}

// An import, structural edit, modal, filter or track switch must not let a
// delayed click seek into a different document or override the new selection.
func (a *NativeApp) finishSubtitleClick(now time.Time) {
	click := a.pendingClick
	if click == nil {
		return
	}
	if a.snapshot == nil || a.settingsOpen || a.savePromptOpen ||
		a.snapshot.Manifest.ID != click.projectID || a.subtitleTab != click.track ||
		a.selected != click.index || click.index >= len(a.activeDocument().Cues) ||
		a.activeDocument().Cues[click.index].ID != click.cueID {
		a.pendingClick = nil
		return
	}
	if !now.Before(click.due) {
		a.pendingClick = nil
		a.selectSubtitle(click.index, click.action)
	}
}
