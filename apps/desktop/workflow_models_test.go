package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/translator"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

func modelApp(t *testing.T) *NativeApp {
	t.Helper()
	a := nativeForTest(t)
	a.addProvider()
	a.settings.Provider().Name = "Studio"
	a.settings.ProviderURL = "https://example.com/v1"
	a.settings.Provider().Catalog = []translator.ModelInfo{
		{ID: "model-a", MaxInputTokens: 96000, MaxOutputTokens: 8192},
		{ID: "model-b", ContextWindow: 128000},
		{ID: "model-c"},
	}
	a.useProviderModel("model-a")
	return a
}

// Exercise the actual native menu on both surfaces. A populated input must
// not require deleting its value or typing a prefix to reveal alternatives.
func TestModelDropDownSwitchesOnBothSurfaces(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		for _, inSettings := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/settings=%t", language, inSettings), func(t *testing.T) {
				inLocale(t, language, func() {
					a := modelApp(t)
					a.keepProviderModel("model-b")
					a.workflowTab = 1
					view := a.workflowPanel
					if inSettings {
						a.workflowTab = 0 // find the modal's model control, not the covered workbench's
						a.settingsOpen, a.settingsTab = true, 2
						view = a.view
					}
					tt := settled(view, 1100, 760)
					if err := tt.Click(locales.TranslationSwitchModel()); err != nil {
						t.Fatal(err)
					}
					if !slices.Equal(tt.Menu(), []string{"model-a", "model-b"}) {
						t.Fatal("drop-down exposed an unchecked model or lost an enabled one", tt.Menu())
					}
					if err := tt.ChooseMenuItem("model-b"); err != nil {
						t.Fatal(err)
					}
					loaded, err := settings.Load(filepath.Join(a.dataRoot, "settings.json"))
					if err != nil || loaded.ProviderModel != "model-b" || loaded.ModelInfo().ContextWindow != 128000 || len(loaded.Provider().Models) != 2 {
						t.Fatal("native selection was not persisted with its limits", err, loaded.ProviderModel)
					}
					// A later frame and opening the other surface still show the
					// same model; there is no view-local selection authority.
					a.settingsOpen, a.settingsTab = true, 2
					tt = settled(a.view, 1100, 760)
					if !tt.HasText("model-b") {
						t.Fatal("model selection did not reach settings")
					}
				})
			})
		}
	}
}

func TestModelLibrarySearchAndRetentionDoNotChangeSelection(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := modelApp(t)
			a.settingsOpen, a.settingsTab = true, 2
			tt := settled(a.view, 1100, 760)
			if err := tt.Click(locales.TranslationKeepModel("model-b")); err != nil {
				t.Fatal(err)
			}
			if !a.settings.Provider().KeepsModel("model-b") || a.settings.ProviderModel != "model-a" {
				t.Fatal("checking a model switched the active selection")
			}
			if err := tt.Click(locales.TranslationSearchModels()); err != nil {
				t.Fatal(err)
			}
			tt.Type("MODEL-B")
			if !tt.HasText("model-b") || tt.HasText("model-c") {
				t.Fatal("catalog search did not filter case-insensitively", tt.Texts())
			}
			if err := tt.Click(locales.TranslationUseModelNamed("model-b")); err != nil {
				t.Fatal(err)
			}
			if a.settings.ProviderModel != "model-b" || !tt.HasText(locales.TranslationModelInUse()) {
				t.Fatal("explicit use action did not switch the model")
			}
			if err := tt.Click(locales.TranslationKeepModel("model-b")); err != nil {
				t.Fatal(err)
			}
			if !a.settings.Provider().KeepsModel("model-b") {
				t.Fatal("the active model could be removed")
			}
			a.addProvider()
			tt.Frame()
			if a.modelQuery != "" || tt.HasText("model-b") {
				t.Fatal("search or catalog leaked between providers")
			}
		})
	}
}

func TestManualModelCommitsWholeIdentifierWithoutPartialSavedModels(t *testing.T) {
	a := modelApp(t)
	a.settingsOpen, a.settingsTab = true, 2
	tt := settled(a.view, 1100, 760)
	name := "private/" + strings.Repeat("long-model-", 24) + "v2"
	if err := tt.Click(locales.TranslationModel()); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type(name)
	if a.settings.ProviderModel != "model-a" || len(a.settings.Provider().Models) != 1 {
		t.Fatal("typing saved a partial identifier")
	}
	tt.Key(0, ui.KeyEnter)
	if a.settings.ProviderModel != name || len(a.settings.Provider().Models) != 2 {
		t.Fatal("Enter did not commit the complete model", a.settings.ProviderModel)
	}
	if err := tt.Click(locales.TranslationModel()); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEnd)
	tt.Type("-edited")
	if err := tt.Click(locales.TranslationSearchModels()); err != nil {
		t.Fatal(err)
	}
	if a.settings.ProviderModel != name+"-edited" {
		t.Fatal("blur lost the end of an elided identifier", a.settings.ProviderModel)
	}
	a.settings.ModelInfo().MaxOutputTokens = 12345
	a.settings.ModelInfo().LimitsSource = "manual"
	a.settings.Provider().Catalog = nil // a service with no /models endpoint
	a.saveSettings()
	loaded, err := settings.Load(filepath.Join(a.dataRoot, "settings.json"))
	if err != nil || len(loaded.Provider().Models) != 3 || len(loaded.Provider().Catalog) != 0 || loaded.ModelInfo().MaxOutputTokens != 12345 {
		t.Fatal("restart discarded manually retained models or limits", err, loaded.Provider())
	}
}

func TestPopulatedModelLibraryGeometry(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		for _, width := range []int{900, 1400} {
			for _, scale := range []float32{1, 1.5, 2} {
				t.Run(fmt.Sprintf("%s/%d/%g", language, width, scale), func(t *testing.T) {
					inLocale(t, language, func() {
						a := modelApp(t)
						longID := "model-000-" + strings.Repeat("long-name/", 35)
						a.settings.Provider().Catalog = []translator.ModelInfo{{ID: longID}, {ID: "model-001"}}
						for i := 2; i < 80; i++ {
							a.settings.Provider().Catalog = append(a.settings.Provider().Catalog, translator.ModelInfo{ID: fmt.Sprintf("model-%03d", i)})
						}
						a.useProviderModel("model-001")
						a.settingsOpen, a.settingsTab = true, 2
						tt := settled(a.view, int(float32(width)*scale), int(640*scale))
						tt.SetPreferences(ui.Preferences{TextScale: scale, ReduceMotion: true})
						for _, dark := range []bool{false, true} {
							tt.SetDark(dark)
							if scale == 1 {
								captureRedesign(t, tt, fmt.Sprintf("model-library-%s-%d-dark%t", language, width, dark))
							}
							insideRedesign(t, tt, locales.A11ySettingsContent(), locales.TranslationSearchModels(), locales.TranslationSwitchModel(), locales.TranslationFetchModels(), locales.TranslationKeepModel(longID), locales.TranslationUseModelNamed(longID), locales.TranslationMaxInput(), locales.TranslationMaxOutput(), locales.TranslationContextWindow())
							catalog, _ := tt.Find(locales.A11yModelCatalog())
							limits, _ := tt.Find(locales.TranslationMaxInput())
							if catalog.Y+catalog.H > limits.Y-8*scale {
								t.Fatal("model list overlaps model limits", catalog, limits)
							}
						}
					})
				})
			}
		}
	}
}

func TestNativeFileDropsEnterSharedRecognitionQueue(t *testing.T) {
	a := storeApp(t, subtitle.Cue{ID: "one", Text: "Unsaved source", OriginalText: "Unsaved source", EndMs: 1000})
	a.unsaved[0] = true
	before := a.snapshot
	dir := t.TempDir()
	files := []string{filepath.Join(dir, "one.mp4"), filepath.Join(dir, "two.wav"), filepath.Join(dir, "three.mkv")}
	for _, file := range files {
		if err := os.WriteFile(file, []byte("media fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	tt := settled(a.view, 1400, 900)
	player, _ := tt.Find(locales.A11yPlayerSurface())
	if !tt.DropFiles(player.X+player.W/2, player.Y+player.H/2, files[0], files[1]) {
		t.Fatal("the native player overlay did not route dropped files")
	}
	if a.workflowTab != 0 || a.recognitionMode != 1 || len(a.batchItems) != 2 || a.snapshot != before || a.savePromptOpen {
		t.Fatal("multiple media replaced the workbench instead of becoming a queue")
	}
	// Drop one file on the queue, then the same file again on a populated row.
	tt = settled(a.workflowPanel, 900, 900)
	area, _ := tt.Find(locales.BatchDropArea())
	for range 2 {
		if !tt.DropFiles(area.X+area.W/2, area.Y+20, files[2]) {
			t.Fatal("queue did not accept a single-file drop")
		}
	}
	if len(a.batchItems) != 3 || a.snapshot != before {
		t.Fatal("queue duplicate or unintended import", a.batchItems)
	}
	if err := tt.Click(locales.BatchRemoveFile("one.mp4")); err != nil {
		t.Fatal(err)
	}
	if len(a.batchItems) != 2 || len(a.settings.BatchSources) != 2 {
		t.Fatal("removing a queued file was not saved")
	}
	captureRedesign(t, tt, "recognition-queue-populated")
	tt.SetDark(true)
	captureRedesign(t, tt, "recognition-queue-populated-dark")
}

func TestFileRoutingPreservesExplicitSubtitlePairs(t *testing.T) {
	a := nativeForTest(t)
	for _, mode := range []int{0, 1} {
		a.recognitionMode = mode
		for _, paths := range [][]string{{"clip.mp4", "clip.srt"}, {"clip.srt"}, {"clip.ass"}} {
			if a.filesTargetQueue(paths) {
				t.Fatal("subtitle import was captured by the media queue", paths)
			}
		}
	}
	a.recognitionMode = 0
	if a.filesTargetQueue([]string{"clip.mp4"}) || a.filesTargetQueue([]string{"clip.mp4", "CLIP.MP4"}) || !a.filesTargetQueue([]string{"one.mp4", "two.wav"}) {
		t.Fatal("single, duplicate and multiple media routing disagree")
	}
}

func TestRecognitionTargetsFreezeEverySharedOption(t *testing.T) {
	a := nativeForTest(t)
	a.settings.AsrModel, a.settings.Aligner = "asr-test", "aligner-test"
	a.settings.ModelPaths = map[string]string{"asr-test": "model-dir", "aligner-test": "aligner-dir"}
	a.settings.Language, a.settings.Backend = "en", "cpu"
	a.settings.ChunkSeconds, a.settings.MaxTokens = 90, 1024
	a.settings.RecognitionFormat, a.settings.RecognitionDir = "vtt", t.TempDir()
	a.settings.StripPunctuation, a.settings.KeepPunctuation = true, "?!"
	a.settings.VocalSeparation, a.settings.Hotwords = true, "OneSub"
	first := a.recognitionPreferences()
	a.recognitionMode = 1
	queue := a.recognitionPreferences()
	if !reflect.DeepEqual(first, queue) {
		t.Fatal("queue settings differ from current-file recognition")
	}
	a.settings.ModelPaths["asr-test"] = "changed-dir"
	a.settings.Language, a.settings.RecognitionFormat = "ja", "ass"
	if first.ModelPaths["asr-test"] != "model-dir" || queue.ModelPaths["asr-test"] != "model-dir" || queue.Language != "en" || queue.RecognitionFormat != "vtt" || queue.ChunkSeconds != 90 || queue.MaxTokens != 1024 || !queue.VocalSeparation || queue.Hotwords != "OneSub" {
		t.Fatal("running recognition did not retain its frozen inputs")
	}
}

func TestQueueRetryCountsOnlyUnfinishedFilesAndGuardsMutations(t *testing.T) {
	a := nativeForTest(t)
	a.batchItems = []batchItem{{Source: "done.mp4", Status: batchDone}, {Source: "failed.mp4", Status: batchFailed}, {Source: "pending.mp4", Status: batchWaiting}}
	done, failed := a.batchCounts()
	if done != 1 || failed != 1 || a.pendingBatchCount() != 2 {
		t.Fatal("queue progress or retry scope is incorrect")
	}
	a.busy = true
	a.clearBatchFiles()
	a.removeBatchFile("done.mp4")
	a.queueBatchFiles(nil)
	if len(a.batchItems) != 3 {
		t.Fatal("a running queue was mutated")
	}
}
