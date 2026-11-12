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

func (a *NativeApp) subtitleActionsMenu(c *ui.Context, layout subtitleToolbarLayout) {
	layout.button(c, locales.SubtitleActionsMenu(), lobeMenuButton(c, locales.SubtitleActionsMenu(), "ellipsis", true, func() []menuCommand {
		return append(a.subtitleStructureCommands(a.selected),
			menuCommand{Divider: true},
			menuCommand{Label: locales.FindReplace(), Hotkey: "mod+h", Run: a.openFindReplace},
			menuCommand{Label: locales.TextReplacePeriods(), Run: func() { a.cleanListPunctuation(false) }},
			menuCommand{Label: locales.TextStripTrailing(), Run: func() { a.cleanListPunctuation(true) }})
	}, func() { a.pendingClick = nil })).Disabled(a.busy || len(a.activeDocument().Cues) == 0 || a.showsTranslationProgress())
}

// Both Lobe menu presentations use these same permissions and commands. Build
// them only for an open menu; a virtual table must not rescan cues per cell.
func (a *NativeApp) subtitleStructureCommands(index int) []menuCommand {
	cues := a.activeDocument().Cues
	id, protected := "", false
	if index >= 0 && index < len(cues) {
		id, protected = cues[index].ID, cues[index].ReadOnly
	}
	disabled := a.busy || a.snapshot == nil || a.showsTranslationProgress()
	targets := a.retranslationTargets()
	_, eligible := targets[id]
	checked := len(a.checkedRetranslationIDs())
	canMerge := func(first int) bool {
		return !disabled && first >= 0 && first+1 < len(cues) && !cues[first].ReadOnly && !cues[first+1].ReadOnly && cues[first].Translated == cues[first+1].Translated
	}
	return []menuCommand{
		{Label: locales.RetranslatePick(), Disabled: disabled || a.translationLive != nil || !eligible, Run: func() { a.setSubtitleChecked(id, !a.subtitleChecked[a.subtitleTab][id]) }},
		{Label: locales.RetranslateSelected(checked), Disabled: disabled || a.translationLive != nil || checked == 0, Run: a.openRetranslation},
		{Label: locales.RetranslateCheckAll(), Disabled: disabled || a.translationLive != nil || len(targets) == 0, Run: a.checkAllSubtitles},
		{Label: locales.RetranslateClear(), Disabled: disabled || checked == 0, Run: func() { a.subtitleChecked[a.subtitleTab] = nil }},
		{Divider: true},
		// Location is not an edit and never inherits click-to-play preferences.
		{Label: locales.SubtitleLocateVideo(), Disabled: disabled || id == "" || !a.playbackAvailable(), Run: func() {
			a.pendingClick, a.rowClick = nil, nil
			a.list.StopScroll()
			a.selectSubtitle(index, "seek-pause")
		}},
		{Divider: true},
		{Label: locales.SubtitleInsertBefore(), Disabled: disabled, Run: func() { a.openSubtitleInsert(id, true) }},
		{Label: locales.SubtitleInsertAfter(), Disabled: disabled, Run: func() { a.openSubtitleInsert(id, false) }},
		{Label: locales.ReadingSplit(), Disabled: disabled || id == "" || protected, Run: func() { a.smartSplitSubtitle(id, false) }},
		{Label: locales.ReadingSplitForced(), Disabled: disabled || id == "" || protected, Run: func() { a.smartSplitSubtitle(id, true) }},
		{Divider: true},
		{Label: locales.SubtitleMergePrevious(), Disabled: !canMerge(index - 1), Run: func() { a.mergeSubtitle(id, true) }},
		{Label: locales.SubtitleMergeNext(), Disabled: !canMerge(index), Run: func() { a.mergeSubtitle(id, false) }},
		{Divider: true},
		{Label: locales.SubtitleDelete(), Hotkey: "delete", Disabled: disabled || id == "" || protected, Danger: true, Run: func() { a.deleteSubtitle(id) }},
	}
}

func (a *NativeApp) subtitleContextMenu(c *ui.Context, target ui.Element, index, row int, selected *int) {
	lobeContextMenu(c, target, func() []menuCommand {
		a.noteBrowse(c.Now())
		if index >= 0 && a.selected != index {
			return nil
		}
		return a.subtitleStructureCommands(index)
	}, func() {
		a.pendingClick = nil
		if index >= 0 && a.selected != index {
			if !a.commitEdit(true) {
				return
			}
			a.selected = index
			if selected != nil {
				*selected = row
			}
			a.loadDraft()
			c.Invalidate()
		}
		a.list.Focus(c)
	})
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
	fixedDialog(c, &a.insertOpen, locales.SubtitleInsertTitle(), 600, 0, func() {
		step := 0.001
		if format := a.activeDocument().Format; format == "ass" || format == "ssa" {
			step = 0.01
		}
		ui.Row(c).Wrap().Gap(em(c, 24)).AlignItems(ui.Start).Children(func() {
			timecodeField(c, locales.EditorStart(), &a.insertStart, step, false, nil)
			timecodeField(c, locales.EditorEnd(), &a.insertEnd, step, false, nil)
		})
		ui.Field(c, locales.TabSource(), func() {
			lobeTextArea(c, &a.insertSource).Height(em(c, 80)).Padding(em(c, 12)).Label(locales.TabSource()).AutoFocus()
		})
		translation := ui.Field(c, locales.SubtitleTranslationOptional(), func() {
			lobeTextArea(c, &a.insertTranslation).Height(em(c, 80)).Padding(em(c, 12)).Label(locales.SubtitleTranslationOptional())
		})
		if a.subtitleTab == 0 {
			translation.Invisible()
		}
	}, func() {
		lobeButton(c, locales.ActionCancel()).OnClick(func() { a.insertOpen = false })
		lobePrimaryButton(c, locales.SubtitleInsertConfirm()).Disabled(strings.TrimSpace(a.insertSource) == "" || a.insertEnd <= a.insertStart).OnClick(a.insertSubtitle)
	})
}
