package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

const (
	reviewAllFindings = iota
	reviewErrorFindings
	reviewWarningFindings
)

type translationFinding struct {
	index   int // stable position in the frozen report, not a subtitle number
	warning bool
	message string
}

// Findings are batch messages without structured cue IDs. Keep both warning
// sources verbatim and errors first; never infer a subtitle link from text.
func (candidate *translationCandidate) reviewFindings(filter int, query string) []translationFinding {
	query = strings.ToLower(strings.TrimSpace(query))
	var findings []translationFinding
	index := 0
	for group, messages := range [][]string{candidate.Checks.Errors, candidate.Result.Report.Warnings, candidate.Checks.Warnings} {
		for _, message := range messages {
			warning := group != 0
			if (filter == reviewAllFindings || filter == reviewErrorFindings && !warning || filter == reviewWarningFindings && warning) && strings.Contains(strings.ToLower(message), query) {
				findings = append(findings, translationFinding{index: index, warning: warning, message: message})
			}
			index++
		}
	}
	return findings
}

// Subtitle search only changes the optional reference list. It never filters
// the check report or loses the selected source/translation comparison.
func (candidate *translationCandidate) reviewRows(query string) []int {
	query = strings.ToLower(strings.TrimSpace(query))
	rows := make([]int, 0, len(candidate.Result.Document.Cues))
	for i, cue := range candidate.Result.Document.Cues {
		text := fmt.Sprint(i+1) + "\n" + subtitle.SourceText(cue) + "\n" + cue.Text
		if strings.Contains(strings.ToLower(text), query) {
			rows = append(rows, i)
		}
	}
	return rows
}

// The default page is a diagnostic reader, not a second subtitle workbench.
// Only an explicit page switch shows the read-only subtitle reference. Both
// pages keep the same fixed shell and retain their own search/selection.
func (a *NativeApp) translationView(c *ui.Context) {
	fixedDialog(c, &a.translationReview, locales.TranslationResults(), 1600, 1000, func() {
		candidate := a.translationCandidate
		if candidate == nil {
			ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				muted(c, locales.ReviewEmpty())
			})
			return
		}
		ui.Column(c).Fill().MinHeight(0).Gap(em(c, 16)).Children(func() {
			ui.Row(c).Wrap().Gap(em(c, 12)).AlignItems(ui.Center).Shrink(0).Children(func() {
				ui.Tabs(c, &a.translationReviewPage, locales.ReviewFindings(), locales.ReviewReference())
				ui.Spacer(c)
				report := candidate.Result.Report
				muted(c, locales.ReviewSummary(report.Translated, report.TimelineChanges))
			})
			if a.translationReviewPage == 1 {
				a.translationReferenceView(c.Key("subtitle-reference"), candidate)
			} else {
				a.translationFindingsView(c.Key("check-findings"), candidate)
			}
		})
	}, func() {
		ui.Button(c, locales.ActionClose()).OnClick(func() { a.translationReview = false })
	})
}

func (a *NativeApp) translationFindingsView(c *ui.Context, candidate *translationCandidate) {
	errors := len(candidate.Checks.Errors)
	warnings := len(candidate.Checks.Warnings) + len(candidate.Result.Report.Warnings)
	ui.Row(c).Wrap().Gap(em(c, 12)).AlignItems(ui.Center).Shrink(0).Children(func() {
		if ui.Tabs(c, &a.translationCheckTab, locales.ReviewAll(errors+warnings), locales.ReviewErrors(errors), locales.ReviewWarnings(warnings)).Changed() {
			a.translationIssueList.ScrollTo(0, ui.Start)
		}
		ui.SearchField(c, &a.translationIssueQuery).Grow(1).MinWidth(em(c, 220)).Label(locales.ReviewSearchFindings()).Placeholder(locales.ReviewSearchFindings()).OnChange(func() {
			a.translationIssueList.ScrollTo(0, ui.Start)
		})
	})
	findings := candidate.reviewFindings(a.translationCheckTab, a.translationIssueQuery)
	selected := slices.IndexFunc(findings, func(f translationFinding) bool { return f.index == a.translationIssueSelected })
	if selected < 0 && len(findings) > 0 {
		selected = 0
		a.translationIssueSelected = findings[0].index
	}
	empty := locales.ReviewNoIssues()
	switch {
	case strings.TrimSpace(a.translationIssueQuery) != "":
		empty = locales.ReviewNoMatches()
	case a.translationCheckTab == reviewErrorFindings:
		empty = locales.ReviewNoErrors()
	case a.translationCheckTab == reviewWarningFindings:
		empty = locales.ReviewNoWarnings()
	}
	ui.Grid(c).Columns(2).RowTracks(ui.Fr(1)).Gap(em(c, 24)).Grow(1).Basis(0).MinHeight(0).Children(func() {
		ui.Column(c).Fill().MinWidth(0).MinHeight(0).Gap(em(c, 12)).Children(func() {
			muted(c, locales.ReviewFindingsVisible(len(findings), errors+warnings)).Shrink(0)
			a.translationIssueList.Selected = &selected
			a.translationIssueList.Key = func(i int) any { return findings[i].index }
			table := subtitleTable(c, &a.translationIssueList, []ui.TableColumn{
				{Title: locales.ReviewSeverity(), Width: em(c, 88), Fixed: true},
				{Title: locales.ReviewMessage(), Fixed: true},
			}, len(findings), func(row, col int) {
				finding := findings[row]
				if col == 0 {
					text, color := locales.ReviewError(), c.Theme().Danger
					if finding.warning {
						text, color = locales.ReviewWarning(), c.Theme().Warning
					}
					label := ui.Text(c, text).SingleLine().AlignSelf(ui.Start)
					if row != selected {
						label.TextColor(color)
					}
					return
				}
				// Keep the list scannable; the selected message remains
				// complete in the adjacent reader, not in a screen-tall row.
				ui.Text(c, finding.message).MaxLines(3).AlignSelf(ui.Start)
			}, func(bool) {}).Label(locales.ReviewFindingsList())
			if table.Changed() && selected >= 0 && selected < len(findings) {
				a.translationIssueSelected = findings[selected].index
			}
		})
		ui.Column(c).Fill().MinWidth(0).MinHeight(0).Gap(em(c, 12)).Label(locales.ReviewFindingDetail()).Children(func() {
			text := empty
			ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Shrink(0).Children(func() {
				if selected >= 0 && selected < len(findings) {
					finding := findings[selected]
					text = finding.message
					label, color := locales.ReviewError(), c.Theme().Danger
					if finding.warning {
						label, color = locales.ReviewWarning(), c.Theme().Warning
					}
					icon(c, "triangle-alert").TextColor(color)
					ui.Text(c, label).TextColor(color).FontWeight(500)
				}
				ui.Text(c, locales.ReviewFindingDetail()).FontWeight(500)
			})
			muted(c, locales.ReviewBatchScope()).Shrink(0).Tooltip(locales.ReviewChecksScope())
			// A real read-only input preserves the entire message and native
			// selection/copy shortcuts, even for long multiline diagnostics.
			reviewReadOnlyText(c, locales.ReviewChecks(), text)
		})
	})
}

// Native text areas initialize their caret at the end. Diagnostics are read
// from the beginning: reset only when the displayed message changes, leaving
// subsequent scrolling and text selection under the reader's control.
func reviewReadOnlyText(c *ui.Context, label, text string) ui.Element {
	area := ui.TextArea(c, &text).ReadOnly(true).Grow(1).MinHeight(0).Label(label)
	previous := ui.Local(area, "read-text", func() string { return "" })
	if *previous != text {
		area.SetTextSelection(0, 0)
		*previous = text
	}
	return area
}

func (a *NativeApp) translationReferenceView(c *ui.Context, candidate *translationCandidate) {
	ui.Grid(c).Columns(2).RowTracks(ui.Fr(1)).Gap(em(c, 24)).Grow(1).Basis(0).MinHeight(0).Children(func() {
		ui.Column(c).Fill().MinWidth(0).MinHeight(0).Gap(em(c, 12)).Children(func() {
			if ui.SearchField(c, &a.translationQuery).Label(locales.ReviewSearch()).Placeholder(locales.ReviewSearch()).Changed() {
				a.translationList.ScrollTo(0, ui.Start)
			}
			cues := candidate.Result.Document.Cues
			rows := candidate.reviewRows(a.translationQuery)
			muted(c, locales.ReviewVisible(len(rows), len(cues))).SingleLine()
			selected := slices.Index(rows, a.translationSelected)
			a.translationList.Selected = &selected
			a.translationList.Key = func(i int) any { return cues[rows[i]].ID }
			table := subtitleTable(c, &a.translationList, []ui.TableColumn{
				numberColumn(c, len(cues)),
				{Title: locales.TabTranslation(), Fixed: true},
			}, len(rows), func(row, col int) {
				index := rows[row]
				if col == 0 {
					ui.Text(c, fmt.Sprint(index+1)).FontFeatures("tnum")
					return
				}
				ui.Text(c, cues[index].Text).SingleLine().Tooltip(cues[index].Text)
			}, func(bool) {}).Label(locales.A11yReviewList())
			if table.Changed() && selected >= 0 && selected < len(rows) {
				a.translationSelected = rows[selected]
			}
		})
		ui.Column(c).Fill().MinWidth(0).MinHeight(0).Gap(em(c, 12)).Label(locales.ReviewCompare()).Children(func() {
			var cue subtitle.Cue
			hasCue := a.translationSelected >= 0 && a.translationSelected < len(candidate.Result.Document.Cues)
			if hasCue {
				cue = candidate.Result.Document.Cues[a.translationSelected]
			}
			ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Wrap().Shrink(0).Children(func() {
				label := locales.ReviewNoSubtitle()
				if hasCue {
					label = fmt.Sprintf("# %d", a.translationSelected+1)
				}
				ui.Text(c, label).FontFeatures("tnum").Grow(1)
				if hasCue {
					muted(c, stamp(float64(cue.StartMs)/1000)+" → "+stamp(float64(cue.EndMs)/1000)).FontFeatures("tnum").Tooltip(locales.ReviewTimelineNote())
					if cue.SourceRange != nil && cue.SourceRange.Reason != "" {
						icon(c, "triangle-alert").TextColor(c.Theme().Warning).Label(locales.ReviewSegmentReason()).Tooltip(cue.SourceRange.Reason)
					}
				}
			})
			for _, field := range []struct{ label, name, text string }{
				{locales.TabSource(), locales.ReviewSource(), subtitle.SourceText(cue)},
				{locales.TabTranslation(), locales.ReviewTranslation(), cue.Text},
			} {
				ui.Column(c).Grow(1).Basis(0).MinHeight(0).Gap(em(c, 6)).Children(func() {
					ui.Text(c, field.label).Shrink(0)
					reviewReadOnlyText(c, field.name, field.text)
				})
			}
		})
	})
}
