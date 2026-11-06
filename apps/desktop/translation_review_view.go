package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// Filtering changes only the list, not the frozen check report or the selected
// comparison. Search matches the displayed number, source and translation.
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

// The check report is read-only. Match the retranslation workbench's large,
// fixed shell and full-height list, without adding another apply/save path.
func (a *NativeApp) translationView(c *ui.Context) {
	fixedDialog(c, &a.translationReview, locales.TranslationResults(), 1600, 1000, func() {
		candidate := a.translationCandidate
		if candidate == nil {
			ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				muted(c, locales.ReviewEmpty())
			})
			return
		}
		report, checks := candidate.Result.Report, candidate.Checks
		ui.Column(c).Fill().MinHeight(0).Gap(em(c, 16)).Children(func() {
			ui.Text(c, locales.ReviewSummary(report.Translated, report.TimelineChanges, len(checks.Errors), len(checks.Warnings)+len(report.Warnings))).Shrink(0)
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
							ui.TextArea(c, &field.text).ReadOnly(true).Grow(1).MinHeight(0).Label(field.name)
						})
					}
					// Findings belong to the whole translation, not the selected
					// cue. Keep all raw messages; do not guess IDs from their text.
					ui.Column(c).Grow(1).Basis(0).MinHeight(0).Gap(em(c, 6)).Children(func() {
						ui.Tabs(c, &a.translationCheckTab, locales.ReviewErrors(len(checks.Errors)), locales.ReviewWarnings(len(checks.Warnings)+len(report.Warnings))).Shrink(0).Tooltip(locales.ReviewChecksScope())
						messages := checks.Errors
						empty := locales.ReviewNoErrors()
						if a.translationCheckTab == 1 {
							messages = append(append([]string{}, report.Warnings...), checks.Warnings...)
							empty = locales.ReviewNoWarnings()
						}
						text := strings.Join(messages, "\n\n")
						if text == "" {
							text = empty
						}
						area := ui.TextArea(c, &text).ReadOnly(true).Grow(1).MinHeight(0).Label(locales.ReviewChecks()).Tooltip(locales.ReviewChecksScope())
						if a.translationCheckTab == 0 && len(messages) > 0 {
							area.TextColor(c.Theme().Danger)
						}
					})
				})
			})
		})
	}, func() {
		ui.Button(c, locales.ActionClose()).OnClick(func() { a.translationReview = false })
	})
}
