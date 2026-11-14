package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/security"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/translator"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

type retranslationResult struct {
	text, problems string
	valid          bool
}
type retranslationRow struct {
	cue     subtitle.Cue
	result  *retranslationResult
	applied bool
	ignored bool
}
type retranslationState struct {
	open, running       bool
	projectID           string
	revision            int64
	source              subtitle.Document
	rows                []retranslationRow
	selected            int
	query               string
	list                ui.ListState
	config              translator.Config
	provider, scope     string
	errorText, progress string
}

// Original and optimized tracks can diverge after a split, merge or source
// edit. Only an exact one-to-one identity may be offered on the original tab.
func (a *NativeApp) retranslationTargets() map[string]subtitle.Cue {
	out := map[string]subtitle.Cue{}
	if a.snapshot == nil {
		return out
	}
	working := make(map[string]subtitle.Cue, len(a.snapshot.Document.Cues))
	for _, cue := range a.snapshot.Document.Cues {
		working[cue.ID] = cue
	}
	for _, cue := range a.activeDocument().Cues {
		target, ok := working[cue.ID]
		if !ok || cue.ReadOnly || target.ReadOnly {
			continue
		}
		if a.subtitleTab == 0 && (cue.StartMs != target.StartMs || cue.EndMs != target.EndMs || subtitle.SourceText(cue) != subtitle.SourceText(target)) {
			continue
		}
		out[cue.ID] = target
	}
	return out
}

func (a *NativeApp) checkedRetranslationIDs() []string {
	ids := []string{}
	targets := a.retranslationTargets()
	for _, cue := range a.activeDocument().Cues {
		if _, ok := targets[cue.ID]; ok && a.subtitleChecked[a.subtitleTab][cue.ID] {
			ids = append(ids, cue.ID)
		}
	}
	return ids
}

func (a *NativeApp) subtitleSelectionMode() bool {
	return len(a.checkedRetranslationIDs()) > 0
}

func (a *NativeApp) setSubtitleChecked(id string, checked bool) {
	if a.subtitleChecked[a.subtitleTab] == nil {
		a.subtitleChecked[a.subtitleTab] = map[string]bool{}
	}
	if checked {
		a.subtitleChecked[a.subtitleTab][id] = true
	} else {
		delete(a.subtitleChecked[a.subtitleTab], id)
	}
	// Checking does not select, seek or start the single-click playback timer.
	a.pendingClick, a.rowClick = nil, nil
}

func (a *NativeApp) checkAllSubtitles() {
	// All means the entire active track, including rows hidden by search.
	for id := range a.retranslationTargets() {
		a.setSubtitleChecked(id, true)
	}
}

func (a *NativeApp) subtitleSelectionTools(c *ui.Context) {
	width, _ := c.Size()
	a.subtitleSelectionActions(c, subtitleToolbarLayout{compact: width < em(c, 1100)})
}

func (a *NativeApp) subtitleSelectionActions(c *ui.Context, layout subtitleToolbarLayout) {
	count := len(a.checkedRetranslationIDs())
	disabled := a.busy || a.translationLive != nil
	if count > 0 {
		// Keep every selection action together before Locate. Wide and narrow
		// toolbars use the same actions; only their presentation changes.
		subtitleTool(c, locales.RetranslateSelected(count), "rotate-cw", layout).Disabled(disabled).OnClick(a.openRetranslation)
		subtitleTool(c, locales.RetranslateCheckAll(), "copy-check", layout).Tooltip(locales.RetranslateAllHint()).Disabled(disabled).OnClick(a.checkAllSubtitles)
		subtitleTool(c, locales.RetranslateClear(), "list-x", layout).Disabled(disabled).OnClick(func() { a.subtitleChecked[a.subtitleTab] = nil })
		return
	}
	cues := a.activeDocument().Cues
	id := ""
	if a.selected >= 0 && a.selected < len(cues) {
		id = cues[a.selected].ID
	}
	_, eligible := a.retranslationTargets()[id]
	// Start by checking the current cue, so there is never an empty checkbox
	// mode. These actions sit before Locate, beside the editing tools.
	subtitleTool(c, locales.RetranslateSelect(), "list-checks", layout).Disabled(disabled || !eligible).OnClick(func() { a.setSubtitleChecked(id, true) })
	subtitleTool(c, locales.RetranslateCheckAll(), "copy-check", layout).Tooltip(locales.RetranslateAllHint()).Disabled(disabled || len(a.retranslationTargets()) == 0).OnClick(a.checkAllSubtitles)
}

func (a *NativeApp) retranslationTools(c *ui.Context) {
	width, _ := c.Size()
	a.retranslationActions(c, subtitleToolbarLayout{compact: width < em(c, 1100)})
}

func (a *NativeApp) retranslationActions(c *ui.Context, layout subtitleToolbarLayout) {
	if a.retranslation != nil {
		if layout.compact {
			toolIcon(c, locales.RetranslateReopen(), "eye").OnClick(a.reopenRetranslation)
		} else {
			subtitleTool(c, locales.RetranslateReopen(), "eye", layout).OnClick(a.reopenRetranslation)
		}
	}
}

func (a *NativeApp) reopenRetranslation() {
	// Viewing is safe during a task and after edits; applying remains guarded
	// by the frozen revision. Closing the modal never discards its results.
	if a.retranslation != nil {
		a.retranslation.open = true
		a.pendingClick, a.rowClick = nil, nil
	}
}

func (a *NativeApp) openRetranslation() {
	if a.busy || a.snapshot == nil || a.translationLive != nil || !a.commitEdit(true) {
		return
	}
	ids := a.checkedRetranslationIDs()
	if len(ids) == 0 {
		return
	}
	if r := a.retranslation; r != nil && !a.retranslationStale(r) && slices.Equal(ids, r.ids(false)) {
		a.reopenRetranslation()
		return
	}
	r := &retranslationState{open: true, projectID: a.snapshot.Manifest.ID, revision: a.snapshot.Revision, source: a.snapshot.Document, config: a.translationConfig(), scope: a.settings.CredentialScope()}
	if provider := a.settings.Provider(); provider != nil {
		r.provider = provider.Name
	}
	selected := make(map[string]bool, len(ids))
	for _, id := range ids {
		selected[id] = true
	}
	for _, cue := range r.source.Cues {
		if selected[cue.ID] {
			r.rows = append(r.rows, retranslationRow{cue: cue})
		}
	}
	if a.retranslation != nil && a.retranslation.hasResults() {
		// A different batch must not silently erase the last review.
		a.retranslationNext = r
	} else {
		a.retranslation = r
	}
	a.pendingClick, a.rowClick = nil, nil
}

func (r *retranslationState) ids(current bool) []string {
	ids := []string{}
	for i, row := range r.rows {
		if !current || i == r.selected {
			ids = append(ids, row.cue.ID)
		}
	}
	return ids
}

func (r *retranslationState) hasResults() bool {
	for _, row := range r.rows {
		if row.result != nil {
			return true
		}
	}
	return false
}

func (a *NativeApp) retranslationStale(r *retranslationState) bool {
	return a.editPending || a.snapshot == nil || a.snapshot.Manifest.ID != r.projectID || a.snapshot.Revision != r.revision
}

// Work returns a UI callback only after the full response and scoped checker
// finish. A failed/cancelled attempt cannot erase the last complete result.
func (a *NativeApp) retranslationWork(r *retranslationState, ids []string, config translator.Config) func(context.Context) (func(), error) {
	selected := make(map[string]bool, len(ids))
	for _, id := range ids {
		selected[id] = true
	}
	source, worker := r.source, a.worker
	root, executable := a.skillTools()
	checker := translator.Checker{Executable: executable, SkillRoot: root, Scratch: worker.Scratch, Subtitle: worker}
	return func(ctx context.Context) (func(), error) {
		result, err := translator.TranslateSelected(ctx, config, source, ids)
		if err != nil {
			return nil, err
		}
		checks, err := checker.CheckSelected(ctx, source, result.Document, ids, config.TargetLanguage)
		if err != nil {
			return nil, err
		}
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		problems := append(append(append([]string{}, checks.Errors...), checks.Warnings...), result.Report.Warnings...)
		details := strings.Join(problems, "\n")
		answers := make(map[string]string, len(ids))
		baselines := make(map[string]subtitle.Cue, len(source.Cues))
		for _, cue := range source.Cues {
			baselines[cue.ID] = cue
		}
		for _, cue := range result.Document.Cues {
			if selected[cue.ID] {
				answers[cue.ID] = cue.Text
			}
		}
		return func() {
			r.running = false
			r.progress = locales.RetranslateReady(len(ids))
			for i := range r.rows {
				row := &r.rows[i]
				if text, ok := answers[row.cue.ID]; ok {
					row.cue = baselines[row.cue.ID]
					row.result = &retranslationResult{text, details, len(checks.Errors) == 0}
					row.applied = false
					row.ignored = false
				}
			}
		}, nil
	}
}

func (a *NativeApp) startRetranslation(current bool) {
	r := a.retranslation
	if r == nil || a.busy || len(r.ids(current)) == 0 {
		return
	}
	if a.retranslationStale(r) {
		r.errorText = locales.RetranslateStale()
		return
	}
	key, err := security.ReadKey(r.scope)
	if err != nil {
		r.errorText = err.Error()
		return
	}
	config := r.config
	config.Key = key
	config.Observe = func(ev translator.Event) {
		a.win.Update(func() {
			r.progress = locales.RetranslateProgress(ev.Done, ev.Total)
		})
	}
	r.running, r.errorText = true, ""
	r.progress = locales.RetranslateProgress(0, len(r.ids(current)))
	a.translationRunning = true
	work := a.retranslationWork(r, r.ids(current), config)
	a.runTimed(timingTranslation, false, locales.RetranslateTitle(), func(ctx context.Context) (func(), error) {
		apply, err := work(ctx)
		if err != nil {
			a.win.Update(func() {
				r.running = false
				r.errorText = err.Error()
				if errors.Is(err, context.Canceled) {
					r.errorText = locales.StatusCancelled()
				}
				r.progress = ""
			})
		}
		return apply, err
	})
}

func (r *retranslationState) replacementEdits(current bool) []project.Edit {
	if current {
		return r.replacementEditsFor(r.selected)
	}
	edits := []project.Edit{}
	for i := range r.rows {
		edits = append(edits, r.replacementEditsFor(i)...)
	}
	return edits
}

func (r *retranslationState) replacementEditsFor(index int) []project.Edit {
	if index < 0 || index >= len(r.rows) {
		return nil
	}
	row := r.rows[index]
	if row.applied || row.ignored || row.result == nil {
		return nil
	}
	result := row.result
	if !result.valid || row.cue.Translated && result.text == row.cue.Text {
		return nil
	}
	return []project.Edit{{ID: row.cue.ID, Text: result.text, StartMs: row.cue.StartMs, EndMs: row.cue.EndMs, Track: "translation"}}
}

func (a *NativeApp) toggleRetranslationIgnore(index int) {
	r := a.retranslation
	if r == nil || index < 0 || index >= len(r.rows) || r.rows[index].result == nil || r.rows[index].applied || a.busy || a.retranslationStale(r) {
		return
	}
	r.rows[index].ignored = !r.rows[index].ignored
	r.errorText = ""
}

func (a *NativeApp) acceptRetranslation(current bool) {
	r := a.retranslation
	if r == nil || a.busy {
		return
	}
	if a.retranslationStale(r) {
		r.errorText = locales.RetranslateStale()
		return
	}
	edits := r.replacementEdits(current)
	if len(edits) == 0 {
		return
	}
	work := a.retranslationAcceptWork(r, edits)
	a.runTimed(timingSubtitle, false, locales.RetranslateApply(), func(ctx context.Context) (func(), error) {
		apply, err := work(ctx)
		if err != nil {
			a.win.Update(func() { r.errorText = err.Error() })
		}
		return apply, err
	})
}

func (a *NativeApp) retranslationAcceptWork(r *retranslationState, edits []project.Edit) func(context.Context) (func(), error) {
	// Reuse strict preflight and the manual translation transaction. Acceptance
	// adds a human lock; one undo restores the entire batch and previous locks.
	work := a.listTextEditWorkFor(r.source, r.revision, edits)
	return func(ctx context.Context) (func(), error) {
		apply, err := work(ctx)
		if err != nil {
			return nil, err
		}
		return func() {
			apply()
			r.errorText = ""
			r.source, r.revision = a.snapshot.Document, a.snapshot.Revision
			applied := make(map[string]bool, len(edits))
			for _, edit := range edits {
				applied[edit.ID] = true
			}
			for i := range r.rows {
				if applied[r.rows[i].cue.ID] {
					// Keep the before/after text visible after acceptance.
					r.rows[i].applied = true
				}
			}
			a.changeSubtitleTab(1)
			a.status = locales.TextProcessResult(len(edits))
			r.progress = a.status
		}, nil
	}
}

func (row *retranslationRow) status() string {
	if row.applied {
		return locales.RetranslateApplied()
	}
	if row.ignored {
		return locales.RetranslateIgnored()
	}
	if row.result == nil {
		return ""
	}
	if !row.result.valid {
		return locales.RetranslateInvalid()
	}
	if row.cue.Translated && row.result.text == row.cue.Text {
		return locales.RetranslateUnchanged()
	}
	return locales.RetranslateAvailable()
}

// Filtering changes only what is visible, never the frozen batch sent by
// Translate all / Replace all. Selection remains a canonical row index.
func (r *retranslationState) filteredRows() []int {
	query := strings.ToLower(strings.TrimSpace(r.query))
	rows := make([]int, 0, len(r.rows))
	for i, row := range r.rows {
		text := fmt.Sprint(row.cue.Position+1) + "\n" + subtitle.SourceText(row.cue) + "\n" + cueDisplayText(row.cue, 1)
		if row.result != nil {
			text += "\n" + row.result.text
		}
		if strings.Contains(strings.ToLower(text), query) {
			rows = append(rows, i)
		}
	}
	return rows
}

func (a *NativeApp) retranslationView(c *ui.Context) {
	if a.retranslationNext != nil {
		open := true
		switch confirmationDialog(c, &open, locales.RetranslateNewTitle(), locales.RetranslateNewBody(), locales.ActionCancel(), locales.RetranslateNewConfirm()) {
		case 0:
			a.retranslationNext = nil
		case 1:
			a.retranslation, a.retranslationNext = a.retranslationNext, nil
		default:
			if !open {
				a.retranslationNext = nil
			}
		}
		return
	}
	r := a.retranslation
	if r == nil {
		return
	}
	stale := a.retranslationStale(r)
	// The frozen batch remains one internally scrolling list. Each row keeps
	// the source, candidate and old translation together beside its actions.
	fixedDialog(c, &r.open, locales.RetranslateTitle(), 1600, 1000, func() {
		ui.Column(c).Fill().MinHeight(0).Gap(em(c, 16)).Children(func() {
			service := r.provider + " · " + r.config.Model + " · " + r.config.TargetLanguage
			if r.config.Model == "" || r.config.Endpoint == "" {
				service = locales.WorkflowNoService()
			}
			ui.Row(c).Gap(em(c, 12)).AlignItems(ui.Center).Shrink(0).Children(func() {
				muted(c, service).SingleLine().Grow(1).MinWidth(0).Tooltip(service + "\n" + r.config.Endpoint + "\n" + locales.RetranslateScope())
				progress := r.progress
				muted(c, progress).FontSize(em(c, captionSize)).Shrink(0)
			})
			lobeSearchField(c, &r.query, func() { r.list.ScrollTo(0, ui.Start) }).Label(locales.RetranslateSearch()).Placeholder(locales.RetranslateSearch())
			rows := r.filteredRows()
			selected := slices.Index(rows, r.selected)
			r.list.Selected = &selected
			r.list.Key = func(i int) any { return r.rows[rows[i]].cue.ID }
			r.list.OnRowClick = func(i int, _ bool) {
				if i >= 0 && i < len(rows) {
					r.selected = rows[i]
					selected = i
				}
			}
			ui.Row(c).Gap(em(c, 4)).AlignItems(ui.Center).Children(func() {
				muted(c, locales.RetranslateVisible(len(rows), len(r.rows))).FontSize(em(c, captionSize)).Grow(1).Tooltip(locales.RetranslateFilterScope())
				ui.Toolbar(c, func() {
					toolIcon(c, locales.ListTop(), "arrow-up-to-line").Disabled(len(rows) == 0).OnClick(func() { r.list.ScrollTo(0, ui.Start) })
					toolIcon(c, locales.ListBottom(), "arrow-down-to-line").Disabled(len(rows) == 0).OnClick(r.list.ScrollToEnd)
				}).Padding(0)
			})
			ui.Column(c).Grow(1).Basis(0).MinHeight(0).Gap(em(c, 8)).Label(locales.RetranslateCompare()).Children(func() {
				list := ui.List(c, &r.list, len(rows), func(i int) {
					chosen := i == selected
					index := rows[i]
					row := &r.rows[index]
					ui.Column(c).Padding(em(c, 12), em(c, 16)).Gap(em(c, 10)).Children(func() {
						ui.Row(c).Gap(em(c, 16)).AlignItems(ui.Start).Children(func() {
							ui.Column(c).Grow(1).MinWidth(0).Gap(em(c, 8)).Children(func() {
								ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Shrink(0).Children(func() {
									ui.Text(c, fmt.Sprintf("# %d", row.cue.Position+1)).FontSize(em(c, captionSize)).FontFeatures("tnum").Grow(1)
									rowMuted(c, stamp(float64(row.cue.StartMs)/1000)+" – "+stamp(float64(row.cue.EndMs)/1000), chosen).FontFeatures("tnum").FontSize(em(c, captionSize)).NoWrap().Shrink(0)
									rowMuted(c, row.status(), chosen).FontSize(em(c, captionSize)).SingleLine().Shrink(0)
								})
								rowMuted(c, locales.TabSource(), chosen).FontSize(em(c, captionSize)).Shrink(0)
								ui.Text(c, subtitle.SourceText(row.cue)).FontSize(em(c, captionSize)).LineHeight(1.45)
								rowMuted(c, locales.RetranslateCandidate(), chosen).FontSize(em(c, captionSize)).Shrink(0)
								if row.result != nil && row.result.text != "" {
									ui.Text(c, row.result.text).Selectable().FontSize(em(c, subtitleSize)).LineHeight(1.5)
								} else {
									muted(c, "")
								}
								rowMuted(c, locales.RetranslateExisting(), chosen).FontSize(em(c, captionSize)).Shrink(0)
								existing := cueDisplayText(row.cue, 1)
								if existing != "" {
									rowMuted(c, existing, chosen).Selectable().FontSize(em(c, bodySize)).LineHeight(1.45)
								} else {
									rowMuted(c, locales.RetranslateNoExisting(), chosen)
								}
							})
							ui.Column(c).Shrink(0).Gap(em(c, 8)).Children(func() {
								lobeButton(c, locales.RetranslateOne()).Disabled(a.busy || stale).OnClick(func() {
									r.selected = index
									a.startRetranslation(true)
								})
								lobeButton(c, locales.RetranslateApplyOne()).Disabled(a.busy || stale || len(r.replacementEditsFor(index)) == 0).OnClick(func() {
									r.selected = index
									a.acceptRetranslation(true)
								})
								ignore := locales.RetranslateIgnore()
								if row.ignored {
									ignore = locales.RetranslateUnignore()
								}
								lobeButton(c, ignore).Disabled(a.busy || stale || row.result == nil || !row.result.valid || row.applied).OnClick(func() { a.toggleRetranslationIgnore(index) })
							})
						})
						ui.Divider(c).Background(studioColors(c).hairline)
					})
				}).Grow(1).MinHeight(0).Gap(0).Label(locales.RetranslateList())
				if list.Changed() && selected >= 0 && selected < len(rows) {
					r.selected = rows[selected]
				}
			})
			note := ""
			if r.selected >= 0 && r.selected < len(r.rows) {
				if result := r.rows[r.selected].result; result != nil && result.problems != "" {
					note = result.problems
				}
			}
			if r.errorText != "" {
				note = r.errorText
			}
			if stale {
				note = locales.RetranslateStale()
			}
			if note != "" {
				// Reuse the diagnostic reader so new messages open at the start,
				// retain padding, and remain selectable without moving the actions.
				ui.Column(c).Height(em(c, 64)).Shrink(0).Padding(em(c, 12)).Background(studioColors(c).panel).Radius(c.Theme().Radius).Children(func() {
					// The outer padding stays clear even while the text scrolls.
					status := reviewReadOnlyText(c, locales.RetranslateChecks(), note).Padding(0).FontSize(em(c, captionSize)).LineHeight(1.45).TextColor(c.Theme().TextMuted)
					if stale || r.errorText != "" {
						status.TextColor(c.Theme().Danger)
					}
				})
			}
		})
	}, func() {
		lobeButton(c, locales.ActionClose()).Tooltip(locales.RetranslateCloseHint()).OnClick(func() { r.open = false })
		ui.Spacer(c)
		if r.running {
			lobeButton(c, locales.ActionCancel()).OnClick(func() {
				if a.cancel != nil {
					a.cancel()
				}
			})
		} else {
			label := locales.RetranslateAgain()
			if r.hasResults() {
				label = locales.RetranslateRetry()
			}
			lobeButton(c, label).Tooltip(locales.RetranslateScope()).Disabled(a.busy || stale).OnClick(func() { a.startRetranslation(false) })
		}
		lobePrimaryButton(c, locales.RetranslateApply()).Disabled(a.busy || len(r.replacementEdits(false)) == 0 || stale).OnClick(func() { a.acceptRetranslation(false) })
	})
}
