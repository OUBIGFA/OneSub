package main

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/styles"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

func TestCompositionRejectsDirectStartBeforeOpeningOutputPicker(t *testing.T) {
	for _, sample := range []bool{false, true} {
		a := storeApp(t, subtitle.Cue{ID: "bad", Text: "Text\U0004e7fd", EndMs: 5000})
		a.snapshot.Manifest.MediaPath = "source.mp4"
		a.settings.ComposeAskPath = true
		a.startCompose(sample)
		if a.busy || a.composeRunning || !a.composeOpen || !strings.Contains(a.errorText, "#1") {
			t.Fatal("direct composition start bypassed the subtitle gate", sample, a.busy, a.composeRunning, a.errorText)
		}
	}
}

// Original-track edits after translation deliberately have different IDs.
// Their saved ancestry must still match the existing translated material.
func TestCompositionAcceptsOriginalSplitAndMergeAfterTranslation(t *testing.T) {
	for _, operation := range []string{"split", "merge"} {
		t.Run(operation, func(t *testing.T) {
			a := storeApp(t,
				subtitle.Cue{ID: "a", Text: "Please check every single file. Then run the whole program.", EndMs: 12000},
				subtitle.Cue{ID: "b", Text: "Second source", StartMs: 12000, EndMs: 17000})
			revision, err := a.store.ApplyTranslation("translate", a.snapshot.Revision, []project.Edit{{ID: "a", Text: "请检查每个文件，再运行程序。", StartMs: 0, EndMs: 12000}, {ID: "b", Text: "第二条原文", StartMs: 12000, EndMs: 17000}})
			if err != nil {
				t.Fatal(err)
			}
			if operation == "split" {
				_, err = a.store.SplitCue(context.Background(), "split-original", revision, true, "a", subtitle.PunctuationOptions{})
			} else {
				_, err = a.store.MergeCues("merge-original", revision, true, "a", "b")
			}
			if err != nil {
				t.Fatal(err)
			}
			state, err := a.store.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			if issues := composeSubtitleIssues(state, subtitle.OutputOptions{Mode: "translation"}, nil); len(issues) > 0 {
				t.Fatal("valid original segmentation was reported as missing translation", composeSubtitleError(issues))
			}
		})
	}
}

// Every composition entry uses the same blocking policy, including soft
// subtitles, five-second samples and a stale output-skip preference.
func TestCompositionRefusesSeriousSubtitlesWithTheirNumbers(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		for _, problem := range []string{"missing-translation", "empty-source", "empty-original", "empty-original-proofreading", "characters", "glyphs"} {
			t.Run(language+"/"+problem, func(t *testing.T) {
				inLocale(t, language, func() {
					a := storeApp(t,
						subtitle.Cue{ID: "a", Text: "First source", StartMs: 0, EndMs: 5000},
						subtitle.Cue{ID: "b", Text: "Second source", StartMs: 6000, EndMs: 11000})
					state := *a.snapshot
					state.Document.Cues = slices.Clone(state.Document.Cues)
					for i := range state.Document.Cues {
						cue := &state.Document.Cues[i]
						cue.TranslationSource, cue.Text, cue.Translated = cue.Text, "译文", true
					}
					bad := &state.Document.Cues[1]
					switch problem {
					case "missing-translation":
						bad.Text, bad.Translated = bad.TranslationSource, false
					case "empty-source":
						bad.TranslationSource = ""
					case "empty-original", "empty-original-proofreading":
						state.SourceDocument.Cues = slices.Clone(state.SourceDocument.Cues)
						state.SourceDocument.Cues[1].Text = ""
						a.proofreading = problem == "empty-original-proofreading"
					case "characters":
						bad.Text += "\U0004e7fd"
					case "glyphs":
						bad.Text += "\U000f0000"
						a.problems.glyphs = []string{"\U000f0000"}
					}
					a.snapshot = &state
					a.snapshot.Manifest.MediaPath = "test.mp4"
					a.settings.SubtitleOutput.Mode, a.composeOpen = "translation", true
					a.settings.SubtitleOutput.SkipMissing = true
					tt := settled(a.composeView, 900, 640)
					if !strings.Contains(strings.Join(tt.Texts(), "\n"), "#2") {
						t.Fatal("composition error did not identify subtitle #2", tt.Texts())
					}
					if !tt.HasText(locales.SubtitleIssuesToggle()) || tt.HasText(locales.TranslationSkipMissing()) {
						t.Fatal("composition must offer correction without a skip bypass", tt.Texts())
					}
					for _, mode := range []string{"hard-mp4", "soft-mkv"} {
						a.settings.ComposeMode = mode
						tt.Frame()
						for _, label := range []string{locales.ComposeSample(), locales.ComposeStart()} {
							control, found := tt.Access(label)
							if !found || !control.Disabled {
								t.Fatal("invalid subtitles can still start composition", label, control, found)
							}
						}
					}
					if err := tt.Click(locales.SubtitleIssuesToggle()); err != nil {
						t.Fatal(err)
					}
					wantTrack := 1
					if strings.HasPrefix(problem, "empty-original") {
						wantTrack = 0
					}
					if a.composeOpen || a.subtitleTab != wantTrack || a.selected != 1 || !a.problems.filter {
						t.Fatal("correction did not open the precise subtitle", a.composeOpen, a.subtitleTab, a.selected)
					}
				})
			})
		}
	}
}

func TestCompositionDialogKeepsGeometryWhenContentRevealsIssues(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := storeApp(t, subtitle.Cue{ID: "a", Text: "A valid original without a translation yet", EndMs: 5000})
			a.composeOpen = true
			tt := settled(func(c *ui.Context) { applyStudioTheme(c); a.composeView(c) }, 1360, 768)
			before, _ := tt.Find(locales.ActionCompose())
			action, _ := tt.Find(locales.ComposeStart())
			for _, mode := range []string{"translation", "bilingual", "source"} {
				a.settings.SubtitleOutput.Mode = mode
				firstFrameSettled(t, tt, "composition-content", func() {})
				after, _ := tt.Find(locales.ActionCompose())
				start, _ := tt.Find(locales.ComposeStart())
				if after != before || start != action || after.Y < 0 || after.Y+after.H > 768 {
					t.Fatal("content choice moved or clipped the composition form", mode, before, after, action, start)
				}
			}
		})
	}
}

func TestCompositionDetectsGlyphsFromFreshStyleReport(t *testing.T) {
	a := withStyleCatalog(t)
	a.styleClient = styles.Client{Executable: os.Getenv("ONESUB_STYLE_WORKER"), Scratch: t.TempDir()}
	state := project.Snapshot{Document: subtitle.Document{Format: "srt", Cues: []subtitle.Cue{{ID: "private", Text: "Private use \U000f0000", EndMs: 5000}}}}
	options := subtitle.OutputOptions{Mode: "source"}
	if issues := composeSubtitleIssues(state, options, nil); len(issues) > 0 {
		t.Fatal("valid private-use text was guessed to be an abnormal character", issues)
	}
	document, err := a.styledDocument(context.Background(), state, a.currentStyle, a.currentStyle, options, 16.0/9)
	if err != nil {
		t.Fatal(err)
	}
	issues := composeSubtitleIssues(state, options, document.Report.MissingGlyphs)
	if len(issues) != 1 || !strings.Contains(composeSubtitleError(issues).Error(), "U+F0000") {
		t.Fatal("fresh font resolution did not identify the unrenderable subtitle", document.Report, issues)
	}
}

func TestCompositionKeepsAdvisoryWarningsNonblocking(t *testing.T) {
	a := storeApp(t, subtitle.Cue{ID: "long", Text: strings.Repeat("Long readable subtitle. ", 8), StartMs: 0, EndMs: 400})
	a.composeOpen = true
	tt := settled(a.composeView, 1200, 900)
	for _, label := range []string{locales.ComposeSample(), locales.ComposeStart()} {
		control, found := tt.Access(label)
		if !found || control.Disabled {
			t.Fatal("reading advice blocked composition", label, control, found)
		}
	}
}

// The optimized track may combine or split source cues. Provenance, not equal
// counts or row numbers, determines whether an original has no corresponding cue.
func TestCompositionChecksOriginalCoverageWithoutComparingRowCounts(t *testing.T) {
	a := storeApp(t,
		subtitle.Cue{ID: "a", Text: "First", StartMs: 0, EndMs: 4000},
		subtitle.Cue{ID: "b", Text: "Second", StartMs: 4000, EndMs: 8000})
	state := *a.snapshot
	state.Document.Cues = []subtitle.Cue{{ID: "merged", Position: 0, Text: "合并译文", TranslationSource: "First Second", Translated: true, SourceCueIDs: []string{"a", "b"}, StartMs: 0, EndMs: 8000}}
	a.snapshot, a.composeOpen = &state, true
	a.settings.SubtitleOutput.Mode = "translation"
	tt := settled(a.composeView, 1200, 900)
	control, found := tt.Access(locales.ComposeStart())
	if !found || control.Disabled {
		t.Fatal("valid merged translation was rejected because counts differ", control, found)
	}
	// Retain the first original only; the second source is now unrepresented.
	state.Document.Cues[0].SourceCueIDs = []string{"a"}
	a.snapshot = &project.Snapshot{Manifest: state.Manifest, Document: state.Document, SourceDocument: state.SourceDocument, Revision: state.Revision + 1}
	tt.Frame()
	control, found = tt.Access(locales.ComposeStart())
	if !found || !control.Disabled || !strings.Contains(strings.Join(tt.Texts(), "\n"), "#2") {
		t.Fatal("unrepresented original subtitle #2 was not reported", fmt.Sprint(control), tt.Texts(), found)
	}
}
