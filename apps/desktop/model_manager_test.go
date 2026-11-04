package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/models"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// Unavailable ASR choices are visible for discovery, but cannot become the
// active recognition model. The manager has a separate downloadable catalog.
func TestSettingsAsrDisablesMissingModelsAndKeepsManagementCatalog(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := nativeForTest(t)
			a.settings.AsrModel = models.FallbackModel
			a.downloadedAsr = []option{{models.FallbackModel, "SenseVoiceSmall INT8"}}
			a.settingsOpen, a.settingsTab = true, 1
			tt := settled(a.view, 1400, 800)
			if err := tt.Click(locales.A11yModelOf(locales.ModelKindAsr())); err != nil {
				t.Fatal(err)
			}
			if !tt.HasText("Qwen3-ASR 1.7B") || !tt.HasText(locales.ModelNotDownloaded()) {
				t.Fatal("settings lost unavailable models or their state", tt.Texts())
			}
			if err := tt.Click("Qwen3-ASR 1.7B"); err != nil || a.settings.AsrModel != models.FallbackModel {
				t.Fatal("missing model became the active ASR choice", a.settings.AsrModel, err)
			}
			tt.Key(0, ui.KeyEscape)
			if err := tt.Click(locales.A11yManageModel(locales.ModelKindAsr())); err != nil {
				t.Fatal(err)
			}
			if err := tt.Click(locales.ModelCatalog()); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem("Qwen3-ASR 1.7B · " + locales.ModelNotDownloaded()); err != nil {
				t.Fatal("manager could not select a download target", err, tt.Menu())
			}
			if a.modelSelection != "Qwen3-ASR-1.7B-hf" || a.settings.AsrModel != models.FallbackModel {
				t.Fatal("management target changed the active ASR model", a.modelSelection, a.settings.AsrModel)
			}
		})
	}
}

// Use the actual installed model directory for download/delete UI states.
// The disabled download button stays gray and says Downloaded; cancelling a
// recycle confirmation leaves the real assets untouched.
func TestDownloadedModelManagerStatesAndFixedLayout(t *testing.T) {
	root := os.Getenv("ONESUB_SHERPA_TEST_MODELS")
	if root == "" {
		t.Skip("set ONESUB_SHERPA_TEST_MODELS for installed model UI states")
	}
	for _, language := range []string{"zh-Hans", "en"} {
		for _, scale := range []float32{1, 1.5, 2} {
			t.Run(fmt.Sprintf("%s/%g", language, scale), func(t *testing.T) {
				inLocale(t, language, func() {
					a := nativeForTest(t)
					a.settings.ModelPaths[models.FallbackModel] = filepath.Join(root, models.FallbackModel)
					a.modelSelection, a.modelOpen = models.FallbackModel, true
					tt := settled(a.view, int(900*scale), int(640*scale))
					tt.SetPreferences(ui.Preferences{TextScale: scale, ReduceMotion: true})
					insideRedesign(t, tt, "SenseVoiceSmall INT8", locales.ModelCatalog(), locales.A11yModelChooseFolder(), locales.ModelVerify(), locales.ModelDelete(), locales.ModelDownload(), locales.ActionClose())
					if !tt.HasText(locales.ModelDownloaded()) {
						t.Fatal("completed model has no downloaded button state", tt.Texts())
					}
					if err := tt.Click(locales.ModelDownload()); err != nil || tt.Focused(locales.ModelDownload()) || a.busy {
						t.Fatal("downloaded model started another download", err)
					}
					if err := tt.Click(locales.ModelDelete()); err != nil || a.modelRecycle != models.FallbackModel {
						t.Fatal("delete did not show a native recycle confirmation", err)
					}
					if !tt.HasText(locales.ModelRecycleNote()) {
						t.Fatal("recycle confirmation is missing")
					}
					if err := tt.Click(locales.ActionCancel()); err != nil || a.modelRecycle != "" || a.busy {
						t.Fatal("cancelled model deletion continued", err)
					}
					model, _ := models.ByID(models.FallbackModel)
					if !model.FilesPresent(a.modelPath(model.ID)) {
						t.Fatal("cancel changed real model files")
					}
				})
			})
		}
	}
}
