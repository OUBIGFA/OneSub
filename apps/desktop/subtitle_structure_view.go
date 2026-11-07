package main

import (
	"context"
	"math"
	"slices"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
	"github.com/google/uuid"
)

func (a *NativeApp) subtitleActionsMenu(c *ui.Context) {
	toolIcon(c, locales.SubtitleActionsMenu(), "ellipsis").Disabled(a.busy || len(a.activeDocument().Cues) == 0 || a.showsTranslationProgress()).Menu(func(m *ui.Menu) {
		a.pendingClick = nil
		a.subtitleStructureMenu(m, a.selected)
		// The same commands remain available when a narrow list moves its
		// secondary toolbar actions into this native overflow menu.
		m.Separator()
		if m.Item(locales.FindReplace()).Shortcut(ui.Cmd, ui.KeyH).Chosen() {
			a.openFindReplace()
		}
		if m.Item(locales.TextReplacePeriods()).Chosen() {
			a.cleanListPunctuation(false)
		}
		if m.Item(locales.TextStripTrailing()).Chosen() {
			a.cleanListPunctuation(true)
		}
	})
}

// The list toolbar and its context menus share commands and availability;
// opening a row menu must not bypass task or format protection.
func (a *NativeApp) subtitleStructureMenu(m *ui.Menu, index int) {
	cues := a.activeDocument().Cues
	id := ""
	protected := false
	if index >= 0 && index < len(cues) {
		id = cues[index].ID
		protected = cues[index].ReadOnly
	}
	disabled := a.busy || a.snapshot == nil || a.showsTranslationProgress()
	targets := a.retranslationTargets()
	_, eligible := targets[id]
	if m.Item(locales.RetranslatePick()).Disabled(disabled || a.translationLive != nil || !eligible).Chosen() {
		a.setSubtitleChecked(id, !a.subtitleChecked[a.subtitleTab][id])
	}
	if m.Item(locales.RetranslateSelected(len(a.checkedRetranslationIDs()))).Disabled(disabled || a.translationLive != nil || len(a.checkedRetranslationIDs()) == 0).Chosen() {
		a.openRetranslation()
	}
	if m.Item(locales.RetranslateCheckAll()).Disabled(disabled || a.translationLive != nil || len(targets) == 0).Chosen() {
		a.checkAllSubtitles()
	}
	if m.Item(locales.RetranslateClear()).Disabled(disabled || len(a.checkedRetranslationIDs()) == 0).Chosen() {
		a.subtitleChecked[a.subtitleTab] = nil
	}
	m.Separator()
	// Locating does not edit protected ASS content. Use an explicit pause
	// action, never the user's configurable single/double-click preference.
	if m.Item(locales.SubtitleLocateVideo()).Disabled(disabled || id == "" || !a.playbackAvailable()).Chosen() {
		a.pendingClick, a.rowClick = nil, nil
		a.list.StopScroll()
		a.selectSubtitle(index, "seek-pause")
	}
	m.Separator()
	if m.Item(locales.SubtitleInsertBefore()).Disabled(disabled).Chosen() {
		a.openSubtitleInsert(id, true)
	}
	if m.Item(locales.SubtitleInsertAfter()).Disabled(disabled).Chosen() {
		a.openSubtitleInsert(id, false)
	}
	if m.Item(locales.ReadingSplit()).Disabled(disabled || id == "" || protected).Chosen() {
		a.smartSplitSubtitle(id, false)
	}
	if m.Item(locales.ReadingSplitForced()).Disabled(disabled || id == "" || protected).Chosen() {
		a.smartSplitSubtitle(id, true)
	}
	m.Separator()
	canMerge := func(first int) bool {
		return !disabled && first >= 0 && first+1 < len(cues) && !cues[first].ReadOnly && !cues[first+1].ReadOnly && cues[first].Translated == cues[first+1].Translated
	}
	if m.Item(locales.SubtitleMergePrevious()).Disabled(!canMerge(index - 1)).Chosen() {
		a.mergeSubtitle(id, true)
	}
	if m.Item(locales.SubtitleMergeNext()).Disabled(!canMerge(index)).Chosen() {
		a.mergeSubtitle(id, false)
	}
	m.Separator()
	if m.Item(locales.SubtitleDelete()).Shortcut(0, ui.KeyDelete).Disabled(disabled || id == "" || protected).Chosen() {
		a.deleteSubtitle(id)
	}
}

func (a *NativeApp) openSubtitleInsert(anchor string, before bool) {
	if a.busy || a.snapshot == nil || !a.commitEdit(true) {
		return
	}
	cues := a.activeDocument().Cues
	at := len(cues)
	if anchor != "" {
		at = slices.IndexFunc(cues, func(cue subtitle.Cue) bool { return cue.ID == anchor })
		if at < 0 {
			a.showError("structure", locales.ErrCuePosition())
			return
		}
		if !before {
			at++
		}
	}
	start := int64(0)
	if at > 0 {
		start = cues[at-1].EndMs
	}
	end := start + 2000
	if at < len(cues) && cues[at].StartMs > start {
		end = min(end, cues[at].StartMs)
	}
	a.insertPosition = at
	a.insertStart, a.insertEnd = float64(start)/1000, float64(end)/1000
	a.insertSource, a.insertTranslation = "", ""
	a.insertOpen = true
}

func (a *NativeApp) insertSubtitle() {
	if a.busy || a.snapshot == nil || a.store == nil {
		return
	}
	_, err := a.store.InsertCue(uuid.NewString(), a.snapshot.Revision, a.subtitleTab == 0, project.Insertion{Position: a.insertPosition, Source: a.insertSource, Translation: a.insertTranslation, StartMs: int64(math.Round(a.insertStart * 1000)), EndMs: int64(math.Round(a.insertEnd * 1000))})
	if err != nil {
		a.showError("structure", err.Error())
		return
	}
	a.insertOpen = false
	a.refreshAfterStructure(a.insertPosition)
	if a.subtitleTab == 1 {
		a.editRebuiltSource = strings.TrimSpace(a.insertTranslation) == ""
		a.loadDraft()
	}
	a.status = locales.SubtitleInserted()
}

// Merging is a local list edit. Strict formats are serialized before the
// transaction so incompatible ASS events or VTT settings never leave an
// unsavable mutation in the project; no source file is written by this check.
func (a *NativeApp) mergeSubtitle(id string, previous bool) {
	if a.busy || a.snapshot == nil || a.store == nil || !a.commitEdit(true) {
		return
	}
	doc := a.activeDocument()
	at := slices.IndexFunc(doc.Cues, func(cue subtitle.Cue) bool { return cue.ID == id })
	if previous {
		at--
	}
	command := uuid.NewString()
	merged, err := subtitle.MergeAdjacent(doc, at, "manual-"+command)
	if err == nil && doc.Format != "srt" {
		_, err = a.worker.Write(context.Background(), merged)
	}
	if err != nil {
		a.showError("structure", err.Error())
		return
	}
	if _, err = a.store.MergeCues(command, a.snapshot.Revision, a.subtitleTab == 0, doc.Cues[at].ID, doc.Cues[at+1].ID); err != nil {
		a.showError("structure", err.Error())
		return
	}
	a.refreshAfterStructure(at)
	// A hand merge is being reviewed, even while playback continues. Keep
	// that result selected until an explicit locate/seek releases the hold.
	a.followHeld = true
	a.status = locales.SubtitleMerged()
}

func (a *NativeApp) deleteSubtitle(id string) {
	if a.busy || a.snapshot == nil || a.store == nil || !a.commitEdit(true) {
		return
	}
	at := slices.IndexFunc(a.activeDocument().Cues, func(cue subtitle.Cue) bool { return cue.ID == id })
	if _, err := a.store.DeleteCue(uuid.NewString(), a.snapshot.Revision, a.subtitleTab == 0, id); err != nil {
		a.showError("structure", err.Error())
		return
	}
	a.refreshAfterStructure(at)
	a.status = locales.SubtitleDeleted()
}

func (a *NativeApp) refreshAfterStructure(index int) {
	state, err := a.store.Snapshot()
	if err != nil {
		a.showError("structure", err.Error())
		return
	}
	a.clearError("structure")
	a.pendingClick, a.rowClick = nil, nil
	a.selected = index
	a.setSnapshot(state)
	a.loadDraft()
	a.query = ""
	a.list.StopScroll()
	a.activeCue = -1
	a.locateCue = a.selected
}

func (a *NativeApp) insertSubtitleView(c *ui.Context) {
	fixedDialog(c, &a.insertOpen, locales.SubtitleInsertTitle(), 600, 480, func() {
		step := 0.001
		if format := a.activeDocument().Format; format == "ass" || format == "ssa" {
			step = 0.01
		}
		ui.Grid(c).Columns(2).Gap(em(c, 24)).Children(func() {
			timecodeField(c, locales.EditorStart(), &a.insertStart, step, false, nil)
			timecodeField(c, locales.EditorEnd(), &a.insertEnd, step, false, nil)
		})
		ui.Field(c, locales.TabSource(), func() {
			ui.TextArea(c, &a.insertSource).Height(em(c, 80)).Padding(em(c, 12)).Label(locales.TabSource()).AutoFocus()
		})
		translation := ui.Field(c, locales.SubtitleTranslationOptional(), func() {
			ui.TextArea(c, &a.insertTranslation).Height(em(c, 80)).Padding(em(c, 12)).Label(locales.SubtitleTranslationOptional())
		})
		if a.subtitleTab == 0 {
			translation.Invisible()
		}
	}, func() {
		ui.Button(c, locales.ActionCancel()).OnClick(func() { a.insertOpen = false })
		ui.PrimaryButton(c, locales.SubtitleInsertConfirm()).Disabled(strings.TrimSpace(a.insertSource) == "" || a.insertEnd <= a.insertStart).OnClick(a.insertSubtitle)
	})
}
