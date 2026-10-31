package main

import (
	"context"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/styles"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// Optional render artifacts come from the production view, not a demo copy.
// They stay in the caller's ignored task directory, never in golden snapshots.
func captureRedesign(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	root := os.Getenv("ONESUB_UI_REVIEW_DIR")
	if root == "" {
		return
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(root, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err = png.Encode(file, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

func insideRedesign(t *testing.T, tt *ui.Tester, parent string, names ...string) {
	t.Helper()
	area, ok := tt.Find(parent)
	if !ok {
		t.Fatal("missing area", parent)
	}
	for _, name := range names {
		r, found := tt.Find(name)
		if !found || r.W <= 0 || r.H <= 0 || r.X < area.X-1 || r.Y < area.Y-1 || r.X+r.W > area.X+area.W+1 || r.Y+r.H > area.Y+area.H+1 {
			t.Errorf("%s outside %s: %v / %v", name, parent, r, area)
		}
	}
}

func TestNativeRedesignSettingsNavigation(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		for _, width := range []int{900, 1400} {
			for _, scale := range []float32{1, 1.5, 2} {
				t.Run(fmt.Sprintf("%s/%d/%g", language, width, scale), func(t *testing.T) {
					inLocale(t, language, func() {
						a := nativeForTest(t)
						a.settingsOpen = true
						tt := settled(a.view, int(float32(width)*scale), int(640*scale))
						tt.SetPreferences(ui.Preferences{TextScale: scale, ReduceMotion: true})
						if !tt.HasText(locales.SettingsCloseTray()) || tt.HasText(locales.SettingsMcpCopy()) {
							t.Fatal("incorrect defaults", tt.Texts())
						}
						before, _ := tt.Find(locales.A11ySettingsContent())
						closeButton, _ := tt.Find(locales.ActionClose())
						insideRedesign(t, tt, locales.A11ySettingsContent(), locales.SettingsLanguage(), locales.SettingsCloseWindow(), locales.SettingsFollowPlayback())
						for tab, label := range []string{locales.SettingsGeneral(), locales.SettingsRecognition(), locales.SettingsTranslation(), locales.SettingsTranslationRules(), locales.SettingsMcp()} {
							if err := tt.Click(label); err != nil {
								t.Fatal(err)
							}
							if a.settingsTab != tab {
								t.Fatal("navigation did not choose page", tab)
							}
							after, _ := tt.Find(locales.A11ySettingsContent())
							button, _ := tt.Find(locales.ActionClose())
							if before != after || button != closeButton {
								t.Fatal("settings shell moved", before, after, closeButton, button)
							}
							if tab == 2 {
								insideRedesign(t, tt, locales.A11ySettingsContent(), locales.TranslationProviderName(), locales.TranslationEndpoint(), "API Key", locales.TranslationMaxInput(), locales.TranslationRequestLimit())
							}
							if tab == 3 {
								insideRedesign(t, tt, locales.A11ySettingsContent(), locales.TranslationGlossary(), locales.TranslationStyle())
							}
							if tab == 4 {
								insideRedesign(t, tt, locales.A11ySettingsContent(), locales.A11yMcpConfig(), locales.SettingsMcpCopy())
								if tt.HasText(locales.SettingsCloseWindow()) {
									t.Fatal("MCP is still nested under General")
								}
							} else if tt.HasText(locales.A11yMcpConfig()) {
								t.Fatal("MCP configuration leaked into another category", tab)
							}
							if scale == 1 {
								captureRedesign(t, tt, fmt.Sprintf("settings-%s-%d-tab%d", language, width, tab))
								if width == 1400 {
									tt.SetDark(true)
									captureRedesign(t, tt, fmt.Sprintf("settings-%s-dark-tab%d", language, tab))
									tt.SetDark(false)
								}
							}
						}
						// Both native navigation variants support Home/End, without
						// putting focus into an invisible page or changing settings.
						tt.Key(0, ui.KeyHome)
						if a.settingsTab != 0 {
							t.Fatal("Home did not select General")
						}
						tt.Key(0, ui.KeyEnd)
						if a.settingsTab != 4 || !tt.HasText(locales.SettingsMcpCopy()) {
							t.Fatal("End did not open the standalone MCP page")
						}
					})
				})
			}
		}
	}
}

func TestNativeRedesignOutputGeometry(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			for _, compose := range []bool{false, true} {
				a := withStyleCatalog(t)
				a.restoreStyles()
				a.exportOpen, a.composeOpen = !compose, compose
				view := a.exportView
				if compose {
					view = a.composeView
				}
				tt := settled(func(c *ui.Context) {
					theme, _ := studioTheme(c.Theme(), false)
					c.SetTheme(&theme)
					view(c)
				}, 900, 640)
				title, action := locales.MenuExportSubtitles(), locales.ExportConfirm()
				if compose {
					title, action = locales.ActionCompose(), locales.ComposeStart()
				}
				button, _ := tt.Find(action)
				for _, mode := range []string{"source", "translation", "bilingual"} {
					for _, quality := range []string{"crf20", "lossless"} {
						for _, videoMode := range []string{"hard-mp4", "soft-mp4", "soft-mkv"} {
							a.settings.SubtitleOutput = subtitle.OutputOptions{Mode: mode}
							a.settings.ComposeQuality, a.settings.ComposeMode = quality, videoMode
							tt.Frame()
							if r, _ := tt.Find(action); r != button {
								t.Fatal("output action moved", r, button)
							}
							insideRedesign(t, tt, title, locales.OutputContent(), locales.OutputOrder(), locales.OutputPrimary(), action, locales.ActionCancel())
						}
					}
				}
				captureRedesign(t, tt, fmt.Sprintf("output-%s-compose%t", language, compose))
			}
		})
	}
}

func TestNativeRedesignSecondaryDefaultAndLabels(t *testing.T) {
	a := withStyleCatalog(t)
	for _, size := range []float64{10, 25, 30, 72} {
		main := a.styleCatalog.DefaultStyle
		main.FontSize = size
		a.settings.LastStyle = &main
		a.restoreStyles()
		if a.secondaryStyle.FontSize != size/2 || a.secondaryStyle.FontName != main.FontName {
			t.Fatal("secondary is not half the selected main style", a.secondaryStyle)
		}
	}
	a.settings.LastStyle = nil
	a.restoreStyles()
	client := styles.Client{Executable: os.Getenv("ONESUB_STYLE_WORKER"), Scratch: t.TempDir()}
	doc := subtitle.Document{Format: "srt", Cues: []subtitle.Cue{{ID: "1", StartMs: 0, EndMs: 2000, Text: "Translation", OriginalText: "Source", TranslationSource: "Source", Translated: true}}}
	built, err := client.BuildOutput(context.Background(), doc, a.currentStyle, a.secondaryStyle, subtitle.OutputOptions{Mode: "bilingual"}, 16.0/9)
	if err != nil || !strings.Contains(built.ASS, fmt.Sprintf("Style: Secondary,%s,%g,", a.secondaryStyle.FontName, a.currentStyle.FontSize/2)) {
		t.Fatal("half-size default did not reach SmartSub ASS", err, built.ASS)
	}
	saved := a.secondaryStyle
	saved.FontSize = 19
	a.settings.SecondaryStyle = &saved
	a.restoreStyles()
	if a.secondaryStyle.FontSize != 19 {
		t.Fatal("saved secondary typography overwritten")
	}
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a.settings.SubtitleOutput = subtitle.OutputOptions{Mode: "bilingual"}
			tt := settled(a.stylePanel, 440, 900)
			for _, text := range tt.Texts() {
				if strings.HasPrefix(text, locales.StylePrimaryLayer()+"（") || strings.HasPrefix(text, locales.StyleSecondaryLayer()+" (") {
					t.Fatal("language role appended to style label", text)
				}
			}
			insideRedesign(t, tt, locales.A11yStylePanel(), locales.StylePrimaryLayer(), locales.StyleSecondaryLayer())
			captureRedesign(t, tt, "style-"+language)
		})
	}
}
