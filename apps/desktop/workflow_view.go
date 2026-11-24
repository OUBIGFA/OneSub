package main

import (
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/models"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// The combined action has its own fixed band above the two workflow tabs.
// Only the three main columns resize; this band follows its content height.
func (a *NativeApp) workflowColumn(c *ui.Context) {
	ui.Column(c).Fill().Label(locales.A11yWorkflowColumn()).Children(func() {
		ui.Column(c).Shrink(0).Padding(em(c, 12), em(c, panelPad)).Background(studioColors(c).panel).Children(func() {
			label := a.processingLabel()
			lobePrimaryButton(c, "").Label(label).MinHeight(em(c, 44)).Padding(em(c, 10), em(c, 12)).Justify(ui.Center).
				Disabled(a.busy || a.snapshot == nil || a.snapshot.Manifest.MediaPath == "").OnClick(a.processMedia).Children(func() {
				icon(c, "languages").Shrink(0)
				ui.Text(c, label).MinWidth(0).TextAlign(ui.Center)
			})
		})
		separator(c)
		ui.Column(c).Grow(1).MinHeight(0).Children(func() { a.workflowPanel(c) })
	})
}

// The command band stays visible while the full-height inspector scrolls.
// Decoding and scheduling remain expanded below their common options.
func (a *NativeApp) workflowPanel(c *ui.Context) {
	panel := ui.Column(c).Fill().Label(locales.WorkflowTitle())
	panel.Children(func() {
		panelHeader(c, nil, func() {
			lobeTabs(c, &a.workflowTab, locales.WorkflowRecognize(), locales.WorkflowTranslate()).Label(locales.WorkflowTitle()).OnChange(func() { a.workflowScroll.Y = 0 })
		}, nil)
		separator(c)
		ui.Column(c).Padding(em(c, 12), em(c, panelPad)).Gap(em(c, 8)).Shrink(0).Background(studioColors(c).panel).Children(func() {
			if a.workflowTab == 0 {
				segmentedFill(c, &a.recognitionMode, locales.RecognitionCurrentFile(), locales.RecognitionFileQueue()).Label(locales.RecognitionTarget())
				hasMedia := a.snapshot != nil && a.snapshot.Manifest.MediaPath != ""
				manualUnavailable := a.settings.RecognitionMode == recognitionManual && !a.selectedAsrDownloaded()
				if a.recognitionMode == 1 {
					startButton(c, locales.BatchStart(), "audio-lines", a.busy || a.pendingBatchCount() == 0 || manualUnavailable, a.startBatchRecognition)
				} else {
					startButton(c, locales.WorkflowStartRecognize(), "audio-lines", a.busy || !hasMedia || manualUnavailable, a.recognize)
				}
			} else {
				hasCues := a.snapshot != nil && len(a.snapshot.Document.Cues) > 0
				startButton(c, locales.WorkflowStartTranslate(), "languages", a.busy || !hasCues, a.translate)
			}
		})
		separator(c)
		// Queue settings live on Current file. Give the queue the remaining
		// viewport directly so only its list scrolls, not another inspector.
		if a.workflowTab == 0 && a.recognitionMode == 1 {
			a.batchRecognitionView(c)
			return
		}
		ui.Scroll(c.Key(a.workflowTab)).Grow(1).MinHeight(0).TrackScroll(&a.workflowScroll).Children(func() {
			ui.Column(c).Padding(em(c, 14), em(c, panelPad), em(c, 20), em(c, panelPad)).Gap(em(c, 20)).Children(func() {
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
	hint := ""
	if a.snapshot == nil || a.snapshot.Manifest.MediaPath == "" {
		hint = locales.WorkflowNeedsVideo()
	} else if !a.busy && a.settings.Processing != nil && a.settings.Processing.Stage == "recognize" {
		hint = locales.RecognitionRestartNote()
	}
	if hint != "" {
		ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Children(func() {
			icon(c, "info").TextColor(c.Theme().TextMuted).Shrink(0)
			muted(c, hint).Grow(1).MinWidth(0)
		})
	}
	routed := a.settings.RecognitionMode != recognitionManual
	parameterGroup(c, locales.WorkflowRecognitionSettings(), func() {
		// Smart routing's models are set by language; a chosen model in the
		// models page.
		page := 1
		if routed {
			page = 5
		}
		toolIcon(c, locales.WorkflowModels(), "settings").OnClick(func() { a.settingsOpen, a.settingsTab = true, page })
	}, func() {
		ui.Field(c, locales.RecognitionMode(), func() {
			idSelect(c, &a.settings.RecognitionMode, []option{{recognitionRouted, locales.RecognitionModeRouted()}, {recognitionManual, locales.RecognitionModeManual()}}).Label(locales.RecognitionMode()).Disabled(a.busy).OnChange(func() {
				// A chosen model keeps only a language it recognizes.
				a.asrModelChanged()
			})
		})
		if routed {
			return
		}
		parameterGrid(c, 210, 2, func() {
			ui.Field(c, locales.WorkflowRecognitionModel(), func() {
				// Management retains the full catalog; recognition lists only
				// complete local model files and never exposes a missing choice.
				options := a.downloadedAsrModels()
				placeholder := locales.RecognitionChooseDownloadedModel()
				if len(options) == 0 {
					placeholder = locales.RecognitionNoDownloadedModel()
				}
				chosen := a.settings.AsrModel
				idSelect(c, &chosen, options, placeholder).Label(locales.WorkflowRecognitionModel()).Disabled(a.busy || len(options) == 0).OnChange(func() {
					a.settings.AsrModel = chosen
					a.asrModelChanged()
				})
			})
		}, func() {
			ui.Field(c, locales.SettingsSourceLanguage(), func() {
				languageSelect(c, &a.settings.Language, modelLanguages(a.settings.AsrModel), a.busy || !a.selectedAsrDownloaded()).Label(locales.SettingsSourceLanguage()).OnChange(a.sourceLanguageChanged)
			})
		})
	})
	parameterGroup(c, locales.WorkflowOutput(), nil, func() {
		ui.Field(c, locales.ExportFormat(), func() {
			idSelect(c, &a.settings.RecognitionFormat, []option{{"srt", "SRT"}, {"vtt", "VTT"}, {"ass", "ASS"}}).Label(locales.ExportFormat()).OnChange(a.saveSettings)
		})
		folderField(c, a.settings.RecognitionDir, locales.RecognitionBesideMedia(), a.busy, a.pickRecognitionDir, func() { a.settings.RecognitionDir = ""; a.saveSettings() })
	})
	// Backend, chunks and tokens are OneAsr's options; a sherpa-onnx model
	// runs on the CPU in inputs of its own length. Smart routing may choose
	// either, so they stay on.
	oneAsr := true
	if model, err := models.ByID(a.settings.AsrModel); err == nil && !routed {
		oneAsr = !model.Sherpa()
	}
	parameterGroup(c, locales.WorkflowRecognitionAdvanced(), nil, func() {
		parameterGrid(c, 170, 3, func() {
			ui.Field(c, locales.SettingsBackend(), func() {
				idSelect(c, &a.settings.Backend, []option{{"auto", locales.BackendAuto()}, {"gpu", "GPU"}, {"cpu", "CPU"}}).Label(locales.SettingsBackend()).Disabled(!oneAsr).OnChange(a.saveSettings)
			})
		}, func() {
			ui.Field(c, locales.SettingsChunk(), func() {
				ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Children(func() {
					lobeNumberInput(c, &a.settings.ChunkSeconds, 30, 180, 1, !oneAsr, a.saveSettings).Label(locales.SettingsChunk())
					muted(c, locales.UnitSeconds())
				})
			})
		}, func() {
			ui.Field(c, locales.SettingsMaxTokens(), func() {
				lobeNumberInput(c, &a.settings.MaxTokens, 1, 32768, 1, !oneAsr, a.saveSettings).Label(locales.SettingsMaxTokens())
			})
		})
	})
}

func folderField(c *ui.Context, dir, fallback string, busy bool, choose, reset func()) {
	ui.Field(c, locales.RecognitionDirectory(), func() {
		ui.Row(c).Gap(em(c, 8)).Wrap().AlignItems(ui.Center).Children(func() {
			if dir == "" {
				muted(c, fallback).Grow(1).Basis(em(c, 180)).MinWidth(0)
			} else {
				lobeTextInput(c, &dir).ReadOnly(true).Grow(1).Basis(em(c, 180)).MinWidth(0).Label(locales.RecognitionDirectory()).Tooltip(dir)
			}
			// The actions are direct items of the wrapping row: in a narrow
			// pane each long translated label takes its own line; neither is cut.
			lobeButton(c, locales.RecognitionChooseFolder()).Shrink(0).Disabled(busy).OnClick(choose)
			lobeButton(c, locales.RecognitionResetFolder()).Shrink(0).Disabled(busy || dir == "").OnClick(reset)
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
		}, func() {
			ui.Field(c, locales.TranslationModel(), func() { a.providerModelSelect(c) })
		})
	})
	parameterGroup(c, locales.WorkflowTranslationSettings(), nil, func() {
		parameterGrid(c, 210, 2, func() {
			ui.Field(c, locales.TranslationTarget(), func() {
				languageSelect(c, &a.settings.TargetLanguage, targetLanguages(), a.busy).Label(locales.TranslationTarget()).OnChange(a.saveSettings)
			})
		}, func() {
			ui.Field(c, locales.TranslationStyle(), func() { a.styleSelect(c).Disabled(a.busy) }).Error(a.profilesError)
		})
		folderField(c, a.settings.TranslationDir, locales.TranslationBesideSource(), a.busy, a.pickTranslationDir, func() { a.settings.TranslationDir = ""; a.saveSettings() })
	})
	parameterGroup(c, locales.WorkflowTranslationAdvanced(), nil, func() {
		// Scheduling is a pair of peer fields. Punctuation has its own
		// group, with the exception field indented under its controlling check.
		// The fields sit flat in the group, as in the other groups: no card.
		ui.Column(c).Gap(em(c, 16)).Children(func() {
			cells := []func(){}
			for _, field := range []struct {
				label string
				value *int
			}{
				{locales.TranslationConcurrency(), &a.settings.TranslationConcurrency},
				{locales.TranslationBatchSize(), &a.settings.TranslationBatchSize},
			} {
				cells = append(cells, func() {
					ui.Field(c, field.label, func() {
						value := float64(*field.value)
						number(c, field.label, &value, 1, 10000, 1, func() { *field.value = int(value); a.saveSettings() })
					})
				})
			}
			parameterGrid(c, 160, 2, cells...)
			separator(c)
			ui.Column(c).Gap(em(c, 12)).Children(func() {
				lobeCheckbox(c, &a.settings.StripPunctuation, locales.WorkflowStripPunctuation()).OnChange(a.saveSettings)
				ui.Field(c, locales.WorkflowKeepPunctuation(), func() {
					lobeTextInput(c, &a.settings.KeepPunctuation).Width(em(c, 120)).Placeholder("?").Label(locales.WorkflowKeepPunctuation()).Disabled(!a.settings.StripPunctuation).OnChange(a.saveSettings)
				}).Gap(em(c, 8)).Margin(0, 0, 0, em(c, 24))
			})
			lobeCheckbox(c, &a.settings.ReplacePeriods, locales.WorkflowReplacePeriods()).Tooltip(locales.WorkflowReplacePeriodsHint()).OnChange(a.saveSettings)
		})
	})
}

// This is a native toggle with a prominent pressed state, not a second submit
// button. Native focus, keyboard and accessibility semantics remain intact.
func (a *NativeApp) timelineRebuildChoice(c *ui.Context) {
	t := c.Theme()
	hint := locales.TranslationOptimizeTimelineHint(locales.TabRebuilt())
	choice := lobeToggle(c, &a.settings.OptimizeTimeline, "").Label(locales.TranslationOptimizeTimeline()).Description(hint).
		MinHeight(em(c, 88)).Padding(em(c, 12)).Gap(em(c, 8)).
		Justify(ui.Start).AlignItems(ui.Center).Disabled(a.busy).OnChange(a.saveSettings)
	choice.TextColor(t.Text)
	dotColor, iconColor := t.Border, t.TextMuted
	if a.settings.OptimizeTimeline {
		dotColor, iconColor = t.Accent, t.Accent
		choice.Background(studioColors(c).selected).Border(1, t.Accent)
		if choice.Hovered() && !a.busy {
			choice.Background(t.Accent.Alpha(0.18))
		}
	} else {
		choice.Border(1, studioColors(c).hairline)
	}
	if a.busy {
		iconColor = t.TextMuted
	}
	choice.Children(func() {
		ui.Box(c).Absolute().Top(em(c, 10)).Left(em(c, 10)).Size(em(c, 6), em(c, 6)).Radius(em(c, 3)).Background(dotColor).IgnoreInput(true)
		icon(c, "scissors").TextColor(iconColor).Shrink(0)
		ui.Column(c).Grow(1).MinWidth(0).Gap(em(c, 6)).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.TranslationOptimizeTimeline()).FontSize(em(c, bodySize)).FontWeight(500).SingleLine()
			muted(c, hint).FontSize(em(c, captionSize)).TextAlign(ui.Center)
		})
	})
}

func startButton(c *ui.Context, label, name string, disabled bool, run func()) {
	// The blue command runs both stages. A single-stage command stays visible
	// below its tab with a neutral outline and its full action name.
	lobeButton(c, "").Label(label).MinHeight(em(c, 40)).Justify(ui.Center).Padding(em(c, 8), em(c, 16)).Disabled(disabled).OnClick(run).Children(func() {
		icon(c, name)
		ui.Text(c, label).SingleLine()
	})
}
