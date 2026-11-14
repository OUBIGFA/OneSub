package main

import (
	"math"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// File actions have a stable first row; the second groups editing, selection
// and review. Narrow panes keep the same order through native compact menus.
func (a *NativeApp) subtitleToolbar(c *ui.Context, width float32, hasCues bool) {
	compact := width < em(c, 680)
	layout := a.subtitleToolbarDensity(c, width-em(c, 32+8), compact)
	ui.Column(c).Padding(em(c, 8), em(c, 12)).Gap(em(c, 8)).Shrink(0).Label(locales.A11ySubtitleActions()).Children(func() {
		ui.Row(c).Gap(em(c, 8)).Wrap().AlignItems(ui.Center).Children(func() {
			a.proofreadingButton(c, compact, width < em(c, 300))
			if compact {
				toolIcon(c, locales.ListImport(), "folder-open").Disabled(a.busy).OnClick(a.pickSubtitle)
			} else {
				iconButton(c, locales.ListImport(), "folder-open").Disabled(a.busy).OnClick(a.pickSubtitle)
			}
			a.subtitleSearch(c, compact)
			a.subtitleSaveActions(c, compact, hasCues)
		})
		ui.Row(c).Gap(layout.spacing(c)).Wrap().AlignItems(ui.Center).Children(func() {
			selectionMode := a.subtitleSelectionMode()
			ui.Toolbar(c, func() {
				a.subtitleActionsMenu(c, layout)
				a.subtitleSelectionActions(c, layout)
				subtitleTool(c, locales.ListLocate(), "locate-fixed", layout).Disabled(!hasCues || selectionMode).OnClick(a.locateCurrent)
				if !compact {
					a.listPunctuationTools(c, layout)
				}
				subtitleTool(c, locales.FindReplace(), "text-search", layout).Disabled(!hasCues || a.busy || a.showsTranslationProgress() || selectionMode).OnClick(a.openFindReplace)
			}).Padding(0).Gap(layout.spacing(c)).Shrink(0)
			ui.Toolbar(c, func() {
				a.retranslationActions(c, layout)
			}).Padding(0).Gap(layout.spacing(c)).Shrink(0)
			if compact {
				a.readingMenu(c, layout)
			} else {
				a.readingTools(c, layout)
			}
			subtitleTool(c, locales.TranslationResults(), "file-warning", layout).Disabled(a.translationCandidate == nil || selectionMode).OnClick(func() { a.translationReview = true })
			a.subtitleProblemToggle(c)
			ui.Spacer(c)
			guide := toolIcon(c, locales.SubtitleLengthGuideToggle(), "ruler").Shrink(0).OnClick(func() { a.subtitleGuide.visible = !a.subtitleGuide.visible })
			if a.subtitleGuide.visible {
				guide.Background(c.Theme().Accent).TextColor(c.Theme().AccentText)
			}
		})
	})
}

// Fit this frame's visible controls, not the full set hidden by narrow-pane
// menus. Once whitespace is exhausted, review/cleanup labels yield one at a
// time; locating and finding subtitles retain icon + text for longest.
func (a *NativeApp) subtitleToolbarDensity(c *ui.Context, width float32, narrow bool) subtitleToolbarLayout {
	labels := []string{}
	if a.retranslation != nil {
		labels = append(labels, locales.RetranslateReopen())
	}
	labels = append(labels, locales.TranslationResults())
	var issueWidth float32
	navigation := float32(0)
	if !narrow {
		issues := a.readingIssues()
		labels = append(labels, locales.TextStripTrailing(), locales.TextReplacePeriods(), locales.ReadingSplitCount(issues[0].count), locales.ReadingIssueType())
		for _, issue := range issues {
			w, _ := c.MeasureText(0, ui.Span{Text: issue.label, Weight: 500})
			issueWidth = max(issueWidth, float32(math.Ceil(float64(w))))
		}
		navigation = 2
	}
	if count := len(a.checkedRetranslationIDs()); count > 0 {
		labels = append(labels, locales.RetranslateClear(), locales.RetranslateCheckAll(), locales.RetranslateSelected(count))
	} else {
		labels = append(labels, locales.RetranslateCheckAll(), locales.RetranslateSelect())
	}
	if narrow {
		labels = append(labels, locales.ReadingInspect())
	}
	labels = append(labels, locales.SubtitleActionsMenu(), locales.FindReplace(), locales.ListLocate())
	widths := make([]float32, len(labels))
	minimum := em(c, 60+navigation*32+(float32(len(labels))+navigation-1)*4)
	for i, label := range labels {
		w, _ := c.MeasureText(0, ui.Span{Text: label, Weight: 500})
		if label == locales.ReadingIssueType() {
			w = issueWidth
		}
		widths[i] = float32(math.Ceil(float64(w))) + em(c, iconSize+12) + 2
		minimum += widths[i]
	}
	comfortable := minimum + em(c, float32(len(labels))*22+(float32(len(labels))+navigation-1)*4)
	layout := subtitleToolbarLayout{tightness: min(1, max(0, (comfortable-width)/(comfortable-minimum)))}
	if width < minimum {
		layout.icons = make(map[string]bool)
		for i, label := range labels {
			if minimum <= width {
				break
			}
			layout.icons[label] = true
			minimum -= widths[i] - em(c, 32)
		}
	}
	// At the smallest supported pane, keep every second-row action square so
	// the shortened Locate label cannot consume the row needed by the list.
	if width < em(c, 400) {
		if layout.icons == nil {
			layout.icons = make(map[string]bool)
		}
		for _, label := range []string{locales.SubtitleActionsMenu(), locales.ListLocate(), locales.FindReplace(), locales.TranslationResults(), locales.ReadingInspect(), locales.RetranslateSelect(), locales.RetranslateCheckAll(), locales.RetranslateClear()} {
			layout.icons[label] = true
		}
		if count := len(a.checkedRetranslationIDs()); count > 0 {
			layout.icons[locales.RetranslateSelected(count)] = true
		}
	}
	return layout
}

// Horizontal density changes only whitespace. Text, icons and hit-target
// height keep their normal size, and every presentation retains its tooltip.
type subtitleToolbarLayout struct {
	compact   bool
	tightness float32
	icons     map[string]bool
}

func (l subtitleToolbarLayout) iconOnly(label string) bool { return l.compact || l.icons[label] }

func (l subtitleToolbarLayout) spacing(c *ui.Context) float32 { return em(c, 8-4*l.tightness) }

func (l subtitleToolbarLayout) button(c *ui.Context, label string, button ui.Element) ui.Element {
	if !l.iconOnly(label) {
		button.PaddingX(em(c, 14-10*l.tightness)).Gap(em(c, 6-2*l.tightness))
	}
	return button.Shrink(0)
}

func subtitleTool(c *ui.Context, label, name string, layout subtitleToolbarLayout) ui.Element {
	if layout.iconOnly(label) {
		return toolIcon(c, label, name)
	}
	return layout.button(c, label, iconButton(c, label, name)).Tooltip(label)
}

func (a *NativeApp) subtitleSearch(c *ui.Context, compact bool) {
	cues := a.activeDocument().Cues
	if a.showsTranslationProgress() {
		cues = a.translationLive.units
	}
	hint := subtitleSearchLabel(cues)
	label := hint
	if compact {
		label = locales.ListSearch()
	}
	width, _ := c.MeasureText(0, ui.Span{Text: label})
	minimum := max(em(c, 140), width+em(c, 64))
	if compact {
		minimum = width + em(c, 64)
	}
	lobeSearchField(c, &a.query, func() {
		a.pendingClick = nil
		a.list.StopScroll()
		a.activeCue = -1
		a.noteBrowse(c.Now())
		if a.translationLive != nil {
			a.translationLive.list.StopScroll()
			a.translationLive.shown = -1
		}
	}, func(frame ui.Element) { frame.MinWidth(minimum).Grow(1).Disabled(len(cues) == 0) }).Placeholder(label).Label(locales.ListSearch()).Tooltip(hint)
}

func (a *NativeApp) subtitleSaveActions(c *ui.Context, compact, hasCues bool) {
	ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Shrink(0).Label(locales.A11ySubtitleFileActions()).Children(func() {
		unsaved := a.trackUnsaved(a.subtitleTab) || a.subtitleTab == 1 && a.trackUnsaved(2)
		label := locales.ListSave()
		if unsaved {
			label = locales.ListUnsaved()
		}
		// Reserve both labels: the status can change without moving controls.
		savedWidth, _ := c.MeasureText(0, ui.Span{Text: locales.ListSave()})
		dirtyWidth, _ := c.MeasureText(0, ui.Span{Text: locales.ListUnsaved()})
		saveWidth := max(savedWidth, dirtyWidth) + em(c, iconSize) + c.Theme().Space(8.5) + 2
		save := iconButton(c, label, "save").Width(saveWidth).Shrink(0).Label(locales.ListSave()).Description(locales.ListSave()).Tooltip(locales.ListSave() + " · Ctrl+S").Disabled(a.busy || a.showsTranslationProgress() || a.snapshot == nil || !unsaved).OnClick(func() { a.saveTrack(a.subtitleTab, false) })
		if unsaved {
			save.TextColor(c.Theme().Danger)
		}
		var saveAs ui.Element
		if compact {
			saveAs = toolIcon(c, locales.ListSaveAs(), "file-down")
		} else {
			saveAs = iconButton(c, locales.ListSaveAs(), "file-down")
		}
		saveAs.Disabled(a.busy || a.showsTranslationProgress() || !hasCues || !(a.trackHasContent(a.subtitleTab) || a.proofreading && a.trackHasContent(2))).OnClick(func() { a.saveTrack(a.subtitleTab, true) })
	})
}
