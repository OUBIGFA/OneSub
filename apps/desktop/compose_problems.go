package main

import (
	"errors"
	"slices"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// The track and original position are kept separately from the translated
// message, so correcting a problem never guesses a row number from error text.
type composeSubtitleIssue struct {
	track, index, number int
	problems             []subtitle.CueProblem
}

func (issue composeSubtitleIssue) label() string {
	track := locales.TabOriginal()
	if issue.track == 1 {
		track = locales.TabRebuilt()
	}
	return locales.ComposeSubtitleIssue(track, issue.number, subtitleProblemDetails(issue.problems))
}

func composeSubtitleIssues(state project.Snapshot, options subtitle.OutputOptions, glyphs []string) []composeSubtitleIssue {
	translated := options.Normalized().Mode != "source" || slices.ContainsFunc(state.Document.Cues, func(cue subtitle.Cue) bool { return cue.Translated })
	tracks := []int{1}
	if translated && state.SourceDocument.Cues != nil {
		tracks = []int{0, 1}
	} else if !translated && options.SourceTrack == "original" {
		tracks = []int{0}
	}
	var issues []composeSubtitleIssue
	for _, track := range tracks {
		doc := state.TrackDocument(track)
		rows := subtitle.InspectProblems(doc, translated && track == 1, glyphs)
		if track == 0 && translated {
			for _, index := range unrepresentedSourceCues(state) {
				rows[index] = append(rows[index], subtitle.CueProblem{Kind: "missing", Track: "translation"})
			}
		}
		for index, problems := range rows {
			var blocking []subtitle.CueProblem
			for _, problem := range problems {
				if problem.BlocksCompose() {
					blocking = append(blocking, problem)
				}
			}
			if len(blocking) > 0 {
				issues = append(issues, composeSubtitleIssue{track, index, doc.Cues[index].Position + 1, blocking})
			}
		}
	}
	return issues
}

func composeSubtitleError(issues []composeSubtitleIssue) error {
	if len(issues) == 0 {
		return nil
	}
	lines := []string{locales.ComposeResolveSubtitles(len(issues))}
	for _, issue := range issues {
		lines = append(lines, issue.label())
	}
	return errors.New(strings.Join(lines, "\n"))
}

// Error details are the permitted internally scrolling list; the form and
// action row keep their natural size. Every error names a concrete cue.
func (a *NativeApp) composeSubtitleActions(c *ui.Context, issues []composeSubtitleIssue) {
	if len(issues) == 0 {
		return
	}
	locate := func() {
		issue := issues[0]
		if !a.commitEdit(true) {
			return
		}
		a.composeOpen = false
		if issue.track == 0 && a.proofreading {
			a.toggleProofreading() // Original segmentation is outside the paired proofreading track.
		}
		a.changeSubtitleTab(issue.track)
		a.problems.filter, a.query, a.followHeld = true, "", true
		a.pendingClick, a.rowClick = nil, nil
		a.selected, a.locateCue = issue.index, issue.index
		a.list.StopScroll()
		a.loadDraft()
	}
	ui.Column(c).Gap(em(c, 8)).Children(func() {
		ui.Row(c).Gap(em(c, 12)).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.ComposeResolveSubtitles(len(issues))).TextColor(c.Theme().Warning).FontSize(em(c, captionSize)).Grow(1)
			lobeButton(c, locales.SubtitleIssuesToggle()).Shrink(0).OnClick(locate)
		})
		ui.Scroll(c).Height(em(c, 80)).AlignSelf(ui.Stretch).Children(func() {
			ui.Column(c).Gap(em(c, 8)).Children(func() {
				for _, issue := range issues {
					ui.Text(c, issue.label()).FontSize(em(c, captionSize)).LineHeight(1.4)
				}
			})
		})
	})
}
