package main

import (
	"strconv"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/models"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

// Settings share the same native fieldset hierarchy as the workbench. The
// section heading and spacing do the grouping; no nested cards or extra rules.
func settingsGroup(c *ui.Context, title string, content func()) {
	parameterGroup(c, title, nil, content)
}

// Settings use one sidebar layout. The active form has explicit native grid
// columns, so opening it never waits for previous-frame bounds to settle.
func (a *NativeApp) settingsView(c *ui.Context) {
	wasOpen := a.settingsOpen
	// Pages keep their numbers; models by language (5) follows recognition.
	labels := []string{locales.SettingsGeneral(), locales.SettingsRecognition(), locales.SettingsTranslation(), locales.SettingsTranslationRules(), locales.SettingsMcp(), locales.SettingsLanguageRoutes()}
	pageIcons := []string{"settings", "audio-lines", "languages", "file-down", "settings", "text-search"}
	order := []int{0, 1, 5, 2, 3, 4}
	fixedDialog(c, &a.settingsOpen, locales.SettingsTitle(), 1280, 800, func() {
		content := func() {
			ui.Column(c.Key(a.settingsTab)).Fill().Gap(em(c, 20)).Label(locales.A11ySettingsContent()).Children(func() {
				a.settingsContent(c, a.settingsTab)
			})
		}
		ui.Row(c).Fill().Gap(em(c, 24)).AlignItems(ui.Stretch).Children(func() {
			selected := strconv.Itoa(a.settingsTab)
			navigationWidth := float32(0)
			for _, label := range labels {
				w, _ := c.MeasureText(0, ui.Span{Text: label})
				navigationWidth = max(navigationWidth, w)
			}
			ui.Sidebar(c, &selected, func() {
				for _, tab := range order {
					ui.SidebarItem(c, strconv.Itoa(tab), icons[pageIcons[tab]], labels[tab])
				}
			}).Width(navigationWidth + em(c, 72)).Label(locales.A11ySettingsTabs()).OnChange(func() {
				a.settingsTab, _ = strconv.Atoi(selected)
			})
			ui.Column(c).Grow(1).MinWidth(0).Children(content)
		})
	}, func() {
		ui.Button(c, locales.ActionClose()).OnClick(func() { a.saveSettings(); a.settingsOpen = false })
	})
	if wasOpen && !a.settingsOpen {
		a.commitProviderModelInput()
		a.refreshDownloadedAsrModels()
	}
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
			ui.Grid(c).Columns(3).GapX(em(c, 20)).GapY(em(c, 16)).AlignItems(ui.Start).Children(func() {
				ui.Field(c, locales.SettingsLanguage(), func() {
					idSelect(c, &a.settings.UILanguage, interfaceLanguages()).Label(locales.SettingsLanguage()).OnChange(func() {
						mygo.App.SetLocale(a.settings.UILanguage)
						a.saveSettings()
					})
				})
				ui.Field(c, locales.SettingsAppearance(), func() {
					idSelect(c, &a.settings.Appearance, appearances()).Label(locales.SettingsAppearance()).OnChange(func() {
						mygo.Theme.SetSource(mygo.ThemeSource(a.settings.Appearance))
						a.saveSettings()
					})
				})
				ui.Field(c, locales.SettingsCloseWindow(), func() {
					idSelect(c, &behavior, []option{{"tray", locales.SettingsCloseTray()}, {"exit", locales.SettingsCloseExit()}}).Label(locales.SettingsCloseWindow()).OnChange(func() {
						a.settings.CloseToTray = behavior == "tray"
						a.saveSettings()
					})
				})
			})
		})
		settingsGroup(c, locales.SettingsPlayback(), func() {
			ui.Grid(c).Columns(2).GapX(em(c, 20)).GapY(em(c, 16)).AlignItems(ui.Start).Children(func() {
				a.playbackActionField(c, locales.SettingsSingleClick(), &a.settings.SingleClickAction)
				a.playbackActionField(c, locales.SettingsDoubleClick(), &a.settings.DoubleClickAction)
			})
			ui.Checkbox(c, &a.settings.FollowPlayback, locales.SettingsFollowPlayback()).OnChange(func() { a.activeCue = -1; a.saveSettings() })
		})
	case 1:
		// The models and their folders. The options of a recognition and its
		// output are beside the subtitles, in recognition and translation.
		ui.Grid(c).Columns(2).Gap(em(c, 24)).AlignItems(ui.Start).Children(func() {
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
	case 3:
		a.translationRulesView(c)
	case 4:
		a.mcpSettingsView(c)
	case 5:
		// The detector and the model of each language group used by smart
		// routing. One full-width group keeps model names usable at the
		// smallest window.
		settingsGroup(c, locales.SettingsLanguageRoutes(), func() {
			lid := models.LanguageDetector
			a.recognitionModelField(c, locales.ModelKindLid(), "lid", &lid)
			cells := []func(){}
			for _, group := range models.RouteGroups {
				cells = append(cells, func() { a.routeField(c, group) })
			}
			parameterGrid(c, 230, 3, cells...)
		})
	}
}

// MCP is a dedicated settings destination, not a disclosure nested under
// General. Its copyable client configuration is immediately visible.
func (a *NativeApp) mcpSettingsView(c *ui.Context) {
	settingsGroup(c, locales.SettingsMcp(), func() {
		config := mcpConfig()
		muted(c, locales.SettingsMcpNote())
		ui.Text(c, config).Font("monospace").Selectable().Label(locales.A11yMcpConfig())
		ui.Button(c, locales.SettingsMcpCopy()).AlignSelf(ui.Start).OnClick(func() {
			mygo.Clipboard.WriteText(config)
			a.status = locales.SettingsMcpCopied()
		})
	})
}

// routeField chooses among downloaded models that recognize the language
// group. Missing saved choices stay in preferences, not in the usable menu.
func (a *NativeApp) routeField(c *ui.Context, group string) {
	title := locales.RouteGroupEu()
	for _, o := range sourceLanguages() {
		if o.ID == group {
			title = o.Label
		}
	}
	ui.Field(c, title, func() {
		options := []option{}
		for _, m := range models.Catalog() {
			if m.GroupSupported(group) && a.asrDownloaded(m.ID) {
				options = append(options, option{m.ID, m.Label})
			}
		}
		chosen := routeModel(a.settings, group)
		placeholder := locales.RecognitionChooseDownloadedModel()
		if len(options) == 0 {
			placeholder = locales.RecognitionNoDownloadedModel()
		}
		idSelect(c, &chosen, options, placeholder).Label(title).Disabled(a.busy || len(options) == 0).OnChange(func() {
			if a.settings.LanguageModels == nil {
				a.settings.LanguageModels = map[string]string{}
			}
			a.settings.LanguageModels[group] = chosen
			a.saveSettings()
		})
	})
}

// modelSummary explains the selected asset, not an unverified quality claim.
// Keep each catalog choice explicit so similarly named ASR and hotword models
// cannot inherit one another's capabilities.
func modelSummary(id string) string {
	switch id {
	case "Qwen3-ASR-0.6B-hf":
		return locales.ModelSummaryQwenSmall()
	case "Qwen3-ASR-1.7B-hf":
		return locales.ModelSummaryQwenLarge()
	case "Qwen3-ASR-0.6B-int8":
		return locales.ModelSummaryQwenSmallInt8()
	case "Qwen3-ASR-1.7B-int8":
		return locales.ModelSummaryQwenLargeInt8()
	case "sherpa-sense-voice-small-int8":
		return locales.ModelSummarySenseVoice()
	case "sherpa-paraformer-zh-int8":
		return locales.ModelSummaryParaformer()
	case "sherpa-parakeet-tdt-0.6b-v3-int8":
		return locales.ModelSummaryParakeetInt8()
	case "sherpa-parakeet-tdt-0.6b-v3-fp32":
		return locales.ModelSummaryParakeetFp32()
	case "omniASR-CTC-300M-v2-hf":
		return locales.ModelSummaryCtc()
	case "Qwen3-ForcedAligner-0.6B-hf":
		return locales.ModelSummaryQwenAligner()
	case "htdemucs_ft":
		return locales.ModelSummaryDemucs()
	case "SenseVoiceSmall":
		return locales.ModelSummaryHotword()
	case models.LanguageDetector:
		return locales.ModelSummaryLid()
	}
	return ""
}

// Kind and IDs are consumed from OneAsr's catalog; auxiliary models cannot be
// selected as the ASR engine or forced aligner. Management stays beside the use.
func (a *NativeApp) recognitionModelField(c *ui.Context, title, kind string, id *string) {
	ui.Fieldset(c, title, func() {
		ui.Row(c).Gap(em(c, 12)).Children(func() {
			options := []option{}
			for _, m := range models.Catalog() {
				if m.Kind == kind && (kind != "lid" || m.FilesPresent(a.modelPath(m.ID))) {
					options = append(options, option{m.ID, m.Label})
				}
			}
			changed := a.saveSettings
			if kind == "asr" {
				changed = a.asrModelChanged
			}
			if kind == "asr" {
				a.settingsAsrSelect(c).Label(locales.A11yModelOf(title)).Grow(1)
			} else {
				// The language detector shares the downloaded-only rule. Its
				// management action still opens the complete download catalog.
				idSelect(c, id, options, locales.ModelNotDownloaded()).Label(locales.A11yModelOf(title)).Grow(1).Disabled(a.busy || len(options) == 0).OnChange(changed)
			}
			ui.Button(c, locales.ModelManage()).Label(locales.A11yManageModel(title)).OnClick(func() { a.modelSelection = *id; a.modelOpen = true })
		})
		// The explanation is immediately below the selected model, before
		// its folder. Native text wraps instead of hiding the useful hint.
		muted(c, modelSummary(*id)).FontSize(em(c, captionSize)).MinWidth(0)
		path := a.modelPath(*id)
		muted(c, path).SingleLine().Tooltip(path).FontSize(em(c, captionSize)).MinWidth(0)
	}).Margin(0).Gap(em(c, 8))
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
	if !a.modelOpen {
		return
	}
	model, err := models.ByID(a.modelSelection)
	if err != nil {
		ui.AlertDialog(c, &a.modelOpen, locales.ModelFiles(), err.Error(), locales.ActionClose())
		return
	}
	downloaded := model.FilesPresent(a.modelPath(model.ID))
	assets, assetErr := modelAssetPaths(model, a.modelPath(model.ID))
	fixedDialog(c, &a.modelOpen, model.Label, 720, 400, func() {
		settingsGroup(c, locales.ModelFiles(), func() {
			ui.Row(c).Gap(em(c, 16)).Wrap().AlignItems(ui.Center).Children(func() {
				ui.MenuButton(c, locales.ModelCatalog(), func(m *ui.Menu) {
					for _, candidate := range models.Catalog() {
						label := candidate.Label
						if !candidate.FilesPresent(a.modelPath(candidate.ID)) {
							label += " · " + locales.ModelNotDownloaded()
						}
						if m.Item(label).Checked(candidate.ID == model.ID).Chosen() {
							a.modelSelection = candidate.ID
						}
					}
				}).Disabled(a.busy)
				ui.Text(c, locales.ModelSize(float64(model.Size())/1e9))
				ui.Spacer(c)
				state := a.modelStates[model.ID]
				if !downloaded && (state == "" || state == locales.ModelStateVerified()) {
					state = locales.ModelNotDownloaded()
				} else if downloaded && state == "" {
					state = locales.ModelDownloaded()
				}
				ui.Badge(c, state).Label(locales.A11yModelState())
			})
			path := a.modelPath(model.ID)
			ui.TextInput(c, &path).ReadOnly(true).Label(locales.A11yModelFolder()).Tooltip(path)
		})
	}, func() {
		ui.Button(c, locales.ModelDelete()).TextColor(c.Theme().Danger).Disabled(a.busy || assetErr != nil || len(assets) == 0).OnClick(func() { a.modelRecycle = model.ID })
		ui.Button(c, locales.ModelChooseFolder()).Label(locales.A11yModelChooseFolder()).Disabled(a.busy).OnClick(func() { a.pickModelDir(model) })
		ui.Button(c, locales.ModelVerify()).Disabled(a.busy).OnClick(func() { a.verifyModel(model) })
		ui.Button(c, locales.ActionClose()).OnClick(func() { a.modelOpen = false })
		downloadWidth, _ := c.MeasureText(0, ui.Span{Text: locales.ModelDownload()})
		readyWidth, _ := c.MeasureText(0, ui.Span{Text: locales.ModelDownloaded()})
		width := max(downloadWidth, readyWidth) + em(c, iconSize) + c.Theme().Space(8.5) + 2
		if downloaded {
			iconButton(c, locales.ModelDownloaded(), "download").Width(width).Label(locales.ModelDownload()).Description(locales.ModelDownloaded()).Disabled(true)
		} else {
			primaryIconButton(c, locales.ModelDownload(), "download").Width(width).Disabled(a.busy).OnClick(func() { a.downloadModel(model) })
		}
	})
}
