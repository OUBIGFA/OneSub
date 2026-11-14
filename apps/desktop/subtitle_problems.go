package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// Snapshot, streamed updates, glyph reports and the editor draft invalidate
// diagnostics. Ordinary paint frames reuse the complete result, as readingLoads
// does, rather than inspecting every subtitle from every toolbar/cell call.
type subtitleProblemState struct {
	filter                       bool
	snapshot                     *project.Snapshot
	tab                          int
	live                         *translationLive
	liveRevision                 uint64
	stopped                      string
	glyphs                       []string
	glyphRevision, checkedGlyphs uint64
	draft                        subtitleProblemDraft
	rows                         [][]subtitle.CueProblem
}

type subtitleProblemDraft struct {
	active, source bool
	index          int
	text           string
	start, end     float64
}

func (a *NativeApp) subtitleProblems() [][]subtitle.CueProblem {
	p := &a.problems
	var live *translationLive
	var serial uint64
	var stopped string
	if a.showsTranslationProgress() {
		live = a.translationLive
		serial, stopped = live.revision, live.stopped
	}
	draft := subtitleProblemDraft{}
	if live == nil && a.editPending {
		draft = subtitleProblemDraft{true, a.editorTextTrack() == 0, a.selected, a.draft, a.start, a.end}
	}
	if p.rows != nil && p.snapshot == a.snapshot && p.tab == a.subtitleTab && p.live == live && p.liveRevision == serial && p.stopped == stopped && p.checkedGlyphs == p.glyphRevision && p.draft == draft {
		return p.rows
	}
	doc := a.activeDocument()
	expectTranslation := false
	if live != nil {
		doc = subtitle.Document{Format: "srt", Cues: slices.Clone(live.units)}
		for i := range doc.Cues {
			cue := &doc.Cues[i]
			if live.final[i] {
				cue.TranslationSource = subtitle.SourceText(*cue)
				cue.Text, cue.Translated = live.texts[i], true
			}
		}
		expectTranslation = stopped != ""
	} else {
		if a.subtitleTab == 1 {
			expectTranslation = slices.ContainsFunc(doc.Cues, func(cue subtitle.Cue) bool { return cue.Translated })
		}
		if draft.active && draft.index >= 0 && draft.index < len(doc.Cues) {
			doc.Cues = slices.Clone(doc.Cues)
			cue := &doc.Cues[draft.index]
			if draft.source && cue.Translated {
				cue.TranslationSource = draft.text
			} else {
				cue.Text = draft.text
			}
			cue.StartMs, cue.EndMs = int64(draft.start*1000), int64(draft.end*1000)
		}
	}
	p.rows = subtitle.InspectProblems(doc, expectTranslation, p.glyphs)
	p.snapshot, p.tab, p.live, p.liveRevision = a.snapshot, a.subtitleTab, live, serial
	p.stopped, p.checkedGlyphs, p.draft = stopped, p.glyphRevision, draft
	return p.rows
}

func subtitleProblemReason(problem subtitle.CueProblem) string {
	var reason string
	switch problem.Kind {
	case "characters", "glyphs":
		codes := make([]string, 0, len(problem.Characters))
		for _, r := range problem.Characters {
			codes = append(codes, fmt.Sprintf("U+%04X", r))
		}
		reason = locales.SubtitleIssueCharacters(strings.Join(codes, ", "))
		if problem.Kind == "glyphs" {
			reason = locales.ErrMissingGlyphs(strings.Join(codes, ", "))
		}
	case "empty":
		reason = locales.SubtitleIssueEmpty()
	case "missing":
		reason = locales.SubtitleIssueMissing()
	case "time":
		reason = locales.SubtitleIssueTime()
	case "overlap":
		reason = locales.SubtitleIssueOverlap(problem.Other)
	case "long":
		reason = locales.ReadingLong()
	case "fast":
		reason = locales.ReadingFast()
	case "brief":
		reason = locales.ReadingBrief()
	}
	if problem.Track == "source" {
		return locales.TabSource() + " · " + reason
	}
	if problem.Track == "translation" {
		return locales.TabTranslation() + " · " + reason
	}
	return reason
}

func subtitleProblemDetails(problems []subtitle.CueProblem) string {
	lines := make([]string, len(problems))
	for i, problem := range problems {
		lines[i] = subtitleProblemReason(problem)
	}
	return strings.Join(lines, "\n")
}

func (a *NativeApp) subtitleProblemToggle(c *ui.Context) {
	lobeToggle(c, &a.problems.filter, "").Size(em(c, 32), em(c, 32)).Padding(0).
		Label(locales.SubtitleIssuesToggle()).Tooltip(locales.SubtitleIssuesToggle()).
		Disabled(a.subtitleSelectionMode()).Children(func() { icon(c, "file-warning") }).OnChange(func() {
		a.query = ""
		if a.problems.filter {
			a.locateSubtitleProblem(1, true)
		}
	})
}

func (a *NativeApp) subtitleProblemSummary(c *ui.Context) {
	rows := a.subtitleProblems()
	cues, count := 0, 0
	for _, problems := range rows {
		if len(problems) > 0 {
			cues++
			count += len(problems)
		}
	}
	if count == 0 && !a.problems.filter {
		return
	}
	ui.Row(c).Padding(em(c, 6), em(c, 12)).Gap(em(c, 8)).Wrap().AlignItems(ui.Center).Shrink(0).Children(func() {
		label := locales.SubtitleIssuesNone()
		if count > 0 {
			label = locales.SubtitleIssuesSummary(cues, count)
		}
		ui.Text(c, label).TextColor(c.Theme().Warning).FontSize(em(c, captionSize)).Grow(1)
		if a.problems.filter {
			toolIcon(c, locales.SubtitleIssuesPrevious(), "chevron-left").Disabled(cues == 0).OnClick(func() { a.locateSubtitleProblem(-1, false) })
			toolIcon(c, locales.SubtitleIssuesNext(), "chevron-right").Disabled(cues == 0).OnClick(func() { a.locateSubtitleProblem(1, false) })
		}
	})
}

func (a *NativeApp) subtitleProblemDetail(c *ui.Context, index int, chosen bool) {
	rows := a.subtitleProblems()
	if index < 0 || index >= len(rows) || len(rows[index]) == 0 || (!a.problems.filter && !chosen) {
		return
	}
	label := ui.Text(c, subtitleProblemDetails(rows[index])).FontSize(em(c, captionSize)).LineHeight(1.4)
	if !chosen {
		label.TextColor(c.Theme().Warning)
	}
}

func (a *NativeApp) locateSubtitleProblem(direction int, includeCurrent bool) {
	if a.subtitleSelectionMode() {
		return
	}
	live := a.showsTranslationProgress()
	current := a.selected
	if live {
		current = a.translationLive.selected
	}
	rows := a.subtitleProblems()
	if includeCurrent && current >= 0 && current < len(rows) && len(rows[current]) > 0 {
		// Keep the current draft, but also hold selection against playback and
		// locate its row in the newly filtered viewport.
		a.followHeld = true
		a.pendingClick, a.rowClick = nil, nil
		if live {
			a.translationLive.locate = current
		} else {
			a.locateCue = current
			a.list.StopScroll()
		}
		return
	}
	if !live && !a.commitEdit(true) {
		return
	}
	rows = a.subtitleProblems()
	if current < 0 || current >= len(rows) {
		current = -1
		if direction < 0 {
			current = 0
		}
	}
	for offset := 1; offset <= len(rows); offset++ {
		index := (current + direction*offset + len(rows)) % len(rows)
		if len(rows[index]) == 0 {
			continue
		}
		a.query, a.followHeld = "", true
		a.pendingClick, a.rowClick = nil, nil
		if live {
			a.translationLive.selected, a.translationLive.locate = index, index
		} else {
			a.selected, a.locateCue = index, index
			a.list.StopScroll()
			a.loadDraft()
		}
		return
	}
}
