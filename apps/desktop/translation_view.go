package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/security"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/translator"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/google/uuid"
)

type translationCandidate struct {
	ProjectID string              `json:"projectId"`
	Revision  int64               `json:"baseRevision"`
	Result    translator.Result   `json:"result"`
	Checks    translator.Findings `json:"checks"`
}

// Providers have two working views: a full-width model library for everyday
// selection, and connection details for setup. Neither is squeezed beside the
// other. The fixed settings shell remains unchanged when switching views.
func (a *NativeApp) translationSettingsView(c *ui.Context) {
	ui.Column(c).Fill().Gap(em(c, 16)).Children(func() {
		ui.Row(c).Gap(em(c, 12)).AlignItems(ui.Center).Children(func() {
			a.providerSelect(c).Grow(1).MinWidth(0)
			lobeButton(c, locales.TranslationAddProvider()).OnClick(func() { a.addProvider(); a.providerSettingsTab = 1 })
			lobeButton(c, locales.TranslationDeleteProvider()).Disabled(a.settings.Provider() == nil).OnClick(func() { a.providerDelete = true })
		})
		ui.Row(c).Gap(em(c, 16)).AlignItems(ui.Center).Children(func() {
			lobeTabs(c, &a.providerSettingsTab, locales.TranslationModels(), locales.TranslationService()).OnChange(a.commitProviderModelInput)
			ui.Spacer(c)
			kept := 0
			if p := a.settings.Provider(); p != nil {
				kept = len(p.Models)
			}
			notice := locales.TranslationModelsKept(kept, len(a.providerModelCatalog()))
			if a.modelsLoading {
				notice = locales.TranslationModelsLoading()
			}
			if a.modelsError != "" {
				notice = locales.TranslationModelsFailed()
			}
			status := muted(c, notice).FontSize(em(c, captionSize)).Tooltip(a.modelsError)
			if a.modelsError != "" {
				status.TextColor(c.Theme().Danger)
			}
		})
		// Both subpages determine the shared settings size before painting.
		ui.Grid(c).Columns(1).Grow(1).Children(func() {
			for tab := 0; tab < 2; tab++ {
				page := ui.Column(c.Key(tab)).ColumnStart(1).RowStart(1)
				if tab != a.providerSettingsTab {
					page.Invisible()
				}
				page.Children(func() {
					if tab == 1 {
						a.providerConnectionFields(c)
					} else {
						a.providerModelFields(c)
					}
				})
			}
		})
	})
}

func (a *NativeApp) providerModelFields(c *ui.Context) {
	ui.Column(c.Key("provider-models")).Grow(1).MinHeight(0).Gap(em(c, 12)).Children(func() {
		ui.Row(c).Gap(em(c, 16)).AlignItems(ui.Center).Children(func() {
			ui.Form(c, func() {
				ui.Field(c, locales.TranslationModel(), func() { a.providerModelSelect(c) })
			}).Grow(1).Basis(0).MinWidth(0)
			lobeSearchField(c, &a.modelQuery, func() { a.modelList.ScrollTo(0, ui.Start) }, func(frame ui.Element) { frame.Grow(1).Basis(0).MinWidth(0) }).Placeholder(locales.TranslationSearchModels()).Label(locales.TranslationSearchModels())
			iconButton(c, locales.TranslationFetchModels(), "rotate-cw").Disabled(a.modelsLoading || a.settings.ProviderURL == "").OnClick(a.fetchProviderModels)
		})
		a.providerModelsView(c)
		ui.Grid(c).Columns(3).Gap(em(c, 20)).Children(func() {
			info := a.settings.ModelInfo()
			for _, field := range []struct {
				label string
				get   func(*translator.ModelInfo) *int
			}{
				{locales.TranslationMaxInput(), func(m *translator.ModelInfo) *int { return &m.MaxInputTokens }},
				{locales.TranslationMaxOutput(), func(m *translator.ModelInfo) *int { return &m.MaxOutputTokens }},
				{locales.TranslationContextWindow(), func(m *translator.ModelInfo) *int { return &m.ContextWindow }},
			} {
				ui.Field(c, field.label, func() {
					value := float64(0)
					if info != nil {
						value = float64(*field.get(info))
					}
					lobeNumberInput(c, &value, 0, 2147483647, 1, info == nil, func() { *field.get(info) = int(value); info.LimitsSource = "manual"; a.saveSettings() }).Label(field.label)
				})
			}
		})
		muted(c, locales.TranslationLimitsNote()).FontSize(em(c, captionSize))
	})
}

func (a *NativeApp) providerConnectionFields(c *ui.Context) {
	ui.Column(c.Key("provider-connection")).Grow(1).Gap(em(c, 12)).Children(func() {
		ui.Grid(c).Columns(2).GapX(em(c, 20)).GapY(em(c, 16)).AlignItems(ui.Start).Children(func() {
			ui.Field(c, locales.TranslationProviderName(), func() {
				p := a.settings.Provider()
				name := ""
				if p != nil {
					name = p.Name
				}
				lobeTextInput(c, &name).MinHeight(em(c, 36)).Label(locales.TranslationProviderName()).Tooltip(name).Disabled(p == nil).OnChange(func() { p.Name = name; a.saveSettings() })
			})
			ui.Field(c, locales.TranslationRequestLimit(), func() {
				lobeNumberInput(c, &a.settings.MaxRequests, 1, 10000, 1, false, a.saveSettings).Label(locales.TranslationRequestLimit())
			})
		})
		// Inputs reserve the same minimum line height for empty, CJK and Latin
		// values, so adding an account cannot move the shared settings shell.
		// Protocol names are product labels, not user content. Reserve a full
		// row so the selected API never becomes an ambiguous ellipsis.
		ui.Field(c, locales.TranslationApi(), func() {
			idSelect(c, &a.settings.ProviderAPI, []option{{translator.APIChatCompletions, "OpenAI Chat Completions"}, {translator.APIResponses, "OpenAI Responses"}}).Label(locales.TranslationApi()).Disabled(a.settings.Provider() == nil).OnChange(a.saveSettings)
		})
		ui.Field(c, locales.TranslationEndpoint(), func() {
			lobeTextInput(c, &a.settings.ProviderURL).MinHeight(em(c, 36)).Placeholder(locales.TranslationEndpointPlaceholder()).Label(locales.TranslationEndpoint()).Tooltip(a.settings.ProviderURL).Disabled(a.settings.Provider() == nil).OnChange(func() {
				a.providerKey, a.keyStatus, a.modelsError = "", "", ""
				a.providerKeyVisible = false
				a.modelsGeneration++
				a.modelsLoading = false
				if p := a.settings.Provider(); p != nil {
					p.Catalog = nil // invalidate discovery, not retained models or manual limits
				}
				a.saveSettings()
			})
		})
		ui.Field(c, "API Key", func() {
			// A shared native grid cell places the action inside the input.
			// Reserve text padding in this frame, without measuring prior bounds.
			ui.Grid(c).Columns(1).Children(func() {
				t := c.Theme()
				input := lobeTextInput(c.Key(a.settings.CredentialScope()), &a.providerKey).ColumnStart(1).RowStart(1).MinHeight(em(c, 36)).MinWidth(0).Padding(t.Space(1.5), em(c, 40), t.Space(1.5), t.Space(2.5)).Placeholder(locales.TranslationKeyPlaceholder()).Label("API Key").Disabled(a.settings.Provider() == nil)
				if !a.providerKeyVisible {
					input.Password()
				}
				label, glyph := locales.TranslationShowKey(), "eye"
				if a.providerKeyVisible {
					label, glyph = locales.TranslationHideKey(), "eye-off"
				}
				toolIcon(c.Key("key-visibility"), label, glyph).ColumnStart(1).RowStart(1).JustifySelf(ui.End).AlignSelf(ui.Center).Size(em(c, 28), em(c, 28)).Margin(0, em(c, 4)).Padding(0).Background(ui.Transparent).Border(0, ui.Transparent).Disabled(a.settings.Provider() == nil).OnClick(a.toggleProviderKey)
			})
		})
		ui.Row(c).Gap(em(c, 12)).AlignItems(ui.Center).Children(func() {
			muted(c, a.keyStatus).FontSize(em(c, captionSize)).Grow(1)
			lobeButton(c, locales.TranslationDeleteKey()).Disabled(a.settings.Provider() == nil).OnClick(func() {
				if err := security.DeleteKey(a.settings.CredentialScope()); err != nil {
					a.showError("provider-key", err.Error())
					return
				}
				a.clearError("provider-key")
				a.providerKey, a.keyStatus, a.modelsError = "", locales.TranslationKeyDeleted(), ""
				a.providerKeyVisible = false
				a.modelsGeneration++
				a.modelsLoading = false
			})
			lobePrimaryButton(c, locales.TranslationSaveKey()).Disabled(a.providerKey == "" || a.settings.ProviderURL == "").OnClick(func() {
				if err := security.SaveKey(a.settings.CredentialScope(), a.providerKey); err != nil {
					a.showError("provider-key", err.Error())
					return
				}
				a.clearError("provider-key")
				a.providerKey, a.keyStatus, a.modelsError = "", locales.TranslationKeySaved(), ""
				a.providerKeyVisible = false
				a.modelsGeneration++
				a.modelsLoading = false
				a.providerSettingsTab = 0
			})
		})
	})
}

// Retrieve a saved credential only after an explicit reveal. Toggling keeps
// an unsaved draft intact and never writes a key into preferences or logs.
func (a *NativeApp) toggleProviderKey() {
	if !a.providerKeyVisible && a.providerKey == "" {
		key, err := security.ReadKey(a.settings.CredentialScope())
		if err != nil {
			a.showError("provider-key", err.Error())
			return
		}
		a.providerKey = key
	}
	a.clearError("provider-key")
	a.providerKeyVisible = !a.providerKeyVisible
}

// Language rules are independent of service credentials. Giving them their own
// settings page leaves room to actually edit a glossary without crowding keys.
func (a *NativeApp) translationRulesView(c *ui.Context) {
	// The page opens with the same flat group heading as the other pages.
	settingsGroup(c, locales.SettingsTranslationRules(), func() {
		ui.Column(c).Gap(em(c, 24)).Children(func() {
			ui.Field(c, locales.TranslationStyle(), func() {
				// Both actions carry their name: two bare icons beside an
				// empty select did not say what they act on.
				ui.Row(c).Gap(em(c, 8)).Wrap().AlignItems(ui.Center).Children(func() {
					a.styleSelect(c).Grow(1).Basis(em(c, 240)).MinWidth(0)
					ui.Row(c).Gap(em(c, 8)).Shrink(0).Children(func() {
						iconButton(c, locales.TranslationReloadStyles(), "rotate-cw").OnClick(a.reloadTranslationProfiles)
						iconButton(c, locales.TranslationStyleFolder(), "folder-open").OnClick(func() {
							path := filepath.Join(a.dataRoot, "translation-styles")
							if err := os.MkdirAll(path, 0700); err != nil {
								a.showError("style-folder", err.Error())
								return
							}
							if err := mygo.Shell.OpenPath(path); err != nil {
								a.showError("style-folder", err.Error())
							} else {
								a.clearError("style-folder")
							}
						})
					})
				})
			}).Error(a.profilesError)
			ui.Field(c, locales.TranslationGlossary(), func() {
				lobeTextArea(c, &a.settings.Glossary).Height(em(c, 220)).Padding(em(c, 12)).Label(locales.TranslationGlossary()).Placeholder(locales.TranslationGlossaryPlaceholder()).OnChange(a.saveSettings)
			})
		})
	})
}

// styleSelect chooses the translation style: the same choice in the
// settings and where a translation starts.
func (a *NativeApp) styleSelect(c *ui.Context) ui.Element {
	options := []option{}
	for _, p := range a.translationProfiles {
		options = append(options, option{p.ID, profileName(p)})
	}
	return idSelect(c, &a.settings.TranslationProfile, options).Label(locales.TranslationStyle()).OnChange(func() {
		a.profilesError = ""
		a.saveSettings()
	})
}

// skillTools are the subtitle skill's files and its checker worker.
func (a *NativeApp) skillTools() (root, worker string) {
	root = filepath.Join(a.resources, "srt-subtitle-translator")
	if value := os.Getenv("ONESUB_SKILL_ROOT"); value != "" {
		root = value
	}
	worker = filepath.Join(a.resources, "skill-tools", "OneSub.SkillTools.exe")
	if value := os.Getenv("ONESUB_SKILL_WORKER"); value != "" {
		worker = value
	}
	return root, worker
}

// translationFile is where a finished translation is saved: beside the
// subtitles, or the video, as lesson-zh.srt, or in the folder chosen for
// translations. "" when the project has no file to name it after.
func (a *NativeApp) translationFile() string {
	base := a.originalSubtitlePath()
	if base == "" && a.snapshot != nil {
		base = a.snapshot.Manifest.MediaPath
	}
	if base == "" {
		return ""
	}
	dir := a.settings.TranslationDir
	if dir == "" {
		dir = filepath.Dir(base)
	}
	stem := strings.TrimSuffix(filepath.Base(base), filepath.Ext(base)) + "-" + translationSuffix(a.settings.TargetLanguage)
	return filepath.Join(dir, stem+"."+a.trackFormats()[0])
}

// startTranslation translates the whole subtitle file at once, without a
// second confirmation: the service and model are the user's own settings.
// The list shows each subtitle's translation as the model writes it. The
// finished, checked translation becomes the translation track in one
// undoable step and is saved beside its source; one the checker finds
// errors in is kept in the project, unsaved. Checks remain available from
// the subtitle toolbar without interrupting the workbench.
func (a *NativeApp) startTranslation() {
	if a.busy || a.snapshot == nil || !a.commitEdit(true) {
		return
	}
	config := a.translationConfig()
	prefs := a.settings
	task := &translationTask{ProjectID: a.snapshot.Manifest.ID, ProjectPath: a.snapshot.Path, Revision: a.snapshot.Revision, Source: a.snapshot.Document, Config: config, CredentialScope: prefs.CredentialScope(), Target: a.translationFile(), Format: a.trackFormats()[0], SavedHash: a.savedHash, SavedPath: a.savedPath}
	if p := prefs.Provider(); p != nil {
		task.ProviderName = p.Name
	}
	a.executeTranslationTask(task, newTranslationLive())
}

// translationConfig freezes the same service, language and rules for whole-file
// translation and selected retranslation. Keys stay outside the stored config.
func (a *NativeApp) translationConfig() translator.Config {
	a.commitProviderModelInput()
	prefs := a.settings
	root, _ := a.skillTools()
	config := translator.Config{Endpoint: prefs.ProviderURL, API: prefs.ProviderAPI, Model: prefs.ProviderModel, TargetLanguage: prefs.TargetLanguage, SkillRoot: root, Glossary: prefs.Glossary, Domain3D: prefs.Domain3D, ProfileID: prefs.TranslationProfile, ProfilesDir: filepath.Join(a.dataRoot, "translation-styles"), OptimizeTimeline: prefs.OptimizeTimeline, MaxRequests: int(prefs.MaxRequests), CacheDir: filepath.Join(a.dataRoot, "cache", "translation")}
	// A new translation starts fresh, including after clearing the workbench.
	// Continue/recovery retains the saved task config and its cache scope.
	config.CacheScope = uuid.NewString()
	config.Concurrency, config.BatchSize = prefs.TranslationConcurrency, prefs.TranslationBatchSize
	config.Punctuation = prefs.TranslationPunctuation()
	if info := prefs.ModelInfo(); info != nil {
		config.MaxOutputTokens = info.MaxOutputTokens
	}
	return config
}

func (a *NativeApp) executeTranslationTask(task *translationTask, live *translationLive) {
	if a.busy || a.snapshot == nil {
		return
	}
	if !a.commitEdit(true) {
		return
	}
	state := *a.snapshot
	key, err := security.ReadKey(task.CredentialScope)
	if err != nil {
		a.showError(locales.ActionTranslate(), err.Error())
		return
	}
	root, executable := a.skillTools()
	config := task.Config
	config.Key, config.Resume = key, live.resume
	if err = a.rememberTranslationTask(task, live); err != nil {
		a.showError(locales.ActionTranslate(), err.Error())
		return
	}
	baseRequests, baseInput, baseOutput := live.requests, live.input, live.output
	var progressErr error
	var stopTask context.CancelFunc
	config.Observe = func(ev translator.Event) {
		ev.Requests += baseRequests
		ev.Input += baseInput
		ev.Output += baseOutput
		if err := writeTranslationRecord(task.ProjectPath, translationRecord{Event: &ev}, false); err != nil && progressErr == nil {
			progressErr = fmt.Errorf("%s", locales.TranslationProgressWriteError(err.Error()))
			if stopTask != nil {
				stopTask()
			}
		}
		a.win.Update(func() {
			if a.translationLive == live {
				live.apply(ev)
			}
		})
	}
	a.translationLive = live
	a.translationTask = task
	live.stopped, live.model = "", config.Model
	a.changeSubtitleTab(1)
	a.translationRunning = true
	checker := translator.Checker{Executable: executable, SkillRoot: root, Scratch: a.worker.Scratch, Subtitle: a.worker}
	target, format := task.Target, task.Format
	a.run(locales.ActionTranslate(), func(ctx context.Context) (apply func(), runErr error) {
		ctx, stopTask = context.WithCancel(ctx)
		defer stopTask()
		defer func() {
			if runErr != nil {
				note := runErr.Error()
				if errors.Is(runErr, context.Canceled) {
					note = locales.StatusCancelled()
				}
				if err := writeTranslationRecord(task.ProjectPath, translationRecord{Error: note}, false); err != nil {
					runErr = fmt.Errorf("%s; %s", runErr, locales.TranslationProgressWriteError(err.Error()))
				}
			}
		}()
		// Always translate the retained source, not a previous machine translation.
		// Locked hand edits stay in place and never enter the writable candidate set.
		source := task.Source
		source.Cues = append([]subtitle.Cue{}, source.Cues...)
		for i := range source.Cues {
			if !source.Cues[i].TextLocked && !source.Cues[i].ReadOnly && source.Cues[i].Translated {
				source.Cues[i].Text = cueDisplayText(source.Cues[i], 0)
			}
		}
		result, err := translator.Translate(ctx, config, source)
		if progressErr != nil {
			return nil, progressErr
		}
		if writeErr := writeTranslationRecord(task.ProjectPath, translationRecord{Result: &result}, false); writeErr != nil {
			return nil, fmt.Errorf("%s", locales.TranslationProgressWriteError(writeErr.Error()))
		}
		if err != nil {
			return nil, err
		}
		a.win.Update(func() { live.stage = "check" })
		checks, err := checker.Check(ctx, source, result.Document, config.TargetLanguage, !result.Report.Rebuilt)
		if err != nil {
			return nil, err
		}
		candidate := &translationCandidate{ProjectID: state.Manifest.ID, Revision: state.Revision, Result: result, Checks: checks}
		data, err := json.MarshalIndent(candidate, "", "  ")
		if err != nil {
			return nil, err
		}
		artifacts := filepath.Join(state.Path, "artifacts")
		if err = os.MkdirAll(artifacts, 0700); err != nil {
			return nil, err
		}
		if err = subtitle.Publish(filepath.Join(artifacts, "translation-"+uuid.NewString()+".json"), data); err != nil {
			return nil, err
		}
		// The file is written only for a translation without check errors.
		var file []byte
		if len(checks.Errors) == 0 && target != "" {
			if file, err = a.encodeTrack(ctx, result.Document, 1, format); err != nil {
				return nil, err
			}
		}
		return func() { a.applyTranslation(candidate, target, file) }, nil
	})
}

// applyTranslation makes a checked translation the translation track, as one
// undoable transaction, then saves it as a new file: an existing one is
// never overwritten. The store repeats the revision and lock checks.
func (a *NativeApp) applyTranslation(candidate *translationCandidate, target string, file []byte) {
	fail := func(detail string) {
		a.showError(locales.ActionTranslate(), locales.TranslationApplyError(detail))
		a.status = locales.StatusFailed()
		if a.translationLive != nil {
			a.translationLive.stopped = a.errorText
		}
	}
	if a.snapshot == nil || candidate.ProjectID != a.snapshot.Manifest.ID || candidate.Revision != a.snapshot.Revision {
		fail(locales.ErrTranslationStale())
		return
	}
	if _, err := a.store.ApplyTranslationDocument(uuid.NewString(), candidate.Revision, candidate.Result.Document); err != nil {
		fail(err.Error())
		return
	}
	state, err := a.store.Snapshot()
	if err != nil {
		fail(err.Error())
		return
	}
	a.clearError(locales.ActionTranslate())
	a.setSnapshot(state)
	// The accepted project is now visible and editable. Keep its recovery
	// pointer until publishing finishes, in case the output write fails.
	a.translationLive = nil
	a.subtitleTab = 1
	// Keep preview and export on the accepted translation, never an implicit
	// source fallback. Users can explicitly switch to source or bilingual.
	a.settings.SubtitleOutput.Mode = "translation"
	a.saveSettings()
	a.loadDraft()
	a.translationCandidate = candidate
	a.translationCheckTab, a.translationIssueSelected, a.translationIssueQuery = reviewAllFindings, 0, ""
	a.translationIssueList = ui.ListState{}
	report, checks := candidate.Result.Report, candidate.Checks
	if len(checks.Errors) > 0 {
		a.completeTranslationRecovery()
		a.status = locales.TranslationDoneUnsaved(report.Translated, len(checks.Errors))
		return
	}
	if file == nil {
		a.completeTranslationRecovery()
		a.status = locales.TranslationDoneNoFile(report.Translated)
		return
	}
	stem := strings.TrimSuffix(filepath.Base(target), filepath.Ext(target))
	path, err := publishBeside(filepath.Dir(target), stem, strings.TrimPrefix(filepath.Ext(target), "."), file)
	if err != nil {
		a.showError(locales.ActionTranslate(), err.Error())
		a.status = locales.TranslationDoneNoFile(report.Translated)
		return
	}
	a.markSaved(1, path)
	a.completeTranslationRecovery()
	a.lastOutput = path
	a.status = locales.TranslationDone(report.Translated, filepath.Base(path), len(checks.Warnings)+len(report.Warnings))
}
