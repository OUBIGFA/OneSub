package main

import (
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/models"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	lobeNav "github.com/lobehub/lobe-mygo/pkg/nav"
)

// Settings share the same native fieldset hierarchy as the workbench. The
// section heading and spacing do the grouping; no nested cards or extra rules.
func settingsGroup(c *ui.Context, title string, content func()) {
	lobeGroup(c, title, nil, content)
}

// Settings are read as rows: a stable label edge and a stable control edge.
// Field retains the native label/control relationship and keyboard behavior.
func settingsRow(c *ui.Context, label string, control func() ui.Element) {
	ui.Field(c, label, func() { control().Width(em(c, 260)) }).Row().Justify(ui.SpaceBetween).
		AlignItems(ui.Center).Gap(em(c, 24)).Padding(em(c, 10), 0).MinHeight(em(c, 52))
	separator(c)
}

// Settings use one sidebar layout. The active form has explicit native grid
// columns, so opening it never waits for previous-frame bounds to settle.
func (a *NativeApp) settingsView(c *ui.Context) {
	wasOpen := a.settingsOpen
	_, height := c.Size()
	// Pages keep their numbers; models by language (5) follows recognition.
	labels := []string{locales.SettingsGeneral(), locales.SettingsRecognition(), locales.SettingsTranslation(), locales.SettingsTranslationRules(), locales.SettingsMcp(), locales.SettingsLanguageRoutes(), locales.SettingsAbout()}
	pageIcons := []string{"settings", "audio-lines", "languages", "file-down", "settings", "text-search", "info"}
	order := []int{0, 1, 5, 2, 3, 4, 6}
	fixedDialog(c, &a.settingsOpen, locales.SettingsTitle(), 1280, 0, func() {
		// A viewport-relative floor gives sparse pages the same generous
		// reading area; larger forms still determine the natural height.
		ui.Row(c).MinHeight(height * .8).Gap(0).AlignItems(ui.Stretch).Children(func() {
			selected := 0
			tabs := make([]lobeNav.TabItem, len(order))
			for i, tab := range order {
				tabs[i] = lobeNav.TabItem{Label: labels[tab], Icon: icons[pageIcons[tab]]}
				if tab == a.settingsTab {
					selected = i
				}
			}
			navigationWidth := float32(0)
			for _, label := range labels {
				w, _ := c.MeasureText(0, ui.Span{Text: label})
				navigationWidth = max(navigationWidth, w)
			}
			lobeNav.Tabs(c, &selected, lobeNav.TabsProps{Items: tabs, Vertical: true, Variant: lobeNav.TabsRounded}, nil).
				Width(max(em(c, 200), navigationWidth+em(c, 88))).Padding(em(c, 16), em(c, 12)).
				Label(locales.A11ySettingsTabs()).OnChange(func() { a.settingsTab = order[selected] })
			separator(c)
			// All pages occupy one native grid cell. Invisible pages still
			// contribute their intrinsic size, but cannot paint or take focus.
			// The largest page therefore sets the shell in this same frame.
			ui.Grid(c).Columns(1).Grow(1).MinWidth(0).Padding(em(c, 24)).Children(func() {
				for _, tab := range order {
					page := ui.Column(c.Key(tab)).ColumnStart(1).RowStart(1).Gap(em(c, 28))
					if tab != a.settingsTab {
						page.Invisible()
					} else {
						page.Label(locales.A11ySettingsContent())
					}
					page.Children(func() { a.settingsContent(c, tab) })
				}
			})
		})
	}, nil)
	if wasOpen && !a.settingsOpen {
		a.providerKey, a.providerKeyVisible = "", false
		a.saveSettings()
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
			ui.Column(c).Children(func() {
				settingsRow(c, locales.SettingsLanguage(), func() ui.Element {
					return idSelect(c, &a.settings.UILanguage, interfaceLanguages()).Label(locales.SettingsLanguage()).OnChange(func() {
						mygo.App.SetLocale(a.settings.UILanguage)
						a.saveSettings()
					})
				})
				settingsRow(c, locales.SettingsAppearance(), func() ui.Element {
					return idSelect(c, &a.settings.Appearance, appearances()).Label(locales.SettingsAppearance()).OnChange(func() {
						mygo.Theme.SetSource(mygo.ThemeSource(a.settings.Appearance))
						a.saveSettings()
					})
				})
				settingsRow(c, locales.SettingsCloseWindow(), func() ui.Element {
					return idSelect(c, &behavior, []option{{"tray", locales.SettingsCloseTray()}, {"exit", locales.SettingsCloseExit()}}).Label(locales.SettingsCloseWindow()).OnChange(func() {
						a.settings.CloseToTray = behavior == "tray"
						a.saveSettings()
					})
				})
			})
		})
		settingsGroup(c, locales.SettingsPlayback(), func() {
			ui.Column(c).Children(func() {
				for _, field := range []struct {
					label string
					value *string
				}{{locales.SettingsSingleClick(), &a.settings.SingleClickAction}, {locales.SettingsDoubleClick(), &a.settings.DoubleClickAction}} {
					settingsRow(c, field.label, func() ui.Element {
						return idSelect(c, field.value, subtitleActions()).Label(field.label).OnChange(a.saveSettings)
					})
				}
				lobeCheckbox(c, &a.settings.FollowPlayback, locales.SettingsFollowPlayback()).Margin(em(c, 12), 0, 0, 0).OnChange(func() { a.activeCue = -1; a.saveSettings() })
			})
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
				lobeCheckbox(c, &a.settings.VocalSeparation, locales.SettingsVocalSeparation()).OnChange(a.saveSettings)
				a.recognitionModelField(c, locales.ModelKindHotword(), "hotword", &hotword)
				ui.Field(c, locales.SettingsHotwords(), func() {
					lobeTextInput(c, &a.settings.Hotwords).Placeholder(locales.SettingsHotwordsPlaceholder()).Label(locales.SettingsHotwords()).OnChange(a.saveSettings)
				})
			})
		})
	case 2:
		a.translationSettingsView(c)
	case 3:
		a.translationRulesView(c)
	case 4:
		a.mcpSettingsView(c)
	case 6:
		a.aboutSettingsView(c)
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
		lobeButton(c, locales.SettingsMcpCopy()).AlignSelf(ui.Start).OnClick(func() {
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
			lobeButton(c, locales.ModelManage()).Label(locales.A11yManageModel(title)).OnClick(func() { a.modelSelection = *id; a.modelOpen = true })
		})
		// The explanation is immediately below the selected model, before
		// its folder. Native text wraps instead of hiding the useful hint.
		muted(c, modelSummary(*id)).FontSize(em(c, captionSize)).MinWidth(0)
		path := a.modelPath(*id)
		elidedText(c, path).TextColor(c.Theme().TextMuted).FontSize(em(c, captionSize))
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
		confirmationDialog(c, &a.modelOpen, locales.ModelFiles(), err.Error(), locales.ActionClose())
		return
	}
	downloaded := model.FilesPresent(a.modelPath(model.ID))
	assets, assetErr := modelAssetPaths(model, a.modelPath(model.ID))
	fixedDialog(c, &a.modelOpen, model.Label, 720, 0, func() {
		settingsGroup(c, locales.ModelFiles(), func() {
			ui.Row(c).Gap(em(c, 16)).Wrap().AlignItems(ui.Center).Children(func() {
				catalog := make([]menuCommand, 0, len(models.Catalog()))
				for _, candidate := range models.Catalog() {
					label := candidate.Label
					if !candidate.FilesPresent(a.modelPath(candidate.ID)) {
						label += " · " + locales.ModelNotDownloaded()
					}
					catalog = append(catalog, menuCommand{Key: candidate.ID, Label: label, Checkable: true, Checked: candidate.ID == model.ID, Run: func() { a.modelSelection = candidate.ID }})
				}
				lobeMenuButton(c, locales.ModelCatalog(), "chevron-down", false, func() []menuCommand { return catalog }).Disabled(a.busy)
				ui.Text(c, locales.ModelSize(float64(model.Size())/1e9))
				ui.Spacer(c)
				state := a.modelStates[model.ID]
				if !downloaded && (state == "" || state == locales.ModelStateVerified()) {
					state = locales.ModelNotDownloaded()
				} else if downloaded && state == "" {
					state = locales.ModelDownloaded()
				}
				lobeBadge(c, state).Label(locales.A11yModelState())
			})
			path := a.modelPath(model.ID)
			lobeTextInput(c, &path).ReadOnly(true).Label(locales.A11yModelFolder()).Tooltip(path)
		})
	}, func() {
		lobeButton(c, locales.ModelDelete()).TextColor(c.Theme().Danger).Disabled(a.busy || assetErr != nil || len(assets) == 0).OnClick(func() { a.modelRecycle = model.ID })
		ui.Spacer(c)
		lobeButton(c, locales.ModelChooseFolder()).Label(locales.A11yModelChooseFolder()).Disabled(a.busy).OnClick(func() { a.pickModelDir(model) })
		lobeButton(c, locales.ModelVerify()).Disabled(a.busy).OnClick(func() { a.verifyModel(model) })
		lobeButton(c, locales.ActionClose()).OnClick(func() { a.modelOpen = false })
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
