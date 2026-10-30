package main

import (
	"strings"
	"unicode"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/models"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// Both task pages reserve the taller layout, so switching tabs neither moves
// the split nor inherits an out-of-range scroll offset. Accounts stay in
// settings; the options used by the next task are directly on the workbench.
func (a *NativeApp) workflowPanel(c *ui.Context) {
	ui.Column(c).Fill().Label(locales.WorkflowTitle()).Children(func() {
		panelHeader(c, nil, func() {
			ui.Segmented(c, &a.workflowTab, locales.WorkflowRecognize(), locales.WorkflowTranslate()).Label(locales.WorkflowTitle())
		}, nil)
		ui.Divider(c)
		ui.Scroll(c).Grow(1).TrackScroll(&a.workflowScroll).Children(func() {
			content := ui.Grid(c).Columns(1).Padding(em(c, 12), em(c, panelPad)).Children(func() {
				for tab := 0; tab < 2; tab++ {
					page := ui.Column(c.Key(tab)).RowStart(1).ColumnStart(1).Gap(em(c, 20)).AlignSelf(ui.Start)
					if tab != a.workflowTab {
						page.Invisible()
					}
					page.Children(func() {
						if tab == 0 {
							a.recognizeTab(c)
						} else {
							a.translateTab(c)
						}
					})
				}
			})
			measurePanelContent(c, content, &a.workflowHeight)
		})
	})
}

func (a *NativeApp) recognizeTab(c *ui.Context) {
	hasMedia := a.snapshot != nil && a.snapshot.Manifest.MediaPath != ""
	startButton(c, locales.WorkflowStartRecognize(), "audio-lines", a.busy || !hasMedia, a.recognize)
	if !hasMedia {
		muted(c, locales.WorkflowNeedsVideo())
	} else if !a.busy && a.settings.Processing != nil && a.settings.Processing.Stage == "recognize" {
		muted(c, locales.RecognitionRestartNote())
	}
	parameterGroup(c, locales.WorkflowRecognitionSettings(), func() {
		toolIcon(c, locales.WorkflowModels(), "settings").OnClick(func() { a.settingsOpen, a.settingsTab = true, 1 })
	}, func() {
		line(c, func() {
			ui.Field(c, locales.ModelKindAsr(), func() {
				options := []option{}
				for _, model := range models.Catalog() {
					if model.Kind == "asr" {
						options = append(options, option{model.ID, model.Label})
					}
				}
				idSelect(c, &a.settings.AsrModel, options).Label(locales.ModelKindAsr()).OnChange(a.saveSettings)
			})
			ui.Field(c, locales.SettingsSourceLanguage(), func() {
				languageSelect(c, &a.settings.Language, sourceLanguages(), a.busy).Label(locales.SettingsSourceLanguage()).OnChange(a.saveSettings)
			}).MinWidth(em(c, 220))
			ui.Field(c, locales.SettingsBackend(), func() {
				idSelect(c, &a.settings.Backend, []option{{"auto", locales.BackendAuto()}, {"gpu", "GPU"}, {"cpu", "CPU"}}).Label(locales.SettingsBackend()).OnChange(a.saveSettings)
			})
			ui.Field(c, locales.SettingsChunk(), func() {
				ui.Row(c).Gap(em(c, 6)).AlignItems(ui.Center).Children(func() {
					number(c, locales.SettingsChunk(), &a.settings.ChunkSeconds, 30, 180, 1).OnChange(a.saveSettings)
					muted(c, locales.UnitSeconds()).SingleLine()
				})
			})
			ui.Field(c, locales.SettingsMaxTokens(), func() {
				number(c, locales.SettingsMaxTokens(), &a.settings.MaxTokens, 1, 32768, 1).OnChange(a.saveSettings)
			})
		})
	})
	parameterGroup(c, locales.WorkflowOutput(), nil, func() {
		line(c, func() {
			ui.Field(c, locales.ExportFormat(), func() {
				idSelect(c, &a.settings.RecognitionFormat, []option{{"srt", "SRT"}, {"vtt", "VTT"}, {"ass", "ASS"}}).Label(locales.ExportFormat()).OnChange(a.saveSettings)
			})
			folderField(c, a.settings.RecognitionDir, locales.RecognitionBesideMedia(), a.busy, a.pickRecognitionDir, func() { a.settings.RecognitionDir = ""; a.saveSettings() })
			ui.Row(c).Gap(em(c, 12)).Wrap().AlignItems(ui.Center).Children(func() {
				ui.Checkbox(c, &a.settings.StripPunctuation, locales.WorkflowStripPunctuation()).OnChange(a.saveSettings)
				ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Children(func() {
					muted(c, locales.WorkflowKeepPunctuation()).SingleLine()
					ui.TextInput(c, &a.settings.KeepPunctuation).Width(em(c, 80)).Placeholder("?").Label(locales.WorkflowKeepPunctuation()).Disabled(!a.settings.StripPunctuation).OnChange(a.saveSettings)
				})
			})
		})
	})
}

// line preserves each field as a group: related controls stay together, and
// only whole groups wrap when a pane cannot fit another one on the same row.
func line(c *ui.Context, fields func()) ui.Element {
	return ui.Row(c).GapX(em(c, 12)).GapY(em(c, 10)).Wrap().AlignItems(ui.End).Children(fields)
}

func folderField(c *ui.Context, dir, fallback string, busy bool, choose, reset func()) {
	ui.Field(c, locales.RecognitionDirectory(), func() {
		path := dir
		if path == "" {
			path = fallback
		}
		// Only user paths may elide; the default location and action names
		// stay complete. Reset has the same meaning for media and subtitles.
		ui.Row(c).Gap(em(c, 8)).Wrap().AlignItems(ui.Center).Children(func() {
			label := muted(c, path).SingleLine().Tooltip(path)
			if dir != "" {
				label.MaxWidth(em(c, 180))
			}
			ui.Row(c).Gap(em(c, 8)).Children(func() {
				ui.Button(c, locales.RecognitionChooseFolder()).Disabled(busy).OnClick(choose)
				ui.Button(c, locales.RecognitionResetFolder()).Disabled(busy || dir == "").OnClick(reset)
			})
		})
	}).AlignSelf(ui.Start)
}

func (a *NativeApp) translateTab(c *ui.Context) {
	hasCues := a.snapshot != nil && len(a.snapshot.Document.Cues) > 0
	startButton(c, locales.WorkflowStartTranslate(), "languages", a.busy || !hasCues, a.translate)
	if !hasCues {
		muted(c, locales.WorkflowNeedsSubtitles())
	}
	if a.translationProfiles == nil {
		a.reloadTranslationProfiles()
	}
	parameterGroup(c, locales.WorkflowAIService(), func() {
		toolIcon(c, locales.WorkflowService(), "settings").OnClick(func() { a.reloadTranslationProfiles(); a.settingsOpen, a.settingsTab = true, 2 })
	}, func() {
		line(c, func() {
			ui.Field(c, locales.TranslationProvider(), func() { a.providerSelect(c) })
			ui.Field(c, locales.TranslationModel(), func() { a.providerModelSelect(c).Width(em(c, 200)) })
			for _, field := range []struct {
				label string
				value *int
			}{
				{locales.TranslationConcurrency(), &a.settings.TranslationConcurrency},
				{locales.TranslationBatchSize(), &a.settings.TranslationBatchSize},
			} {
				ui.Field(c, field.label, func() {
					value := float64(*field.value)
					// Running tasks already own a settings snapshot. These changes
					// are saved for the next task, not applied to in-flight requests.
					number(c, field.label, &value, 1, 10000, 1).OnChange(func() { *field.value = int(value); a.saveSettings() })
				})
			}
		})
	})
	parameterGroup(c, locales.WorkflowTranslationSettings(), nil, func() {
		line(c, func() {
			ui.Field(c, locales.TranslationTarget(), func() {
				languageSelect(c, &a.settings.TargetLanguage, targetLanguages(), a.busy).Label(locales.TranslationTarget()).OnChange(a.saveSettings)
			}).MinWidth(em(c, 220))
			ui.Field(c, locales.TranslationStyle(), func() { a.styleSelect(c).Disabled(a.busy) }).MinWidth(em(c, 180))
			folderField(c, a.settings.TranslationDir, locales.TranslationBesideSource(), a.busy, a.pickTranslationDir, func() { a.settings.TranslationDir = ""; a.saveSettings() })
			ui.Checkbox(c, &a.settings.OptimizeTimeline, locales.TranslationOptimizeTimeline()).Disabled(a.busy).OnChange(a.saveSettings)
		})
	})
}

func startButton(c *ui.Context, label, name string, disabled bool, run func()) {
	ui.PrimaryButton(c, "").Label(label).Tooltip(label).Justify(ui.Center).Padding(em(c, 10), em(c, 16)).Disabled(disabled).OnClick(run).Children(func() {
		icon(c, name)
		ui.Text(c, label).SingleLine()
	})
}

// pairedMarks are the ASCII and full-width forms of the same mark: keeping
// one keeps both.
var pairedMarks = map[rune]rune{'?': '？', '!': '！', ',': '，', '.': '。', ';': '；', ':': '：', '？': '?', '！': '!', '，': ',', '。': '.', '；': ';', '：': ':'}

// stripTrailingPunctuation removes punctuation at the end of each line of
// text, except the marks in keep, in either width. Closing quotes and
// brackets stay, as removing them would leave their opening half alone.
func stripTrailingPunctuation(text, keep string) string {
	kept := map[rune]bool{}
	for _, r := range keep {
		kept[r] = true
		if p, ok := pairedMarks[r]; ok {
			kept[p] = true
		}
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		runes := []rune(strings.TrimRight(line, " \t\r"))
		end := len(runes)
		for end > 0 {
			r := runes[end-1]
			if !unicode.IsPunct(r) || kept[r] || unicode.In(r, unicode.Pe, unicode.Pf) || r == '"' || r == '\'' {
				break
			}
			end--
		}
		if end > 0 {
			lines[i] = string(runes[:end])
		} else {
			lines[i] = strings.TrimRight(line, " \t\r")
		}
	}
	return strings.Join(lines, "\n")
}
