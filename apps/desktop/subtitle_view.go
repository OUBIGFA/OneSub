package main

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
	"github.com/google/uuid"
)

// editCommitDelay is the typing pause after which a draft becomes one undo
// step in the project. Selecting, saving or any task commits it at once.
const editCommitDelay = 600 * time.Millisecond

func (a *NativeApp) subtitlesView(c *ui.Context) {
	ui.Column(c).Fill().Children(func() {
		// The translate action sits on the left; equal flexible cells keep
		// the source/translation tabs centred as one group.
		ui.Row(c).Padding(8, 12).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Grow(1).Basis(0).Children(func() {
				iconButton(c, locales.ActionTranslate(), "languages").Disabled(a.busy || a.snapshot == nil || len(a.snapshot.Document.Cues) == 0).OnClick(a.translate)
			})
			previousTab := a.subtitleTab
			ui.Tabs(c, &a.subtitleTab, locales.TabSource(), locales.TabTranslation()).Label(locales.A11ySubtitleTabs()).OnChange(func() {
				nextTab := a.subtitleTab
				a.subtitleTab = previousTab
				a.changeSubtitleTab(nextTab)
			})
			ui.Row(c).Grow(1).Basis(0)
		})
		ui.Toolbar(c, func() {
			ui.TextInput(c, &a.query).Placeholder(locales.ListSearch()).Label(locales.ListSearch()).Grow(1)
			if a.translationCandidate != nil {
				ui.Button(c, locales.ListReview()).OnClick(func() { a.translationReview = true })
			}
			unsaved := a.trackUnsaved(a.subtitleTab)
			if unsaved {
				ui.Text(c, locales.ListUnsaved()).TextColor(c.Theme().Warning).SingleLine()
			}
			iconButton(c, locales.ListSave(), "save").Disabled(a.busy || a.snapshot == nil || !unsaved).OnClick(func() { a.saveTrack(a.subtitleTab, false) })
			ui.Button(c, locales.ListSaveAs()).Disabled(a.busy || !a.trackHasContent(a.subtitleTab)).OnClick(func() { a.saveTrack(a.subtitleTab, true) })
		}).Padding(8, 12).Label(locales.A11ySubtitleActions())
		if a.snapshot == nil || len(a.snapshot.Document.Cues) == 0 {
			ui.Column(c).Grow(1).Center().Children(func() { ui.Text(c, locales.ListEmpty()).TextColor(c.Theme().TextMuted) })
			return
		}
		cues := a.snapshot.Document.Cues
		rows := []int{}
		for i, cue := range cues {
			if strings.Contains(strings.ToLower(cueDisplayText(cue, a.subtitleTab)), strings.ToLower(a.query)) {
				rows = append(rows, i)
			}
		}
		selected := -1
		for i, index := range rows {
			if index == a.selected {
				selected = i
			}
		}
		a.list.Selected = &selected
		a.list.Key = func(i int) any { return cues[rows[i]].ID }
		maximum := 1
		for _, cue := range cues {
			maximum = max(maximum, cue.Position+1)
		}
		cols := []ui.TableColumn{numberColumn(c, maximum), {Title: locales.ListColumnStart(), Width: 128}, {Title: locales.ListColumnEnd(), Width: 128}, {Title: locales.ListColumnText()}}
		table := ui.Table(c, &a.list, cols, len(rows), func(row, col int) {
			index := rows[row]
			cue := cues[index]
			// The row being edited shows the draft, so typing updates the
			// list before the edit reaches the project.
			text, start, end := cueDisplayText(cue, a.subtitleTab), float64(cue.StartMs)/1000, float64(cue.EndMs)/1000
			if index == a.selected && a.editPending {
				text, start, end = a.draft, a.start, a.end
			}
			switch col {
			case 0:
				ui.Text(c, fmt.Sprint(cue.Position+1)).SingleLine()
			case 1:
				ui.Text(c, stamp(start)).SingleLine()
			case 2:
				ui.Text(c, stamp(end)).SingleLine()
			default:
				// Every line is shown; rows grow to their text.
				ui.Text(c, text)
			}
		}).Grow(1).Label(locales.A11ySubtitleList())
		if (table.Changed() || table.Submitted()) && selected >= 0 && selected < len(rows) {
			action := a.settings.SingleClickAction
			if table.Submitted() {
				action = a.settings.DoubleClickAction
			}
			a.selectSubtitle(rows[selected], action)
		}
		// While the video plays the list follows it, also after the user
		// scrolled away. Typing keeps its cue; the list still scrolls.
		if a.settings.FollowPlayback && !a.paused && !a.savePromptOpen {
			active := cueAt(cues, a.position)
			if active >= 0 && active != a.activeCue {
				a.activeCue = active
				for row, index := range rows {
					if index != active {
						continue
					}
					if !a.editing && !a.composing {
						a.commitEdit(true)
						a.selected = index
						a.loadDraft()
					}
					a.list.ScrollIntoView(row)
					break
				}
			}
		}
	})
}

func (a *NativeApp) editorView(c *ui.Context) {
	valid := a.snapshot != nil && a.selected >= 0 && a.selected < len(a.snapshot.Document.Cues)
	ui.Scroll(c).Fill().Background(c.Theme().Surface).Children(func() {
		ui.Column(c).Padding(18, 20).Gap(12).Children(func() {
			// The same header height as the style panel keeps both titles level.
			ui.Row(c).Height(34).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, locales.EditorTitle()).FontWeight(600)
				if valid {
					ui.Text(c, locales.EditorCueNumber(a.snapshot.Document.Cues[a.selected].Position+1)).TextColor(c.Theme().TextMuted).SingleLine()
				}
				ui.Spacer(c)
				toolIcon(c, locales.EditorUndo(), "undo-2").Disabled(a.busy || a.snapshot == nil || !(a.snapshot.CanUndo || a.editPending)).OnClick(func() { a.undo(false) })
				toolIcon(c, locales.EditorRedo(), "redo-2").Disabled(a.busy || a.snapshot == nil || !a.snapshot.CanRedo).OnClick(func() { a.undo(true) })
			})
			if !valid {
				ui.Column(c).Height(160).Center().Children(func() { ui.Text(c, locales.EditorNone()).TextColor(c.Theme().TextMuted) })
				return
			}
			cue := a.snapshot.Document.Cues[a.selected]
			readonly := cue.ReadOnly || a.busy
			fixedTiming := readonly || a.snapshot.Document.Format == "ass" || a.snapshot.Document.Format == "ssa"
			ui.Row(c).Gap(12).Wrap().AlignItems(ui.Center).Children(func() {
				ui.Text(c, locales.EditorStart())
				ui.NumberInput(c, &a.start, 0, 9007199254740, 0.001).Width(150).Label(locales.EditorStart()).Disabled(fixedTiming).OnChange(func() { a.changed(c.Now()) })
				ui.Text(c, locales.EditorEnd())
				ui.NumberInput(c, &a.end, 0, 9007199254740, 0.001).Width(150).Label(locales.EditorEnd()).Disabled(fixedTiming).OnChange(func() { a.changed(c.Now()) })
			})
			// The text speaks for itself; the name is for assistive technology.
			area := ui.TextArea(c.Key(cue.ID), &a.draft).Height(110).ReadOnly(readonly).Label(locales.A11yCueText())
			a.composing = area.Composing()
			a.editing = area.Focused()
			if area.Changed() {
				a.changed(c.Now())
			}
		})
	})
}

// changed records that the draft differs from the stored cue and schedules
// its commit after the typing pause.
func (a *NativeApp) changed(now time.Time) {
	if a.snapshot == nil || a.selected < 0 || a.selected >= len(a.snapshot.Document.Cues) {
		return
	}
	cue := a.snapshot.Document.Cues[a.selected]
	a.editPending = a.draft != cueDisplayText(cue, a.subtitleTab) || int64(math.Round(a.start*1000)) != cue.StartMs || int64(math.Round(a.end*1000)) != cue.EndMs
	a.editDue = now.Add(editCommitDelay)
}
func (a *NativeApp) loadDraft() {
	if a.snapshot != nil && a.selected >= 0 && a.selected < len(a.snapshot.Document.Cues) {
		cue := a.snapshot.Document.Cues[a.selected]
		a.draft = cueDisplayText(cue, a.subtitleTab)
		a.start = float64(cue.StartMs) / 1000
		a.end = float64(cue.EndMs) / 1000
	} else {
		a.draft = ""
		a.start = 0
		a.end = 0
	}
	a.editPending = false
	a.editDue = time.Time{}
}

// commitEdit writes the draft as one undo step. While typing, an invalid
// draft (empty text, end before start) waits for the next change. Leaving
// the cue restores the stored values instead and says why.
func (a *NativeApp) commitEdit(leaving bool) bool {
	if !a.editPending {
		return true
	}
	if a.store == nil || a.snapshot == nil || a.selected < 0 || a.selected >= len(a.snapshot.Document.Cues) {
		a.loadDraft()
		return true
	}
	cue := a.snapshot.Document.Cues[a.selected]
	track := "source"
	if a.subtitleTab == 1 {
		track = "translation"
	}
	_, err := a.store.Apply(uuid.NewString(), a.snapshot.Revision, []project.Edit{{ID: cue.ID, Text: a.draft, Track: track, StartMs: int64(math.Round(a.start * 1000)), EndMs: int64(math.Round(a.end * 1000))}})
	if err != nil {
		if !leaving {
			a.editDue = time.Time{}
			return false
		}
		a.errorText = locales.ErrEditRestored(err.Error())
		a.loadDraft()
		return true
	}
	state, err := a.store.Snapshot()
	if err != nil {
		a.errorText = err.Error()
		return false
	}
	a.setSnapshot(state)
	a.editPending = false
	a.editDue = time.Time{}
	a.errorText = ""
	return true
}
func (a *NativeApp) undo(redo bool) {
	if a.store == nil || a.snapshot == nil {
		return
	}
	// An uncommitted draft is the newest change: undo starts with it.
	if a.editPending && !redo {
		a.loadDraft()
		return
	}
	if !a.commitEdit(true) {
		return
	}
	if _, err := a.store.Undo(uuid.NewString(), a.snapshot.Revision, redo); err != nil {
		a.errorText = err.Error()
		return
	}
	state, err := a.store.Snapshot()
	if err != nil {
		a.errorText = err.Error()
		return
	}
	a.setSnapshot(state)
	if a.subtitleTab == 1 {
		hasTranslation := false
		for _, cue := range state.Document.Cues {
			if cue.Translated {
				hasTranslation = true
				break
			}
		}
		if !hasTranslation {
			a.subtitleTab = 0
			a.settings.SubtitleOutput.Mode = "source"
		}
	}
	a.loadDraft()
}
func (a *NativeApp) changeSubtitleTab(tab int) {
	a.commitEdit(true)
	a.subtitleTab = tab
	a.loadDraft()
}
func (a *NativeApp) selectSubtitle(index int, action string) {
	a.commitEdit(true)
	a.selected = index
	a.loadDraft()
	a.seekSubtitle(action)
}
