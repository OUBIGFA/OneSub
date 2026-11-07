package main

import (
	"slices"
	"strings"

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

// This window only reads batch diagnostics. Subtitle browsing and editing
// belong to the workbench, never to the check report.
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
			a.translationFindingsView(c, candidate)
		})
	}, func() {
		ui.Spacer(c)
		if candidate := a.translationCandidate; candidate != nil {
			muted(c, locales.ReviewSummary(candidate.Result.Report.Translated, candidate.Result.Report.TimelineChanges))
		}
		ui.Button(c, locales.ActionClose()).OnClick(func() { a.translationReview = false })
	})
}

func (a *NativeApp) translationFindingsView(c *ui.Context, candidate *translationCandidate) {
	errors := len(candidate.Checks.Errors)
	warnings := len(candidate.Checks.Warnings) + len(candidate.Result.Report.Warnings)
	ui.Row(c).Wrap().Gap(em(c, 16)).AlignItems(ui.Center).Shrink(0).Children(func() {
		if segmentedFill(c, &a.translationCheckTab, locales.ReviewAll(errors+warnings), locales.ReviewErrors(errors), locales.ReviewWarnings(warnings)).MinWidth(em(c, 320)).Grow(1).Changed() {
			a.translationIssueList.ScrollTo(0, ui.Start)
		}
		ui.SearchField(c, &a.translationIssueQuery).Grow(1).Basis(0).MinWidth(em(c, 220)).Label(locales.ReviewSearchFindings()).Placeholder(locales.ReviewSearchFindings()).OnChange(func() {
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
	ui.Grid(c).ColumnTracks(ui.Fr(1), ui.Fr(1.2)).RowTracks(ui.Fr(1)).Gap(em(c, 24)).Grow(1).Basis(0).MinHeight(0).Children(func() {
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
				ui.Text(c, finding.message).MaxLines(3).LineHeight(1.4).AlignSelf(ui.Start)
			}, func(bool) {}).Label(locales.ReviewFindingsList())
			if table.Changed() && selected >= 0 && selected < len(findings) {
				a.translationIssueSelected = findings[selected].index
			}
		})
		ui.Column(c).Fill().MinWidth(0).MinHeight(0).Gap(em(c, 12)).Padding(em(c, 16)).Background(studioColors(c).panel).Border(1, studioColors(c).hairline).Radius(c.Theme().Radius).Label(locales.ReviewFindingDetail()).Children(func() {
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
	area := ui.TextArea(c, &text).ReadOnly(true).Grow(1).MinHeight(0).Padding(em(c, 12)).FontSize(em(c, 16)).LineHeight(1.5).Background(studioColors(c).panel).Border(0, ui.Transparent).Label(label)
	previous := ui.Local(area, "read-text", func() string { return "" })
	if *previous != text {
		area.SetTextSelection(0, 0)
		*previous = text
	}
	return area
}
