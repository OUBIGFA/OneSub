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

type retranslationChoice struct {
	text, problems string
	valid          bool
}
type retranslationRow struct {
	cue     subtitle.Cue
	checked bool
	choices []retranslationChoice
	choice  int
}
type retranslationState struct {
	open, running       bool
	projectID           string
	revision            int64
	source              subtitle.Document
	rows                []retranslationRow
	selected            int
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
	for _, cue := range a.snapshot.Document.Cues {
		if !cue.ReadOnly {
			out[cue.ID] = cue
		}
	}
	if a.subtitleTab == 0 {
		for _, cue := range a.activeDocument().Cues {
			working, ok := out[cue.ID]
			if !ok || cue.ReadOnly || cue.StartMs != working.StartMs || cue.EndMs != working.EndMs || subtitle.SourceText(cue) != subtitle.SourceText(working) {
				delete(out, cue.ID)
			}
		}
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

func (a *NativeApp) retranslationTools(c *ui.Context) {
	count := len(a.checkedRetranslationIDs())
	if count == 0 {
		toolIcon(c, locales.RetranslateSelected(0), "rotate-cw").Disabled(true)
		return
	}
	iconButton(c, locales.RetranslateSelected(count), "rotate-cw").Disabled(a.busy || a.translationLive != nil).OnClick(a.openRetranslation)
}

func (a *NativeApp) openRetranslation() {
	if a.busy || a.snapshot == nil || a.translationLive != nil || !a.commitEdit(true) {
		return
	}
	ids := a.checkedRetranslationIDs()
	if len(ids) == 0 {
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
			r.rows = append(r.rows, retranslationRow{cue: cue, checked: true})
		}
	}
	a.retranslation = r
	a.pendingClick, a.rowClick = nil, nil
}

func (r *retranslationState) checkedIDs() []string {
	ids := []string{}
	for _, row := range r.rows {
		if row.checked {
			ids = append(ids, row.cue.ID)
		}
	}
	return ids
}

func (a *NativeApp) retranslationStale(r *retranslationState) bool {
	return a.snapshot == nil || a.snapshot.Manifest.ID != r.projectID || a.snapshot.Revision != r.revision
}

// Work returns a UI callback only after the full response and scoped checker
// finish. A failed/cancelled attempt cannot erase previously reviewed choices.
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
					row.choices = append(row.choices, retranslationChoice{text, details, len(checks.Errors) == 0})
					row.choice = len(row.choices) - 1
				}
			}
		}, nil
	}
}

func (a *NativeApp) startRetranslation() {
	r := a.retranslation
	if r == nil || a.busy || len(r.checkedIDs()) == 0 {
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
	r.progress = locales.RetranslateProgress(0, len(r.checkedIDs()))
	a.translationRunning = true
	work := a.retranslationWork(r, r.checkedIDs(), config)
	a.run(locales.RetranslateTitle(), func(ctx context.Context) (func(), error) {
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

func (r *retranslationState) replacementEdits() []project.Edit {
	edits := []project.Edit{}
	for _, row := range r.rows {
		if !row.checked || len(row.choices) == 0 {
			continue
		}
		choice := row.choices[row.choice]
		if !choice.valid || row.cue.Translated && choice.text == row.cue.Text {
			continue
		}
		edits = append(edits, project.Edit{ID: row.cue.ID, Text: choice.text, StartMs: row.cue.StartMs, EndMs: row.cue.EndMs, Track: "translation"})
	}
	return edits
}

func (a *NativeApp) acceptRetranslation() {
	r := a.retranslation
	if r == nil || a.busy {
		return
	}
	if a.retranslationStale(r) {
		r.errorText = locales.RetranslateStale()
		return
	}
	edits := r.replacementEdits()
	if len(edits) == 0 {
		return
	}
	// Reuse strict format preflight and the manual translation transaction.
	// Explicit acceptance preserves existing locks and adds a lock to the new
	// human-approved wording; undo restores the complete previous lock state.
	work := a.listTextEditWorkFor(r.source, r.revision, edits)
	a.run(locales.RetranslateApply(), func(ctx context.Context) (func(), error) {
		apply, err := work(ctx)
		if err != nil {
			a.win.Update(func() { r.errorText = err.Error() })
			return nil, err
		}
		return func() {
			apply()
			r.errorText = ""
			r.source, r.revision = a.snapshot.Document, a.snapshot.Revision
			current := make(map[string]subtitle.Cue, len(r.source.Cues))
			for _, cue := range r.source.Cues {
				current[cue.ID] = cue
			}
			for i := range r.rows {
				r.rows[i].cue = current[r.rows[i].cue.ID]
			}
			a.changeSubtitleTab(1)
			a.status = locales.TextProcessResult(len(edits))
			r.progress = a.status
		}, nil
	})
}

func (a *NativeApp) retranslationView(c *ui.Context) {
	r := a.retranslation
	if r == nil {
		return
	}
	_, height := c.Size()
	listHeight, sourceHeight, noteHeight := float32(130), float32(66), float32(48)
	if height < em(c, 760) {
		listHeight, sourceHeight, noteHeight = 80, 44, 36
	}
	fixedDialog(c, &r.open, locales.RetranslateTitle(), 1000, 720, func() {
		ui.Column(c).Fill().Gap(em(c, 10)).Children(func() {
			service := r.provider + " · " + r.config.Model + " · " + r.config.TargetLanguage
			if r.config.Model == "" || r.config.Endpoint == "" {
				service = locales.WorkflowNoService()
			}
			ui.Text(c, service).SingleLine().Tooltip(service + "\n" + r.config.Endpoint)
			muted(c, locales.RetranslateScope()).FontSize(em(c, captionSize))
			ui.Row(c).Gap(em(c, 12)).AlignItems(ui.Center).Children(func() {
				all := len(r.checkedIDs()) == len(r.rows)
				ui.Checkbox(c, &all, locales.RetranslateCheckAll()).Disabled(a.busy).OnChange(func() {
					for i := range r.rows {
						r.rows[i].checked = all
					}
				})
				muted(c, r.progress).Grow(1)
				if r.running {
					ui.Button(c, locales.ActionCancel()).OnClick(func() {
						if a.cancel != nil {
							a.cancel()
						}
					})
				}
			})
			r.list.Selected = &r.selected
			r.list.Key = func(i int) any { return r.rows[i].cue.ID }
			ui.Table(c, &r.list, []ui.TableColumn{
				{Title: locales.RetranslatePick(), Width: em(c, 64), Fixed: true},
				numberColumn(c, len(r.source.Cues)),
				{Title: locales.TabSource()},
				{Title: locales.TranslationModelState(), Width: em(c, 150), Fixed: true},
			}, len(r.rows), func(i, col int) {
				row := &r.rows[i]
				switch col {
				case 0:
					ui.Checkbox(c, &row.checked, "").Label(locales.RetranslateCheckCue(row.cue.Position + 1)).Disabled(a.busy)
				case 1:
					ui.Text(c, fmt.Sprint(row.cue.Position+1))
				case 2:
					text := subtitle.SourceText(row.cue)
					ui.Text(c, text).SingleLine().Tooltip(text)
				case 3:
					label := locales.RetranslatePending()
					if len(row.choices) > 0 {
						label = locales.RetranslateVersion(row.choice + 1)
						if !row.choices[row.choice].valid {
							label = locales.RetranslateInvalid()
						}
					}
					ui.Text(c, label).SingleLine()
				}
			}).Height(em(c, listHeight)).Label(locales.RetranslateList())
			if r.selected >= 0 && r.selected < len(r.rows) {
				row := &r.rows[r.selected]
				ui.Row(c).Gap(em(c, 12)).AlignItems(ui.Center).Children(func() {
					ui.Text(c, locales.TabSource()).Shrink(0)
					ui.Text(c, fmt.Sprintf("# %d · %s → %s", row.cue.Position+1, stamp(float64(row.cue.StartMs)/1000), stamp(float64(row.cue.EndMs)/1000))).Grow(1)
					options := []string{locales.RetranslatePending()}
					if len(row.choices) > 0 {
						options = nil
						for i := range row.choices {
							options = append(options, locales.RetranslateVersion(i+1))
						}
					}
					value := options[row.choice]
					ui.Select(c, &value, options).Label(locales.RetranslateVersionLabel()).Disabled(a.busy || len(row.choices) == 0).OnChange(func() { row.choice = slices.Index(options, value) })
				})
				source := subtitle.SourceText(row.cue)
				ui.TextArea(c, &source).ReadOnly(true).Height(em(c, sourceHeight)).MinHeight(0).Label(locales.TabSource())
				ui.Grid(c).Columns(2).Gap(em(c, 16)).Grow(1).MinHeight(0).Label(locales.RetranslateCompare()).Children(func() {
					text := cueDisplayText(row.cue, 1)
					candidate := ""
					if len(row.choices) > 0 {
						candidate = row.choices[row.choice].text
					}
					for _, field := range []struct{ label, text string }{{locales.RetranslateExisting(), text}, {locales.RetranslateCandidate(), candidate}} {
						ui.Column(c).Fill().MinWidth(0).Gap(em(c, 6)).Children(func() {
							ui.Text(c, field.label)
							ui.TextArea(c, &field.text).ReadOnly(true).Grow(1).MinHeight(0).Label(field.label)
						})
					}
				})
				note := locales.RetranslateNoWrite()
				if len(row.choices) > 0 && row.choices[row.choice].problems != "" {
					note = row.choices[row.choice].problems
				}
				if a.retranslationStale(r) {
					note = locales.RetranslateStale()
				}
				if r.errorText != "" {
					note = r.errorText
				}
				ui.TextArea(c, &note).ReadOnly(true).Height(em(c, noteHeight)).MinHeight(0).Label(locales.RetranslateChecks())
			}
		})
	}, func() {
		ui.Button(c, locales.ActionClose()).OnClick(func() { r.open = false })
		label := locales.RetranslateAgain()
		for _, row := range r.rows {
			if len(row.choices) > 0 {
				label = locales.RetranslateRetry()
				break
			}
		}
		ui.Button(c, label).Disabled(a.busy || len(r.checkedIDs()) == 0 || a.retranslationStale(r)).OnClick(a.startRetranslation)
		ui.PrimaryButton(c, locales.RetranslateApply()).Disabled(a.busy || len(r.replacementEdits()) == 0 || a.retranslationStale(r)).OnClick(a.acceptRetranslation)
	})
}
