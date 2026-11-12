package main

import (
	"errors"
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

// browseGrace protects transient clicks and search input. Manual scrolling
// has a separate persistent hold and never resumes on this timeout.
const browseGrace = 4 * time.Second

// noteBrowse protects transient pointer/search interaction and live translation
// progress. Playback-list scrolling instead uses the persistent browseHeld flag.
func (a *NativeApp) noteBrowse(now time.Time) {
	a.browseUntil = now.Add(browseGrace)
}

// browsing says the user's own browsing still holds the viewport.
func (a *NativeApp) browsing(now time.Time) bool {
	return a.browseHeld || (!a.browseUntil.IsZero() && now.Before(a.browseUntil))
}

// resumeFollow releases selection and transient input holds. It deliberately
// preserves manual scrolling across playback, seeks and track changes.
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
	width, _ := c.Size()
	a.subtitlesViewAtWidth(c, width)
}

func (a *NativeApp) subtitlesViewAtWidth(c *ui.Context, width float32) {
	compact := width < em(c, 680)
	ui.Column(c).Fill().Background(studioColors(c).panel).Children(func() {
		hasCues := len(a.activeDocument().Cues) > 0
		// Keep the track switch centered; all list commands live in the
		// wrapping toolbar below, so narrow headers never crowd the tabs.
		panelHeader(c, nil, func() {
			if a.proofreading {
				tab := 0
				lobeTabs(c, &tab, locales.TabProofreading()).Label(locales.A11ySubtitleTabs())
				return
			}
			tab := a.subtitleTab
			if lobeTabs(c, &tab, locales.TabOriginal(), locales.TabRebuilt()).Label(locales.A11ySubtitleTabs()).Changed() {
				a.changeSubtitleTab(tab)
			}
		}, nil)
		separator(c)
		// An empty track has one useful next action. Avoid presenting a wall
		// of disabled editing commands before there is anything to edit.
		if !hasCues && !a.showsTranslationProgress() && !a.proofreading {
			label := locales.ListEmpty()
			if a.subtitleTab == 1 {
				label = locales.ListRebuiltEmpty()
			}
			empty := ui.Column(c).Grow(1).Center().Gap(em(c, 12)).Children(func() {
				muted(c, label)
				a.proofreadingButton(c, false)
				iconButton(c, locales.ListImport(), "folder-open").Disabled(a.busy).OnClick(a.pickSubtitle)
			})
			a.subtitleContextMenu(c, empty, -1, -1, nil)
			return
		}
		a.subtitleToolbar(c, width, hasCues)
		// The toolbar boundary spans the whole pane, including the gutter.
		separator(c)
		if a.showsTranslationProgress() {
			a.translationProgressView(c)
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
			index := rows[row]
			if a.subtitleSelectionMode() {
				// Selection mode treats the whole row as the hit target. Keep
				// batch checks independent from the editing row and playback.
				a.setSubtitleChecked(cues[index].ID, !a.subtitleChecked[a.subtitleTab][cues[index].ID])
				return
			}
			a.rowClick = &subtitleRowClick{index: index, double: double}
		}
		maximum := 1
		maximumTime := int64(math.Max(a.start, a.end) * 1000)
		for _, cue := range cues {
			maximum = max(maximum, cue.Position+1)
			maximumTime = max(maximumTime, cue.StartMs, cue.EndMs)
		}
		targets := a.retranslationTargets()
		showChecks := len(a.checkedRetranslationIDs()) > 0
		numbers := numberColumn(c, maximum)
		// The left gutter always mirrors the fixed right navigation rail. Only
		// its row controls are conditional, so enabling selection never shifts
		// the subtitle columns under the reader.
		cols := []ui.TableColumn{subtitleSelectionColumn(c), numbers, timeColumn(c, locales.ListColumnStart(), maximumTime), timeColumn(c, locales.ListColumnEnd(), maximumTime), {Title: locales.ListColumnText(), Fixed: true}}
		numberIndex, startIndex, endIndex := 1, 2, 3
		if compact {
			// Start and end remain explicit, but share a column so subtitle
			// text can be read without scrolling horizontally in a small pane.
			timing := timeColumn(c, locales.ListColumnStart()+" / "+locales.ListColumnEnd(), maximumTime)
			timing.ID = "timing-pair"
			cols = []ui.TableColumn{subtitleSelectionColumn(c), numbers, timing, {Title: locales.ListColumnText(), Fixed: true}}
			endIndex = -1
		}
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
				t := ui.Text(c, value).FontSize(em(c, captionSize)).FontFeatures("tnum").NoWrap().Shrink(0)
				if index != a.selected {
					t.TextColor(c.Theme().TextMuted)
				}
				return t
			}
			switch {
			case col == 0:
				if showChecks {
					ui.Row(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Children(func() {
						checked := a.subtitleChecked[a.subtitleTab][cue.ID]
						_, eligible := targets[cue.ID]
						hint := locales.RetranslateCheckCue(cue.Position + 1)
						if !eligible {
							hint = locales.RetranslateUseOptimized()
						}
						lobeCheckbox(c, &checked, "").Label(locales.RetranslateCheckCue(cue.Position + 1)).Tooltip(hint).Disabled(a.busy || a.translationLive != nil || !eligible).OnChange(func() { a.setSubtitleChecked(cue.ID, checked) })
					})
				}
			case col == numberIndex:
				number := cell(fmt.Sprint(cue.Position + 1))
				// Selected rows retain MyGo's uniform accent and readable text.
				if index != a.selected && a.draftReading(cue, index, loads[index]).Problem() {
					number.TextColor(c.Theme().Danger)
				}
			case col == startIndex:
				if compact {
					ui.Column(c).Gap(em(c, 4)).Children(func() {
						cell(stamp(start))
						cell(stamp(end))
					})
				} else {
					cell(stamp(start))
				}
			case col == endIndex:
				cell(stamp(end))
			default:
				ui.Column(c).Gap(em(c, 8)).Padding(em(c, 6), em(c, 2)).Children(func() {
					if a.subtitleTab == 1 {
						source := cueDisplayText(cue, 0)
						if index == a.selected && a.editPending && a.editRebuiltSource {
							source = a.draft
						}
						label := muted(c, source).FontSize(em(c, captionSize)).LineHeight(1.5)
						if index == a.selected {
							label.TextColor(c.Theme().AccentText)
						}
					}
					ui.Text(c, text).FontSize(em(c, subtitleSize)).LineHeight(1.5)
				})
			}
		}, func(bool) {
			// Navigation changes only the viewport. The playback still
			// chooses the cue at the playhead; the list stays where the
			// reader put it until the locate button is pressed.
			a.pendingClick = nil
			a.browseHeld = true
			a.locateCue = -1
		}, func(row int, target ui.Element) {
			a.subtitleContextMenu(c, target, rows[row], row, &selected)
		}).Label(locales.A11ySubtitleList())
		// Blank space keeps the current selection; a cell prepares its own cue.
		a.subtitleContextMenu(c, table, a.selected, rowOf(a.selected), &selected)
		// The wheel and the touchpad scroll the list by the user's own hand.
		// Only the locate button may return the viewport to playback.
		table.HandleInput(func(ev ui.InputEvent) bool {
			if ev.Kind == ui.InputScroll {
				a.pendingClick = nil
				a.browseHeld = true
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
		} else if !showChecks && (table.Changed() || table.Submitted()) && selected >= 0 && selected < len(rows) {
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
		if a.followRow >= 0 && !a.followHeld && a.pendingClick == nil && !locatedThisFrame {
			if first, last := a.list.Visible(); last >= first && (a.followRow < first || a.followRow > last) {
				a.browseHeld = true
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
					if !a.editing && !a.composing && a.commitEdit(true) {
						a.selected = active
						a.loadDraft()
					}
				}
				if row := rowOf(active); row >= 0 {
					// Keep choosing the cue at the playhead without moving
					// the reader's viewport until they explicitly locate it.
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
		// Only transient input holds need a timer. A manual scroll has no
		// scheduled return and must not queue a past-due frame repeatedly.
		if !a.browseHeld && a.browsing(c.Now()) {
			c.After(a.browseUntil.Sub(c.Now()))
		}
	})
}

// locateCurrent chooses the cue at the playhead, shows it in the middle of
// the list and follows playback again. A search hiding it is cleared.
func (a *NativeApp) locateCurrent() {
	if a.subtitleSelectionMode() {
		return
	}
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
	a.browseHeld = false
	a.resumeFollow()
	a.activeCue = index
	if !a.editing && !a.composing && index != a.selected {
		if !a.commitEdit(true) {
			return
		}
		a.selected = index
		a.loadDraft()
	}
	a.locateCue = index
}

// timeColumn fits a timestamp in the active font, digits of one width.
func timeColumn(c *ui.Context, title string, maximum ...int64) ui.TableColumn {
	var longest int64
	for _, value := range maximum {
		longest = max(longest, value)
	}
	width, _ := c.MeasureText(0, ui.Span{Text: stamp(float64(longest) / 1000), Features: "tnum"})
	width += 2*c.Theme().Space(2.5) + 4
	return ui.TableColumn{Title: title, Width: width, MinWidth: width}
}

func (a *NativeApp) editorView(c *ui.Context) {
	_, height := c.Size()
	a.editorViewAtHeight(c, height)
}

func (a *NativeApp) editorViewAtHeight(c *ui.Context, height float32) {
	folded := height <= em(c, headerSize)+1.01
	cues := a.activeDocument().Cues
	valid := !a.showsTranslationProgress() && a.snapshot != nil && a.selected >= 0 && a.selected < len(cues)
	ui.Column(c).Fill().Label(locales.A11yEditorPanel()).Children(func() {
		panelHeader(c, nil, func() { panelTitle(c, locales.EditorTitle()) }, func() {
			if folded {
				return
			}
			ui.Toolbar(c, func() {
				toolIcon(c, locales.EditorUndo(), "undo-2").Disabled(a.busy || a.snapshot == nil || !(a.snapshot.CanUndo || a.editPending)).OnClick(func() { a.undo(false) })
				toolIcon(c, locales.EditorRedo(), "redo-2").Disabled(a.busy || a.snapshot == nil || !a.snapshot.CanRedo).OnClick(func() { a.undo(true) })
			}).Padding(0).Gap(em(c, 2))
		})
		separator(c)
		if folded {
			a.editing, a.composing = false, false
			return
		}
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
				muted(c, locales.EditorSeconds(math.Max(0, a.end-a.start))).Label(locales.EditorDuration()).FontFeatures("tnum").NoWrap().Shrink(0)
			})
			timeFocused, timeComposing := false, false
			// Start and end are peer fields. No connector sits between them: it
			// pushed the pair past the pane width and stayed behind after a wrap.
			ui.Row(c).GapX(em(c, 16)).GapY(em(c, 8)).Wrap().AlignItems(ui.Center).Justify(ui.Center).Shrink(0).Label(locales.EditorTiming()).Children(func() {
				for i, field := range []struct {
					label string
					value *float64
				}{{locales.EditorStart(), &a.start}, {locales.EditorEnd(), &a.end}} {
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
				if segmentedFill(c, &textTrack, locales.TabSource(), locales.TabTranslation()).Shrink(0).Changed() && a.commitEdit(true) {
					a.editRebuiltSource = textTrack == 0
					a.loadDraft()
				}
			}
			// One full line of reading text always shows: line box plus the
			// field's own padding. Extra lines scroll inside the native field.
			area := lobeTextArea(c.Key(cue.ID), &a.draft).Grow(1).MinHeight(em(c, editSize*1.5+24)).Padding(em(c, 12), em(c, 16)).FontSize(em(c, editSize)).LineHeight(1.5).Background(studioColors(c).panel).ReadOnly(readonly).Label(locales.A11yCueText())
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
// the cue restores invalid input and says why. Storage and revision failures
// retain the draft and block actions that would replace it.
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
		a.editDue = time.Time{}
		if !errors.Is(err, project.ErrInvalidEdit) {
			a.showError("edit", err.Error())
			return false
		}
		if !leaving {
			return false
		}
		a.showError("edit", locales.ErrEditRestored(err.Error()))
		a.loadDraft()
		return true
	}
	state, err := a.store.Snapshot()
	if err != nil {
		a.editDue = time.Time{}
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
	if a.subtitleTab == 1 && !a.proofreading {
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
	if a.proofreading {
		tab = 1
	}
	a.pendingClick = nil
	if !a.commitEdit(true) {
		return
	}
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
