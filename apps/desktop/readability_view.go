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

func (a *NativeApp) readingTools(c *ui.Context) {
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
	// The action itself names its target and count. Native wrapping keeps
	// that complete label visible even with 20,000 cues in a narrow pane.
	ui.Row(c).Gap(em(c, 8)).Wrap().Shrink(1).AlignItems(ui.Center).Children(func() {
		ui.Button(c, locales.ReadingSplitCount(long)).Disabled(a.busy || a.showsTranslationProgress() || long == 0).Tooltip(locales.ReadingSplitHint()).OnClick(a.smartSplitSubtitles)
		ui.MenuButton(c, locales.ReadingInspect(), func(m *ui.Menu) {
			for _, entry := range []struct {
				kind, label string
				count       int
			}{{"long", locales.ReadingViewLong(long), long}, {"fast", locales.ReadingViewFast(fast), fast}, {"brief", locales.ReadingViewBrief(brief), brief}} {
				if m.Item(entry.label).Disabled(entry.count == 0).Chosen() {
					a.locateReadingIssue(entry.kind)
				}
			}
		}).Disabled(a.showsTranslationProgress() || long+fast+brief == 0).Tooltip(locales.ReadingInspectHint())
	})
}

func (a *NativeApp) locateReadingIssue(kind string) {
	if a.snapshot == nil || a.showsTranslationProgress() || !a.commitEdit(true) {
		return
	}
	loads := a.readingLoads()
	for offset := 1; offset <= len(loads); offset++ {
		index := (max(a.selected, -1) + offset) % len(loads)
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
