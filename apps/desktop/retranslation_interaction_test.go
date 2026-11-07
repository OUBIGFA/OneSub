package main

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

func TestRetranslationSelectionToolbarAtMinimumWindow(t *testing.T) {
	for _, lang := range []string{"zh-Hans", "en"} {
		inLocale(t, lang, func() {
			a := checkedReviewApp(t)
			a.retranslation.open = false
			headerWidths := map[string]float32{}
			tt := settled(func(c *ui.Context) {
				a.view(c)
				for _, label := range []string{locales.ListColumnStart() + " / " + locales.ListColumnEnd(), locales.ListColumnText()} {
					headerWidths[label], _ = c.MeasureText(0, ui.Span{Text: label, Weight: 600})
				}
			}, 900, 640)
			list, _ := tt.Find(locales.A11ySubtitleList())
			// Narrow lists stack the time pair. Headers and subtitle text must
			// now be readable without horizontal scrolling, including selection.
			for label, width := range headerWidths {
				header, found := tt.Find(label)
				if !found || header.W < width-1 {
					t.Fatal("column header ellipsized with selection active", label, header, width)
				}
			}
			for _, label := range []string{locales.RetranslateSelected(2), locales.RetranslateCheckAll(), locales.RetranslateClear(), locales.RetranslateReopen()} {
				insideRedesign(t, tt, locales.A11ySubtitleActions(), label)
			}
			captureRedesign(t, tt, "retranslation-selected-min-"+lang)
			if list.H < 60 {
				t.Fatal("selection tools consumed the subtitle list", lang, list)
			}
		})
	}
}

func TestRetranslationSelectAllAndClearIncludeHiddenRows(t *testing.T) {
	a := checkedReviewApp(t)
	a.retranslation = nil
	a.subtitleChecked[0] = nil
	a.query = "First"
	tt := settled(a.subtitlesView, 900, 640)
	if _, ok := tt.Find(locales.RetranslateCheckCue(1)); ok {
		t.Fatal("empty selection exposes checks")
	}
	if err := tt.Click(locales.RetranslateCheckAll()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a.checkedRetranslationIDs(), []string{"a", "b"}) {
		t.Fatal("select all ignored hidden rows")
	}
	if _, ok := tt.Find(locales.RetranslateCheckCue(1)); !ok {
		t.Fatal("checks not shown after select all")
	}
	if err := tt.Click(locales.RetranslateClear()); err != nil {
		t.Fatal(err)
	}
	if len(a.checkedRetranslationIDs()) != 0 {
		t.Fatal("clear did not include hidden rows")
	}
	if _, ok := tt.Find(locales.RetranslateCheckCue(1)); ok {
		t.Fatal("clear did not hide checks")
	}
	if err := tt.Click(locales.RetranslateSelect()); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(locales.RetranslateCheckCue(1)); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find(locales.RetranslateCheckCue(1)); ok {
		t.Fatal("last unchecked cue did not hide checks")
	}
}

func TestSubtitleSelectionUsesMirroredLeadingGutter(t *testing.T) {
	a := storeApp(t, subtitle.Cue{ID: "a", Text: "First source", StartMs: 0, EndMs: 4000}, subtitle.Cue{ID: "b", Text: "Second source", StartMs: 5000, EndMs: 9000})
	tt := settled(a.subtitlesView, 900, 640)
	list, _ := tt.Find(locales.A11ySubtitleList())
	number, ok := tt.Find("#")
	if !ok || number.X < list.X+subtitleNavigationRailSize || tt.HasText(locales.RetranslateCheckCue(1)) {
		t.Fatal("selection gutter is missing or exposes a checkbox while inactive", list, number)
	}
	a.setSubtitleChecked("a", true)
	tt.Frame()
	check, ok := tt.Find(locales.RetranslateCheckCue(1))
	if !ok || check.X < list.X || check.X+check.W > list.X+subtitleNavigationRailSize {
		t.Fatal("checkbox is outside the mirrored leading gutter", check, list)
	}
	if after, _ := tt.Find("#"); after.X != number.X || after.W != number.W {
		t.Fatal("enabling selection shifted the subtitle columns", number, after)
	}
}

func TestRetranslationReopenAfterEditsIsReadOnly(t *testing.T) {
	a := checkedReviewApp(t)
	r := a.retranslation
	r.rows[0].result = &retranslationResult{text: "新译文", valid: true}
	r.open = false
	a.subtitleChecked[0] = nil
	work := a.listTextEditWorkFor(r.source, r.revision, []project.Edit{{ID: "a", Track: "translation", Text: "后来手改的译文", StartMs: 0, EndMs: 4000}})
	apply, err := work(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	apply()
	tt := settled(func(c *ui.Context) { a.subtitlesView(c); a.retranslationView(c) }, 1000, 820)
	if err := tt.Click(locales.RetranslateReopen()); err != nil {
		t.Fatal(err)
	}
	if a.retranslation != r || !r.open || !a.retranslationStale(r) || r.rows[0].result.text != "新译文" {
		t.Fatal("reopening changed results or lost stale guard")
	}
	before := *a.snapshot
	for _, label := range []string{locales.RetranslateApply(), locales.RetranslateApplyOne(), locales.RetranslateOne(), locales.RetranslateRetry()} {
		// Disabled controls can still be hit-tested by the headless driver;
		// the guarded callbacks must not run.
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		if r.errorText != "" {
			t.Fatalf("stale action callback ran: %s", label)
		}
	}
	a.acceptRetranslation(false)
	a.startRetranslation(false)
	if !reflect.DeepEqual(before, *a.snapshot) || r.errorText != locales.RetranslateStale() {
		t.Fatal("stale review overwrote later edits")
	}
}

func TestRetranslationNewBatchRequiresExplicitDiscard(t *testing.T) {
	a := checkedReviewApp(t)
	r := a.retranslation
	r.rows[0].result = &retranslationResult{text: "保留的结果", valid: true}
	r.open = false
	a.openRetranslation()
	if a.retranslation != r || !r.open || a.retranslationNext != nil {
		t.Fatal("same batch did not reopen")
	}
	r.open = false
	a.setSubtitleChecked("b", false)
	a.openRetranslation()
	if a.retranslation != r || a.retranslationNext == nil {
		t.Fatal("different batch silently replaced results")
	}
	tt := settled(a.retranslationView, 1000, 820)
	if err := tt.Click(locales.ActionCancel()); err != nil {
		t.Fatal(err)
	}
	if a.retranslation != r || a.retranslationNext != nil || r.rows[0].result.text != "保留的结果" {
		t.Fatal("cancel lost last results")
	}
	a.openRetranslation()
	tt.Frame()
	if err := tt.Click(locales.RetranslateNewConfirm()); err != nil {
		t.Fatal(err)
	}
	if a.retranslation == r || a.retranslationNext != nil || len(a.retranslation.rows) != 1 {
		t.Fatal("explicit new batch not created")
	}
}

func TestRetranslationPendingDraftDisablesOldResults(t *testing.T) {
	a := checkedReviewApp(t)
	r := a.retranslation
	r.rows[0].result = &retranslationResult{text: "新译文", valid: true}
	a.editPending = true
	a.reopenRetranslation()
	if !r.open || !a.retranslationStale(r) {
		t.Fatal("uncommitted draft left old results writable")
	}
}

func TestRetranslationReplaceAllIsOneUndo(t *testing.T) {
	a := checkedReviewApp(t)
	r := a.retranslation
	r.rows[0].result = &retranslationResult{text: "新的第一条", valid: true}
	r.rows[1].result = &retranslationResult{text: "新的第二条", valid: true}
	before := *a.snapshot
	apply, err := a.retranslationAcceptWork(r, r.replacementEdits(false))(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	apply()
	if !r.rows[0].applied || !r.rows[1].applied || len(r.replacementEdits(false)) != 0 {
		t.Fatal("batch acceptance did not apply every valid result once")
	}
	a.undo(false)
	if !reflect.DeepEqual(before.Document, a.snapshot.Document) || !reflect.DeepEqual(before.SourceDocument, a.snapshot.SourceDocument) {
		t.Fatal("one undo did not restore the whole batch")
	}
}

func TestRetranslationAcceptKeepsComparisonAndRemainingResults(t *testing.T) {
	a := checkedReviewApp(t)
	r := a.retranslation
	r.rows[0].result = &retranslationResult{text: "新的第一条", valid: true}
	r.rows[1].result = &retranslationResult{text: "新的第二条", valid: true}
	before := *a.snapshot
	apply, err := a.retranslationAcceptWork(r, r.replacementEdits(true))(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	apply()
	if r.rows[0].cue.Text != "原有人工译文" || !r.rows[0].applied || a.retranslationStale(r) || len(r.replacementEdits(true)) != 0 {
		t.Fatal("accept erased comparison or kept an already applied edit")
	}
	edits := r.replacementEdits(false)
	if len(edits) != 1 || edits[0].ID != "b" {
		t.Fatal("replace all did not limit itself to remaining results", edits)
	}
	if a.snapshot.Document.Cues[1].Translated || a.snapshot.Document.Cues[0].Text != "新的第一条" {
		t.Fatal("single acceptance touched another cue")
	}
	a.undo(false)
	if !reflect.DeepEqual(before.Document, a.snapshot.Document) || !a.retranslationStale(r) {
		t.Fatal("undo did not restore old translation and invalidate old results")
	}
}
