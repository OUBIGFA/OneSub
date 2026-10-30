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
		hasCues := len(a.activeDocument().Cues) > 0
		// Keep the track switch centered; all list commands live in the
		// wrapping toolbar below, so narrow headers never crowd the tabs.
		panelHeader(c, nil, func() {
			tab := a.subtitleTab
			if ui.Segmented(c, &tab, locales.TabOriginal(), locales.TabRebuilt()).Label(locales.A11ySubtitleTabs()).Changed() {
				a.changeSubtitleTab(tab)
			}
		}, nil)
		ui.Divider(c)
		ui.Row(c).Padding(em(c, 8), em(c, 12)).Gap(em(c, 8)).Wrap().AlignItems(ui.Center).Shrink(0).Label(locales.A11ySubtitleActions()).Children(func() {
			ui.Toolbar(c, func() {
				a.subtitleActionsMenu(c)
				toolIcon(c, locales.ListLocate(), "locate-fixed").Disabled(!hasCues).OnClick(a.locateCurrent)
			}).Padding(0).Shrink(0)
			if ui.SearchField(c, &a.query).Placeholder(locales.ListSearch()).Label(locales.ListSearch()).MinWidth(em(c, 140)).Grow(1).Changed() {
				a.followRow = -1
			}
			if a.translationCandidate != nil {
				ui.Button(c, locales.TranslationResults()).OnClick(func() { a.translationReview = true })
			}
			ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Children(func() {
				unsaved := a.trackUnsaved(a.subtitleTab)
				if unsaved {
					ui.Badge(c, locales.ListUnsaved()).Background(c.Theme().Warning.Alpha(0.16)).TextColor(c.Theme().Warning)
				}
				iconButton(c, locales.ListSave(), "save").Disabled(a.busy || a.showsTranslationProgress() || a.snapshot == nil || !unsaved).OnClick(func() { a.saveTrack(a.subtitleTab, false) })
				ui.Button(c, locales.ListSaveAs()).Disabled(a.busy || a.showsTranslationProgress() || !a.trackHasContent(a.subtitleTab)).OnClick(func() { a.saveTrack(a.subtitleTab, true) })
			})
		})
		if a.showsTranslationProgress() {
			a.translationProgressView(c)
			return
		}
		if !hasCues {
			ui.Column(c).Grow(1).Center().Children(func() { muted(c, locales.ListEmpty()) }).ContextMenu(func(m *ui.Menu) {
				a.subtitleStructureMenu(m, -1)
			})
			return
		}
		cues := a.activeDocument().Cues
		rows := []int{}
		for i, cue := range cues {
			if strings.Contains(strings.ToLower(cueDisplayText(cue, a.subtitleTab)+"\n"+cueDisplayText(cue, 0)), strings.ToLower(a.query)) {
				rows = append(rows, i)
			}
		}
		rowOf := func(index int) int {
			for row, i := range rows {
				if i == index {
					return row
				}
			}
			return -1
		}
		selected := rowOf(a.selected)
		a.list.Selected = &selected
		a.list.Key = func(i int) any { return cues[rows[i]].ID }
		a.list.CellMenu = func(m *ui.Menu, row, col int) {
			index := rows[row]
			// MyGo's single-selection list keeps its old selection on a
			// right-click. Select this cue explicitly, without a playback action.
			if a.selected != index {
				if !a.commitEdit(true) {
					return
				}
				a.selected, selected = index, row
				a.loadDraft()
				c.Invalidate()
			}
			a.list.Focus(c)
			a.subtitleStructureMenu(m, index)
		}
		maximum := 1
		for _, cue := range cues {
			maximum = max(maximum, cue.Position+1)
		}
		cols := []ui.TableColumn{numberColumn(c, maximum), timeColumn(c, locales.ListColumnStart()), timeColumn(c, locales.ListColumnEnd()), {Title: locales.ListColumnText()}}
		table := subtitleTable(c, &a.list, cols, len(rows), func(row, col int) {
			index := rows[row]
			cue := cues[index]
			// The row being edited shows the draft, so typing updates the
			// list before the edit reaches the project.
			text, start, end := cueDisplayText(cue, a.subtitleTab), float64(cue.StartMs)/1000, float64(cue.EndMs)/1000
			if index == a.selected && a.editPending {
				if a.editorTextTrack() == a.subtitleTab {
					text = a.draft
				}
				start, end = a.start, a.end
			}
			// Secondary columns are muted, but on the accent of the chosen
			// row they take its text color.
			cell := func(value string) {
				t := ui.Text(c, value).FontFeatures("tnum").SingleLine()
				if index != a.selected {
					t.TextColor(c.Theme().TextMuted)
				}
			}
			switch col {
			case 0:
				cell(fmt.Sprint(cue.Position + 1))
			case 1:
				cell(stamp(start))
			case 2:
				cell(stamp(end))
			default:
				// Every line is shown; rows grow to their text.
				ui.Column(c).Gap(em(c, 4)).Children(func() {
					if a.subtitleTab == 1 {
						source := cueDisplayText(cue, 0)
						if index == a.selected && a.editPending && a.editRebuiltSource {
							source = a.draft
						}
						muted(c, source)
					}
					ui.Text(c, text)
				})
			}
		}, func(bool) {
			// Reading another part of the list must not seek or edit a cue,
			// nor be undone immediately by a pending playback locate.
			a.followPaused, a.locateCue = true, -1
		}).Label(locales.A11ySubtitleList()).ContextMenu(func(m *ui.Menu) {
			// The blank list area and keyboard menu use the current selection.
			a.list.Focus(c)
			a.subtitleStructureMenu(m, a.selected)
		})
		// A hand scroll while the video plays stops the list following it;
		// the scroll still happens. The cue at the playhead stays chosen.
		table.HandleInput(func(ev ui.InputEvent) bool {
			if ev.Kind == ui.InputScroll && !a.paused {
				a.followPaused = true
			}
			return false
		})
		if (table.Changed() || table.Submitted()) && selected >= 0 && selected < len(rows) {
			action := a.settings.SingleClickAction
			if table.Submitted() {
				action = a.settings.DoubleClickAction
			}
			a.selectSubtitle(rows[selected], action)
		}
		// Dragging the scroll bar moves the followed row out of view, which
		// stops following as a hand scroll does. The rows in view are those of
		// the last frame, so this comes before this frame asks to scroll.
		if !a.paused && !a.followPaused && a.followRow >= 0 {
			if first, last := a.list.Visible(); last >= first && (a.followRow < first || a.followRow > last) {
				a.followPaused = true
			}
		}
		if a.locateCue >= 0 {
			if row := rowOf(a.locateCue); row >= 0 {
				a.list.ScrollTo(row, ui.Center)
				a.followRow = row
			}
			a.locateCue = -1
		}
		// While the video plays the cue at the playhead is chosen; the list
		// keeps it in the middle until the user scrolls away. Typing keeps
		// the cue being edited.
		if a.settings.FollowPlayback && !a.paused && !a.savePromptOpen {
			active := cueAt(cues, a.position)
			if active >= 0 && active != a.activeCue {
				a.activeCue = active
				if row := rowOf(active); row >= 0 {
					if !a.editing && !a.composing {
						a.commitEdit(true)
						a.selected = active
						a.loadDraft()
					}
					if !a.followPaused {
						a.list.ScrollTo(row, ui.Center)
						a.followRow = row
					}
				}
			}
		}
	})
}

// locateCurrent chooses the cue at the playhead, shows it in the middle of
// the list and follows playback again. A search hiding it is cleared.
func (a *NativeApp) locateCurrent() {
	if a.snapshot == nil {
		return
	}
	cues := a.activeDocument().Cues
	index := cueNear(cues, a.position)
	if index < 0 {
		return
	}
	if !strings.Contains(strings.ToLower(cueDisplayText(cues[index], a.subtitleTab)), strings.ToLower(a.query)) {
		a.query = ""
	}
	a.followPaused, a.followRow = false, -1
	a.activeCue = index
	if !a.editing && !a.composing && index != a.selected {
		a.commitEdit(true)
		a.selected = index
		a.loadDraft()
	}
	a.locateCue = index
}

// timeColumn fits a timestamp in the active font, digits of one width.
func timeColumn(c *ui.Context, title string) ui.TableColumn {
	width, _ := c.MeasureText(0, ui.Span{Text: stamp(0)})
	width += 2*c.Theme().Space(2.5) + 4
	return ui.TableColumn{Title: title, Width: width, MinWidth: width}
}

func (a *NativeApp) editorView(c *ui.Context) {
	valid := !a.showsTranslationProgress() && a.snapshot != nil && a.selected >= 0 && a.selected < len(a.activeDocument().Cues)
	ui.Column(c).Fill().Label(locales.A11yEditorPanel()).Children(func() {
		panelHeader(c, nil, func() { panelTitle(c, locales.EditorTitle()) }, func() {
			ui.Toolbar(c, func() {
				toolIcon(c, locales.EditorUndo(), "undo-2").Disabled(a.busy || a.snapshot == nil || !(a.snapshot.CanUndo || a.editPending)).OnClick(func() { a.undo(false) })
				toolIcon(c, locales.EditorRedo(), "redo-2").Disabled(a.busy || a.snapshot == nil || !a.snapshot.CanRedo).OnClick(func() { a.undo(true) })
			}).Padding(0).Gap(em(c, 2))
		})
		ui.Divider(c)
		if !valid {
			ui.Column(c).Grow(1).Center().Children(func() { muted(c, locales.EditorNone()) })
			return
		}
		cue := a.activeDocument().Cues[a.selected]
		readonly := cue.ReadOnly || a.busy
		fixedTiming := readonly || a.snapshot.Document.Format == "ass" || a.snapshot.Document.Format == "ssa"
		// A short panel scrolls rather than hide the text; a tall one gives
		// the text box the room left.
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).MinHeightPercent(100).Padding(em(c, 12), em(c, panelPad)).Gap(em(c, 12)).Children(func() {
				// The cue being edited heads the editor, centered over its fields.
				ui.Text(c, locales.EditorCueNumber(cue.Position+1)).FontWeight(600).FontFeatures("tnum").SingleLine().AlignSelf(ui.Center)
				// Each label stays with its value when the row wraps.
				pair := func(label string, value func()) {
					ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Children(func() {
						muted(c, label).SingleLine()
						value()
					})
				}
				ui.Row(c).GapX(em(c, 16)).GapY(em(c, 8)).Wrap().AlignItems(ui.Center).Justify(ui.Center).Children(func() {
					pair(locales.EditorStart(), func() {
						ui.NumberInput(c, &a.start, 0, 9007199254740, 0.001).Label(locales.EditorStart()).Disabled(fixedTiming).OnChange(func() { a.changed(c.Now()) })
					})
					pair(locales.EditorEnd(), func() {
						ui.NumberInput(c, &a.end, 0, 9007199254740, 0.001).Label(locales.EditorEnd()).Disabled(fixedTiming).OnChange(func() { a.changed(c.Now()) })
					})
					pair(locales.EditorDuration(), func() {
						ui.Text(c, locales.EditorSeconds(math.Max(0, a.end-a.start))).FontFeatures("tnum").SingleLine()
					})
				})
				// The text speaks for itself; the name is for assistive technology.
				if a.subtitleTab == 1 {
					textTrack := a.editorTextTrack()
					if ui.Segmented(c, &textTrack, locales.TabSource(), locales.TabTranslation()).AlignSelf(ui.Center).Changed() {
						a.commitEdit(true)
						a.editRebuiltSource = textTrack == 0
						a.loadDraft()
					}
				}
				area := ui.TextArea(c.Key(cue.ID), &a.draft).Grow(1).MinHeight(em(c, 64)).ReadOnly(readonly).Label(locales.A11yCueText())
				a.composing = area.Composing()
				a.editing = area.Focused()
				if area.Changed() {
					a.changed(c.Now())
				}
			})
		})
	})
}

// changed records that the draft differs from the stored cue and schedules
// its commit after the typing pause.
func (a *NativeApp) changed(now time.Time) {
	if a.snapshot == nil || a.selected < 0 || a.selected >= len(a.activeDocument().Cues) {
		return
	}
	cue := a.activeDocument().Cues[a.selected]
	a.editPending = a.draft != cueDisplayText(cue, a.editorTextTrack()) || int64(math.Round(a.start*1000)) != cue.StartMs || int64(math.Round(a.end*1000)) != cue.EndMs
	a.editDue = now.Add(editCommitDelay)
}
func (a *NativeApp) loadDraft() {
	if a.snapshot != nil && a.selected >= 0 && a.selected < len(a.activeDocument().Cues) {
		cue := a.activeDocument().Cues[a.selected]
		a.draft = cueDisplayText(cue, a.editorTextTrack())
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
	if a.store == nil || a.snapshot == nil || a.selected < 0 || a.selected >= len(a.activeDocument().Cues) {
		a.loadDraft()
		return true
	}
	cue := a.activeDocument().Cues[a.selected]
	track := "original"
	if a.subtitleTab == 1 {
		track = "source"
		if !a.editRebuiltSource {
			track = "translation"
		}
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
	a.selected = min(a.selected, len(a.activeDocument().Cues)-1)
	if a.selected < 0 && len(a.activeDocument().Cues) > 0 {
		a.selected = 0
	}
	a.followRow, a.activeCue = -1, -1
	a.loadDraft()
}
func (a *NativeApp) selectSubtitle(index int, action string) {
	a.commitEdit(true)
	a.selected = index
	a.loadDraft()
	// A cue chosen to seek to is where playback goes on: follow it again.
	if action != "none" && action != "" {
		a.followPaused = false
		a.followRow = -1
	}
	a.seekSubtitle(action)
}
