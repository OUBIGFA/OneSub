package main

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/translator"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// Copy through the actual read-only text input. Tester.Texts reports labels,
// not editable values, and cannot prove long report text remains accessible.
func reviewFieldText(t *testing.T, tt *ui.Tester, label string) string {
	t.Helper()
	if err := tt.Click(label); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Key(ui.Cmd, ui.KeyC)
	return tt.Clipboard()
}

func bulkTranslationReviewApp(t *testing.T) *NativeApp {
	t.Helper()
	a := nativeForTest(t)
	cues := make([]subtitle.Cue, 200)
	for i := range cues {
		cues[i] = subtitle.Cue{ID: fmt.Sprint(i), Position: i, Text: fmt.Sprintf("Translated cue %03d", i+1), TranslationSource: fmt.Sprintf("Source sentence %03d", i+1), Translated: true, StartMs: int64(i * 4000), EndMs: int64((i + 1) * 4000)}
	}
	a.translationCandidate = &translationCandidate{
		Result: translator.Result{Document: subtitle.Document{Format: "srt", Cues: cues}, Report: translator.Report{Translated: len(cues), TimelineChanges: 4, Warnings: []string{"Timeline warning"}}},
		Checks: translator.Findings{Errors: []string{"#1: protected markup changed", "#150: missing source text"}, Warnings: []string{"Reading speed warning"}},
	}
	a.translationReview = true
	return a
}

// The full-height list and three reading panes use the same fixed shell as
// retranslation. Long text, check categories and search cannot resize it.
func TestTranslationReviewUsesWorkingHeightAndStableLayout(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		for _, scale := range []float32{1, 1.5, 2} {
			for _, size := range [][2]int{{900, 640}, {1800, 1200}} {
				t.Run(fmt.Sprintf("%s/%g/%d", language, scale, size[0]), func(t *testing.T) {
					inLocale(t, language, func() {
						a := bulkTranslationReviewApp(t)
						a.translationReview = false
						tt := settled(a.translationView, int(float32(size[0])*scale), int(float32(size[1])*scale))
						tt.SetPreferences(ui.Preferences{TextScale: scale, ReduceMotion: true})
						for _, dark := range []bool{false, true} {
							tt.SetDark(dark)
							firstFrameSettled(t, tt, "open", func() { a.translationReview = true })
							insideRedesign(t, tt, locales.TranslationResults(), locales.A11yReviewList(), locales.ReviewSearch(), locales.ReviewCompare(), locales.ReviewChecks(), locales.ActionClose())
							insideRedesign(t, tt, locales.ReviewCompare(), locales.ReviewSource(), locales.ReviewTranslation(), locales.ReviewChecks())
							list, _ := tt.Find(locales.A11yReviewList())
							compare, _ := tt.Find(locales.ReviewCompare())
							if list.X+list.W > compare.X || list.H < compare.H*.7 {
								t.Fatal("review list is not full-height beside comparison", list, compare)
							}
							first, last := a.translationList.Visible()
							minimum := 6
							if size[0] > 1000 {
								minimum = 20
							}
							if last-first+1 < minimum {
								t.Fatal("review list still shows too few cues", last-first+1, minimum)
							}
							before, _ := tt.Find(locales.TranslationResults())
							firstFrameSettled(t, tt, "long-content", func() {
								a.translationCandidate.Result.Document.Cues[0].Text = strings.Repeat("Long translation.\n", 200)
								a.translationCandidate.Result.Document.Cues[0].TranslationSource = strings.Repeat("Long source.\n", 200)
								a.translationCandidate.Checks.Warnings = []string{strings.Repeat("Detailed warning.\n", 200)}
								a.translationCheckTab = 1
							})
							after, _ := tt.Find(locales.TranslationResults())
							if before != after {
								t.Fatal("long content resized the review dialog", before, after)
							}
							captureRedesign(t, tt, fmt.Sprintf("translation-review-%s-%g-%d-dark%t", language, scale, size[0], dark))
							if err := tt.Click(locales.ListBottom()); err != nil {
								t.Fatal(err)
							}
							_, last = a.translationList.Visible()
							if last != 199 || a.translationSelected != 0 {
								t.Fatal("review bottom navigation changed selection or missed the last cue", last, a.translationSelected)
							}
							a.translationList.ScrollTo(0, ui.Start)
							tt.Frame()
						}
					})
				})
			}
		}
	}
}

func TestTranslationReviewSearchKeepsSelectionAndWholeReport(t *testing.T) {
	a := bulkTranslationReviewApp(t)
	candidate := a.translationCandidate
	candidate.Result.Document.Cues[149].Text = "查找译文"
	for query, want := range map[string][]int{"150": {149}, " SOURCE SENTENCE 150 ": {149}, "查找译文": {149}, "absent": {}} {
		if rows := candidate.reviewRows(query); !slices.Equal(rows, want) {
			t.Fatal(query, rows, want)
		}
	}
	before := *candidate
	before.Result.Document.Cues = append([]subtitle.Cue{}, candidate.Result.Document.Cues...)
	before.Result.Report.Warnings = append([]string{}, candidate.Result.Report.Warnings...)
	before.Checks.Errors = append([]string{}, candidate.Checks.Errors...)
	before.Checks.Warnings = append([]string{}, candidate.Checks.Warnings...)
	a.position = 7.5
	tt := settled(a.translationView, 1800, 1200)
	if err := tt.Click(locales.ReviewSearch()); err != nil {
		t.Fatal(err)
	}
	tt.Type("150")
	if err := tt.Click("查找译文"); err != nil {
		t.Fatal(err)
	}
	checks := reviewFieldText(t, tt, locales.ReviewChecks())
	if a.translationSelected != 149 || a.position != 7.5 || !strings.Contains(checks, "#150: missing source text") || !strings.Contains(checks, "#1: protected markup changed") {
		t.Fatal("search changed report scope or selected the wrong cue", a.translationSelected)
	}
	if err := tt.Click(locales.ReviewWarnings(2)); err != nil {
		t.Fatal(err)
	}
	warnings := reviewFieldText(t, tt, locales.ReviewChecks())
	for _, message := range []string{"Timeline warning", "Reading speed warning"} {
		if !strings.Contains(warnings, message) {
			t.Fatal("warning category discarded a report source", message)
		}
	}
	a.translationQuery = "absent"
	tt.Frame()
	if a.translationSelected != 149 || reviewFieldText(t, tt, locales.ReviewTranslation()) != "查找译文" || !tt.HasText(locales.ReviewVisible(0, 200)) {
		t.Fatal("empty search discarded the current comparison")
	}
	// Both reading panes use real read-only text inputs, not editable copies.
	if err := tt.Click(locales.ReviewSource()); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("must not modify the source")
	if !reflect.DeepEqual(before, *candidate) || reviewFieldText(t, tt, locales.ReviewSource()) != "Source sentence 150" {
		t.Fatal("review changed the frozen translation")
	}
	if err := tt.Click(locales.ActionClose()); err != nil || a.translationReview {
		t.Fatal("close did not hide the review", err)
	}
	a.translationReview = true
	tt.Frame()
	if a.translationCandidate != candidate || a.translationSelected != 149 || a.translationQuery != "absent" || a.translationCheckTab != 1 {
		t.Fatal("reopening discarded the review state")
	}
}

func TestTranslationReviewShowsEmptyStatesAndErrorsWithoutCues(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := nativeForTest(t)
			a.translationReview = true
			tt := settled(a.translationView, 900, 640)
			if !tt.HasText(locales.ReviewEmpty()) {
				t.Fatal("missing report has no empty state")
			}
			a.translationCandidate = &translationCandidate{Checks: translator.Findings{Errors: []string{"No valid subtitle output"}}}
			tt.Frame()
			if reviewFieldText(t, tt, locales.ReviewChecks()) != "No valid subtitle output" || !tt.HasText(locales.ReviewNoSubtitle()) {
				t.Fatal("error-only report lost its findings")
			}
			if err := tt.Click(locales.ReviewWarnings(0)); err != nil || reviewFieldText(t, tt, locales.ReviewChecks()) != locales.ReviewNoWarnings() {
				t.Fatal("empty warnings have no clear state", err)
			}
			a.translationCandidate.Checks.Errors = nil
			a.translationCheckTab = 0
			tt.Frame()
			if reviewFieldText(t, tt, locales.ReviewChecks()) != locales.ReviewNoErrors() {
				t.Fatal("cleared findings left stale error text")
			}
		})
	}
}
