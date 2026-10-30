package main

import (
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
	cues := a.activeDocument().Cues
	id := ""
	protected := false
	if a.selected >= 0 && a.selected < len(cues) {
		id = cues[a.selected].ID
		protected = cues[a.selected].ReadOnly
	}
	ui.MenuButton(c, "", func(m *ui.Menu) {
		if m.Item(locales.SubtitleInsertBefore()).Chosen() {
			a.openSubtitleInsert(id, true)
		}
		if m.Item(locales.SubtitleInsertAfter()).Chosen() {
			a.openSubtitleInsert(id, false)
		}
		m.Separator()
		if m.Item(locales.SubtitleDelete()).Disabled(id == "" || protected).Chosen() {
			a.deleteSubtitle(id)
		}
	}).Label(locales.SubtitleActionsMenu()).Tooltip(locales.SubtitleActionsMenu()).Disabled(a.busy || a.snapshot == nil || a.showsTranslationProgress()).Children(func() { icon(c, "ellipsis") })
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
			a.errorText = locales.ErrCuePosition()
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
		a.errorText = err.Error()
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

func (a *NativeApp) deleteSubtitle(id string) {
	if a.busy || a.snapshot == nil || a.store == nil || !a.commitEdit(true) {
		return
	}
	at := slices.IndexFunc(a.activeDocument().Cues, func(cue subtitle.Cue) bool { return cue.ID == id })
	if _, err := a.store.DeleteCue(uuid.NewString(), a.snapshot.Revision, a.subtitleTab == 0, id); err != nil {
		a.errorText = err.Error()
		return
	}
	a.refreshAfterStructure(at)
	a.status = locales.SubtitleDeleted()
}

func (a *NativeApp) refreshAfterStructure(index int) {
	state, err := a.store.Snapshot()
	if err != nil {
		a.errorText = err.Error()
		return
	}
	a.selected = index
	a.setSnapshot(state)
	a.loadDraft()
	a.query = ""
	a.followRow, a.activeCue = -1, -1
	a.locateCue = a.selected
}

func (a *NativeApp) insertSubtitleView(c *ui.Context) {
	ui.Modal(c, &a.insertOpen, func() {
		ui.Column(c).Width(em(c, 520)).Height(em(c, 360)).Gap(em(c, 12)).Children(func() {
			dialogTitle(c, locales.SubtitleInsertTitle())
			step := 0.001
			if format := a.activeDocument().Format; format == "ass" || format == "ssa" {
				step = 0.01
			}
			line(c, func() {
				ui.Field(c, locales.EditorStart(), func() { ui.NumberInput(c, &a.insertStart, 0, 9007199254740, step).Label(locales.EditorStart()) })
				ui.Field(c, locales.EditorEnd(), func() { ui.NumberInput(c, &a.insertEnd, 0, 9007199254740, step).Label(locales.EditorEnd()) })
			})
			ui.Field(c, locales.TabSource(), func() { ui.TextArea(c, &a.insertSource).Height(em(c, 66)).Label(locales.TabSource()).AutoFocus() })
			translation := ui.Field(c, locales.SubtitleTranslationOptional(), func() {
				ui.TextArea(c, &a.insertTranslation).Height(em(c, 66)).Label(locales.SubtitleTranslationOptional())
			})
			if a.subtitleTab == 0 {
				translation.Invisible()
			}
			ui.Spacer(c)
			ui.Row(c).Gap(em(c, 8)).Justify(ui.End).Children(func() {
				ui.Button(c, locales.ActionCancel()).OnClick(func() { a.insertOpen = false })
				ui.PrimaryButton(c, locales.SubtitleInsertConfirm()).Disabled(strings.TrimSpace(a.insertSource) == "" || a.insertEnd <= a.insertStart).OnClick(a.insertSubtitle)
			})
		})
	})
}
