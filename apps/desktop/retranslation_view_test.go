package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/translator"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

func checkedReviewApp(t *testing.T) *NativeApp {
	t.Helper()
	a := storeApp(t, subtitle.Cue{ID: "a", Text: "First source", StartMs: 0, EndMs: 4000}, subtitle.Cue{ID: "b", Text: "Second source", StartMs: 5000, EndMs: 9000})
	if _, err := a.store.Apply("manual", a.snapshot.Revision, []project.Edit{{ID: "a", Track: "translation", Text: "原有人工译文", StartMs: 0, EndMs: 4000}}); err != nil {
		t.Fatal(err)
	}
	state, _ := a.store.Snapshot()
	a.setSnapshot(state)
	a.markSaved(1, "translated.srt")
	a.setSubtitleChecked("a", true)
	a.setSubtitleChecked("b", true)
	a.openRetranslation()
	return a
}

func TestRetranslationChecksDoNotSelectOrSeekAndSurviveFiltering(t *testing.T) {
	a := checkedReviewApp(t)
	a.retranslation = nil
	a.subtitleChecked[0] = nil
	a.mediaURL = "test-video"
	a.settings.SingleClickAction = "seek-play"
	selected, position := a.selected, a.position
	tt := settled(a.subtitlesView, 850, 650)
	if _, ok := tt.Find(locales.RetranslateCheckCue(1)); ok {
		t.Fatal("unchecked list shows checkboxes")
	}
	if err := tt.Click(locales.RetranslateSelect()); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(locales.RetranslateCheckCue(2)); err != nil {
		t.Fatal(err)
	}
	if a.selected != selected || a.position != position || !a.paused || a.pendingClick != nil || len(a.checkedRetranslationIDs()) != 2 {
		t.Fatal("checkbox selected/played a cue", a.selected, a.pendingClick, a.checkedRetranslationIDs())
	}
	a.query = "First"
	tt.Frame()
	if !slices.Equal(a.checkedRetranslationIDs(), []string{"a", "b"}) {
		t.Fatal("search lost checks")
	}
	if err := tt.Click(locales.RetranslateSelected(2)); err != nil {
		t.Fatal(err)
	}
	if a.retranslation == nil || len(a.retranslation.rows) != 2 {
		t.Fatal("review did not include hidden checked cue")
	}
}

// Manual subtitle selection uses the whole row as a target. It must remain
// independent from playback and the editing selection.
func TestSubtitleSelectionTogglesFromRowBody(t *testing.T) {
	a := storeApp(t, subtitle.Cue{ID: "a", Text: "First source", StartMs: 0, EndMs: 4000}, subtitle.Cue{ID: "b", Text: "Second source", StartMs: 5000, EndMs: 9000})
	a.subtitleChecked[0] = nil
	selected, position := a.selected, a.position
	tt := settled(a.subtitlesView, 900, 640)
	if err := tt.Click(locales.RetranslateSelect()); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Second source"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a.checkedRetranslationIDs(), []string{"a", "b"}) || a.selected != selected || a.position != position || a.pendingClick != nil {
		t.Fatal("row selection changed playback or the editing selection", a.checkedRetranslationIDs(), a.selected, a.position)
	}
	if err := tt.Click("First source"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a.checkedRetranslationIDs(), []string{"b"}) {
		t.Fatal("clicking a selected row did not clear its check", a.checkedRetranslationIDs())
	}
}
func TestRetranslationAcceptSubsetLocksUndoAndStaleProtection(t *testing.T) {
	a := checkedReviewApp(t)
	r := a.retranslation
	before := *a.snapshot
	r.rows[0].result = &retranslationResult{text: "新的译文", valid: true}
	r.rows[1].result = &retranslationResult{text: "另一条译文", valid: true}
	edits := r.replacementEdits(true)
	if len(edits) != 1 || edits[0].Text != "新的译文" {
		t.Fatal(edits)
	}
	// This is the same frozen transaction used by the dialog, while the main
	// list still displays original subtitles. It must write only translations.
	work := a.listTextEditWorkFor(r.source, r.revision, edits)
	apply, err := work(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	apply()
	if a.snapshot.Document.Cues[0].Text != "新的译文" || !a.snapshot.Document.Cues[0].TextLocked || !a.trackUnsaved(1) || a.savedPath[1] != "translated.srt" {
		t.Fatal("replacement lost lock or saved file implicitly", a.snapshot)
	}
	if !reflect.DeepEqual(a.snapshot.SourceDocument, before.SourceDocument) || !reflect.DeepEqual(a.snapshot.Document.Cues[1], before.Document.Cues[1]) {
		t.Fatal("replacement changed original or unselected cue")
	}
	if !a.retranslationStale(r) {
		t.Fatal("old candidates not marked stale")
	}
	if _, err := work(context.Background()); err == nil {
		t.Fatal("stale work overwrote a later revision")
	}
	a.undo(false)
	if !reflect.DeepEqual(a.snapshot.Document, before.Document) || !reflect.DeepEqual(a.snapshot.SourceDocument, before.SourceDocument) || a.trackUnsaved(1) {
		t.Fatal("one undo failed to restore content and locks")
	}
	r.rows[0].result.valid = false
	if len(r.replacementEdits(true)) != 0 {
		t.Fatal("invalid result accepted")
	}
}

func TestRetranslationOriginalMappingDoesNotGuessAfterDivergence(t *testing.T) {
	a := checkedReviewApp(t)
	if len(a.checkedRetranslationIDs()) != 2 {
		t.Fatal("original same-ID mapping unavailable")
	}
	cue := a.snapshot.Document.Cues[0]
	if _, err := a.store.Apply("correct-working", a.snapshot.Revision, []project.Edit{{ID: cue.ID, Track: "source", Text: "Changed working source", StartMs: cue.StartMs, EndMs: cue.EndMs}}); err != nil {
		t.Fatal(err)
	}
	state, _ := a.store.Snapshot()
	a.setSnapshot(state)
	if slices.Contains(a.checkedRetranslationIDs(), "a") {
		t.Fatal("guessed divergent original mapping")
	}
	a.changeSubtitleTab(1)
	a.setSubtitleChecked("a", true)
	a.openRetranslation()
	if a.retranslation.rows[0].cue.TranslationSource != "Changed working source" {
		t.Fatal("did not use edited source")
	}
	if _, err := a.store.DeleteCue("delete", a.snapshot.Revision, false, "a"); err != nil {
		t.Fatal(err)
	}
	state, _ = a.store.Snapshot()
	a.setSnapshot(state)
	if a.subtitleChecked[1]["a"] {
		t.Fatal("deleted cue kept its check")
	}
}

func TestRetranslationWorkRetainsCandidatesAcrossRetryFailureAndCancel(t *testing.T) {
	a := checkedReviewApp(t)
	a.worker = subtitle.Client{Executable: os.Getenv("ONESUB_SUBTITLE_WORKER"), Scratch: t.TempDir()}
	r := a.retranslation
	before := *a.snapshot
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests++
		if requests == 3 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body struct{ Messages []struct{ Content string } }
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		var payload struct{ Units []struct{ ID, Text string } }
		if err := json.Unmarshal([]byte(body.Messages[1].Content), &payload); err != nil {
			t.Error(err)
			return
		}
		if len(payload.Units) != 1 || payload.Units[0].Text != "First source" {
			t.Error("wrong selected source", payload)
		}
		answer := map[string]string{payload.Units[0].ID: fmt.Sprintf("候选翻译第%d版", requests)}
		text, _ := json.Marshal(answer)
		chunk, _ := json.Marshal(string(text))
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%s}}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", chunk)
	}))
	defer server.Close()
	config := translator.Config{Endpoint: server.URL, Model: "protocol-test", TargetLanguage: "zh-CN", SkillRoot: os.Getenv("ONESUB_SKILL_ROOT"), Client: server.Client()}
	for range 2 {
		apply, err := a.retranslationWork(r, []string{"a"}, config)(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		apply()
	}
	if requests != 2 || r.rows[0].result == nil || r.rows[1].result != nil || !r.rows[0].result.valid || r.rows[0].result.text != "候选翻译第 2 版" {
		t.Fatalf("wrong latest result: %+v, requests=%d", r.rows[0].result, requests)
	}
	if _, err := a.retranslationWork(r, []string{"a"}, config)(context.Background()); err == nil {
		t.Fatal("authentication failure accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.retranslationWork(r, []string{"a"}, config)(ctx); err == nil {
		t.Fatal("cancelled attempt accepted")
	}
	if r.rows[0].result.text != "候选翻译第 2 版" {
		t.Fatal("failure erased prior result")
	}
	state, _ := a.store.Snapshot()
	if !reflect.DeepEqual(state, before) || a.trackUnsaved(1) {
		t.Fatal("generating candidates changed project or save baseline")
	}
}

func TestRetranslationWaitsForWholeFileRecovery(t *testing.T) {
	a := checkedReviewApp(t)
	a.retranslation = nil
	a.translationLive = newTranslationLive()
	a.openRetranslation()
	if a.retranslation != nil {
		t.Fatal("selected translation interfered with whole-file recovery")
	}
}

func TestRetranslationDialogLayoutAndReopen(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		for _, scale := range []float32{1, 1.5, 2} {
			for _, dark := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%g/dark%t", language, scale, dark), func(t *testing.T) {
					inLocale(t, language, func() {
						a := checkedReviewApp(t)
						r := a.retranslation
						r.open = false
						// Use the application root so the production Lobe theme also
						// participates in dark-mode and text-scale layout checks.
						tt := settled(a.view, int(1000*scale), int(820*scale))
						tt.SetPreferences(ui.Preferences{TextScale: scale, ReduceMotion: true})
						tt.SetDark(dark)
						firstFrameSettled(t, tt, "retranslation", func() { r.open = true })
						insideRedesign(t, tt, locales.RetranslateTitle(), locales.RetranslateList(), locales.RetranslateExisting(), locales.RetranslateCandidate(), locales.RetranslateAgain(), locales.RetranslateApply())
						before, _ := tt.Find(locales.RetranslateTitle())
						firstFrameSettled(t, tt, "result", func() {
							r.rows[0].result = &retranslationResult{text: strings.Repeat("新译文包含完整的长译文。", 80), valid: true}
						})
						after, _ := tt.Find(locales.RetranslateTitle())
						if before != after {
							t.Fatal("result resized review dialog", before, after)
						}
						captureRedesign(t, tt, fmt.Sprintf("retranslation-%s-%g-%t", language, scale, dark))
						if err := tt.Click(locales.ActionClose()); err != nil {
							t.Fatal(err)
						}
						if r.open || a.retranslation != r || r.rows[0].result == nil {
							t.Fatal("closing discarded the results")
						}
						tt.Move(0, 0) // Do not carry the Close button's hover into the new modal.
						firstFrameSettled(t, tt, "reopen", a.reopenRetranslation)
						insideRedesign(t, tt, locales.RetranslateTitle(), locales.RetranslateOne(), locales.RetranslateApplyOne(), locales.RetranslateCandidate())
						compact := settled(a.view, int(900*scale), int(640*scale))
						compact.SetPreferences(ui.Preferences{TextScale: scale, ReduceMotion: true})
						compact.SetDark(dark)
						compact.Frame()
						insideRedesign(t, compact, locales.RetranslateTitle(), locales.RetranslateCompare(), locales.RetranslateRetry(), locales.RetranslateApply())
						if _, ok := compact.Find(locales.RetranslateChecks()); ok {
							t.Fatal("review without diagnostics keeps a redundant status region")
						}
						apply, _ := compact.Find(locales.RetranslateApply())
						r.errorText = strings.Repeat("Translation service unavailable. ", 30)
						compact.Frame()
						insideRedesign(t, compact, locales.RetranslateTitle(), locales.RetranslateChecks())
						captureRedesign(t, compact, fmt.Sprintf("retranslation-error-%s-%g-%t", language, scale, dark))
						if next, _ := compact.Find(locales.RetranslateApply()); next != apply {
							t.Fatal("diagnostic moved the dialog actions", apply, next)
						}
						r.errorText = ""
						compact.Frame()
						comparison, ok := compact.Find(locales.RetranslateCompare())
						if !ok || comparison.H < 75*scale {
							t.Fatal("minimum window lost readable comparison", comparison)
						}
						captureRedesign(t, compact, fmt.Sprintf("retranslation-min-%s-%g-%t", language, scale, dark))
					})
				})
			}
		}
	}
}
