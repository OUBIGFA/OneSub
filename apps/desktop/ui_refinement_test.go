package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

func TestSearchShowsCurrentTrackRange(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := nativeForTest(t)
			a.snapshot = &project.Snapshot{SourceDocument: subtitle.Document{Cues: navigationCues(361)}, Document: subtitle.Document{Cues: navigationCues(200)}}
			for _, track := range []int{0, 1} {
				a.changeSubtitleTab(track)
				want := locales.ListSearchRange(1, len(a.activeDocument().Cues))
				if got := subtitleSearchLabel(a.activeDocument().Cues); got != want {
					t.Fatal(got, want)
				}
				tt := settled(a.subtitlesView, 700, 500)
				captureRedesign(t, tt, fmt.Sprintf("search-%s-track%d", language, track))
				if err := tt.Click(locales.ListSearch()); err != nil {
					t.Fatal(err)
				}
				tt.Type("Subtitle 9")
				if a.query != "Subtitle 9" || !tt.HasText("Subtitle 9") {
					t.Fatal("search stopped filtering")
				}
				a.query = ""
			}
			if got := subtitleSearchLabel(nil); got != locales.ListSearchRange(0, 0) {
				t.Fatal("empty range", got)
			}
			if got := subtitleSearchLabel(navigationCues(20000)); strings.Contains(got, "20,000") {
				t.Fatal("subtitle IDs were grouped", got)
			}
		})
	}
}

// Only actual optimization provenance controls output labels. A different next
// task option, a corrected source, a reopen or undo cannot relabel old results.
func TestTranslationLabelsFollowPersistedProvenance(t *testing.T) {
	a := storeApp(t,
		subtitle.Cue{ID: "a", Text: "Hello", OriginalText: "Hello", StartMs: 0, EndMs: 1000},
		subtitle.Cue{ID: "b", Position: 1, Text: "world", OriginalText: "world", StartMs: 1000, EndMs: 2000})
	original, optimized := "译文字幕（原始）", "译文字幕（优化）"
	if a.outputModes()[2].Label != original {
		t.Fatal(a.outputModes())
	}
	candidate, _, err := subtitle.Resegment(a.snapshot.Document, []subtitle.SourceRange{{FirstAtom: "a", LastAtom: "b", Reason: "sense group"}})
	if err != nil {
		t.Fatal(err)
	}
	candidate.Cues[0].TranslationSource = candidate.Cues[0].Text
	candidate.Cues[0].Text, candidate.Cues[0].Translated = "你好世界", true
	if _, err = a.store.ApplyTranslationDocument("optimize", a.snapshot.Revision, candidate); err != nil {
		t.Fatal(err)
	}
	state, err := a.store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	a.setSnapshot(state)
	a.settings.OptimizeTimeline = false
	if a.outputModes()[2].Label != optimized || !strings.Contains(a.unsavedText(), optimized) {
		t.Fatal("setting relabeled stored translation", a.outputModes())
	}
	cue := candidate.Cues[0]
	if _, err = a.store.Apply("source-edit", a.snapshot.Revision, []project.Edit{{ID: cue.ID, Track: "source", Text: "Hello, world", StartMs: cue.StartMs, EndMs: cue.EndMs}}); err != nil {
		t.Fatal(err)
	}
	path := a.snapshot.Path
	a.store.Close()
	a.store, err = project.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.store.Close()
	state, err = a.store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	a.setSnapshot(state)
	if a.outputModes()[2].Label != optimized {
		t.Fatal("source edit or reopen lost optimization provenance")
	}
	a.undo(false)
	a.undo(false)
	if a.outputModes()[2].Label != original {
		t.Fatal("undo kept stale optimized label")
	}
	a.undo(true)
	if a.outputModes()[2].Label != optimized {
		t.Fatal("redo lost optimized label")
	}
	inLocale(t, "en", func() {
		if got := a.outputModes()[2].Label; got != "Translated subtitles (optimized)" {
			t.Fatal(got)
		}
	})
}

func TestLongProviderFieldsRemainEditable(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := nativeForTest(t)
			a.addProvider()
			a.settingsOpen, a.settingsTab = true, 2
			address := "https://example.com/" + strings.Repeat("long-segment/", 20) + "v1"
			a.settings.ProviderURL = address
			a.settings.ProviderModel = strings.Repeat("model-", 30)
			tt := settled(a.view, 900, 640)
			captureRedesign(t, tt, "long-inputs-"+language)
			if err := tt.Click(locales.TranslationEndpoint()); err != nil {
				t.Fatal(err)
			}
			tt.Key(0, ui.KeyEnd)
			tt.Type("/edited")
			if a.settings.ProviderURL != address+"/edited" {
				t.Fatal("hidden endpoint text was lost", a.settings.ProviderURL)
			}
			if err := tt.Click(locales.TranslationModel()); err != nil {
				t.Fatal(err)
			}
			tt.Key(0, ui.KeyEnd)
			tt.Type("tail")
			prefs, err := settings.Load(filepath.Join(a.dataRoot, "settings.json"))
			if err != nil || prefs.ProviderURL != address+"/edited" || prefs.ProviderModel != strings.Repeat("model-", 30)+"tail" {
				t.Fatal("full fields were not saved", err)
			}
		})
	}
}

func TestTimelineFeatureSwitchIsProminentAndNative(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		for _, width := range []int{320, 500, 900} {
			for _, scale := range []float32{1, 1.5, 2} {
				t.Run(fmt.Sprintf("%s/%d/%g", language, width, scale), func(t *testing.T) {
					inLocale(t, language, func() {
						a := nativeForTest(t)
						a.workflowTab = 1
						w := int(float32(width) * scale)
						tt := settled(a.workflowPanel, w, int(850*scale))
						tt.SetPreferences(ui.Preferences{TextScale: scale})
						feature, ok := tt.Find(locales.TranslationOptimizeTimeline())
						start, _ := tt.Find(locales.WorkflowStartTranslate())
						service, _ := tt.Find(locales.WorkflowAIService())
						if !ok || feature.X < 0 || feature.X+feature.W > float32(w) || feature.Y <= start.Y || feature.Y+feature.H >= service.Y {
							t.Fatal("feature is not prominently before secondary parameters", feature, start, service)
						}
						// The output track name is visible beside the choice, in either
						// state, rather than hidden in a tooltip or changed by a toggle.
						hintLabel := locales.TranslationOptimizeTimelineHint(locales.TabRebuilt())
						hint, found := tt.Find(hintLabel)
						if !found || hint.W <= 0 || hint.H <= 0 || hint.X < 0 || hint.X+hint.W > float32(w) || hint.Y < feature.Y+feature.H || hint.Y+hint.H >= service.Y {
							t.Fatal("result explanation is missing or clipped", hint, feature, service)
						}
						before := a.settings.OptimizeTimeline
						if err := tt.Click(locales.TranslationOptimizeTimeline()); err != nil {
							t.Fatal(err)
						}
						if a.settings.OptimizeTimeline == before || !tt.HasText(hintLabel) {
							t.Fatal("feature label did not toggle or lost the result explanation")
						}
						tt.Key(0, ui.KeySpace)
						if a.settings.OptimizeTimeline != before {
							t.Fatal("native switch keyboard action failed")
						}
						prefs, err := settings.Load(filepath.Join(a.dataRoot, "settings.json"))
						if err != nil || prefs.OptimizeTimeline != before {
							t.Fatal("switch not remembered", err)
						}
						a.busy = true
						tt.Frame()
						if err := tt.Click(locales.TranslationOptimizeTimeline()); err != nil {
							t.Fatal(err)
						}
						if a.settings.OptimizeTimeline != before {
							t.Fatal("disabled feature changed the active task")
						}
						a.busy = false
						tt.Advance(time.Second)
						if scale == 1 {
							captureRedesign(t, tt, fmt.Sprintf("timeline-feature-%s-%d", language, width))
							tt.SetDark(true)
							captureRedesign(t, tt, fmt.Sprintf("timeline-feature-%s-%d-dark", language, width))
						}
					})
				})
			}
		}
	}
}
