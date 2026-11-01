package main

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
	"github.com/google/uuid"
)

// editCommitDelay is the typing pause after which a draft becomes one undo
// step in the project. Selecting, saving or any task commits it at once.
const editCommitDelay = 600 * time.Millisecond

// Reuse MyGo's native easing; no timers or row transforms in the workbench.
var subtitleFollowAnimation = ui.Animation{Duration: 220 * time.Millisecond, Ease: ui.EaseOut}

// browseGrace is how long the user's own browsing keeps the subtitle list
// where they left it. The playback keeps choosing the cue at the playhead,
// but the list does not move under the reader; after this pause the follow
// takes the list back, so nothing stops following for good.
const browseGrace = 4 * time.Second

// noteBrowse says the user took the list somewhere themselves: the wheel, the
// scroll bar, the first and last buttons, or the keys that move the choice.
// The viewport stays theirs until the grace is over.
func (a *NativeApp) noteBrowse(now time.Time) {
	a.browseUntil = now.Add(browseGrace)
}

// browsing says the user's own browsing still holds the viewport.
func (a *NativeApp) browsing(now time.Time) bool {
	return !a.browseUntil.IsZero() && now.Before(a.browseUntil)
}

// resumeFollow gives the viewport back to the playback at once, as the
// locate button and a new playback do.
func (a *NativeApp) resumeFollow() {
	a.browseUntil = time.Time{}
	a.followRow, a.followPending = -1, false
}

func subtitleSearchLabel(cues []subtitle.Cue) string {
	if len(cues) == 0 {
		return locales.ListSearchRange(0, 0)
	}
	return locales.ListSearchRange(cues[0].Position+1, cues[len(cues)-1].Position+1)
}

func (a *NativeApp) subtitlesView(c *ui.Context) {
	ui.Column(c).Fill().Children(func() {
		hasCues := len(a.activeDocument().Cues) > 0
		// Keep the track switch centered; all list commands live in the
		// wrapping toolbar below, so narrow headers never crowd the tabs.
		panelHeader(c, nil, func() {
			tab := a.subtitleTab
			if ui.Tabs(c, &tab, locales.TabOriginal(), locales.TabRebuilt()).Label(locales.A11ySubtitleTabs()).Changed() {
				a.changeSubtitleTab(tab)
			}
		}, nil)
		ui.Divider(c)
		ui.Column(c).Padding(em(c, 12), em(c, 16)).Gap(em(c, 12)).Shrink(0).Label(locales.A11ySubtitleActions()).Children(func() {
			ui.Row(c).Gap(em(c, 12)).AlignItems(ui.Center).Children(func() {
				ui.Toolbar(c, func() {
					a.subtitleActionsMenu(c)
					toolIcon(c, locales.ListLocate(), "locate-fixed").Disabled(!hasCues).OnClick(a.locateCurrent)
				}).Padding(0).Shrink(0)
				searchCues := a.activeDocument().Cues
				if a.showsTranslationProgress() {
					searchCues = a.translationLive.units
				}
				searchLabel := subtitleSearchLabel(searchCues)
				searchWidth, _ := c.MeasureText(0, ui.Span{Text: searchLabel})
				// Counts are UI labels, not user content: reserve the native icon,
				// gaps and padding so even the largest track range remains whole.
				if ui.SearchField(c, &a.query).Placeholder(searchLabel).Label(locales.ListSearch()).Tooltip(searchLabel).MinWidth(max(em(c, 140), searchWidth+c.Theme().Space(11))).Grow(1).Changed() {
					a.list.StopScroll()
					a.activeCue = -1
					// A search is the user's own browsing of the list.
					a.noteBrowse(c.Now())
					if a.translationLive != nil {
						a.translationLive.list.StopScroll()
						a.translationLive.shown = -1
					}
				}
			})
			ui.Row(c).Gap(em(c, 12)).Wrap().AlignItems(ui.Center).Children(func() {
				if a.translationCandidate != nil {
					ui.Button(c, locales.TranslationResults()).OnClick(func() { a.translationReview = true })
				}
				ui.Spacer(c)
				ui.Row(c).Gap(em(c, 12)).AlignItems(ui.Center).Children(func() {
					unsaved := a.trackUnsaved(a.subtitleTab)
					if unsaved {
						ui.Badge(c, locales.ListUnsaved()).Background(studioColors(c).warningSoft).TextColor(c.Theme().Warning)
					}
					iconButton(c, locales.ListSave(), "save").Disabled(a.busy || a.showsTranslationProgress() || a.snapshot == nil || !unsaved).OnClick(func() { a.saveTrack(a.subtitleTab, false) })
					ui.Button(c, locales.ListSaveAs()).Disabled(a.busy || a.showsTranslationProgress() || !a.trackHasContent(a.subtitleTab)).OnClick(func() { a.saveTrack(a.subtitleTab, true) })
				})
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
				t := ui.Text(c, value).FontSize(em(c, captionSize)).FontFeatures("tnum").SingleLine()
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
				ui.Column(c).Gap(em(c, 8)).Padding(em(c, 6), em(c, 2)).Children(func() {
					if a.subtitleTab == 1 {
						source := cueDisplayText(cue, 0)
						if index == a.selected && a.editPending && a.editRebuiltSource {
							source = a.draft
						}
						label := muted(c, source)
						if index == a.selected {
							label.TextColor(c.Theme().AccentText)
						}
					}
					ui.Text(c, text)
				})
			}
		}, func(bool) {
			// Navigation changes only the viewport. The playback still
			// chooses the cue at the playhead; the list stays where the
			// reader put it until the grace is over.
			a.noteBrowse(c.Now())
			a.locateCue = -1
		}).Label(locales.A11ySubtitleList()).ContextMenu(func(m *ui.Menu) {
			// The blank list area and keyboard menu use the current selection.
			a.list.Focus(c)
			a.subtitleStructureMenu(m, a.selected)
		})
		// The wheel and the touchpad scroll the list by the user's own hand.
		// The scroll still happens; the playback's follow waits for the
		// reading to be over.
		table.HandleInput(func(ev ui.InputEvent) bool {
			if ev.Kind == ui.InputScroll && !a.paused {
				a.noteBrowse(ev.Time)
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
		if a.locateCue >= 0 {
			if row := rowOf(a.locateCue); row >= 0 {
				a.list.ScrollToAnimated(row, ui.Center, subtitleFollowAnimation)
				a.followRow = row
			}
			a.locateCue = -1
		}
		// Dragging the scroll bar moves the followed row out of view, which
		// the wheel's notches do too. The rows in view are those of the last
		// frame, so this comes before this frame asks to scroll.
		if !a.paused && a.followRow >= 0 {
			if first, last := a.list.Visible(); last >= first && (a.followRow < first || a.followRow > last) {
				a.noteBrowse(c.Now())
				a.followPending = true
			}
		}
		// While the video plays the cue at the playhead is chosen; the list
		// keeps it in the middle until the user scrolls away. Typing keeps
		// the cue being edited.
		if a.settings.FollowPlayback && !a.paused && !a.savePromptOpen {
			active := cueAt(cues, a.position)
			if active >= 0 && (active != a.activeCue || a.followPending) {
				if active != a.activeCue {
					a.activeCue = active
					if !a.editing && !a.composing {
						a.commitEdit(true)
						a.selected = active
						a.loadDraft()
					}
				}
				if row := rowOf(active); row >= 0 {
					// The user's own browsing keeps the viewport for a
					// while: the cue at the playhead is still chosen, but
					// the list is not pulled away from the reader. The
					// grace ends and the follow takes it back.
					if a.browsing(c.Now()) {
						a.followPending = true
						a.followRow = -1
					} else {
						a.list.ScrollToAnimated(row, ui.Center, subtitleFollowAnimation)
						a.followPending = false
						a.followRow = row
					}
				}
			}
		}
		// The grace ends on its own: the frame that follows takes the list
		// back to the cue at the playhead.
		if a.browsing(c.Now()) {
			c.After(a.browseUntil.Sub(c.Now()))
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
	a.list.StopScroll()
	a.resumeFollow()
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
		scroll := ui.Scroll(c).Grow(1)
		viewport := ui.Local(scroll, "editor-viewport", func() float32 { return 0 })
		if height := scroll.Bounds().H; height != *viewport {
			*viewport = height
			c.After(0)
		}
		minimum := float32(340)
		if a.subtitleTab == 1 {
			minimum = 400
		}
		scroll.Children(func() {
			ui.Column(c).Height(max(*viewport, em(c, minimum))).Padding(em(c, 16), em(c, panelPad)).Gap(em(c, 20)).Children(func() {
				// Cue number and duration are peer metadata at opposite edges.
				ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
					muted(c, locales.EditorCueNumber(cue.Position+1)).FontFeatures("tnum").SingleLine().Shrink(0)
					muted(c, locales.EditorSeconds(math.Max(0, a.end-a.start))).Label(locales.EditorDuration()).FontFeatures("tnum").SingleLine().Shrink(0)
				})
				parameterGrid(c, 160, 2, func() {
					ui.Field(c, locales.EditorStart(), func() {
						ui.NumberInput(c, &a.start, 0, 9007199254740, 0.001).Label(locales.EditorStart()).Disabled(fixedTiming).OnChange(func() { a.changed(c.Now()) })
					})
					ui.Field(c, locales.EditorEnd(), func() {
						ui.NumberInput(c, &a.end, 0, 9007199254740, 0.001).Label(locales.EditorEnd()).Disabled(fixedTiming).OnChange(func() { a.changed(c.Now()) })
					})
				})
				ui.Divider(c)
				// The text speaks for itself; the name is for assistive technology.
				if a.subtitleTab == 1 {
					textTrack := a.editorTextTrack()
					if ui.Tabs(c, &textTrack, locales.TabSource(), locales.TabTranslation()).AlignSelf(ui.Start).Changed() {
						a.commitEdit(true)
						a.editRebuiltSource = textTrack == 0
						a.loadDraft()
					}
				}
				area := ui.TextArea(c.Key(cue.ID), &a.draft).Grow(1).MinHeight(em(c, 104)).Padding(em(c, 16)).FontSize(em(c, 17)).ReadOnly(readonly).Label(locales.A11yCueText())
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
		a.showError("edit", locales.ErrEditRestored(err.Error()))
		a.loadDraft()
		return true
	}
	state, err := a.store.Snapshot()
	if err != nil {
		a.showError("edit", err.Error())
		return false
	}
	a.setSnapshot(state)
	a.editPending = false
	a.editDue = time.Time{}
	a.clearError("edit")
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
		a.showError("edit", err.Error())
		return
	}
	state, err := a.store.Snapshot()
	if err != nil {
		a.showError("edit", err.Error())
		return
	}
	a.clearError("edit")
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
	a.list.StopScroll()
	a.activeCue = -1
	// The other track is a new list: the follow starts again from the cue at
	// the playhead, as it does for a new playback.
	a.resumeFollow()
	a.loadDraft()
}
func (a *NativeApp) selectSubtitle(index int, action string) {
	a.commitEdit(true)
	a.selected = index
	a.loadDraft()
	// A cue chosen to seek to is where the playback goes on: the follow
	// takes the list back to it.
	if action != "none" && action != "" {
		a.resumeFollow()
	}
	a.seekSubtitle(action)
}
