package main

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

func bulkRetranslationApp(t *testing.T) *NativeApp {
	t.Helper()
	cues := make([]subtitle.Cue, 200)
	for i := range cues {
		cues[i] = subtitle.Cue{ID: fmt.Sprint(i), Text: fmt.Sprintf("Source sentence %03d", i+1), StartMs: int64(i * 4000), EndMs: int64((i + 1) * 4000)}
	}
	a := storeApp(t, cues...)
	a.checkAllSubtitles()
	a.openRetranslation()
	return a
}

// Review text is naturally sized within one scrolling reader. It must still
// be selectable in full, read-only, and reset to the next sentence's start.
func TestRetranslationReaderCopiesWholeTextWithoutEditingBatch(t *testing.T) {
	a := checkedReviewApp(t)
	r := a.retranslation
	long := "A new translation.\n" + strings.Repeat("Keep every word in the review.\n", 70) + "Final line."
	r.rows[0].result = &retranslationResult{text: long, valid: true}
	before := r.rows[0]
	tt := settled(a.retranslationView, 1000, 820)
	box, ok := tt.Find(long)
	if !ok || box.H <= 0 {
		t.Fatal("candidate has no readable area", box)
	}
	if err := tt.Click(long); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Key(ui.Cmd, ui.KeyC)
	if tt.Clipboard() != long {
		t.Fatal("candidate copy lost text", len(tt.Clipboard()), len(long))
	}
	tt.Type("must not change the translation")
	if !reflect.DeepEqual(r.rows[0], before) || r.rows[0].result.text != long || a.trackUnsaved(1) {
		t.Fatal("reader edited the candidate or saved subtitle")
	}
	reader, _ := tt.Find(locales.RetranslateCompare())
	tt.Scroll(reader.X+reader.W/2, reader.Y+reader.H/2, 0, 4000)
	if err := tt.Click("Second source"); err != nil {
		t.Fatal(err)
	}
	if r.selected != 1 || !tt.HasText(locales.RetranslateNoExisting()) {
		t.Fatal("directory did not select the next sentence")
	}
	text, found := tt.Find("Second source")
	if !found || text.H <= 0 {
		t.Fatal("next sentence retained old scroll position", text)
	}
}

func TestRetranslationLargeBatchUsesWorkingHeight(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		for _, scale := range []float32{1, 1.5, 2} {
			for _, size := range [][2]int{{900, 640}, {1800, 1200}} {
				t.Run(fmt.Sprintf("%s/%g/%d", language, scale, size[0]), func(t *testing.T) {
					inLocale(t, language, func() {
						a := bulkRetranslationApp(t)
						r := a.retranslation
						r.open = false
						tt := settled(a.retranslationView, int(float32(size[0])*scale), int(float32(size[1])*scale))
						tt.SetPreferences(ui.Preferences{TextScale: scale, ReduceMotion: true})
						firstFrameSettled(t, tt, "large-batch", func() { r.open = true })
						if tt.HasText(locales.RetranslatePending()) {
							t.Fatal("missing candidate must stay empty")
						}
						insideRedesign(t, tt, locales.RetranslateTitle(), locales.RetranslateList(), locales.RetranslateSearch(), locales.RetranslateCompare(), locales.RetranslateOne(), locales.RetranslateApplyOne())
						list, _ := tt.Find(locales.RetranslateList())
						compare, _ := tt.Find(locales.RetranslateCompare())
						if list.W < compare.W*.9 || list.H < compare.H*.7 {
							t.Fatal("retranslation list does not fill the review area", list, compare)
						}
						first, last := r.list.Visible()
						minimum := 2
						if size[0] > 1000 {
							minimum = 4
						}
						if last-first+1 < minimum {
							t.Fatalf("only %d cues visible, want at least %d", last-first+1, minimum)
						}
						before, _ := tt.Find(locales.RetranslateTitle())
						firstFrameSettled(t, tt, "running", func() { r.running = true; r.progress = locales.RetranslateProgress(1, 200) })
						after, _ := tt.Find(locales.RetranslateTitle())
						if before != after {
							t.Fatal("progress resized the dialog")
						}
						r.running = false
						tt.Frame()
						captureRedesign(t, tt, fmt.Sprintf("retranslation-bulk-%s-%g-%d", language, scale, size[0]))
						if err := tt.Click(locales.ListBottom()); err != nil {
							t.Fatal(err)
						}
						tt.Frame()
						_, last = r.list.Visible()
						if last != 199 || r.selected != 0 {
							t.Fatal("bottom navigation lost last row or changed selection", last, r.selected)
						}
					})
				})
			}
		}
	}
}

func TestRetranslationSearchKeepsBatchAndCanonicalSelection(t *testing.T) {
	a := bulkRetranslationApp(t)
	r := a.retranslation
	r.rows[149].result = &retranslationResult{text: "查找新译文", valid: true}
	ids := r.ids(false)
	for query, want := range map[string][]int{"150": {149}, " SOURCE SENTENCE 150 ": {149}, "查找新译文": {149}, "absent": {}} {
		r.query = query
		if rows := r.filteredRows(); !slices.Equal(rows, want) {
			t.Fatal(query, rows, want)
		}
		if !slices.Equal(ids, r.ids(false)) || len(r.replacementEdits(false)) != 1 {
			t.Fatal("search changed batch scope")
		}
	}
	r.query = "150"
	tt := settled(a.retranslationView, 1800, 1200)
	if err := tt.Click("Source sentence 150"); err != nil {
		t.Fatal(err)
	}
	if r.selected != 149 || !slices.Equal(r.ids(true), []string{"149"}) {
		t.Fatal("filtered click selected a different cue", r.selected)
	}
	r.query = "absent"
	tt.Frame()
	if r.selected != 149 {
		t.Fatal("empty search discarded the current comparison")
	}
	r.query = ""
	tt.Frame()
	if r.selected != 149 || !slices.Equal(ids, r.ids(false)) {
		t.Fatal("clearing search changed the selected cue or batch")
	}
}
