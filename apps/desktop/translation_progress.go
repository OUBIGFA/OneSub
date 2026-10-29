package main

import (
	"fmt"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// translationProgressView takes the list's place while a translation runs:
// its stage and progress, then each translated batch as it arrives, in a
// read-only list that follows the newest line. The batches are a preview:
// the editable document is left alone until the full-file checker and the
// user's Accept action succeed.
func (a *NativeApp) translationProgressView(c *ui.Context) {
	p := a.translationProgress
	label := locales.TranslationPreparing()
	switch p.Stage {
	case "translate":
		label = locales.TranslationProgress(p.Done, p.Total)
	case "checking":
		label = locales.TranslationChecking()
	}
	ui.Column(c).Padding(em(c, 8), em(c, 12)).Gap(em(c, 8)).Shrink(0).Label(locales.TranslationRunning()).Children(func() {
		ui.Row(c).Gap(em(c, 12)).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Grow(1).Wrap().Gap(em(c, 8)).AlignItems(ui.Center).Children(func() {
				ui.Text(c, label).FontFeatures("tnum")
				muted(c, locales.TranslationDraftNote())
			})
			ui.Button(c, locales.ActionCancelTask()).Disabled(a.cancel == nil).OnClick(func() {
				if a.cancel != nil {
					a.cancel()
				}
			})
		})
		fraction := float64(-1)
		if p.Stage == "translate" && p.Total > 0 {
			fraction = float64(p.Done) / float64(p.Total)
		}
		ui.Progress(c, fraction).Label(label)
	})
	ui.Divider(c)
	rows := a.translationDraft
	if len(rows) == 0 {
		ui.Column(c).Grow(1).Center().Children(func() { muted(c, label) })
		return
	}
	maximum := 1
	for _, cue := range rows {
		maximum = max(maximum, cue.Position+1)
	}
	cols := []ui.TableColumn{numberColumn(c, maximum), timeColumn(c, locales.ListColumnStart()), timeColumn(c, locales.ListColumnEnd()), {Title: locales.TabTranslation()}}
	ui.Table(c, &a.draftList, cols, len(rows), func(row, col int) {
		cue := rows[row]
		switch col {
		case 0:
			muted(c, fmt.Sprint(cue.Position+1)).FontFeatures("tnum").SingleLine()
		case 1:
			muted(c, stamp(float64(cue.StartMs)/1000)).FontFeatures("tnum").SingleLine()
		case 2:
			muted(c, stamp(float64(cue.EndMs)/1000)).FontFeatures("tnum").SingleLine()
		default:
			ui.Text(c, cue.Text)
		}
	}).Grow(1).Label(locales.TranslationLastBatch())
	// The list follows the batches as they arrive.
	if len(rows) > a.draftShown {
		a.draftShown = len(rows)
		a.draftList.ScrollTo(len(rows)-1, ui.End)
	}
}
