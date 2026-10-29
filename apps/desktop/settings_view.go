package main

import (
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/models"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

// Native sections share one spacing rhythm; no nested cards or drawn controls.
func settingsGroup(c *ui.Context, title string, content func()) {
	ui.Column(c).Gap(em(c, 12)).Children(func() {
		ui.Text(c, title).FontSize(em(c, headingSize)).FontWeight(600)
		ui.Divider(c)
		content()
	})
}

// interfaceLanguages lists "follow the system" and each catalog language
// named in itself, as language menus do ("English", "简体中文").
func interfaceLanguages() []option {
	options := []option{{"", locales.LanguageSystem()}}
	for _, tag := range mygo.App.Languages() {
		options = append(options, option{tag, i18n.Autonym(tag)})
	}
	return options
}

func (a *NativeApp) settingsContent(c *ui.Context, tab int) {
	switch tab {
	case 0:
		settingsGroup(c, locales.SettingsInterface(), func() {
			behavior := "exit"
			if a.settings.CloseToTray {
				behavior = "tray"
			}
			ui.Form(c, func() {
				ui.Field(c, locales.SettingsLanguage(), func() {
					idSelect(c, &a.settings.UILanguage, interfaceLanguages()).Width(260).Label(locales.SettingsLanguage()).OnChange(func() {
						mygo.App.SetLocale(a.settings.UILanguage)
						a.saveSettings()
					})
				})
				ui.Field(c, locales.SettingsAppearance(), func() {
					idSelect(c, &a.settings.Appearance, appearances()).Width(260).Label(locales.SettingsAppearance()).OnChange(func() {
						mygo.Theme.SetSource(mygo.ThemeSource(a.settings.Appearance))
						a.saveSettings()
					})
				})
				ui.Field(c, locales.SettingsCloseWindow(), func() {
					idSelect(c, &behavior, []option{{"exit", locales.SettingsCloseExit()}, {"tray", locales.SettingsCloseTray()}}).Width(260).Label(locales.SettingsCloseWindow()).OnChange(func() {
						a.settings.CloseToTray = behavior == "tray"
						a.saveSettings()
					})
				})
			})
		})
		settingsGroup(c, locales.SettingsPlayback(), func() {
			ui.Form(c, func() {
				a.playbackActionField(c, locales.SettingsSingleClick(), &a.settings.SingleClickAction)
				a.playbackActionField(c, locales.SettingsDoubleClick(), &a.settings.DoubleClickAction)
			})
			ui.Checkbox(c, &a.settings.FollowPlayback, locales.SettingsFollowPlayback()).OnChange(func() { a.activeCue = -1; a.saveSettings() })
		})
		// The configuration an MCP client needs to start OneSub's server,
		// to copy into the agent's settings as it is.
		settingsGroup(c, locales.SettingsMcp(), func() {
			config := mcpConfig()
			muted(c, locales.SettingsMcpNote())
			ui.Text(c, config).Font("monospace").Selectable().Label(locales.SettingsMcp())
			ui.Button(c, locales.SettingsMcpCopy()).AlignSelf(ui.Start).OnClick(func() {
				mygo.Clipboard.WriteText(config)
				a.status = locales.SettingsMcpCopied()
			})
		})
	case 1:
		// The models and their folders. The options of a recognition and its
		// output are beside the subtitles, in recognition and translation.
		ui.Grid(c).Columns(2).Gap(em(c, 36)).AlignItems(ui.Start).Children(func() {
			settingsGroup(c, locales.SettingsModels(), func() {
				a.recognitionModelField(c, locales.ModelKindAsr(), "asr", &a.settings.AsrModel)
				a.recognitionModelField(c, locales.ModelKindAligner(), "aligner", &a.settings.Aligner)
			})
			settingsGroup(c, locales.SettingsAuxModels(), func() {
				hotword, demucs := "SenseVoiceSmall", "htdemucs_ft"
				a.recognitionModelField(c, locales.ModelKindDemucs(), "separation", &demucs)
				ui.Checkbox(c, &a.settings.VocalSeparation, locales.SettingsVocalSeparation()).OnChange(a.saveSettings)
				a.recognitionModelField(c, locales.ModelKindHotword(), "hotword", &hotword)
				ui.Field(c, locales.SettingsHotwords(), func() {
					ui.TextInput(c, &a.settings.Hotwords).Placeholder(locales.SettingsHotwordsPlaceholder()).Label(locales.SettingsHotwords()).OnChange(a.saveSettings)
				})
			})
		})
	case 2:
		a.translationSettingsView(c)
	}
}

// Kind and IDs are consumed from OneAsr's catalog; auxiliary models cannot be
// selected as the ASR engine or forced aligner. Management stays beside the use.
func (a *NativeApp) recognitionModelField(c *ui.Context, title, kind string, id *string) {
	ui.Column(c).Gap(8).Children(func() {
		ui.Text(c, title)
		ui.Row(c).Gap(10).Children(func() {
			options := []option{}
			for _, m := range models.Catalog() {
				if m.Kind == kind {
					options = append(options, option{m.ID, m.Label})
				}
			}
			idSelect(c, id, options).Label(locales.A11yModelOf(title)).Grow(1).OnChange(a.saveSettings)
			ui.Button(c, locales.ModelManage()).Label(locales.A11yManageModel(title)).OnClick(func() { a.modelSelection = *id; a.modelOpen = true })
		})
	})
}

// modelStageLabel says the download stages of package models.
func modelStageLabel(stage string) string {
	switch stage {
	case models.StageDownload:
		return locales.ModelStageDownload()
	case models.StageVerify:
		return locales.ModelStageVerify()
	case models.StageDone:
		return locales.ModelStateVerified()
	}
	return stage
}

func (a *NativeApp) modelManagerView(c *ui.Context) {
	ui.Modal(c, &a.modelOpen, func() {
		model, err := models.ByID(a.modelSelection)
		if err != nil {
			ui.Text(c, err.Error())
			return
		}
		ui.Column(c).Width(em(c, 660)).Gap(em(c, 24)).Children(func() {
			dialogTitle(c, model.Label)
			settingsGroup(c, locales.ModelFiles(), func() {
				ui.Text(c, locales.ModelSize(float64(model.Size())/1e9))
				ui.Text(c, a.modelPath(model.ID)).Selectable().Label(locales.A11yModelFolder())
				state := a.modelStates[model.ID]
				if state == "" {
					state = locales.ModelStateUnknown()
				}
				ui.Text(c, state).Label(locales.A11yModelState())
			})
			ui.Row(c).Gap(12).Children(func() {
				iconButton(c, locales.ModelDownload(), "download").Disabled(a.busy).OnClick(func() { a.downloadModel(model) })
				ui.Button(c, locales.ModelChooseFolder()).Disabled(a.busy).OnClick(func() { a.pickModelDir(model) })
				ui.Button(c, locales.ModelVerify()).Disabled(a.busy).OnClick(func() { a.verifyModel(model) })
				ui.Spacer(c)
				ui.Button(c, locales.ActionClose()).OnClick(func() { a.modelOpen = false })
			})
		})
	})
}
