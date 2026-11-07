package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/translator"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// Render the complete production UI, not isolated approximations. These
// optional review images cover the same information hierarchy across all
// workbench panels and dialog families; actual media playback is tested on
// the desktop separately.
func TestWholeStudioPresentation(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := withStyleCatalog(t)
			a.addProvider()
			a.settings.Provider().Name = "Studio"
			a.settings.ProviderURL = "https://example.com/v1"
			for i := 0; i < 30; i++ {
				a.settings.Provider().Catalog = append(a.settings.Provider().Catalog, translator.ModelInfo{ID: fmt.Sprintf("studio-model-%02d", i), MaxInputTokens: 96000, MaxOutputTokens: 8192})
			}
			a.useProviderModel("studio-model-00")
			a.keepProviderModel("studio-model-01")
			a.snapshot.Manifest.MediaPath = "subtitle-workshop.mp4"
			a.snapshot.Document.Cues = navigationCues(24)
			for i := range a.snapshot.Document.Cues {
				a.snapshot.Document.Cues[i].Text = "Review the timing, then adjust the subtitle style."
				a.snapshot.Document.Cues[i].OriginalText = a.snapshot.Document.Cues[i].Text
			}
			a.snapshot.SourceDocument = a.snapshot.Document
			a.mediaURL, a.duration = "http://127.0.0.1/test-media", 60
			a.splitY = 44
			a.loadDraft()
			for _, size := range [][2]int{{1600, 1100}, {1920, 1200}, {900, 720}} {
				tt := settled(a.view, size[0], size[1])
				for range 6 {
					tt.Frame()
				}
				for _, dark := range []bool{false, true} {
					tt.SetDark(dark)
					capture := func(name string) {
						for range 4 {
							tt.Frame()
						}
						captureRedesign(t, tt, fmt.Sprintf("studio-%s-%d-dark%t-%s", language, size[0], dark, name))
					}
					a.workflowTab, a.styleTab, a.recognitionMode = 0, 0, 0
					a.settingsOpen, a.exportOpen, a.composeOpen, a.modelOpen = false, false, false, false
					capture("workbench")
					a.workflowTab, a.styleTab = 1, 1
					capture("translation-appearance")
					a.styleTab = 2
					capture("placement")
					a.workflowTab, a.recognitionMode = 0, 1
					a.batchItems = []batchItem{{Source: "lesson-01.mp4", Status: batchDone, Output: "lesson-01.srt"}, {Source: "lesson-02.mp4", Status: batchWaiting}, {Source: "lesson-03.wav", Status: batchWaiting}}
					capture("recognition-queue")
					a.settingsOpen = true
					for tab := 0; tab < 7; tab++ {
						a.settingsTab = tab
						a.providerSettingsTab = 0
						capture(fmt.Sprintf("settings-%d", tab))
						if tab == 2 {
							a.providerSettingsTab = 1
							capture("provider-connection")
						}
					}
					a.settingsOpen = false
					a.exportOpen = true
					a.settings.SubtitleOutput = subtitle.OutputOptions{Mode: "source", SourceTrack: "original"}
					capture("export")
					a.exportOpen, a.composeOpen = false, true
					capture("compose")
					a.composeOpen, a.modelOpen = false, true
					a.modelSelection = a.settings.AsrModel
					capture("model-files")
					a.modelOpen = false
				}
			}
		})
	}
}

func TestStudioCommonOptionsAndAdvancedOptionsAreSeparate(t *testing.T) {
	a := nativeForTest(t)
	a.settings.RecognitionMode = recognitionManual
	tt := settled(a.workflowPanel, 900, 1000)
	for _, label := range []string{locales.RecognitionMode(), locales.WorkflowRecognitionModel(), locales.SettingsSourceLanguage(), locales.ExportFormat(), locales.WorkflowRecognitionAdvanced()} {
		if !tt.HasText(label) {
			t.Fatal("common recognition option missing", label)
		}
	}
	if tt.HasText(locales.SettingsMaxTokens()) {
		t.Fatal("advanced decoding crowds common options")
	}
	if err := tt.Click(locales.WorkflowRecognitionAdvanced()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	if !tt.HasText(locales.SettingsMaxTokens()) {
		t.Fatal("advanced decoding is not reachable")
	}
	a.workflowTab = 1
	tt = settled(a.workflowPanel, 900, 1000)
	if tt.HasText(locales.TranslationConcurrency()) {
		t.Fatal("request scheduling crowds common translation options")
	}
	if err := tt.Click(locales.WorkflowTranslationAdvanced()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	if !tt.HasText(locales.TranslationConcurrency()) {
		t.Fatal("request scheduling is not reachable")
	}
}
