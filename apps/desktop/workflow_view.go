package main

import (
	"strings"
	"unicode"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/models"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// The command band stays visible while the inspector scrolls. Common options
// have a deliberate two-column density; decoding and scheduling are secondary
// disclosures, not equally weighted controls in one long wrapping row.
func (a *NativeApp) workflowPanel(c *ui.Context) {
	panel := ui.Column(c).Fill().Label(locales.WorkflowTitle())
	panel.Children(func() {
		panelHeader(c, nil, func() {
			ui.Tabs(c, &a.workflowTab, locales.WorkflowRecognize(), locales.WorkflowTranslate()).Label(locales.WorkflowTitle()).OnChange(func() { a.workflowScroll.Y = 0 })
		}, nil)
		ui.Divider(c)
		ui.Column(c).Padding(em(c, 16), em(c, panelPad)).Gap(em(c, 12)).Shrink(0).Children(func() {
			if a.workflowTab == 0 {
				ui.Segmented(c, &a.recognitionMode, locales.RecognitionCurrentFile(), locales.RecognitionFileQueue()).AlignSelf(ui.Center).Label(locales.RecognitionTarget())
				hasMedia := a.snapshot != nil && a.snapshot.Manifest.MediaPath != ""
				if a.recognitionMode == 1 {
					startButton(c, locales.BatchStart(), "audio-lines", a.busy || a.pendingBatchCount() == 0, a.startBatchRecognition)
				} else {
					startButton(c, locales.WorkflowStartRecognize(), "audio-lines", a.busy || !hasMedia, a.recognize)
				}
			} else {
				hasCues := a.snapshot != nil && len(a.snapshot.Document.Cues) > 0
				startButton(c, locales.WorkflowStartTranslate(), "languages", a.busy || !hasCues, a.translate)
			}
		})
		// Queue settings live on Current file. Give the queue the remaining
		// viewport directly so only its list scrolls, not another inspector.
		if a.workflowTab == 0 && a.recognitionMode == 1 {
			a.batchRecognitionView(c)
			return
		}
		ui.Scroll(c.Key(a.workflowTab)).Grow(1).MinHeight(0).TrackScroll(&a.workflowScroll).Children(func() {
			ui.Column(c).Padding(0, em(c, panelPad), em(c, 24), em(c, panelPad)).Gap(em(c, 28)).Children(func() {
				if a.workflowTab == 0 {
					a.recognizeTab(c)
				} else {
					a.translateTab(c)
				}
			})
		})
	})
	if paths := panel.DroppedFiles(); len(paths) > 0 {
		a.acceptFiles(paths)
	}
}

func (a *NativeApp) recognizeTab(c *ui.Context) {
	if a.snapshot == nil || a.snapshot.Manifest.MediaPath == "" {
		muted(c, locales.WorkflowNeedsVideo())
	} else if !a.busy && a.settings.Processing != nil && a.settings.Processing.Stage == "recognize" {
		muted(c, locales.RecognitionRestartNote())
	}
	parameterGroup(c, locales.WorkflowRecognitionSettings(), func() {
		toolIcon(c, locales.WorkflowModels(), "settings").OnClick(func() { a.settingsOpen, a.settingsTab = true, 1 })
	}, func() {
		parameterGrid(c, 210, 2, func() {
			ui.Field(c, locales.WorkflowRecognitionModel(), func() {
				options := []option{}
				for _, model := range models.Catalog() {
					if model.Kind == "asr" {
						options = append(options, option{model.ID, model.Label})
					}
				}
				idSelect(c, &a.settings.AsrModel, options).Label(locales.WorkflowRecognitionModel()).OnChange(a.saveSettings)
			})
			ui.Field(c, locales.SettingsSourceLanguage(), func() {
				languageSelect(c, &a.settings.Language, sourceLanguages(), a.busy).Label(locales.SettingsSourceLanguage()).OnChange(a.saveSettings)
			})
		})
	})
	parameterGroup(c, locales.WorkflowOutput(), nil, func() {
		ui.Field(c, locales.ExportFormat(), func() {
			idSelect(c, &a.settings.RecognitionFormat, []option{{"srt", "SRT"}, {"vtt", "VTT"}, {"ass", "ASS"}}).Label(locales.ExportFormat()).OnChange(a.saveSettings)
		})
		folderField(c, a.settings.RecognitionDir, locales.RecognitionBesideMedia(), a.busy, a.pickRecognitionDir, func() { a.settings.RecognitionDir = ""; a.saveSettings() })
	})
	ui.Collapsible(c, locales.WorkflowRecognitionAdvanced(), &a.recognitionAdvanced, func() {
		parameterGrid(c, 170, 3, func() {
			ui.Field(c, locales.SettingsBackend(), func() {
				idSelect(c, &a.settings.Backend, []option{{"auto", locales.BackendAuto()}, {"gpu", "GPU"}, {"cpu", "CPU"}}).Label(locales.SettingsBackend()).OnChange(a.saveSettings)
			})
			ui.Field(c, locales.SettingsChunk(), func() {
				ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Children(func() {
					number(c, locales.SettingsChunk(), &a.settings.ChunkSeconds, 30, 180, 1).OnChange(a.saveSettings)
					muted(c, locales.UnitSeconds())
				})
			})
			ui.Field(c, locales.SettingsMaxTokens(), func() {
				number(c, locales.SettingsMaxTokens(), &a.settings.MaxTokens, 1, 32768, 1).OnChange(a.saveSettings)
			})
		})
		ui.Divider(c)
		ui.Checkbox(c, &a.settings.StripPunctuation, locales.WorkflowStripPunctuation()).OnChange(a.saveSettings)
		ui.Field(c, locales.WorkflowKeepPunctuation(), func() {
			ui.TextInput(c, &a.settings.KeepPunctuation).Width(em(c, 120)).Placeholder("?").Label(locales.WorkflowKeepPunctuation()).Disabled(!a.settings.StripPunctuation).OnChange(a.saveSettings)
		})
	})
}

func folderField(c *ui.Context, dir, fallback string, busy bool, choose, reset func()) {
	ui.Field(c, locales.RecognitionDirectory(), func() {
		ui.Row(c).Gap(em(c, 12)).Wrap().AlignItems(ui.Center).Children(func() {
			if dir == "" {
				muted(c, fallback).Grow(1).Basis(em(c, 180)).MinWidth(0)
			} else {
				ui.TextInput(c, &dir).ReadOnly(true).Grow(1).Basis(em(c, 180)).MinWidth(0).Label(locales.RecognitionDirectory()).Tooltip(dir)
			}
			ui.Row(c).Gap(em(c, 8)).Shrink(0).Children(func() {
				ui.Button(c, locales.RecognitionChooseFolder()).Disabled(busy).OnClick(choose)
				ui.Button(c, locales.RecognitionResetFolder()).Disabled(busy || dir == "").OnClick(reset)
			})
		})
	})
}

func (a *NativeApp) translateTab(c *ui.Context) {
	a.timelineRebuildChoice(c)
	if a.translationProfiles == nil {
		a.reloadTranslationProfiles()
	}
	parameterGroup(c, locales.WorkflowAIService(), func() {
		toolIcon(c, locales.WorkflowService(), "settings").OnClick(func() { a.reloadTranslationProfiles(); a.settingsOpen, a.settingsTab = true, 2 })
	}, func() {
		parameterGrid(c, 190, 2, func() {
			ui.Field(c, locales.TranslationProvider(), func() { a.providerSelect(c) })
			ui.Field(c, locales.TranslationModel(), func() { a.providerModelSelect(c) })
		})
	})
	parameterGroup(c, locales.WorkflowTranslationSettings(), nil, func() {
		parameterGrid(c, 210, 2, func() {
			ui.Field(c, locales.TranslationTarget(), func() {
				languageSelect(c, &a.settings.TargetLanguage, targetLanguages(), a.busy).Label(locales.TranslationTarget()).OnChange(a.saveSettings)
			})
			ui.Field(c, locales.TranslationStyle(), func() { a.styleSelect(c).Disabled(a.busy) })
		})
		folderField(c, a.settings.TranslationDir, locales.TranslationBesideSource(), a.busy, a.pickTranslationDir, func() { a.settings.TranslationDir = ""; a.saveSettings() })
	})
	ui.Collapsible(c, locales.WorkflowTranslationAdvanced(), &a.translationAdvanced, func() {
		parameterGrid(c, 180, 2, func() {
			for _, field := range []struct {
				label string
				value *int
			}{
				{locales.TranslationConcurrency(), &a.settings.TranslationConcurrency},
				{locales.TranslationBatchSize(), &a.settings.TranslationBatchSize},
			} {
				ui.Field(c, field.label, func() {
					value := float64(*field.value)
					number(c, field.label, &value, 1, 10000, 1).OnChange(func() { *field.value = int(value); a.saveSettings() })
				})
			}
		})
	})
}

// This is a native toggle with a prominent pressed state, not a second submit
// button. Native focus, keyboard and accessibility semantics remain intact.
func (a *NativeApp) timelineRebuildChoice(c *ui.Context) {
	t := c.Theme()
	hint := locales.TranslationOptimizeTimelineHint(locales.TabRebuilt())
	choice := ui.Toggle(c, &a.settings.OptimizeTimeline, "").Label(locales.TranslationOptimizeTimeline()).Description(hint).
		MinHeight(em(c, 88)).Padding(em(c, 16)).Gap(em(c, 12)).
		Justify(ui.Start).AlignItems(ui.Center).Disabled(a.busy).OnChange(a.saveSettings)
	state := locales.TranslationTimelineOff()
	if a.settings.OptimizeTimeline {
		state = locales.TranslationTimelineOn()
		choice.Background(studioColors(c).selected).Border(1, t.Accent)
		if choice.Hovered() && !a.busy {
			choice.Background(t.Accent.Alpha(0.18))
		}
	}
	choice.Children(func() {
		icon(c, "audio-lines").Size(em(c, 24), em(c, 24)).Shrink(0)
		ui.Column(c).Grow(1).MinWidth(0).Gap(em(c, 6)).Children(func() {
			ui.Text(c, locales.TranslationOptimizeTimeline()).FontSize(em(c, 17)).FontWeight(600)
			muted(c, hint).FontSize(em(c, captionSize))
		})
		ui.Badge(c, state).Shrink(0)
	})
}

func startButton(c *ui.Context, label, name string, disabled bool, run func()) {
	ui.PrimaryButton(c, "").Label(label).Tooltip(label).MinHeight(em(c, 42)).Justify(ui.Center).Padding(em(c, 10), em(c, 16)).Disabled(disabled).OnClick(run).Children(func() {
		icon(c, name)
		ui.Text(c, label).SingleLine()
	})
}

// Keeping either width of a punctuation mark keeps both.
var pairedMarks = map[rune]rune{'?': '？', '!': '！', ',': '，', '.': '。', ';': '；', ':': '：', '？': '?', '！': '!', '，': ',', '。': '.', '；': ';', '：': ':'}

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
