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

// Copy through the actual read-only input: labels alone cannot prove long
// report text is accessible or that keyboard input cannot edit the report.
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

// Checks, not subtitle editing, own the default page and its working height.
// Pages, themes, text scales and long diagnostics must not resize the shell.
func TestTranslationReviewPrioritizesFindingsAndStableLayout(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		for _, scale := range []float32{1, 1.5, 2} {
			for _, size := range [][2]int{{900, 640}, {1800, 1200}} {
				t.Run(fmt.Sprintf("%s/%g/%d", language, scale, size[0]), func(t *testing.T) {
					inLocale(t, language, func() {
						a := bulkTranslationReviewApp(t)
						for i := range 196 {
							a.translationCandidate.Checks.Warnings = append(a.translationCandidate.Checks.Warnings, fmt.Sprintf("Subtitle %d exceeds the reading speed limit. Check its timing.", i+1))
						}
						a.translationReview = false
						tt := settled(func(c *ui.Context) {
							theme, _ := studioTheme(c.Theme(), false)
							c.SetTheme(&theme)
							a.translationView(c)
						}, int(float32(size[0])*scale), int(float32(size[1])*scale))
						tt.SetPreferences(ui.Preferences{TextScale: scale, ReduceMotion: true})
						for _, dark := range []bool{false, true} {
							tt.SetDark(dark)
							firstFrameSettled(t, tt, "open", func() { a.translationReview = true })
							insideRedesign(t, tt, locales.TranslationResults(), locales.ReviewFindingsList(), locales.ReviewSearchFindings(), locales.ReviewFindingDetail(), locales.ReviewChecks(), locales.ActionClose())
							if _, found := tt.Find(locales.ReviewSource()); found {
								t.Fatal("default check page still shows subtitle comparison")
							}
							list, _ := tt.Find(locales.ReviewFindingsList())
							detail, _ := tt.Find(locales.ReviewFindingDetail())
							if list.X+list.W > detail.X || list.H < detail.H*.7 {
								t.Fatal("findings do not fill the work area beside their details", list, detail)
							}
							first, last := a.translationIssueList.Visible()
							if last-first+1 < 6 {
								t.Fatal("too little working height for findings", first, last)
							}
							captureRedesign(t, tt, fmt.Sprintf("check-results-%s-%g-%d-dark%t", language, scale, size[0], dark))
							before, _ := tt.Find(locales.TranslationResults())
							closeButton, _ := tt.Find(locales.ActionClose())
							firstFrameSettled(t, tt, "reference", func() { a.translationReviewPage = 1 })
							insideRedesign(t, tt, locales.TranslationResults(), locales.A11yReviewList(), locales.ReviewSearch(), locales.ReviewCompare(), locales.ActionClose())
							insideRedesign(t, tt, locales.ReviewCompare(), locales.ReviewSource(), locales.ReviewTranslation())
							if err := tt.Click(locales.ListBottom()); err != nil {
								t.Fatal(err)
							}
							_, last = a.translationList.Visible()
							if last != 199 || a.translationSelected != 0 {
								t.Fatal("reference navigation missed bottom or changed selection", last, a.translationSelected)
							}
							tt.Move(0, 0) // Do not carry the reference navigation hover across pages.
							firstFrameSettled(t, tt, "long-finding", func() {
								a.translationReviewPage = 0
								a.translationCandidate.Checks.Errors[0] = strings.Repeat("Detailed check error.\n", 200)
							})
							after, _ := tt.Find(locales.TranslationResults())
							button, _ := tt.Find(locales.ActionClose())
							if before != after || closeButton != button {
								t.Fatal("page or long diagnostics resized the dialog", before, after)
							}
							a.translationCandidate.Checks.Errors[0] = "#1: protected markup changed"
							a.translationIssueList.ScrollTo(0, ui.Start)
							tt.Frame()
						}
					})
				})
			}
		}
	}
}

func TestTranslationReviewFindingsFilterAndReadOnlyDetails(t *testing.T) {
	a := bulkTranslationReviewApp(t)
	candidate := a.translationCandidate
	before := *candidate
	before.Checks.Errors = slices.Clone(candidate.Checks.Errors)
	a.position = 7.5
	tt := settled(a.translationView, 1800, 1200)
	if got := reviewFieldText(t, tt, locales.ReviewChecks()); got != candidate.Checks.Errors[0] {
		t.Fatal("errors were not prioritized on opening", got)
	}
	if err := tt.Click("#150: missing source text"); err != nil {
		t.Fatal(err)
	}
	if a.translationIssueSelected != 1 || a.translationSelected != 0 || a.position != 7.5 {
		t.Fatal("finding text was guessed to be a cue link")
	}
	if got := reviewFieldText(t, tt, locales.ReviewChecks()); got != candidate.Checks.Errors[1] {
		t.Fatal("detail does not match the selected issue", got)
	}
	tt.Type("do not edit diagnostics")
	if got := reviewFieldText(t, tt, locales.ReviewChecks()); got != candidate.Checks.Errors[1] || !reflect.DeepEqual(before, *candidate) {
		t.Fatal("read-only diagnostic was changed", got)
	}
	if err := tt.Click(locales.ReviewWarnings(2)); err != nil {
		t.Fatal(err)
	}
	if got := reviewFieldText(t, tt, locales.ReviewChecks()); got != "Timeline warning" {
		t.Fatal("warning filter left stale error details", got)
	}
	if err := tt.Click(locales.ReviewSearchFindings()); err != nil {
		t.Fatal(err)
	}
	tt.Type(" READING SPEED ")
	if !tt.HasText(locales.ReviewFindingsVisible(1, 4)) || reviewFieldText(t, tt, locales.ReviewChecks()) != "Reading speed warning" {
		t.Fatal("search did not include checker warnings")
	}
	a.translationIssueQuery = "absent"
	tt.Frame()
	if reviewFieldText(t, tt, locales.ReviewChecks()) != locales.ReviewNoMatches() {
		t.Fatal("empty filter left unrelated details visible")
	}
	if err := tt.Click(locales.ActionClose()); err != nil || a.translationReview {
		t.Fatal("close did not hide the results", err)
	}
	a.translationReview = true
	tt.Frame()
	if a.translationCandidate != candidate || a.translationIssueQuery != "absent" || a.translationCheckTab != reviewWarningFindings || a.translationIssueSelected != 3 {
		t.Fatal("reopening discarded check state")
	}
	// Warning-only reports must show a warning immediately; duplicate raw
	// messages remain separate findings with stable positions.
	candidate.Checks.Errors = nil
	candidate.Checks.Warnings = []string{"Timeline warning"}
	a.translationIssueQuery, a.translationCheckTab = "", reviewAllFindings
	tt.Frame()
	if got := candidate.reviewFindings(reviewAllFindings, "timeline"); len(got) != 2 || got[0].index == got[1].index || reviewFieldText(t, tt, locales.ReviewChecks()) != "Timeline warning" {
		t.Fatal("warning-only or duplicate findings lost", got)
	}
}

func TestTranslationReviewReferenceIsSecondaryAndIndependent(t *testing.T) {
	a := bulkTranslationReviewApp(t)
	candidate := a.translationCandidate
	candidate.Result.Document.Cues[149].Text = "查找译文"
	for query, want := range map[string][]int{"150": {149}, " SOURCE SENTENCE 150 ": {149}, "查找译文": {149}, "absent": {}} {
		if rows := candidate.reviewRows(query); !slices.Equal(rows, want) {
			t.Fatal(query, rows, want)
		}
	}
	a.translationIssueQuery = "protected"
	tt := settled(a.translationView, 1800, 1200)
	if err := tt.Click(locales.ReviewReference()); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(locales.ReviewSearch()); err != nil {
		t.Fatal(err)
	}
	tt.Type("150")
	if err := tt.Click("查找译文"); err != nil {
		t.Fatal(err)
	}
	a.translationQuery = "absent"
	tt.Frame()
	if a.translationSelected != 149 || reviewFieldText(t, tt, locales.ReviewTranslation()) != "查找译文" || !tt.HasText(locales.ReviewVisible(0, 200)) {
		t.Fatal("empty reference search discarded the comparison")
	}
	for _, field := range []struct{ label, text string }{{locales.ReviewSource(), "Source sentence 150"}, {locales.ReviewTranslation(), "查找译文"}} {
		if reviewFieldText(t, tt, field.label) != field.text {
			t.Fatal("wrong reference text", field.label)
		}
		tt.Type("do not edit subtitles")
		if reviewFieldText(t, tt, field.label) != field.text {
			t.Fatal("reference was editable", field.label)
		}
	}
	if err := tt.Click(locales.ReviewFindings()); err != nil {
		t.Fatal(err)
	}
	if a.translationIssueQuery != "protected" || reviewFieldText(t, tt, locales.ReviewChecks()) != candidate.Checks.Errors[0] || !tt.HasText(locales.ReviewFindingsVisible(1, 4)) {
		t.Fatal("reference browsing changed the independent report filter")
	}
}

func TestTranslationReviewLongTextStartsAtBeginningAndStaysCopyable(t *testing.T) {
	text := "First line\n" + strings.Repeat("Detailed diagnostic.\n", 200) + "Last line"
	start, end := -1, -1
	tt := settled(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			area := reviewReadOnlyText(c, "Long diagnostic", text)
			start, end = area.TextSelection()
		})
	}, 600, 300)
	if start != 0 || end != 0 {
		t.Fatal("new diagnostic opened at the end", start, end)
	}
	if got := reviewFieldText(t, tt, "Long diagnostic"); got != text {
		t.Fatal("long diagnostic could not be copied in full")
	}
	tt.Frame()
	if start == end {
		t.Fatal("reader reset the user's selection every frame")
	}
	text = "Next issue\n" + strings.Repeat("Next detail.\n", 200)
	tt.Frame()
	if start != 0 || end != 0 || reviewFieldText(t, tt, "Long diagnostic") != text {
		t.Fatal("changing issue did not reset to the beginning or retained old text")
	}
}

func TestTranslationReviewEmptyAndErrorOnlyStates(t *testing.T) {
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
			if reviewFieldText(t, tt, locales.ReviewChecks()) != "No valid subtitle output" {
				t.Fatal("error-only report lost its findings")
			}
			if err := tt.Click(locales.ReviewWarnings(0)); err != nil || reviewFieldText(t, tt, locales.ReviewChecks()) != locales.ReviewNoWarnings() {
				t.Fatal("empty warning category has no state", err)
			}
			a.translationCandidate.Checks.Errors = nil
			for filter, want := range map[int]string{reviewAllFindings: locales.ReviewNoIssues(), reviewErrorFindings: locales.ReviewNoErrors(), reviewWarningFindings: locales.ReviewNoWarnings()} {
				a.translationCheckTab = filter
				tt.Frame()
				if reviewFieldText(t, tt, locales.ReviewChecks()) != want {
					t.Fatal("empty findings left stale details", filter)
				}
			}
			if err := tt.Click(locales.ReviewReference()); err != nil || !tt.HasText(locales.ReviewNoSubtitle()) {
				t.Fatal("missing subtitles have no reference state", err)
			}
		})
	}
}
