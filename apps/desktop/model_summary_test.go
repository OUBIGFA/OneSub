package main

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/models"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// Each selectable asset gets its own short explanation. In particular,
// similarly named SenseVoice ASR/hotword assets describe different workflows.
func TestModelSummariesCoverCatalogWithShortSentences(t *testing.T) {
	ends := regexp.MustCompile(`[.!?](?:\s+|$)`)
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			for _, model := range models.Catalog() {
				hint := modelSummary(model.ID)
				if hint == "" {
					t.Fatal("model has no explanation", model.ID)
				}
				if language == "en" {
					for _, sentence := range ends.Split(hint, -1) {
						if len(strings.Fields(sentence)) > 25 {
							t.Fatal("model explanation exceeds 25 words", model.ID, hint)
						}
					}
				}
			}
			if modelSummary(models.FallbackModel) == modelSummary("SenseVoiceSmall") {
				t.Fatal("ASR and the unconnected hotword model share a capability claim")
			}
		})
	}
}

// A long model name may elide in the selector, but native hover must reveal
// the full choice, including its precision, rather than only its field label.
func TestModelChoiceHasFullNativeTooltip(t *testing.T) {
	a := nativeForTest(t)
	a.settings.AsrModel = "sherpa-parakeet-tdt-0.6b-v3-fp32"
	tt := settled(func(c *ui.Context) {
		theme, _ := studioTheme(c.Theme(), false)
		c.SetTheme(&theme)
		a.recognitionModelField(c, locales.ModelKindAsr(), "asr", &a.settings.AsrModel)
	}, 320, 400)
	selectBox, found := tt.Find(locales.A11yModelOf(locales.ModelKindAsr()))
	if !found {
		t.Fatal("model selector is missing")
	}
	tt.Move(selectBox.X+selectBox.W/2, selectBox.Y+selectBox.H/2)
	tt.Advance(time.Second)
	count := 0
	for _, text := range tt.Texts() {
		if text == "Parakeet-TDT 0.6B v3 FP32" {
			count++
		}
	}
	if count < 2 {
		t.Fatal("full model name is not shown in its native tooltip", tt.Texts())
	}
}

// Descriptions use the real settings fields. They wrap below the selector,
// stay inside the fixed dialog and never move its close action or content box.
func TestModelSummariesFitFixedSettings(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		for _, width := range []int{900, 1400} {
			for _, scale := range []float32{1, 1.5, 2} {
				t.Run(fmt.Sprintf("%s/%d/%g", language, width, scale), func(t *testing.T) {
					inLocale(t, language, func() {
						a := nativeForTest(t)
						a.settingsOpen, a.settingsTab = true, 1
						tt := settled(a.view, int(float32(width)*scale), int(640*scale))
						tt.SetPreferences(ui.Preferences{TextScale: scale, ReduceMotion: true})
						area, _ := tt.Find(locales.A11ySettingsContent())
						closeButton, _ := tt.Find(locales.ActionClose())
						for _, dark := range []bool{false, true} {
							tt.SetDark(dark)
							for _, model := range models.Catalog() {
								a.settingsTab = 1
								title := ""
								switch model.Kind {
								case "asr":
									a.settings.AsrModel, title = model.ID, locales.ModelKindAsr()
								case "aligner":
									a.settings.Aligner, title = model.ID, locales.ModelKindAligner()
								case "separation":
									title = locales.ModelKindDemucs()
								case "hotword":
									title = locales.ModelKindHotword()
								case "lid":
									a.settingsTab, title = 5, locales.ModelKindLid()
								}
								tt.Frame()
								hint := modelSummary(model.ID)
								insideRedesign(t, tt, locales.A11ySettingsContent(), hint, locales.A11yModelOf(title))
								if a.settingsTab == 1 {
									insideRedesign(t, tt, locales.A11ySettingsContent(), locales.SettingsHotwords(), locales.SettingsVocalSeparation(), locales.A11yModelOf(locales.ModelKindAligner()))
									last, _ := tt.Find(locales.SettingsHotwords())
									if last.Y+last.H+17*scale > area.Y+area.H+1 {
										t.Fatal("auxiliary model panel crosses the settings footer", last, area)
									}
								}
								text, _ := tt.Find(hint)
								selectBox, _ := tt.Find(locales.A11yModelOf(title))
								if text.Y < selectBox.Y+selectBox.H-1 {
									t.Fatal("model explanation is not below its selector", model.ID, text, selectBox)
								}
								if after, _ := tt.Find(locales.A11ySettingsContent()); after != area {
									t.Fatal("model description resized the settings content", model.ID, area, after)
								}
								if button, _ := tt.Find(locales.ActionClose()); button != closeButton {
									t.Fatal("model description moved the close action", model.ID)
								}
								if scale == 1 && model.ID == "sherpa-parakeet-tdt-0.6b-v3-fp32" {
									captureRedesign(t, tt, fmt.Sprintf("model-hints-%s-%d-dark%t", language, width, dark))
								}
							}
						}
					})
				})
			}
		}
	}
}
