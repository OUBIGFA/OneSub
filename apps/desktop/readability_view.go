package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
	"github.com/google/uuid"
)

// Cache by immutable project snapshot and track. Painting a 20,000-cue list
// never reparses every subtitle; only an uncommitted editor draft is rechecked.
func (a *NativeApp) readingLoads() []subtitle.CueReadingLoad {
	if a.readingSnapshot != a.snapshot || a.readingTab != a.subtitleTab || a.reading == nil {
		a.readingSnapshot, a.readingTab = a.snapshot, a.subtitleTab
		cues := a.activeDocument().Cues
		a.reading = make([]subtitle.CueReadingLoad, len(cues))
		for i, cue := range cues {
			a.reading[i] = subtitle.InspectCueReading(cue)
		}
	}
	return a.reading
}

func (a *NativeApp) cueReading(index int) subtitle.CueReadingLoad {
	loads := a.readingLoads()
	if index < 0 || index >= len(loads) {
		return subtitle.CueReadingLoad{}
	}
	return a.draftReading(a.activeDocument().Cues[index], index, loads[index])
}

func (a *NativeApp) draftReading(cue subtitle.Cue, index int, load subtitle.CueReadingLoad) subtitle.CueReadingLoad {
	if index != a.selected || !a.editPending {
		return load
	}
	if a.editorTextTrack() == 0 && cue.Translated {
		cue.TranslationSource = a.draft
	} else {
		cue.Text = a.draft
	}
	cue.StartMs, cue.EndMs = int64(a.start*1000), int64(a.end*1000)
	return subtitle.InspectCueReading(cue)
}

// Counts and list navigation share each category's predicate. In particular,
// speed and short duration never inflate the batch split's long-cue count.
func matchesReadingIssue(load subtitle.CueReadingLoad, kind string) bool {
	switch kind {
	case "long":
		return load.Long()
	case "fast":
		return load.Source.TooFast || load.Translation.TooFast
	case "brief":
		return load.Source.TooBrief || load.Translation.TooBrief
	}
	return false
}

type subtitleReadingIssue struct {
	kind, label, summary string
	count                int
}

func (a *NativeApp) readingIssues() []subtitleReadingIssue {
	long, fast, brief := 0, 0, 0
	for i, load := range a.readingLoads() {
		// Only the selected draft can differ from the cached snapshot. Do
		// not resolve the full optimized document once per list item.
		if i == a.selected && a.editPending {
			load = a.cueReading(i)
		}
		// A paired source/translation is still one cue in each category.
		if matchesReadingIssue(load, "long") {
			long++
		}
		if matchesReadingIssue(load, "fast") {
			fast++
		}
		if matchesReadingIssue(load, "brief") {
			brief++
		}
	}
	return []subtitleReadingIssue{
		{"long", locales.ReadingLongCues(), locales.ReadingViewLong(long), long},
		{"fast", locales.ReadingFastCues(), locales.ReadingViewFast(fast), fast},
		{"brief", locales.ReadingBriefCues(), locales.ReadingViewBrief(brief), brief},
	}
}

func (a *NativeApp) readingTools(c *ui.Context) {
	issues := a.readingIssues()
	long, fast, brief := issues[0].count, issues[1].count, issues[2].count
	selected := issues[a.readingIssue]
	// Reserve the longest category name, not its changing count. The picker
	// and both navigation buttons stay together without moving when it changes.
	var labelWidth float32
	for _, issue := range issues {
		width, _ := c.MeasureText(0, ui.Span{Text: issue.label})
		labelWidth = max(labelWidth, width)
	}
	ui.Row(c).Gap(em(c, 8)).Wrap().Shrink(1).AlignItems(ui.Center).Children(func() {
		ui.Button(c, locales.ReadingSplitCount(long)).Disabled(a.busy || a.showsTranslationProgress() || long == 0).Tooltip(locales.ReadingSplitHint()).OnClick(a.smartSplitSubtitles)
		ui.Row(c).Gap(em(c, 4)).Shrink(0).AlignItems(ui.Center).Children(func() {
			ui.MenuButton(c, selected.label, func(m *ui.Menu) {
				for i, issue := range issues {
					if m.Item(issue.summary).Checked(i == a.readingIssue).Disabled(issue.count == 0).Chosen() {
						// Choosing a category changes only the navigation target.
						// The adjacent buttons perform explicit list selection.
						a.readingIssue = i
					}
				}
			}).Label(locales.ReadingIssueType()).Width(labelWidth + c.Theme().Space(11) + 2).Shrink(0).
				Disabled(a.showsTranslationProgress() || long+fast+brief == 0).Tooltip(selected.summary + "\n" + locales.ReadingInspectHint())
			disabled := a.showsTranslationProgress() || selected.count == 0
			toolIcon(c, locales.ReadingPrevious(), "chevron-left").Shrink(0).
				Disabled(disabled).Tooltip(locales.ReadingPreviousHint()).OnClick(func() { a.locateReadingIssue(selected.kind, -1) })
			toolIcon(c, locales.ReadingNext(), "chevron-right").Shrink(0).
				Disabled(disabled).Tooltip(locales.ReadingNextHint()).OnClick(func() { a.locateReadingIssue(selected.kind, 1) })
		})
	})
}

// Keep review and splitting reachable without sacrificing the subtitle rows
// to three extra toolbar lines. Both presentations use the same issue counts.
func (a *NativeApp) readingMenu(c *ui.Context) {
	issues := a.readingIssues()
	selected := issues[a.readingIssue]
	toolIcon(c, locales.ReadingInspect(), "triangle-alert").Disabled(a.showsTranslationProgress()).Tooltip(locales.ReadingInspect() + "\n" + selected.summary).Menu(func(m *ui.Menu) {
		for i, issue := range issues {
			if m.Item(issue.summary).Checked(i == a.readingIssue).Disabled(issue.count == 0).Chosen() {
				a.readingIssue = i
			}
		}
		m.Separator()
		if m.Item(locales.ReadingPrevious()).Disabled(selected.count == 0).Chosen() {
			a.locateReadingIssue(selected.kind, -1)
		}
		if m.Item(locales.ReadingNext()).Disabled(selected.count == 0).Chosen() {
			a.locateReadingIssue(selected.kind, 1)
		}
		m.Separator()
		if m.Item(locales.ReadingSplitCount(issues[0].count)).Disabled(a.busy || issues[0].count == 0).Chosen() {
			a.smartSplitSubtitles()
		}
	})
}

// Navigate in document order and wrap at either end, including cues hidden by
// search. With no selection, forward starts at the first and backward at the last.
func (a *NativeApp) locateReadingIssue(kind string, direction int) {
	if a.snapshot == nil || a.showsTranslationProgress() || !a.commitEdit(true) {
		return
	}
	loads := a.readingLoads()
	start := a.selected
	if start < 0 || start >= len(loads) {
		start = -1
		if direction < 0 {
			start = 0
		}
	}
	for offset := 1; offset <= len(loads); offset++ {
		index := (start + direction*offset + len(loads)) % len(loads)
		if !matchesReadingIssue(loads[index], kind) {
			continue
		}
		a.pendingClick, a.rowClick = nil, nil
		a.query, a.selected = "", index
		a.list.StopScroll()
		a.loadDraft()
		a.locateCue, a.followHeld = index, true
		return
	}
}

func readingReason(reason string) string {
	switch reason {
	case "protected":
		return locales.ReadingProtected()
	case "duration":
		return locales.ReadingShortTime()
	case "bilingual":
		return locales.ReadingPairing()
	case "capacity":
		return locales.ReadingCapacity()
	case "boundary":
		return locales.ReadingNoBoundary()
	case "readable":
		return locales.ReadingReadable()
	}
	return ""
}

func (a *NativeApp) readingLabel(cue subtitle.Cue, load subtitle.CueReadingLoad) (string, string) {
	labels, details := []string{}, []string{}
	if load.Long() {
		labels = append(labels, locales.ReadingLong())
	}
	if load.Source.TooFast || load.Translation.TooFast {
		labels = append(labels, locales.ReadingFast())
	}
	if load.Source.TooBrief || load.Translation.TooBrief {
		labels = append(labels, locales.ReadingBrief())
	}
	// Keep the status bar short. Timing and pairing review notes remain in
	// its tooltip alongside the metrics, never inside the subtitle text.
	for i, value := range []subtitle.ReadingLoad{load.Source, load.Translation} {
		if value.Units == 0 {
			continue
		}
		name := locales.TabSource()
		if i == 1 {
			name = locales.TabTranslation()
		}
		details = append(details, locales.ReadingMetrics(name, fmt.Sprintf("%.1f", value.Units), fmt.Sprintf("%.0f", value.MaxUnits), fmt.Sprintf("%.1f", value.Rate), fmt.Sprintf("%.1f", value.MaxRate)))
	}
	if reason := readingReason(a.splitSkipped[cue.ID]); reason != "" {
		details = append(details, reason)
	}
	if cue.TimingQuality == "estimated" {
		details = append(details, locales.ReadingEstimatedHint())
	}
	if cue.SplitReview {
		details = append(details, locales.ReadingReviewHint())
	}
	return strings.Join(labels, " · "), strings.Join(details, "\n")
}

func (a *NativeApp) smartSplitSubtitles() { a.smartSplitSubtitle("", false) }

func (a *NativeApp) smartSplitSubtitle(id string, force bool) {
	if a.busy || a.snapshot == nil || a.store == nil || a.showsTranslationProgress() || !a.commitEdit(true) {
		return
	}
	a.run(locales.ReadingSplit(), a.smartSplitWork(id, force))
}

// Freeze the track, target and revision before the task starts. The toolbar
// calls the whole-track plan; the context menu calls the single-cue plan.
func (a *NativeApp) smartSplitWork(id string, force bool) func(context.Context) (func(), error) {
	doc, original, revision, store := a.activeDocument(), a.subtitleTab == 0, a.snapshot.Revision, a.store
	command := uuid.NewString()
	worker := a.worker
	// Explicit splitting cleans both resulting text layers even when automatic
	// translation cleanup is off; retain the user's chosen exception marks.
	punctuation := subtitle.PunctuationOptions{StripTrailing: true, ReplacePeriods: true, Keep: a.settings.KeepPunctuation}
	return func(ctx context.Context) (func(), error) {
		var report subtitle.SplitReport
		var err error
		if id == "" {
			report, err = subtitle.SmartSplit(ctx, doc, command, original, punctuation)
		} else if force {
			report, err = subtitle.SmartSplitCueForced(ctx, doc, command, original, id, punctuation)
		} else {
			report, err = subtitle.SmartSplitCue(ctx, doc, command, original, id, punctuation)
		}
		if err != nil {
			return nil, err
		}
		if report.Changed > 0 {
			// Both displayed languages must remain serializable. This is a
			// preflight only: no subtitle file is written by this operation.
			if doc.Format != "srt" {
				if _, err = worker.Write(ctx, report.Document); err != nil {
					return nil, err
				}
				source := report.Document
				source.Cues = append([]subtitle.Cue{}, source.Cues...)
				for i := range source.Cues {
					source.Cues[i].Text = subtitle.SourceText(source.Cues[i])
				}
				if _, err = worker.Write(ctx, source); err != nil {
					return nil, err
				}
			}
			if id == "" {
				_, err = store.SplitCues(ctx, command, revision, original, punctuation)
			} else if force {
				_, err = store.SplitCueForced(ctx, command, revision, original, id, punctuation)
			} else {
				_, err = store.SplitCue(ctx, command, revision, original, id, punctuation)
			}
			if err != nil {
				return nil, err
			}
		}
		return func() {
			if report.Changed > 0 {
				a.refreshAfterStructure(report.First)
				a.followHeld = true
			}
			a.splitSkipped = report.Skipped
			a.status = locales.ReadingResult(report.Changed, report.Added, report.Estimated, report.Remaining)
			if report.Changed == 0 {
				a.status = locales.ReadingNoChanges(report.Remaining)
				if id != "" {
					a.status = readingReason(report.Skipped[id])
				}
			}
		}, nil
	}
}
