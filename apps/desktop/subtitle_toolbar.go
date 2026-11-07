package main

import (
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// File actions have a stable first row; the second groups editing, selection
// and review. Narrow panes keep the same order through native compact menus.
func (a *NativeApp) subtitleToolbar(c *ui.Context, compact, hasCues bool) {
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
		ui.Row(c).Gap(em(c, 8)).Wrap().AlignItems(ui.Center).Children(func() {
			ui.Toolbar(c, func() {
				a.subtitleActionsMenu(c)
				toolIcon(c, locales.ListLocate(), "locate-fixed").Disabled(!hasCues).OnClick(a.locateCurrent)
				if !compact {
					a.listPunctuationTools(c)
				}
				toolIcon(c, locales.FindReplace(), "text-search").Disabled(!hasCues || a.busy || a.showsTranslationProgress()).OnClick(a.openFindReplace)
			}).Padding(0).Shrink(0)
			ui.Toolbar(c, func() {
				a.subtitleSelectionActions(c, compact)
				a.retranslationActions(c, compact)
			}).Padding(0).Shrink(0)
			if compact {
				a.readingMenu(c)
			} else {
				a.readingTools(c)
			}
			toolIcon(c, locales.TranslationResults(), "file-warning").Disabled(a.translationCandidate == nil).OnClick(func() { a.translationReview = true })
		})
	})
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
	minimum := max(em(c, 140), width+c.Theme().Space(11))
	if compact {
		minimum = width + c.Theme().Space(11)
	}
	if ui.SearchField(c, &a.query).Placeholder(label).Label(locales.ListSearch()).Tooltip(hint).Disabled(len(cues) == 0).MinWidth(minimum).Grow(1).Changed() {
		a.pendingClick = nil
		a.list.StopScroll()
		a.activeCue = -1
		a.noteBrowse(c.Now())
		if a.translationLive != nil {
			a.translationLive.list.StopScroll()
			a.translationLive.shown = -1
		}
	}
}

func (a *NativeApp) subtitleSaveActions(c *ui.Context, compact, hasCues bool) {
	ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Shrink(0).Label(locales.A11ySubtitleFileActions()).Children(func() {
		if !compact {
			ui.Divider(c).Height(em(c, 20)).AlignSelf(ui.Center).Margin(0, em(c, 4))
		}
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
