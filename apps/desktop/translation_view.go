package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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

func (a *NativeApp) translationSettingsView(c *ui.Context) {
	settingsGroup(c, locales.TranslationService(), func() {
		ui.Form(c, func() {
			// Both protocols are OpenAI's own names, the same in every language.
			ui.Field(c, locales.TranslationApi(), func() {
				idSelect(c, &a.settings.ProviderAPI, []option{{translator.APIChatCompletions, "OpenAI Chat Completions"}, {translator.APIResponses, "OpenAI Responses"}}).Label(locales.TranslationApi()).OnChange(a.saveSettings)
			})
			ui.Field(c, locales.TranslationEndpoint(), func() {
				address := ui.TextInput(c, &a.settings.ProviderURL).Placeholder(locales.TranslationEndpointPlaceholder()).Label(locales.TranslationEndpoint()).OnChange(func() {
					a.providerKey = ""
					a.keyStatus = ""
					a.providerModels = nil
					a.modelsError = ""
					a.saveSettings()
				})
				// Wait until editing ends; do not query partial hostnames as they are typed.
				if a.settingsTab == 2 && !address.Focused() && a.settings.ProviderURL != a.modelsEndpoint {
					a.fetchProviderModels()
				}
			})
			ui.Field(c, locales.TranslationModel(), func() {
				ui.Row(c).Gap(8).Children(func() {
					ui.Combobox(c, &a.settings.ProviderModel, a.providerModels).Placeholder(locales.TranslationModelPlaceholder()).Label(locales.TranslationModel()).Grow(1).OnChange(a.saveSettings)
					ui.Button(c, locales.TranslationFetchModels()).Disabled(a.modelsLoading || a.settings.ProviderURL == "").OnClick(a.fetchProviderModels)
				})
			})
			if a.modelsLoading {
				ui.Text(c, locales.TranslationModelsLoading()).TextColor(c.Theme().TextMuted)
			}
			if a.modelsError != "" {
				ui.Text(c, a.modelsError).TextColor(c.Theme().Warning)
			}
			ui.Field(c, "API Key", func() {
				ui.Row(c).Gap(8).Children(func() {
					ui.TextInput(c, &a.providerKey).Password().Placeholder(locales.TranslationKeyPlaceholder()).Label("API Key").Grow(1)
					ui.Button(c, locales.TranslationSaveKey()).Disabled(a.providerKey == "").OnClick(func() {
						if err := security.SaveKey(a.settings.ProviderURL, a.providerKey); err != nil {
							a.errorText = err.Error()
							return
						}
						a.providerKey = ""
						a.keyStatus = locales.TranslationKeySaved()
						a.modelsEndpoint = ""
						a.fetchProviderModels()
					})
					ui.Button(c, locales.TranslationDeleteKey()).OnClick(func() {
						if err := security.DeleteKey(a.settings.ProviderURL); err != nil {
							a.errorText = err.Error()
							return
						}
						a.providerKey = ""
						a.keyStatus = locales.TranslationKeyDeleted()
					})
				})
			})
			if a.keyStatus != "" {
				ui.Field(c, "", func() { ui.Text(c, a.keyStatus).TextColor(c.Theme().TextMuted) })
			}
		})
	})
	settingsGroup(c, locales.TranslationOptions(), func() {
		// The target language is chosen beside the subtitles, where a
		// translation starts.
		ui.Field(c, locales.TranslationRequestLimit(), func() {
			ui.NumberInput(c, &a.settings.MaxRequests, 1, 10000, 1).Label(locales.TranslationRequestLimit()).AlignSelf(ui.Start).OnChange(a.saveSettings)
		})
	})
	settingsGroup(c, locales.TranslationStyle(), func() {
		options := []option{}
		for _, p := range a.translationProfiles {
			options = append(options, option{p.ID, profileName(p)})
		}
		ui.Row(c).Gap(12).Children(func() {
			idSelect(c, &a.settings.TranslationProfile, options).Label(locales.TranslationStyle()).Grow(1).OnChange(func() {
				a.profilesError = ""
				a.saveSettings()
			})
			ui.Button(c, locales.TranslationReloadStyles()).OnClick(a.reloadTranslationProfiles)
			ui.Button(c, locales.TranslationStyleFolder()).OnClick(func() {
				path := filepath.Join(a.dataRoot, "translation-styles")
				if err := os.MkdirAll(path, 0700); err != nil {
					a.errorText = err.Error()
					return
				}
				if err := mygo.Shell.OpenPath(path); err != nil {
					a.errorText = err.Error()
				}
			})
		})
		if a.profilesError != "" {
			ui.Text(c, a.profilesError).TextColor(c.Theme().Warning)
		}
		ui.Field(c, locales.TranslationGlossary(), func() {
			ui.TextArea(c, &a.settings.Glossary).Height(em(c, 64)).Label(locales.TranslationGlossary()).Placeholder(locales.TranslationGlossaryPlaceholder()).OnChange(a.saveSettings)
		})
	})
}

func (a *NativeApp) translationView(c *ui.Context) {
	ui.Modal(c, &a.translationConfirm, func() {
		ui.Column(c).Width(em(c, 560)).Gap(em(c, 12)).Children(a.translationConfirmContent(c))
	})
	ui.Modal(c, &a.translationReview, func() {
		ui.Column(c).Width(em(c, 760)).Gap(em(c, 12)).Children(a.translationReviewContent(c))
	})
}

// translationConfirmContent says what is sent and to where before anything
// is sent.
func (a *NativeApp) translationConfirmContent(c *ui.Context) func() {
	return func() {
		dialogTitle(c, locales.ActionTranslate())
		ui.Text(c, a.settings.ProviderModel)
		muted(c, a.settings.ProviderURL)
		if a.snapshot != nil {
			total, locked := 0, 0
			for _, cue := range a.snapshot.Document.Cues {
				if cue.ReadOnly || cue.TextLocked {
					locked++
				} else {
					total++
				}
			}
			ui.Text(c, locales.TranslationScope(total, a.settings.TargetLanguage, locked))
		}
		ui.Text(c, locales.TranslationPrivacy())
		if a.settings.OptimizeTimeline {
			ui.Text(c, locales.TranslationTimelineNote()).TextColor(c.Theme().TextMuted)
		}
		ui.Text(c, locales.TranslationCostNote(int(a.settings.MaxRequests))).TextColor(c.Theme().TextMuted)
		ui.Row(c).Gap(em(c, 8)).Justify(ui.End).Margin(em(c, 8), 0, 0, 0).Children(func() {
			ui.Button(c, locales.ActionCancel()).OnClick(func() { a.translationConfirm = false })
			ui.PrimaryButton(c, locales.TranslationStart()).OnClick(func() { a.translationConfirm = false; a.startTranslation() })
		})
	}
}

// translationReviewContent shows the checks, the result beside its source
// and the chosen line in full before the translation is accepted.
func (a *NativeApp) translationReviewContent(c *ui.Context) func() {
	return func() {
		candidate := a.translationCandidate
		if candidate == nil {
			return
		}
		dialogTitle(c, locales.ListReview())
		ui.Text(c, locales.ReviewSummary(candidate.Result.Report.Translated, candidate.Result.Report.TimelineChanges, len(candidate.Checks.Errors), len(candidate.Checks.Warnings)+len(candidate.Result.Report.Warnings)))
		ui.Scroll(c).Height(em(c, 130)).Children(func() {
			for _, item := range candidate.Checks.Errors {
				ui.Text(c, item).TextColor(c.Theme().Danger)
			}
			for _, item := range candidate.Result.Report.Warnings {
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
		ui.Row(c).Gap(em(c, 8)).Justify(ui.End).Children(func() {
			ui.Button(c, locales.ReviewLater()).OnClick(func() { a.translationReview = false })
			ui.PrimaryButton(c, locales.ReviewAccept()).Disabled(len(candidate.Checks.Errors) > 0 || a.busy).OnClick(a.acceptTranslation)
		})
	}
}

func (a *NativeApp) startTranslation() {
	if a.busy || a.snapshot == nil {
		return
	}
	a.commitEdit(true)
	state := *a.snapshot
	prefs := a.settings
	key, err := security.ReadKey(prefs.ProviderURL)
	if err != nil {
		a.errorText = err.Error()
		return
	}
	root := filepath.Join(a.resources, "srt-subtitle-translator")
	if value := os.Getenv("ONESUB_SKILL_ROOT"); value != "" {
		root = value
	}
	executable := filepath.Join(a.resources, "skill-tools", "OneSub.SkillTools.exe")
	if value := os.Getenv("ONESUB_SKILL_WORKER"); value != "" {
		executable = value
	}
	config := translator.Config{Endpoint: prefs.ProviderURL, API: prefs.ProviderAPI, Model: prefs.ProviderModel, Key: key, TargetLanguage: prefs.TargetLanguage, SkillRoot: root, Glossary: prefs.Glossary, Domain3D: prefs.Domain3D, ProfileID: prefs.TranslationProfile, ProfilesDir: filepath.Join(a.dataRoot, "translation-styles"), OptimizeTimeline: prefs.OptimizeTimeline, MaxRequests: int(prefs.MaxRequests)}
	config.Observe = func(p translator.Progress) {
		a.win.Update(func() {
			a.translationProgress = p
			a.translationDraft = append(a.translationDraft, p.Preview...)
		})
	}
	a.translationRunning = true
	a.translationProgress = translator.Progress{Stage: "context"}
	a.translationDraft, a.draftShown, a.draftList = nil, 0, ui.ListState{}
	checker := translator.Checker{Executable: executable, SkillRoot: root, Scratch: a.worker.Scratch, Subtitle: a.worker}
	a.run(locales.ActionTranslate(), func(ctx context.Context) (func(), error) {
		// Always translate the retained source, not a previous machine translation.
		// Locked hand edits stay in place and never enter the writable candidate set.
		source := state.Document
		source.Cues = append([]subtitle.Cue{}, source.Cues...)
		for i := range source.Cues {
			if !source.Cues[i].TextLocked && !source.Cues[i].ReadOnly && source.Cues[i].Translated {
				source.Cues[i].Text = cueDisplayText(source.Cues[i], 0)
			}
		}
		result, err := translator.Translate(ctx, config, source, func(done, total int) {
			a.win.Update(func() { a.status = locales.TranslationProgress(done, total) })
		})
		if err != nil {
			return nil, err
		}
		a.win.Update(func() { a.status = locales.TranslationChecking(); a.translationProgress.Stage = "checking" })
		checks, err := checker.Check(ctx, result.Source, result.Document, prefs.TargetLanguage)
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
		return func() {
			a.translationCandidate = candidate
			a.translationReview = true
			a.translationSelected = 0
			a.status = locales.TranslationAwaitingReview()
		}, nil
	})
}

// Accepting the whole reviewed result is one undoable transaction. The store
// repeats revision/lock checks, so an old review cannot overwrite later edits.
func (a *NativeApp) acceptTranslation() {
	if a.snapshot == nil || a.translationCandidate == nil {
		return
	}
	a.commitEdit(true)
	candidate := a.translationCandidate
	if candidate.ProjectID != a.snapshot.Manifest.ID || candidate.Revision != a.snapshot.Revision {
		a.errorText = locales.ErrTranslationStale()
		return
	}
	if len(candidate.Checks.Errors) > 0 {
		a.errorText = locales.ErrTranslationChecks()
		return
	}
	_, err := a.store.ApplyTranslationDocument(uuid.NewString(), candidate.Revision, candidate.Result.Document)
	if err != nil {
		a.errorText = err.Error()
		return
	}
	state, err := a.store.Snapshot()
	if err != nil {
		a.errorText = err.Error()
		return
	}
	a.setSnapshot(state)
	a.subtitleTab = 1
	// Keep preview and export on the accepted translation, never an implicit
	// source fallback. Users can explicitly switch to source or bilingual.
	a.settings.SubtitleOutput.Mode = "translation"
	a.saveSettings()
	a.loadDraft()
	a.translationReview = false
	a.translationCandidate = nil
	a.errorText = ""
	a.status = locales.TranslationAccepted()
}
