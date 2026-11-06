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
	a.followHeld = false
	a.followRow, a.followPending = -1, true
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
		// One command row gives the table its height back. Native wrapping is
		// only needed when a deliberately narrow pane cannot fit whole groups.
		ui.Row(c).Padding(em(c, 12), em(c, 16)).Gap(em(c, 8)).Wrap().AlignItems(ui.Center).Shrink(0).Label(locales.A11ySubtitleActions()).Children(func() {
			ui.Toolbar(c, func() {
				toolIcon(c, locales.ListImport(), "folder-open").Disabled(a.busy).OnClick(a.pickSubtitle)
				a.subtitleActionsMenu(c)
				toolIcon(c, locales.ListLocate(), "locate-fixed").Disabled(!hasCues).OnClick(a.locateCurrent)
				// Keep every command visible; the current track controls its
				// availability, not whether it is built into the toolbar.
				ui.Divider(c).Height(em(c, 20)).AlignSelf(ui.Center).Margin(0, em(c, 4))
				a.listPunctuationTools(c)
				toolIcon(c, locales.FindReplace(), "text-search").Disabled(!hasCues || a.busy || a.showsTranslationProgress()).OnClick(a.openFindReplace)
			}).Padding(0).Shrink(0)
			a.subtitleSelectionTools(c)
			searchCues := a.activeDocument().Cues
			if a.showsTranslationProgress() {
				searchCues = a.translationLive.units
			}
			searchLabel := subtitleSearchLabel(searchCues)
			searchWidth, _ := c.MeasureText(0, ui.Span{Text: searchLabel})
			searchMinimum := max(em(c, 140), searchWidth+c.Theme().Space(11))
			// Editing tools form one group before search, so Find and replace
			// stays next to punctuation even when the outer toolbar wraps.
			if ui.SearchField(c, &a.query).Placeholder(searchLabel).Label(locales.ListSearch()).Tooltip(searchLabel).Disabled(len(searchCues) == 0).MinWidth(searchMinimum).Grow(1).Changed() {
				a.pendingClick = nil
				a.list.StopScroll()
				a.activeCue = -1
				a.noteBrowse(c.Now())
				if a.translationLive != nil {
					a.translationLive.list.StopScroll()
					a.translationLive.shown = -1
				}
			}
			a.readingTools(c)
			a.retranslationTools(c)
			// Keep the review slot icon-sized. A long disabled label would
			// wrap again and leave no subtitle row at the minimum pane size.
			toolIcon(c, locales.TranslationResults(), "file-warning").Disabled(a.translationCandidate == nil).OnClick(func() { a.translationReview = true })
			ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Shrink(0).Label(locales.A11ySubtitleFileActions()).Children(func() {
				// The divider travels with the file actions when groups wrap.
				ui.Divider(c).Height(em(c, 20)).AlignSelf(ui.Center).Margin(0, em(c, 4))
				unsaved := a.trackUnsaved(a.subtitleTab)
				label := locales.ListSave()
				if unsaved {
					label = locales.ListUnsaved()
				}
				// Reserve the longer state label plus MyGo's native icon, gap,
				// padding and border. Editing must not resize the same save face.
				savedWidth, _ := c.MeasureText(0, ui.Span{Text: locales.ListSave()})
				dirtyWidth, _ := c.MeasureText(0, ui.Span{Text: locales.ListUnsaved()})
				saveWidth := max(savedWidth, dirtyWidth) + em(c, iconSize) + c.Theme().Space(8.5) + 2
				// The file status is the save action itself; keep its accessible
				// action name and tooltip so "Unsaved" still clearly means Save.
				save := iconButton(c, label, "save").Width(saveWidth).Shrink(0).Label(locales.ListSave()).Description(locales.ListSave()).Tooltip(locales.ListSave()).Disabled(a.busy || a.showsTranslationProgress() || a.snapshot == nil || !unsaved).OnClick(func() { a.saveTrack(a.subtitleTab, false) })
				if unsaved {
					save.TextColor(c.Theme().Danger)
				}
				iconButton(c, locales.ListSaveAs(), "file-down").Disabled(a.busy || a.showsTranslationProgress() || !hasCues || !a.trackHasContent(a.subtitleTab)).OnClick(func() { a.saveTrack(a.subtitleTab, true) })
			})
		})
		if a.showsTranslationProgress() {
			a.translationProgressView(c)
			return
		}
		if !hasCues {
			label := locales.ListEmpty()
			if a.subtitleTab == 1 {
				label = locales.ListRebuiltEmpty()
			}
			ui.Column(c).Grow(1).Center().Children(func() { muted(c, label) }).ContextMenu(func(m *ui.Menu) {
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
		a.list.KeepClickPosition = true
		a.list.OnRowClick = func(row int, double bool) {
			a.rowClick = &subtitleRowClick{index: rows[row], double: double}
		}
		a.list.CellMenu = func(m *ui.Menu, row, col int) {
			a.pendingClick = nil
			a.noteBrowse(c.Now())
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
		targets := a.retranslationTargets()
		showChecks := len(a.checkedRetranslationIDs()) > 0
		numbers := numberColumn(c, maximum)
		if showChecks {
			numbers.Width += em(c, 28)
			numbers.MinWidth = numbers.Width
		}
		cols := []ui.TableColumn{numbers, timeColumn(c, locales.ListColumnStart()), timeColumn(c, locales.ListColumnEnd()), {Title: locales.ListColumnText(), Fixed: true}}
		loads := a.readingLoads()
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
			cell := func(value string) ui.Element {
				t := ui.Text(c, value).FontSize(em(c, captionSize)).FontFeatures("tnum").SingleLine()
				if index != a.selected {
					t.TextColor(c.Theme().TextMuted)
				}
				return t
			}
			switch col {
			case 0:
				ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Children(func() {
					if showChecks {
						checked := a.subtitleChecked[a.subtitleTab][cue.ID]
						_, eligible := targets[cue.ID]
						hint := locales.RetranslateCheckCue(cue.Position + 1)
						if !eligible {
							hint = locales.RetranslateUseOptimized()
						}
						ui.Checkbox(c, &checked, "").Label(locales.RetranslateCheckCue(cue.Position + 1)).Tooltip(hint).Disabled(a.busy || a.translationLive != nil || !eligible).OnChange(func() { a.setSubtitleChecked(cue.ID, checked) })
					}
					number := cell(fmt.Sprint(cue.Position + 1))
					// Selected rows retain MyGo's uniform accent and readable text.
					if index != a.selected && a.draftReading(cue, index, loads[index]).Problem() {
						number.TextColor(c.Theme().Danger)
					}
				})
			case 1:
				cell(stamp(start))
			case 2:
				cell(stamp(end))
			default:
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
			a.pendingClick = nil
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
			if ev.Kind == ui.InputScroll {
				a.pendingClick = nil
				a.noteBrowse(ev.Time)
			} else if ev.Kind == ui.InputPointerDown {
				// Stop following as soon as the first press begins, not just
				// after release: the target row must stay under the pointer.
				a.noteBrowse(ev.Time)
			}
			return false
		})
		if click := a.rowClick; click != nil {
			a.rowClick = nil
			a.clickSubtitle(click.index, click.double, c.Now())
		} else if (table.Changed() || table.Submitted()) && selected >= 0 && selected < len(rows) {
			// Keyboard navigation selects without a mouse delay; Enter uses
			// the explicit double-click action, as native lists activate rows.
			a.pendingClick = nil
			action := "none"
			if table.Submitted() {
				action = a.settings.DoubleClickAction
			}
			a.selectSubtitle(rows[selected], action)
		}
		a.finishSubtitleClick(c.Now())
		if a.pendingClick != nil {
			c.After(a.pendingClick.due.Sub(c.Now()))
		}
		locatedThisFrame := a.locateCue >= 0
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
		if !a.paused && a.followRow >= 0 && !a.followHeld && a.pendingClick == nil && !locatedThisFrame {
			if first, last := a.list.Visible(); last >= first && (a.followRow < first || a.followRow > last) {
				a.noteBrowse(c.Now())
				a.followPending = true
			}
		}
		// While the video plays the cue at the playhead is chosen; the list
		// keeps it in the middle until the user scrolls away. Typing keeps
		// the cue being edited.
		if a.settings.FollowPlayback && !a.paused && !a.savePromptOpen && !a.followHeld && a.pendingClick == nil {
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
	a.pendingClick = nil
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
	cues := a.activeDocument().Cues
	valid := !a.showsTranslationProgress() && a.snapshot != nil && a.selected >= 0 && a.selected < len(cues)
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
		cue := cues[a.selected]
		readonly := cue.ReadOnly || a.busy
		fixedTiming := readonly || a.snapshot.Document.Format == "ass" || a.snapshot.Document.Format == "ssa"
		// Allocate the remaining space in this frame, as the recognition
		// queue does. TextArea owns scrolling; no Bounds feedback resizes it.
		ui.Column(c).Grow(1).MinHeight(0).Padding(em(c, 12), em(c, panelPad)).Gap(em(c, 8)).Children(func() {
			// Cue number and duration are peer metadata at opposite edges.
			ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Justify(ui.SpaceBetween).Shrink(0).Children(func() {
				muted(c, locales.EditorCueNumber(cue.Position+1)).FontFeatures("tnum").SingleLine().Shrink(0)
				muted(c, locales.EditorSeconds(math.Max(0, a.end-a.start))).Label(locales.EditorDuration()).FontFeatures("tnum").SingleLine().Shrink(0)
			})
			timeFocused, timeComposing := false, false
			ui.Row(c).Gap(em(c, 8)).Wrap().AlignItems(ui.Center).Justify(ui.Center).Shrink(0).Label(locales.EditorTiming()).Children(func() {
				for i, field := range []struct {
					label string
					value *float64
				}{{locales.EditorStart(), &a.start}, {locales.EditorEnd(), &a.end}} {
					if i == 1 {
						ui.Divider(c).Width(em(c, 16)).Height(1).AlignSelf(ui.Center)
					}
					ui.Row(c).Gap(em(c, 4)).AlignItems(ui.Center).Shrink(0).Children(func() {
						input := timecodeField(c.Key(cue.ID), field.label, field.value, .001, fixedTiming, func() { a.changed(c.Now()) })
						timeFocused = timeFocused || input.Focused()
						timeComposing = timeComposing || input.Composing()
						// Compare the live draft with real document neighbors, never
						// the search-filtered rows. Touching boundaries are not overlap.
						millis := int64(math.Round(*field.value * 1000))
						overlap := ""
						if i == 0 && a.selected > 0 && millis < cues[a.selected-1].EndMs {
							overlap = locales.EditorStartOverlap(cues[a.selected-1].Position + 1)
						} else if i == 1 && a.selected+1 < len(cues) && millis > cues[a.selected+1].StartMs {
							overlap = locales.EditorEndOverlap(cues[a.selected+1].Position + 1)
						}
						if overlap != "" {
							input.TextColor(c.Theme().Danger).Tooltip(stamp(*field.value) + "\n" + overlap)
							icon(c, "triangle-alert").TextColor(c.Theme().Danger).Shrink(0).Label(locales.A11yWarningIcon()).Tooltip(overlap)
						}
					})
				}
			})
			// The text speaks for itself; the name is for assistive technology.
			if a.subtitleTab == 1 {
				textTrack := a.editorTextTrack()
				if ui.Tabs(c, &textTrack, locales.TabSource(), locales.TabTranslation()).AlignSelf(ui.Start).Shrink(0).Changed() {
					a.commitEdit(true)
					a.editRebuiltSource = textTrack == 0
					a.loadDraft()
				}
			}
			area := ui.TextArea(c.Key(cue.ID), &a.draft).Grow(1).MinHeight(0).Padding(em(c, 12)).FontSize(em(c, 17)).ReadOnly(readonly).Label(locales.A11yCueText())
			a.composing = area.Composing() || timeComposing
			a.editing = area.Focused() || timeFocused
			if area.Changed() {
				a.changed(c.Now())
			}
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
	a.pendingClick = nil
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
	if !a.followHeld {
		a.resumeFollow()
	}
	a.loadDraft()
}
func (a *NativeApp) selectSubtitle(index int, action string) {
	if !a.commitEdit(true) {
		return
	}
	a.selected = index
	a.loadDraft()
	if action == "none" || action == "" || !a.playbackAvailable() {
		a.followHeld = true
		a.list.StopScroll()
		return
	}
	a.resumeFollow()
	a.seekSubtitle(action)
}
