package main

import (
	"fmt"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// Check the bottom-most controls against the action row, not merely against
// the modal's rectangle: an overflowing form can otherwise hide behind its
// footer and still pass a coarse containment assertion.
func TestAllFormDialogsExposeCompleteContents(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			for _, size := range [][2]int{{900, 640}, {1440, 1000}} {
				for _, scale := range []float32{1, 1.5, 2} {
					for _, dark := range []bool{false, true} {
						for _, name := range []string{"export", "compose", "compress", "find", "insert", "preset", "model"} {
							t.Run(fmt.Sprintf("%s/%s/%d/%g/dark%t", language, name, size[0], scale, dark), func(t *testing.T) {
								a := nativeForTest(t)
								var view func(*ui.Context)
								var title, action string
								var fields []string
								switch name {
								case "export":
									a.exportOpen = true
									view, title, action = a.exportView, locales.MenuExportSubtitles(), locales.ExportConfirm()
									fields = []string{locales.OutputContent(), locales.OutputOrder(), locales.OutputPrimary(), locales.ExportFormat(), locales.ExportLines()}
								case "compose":
									a.composeOpen = true
									view, title, action = a.composeView, locales.ActionCompose(), locales.ComposeStart()
									fields = []string{locales.OutputContent(), locales.OutputOrder(), locales.OutputPrimary(), locales.ComposeMode(), locales.ComposeQuality(), locales.ComposeCrfRecommendation(), locales.ComposeAskPath()}
								case "compress":
									a.compressOpen = true
									view, title, action = a.compressView, locales.ActionCompress(), locales.CompressStart()
									fields = []string{locales.CompressFormat(), locales.ComposeQuality(), locales.ComposeCrfNote(), locales.ComposeAskPath()}
								case "find":
									a.findReplace.open = true
									view, title, action = a.findReplaceView, locales.FindReplace(), locales.ReplaceAll()
									fields = []string{locales.FindText(), locales.ReplaceWith(), locales.FindMatchCase(), locales.FindLiteralHint()}
								case "insert":
									a.insertOpen, a.subtitleTab, a.insertEnd = true, 1, 2
									view, title, action = a.insertSubtitleView, locales.SubtitleInsertTitle(), locales.SubtitleInsertConfirm()
									fields = []string{locales.EditorStart(), locales.EditorEnd(), locales.TabSource(), locales.SubtitleTranslationOptional()}
								case "preset":
									a.presetDialog = "new"
									view, title, action = a.presetDialogView, locales.PresetSaveNew(), locales.PresetSaveConfirm()
									fields = []string{locales.PresetNameLabel()}
								case "model":
									a.modelOpen, a.modelSelection = true, a.settings.AsrModel
									view, action = a.modelManagerView, locales.ModelDownload()
									fields = []string{locales.A11yModelFolder(), locales.ModelCatalog(), locales.A11yModelState()}
								}
								show := false
								tt := ui.NewTester(func(c *ui.Context) {
									theme, _ := studioTheme(c.Theme(), false)
									c.SetTheme(&theme)
									if show {
										view(c)
									}
								}, int(float32(size[0])*scale), int(float32(size[1])*scale))
								tt.SetPreferences(ui.Preferences{TextScale: scale, ReduceMotion: true})
								tt.SetDark(dark)
								firstFrameSettled(t, tt, name, func() { show = true })
								button, found := tt.Find(action)
								if !found || button.Y+button.H > float32(size[1])*scale || button.X+button.W > float32(size[0])*scale {
									t.Fatal("dialog action is clipped", action, button)
								}
								for _, label := range fields {
									field, ok := tt.Find(label)
									if !ok || field.W <= 0 || field.H <= 0 || field.Y < 0 || field.Y+field.H > button.Y-8*scale {
										t.Errorf("form field is missing or behind footer: %s %v, action %v", label, field, button)
									}
								}
								if title != "" {
									insideRedesign(t, tt, title, append(fields, action)...)
								}
								if name == "compose" || name == "compress" {
									previous := a.settings.ComposeAskPath
									if err := tt.Click(locales.ComposeAskPath()); err != nil || a.settings.ComposeAskPath == previous {
										t.Fatal("save prompt option is not operable", err)
									}
									loaded, err := settings.Load(a.dataRoot + "/settings.json")
									if err != nil || loaded.ComposeAskPath != a.settings.ComposeAskPath {
										t.Fatal("save prompt preference was not retained", err)
									}
								}
								if scale == 1 {
									captureRedesign(t, tt, fmt.Sprintf("form-%s-%s-%d-dark%t", name, language, size[0], dark))
								}
							})
						}
					}
				}
			}
		})
	}
}

func TestConfirmationCancelNeverChoosesDestructiveAction(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			open, chosen := true, -1
			tt := settled(func(c *ui.Context) {
				result := confirmationDialog(c, &open, locales.ClearTitle(), locales.ClearBody(), locales.ActionCancel(), locales.ClearConfirm())
				if result != -1 {
					chosen = result
				}
			}, 900, 640)
			visibleRedesign(t, tt, locales.ClearTitle(), locales.ActionCancel(), locales.ClearConfirm())
			captureRedesign(t, tt, "confirmation-"+language)
			tt.Key(0, ui.KeyEscape)
			if open || chosen != ui.AlertDismissed {
				t.Fatal("Escape changed the document instead of cancelling", open, chosen)
			}
			open = true
			tt.Frame()
			if err := tt.Click(locales.ActionCancel()); err != nil || open || chosen != 0 {
				t.Fatal("Cancel did not preserve the document", err, open, chosen)
			}
		})
	}
}
