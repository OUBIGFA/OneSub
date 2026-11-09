package main

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/importing"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// Audit the production pages, not a gallery reproducing their widgets. Light
// establishes the geometry; dark must keep it and contain no white controls.
func TestLobeVisualAudit(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			pages := []string{"workbench", "translate", "style-appearance", "style-layout", "settings-0", "settings-1", "settings-2", "settings-3", "settings-4", "settings-5", "settings-6", "provider-connection", "compress", "audio", "compose", "export", "insert", "candidates", "model", "review", "retranslation"}
			for _, page := range pages {
				t.Run(language+"/"+page, func(t *testing.T) {
					a := withStyleCatalog(t)
					switch page {
					case "translate":
						a.workflowTab = 1
					case "style-appearance":
						a.styleTab = 1
					case "style-layout":
						a.styleTab = 2
					case "provider-connection":
						a.settingsOpen, a.settingsTab, a.providerSettingsTab = true, 2, 1
					case "compress":
						a.compressOpen = true
					case "audio":
						a.compressOpen, a.settings.MediaProcessMode = true, "audio"
					case "compose":
						a.composeOpen = true
					case "export":
						a.exportOpen = true
					case "insert":
						a.insertOpen, a.insertStart, a.insertEnd = true, 360000.123, 360003.789
					case "candidates":
						a.candidatesOpen = true
						a.pending = &importing.Loaded{Selection: importing.Selection{Candidates: []importing.Candidate{{Path: "Example lesson.en.srt", Score: 96, Reason: "language"}, {Path: "Example lessons.vtt", Score: 88, Reason: "similar"}}}}
					case "model":
						a.modelOpen, a.modelSelection = true, a.settings.AsrModel
					case "review":
						a = bulkTranslationReviewApp(t)
					case "retranslation":
						a = checkedReviewApp(t)
					default:
						if strings.HasPrefix(page, "settings-") {
							a.settingsOpen = true
							fmt.Sscanf(page, "settings-%d", &a.settingsTab)
						}
					}
					tt := ui.NewTester(a.view, 1440, 1000)
					tt.SetPreferences(ui.Preferences{ReduceMotion: true})
					var light []ui.AccessInfo
					for _, dark := range []bool{false, true} {
						tt.SetDark(dark)
						tt.Frame()
						controls := auditControls(tt)
						if !dark {
							light = controls
						} else {
							if len(light) != len(controls) {
								t.Errorf("appearance changes control count: %d -> %d", len(light), len(controls))
							}
							for i := range min(len(light), len(controls)) {
								if light[i].Role != controls[i].Role || light[i].Name != controls[i].Name || light[i].Bounds != controls[i].Bounds {
									t.Errorf("dark differs from light layout: %s %v -> %s %v", light[i].Name, light[i].Bounds, controls[i].Name, controls[i].Bounds)
								}
							}
							checkDarkControls(t, tt, controls)
						}
						if a.settingsOpen {
							checkSettingsLabelsLeftAligned(t, tt)
						}
						captureRedesign(t, tt, fmt.Sprintf("audit-%s-%s-dark%t", language, page, dark))
					}
				})
			}
		})
	}
}

func auditControls(tt *ui.Tester) []ui.AccessInfo {
	var controls []ui.AccessInfo
	for _, control := range tt.AccessAll() {
		switch control.Role {
		case ui.RoleButton, ui.RoleMenuButton, ui.RoleTextField, ui.RoleComboBox, ui.RoleCheckBox, ui.RoleTab, ui.RoleRadio, ui.RoleToggleButton:
			if !control.Offscreen && control.Bounds.W > 0 && control.Bounds.H > 0 {
				controls = append(controls, control)
			}
		}
	}
	return controls
}

func checkDarkControls(t *testing.T, tt *ui.Tester, controls []ui.AccessInfo) {
	t.Helper()
	image := tt.Image()
	for _, control := range controls {
		// Color samples are content, not theme backgrounds.
		if control.Name == locales.StyleTextColor() || control.Name == locales.StyleOutlineColor() || control.Name == locales.StyleShadowColor() || control.Name == locales.StyleBoxColor() {
			continue
		}
		r := control.Bounds
		bright, sampled := 0, 0
		for y := int(r.Y + 3); y < int(r.Y+r.H-3); y += 3 {
			for x := int(r.X + 3); x < int(r.X+r.W-3); x += 3 {
				if x < 0 || y < 0 || x >= image.Bounds().Dx() || y >= image.Bounds().Dy() {
					continue
				}
				p := image.RGBAAt(x, y)
				if p.R > 200 && p.G > 200 && p.B > 200 {
					bright++
				}
				sampled++
			}
		}
		if sampled > 30 && bright*2 > sampled {
			t.Errorf("white control in dark appearance: %s %v (%d/%d pixels)", control.Name, r, bright, sampled)
		}
	}
}

func checkSettingsLabelsLeftAligned(t *testing.T, tt *ui.Tester) {
	t.Helper()
	var left float32
	for i, label := range []string{locales.SettingsGeneral(), locales.SettingsRecognition(), locales.SettingsLanguageRoutes(), locales.SettingsTranslation(), locales.SettingsTranslationRules(), locales.SettingsMcp(), locales.SettingsAbout()} {
		r, ok := tt.Find(label)
		if !ok {
			t.Errorf("missing settings destination %s", label)
			continue
		}
		if i == 0 {
			left = r.X
		} else if math.Abs(float64(left-r.X)) > .5 {
			t.Errorf("settings labels not left aligned: %s at %g, want %g", label, r.X, left)
		}
	}
}
