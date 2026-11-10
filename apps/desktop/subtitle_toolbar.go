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
	// Measure this frame's actual labels at the native button's weight.
	// Spend horizontal padding and gaps before giving up icon + text.
	labels := []string{locales.SubtitleActionsMenu(), locales.ListLocate(), locales.TextReplacePeriods(), locales.TextStripTrailing(), locales.FindReplace(), locales.RetranslateCheckAll(), locales.TranslationResults()}
	if count := len(a.checkedRetranslationIDs()); count > 0 {
		labels = append(labels, locales.RetranslateSelected(count), locales.RetranslateClear())
	} else {
		labels = append(labels, locales.RetranslateSelect())
	}
	if a.retranslation != nil {
		labels = append(labels, locales.RetranslateReopen())
	}
	issues := a.readingIssues()
	labels = append(labels, locales.ReadingSplitCount(issues[0].count))
	var textWidth, issueWidth float32
	for _, label := range labels {
		w, _ := c.MeasureText(0, ui.Span{Text: label, Weight: 500})
		textWidth += float32(math.Ceil(float64(w)))
	}
	for _, issue := range issues {
		w, _ := c.MeasureText(0, ui.Span{Text: issue.label, Weight: 500})
		issueWidth = max(issueWidth, float32(math.Ceil(float64(w))))
	}
	// Each labelled button has an icon, a border and an inner gap. Two
	// navigation buttons remain square; all neighbouring controls share a gap.
	count := float32(len(labels) + 1)
	fixed := textWidth + issueWidth + count*(em(c, iconSize)+2) + em(c, 24+2*32)
	minimum := fixed + count*em(c, 12) + (count+1)*em(c, 4)
	comfortable := fixed + count*em(c, 34) + (count+1)*em(c, 8)
	layout := subtitleToolbarLayout{
		compact:   width < minimum,
		tightness: min(1, max(0, (comfortable-width)/(comfortable-minimum))),
	}
	ui.Column(c).Padding(em(c, 8), em(c, 12)).Gap(em(c, 8)).Shrink(0).Label(locales.A11ySubtitleActions()).Children(func() {
		ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Children(func() {
			if compact {
				toolIcon(c, locales.ListImport(), "folder-open").Disabled(a.busy).OnClick(a.pickSubtitle)
			} else {
				iconButton(c, locales.ListImport(), "folder-open").Disabled(a.busy).OnClick(a.pickSubtitle)
			}
			a.subtitleSearch(c, compact)
			a.subtitleSaveActions(c, compact, hasCues)
		})
		ui.Row(c).Gap(layout.spacing(c)).Wrap().AlignItems(ui.Center).Children(func() {
			ui.Toolbar(c, func() {
				a.subtitleActionsMenu(c, layout)
				subtitleTool(c, locales.ListLocate(), "locate-fixed", layout).Disabled(!hasCues).OnClick(a.locateCurrent)
				if !compact {
					a.listPunctuationTools(c, layout)
				}
				subtitleTool(c, locales.FindReplace(), "text-search", layout).Disabled(!hasCues || a.busy || a.showsTranslationProgress()).OnClick(a.openFindReplace)
			}).Padding(0).Gap(layout.spacing(c)).Shrink(0)
			ui.Toolbar(c, func() {
				a.subtitleSelectionActions(c, layout)
				a.retranslationActions(c, layout)
			}).Padding(0).Gap(layout.spacing(c)).Shrink(0)
			if compact {
				a.readingMenu(c)
			} else {
				a.readingTools(c, layout)
			}
			subtitleTool(c, locales.TranslationResults(), "file-warning", layout).Disabled(a.translationCandidate == nil).OnClick(func() { a.translationReview = true })
		})
	})
}

// Horizontal density changes only whitespace. Text, icons and hit-target
// height keep their normal size, and every presentation retains its tooltip.
type subtitleToolbarLayout struct {
	compact   bool
	tightness float32
}

func (l subtitleToolbarLayout) spacing(c *ui.Context) float32 { return em(c, 8-4*l.tightness) }

func (l subtitleToolbarLayout) button(c *ui.Context, button ui.Element) ui.Element {
	if !l.compact {
		button.PaddingX(em(c, 14-10*l.tightness)).Gap(em(c, 6-2*l.tightness))
	}
	return button.Shrink(0)
}

func subtitleTool(c *ui.Context, label, name string, layout subtitleToolbarLayout) ui.Element {
	if layout.compact {
		return toolIcon(c, label, name)
	}
	return layout.button(c, iconButton(c, label, name)).Tooltip(label)
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
		unsaved := a.trackUnsaved(a.subtitleTab)
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
		saveAs.Disabled(a.busy || a.showsTranslationProgress() || !hasCues || !a.trackHasContent(a.subtitleTab)).OnClick(func() { a.saveTrack(a.subtitleTab, true) })
	})
}
