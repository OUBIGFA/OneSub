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
			ui.Button(c, locales.TranslationAddProvider()).OnClick(func() { a.addProvider(); a.providerSettingsTab = 1 })
			ui.Button(c, locales.TranslationDeleteProvider()).Disabled(a.settings.Provider() == nil).OnClick(func() { a.providerDelete = true })
		})
		ui.Row(c).Gap(em(c, 16)).AlignItems(ui.Center).Children(func() {
			ui.Tabs(c, &a.providerSettingsTab, locales.TranslationModels(), locales.TranslationService()).OnChange(a.commitProviderModelInput)
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
		if a.providerSettingsTab == 1 {
			a.providerConnectionFields(c)
			return
		}
		ui.Column(c.Key("provider-models")).Grow(1).MinHeight(0).Gap(em(c, 12)).Children(func() {
			ui.Row(c).Gap(em(c, 16)).AlignItems(ui.Center).Children(func() {
				ui.Form(c, func() {
					ui.Field(c, locales.TranslationModel(), func() { a.providerModelSelect(c) })
				}).Grow(1).Basis(0).MinWidth(0)
				ui.SearchField(c, &a.modelQuery).Grow(1).Basis(0).MinWidth(0).Placeholder(locales.TranslationSearchModels()).Label(locales.TranslationSearchModels()).OnChange(func() { a.modelList.ScrollTo(0, ui.Start) })
				iconButton(c, locales.TranslationFetchModels(), "rotate-cw").Disabled(a.modelsLoading || a.settings.ProviderURL == "").OnClick(a.fetchProviderModels)
			})
			a.providerModelsView(c)
			ui.Divider(c)
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
						ui.NumberInput(c, &value, 0, 2147483647, 1).Label(field.label).Disabled(info == nil).OnChange(func() { *field.get(info) = int(value); info.LimitsSource = "manual"; a.saveSettings() })
					})
				}
			})
			muted(c, locales.TranslationLimitsNote()).FontSize(em(c, captionSize))
		})
		if a.settings.ProviderURL != a.modelsEndpoint {
			a.fetchProviderModels()
		}
	})
}

func (a *NativeApp) providerConnectionFields(c *ui.Context) {
	ui.Column(c.Key("provider-connection")).Grow(1).Gap(em(c, 20)).Children(func() {
		parameterGrid(c, 200, 3, func() {
			ui.Field(c, locales.TranslationProviderName(), func() {
				p := a.settings.Provider()
				name := ""
				if p != nil {
					name = p.Name
				}
				ui.TextInput(c, &name).Label(locales.TranslationProviderName()).Tooltip(name).Disabled(p == nil).OnChange(func() { p.Name = name; a.saveSettings() })
			})
			ui.Field(c, locales.TranslationApi(), func() {
				idSelect(c, &a.settings.ProviderAPI, []option{{translator.APIChatCompletions, "OpenAI Chat Completions"}, {translator.APIResponses, "OpenAI Responses"}}).Label(locales.TranslationApi()).Disabled(a.settings.Provider() == nil).OnChange(a.saveSettings)
			})
			ui.Field(c, locales.TranslationRequestLimit(), func() {
				ui.NumberInput(c, &a.settings.MaxRequests, 1, 10000, 1).Label(locales.TranslationRequestLimit()).OnChange(a.saveSettings)
			})
		})
		ui.Field(c, locales.TranslationEndpoint(), func() {
			address := ui.TextInput(c, &a.settings.ProviderURL).Placeholder(locales.TranslationEndpointPlaceholder()).Label(locales.TranslationEndpoint()).Tooltip(a.settings.ProviderURL).Disabled(a.settings.Provider() == nil).OnChange(func() {
				a.providerKey, a.keyStatus, a.modelsError = "", "", ""
				a.modelsGeneration++
				a.modelsLoading = false
				if p := a.settings.Provider(); p != nil {
					p.Catalog = nil // invalidate discovery, not retained models or manual limits
				}
				a.saveSettings()
			})
			if !address.Focused() && a.settings.ProviderURL != a.modelsEndpoint {
				a.fetchProviderModels()
			}
		})
		ui.Field(c, "API Key", func() {
			ui.TextInput(c, &a.providerKey).Password().Placeholder(locales.TranslationKeyPlaceholder()).Label("API Key").Disabled(a.settings.Provider() == nil)
		})
		ui.Row(c).Gap(em(c, 12)).AlignItems(ui.Center).Children(func() {
			muted(c, a.keyStatus).FontSize(em(c, captionSize)).Grow(1)
			ui.Button(c, locales.TranslationDeleteKey()).Disabled(a.settings.Provider() == nil).OnClick(func() {
				if err := security.DeleteKey(a.settings.CredentialScope()); err != nil {
					a.showError("provider-key", err.Error())
					return
				}
				a.clearError("provider-key")
				a.providerKey, a.keyStatus, a.modelsEndpoint = "", locales.TranslationKeyDeleted(), ""
				a.modelsGeneration++
				a.modelsLoading = false
			})
			ui.PrimaryButton(c, locales.TranslationSaveKey()).Disabled(a.providerKey == "" || a.settings.ProviderURL == "").OnClick(func() {
				if err := security.SaveKey(a.settings.CredentialScope(), a.providerKey); err != nil {
					a.showError("provider-key", err.Error())
					return
				}
				a.clearError("provider-key")
				a.providerKey, a.keyStatus, a.modelsEndpoint = "", locales.TranslationKeySaved(), ""
				a.modelsGeneration++
				a.modelsLoading = false
				a.providerSettingsTab = 0
				a.fetchProviderModels()
			})
		})
	})
}

// Language rules are independent of service credentials. Giving them their own
// settings page leaves room to actually edit a glossary without crowding keys.
func (a *NativeApp) translationRulesView(c *ui.Context) {
	ui.Column(c).Fill().Gap(em(c, 24)).Children(func() {
		ui.Column(c).Gap(em(c, 24)).Children(func() {
			ui.Field(c, locales.TranslationStyle(), func() {
				ui.Row(c).Gap(em(c, 8)).Children(func() {
					a.styleSelect(c).Grow(1).MinWidth(0)
					ui.Toolbar(c, func() {
						toolIcon(c, locales.TranslationReloadStyles(), "rotate-cw").OnClick(a.reloadTranslationProfiles)
						toolIcon(c, locales.TranslationStyleFolder(), "folder-open").OnClick(func() {
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
					}).Padding(0)
				})
			}).Error(a.profilesError)
			ui.Field(c, locales.TranslationGlossary(), func() {
				ui.TextArea(c, &a.settings.Glossary).Height(em(c, 220)).Padding(em(c, 12)).Label(locales.TranslationGlossary()).Placeholder(locales.TranslationGlossaryPlaceholder()).OnChange(a.saveSettings)
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

// translationView is the check results of the last translation: what the
// subtitle skill's checker found, and every subtitle beside its source.
func (a *NativeApp) translationView(c *ui.Context) {
	fixedDialog(c, &a.translationReview, locales.TranslationResults(), 900, 640, func() {
		ui.Scroll(c).Grow(1).Children(a.translationReviewContent(c))
	}, func() {
		ui.PrimaryButton(c, locales.ActionClose()).OnClick(func() { a.translationReview = false })
	})
}

func (a *NativeApp) translationReviewContent(c *ui.Context) func() {
	return func() {
		candidate := a.translationCandidate
		if candidate == nil {
			return
		}
		report := candidate.Result.Report
		ui.Text(c, locales.ReviewSummary(report.Translated, report.TimelineChanges, len(candidate.Checks.Errors), len(candidate.Checks.Warnings)+len(report.Warnings)))
		ui.Scroll(c).Height(em(c, 130)).Children(func() {
			for _, item := range candidate.Checks.Errors {
				ui.Text(c, item).TextColor(c.Theme().Danger)
			}
			for _, item := range report.Warnings {
				ui.Text(c, item).TextColor(c.Theme().Warning)
			}
			for _, item := range candidate.Checks.Warnings {
				ui.Text(c, item).TextColor(c.Theme().Warning)
			}
			ui.Text(c, locales.ReviewTimelineNote()).TextColor(c.Theme().TextMuted)
		})
		a.translationList.Selected = &a.translationSelected
		cues := candidate.Result.Document.Cues
		ui.Table(c, &a.translationList, []ui.TableColumn{numberColumn(c, len(cues)), {Title: locales.TabSource()}, {Title: locales.TabTranslation()}}, len(cues), func(row, col int) {
			cue := cues[row]
			text := cue.Text
			if col == 0 {
				text = fmt.Sprint(row + 1)
			} else if col == 1 {
				text = subtitle.SourceText(cue)
			}
			ui.Text(c, text).SingleLine()
		}).Height(em(c, 200)).Label(locales.A11yReviewList())
		if a.translationSelected >= 0 && a.translationSelected < len(cues) {
			cue := cues[a.translationSelected]
			ui.Text(c, stamp(float64(cue.StartMs)/1000)+" → "+stamp(float64(cue.EndMs)/1000))
			ui.Text(c, subtitle.SourceText(cue))
			ui.Text(c, cue.Text)
			if cue.SourceRange != nil && cue.SourceRange.Reason != "" {
				ui.Text(c, cue.SourceRange.Reason).TextColor(c.Theme().Warning)
			}
		}
	}
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
// errors in is kept in the project, unsaved, with the errors shown.
func (a *NativeApp) startTranslation() {
	if a.busy || a.snapshot == nil || !a.commitEdit(true) {
		return
	}
	prefs := a.settings
	root, _ := a.skillTools()
	config := translator.Config{Endpoint: prefs.ProviderURL, API: prefs.ProviderAPI, Model: prefs.ProviderModel, TargetLanguage: prefs.TargetLanguage, SkillRoot: root, Glossary: prefs.Glossary, Domain3D: prefs.Domain3D, ProfileID: prefs.TranslationProfile, ProfilesDir: filepath.Join(a.dataRoot, "translation-styles"), OptimizeTimeline: prefs.OptimizeTimeline, MaxRequests: int(prefs.MaxRequests), CacheDir: filepath.Join(a.dataRoot, "cache", "translation")}
	config.Concurrency, config.BatchSize = prefs.TranslationConcurrency, prefs.TranslationBatchSize
	if info := prefs.ModelInfo(); info != nil {
		config.MaxOutputTokens = info.MaxOutputTokens
	}
	task := &translationTask{ProjectID: a.snapshot.Manifest.ID, ProjectPath: a.snapshot.Path, Revision: a.snapshot.Revision, Source: a.snapshot.Document, Config: config, CredentialScope: prefs.CredentialScope(), Target: a.translationFile(), Format: a.trackFormats()[0], SavedHash: a.savedHash, SavedPath: a.savedPath}
	if p := prefs.Provider(); p != nil {
		task.ProviderName = p.Name
	}
	a.executeTranslationTask(task, newTranslationLive())
}

func (a *NativeApp) executeTranslationTask(task *translationTask, live *translationLive) {
	if a.busy || a.snapshot == nil {
		return
	}
	a.commitEdit(true)
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
	a.translationSelected = 0
	report, checks := candidate.Result.Report, candidate.Checks
	if len(checks.Errors) > 0 {
		a.completeTranslationRecovery()
		a.status = locales.TranslationDoneUnsaved(report.Translated, len(checks.Errors))
		a.translationReview = true
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
